package hub

// Hub defines the interface for the hub channels
type Hub interface {
	Inbound() chan *Message
	Outbound() chan *Message
	AggregateChan() chan int
	SendChan() chan *Message
	NewRoundChan() chan int
	ExportChan() chan string
	ParticipantUpdateChan() chan Participant
	RoundChangedChan() chan int
	ExportCompleteChan() chan string
	ErrorChan() chan error
}

// HubChannels provides typed channels for communication between TUI and hub
type HubChannels struct {
	inbound               chan *Message
	outbound              chan *Message
	aggregateChan         chan int
	sendChan              chan *Message
	newRoundChan          chan int
	exportChan            chan string
	participantUpdateChan chan Participant
	roundChangedChan      chan int
	exportCompleteChan    chan string
	errorChan             chan error
}

func NewHubChannels() *HubChannels {
	return &HubChannels{
		inbound:               make(chan *Message, 100),
		outbound:              make(chan *Message, 100),
		aggregateChan:         make(chan int, 10),
		sendChan:              make(chan *Message, 10),
		newRoundChan:          make(chan int, 10),
		exportChan:            make(chan string, 10),
		participantUpdateChan: make(chan Participant, 100),
		roundChangedChan:      make(chan int, 10),
		exportCompleteChan:    make(chan string, 10),
		errorChan:             make(chan error, 10),
	}
}

func (h *HubChannels) Inbound() chan *Message                { return h.inbound }
func (h *HubChannels) Outbound() chan *Message               { return h.outbound }
func (h *HubChannels) AggregateChan() chan int               { return h.aggregateChan }
func (h *HubChannels) SendChan() chan *Message               { return h.sendChan }
func (h *HubChannels) NewRoundChan() chan int                { return h.newRoundChan }
func (h *HubChannels) ExportChan() chan string               { return h.exportChan }
func (h *HubChannels) ParticipantUpdateChan() chan Participant { return h.participantUpdateChan }
func (h *HubChannels) RoundChangedChan() chan int            { return h.roundChangedChan }
func (h *HubChannels) ExportCompleteChan() chan string       { return h.exportCompleteChan }
func (h *HubChannels) ErrorChan() chan error                 { return h.errorChan }