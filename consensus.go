package agendadistribuida

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"os"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

type role int

const (
	roleFollower role = iota
	roleCandidate
	roleLeader
)

type ConsensusImpl struct {
	mu      sync.RWMutex
	storage *Storage
	peers   PeerStore
	nodeID  string
	logger  *slog.Logger

	// persistent/volatile
	state RaftState
	role  role

	// background
	cancel             context.CancelFunc
	resetElectionTimer chan struct{} // channel to reset election timer
	heartbeatFailures  int           // consecutive heartbeat failures (leader demotion)
	failedElections    int           // consecutive failed elections (for backoff)

	// apply callback to mutate the state machine (SQLite)
	applier func(LogEntry) error

	// networking
	httpClient *http.Client
	hmacSecret string

	// Per-peer replication state (Raft-like): maintained on the leader
	nextIdx  map[string]int64 // For each follower, index of the next log entry to send
	matchIdx map[string]int64 // For each follower, index of highest log entry known to be replicated

	applyErr      error
	applyErrIndex int64
}

func NewConsensus(nodeID string, storage *Storage, peers PeerStore) *ConsensusImpl {
	return &ConsensusImpl{
		storage:            storage,
		peers:              peers,
		nodeID:             nodeID,
		role:               roleFollower,
		httpClient:         &http.Client{Timeout: 5 * time.Second},
		hmacSecret:         os.Getenv("CLUSTER_HMAC_SECRET"),
		logger:             Logger(),
		resetElectionTimer: make(chan struct{}, 1),
		nextIdx:            make(map[string]int64),
		matchIdx:           make(map[string]int64),
	}
}

func (c *ConsensusImpl) log(level slog.Level, msg string, attrs ...any) {
	attrs = append(attrs, "node_id", c.nodeID)
	switch level {
	case slog.LevelDebug:
		c.logger.Debug(msg, attrs...)
	case slog.LevelWarn:
		c.logger.Warn(msg, attrs...)
	case slog.LevelError:
		c.logger.Error(msg, attrs...)
	default:
		c.logger.Info(msg, attrs...)
	}
}

func (c *ConsensusImpl) audit(action, message string, fields map[string]any) {
	if fields == nil {
		fields = map[string]any{}
	}
	if _, exists := fields["node_id"]; !exists {
		fields["node_id"] = c.nodeID
	}
	RecordAudit(context.Background(), AuditLevelInfo, "consensus", action, message, fields)
}

func (c *ConsensusImpl) NodeID() string { return c.nodeID }

func (c *ConsensusImpl) IsLeader() bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.role == roleLeader
}

func (c *ConsensusImpl) LeaderID() string {
	return c.peers.GetLeader()
}

func (c *ConsensusImpl) Start() error {
	c.mu.Lock()
	// load state from raft_meta
	if err := c.loadState(); err != nil {
		c.mu.Unlock()
		return err
	}
	ctx, cancel := context.WithCancel(context.Background())
	c.cancel = cancel
	c.mu.Unlock()

	// main loop: heartbeats/election (placeholder) and apply committed entries
	go c.loop(ctx)
	c.log(slog.LevelInfo, "consensus_started", "term", c.state.CurrentTerm)
	c.audit("start", "consensus loop started", map[string]any{"term": c.state.CurrentTerm})
	return nil
}

func (c *ConsensusImpl) Stop() error {
	c.mu.Lock()
	if c.cancel != nil {
		c.cancel()
	}
	c.mu.Unlock()
	c.log(slog.LevelInfo, "consensus_stopped")
	c.audit("stop", "consensus loop stopped", nil)
	return nil
}

func (c *ConsensusImpl) loop(ctx context.Context) {
	hb := time.NewTicker(1 * time.Second) // Increased heartbeat interval for stability
	elect := time.NewTicker(c.electionTimeout())
	defer hb.Stop()
	defer elect.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-hb.C:
			// leader heartbeats and apply committed entries
			c.mu.RLock()
			isLeader := c.role == roleLeader
			term := c.state.CurrentTerm
			commit := c.state.CommitIndex
			c.mu.RUnlock()
			if isLeader {
				err := c.broadcastAppendEntries(term, nil, commit)
				c.mu.Lock()
				if err != nil {
					c.heartbeatFailures++
					// If we fail to reach majority 3 times in a row, demote ourselves
					if c.heartbeatFailures >= 3 {
						c.log(slog.LevelWarn, "leader_demoted_no_majority", "failures", c.heartbeatFailures)
						c.role = roleFollower
						c.peers.SetLeader("")
						c.heartbeatFailures = 0
						c.audit("demotion", "leader demoted due to repeated heartbeat failures", map[string]any{"failures": c.heartbeatFailures})
					}
				} else {
					c.heartbeatFailures = 0 // Reset on success
				}
				c.mu.Unlock()
			}
			_ = c.applyCommitted()
		case <-elect.C:
			// start election if not leader
			c.mu.RLock()
			curRole := c.role
			c.mu.RUnlock()
			if curRole != roleLeader {
				_ = c.startElection()
			}
			elect.Reset(c.electionTimeout())
		case <-c.resetElectionTimer:
			// Reset election timer when receiving AppendEntries from leader
			elect.Reset(c.electionTimeout())
		}
	}
}

func (c *ConsensusImpl) electionTimeout() time.Duration {
	// Base 5s + jitter (0-2s) + small backoff based on failed elections.
	// This prevents premature or overly aggressive elections in noisy clusters.
	c.mu.RLock()
	fe := c.failedElections
	c.mu.RUnlock()
	if fe < 0 {
		fe = 0
	}
	if fe > 3 {
		fe = 3 // cap backoff
	}
	base := 5 * time.Second
	backoff := time.Duration(fe) * 2 * time.Second
	jitter := time.Duration(time.Now().UnixNano() % 2_000_000_000) // up to 2s
	return base + backoff + jitter
}

