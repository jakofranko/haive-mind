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

	var transcript *Transcript
	if *dataDir != "" {
		transcript = &Transcript{
			Messages:     make([]*hub.Message, 0),
			Participants: make([]*hub.Participant, 0),
			StartedAt:    time.Now(),
		}
	}

	// Track participant responses per round for aggregation
	responsesByRound := make(map[int][]*hub.Message)
	currentRound := 1

	// Subscribe to messages for transcript collection
	if transcript != nil {
		messageTypes := []hub.MessageType{
			hub.MessageTypeDirective,
			hub.MessageTypeResponse,
			hub.MessageTypeSystem,
			hub.MessageTypeAggregate,
			hub.MessageTypeNewRound,
			hub.MessageTypeRoundChange,
			hub.MessageTypeExport,
			hub.MessageTypeExportComplete,
		}

		for _, msgType := range messageTypes {
			hubChans.Subscribe(ctx, msgType, func(msg *hub.Message) error {
				transcript.Messages = append(transcript.Messages, msg)
if *debug {
					slog.Debug("hub event", "type", msg.Type, "from", msg.From, "content", msg.Content, "round", msg.Round)
				}
				return nil
			})
		}

		// Subscribe to participant events
		hubChans.Subscribe(ctx, hub.MessageTypeParticipantEvent, func(msg *hub.Message) error {
			participants := hubChans.GetParticipants()
			for _, p := range participants {
				if p.Name == msg.ID {
					transcript.Participants = append(transcript.Participants, p)
					break
				}
			}
			return nil
		})
	}

	// Subscribe to track participant responses for aggregation
	hubChans.Subscribe(ctx, hub.MessageTypeResponse, func(msg *hub.Message) error {
		if msg.From != "agent" {
			responsesByRound[msg.Round] = append(responsesByRound[msg.Round], msg)
			slog.Debug("Tracked participant response", "round", msg.Round, "from", msg.From)
		}
		return nil
	})

	// Route directives to agent
	hubChans.Subscribe(ctx, hub.MessageTypeDirective, func(msg *hub.Message) error {
		slog.Debug("Sending directive to agent", "from", msg.From, "round", msg.Round, "content", msg.Content)
		response, err := hubChans.SendToAgent(ctx, msg.Content)
		if err != nil {
			slog.Error("Agent error on directive", "error", err)
			errMsg := hub.NewMessage(hub.MessageTypeSystem, "system", "Agent error: "+err.Error(), currentRound)
			hubChans.Send(ctx, errMsg)
			return nil
		}
		slog.Debug("Agent replied to directive", "response", response)
		agentReply := hub.NewMessage(hub.MessageTypeResponse, "agent", response, currentRound)
		hubChans.Send(ctx, agentReply)
		return nil
	})

	// Route aggregates to agent
	hubChans.Subscribe(ctx, hub.MessageTypeAggregate, func(msg *hub.Message) error {
		// Skip aggregate_request signals
		if msg.Content == "aggregate_request" {
			return nil
		}
		slog.Debug("Sending aggregate to agent", "round", msg.Round, "content", msg.Content)
		response, err := hubChans.SendToAgent(ctx, msg.Content)
		if err != nil {
			slog.Error("Agent error on aggregate", "error", err)
			errMsg := hub.NewMessage(hub.MessageTypeSystem, "system", "Agent error: "+err.Error(), currentRound)
			hubChans.Send(ctx, errMsg)
			return nil
		}
		slog.Debug("Agent replied to aggregate", "response", response)
		agentReply := hub.NewMessage(hub.MessageTypeResponse, "agent", response, currentRound)
		hubChans.Send(ctx, agentReply)
		return nil
	})

	// Handle aggregate requests from host - build aggregate from tracked responses
	hubChans.Subscribe(ctx, hub.MessageTypeAggregate, func(msg *hub.Message) error {
		responses := responsesByRound[currentRound]
		if len(responses) > 0 {
			var parts []string
			for _, r := range responses {
				parts = append(parts, r.From+": "+r.Content)
			}
			aggregateContent := "Aggregated responses: " + joinStrings(parts, "; ")
			aggregateMsg := hub.NewMessage(hub.MessageTypeAggregate, "Host", aggregateContent, currentRound)
			hubChans.Send(ctx, aggregateMsg)
		}
		return nil
	})

	// Handle new round requests
	hubChans.Subscribe(ctx, hub.MessageTypeNewRound, func(msg *hub.Message) error {
		currentRound++
		slog.Debug("New round started", "round", currentRound)
		responsesByRound[currentRound] = make([]*hub.Message, 0)
		roundMsg := hub.NewMessage(hub.MessageTypeRoundChange, "hub", "", currentRound)
		hubChans.Send(ctx, roundMsg)
		return nil
	})

	// Handle export requests
	hubChans.Subscribe(ctx, hub.MessageTypeExport, func(msg *hub.Message) error {
		if transcript != nil && *dataDir != "" {
			fullPath := filepath.Join(*dataDir, msg.Content)
			if err := saveTranscript(transcript, *dataDir, msg.Content); err != nil {
				slog.Error("Failed to export transcript", "error", err)
			} else {
				completeMsg := hub.NewMessage(hub.MessageTypeExportComplete, "hub", fullPath, 0)
				hubChans.Send(ctx, completeMsg)
			}
		}
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
		shutdown(server, hubChans, transcript, *dataDir)
	case err := <-serverErr:
		slog.Error("Server error", "error", err)
		shutdown(server, hubChans, transcript, *dataDir)
		os.Exit(1)
	}
}

func joinStrings(parts []string, sep string) string {
	if len(parts) == 0 {
		return ""
	}
	result := parts[0]
	for i := 1; i < len(parts); i++ {
		result += sep + parts[i]
	}
	return result
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

func shutdown(server *ssh.Server, h hub.Hub, t *Transcript, dataDir string) {
	slog.Info("Broadcasting session ended message")
	endedMsg := hub.NewMessage(hub.MessageTypeSystem, "system", "Session ended", 0)
	h.Send(context.Background(), endedMsg)

	slog.Info("Waiting for connections to close")
	time.Sleep(2 * time.Second)

	if t != nil && dataDir != "" {
		t.EndedAt = time.Now()
		if err := saveTranscript(t, dataDir, "transcript.json"); err != nil {
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