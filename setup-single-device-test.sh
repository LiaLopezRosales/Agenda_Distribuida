#!/bin/bash
#
# Script para configurar un cluster de 7 nodos en un solo dispositivo
# para facilitar las pruebas de partición de red
#

set -e

# Colores
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
RED='\033[0;31m'
NC='\033[0m'

# Configuración
IMAGE_NAME="${IMAGE_NAME:-agenda-distribuida:latest}"
NETWORK_NAME="${NETWORK_NAME:-agenda_net}"
CLUSTER_SECRET="${CLUSTER_SECRET:-5c1d0c7f1d6a0c8a2e9f0b3a4c5d6e7f8a9b0c1d2e3f4a5b6c7d8e9f0a1b2c3}"
AUDIT_TOKEN="${AUDIT_TOKEN:-mi-token-auditoria-123}"

# Obtener la IP local del host (no localhost)
get_local_ip() {
    # Intenta obtener la IP de la interfaz principal
    local ip=$(hostname -I | awk '{print $1}' 2>/dev/null || echo "")
    if [ -z "$ip" ]; then
        # Fallback: obtener IP de la primera interfaz que no sea loopback
        ip=$(ip route get 8.8.8.8 2>/dev/null | grep -oP 'src \K\S+' || echo "127.0.0.1")
    fi
    echo "$ip"
}

LOCAL_IP=$(get_local_ip)

echo -e "${BLUE}=== Configuración de Cluster en Un Solo Dispositivo ===${NC}\n"
echo -e "IP local detectada: ${GREEN}$LOCAL_IP${NC}"
echo -e "Red Docker: ${GREEN}$NETWORK_NAME${NC}"
echo ""

# Crear red si no existe
if ! docker network ls | grep -q "$NETWORK_NAME"; then
    echo -e "${YELLOW}Creando red Docker: $NETWORK_NAME${NC}"
    docker network create "$NETWORK_NAME" || true
else
    echo -e "${GREEN}Red $NETWORK_NAME ya existe${NC}"
fi

# Crear directorio de logs
mkdir -p logs

# Definir direcciones para todos los nodos (usando localhost para accesibilidad)
# Pero ADVERTISE_ADDR usa la IP real para que funcione correctamente
PEERS_LIST=(
    "$LOCAL_IP:8080"
    "$LOCAL_IP:8082"
    "$LOCAL_IP:8083"
    "$LOCAL_IP:8084"
    "$LOCAL_IP:18080"
    "$LOCAL_IP:18082"
    "$LOCAL_IP:18083"
    "agenda-1:8080"
    "agenda-2:8080"
    "agenda-3:8080"
    "agenda-4:8080"
    "agenda-5:8080"
    "agenda-6:8080"
    "agenda-7:8080"
)

DISCOVERY_SEEDS=$(IFS=,; echo "${PEERS_LIST[*]}")

echo -e "${BLUE}Configurando nodos...${NC}\n"

# Función para crear un nodo
create_node() {
    local name=$1
    local node_id=$2
    local host_port=$3
    local advertise_addr=$4
    
    # Detener y eliminar si existe
    docker rm -f "$name" 2>/dev/null || true
    
    echo -e "${YELLOW}Creando $name...${NC}"
    docker run -d \
        --name "$name" \
        --hostname "$name" \
        --network "$NETWORK_NAME" \
        -p "$host_port:8080" \
        -v "$(pwd)/logs:/logs" \
        -e NODE_ID="$node_id" \
        -e ADVERTISE_ADDR="$advertise_addr" \
        -e HTTP_ADDR="0.0.0.0:8080" \
        -e DATABASE_DSN="file:agenda.db?cache=shared&_fk=1" \
        -e LOG_LEVEL="info" \
        -e LOG_FORMAT="json" \
        -e LOG_DEST="file:/logs/${name}.log" \
        -e CLUSTER_HMAC_SECRET="$CLUSTER_SECRET" \
        -e AUDIT_API_TOKEN="$AUDIT_TOKEN" \
        -e DISCOVERY_SEEDS="$DISCOVERY_SEEDS" \
        -e PEERS="$PEERS_LIST" \
        "$IMAGE_NAME"
    
    if [ $? -eq 0 ]; then
        echo -e "${GREEN}✓ $name creado (puerto $host_port)${NC}"
    else
        echo -e "${RED}✗ Error creando $name${NC}"
        return 1
    fi
}

# Crear Grupo 1 (nodos 1-4)
echo -e "${BLUE}--- Grupo 1 (4 nodos) ---${NC}"
create_node "agenda-1" "agenda-1" "8080" "$LOCAL_IP:8080"
sleep 1
create_node "agenda-2" "agenda-2" "8082" "$LOCAL_IP:8082"
sleep 1
create_node "agenda-3" "agenda-3" "8083" "$LOCAL_IP:8083"
sleep 1
create_node "agenda-4" "agenda-4" "8084" "$LOCAL_IP:8084"
sleep 2

# Crear Grupo 2 (nodos 5-7)
echo -e "\n${BLUE}--- Grupo 2 (3 nodos) ---${NC}"
create_node "agenda-5" "agenda-5" "18080" "$LOCAL_IP:18080"
sleep 1
create_node "agenda-6" "agenda-6" "18082" "$LOCAL_IP:18082"
sleep 1
create_node "agenda-7" "agenda-7" "18083" "$LOCAL_IP:18083"

echo -e "\n${GREEN}=== Configuración completada ===${NC}\n"

# Esperar a que los nodos inicien
echo -e "${YELLOW}Esperando a que los nodos inicien (10 segundos)...${NC}"
sleep 10

# Verificar estado
echo -e "\n${BLUE}=== Verificando estado de los nodos ===${NC}\n"
for port in 8080 8082 8083 8084 18080 18082 18083; do
    node_name=$(echo "$port" | sed 's/18080/agenda-5/; s/18082/agenda-6/; s/18083/agenda-7/; s/8080/agenda-1/; s/8082/agenda-2/; s/8083/agenda-3/; s/8084/agenda-4/')
    if curl -s --max-time 2 "http://localhost:$port/raft/health" > /dev/null 2>&1; then
        health=$(curl -s --max-time 2 "http://localhost:$port/raft/health" 2>/dev/null | jq -r '.node_id // "error"' || echo "error")
        echo -e "${GREEN}✓${NC} $node_name (localhost:$port) - $health"
    else
        echo -e "${RED}✗${NC} $node_name (localhost:$port) - no responde"
    fi
done

echo -e "\n${BLUE}=== Comandos útiles ===${NC}"
echo -e "Ver estado de partición:   ${GREEN}sudo ./simulate-network-partition.sh status${NC}"
echo -e "Crear partición:           ${GREEN}sudo ./simulate-network-partition.sh partition${NC}"
echo -e "Reunificar:                ${GREEN}sudo ./simulate-network-partition.sh reunify${NC}"
echo -e "Ver logs de un nodo:       ${GREEN}docker logs -f agenda-1${NC}"
echo -e "Ver estado Raft:           ${GREEN}curl http://localhost:8080/raft/health | jq${NC}"
echo ""



