package main

import (
	"flag"
	"gaie/internal/config"
	"gaie/internal/iotjobs"
	"log"
	"os"
	"os/signal"
	"syscall"
)

var (
	configPath = flag.String("config", "configs/config.yaml", "Path to config file")
	envFlag    = flag.String("env", "", "override environment name")
)

func main() {
	cfg, err := config.LoadConfig(*configPath)
	if err != nil {
		log.Fatalf("Failed to load config: %v", err)
	}
	// Get environment configuration
	env, err := cfg.GetCurrentEnvironment(*envFlag)
	if err != nil {
		log.Fatalf("Failed to get environment: %v", err)
	}

	// Validate environment configuration
	if err := env.Validate(); err != nil {
		log.Fatalf("Invalid environment config: %v", err)
	}

	client, err := iotjobs.NewIoTClient(env)
	if err != nil {
		log.Fatalf("Failed to create IoT client: %v", err)
	}
	defer client.Close()

	// Setup shutdown channel
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)

	log.Println("IoT Agent started. Press CTRL+C to exit")
	<-sigChan
	log.Println("Shutting down...")
}
