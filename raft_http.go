package agendadistribuida

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"strings"
	"time"

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
			// Add CORS headers for browser requests
			origin := r.Header.Get("Origin")
			if origin != "" {
				w.Header().Set("Access-Control-Allow-Origin", origin)
			} else {
				w.Header().Set("Access-Control-Allow-Origin", "*")
			}
			w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, PATCH, OPTIONS")
			w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")
			w.Header().Set("Access-Control-Allow-Credentials", "true")
			w.Header().Set("Access-Control-Max-Age", "3600")

			// Handle preflight OPTIONS requests
			if r.Method == http.MethodOptions {
				w.WriteHeader(http.StatusOK)
				return
			}

			path := r.URL.Path

			// Never redirect internal control endpoints or public auth endpoints
			// /register and /login can be handled by any node (they don't require consensus)
			if strings.HasPrefix(path, "/raft/") ||
				strings.HasPrefix(path, "/cluster/") ||
				strings.HasPrefix(path, "/ws") ||
				strings.HasPrefix(path, "/ui/") ||
				path == "/register" ||
				path == "/login" {
				// Log for debugging
				Logger().Debug("middleware_allowing_local", "path", path, "method", r.Method, "is_leader", cons != nil && cons.IsLeader())
				next.ServeHTTP(w, r)
				return
			}

			// Only redirect write operations under /api/ to leader
			if cons == nil || cons.IsLeader() {
				next.ServeHTTP(w, r)
				return
			}

			if strings.HasPrefix(path, "/api/") {
				if r.Method == http.MethodPost || r.Method == http.MethodPut ||
					r.Method == http.MethodDelete || r.Method == http.MethodPatch {
					leaderID := cons.LeaderID()
					if leaderID != "" {
						addr := leaderAddrResolver(leaderID)
						// For browser requests, use proxy approach instead of redirect
						// because browsers may not follow redirects to internal IPs
						userAgent := r.Header.Get("User-Agent")
						if strings.Contains(userAgent, "Mozilla") ||
							strings.Contains(userAgent, "Chrome") ||
							strings.Contains(userAgent, "Safari") ||
							strings.Contains(userAgent, "Firefox") ||
							strings.Contains(userAgent, "Edg") {
							// Proxy the request to the leader
							proxyRequestToLeader(w, r, addr)
							return
						}
						// For non-browser requests, use redirect
						http.Redirect(w, r, "http://"+addr+r.RequestURI, http.StatusTemporaryRedirect)
						return
					}
				}
			}
			next.ServeHTTP(w, r)
		})
	}
}

// proxyRequestToLeader forwards the request to the leader and returns the response
func proxyRequestToLeader(w http.ResponseWriter, r *http.Request, leaderAddr string) {
	// Create a new request to the leader using internal Docker address
	leaderURL := "http://" + leaderAddr + r.RequestURI
	req, err := http.NewRequest(r.Method, leaderURL, r.Body)
	if err != nil {
		Logger().Error("proxy_request_create_failed", "err", err, "url", leaderURL)
		http.Error(w, "failed to create proxy request: "+err.Error(), http.StatusInternalServerError)
		return
	}

	// Copy headers but remove Host header to avoid issues
	for key, values := range r.Header {
		if strings.ToLower(key) == "host" {
			continue
		}
		for _, value := range values {
			req.Header.Add(key, value)
		}
	}

	// Make the request
	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		Logger().Error("proxy_request_failed", "err", err, "url", leaderURL)
		http.Error(w, "failed to reach leader: "+err.Error(), http.StatusBadGateway)
		return
	}
	defer resp.Body.Close()

	// Copy response headers (but preserve CORS headers we set)
	for key, values := range resp.Header {
		// Don't overwrite CORS headers we already set
		if strings.HasPrefix(strings.ToLower(key), "access-control-") {
			continue
		}
		for _, value := range values {
			w.Header().Add(key, value)
		}
	}

	// Copy status code and body
	w.WriteHeader(resp.StatusCode)
	if _, err := io.Copy(w, resp.Body); err != nil {
		Logger().Error("proxy_response_copy_failed", "err", err)
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
