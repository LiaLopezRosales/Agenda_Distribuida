package agendadistribuida

import "sync"

type EnvPeerStore struct {
	mu      sync.RWMutex
	localID string
	peers   map[string]string // nodeID -> address
	leader  string
}

func NewEnvPeerStore(localID string, peers []string) *EnvPeerStore {
	store := &EnvPeerStore{localID: localID, peers: map[string]string{}}
	store.SetPeers(peers)
	return store
}

func (p *EnvPeerStore) LocalID() string { return p.localID }

func (p *EnvPeerStore) ListPeers() []string {
	p.mu.RLock()
	defer p.mu.RUnlock()
	ids := make([]string, 0, len(p.peers))
	for id := range p.peers {
		ids = append(ids, id)
	}
	return ids
}

func (p *EnvPeerStore) SetLeader(id string) { p.mu.Lock(); p.leader = id; p.mu.Unlock() }
func (p *EnvPeerStore) GetLeader() string   { p.mu.RLock(); defer p.mu.RUnlock(); return p.leader }

func (p *EnvPeerStore) ResolveAddr(id string) string {
	p.mu.RLock()
	defer p.mu.RUnlock()
	if v, ok := p.peers[id]; ok && v != "" {
		return v
	}
	return id
}

// SetPeers replaces the peer list using the supplied identifiers.
func (p *EnvPeerStore) SetPeers(ids []string) {
	snapshot := make(map[string]string)
	for _, id := range ids {
		if id == "" || id == p.localID {
			continue
		}
		snapshot[id] = id
	}
	p.SetSnapshot(snapshot)
}

// SetSnapshot replaces the peer map using nodeID->address pairs.
func (p *EnvPeerStore) SetSnapshot(entries map[string]string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.peers = make(map[string]string)
	for id, addr := range entries {
		if id == "" || id == p.localID {
			continue
		}
		if addr == "" {
			addr = id
		}
		p.peers[id] = addr
	}
}

// UpsertPeer inserts or updates a peer entry.
func (p *EnvPeerStore) UpsertPeer(id, addr string) {
	if id == "" || id == p.localID {
		return
	}
	if addr == "" {
		addr = id
	}
	p.mu.Lock()
	p.peers[id] = addr
	p.mu.Unlock()
}

// RemovePeer removes a peer from the store.
func (p *EnvPeerStore) RemovePeer(id string) {
	if id == "" {
		return
	}
	p.mu.Lock()
	delete(p.peers, id)
	p.mu.Unlock()
}
