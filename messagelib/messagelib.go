package messagelib

import (
	"ephemeralrocket/implementation"
	"errors"
)

// Use this struct for RPC methods
type MessageLibRPC struct {
}

type MessageLib struct {
}

func NewMessageLib() *MessageLib {
	return &MessageLib{}
}

func (m *MessageLib) Start(config implementation.ClientConfig) error {
	return errors.New("not implemented")
}

func (m *MessageLib) ViewClients() []string {
	return make([]string, 0)
}

func (m *MessageLib) SendMessage(message implementation.MessageStruct) error {
	return errors.New("not implemented")
}

func (m *MessageLib) RetrieveMessages(sourceClientId string) []implementation.MessageStruct {
	return make([]implementation.MessageStruct, 0)
}

func (m *MessageLib) Stop() {

}

// MessageLib is only an RPC Client. It will only call RPC methods on the coord and server.

// Required Internal Methods: BEGIN

// Decide based on how we split up the code. Everything apart from Start and Stop is blocking.

// Required Internal Methods: END