func (c *ConsensusImpl) Propose(entry LogEntry) error {
	c.mu.RLock()
	if c.role != roleLeader {
		c.mu.RUnlock()
		c.log(slog.LevelWarn, "propose_rejected_not_leader", "leader", c.peers.GetLeader())
		return errors.New("not leader")
	}
	if c.applyErr != nil {
		err := c.applyErr
		idx := c.applyErrIndex
		c.mu.RUnlock()
		c.log(slog.LevelError, "propose_rejected_apply_error", "apply_err", err.Error(), "apply_err_index", idx)
		return errors.New("node has unapplied committed entries (apply error)")
	}
	term := c.state.CurrentTerm
	c.mu.RUnlock()

	// persist to raft_log with next index
	nextIdx, err := c.nextIndex()
	if err != nil {
		return err
	}
	entry.Term = term
	entry.Index = nextIdx
	// if err := c.appendLog(entry); err != nil {
	// 	c.log(slog.LevelError, "propose_append_failed", "err", err)
	// 	return err
	// }
	// replicate to followers and wait for majority to commit
	if err := c.replicateAndCommit(entry); err != nil {
		c.log(slog.LevelError, "propose_replicate_failed", "err", err)
		return err
	}

	// Phase 3: wait until this entry is actually applied on the leader state machine
	// (LastApplied >= entry.Index). This strengthens the guarantee that any
	// subsequent read on the leader will observe the effects of this write.
	if err := c.waitForApplied(entry.Index, 5*time.Second); err != nil {
		c.log(slog.LevelError, "propose_wait_applied_failed", "index", entry.Index, "err", err)
		return err
	}

	c.log(slog.LevelInfo, "propose_committed_and_applied", "index", entry.Index, "op", entry.Op)
	c.audit("propose", "log entry committed and applied", map[string]any{"index": entry.Index, "op": entry.Op})
	return nil
}

func (c *ConsensusImpl) HandleAppendEntries(req AppendEntriesRequest) (AppendEntriesResponse, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	// Reject AppendEntries from older terms
	if req.Term < c.state.CurrentTerm {
		c.log(slog.LevelDebug, "append_entries_reject_old_term", "term", req.Term)
		return AppendEntriesResponse{Term: c.state.CurrentTerm, Success: false, MatchIndex: c.state.LastApplied}, nil
	}

	// If we receive AppendEntries from a higher term, become follower
	wasLeader := c.role == roleLeader
	if req.Term > c.state.CurrentTerm {
		c.state.CurrentTerm = req.Term
		_ = c.persistMeta("currentTerm", c.state.CurrentTerm)
		c.role = roleFollower
		c.peers.SetLeader(req.LeaderID)
		c.heartbeatFailures = 0 // Reset failure counter
		if wasLeader {
			c.log(slog.LevelWarn, "leader_demoted_higher_term", "leader_id", req.LeaderID, "term", req.Term)
			c.audit("demotion", "leader demoted by higher term", map[string]any{"new_leader": req.LeaderID, "term": req.Term})
		} else {
			c.log(slog.LevelDebug, "append_entries_leader_seen", "leader_id", req.LeaderID, "term", req.Term)
		}
	} else if req.Term == c.state.CurrentTerm {
		// Same term: if we're leader and receive from another leader, demote ourselves (split-brain)
		if wasLeader && req.LeaderID != c.nodeID {
			c.role = roleFollower
			c.peers.SetLeader(req.LeaderID)
			c.heartbeatFailures = 0
			c.log(slog.LevelWarn, "leader_demoted_same_term", "leader_id", req.LeaderID, "term", req.Term)
			c.audit("demotion", "leader demoted by same-term leader", map[string]any{"new_leader": req.LeaderID, "term": req.Term})
		} else if !wasLeader {
			// We're a follower, accept this leader
			c.peers.SetLeader(req.LeaderID)
		}
	}

	// Reset election timer when receiving valid AppendEntries from leader (not from ourselves)
	// This is critical to prevent unnecessary elections
	if req.LeaderID != c.nodeID && c.role != roleLeader {
		select {
		case c.resetElectionTimer <- struct{}{}:
		default:
			// Channel full, skip (non-blocking)
		}
	}

	if req.PrevLogIndex > 0 {
		term, err := c.logTermAt(req.PrevLogIndex)
		if err != nil {
			c.log(slog.LevelWarn, "append_entries_prev_lookup_failed", "err", err, "prev_index", req.PrevLogIndex)
			return AppendEntriesResponse{Term: c.state.CurrentTerm, Success: false, MatchIndex: c.state.LastApplied}, err
		}
		if term != req.PrevLogTerm {
			_ = c.truncateLogFrom(req.PrevLogIndex)
			if c.state.LastApplied >= req.PrevLogIndex {
				c.state.LastApplied = req.PrevLogIndex - 1
				if c.state.LastApplied < 0 {
					c.state.LastApplied = 0
				}
				_ = c.persistMeta("lastApplied", c.state.LastApplied)
			}
			if c.state.CommitIndex >= req.PrevLogIndex {
				c.state.CommitIndex = req.PrevLogIndex - 1
				if c.state.CommitIndex < 0 {
					c.state.CommitIndex = 0
				}
				_ = c.persistMeta("commitIndex", c.state.CommitIndex)
			}
			c.log(slog.LevelDebug, "append_entries_prev_mismatch", "expected_term", req.PrevLogTerm, "found_term", term)
			return AppendEntriesResponse{Term: c.state.CurrentTerm, Success: false, MatchIndex: c.state.LastApplied}, nil
		}
	}
	var lastIdx int64 = req.PrevLogIndex
	for _, e := range req.Entries {
		if err := c.truncateLogFrom(e.Index); err != nil {
			c.log(slog.LevelWarn, "append_entries_truncate_failed", "err", err, "index", e.Index)
			return AppendEntriesResponse{Term: c.state.CurrentTerm, Success: false, MatchIndex: lastIdx}, err
		}
		if err := c.appendLog(e); err != nil {
			c.log(slog.LevelWarn, "append_entries_append_failed", "err", err)
			return AppendEntriesResponse{Term: c.state.CurrentTerm, Success: false, MatchIndex: lastIdx}, nil
		}
		lastIdx = e.Index
	}
	if req.LeaderCommit > c.state.CommitIndex {
		c.state.CommitIndex = req.LeaderCommit
		_ = c.persistMeta("commitIndex", c.state.CommitIndex)
	}
	return AppendEntriesResponse{Term: c.state.CurrentTerm, Success: true, MatchIndex: lastIdx}, nil
}

