package fchecker

import (
	"bytes"
	"encoding/gob"
	"errors"
	"fmt"
	"net"
	"time"
)

// Heartbeat message.
type HBeatMessage struct {
	EpochNonce uint64 // Identifies this fchecker instance/epoch.
	SeqNum     uint64 // Unique for each heartbeat in an epoch.
}

// An ack message; response to a heartbeat.
type AckMessage struct {
	HBEatEpochNonce uint64 // Copy of what was received in the heartbeat.
	HBEatSeqNum     uint64 // Copy of what was received in the heartbeat.
}

// Notification of a failure, signal back to the client using this
// library.
type FailureDetected struct {
	UDPIpPort string    // The RemoteIP:RemotePort of the failed node.
	ServerID  uint8     // The ID of the server that failed
	Timestamp time.Time // The time when the failure was detected.
}

type StartStruct struct {
	AckLocalIPAckLocalPort string
	EpochNonce             uint64
	HBeatLocalIPPortList   []string // list of local ip:ports that will be monitoring servers and responding to hbeats.
	HBeatRemoteIPPortList  []string // list of remote ip:ports for the monitored servers, sorted by server id.
	ServerIds              []uint8
	LostMsgThresh          uint8
}

var (
	monitorCount int
	stopRespond  chan bool
	stopMonitor  chan bool
	running      bool = false
)

// Starts the fcheck library.
func Start(arg StartStruct) (notifyCh <-chan FailureDetected, err error) {
	running = false

	if arg.HBeatLocalIPPortList == nil || len(arg.HBeatLocalIPPortList) == 0 {
		// Start fcheck without monitoring any node, but responding to heartbeats
		respondConn, _, err := setup(&arg)
		if err != nil {
			return nil, err
		}

		stopRespond = make(chan bool)
		go respond(respondConn, stopRespond)
		running = true
		return nil, nil

	} else {
		if running {
			return nil, errors.New("fcheck library is already running")
		}
		// Start the fcheck library by monitoring a list of nodes.
		_, monitorConnList, err := setup(&arg)
		if err != nil {
			return nil, err
		}

		monitorCount = len(arg.HBeatRemoteIPPortList)
		notify := make(chan FailureDetected)
		stopMonitor = make(chan bool, monitorCount)
		fmt.Println("FCHECK: MONITORING SERVERS")
		for idx, monitorConn := range monitorConnList {
			go monitor(monitorConn, notify, stopMonitor, &arg, idx)
		}
		running = true
		return notify, nil
	}
}

// Tells the library to stop monitoring/responding acks.
func Stop() {
	running = false

	if stopRespond != nil {
		stopRespond <- true
	}

	if stopMonitor != nil {
		for i := 0; i < monitorCount; i++ {
			stopMonitor <- true
		}
	}
}

// Setup the initial connections required to respond to HBeatMessages from other nodes
// and send messages to nodes for monitoring purposes.
func setup(arg *StartStruct) (*net.UDPConn, []*net.UDPConn, error) {
	if arg.HBeatLocalIPPortList != nil && len(arg.HBeatLocalIPPortList) > 0 {
		// list of hbeat ports provided, set up monitoring connections.
		monitorConnList := make([]*net.UDPConn, len(arg.HBeatLocalIPPortList))
		for idx := range arg.HBeatLocalIPPortList {
			localHBeatPort, err := net.ResolveUDPAddr("udp", arg.HBeatLocalIPPortList[idx])
			if err != nil {
				msg := fmt.Sprintf("error: could not resolve HBeatLocalIPPortList[%d]: %s", idx, arg.HBeatLocalIPPortList[idx])
				return nil, nil, errors.New(msg)
			}

			remoteHBeatPort, err := net.ResolveUDPAddr("udp", arg.HBeatRemoteIPPortList[idx])
			if err != nil {
				msg := fmt.Sprintf("error: could not resolve HBeatRemoteIPPortList[%d]: %s", idx, arg.HBeatRemoteIPPortList[idx])
				return nil, nil, errors.New(msg)
			}

			monitorConn, err := net.DialUDP("udp", localHBeatPort, remoteHBeatPort)
			if err != nil {
				msg := fmt.Sprintf("error: could not dial HBeatRemoteIPPortList[%d]: %s", idx, arg.HBeatRemoteIPPortList[idx])
				return nil, nil, errors.New(msg)
			}
			monitorConnList[idx] = monitorConn
		}
		return nil, monitorConnList, nil
	} else {
		// no hbeat ports available, set up connection to respond to hbeats.
		localAckPort, err := net.ResolveUDPAddr("udp", arg.AckLocalIPAckLocalPort)
		if err != nil {
			return nil, nil, errors.New("error: could not resolve AckLocalIPAckLocalPort IP/Port")
		}

		respondConn, err := net.ListenUDP("udp", localAckPort)
		if err != nil {
			return nil, nil, errors.New("error: could not listen in ACKLocalIPACKLocalPort")
		}
		return respondConn, nil, nil
	}
}

