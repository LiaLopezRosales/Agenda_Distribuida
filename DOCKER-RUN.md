# Runbook: 7 réplicas sin services/compose (2 hosts)

Prereqs: Docker Swarm solo para redes overlay attachable.

1) Host A
```
docker swarm init
docker network create -d overlay --attachable cluster_net
docker network create -d overlay --attachable client_net
export IMAGE=agenda:latest
export SECRET=changeme

docker run -d --name agenda-A1 --network cluster_net -p 18081:8080 \
  -e NODE_ID=A1 -e PEERS="A1:18081,A2:18082,A3:18083,A4:18084,B1:28081,B2:28082,B3:28083" \
  -e CLUSTER_HMAC_SECRET=$SECRET $IMAGE
docker run -d --name agenda-A2 --network cluster_net -p 18082:8080 \
  -e NODE_ID=A2 -e PEERS="A1:18081,A2:18082,A3:18083,A4:18084,B1:28081,B2:28082,B3:28083" \
  -e CLUSTER_HMAC_SECRET=$SECRET $IMAGE
docker run -d --name agenda-A3 --network cluster_net -p 18083:8080 \
  -e NODE_ID=A3 -e PEERS="A1:18081,A2:18082,A3:18083,A4:18084,B1:28081,B2:28082,B3:28083" \
  -e CLUSTER_HMAC_SECRET=$SECRET $IMAGE
docker run -d --name agenda-A4 --network cluster_net -p 18084:8080 \
  -e NODE_ID=A4 -e PEERS="A1:18081,A2:18082,A3:18083,A4:18084,B1:28081,B2:28082,B3:28083" \
  -e CLUSTER_HMAC_SECRET=$SECRET $IMAGE
```

2) Host B
```
docker swarm join <token-from-init>
export IMAGE=agenda:latest
export SECRET=changeme

docker run -d --name agenda-B1 --network cluster_net -p 28081:8080 \
  -e NODE_ID=B1 -e PEERS="A1:18081,A2:18082,A3:18083,A4:18084,B1:28081,B2:28082,B3:28083" \
  -e CLUSTER_HMAC_SECRET=$SECRET $IMAGE
docker run -d --name agenda-B2 --network cluster_net -p 28082:8080 \
  -e NODE_ID=B2 -e PEERS="A1:18081,A2:18082,A3:18083,A4:18084,B1:28081,B2:28082,B3:28083" \
  -e CLUSTER_HMAC_SECRET=$SECRET $IMAGE
docker run -d --name agenda-B3 --network cluster_net -p 28083:8080 \
  -e NODE_ID=B3 -e PEERS="A1:18081,A2:18082,A3:18083,A4:18084,B1:28081,B2:28082,B3:28083" \
  -e CLUSTER_HMAC_SECRET=$SECRET $IMAGE
```

UI: `http://<hostA>:18081/ui/` o `http://<hostB>:28081/ui/`


