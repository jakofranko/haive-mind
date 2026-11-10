package main

import (
	"context"
	"encoding/json"
	"flag"
	"io"
	"log/slog"
	"net"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"github.com/charmbracelet/ssh"
	"jakofranko/haive-mind/agent"
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
	bind := flag.String("bind", "", "Address to listen on; empty means every interface (use 127.0.0.1 to accept local connections only)")
	hostKey := flag.String("host-key", "", "Path to SSH host key. If empty, an id_ed25519 key is generated in (or reused from) the current directory")
	dataDir := flag.String("data-dir", "", "Data directory for transcript storage")
	debug := flag.Bool("debug", false, "Enable debug logging (hub event trace)")
	logFile := flag.String("log-file", "", "Log file path (JSON format)")
	agentTimeout := flag.Duration("agent-timeout", 120*time.Second, "How long to wait for the agent (and the synthesizer) to answer")

	flag.Parse()

	setupLogging(*logFile, *debug)
	slog.Info("Debug flag value", "value", *debug)

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	hubChans := hub.NewHub()
	hubChans.SetAgentRunner(agent.NewRunner(*agentTimeout))
	hubChans.SetSynthesizer(agent.NewRunner(*agentTimeout))
	slog.Debug("Hub created")

	server := sshserver.NewServer(hubChans, net.JoinHostPort(*bind, *port), *hostKey)

	// The coordinator owns the transcript, rounds and response tracking. The
	// hub owns agent invocation, so nothing here calls the agent.
	coord := coordinator.New(hubChans)
	if err := coord.Start(ctx); err != nil {
		slog.Error("Failed to start coordinator", "error", err)
		os.Exit(1)
	}
	startedAt := time.Now()

	if *debug {
		for _, msgType := range hub.RecordedTypes() {
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
	logger, _, err := newLogger(logFile, debug, os.Stdout)
	if err != nil {
		slog.Error("Failed to open log file", "error", err)
		os.Exit(1)
	}
	// SetDefault also routes the standard library's log package through here.
	slog.SetDefault(logger)
	slog.Info("Logging configured", "logFile", logFile, "debug", debug)
}

// newLogger writes readable text to stdout and, when logFile is set, the same
// events as JSON to that file. Debug detail appears only when debug is set.
// The returned func closes the log file.
func newLogger(logFile string, debug bool, stdout io.Writer) (*slog.Logger, func(), error) {
	level := slog.LevelInfo
	if debug {
		level = slog.LevelDebug
	}
	opts := &slog.HandlerOptions{Level: level}
	text := slog.NewTextHandler(stdout, opts)
	if logFile == "" {
		return slog.New(text), func() {}, nil
	}
	f, err := os.OpenFile(logFile, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return nil, nil, err
	}
	return slog.New(slog.NewMultiHandler(text, slog.NewJSONHandler(f, opts))), func() { f.Close() }, nil
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
