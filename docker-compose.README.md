# Docker Compose Setup (Sin Docker Swarm)

Este archivo `docker-compose.yml` levanta el cluster de Agenda Distribuida **sin usar Docker Swarm**, evitando así los problemas del balanceador de Swarm.

## Ventajas

- ✅ Evita el balanceador de Swarm que causa problemas con las peticiones POST
- ✅ Cada contenedor tiene su propio puerto publicado directamente
- ✅ Acceso directo desde el host sin problemas de red
- ✅ Más simple de depurar y diagnosticar

## Uso

### 1. Construir la imagen

```bash
docker build -t agenda-distribuida:latest .
```

### 2. Levantar el cluster

```bash
docker-compose up -d
```

### 3. Ver los logs

```bash
# Todos los servicios
docker-compose logs -f

# Un servicio específico
docker-compose logs -f agenda-1
```

### 4. Acceder a la aplicación

- **UI**: http://localhost:8080/ui/
- **API**: http://localhost:8080/api/
- **Raft Health**: http://localhost:8080/raft/health

### 5. Detener el cluster

```bash
docker-compose down
```

### 6. Detener y eliminar volúmenes

```bash
docker-compose down -v
```

## Puertos

- `agenda-1`: Puerto 8080 (principal) y 8081 (alternativo)
- `agenda-2`: Puerto 8082
- `agenda-3`: Puerto 8083
- `agenda-4`: Puerto 8084

## Descubrimiento de Peers

Los nodos se descubren usando la variable de entorno `PEERS` que contiene una lista separada por comas de `hostname:puerto`. En Docker Compose, los nombres de servicio son resolubles por DNS, por lo que cada nodo puede comunicarse con los demás usando sus nombres.

## Migración desde Docker Swarm

Si tienes un stack de Swarm corriendo:

1. Detén el stack:
   ```bash
   docker stack rm agenda
   ```

2. Levanta con docker-compose:
   ```bash
   docker-compose up -d
   ```

3. Verifica que todo funciona:
   ```bash
   curl http://localhost:8080/raft/health
   ```

## Troubleshooting

### Verificar que los contenedores están corriendo

```bash
docker-compose ps
```

### Verificar conectividad entre contenedores

```bash
docker exec -it agenda-1 curl http://agenda-2:8080/raft/health
```

### Ver logs de un nodo específico

```bash
docker-compose logs agenda-1 | grep -i leader
```



