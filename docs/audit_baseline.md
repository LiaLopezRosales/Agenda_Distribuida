# Baseline Audit — Distributed Agenda (2025-11-24)

This document captures the current state of the centralized agenda prior to expanding the distributed features. It summarizes which components are production-ready and which ones still require work for Delivery 1.

## Runtime Entry Point

- `cmd/server/main.go` wires SQLite storage, the WebSocket manager, the service layer, and the HTTP API generated in `http_handlers_scaffold.go`.
- The binary already boots the Raft scaffolding (`ConsensusImpl`) and injects it into the appointment service, but the cluster metadata (peer IDs, NODE_ID) is still loaded from environment variables only.

## HTTP & Service Layer

- `http_handlers_scaffold.go` exposes REST endpoints for auth, groups, appointments, notifications, and profile updates, all delegating to the concrete services defined in `services.go`.
- The legacy `handlers.go` file still contains Storage-centric handlers (direct SQL access, duplicate validation logic). The server no longer registers them, but the duplication introduces drift and makes it unclear which handler set is canonical.
- Business rules inside `services.go` (auth, groups, appointments, agenda filtering, notifications) are mostly implemented. However, none of the services emit structured logs, and only the appointment service is partially wired to Raft.

## Persistence Layer

- `storage.go` implements repositories for users, groups, appointments, participants, notifications, events, and the Raft metadata/log tables. All reads/writes occur on a single SQLite file (`agenda.db`) with no replication or backups.
- There is no separation between the application tables and operational telemetry (logs/audits). Error cases are returned upward but not captured anywhere for later inspection.

## WebSocket & Notifications

- `websocket.go` maintains per-user connection pools and exposes helpers to broadcast notifications. It trusts whatever auth wrapper invokes `ServeWS`; there is no message-level authorization or tracing.
- Notification workflows in `services.go` emit events for appointment invitations/updates, but they are not persisted as part of a durable audit log beyond the existing `notifications` table.

## Consensus & Replication

- `consensus.go`, `raft_apply.go`, and `domain_propose.go` provide a minimal Raft-like flow (leader election, append entries, log application). Notable gaps:
  - Log consistency is not validated (`TODO` around `prevLog` checks); followers append blindly and can diverge.
  - No log compaction, snapshotting, or state transfer when a node falls behind.
  - Membership management is static—there is no API to add/remove peers, and quorum size equals the hard-coded `PEERS` list.
  - Only personal appointment create/update/delete operations are expressed as log entries; group appointments, participants, notifications, and profile changes bypass the consensus path.

## Discovery & Cluster Health

- `EnvPeerStore` only understands peers declared via the `PEERS` environment variable. Addresses default to `id` unless explicitly overridden with `SetAddr`, and there is no persistence.
- `heartbeat.go` simply polls `/raft/health` every 2 seconds to cache the leader ID. There is no gossip, DNS lookup, or multi-strategy discovery yet.

## Security & Networking

- `raft_http.go` exposes `/raft/*` endpoints guarded by an optional HMAC header. If `CLUSTER_HMAC_SECRET` is unset, every request is accepted, leaving the cluster unauthenticated.
- Client-facing HTTP routes rely on JWTs, but transport security (TLS) is not configured anywhere. Inter-node traffic (append entries, request vote) occurs over plain HTTP.

## Observability

- Across the codebase (`cmd/server/main.go`, `consensus.go`, `websocket.go`, etc.) the standard `log` package is used sparingly, and many branches swallow errors silently.
- There is no centralized log registry or structured correlation ID that links HTTP requests, consensus decisions, and database mutations.

## Key Gaps to Close for Delivery 1

1. **Canonical HTTP stack:** remove or adapt the legacy handlers to avoid duplicate business logic and ensure the service layer is the single integration point.
2. **Observability:** introduce structured logging plus an auditable event log for critical operations (auth, group mutations, consensus transitions, storage writes).
3. **Discovery & membership:** implement runtime peer discovery (Docker DNS + gossip/registry) and persist membership state so nodes can rejoin automatically.
4. **Consensus completeness:** finish Raft log matching, snapshots, membership changes, and cover all mutating domain commands.
5. **Security hardening:** require cluster HMAC (or TLS mutual auth), and clarify how JWT-protected APIs are exposed over HTTPS.
6. **Deployment artifacts:** define Docker/Swarm manifests that codify the expected seven-container topology across two hosts.

