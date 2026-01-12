package agendadistribuida

import (
	"encoding/json"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"
)

// StartInvitationReconciler replays invitation status changes (accept/reject)
// based on local events from all peers by reproposing the corresponding Raft
// operations. OpInvitationAccept/Reject are idempotent at apply time.
func StartInvitationReconciler(store *Storage, cons Consensus, peers PeerStore) {
	// Reduced interval for faster reconciliation to avoid leader changes interrupting it
	interval := 10 * time.Second
	client := &http.Client{Timeout: 3 * time.Second}
	secret := strings.TrimSpace(os.Getenv("CLUSTER_HMAC_SECRET"))
	if secret == "" {
		Logger().Warn("invitation_reconciler_disabled_no_secret")
		return
	}

	type peerState struct {
		since time.Time
	}
	mu := sync.Mutex{}
	perPeer := make(map[string]*peerState)

	type invitationPayload struct {
		AppointmentID           int64      `json:"appointment_id"`
		UserID                  int64      `json:"user_id"`
		Username                string     `json:"username"` // For user ID mapping
		Status                  ApptStatus `json:"status"`
		ApptOwnerUsername       string     `json:"appt_owner_username"`       // For appointment ID mapping
		ApptGroupID             *int64     `json:"appt_group_id"`              // For appointment ID mapping
		ApptGroupName           string     `json:"appt_group_name"`           // For appointment ID mapping
		ApptGroupCreatorUsername string    `json:"appt_group_creator_username"` // For appointment ID mapping
		ApptTitle               string     `json:"appt_title"`                // For appointment ID mapping
		ApptStart               string     `json:"appt_start"`                // For appointment ID mapping
		ApptEnd                 string     `json:"appt_end"`                   // For appointment ID mapping
	}

	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for range ticker.C {
			// Reconcile from all reachable peers (works on both leaders and followers)
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

				url := "http://" + addr + "/cluster/local-events/invitations"
				if !since.IsZero() {
					url += "?since=" + since.UTC().Format(time.RFC3339)
				}

				req, err := http.NewRequest(http.MethodGet, url, nil)
				if err != nil {
					Logger().Warn("invitation_reconcile_build_request_failed", "peer", id, "err", err)
					continue
				}
				sig := computeHMACSHA256Hex(nil, secret)
				req.Header.Set("X-Cluster-Signature", sig)

				resp, err := client.Do(req)
				if err != nil {
					Logger().Debug("invitation_reconcile_request_failed", "peer", id, "err", err)
					continue
				}
				defer resp.Body.Close()
				
				// Verify HTTP status code before decoding
				if resp.StatusCode < 200 || resp.StatusCode >= 300 {
					Logger().Debug("invitation_reconcile_bad_status", "peer", id, "status", resp.StatusCode)
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
					Logger().Warn("invitation_reconcile_decode_failed", "peer", id, "err", err)
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
					var p invitationPayload
					if err := json.Unmarshal([]byte(ev.Payload), &p); err != nil {
						continue
					}
					// CRITICAL: Map remote user ID to local user ID using username
					// During partitions, the same user may have different IDs in different partitions.
					var localUserID int64
					if p.Username != "" {
						if localUser, err := store.GetUserByUsername(p.Username); err == nil && localUser != nil {
							localUserID = localUser.ID
						} else {
							// User not found locally, skip this invitation
							Logger().Debug("invitation_reconcile_user_not_found", "peer", id, "username", p.Username)
							continue
						}
					} else if p.UserID != 0 {
						// Fallback: try to use remote ID if username not available (backward compatibility)
						localUserID = p.UserID
						Logger().Warn("invitation_reconcile_no_username", "peer", id, "using_remote_id", p.UserID)
					} else {
						// No user info, skip
						continue
					}
					
					// CRITICAL: Map remote appointment ID to local appointment ID using signature
					// During partitions, the same appointment may have different IDs in different partitions.
					var localAppointmentID int64
					if p.ApptTitle != "" && p.ApptStart != "" && p.ApptEnd != "" {
						// Parse appointment times
						apptStart, err1 := time.Parse(time.RFC3339, p.ApptStart)
						apptEnd, err2 := time.Parse(time.RFC3339, p.ApptEnd)
						if err1 != nil || err2 != nil {
							Logger().Debug("invitation_reconcile_invalid_appt_times", "peer", id, "err1", err1, "err2", err2)
							continue
						}
						
						// Map owner username to local owner ID
						var localOwnerID int64
						if p.ApptOwnerUsername != "" {
							if localOwner, err := store.GetUserByUsername(p.ApptOwnerUsername); err == nil && localOwner != nil {
								localOwnerID = localOwner.ID
							} else {
								Logger().Debug("invitation_reconcile_appt_owner_not_found", "peer", id, "owner_username", p.ApptOwnerUsername)
								continue
							}
						} else {
							// Cannot map appointment without owner username
							Logger().Debug("invitation_reconcile_no_appt_owner_username", "peer", id)
							continue
						}
						
						// Map group ID if it's a group appointment
						var localGroupIDPtr *int64
						if p.ApptGroupID != nil && *p.ApptGroupID != 0 {
							if p.ApptGroupName != "" && p.ApptGroupCreatorUsername != "" {
								if localGroupCreator, err := store.GetUserByUsername(p.ApptGroupCreatorUsername); err == nil && localGroupCreator != nil {
									if localGroupID, err := store.FindGroupBySignature(p.ApptGroupName, localGroupCreator.ID, "hierarchical"); err == nil && localGroupID != 0 {
										localGroupIDPtr = &localGroupID
									} else {
										Logger().Debug("invitation_reconcile_appt_group_not_found", "peer", id, "group_name", p.ApptGroupName)
										continue
									}
								} else {
									Logger().Debug("invitation_reconcile_appt_group_creator_not_found", "peer", id, "creator_username", p.ApptGroupCreatorUsername)
									continue
								}
							} else {
								// Fallback: try remote group ID
								if _, err := store.GetGroupByID(*p.ApptGroupID); err == nil {
									localGroupIDPtr = p.ApptGroupID
									Logger().Warn("invitation_reconcile_no_appt_group_info", "peer", id, "using_remote_group_id", *p.ApptGroupID)
								} else {
									Logger().Debug("invitation_reconcile_appt_group_id_not_found", "peer", id, "group_id", *p.ApptGroupID)
									continue
								}
							}
						}
						
						// Find appointment by signature
						if foundID, err := store.FindAppointmentBySignature(localOwnerID, localGroupIDPtr, apptStart, apptEnd, p.ApptTitle); err == nil && foundID != 0 {
							localAppointmentID = foundID
						} else {
							// Appointment not found locally, skip this invitation (will be retried when appointment is reconciled)
							Logger().Debug("invitation_reconcile_appt_not_found", "peer", id, "title", p.ApptTitle)
							continue
						}
					} else if p.AppointmentID != 0 {
						// Fallback: try to use remote ID if appointment info not available (backward compatibility)
						// But verify appointment exists locally
						if _, err := store.GetAppointmentByID(p.AppointmentID); err == nil {
							localAppointmentID = p.AppointmentID
							Logger().Warn("invitation_reconcile_no_appt_info", "peer", id, "using_remote_appt_id", p.AppointmentID)
						} else {
							Logger().Debug("invitation_reconcile_appt_id_not_found", "peer", id, "appointment_id", p.AppointmentID)
							continue
						}
					} else {
						// No appointment info, skip
						continue
					}
					
					// If participant already has this status locally, skip
					if existing, err := store.GetParticipantByAppointmentAndUser(localAppointmentID, localUserID); err == nil && existing != nil {
						if existing.Status == p.Status {
							continue
						}
					}

					entry, err := BuildEntryInvitationStatus(localAppointmentID, localUserID, p.Status)
					if err != nil {
						Logger().Warn("invitation_reconcile_build_entry_failed", "peer", id, "appointment_id", localAppointmentID, "username", p.Username, "status", p.Status, "err", err)
						continue
					}
					if err := cons.Propose(entry); err != nil {
						Logger().Warn("invitation_reconcile_propose_failed", "peer", id, "appointment_id", localAppointmentID, "username", p.Username, "status", p.Status, "err", err)
						continue
					}
					Logger().Debug("invitation_reconcile_proposed", "peer", id, "appointment_id", localAppointmentID, "username", p.Username, "status", p.Status)
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
