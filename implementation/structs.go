package implementation

import "time"

// Add common structs used between nodes, e.g. those used in RPC calls here.

// Server <-> Coord
type ServerRequestToJoinReq struct {
	ServerId          uint8
	ServerConnectAddr string // Address that coord can use to make RPC calls to server.
	ServerServerAddr  string // Address that server uses to listen to other servers.
	ServerClientAddr  string // Address that server uses to listen to clients.
	FCheckAddr        string // New address that server uses to listen to heartbeats using Fcheck.
}

type ConnectRingReq struct {
	PrevServerId   uint8
	PrevServerAddr string
	NextServerId   uint8
	NextServerAddr string
}

type AssignRoleReq struct {
	ClientId string
	Role     ServerRole
}

type HandleFailureReq struct {
	PrevServerId   uint8
	PrevServerAddr string
	NextServerId   uint8
	NextServerAddr string
	ClientIds      []string
	// Server needs to be aware of new clients that now see this server as their primary.
	// List of client IDs. When HandleFailure is called, the server may now become a primary
	// for some new clients.

	// Server must then retrieve cached data from one of its secondaries for the clients in ClientIds
}

// Client <-> Coord
// Use this for all RPC calls between Client and Coord
type ClientCoordReq struct {
	ClientId string
}

type ClientJoinRes struct {
	IsReady bool
}

type GetPrimaryServerRes struct {
	PrimaryServerAddr string
}

type RetrieveClientsRes struct {
	ClientIds []string
}

// Client <-> Server
type RetrieveMessageReq struct {
	ClientId      string
	OtherClientId string // leave as "" to retrieve all unread messages.
}

// Server <-> Server

// Common
type MessageStruct struct {
	sourceId      string
	destinationId string
	data          string    // must be less than 300 characters
	timestamp     time.Time // leave blank as client
}

// Other Types/Enums
type ServerRole uint8

const (
	Primary ServerRole = iota
	Secondary
	Routing // may not need this.
)
