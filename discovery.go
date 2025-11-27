package agendadistribuida

import (
	"bytes"
	"context"
	"encoding/json"
	"net"
	"net/http"
	"os"
	"strings"
	"time"
)

type DiscoveryManager struct {
	store         *Storage
	peers         *EnvPeerStore
	localID       string
	advertiseAddr string
	dnsName       string
	dnsPort       string
	swarmService  string
	swarmPort     string
	seeds         []string
	stop          chan struct{}
	httpClient    *http.Client
	maxPeerAge    time.Duration
}

func NewDiscoveryManager(store *Storage, peers *EnvPeerStore, localID, advertiseAddr string) *DiscoveryManager {
	return &DiscoveryManager{
		store:         store,
		peers:         peers,
		localID:       localID,
		advertiseAddr: advertiseAddr,
		dnsName:       os.Getenv("DISCOVERY_DNS_NAME"),
		dnsPort:       fallback(os.Getenv("DISCOVERY_DNS_PORT"), "8080"),
		swarmService:  os.Getenv("SWARM_SERVICE_NAME"),
		swarmPort:     fallback(os.Getenv("SWARM_SERVICE_PORT"), "8080"),
		seeds:         parseCSV(os.Getenv("DISCOVERY_SEEDS")),
		stop:          make(chan struct{}),
		httpClient:    &http.Client{Timeout: 2 * time.Second},
		maxPeerAge:    2 * time.Minute,
	}
}

func (d *DiscoveryManager) Start() {
	// Register local node
	_ = d.store.UpsertClusterNode(&ClusterNode{
		NodeID:   d.localID,
		Address:  d.advertiseAddr,
		Source:   "local",
		LastSeen: time.Now(),
	})
	go d.syncLoop()
	if d.dnsName != "" {
		go d.dnsLoop()
	}
	if d.swarmService != "" {
		go d.dockerDNSLoop()
	}
	if len(d.seeds) > 0 {
		go d.seedLoop()
	}
}

func (d *DiscoveryManager) Stop() {
	close(d.stop)
}

func (d *DiscoveryManager) syncLoop() {
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-d.stop:
			return
		case <-ticker.C:
			d.updatePeersFromStorage()
		}
	}
}

func (d *DiscoveryManager) dnsLoop() {
	ticker := time.NewTicker(15 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-d.stop:
			return
		case <-ticker.C:
			hosts, err := net.LookupHost(d.dnsName)
			if err != nil {
				Logger().Debug("dns_discovery_failed", "err", err, "name", d.dnsName)
				continue
			}
			for _, host := range hosts {
				addr := net.JoinHostPort(host, d.dnsPort)
				if addr == d.advertiseAddr {
					continue
				}
				nodeID := "dns:" + addr
				_ = d.store.UpsertClusterNode(&ClusterNode{
					NodeID:   nodeID,
					Address:  addr,
					Source:   "dns",
					LastSeen: time.Now(),
				})
			}
		}
	}
}

func (d *DiscoveryManager) dockerDNSLoop() {
	name := "tasks." + d.swarmService
	ticker := time.NewTicker(20 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-d.stop:
			return
		case <-ticker.C:
			ips, err := net.LookupIP(name)
			if err != nil {
				Logger().Debug("docker_dns_lookup_failed", "service", d.swarmService, "err", err)
				continue
			}
			for _, ip := range ips {
				addr := net.JoinHostPort(ip.String(), d.swarmPort)
				if addr == d.advertiseAddr {
					continue
				}
				nodeID := "docker:" + addr
				_ = d.store.UpsertClusterNode(&ClusterNode{
					NodeID:   nodeID,
					Address:  addr,
					Source:   "docker-dns",
					LastSeen: time.Now(),
				})
			}
		}
	}
}

func (d *DiscoveryManager) seedLoop() {
	ticker := time.NewTicker(25 * time.Second)
	defer ticker.Stop()
	d.announceToSeeds()
	for {
		select {
		case <-d.stop:
			return
		case <-ticker.C:
			d.announceToSeeds()
		}
	}
}

func (d *DiscoveryManager) announceToSeeds() {
	payload := map[string]string{
		"node_id": d.localID,
		"address": d.advertiseAddr,
		"source":  "gossip",
	}
	body, _ := json.Marshal(payload)
	for _, seed := range d.seeds {
		if seed == "" {
			continue
		}
		url := ensureHTTP(seed) + "/cluster/join"
		req, _ := http.NewRequest(http.MethodPost, url, bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		if secret := os.Getenv("CLUSTER_HMAC_SECRET"); secret != "" {
			req.Header.Set("X-Cluster-Signature", computeHMACSHA256Hex(body, secret))
		}
		resp, err := d.httpClient.Do(req)
		if err != nil {
			Logger().Debug("seed_join_failed", "seed", seed, "err", err)
			continue
		}
		var out struct {
			Nodes []ClusterNode `json:"nodes"`
		}
		if err := json.NewDecoder(resp.Body).Decode(&out); err == nil {
			for _, node := range out.Nodes {
				_ = d.store.UpsertClusterNode(&ClusterNode{
					NodeID:   node.NodeID,
					Address:  node.Address,
					Source:   "gossip",
					LastSeen: time.Now(),
				})
			}
		}
		resp.Body.Close()
	}
}

func (d *DiscoveryManager) updatePeersFromStorage() {
	nodes, err := d.store.ListClusterNodes()
	if err != nil {
		Logger().Warn("cluster_nodes_list_failed", "err", err)
		return
	}
	now := time.Now()
	snapshot := make(map[string]string)
	for _, node := range nodes {
		if node.NodeID == d.localID || node.Address == d.advertiseAddr {
			continue
		}
		if now.Sub(node.LastSeen) > d.maxPeerAge {
			continue
		}
		addr := node.Address
		if addr == "" {
			addr = node.NodeID
		}
		snapshot[node.NodeID] = addr
	}
	d.peers.SetSnapshot(snapshot)
	RecordAudit(context.Background(), AuditLevelInfo, "cluster", "peers_synced", "peer snapshot refreshed", map[string]any{
		"peer_count": len(snapshot),
	})
}

func fallback(val, def string) string {
	if strings.TrimSpace(val) == "" {
		return def
	}
	return val
}

func parseCSV(raw string) []string {
	if raw == "" {
		return nil
	}
	chunks := strings.Split(raw, ",")
	var out []string
	for _, c := range chunks {
		c = strings.TrimSpace(c)
		if c != "" {
			out = append(out, c)
		}
	}
	return out
}

func ensureHTTP(addr string) string {
	if strings.HasPrefix(addr, "http://") || strings.HasPrefix(addr, "https://") {
		return addr
	}
	return "http://" + addr
}
