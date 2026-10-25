package main

import (
	"flag"
	"log"
	"os"

	"jakofranko/haive-mind/hub"
	"jakofranko/haive-mind/sshserver"
)

func main() {
	port := flag.String("port", "2222", "SSH server port")
	hostKey := flag.String("host-key", "", "Path to SSH host key (generated if not provided)")
	dataDir := flag.String("data-dir", "", "Data directory")
	debug := flag.Bool("debug", false, "Enable debug logging")
	logFile := flag.String("log-file", "", "Log file path")

	flag.Parse()

	if *debug {
		log.SetFlags(log.LstdFlags | log.Lshortfile)
	}

	if *logFile != "" {
		f, err := os.OpenFile(*logFile, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
		if err != nil {
			log.Fatalf("Failed to open log file: %v", err)
		}
		log.SetOutput(f)
	}

	_ = dataDir

	hubChans := hub.NewHubChannels()
	server := sshserver.NewServer(hubChans, *port, *hostKey)

	log.Printf("Starting haive-mind SSH server on port %s", *port)
	if err := server.ListenAndServe(); err != nil {
		log.Fatal(err)
	}
}