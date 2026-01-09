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
	interval := 30 * time.Second
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
		OwnerID     int64   `json:"owner_id"`
		GroupID     *int64  `json:"group_id"`
		Title       string  `json:"title"`
		Description string  `json:"description"`
		Start       string  `json:"start"`
		End         string  `json:"end"`
		Privacy     Privacy `json:"privacy"`
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
				var events []Event
				if err := json.NewDecoder(resp.Body).Decode(&events); err != nil {
					resp.Body.Close()
					Logger().Warn("appt_reconcile_decode_failed", "peer", id, "err", err)
					continue
				}
				resp.Body.Close()

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

					// Determine group signature pointer
					var groupIDPtr *int64
					if p.GroupID != nil && *p.GroupID != 0 {
						groupIDPtr = p.GroupID
					}

					// If appointment already exists locally, skip
					if existingID, err := store.FindAppointmentBySignature(p.OwnerID, groupIDPtr, start, end, p.Title); err == nil && existingID != 0 {
						continue
					}

					// Reconstruct minimal appointment
					a := Appointment{
						Title:       p.Title,
						Description: p.Description,
						OwnerID:     p.OwnerID,
						Start:       start,
						End:         end,
						Privacy:     p.Privacy,
						OriginNode:  ev.OriginNode,
					}
					if groupIDPtr != nil {
						a.GroupID = groupIDPtr
					}

					var entry LogEntry
					if a.GroupID == nil {
						entry, err = BuildEntryApptCreatePersonal(p.OwnerID, a)
					} else {
						entry, err = BuildEntryApptCreateGroup(p.OwnerID, a)
					}
					if err != nil {
						Logger().Warn("appt_reconcile_build_entry_failed", "peer", id, "err", err)
						continue
					}
					if err := cons.Propose(entry); err != nil {
						Logger().Warn("appt_reconcile_propose_failed", "peer", id, "err", err)
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
