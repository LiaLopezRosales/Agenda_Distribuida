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
	// Reduced interval for faster reconciliation to avoid leader changes interrupting it
	interval := 10 * time.Second
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
		CreatorID       string    `json:"creator_id"`
		CreatorUsername string    `json:"creator_username"`
		GroupType       GroupType `json:"group_type"`
	}

	type memberPayload struct {
		GroupID  string `json:"group_id"`
		UserID   string `json:"user_id"`
		Username string `json:"username"` // For ID mapping during reconciliation
		Rank     int    `json:"rank"`
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
					defer resp.Body.Close()

					// Verify HTTP status code before decoding
					if resp.StatusCode < 200 || resp.StatusCode >= 300 {
						Logger().Debug("group_reconcile_bad_status", "peer", id, "status", resp.StatusCode)
						goto MEMBERS
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
						Logger().Warn("group_reconcile_decode_failed", "peer", id, "err", err)
						goto MEMBERS
					}
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
						if p.Name == "" {
							continue
						}

						// CRITICAL: Map remote creator ID to local creator ID using username
						// During partitions, the same user may have different IDs in different partitions.
						var localCreatorID string
						if p.CreatorUsername != "" {
							if localCreator, err := store.GetUserByUsername(p.CreatorUsername); err == nil && localCreator != nil {
								localCreatorID = localCreator.ID
							} else {
								// Creator not found locally, skip this group
								Logger().Debug("group_reconcile_creator_not_found", "peer", id, "creator_username", p.CreatorUsername)
								continue
							}
						} else if p.CreatorID != "" {
							localCreatorID = p.CreatorID
							Logger().Warn("group_reconcile_no_creator_username", "peer", id, "using_remote_id", p.CreatorID)
						} else {
							// No creator info, skip
							continue
						}

						// If group already exists locally by natural key, skip
						if existingID, err := store.FindGroupBySignature(p.Name, localCreatorID, p.GroupType); err == nil && existingID != "" {
							continue
						}
						g := &Group{
							Name:            p.Name,
							Description:     p.Description,
							CreatorID:       localCreatorID, // Use local ID, not remote ID
							CreatorUserName: p.CreatorUsername,
							GroupType:       p.GroupType,
						}
						entry, err := BuildEntryGroupCreate(g)
						if err != nil {
							Logger().Warn("group_reconcile_build_entry_failed", "peer", id, "group_name", p.Name, "creator_username", p.CreatorUsername, "err", err)
							continue
						}
						if err := cons.Propose(entry); err != nil {
							Logger().Warn("group_reconcile_propose_failed", "peer", id, "group_name", p.Name, "creator_username", p.CreatorUsername, "err", err)
							continue
						}
						Logger().Debug("group_reconcile_proposed", "peer", id, "group_name", p.Name, "creator_username", p.CreatorUsername)
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
					defer resp.Body.Close()

					// Verify HTTP status code before decoding
					if resp.StatusCode < 200 || resp.StatusCode >= 300 {
						Logger().Debug("group_member_reconcile_bad_status", "peer", id, "status", resp.StatusCode)
						goto UPDATE_STATE
					}

					// Update LastSeen for successfully contacted peer (already updated above, but do it again for consistency)
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
						Logger().Warn("group_member_reconcile_decode_failed", "peer", id, "err", err)
						goto UPDATE_STATE
					}
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
						if p.GroupID == "" {
							continue
						}

						// CRITICAL: Map remote user ID to local user ID using username.
						// Do NOT fallback to remote IDs: IDs may diverge across partitions.
						var localUserID string
						if strings.TrimSpace(p.Username) == "" {
							Logger().Warn("group_member_reconcile_missing_username", "peer", id, "group_id", p.GroupID)
							continue
						}
						if localUser, err := store.GetUserByUsername(p.Username); err == nil && localUser != nil {
							localUserID = localUser.ID
						} else {
							Logger().Debug("group_member_reconcile_user_not_found", "peer", id, "username", p.Username)
							continue
						}

						// Ensure the group exists locally; if not, skip until next iteration
						if _, err := store.GetGroupByID(p.GroupID); err != nil {
							continue
						}
						// Use repair op for idempotent membership with LOCAL user ID
						entry, err := BuildEntryRepairEnsureGroupMember(p.GroupID, localUserID, p.Rank)
						if err != nil {
							Logger().Warn("group_member_reconcile_build_entry_failed", "peer", id, "group_id", p.GroupID, "username", p.Username, "err", err)
							continue
						}
						if err := cons.Propose(entry); err != nil {
							Logger().Warn("group_member_reconcile_propose_failed", "peer", id, "group_id", p.GroupID, "username", p.Username, "err", err)
							continue
						}
						Logger().Debug("group_member_reconcile_proposed", "peer", id, "group_id", p.GroupID, "username", p.Username)
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
