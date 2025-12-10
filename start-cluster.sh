#!/bin/bash
set -e

IMAGE_NAME="agenda-distribuida:latest"
NETWORK_NAME="agenda_net"
CLUSTER_SECRET="5c1d0c7f1d6a0c8a2e9f0b3a4c5d6e7f8a9b0c1d2e3f4a5b6c7d8e9f0a1b2c3"
AUDIT_TOKEN="mi-token-auditoria-123"

# IP del host vista desde los contenedores. Para entornos multi-host de 7 nodos,
# esta IP deberá ajustarse explícitamente por máquina si hostname -I no es fiable.
HOST_IP="$(hostname -I | awk '{print $1}')"

echo "🚀 Iniciando cluster manual con red Swarm overlay attachable"

# 1. Asegurar que Swarm está inicializado
if ! docker info --format '{{.Swarm.LocalNodeState}}' | grep -q "active"; then
    echo "🟡 Swarm no está activo. Inicializando..."
    docker swarm init
fi

# 2. Crear red overlay attachable (Swarm)
if ! docker network ls | grep -q "${NETWORK_NAME}"; then
    echo "📡 Creando red overlay attachable: ${NETWORK_NAME}"
    docker network create \
        --driver=overlay \
        --attachable \
        "${NETWORK_NAME}"
else
    echo "ℹ️  Red ${NETWORK_NAME} ya existe"
fi

# 3. Limpiar nodos previos
echo "🧹 Eliminando contenedores previos..."
docker rm -f agenda-1 agenda-2 agenda-3 agenda-4 2>/dev/null || true

# 4. Crear logs
mkdir -p logs

# 5. PEERS / DISCOVERY_SEEDS basados en IP:puerto publicado del host
# Esto permite que el consenso y el discovery sigan funcionando incluso si falla
# el DNS interno de Docker para los nombres de contenedor.
PEERS_IP="${HOST_IP}:8080,${HOST_IP}:8082,${HOST_IP}:8083,${HOST_IP}:8084"

start_node () {
    local NAME=$1
    local PORT=$2

    echo "  → Levantando $NAME (puerto $PORT)..."

    docker run -d \
        --name "$NAME" \
        --hostname "$NAME" \
        --network "$NETWORK_NAME" \
        -p "$PORT:8080" \
        -v "$(pwd)/logs:/logs" \
        -e NODE_ID="$NAME" \
        -e ADVERTISE_ADDR="${HOST_IP}:$PORT" \
        -e HTTP_ADDR="0.0.0.0:8080" \
        -e DATABASE_DSN="file:agenda.db?cache=shared&_fk=1" \
        -e LOG_LEVEL="info" \
        -e LOG_FORMAT="json" \
        -e LOG_DEST="file:/logs/$NAME.log" \
        -e CLUSTER_HMAC_SECRET="$CLUSTER_SECRET" \
        -e AUDIT_API_TOKEN="$AUDIT_TOKEN" \
        -e DISCOVERY_SEEDS="$PEERS_IP" \
        "${IMAGE_NAME}"
}

start_node agenda-1 8080
start_node agenda-2 8082
start_node agenda-3 8083
start_node agenda-4 8084

echo "⏳ Esperando a que arranquen..."
sleep 5

echo "🏥 Verificando nodos:"
for i in 1 2 3 4; do
    docker exec agenda-$i curl -sf http://localhost:8080/raft/health \
        && echo "  agenda-$i OK" \
        || echo "  agenda-$i 🔥 FALLA"
done

echo ""
echo "🎯 Cluster iniciado sin load-balancer, con red overlay Swarm"
echo "👉 Líder visible en http://localhost:8080/raft/status"

