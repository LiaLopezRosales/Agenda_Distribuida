# Networking & Security Contracts

This document summarizes the node-to-node RPC contracts, the security mechanisms that protect them, and the knobs required to operate the agenda cluster safely across Docker Swarm.

## Cluster RPC Surface

All inter-node requests **must** include an `X-Cluster-Signature` header that contains an HMAC-SHA256 checksum of the HTTP body. The shared key is supplied via the mandatory `CLUSTER_HMAC_SECRET` environment variable. Requests without the header or with an invalid signature are rejected with `401`.

| Endpoint | Method | Purpose | Body |
| --- | --- | --- | --- |
| `/raft/request-vote` | `POST` | Casts an election vote | `{"term":<int>,"candidate_id":"node-1","last_log_index":12,"last_log_term":4}` |
| `/raft/append-entries` | `POST` | Replicates log batches and commits indices | `{"term":4,"leader_id":"node-1","prev_log_index":11,"prev_log_term":4,"entries":[...],"leader_commit":10}` |
| `/cluster/join` | `POST` | Adds/refreshes peer metadata | `{"node_id":"docker:10.0.0.5:8080","address":"10.0.0.5:8080","source":"docker-dns"}` |
| `/cluster/leave` | `POST` | Removes a peer | `{"node_id":"node-4"}` |
| `/cluster/nodes` | `GET` | Returns the current peer snapshot | none |

Responses follow the Go structs declared in `interfaces.go`. A successful `append-entries` reply includes `{ "term": <int>, "success": true, "match_index": <int> }`.

## TLS & Client-Facing APIs

The public REST+WebSocket API listens on `HTTP_ADDR` (default `:8080`). When both `TLS_CERT_FILE` and `TLS_KEY_FILE` are set, the server automatically enables TLS for every route (`/api/*`, `/ws`, `/ui/*`). If the variables are unset, the process refuses to serve cluster RPCs but still allows HTTP for local development.

JWT-based user authentication continues to protect `/api/*`. Additionally, `/api/admin/audit/logs` requires the `X-Audit-Token` header whose shared secret lives in `AUDIT_API_TOKEN`.

## Discovery & Docker DNS

Each node should export:

- `SWARM_SERVICE_NAME`: the Swarm service name (e.g., `agenda`). The discovery manager resolves `tasks.<service>` to enumerate replica IPs.
- `SWARM_SERVICE_PORT`: port exposed by the containers (`8080` by default).
- `DISCOVERY_DNS_NAME`: optional static DNS name (e.g., load balancer or Consul record).
- `DISCOVERY_SEEDS`: comma-separated `host:port` list used for the HTTP gossip joiner.

These dual strategies satisfy the requirement for Docker DNS discovery plus a second method based on gossip/seed registries.



