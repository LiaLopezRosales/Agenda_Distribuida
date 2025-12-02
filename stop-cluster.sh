#!/bin/bash

# Script para detener el cluster manual

set -e

RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m'

echo -e "${YELLOW}🛑 Deteniendo cluster...${NC}"

# Detener contenedores
for i in 1 2 3 4; do
    if docker ps --format '{{.Names}}' | grep -q "^agenda-${i}$"; then
        echo -e "  ${YELLOW}→${NC} Deteniendo agenda-${i}..."
        docker stop "agenda-${i}"
    fi
done

# Eliminar contenedores
for i in 1 2 3 4; do
    if docker ps -a --format '{{.Names}}' | grep -q "^agenda-${i}$"; then
        echo -e "  ${YELLOW}→${NC} Eliminando agenda-${i}..."
        docker rm "agenda-${i}"
    fi
done

echo -e "${GREEN}✅ Cluster detenido${NC}"
echo ""
echo -e "${YELLOW}ℹ️  Nota: Los contenedores se han eliminado, pero:${NC}"
echo "  - Los logs se mantienen en ./logs/"
echo "  - La red 'agenda_net' se mantiene (puedes eliminarla con: docker network rm agenda_net)"
echo ""



