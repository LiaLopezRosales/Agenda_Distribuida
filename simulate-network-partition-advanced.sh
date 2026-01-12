#!/bin/bash
#
# Método avanzado para simular partición usando tc (traffic control)
# y desconexión de red Docker
#

set -e

RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m'

GROUP1_CONTAINERS=("agenda-1" "agenda-2" "agenda-3" "agenda-4")
GROUP2_CONTAINERS=("agenda-5" "agenda-6" "agenda-7")

NETWORK_NAME="${NETWORK_NAME:-agenda_net}"

# Obtener interfaz virtual de un contenedor
get_container_veth() {
    local container=$1
    local pid=$(docker inspect -f '{{.State.Pid}}' "$container" 2>/dev/null || echo "")
    
    if [ -z "$pid" ]; then
        echo ""
        return
    fi
    
    # Buscar la interfaz virtual asociada al contenedor
    local ifindex=$(nsenter -t "$pid" -n ip -o link | grep -v lo | awk -F': ' '{print $2}' | head -n 1)
    
    # Encontrar el veth pair en el host
    local veth=$(ip -o link | grep "^[0-9]*: veth" | while read num name rest; do
        local peer=$(ethtool -S "$name" 2>/dev/null | grep -i peer | awk '{print $2}' || echo "")
        if [ -n "$peer" ]; then
            echo "$name"
            break
        fi
    done | head -n 1)
    
    echo "$veth"
}

# Bloquear comunicación usando tc
block_with_tc() {
    local container=$1
    local target_ip=$2
    
    local veth=$(get_container_veth "$container")
    
    if [ -z "$veth" ]; then
        echo -e "${YELLOW}  ⚠${NC} No se encontró veth para $container"
        return 1
    fi
    
    # Verificar si ya existe la regla
    if tc qdisc show dev "$veth" | grep -q "netem.*loss 100"; then
        echo -e "${YELLOW}  ⚠${NC} Ya existe bloqueo en $veth"
        return 0
    fi
    
    # Agregar qdisc para bloquear tráfico hacia target_ip
    if tc qdisc add dev "$veth" root handle 1: prio >/dev/null 2>&1; then
        if tc filter add dev "$veth" parent 1:0 protocol ip prio 1 u32 match ip dst "$target_ip" flowid 1:3 >/dev/null 2>&1; then
            if tc qdisc add dev "$veth" parent 1:3 handle 30: netem loss 100% >/dev/null 2>&1; then
                echo -e "${GREEN}  ✓${NC} Bloqueado $container -> $target_ip usando tc"
                return 0
            fi
        fi
    fi
    
    echo -e "${RED}  ✗${NC} No se pudo bloquear con tc"
    return 1
}

