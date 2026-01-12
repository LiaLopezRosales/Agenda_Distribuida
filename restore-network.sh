#!/bin/bash
#
# Script para restaurar completamente la red a su estado normal
# Reunifica todo y limpia todas las reglas de partición
#

set -e

GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
RED='\033[0;31m'
NC='\033[0m'

if [ "$EUID" -ne 0 ]; then
    echo -e "${RED}Error: Este script requiere permisos de root (sudo)${NC}"
    exit 1
fi

GROUP1_CONTAINERS=("agenda-1" "agenda-2" "agenda-3" "agenda-4")
GROUP2_CONTAINERS=("agenda-5" "agenda-6" "agenda-7")
NETWORK_NAME="${NETWORK_NAME:-agenda_net}"

echo -e "${BLUE}=== Restaurando red a estado normal ===${NC}\n"

# 1. Eliminar todas las reglas de iptables
echo -e "${YELLOW}1. Eliminando reglas de iptables...${NC}"
deleted=0

# Eliminar de todas las cadenas
chains=("OUTPUT" "INPUT" "FORWARD" "DOCKER-USER")
for chain in "${chains[@]}"; do
    while iptables -L "$chain" -n --line-numbers 2>/dev/null | grep -q "AGENDA_PARTITION"; do
        line_num=$(iptables -L "$chain" -n --line-numbers 2>/dev/null | grep "AGENDA_PARTITION" | head -n 1 | awk '{print $1}')
        if [ -n "$line_num" ]; then
            iptables -D "$chain" "$line_num" 2>/dev/null || break
            ((deleted++))
        else
            break
        fi
    done
done

# Eliminar de NAT
if iptables -t nat -L PREROUTING -n --line-numbers 2>/dev/null | grep -q "AGENDA_PARTITION"; then
    while iptables -t nat -L PREROUTING -n --line-numbers 2>/dev/null | grep -q "AGENDA_PARTITION"; do
        line_num=$(iptables -t nat -L PREROUTING -n --line-numbers 2>/dev/null | grep "AGENDA_PARTITION" | head -n 1 | awk '{print $1}')
        if [ -n "$line_num" ]; then
            iptables -t nat -D PREROUTING "$line_num" 2>/dev/null || break
            ((deleted++))
        else
            break
        fi
    done
fi

if [ $deleted -gt 0 ]; then
    echo -e "${GREEN}  ✓${NC} Eliminadas $deleted reglas de iptables"
else
    echo -e "${YELLOW}  ⚠${NC} No se encontraron reglas de iptables para eliminar"
fi

# 2. Desconectar de redes secundarias y reconectar a red principal
echo -e "\n${YELLOW}2. Reconectando contenedores a la red principal...${NC}"

all_containers=("${GROUP1_CONTAINERS[@]}" "${GROUP2_CONTAINERS[@]}")

for container in "${all_containers[@]}"; do
    if docker ps --format '{{.Names}}' | grep -q "^${container}$"; then
        # Desconectar de redes secundarias
        docker network disconnect "agenda_group1" "$container" 2>/dev/null || true
        docker network disconnect "agenda_group2" "$container" 2>/dev/null || true
        
        # Reconectar a red principal (si no está ya conectado)
        if ! docker inspect "$container" --format '{{range $net, $conf := .NetworkSettings.Networks}}{{$net}} {{end}}' 2>/dev/null | grep -q "$NETWORK_NAME"; then
            docker network connect "$NETWORK_NAME" "$container" 2>/dev/null || true
            echo -e "${GREEN}  ✓${NC} $container reconectado a $NETWORK_NAME"
        else
            echo -e "${YELLOW}  ✓${NC} $container ya está en $NETWORK_NAME"
        fi
    else
        echo -e "${RED}  ✗${NC} $container no está corriendo"
    fi
done

# 3. Verificar que todos estén en la red principal
echo -e "\n${YELLOW}3. Verificando estado final...${NC}"

all_in_main_net=true
for container in "${all_containers[@]}"; do
    if docker ps --format '{{.Names}}' | grep -q "^${container}$"; then
        networks=$(docker inspect "$container" --format '{{range $net, $conf := .NetworkSettings.Networks}}{{$net}} {{end}}' 2>/dev/null || echo "")
        if echo "$networks" | grep -q "$NETWORK_NAME"; then
            # Verificar que NO esté en redes secundarias
            if echo "$networks" | grep -qE "(agenda_group1|agenda_group2)"; then
                echo -e "${YELLOW}  ⚠${NC} $container todavía está en una red secundaria"
                all_in_main_net=false
            else
                echo -e "${GREEN}  ✓${NC} $container está solo en $NETWORK_NAME"
            fi
        else
            echo -e "${RED}  ✗${NC} $container NO está en $NETWORK_NAME"
            all_in_main_net=false
        fi
    fi
done

echo -e "\n${BLUE}=== Resumen ===${NC}\n"

if [ "$all_in_main_net" = true ]; then
    echo -e "${GREEN}✓ Red restaurada exitosamente${NC}"
    echo -e "  - Todas las reglas de iptables eliminadas"
    echo -e "  - Todos los contenedores en la red principal ($NETWORK_NAME)"
    echo -e "  - Redes secundarias desconectadas"
    echo -e "\n${YELLOW}Espera unos segundos para que los nodos se redescubran...${NC}"
else
    echo -e "${YELLOW}⚠ Algunos contenedores pueden necesitar atención manual${NC}"
fi

echo ""
echo -e "${BLUE}Para verificar el estado:${NC}"
echo -e "  ./check-partition.sh"
echo -e "  curl http://localhost:8080/raft/health | jq"



