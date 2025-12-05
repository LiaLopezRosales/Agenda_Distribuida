#!/bin/bash

# Script para verificar que el sistema distribuido funciona correctamente
# y que los cambios se replican entre nodos

set -e

RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m'

BASE_URL="http://localhost:8080"
PORTS=(8080 8082 8083 8084)
NODES=("agenda-1" "agenda-2" "agenda-3" "agenda-4")

echo -e "${BLUE}═══════════════════════════════════════════════════════════${NC}"
echo -e "${BLUE}  Verificación de Sistema Distribuido - Agenda RAFT${NC}"
echo -e "${BLUE}═══════════════════════════════════════════════════════════${NC}"
echo ""

# Función para hacer requests con formato JSON
json_request() {
    local method=$1
    local url=$2
    local data=$3
    local token=$4
    
    if [ -n "$token" ]; then
        curl -s -X "$method" "$url" \
            -H "Content-Type: application/json" \
            -H "Authorization: Bearer $token" \
            -d "$data"
    else
        curl -s -X "$method" "$url" \
            -H "Content-Type: application/json" \
            -d "$data"
    fi
}

# Función para consultar SQLite dentro de un contenedor (si está disponible)
query_db() {
    local container=$1
    local query=$2
    # Intentar con sqlite3 primero
    docker exec "$container" sqlite3 /agenda.db "$query" 2>/dev/null || \
    docker exec "$container" sqlite3 /app/agenda.db "$query" 2>/dev/null || \
    echo "ERROR"
}

# Función para verificar si un usuario existe consultando la API
check_user_via_api() {
    local port=$1
    local username=$2
    local token=$3
    
    # Intentar hacer login con el usuario (si existe, el login funcionará)
    login_data=$(cat <<EOF
{
  "username": "$username",
  "password": "test123"
}
EOF
)
    response=$(json_request "POST" "http://localhost:${port}/login" "$login_data")
    if echo "$response" | grep -q '"token"'; then
        echo "1"
    else
        echo "0"
    fi
}

# Función para verificar si una cita existe consultando la API
check_appointment_via_api() {
    local port=$1
    local appt_id=$2
    local token=$3
    
    response=$(curl -s -X "GET" "http://localhost:${port}/api/appointments/${appt_id}" \
        -H "Authorization: Bearer $token" 2>/dev/null)
    
    if echo "$response" | grep -q '"id"'; then
        echo "1"
    else
        echo "0"
    fi
}

# ============================================
# 1. Verificar estado del cluster
# ============================================
echo -e "${GREEN}[1/6] Verificando estado del cluster...${NC}"
echo ""

LEADER_NODE=""
LEADER_PORT=""

for i in "${!NODES[@]}"; do
    node=${NODES[$i]}
    port=${PORTS[$i]}
    health=$(curl -s "http://localhost:${port}/raft/health" 2>/dev/null || echo "{}")
    
    if [ "$health" != "{}" ]; then
        node_id=$(echo "$health" | grep -o '"node_id":"[^"]*"' | cut -d'"' -f4)
        is_leader=$(echo "$health" | grep -o '"is_leader":[^,}]*' | cut -d':' -f2 | tr -d ' ')
        leader_id=$(echo "$health" | grep -o '"leader":"[^"]*"' | cut -d'"' -f4)
        
        if [ "$is_leader" = "true" ]; then
            LEADER_NODE="$node"
            LEADER_PORT="$port"
            echo -e "  ${GREEN}✅${NC} $node (puerto $port): ${GREEN}LÍDER${NC} (term: $leader_id)"
        else
            echo -e "  ${BLUE}ℹ️${NC}  $node (puerto $port): seguidor (líder: $leader_id)"
        fi
    else
        echo -e "  ${RED}❌${NC} $node (puerto $port): no disponible"
    fi
done

if [ -z "$LEADER_NODE" ]; then
    echo -e "${RED}❌ No se encontró un líder. El cluster no está funcionando correctamente.${NC}"
    exit 1
fi

echo ""
echo -e "${GREEN}✅ Líder identificado: $LEADER_NODE (puerto $LEADER_PORT)${NC}"
echo ""

# ============================================
# 2. Verificar estado de replicación RAFT
# ============================================
echo -e "${GREEN}[2/6] Verificando estado de replicación RAFT...${NC}"
echo ""

