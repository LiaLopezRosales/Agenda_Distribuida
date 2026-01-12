#!/bin/bash
#
# Script para simular una partición de red entre dos grupos de nodos
# Grupo 1: agenda-1, agenda-2, agenda-3, agenda-4 (4 nodos)
# Grupo 2: agenda-5, agenda-6, agenda-7 (3 nodos)
#
# Uso:
#   ./simulate-network-partition.sh partition    # Crear la partición
#   ./simulate-network-partition.sh reunify      # Eliminar la partición
#   ./simulate-network-partition.sh status       # Ver estado actual
#   ./simulate-network-partition.sh clear        # Limpiar todas las reglas

set -e

# Colores para output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

# Grupos de nodos
GROUP1_CONTAINERS=("agenda-1" "agenda-2" "agenda-3" "agenda-4")
GROUP2_CONTAINERS=("agenda-5" "agenda-6" "agenda-7")

# Prefijo para identificar nuestras reglas
RULE_PREFIX="AGENDA_PARTITION"

# Función para obtener la IP de un contenedor
get_container_ip() {
    local container_name=$1
    docker inspect -f '{{range.NetworkSettings.Networks}}{{.IPAddress}}{{end}}' "$container_name" 2>/dev/null || echo ""
}

# Función para obtener todas las IPs de los contenedores
# Imprime mensajes de estado a stderr, IPs a stdout
get_group_ips() {
    local group_containers=("$@")
    local ips=()
    
    for container in "${group_containers[@]}"; do
        local ip=$(get_container_ip "$container")
        if [ -n "$ip" ]; then
            ips+=("$ip")
            echo -e "${BLUE}  ✓${NC} $container -> $ip" >&2
        else
            echo -e "${YELLOW}  ⚠${NC} $container no encontrado o sin IP" >&2
        fi
    done
    
    # Imprimir IPs a stdout (separado de los mensajes)
    printf '%s\n' "${ips[@]}"
}

# Función para verificar si una regla existe
rule_exists() {
    local chain=$1
    local source=$2
    local dest=$3
    
    # Verificar si existe regla con DROP (puede ser REJECT o DROP)
    iptables -C "$chain" -s "$source" -d "$dest" -j DROP -m comment --comment "$RULE_PREFIX" 2>/dev/null || \
    iptables -C "$chain" -s "$source" -d "$dest" -j REJECT -m comment --comment "$RULE_PREFIX" 2>/dev/null
}

# Función para obtener el nombre del bridge de Docker
get_docker_bridge() {
    local network_name="${NETWORK_NAME:-agenda_net}"
    docker network inspect "$network_name" --format '{{range $key, $value := .Options}}{{$key}}={{$value}}{{end}}' 2>/dev/null | grep -oP 'com.docker.network.bridge.name=\K[^ ]+' || \
    docker network inspect "$network_name" --format '{{(index .Options "com.docker.network.bridge.name")}}' 2>/dev/null || \
    echo "br-$(docker network inspect "$network_name" --format '{{substr .Id 0 12}}' 2>/dev/null)"
}

# Función para crear reglas de bloqueo bidireccional usando múltiples métodos
block_communication() {
    local source_ip=$1
    local dest_ip=$2
    
    local created=0
    
    # Método 1: iptables en DOCKER-USER (para tráfico externo)
    if iptables -L DOCKER-USER >/dev/null 2>&1; then
        if ! rule_exists "DOCKER-USER" "$source_ip" "$dest_ip"; then
            if iptables -I DOCKER-USER 1 -s "$source_ip" -d "$dest_ip" -j DROP -m comment --comment "$RULE_PREFIX" 2>/dev/null; then
                created=1
            fi
        fi
        if ! rule_exists "DOCKER-USER" "$dest_ip" "$source_ip"; then
            if iptables -I DOCKER-USER 1 -s "$dest_ip" -d "$source_ip" -j DROP -m comment --comment "$RULE_PREFIX" 2>/dev/null; then
                created=1
            fi
        fi
    fi
    
    # Método 2: iptables en FORWARD (respaldo)
    if iptables -L FORWARD >/dev/null 2>&1; then
        if ! rule_exists "FORWARD" "$source_ip" "$dest_ip"; then
            if iptables -I FORWARD 1 -s "$source_ip" -d "$dest_ip" -j DROP -m comment --comment "$RULE_PREFIX" 2>/dev/null; then
                created=1
            fi
        fi
        if ! rule_exists "FORWARD" "$dest_ip" "$source_ip"; then
            if iptables -I FORWARD 1 -s "$dest_ip" -d "$source_ip" -j DROP -m comment --comment "$RULE_PREFIX" 2>/dev/null; then
                created=1
            fi
        fi
    fi
    
    # Método 3: Reglas en el bridge de Docker (para tráfico interno)
    local bridge=$(get_docker_bridge)
    if [ -n "$bridge" ] && [ -d "/sys/class/net/$bridge" ]; then
        # Usar ebtables para bloquear a nivel de bridge
        if command -v ebtables >/dev/null 2>&1; then
            if ! ebtables -L FORWARD 2>/dev/null | grep -q "$RULE_PREFIX.*$source_ip.*$dest_ip"; then
                if ebtables -A FORWARD -s "$(ip_to_mac "$source_ip")" -d "$(ip_to_mac "$dest_ip")" -j DROP -m comment --comment "$RULE_PREFIX" 2>/dev/null; then
                    created=1
                fi
            fi
        fi
        
        # Usar tc (traffic control) en el bridge para bloquear tráfico
        # Esto es más complejo, lo dejamos como método alternativo
    fi
    
    if [ $created -eq 1 ]; then
        echo -e "${GREEN}  ✓${NC} Bloqueo: $source_ip ↔ $dest_ip"
    else
        echo -e "${YELLOW}  ⚠${NC} No se pudo bloquear: $source_ip ↔ $dest_ip" >&2
    fi
}

