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
	// runOnce          bool

	primaryClientMessages   map[string][]MessageStruct // primary clients' messages
	secondaryClientMessages map[string][]MessageStruct // secondary clients' messages
	routedNextMessages      []MessageStruct            // un-acked that need to be routed to the next server
	routedPrevMessages      []MessageStruct            // un-acked that need to be routed to the prev server
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
		serverId:                config.ServerId,
		serverAddr:              config.ServerAddr,
		coordAddr:               config.CoordAddr,
		serverListenAddr:        config.ServerListenAddr, // address for coord to call rpc methods on this server
		clientListenAddr:        config.ClientListenAddr, // address for client to call rpc methods on this server
		prevServerAddr:          "",
		nextServerAddr:          "",
		serverServerAddr:        config.ServerServerAddr, // address for server to call rpc methods on this server
		block:                   make(chan bool),
		primaryClientMessages:   make(map[string][]MessageStruct),
		secondaryClientMessages: make(map[string][]MessageStruct),
		routedNextMessages:      make([]MessageStruct, 0),
		routedPrevMessages:      make([]MessageStruct, 0),
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
	conn.SetLinger(0)
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
	coordRpc.Close()

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
		cachedMessages := sRPC.primaryClientMessages[req.DestinationId]
		cachedMessages = append(cachedMessages, *req)
		sRPC.primaryClientMessages[req.DestinationId] = cachedMessages
		msgMap := make(map[string][]MessageStruct)
		msgMap[req.DestinationId] = append([]MessageStruct{}, *req)
		CacheMessagesInSecondaries(sRPC, msgMap)
		res = req
		return nil
	}
	// select random direction to send message in (prev or next)
	serverAddr, serverId := ChooseRandomDirection(sRPC)

	conn, connerr := util.GetTCPConn(serverAddr)
	if connerr != nil {
		// handle error, cache message as not forwarded
		CacheUnackedMessage(sRPC, *req, serverId)
		return nil
	}
	client := rpc.NewClient(conn)
	fwdReq := ForwardMessageReq{ServerId: sRPC.serverId, Message: *req}
	var fwdRes ForwardMessageRes
	fmt.Printf("SERVER%d LOG: Forwarding Message from client: %s to client: %s to server with id %d\n", sRPC.serverId, req.SourceId, req.DestinationId, serverId)
	err := client.Call("ServerRPC.ForwardMessage", &fwdReq, &fwdRes)
	if err != nil {
		// handle error, cache message as not forwarded
		fmt.Printf("SERVER%d LOG: Error Forwarding Message from client: %s to client: %s to server with id %d, caching message\n", sRPC.serverId, req.SourceId, req.DestinationId, serverId)
		CacheUnackedMessage(sRPC, *req, serverId)
		client.Close()
		return nil
	}
	res = req
	client.Close()
	return nil
}

// ForwardMessage
// Server calls this, forwarding the message to the next server in the chain. Cache the message
// if it cannot be forwarded, so that it can be forwarded once the server failures are handled.
func (sRPC *ServerRPC) ForwardMessage(req *ForwardMessageReq, res *ForwardMessageRes) error {
	fmt.Printf("SERVER%d LOG: Received a forwarded message from server%d, from client '%s' to client '%s'\n", sRPC.serverId, req.ServerId, req.Message.SourceId, req.Message.DestinationId)
	msg := req.Message
	if util.FindElement(sRPC.primaryClients, msg.DestinationId) { // this server is the primary server for the destination client
		fmt.Printf("SERVER%d LOG: This is the primary server for client %s\n", sRPC.serverId, req.Message.DestinationId)
		// cache messages
		currMessages := sRPC.primaryClientMessages[msg.DestinationId]
		currMessages = append(currMessages, msg)
		sRPC.primaryClientMessages[msg.DestinationId] = currMessages
		msgMap := make(map[string][]MessageStruct)
		msgMap[msg.DestinationId] = append([]MessageStruct{}, msg)
		CacheMessagesInSecondaries(sRPC, msgMap)
		res.ServerId = sRPC.serverId
		res.Message = msg
		return nil
	} else { // this server is NOT the primary server for the destination client
		serverAddr, serverId := GetOtherServerAddrAndId(sRPC, req.ServerId)
		conn, connerr := util.GetTCPConn(serverAddr)
		if connerr != nil {
			CacheUnackedMessage(sRPC, req.Message, serverId)
			return nil
		}
		fwdReq := ForwardMessageReq{ServerId: sRPC.serverId, Message: req.Message}
		var fwdRes ForwardMessageRes
		client := rpc.NewClient(conn)
		fmt.Printf("SERVER%d LOG: Forwarding Message from client: %s to client: %s to server with id %d\n", sRPC.serverId, req.Message.SourceId, req.Message.DestinationId, serverId)
		err := client.Call("ServerRPC.ForwardMessage", &fwdReq, &fwdRes)
		if err != nil {
			fmt.Printf("SERVER%d LOG: Error Forwarding Message from client: %s to client: %s to server with id %d, caching message\n", sRPC.serverId, req.Message.SourceId, req.Message.DestinationId, serverId)
			CacheUnackedMessage(sRPC, req.Message, serverId)
			client.Close()
			return nil
		}
		client.Close()
		res.ServerId = sRPC.serverId
		res.Message = msg
		return nil
	}
}