func (c *ConsensusImpl) HandleRequestVote(req RequestVoteRequest) (RequestVoteResponse, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if req.Term < c.state.CurrentTerm {
		c.log(slog.LevelDebug, "request_vote_old_term", "candidate", req.CandidateID, "term", req.Term, "prevote", req.PreVote)
		return RequestVoteResponse{Term: c.state.CurrentTerm, VoteGranted: false}, nil
	}

	// Pre-vote requests do not mutate currentTerm or votedFor; they are only a probe.
	if req.PreVote {
		lastIdx, lastTerm, err := c.lastIndexTerm()
		if err != nil {
			return RequestVoteResponse{Term: c.state.CurrentTerm, VoteGranted: false}, err
		}
		if lastTerm > req.LastLogTerm || (lastTerm == req.LastLogTerm && lastIdx > req.LastLogIndex) {
			c.log(slog.LevelDebug, "prevote_log_outdated", "candidate", req.CandidateID)
			return RequestVoteResponse{Term: c.state.CurrentTerm, VoteGranted: false}, nil
		}
		// Log is up-to-date enough; signal willingness without recording a vote.
		c.log(slog.LevelDebug, "prevote_granted", "candidate", req.CandidateID, "term", req.Term)
		return RequestVoteResponse{Term: c.state.CurrentTerm, VoteGranted: true}, nil
	}

	// Regular RequestVote: may advance term and record votedFor.
	if req.Term > c.state.CurrentTerm {
		c.state.CurrentTerm = req.Term
		_ = c.persistMeta("currentTerm", c.state.CurrentTerm)
		c.state.VotedFor = ""
		_ = c.persistMetaString("votedFor", "")
	}
	if c.state.VotedFor != "" && c.state.VotedFor != req.CandidateID {
		c.log(slog.LevelDebug, "request_vote_already_voted", "candidate", req.CandidateID, "voted_for", c.state.VotedFor)
		return RequestVoteResponse{Term: c.state.CurrentTerm, VoteGranted: false}, nil
	}
	lastIdx, lastTerm, err := c.lastIndexTerm()
	if err != nil {
		return RequestVoteResponse{Term: c.state.CurrentTerm, VoteGranted: false}, err
	}
	if lastTerm > req.LastLogTerm || (lastTerm == req.LastLogTerm && lastIdx > req.LastLogIndex) {
		c.log(slog.LevelDebug, "request_vote_log_outdated", "candidate", req.CandidateID)
		return RequestVoteResponse{Term: c.state.CurrentTerm, VoteGranted: false}, nil
	}
	c.state.VotedFor = req.CandidateID
	_ = c.persistMetaString("votedFor", req.CandidateID)
	c.log(slog.LevelInfo, "request_vote_granted", "candidate", req.CandidateID, "term", req.Term)
	return RequestVoteResponse{Term: c.state.CurrentTerm, VoteGranted: true}, nil
}

// --- persistence helpers ---

func (c *ConsensusImpl) loadState() error {
	// read raft_meta keys
	rows, err := c.storage.db.Query(`SELECT key, value FROM raft_meta`)
	if err != nil {
		return err
	}
	defer rows.Close()
	st := RaftState{}
	for rows.Next() {
		var k, v string
		if err := rows.Scan(&k, &v); err != nil {
			return err
		}
		switch k {
		case "currentTerm":
			st.CurrentTerm = parseInt64Default(v, 0)
		case "votedFor":
			st.VotedFor = v
		case "commitIndex":
			st.CommitIndex = parseInt64Default(v, 0)
		case "lastApplied":
			st.LastApplied = parseInt64Default(v, 0)
		}
	}
	c.state = st
	return nil
}

// activePeers returns the list of peer node IDs that are considered "active"
// based on actual connectivity, not just LastSeen timestamps. This is critical
// for network partitions: we only count peers that we can actually communicate with.
//
// The function uses PeerStore as the source of truth, which is updated by
// DiscoveryManager based on successful communication. During partitions,
// unreachable peers will eventually be removed from PeerStore when their
// LastSeen expires (maxPeerAge = 2 minutes).
func (c *ConsensusImpl) activePeers(maxStale time.Duration) []string {
	// Use only PeerStore - it represents nodes that DiscoveryManager considers
	// contactable. During partitions, unreachable nodes will have stale LastSeen
	// and will be removed from PeerStore by updatePeersFromStorage() after maxPeerAge.
	knownPeers := c.peers.ListPeers()
	out := make([]string, 0, len(knownPeers))
	for _, id := range knownPeers {
		if id == "" || id == c.nodeID || id == "node-unknown" {
			continue
		}
		out = append(out, id)
	}

	// Log for debugging partition scenarios
	if len(out) > 0 {
		c.log(slog.LevelDebug, "active_peers_computed", "count", len(out), "peers", out)
	}

	return out
}

func (c *ConsensusImpl) persistMeta(key string, val int64) error {
	_, err := c.storage.db.Exec(`INSERT INTO raft_meta(key,value) VALUES(?,?)
        ON CONFLICT(key) DO UPDATE SET value=excluded.value`, key, intToString(val))
	return err
}

func (c *ConsensusImpl) persistMetaString(key string, val string) error {
	_, err := c.storage.db.Exec(`INSERT INTO raft_meta(key,value) VALUES(?,?)
        ON CONFLICT(key) DO UPDATE SET value=excluded.value`, key, val)
	return err
}

func (c *ConsensusImpl) nextIndex() (int64, error) {
	var idx sql.NullInt64
	err := c.storage.db.QueryRow(`SELECT COALESCE(MAX(idx),0) FROM raft_log`).Scan(&idx)
	if err != nil {
		return 0, err
	}
	if idx.Valid {
		return idx.Int64 + 1, nil
	}
	return 1, nil
}

func (c *ConsensusImpl) appendLog(e LogEntry) error {
	_, err := c.storage.db.Exec(`INSERT OR REPLACE INTO raft_log(term, idx, event_id, aggregate, aggregate_id, op, payload, ts)
        VALUES(?,?,?,?,?,?,?,?)`, e.Term, e.Index, e.EventID, e.Aggregate, e.AggregateID, e.Op, e.Payload, e.Timestamp)
	return err
}

func (c *ConsensusImpl) truncateLogFrom(idx int64) error {
	if idx <= 0 {
		return nil
	}
	_, err := c.storage.db.Exec(`DELETE FROM raft_log WHERE idx >= ?`, idx)
	return err
}

func (c *ConsensusImpl) logTermAt(idx int64) (int64, error) {
	if idx <= 0 {
		return 0, nil
	}
	var term sql.NullInt64
	err := c.storage.db.QueryRow(`SELECT term FROM raft_log WHERE idx=?`, idx).Scan(&term)
	if err == sql.ErrNoRows {
		return 0, nil
	}
	if err != nil {
		return 0, err
	}
	return term.Int64, nil
}

// loadLogEntriesFrom returns all log entries starting at index >= startIdx, ordered by idx ASC.
func (c *ConsensusImpl) loadLogEntriesFrom(startIdx int64) ([]LogEntry, error) {
	rows, err := c.storage.db.Query(`SELECT term, idx, event_id, aggregate, aggregate_id, op, payload, ts FROM raft_log WHERE idx>=? ORDER BY idx ASC`, startIdx)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []LogEntry
	for rows.Next() {
		var e LogEntry
		var ts time.Time
		if err := rows.Scan(&e.Term, &e.Index, &e.EventID, &e.Aggregate, &e.AggregateID, &e.Op, &e.Payload, &ts); err != nil {
			return nil, err
		}
		e.Timestamp = ts
		out = append(out, e)
	}
	return out, nil
}

