package main

import (
	"github.com/dgraph-io/badger/v4"
	"github.com/dontlaugh/disorder"
	"github.com/gorilla/mux"
	"log"
	"net/http"
)

func main() {
	// Open Badger DB
	db, err := badger.Open(badger.DefaultOptions("./data"))
	if err != nil {
		log.Fatalf("failed to open badger: %v", err)
	}
	defer db.Close()

	r := mux.NewRouter()
	xeno := &disorder.Xeno{DB: db}
	r.Handle("/route/{uuid}", xeno)
	xeno.R = r

	log.Println("Server is running on :8080")
	if err := http.ListenAndServe(":8080", r); err != nil {
		log.Fatalf("server failed: %v", err)
	}
}
