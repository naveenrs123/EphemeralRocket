package implementation

import "errors"

type ServerConfig struct {
	ServerId         uint8
	CoordAddr        string
	LocalCoordAddr   string
	ServerListenAddr string
	ClientListenAddr string
}

// Use this struct for RPC methods
type ServerRPC struct {
}

type Server struct {
}

func NewServer() *Server {
	return &Server{}
}

func (s *Server) Start(config ServerConfig) error {
	return errors.New("not implemented")
}

// Required RPC Calls: BEGIN
// ConnectRing
// Coord calls this, informing the server what its adjacent servers are in the ring structure.

// AssignRole
// Coord calls this, assigning the server a role as either a primary or a secondary.

// ReceiveSenderMessage
// Client calls this through MessageLib. Server will receive a MessageStruct from the client and will
// start sending it through the ring to the primary server of the receiver.

// ForwardMessage
// Server calls this, forwarding the message to the next server in the chain. Cache the message
// if it cannot be forwarded, so that it can be forwarded once the server failures are handled.

// RetrieveMessages
// Client calls this through MessageLib on its primary server. If a second client id is provided, retrieves unread messages
// between the calling client and the other client. If not, retrieves all unread messages for the client. Returns a
// slice of MessageStructs.

// HandleFailure
// Coord calls this, informing the server about what its new role and adjacent servers are. The server
// may need to forward any unacknowledged messages once the chain is reconfigured.

// RetrieveCachedMessages
// Server calls this when it is a new primary and needs to retrieve cached messages from one of its secondaries.

// SendCachedMessages
// Server calls this when it is an existing primary and needs to send cached messages to a new secondary.

// Required RPC Calls: END

// Required Internal Methods: BEGIN

// Required Internal Methods: END
