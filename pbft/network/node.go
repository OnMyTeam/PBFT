package network

import (
	"github.com/bigpicturelabs/consensusPBFT/pbft/consensus"
	"encoding/json"
	"fmt"
	"time"
	//"errors"
	//"context"
	//"log"
	"sync"
	"sync/atomic"
	"crypto/ecdsa"
)

type Node struct {
	MyInfo          *NodeInfo
	PrivKey         *ecdsa.PrivateKey
	NodeTable       []*NodeInfo
	View            *View
	States          map[int64]consensus.PBFT // key: sequenceID, value: state
	Committed		[1000]int64

	//ViewChangeState *consensus.ViewChangeState
	//CommittedMsgs   []*consensus.PrepareMsg // kinda block.
	TotalConsensus  int64 // atomic. number of consensus started so far.

	// Channels
	MsgEntrance   chan interface{}
	MsgDelivery   chan interface{}
	MsgOutbound   chan *MsgOut
	MsgError      chan []error
	//ViewMsgEntrance chan interface{}

	// Mutexes for preventing from concurrent access
	StatesMutex sync.RWMutex
	CommittedMutex sync.RWMutex

	// CheckpointMsg save
	//StableCheckPoint    int64
	//CheckPointSendPoint int64
	//CheckPointMsgsLog   map[int64]map[string]*consensus.CheckPointMsg // key: sequenceID, value: map(key: nodeID, value: checkpointmsg)
}

type NodeInfo struct {
	NodeID string `json:"nodeID"`
	Url    string `json:"url"`
	PubKey *ecdsa.PublicKey
}

type View struct {
	ID      int64
	Primary *NodeInfo
}

// Outbound message
type MsgOut struct {
	IP   string
	Msg  []byte
	Path string
}

// Deadline for the consensus state.
const ConsensusDeadline = time.Millisecond * 1000

// Cooling time to escape frequent error, or message sending retry.
const CoolingTime = time.Millisecond * 2

// Number of error messages to start cooling.
const CoolingTotalErrMsg = 30

// Number of outbound connection for a node.
const MaxOutboundConnection = 1000

func NewNode(myInfo *NodeInfo, nodeTable []*NodeInfo, viewID int64, decodePrivKey *ecdsa.PrivateKey) *Node {
	node := &Node{
		MyInfo:    myInfo,
		PrivKey: decodePrivKey,
		NodeTable: nodeTable,
		View:      &View{},

		// Consensus-related struct
		States:          make(map[int64]consensus.PBFT),

		//CommittedMsgs:   make([]*consensus.PrepareMsg, 0),
		//ViewChangeState: nil,

		// Channels
		MsgEntrance: make(chan interface{}, len(nodeTable) * 100),
		MsgDelivery: make(chan interface{}, len(nodeTable) * 100), // TODO: enough?
		//MsgExecution: make(chan *MsgPair, len(nodeTable)*100),
		MsgOutbound: make(chan *MsgOut, len(nodeTable)*100),
		MsgError: make(chan []error, len(nodeTable)*100),
		//ViewMsgEntrance: make(chan interface{}, len(nodeTable)*3),

		//StableCheckPoint:  0,
		//CheckPointSendPoint: 0,
		//CheckPointMsgsLog: make(map[int64]map[string]*consensus.CheckPointMsg),
	}

	atomic.StoreInt64(&node.TotalConsensus, 0)
	node.updateView(viewID)

	// Start message dispatcher
	//for i:=0; i<16; i++ {
	//go node.dispatchMsg()
	//}

	for i := 0; i < 10; i++ {
		// Start message resolver
		go node.resolveMsg()
	}

	// Start message executor
	//go node.executeMsg()

	// Start outbound message sender
	go node.sendMsg()

	// Start message error logger
	go node.logErrorMsg()

	return node
}

// Broadcast marshalled message.
func (node *Node) Broadcast(msg interface{}, path string) {
	jsonMsg, err := json.Marshal(msg)
	if err != nil {
		node.MsgError <- []error{err}
		return
	}

	//node.MsgOutbound <- &MsgOut{Path: node.MyInfo.Url + path, Msg: jsonMsg}
	node.MsgOutbound <- &MsgOut{IP: node.MyInfo.Url, Msg: jsonMsg, Path: path}
}

