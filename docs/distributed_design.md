# Distributed Design Dossier

This dossier covers the delivery-2 requirements for the distributed agenda: architecture, roles, processes, communication, coordination, naming, consistency, replication, fault tolerance, and security.

## 1. Arquitectura

- **Topología:** siete contenedores idénticos desplegados en un clúster Docker Swarm sobre al menos dos hosts físicos. Cada réplica empaqueta API HTTP, WebSockets, almacenamiento SQLite y el motor de consenso (Raft).
- **Servicios principales:** API HTTP (`http_handlers_scaffold.go`), consenso (`consensus.go` + `raft_http.go`), descubrimiento (`discovery.go`), almacenamiento (`storage.go`), y mensajería WebSocket (`websocket.go`).
- **Indiferencia frente a una solución centralizada:** cualquier nodo puede aceptar lecturas; los writes se redirigen automático al líder actual (`LeaderWriteMiddleware`), por lo que un cliente no necesita conocer una URL especial.

## 2. Organización del sistema

- **Roles:** usuarios finales (consumen UI/API), nodos worker (ejecutan contenedores), nodos manager (orquestan Swarm), líder del clúster (coordina consenso) y seguidores.
- **Distribución de servicios:** todos los pods sirven la API completa; el equilibrio se logra vía ingress y Docker DNS. Las responsabilidades “fortes” (elecciones, replicación) recaen en el líder; el resto continúa operando como seguidores.
- **Redes:** `agenda_net` (overlay Swarm) conecta a los nodos; una red de frontend (ingress) expone el puerto 8080 a clientes y UI.

## 3. Procesos

- **Servicios en ejecución:** (1) Servidor HTTP/API, (2) Motor Raft + heartbeats, (3) Gestor de descubrimiento dual (Docker DNS + gossip), (4) Gestor de WebSockets, (5) Trabajador de auditoría/observabilidad.
- **Patrones de concurrencia:** goroutines + canales (Go), timers para heartbeats y elecciones, `sync.Mutex/RWMutex` para el estado del consenso.
- **Agrupación:** todos los procesos viven en el mismo contenedor, simplificando despliegue; Swarm escala los contenedores horizontalmente para alcanzar los siete nodos requeridos.

## 4. Comunicación

- **Cliente ↔ Servidor:** REST JSON (mux) y WebSockets para notificaciones. TLS opcional (`TLS_CERT_FILE/TLS_KEY_FILE`). JWT protege las rutas `/api/*`.
- **Servidor ↔ Servidor:** RPC HTTP (AppendEntries/RequestVote) firmado mediante `X-Cluster-Signature` (HMAC-SHA256). Descubrimiento se hace vía `/cluster/join` + Docker DNS.
- **Comunicación interna:** canales Go para WebSockets (`websocket.go`) y colas en memoria; SQLite como bus de eventos persistente (`events`/`audit_logs`).

## 5. Coordinación

- **Sincronización:** Raft asegura una única secuencia de comandos (Create/Update/Delete de citas) y mantiene `commitIndex/lastApplied` replicados.
- **Acceso exclusivo:** mutaciones pasan por el líder gracias al middleware de redirección; los followers responden HTTP 307 hacia el líder.
- **Toma de decisiones distribuida:** elecciones Raft (votos, términos, quorum) + registro de auditoría (`audit.go`) que evidencia cada transición de rol.

## 6. Nombrado y localización

- **Identificación:** `NODE_ID=agenda-{{.Task.Slot}}` + `ADVERTISE_ADDR={{.Task.Name}}:8080`.
- **Ubicación:** Tabla `cluster_nodes` guarda `node_id`, `address`, `source`, `last_seen`. El discovery manager combina:
  - `SWARM_SERVICE_NAME` → consultas DNS `tasks.<service>`.
  - `DISCOVERY_SEEDS` → HTTP gossip hacia `/cluster/join`.
- **Resolución:** `EnvPeerStore` publica snapshots a `ConsensusImpl` y al middleware para redirecciones.

## 7. Consistencia y replicación

- **Distribución de datos:** cada nodo mantiene SQLite local con la misma estructura; los commits Raft replican eventos deterministas (`raft_apply.go`).
- **Replicación:** entradas del log contienen `event_id` + `payload`. La tabla `raft_applied` brinda idempotencia tras reinicios.
- **Confiabilidad:** el follower valida `prev_log_index/term`, trunca conflictos (`truncateLogFrom`) y solo aplica comandos una vez registrados como comprometidos.

## 8. Tolerancia a fallos

- **Respuesta a errores:** watchdogs de heartbeats detectan líderes caídos y disparan elecciones. `RecordAudit` deja rastro en `audit_logs` para cada transición.
- **Nivel esperado:** soporte para fallas de hasta dos nodos (quorum ≥ 4 de 7). La política Swarm `max_replicas_per_node=4` distribuye réplicas para sobrevivir a la caída de un host completo.
- **Reincorporación:** nodos que regresan ejecutan sincronización incremental (`/cluster/nodes` + AppendEntries) y marcan `last_seen`.

## 9. Seguridad

- **Comunicación:** HMAC obligatorio para RPC de consenso y cluster; TLS habilitable para todo el frontdoor. Docker overlay evita exposición directa de puertos internos.
- **Diseño:** auditoría estructurada (`audit_logs`, `/api/admin/audit/logs`) permite rastrear operaciones críticas; tokens segregan privilegios.
- **Autenticación y autorización:** JWT protege rutas, `AUDIT_API_TOKEN` protege el endpoint de auditoría, y `CLUSTER_HMAC_SECRET` encapsula la confianza entre nodos.

---

Los anexos adicionales incluyen:

- `docs/networking.md`: contratos de RPC y requisitos de firma/TLS.
- `deploy/docker-stack.yml`: manifiesto Swarm para cumplir con la topología de siete contenedores.





