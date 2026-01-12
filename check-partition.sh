#!/bin/bash
#
# Script helper para verificar el estado de comunicación entre nodos
# Útil para validar que la partición funciona correctamente
#

set -e

# Colores
GREEN='\033[0;32m'
RED='\033[0;31m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m'

# Grupos
GROUP1=("agenda-1" "agenda-2" "agenda-3" "agenda-4")
GROUP2=("agenda-5" "agenda-6" "agenda-7")

# Función para obtener IP de un contenedor
get_container_ip() {
    docker inspect -f '{{range.NetworkSettings.Networks}}{{.IPAddress}}{{end}}' "$1" 2>/dev/null || echo ""
}

# Función para probar comunicación
test_connection() {
    local from_container=$1
    local to_container=$2
    local to_ip=$3
    
    local result=$(docker exec "$from_container" curl -s --max-time 2 "http://$to_ip:8080/raft/health" 2>&1)
    
    if echo "$result" | grep -q "node_id\|is_leader"; then
        echo -e "${GREEN}✓${NC}"
        return 0
    else
        echo -e "${RED}✗${NC}"
        return 1
    fi
}

echo -e "${BLUE}=== Verificación de Comunicación entre Nodos ===${NC}\n"

# Obtener IPs de todos los contenedores
declare -A container_ips

echo -e "${YELLOW}Obteniendo IPs de contenedores...${NC}\n"

all_containers=("${GROUP1[@]}" "${GROUP2[@]}")
for container in "${all_containers[@]}"; do
    ip=$(get_container_ip "$container")
    if [ -n "$ip" ]; then
        container_ips["$container"]=$ip
        echo -e "  ${GREEN}$container${NC} -> $ip"
    else
        echo -e "  ${RED}$container${NC} -> no encontrado"
        exit 1
    fi
done

echo -e "\n${BLUE}=== Matriz de Comunicación ===${NC}\n"
echo -e "Probando comunicaciones HTTP entre nodos...\n"

# Crear matriz de comunicación
echo -e "${YELLOW}Matriz: fila = origen, columna = destino${NC}"
echo -e "${YELLOW}Grupo 1: ${GROUP1[0]} ${GROUP1[1]} ${GROUP1[2]} ${GROUP1[3]} | Grupo 2: ${GROUP2[0]} ${GROUP2[1]} ${GROUP2[2]}${NC}"
echo ""

# Encabezado
printf "%-10s" "DESDE →"
for to in "${all_containers[@]}"; do
    printf "%8s" "${to#agenda-}"
done
echo ""

# Separador
printf "%-10s" "─────────"
for to in "${all_containers[@]}"; do
    printf "%8s" "────"
done
echo ""

# Filas de la matriz
for from in "${all_containers[@]}"; do
    printf "%-10s" "${from}"
    
    for to in "${all_containers[@]}"; do
        if [ "$from" == "$to" ]; then
            printf "%8s" "-"
        else
            to_ip="${container_ips[$to]}"
            if test_connection "$from" "$to" "$to_ip" > /dev/null 2>&1; then
                printf "%8s" "✓"
            else
                printf "%8s" "✗"
            fi
        fi
    done
    echo ""
done

echo -e "\n${BLUE}=== Análisis ===${NC}\n"

# Verificar si hay partición
group1_to_group2_ok=0
group1_to_group2_total=0
group2_to_group1_ok=0
group2_to_group1_total=0

for from in "${GROUP1[@]}"; do
    for to in "${GROUP2[@]}"; do
        ((group1_to_group2_total++))
        to_ip="${container_ips[$to]}"
        if test_connection "$from" "$to" "$to_ip" > /dev/null 2>&1; then
            ((group1_to_group2_ok++))
        fi
    done
done

for from in "${GROUP2[@]}"; do
    for to in "${GROUP1[@]}"; do
        ((group2_to_group1_total++))
        to_ip="${container_ips[$to]}"
        if test_connection "$from" "$to" "$to_ip" > /dev/null 2>&1; then
            ((group2_to_group1_ok++))
        fi
    done
done

total_between_groups=$((group1_to_group2_ok + group2_to_group1_ok))
total_tests=$((group1_to_group2_total + group2_to_group1_total))

if [ $total_between_groups -eq 0 ]; then
    echo -e "${RED}✗ RED PARTICIONADA${NC}"
    echo -e "  ${RED}No hay comunicación entre Grupo 1 y Grupo 2${NC}"
    echo -e "  ${YELLOW}Comportamiento esperado:${NC}"
    echo -e "    - Grupo 1 (agenda-1..4) puede comunicarse entre sí"
    echo -e "    - Grupo 2 (agenda-5..7) puede comunicarse entre sí"
    echo -e "    - NO hay comunicación entre grupos"
    echo -e "  ${BLUE}Bloqueadas: $total_tests conexiones${NC}"
elif [ $total_between_groups -eq $total_tests ]; then
    echo -e "${GREEN}✓ RED REUNIFICADA${NC}"
    echo -e "  ${GREEN}Comunicación completa entre todos los nodos${NC}"
    echo -e "  ${YELLOW}Estado normal:${NC} Todos los nodos pueden comunicarse libremente"
    echo -e "  ${BLUE}Exitosas: $total_between_groups/$total_tests conexiones entre grupos${NC}"
else
    echo -e "${YELLOW}⚠ ESTADO PARCIAL${NC}"
    echo -e "  ${YELLOW}Algunas conexiones funcionan, otras no${NC}"
    echo -e "  ${BLUE}Funcionan: $total_between_groups/$total_tests conexiones entre grupos${NC}"
    echo -e "  ${RED}Problema:${NC} Puede haber un problema con las reglas de iptables"
    echo -e "  ${YELLOW}Recomendación:${NC} Ejecuta 'sudo ./simulate-network-partition.sh clear' y vuelve a intentar"
fi

echo ""
echo -e "${BLUE}=== Estado de Líderes ===${NC}\n"

for port in 8080 8082 8083 8084 18080 18082 18083; do
    node_name=$(echo "$port" | sed 's/18080/agenda-5/; s/18082/agenda-6/; s/18083/agenda-7/; s/8080/agenda-1/; s/8082/agenda-2/; s/8083/agenda-3/; s/8084/agenda-4/')
    health=$(curl -s --max-time 2 "http://localhost:$port/raft/health" 2>/dev/null || echo "")
    
    if [ -n "$health" ]; then
        node_id=$(echo "$health" | jq -r '.node_id // "unknown"' 2>/dev/null || echo "unknown")
        is_leader=$(echo "$health" | jq -r '.is_leader // false' 2>/dev/null || echo "false")
        leader=$(echo "$health" | jq -r '.leader // "unknown"' 2>/dev/null || echo "unknown")
        
        if [ "$is_leader" = "true" ]; then
            echo -e "${GREEN}✓${NC} $node_name ($node_id) - ${YELLOW}LÍDER${NC}"
        else
            echo -e "  $node_name ($node_id) - líder: $leader"
        fi
    else
        echo -e "${RED}✗${NC} $node_name - no responde"
    fi
done

echo ""

