# Cluster Manual (Sin Docker Compose)

Este método levanta el cluster usando `docker run` directamente, evitando Docker Compose y Docker Swarm.

## Características

- ✅ **Sin Docker Compose**: Usa `docker run` directamente
- ✅ **Sin volúmenes para BD**: La base de datos se guarda dentro del contenedor
- ✅ **Volúmenes solo para logs**: Los logs se guardan en `./logs/` en el host
- ✅ **Sin balanceador de Swarm**: Cada contenedor tiene su puerto publicado directamente
- ✅ **Acceso directo**: `localhost:8080` funciona sin problemas

## Requisitos

- Docker instalado
- Imagen construida: `agenda-distribuida:latest`

## Uso

### 1. Construir la imagen (si no está construida)

```bash
docker build -t agenda-distribuida:latest .
```

### 2. Levantar el cluster

```bash
./start-cluster.sh
```

Este script:
- Crea la red `agenda_net` si no existe
- Levanta 4 contenedores (`agenda-1` a `agenda-4`)
- Configura los puertos: 8080, 8082, 8083, 8084
- Monta volúmenes solo para logs en `./logs/`
- **NO** monta volúmenes para la base de datos (se guarda dentro del contenedor)

### 3. Verificar que está funcionando

```bash
# Ver estado de contenedores
docker ps --filter 'name=agenda-'

# Ver logs de un nodo
docker logs -f agenda-1

# Ver logs desde archivos
tail -f logs/agenda-1.log

# Verificar salud
curl http://localhost:8080/raft/health
```

### 4. Detener el cluster

```bash
./stop-cluster.sh
```

Esto detiene y elimina todos los contenedores, pero **mantiene**:
- Los logs en `./logs/`
- La red `agenda_net` (puedes eliminarla manualmente si quieres)

## Puertos

- **agenda-1**: `8080` (puerto principal para acceso desde el host)
- **agenda-2**: `8082`
- **agenda-3**: `8083`
- **agenda-4**: `8084`

## Acceso

- **UI**: http://localhost:8080/ui/
- **API**: http://localhost:8080/api/
- **Raft Health**: http://localhost:8080/raft/health

## Estructura de Logs

Los logs se guardan en `./logs/`:
```
logs/
├── agenda-1.log
├── agenda-2.log
├── agenda-3.log
└── agenda-4.log
```

## Base de Datos

⚠️ **Importante**: La base de datos se guarda **dentro del contenedor**. Si eliminas el contenedor, perderás los datos.

Si necesitas persistir la base de datos, puedes modificar el script para montar un volumen, pero por defecto se guarda en el contenedor.

## Troubleshooting

### Ver logs de un contenedor específico

```bash
docker logs -f agenda-1
```

### Ver logs desde archivos

```bash
tail -f logs/agenda-1.log
```

### Verificar conectividad entre contenedores

```bash
docker exec agenda-1 curl http://agenda-2:8080/raft/health
```

### Ver qué nodo es el líder

```bash
docker logs agenda-1 | grep -i leader
```

### Reiniciar un nodo específico

```bash
docker restart agenda-1
```

### Ver estado de la red

```bash
docker network inspect agenda_net
```

## Migración desde Docker Swarm

Si tienes un stack de Swarm corriendo:

```bash
# 1. Detener el stack
docker stack rm agenda

# 2. Esperar a que se detenga
sleep 10

# 3. Levantar con el script manual
./start-cluster.sh
```

## Verificación de Replicación

Para verificar que el sistema distribuido funciona correctamente y que los cambios se replican entre nodos:

```bash
./verify-replication.sh
```

Este script:
1. Verifica el estado del cluster y identifica el líder
2. Muestra el estado de replicación RAFT (términos, índices, log entries)
3. Crea un usuario de prueba
4. Verifica que el usuario se replicó en todos los nodos
5. Crea una cita de prueba
6. Verifica que la cita se replicó en todos los nodos

**Nota**: El script usa la API para verificar la replicación, ya que `sqlite3` no está instalado en los contenedores por defecto.

## Verificación Manual

### Ver estado del líder

```bash
# Ver estado de todos los nodos
for port in 8080 8082 8083 8084; do
    echo "Puerto $port:"
    curl -s "http://localhost:${port}/raft/health" | jq
    echo ""
done
```

### Crear y verificar replicación manualmente

```bash
# 1. Crear usuario
curl -X POST http://localhost:8080/register \
  -H "Content-Type: application/json" \
  -d '{"username":"test","email":"test@example.com","password":"test123","name":"Test"}'

# 2. Login y obtener token
TOKEN=$(curl -s -X POST http://localhost:8080/login \
  -H "Content-Type: application/json" \
  -d '{"username":"test","password":"test123"}' | jq -r '.token')

# 3. Crear cita en un nodo (no líder)
curl -X POST http://localhost:8082/api/appointments \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer $TOKEN" \
  -d '{
    "title": "Cita de prueba",
    "description": "Verificar replicación",
    "start": "2025-12-25T10:00:00Z",
    "end": "2025-12-25T11:00:00Z",
    "privacy": "full"
  }'

# 4. Verificar que la cita aparece en todos los nodos
for port in 8080 8082 8083 8084; do
    echo "Puerto $port:"
    curl -s "http://localhost:${port}/api/agenda?start=2025-12-25T00:00:00Z&end=2025-12-26T00:00:00Z" \
      -H "Authorization: Bearer $TOKEN" | jq '.[] | select(.title == "Cita de prueba")'
    echo ""
done
```

## Limpieza completa

Si quieres eliminar todo (contenedores, red, logs):

```bash
# Detener y eliminar contenedores
./stop-cluster.sh

# Eliminar red
docker network rm agenda_net

# Eliminar logs (opcional)
rm -rf logs/
```


