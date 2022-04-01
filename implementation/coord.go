package implementation

import (
	"ephemeralrocket/util"
	"math"
	"net"
	"net/rpc"
)

type CoordConfig struct {
	ClientListenAddr string
	ServerListenAddr string
	LostMsgsThresh   uint8
	NumServers       uint8
}

type ServerDetails struct {
	ServerId         uint8
	ServerAddr       string   // IP:Port that the server uses to listen to requests from other servers
	CoordAddr        string   // IP:Port that coord can use to send messages to server
	ClientAddr       string   // IP:Port that the servers uses to listen to requests from clients
	FCheckAddr       string   // IP:Port used to monitor server for fcheck
	PrevId           uint8    // Id of the previous server in the ring
	NextId           uint8    // Id of the next server in the ring
	PrimaryClients   []string // List of client IDs for which this server is a primary.
	SecondaryClients []string // List of client IDs for which this server is a secondary.
}

// Use this struct for RPC methods
type CoordRPC struct {
	NumServers       uint8                    // Expected number of servers
	LostMsgsThresh   uint8                    // FCheck threshold
	FCheckIP         string                   // IP used for fcheck
	IsRingReady      bool                     // Flag for when ring is being reconfigured
	ServerDetailsMap map[uint8]*ServerDetails // Maps server id to its details, including clients for which it is a primary.
	PrimaryClientMap map[string]uint8         // Maps a client id to its primary server id
	ClientList       []string                 // List of client ids in the system
}

type Coord struct {
	cRPC CoordRPC
}

func NewCoord() *Coord {
	return &Coord{}
}

// ! This function is mostly identical to one used in A3.
func (c *Coord) Start(config CoordConfig) error {
	c.cRPC = CoordRPC{
		NumServers:     config.NumServers,
		LostMsgsThresh: config.LostMsgsThresh,
		FCheckIP:       config.ServerListenAddr,
	}

	// Resolve addresses
	localClientAddr, err := net.ResolveTCPAddr("tcp", config.ClientListenAddr)
	util.CheckErr(err, "Could not resolve local client address: %s\n", config.ClientListenAddr)
	localServerAddr, err := net.ResolveTCPAddr("tcp", config.ServerListenAddr)
	util.CheckErr(err, "Could not resolve local server address: %s\n", config.ServerListenAddr)

	// Register RPC server and setup listeners
	rpc.Register(&c.cRPC)
	clientListener, err := net.ListenTCP("tcp", localClientAddr)
	util.CheckErr(err, "Could not listen on IP/Port: %s\n", config.ClientListenAddr)
	serverListener, err := net.ListenTCP("tcp", localServerAddr)
	util.CheckErr(err, "Could not listen on IP/Port: %s\n", config.ServerListenAddr)

	go rpc.Accept(clientListener)
	go rpc.Accept(serverListener)

	// Block forever using empty channel
	block := make(chan int)
	<-block

	return nil
}

// Required RPC Calls: BEGIN

// ServerRequestToJoin
// Server calls this, requesting to join the system. Coord caches this request.
// Server must provide an address that the coord can use to contact them, as well as an address to use for fcheck.
// ! This function is mostly identical to one used in A3.
func (c *CoordRPC) ServerRequestToJoin(req *ServerRequestToJoinReq, res *interface{}) error {
	println("server requested to join: %d", req.ServerId)
	// Cache server details
	c.ServerDetailsMap[req.ServerId] = &ServerDetails{
		ServerId:         req.ServerId,
		ServerAddr:       req.ServerServerAddr,
		CoordAddr:        req.ServerConnectAddr,
		ClientAddr:       req.ServerClientAddr,
		FCheckAddr:       req.FCheckAddr,
		PrimaryClients:   make([]string, 0),
		SecondaryClients: make([]string, 0),
	}

	if len(c.ServerDetailsMap) == int(c.NumServers) {
		go CreateServerRing(c)
	}
	println("server successfully joined: %d", req.ServerId)
	return nil
}

// ClientJoin
// Client calls this through MessageLib, requesting to join the system. Coord processes this
// and acknowledges that the client has joined.
func (c *CoordRPC) ClientJoin(req *ClientCoordReq, res *ClientJoinRes) error {

	// Return if ring is being configured
	if !c.IsRingReady {
		res.IsReady = false
		return nil
	}

	// Assign primary server with helper.
	c.ClientList = append(c.ClientList, req.ClientId)
	c.PrimaryClientMap[req.ClientId] = AssignPrimaryServer(c)
	res.IsReady = true
	return nil
}