for i in "${!NODES[@]}"; do
    node=${NODES[$i]}
    port=${PORTS[$i]}
    
    if docker ps --format '{{.Names}}' | grep -q "^${node}$"; then
        # Intentar consultar la BD directamente
        term=$(query_db "$node" "SELECT value FROM raft_meta WHERE key='currentTerm';" 2>/dev/null || echo "?")
        commit_idx=$(query_db "$node" "SELECT value FROM raft_meta WHERE key='commitIndex';" 2>/dev/null || echo "?")
        last_applied=$(query_db "$node" "SELECT value FROM raft_meta WHERE key='lastApplied';" 2>/dev/null || echo "?")
        log_count=$(query_db "$node" "SELECT COUNT(*) FROM raft_log;" 2>/dev/null || echo "?")
        
        if [ "$term" = "ERROR" ] || [ "$term" = "?" ]; then
            echo -e "  ${BLUE}📊${NC} $node (puerto $port): ${YELLOW}(BD no accesible directamente)${NC}"
        else
            echo -e "  ${BLUE}📊${NC} $node (puerto $port):"
            echo -e "     Term: $term | Commit: $commit_idx | Applied: $last_applied | Log entries: $log_count"
        fi
    fi
done

echo ""

# ============================================
# 3. Crear un usuario de prueba
# ============================================
echo -e "${GREEN}[3/6] Creando usuario de prueba en el líder...${NC}"

TEST_USERNAME="test_user_$(date +%s)"
TEST_EMAIL="test_${TEST_USERNAME}@example.com"
TEST_PASSWORD="test123"

register_data=$(cat <<EOF
{
  "username": "$TEST_USERNAME",
  "email": "$TEST_EMAIL",
  "password": "$TEST_PASSWORD",
  "name": "Test User"
}
EOF
)

register_response=$(json_request "POST" "http://localhost:${LEADER_PORT}/register" "$register_data")

if echo "$register_response" | grep -q "id"; then
    USER_ID=$(echo "$register_response" | grep -o '"id":[0-9]*' | head -1 | cut -d':' -f2)
    echo -e "  ${GREEN}✅${NC} Usuario creado: $TEST_USERNAME (ID: $USER_ID)"
else
    echo -e "  ${RED}❌${NC} Error al crear usuario: $register_response"
    exit 1
fi

# Obtener token de autenticación
login_data=$(cat <<EOF
{
  "username": "$TEST_USERNAME",
  "password": "$TEST_PASSWORD"
}
EOF
)

login_response=$(json_request "POST" "http://localhost:${LEADER_PORT}/login" "$login_data")
TOKEN=$(echo "$login_response" | grep -o '"token":"[^"]*"' | cut -d'"' -f4)

if [ -z "$TOKEN" ]; then
    echo -e "  ${RED}❌${NC} Error al obtener token de autenticación"
    exit 1
fi

echo -e "  ${GREEN}✅${NC} Token de autenticación obtenido"
echo ""

# Esperar un momento para que se replique
sleep 2

# ============================================
# 4. Verificar que el usuario se replicó en todos los nodos
# ============================================
echo -e "${GREEN}[4/6] Verificando replicación del usuario en todos los nodos...${NC}"
echo ""

ALL_REPLICATED=true

for i in "${!NODES[@]}"; do
    node=${NODES[$i]}
    port=${PORTS[$i]}
    
    if docker ps --format '{{.Names}}' | grep -q "^${node}$"; then
        # Intentar consultar BD directamente primero
        user_count=$(query_db "$node" "SELECT COUNT(*) FROM users WHERE username='$TEST_USERNAME';" 2>/dev/null || echo "ERROR")
        
        if [ "$user_count" = "ERROR" ]; then
            # Fallback: usar API para verificar
            user_count=$(check_user_via_api "$port" "$TEST_USERNAME" "$TOKEN")
            if [ "$user_count" = "1" ]; then
                echo -e "  ${GREEN}✅${NC} $node (puerto $port): usuario encontrado (vía API)"
            else
                echo -e "  ${RED}❌${NC} $node (puerto $port): usuario NO encontrado (vía API)"
                ALL_REPLICATED=false
            fi
        elif [ "$user_count" = "1" ]; then
            user_id_db=$(query_db "$node" "SELECT id FROM users WHERE username='$TEST_USERNAME';" 2>/dev/null || echo "?")
            echo -e "  ${GREEN}✅${NC} $node (puerto $port): usuario encontrado (ID: $user_id_db)"
        else
            echo -e "  ${RED}❌${NC} $node (puerto $port): usuario NO encontrado (count: $user_count)"
            ALL_REPLICATED=false
        fi
    fi
