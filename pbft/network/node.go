package network

import (
	"github.com/bigpicturelabs/consensusPBFT/pbft/consensus"
	"encoding/json"
	"fmt"
	"time"
	//"errors"
	"context"
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
	//ViewChangeState *consensus.ViewChangeState
	CommittedMsgs   []*consensus.PrepareMsg // kinda block.
	TotalConsensus  int64 // atomic. number of consensus started so far.

	// Channels
	MsgEntrance   chan interface{}
	MsgDelivery   chan interface{}
	MsgExecution  chan *MsgPair
	MsgOutbound   chan *MsgOut
	MsgError      chan []error
	//ViewMsgEntrance chan interface{}

	// Mutexes for preventing from concurrent access
	StatesMutex sync.RWMutex

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

type MsgPair struct {
	replyMsg     *consensus.ReplyMsg
	committedMsg *consensus.PrepareMsg
}

// Outbound message
type MsgOut struct {
	Path string
	Msg  []byte
}

// Deadline for the consensus state.
const ConsensusDeadline = time.Millisecond * 5000

// Cooling time to escape frequent error, or message sending retry.
const CoolingTime = time.Millisecond * 2

// Number of error messages to start cooling.
const CoolingTotalErrMsg = 5

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
		CommittedMsgs:   make([]*consensus.PrepareMsg, 0),
		//ViewChangeState: nil,

		// Channels
		MsgEntrance: make(chan interface{}, len(nodeTable) * 3),
		MsgDelivery: make(chan interface{}, len(nodeTable) * 3), // TODO: enough?
		MsgExecution: make(chan *MsgPair, len(nodeTable)*3),
		MsgOutbound: make(chan *MsgOut, len(nodeTable)*3),
		MsgError: make(chan []error, len(nodeTable)*3),
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

	for i := 0; i < 5; i++ {
		// Start message resolver
		go node.resolveMsg()
	}

	// Start message executor
	go node.executeMsg()

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

	node.MsgOutbound <- &MsgOut{Path: node.MyInfo.Url + path, Msg: jsonMsg}
}

func (node *Node) Reply(msg *consensus.ReplyMsg) {
	// Broadcast reply.
	node.Broadcast(msg, "/reply")
}
func (node *Node) startTransitionWithDeadline(msg *consensus.PrepareMsg) {
	//time.Sleep(time.Millisecond*sendPeriod)
	// Set deadline based on the given timestamp.
	var timeStamp int64 = time.Now().UnixNano()
	sec := timeStamp / int64(time.Second)
	nsec := timeStamp % int64(time.Second)
	d := time.Unix(sec, nsec).Add(ConsensusDeadline)
	ctx, cancel := context.WithDeadline(context.Background(), d)
	defer cancel()
	// Check the time is skewed.
	//timeDiff := time.Until(d).Nanoseconds()


	newTotalConsensus := atomic.AddInt64(&node.TotalConsensus, 1)
	fmt.Printf("Consensus Process.. newTotalConsensus num is %d\n", newTotalConsensus)
	state := node.createState(newTotalConsensus)
	node.StatesMutex.Lock()
	node.States[newTotalConsensus] = state
	node.StatesMutex.Unlock()

	if msg == nil{
		// node.TimerStart(state, "Prepare")
	 	// node.TimerStart(state, "ViewChange")
	} else {
		// node.TimerStart(state, "ViewChange")
		node.GetPrepare(state, msg)
	}

	// The node can receive messages for any consensus stage,
	// regardless of the current stage for the state.
	ch := state.GetMsgReceiveChannel()
	//ch2 := state.GetMsgExitReceiveChannel()
	for {
		select {
		case msgState := <-ch:
			switch msg := msgState.(type) {
			//case *consensus.PrePrepareMsg:
			//	node.GetPrePrepare(state, msg)
			case *consensus.PrepareMsg:
				node.GetPrepare(state, msg)
			case *consensus.VoteMsg:
				node.GetVote(state, msg)
			case *consensus.CollateMsg:
				node.GetCollate(state, msg)
			}
		//case <-ch2:
		case <-ctx.Done():
			fmt.Println("thread finished...")
			//time.Sleep(time.Millisecond*sendPeriod*10)
			return
		}
	}
}

