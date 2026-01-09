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
	r.HandleFunc("/cluster/local-audit/users", clusterLocalUserAuditHandler(store)).Methods(http.MethodGet)
	r.HandleFunc("/cluster/local-events/appointments", clusterLocalAppointmentsHandler(store)).Methods(http.MethodGet)
	r.HandleFunc("/cluster/local-events/groups", clusterLocalGroupsHandler(store)).Methods(http.MethodGet)
	r.HandleFunc("/cluster/local-events/group-members", clusterLocalGroupMembersHandler(store)).Methods(http.MethodGet)
	r.HandleFunc("/cluster/local-events/invitations", clusterLocalInvitationsHandler(store)).Methods(http.MethodGet)
	r.HandleFunc("/cluster/local-events/notifications", clusterLocalNotificationsHandler(store)).Methods(http.MethodGet)
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

func clusterLocalInvitationsHandler(store *Storage) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !validateClusterHMAC(w, r) {
			return
		}
		q := r.URL.Query().Get("since")
		filter := EventFilter{Entity: "invitation", Action: "status_change"}
		if q != "" {
			if ts, err := time.Parse(time.RFC3339, q); err == nil {
				filter.Since = ts
			}
		}
		filter.Limit = 1000
		events, err := store.ListEvents(filter)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(events)
	}
}

func clusterLocalNotificationsHandler(store *Storage) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !validateClusterHMAC(w, r) {
			return
		}
		q := r.URL.Query().Get("since")
		filter := EventFilter{Entity: "notification", Action: "create"}
		if q != "" {
			if ts, err := time.Parse(time.RFC3339, q); err == nil {
				filter.Since = ts
			}
		}
		filter.Limit = 1000
		events, err := store.ListEvents(filter)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(events)
	}
}

func clusterLocalGroupsHandler(store *Storage) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !validateClusterHMAC(w, r) {
			return
		}
		q := r.URL.Query().Get("since")
		filter := EventFilter{Entity: "group", Action: "create"}
		if q != "" {
			if ts, err := time.Parse(time.RFC3339, q); err == nil {
				filter.Since = ts
			}
		}
		filter.Limit = 1000
		events, err := store.ListEvents(filter)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(events)
	}
}

func clusterLocalGroupMembersHandler(store *Storage) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !validateClusterHMAC(w, r) {
			return
		}
		q := r.URL.Query().Get("since")
		filter := EventFilter{Entity: "group_member", Action: "add"}
		if q != "" {
			if ts, err := time.Parse(time.RFC3339, q); err == nil {
				filter.Since = ts
			}
		}
		filter.Limit = 1000
		events, err := store.ListEvents(filter)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(events)
	}
}

// clusterLocalAppointmentsHandler exposes local appointment events for
// reconciliation. It returns events with Entity="appointment" and
// Action="create" since an optional timestamp.
func clusterLocalAppointmentsHandler(store *Storage) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !validateClusterHMAC(w, r) {
			return
		}
		q := r.URL.Query().Get("since")
		filter := EventFilter{
			Entity: "appointment",
			Action: "create",
		}
		if q != "" {
			if ts, err := time.Parse(time.RFC3339, q); err == nil {
				filter.Since = ts
			}
		}
		// Reasonable safety cap to avoid flooding; reconciler keeps its own cursor.
		filter.Limit = 1000
		events, err := store.ListEvents(filter)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(events)
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

// clusterLocalUserAuditHandler exposes local user registration audit logs for
// reconciliation. It is intended for intra-cluster use only and is protected
// by the same HMAC mechanism as other cluster endpoints.
func clusterLocalUserAuditHandler(store *Storage) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !validateClusterHMAC(w, r) {
			return
		}
		q := r.URL.Query().Get("since")
		filter := AuditFilter{
			Component: "auth",
			Action:    "register",
		}
		if q != "" {
			if ts, err := time.Parse(time.RFC3339, q); err == nil {
				filter.Since = ts
			}
		}
		logs, err := store.ListAuditLogs(filter)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		// Return a reduced view with only the fields needed for reconciliation.
		// We expect the payload to contain username/email/display_name.
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(logs)
	}
}
