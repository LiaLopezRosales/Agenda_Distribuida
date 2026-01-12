package agendadistribuida

import (
	"encoding/json"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"
)

// StartUserReconciler starts a background goroutine in this node that,
// when leader, periodically pulls user registration audit logs from peers
// and proposes RepairEnsureUser entries for users that are missing locally.
func StartUserReconciler(store *Storage, cons Consensus, peers PeerStore) {
	// Reduced interval for faster reconciliation to avoid leader changes interrupting it
	interval := 10 * time.Second
	client := &http.Client{Timeout: 3 * time.Second}
	secret := strings.TrimSpace(os.Getenv("CLUSTER_HMAC_SECRET"))
	if secret == "" {
		// Without a shared secret we cannot safely call cluster endpoints.
		Logger().Warn("user_reconciler_disabled_no_secret")
		return
	}

	type peerState struct {
		since time.Time
	}
	mu := sync.Mutex{}
	perPeer := make(map[string]*peerState)

	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for range ticker.C {
			// Reconcile from all reachable peers (works on both leaders and followers)
			// This ensures reconciliation continues even if leadership changes.
			// Entries are proposed via Raft and will be applied on all nodes.
			ids := peers.ListPeers()
			for _, id := range ids {
				if id == "" || id == cons.NodeID() {
					continue
				}
				addr := peers.ResolveAddr(id)
				if addr == "" {
					continue
				}
				// Build URL with optional since parameter.
				mu.Lock()
				ps, ok := perPeer[id]
				if !ok {
					ps = &peerState{}
					perPeer[id] = ps
				}
				since := ps.since
				mu.Unlock()

				url := "http://" + addr + "/cluster/local-audit/users"
				if !since.IsZero() {
					url += "?since=" + since.UTC().Format(time.RFC3339)
				}

				req, err := http.NewRequest(http.MethodGet, url, nil)
				if err != nil {
					Logger().Warn("user_reconcile_build_request_failed", "peer", id, "err", err)
					continue
				}
				// Attach HMAC for cluster auth.
				if secret != "" {
					sig := computeHMACSHA256Hex(nil, secret)
					req.Header.Set("X-Cluster-Signature", sig)
				}

				resp, err := client.Do(req)
				if err != nil {
					Logger().Debug("user_reconcile_request_failed", "peer", id, "err", err)
					continue
				}
				defer resp.Body.Close()
				
				// Verify HTTP status code before decoding
				if resp.StatusCode < 200 || resp.StatusCode >= 300 {
					Logger().Debug("user_reconcile_bad_status", "peer", id, "status", resp.StatusCode)
					continue
				}
				
				// Update LastSeen for successfully contacted peer
				// Critical during partitions: reachable peers stay in PeerStore
				if store != nil && id != "" && id != cons.NodeID() {
					_ = store.UpsertClusterNode(&ClusterNode{
						NodeID:   id,
						Address:  addr,
						Source:   "reconciler",
						LastSeen: time.Now(),
					})
				}
				
				var logs []AuditLog
				if err := json.NewDecoder(resp.Body).Decode(&logs); err != nil {
					Logger().Warn("user_reconcile_decode_failed", "peer", id, "err", err)
					continue
				}

				// Process logs: for each, attempt to ensure the user exists via repair.
				var maxTS time.Time
				for _, entry := range logs {
					if entry.OccurredAt.After(maxTS) {
						maxTS = entry.OccurredAt
					}
					// Parse minimal info from payload.
					if entry.Payload == "" {
						continue
					}
					var payload map[string]any
					if err := json.Unmarshal([]byte(entry.Payload), &payload); err != nil {
						continue
					}
					username, _ := payload["username"].(string)
					email, _ := payload["email"].(string)
					displayName, _ := payload["display_name"].(string)
					// Note: user_id is extracted but not used since EnsureUser ignores it
					// and creates a new ID based on username/email matching
					_, _ = payload["user_id"].(float64) // Extract but ignore for now
					if strings.TrimSpace(username) == "" && strings.TrimSpace(email) == "" {
						continue
					}
					// If user already exists locally, skip.
					if username != "" {
						if existing, err := store.GetUserByUsername(username); err == nil && existing != nil {
							continue
						}
					}
					if email != "" {
						if existing, err := store.GetUserByEmail(email); err == nil && existing != nil {
							continue
						}
					}
					// Construct a minimal user; password hash is unknown and left empty.
					u := &User{
						Username:    username,
						Email:       email,
						DisplayName: displayName,
					}
					entryLog, err := BuildEntryRepairEnsureUser(u)
					if err != nil {
						Logger().Warn("user_reconcile_build_entry_failed", "peer", id, "username", username, "email", email, "err", err)
						continue
					}
					if err := cons.Propose(entryLog); err != nil {
						Logger().Warn("user_reconcile_propose_failed", "peer", id, "username", username, "email", email, "err", err)
						continue
					}
					Logger().Debug("user_reconcile_proposed", "peer", id, "username", username, "email", email)
				}

				if !maxTS.IsZero() {
					mu.Lock()
					ps.since = maxTS
					mu.Unlock()
				}
			}
		}
	}()
}
