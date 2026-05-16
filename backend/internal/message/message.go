package message

type Message struct {
	From string `json:"from"`
	Text string `json:"text"`
	To   string `json:"to"`
	TS   int64  `json:"ts"`
}
