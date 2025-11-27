package agendadistribuida

import (
	"context"
	"encoding/json"
	"net/http"
	"time"
)

// StartHeartbeats polls peers' /raft/health to discover current leader.
// It's a best-effort mechanism until full AppendEntries heartbeats are implemented.
func StartHeartbeats(ps *EnvPeerStore, stopCh <-chan struct{}) {
	go func() {
		ticker := time.NewTicker(2 * time.Second)
		defer ticker.Stop()
		client := &http.Client{Timeout: 1200 * time.Millisecond}
		for {
			select {
			case <-stopCh:
				return
			case <-ticker.C:
				for _, id := range ps.ListPeers() {
					addr := ps.ResolveAddr(id)
					resp, err := client.Get("http://" + addr + "/raft/health")
					if err != nil {
						Logger().Debug("heartbeat_peer_unreachable", "peer_id", id, "err", err)
						continue
					}
					var h struct {
						NodeID   string `json:"node_id"`
						IsLeader bool   `json:"is_leader"`
					}
					_ = json.NewDecoder(resp.Body).Decode(&h)
					resp.Body.Close()
					if h.IsLeader {
						prev := ps.GetLeader()
						ps.SetLeader(id)
						if prev != id {
							Logger().Info("heartbeat_leader_detected", "leader_id", id)
							RecordAudit(context.Background(), AuditLevelInfo, "cluster", "leader_detected", "leader discovered via heartbeat", map[string]any{
								"leader_id": id,
							})
						}
						break
					}
				}
			}
		}
	}()
}