# Función helper para convertir IP a MAC (simplificada)
ip_to_mac() {
    local ip=$1
    # Intentar obtener MAC del contenedor con esa IP
    local container=$(docker ps --format '{{.Names}}' | while read name; do
        if docker inspect "$name" --format '{{range.NetworkSettings.Networks}}{{.IPAddress}}{{end}}' 2>/dev/null | grep -q "^$ip$"; then
            echo "$name"
            break
        fi
    done)
    
    if [ -n "$container" ]; then
        docker inspect "$container" --format '{{range.NetworkSettings.Networks}}{{.MacAddress}}{{end}}' 2>/dev/null || echo ""
    fi
}

# Función para particionar la red
partition_network() {
    echo -e "${BLUE}=== Simulando partición de red ===${NC}\n"
    
    echo -e "${YELLOW}Obteniendo IPs del Grupo 1 (agenda-1..4):${NC}" >&2
    local group1_ips=()
    while IFS= read -r ip; do
        [ -n "$ip" ] && group1_ips+=("$ip")
    done < <(get_group_ips "${GROUP1_CONTAINERS[@]}")
    
    echo -e "\n${YELLOW}Obteniendo IPs del Grupo 2 (agenda-5..7):${NC}" >&2
    local group2_ips=()
    while IFS= read -r ip; do
        [ -n "$ip" ] && group2_ips+=("$ip")
    done < <(get_group_ips "${GROUP2_CONTAINERS[@]}")
    
    if [ ${#group1_ips[@]} -eq 0 ] || [ ${#group2_ips[@]} -eq 0 ]; then
        echo -e "${RED}✗ Error: No se encontraron suficientes contenedores.${NC}"
        echo "  Asegúrate de que todos los nodos estén ejecutándose."
        exit 1
    fi
    
    echo -e "\n${YELLOW}IPs obtenidas:${NC}"
    echo -e "${BLUE}Grupo 1:${NC} ${group1_ips[*]}"
    echo -e "${BLUE}Grupo 2:${NC} ${group2_ips[*]}"
    
    if [ ${#group1_ips[@]} -eq 0 ] || [ ${#group2_ips[@]} -eq 0 ]; then
        echo -e "${RED}✗ Error: No se obtuvieron IPs válidas${NC}"
        echo -e "  Grupo 1: ${#group1_ips[@]} IPs"
        echo -e "  Grupo 2: ${#group2_ips[@]} IPs"
        exit 1
    fi
    
    echo -e "\n${YELLOW}Creando reglas de bloqueo bidireccional...${NC}"
    local count=0
    local success_count=0
    
    for ip1 in "${group1_ips[@]}"; do
        for ip2 in "${group2_ips[@]}"; do
            if [ -n "$ip1" ] && [ -n "$ip2" ] && [ "$ip1" != "$ip2" ]; then
                echo -e "${YELLOW}  Bloqueando $ip1 ↔ $ip2...${NC}" >&2
                block_communication "$ip1" "$ip2"
                ((count++))
                success_count=$((success_count + 1))
            fi
        done
    done
    
    echo -e "\n${GREEN}✓ Intento de partición completado${NC}"
    echo -e "  Procesados ${count} pares de comunicaciones"
    echo -e "  ${BLUE}Nota:${NC} Si la partición no funciona completamente, puede ser necesario usar tc (traffic control)"
    echo -e "       o desconectar contenedores de la red (docker network disconnect)."
    echo -e "\n${BLUE}Para verificar:${NC} Ejecuta ./check-partition.sh"
    
    # Mostrar advertencia sobre Docker bridge
    echo -e "\n${YELLOW}⚠ Advertencia sobre Docker Bridge:${NC}"
    echo -e "  Docker bridge networks pueden no respetar completamente las reglas de iptables."
    echo -e "  Si la partición no funciona, considera usar el método alternativo:"
    echo -e "  ${BLUE}sudo ./simulate-network-partition-advanced.sh partition${NC}"
}

# Función para reunificar la red
reunify_network() {
    echo -e "${BLUE}=== Reunificando la red ===${NC}\n"
    
    # Eliminar todas las reglas con nuestro comentario de todas las cadenas relevantes
    local deleted=0
    local chains=("DOCKER-USER" "FORWARD")
    
    for chain in "${chains[@]}"; do
        # Verificar si la cadena existe
        if ! iptables -L "$chain" >/dev/null 2>&1; then
            continue
        fi
        
        # Eliminar reglas de esta cadena
        while iptables -L "$chain" -n --line-numbers 2>/dev/null | grep -q "$RULE_PREFIX"; do
            local line_num=$(iptables -L "$chain" -n --line-numbers 2>/dev/null | grep "$RULE_PREFIX" | head -n 1 | awk '{print $1}')
            if [ -n "$line_num" ]; then
                iptables -D "$chain" "$line_num" 2>/dev/null || break
                ((deleted++))
            else
                break
            fi
        done
    done
    
    if [ $deleted -gt 0 ]; then
        echo -e "${GREEN}✓ Eliminadas $deleted reglas de bloqueo${NC}"
        echo -e "${GREEN}✓ Red reunificada - Todos los nodos pueden comunicarse ahora${NC}"
    else
        echo -e "${YELLOW}⚠ No se encontraron reglas de partición para eliminar${NC}"
    fi
}

# Función para mostrar el estado
show_status() {
    echo -e "${BLUE}=== Estado de la partición de red ===${NC}\n"
    
    # Contar reglas en todas las cadenas relevantes
    local rules=0
    local chains=("DOCKER-USER" "FORWARD")
    
    for chain in "${chains[@]}"; do
        if iptables -L "$chain" >/dev/null 2>&1; then
            local chain_rules=$(iptables -L "$chain" -n 2>/dev/null | grep -c "$RULE_PREFIX" 2>/dev/null || true)
            # Asegurar que chain_rules es un número válido
            if [ -z "$chain_rules" ] || ! [[ "$chain_rules" =~ ^[0-9]+$ ]]; then
                chain_rules=0
            fi
            rules=$((rules + chain_rules))
        fi
    done
    
    if [ "$rules" -gt 0 ]; then
        echo -e "${RED}✗ RED PARTICIONADA${NC} ($rules reglas activas)"
        echo -e "\n${YELLOW}Reglas activas:${NC}"
        for chain in "${chains[@]}"; do
            if iptables -L "$chain" >/dev/null 2>&1; then
                local chain_rules=$(iptables -L "$chain" -n -v 2>/dev/null | grep "$RULE_PREFIX" | head -n 5)
                if [ -n "$chain_rules" ]; then
                    echo -e "${BLUE}[$chain]:${NC}"
                    echo "$chain_rules" | sed 's/^/  /'
                fi
            fi
        done
        if [ "$rules" -gt 10 ]; then
            echo "  ... y $((rules - 10)) más"
        fi
    else
        echo -e "${GREEN}✓ RED REUNIFICADA${NC} (sin partición)"
    fi
    
    echo -e "\n${YELLOW}Estado de contenedores:${NC}"
    echo -e "${BLUE}Grupo 1:${NC}"
    for container in "${GROUP1_CONTAINERS[@]}"; do
        local ip=$(get_container_ip "$container")
        if [ -n "$ip" ]; then
            local status=$(docker ps --filter "name=$container" --format "{{.Status}}" 2>/dev/null || echo "no existe")
            echo -e "  $container: $ip ($status)"
        else
            echo -e "  $container: ${RED}no encontrado${NC}"
        fi
    done
    
    echo -e "\n${BLUE}Grupo 2:${NC}"
    for container in "${GROUP2_CONTAINERS[@]}"; do
        local ip=$(get_container_ip "$container")
        if [ -n "$ip" ]; then
            local status=$(docker ps --filter "name=$container" --format "{{.Status}}" 2>/dev/null || echo "no existe")
            echo -e "  $container: $ip ($status)"
        else
            echo -e "  $container: ${RED}no encontrado${NC}"
        fi
    done
}

# Función para limpiar todas las reglas
clear_all_rules() {
    echo -e "${YELLOW}=== Limpiando todas las reglas de partición ===${NC}\n"
    reunify_network
}

# Verificar permisos
if [ "$EUID" -ne 0 ]; then
    echo -e "${RED}Error: Este script requiere permisos de root (sudo)${NC}"
    echo "Uso: sudo $0 [partition|reunify|status|clear]"
    exit 1
fi

# Verificar que iptables está disponible
if ! command -v iptables &> /dev/null; then
    echo -e "${RED}Error: iptables no está instalado${NC}"
    exit 1
fi

# Procesar comando
case "${1:-status}" in
    partition)
        partition_network
        ;;
    reunify)
        reunify_network
        ;;
    status)
        show_status
        ;;
    clear)
        clear_all_rules
        ;;
    *)
        echo "Uso: $0 [partition|reunify|status|clear]"
        echo ""
        echo "Comandos:"
        echo "  partition  - Crear partición de red entre Grupo 1 y Grupo 2"
        echo "  reunify    - Eliminar partición y reunificar la red"
        echo "  status     - Mostrar estado actual de la partición"
        echo "  clear      - Limpiar todas las reglas de partición"
        exit 1
        ;;
esac

