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
	interval := 30 * time.Second
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
		AppointmentID int64      `json:"appointment_id"`
		UserID        int64      `json:"user_id"`
		Status        ApptStatus `json:"status"`
	}

	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for range ticker.C {
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
					if p.AppointmentID == 0 || p.UserID == 0 {
						continue
					}
					// If participant already has this status locally, skip
					if existing, err := store.GetParticipantByAppointmentAndUser(p.AppointmentID, p.UserID); err == nil && existing != nil {
						if existing.Status == p.Status {
							continue
						}
					}

					entry, err := BuildEntryInvitationStatus(p.AppointmentID, p.UserID, p.Status)
					if err != nil {
						Logger().Warn("invitation_reconcile_build_entry_failed", "peer", id, "err", err)
						continue
					}
					if err := cons.Propose(entry); err != nil {
						Logger().Warn("invitation_reconcile_propose_failed", "peer", id, "err", err)
						continue
					}
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
