package agendadistribuida

import (
	"encoding/json"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"
)

// StartAppointmentReconciler starts a background goroutine that, when this
// node is leader, periodically pulls appointment creation events from peers
// and proposes Raft entries to ensure missing appointments are recreated.
func StartAppointmentReconciler(store *Storage, cons Consensus, peers PeerStore) {
	// Reduced interval for faster reconciliation to avoid leader changes interrupting it
	interval := 10 * time.Second
	client := &http.Client{Timeout: 3 * time.Second}
	secret := strings.TrimSpace(os.Getenv("CLUSTER_HMAC_SECRET"))
	if secret == "" {
		Logger().Warn("appt_reconciler_disabled_no_secret")
		return
	}

	type peerState struct {
		since time.Time
	}
	mu := sync.Mutex{}
	perPeer := make(map[string]*peerState)

	type apptPayload struct {
		OwnerID              string    `json:"owner_id"`
		OwnerUsername        string    `json:"owner_username"` // For ID mapping during reconciliation
		GroupID              *string   `json:"group_id"`
		GroupName            string    `json:"group_name"`             // For group ID mapping
		GroupCreatorUsername string    `json:"group_creator_username"` // For group ID mapping
		GroupType            GroupType `json:"group_type"`             // For group ID mapping
		Title                string    `json:"title"`
		Description          string    `json:"description"`
		Start                string    `json:"start"`
		End                  string    `json:"end"`
		Privacy              Privacy   `json:"privacy"`
	}

	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for range ticker.C {
			// CRITICAL: Only the leader executes reconciliation to prevent storms.
			if !cons.IsLeader() {
				continue
			}
			// This ensures reconciliation continues even if leadership changes.
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

				url := "http://" + addr + "/cluster/local-events/appointments"
				if !since.IsZero() {
					url += "?since=" + since.UTC().Format(time.RFC3339)
				}

				req, err := http.NewRequest(http.MethodGet, url, nil)
				if err != nil {
					Logger().Warn("appt_reconcile_build_request_failed", "peer", id, "err", err)
					continue
				}
				sig := computeHMACSHA256Hex(nil, secret)
				req.Header.Set("X-Cluster-Signature", sig)

				resp, err := client.Do(req)
				if err != nil {
					Logger().Debug("appt_reconcile_request_failed", "peer", id, "err", err)
					continue
				}
				defer resp.Body.Close()

				// Verify HTTP status code before decoding
				if resp.StatusCode < 200 || resp.StatusCode >= 300 {
					Logger().Debug("appt_reconcile_bad_status", "peer", id, "status", resp.StatusCode)
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
					Logger().Warn("appt_reconcile_decode_failed", "peer", id, "err", err)
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
					var p apptPayload
					if err := json.Unmarshal([]byte(ev.Payload), &p); err != nil {
						continue
					}
					// Parse times
					start, err1 := time.Parse(time.RFC3339, p.Start)
					end, err2 := time.Parse(time.RFC3339, p.End)
					if err1 != nil || err2 != nil {
						continue
					}

					// CRITICAL: Map remote owner ID to local owner ID using username
					// During partitions, the same user may have different IDs in different partitions.
					// We use username/email as the stable identifier.
					var localOwnerID string
					if strings.TrimSpace(p.OwnerUsername) == "" {
						// Without stable identifier we must skip to avoid assigning to wrong user.
						Logger().Warn("appt_reconcile_missing_owner_username", "peer", id, "title", p.Title)
						continue
					}
					if localOwner, err := store.GetUserByUsername(p.OwnerUsername); err == nil && localOwner != nil {
						localOwnerID = localOwner.ID
					} else {
						Logger().Debug("appt_reconcile_owner_not_found", "peer", id, "owner_username", p.OwnerUsername)
						continue
					}

					// CRITICAL: Map remote group ID to local group ID using group name and creator username
					// During partitions, the same group may have different IDs in different partitions.
					var localGroupIDPtr *string
					if p.GroupID != nil && strings.TrimSpace(*p.GroupID) != "" {
						if strings.TrimSpace(p.GroupName) == "" || strings.TrimSpace(p.GroupCreatorUsername) == "" || strings.TrimSpace(string(p.GroupType)) == "" {
							Logger().Warn("appt_reconcile_missing_group_signature", "peer", id, "title", p.Title, "group_id", *p.GroupID)
							continue
						}
						if localCreator, err := store.GetUserByUsername(p.GroupCreatorUsername); err == nil && localCreator != nil {
							if localGroupID, err := store.FindGroupBySignature(p.GroupName, localCreator.ID, p.GroupType); err == nil && strings.TrimSpace(localGroupID) != "" {
								localGroupIDPtr = &localGroupID
							} else {
								Logger().Debug("appt_reconcile_group_not_found", "peer", id, "group_name", p.GroupName, "creator_username", p.GroupCreatorUsername)
								continue
							}
						} else {
							Logger().Debug("appt_reconcile_group_creator_not_found", "peer", id, "creator_username", p.GroupCreatorUsername)
							continue
						}
					}

					// If appointment already exists locally, skip
					if existingID, err := store.FindAppointmentBySignature(localOwnerID, localGroupIDPtr, start, end, p.Title); err == nil && strings.TrimSpace(existingID) != "" {
						continue
					}

					// Reconstruct minimal appointment with LOCAL owner ID and LOCAL group ID
					a := Appointment{
						Title:       p.Title,
						Description: p.Description,
						OwnerID:     localOwnerID, // Use local ID, not remote ID
						Start:       start,
						End:         end,
						Privacy:     p.Privacy,
						OriginNode:  ev.OriginNode,
					}
					if localGroupIDPtr != nil {
						a.GroupID = localGroupIDPtr // Use local group ID, not remote ID
					}

					var entry LogEntry
					if a.GroupID == nil {
						entry, err = BuildEntryApptCreatePersonal(localOwnerID, a) // Use local ID
					} else {
						entry, err = BuildEntryApptCreateGroup(localOwnerID, a) // Use local ID
					}
					if err != nil {
						Logger().Warn("appt_reconcile_build_entry_failed", "peer", id, "title", p.Title, "owner_username", p.OwnerUsername, "err", err)
						continue
					}
					if err := cons.Propose(entry); err != nil {
						Logger().Warn("appt_reconcile_propose_failed", "peer", id, "title", p.Title, "owner_username", p.OwnerUsername, "err", err)
						continue
					}
					Logger().Debug("appt_reconcile_proposed", "peer", id, "title", p.Title, "owner_username", p.OwnerUsername, "group_id", a.GroupID)
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
