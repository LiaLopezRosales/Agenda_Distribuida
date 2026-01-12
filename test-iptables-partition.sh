#!/bin/bash
#
# Script de prueba para verificar que iptables funciona con Docker
# Ejecutar antes de usar simulate-network-partition.sh
#

set -e

GREEN='\033[0;32m'
RED='\033[0;31m'
YELLOW='\033[1;33m'
NC='\033[0m'

if [ "$EUID" -ne 0 ]; then
    echo -e "${RED}Error: Este script requiere permisos de root (sudo)${NC}"
    exit 1
fi

echo -e "${YELLOW}=== Verificando configuración de iptables para Docker ===${NC}\n"

# Verificar que existen las cadenas necesarias
echo -e "${YELLOW}Verificando cadenas de iptables...${NC}"
if iptables -L DOCKER-USER >/dev/null 2>&1; then
    echo -e "${GREEN}✓ Cadena DOCKER-USER existe${NC}"
else
    echo -e "${RED}✗ Cadena DOCKER-USER no existe${NC}"
    echo -e "  Docker puede no estar configurado correctamente"
fi

if iptables -L FORWARD >/dev/null 2>&1; then
    echo -e "${GREEN}✓ Cadena FORWARD existe${NC}"
else
    echo -e "${RED}✗ Cadena FORWARD no existe${NC}"
    exit 1
fi

# Verificar red Docker
echo -e "\n${YELLOW}Verificando red Docker...${NC}"
if docker network inspect agenda_net >/dev/null 2>&1; then
    echo -e "${GREEN}✓ Red agenda_net existe${NC}"
    driver=$(docker network inspect agenda_net --format '{{.Driver}}')
    echo -e "  Driver: $driver"
else
    echo -e "${YELLOW}⚠ Red agenda_net no existe${NC}"
    echo -e "  Esto puede estar bien si usas otra red"
fi

# Verificar contenedores
echo -e "\n${YELLOW}Verificando contenedores...${NC}"
containers=("agenda-1" "agenda-2" "agenda-5" "agenda-6")
all_ok=true

for container in "${containers[@]}"; do
    if docker ps --format '{{.Names}}' | grep -q "^${container}$"; then
        ip=$(docker inspect -f '{{range.NetworkSettings.Networks}}{{.IPAddress}}{{end}}' "$container" 2>/dev/null || echo "")
        if [ -n "$ip" ]; then
            echo -e "${GREEN}✓${NC} $container -> $ip"
        else
            echo -e "${RED}✗${NC} $container existe pero no tiene IP"
            all_ok=false
        fi
    else
        echo -e "${YELLOW}⚠${NC} $container no está corriendo"
    fi
done

# Test de regla iptables
echo -e "\n${YELLOW}Probando creación de regla de prueba...${NC}"
test_ip1="172.19.0.2"
test_ip2="172.19.0.6"

if iptables -C DOCKER-USER -s "$test_ip1" -d "$test_ip2" -j DROP -m comment --comment "TEST_PARTITION" 2>/dev/null; then
    echo -e "${YELLOW}⚠ Regla de prueba ya existe, eliminándola...${NC}"
    iptables -D DOCKER-USER -s "$test_ip1" -d "$test_ip2" -j DROP -m comment --comment "TEST_PARTITION" 2>/dev/null || true
fi

if iptables -I DOCKER-USER 1 -s "$test_ip1" -d "$test_ip2" -j DROP -m comment --comment "TEST_PARTITION" 2>/dev/null; then
    echo -e "${GREEN}✓ Regla de prueba creada exitosamente${NC}"
    
    # Verificar que existe
    if iptables -L DOCKER-USER -n | grep -q "TEST_PARTITION"; then
        echo -e "${GREEN}✓ Regla verificada en iptables${NC}"
    else
        echo -e "${RED}✗ Regla no encontrada en iptables${NC}"
        all_ok=false
    fi
    
    # Eliminar regla de prueba
    iptables -D DOCKER-USER -s "$test_ip1" -d "$test_ip2" -j DROP -m comment --comment "TEST_PARTITION" 2>/dev/null || true
    echo -e "${GREEN}✓ Regla de prueba eliminada${NC}"
else
    echo -e "${RED}✗ No se pudo crear regla de prueba${NC}"
    echo -e "  Esto indica que iptables no está funcionando correctamente"
    all_ok=false
fi

echo -e "\n${YELLOW}=== Resumen ===${NC}\n"

if [ "$all_ok" = true ]; then
    echo -e "${GREEN}✓ Sistema listo para simular particiones${NC}"
    echo -e "\nPuedes ejecutar: sudo ./simulate-network-partition.sh partition"
else
    echo -e "${RED}✗ Hay problemas con la configuración${NC}"
    echo -e "\n${YELLOW}Posibles soluciones:${NC}"
    echo -e "1. Verifica que Docker esté ejecutándose"
    echo -e "2. Verifica que los contenedores estén corriendo"
    echo -e "3. Verifica permisos de sudo"
    echo -e "4. Si usas Docker Swarm, puede ser necesario otro método"
    exit 1
fi



