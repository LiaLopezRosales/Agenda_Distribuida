package agendadistribuida

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/gorilla/mux"
)

func RegisterClusterHTTP(r *mux.Router, store *Storage, peers *EnvPeerStore) {
	r.HandleFunc("/cluster/join", clusterJoinHandler(store, peers)).Methods(http.MethodPost)
	r.HandleFunc("/cluster/leave", clusterLeaveHandler(store, peers)).Methods(http.MethodPost)
	r.HandleFunc("/cluster/nodes", clusterNodesHandler(store)).Methods(http.MethodGet)
}

func clusterJoinHandler(store *Storage, peers *EnvPeerStore) http.HandlerFunc {
	type joinReq struct {
		NodeID  string `json:"node_id"`
		Address string `json:"address"`
		Source  string `json:"source"`
	}
	return func(w http.ResponseWriter, r *http.Request) {
		if !validateClusterHMAC(w, r) {
			return
		}
		var req joinReq
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		if req.Address == "" {
			http.Error(w, "address is required", http.StatusBadRequest)
			return
		}
		if req.NodeID == "" {
			req.NodeID = req.Address
		}
		node := &ClusterNode{
			NodeID:   req.NodeID,
			Address:  req.Address,
			Source:   fallback(req.Source, "gossip"),
			LastSeen: time.Now(),
		}
		if err := store.UpsertClusterNode(node); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		peers.UpsertPeer(node.NodeID, node.Address)
		nodes, err := store.ListClusterNodes()
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		RecordAudit(r.Context(), AuditLevelInfo, "cluster", "join", "peer joined cluster", map[string]any{
			"node_id": node.NodeID,
			"address": node.Address,
		})
		json.NewEncoder(w).Encode(map[string]any{
			"status": "joined",
			"nodes":  nodes,
		})
	}
}

func clusterLeaveHandler(store *Storage, peers *EnvPeerStore) http.HandlerFunc {
	type leaveReq struct {
		NodeID string `json:"node_id"`
	}
	return func(w http.ResponseWriter, r *http.Request) {
		if !validateClusterHMAC(w, r) {
			return
		}
		var req leaveReq
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		if req.NodeID == "" {
			http.Error(w, "node_id is required", http.StatusBadRequest)
			return
		}
		if err := store.RemoveClusterNode(req.NodeID); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		peers.RemovePeer(req.NodeID)
		RecordAudit(r.Context(), AuditLevelInfo, "cluster", "leave", "peer left cluster", map[string]any{
			"node_id": req.NodeID,
		})
		json.NewEncoder(w).Encode(map[string]string{"status": "removed"})
	}
}

func clusterNodesHandler(store *Storage) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !validateClusterHMAC(w, r) {
			return
		}
		nodes, err := store.ListClusterNodes()
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		json.NewEncoder(w).Encode(map[string]any{"nodes": nodes})
	}
}
