package agendadistribuida

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"strings"

	"github.com/gorilla/mux"
)

func RegisterRaftHTTP(r *mux.Router, cons Consensus) {
	r.HandleFunc("/raft/health", func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]any{
			"node_id":   cons.NodeID(),
			"is_leader": cons.IsLeader(),
			"leader":    cons.LeaderID(),
		})
	}).Methods("GET")

	r.HandleFunc("/raft/request-vote", func(w http.ResponseWriter, r *http.Request) {
		if !validateClusterHMAC(w, r) {
			return
		}
		var req RequestVoteRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		resp, err := cons.HandleRequestVote(req)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		json.NewEncoder(w).Encode(resp)
	}).Methods("POST")

	r.HandleFunc("/raft/append-entries", func(w http.ResponseWriter, r *http.Request) {
		if !validateClusterHMAC(w, r) {
			return
		}
		var req AppendEntriesRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		resp, err := cons.HandleAppendEntries(req)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		json.NewEncoder(w).Encode(resp)
	}).Methods("POST")
}

// Middleware that redirects write methods to leader if current node is follower.
func LeaderWriteMiddleware(cons Consensus, leaderAddrResolver func(string) string) mux.MiddlewareFunc {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if cons == nil || cons.IsLeader() {
				next.ServeHTTP(w, r)
				return
			}
			// Only redirect mutating methods under /api
			if r.Method == http.MethodPost || r.Method == http.MethodPut || r.Method == http.MethodDelete || r.Method == http.MethodPatch {
				leaderID := cons.LeaderID()
				if leaderID != "" {
					addr := leaderAddrResolver(leaderID)
					// send HTTP 307 to preserve method and body
					http.Redirect(w, r, "http://"+addr+r.RequestURI, http.StatusTemporaryRedirect)
					return
				}
			}
			next.ServeHTTP(w, r)
		})
	}
}

// --- HMAC guard ---

func validateClusterHMAC(w http.ResponseWriter, r *http.Request) bool {
	secret := strings.TrimSpace(os.Getenv("CLUSTER_HMAC_SECRET"))
	if secret == "" {
		http.Error(w, "cluster secret not configured", http.StatusInternalServerError)
		return false
	}
	sig := r.Header.Get("X-Cluster-Signature")
	if sig == "" {
		http.Error(w, "missing signature", http.StatusUnauthorized)
		return false
	}
	body, _ := io.ReadAll(r.Body)
	r.Body = io.NopCloser(bytes.NewReader(body))
	if !verifyHMACSHA256Hex(body, secret, sig) {
		http.Error(w, "invalid signature", http.StatusUnauthorized)
		return false
	}
	return true
}
