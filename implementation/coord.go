package implementation

import "errors"

type CoordConfig struct {
	ClientListenAddr string
	ServerListenAddr string
	LostMsgsThresh   uint8
	NumServers       uint8
}

// Use this struct for RPC methods
type CoordRPC struct {
}

type Coord struct {
}

func NewCoord() *Coord {
	return &Coord{}
}

func (c *Coord) Start(config CoordConfig) error {
	return errors.New("not implemented")
}

// Required RPC Calls: BEGIN

// ServerRequestToJoin
// Server calls this, requesting to join the system. Coord caches this request.
// Server must provide an address that the coord can use to contact them, as well as an address to use for fcheck.

// ClientJoin
// Client calls this through MessageLib, requesting to join the system. Coord processes this
// and acknowledges that the client has joined.

// GetPrimaryServer
// Client calls this through MessageLib, requesting the details of its primary server. Coord
// returns the primary server for that client.

// RetrieveClients
// Client calls this through MessageLib, requesting all known clients in the system. Coord returns
// the list of client IDs representing the clients that are known to the system.

// Required RPC Calls: END

// Required Internal Methods: BEGIN

// CreateServerRing
// Called once all servers have joined the system. Coord will have to make an RPC call on the server.

// MonitorServerFailures
// Uses fcheck under the hood. All servers in the system will be monitored using the addresses they provided.
// This triggers the failure protocol, which involves RPC calls on the servers adjacent to the failed server.

// Required Internal Methods: END