// RetrieveMessages
// Client calls this through MessageLib on its primary server. Retrieves all unread messages for the client. Returns a
// slice of MessageStructs. Also, after retrieval, the cached messages are deleted from the primary and secondary servers
func (sRPC *ServerRPC) RetrieveMessages(req *RetrieveMessageReq, res *RetrieveMessageRes) error {
	if messages, ok := sRPC.primaryClientMessages[req.ClientId]; ok {
		delete(sRPC.primaryClientMessages, req.ClientId)
		//if messages were deleted, clear cache in secondary servers
		if len(messages) != 0 {
			fmt.Printf("SERVER%d LOG: Client with id '%s' retrieved %d messages from primary\n", sRPC.serverId, req.ClientId, len(messages))
			ClearCacheMessagesInSecondaries(sRPC, req.ClientId)
		}
		res.ClientId = req.ClientId
		res.Messages = messages

	} else {
		res.ClientId = req.ClientId
		res.Messages = make([]MessageStruct, 0)
	}
	return nil
}

// HandleFailure
// Coord calls this, informing the server about what its new role and adjacent servers are. The server
// may need to forward any unacknowledged messages once the chain is reconfigured.
// Server must then retrieve cached data from one of its secondaries for the clients in ClientIds
func (sRPC *ServerRPC) HandleFailure(req *HandleFailureReq, res *interface{}) error {
	fmt.Printf("SERVER%d LOG: Handling Server Failure\n", sRPC.serverId)

	// PrevServerId       uint8
	// PrevServerAddr     string
	// NextServerId       uint8
	// NextServerAddr     string
	// PrimaryClientIds   []string
	// SecondaryClientIds []string

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

			fmt.Printf("SERVER%d LOG: New Secondary For Clients: %v \n", sRPC.serverId, req.SecondaryClientIds)

			req := GetCachedMessagesFromPrimaryReq{}
			var resPrimaryClientMessages GetCachedMessagesFromPrimaryRes
			err = client.Call("ServerRPC.GetCachedMessagesFromPrimary", &req, &resPrimaryClientMessages)

			if err != nil {
				fmt.Printf("Error encountered getting cached messages from primary %e\n", err)
			}

			// get all the primary client messages from the primary server and make this server store them in secondaryClientMessages
			for clientId := range resPrimaryClientMessages.Messages {

				assignReq := AssignRoleReq{clientId, ServerRole(2)}
				AssignRoleHelper(&assignReq, sRPC)

				sRPC.secondaryClientMessages[clientId] = resPrimaryClientMessages.Messages[clientId]
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

			fmt.Printf("SERVER%d LOG: New Secondary For Clients: %v \n", sRPC.serverId, req.SecondaryClientIds)

			req := GetCachedMessagesFromPrimaryReq{}
			var resPrimaryClientMessages GetCachedMessagesFromPrimaryRes
			err = client.Call("ServerRPC.GetCachedMessagesFromPrimary", &req, &resPrimaryClientMessages)

			if err != nil {
				fmt.Printf("Error encountered getting cached messages from primary %e\n", err)
			}

			// get all the primary client messages from the primary server and make this server store them in secondaryClientMessages
			for clientId := range resPrimaryClientMessages.Messages {

				assignReq := AssignRoleReq{clientId, ServerRole(2)}
				AssignRoleHelper(&assignReq, sRPC)

				sRPC.secondaryClientMessages[clientId] = resPrimaryClientMessages.Messages[clientId]
			}

		}
		client.Close()
	}

	sRPC.nextServerAddr = newNext
	sRPC.prevServerAddr = newPrev
	sRPC.nextServerId = req.NextServerId
	sRPC.prevServerId = req.PrevServerId

	// resend any unacked messages
	go ResendUnackedMessages(sRPC, "prev")
	go ResendUnackedMessages(sRPC, "next")

	return nil
}

// SendCachedMessages
// Secondary server calls this on a primary server to get its primaryClients cached messages.
func (sRPC *ServerRPC) GetCachedMessagesFromPrimary(req *GetCachedMessagesFromPrimaryReq, res *GetCachedMessagesFromPrimaryRes) error {
	fmt.Printf("SERVER%d LOG: Primary Received Cached Messages Fetch Request From Secondary Server\n", sRPC.serverId)
	res.Messages = sRPC.primaryClientMessages
	return nil
}

