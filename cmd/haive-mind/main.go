package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"net"
	"os"
	"os/signal"
	"path/filepath"
	"runtime"
	"syscall"
	"time"

	"github.com/charmbracelet/ssh"
	"github.com/jakofranko/haive-mind/agent"
	"github.com/jakofranko/haive-mind/coordinator"
	"github.com/jakofranko/haive-mind/hub"
	"github.com/jakofranko/haive-mind/sshserver"
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
	hostKey := flag.String("host-key", "", "Path to the SSH host key; generated if missing. Default: id_ed25519 in the data directory (see README for per-OS locations)")
	dataDir := flag.String("data-dir", "", "Directory for saved transcripts. Default: haive-mind under $XDG_DATA_HOME (~/.local/share; %LocalAppData% on Windows; ~/Library/Application Support on macOS)")
	debug := flag.Bool("debug", false, "Enable debug logging (hub event trace)")
	logFile := flag.String("log-file", "", "Log file path (JSON format). Default: haive-mind.log under $XDG_STATE_HOME (~/.local/state; %LocalAppData% on Windows; ~/Library/Logs on macOS), with logs also printed when stdout is a terminal")
	agentTimeout := flag.Duration("agent-timeout", 120*time.Second, "How long to wait for the agent (and the synthesizer) to answer")

	flag.Parse()

	home, _ := os.UserHomeDir() // empty when unknown; resolveLocations then needs XDG variables
	loc, err := resolveLocations(*hostKey, *dataDir, *logFile, os.Getenv, runtime.GOOS, home)
	if err != nil {
		fmt.Fprintln(os.Stderr, "haive-mind:", err)
		os.Exit(1)
	}
	*hostKey, *dataDir = loc.HostKey, loc.DataDir

	setupLogging(loc.LogFile, *debug, logStdout(loc.LogIsDefault, isTerminal(os.Stdout), os.Stdout))
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
	startedAt := time.Now()
	transcriptOf := func(messages []*hub.Message) *Transcript {
		return &Transcript{
			Messages:     messages,
			Participants: hubChans.GetParticipants(),
			StartedAt:    startedAt,
			EndedAt:      time.Now(),
		}
	}
	snapshot := func() *Transcript { return transcriptOf(coord.Transcript()) }
	// The Coordinator handles export requests in order with everything else and
	// hands the transcript as it stood to this writer. With no data directory
	// there is no writer, and an export does nothing.
	if *dataDir != "" {
		coord.SetExportWriter(func(messages []*hub.Message, name string) (string, error) {
			return saveTranscript(transcriptOf(messages), *dataDir, name)
		})
	}
	if err := coord.Start(ctx); err != nil {
		slog.Error("Failed to start coordinator", "error", err)
		os.Exit(1)
	}

	if *debug {
		for _, msgType := range hub.RecordedTypes() {
			hubChans.Subscribe(ctx, msgType, func(msg *hub.Message) error {
				slog.Debug("hub event", "type", msg.Type, "from", msg.From, "content", msg.Content, "round", msg.Round)
				return nil
			})
		}
	}

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

// isTerminal reports whether f is an interactive terminal rather than a pipe
// or a file.
func isTerminal(f *os.File) bool {
	fi, err := f.Stat()
	return err == nil && fi.Mode()&os.ModeCharDevice != 0
}

func setupLogging(logFile string, debug bool, stdout io.Writer) {
	logger, _, err := newLogger(logFile, debug, stdout)
	if err != nil {
		slog.Error("Failed to open log file", "error", err)
		os.Exit(1)
	}
	// SetDefault also routes the standard library's log package through here.
	slog.SetDefault(logger)
	slog.Info("Logging configured", "logFile", logFile, "debug", debug)
}

// newLogger writes the events as JSON to logFile (when set) and as readable
// text to stdout (when not nil). Debug detail appears only when debug is set.
// The returned func closes the log file.
func newLogger(logFile string, debug bool, stdout io.Writer) (*slog.Logger, func(), error) {
	level := slog.LevelInfo
	if debug {
		level = slog.LevelDebug
	}
	opts := &slog.HandlerOptions{Level: level}
	var handlers []slog.Handler
	if stdout != nil {
		handlers = append(handlers, slog.NewTextHandler(stdout, opts))
	}
	closeLog := func() {}
	if logFile != "" {
		f, err := os.OpenFile(logFile, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
		if err != nil {
			return nil, nil, err
		}
		handlers = append(handlers, slog.NewJSONHandler(f, opts))
		closeLog = func() { f.Close() }
	}
	if len(handlers) == 1 {
		return slog.New(handlers[0]), closeLog, nil
	}
	return slog.New(slog.NewMultiHandler(handlers...)), closeLog, nil
}

func shutdown(server *ssh.Server, h hub.Hub, snapshot func() *Transcript, dataDir string) {
	slog.Info("Broadcasting session ended message")
	endedMsg := hub.NewMessage(hub.MessageTypeSystem, "system", "Session ended", 0)
	h.Send(context.Background(), endedMsg)

	slog.Info("Waiting for connections to close")
	time.Sleep(2 * time.Second)

	if dataDir != "" {
		if path, err := saveTranscript(snapshot(), dataDir, "transcript.json"); err != nil {
			slog.Error("Failed to save transcript", "error", err)
		} else {
			slog.Info("Transcript saved", "path", path)
		}
	}

	slog.Info("Shutting down server")
	server.Close()
}

// saveTranscript writes t as indented JSON to filename in dataDir and returns
// the path it wrote.
func saveTranscript(t *Transcript, dataDir, filename string) (string, error) {
	if err := os.MkdirAll(dataDir, 0755); err != nil {
		return "", err
	}
	path := filepath.Join(dataDir, filename)
	f, err := os.Create(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	enc := json.NewEncoder(f)
	enc.SetIndent("", "  ")
	return path, enc.Encode(t)
}

// logStdout is where the readable log text goes: out, unless the log file is
// the default one and out is not a terminal. The default log file is for
// unattended runs, so stdout gets the logs only when someone is watching it; a
// log file the user named keeps logs on stdout too.
func logStdout(logIsDefault, terminal bool, out io.Writer) io.Writer {
	if logIsDefault && !terminal {
		return nil
	}
	return out
}