done

if [ "$ALL_REPLICATED" = false ]; then
    echo -e "${RED}❌ La replicación del usuario falló en algunos nodos${NC}"
    exit 1
fi

echo -e "${GREEN}✅ Usuario replicado correctamente en todos los nodos${NC}"
echo ""

# ============================================
# 5. Crear una cita de prueba
# ============================================
echo -e "${GREEN}[5/6] Creando cita de prueba...${NC}"

# Calcular fechas (mañana a las 10:00)
START_TIME=$(date -d "tomorrow 10:00" -u +"%Y-%m-%dT%H:%M:%SZ" 2>/dev/null || date -v+1d -v10H -v0M -v0S -u +"%Y-%m-%dT%H:%M:%SZ" 2>/dev/null || echo "2025-12-25T10:00:00Z")
END_TIME=$(date -d "tomorrow 11:00" -u +"%Y-%m-%dT%H:%M:%SZ" 2>/dev/null || date -v+1d -v11H -v0M -v0S -u +"%Y-%m-%dT%H:%M:%SZ" 2>/dev/null || echo "2025-12-25T11:00:00Z")

appointment_data=$(cat <<EOF
{
  "title": "Cita de Prueba Replicación",
  "description": "Esta cita debe replicarse a todos los nodos",
  "start": "$START_TIME",
  "end": "$END_TIME",
  "privacy": "full"
}
EOF
)

# Intentar crear en un nodo que NO sea el líder (si es posible)
NON_LEADER_PORT=""
for i in "${!NODES[@]}"; do
    if [ "${PORTS[$i]}" != "$LEADER_PORT" ]; then
        NON_LEADER_PORT="${PORTS[$i]}"
        break
    fi
done

# Si no hay otro puerto, usar el líder
if [ -z "$NON_LEADER_PORT" ]; then
    NON_LEADER_PORT="$LEADER_PORT"
fi

create_response=$(json_request "POST" "http://localhost:${NON_LEADER_PORT}/api/appointments" "$appointment_data" "$TOKEN")

if echo "$create_response" | grep -q '"id"'; then
    APPT_ID=$(echo "$create_response" | grep -o '"id":[0-9]*' | head -1 | cut -d':' -f2)
    echo -e "  ${GREEN}✅${NC} Cita creada: ID $APPT_ID (creada en puerto $NON_LEADER_PORT)"
else
    echo -e "  ${YELLOW}⚠️${NC}  Respuesta: $create_response"
    # Intentar extraer el ID de otra forma
    APPT_ID=$(echo "$create_response" | grep -oE '[0-9]+' | head -1)
    if [ -z "$APPT_ID" ]; then
        echo -e "  ${RED}❌${NC} No se pudo crear la cita"
        exit 1
    fi
fi

# Esperar un momento para que se replique
sleep 3

# ============================================
# 6. Verificar que la cita se replicó en todos los nodos
# ============================================
echo -e "${GREEN}[6/6] Verificando replicación de la cita en todos los nodos...${NC}"
echo ""

ALL_REPLICATED=true

for i in "${!NODES[@]}"; do
    node=${NODES[$i]}
    port=${PORTS[$i]}
    
    if docker ps --format '{{.Names}}' | grep -q "^${node}$"; then
        # Intentar consultar BD directamente primero
        appt_count=$(query_db "$node" "SELECT COUNT(*) FROM appointments WHERE id=$APPT_ID AND deleted=0;" 2>/dev/null || echo "ERROR")
        
        if [ "$appt_count" = "ERROR" ]; then
            # Fallback: usar API para verificar
            appt_count=$(check_appointment_via_api "$port" "$APPT_ID" "$TOKEN")
            if [ "$appt_count" = "1" ]; then
                echo -e "  ${GREEN}✅${NC} $node (puerto $port): cita encontrada (ID: $APPT_ID, vía API)"
            else
                echo -e "  ${RED}❌${NC} $node (puerto $port): cita NO encontrada (vía API)"
                ALL_REPLICATED=false
            fi
        elif [ "$appt_count" = "1" ]; then
            appt_title=$(query_db "$node" "SELECT title FROM appointments WHERE id=$APPT_ID;" 2>/dev/null || echo "?")
            echo -e "  ${GREEN}✅${NC} $node (puerto $port): cita encontrada (ID: $APPT_ID, título: $appt_title)"
        else
            echo -e "  ${RED}❌${NC} $node (puerto $port): cita NO encontrada (count: $appt_count)"
            ALL_REPLICATED=false
        fi
    fi