func (node *Node) Reply(msg *consensus.ReplyMsg) {
	// Broadcast reply.
	//node.Broadcast(*msg, "/reply")
}
func (node *Node) startTransitionWithDeadline(msg *consensus.ReqPrePareMsgs) {
	//time.Sleep(time.Millisecond*sendPeriod)

	newTotalConsensus := atomic.AddInt64(&node.TotalConsensus, 1)
	fmt.Printf("Consensus Process.. newTotalConsensus num is %d\n", newTotalConsensus)
	state := node.createState(newTotalConsensus)
	node.StatesMutex.Lock()
	node.States[newTotalConsensus] = state
	node.StatesMutex.Unlock()

	if msg == nil{
		node.TimerStart(state, "Prepare")
	 	node.TimerStart(state, "ViewChange")
	} else {
		node.TimerStart(state, "ViewChange")
		node.GetPrepare(state, msg)
	}

	// The node can receive messages for any consensus stage,
	// regardless of the current stage for the state.
	ch := state.GetMsgReceiveChannel()
	ch2 := state.GetMsgExitReceiveChannel()
	for {
		select {
		case msgState := <-ch:
			switch msg := msgState.(type) {
			//case *consensus.PrePrepareMsg:
			//	node.GetPrePrepare(state, msg)
			case *consensus.ReqPrePareMsgs:
				node.GetPrepare(state, msg)
			case *consensus.VoteMsg:
				node.GetVote(state, msg)
			case *consensus.CollateMsg:
				node.GetCollate(state, msg)
			}
		case <-ch2:
			//time.Sleep(time.Millisecond*sendPeriod*10)
			return
		}
	}
}

func (node *Node) GetPrepare(state consensus.PBFT, ReqPrePareMsgs *consensus.ReqPrePareMsgs) {
	prepareMsg := ReqPrePareMsgs.PrepareMsg
	requestMsg := ReqPrePareMsgs.RequestMsg
	fmt.Printf("[GetPrepare] to %s from %s sequenceID: %d\n", 
						node.MyInfo.NodeID, prepareMsg.NodeID, prepareMsg.SequenceID)
	voteMsg, err := state.Prepare(prepareMsg, requestMsg)
	if err != nil {
		node.MsgError <- []error{err}
	}

	//Check VoteMsg created
	if voteMsg.SequenceID == 0 {
		return
	}
	fmt.Printf("[GetPrepare] seq %d\n", voteMsg.SequenceID)
	// Attach node ID to the message
	voteMsg.NodeID = node.MyInfo.NodeID

	if prepareMsg.SequenceID != 1 {
		node.TimerStop(state, "Prepare")
	} 
	node.TimerStart(state, "Vote")

	node.Broadcast(voteMsg, "/vote")

	go node.startTransitionWithDeadline(nil)
}
func (node *Node) GetVote(state consensus.PBFT, voteMsg *consensus.VoteMsg) {
	fmt.Printf("[GetVote] to %s from %s sequenceID: %d\n", 
					node.MyInfo.NodeID, voteMsg.NodeID, voteMsg.SequenceID)

	collateMsg, err := state.Vote(voteMsg)
	if err != nil {
		node.MsgError <- []error{err}
	}

	// Check COLLATE message created.
	if collateMsg.SequenceID == 0 {
		return
	}

	// Attach node ID to the message
	collateMsg.NodeID = node.MyInfo.NodeID

	switch collateMsg.MsgType {
	case consensus.COMMITTED:
		node.TimerStop(state, "Vote")

		if node.Committed[voteMsg.SequenceID] == 0 {
			fmt.Printf("[Committed] %d vote is in executed \n", voteMsg.SequenceID)
			atomic.AddInt64(&node.Committed[voteMsg.SequenceID], 1)
			node.Broadcast(collateMsg, "/collate")
			state, err := node.getState(collateMsg.SequenceID)
			if err != nil {
				// Print error.
				node.MsgError <- []error{err}
				// Send message into dispatcher.
				return
			}			
			ch := state.GetMsgExitSendChannel()
			ch <- 0
		}

		//Done
	// case consensus.UNCOMMITTED:
	// 	node.TimerStart(state, "Collate")

	// 	LogStage("Vote", true)
	// 	node.Broadcast(collateMsg, "/collate")
	// 	LogStage("commit", false)
		}
}
func (node *Node) GetCollate(state consensus.PBFT, collateMsg *consensus.CollateMsg) {
	fmt.Printf("[GetCollate] to %s from %s sequenceID: %d\n", 
					node.MyInfo.NodeID, collateMsg.NodeID, collateMsg.SequenceID)
	switch collateMsg.MsgType {
		case consensus.COMMITTED:
			node.TimerStop(state, "Vote")

			if node.Committed[collateMsg.SequenceID] == 0 {
				fmt.Printf("[Committed] %d vote is in executed Late\n", collateMsg.SequenceID)
				atomic.AddInt64(&node.Committed[collateMsg.SequenceID], 1)
				node.Broadcast(collateMsg, "/collate")
				state, err := node.getState(collateMsg.SequenceID)
				if err != nil {
					// Print error.
					node.MsgError <- []error{err}
					// Send message into dispatcher.
					return
				}			
				ch := state.GetMsgExitSendChannel()
				ch <- 0
			}
		case consensus.UNCOMMITTED: 
			newcollateMsg, isVoting, err := state.Collate(collateMsg)
			if err != nil {
				node.MsgError <- []error{err}
			}

			if newcollateMsg.SequenceID == 0 { 	//Only COMMITTED msg is created
				fmt.Println("newcollateMsg : ", newcollateMsg)
				return
			}

			if isVoting {		// vote timer is running.. collate timer is not running..
				node.TimerStop(state, "Vote")
			} else {							// vote timer is not running.. collate timer is running..
				node.TimerStop(state, "Collate")
			}

			// Attach node ID to the message
			newcollateMsg.NodeID = node.MyInfo.NodeID

			if node.Committed[collateMsg.SequenceID] == 0 {
				fmt.Printf("[Committed] collate is in executed\n")
				atomic.AddInt64(&node.Committed[collateMsg.SequenceID], 1)
				node.Broadcast(newcollateMsg, "/collate")
				ch := node.States[collateMsg.SequenceID].GetMsgExitSendChannel()
				ch <- 0
			}		

	}

}

