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

	r := ad.NewRouter(storage)

	// Serve static UI under /ui/
	r.PathPrefix("/ui/").Handler(http.StripPrefix("/ui/", http.FileServer(http.Dir("web"))))

	log.Println("listening on :8080 (API) and serving UI at /ui/")
	if err := http.ListenAndServe(":8080", r); err != nil {
		log.Fatal(err)
	}
}