# Método alternativo: desconectar y reconectar con restricciones
partition_with_docker() {
    echo -e "${BLUE}=== Simulando partición usando Docker network disconnect ===${NC}\n"
    echo -e "${YELLOW}Este método desconecta los grupos de la red principal y los conecta a redes separadas${NC}\n"
    
    # Crear redes separadas si no existen
    if ! docker network inspect "agenda_group1" >/dev/null 2>&1; then
        docker network create --driver bridge "agenda_group1" >/dev/null 2>&1
        echo -e "${GREEN}✓ Red agenda_group1 creada${NC}"
    fi
    
    if ! docker network inspect "agenda_group2" >/dev/null 2>&1; then
        docker network create --driver bridge "agenda_group2" >/dev/null 2>&1
        echo -e "${GREEN}✓ Red agenda_group2 creada${NC}"
    fi
    
    # Desconectar ambos grupos de la red principal
    echo -e "\n${YELLOW}Desconectando ambos grupos de la red principal ($NETWORK_NAME)...${NC}"
    
    all_containers=("${GROUP1_CONTAINERS[@]}" "${GROUP2_CONTAINERS[@]}")
    for container in "${all_containers[@]}"; do
        if docker ps --format '{{.Names}}' | grep -q "^${container}$"; then
            # Verificar si está conectado a la red principal
            if docker inspect "$container" --format '{{range $net, $conf := .NetworkSettings.Networks}}{{$net}} {{end}}' 2>/dev/null | grep -q "$NETWORK_NAME"; then
                docker network disconnect "$NETWORK_NAME" "$container" 2>/dev/null || true
                echo -e "${GREEN}  ✓${NC} $container desconectado de $NETWORK_NAME"
            fi
        fi
    done
    
    # Conectar cada grupo SOLO a su red separada (no a la principal)
    echo -e "\n${YELLOW}Conectando grupos a redes separadas...${NC}"
    for container in "${GROUP1_CONTAINERS[@]}"; do
        if docker ps --format '{{.Names}}' | grep -q "^${container}$"; then
            # Asegurar que esté conectado a agenda_group1
            if ! docker inspect "$container" --format '{{range $net, $conf := .NetworkSettings.Networks}}{{$net}} {{end}}' 2>/dev/null | grep -q "agenda_group1"; then
                docker network connect "agenda_group1" "$container" 2>/dev/null || true
                echo -e "${GREEN}  ✓${NC} $container conectado a agenda_group1"
            else
                echo -e "${YELLOW}  ✓${NC} $container ya está en agenda_group1"
            fi
        fi
    done
    
    for container in "${GROUP2_CONTAINERS[@]}"; do
        if docker ps --format '{{.Names}}' | grep -q "^${container}$"; then
            # Asegurar que esté conectado a agenda_group2
            if ! docker inspect "$container" --format '{{range $net, $conf := .NetworkSettings.Networks}}{{$net}} {{end}}' 2>/dev/null | grep -q "agenda_group2"; then
                docker network connect "agenda_group2" "$container" 2>/dev/null || true
                echo -e "${GREEN}  ✓${NC} $container conectado a agenda_group2"
            else
                echo -e "${YELLOW}  ✓${NC} $container ya está en agenda_group2"
            fi
        fi
    done
    
    # Verificar que ningún grupo esté en la red principal
    echo -e "\n${YELLOW}Verificando aislamiento...${NC}"
    local still_connected=0
    for container in "${all_containers[@]}"; do
        if docker inspect "$container" --format '{{range $net, $conf := .NetworkSettings.Networks}}{{$net}} {{end}}' 2>/dev/null | grep -q "$NETWORK_NAME"; then
            echo -e "${RED}  ✗${NC} $container todavía está en $NETWORK_NAME"
            still_connected=1
        fi
    done
    
    if [ $still_connected -eq 0 ]; then
        echo -e "${GREEN}  ✓${NC} Todos los contenedores desconectados de $NETWORK_NAME"
    fi
    
    # IMPORTANTE: Los nodos usan ADVERTISE_ADDR con IPs del host (ej: 10.207.119.228:8080)
    # El tráfico va desde las IPs internas de Docker hacia la IP del host
    # Necesitamos bloquear usando iptables en las IPs internas hacia los puertos del otro grupo
    
    echo -e "\n${YELLOW}Bloqueando comunicación a nivel de host...${NC}"
    
    # Obtener IPs internas de los contenedores
    local GROUP1_INTERNAL_IPS=()
    local GROUP2_INTERNAL_IPS=()
    
    for container in "${GROUP1_CONTAINERS[@]}"; do
        local ip=$(docker inspect -f '{{range.NetworkSettings.Networks}}{{.IPAddress}}{{end}}' "$container" 2>/dev/null || echo "")
        if [ -n "$ip" ]; then
            GROUP1_INTERNAL_IPS+=("$ip")
        fi
    done
    
    for container in "${GROUP2_CONTAINERS[@]}"; do
        local ip=$(docker inspect -f '{{range.NetworkSettings.Networks}}{{.IPAddress}}{{end}}' "$container" 2>/dev/null || echo "")
        if [ -n "$ip" ]; then
            GROUP2_INTERNAL_IPS+=("$ip")
        fi
    done
    
    echo -e "${BLUE}  Grupo 1 IPs: ${GROUP1_INTERNAL_IPS[*]}${NC}"
    echo -e "${BLUE}  Grupo 2 IPs: ${GROUP2_INTERNAL_IPS[*]}${NC}"
    
    # Puertos del Grupo 1: 8080, 8082, 8083, 8084
    # Puertos del Grupo 2: 18080, 18082, 18083
    local GROUP1_PORTS=(8080 8082 8083 8084)
    local GROUP2_PORTS=(18080 18082 18083)
    
    # Bloquear: desde IPs del Grupo 1 hacia puertos del Grupo 2
    for ip1 in "${GROUP1_INTERNAL_IPS[@]}"; do
        for port2 in "${GROUP2_PORTS[@]}"; do
            # Bloquear FORWARD desde IP Grupo 1 hacia cualquier destino en puerto Grupo 2
            if ! iptables -C FORWARD -p tcp -s "$ip1" -m multiport --dports $(IFS=,; echo "${GROUP2_PORTS[*]}") -j DROP -m comment --comment "AGENDA_PARTITION" 2>/dev/null; then
                iptables -I FORWARD 1 -p tcp -s "$ip1" -m multiport --dports $(IFS=,; echo "${GROUP2_PORTS[*]}") -j DROP -m comment --comment "AGENDA_PARTITION" 2>/dev/null || true
            fi
        done
    done
    
    # Bloquear: desde IPs del Grupo 2 hacia puertos del Grupo 1
    for ip2 in "${GROUP2_INTERNAL_IPS[@]}"; do
        for port1 in "${GROUP1_PORTS[@]}"; do
            # Bloquear FORWARD desde IP Grupo 2 hacia cualquier destino en puerto Grupo 1
            if ! iptables -C FORWARD -p tcp -s "$ip2" -m multiport --dports $(IFS=,; echo "${GROUP1_PORTS[*]}") -j DROP -m comment --comment "AGENDA_PARTITION" 2>/dev/null; then
                iptables -I FORWARD 1 -p tcp -s "$ip2" -m multiport --dports $(IFS=,; echo "${GROUP1_PORTS[*]}") -j DROP -m comment --comment "AGENDA_PARTITION" 2>/dev/null || true
            fi
        done
    done
    
    # Bloquear en DOCKER-USER (Docker lo consulta antes de las reglas normales)
    # Esto bloquea conexiones desde IPs del Grupo 1 hacia puertos del Grupo 2
    for ip1 in "${GROUP1_INTERNAL_IPS[@]}"; do
        if ! iptables -C DOCKER-USER -p tcp -s "$ip1" -m multiport --dports $(IFS=,; echo "${GROUP2_PORTS[*]}") -j DROP -m comment --comment "AGENDA_PARTITION" 2>/dev/null; then
            iptables -I DOCKER-USER 1 -p tcp -s "$ip1" -m multiport --dports $(IFS=,; echo "${GROUP2_PORTS[*]}") -j DROP -m comment --comment "AGENDA_PARTITION" 2>/dev/null || true
            echo -e "${GREEN}  ✓${NC} Bloqueado DOCKER-USER: $ip1 -> puertos Grupo 2"
        fi
    done
    
    # Bloquear conexiones desde IPs del Grupo 2 hacia puertos del Grupo 1
    for ip2 in "${GROUP2_INTERNAL_IPS[@]}"; do
        if ! iptables -C DOCKER-USER -p tcp -s "$ip2" -m multiport --dports $(IFS=,; echo "${GROUP1_PORTS[*]}") -j DROP -m comment --comment "AGENDA_PARTITION" 2>/dev/null; then
            iptables -I DOCKER-USER 1 -p tcp -s "$ip2" -m multiport --dports $(IFS=,; echo "${GROUP1_PORTS[*]}") -j DROP -m comment --comment "AGENDA_PARTITION" 2>/dev/null || true
            echo -e "${GREEN}  ✓${NC} Bloqueado DOCKER-USER: $ip2 -> puertos Grupo 1"
        fi
    done
    
    # También bloquear en la tabla NAT PREROUTING (para conexiones entrantes)
    # Esto es importante porque Docker usa NAT para el port forwarding
    for port2 in "${GROUP2_PORTS[@]}"; do
        # Bloquear PREROUTING hacia puertos del Grupo 2 si viene de IPs del Grupo 1
        for ip1 in "${GROUP1_INTERNAL_IPS[@]}"; do
            if ! iptables -t nat -C PREROUTING -p tcp -s "$ip1" --dport "$port2" -j DROP -m comment --comment "AGENDA_PARTITION" 2>/dev/null; then
                iptables -t nat -I PREROUTING 1 -p tcp -s "$ip1" --dport "$port2" -j DROP -m comment --comment "AGENDA_PARTITION" 2>/dev/null || true
            fi
        done
    done
    
    for port1 in "${GROUP1_PORTS[@]}"; do
        # Bloquear PREROUTING hacia puertos del Grupo 1 si viene de IPs del Grupo 2
        for ip2 in "${GROUP2_INTERNAL_IPS[@]}"; do
            if ! iptables -t nat -C PREROUTING -p tcp -s "$ip2" --dport "$port1" -j DROP -m comment --comment "AGENDA_PARTITION" 2>/dev/null; then
                iptables -t nat -I PREROUTING 1 -p tcp -s "$ip2" --dport "$port1" -j DROP -m comment --comment "AGENDA_PARTITION" 2>/dev/null || true
            fi
        done
    done
    
    echo -e "${GREEN}  ✓${NC} Reglas de NAT PREROUTING creadas"
    
    echo -e "${GREEN}  ✓${NC} Reglas de iptables creadas para bloquear comunicación entre grupos"
    
    echo -e "\n${GREEN}✓ Partición creada usando Docker networks + iptables${NC}"
    echo -e "${YELLOW}Nota:${NC} Los contenedores están en redes separadas Y las conexiones"
    echo -e "      a través de localhost están bloqueadas a nivel del host."
}