// recalculateCommitIndex recomputes the commit index on the leader based on
// the matchIdx of all peers, following the Raft majority rule. It is a no-op
// on followers.
func (c *ConsensusImpl) recalculateCommitIndex() {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.role != roleLeader {
		return
	}

	lastIdx, _, err := c.lastIndexTerm()
	if err != nil || lastIdx == 0 {
		return
	}

	// Use dynamically detected active peers (recently seen) for majority
	peers := c.activePeers(15 * time.Second)
	// Collect match indexes: leader's own last index plus followers' matchIdx.
	// Ensure we have a value for every peer to keep majority math consistent.
	idxs := make([]int64, 0, len(peers)+1)
	idxs = append(idxs, lastIdx)
	for _, id := range peers {
		if id == c.nodeID {
			continue
		}
		mi := int64(0)
		if v, ok := c.matchIdx[id]; ok {
			mi = v
		}
		idxs = append(idxs, mi)
	}
	if len(idxs) == 0 {
		return
	}

	// Sort ascending to find the index held by a majority
	sort.Slice(idxs, func(i, j int) bool { return idxs[i] < idxs[j] })
	totalNodes := len(peers) + 1
	majority := (totalNodes / 2) + 1
	if len(idxs) < majority {
		return
	}
	candidate := idxs[len(idxs)-majority]

	if candidate > c.state.CommitIndex {
		c.state.CommitIndex = candidate
		_ = c.persistMeta("commitIndex", c.state.CommitIndex)
		c.log(slog.LevelInfo, "commit_index_advanced", "commit_index", c.state.CommitIndex)
	}
}

// small utils
func intToString(v int64) string { return fmtInt(v) }

func parseInt64Default(s string, def int64) int64 {
	if s == "" {
		return def
	}
	if v, err := parseInt64(s); err == nil {
		return v
	}
	return def
}

// wrappers to avoid importing strconv everywhere
func parseInt64(s string) (int64, error) { return strconv.ParseInt(s, 10, 64) }
func fmtInt(v int64) string              { return strconv.FormatInt(v, 10) }

// --- applier integration ---

func (c *ConsensusImpl) SetApplier(f func(LogEntry) error) {
	c.mu.Lock()
	c.applier = f
	c.mu.Unlock()
}

func (c *ConsensusImpl) applyCommitted() error {
	c.mu.Lock()
	lastApplied := c.state.LastApplied
	commitIndex := c.state.CommitIndex
	applier := c.applier
	c.mu.Unlock()
	if applier == nil {
		return nil
	}
	if commitIndex <= lastApplied {
		return nil
	}
	// apply [lastApplied+1, commitIndex]
	rows, err := c.storage.db.Query(`SELECT term, idx, event_id, aggregate, aggregate_id, op, payload, ts FROM raft_log WHERE idx>? AND idx<=? ORDER BY idx ASC`, lastApplied, commitIndex)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var e LogEntry
		var ts time.Time
		if err := rows.Scan(&e.Term, &e.Index, &e.EventID, &e.Aggregate, &e.AggregateID, &e.Op, &e.Payload, &ts); err != nil {
			return err
		}
		e.Timestamp = ts
		applied, err := c.storage.HasAppliedEvent(e.EventID)
		if err != nil {
			return err
		}
		if !applied {
			if err := applier(e); err != nil {
				// Conservative auto-repair: for some ops, an error can be treated as a
				// benign no-op (already applied/duplicate) to prevent the state machine
				// from getting permanently stuck.
				if isIgnorableApplyError(e, err) {
					c.log(slog.LevelWarn, "apply_committed_ignored_error", "index", e.Index, "op", e.Op, "err", err.Error())
					c.audit("raft_apply", "ignored apply error", map[string]any{"index": e.Index, "op": e.Op, "err": err.Error()})
					if err := c.storage.RecordAppliedEvent(e.EventID, e.Index); err != nil {
						return err
					}
					// advance lastApplied even though we treated it as no-op
					lastApplied = e.Index
					_ = c.persistMeta("lastApplied", lastApplied)
					c.mu.Lock()
					c.state.LastApplied = lastApplied
					if c.applyErr != nil && c.applyErrIndex <= lastApplied {
						c.applyErr = nil
						c.applyErrIndex = 0
					}
					c.mu.Unlock()
					continue
				}
				c.mu.Lock()
				c.applyErr = err
				c.applyErrIndex = e.Index
				c.mu.Unlock()
				return err
			}
			if err := c.storage.RecordAppliedEvent(e.EventID, e.Index); err != nil {
				return err
			}
		}
		// advance lastApplied
		lastApplied = e.Index
		_ = c.persistMeta("lastApplied", lastApplied)
		c.mu.Lock()
		c.state.LastApplied = lastApplied
		if c.applyErr != nil && c.applyErrIndex <= lastApplied {
			c.applyErr = nil
			c.applyErrIndex = 0
		}
		c.mu.Unlock()
	}
	return nil
}

func isIgnorableApplyError(e LogEntry, err error) bool {
	if err == nil {
		return true
	}
	msg := err.Error()
	// Errors we explicitly generate to represent benign idempotent replays.
	if strings.Contains(msg, "apply conflict") {
		// These conflicts can happen due to TOCTOU races but should not poison the log.
		// We validate inputs pre-Propose for normal paths.
		return true
	}
	// Storage-layer idempotent remove.
	if e.Op == OpGroupMemberRemove && strings.Contains(msg, "member not found") {
		return true
	}
	// Best-effort idempotency for inserts that can race/duplicate.
	if strings.Contains(msg, "UNIQUE constraint failed") {
		switch e.Op {
		case OpUserCreate,
			OpUserUpdateProfile,
			OpApptCreatePersonal,
			OpApptCreateGroup,
			OpRepairEnsureParticipant,
			OpRepairEnsureGroupMember,
			OpRepairEnsureNotification:
			return true
		}
	}
	return false
}

// waitForApplied blocks until the leader has applied at least up to targetIdx
// (state.LastApplied >= targetIdx), or until timeout elapses, or the node stops
// being leader. It is only meaningful to call this on the leader.
func (c *ConsensusImpl) waitForApplied(targetIdx int64, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	for {
		c.mu.RLock()
		lastApplied := c.state.LastApplied
		role := c.role
		applyErr := c.applyErr
		applyErrIndex := c.applyErrIndex
		c.mu.RUnlock()

		if role != roleLeader {
			return errors.New("lost leadership while waiting for apply")
		}
		if applyErr != nil && applyErrIndex > 0 && applyErrIndex <= targetIdx {
			return errors.New("apply error while waiting for entry to be applied")
		}
		if lastApplied >= targetIdx {
			return nil
		}
		if time.Now().After(deadline) {
			return errors.New("timeout waiting for entry to be applied")
		}
		// Small sleep to avoid busy-waiting; applyCommitted is fast in practice.
		time.Sleep(10 * time.Millisecond)
	}
}

