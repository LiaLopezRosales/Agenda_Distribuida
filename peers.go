package agendadistribuida

import "sync"

type EnvPeerStore struct {
	mu      sync.RWMutex
	localID string
	peers   []string
	leader  string
	// optional map id->address (id equals address by default)
	idToAddr map[string]string
}

func NewEnvPeerStore(localID string, peers []string) *EnvPeerStore {
	return &EnvPeerStore{localID: localID, peers: peers, idToAddr: map[string]string{}}
}

func (p *EnvPeerStore) LocalID() string     { return p.localID }
func (p *EnvPeerStore) ListPeers() []string { return append([]string{}, p.peers...) }
func (p *EnvPeerStore) SetLeader(id string) { p.mu.Lock(); p.leader = id; p.mu.Unlock() }
func (p *EnvPeerStore) GetLeader() string   { p.mu.RLock(); defer p.mu.RUnlock(); return p.leader }

func (p *EnvPeerStore) SetAddr(id, addr string) { p.mu.Lock(); p.idToAddr[id] = addr; p.mu.Unlock() }
func (p *EnvPeerStore) ResolveAddr(id string) string {
	p.mu.RLock()
	defer p.mu.RUnlock()
	if v, ok := p.idToAddr[id]; ok {
		return v
	}
	return id
}
