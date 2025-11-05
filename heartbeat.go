package agendadistribuida

import (
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
						continue
					}
					var h struct {
						NodeID   string `json:"node_id"`
						IsLeader bool   `json:"is_leader"`
					}
					_ = json.NewDecoder(resp.Body).Decode(&h)
					resp.Body.Close()
					if h.IsLeader {
						ps.SetLeader(id)
						break
					}
				}
			}
		}
	}()
}
