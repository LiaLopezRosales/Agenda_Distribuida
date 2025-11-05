# Pruebas de tolerancia a fallos (7 réplicas)

## Descubrir líder
```
for p in 18081 18082 18083 18084; do curl -s http://HOST_A:$p/raft/health; echo; done
for p in 28081 28082 28083; do curl -s http://HOST_B:$p/raft/health; echo; done
```

## Caso 1: Caída del líder (1 contenedor)
1. Identifica líder con `/raft/health` (is_leader=true)
2. `docker stop <container-del-lider>`
3. Espera ~3-5s, vuelve a consultar `/raft/health` hasta ver nuevo líder
4. Crea una cita personal vía `POST /api/appointments` en un follower: debe redirigir (`307`) al nuevo líder y crearla

## Caso 2: Caen 2 contenedores adicionales (total 3 caídos)
1. Detén dos contenedores más en el host con 3 réplicas (B1..B3)
2. Sistema debe mantener quórum (4/7) si los 3 caídos están en Host B
3. Crea/actualiza/elimina una cita; valida consistencia vía GET `/api/agenda` en múltiples nodos

## Caso 3: Partición temporal de un follower
1. Bloquea temporalmente la red de un follower (iptables o `docker network disconnect`)
2. Realiza escrituras
3. Reincorpora el follower; verifica que aplica entradas comprometidas (agenda consistente)

## Comandos ejemplo
```
# Crear cita
curl -i -X POST http://HOST_A:18081/api/appointments \
  -H "Authorization: Bearer $TOKEN" -H "Content-Type: application/json" \
  -d '{"title":"Demo","description":"raft","start":"2025-01-01T10:00:00Z","end":"2025-01-01T11:00:00Z","privacy":"full"}'

# Agenda del usuario
curl -s 'http://HOST_B:28081/api/agenda?start=2025-01-01T00:00:00Z&end=2025-01-02T00:00:00Z' -H "Authorization: Bearer $TOKEN"
```

Notas:
- Con 2 hosts (4+3), la caída de Host A (4) elimina quórum (3/7 < 4): el sistema entra en solo-lectura/indisponible para writes.
- La seguridad HMAC para RPC se puede activar luego; actualmente los endpoints de consenso están abiertos en red de clúster.

