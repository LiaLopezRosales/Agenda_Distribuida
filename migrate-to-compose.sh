#!/bin/bash

# Script para migrar de Docker Swarm a Docker Compose

set -e

echo "🔄 Migrando de Docker Swarm a Docker Compose..."

# 1. Detener el stack de Swarm si existe
echo "📦 Deteniendo stack de Docker Swarm..."
if docker stack ls | grep -q "agenda"; then
    docker stack rm agenda
    echo "⏳ Esperando a que el stack se detenga completamente..."
    sleep 5
else
    echo "ℹ️  No hay stack de Swarm corriendo"
fi

# 2. Construir la imagen si no existe
echo "🔨 Construyendo imagen..."
if ! docker images | grep -q "agenda-distribuida.*latest"; then
    docker build -t agenda-distribuida:latest .
else
    echo "ℹ️  Imagen ya existe, omitiendo construcción"
fi

# 3. Levantar con docker-compose
echo "🚀 Levantando cluster con docker-compose..."
docker-compose up -d

# 4. Esperar a que los servicios estén listos
echo "⏳ Esperando a que los servicios estén listos..."
sleep 10

# 5. Verificar salud
echo "🏥 Verificando salud de los servicios..."
for i in {1..4}; do
    echo -n "  agenda-$i: "
    if docker-compose exec -T agenda-$i curl -sf http://localhost:8080/raft/health > /dev/null 2>&1; then
        echo "✅ OK"
    else
        echo "❌ No responde"
    fi
done

# 6. Verificar desde el host
echo "🌐 Verificando acceso desde el host..."
if curl -sf http://localhost:8080/raft/health > /dev/null 2>&1; then
    echo "✅ Acceso desde host: OK"
else
    echo "⚠️  Acceso desde host: No responde aún (puede tardar unos segundos más)"
fi

echo ""
echo "✅ Migración completada!"
echo ""
echo "📋 Comandos útiles:"
echo "  - Ver logs: docker-compose logs -f"
echo "  - Ver estado: docker-compose ps"
echo "  - Detener: docker-compose down"
echo "  - Acceder a UI: http://localhost:8080/ui/"
echo ""