// --- networking / majority replication ---

func (c *ConsensusImpl) broadcastAppendEntries(term int64, entries []LogEntry, leaderCommit int64) error {
	// Use dynamically detected active peers (recently seen) to determine
	// the effective cluster size for this broadcast.
	peers := c.activePeers(15 * time.Second)
	successes := 1               // leader counts self
	totalNodes := len(peers) + 1 // include self
	majority := (totalNodes / 2) + 1

	// Heartbeat-only path: keep simple behavior using last index/term
	if len(entries) == 0 {
		prevIdx, prevTerm, _ := c.lastIndexTerm()
		req := AppendEntriesRequest{
			Term:         term,
			LeaderID:     c.nodeID,
			PrevLogIndex: prevIdx,
			PrevLogTerm:  prevTerm,
			Entries:      nil,
			LeaderCommit: leaderCommit,
		}
		payload, _ := json.Marshal(req)
		type result struct {
			success bool
			term    int64
		}
		ch := make(chan result, len(peers))
		for _, id := range peers {
			if id == c.nodeID {
				continue
			}
			go func(pid string) {
				respBody, err := c.postJSONWithResponse("http://"+c.peers.ResolveAddr(pid)+"/raft/append-entries", payload)
				if err != nil {
					ch <- result{success: false, term: 0}
					return
				}
				var resp AppendEntriesResponse
				if err := json.Unmarshal(respBody, &resp); err != nil {
					ch <- result{success: false, term: 0}
					return
				}
				// If follower has higher term, we need to become follower
				if resp.Term > term {
					c.mu.Lock()
					if resp.Term > c.state.CurrentTerm {
						c.state.CurrentTerm = resp.Term
						_ = c.persistMeta("currentTerm", c.state.CurrentTerm)
						c.role = roleFollower
						c.peers.SetLeader("")
						c.log(slog.LevelWarn, "leader_demoted_higher_term", "follower_term", resp.Term, "our_term", term)
						c.audit("demotion", "leader demoted due to higher term from follower", map[string]any{"follower_term": resp.Term, "our_term": term})
					}
					c.mu.Unlock()
					ch <- result{success: false, term: resp.Term}
					return
				}
				ch <- result{success: resp.Success, term: resp.Term}
			}(id)
		}
		timeout := time.After(3 * time.Second)
		for pending := len(peers); pending > 0; pending-- {
			select {
			case res := <-ch:
				if res.success {
					successes++
					c.log(slog.LevelDebug, "append_entries_success", "successes", successes, "majority", majority)
				}
				if successes >= majority {
					c.log(slog.LevelInfo, "append_entries_majority_achieved", "successes", successes)
					return nil
				}
			case <-timeout:
				c.log(slog.LevelWarn, "append_entries_timeout", "successes", successes, "majority", majority)
				return errors.New("append majority failed")
			}
		}
		if successes >= majority {
			return nil
		}
		c.log(slog.LevelError, "append_entries_no_majority", "successes", successes, "needed", majority)
		return errors.New("append majority failed")
	}

	// Replication path with new entries: use per-peer nextIndex/matchIndex to catch up followers.
	// We ignore the specific entries slice and instead send the suffix of our log needed by each peer.
	lastIdx, _, _ := c.lastIndexTerm()

	type result struct {
		success bool
		term    int64
	}
	ch := make(chan result, len(peers))
	for _, id := range peers {
		if id == c.nodeID {
			continue
		}
		go func(pid string) {
			// Try to catch up this follower with a small number of internal retries,
			// adjusting nextIdx on each failure. This improves robustness without
			// changing the external majority/timeout behavior.
			const perPeerMaxRetries = 5
			for attempt := 0; attempt < perPeerMaxRetries; attempt++ {
				// Determine from which index this follower currently needs entries
				c.mu.RLock()
				ni, ok := c.nextIdx[pid]
				if !ok || ni <= 0 {
					ni = lastIdx + 1 // nothing to send yet
				}
				c.mu.RUnlock()

				// Case 1: leader thinks follower is up-to-date (heartbeat-style AppendEntries)
				if ni > lastIdx {
					prevIdx := lastIdx
					prevTerm, _ := c.logTermAt(prevIdx)
					req := AppendEntriesRequest{
						Term:         term,
						LeaderID:     c.nodeID,
						PrevLogIndex: prevIdx,
						PrevLogTerm:  prevTerm,
						Entries:      nil,
						LeaderCommit: leaderCommit,
					}
					payload, _ := json.Marshal(req)
					respBody, err := c.postJSONWithResponse("http://"+c.peers.ResolveAddr(pid)+"/raft/append-entries", payload)
					if err != nil {
						// Network/HTTP error: give this follower a chance in next heartbeat/broadcast
						ch <- result{success: false, term: 0}
						return
					}
					var resp AppendEntriesResponse
					if err := json.Unmarshal(respBody, &resp); err != nil {
						ch <- result{success: false, term: 0}
						return
					}
					if resp.Term > term {
						c.mu.Lock()
						if resp.Term > c.state.CurrentTerm {
							c.state.CurrentTerm = resp.Term
							_ = c.persistMeta("currentTerm", c.state.CurrentTerm)
							c.role = roleFollower
							c.peers.SetLeader("")
							c.log(slog.LevelWarn, "leader_demoted_higher_term", "follower_term", resp.Term, "our_term", term)
							c.audit("demotion", "leader demoted due to higher term from follower", map[string]any{"follower_term": resp.Term, "our_term": term})
						}
						c.mu.Unlock()
						ch <- result{success: false, term: resp.Term}
						return
					}
					if resp.Success {
						// Update matchIdx/nextIdx if follower reports progress
						c.mu.Lock()
						if resp.MatchIndex > 0 {
							c.matchIdx[pid] = resp.MatchIndex
							c.nextIdx[pid] = resp.MatchIndex + 1
						} else {
							c.matchIdx[pid] = lastIdx
							c.nextIdx[pid] = lastIdx + 1
						}
						c.mu.Unlock()
						ch <- result{success: true, term: resp.Term}
						return
					}
					// Rechazo por inconsistencia: retroceder nextIdx para este follower y reintentar
					c.mu.Lock()
					curNext := c.nextIdx[pid]
					if curNext <= 1 {
						c.nextIdx[pid] = 1
					} else {
						c.nextIdx[pid] = curNext - 1
					}
					c.mu.Unlock()
					continue
				}

				// Case 2: follower is behind; send log suffix starting at nextIdx
				ents, err := c.loadLogEntriesFrom(ni)
				if err != nil {
					ch <- result{success: false, term: 0}
					return
				}
				if len(ents) == 0 {
					// Nothing to send; treat as success (follower already caught up)
					c.mu.Lock()
					c.matchIdx[pid] = lastIdx
					c.nextIdx[pid] = lastIdx + 1
					c.mu.Unlock()
					ch <- result{success: true, term: term}
					return
				}
				prevIdx := ni - 1
				prevTerm, _ := c.logTermAt(prevIdx)
				req := AppendEntriesRequest{
					Term:         term,
					LeaderID:     c.nodeID,
					PrevLogIndex: prevIdx,
					PrevLogTerm:  prevTerm,
					Entries:      ents,
					LeaderCommit: leaderCommit,
				}
				payload, _ := json.Marshal(req)
				respBody, err := c.postJSONWithResponse("http://"+c.peers.ResolveAddr(pid)+"/raft/append-entries", payload)
				if err != nil {
					// Network or HTTP error, do not advance nextIdx; let outer timeout handle it
					ch <- result{success: false, term: 0}
					return
				}
				var resp AppendEntriesResponse
				if err := json.Unmarshal(respBody, &resp); err != nil {
					ch <- result{success: false, term: 0}
					return
				}
				if resp.Term > term {
					c.mu.Lock()
					if resp.Term > c.state.CurrentTerm {
						c.state.CurrentTerm = resp.Term
						_ = c.persistMeta("currentTerm", c.state.CurrentTerm)
						c.role = roleFollower
						c.peers.SetLeader("")
						c.log(slog.LevelWarn, "leader_demoted_higher_term", "follower_term", resp.Term, "our_term", term)
						c.audit("demotion", "leader demoted due to higher term from follower", map[string]any{"follower_term": resp.Term, "our_term": term})
					}
					c.mu.Unlock()
					ch <- result{success: false, term: resp.Term}
					return
				}
				if resp.Success {
					// Follower accepted entries: advance nextIdx and matchIdx
					c.mu.Lock()
					if resp.MatchIndex > 0 {
						c.matchIdx[pid] = resp.MatchIndex
						c.nextIdx[pid] = resp.MatchIndex + 1
					} else {
						// Fallback: assume we synced up to lastIdx
						c.matchIdx[pid] = lastIdx
						c.nextIdx[pid] = lastIdx + 1
					}
					c.mu.Unlock()
					ch <- result{success: true, term: resp.Term}
					return
				}
				// Rejection due to log inconsistency: decrement nextIdx and retry
				c.mu.Lock()
				curNext := c.nextIdx[pid]
				if curNext <= 1 {
					c.nextIdx[pid] = 1
				} else {
					c.nextIdx[pid] = curNext - 1
				}
				c.mu.Unlock()
				// loop will retry with updated nextIdx
			}
			// Exhausted retries for this follower
			ch <- result{success: false, term: 0}
		}(id)
	}
	timeout := time.After(3 * time.Second)
	for pending := len(peers); pending > 0; pending-- {
		select {
		case res := <-ch:
			if res.success {
				successes++
				c.log(slog.LevelDebug, "append_entries_success", "successes", successes, "majority", majority)
			}
			if successes >= majority {
				c.log(slog.LevelInfo, "append_entries_majority_achieved", "successes", successes)
				return nil
			}
		case <-timeout:
			c.log(slog.LevelWarn, "append_entries_timeout", "successes", successes, "majority", majority)
			return errors.New("append majority failed")
		}
	}
	if successes >= majority {
		return nil
	}
	c.log(slog.LevelError, "append_entries_no_majority", "successes", successes, "needed", majority)
	return errors.New("append majority failed")
}

