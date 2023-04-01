package message

import (
	"encoding/json"
	"time"
)

type Message struct {
	ID      string
	Time    time.Time
	Payload []byte
}

func (m *Message) Serialize() []byte {
	bytes, err := json.Marshal(m)
	if err != nil {
		panic(err)
	}
	return bytes
}
