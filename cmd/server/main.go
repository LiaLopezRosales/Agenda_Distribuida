package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"strings"
	"time"

	ad "distributed-agenda"
)

func main() {
	dsn := strings.TrimSpace(os.Getenv("DATABASE_DSN"))
	if dsn == "" {
		dsn = "file:agenda.db?cache=shared&_fk=1"
	}
	storage, err := ad.NewStorage(dsn)
	if err != nil {
		log.Fatalf("storage init: %v", err)
	}
	ad.SetAuditRepository(storage)
	clusterSecret := strings.TrimSpace(os.Getenv("CLUSTER_HMAC_SECRET"))
	if clusterSecret == "" {
		log.Fatal("CLUSTER_HMAC_SECRET must be defined to secure cluster RPC traffic")
	}

	wsManager := ad.NewWSManager()
	go wsManager.Run()

	// Build services
	auth := ad.NewAuthService(storage)
	groups := ad.NewGroupService(storage, storage)
	apps := ad.NewAppointmentService(storage, storage, storage, ad.NewNoopEventBus(), ad.NewNoopReplication())
	agenda := ad.NewAgendaService(storage, storage)
	notes := ad.NewNotificationService(storage)

	// Consensus wiring
	nodeID := os.Getenv("NODE_ID")
	if nodeID == "" {
		nodeID = "node-unknown"
	}
	ad.SetNodeMetadata(nodeID)
	advertiseAddr := os.Getenv("ADVERTISE_ADDR")
	if advertiseAddr == "" {
		advertiseAddr = "localhost:8080"
	}
	peersEnv := os.Getenv("PEERS") // comma-separated host:port ids
	var peerIDs []string
	if peersEnv != "" {
		peerIDs = strings.Split(peersEnv, ",")
	}
	ps := ad.NewEnvPeerStore(nodeID, peerIDs)
	discovery := ad.NewDiscoveryManager(storage, ps, nodeID, advertiseAddr)
	discovery.Start()
	cons := ad.NewConsensus(nodeID, storage, ps)
	cons.SetApplier(ad.NewRaftApplier(storage))
	if err := cons.Start(); err != nil {
		log.Fatalf("consensus: %v", err)
	}
	ad.RecordAudit(context.Background(), ad.AuditLevelInfo, "node", "start", "node boot sequence", map[string]any{
		"node_id": nodeID,
		"peers":   peerIDs,
		"addr":    advertiseAddr,
	})
	// best-effort leader discovery via health polling
	stopHB := make(chan struct{})
	ad.StartHeartbeats(ps, stopHB)

	// Pass storage as UserRepository, GroupRepository, and AppointmentRepository
	api := ad.NewAPI(auth, groups, apps, agenda, notes, storage, storage, storage, storage)
	r := api.Router()
	// inject consensus into appointment service for write proposals
	apps.SetConsensus(cons)

	// Leader redirect middleware for writes
	r.Use(ad.LeaderWriteMiddleware(cons, ps.ResolveAddr))

	// Register Raft HTTP endpoints
	ad.RegisterRaftHTTP(r, cons)
	ad.RegisterClusterHTTP(r, storage, ps)

	// Serve static UI under /ui/
	r.PathPrefix("/ui/").Handler(http.StripPrefix("/ui/", http.FileServer(http.Dir("web"))))
	// WebSocket endpoint
	r.HandleFunc("/ws", ad.ServeWS(storage, wsManager))

	addr := strings.TrimSpace(os.Getenv("HTTP_ADDR"))
	if addr == "" {
		addr = ":8080"
	}
	server := &http.Server{
		Addr:         addr,
		Handler:      r,
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 15 * time.Second,
		IdleTimeout:  60 * time.Second,
	}
	cert := strings.TrimSpace(os.Getenv("TLS_CERT_FILE"))
	key := strings.TrimSpace(os.Getenv("TLS_KEY_FILE"))
	if cert != "" && key != "" {
		log.Printf("listening on %s with TLS enabled", addr)
		if err := server.ListenAndServeTLS(cert, key); err != nil {
			log.Fatal(err)
		}
	} else {
		log.Printf("listening on %s over HTTP (set TLS_CERT_FILE/TLS_KEY_FILE for TLS)", addr)
		if err := server.ListenAndServe(); err != nil {
			log.Fatal(err)
		}
	}
}
