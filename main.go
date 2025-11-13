package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"
)

const version = "0.1.0"

type Config struct {
	SSHPort    int
	SSHHostKey string
	ModemID    int
	APN        string
	Debug      bool
}

func main() {
	config := Config{}

	flag.IntVar(&config.SSHPort, "port", 2222, "SSH server port")
	flag.StringVar(&config.SSHHostKey, "hostkey", "~/.ssh/landline_host_key", "SSH host key path")
	flag.IntVar(&config.ModemID, "modem", 0, "ModemManager modem ID")
	flag.StringVar(&config.APN, "apn", "RESELLER", "APN for cellular connection")
	flag.BoolVar(&config.Debug, "debug", false, "Enable debug logging")
	showVersion := flag.Bool("version", false, "Show version")

	flag.Parse()

	if *showVersion {
		fmt.Printf("landline v%s\n", version)
		os.Exit(0)
	}

	if config.Debug {
		log.SetFlags(log.LstdFlags | log.Lshortfile)
	}

	log.Printf("Starting landline v%s", version)

	// Initialize modem
	modem, err := NewModem(config.ModemID, config.APN)
	if err != nil {
		log.Fatalf("Failed to initialize modem: %v", err)
	}

	// Connect to network
	log.Println("Connecting to cellular network...")
	if err := modem.Connect(); err != nil {
		log.Fatalf("Failed to connect modem: %v", err)
	}
	log.Println("Modem connected successfully")

	// Start SMS monitoring
	go modem.StartSMSMonitoring()

	// Start SSH server
	server, err := NewSSHServer(config.SSHPort, config.SSHHostKey, modem)
	if err != nil {
		log.Fatalf("Failed to create SSH server: %v", err)
	}

	go func() {
		log.Printf("SSH server listening on port %d", config.SSHPort)
		if err := server.Start(); err != nil {
			log.Fatalf("SSH server error: %v", err)
		}
	}()

	// Wait for interrupt
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)
	<-sigChan

	log.Println("Shutting down...")
	if err := modem.Disconnect(); err != nil {
		log.Printf("Error disconnecting modem: %v", err)
	}
	server.Stop()
	log.Println("Goodbye!")
}
