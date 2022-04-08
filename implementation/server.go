package implementation

import (
	fchecker "ephemeralrocket/fcheck"
	"ephemeralrocket/util"
	"fmt"
	"net"
	"net/rpc"
	"time"
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
	prevServerId     uint8
	nextServerAddr   string
	nextServerId     uint8
	serverServerAddr string
	primaryClients   []string
	secondaryClients []string
	block            chan bool
	cachedMessages   map[string][]MessageStruct // maps clientId -> list of messages

	primaryClientMessages   map[string][]MessageStruct // primary clients' messages
	secondaryClientMessages map[string][]MessageStruct // secondary clients' messages
	routedNextMessages      map[string][]MessageStruct // un-acked that need to be routed to the next server
	routedPrevMessages      map[string][]MessageStruct // un-acked that need to be routed to the prev server
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
		cachedMessages:   make(map[string][]MessageStruct),
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
func (sRPC *ServerRPC) ConnectRing(req *ConnectRingReq, res *interface{}) error {
	sRPC.nextServerAddr = req.NextServerAddr
	sRPC.prevServerAddr = req.PrevServerAddr
	// not using Ids because there doesn't seem to be a use, add them to sRPC struct if necessary
	sRPC.nextServerId = req.NextServerId
	sRPC.prevServerId = req.PrevServerId

	return nil
}

// AssignRole
// Coord calls this, assigning the server a role as either a primary or secondary.
func (sRPC *ServerRPC) AssignRole(req *AssignRoleReq, res *interface{}) error {
	AssignRoleHelper(req, sRPC)
	return nil
}

func AssignRoleHelper(req *AssignRoleReq, sRPC *ServerRPC) error {
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
	return nil
}

// ReceiveSenderMessage
// Client calls this through MessageLib. Server will receive a MessageStruct from the client and will
// start sending it through the ring to the primary server of the receiver.
func (sRPC *ServerRPC) ReceiveSenderMessage(req *MessageStruct, res *MessageStruct) error {
	fmt.Printf("SERVER%d LOG: Received sender message from %s\n", sRPC.serverId, req.SourceId)
	if util.FindElement(sRPC.primaryClients, req.DestinationId) { // this server is also the primary server for the destination client
		cachedMessages := sRPC.cachedMessages[req.DestinationId]
		cachedMessages = append(cachedMessages, *req)
		sRPC.cachedMessages[req.DestinationId] = cachedMessages
		res = req
		return nil
	}
	// select random direction to send message in (prev or next)
	serverAddr, serverId := ChooseRandomDirection(sRPC)

	conn, connerr := util.GetTCPConn(serverAddr)
	if connerr != nil {
		// handle error, cache message as not forwarded
	}
	client := rpc.NewClient(conn)
	fwdReq := ForwardMessageReq{ServerId: sRPC.serverId, Message: *req}
	var fwdRes ForwardMessageRes
	err := client.Call("ServerRPC.ForwardMessage", &fwdReq, &fwdRes)
	if err != nil {
		// handle error, cache message as not forwarded
	}
	fmt.Printf("SERVER%d LOG: Message from client: %s to client %s forwarded to server with id %d\n", sRPC.serverId, req.SourceId, req.DestinationId, serverId)
	res = req
	client.Close()
	return nil
}

// ForwardMessage
// Server calls this, forwarding the message to the next server in the chain. Cache the message
// if it cannot be forwarded, so that it can be forwarded once the server failures are handled.
func (sRPC *ServerRPC) ForwardMessage(req *ForwardMessageReq, res *ForwardMessageRes) error {
	fmt.Printf("SERVER%d LOG: Received a forwarded message from client '%s' to client '%s'\n", sRPC.serverId, req.Message.SourceId, req.Message.DestinationId)
	msg := req.Message
	if util.FindElement(sRPC.primaryClients, msg.DestinationId) { // this server is the primary server for the destination client
		fmt.Printf("SERVER%d LOG: this is the primary server for client %s", sRPC.serverId, req.Message.SourceId)
		// cache messages
		currMessages := sRPC.cachedMessages[msg.DestinationId]
		currMessages = append(currMessages, msg)
		sRPC.cachedMessages[msg.DestinationId] = currMessages
		//TODO: make RPC call to secondary servers to cache message
		res.ServerId = sRPC.serverId
		res.Message = msg
		return nil
	} else { // this server is NOT the primary server for the destination client
		serverAddr, serverId := GetOtherServerAddrAndId(sRPC, req.ServerId)
		conn, connerr := util.GetTCPConn(serverAddr)
		if connerr != nil {
			// handle error, cache message as unacked
		}
		fwdReq := ForwardMessageReq{ServerId: sRPC.serverId, Message: req.Message}
		var fwdRes ForwardMessageRes
		client := rpc.NewClient(conn)
		err := client.Call("ServerRPC.ForwardMessage", &fwdReq, &fwdRes)
		if err != nil {
			// handle error, cache message as unacked
		}
		client.Close()
		fmt.Printf("SERVER%d LOG: Message from client: %s to client %s forwarded to server with id %d\n", sRPC.serverId, req.Message.SourceId, req.Message.DestinationId, serverId)
		res.ServerId = sRPC.serverId
		res.Message = msg
		return nil
	}
}

// RetrieveMessages
// Client calls this through MessageLib on its primary server. If a second client id is provided, retrieves unread messages
// between the calling client and the other client. If not, retrieves all unread messages for the client. Returns a
// slice of MessageStructs. Also, after retrieval, the cached messages are deleted from the primary and secondary servers
func (sRPC *ServerRPC) RetrieveMessages(req *RetrieveMessageReq, res *RetrieveMessageRes) error {
	if req.SourceClientId == "" { // retrieve all messages
		messages := sRPC.cachedMessages[req.ClientId]
		delete(sRPC.cachedMessages, req.ClientId)
		//TODO: make RPC call to secondary servers to clear cache!
		res.ClientId = req.ClientId
		res.Messages = messages
		return nil
	} else {
		cachedMessages := sRPC.cachedMessages[req.ClientId]
		messages := []MessageStruct{}
		leftovers := []MessageStruct{}
		for i := range cachedMessages {
			msg := cachedMessages[i]
			if msg.SourceId != req.SourceClientId {
				leftovers = append(leftovers, msg)
			} else {
				messages = append(messages, msg)
			}
		}
		sRPC.cachedMessages[req.ClientId] = leftovers
		//TODO: make RPC call to secondary servers to clear cache!
		res.ClientId = req.ClientId
		res.Messages = messages
		return nil
	}
}

// HandleFailure
// Coord calls this, informing the server about what its new role and adjacent servers are. The server
// may need to forward any unacknowledged messages once the chain is reconfigured.

// Server must then retrieve cached data from one of its secondaries for the clients in ClientIds
//}

// type HandleFailureReq struct {
// 	PrevServerId   uint8
// 	PrevServerAddr string
// 	NextServerId   uint8
// 	NextServerAddr string
// 	ClientIds      []string
// 	// Server needs to be aware of new clients that now see this server as their primary.
// 	// List of client IDs. When HandleFailure is called, the server may now become a primary
// 	// for some new clients.

// 	// Server must then retrieve cached data from one of its secondaries for the clients in ClientIds
// }

func (sRPC *ServerRPC) HandleFailure(req *HandleFailureReq, res *interface{}) error {
	// TODO: Recalibrate prev and next servers, add the clients that now have this server as its primary server.
	// TODO: think about what happens to potentially lost messages

	newNext := req.NextServerAddr
	newPrev := req.PrevServerAddr
	// s1 -> s2 -> s3

	// Handles case when server becomes primary for clients
	for _, id := range req.PrimaryClientIds {
		// assign this server a new role in the role lists
		assignReq := AssignRoleReq{id, ServerRole(1)}
		AssignRoleHelper(&assignReq, sRPC)

		// messages are guaranteed to be on this server because we know it had to have been a secondary for this client before
		// so just grab them from secondary list and put them in primary list
		sRPC.primaryClientMessages[id] = sRPC.secondaryClientMessages[id]
		delete(sRPC.secondaryClientMessages, id)
	}

	// s1
	if sRPC.nextServerAddr != newNext && newNext != "" {
		client, err := rpc.Dial("tcp", newNext)
		util.CheckErr(err, "Failed to connect to new next server.")

		// We are sent a non-empty list of new secondary clients. This means s3 is the primary and this is a new secondary
		if len(req.SecondaryClientIds) > 0 {

			req := GetCachedMessagesFromPrimaryReq{}
			var resPrimaryClientMessages GetCachedMessagesFromPrimaryRes
			err = client.Call("ServerRPC.GetCachedMessagesFromPrimary", &req, &resPrimaryClientMessages)

			// get all the primary client messages from the primary server and make this server store them in secondaryClientMessages
			for clientId := range resPrimaryClientMessages.messages {

				assignReq := AssignRoleReq{clientId, ServerRole(2)}
				AssignRoleHelper(&assignReq, sRPC)

				sRPC.secondaryClientMessages[clientId] = resPrimaryClientMessages.messages[clientId]
			}
		}

		client.Close()
	}

	// if a new prev server address is sent, update this server's prev address
	// s3
	if newPrev != sRPC.prevServerAddr && newPrev != "" {
		client, err := rpc.Dial("tcp", newPrev)
		util.CheckErr(err, "Failed to connect to new next server.")

		// We are sent a non-empty list of new secondary clients. This means s1 is the primary and this is a new secondary
		if len(req.SecondaryClientIds) > 0 {

			req := GetCachedMessagesFromPrimaryReq{}
			var resPrimaryClientMessages GetCachedMessagesFromPrimaryRes
			err = client.Call("ServerRPC.GetCachedMessagesFromPrimary", &req, &resPrimaryClientMessages)

			// get all the primary client messages from the primary server and make this server store them in secondaryClientMessages
			for clientId := range resPrimaryClientMessages.messages {

				assignReq := AssignRoleReq{clientId, ServerRole(2)}
				AssignRoleHelper(&assignReq, sRPC)

				sRPC.secondaryClientMessages[clientId] = resPrimaryClientMessages.messages[clientId]
			}

		}
		client.Close()
	}

	sRPC.nextServerAddr = newNext
	sRPC.prevServerAddr = newPrev
	sRPC.nextServerId = req.NextServerId
	sRPC.prevServerId = req.PrevServerId

	return nil
}

// SendCachedMessages
// Secondary server calls this on a primary send cached messages.
func (sRPC *ServerRPC) GetCachedMessagesFromPrimary(req *GetCachedMessagesFromPrimaryReq, res *GetCachedMessagesFromPrimaryRes) error {
	// for argument maybe we need a direction and clientIds? For every clientId, send the cached messages over to the new secondary
	res.messages = sRPC.primaryClientMessages
	return nil

	// s1, err := rpc.Dial("tcp", sRPC.nextServerAddr) // secondary 1
	// util.CheckErr(err, "Failed to dial s1.")
	// err = s1.Call("RecvCachedMessagesFromPrimary", cachedMessages, nil)
	// util.CheckErr(err, "Failed to send cached messages to s1.")
	// s1.Close()

	// s2, err := rpc.Dial("tcp", sRPC.prevServerAddr) // secondary 2
	// util.CheckErr(err, "Failed to dial s2.")
	// err = s2.Call("RecvCachedMessagesFromPrimary", cachedMessages, nil)
	// util.CheckErr(err, "Failed to send cached messages to s2.")
	// s2.Close()

	// return nil

}

// Required RPC Calls: END

// Required Internal Methods: BEGIN
func (sRPC *ServerRPC) RecvCachedMessagesFromPrimary(req *SendCachedMessagesReq, res *interface{}) error {
	sRPC.cachedMessages = req.messages
	return nil
}

// randomly chooses either prev or next server addr and id to send a message in that direction
func ChooseRandomDirection(sRPC *ServerRPC) (string, uint8) {
	now := time.Now().Nanosecond()
	if now%2 == 0 {
		return sRPC.nextServerAddr, sRPC.nextServerId
	} else {
		return sRPC.prevServerAddr, sRPC.prevServerId
	}
}

// given a server id, get the other server address and id (e.g. if the prev id is provided, return the next addr and id)
func GetOtherServerAddrAndId(sRPC *ServerRPC, serverId uint8) (string, uint8) {
	if serverId == sRPC.nextServerId {
		return sRPC.prevServerAddr, sRPC.prevServerId
	} else {
		return sRPC.nextServerAddr, sRPC.nextServerId
	}
}

// Required Internal Methods: END
