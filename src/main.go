package main

import (
	"log"
	"lynx_agent_api/src/config"
	"lynx_agent_api/src/infra/db"
	schemas "lynx_agent_api/src/infra/schemas"
	"lynx_agent_api/src/network"
	"net/http"
)

func main() {

	config.LoadConfig()

	if err := db.InitDB(); err != nil {
		log.Fatalf("Failed to initialize database: %v", err)
	}

	if err := db.InitRedis(); err != nil {
		log.Fatalf("Failed to initialize Redis: %v", err)
	}

	if err := db.AutoMigrate(
		&schemas.Servers{},
	); err != nil {
		log.Fatalf("Failed to run migrations: %v", err)
	}

	router := network.SetupRouter()

	cfg := config.AppConfig
	addr := ":" + cfg.Server.Port

	log.Printf("Starting server on port %s...", cfg.Server.Port)
	if err := http.ListenAndServe(addr, router); err != nil {
		log.Fatalf("Failed to start server: %v", err)
	}
}