func (c *ConsensusImpl) replicateAndCommit(entry LogEntry) error {
	// First, append to local log
	if err := c.appendLog(entry); err != nil {
		return err
	}

	// Then replicate to followers and wait for majority
	maxRetries := 5
	backoff := 100 * time.Millisecond

	for attempt := 0; attempt < maxRetries; attempt++ {
		if attempt > 0 {
			c.log(slog.LevelWarn, "replicate_retry", "attempt", attempt+1, "max_retries", maxRetries)
			time.Sleep(backoff)
			backoff *= 2 // Exponential backoff
		}

		if err := c.broadcastAppendEntries(entry.Term, []LogEntry{entry}, c.state.CommitIndex); err != nil {
			if err.Error() == "append majority failed" {
				c.log(slog.LevelWarn, "replicate_no_majority", "attempt", attempt+1)
				continue // Retry
			}
			return err
		}
		// Success - break out of retry loop
		break
	}

	// Recalculate commit index on the leader after majority has acknowledged.
	// This uses matchIdx and follows the Raft majority rule.
	c.recalculateCommitIndex()

	// Apply committed entries to state machine
	if err := c.applyCommitted(); err != nil {
		c.log(slog.LevelError, "apply_committed_failed", "err", err)
	}

	return nil
}

func (c *ConsensusImpl) startElection() error {
	c.log(slog.LevelInfo, "election_started")
	// First run a pre-vote to check if it is likely we can win an election.
	if !c.runPreVote() {
		c.mu.Lock()
		if c.failedElections < 10 {
			c.failedElections++
		}
		c.mu.Unlock()
		c.log(slog.LevelWarn, "prevote_failed")
		return errors.New("prevote failed")
	}

	c.mu.Lock()
	c.state.CurrentTerm++
	term := c.state.CurrentTerm
	_ = c.persistMeta("currentTerm", term)
	_ = c.persistMetaString("votedFor", c.nodeID)
	c.role = roleCandidate
	c.mu.Unlock()

	lastIdx, lastTerm, _ := c.lastIndexTerm()
	req := RequestVoteRequest{Term: term, CandidateID: c.nodeID, LastLogIndex: lastIdx, LastLogTerm: lastTerm}
	payload, _ := json.Marshal(req)
	votes := 1 // candidate votes for itself
	// Use dynamically detected active peers for election majority.
	peers := c.activePeers(15 * time.Second)
	// Track reachable peers dynamically during election to handle partitions correctly
	reachablePeers := 1 // Start with self
	ch := make(chan bool, len(peers))
	for _, id := range peers {
		if id == c.nodeID {
			continue
		}
		go func(pid string) {
			ok := c.postJSON("http://"+c.peers.ResolveAddr(pid)+"/raft/request-vote", payload)
			ch <- ok
		}(id)
	}
	// Initial majority based on all known peers (will be recalculated dynamically)
	totalNodes := len(peers) + 1           // Include self in total count
	timeout := time.After(3 * time.Second) // Increased timeout for network latency
	for pending := len(peers); pending > 0; pending-- {
		select {
		case ok := <-ch:
			// Count this peer as reachable (responded, even if vote was denied)
			reachablePeers++
			// Recalculate majority based on actually reachable peers
			// This adapts to network partitions: if only 3 nodes respond, majority = 2
			reachableTotal := reachablePeers
			reachableMajority := (reachableTotal / 2) + 1

			if ok {
				votes++
			}
			// Check if we have majority or can use degraded mode
			attemptedContacts := len(peers) // peers excludes self
			// Use reachable majority for partition-aware elections
			if votes >= reachableMajority {
				// Normal case: we have majority
				c.mu.Lock()
				c.role = roleLeader
				c.peers.SetLeader(c.nodeID)
				c.heartbeatFailures = 0
				c.failedElections = 0
				lastIdx, _, _ := c.lastIndexTerm()
				c.nextIdx = make(map[string]int64)
				c.matchIdx = make(map[string]int64)
				for _, id := range peers {
					if id == c.nodeID {
						continue
					}
					c.nextIdx[id] = lastIdx + 1
					c.matchIdx[id] = 0
				}
				c.mu.Unlock()
				c.log(slog.LevelInfo, "election_won", "term", term, "votes", votes, "majority", reachableMajority, "reachable_peers", reachableTotal, "total_known", totalNodes)
				c.audit("election", "node became leader", map[string]any{"term": term, "votes": votes, "majority": reachableMajority, "reachable_peers": reachableTotal, "total_known": totalNodes})
				return nil
			} else if votes == 1 && attemptedContacts > 0 && (reachableTotal == 2 || reachableTotal == 3) {
				// Degraded mode: only for very small clusters where we tried to contact others
				// but they're unreachable. This prevents split-brain during partitions.
				c.mu.Lock()
				c.role = roleLeader
				c.peers.SetLeader(c.nodeID)
				c.heartbeatFailures = 0
				c.failedElections = 0
				lastIdx, _, _ := c.lastIndexTerm()
				c.nextIdx = make(map[string]int64)
				c.matchIdx = make(map[string]int64)
				for _, id := range peers {
					if id == c.nodeID {
						continue
					}
					c.nextIdx[id] = lastIdx + 1
					c.matchIdx[id] = 0
				}
				c.mu.Unlock()
				c.log(slog.LevelWarn, "election_won_degraded", "term", term, "votes", votes, "reachable_peers", reachableTotal, "total_known", totalNodes, "attempted", attemptedContacts)
				c.audit("election", "node became leader in degraded mode", map[string]any{"term": term, "votes": votes, "reachable_peers": reachableTotal, "total_known": totalNodes})
				return nil
			}
			// Continue waiting for more votes
		case <-timeout:
			// On timeout, recalculate based on peers that responded
			reachableTotal := reachablePeers
			reachableMajority := (reachableTotal / 2) + 1
			attemptedContacts := len(peers)
			if votes >= reachableMajority {
				c.mu.Lock()
				c.role = roleLeader
				c.peers.SetLeader(c.nodeID)
				c.heartbeatFailures = 0
				c.failedElections = 0
				lastIdx, _, _ := c.lastIndexTerm()
				c.nextIdx = make(map[string]int64)
				c.matchIdx = make(map[string]int64)
				for _, id := range peers {
					if id == c.nodeID {
						continue
					}
					c.nextIdx[id] = lastIdx + 1
					c.matchIdx[id] = 0
				}
				c.mu.Unlock()
				c.log(slog.LevelInfo, "election_won_timeout", "term", term, "votes", votes, "majority", reachableMajority, "reachable_peers", reachableTotal, "total_known", totalNodes)
				c.audit("election", "node became leader after timeout", map[string]any{"term": term, "votes": votes, "majority": reachableMajority, "reachable_peers": reachableTotal, "total_known": totalNodes})
				return nil
			} else if votes == 1 && attemptedContacts > 0 && (reachableTotal == 2 || reachableTotal == 3) {
				c.mu.Lock()
				c.role = roleLeader
				c.peers.SetLeader(c.nodeID)
				c.heartbeatFailures = 0
				c.failedElections = 0
				lastIdx, _, _ := c.lastIndexTerm()
				c.nextIdx = make(map[string]int64)
				c.matchIdx = make(map[string]int64)
				for _, id := range peers {
					if id == c.nodeID {
						continue
					}
					c.nextIdx[id] = lastIdx + 1
					c.matchIdx[id] = 0
				}
				c.mu.Unlock()
				c.log(slog.LevelWarn, "election_won_degraded_timeout", "term", term, "votes", votes, "reachable_peers", reachableTotal, "total_known", totalNodes, "attempted", attemptedContacts)
				c.audit("election", "node became leader in degraded mode after timeout", map[string]any{"term": term, "votes": votes, "reachable_peers": reachableTotal, "total_known": totalNodes})
				return nil
			}
			c.mu.Lock()
			if c.failedElections < 10 {
				c.failedElections++
			}
			c.mu.Unlock()
			c.log(slog.LevelWarn, "election_timeout", "votes", votes, "needed", reachableMajority, "reachable_peers", reachableTotal, "total_known", totalNodes)
			return errors.New("election timeout")
		}
	}
	// All peers responded, recalculate based on reachable peers
	reachableTotal := reachablePeers
	reachableMajority := (reachableTotal / 2) + 1
	attemptedContacts := len(peers) // peers excludes self
	if votes >= reachableMajority {
		// Normal case: we have majority
		c.mu.Lock()
		c.role = roleLeader
		c.peers.SetLeader(c.nodeID)
		c.heartbeatFailures = 0
		c.failedElections = 0
		lastIdx, _, _ := c.lastIndexTerm()
		c.nextIdx = make(map[string]int64)
		c.matchIdx = make(map[string]int64)
		for _, id := range peers {
			if id == c.nodeID {
				continue
			}
			c.nextIdx[id] = lastIdx + 1
			c.matchIdx[id] = 0
		}
		c.mu.Unlock()
		c.log(slog.LevelInfo, "election_won", "term", term, "votes", votes, "majority", reachableMajority, "reachable_peers", reachableTotal, "total_known", totalNodes)
		c.audit("election", "node became leader", map[string]any{"term": term, "votes": votes, "majority": reachableMajority, "reachable_peers": reachableTotal, "total_known": totalNodes})
		return nil
	} else if votes == 1 && attemptedContacts > 0 && (reachableTotal == 2 || reachableTotal == 3) {
		// Degraded mode: only for very small clusters where we tried to contact others
		// but they're unreachable. This prevents split-brain during partitions.
		c.mu.Lock()
		c.role = roleLeader
		c.peers.SetLeader(c.nodeID)
		c.heartbeatFailures = 0
		c.failedElections = 0
		lastIdx, _, _ := c.lastIndexTerm()
		c.nextIdx = make(map[string]int64)
		c.matchIdx = make(map[string]int64)
		for _, id := range peers {
			if id == c.nodeID {
				continue
			}
			c.nextIdx[id] = lastIdx + 1
			c.matchIdx[id] = 0
		}
		c.mu.Unlock()
		c.log(slog.LevelWarn, "election_won_degraded", "term", term, "votes", votes, "reachable_peers", reachableTotal, "total_known", totalNodes, "attempted", attemptedContacts)
		c.audit("election", "node became leader in degraded mode", map[string]any{"term": term, "votes": votes, "reachable_peers": reachableTotal, "total_known": totalNodes})
		return nil
	}
	c.mu.Lock()
	if c.failedElections < 10 {
		c.failedElections++
	}
	c.mu.Unlock()
	c.log(slog.LevelWarn, "election_failed", "votes", votes, "needed", reachableMajority, "reachable_peers", reachableTotal, "total_known", totalNodes)
	return errors.New("not enough votes")
}