func (node *Node) GetPrepare(state consensus.PBFT, prepareMsg *consensus.PrepareMsg) {
	fmt.Printf("[GetPrepare] to %s from %s sequenceID: %d\n", 
						node.MyInfo.NodeID, prepareMsg.NodeID, prepareMsg.SequenceID)
	voteMsg, err := state.Prepare(prepareMsg)
	if err != nil {
		node.MsgError <- []error{err}
	}

	//Check VoteMsg created
	if voteMsg == nil {
		return
	}
	
	// Attach node ID to the message
	voteMsg.NodeID = node.MyInfo.NodeID

	// if prepareMsg.SequenceID != 1 {
	// 	node.TimerStop(state, "Prepare")
	// } 
	// node.TimerStart(state, "Vote")

	LogStage("Prepare", true)
	node.Broadcast(voteMsg, "/vote")
	fmt.Printf("[GetVote] %s Broad Cast sequenceID: %d \n", node.MyInfo.NodeID,prepareMsg.SequenceID)
	node.GetVote(state, voteMsg)
	LogStage("Vote", false)

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
	if collateMsg == nil {
		return
	}

	// Attach node ID to the message
	collateMsg.NodeID = node.MyInfo.NodeID

	switch collateMsg.MsgType {
	case consensus.COMMITTED:
		// node.TimerStop(state, "Vote")
		// Commit the Msg..
		replyMsg, committedMsg := state.Commit()
		// Attach node ID to the message
		replyMsg.NodeID = node.MyInfo.NodeID
		//sdf
		// Pass the incomplete reply message through MsgExecution
		// channel to run its operation sequentially.
		node.MsgExecution <- &MsgPair{replyMsg, committedMsg}

		LogStage("Vote", true)
		// node.Broadcast(collateMsg, "/collate")
		// LogStage("Collate", false)

		//Done
	// case consensus.UNCOMMITTED:
	// 	node.TimerStart(state, "Collate")

	// 	LogStage("Vote", true)
	// 	node.Broadcast(collateMsg, "/collate")
	// 	LogStage("commit", false)
		}
}
func (node *Node) GetCollate(state consensus.PBFT, collateMsg *consensus.CollateMsg) {
	// newcollateMsg, isVoting, err := state.Collate(collateMsg)
	newcollateMsg, _, err := state.Collate(collateMsg)
	if err != nil {
		node.MsgError <- []error{err}
	}

	if newcollateMsg == nil { 	//Only COMMITTED msg is created
		fmt.Println("newcollateMsg : ", newcollateMsg)
		return
	}

	// if isVoting {		// vote timer is running.. collate timer is not running..
	// 	node.TimerStop(state, "Vote")
	// } else {							// vote timer is not running.. collate timer is running..
	// 	node.TimerStop(state, "Collate")
	// }

	// Attach node ID to the message
	newcollateMsg.NodeID = node.MyInfo.NodeID

	LogStage("collate", true)
	node.Broadcast(newcollateMsg, "/collate")
	LogStage("commit", false)

	replyMsg, committedMsg := state.Commit()

	// Attach node ID to the message
	replyMsg.NodeID = node.MyInfo.NodeID

	// Pass the incomplete reply message through MsgExecution
	// channel to run its operation sequentially.
	node.MsgExecution <- &MsgPair{replyMsg, committedMsg}
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
/*
func (node *Node) dispatchMsg() {
	for {
		select {
		case msg := <-node.MsgEntrance:

			node.routeMsg(msg)
		//case viewmsg := <-node.ViewMsgEntrance:
		//	fmt.Println("dispatchMsg()")
		//	node.routeMsg(viewmsg)
		}
	}
}
*/
/*
func (node *Node) routeMsg(msgEntered interface{}) {
	switch msg := msgEntered.(type) {
	// Messages are broadcasted from the node, so
	// the message sent to itself can exist.
	case *consensus.PrepareMsg:
		//if !node.isMyNodePrimary() {
		//if node.MyInfo.NodeID != msg.NodeID{
		node.MsgDelivery <- msg
		//}
	case *consensus.VoteMsg:
		if node.MyInfo.NodeID != msg.NodeID {
			node.MsgDelivery <- msg
		}
	case *consensus.CollateMsg:
		if node.MyInfo.NodeID != msg.NodeID {
			node.MsgDelivery <- msg
		}
	case *consensus.ReplyMsg:
		node.MsgDelivery <- msg
	
	case *consensus.CheckPointMsg:
		node.MsgDelivery <- msg
	case *consensus.ViewChangeMsg:
		node.MsgDelivery <- msg
	case *consensus.NewViewMsg:
		node.MsgDelivery <- msg
		
	}
}
*/
func (node *Node) resolveMsg() {
	for {
		var state consensus.PBFT
		var err error = nil
		msgDelivered := <-node.MsgDelivery
		// Resolve the message.
		switch msg := msgDelivered.(type) {
		// If even the previous sequence of entered sequence thread is not created
		// ignore the entered message.
		case *consensus.PrepareMsg:
			fmt.Printf("[resolveMsg] %d sequence id\n", msg.SequenceID)
			state, err = node.getState(msg.SequenceID)
			if state != nil {
				ch := state.GetMsgSendChannel()
				ch <- msg
			} else {
				if msg.SequenceID == 1 {
					node.startTransitionWithDeadline(msg)
				}
			}
		case *consensus.VoteMsg:
			if int64(len(node.CommittedMsgs))>=msg.SequenceID&&node.CommittedMsgs[msg.SequenceID-1]!=nil {
				continue
			}
			if node.MyInfo.NodeID != msg.NodeID {
				state, err = node.getState(msg.SequenceID)
				if state != nil {
					ch := state.GetMsgSendChannel()
					ch <- msg
				} 
			}
		case *consensus.CollateMsg:
			if int64(len(node.CommittedMsgs))>=msg.SequenceID&&node.CommittedMsgs[msg.SequenceID-1]!=nil {
				continue
			}
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

// Fill the result field, after all execution for
// other states which the sequence number is smaller,
// i.e., the sequence number of the last committed message is
// one smaller than the current message.
func (node *Node) executeMsg() {
	var committedMsgs []*consensus.PrepareMsg
	pairs := make(map[int64]*MsgPair)
	for {
		msgPair := <-node.MsgExecution
		fmt.Printf("%s have %d votemsg sequence for %d \n", node.MyInfo.NodeID, len(node.States[msgPair.committedMsg.SequenceID].GetVoteMsgs()), msgPair.committedMsg.SequenceID)
		pairs[msgPair.committedMsg.SequenceID] = msgPair
		committedMsgs = make([]*consensus.PrepareMsg, 0)

		// Execute operation for all the consecutive messages.
		for {
			var lastSequenceID int64

			// Find the last committed message.
			msgTotalCnt := len(node.CommittedMsgs)
			if msgTotalCnt > 0 {
				lastCommittedMsg := node.CommittedMsgs[msgTotalCnt - 1]
				lastSequenceID = lastCommittedMsg.SequenceID
			} else {
				lastSequenceID = 0
			}

			// Stop execution if the message for the
			// current sequence is not ready to execute.
			p := pairs[lastSequenceID + 1]
			if p == nil {
				break
			}
			// Add the committed message in a private log queue
			// to print the orderly executed messages.
			committedMsgs = append(committedMsgs, p.committedMsg)
			LogStage("Commit", true)
			fmt.Println("[executeMsg]sequenceID is ",p.committedMsg.SequenceID)
			// TODO: execute appropriate operation.
			p.replyMsg.Result = "Executed"

			// After executing the operation, log the
			// corresponding committed message to node.
			node.CommittedMsgs = append(node.CommittedMsgs, p.committedMsg)
			node.Reply(p.replyMsg)
			// ch := node.States[p.committedMsg.SequenceID].GetMsgExitSendChannel()
			// ch <- 0
			LogStage("Reply", true)

			/*
			nCheckPoint := node.CheckPointSendPoint + periodCheckPoint
			msgTotalCnt1 := len(node.CommittedMsgs)

			if node.CommittedMsgs[msgTotalCnt1 - 1].SequenceID ==  nCheckPoint{
				node.CheckPointSendPoint = nCheckPoint

				SequenceID := node.CommittedMsgs[len(node.CommittedMsgs) - 1].SequenceID
				checkPointMsg, _ := node.getCheckPointMsg(SequenceID, node.MyInfo.NodeID, node.CommittedMsgs[msgTotalCnt1 - 1])
				LogStage("CHECKPOINT", false)
				node.Broadcast(checkPointMsg, "/checkpoint")
				node.CheckPoint(checkPointMsg)
			}*/

			delete(pairs, lastSequenceID + 1)
			
		}

		// Print all committed messages.
		/*
		for _, v := range committedMsgs {
			digest, _ := consensus.Digest(v.RequestMsg.Data)
			fmt.Printf("***committedMsgs[%d]: clientID=%s, operation=%s, timestamp=%d, data(digest)=%s***\n",
			           v.RequestMsg.SequenceID, v.RequestMsg.ClientID, v.RequestMsg.Operation, v.RequestMsg.Timestamp, digest)
		}
		*/
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
				broadcast(errCh, msg.Path, msg.Msg, node.PrivKey)
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