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

	hubChans := hub.NewHubChannels()
	slog.Debug("Hub channels created")
	server := sshserver.NewServer(hubChans, *port, *hostKey)

	var transcript *Transcript
	transcriptChan := make(chan *hub.Message, 100)
	participantUpdateChan := make(chan hub.Participant, 100)
	if *dataDir != "" {
		transcript = &Transcript{
			Messages:     make([]*hub.Message, 0),
			Participants: make([]*hub.Participant, 0),
			StartedAt:    time.Now(),
		}
		go collectTranscript(ctx, transcriptChan, participantUpdateChan, transcript, *debug)
	}

	// Track participant responses per round for aggregation
	responsesByRound := make(map[int][]*hub.Message)
	currentRound := 1

	// Consume SendChan: track participant responses, forward to Outbound (for broadcast) and transcriptChan, send directives/aggregates to agent
	go func() {
		slog.Debug("SendChan consumer started")
		for {
			select {
			case <-ctx.Done():
				return
			case msg := <-hubChans.SendChan():
				slog.Debug("SendChan message received", "type", msg.Type, "from", msg.From, "round", msg.Round)
				// Track participant responses before forwarding
				if msg.Type == hub.MessageTypeResponse && msg.From != "agent" {
					// Use server's current round for tracking
					responsesByRound[currentRound] = append(responsesByRound[currentRound], msg)
					slog.Debug("Tracked participant response", "round", currentRound, "from", msg.From)
				}
				// Forward to Outbound for broadcastLoop
				hubChans.Outbound() <- msg
				// Also send to transcriptChan for collection
				if transcript != nil {
					transcriptChan <- msg
				}
				slog.Debug("Forwarded to Outbound and transcriptChan", "type", msg.Type, "from", msg.From)

				// Send directives and aggregates to agent
				if msg.Type == hub.MessageTypeDirective || msg.Type == hub.MessageTypeAggregate {
					slog.Debug("Sending to agent", "type", msg.Type, "from", msg.From, "round", currentRound)
					response, err := hubChans.SendToAgent(ctx, msg.Content)
					if err != nil {
						slog.Error("Agent error", "error", err)
						errMsg := hub.NewMessage(hub.MessageTypeSystem, "system", "Agent error: "+err.Error(), currentRound)
						hubChans.Outbound() <- errMsg
						if transcript != nil {
							transcriptChan <- errMsg
						}
						continue
					}
					slog.Debug("Agent replied", "response", response)
					agentReply := hub.NewMessage(hub.MessageTypeResponse, "agent", response, currentRound)
					hubChans.Outbound() <- agentReply
					if transcript != nil {
						transcriptChan <- agentReply
					}
				}
			}
		}
	}()

	// Forward participant updates to transcript
	go func() {
		for {
			select {
			case <-ctx.Done():
				return
			case p := <-hubChans.ParticipantUpdateChan():
				if transcript != nil {
					participantUpdateChan <- p
				}
			}
		}
	}()

	// Consume AggregateChan: build aggregate from tracked responses and send to SendChan
	go func() {
		slog.Debug("AggregateChan consumer started")
		for {
			select {
			case <-ctx.Done():
				return
			case <-hubChans.AggregateChan():
				responses := responsesByRound[currentRound]
				if len(responses) == 0 {
					slog.Debug("No responses to aggregate", "round", currentRound)
					continue
				}
				var parts []string
				for _, r := range responses {
					parts = append(parts, r.From+": "+r.Content)
				}
				aggregateContent := "Aggregated responses: " + joinStrings(parts, "; ")
				aggregateMsg := hub.NewMessage(hub.MessageTypeAggregate, "Host", aggregateContent, currentRound)
				// Send to SendChan only (it will forward to Outbound for broadcast + agent)
				hubChans.SendChan() <- aggregateMsg
			}
		}
	}()

	// Consume NewRoundChan: increment round and notify all
	go func() {
		slog.Debug("NewRoundChan consumer started")
		for {
			select {
			case <-ctx.Done():
				return
			case <-hubChans.NewRoundChan():
				currentRound++
				slog.Debug("New round started", "round", currentRound)
				// Clear old responses
				responsesByRound[currentRound] = make([]*hub.Message, 0)
				// Notify all TUIs of new round
				hubChans.RoundChangedChan() <- currentRound
			}
		}
	}()

	// Consume ExportChan: save transcript and notify completion
	go func() {
		slog.Debug("ExportChan consumer started")
		for {
			select {
			case <-ctx.Done():
				return
			case path := <-hubChans.ExportChan():
				if transcript != nil && *dataDir != "" {
					fullPath := filepath.Join(*dataDir, path)
					if err := saveTranscript(transcript, *dataDir, path); err != nil {
						slog.Error("Failed to export transcript", "error", err)
					} else {
						hubChans.ExportCompleteChan() <- fullPath
					}
				}
			}
		}
	}()

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

func collectTranscript(ctx context.Context, msgCh <-chan *hub.Message, participantCh <-chan hub.Participant, t *Transcript, debug bool) {
	for {
		select {
		case <-ctx.Done():
			return
		case msg := <-msgCh:
			t.Messages = append(t.Messages, msg)
			if debug {
				slog.Debug("hub event", "type", msg.Type, "from", msg.From, "content", msg.Content, "round", msg.Round)
			}
		case p := <-participantCh:
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