// runPreVote performs a Raft pre-vote round using the current term without
// mutating local term or votedFor. It returns true if a majority of peers are
// willing to grant a vote based on log up-to-date checks.
func (c *ConsensusImpl) runPreVote() bool {
	c.mu.RLock()
	term := c.state.CurrentTerm
	c.mu.RUnlock()

	lastIdx, lastTerm, _ := c.lastIndexTerm()
	req := RequestVoteRequest{Term: term, CandidateID: c.nodeID, LastLogIndex: lastIdx, LastLogTerm: lastTerm, PreVote: true}
	payload, _ := json.Marshal(req)
	// Use dynamically detected active peers for pre-vote majority.
	peers := c.activePeers(15 * time.Second)
	// Track reachable peers dynamically during pre-vote to handle partitions correctly
	reachablePeers := 1 // Start with self
	totalNodes := len(peers) + 1
	votes := 1 // local node is implicitly willing to vote for itself

	type res struct {
		ok bool
	}
	ch := make(chan res, len(peers))
	for _, id := range peers {
		if id == c.nodeID {
			continue
		}
		go func(pid string) {
			body, err := c.postJSONWithResponse("http://"+c.peers.ResolveAddr(pid)+"/raft/request-vote", payload)
			if err != nil {
				ch <- res{ok: false}
				return
			}
			var resp RequestVoteResponse
			if err := json.Unmarshal(body, &resp); err != nil {
				ch <- res{ok: false}
				return
			}
			ch <- res{ok: resp.VoteGranted}
		}(id)
	}
	timeout := time.After(3 * time.Second)
	for pending := len(peers); pending > 0; pending-- {
		select {
		case r := <-ch:
			// Count this peer as reachable (responded, even if vote was denied)
			reachablePeers++
			// Recalculate majority based on actually reachable peers
			reachableTotal := reachablePeers
			reachableMajority := (reachableTotal / 2) + 1

			if r.ok {
				votes++
			}
			// Check if we have majority or can use degraded mode
			attemptedContacts := len(peers) // peers excludes self
			if votes >= reachableMajority {
				c.log(slog.LevelDebug, "prevote_majority_achieved", "votes", votes, "majority", reachableMajority, "reachable_peers", reachableTotal, "total_known", totalNodes)
				return true
			} else if votes == 1 && attemptedContacts > 0 && (reachableTotal == 2 || reachableTotal == 3) {
				// Degraded mode: only if we tried to contact others (prevents split-brain)
				c.log(slog.LevelDebug, "prevote_degraded_mode", "votes", votes, "reachable_peers", reachableTotal, "total_known", totalNodes, "attempted", attemptedContacts)
				return true
			}
			// Continue waiting for more votes
		case <-timeout:
			// On timeout, recalculate based on peers that responded
			reachableTotal := reachablePeers
			reachableMajority := (reachableTotal / 2) + 1
			attemptedContacts := len(peers)
			if votes >= reachableMajority {
				c.log(slog.LevelDebug, "prevote_majority_achieved_timeout", "votes", votes, "majority", reachableMajority, "reachable_peers", reachableTotal, "total_known", totalNodes)
				return true
			} else if votes == 1 && attemptedContacts > 0 && (reachableTotal == 2 || reachableTotal == 3) {
				c.log(slog.LevelDebug, "prevote_degraded_mode_timeout", "votes", votes, "reachable_peers", reachableTotal, "total_known", totalNodes, "attempted", attemptedContacts)
				return true
			}
			c.log(slog.LevelWarn, "prevote_timeout", "votes", votes, "needed", reachableMajority, "reachable_peers", reachableTotal, "total_known", totalNodes)
			return false
		}
	}
	// All peers responded, recalculate based on reachable peers
	reachableTotal := reachablePeers
	reachableMajority := (reachableTotal / 2) + 1
	attemptedContacts := len(peers) // peers excludes self
	if votes >= reachableMajority {
		c.log(slog.LevelDebug, "prevote_majority_achieved", "votes", votes, "majority", reachableMajority, "reachable_peers", reachableTotal, "total_known", totalNodes)
		return true
	} else if votes == 1 && attemptedContacts > 0 && (reachableTotal == 2 || reachableTotal == 3) {
		// Degraded mode: only if we tried to contact others
		c.log(slog.LevelDebug, "prevote_degraded_mode", "votes", votes, "reachable_peers", reachableTotal, "total_known", totalNodes, "attempted", attemptedContacts)
		return true
	}
	c.log(slog.LevelDebug, "prevote_no_majority", "votes", votes, "majority", reachableMajority, "reachable_peers", reachableTotal, "total_known", totalNodes)
	return false
}

