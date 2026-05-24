package message

type Message struct {
	From string `json:"from"`
	Text string `json:"text"`
	To   string `json:"to"`
	Room string `json:"room"`
	TS   int64  `json:"ts"`
	Ping bool   `json:"ping,omitempty"` // keepalive — not routed to the hub
}