done

if [ "$ALL_REPLICATED" = false ]; then
    echo -e "${RED}❌ La replicación de la cita falló en algunos nodos${NC}"
    exit 1
fi

echo -e "${GREEN}✅ Cita replicada correctamente en todos los nodos${NC}"
echo ""

# ============================================
# 7. Crear un grupo de prueba
# ============================================
echo -e "${GREEN}[7/7] Creando grupo de prueba y verificando replicación...${NC}"

GROUP_NAME="Grupo Prueba Replicación"
GROUP_DESC="Este grupo debe replicarse a todos los nodos"

group_data=$(cat <<EOF
{
  "name": "$GROUP_NAME",
  "description": "$GROUP_DESC",
  "group_type": "non_hierarchical"
}
EOF
)

group_response=$(json_request "POST" "http://localhost:${LEADER_PORT}/api/groups" "$group_data" "$TOKEN")

if echo "$group_response" | grep -q '"name"'; then
    echo -e "  ${GREEN}✅${NC} Grupo creado en el líder"
else
    echo -e "  ${RED}❌${NC} Error al crear grupo: $group_response"
    exit 1
fi

# Esperar a que la operación se replique
sleep 3

ALL_GROUPS_REPLICATED=true

for i in "${!NODES[@]}"; do
    node=${NODES[$i]}
    port=${PORTS[$i]}

    if docker ps --format '{{.Names}}' | grep -q "^${node}$"; then
        group_count=$(query_db "$node" "SELECT COUNT(*) FROM groups WHERE name='$GROUP_NAME';" 2>/dev/null || echo "ERROR")
        if [ "$group_count" = "1" ]; then
            echo -e "  ${GREEN}✅${NC} $node (puerto $port): grupo encontrado (nombre: $GROUP_NAME)"
        else
            echo -e "  ${RED}❌${NC} $node (puerto $port): grupo NO encontrado (count: $group_count)"
            ALL_GROUPS_REPLICATED=false
        fi
    fi
done

if [ "$ALL_GROUPS_REPLICATED" = false ]; then
    echo -e "${RED}❌ La replicación del grupo falló en algunos nodos${NC}"
    exit 1
fi

echo -e "${GREEN}✅ Grupo replicado correctamente en todos los nodos${NC}"
echo ""

# ============================================
# Resumen final
# ============================================
echo -e "${BLUE}═══════════════════════════════════════════════════════════${NC}"
echo -e "${GREEN}✅ VERIFICACIÓN COMPLETA${NC}"
echo -e "${BLUE}═══════════════════════════════════════════════════════════${NC}"
echo ""
echo -e "${GREEN}Resumen:${NC}"
echo "  - Líder: $LEADER_NODE (puerto $LEADER_PORT)"
echo "  - Usuario de prueba: $TEST_USERNAME (ID: $USER_ID)"
echo "  - Cita de prueba: ID $APPT_ID"
echo "  - Grupo de prueba: $GROUP_NAME"
echo "  - Replicación: ${GREEN}✅ FUNCIONANDO${NC}"
echo ""
echo -e "${BLUE}Para verificar manualmente:${NC}"
echo "  - Ver estado RAFT: curl -s http://localhost:8080/raft/health | jq"
echo "  - Ver agenda (citas): curl -s \"http://localhost:8080/api/agenda?start=2025-01-01T00:00:00Z&end=2026-01-01T00:00:00Z\" -H \"Authorization: Bearer $TOKEN\" | jq"
echo "  - Ver cita específica: curl -s http://localhost:8080/api/appointments/$APPT_ID -H \"Authorization: Bearer $TOKEN\" | jq"
echo ""
echo -e "${YELLOW}Nota:${NC} Para consultar la BD directamente, necesitarías instalar sqlite3 en los contenedores"
echo ""