func (sRPC *ServerRPC) RecvCachedMessagesFromPrimary(req *SendCachedMessagesReq, res *interface{}) error {
	fmt.Printf("SERVER%d LOG: Secondary server received cache request from primary server\n", sRPC.serverId)
	msgMap := req.Messages
	for key, element := range msgMap {
		if val, ok := sRPC.secondaryClientMessages[key]; ok {
			newMessages := append(val, element...)
			sRPC.secondaryClientMessages[key] = newMessages
		} else {
			sRPC.secondaryClientMessages[key] = element
		}
	}
	return nil
}

func (sRPC *ServerRPC) ClearCache(req *ClearCacheReq, res *interface{}) error {
	fmt.Printf("SERVER%d LOG: Secondary server received cache deletion request from primary server for client with id '%s'\n", sRPC.serverId, req.ClientId)
	delete(sRPC.secondaryClientMessages, req.ClientId)
	return nil
}

// Required RPC Calls: END

// Required Internal Methods: BEGIN
func ClearCacheMessagesInSecondaries(sRPC *ServerRPC, clientId string) {
	connPrev, connPrevErr := util.GetTCPConn(sRPC.prevServerAddr)
	if connPrevErr != nil {
		fmt.Printf("Error encountered connecting to prev server %e\n", connPrevErr)
	}
	prevClient := rpc.NewClient(connPrev)
	req := ClearCacheReq{ClientId: clientId}
	var res interface{}
	err := prevClient.Call("ServerRPC.ClearCache", &req, &res)
	if err != nil {
		fmt.Printf("Error encountered making rpc call %e\n", err)
	}
	prevClient.Close()
	connNext, connNextErr := util.GetTCPConn(sRPC.nextServerAddr)
	if connNextErr != nil {
		fmt.Printf("Error encountered connecting to next server %e\n", connPrevErr)
	}
	nextClient := rpc.NewClient(connNext)
	err = nextClient.Call("ServerRPC.ClearCache", &req, &res)
	if err != nil {
		fmt.Printf("Error encountered making rpc call %e\n", err)
	}
	nextClient.Close()

}

func CacheMessagesInSecondaries(sRPC *ServerRPC, messages map[string][]MessageStruct) {
	connPrev, connPrevErr := util.GetTCPConn(sRPC.prevServerAddr)
	if connPrevErr != nil {
		fmt.Printf("Error encountered connecting to prev server %e\n", connPrevErr)
	}
	prevClient := rpc.NewClient(connPrev)
	req := SendCachedMessagesReq{Messages: messages}
	var res interface{}
	err := prevClient.Call("ServerRPC.RecvCachedMessagesFromPrimary", &req, &res)
	if err != nil {
		fmt.Printf("Error encountered making rpc call %e\n", err)
	}
	prevClient.Close()
	connNext, connNextErr := util.GetTCPConn(sRPC.nextServerAddr)
	if connNextErr != nil {
		fmt.Printf("Error encountered connecting to next server %e\n", connPrevErr)
	}
	nextClient := rpc.NewClient(connNext)
	err = nextClient.Call("ServerRPC.RecvCachedMessagesFromPrimary", &req, &res)
	if err != nil {
		fmt.Printf("Error encountered making rpc call %e\n", err)
	}
	nextClient.Close()

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

func CacheUnackedMessage(sRPC *ServerRPC, msg MessageStruct, serverId uint8) {
	// handle error, cache message as not forwarded
	if serverId == sRPC.nextServerId {
		sRPC.routedNextMessages = append(sRPC.routedNextMessages, msg)
	} else {
		sRPC.routedPrevMessages = append(sRPC.routedPrevMessages, msg)
	}
}

func ResendUnackedMessages(sRPC *ServerRPC, prevOrNext string) {
	time.Sleep(time.Millisecond * 10)
	var success bool = true

	var addr string
	var cachedMessages []MessageStruct
	if prevOrNext == "prev" {
		addr = sRPC.prevServerAddr
		cachedMessages = sRPC.routedPrevMessages
	} else {
		addr = sRPC.nextServerAddr
		cachedMessages = sRPC.routedNextMessages
	}

	if len(cachedMessages) > 0 {
		client, cerr := rpc.Dial("tcp", addr)
		if cerr != nil {
			return
		}
		fmt.Printf("SERVER%d LOG: Resending %d unacked messages to %s server with id %d \n", sRPC.serverId, len(cachedMessages), prevOrNext, sRPC.nextServerId)
		for _, msg := range cachedMessages {

			fwdReq := ForwardMessageReq{ServerId: sRPC.serverId, Message: msg}
			var fwdRes ForwardMessageRes
			err := client.Call("ServerRPC.ForwardMessage", &fwdReq, &fwdRes)
			if err != nil {
				success = false
				break
			}
		}
		client.Close()
		if success { // cleanup unacked cache if successful
			if prevOrNext == "prev" {
				sRPC.routedPrevMessages = make([]MessageStruct, 0)
			} else {
				sRPC.routedNextMessages = make([]MessageStruct, 0)
			}
		}
	}
}

// Required Internal Methods: END