// Listen for HBeatMessage and respond with AckMessage
func respond(conn *net.UDPConn, stopCh <-chan bool) {
	for {
		select {
		case <-stopCh:
			conn.Close()
			return
		default:
			res := make([]byte, 1024)
			conn.SetReadDeadline(time.Now().Add(time.Duration(1) * time.Second))
			len, sender, err := conn.ReadFromUDP(res)
			if err != nil {
				break // reset on error and wait for another HBeatMessage
			}
			hbeat, _ := decodeHBeat(res, len)
			time.Sleep(100 * time.Millisecond)
			msg := encodeAck(&AckMessage{hbeat.EpochNonce, hbeat.SeqNum})
			conn.WriteTo(msg, sender)
		}
	}
}

// Send HBeatMessage and listen for AckMessage
func monitor(conn *net.UDPConn, notifyCh chan<- FailureDetected, stopCh <-chan bool, arg *StartStruct, addrIdx int) {
	seqNum, RTT, lost := uint64(0), int64(3000000), uint8(0)
	rttMap := make(map[uint64]time.Time)

	for {
		select {
		case <-stopCh:
			conn.Close()
			return
		default:
			msg := encodeHBeat(&HBeatMessage{EpochNonce: arg.EpochNonce, SeqNum: seqNum})
			rttMap[seqNum] = time.Now()
			seqNum++
			conn.Write(msg)
			// assume it went through, then retry after timeout if it did not
			res := make([]byte, 1024)
			conn.SetReadDeadline(time.Now().Add(time.Duration(RTT) * time.Microsecond))
			len, err := conn.Read(res)

			if err != nil {
				lost++
				if lost > arg.LostMsgThresh {
					fmt.Println(FailureDetected{
						UDPIpPort: arg.HBeatRemoteIPPortList[addrIdx],
						ServerID:  arg.ServerIds[addrIdx],
						Timestamp: time.Now(),
					})
					notifyCh <- FailureDetected{
						UDPIpPort: arg.HBeatRemoteIPPortList[addrIdx],
						ServerID:  arg.ServerIds[addrIdx],
						Timestamp: time.Now(),
					}
					monitorCount--
					return
				}
				break // send a HBeatMessage again
			}
			recvTime := time.Now()
			lost = 0
			ack, _ := decodeAck(res, len)

			if ack.HBEatEpochNonce != arg.EpochNonce {
				break // ignore AckMessage that doesn't match the current EpochNonce
			}

			diff := recvTime.Sub(rttMap[ack.HBEatSeqNum])
			RTT = calculateNewRTT(RTT, diff.Microseconds())
			delete(rttMap, ack.HBEatSeqNum)
		}
	}

}

// Calculate the new RTT based on the old average and the measured RTT.
func calculateNewRTT(oldRTT, measuredRTT int64) int64 {
	return int64((float64(oldRTT) + float64(measuredRTT)) / 2)
}

func encodeAck(ack *AckMessage) []byte {
	var buf bytes.Buffer
	gob.NewEncoder(&buf).Encode(ack)
	return buf.Bytes()
}

func encodeHBeat(HBeat *HBeatMessage) []byte {
	var buf bytes.Buffer
	gob.NewEncoder(&buf).Encode(HBeat)
	return buf.Bytes()
}

func decodeHBeat(buf []byte, len int) (HBeatMessage, error) {
	var decoded HBeatMessage
	err := gob.NewDecoder(bytes.NewBuffer(buf[0:len])).Decode(&decoded)
	if err != nil {
		return HBeatMessage{}, err
	}
	return decoded, nil
}
func decodeAck(buf []byte, len int) (AckMessage, error) {
	var decoded AckMessage
	err := gob.NewDecoder(bytes.NewBuffer(buf[0:len])).Decode(&decoded)
	if err != nil {
		return AckMessage{}, err
	}
	return decoded, nil
}