reunify_with_docker() {
    echo -e "${BLUE}=== Reunificando red usando Docker ===${NC}\n"
    
    # Eliminar reglas de iptables (tanto AGENDA_PARTITION como AGENDA_PARTITION_ALLOW)
    echo -e "${YELLOW}Eliminando reglas de bloqueo de iptables...${NC}"
    local deleted=0
    local chains=("OUTPUT" "INPUT")
    
    # Eliminar reglas de FORWARD también
    chains=("OUTPUT" "INPUT" "FORWARD")
    
    for chain in "${chains[@]}"; do
        # Eliminar todas las reglas AGENDA_PARTITION (incluyendo variantes)
        while iptables -L "$chain" -n --line-numbers 2>/dev/null | grep -q "AGENDA_PARTITION"; do
            local line_num=$(iptables -L "$chain" -n --line-numbers 2>/dev/null | grep "AGENDA_PARTITION" | head -n 1 | awk '{print $1}')
            if [ -n "$line_num" ]; then
                iptables -D "$chain" "$line_num" 2>/dev/null || break
                ((deleted++))
            else
                break
            fi
        done
    done
    
    if [ $deleted -gt 0 ]; then
        echo -e "${GREEN}  ✓${NC} Eliminadas $deleted reglas de iptables"
    fi
    
    # Reconectar todos a la red principal
    echo -e "\n${YELLOW}Reconectando contenedores a la red principal...${NC}"
    
    all_containers=("${GROUP1_CONTAINERS[@]}" "${GROUP2_CONTAINERS[@]}")
    
    for container in "${all_containers[@]}"; do
        if docker ps --format '{{.Names}}' | grep -q "^${container}$"; then
            # Desconectar de redes secundarias
            docker network disconnect "agenda_group1" "$container" 2>/dev/null || true
            docker network disconnect "agenda_group2" "$container" 2>/dev/null || true
            
            # Reconectar a red principal
            docker network connect "$NETWORK_NAME" "$container" 2>/dev/null || true
            echo -e "${GREEN}  ✓${NC} $container reconectado a $NETWORK_NAME"
        fi
    done
    
    echo -e "\n${GREEN}✓ Red reunificada${NC}"
}

clear_tc_rules() {
    echo -e "${YELLOW}Limpiando reglas de tc...${NC}"
    
    # Buscar todas las interfaces veth
    ip -o link | grep "^[0-9]*: veth" | awk -F': ' '{print $2}' | while read veth; do
        if tc qdisc show dev "$veth" >/dev/null 2>&1; then
            tc qdisc del dev "$veth" root >/dev/null 2>&1 || true
            echo -e "${GREEN}  ✓${NC} Limpiado $veth"
        fi
    done
}

case "${1:-partition}" in
    partition)
        partition_with_docker
        ;;
    reunify)
        reunify_with_docker
        ;;
    clear)
        clear_tc_rules
        reunify_with_docker
        ;;
    *)
        echo "Uso: $0 [partition|reunify|clear]"
        echo ""
        echo "Método avanzado usando Docker network disconnect/connect"
        exit 1
        ;;
esac

