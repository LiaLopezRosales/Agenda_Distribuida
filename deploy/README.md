# Swarm Deployment Guide

This stack definition spins up seven replicas of the agenda service across a Docker Swarm cluster with at least two worker nodes.

## Prerequisites

1. Initialize a Swarm on the first manager: `docker swarm init`.
2. Join at least one additional host as a worker so replicas can be distributed.
3. Build and push the application image from the repository root:
   ```bash
   docker build -t agenda-distribuida:latest .
   docker save agenda-distribuida:latest | ssh worker1 docker load
   ```
   (Alternatively push to a registry accessible to every node.)

## Configuration

1. Copy `deploy/stack.env.example` to `deploy/stack.env` and edit the variables:
   - `CLUSTER_HMAC_SECRET`: long random string shared by every node.
   - `AUDIT_API_TOKEN`: token required to access `/api/admin/audit/logs`.
   - `DISCOVERY_SEEDS`: optional comma-separated list of manager addresses to accelerate gossip discovery.
2. The stack file sets:
   - `NODE_ID=agenda-{{.Task.Slot}}` to give every replica a stable ID.
   - `SWARM_SERVICE_NAME=agenda` so Docker DNS discovery (`tasks.agenda`) feeds the discovery manager.
   - Persistent state under `/data/agenda.db` (backed by the `agenda_data` volume).

## Deploy

```bash
docker network create --driver overlay --attachable agenda_net || true
docker stack deploy -c deploy/docker-stack.yml --with-registry-auth agenda
```

Use `docker service ps agenda_agenda` to verify that seven tasks are running. The placement policy limits replicas per node to encourage spreading across hosts.

## Operational Notes

- Every replica advertises itself via Docker DNS (`tasks.agenda`) and gossips through `/cluster/join`, satisfying the dual discovery requirement.
- TLS can be enabled by mounting cert/key files and setting `TLS_CERT_FILE/TLS_KEY_FILE` in `stack.env`.
- Logs and audits are persisted on each node's `/data` volume and can be queried via `/api/admin/audit/logs` using the configured token.


