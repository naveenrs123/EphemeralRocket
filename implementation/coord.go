package implementation

import (
	fchecker "ephemeralrocket/fcheck"
	"ephemeralrocket/util"
	"fmt"
	"math/rand"
	"net"
	"net/rpc"
	"time"
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
	ServerRing       []uint8                  // order of servers
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
	fmt.Println("COORD LOG: Started")

	c.cRPC = CoordRPC{
		NumServers:       config.NumServers,
		LostMsgsThresh:   config.LostMsgsThresh,
		FCheckIP:         config.ServerListenAddr,
		IsRingReady:      false,
		ServerRing:       make([]uint8, 0),
		ServerDetailsMap: make(map[uint8]*ServerDetails),
		PrimaryClientMap: make(map[string]uint8),
		ClientList:       make([]string, 0),
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

	// Listen for clients and servers.
	go rpc.Accept(clientListener)
	go rpc.Accept(serverListener)

	// Block forever using empty channel
	block := make(chan int)
	<-block

	return nil
}

// Server calls this, requesting to join the system. Coord caches this request.
// Server must provide an address that the coord can use to contact them, as well as an address to use for fcheck.
// ! This function is mostly identical to one used in A3.
func (c *CoordRPC) ServerJoin(req *ServerRequestToJoinReq, res *interface{}) error {
	fmt.Printf("COORD LOG: Server %d Join Request\n", req.ServerId)

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

	fmt.Printf("COORD LOG: Server %d Acknowledged\n", req.ServerId)
	if len(c.ServerDetailsMap) == int(c.NumServers) {
		// Create the ring when all servers have joined.
		go CreateServerRing(c)
	}
	return nil
}

// Client calls this through MessageLib, requesting the details of its primary server. Coord
// returns the primary server for that client, and adds the client
func (c *CoordRPC) RetrievePrimaryServer(req *PrimaryServerReq, res *PrimaryServerRes) error {
	res.ClientId = req.ClientId

	// Block clients from joining while server chain is being configured
	if !c.IsRingReady {
		res.ChainReady = false
		return nil
	}

	// Add client to system
	if !util.FindElement(c.ClientList, req.ClientId) {
		c.ClientList = append(c.ClientList, req.ClientId)
	}

	if v, ok := c.PrimaryClientMap[req.ClientId]; ok {
		fmt.Printf("COORD LOG: Existing Client: %s\n", req.ClientId)
		// If a primary has been assigned, retrieve it.
		res.PrimaryServerIPPort = c.ServerDetailsMap[v].ClientAddr
	} else {
		fmt.Printf("COORD LOG: New Client: %s\n", req.ClientId)
		// Otherwise, assign a new primary and provide it to the client.
		primaryServerId := AssignPrimaryServer(c)
		c.PrimaryClientMap[req.ClientId] = primaryServerId
		res.PrimaryServerIPPort = c.ServerDetailsMap[primaryServerId].ClientAddr

		// Retrieve server info of primary and secondary servers
		primaryServer := c.ServerDetailsMap[c.PrimaryClientMap[res.ClientId]]
		prevServer := c.ServerDetailsMap[primaryServer.PrevId]
		nextServer := c.ServerDetailsMap[primaryServer.NextId]

		// TODO: Deal with failures later
		primaryClient, _ := rpc.Dial("tcp", primaryServer.CoordAddr)
		prevClient, _ := rpc.Dial("tcp", prevServer.CoordAddr)
		nextClient, _ := rpc.Dial("tcp", nextServer.CoordAddr)

		var emptyRes interface{}
		primReq := AssignRoleReq{ClientId: res.ClientId, Role: ServerRole(1)}
		primaryClient.Call("ServerRPC.AssignRole", &primReq, &emptyRes)

		secReq := AssignRoleReq{ClientId: res.ClientId, Role: ServerRole(2)}
		prevClient.Call("ServerRPC.AssignRole", &secReq, &emptyRes)
		nextClient.Call("ServerRPC.AssignRole", &secReq, &emptyRes)

		// Update list of primary and secondary clients for each server
		primaryServer.PrimaryClients = append(primaryServer.PrimaryClients, req.ClientId)
		prevServer.SecondaryClients = append(prevServer.SecondaryClients, req.ClientId)
		nextServer.SecondaryClients = append(nextServer.SecondaryClients, req.ClientId)

		primaryClient.Close()
		prevClient.Close()
		nextClient.Close()
	}

	printClient(c, req.ClientId)

	res.ChainReady = true
	return nil
}

// Client calls this through MessageLib, requesting all known clients in the system. Coord returns
// the list of client IDs representing the clients that are known to the system.
func (c *CoordRPC) RetrieveClients(req *interface{}, res *RetrieveClientsRes) error {
	fmt.Printf("COORD LOG: Retrieve Clients\n")
	res.ClientIds = c.ClientList
	return nil
}

// Called once all servers have joined the system. Coord will have to make an RPC call on the server.
func CreateServerRing(c *CoordRPC) {
	fmt.Println("COORD LOG: Create Server Ring")
	length := len(c.ServerDetailsMap)
	serverOrder := make([]uint8, length)
	serverPrevOrder := make([]uint8, length)
	serverNextOrder := make([]uint8, length)

	counter := 1
	for k := range c.ServerDetailsMap {
		// use modulo to assign each server to the right position in the prev and next order.
		serverPrevOrder[(counter+1)%length] = k
		serverOrder[counter%length] = k
		serverNextOrder[(counter-1)%length] = k
		counter++
	}

	for i, idx := range serverOrder {
		// assign each server its prev and next server.
		v := c.ServerDetailsMap[idx]
		pos := i % length
		v.PrevId = serverPrevOrder[pos]
		v.NextId = serverNextOrder[pos]
	}

	// Make RPC call to each server with its prev and next server info
	serverCalls := make([]*rpc.Call, length)
	doneChan := make(chan *rpc.Call, length)
	for i, idx := range serverOrder {
		v := c.ServerDetailsMap[idx]
		client, err := rpc.Dial("tcp", v.CoordAddr)
		util.CheckErr(err, "could not connect to server with ID: %d", v.ServerId) // This error should never happen.
		req := ConnectRingReq{
			PrevServerId:   v.PrevId,
			PrevServerAddr: c.ServerDetailsMap[v.PrevId].ServerAddr,
			NextServerId:   v.NextId,
			NextServerAddr: c.ServerDetailsMap[v.NextId].ServerAddr,
		}
		var res interface{}
		serverCalls[i] = client.Go("ServerRPC.ConnectRing", &req, &res, doneChan)
		fmt.Printf("COORD LOG: Server %d Joining\n", v.ServerId)
	}

	for i := range serverOrder {
		// This error should never happen.
		util.CheckErr(serverCalls[i].Error, "Error during call to ServerRPC.ConnectRing for server")
		<-doneChan
	}

	c.ServerRing = serverOrder
	fmt.Printf("COORD LOG: New Ring - %v\n", c.ServerRing)
	c.IsRingReady = true
	go MonitorServerFailures(c)
	fmt.Println("COORD LOG: All Servers Joined")
}

// Called when a client is joining. Assigns a primary server based on a simple algorithm, used to calculate load on the server.
// The load on a server is given by 2 * number of primary clients + number of secondary clients.
func AssignPrimaryServer(c *CoordRPC) uint8 {
	primaryServerId := uint8(0)
	minLoad := int(10000000000000)

	for _, v := range c.ServerDetailsMap {
		serverLoad := calculateLoad(c, v.ServerId)
		if serverLoad < minLoad {
			minLoad = serverLoad
			primaryServerId = v.ServerId
		}
	}
	return primaryServerId
}

// Uses fcheck under the hood. All servers in the system will be monitored using the addresses they provided.
// This triggers the failure protocol, which involves RPC calls on the servers adjacent to the failed server.
func MonitorServerFailures(c *CoordRPC) {
	laddrList := make([]string, c.NumServers)
	raddrList := make([]string, c.NumServers)
	serverIds := make([]uint8, c.NumServers)

	// Extract local addresses for fcheck
	coordIP := util.ExtractIP(c.FCheckIP)
	for i := range laddrList {
		var addr string
		for {
			addr = coordIP + ":" + util.GetRandomPort(c.FCheckIP)
			if idx := util.FindIndex(laddrList, addr); idx == -1 {
				break
			}
		}
		laddrList[i] = addr
	}

	// extract remote addresses for fcheck
	i := 0
	for _, v := range c.ServerDetailsMap {
		serverIds[i] = v.ServerId
		raddrList[i] = v.FCheckAddr
		i++
	}

	// start fcheck
	notifyCh, err := fchecker.Start(fchecker.StartStruct{
		AckLocalIPAckLocalPort: "",
		EpochNonce:             rand.Uint64(),
		HBeatLocalIPPortList:   laddrList,
		HBeatRemoteIPPortList:  raddrList,
		LostMsgThresh:          c.LostMsgsThresh,
		ServerIds:              serverIds,
	})
	util.CheckErr(err, "fcheck error: %v", err)

	for {
		select {
		case f := <-notifyCh:
			c.IsRingReady = false
			fmt.Printf("COORD LOG: Server%d Failed @ %v\n", f.ServerID, f.Timestamp)

			failing := c.ServerDetailsMap[f.ServerID]
			prev, next := c.ServerDetailsMap[failing.PrevId], c.ServerDetailsMap[failing.NextId]
			prevPrim, nextPrim := partitionPrimaryClients(c, failing.PrimaryClients, failing.PrevId, failing.NextId)
			prevSec, nextSec := partitionSecondaryClients(c, failing.SecondaryClients, failing.PrevId)

			isPrimaryFailure := len(failing.PrimaryClients) > 0
			isPrevChosen := len(prevPrim) > 0 // no impact for a secondary or routing failure.

			if isPrimaryFailure {
				fmt.Printf("COORD LOG: Failure Scenario - Primary\n")
				// Reconfigure primary client map based on partitioned clients.
				for i, v := range c.PrimaryClientMap {
					if v == failing.ServerId {
						if isPrevChosen {
							c.PrimaryClientMap[i] = prev.ServerId
						} else {
							c.PrimaryClientMap[i] = next.ServerId
						}
					}
				}
			} else {
				fmt.Printf("COORD LOG: Failure Scenario - Secondary Or Routing\n")
			}

			var res interface{}

			prevReq := HandleFailureReq{
				PrevServerId:       prev.PrevId,
				PrevServerAddr:     c.ServerDetailsMap[prev.PrevId].ServerAddr,
				NextServerId:       next.ServerId,
				NextServerAddr:     next.ServerAddr,
				PrimaryClientIds:   prevPrim,
				SecondaryClientIds: prevSec,
			}

			nextReq := HandleFailureReq{
				PrevServerId:       prev.ServerId,
				PrevServerAddr:     prev.ServerAddr,
				NextServerId:       next.NextId,
				NextServerAddr:     c.ServerDetailsMap[next.NextId].ServerAddr,
				PrimaryClientIds:   nextPrim,
				SecondaryClientIds: nextSec,
			}

			prevClient, prevErr := rpc.Dial("tcp", prev.CoordAddr)
			nextClient, nextErr := rpc.Dial("tcp", next.CoordAddr)

			if isPrevChosen {
				prev.PrimaryClients = append(prev.PrimaryClients, prevPrim...)
				prevErr = prevClient.Call("ServerRPC.HandleFailure", prevReq, &res)
				nextErr = nextClient.Call("ServerRPC.HandleFailure", nextReq, &res)
			} else {
				next.PrimaryClients = append(prev.PrimaryClients, nextPrim...)
				nextErr = nextClient.Call("ServerRPC.HandleFailure", nextReq, &res)
				prevErr = prevClient.Call("ServerRPC.HandleFailure", prevReq, &res)
			}

			// These errors shouldn't happen
			util.CheckErr(prevErr, "Error handling failure for prev server, Server ID: %d", prev.ServerId)
			util.CheckErr(nextErr, "Error handling failure for next server, Server ID: %d", next.ServerId)

			// Close client connections
			prevClient.Close()
			nextClient.Close()

			if isPrimaryFailure {
				var newSec *ServerDetails
				var primClients []string

				if isPrevChosen {
					primClients = prevPrim
					newSec = c.ServerDetailsMap[prev.PrevId]
				} else {
					primClients = nextPrim
					newSec = c.ServerDetailsMap[next.NextId]
				}

				newSecReq := HandleFailureReq{
					PrevServerId:       newSec.PrevId,
					PrevServerAddr:     c.ServerDetailsMap[newSec.PrevId].ServerAddr,
					NextServerId:       newSec.NextId,
					NextServerAddr:     c.ServerDetailsMap[newSec.NextId].ServerAddr,
					SecondaryClientIds: primClients,
				}

				newSecClient, _ := rpc.Dial("tcp", newSec.CoordAddr)
				err = newSecClient.Call("ServerRPC.HandleFailure", newSecReq, &res)
				util.CheckErr(err, "Error handling failure for new secondary server, Server ID: %d", newSec.ServerId)
				newSecClient.Close()
			}

			// Reconfigure ring and remove failing server from the map.
			prev.NextId, next.PrevId = next.ServerId, prev.ServerId
			c.ServerRing = util.RemoveUint8(c.ServerRing, failing.ServerId)
			fmt.Printf("COORD LOG: New Ring - %v\n", c.ServerRing)
			printClientMap(c)

			delete(c.ServerDetailsMap, failing.ServerId)
			c.IsRingReady = true
			fmt.Printf("COORD LOG: Server%d Failure Handled\n", f.ServerID)
		default:
			time.Sleep(50 * time.Millisecond)
		}
	}
}

func partitionPrimaryClients(c *CoordRPC, clients []string, prevId uint8, nextId uint8) ([]string, []string) {
	var prev []string
	var next []string

	// Determine a new primary based on load.
	prevLoad := calculateLoad(c, prevId)
	nextLoad := calculateLoad(c, nextId)

	if prevLoad < nextLoad {
		prev = make([]string, len(clients))
		next = make([]string, 0)
		copy(prev, clients)
		if len(clients) > 0 {
			fmt.Printf("COORD LOG: New Primary - Server%d, Clients: %v\n", prevId, prev)
		}

	} else {
		next = make([]string, len(clients))
		prev = make([]string, 0)
		copy(next, clients)
		if len(clients) > 0 {
			fmt.Printf("COORD LOG: New Primary - Server%d, Clients: %v\n", nextId, next)
		}
	}
	return prev, next
}

func partitionSecondaryClients(c *CoordRPC, clients []string, prevServerId uint8) ([]string, []string) {
	prev := make([]string, 0)
	next := make([]string, 0)

	prevServer := c.ServerDetailsMap[prevServerId]

	for _, v := range clients {
		if util.FindElement(prevServer.PrimaryClients, v) {
			// If the failing server was a secondary for its prev, next becomes new secondary.
			next = append(next, v)
		} else {
			// If the failing server was a secondary for its next, prev becomes new secondary.
			prev = append(prev, v)
		}
	}
	return prev, next
}

func calculateLoad(c *CoordRPC, serverId uint8) int {
	server := c.ServerDetailsMap[serverId]
	return 2*len(server.PrimaryClients) + len(server.SecondaryClients)
}

func printClientMap(c *CoordRPC) {
	for client := range c.PrimaryClientMap {
		printClient(c, client)
	}
}

func printClient(c *CoordRPC, client string) {
	serverId := c.PrimaryClientMap[client]
	primServer := c.ServerDetailsMap[serverId]
	fmt.Printf("COORD LOG: %s -> Prev: %d, Primary: %d, Next: %d\n", client, primServer.PrevId, serverId, primServer.NextId)
}