// GetPrimaryServer
// Client calls this through MessageLib, requesting the details of its primary server. Coord
// returns the primary server for that client.
func (c *CoordRPC) RetrievePrimaryServer(req *PrimaryServerReq, res *PrimaryServerRes) error {
	res.ClientId = req.ClientId
	if !c.IsRingReady {
		res.ChainReady = false
		return nil
	}

	if v, ok := c.PrimaryClientMap[req.ClientId]; ok {
		// If a primary has been assigned, retrieve it.
		res.PrimaryServerIPPort = c.ServerDetailsMap[v].ClientAddr
	} else {
		// Otherwise, assign a new primary and provide it to the client.
		primaryServerId := AssignPrimaryServer(c)
		c.PrimaryClientMap[req.ClientId] = primaryServerId
		res.PrimaryServerIPPort = c.ServerDetailsMap[primaryServerId].ClientAddr
	}
	res.ChainReady = true
	return nil
}

// RetrieveClients
// Client calls this through MessageLib, requesting all known clients in the system. Coord returns
// the list of client IDs representing the clients that are known to the system.
func (c *CoordRPC) RetrieveClients(req *interface{}, res *RetrieveClientsRes) error {
	res.ClientIds = c.ClientList
	return nil
}

// Required RPC Calls: END

// Required Internal Methods: BEGIN

// CreateServerRing
// Called once all servers have joined the system. Coord will have to make an RPC call on the server.
func CreateServerRing(c *CoordRPC) {
	length := len(c.ServerDetailsMap)
	serverNextOrder := make([]uint8, length)
	serverPrevOrder := make([]uint8, length)

	counter := 1
	for k := range c.ServerDetailsMap {
		// use modulo to assign each server to the right position in the prev and next order.
		serverPrevOrder[counter] = k
		serverNextOrder[(length-1)-counter] = k
		counter = (counter + 1) % length
	}

	counter = 0
	for _, v := range c.ServerDetailsMap {
		// assign each server its prev and next server.
		v.PrevId = serverPrevOrder[counter]
		v.NextId = serverNextOrder[(length-1)-counter]
		counter++
	}

	// Make RPC call to each server with its prev and next server info
	serverCalls := make([]*rpc.Call, length)
	doneChan := make(chan *rpc.Call, length)
	counter = 0
	for _, v := range c.ServerDetailsMap {
		client, err := rpc.Dial("tcp", v.CoordAddr)
		util.CheckErr(err, "could not connect to server with ID: %d", v.ServerId) // This error should never happen.
		req := ConnectRingReq{
			PrevServerId:   v.PrevId,
			PrevServerAddr: c.ServerDetailsMap[v.PrevId].ServerAddr,
			NextServerId:   v.NextId,
			NextServerAddr: c.ServerDetailsMap[v.NextId].ServerAddr,
		}
		var res interface{}
		serverCalls[counter] = client.Go("ServerRPC.ConnectRing", &req, &res, doneChan)
	}

	counter = 0
	for counter < length {
		// This error should never happen.
		util.CheckErr(serverCalls[counter].Error, "Error during call to ServerRPC.ConnectRing for server")
		<-serverCalls[counter].Done
	}
	println("Successfully joined all servers.")
}

// AssignPrimaryServer
// Called when a client is joining. Assigns a primary server based on a simple algorithm, used to calculate load on the server.
// The load on a server is given by 2 * number of primary clients + number of secondary clients.
func AssignPrimaryServer(c *CoordRPC) uint8 {
	primaryServerId := uint8(0)
	minLoad := int(math.Inf(1))

	for k, v := range c.ServerDetailsMap {
		serverLoad := 2*len(v.PrimaryClients) + len(v.SecondaryClients)
		if serverLoad < minLoad {
			minLoad = serverLoad
			primaryServerId = k
		}
	}
	return primaryServerId
}

// MonitorServerFailures
// Uses fcheck under the hood. All servers in the system will be monitored using the addresses they provided.
// This triggers the failure protocol, which involves RPC calls on the servers adjacent to the failed server.
func MonitorServerFailures(c *CoordRPC) {

}

// Required Internal Methods: END
