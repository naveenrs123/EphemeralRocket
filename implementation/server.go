package implementation

import (
	fchecker "ephemeralrocket/fcheck"
	"ephemeralrocket/util"
	"net"
	"net/rpc"
)

type ServerConfig struct {
	ServerId         uint8
	CoordAddr        string
	ServerAddr       string
	ServerServerAddr string
	ServerListenAddr string
	ClientListenAddr string
}

// Use this struct for RPC methods
type ServerRPC struct {
	serverId         uint8
	serverAddr       string
	coordAddr        string
	serverListenAddr string // the listening address for adjacent servers to connect to
	clientListenAddr string
	prevServerAddr   string
	nextServerAddr   string
	serverServerAddr string
	primaryClients   []string
	secondaryClients []string
	block            chan bool
	cachedMessages   map[string][]MessageStruct // maps clientId -> list of messages
}

type Server struct {
	sRPC ServerRPC
}

func NewServer() *Server {
	return &Server{}
}

func SetupListeners(sRPC *ServerRPC, serverListenAddr string, addrToSendToCoord string, server *rpc.Server) {
	// RPC server for coord
	listenForCoordAddr, err := net.ResolveTCPAddr("tcp", addrToSendToCoord)
	util.CheckErr(err, "Failed to resolve address %s", addrToSendToCoord)
	listenForCoord, err := net.ListenTCP("tcp", listenForCoordAddr)
	util.CheckErr(err, "Server at %s failed to listen for coord node connection", sRPC.serverAddr)
	go server.Accept(listenForCoord)

	// RPC server for other servers
	serverListenAddrTCP, err := net.ResolveTCPAddr("tcp", serverListenAddr)
	util.CheckErr(err, "Failed to resolve address %s", serverListenAddr)
	listenForServers, err := net.ListenTCP("tcp", serverListenAddrTCP)
	util.CheckErr(err, "Server at %s failed to listen for servers connection", sRPC.serverAddr)
	go server.Accept(listenForServers)

	// RPC server for clients
	listenForClientsAddr, err := net.ResolveTCPAddr("tcp", sRPC.clientListenAddr)
	util.CheckErr(err, "Failed to resolve address %s", sRPC.clientListenAddr)
	listenForClients, err := net.ListenTCP("tcp", listenForClientsAddr)
	util.CheckErr(err, "Server at %s failed to listen for clients connection", sRPC.serverAddr)
	go server.Accept(listenForClients)
}

func (s *Server) Start(config ServerConfig) error {

	s.sRPC = ServerRPC{
		serverId:         config.ServerId,
		serverAddr:       config.ServerAddr,
		coordAddr:        config.CoordAddr,
		serverListenAddr: config.ServerListenAddr, // address for coord to call rpc methods on this server
		clientListenAddr: config.ClientListenAddr, // address for client to call rpc methods on this server
		prevServerAddr:   "",
		nextServerAddr:   "",
		serverServerAddr: config.ServerServerAddr, // address for server to call rpc methods on this server
		block:            make(chan bool),
	}

	server := rpc.NewServer()
	server.Register(&s.sRPC)

	// Create IP:Port so coord can connect to rpc on this server
	ip := util.ExtractIP(s.sRPC.serverAddr)
	portToSendToCoord := util.GetRandomPort(ip)
	addrToSendToCoord := ip + ":" + portToSendToCoord

	// Start Fcheck
	portToSendToCoordForFcheck := util.GetRandomPort(ip)
	addrToSendToCoordForFcheck := ip + ":" + portToSendToCoordForFcheck
	_, err := fchecker.Start(fchecker.StartStruct{AckLocalIPAckLocalPort: addrToSendToCoordForFcheck})
	util.CheckErr(err, "error starting fcheck")

	SetupListeners(&s.sRPC, s.sRPC.serverListenAddr, addrToSendToCoord, server)

	// RPC client for coord
	serverAddrTCP, err := net.ResolveTCPAddr("tcp", s.sRPC.serverAddr)
	util.CheckErr(err, "error resolving address %s", s.sRPC.serverAddr)
	coordAddrTCP, err := net.ResolveTCPAddr("tcp", s.sRPC.coordAddr)
	util.CheckErr(err, "error resolving address %s", s.sRPC.coordAddr)
	conn, err := net.DialTCP("tcp", serverAddrTCP, coordAddrTCP)
	util.CheckErr(err, "Server at %s failed to connect to coord node", s.sRPC.serverAddr)
	coordRpc := rpc.NewClient(conn)

	// Connect to coord and send join request
	serverJoinReq := ServerRequestToJoinReq{
		ServerId:          s.sRPC.serverId,
		ServerConnectAddr: s.sRPC.serverListenAddr,
		ServerServerAddr:  addrToSendToCoord,
		ServerClientAddr:  s.sRPC.clientListenAddr,
		FCheckAddr:        addrToSendToCoordForFcheck,
	}

	var serverJoinRes interface{}
	err = coordRpc.Call("CoordRPC.ServerJoin", serverJoinReq, &serverJoinRes)
	util.CheckErr(err, "Unable to call ServerJoin to coord from %s id %d", s.sRPC.serverAddr, s.sRPC.serverId)

	// Block forever using empty channel
	<-s.sRPC.block

	return nil
}

