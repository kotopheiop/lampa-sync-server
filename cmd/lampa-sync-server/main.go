package main

import (
	"log"

	"github.com/kotopheiop/lampa-sync-server/internal/config"
	"github.com/kotopheiop/lampa-sync-server/internal/httpserver"
	"github.com/kotopheiop/lampa-sync-server/internal/store"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		log.Fatal(err)
	}

	st, err := store.New(cfg.DataDir)
	if err != nil {
		log.Fatal(err)
	}
	if err := st.Init(); err != nil {
		log.Fatal(err)
	}

	srv := httpserver.New(cfg, st)
	if err := srv.ListenAndServe(); err != nil {
		log.Fatal(err)
	}
}
