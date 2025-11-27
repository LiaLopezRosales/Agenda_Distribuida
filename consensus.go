package agendadistribuida

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"strconv"
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
	cancel context.CancelFunc

	// apply callback to mutate the state machine (SQLite)
	applier func(LogEntry) error

	// networking
	httpClient *http.Client
	hmacSecret string
}

func NewConsensus(nodeID string, storage *Storage, peers PeerStore) *ConsensusImpl {
	return &ConsensusImpl{
		storage:    storage,
		peers:      peers,
		nodeID:     nodeID,
		role:       roleFollower,
		httpClient: &http.Client{Timeout: 1500 * time.Millisecond},
		hmacSecret: os.Getenv("CLUSTER_HMAC_SECRET"),
		logger:     Logger(),
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
	hb := time.NewTicker(500 * time.Millisecond)
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
				_ = c.broadcastAppendEntries(term, nil, commit)
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
		}
	}
}

func (c *ConsensusImpl) electionTimeout() time.Duration {
	// 1200-1800ms randomized
	return 1200*time.Millisecond + time.Duration(time.Now().UnixNano()%600_000_000)
}

func (c *ConsensusImpl) Propose(entry LogEntry) error {
	c.mu.RLock()
	if c.role != roleLeader {
		c.mu.RUnlock()
		c.log(slog.LevelWarn, "propose_rejected_not_leader", "leader", c.peers.GetLeader())
		return errors.New("not leader")
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
	if err := c.appendLog(entry); err != nil {
		c.log(slog.LevelError, "propose_append_failed", "err", err)
		return err
	}
	// replicate to followers and wait for majority to commit
	if err := c.replicateAndCommit(entry); err != nil {
		c.log(slog.LevelError, "propose_replicate_failed", "err", err)
		return err
	}
	c.log(slog.LevelInfo, "propose_committed", "index", entry.Index, "op", entry.Op)
	c.audit("propose", "log entry committed", map[string]any{"index": entry.Index, "op": entry.Op})
	return nil
}

func (c *ConsensusImpl) HandleAppendEntries(req AppendEntriesRequest) (AppendEntriesResponse, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if req.Term < c.state.CurrentTerm {
		c.log(slog.LevelDebug, "append_entries_reject_old_term", "term", req.Term)
		return AppendEntriesResponse{Term: c.state.CurrentTerm, Success: false, MatchIndex: c.state.LastApplied}, nil
	}
	// become follower on newer term and accept leader
	if req.Term > c.state.CurrentTerm {
		c.state.CurrentTerm = req.Term
		_ = c.persistMeta("currentTerm", c.state.CurrentTerm)
		c.audit("term_update", "term advanced from append entries", map[string]any{"term": c.state.CurrentTerm})
	}
	c.role = roleFollower
	c.peers.SetLeader(req.LeaderID)
	c.log(slog.LevelDebug, "append_entries_leader_seen", "leader_id", req.LeaderID, "term", req.Term)

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
		c.log(slog.LevelDebug, "request_vote_old_term", "candidate", req.CandidateID, "term", req.Term)
		return RequestVoteResponse{Term: c.state.CurrentTerm, VoteGranted: false}, nil
	}
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
		c.mu.Unlock()
	}
	return nil
}

// --- networking / majority replication ---

func (c *ConsensusImpl) broadcastAppendEntries(term int64, entries []LogEntry, leaderCommit int64) error {
	prevIdx, prevTerm, _ := c.lastIndexTerm()
	req := AppendEntriesRequest{
		Term:         term,
		LeaderID:     c.nodeID,
		PrevLogIndex: prevIdx,
		PrevLogTerm:  prevTerm,
		Entries:      entries,
		LeaderCommit: leaderCommit,
	}
	payload, _ := json.Marshal(req)
	successes := 1 // leader counts self
	peers := c.peers.ListPeers()
	ch := make(chan bool, len(peers))
	for _, id := range peers {
		if id == c.nodeID {
			continue
		}
		go func(pid string) {
			ok := c.postJSON("http://"+c.peers.ResolveAddr(pid)+"/raft/append-entries", payload)
			ch <- ok
		}(id)
	}
	majority := (len(peers) / 2) + 1
	timeout := time.After(1200 * time.Millisecond)
	for pending := len(peers) - 1; pending > 0; pending-- {
		select {
		case ok := <-ch:
			if ok {
				successes++
			}
			if successes >= majority {
				return nil
			}
		case <-timeout:
			c.log(slog.LevelWarn, "append_entries_timeout", "pending", pending)
			return errors.New("append majority timeout")
		}
	}
	if successes >= majority {
		return nil
	}
	c.log(slog.LevelError, "append_entries_no_majority", "successes", successes, "needed", majority)
	return errors.New("append majority failed")
}

func (c *ConsensusImpl) replicateAndCommit(entry LogEntry) error {
	if err := c.broadcastAppendEntries(entry.Term, []LogEntry{entry}, c.state.CommitIndex); err != nil {
		c.log(slog.LevelWarn, "replicate_failed", "err", err)
		return err
	}
	c.mu.Lock()
	if entry.Index > c.state.CommitIndex {
		c.state.CommitIndex = entry.Index
		_ = c.persistMeta("commitIndex", c.state.CommitIndex)
	}
	c.mu.Unlock()
	return nil
}

func (c *ConsensusImpl) startElection() error {
	c.log(slog.LevelInfo, "election_started")
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
	votes := 1
	peers := c.peers.ListPeers()
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
	majority := (len(peers) / 2) + 1
	timeout := time.After(1500 * time.Millisecond)
	for pending := len(peers) - 1; pending > 0; pending-- {
		select {
		case ok := <-ch:
			if ok {
				votes++
			}
			if votes >= majority {
				c.mu.Lock()
				c.role = roleLeader
				c.peers.SetLeader(c.nodeID)
				c.mu.Unlock()
				c.log(slog.LevelInfo, "election_won", "term", term)
				c.audit("election", "node became leader", map[string]any{"term": term})
				return nil
			}
		case <-timeout:
			c.log(slog.LevelWarn, "election_timeout")
			return errors.New("election timeout")
		}
	}
	if votes >= majority {
		c.mu.Lock()
		c.role = roleLeader
		c.peers.SetLeader(c.nodeID)
		c.mu.Unlock()
		c.log(slog.LevelInfo, "election_won", "term", term)
		c.audit("election", "node became leader", map[string]any{"term": term})
		return nil
	}
	c.log(slog.LevelWarn, "election_failed", "votes", votes, "needed", majority)
	return errors.New("not enough votes")
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