// Required RPC Calls: BEGIN
// ConnectRing
// Coord calls this, informing the server what its adjacent servers are in the ring structure.
func (sRPC *ServerRPC) ConnectRing(req *ConnectRingReq, res *ConnectRingRes) error {
	sRPC.nextServerAddr = req.NextServerAddr
	sRPC.prevServerAddr = req.PrevServerAddr
	// not using Ids because there doesn't seem to be a use, add them to sRPC struct if necessary

	res.ServerId = sRPC.serverId
	return nil
}

// AssignRole
// Coord calls this, assigning the server a role as either a primary or secondary.
func (sRPC *ServerRPC) AssignRole(req *AssignRoleReq, res *AssignRoleRes) error {
	if req.Role == ServerRole(1) { // primary server
		sRPC.primaryClients = append(sRPC.primaryClients, req.ClientId)
		sRPC.secondaryClients = util.RemoveElement(sRPC.secondaryClients, req.ClientId)
	} else if req.Role == ServerRole(2) { // secondary server
		sRPC.secondaryClients = append(sRPC.secondaryClients, req.ClientId)
		sRPC.primaryClients = util.RemoveElement(sRPC.primaryClients, req.ClientId)
	} else { // routing server. TODO: we should probably split the data structures that cache messages for primary/secondary vs being-routed-to clients
		sRPC.primaryClients = util.RemoveElement(sRPC.primaryClients, req.ClientId)
		sRPC.secondaryClients = util.RemoveElement(sRPC.secondaryClients, req.ClientId)
	}
	res.ServerId = sRPC.serverId
	return nil
}

// ReceiveSenderMessage
// Client calls this through MessageLib. Server will receive a MessageStruct from the client and will
// start sending it through the ring to the primary server of the receiver.

// ForwardMessage
// Server calls this, forwarding the message to the next server in the chain. Cache the message
// if it cannot be forwarded, so that it can be forwarded once the server failures are handled.

// RetrieveMessages
// Client calls this through MessageLib on its primary server. If a second client id is provided, retrieves unread messages
// between the calling client and the other client. If not, retrieves all unread messages for the client. Returns a
// slice of MessageStructs. Also, after retrieval, the cached messages are deleted from the primary and secondary servers

// HandleFailure
// Coord calls this, informing the server about what its new role and adjacent servers are. The server
// may need to forward any unacknowledged messages once the chain is reconfigured.

// Server must then retrieve cached data from one of its secondaries for the clients in ClientIds
//}

func (sRPC *ServerRPC) HandleFailure(req *HandleFailureReq, res *interface{}) error {
	// TODO: Recalibrate prev and next servers, add the clients that now have this server as its primary server.
	// TODO: think about what happens to potentially lost messages
	return nil
}

// RetrieveCachedMessages
// Server calls this when it is a new primary and needs to retrieve cached messages from one of its secondaries.

// func (sRPC *ServerRPC) RetrieveCachedMessages(req *RetrieveCachedMessagesReq, res *RetrieveCachedMessagesRes) error {
// 	// TODO: discuss as group. Do we need this? Unless there's information asymmetry between secondaries at any point, we don't need this.

// }

// SendCachedMessages
// Server calls this when it is an existing primary and needs to send cached messages to a new secondary.

func (sRPC *ServerRPC) SendCachedMessages(req *SendCachedMessagesReq, res *interface{}) error {

	cachedMessages := SendCachedMessagesReq{sRPC.cachedMessages}

	s1, err := rpc.Dial("tcp", sRPC.nextServerAddr) // secondary 1
	util.CheckErr(err, "Failed to dial s1.")
	err = s1.Call("RecvCachedMessagesFromPrimary", cachedMessages, nil)
	util.CheckErr(err, "Failed to send cached messages to s1.")
	s1.Close()

	s2, err := rpc.Dial("tcp", sRPC.prevServerAddr) // secondary 2
	util.CheckErr(err, "Failed to dial s2.")
	err = s2.Call("RecvCachedMessagesFromPrimary", cachedMessages, nil)
	util.CheckErr(err, "Failed to send cached messages to s2.")
	s2.Close()

	return nil

}

// Required RPC Calls: END

// Required Internal Methods: BEGIN
func (sRPC *ServerRPC) RecvCachedMessagesFromPrimary(req *SendCachedMessagesReq, res *interface{}) error {
	sRPC.cachedMessages = req.messages
	return nil
}

// Required Internal Methods: END
