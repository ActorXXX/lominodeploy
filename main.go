package main

import (
	"embed"
	"fmt"
	"log"
	"os"

	"github.com/lominodev/lominodeploy/internal/config"
	"github.com/lominodev/lominodeploy/internal/server"
)

//go:embed web
var webFS embed.FS

const (
	Version    = "1.0.0"
	LLSBaseURL = "https://license.lominodev.com"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("Error cargando configuración: %v", err)
	}

	port := cfg.Server.Port
	if port == 0 {
		port = 8888
	}

	srv := server.New(cfg, webFS, Version, LLSBaseURL)

	fmt.Printf("\n")
	fmt.Printf("  LominoDeploy v%s\n", Version)
	fmt.Printf("  Panel disponible en → http://localhost:%d\n\n", port)

	if err := srv.Start(port); err != nil {
		log.Fatalf("Error iniciando servidor: %v", err)
		os.Exit(1)
	}
}
