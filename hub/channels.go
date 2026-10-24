package hub

// HubChannels provides typed channels for communication between TUI and hub
type HubChannels struct {
	Inbound  chan *Message
	Outbound chan *Message

	// Commands (typed channels)
	AggregateChan chan int      // round number
	SendChan      chan *Message // directive to send
	NewRoundChan  chan int      // next round number
	ExportChan    chan string   // export path

	// Events (typed channels)
	ParticipantUpdateChan chan Participant
	RoundChangedChan      chan int
	ExportCompleteChan    chan string
	ErrorChan             chan error
}

func NewHubChannels() *HubChannels {
	return &HubChannels{
		Inbound:               make(chan *Message, 100),
		Outbound:              make(chan *Message, 100),
		AggregateChan:         make(chan int, 10),
		SendChan:              make(chan *Message, 10),
		NewRoundChan:          make(chan int, 10),
		ExportChan:            make(chan string, 10),
		ParticipantUpdateChan: make(chan Participant, 100),
		RoundChangedChan:      make(chan int, 10),
		ExportCompleteChan:    make(chan string, 10),
		ErrorChan:             make(chan error, 10),
	}
}