func (node *Node) GetReply(msg *consensus.ReplyMsg) {
	LogMsg(msg)
}

func (node *Node) createState(seqID int64) consensus.PBFT {
	// TODO: From TOCS: To guarantee exactly once semantics,
	// replicas discard requests whose timestamp is lower than
	// the timestamp in the last reply they sent to the client.

	return consensus.CreateState(node.View.ID, node.MyInfo.NodeID, len(node.NodeTable),  seqID)
}
func (node *Node) resolveMsg() {
	for {
		var state consensus.PBFT
		var err error = nil
		msgDelivered := <-node.MsgDelivery
		// Resolve the message.
		switch msg := msgDelivered.(type) {
		// If even the previous sequence of entered sequence thread is not created
		// ignore the entered message.
		case *consensus.ReqPrePareMsgs:
			state, err = node.getState(msg.PrepareMsg.SequenceID)
			if state != nil {
				ch := state.GetMsgSendChannel()
				ch <- msg
			} else {
				if msg.PrepareMsg.SequenceID == 1 {
					node.startTransitionWithDeadline(msg)
				}
			}
		case *consensus.VoteMsg:
			if  node.Committed[msg.SequenceID] == 1{
				continue
			}
			state, err = node.getState(msg.SequenceID)
			if state != nil {
				ch := state.GetMsgSendChannel()
				ch <- msg
			}
		case *consensus.CollateMsg:
			// if node.Committed[msg.SequenceID] == 1 {
			// 	continue
			// }
			if node.MyInfo.NodeID != msg.NodeID {
				state, err = node.getState(msg.SequenceID)
				if state != nil {
					ch := state.GetMsgSendChannel()
					ch <- msg
				}
			}

		case *consensus.ReplyMsg:
			node.GetReply(msg)
		/*
		case *consensus.CheckPointMsg:
			node.GetCheckPoint(msg)
		case *consensus.ViewChangeMsg:
			err = node.GetViewChange(msg)
		case *consensus.NewViewMsg:
			err = node.GetNewView(msg)
		*/
		}

		if err != nil {
			// Print error.
			node.MsgError <- []error{err}
			// Send message into dispatcher.
			node.MsgEntrance <- msgDelivered
		}
	}
}

func (node *Node) sendMsg() {
	sem := make(chan bool, MaxOutboundConnection)

	for {
		msg := <-node.MsgOutbound

		// Goroutine for concurrent broadcast() with timeout
		sem <- true
		go func() {
			defer func() { <-sem }()
			errCh := make(chan error, 1)

			// Goroutine for concurrent broadcast()
			go func() {
				broadcast(errCh, msg.IP, msg.Msg, msg.Path, node.PrivKey)
			}()

			select {
			case err := <-errCh:
				if err != nil {
					node.MsgError <- []error{err}
					// TODO: view change.
				}
			}
		}()
	}
}

func (node *Node) logErrorMsg() {
	coolingMsgLeft := CoolingTotalErrMsg

	for {
		errs := <-node.MsgError
		for _, err := range errs {
			coolingMsgLeft--
			if coolingMsgLeft == 0 {
				fmt.Printf("%d error messages detected! cool down for %d milliseconds\n",
				           CoolingTotalErrMsg, CoolingTime / time.Millisecond)
				time.Sleep(CoolingTime)
				coolingMsgLeft = CoolingTotalErrMsg
			}
			fmt.Println(err)
		}
	}
}

func (node *Node) getState(sequenceID int64) (consensus.PBFT, error) {
	node.StatesMutex.RLock()
	state := node.States[sequenceID]
	node.StatesMutex.RUnlock()

	if state == nil {
		return nil, fmt.Errorf("State for sequence number %d has not created yet.", sequenceID)
	}

	return state, nil
}
func (node *Node) TimerStart(state consensus.PBFT, phase string) {
	//log.Printf("%s Timer start function.. phase is %s", node.MyInfo.NodeID,phase)
	/*
	state.SetTimer(phase)
	go func() {
		select {
		case <-state.GetPhaseTimer(phase).C:
			//node.GetPrepare(state, nil)
		case <-state.GetCancelTimerCh(phase):
		}
	}()
	*/
}
func (node *Node) TimerStop(state consensus.PBFT, phase string) {
	//log.Printf("%s Timer stop function.. phase is %s", node.MyInfo.NodeID,phase)
	/*
	state.GetPhaseTimer(phase).Stop()
	state.GetCancelTimerCh(phase) <- struct {}{}
	*/
}