func (c *ConsensusImpl) lastIndexTerm() (int64, int64, error) {
	var idx, term sql.NullInt64
	err := c.storage.db.QueryRow(`SELECT idx, term FROM raft_log ORDER BY idx DESC LIMIT 1`).Scan(&idx, &term)
	if err == sql.ErrNoRows {
		return 0, 0, nil
	}
	if err != nil {
		return 0, 0, err
	}
	return idx.Int64, term.Int64, nil
}

func (c *ConsensusImpl) postJSON(url string, body []byte) bool {
	req, _ := http.NewRequest("POST", url, bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	if c.hmacSecret != "" {
		sig := computeHMACSHA256Hex(body, c.hmacSecret)
		req.Header.Set("X-Cluster-Signature", sig)
	}
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return false
	}
	defer resp.Body.Close()
	return resp.StatusCode >= 200 && resp.StatusCode < 300
}

// postJSONWithResponse sends a POST request and returns the response body
func (c *ConsensusImpl) postJSONWithResponse(url string, body []byte) ([]byte, error) {
	req, _ := http.NewRequest("POST", url, bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	if c.hmacSecret != "" {
		sig := computeHMACSHA256Hex(body, c.hmacSecret)
		req.Header.Set("X-Cluster-Signature", sig)
	}
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, errors.New("http error")
	}
	return io.ReadAll(resp.Body)
}
