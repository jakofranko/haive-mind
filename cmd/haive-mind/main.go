package main

import (
	"context"
	"encoding/json"
	"flag"
	"log/slog"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"github.com/charmbracelet/ssh"
	"jakofranko/haive-mind/hub"
	"jakofranko/haive-mind/sshserver"
)

type Transcript struct {
	Messages   []*hub.Message `json:"messages"`
	Participants []*hub.Participant `json:"participants"`
	StartedAt  time.Time      `json:"started_at"`
	EndedAt    time.Time      `json:"ended_at"`
}

func main() {
	port := flag.String("port", "2222", "SSH server port")
	hostKey := flag.String("host-key", "", "Path to SSH host key (generated if not provided)")
	dataDir := flag.String("data-dir", "", "Data directory for transcript storage")
	debug := flag.Bool("debug", false, "Enable debug logging (hub event trace)")
	logFile := flag.String("log-file", "", "Log file path (JSON format)")

	flag.Parse()

	setupLogging(*logFile, *debug)

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	hubChans := hub.NewHubChannels()
	server := sshserver.NewServer(hubChans, *port, *hostKey)

	var transcript *Transcript
	if *dataDir != "" {
		transcript = &Transcript{
			Messages:     make([]*hub.Message, 0),
			Participants: make([]*hub.Participant, 0),
			StartedAt:    time.Now(),
		}
		go collectTranscript(ctx, hubChans, transcript, *debug)
	}

	serverErr := make(chan error, 1)
	go func() {
		slog.Info("Starting haive-mind SSH server", "port", *port)
		if err := server.ListenAndServe(); err != nil {
			serverErr <- err
		}
	}()

	select {
	case <-ctx.Done():
		slog.Info("Shutdown signal received, initiating graceful shutdown")
		shutdown(server, hubChans, transcript, *dataDir)
	case err := <-serverErr:
		slog.Error("Server error", "error", err)
		shutdown(server, hubChans, transcript, *dataDir)
		os.Exit(1)
	}
}

func setupLogging(logFile string, debug bool) {
	var handler slog.Handler
	if logFile != "" {
		f, err := os.OpenFile(logFile, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
		if err != nil {
			slog.Error("Failed to open log file", "error", err)
			os.Exit(1)
		}
		handler = slog.NewJSONHandler(f, &slog.HandlerOptions{
			Level: slog.LevelDebug,
		})
	} else {
		level := slog.LevelInfo
		if debug {
			level = slog.LevelDebug
		}
		handler = slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
			Level: level,
		})
	}
	slog.SetDefault(slog.New(handler))
}

func collectTranscript(ctx context.Context, h hub.Hub, t *Transcript, debug bool) {
	for {
		select {
		case <-ctx.Done():
			return
		case msg := <-h.Outbound():
			t.Messages = append(t.Messages, msg)
			if debug {
				slog.Debug("hub event", "type", msg.Type, "from", msg.From, "content", msg.Content, "round", msg.Round)
			}
		case p := <-h.ParticipantUpdateChan():
			pCopy := p
			t.Participants = append(t.Participants, &pCopy)
		}
	}
}

func shutdown(server *ssh.Server, h hub.Hub, t *Transcript, dataDir string) {
	slog.Info("Broadcasting session ended message")
	endedMsg := hub.NewMessage(hub.MessageTypeSystem, "system", "Session ended", 0)
	h.SendChan() <- endedMsg

	slog.Info("Waiting for connections to close")
	time.Sleep(2 * time.Second)

	if t != nil && dataDir != "" {
		t.EndedAt = time.Now()
		if err := saveTranscript(t, dataDir); err != nil {
			slog.Error("Failed to save transcript", "error", err)
		} else {
			slog.Info("Transcript saved", "path", filepath.Join(dataDir, "transcript.json"))
		}
	}

	slog.Info("Shutting down server")
	server.Close()
}

func saveTranscript(t *Transcript, dataDir string) error {
	if err := os.MkdirAll(dataDir, 0755); err != nil {
		return err
	}
	path := filepath.Join(dataDir, "transcript.json")
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	enc := json.NewEncoder(f)
	enc.SetIndent("", "  ")
	return enc.Encode(t)
}