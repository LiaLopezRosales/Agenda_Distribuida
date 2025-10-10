package main

import (
	"log"
	"net/http"

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

	// Pass storage as UserRepository, GroupRepository, and AppointmentRepository
	api := ad.NewAPI(auth, groups, apps, agenda, notes, storage, storage, storage)
	r := api.Router()

	// Serve static UI under /ui/
	r.PathPrefix("/ui/").Handler(http.StripPrefix("/ui/", http.FileServer(http.Dir("web"))))
	// WebSocket endpoint
	r.HandleFunc("/ws", ad.ServeWS(storage, wsManager))

	log.Println("listening on :8080 (API) and serving UI at /ui/")
	if err := http.ListenAndServe(":8080", r); err != nil {
		log.Fatal(err)
	}
}
