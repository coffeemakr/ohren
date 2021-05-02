package websocket

type ClientMessageType int

type ClientMessage struct {
	Type            ClientMessageType
	RequestedAmount int `json:"ra,omitempty"`
}

var (
	TypeGenerate ClientMessageType = 1
)
