# Pruebas progresivas (comandos actuales)

## Reiniciar Docker

sudo systemctl stop docker.socket
sudo systemctl stop docker
sudo systemctl stop containerd
sudo iptables -F
sudo iptables -t nat -F
sudo systemctl start containerd
sudo systemctl start docker
sudo systemctl start docker.socket
docker network rm agenda_net 2>/dev/null || true
docker network rm ingress 2>/dev/null || true
docker network rm docker_gwbridge 2>/dev/null || true

## Setup del entorno del cluster

scp agenda-distribuida_latest.tar user@${IP_B}:/tmp/

docker rm -f agenda-1 agenda-2 agenda-3 agenda-4 agenda-5 agenda-6 agenda-7 2>/dev/null || true
rm -f logs/*.log
docker swarm init
 docker network ls --format '{{.Name}}' | grep -qx 'agenda_net' ||   docker network create --driver=overlay --attachable agenda_net
 mkdir -p logs
 export IMAGE_NAME="agenda-distribuida:latest"
 export NETWORK_NAME="agenda_net"
export CLUSTER_SECRET="5c1d0c7f1d6a0c8a2e9f0b3a4c5d6e7f8a9b0c1d2e3f4a5b6c7d8e9f0a1b2c3"
export AUDIT_TOKEN="mi-token-auditoria-123"
export IP_A="10.6.121.130" 
export IP_B="10.6.121.214"
export PEERS_IP="${IP_A}:8080,${IP_A}:8082,${IP_A}:8083,${IP_A}:8084,${IP_B}:18080,${IP_B}:18082,${IP_B}:18083"

export PEERS_IP="${IP_A}:8080,${IP_A}:8082,${IP_A}:8083,${IP_A}:8084,${IP_B}:18080,${IP_B}:18082,${IP_B}:18083,agenda-1:8080,agenda-2:8080,agenda-3:8080,agenda-4:8080,agenda-5:8080,agenda-6:8080,agenda-7:8080"

## Crear nodos
### En A
docker run -d   --name "agenda-1"   --hostname "agenda-1"   --network "$NETWORK_NAME"   -p "8080:8080"   -v "$(pwd)/logs:/logs"   -e NODE_ID="agenda-1"   -e ADVERTISE_ADDR="${IP_A}:8080"   -e HTTP_ADDR="0.0.0.0:8080"   -e DATABASE_DSN="file:agenda.db?cache=shared&_fk=1"   -e LOG_LEVEL="info"   -e LOG_FORMAT="json"   -e LOG_DEST="file:/logs/agenda-1.log"   -e CLUSTER_HMAC_SECRET="$CLUSTER_SECRET"   -e AUDIT_API_TOKEN="$AUDIT_TOKEN"   -e DISCOVERY_SEEDS="$PEERS_IP"   "$IMAGE_NAME"

docker run -d   --name "agenda-2"   --hostname "agenda
-2"   --network "$NETWORK_NAME"   -p "8082:8080"   -v "$(pwd)/logs:/logs"   -e NODE_ID="agenda-2"   -e ADVERTISE_ADDR="${IP_A}:
8082"   -e HTTP_ADDR="0.0.0.0:8080"   -e DATABASE_DSN="file:agenda.db?cache=shared&_fk=1"   -e LOG_LEVEL="info"   -e LOG_FORMAT
="json"   -e LOG_DEST="file:/logs/agenda-2.log"   -e CLUSTER_HMAC_SECRET="$CLUSTER_SECRET"   -e AUDIT_API_TOKEN="$AUDIT_TOKEN" 
  -e DISCOVERY_SEEDS="$PEERS_IP"   "$IMAGE_NAME"

docker run -d   --name "agenda-3"   --hostname "agenda
-3"   --network "$NETWORK_NAME"   -p "8083:8080"   -v "$(pwd)/logs:/logs"   -e NODE_ID="agenda-3"   -e ADVERTISE_ADDR="${IP_A}:
8083"   -e HTTP_ADDR="0.0.0.0:8080"   -e DATABASE_DSN="file:agenda.db?cache=shared&_fk=1"   -e LOG_LEVEL="info"   -e LOG_FORMAT
="json"   -e LOG_DEST="file:/logs/agenda-3.log"   -e CLUSTER_HMAC_SECRET="$CLUSTER_SECRET"   -e AUDIT_API_TOKEN="$AUDIT_TOKEN" 
  -e DISCOVERY_SEEDS="$PEERS_IP"   "$IMAGE_NAME"

docker run -d   --name "agenda-4"   --hostname "agenda
-4"   --network "$NETWORK_NAME"   -p "8084:8080"   -v "$(pwd)/logs:/logs"   -e NODE_ID="agenda-4"   -e ADVERTISE_ADDR="${IP_A}:
8084"   -e HTTP_ADDR="0.0.0.0:8080"   -e DATABASE_DSN="file:agenda.db?cache=shared&_fk=1"   -e LOG_LEVEL="info"   -e LOG_FORMAT
="json"   -e LOG_DEST="file:/logs/agenda-4.log"   -e CLUSTER_HMAC_SECRET="$CLUSTER_SECRET"   -e AUDIT_API_TOKEN="$AUDIT_TOKEN" 
  -e DISCOVERY_SEEDS="$PEERS_IP"   "$IMAGE_NAME"

### En B
# agenda-5
docker run -d \
  --name "agenda-5" \
  --hostname "agenda-5" \
  --network "$NETWORK_NAME" \
  -p "18080:8080" \
  -v "$(pwd)/logs:/logs" \
  -e NODE_ID="agenda-5" \
  -e ADVERTISE_ADDR="${IP_B}:18080" \
  -e HTTP_ADDR="0.0.0.0:8080" \
  -e DATABASE_DSN="file:agenda.db?cache=shared&_fk=1" \
  -e LOG_LEVEL="info" \
  -e LOG_FORMAT="json" \
  -e LOG_DEST="file:/logs/agenda-5.log" \
  -e CLUSTER_HMAC_SECRET="$CLUSTER_SECRET" \
  -e AUDIT_API_TOKEN="$AUDIT_TOKEN" \
  -e DISCOVERY_SEEDS="$PEERS_IP" \
  "$IMAGE_NAME"
# agenda-6
docker run -d \
  --name "agenda-6" \
  --hostname "agenda-6" \
  --network "$NETWORK_NAME" \
  -p "18082:8080" \
  -v "$(pwd)/logs:/logs" \
  -e NODE_ID="agenda-6" \
  -e ADVERTISE_ADDR="${IP_B}:18082" \
  -e HTTP_ADDR="0.0.0.0:8080" \
  -e DATABASE_DSN="file:agenda.db?cache=shared&_fk=1" \
  -e LOG_LEVEL="info" \
  -e LOG_FORMAT="json" \
  -e LOG_DEST="file:/logs/agenda-6.log" \
  -e CLUSTER_HMAC_SECRET="$CLUSTER_SECRET" \
  -e AUDIT_API_TOKEN="$AUDIT_TOKEN" \
  -e DISCOVERY_SEEDS="$PEERS_IP" \
  "$IMAGE_NAME"
# agenda-7
docker run -d \
  --name "agenda-7" \
  --hostname "agenda-7" \
  --network "$NETWORK_NAME" \
  -p "18083:8080" \
  -v "$(pwd)/logs:/logs" \
  -e NODE_ID="agenda-7" \
  -e ADVERTISE_ADDR="${IP_B}:18083" \
  -e HTTP_ADDR="0.0.0.0:8080" \
  -e DATABASE_DSN="file:agenda.db?cache=shared&_fk=1" \
  -e LOG_LEVEL="info" \
  -e LOG_FORMAT="json" \
  -e LOG_DEST="file:/logs/agenda-7.log" \
  -e CLUSTER_HMAC_SECRET="$CLUSTER_SECRET" \
  -e AUDIT_API_TOKEN="$AUDIT_TOKEN" \
  -e DISCOVERY_SEEDS="$PEERS_IP" \
  "$IMAGE_NAME"

### Revisar salud y liderazgo
for n in "${NODES[@]}"; do
  echo "== $n =="
  curl -s --max-time 2 "http://$n/raft/health"
  echo; echo
done

for n in "${NODES[@]}"; do
  h=$(curl -s --max-time 2 "http://$n/raft/health")
  commit=$(echo "$h" | jq -r '.commit_index')
  applied=$(echo "$h" | jq -r '.last_applied')
  echo "$n lag=$((commit-applied)) commit=$commit applied=$applied"
done

docker logs agenda-1 2>&1 | grep -E "leader|election|append_entries|commit_index|apply_error" | tail -n 200

docker logs -f agenda-1 2>&1 | grep -E "leader|election|commit_index|apply"