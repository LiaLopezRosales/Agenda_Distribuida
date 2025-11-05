package main

import (
	"log"
	"net/http"
	"os"
	"strings"

	ad "distributed-agenda"
)

func main() {
	storage, err := ad.NewStorage("file:agenda.db?cache=shared&_fk=1")
	if err != nil {
		log.Fatalf("storage init: %v", err)
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
	peersEnv := os.Getenv("PEERS") // comma-separated host:port ids
	var peerIDs []string
	if peersEnv != "" {
		peerIDs = strings.Split(peersEnv, ",")
	}
	ps := ad.NewEnvPeerStore(nodeID, peerIDs)
	cons := ad.NewConsensus(nodeID, storage, ps)
	cons.SetApplier(ad.NewRaftApplier(storage))
	if err := cons.Start(); err != nil {
		log.Fatalf("consensus: %v", err)
	}
	// best-effort leader discovery via health polling
	stopHB := make(chan struct{})
	ad.StartHeartbeats(ps, stopHB)

	// Pass storage as UserRepository, GroupRepository, and AppointmentRepository
	api := ad.NewAPI(auth, groups, apps, agenda, notes, storage, storage, storage)
	r := api.Router()
	// inject consensus into appointment service for write proposals
	apps.SetConsensus(cons)

	// Leader redirect middleware for writes
	r.Use(ad.LeaderWriteMiddleware(cons, ps.ResolveAddr))

	// Register Raft HTTP endpoints
	ad.RegisterRaftHTTP(r, cons)

	// Serve static UI under /ui/
	r.PathPrefix("/ui/").Handler(http.StripPrefix("/ui/", http.FileServer(http.Dir("web"))))
	// WebSocket endpoint
	r.HandleFunc("/ws", ad.ServeWS(storage, wsManager))

	log.Println("listening on :8080 (API) and serving UI at /ui/")
	if err := http.ListenAndServe(":8080", r); err != nil {
		log.Fatal(err)
	}
}
