package agendadistribuida

import (
	"encoding/json"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"
)

// StartNotificationReconciler ensures that important notifications are
// eventually present on all nodes by replaying repair.ensure_notification
// operations based on local notification events.
func StartNotificationReconciler(store *Storage, cons Consensus, peers PeerStore) {
	// Reduced interval for faster reconciliation to avoid leader changes interrupting it
	interval := 10 * time.Second
	client := &http.Client{Timeout: 3 * time.Second}
	secret := strings.TrimSpace(os.Getenv("CLUSTER_HMAC_SECRET"))
	if secret == "" {
		Logger().Warn("notification_reconciler_disabled_no_secret")
		return
	}

	type peerState struct {
		since time.Time
	}
	mu := sync.Mutex{}
	perPeer := make(map[string]*peerState)

	type notifPayload struct {
		UserID   string `json:"user_id"`
		Username string `json:"username"` // For ID mapping during reconciliation
		Type     string `json:"type"`
		Payload  string `json:"payload"` // This is the actual notification payload
	}

	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for range ticker.C {
			// CRITICAL: Only the leader executes reconciliation to prevent storms.
			if !cons.IsLeader() {
				continue
			}
			ids := peers.ListPeers()
			for _, id := range ids {
				if id == "" || id == cons.NodeID() {
					continue
				}
				addr := peers.ResolveAddr(id)
				if addr == "" {
					continue
				}

				mu.Lock()
				ps, ok := perPeer[id]
				if !ok {
					ps = &peerState{}
					perPeer[id] = ps
				}
				since := ps.since
				mu.Unlock()

				url := "http://" + addr + "/cluster/local-events/notifications"
				if !since.IsZero() {
					url += "?since=" + since.UTC().Format(time.RFC3339)
				}

				req, err := http.NewRequest(http.MethodGet, url, nil)
				if err != nil {
					Logger().Warn("notification_reconcile_build_request_failed", "peer", id, "err", err)
					continue
				}
				sig := computeHMACSHA256Hex(nil, secret)
				req.Header.Set("X-Cluster-Signature", sig)

				resp, err := client.Do(req)
				if err != nil {
					Logger().Debug("notification_reconcile_request_failed", "peer", id, "err", err)
					continue
				}
				defer resp.Body.Close()

				// Verify HTTP status code before decoding
				if resp.StatusCode < 200 || resp.StatusCode >= 300 {
					Logger().Debug("notification_reconcile_bad_status", "peer", id, "status", resp.StatusCode)
					continue
				}

				// Update LastSeen for successfully contacted peer
				if store != nil && id != "" && id != cons.NodeID() {
					_ = store.UpsertClusterNode(&ClusterNode{
						NodeID:   id,
						Address:  addr,
						Source:   "reconciler",
						LastSeen: time.Now(),
					})
				}

				var events []Event
				if err := json.NewDecoder(resp.Body).Decode(&events); err != nil {
					Logger().Warn("notification_reconcile_decode_failed", "peer", id, "err", err)
					continue
				}

				var maxTS time.Time
				for _, ev := range events {
					if ev.CreatedAt.After(maxTS) {
						maxTS = ev.CreatedAt
					}
					if ev.Payload == "" {
						continue
					}
					var p notifPayload
					if err := json.Unmarshal([]byte(ev.Payload), &p); err != nil {
						Logger().Debug("notification_reconcile_decode_payload_failed", "peer", id, "err", err)
						continue
					}
					if p.Type == "" || p.Payload == "" {
						continue
					}

					// CRITICAL: Map remote user ID to local user ID using username
					// During partitions, the same user may have different IDs in different partitions.
					var localUserID string
					if p.Username != "" {
						if localUser, err := store.GetUserByUsername(p.Username); err == nil && localUser != nil {
							localUserID = localUser.ID
						} else {
							// User not found locally, skip this notification
							Logger().Debug("notification_reconcile_user_not_found", "peer", id, "username", p.Username)
							continue
						}
					} else if p.UserID != "" {
						localUserID = p.UserID
						Logger().Warn("notification_reconcile_no_username", "peer", id, "using_remote_id", p.UserID)
					} else {
						// No user info, skip
						Logger().Debug("notification_reconcile_no_user_info", "peer", id)
						continue
					}

					// If notification already exists locally, skip
					if existingID, err := store.FindNotificationBySignature(localUserID, p.Type, p.Payload); err == nil && existingID != "" {
						continue
					}

					entry, err := BuildEntryRepairEnsureNotification(localUserID, p.Type, p.Payload)
					if err != nil {
						Logger().Warn("notification_reconcile_build_entry_failed", "peer", id, "username", p.Username, "type", p.Type, "err", err)
						continue
					}
					if err := cons.Propose(entry); err != nil {
						Logger().Warn("notification_reconcile_propose_failed", "peer", id, "username", p.Username, "type", p.Type, "err", err)
						continue
					}
					Logger().Debug("notification_reconcile_proposed", "peer", id, "username", p.Username, "type", p.Type)
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
