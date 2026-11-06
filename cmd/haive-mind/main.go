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
	"jakofranko/haive-mind/coordinator"
	"jakofranko/haive-mind/hub"
	"jakofranko/haive-mind/sshserver"
)

type Transcript struct {
	Messages     []*hub.Message     `json:"messages"`
	Participants []*hub.Participant `json:"participants"`
	StartedAt    time.Time          `json:"started_at"`
	EndedAt      time.Time          `json:"ended_at"`
}

func main() {
	port := flag.String("port", "2222", "SSH server port")
	hostKey := flag.String("host-key", "", "Path to SSH host key (generated if not provided)")
	dataDir := flag.String("data-dir", "", "Data directory for transcript storage")
	debug := flag.Bool("debug", false, "Enable debug logging (hub event trace)")
	logFile := flag.String("log-file", "", "Log file path (JSON format)")

	flag.Parse()

	setupLogging(*logFile, *debug)
	slog.Info("Debug flag value", "value", *debug)

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	hubChans := hub.NewHub()
	slog.Debug("Hub created")

	server := sshserver.NewServer(hubChans, *port, *hostKey)

	// The coordinator owns the transcript, rounds and response tracking. The
	// hub owns agent invocation, so nothing here calls the agent.
	coord := coordinator.New(hubChans)
	if err := coord.Start(ctx); err != nil {
		slog.Error("Failed to start coordinator", "error", err)
		os.Exit(1)
	}
	startedAt := time.Now()

	if *debug {
		for _, msgType := range []hub.MessageType{
			hub.MessageTypeDirective,
			hub.MessageTypeResponse,
			hub.MessageTypeSystem,
			hub.MessageTypeAggregate,
			hub.MessageTypeNewRound,
			hub.MessageTypeRoundChange,
		} {
			hubChans.Subscribe(ctx, msgType, func(msg *hub.Message) error {
				slog.Debug("hub event", "type", msg.Type, "from", msg.From, "content", msg.Content, "round", msg.Round)
				return nil
			})
		}
	}

	snapshot := func() *Transcript {
		return &Transcript{
			Messages:     coord.Transcript(),
			Participants: hubChans.GetParticipants(),
			StartedAt:    startedAt,
			EndedAt:      time.Now(),
		}
	}

	// Handle export requests
	hubChans.Subscribe(ctx, hub.MessageTypeExport, func(msg *hub.Message) error {
		if *dataDir == "" {
			return nil
		}
		fullPath := filepath.Join(*dataDir, msg.Content)
		if err := saveTranscript(snapshot(), *dataDir, msg.Content); err != nil {
			slog.Error("Failed to export transcript", "error", err)
			return nil
		}
		hubChans.Send(ctx, hub.NewMessage(hub.MessageTypeExportComplete, "hub", fullPath, 0))
		return nil
	})

	// Start the hub
	if err := hubChans.Start(); err != nil {
		slog.Error("Failed to start hub", "error", err)
		os.Exit(1)
	}
	defer hubChans.Stop()

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
		shutdown(server, hubChans, snapshot, *dataDir)
	case err := <-serverErr:
		slog.Error("Server error", "error", err)
		shutdown(server, hubChans, snapshot, *dataDir)
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
	slog.Info("Logging configured", "logFile", logFile, "debug", debug)
}

func shutdown(server *ssh.Server, h hub.Hub, snapshot func() *Transcript, dataDir string) {
	slog.Info("Broadcasting session ended message")
	endedMsg := hub.NewMessage(hub.MessageTypeSystem, "system", "Session ended", 0)
	h.Send(context.Background(), endedMsg)

	slog.Info("Waiting for connections to close")
	time.Sleep(2 * time.Second)

	if dataDir != "" {
		if err := saveTranscript(snapshot(), dataDir, "transcript.json"); err != nil {
			slog.Error("Failed to save transcript", "error", err)
		} else {
			slog.Info("Transcript saved", "path", filepath.Join(dataDir, "transcript.json"))
		}
	}

	slog.Info("Shutting down server")
	server.Close()
}

func saveTranscript(t *Transcript, dataDir, filename string) error {
	if err := os.MkdirAll(dataDir, 0755); err != nil {
		return err
	}
	path := filepath.Join(dataDir, filename)
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	enc := json.NewEncoder(f)
	enc.SetIndent("", "  ")
	return enc.Encode(t)
}
