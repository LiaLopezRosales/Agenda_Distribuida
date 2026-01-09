package agendadistribuida

import (
	"encoding/json"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"
)

// StartGroupReconciler reconciles groups and group memberships based on
// locally stored events on each peer.
func StartGroupReconciler(store *Storage, cons Consensus, peers PeerStore) {
	interval := 30 * time.Second
	client := &http.Client{Timeout: 3 * time.Second}
	secret := strings.TrimSpace(os.Getenv("CLUSTER_HMAC_SECRET"))
	if secret == "" {
		Logger().Warn("group_reconciler_disabled_no_secret")
		return
	}

	type peerState struct {
		sinceGroups       time.Time
		sinceGroupMembers time.Time
	}
	mu := sync.Mutex{}
	perPeer := make(map[string]*peerState)

	type groupPayload struct {
		Name            string    `json:"name"`
		Description     string    `json:"description"`
		CreatorID       int64     `json:"creator_id"`
		CreatorUsername string    `json:"creator_username"`
		GroupType       GroupType `json:"group_type"`
	}

	type memberPayload struct {
		GroupID int64 `json:"group_id"`
		UserID  int64 `json:"user_id"`
		Rank    int   `json:"rank"`
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
				sinceGroups := ps.sinceGroups
				sinceMembers := ps.sinceGroupMembers
				mu.Unlock()

				// First reconcile groups themselves
				var maxGroupTS time.Time
				{
					url := "http://" + addr + "/cluster/local-events/groups"
					if !sinceGroups.IsZero() {
						url += "?since=" + sinceGroups.UTC().Format(time.RFC3339)
					}
					req, err := http.NewRequest(http.MethodGet, url, nil)
					if err != nil {
						Logger().Warn("group_reconcile_build_request_failed", "peer", id, "err", err)
						goto MEMBERS
					}
					sig := computeHMACSHA256Hex(nil, secret)
					req.Header.Set("X-Cluster-Signature", sig)
					resp, err := client.Do(req)
					if err != nil {
						Logger().Debug("group_reconcile_request_failed", "peer", id, "err", err)
						goto MEMBERS
					}
					var events []Event
					if err := json.NewDecoder(resp.Body).Decode(&events); err != nil {
						resp.Body.Close()
						Logger().Warn("group_reconcile_decode_failed", "peer", id, "err", err)
						goto MEMBERS
					}
					resp.Body.Close()
					for _, ev := range events {
						if ev.CreatedAt.After(maxGroupTS) {
							maxGroupTS = ev.CreatedAt
						}
						if ev.Payload == "" {
							continue
						}
						var p groupPayload
						if err := json.Unmarshal([]byte(ev.Payload), &p); err != nil {
							continue
						}
						if p.Name == "" || p.CreatorID == 0 {
							continue
						}
						// If group already exists locally by natural key, skip
						if existingID, err := store.FindGroupBySignature(p.Name, p.CreatorID, p.GroupType); err == nil && existingID != 0 {
							continue
						}
						g := &Group{
							Name:            p.Name,
							Description:     p.Description,
							CreatorID:       p.CreatorID,
							CreatorUserName: p.CreatorUsername,
							GroupType:       p.GroupType,
						}
						entry, err := BuildEntryGroupCreate(g)
						if err != nil {
							Logger().Warn("group_reconcile_build_entry_failed", "peer", id, "err", err)
							continue
						}
						if err := cons.Propose(entry); err != nil {
							Logger().Warn("group_reconcile_propose_failed", "peer", id, "err", err)
							continue
						}
					}
				}

			MEMBERS:
				// Then reconcile group memberships
				var maxMemberTS time.Time
				{
					url := "http://" + addr + "/cluster/local-events/group-members"
					if !sinceMembers.IsZero() {
						url += "?since=" + sinceMembers.UTC().Format(time.RFC3339)
					}
					req, err := http.NewRequest(http.MethodGet, url, nil)
					if err != nil {
						Logger().Warn("group_member_reconcile_build_request_failed", "peer", id, "err", err)
						goto UPDATE_STATE
					}
					sig := computeHMACSHA256Hex(nil, secret)
					req.Header.Set("X-Cluster-Signature", sig)
					resp, err := client.Do(req)
					if err != nil {
						Logger().Debug("group_member_reconcile_request_failed", "peer", id, "err", err)
						goto UPDATE_STATE
					}
					var events []Event
					if err := json.NewDecoder(resp.Body).Decode(&events); err != nil {
						resp.Body.Close()
						Logger().Warn("group_member_reconcile_decode_failed", "peer", id, "err", err)
						goto UPDATE_STATE
					}
					resp.Body.Close()
					for _, ev := range events {
						if ev.CreatedAt.After(maxMemberTS) {
							maxMemberTS = ev.CreatedAt
						}
						if ev.Payload == "" {
							continue
						}
						var p memberPayload
						if err := json.Unmarshal([]byte(ev.Payload), &p); err != nil {
							continue
						}
						if p.GroupID == 0 || p.UserID == 0 {
							continue
						}
						// Ensure the group exists locally; if not, skip until next iteration
						if _, err := store.GetGroupByID(p.GroupID); err != nil {
							continue
						}
						// Use repair op for idempotent membership
						entry, err := BuildEntryRepairEnsureGroupMember(p.GroupID, p.UserID, p.Rank)
						if err != nil {
							Logger().Warn("group_member_reconcile_build_entry_failed", "peer", id, "err", err)
							continue
						}
						if err := cons.Propose(entry); err != nil {
							Logger().Warn("group_member_reconcile_propose_failed", "peer", id, "err", err)
							continue
						}
					}
				}

			UPDATE_STATE:
				if !maxGroupTS.IsZero() || !maxMemberTS.IsZero() {
					mu.Lock()
					if !maxGroupTS.IsZero() {
						ps.sinceGroups = maxGroupTS
					}
					if !maxMemberTS.IsZero() {
						ps.sinceGroupMembers = maxMemberTS
					}
					mu.Unlock()
				}
			}
		}
	}()
}
