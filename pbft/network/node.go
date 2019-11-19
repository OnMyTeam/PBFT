package network

import (
	"encoding/json"
	"fmt"
	"github.com/bigpicturelabs/consensusPBFT/pbft/consensus"
	"time"
	"context"
	"crypto/ecdsa"
	//"log"
	"sync"
	"sync/atomic"
)

type Node struct {
	MyInfo          *NodeInfo
	PrivKey         *ecdsa.PrivateKey
	NodeTable       []*NodeInfo
	View            *View
	EpochID			int64
	States          map[int64]consensus.PBFT // key: sequenceID, value: state
	VCStates		map[int64]*consensus.VCState
	CommittedMsgs   map[int64]*consensus.PrepareMsg // kinda block.
	Committed		[1000]int64
	Prepared 		[1000]int64

	//ViewChangeState *consensus.ViewChangeState
	//CommittedMsgs   []*consensus.PrepareMsg // kinda block.
	TotalConsensus  int64 // atomic. number of consensus started so far.
	IsViewChanging  bool
	NextCandidateIdx int64

	// Channels
	MsgEntrance   chan interface{}
	MsgDelivery   chan interface{}
	MsgExecution  chan *consensus.PrepareMsg
	MsgOutbound   chan *MsgOut
	MsgError      chan []error
	ViewMsgEntrance chan interface{}
	ViewChangeChan chan ViewChangeChannel

	// Mutexes for preventing from concurrent access
	StatesMutex sync.RWMutex
	VCStatesMutex sync.RWMutex
	CommittedMutex sync.RWMutex
	PreparedMutex sync.RWMutex

	// Saved checkpoint messages on this node
	// key: sequenceID, value: map(key: nodeID, value: checkpointMsg)
	CheckPointMutex     sync.RWMutex
	CheckPointMsgsLog   map[int64]map[string]*consensus.CheckPointMsg

	// The stable checkpoint that 2f + 1 nodes agreed
	StableCheckPoint    int64
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

type ViewChangeChannel struct {
	Min_S  int64 `json:"min_s"`
	VCSCheck    bool `json:"vcscheck"`
}

// Deadline for the consensus state.
const ConsensusDeadline = time.Millisecond * 380

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
		EpochID:	0,
		IsViewChanging: false,
		NextCandidateIdx: 10,

		// Consensus-related struct
		States:          make(map[int64]consensus.PBFT),
		VCStates: 		 make(map[int64]*consensus.VCState),
		
		CheckPointMsgsLog: make(map[int64]map[string]*consensus.CheckPointMsg),
		StableCheckPoint:  0,

		CommittedMsgs:   make(map[int64]*consensus.PrepareMsg),

		// Channels
		MsgEntrance: make(chan interface{}, len(nodeTable) * 100),
		MsgDelivery: make(chan interface{}, len(nodeTable) * 100), // TODO: enough?
		MsgExecution: make(chan *consensus.PrepareMsg, len(nodeTable)*100),
		MsgOutbound: make(chan *MsgOut, len(nodeTable)*100),
		MsgError: make(chan []error, len(nodeTable)*100),
		ViewMsgEntrance: make(chan interface{}, len(nodeTable)*100),
		ViewChangeChan: make(chan ViewChangeChannel, len(nodeTable)*100),

	}

	atomic.StoreInt64(&node.TotalConsensus, 0)
	node.updateView(viewID)

	// Start message dispatcher
	for i:=0; i < 20; i++ {
	go node.dispatchMsg()
	}

	for i := 0; i < 10; i++ {
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

	//node.MsgOutbound <- &MsgOut{Path: node.MyInfo.Url + path, Msg: jsonMsg}
	node.MsgOutbound <- &MsgOut{IP: node.MyInfo.Url, Msg: jsonMsg, Path: path}
}

func (node *Node) Reply(msg *consensus.ReplyMsg) {
	// Broadcast reply.
	//node.Broadcast(*msg, "/reply")
}
func (node *Node) startTransitionWithDeadline(msg *consensus.ReqPrePareMsgs) {
	// time.Sleep(time.Millisecond*sendPeriod)
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
			return
		case <-ctx.Done(): 
			//node.GetPrepare(state, nil)
		
			var lastCommittedMsg *consensus.PrepareMsg = nil
			msgTotalCnt := int64(len(node.CommittedMsgs))
			if msgTotalCnt > 0 {
				lastCommittedMsg = node.CommittedMsgs[msgTotalCnt]
			}
		
			if msgTotalCnt == 0 ||
				lastCommittedMsg.SequenceID < state.GetSequenceID() {
				
				fmt.Println("Start ViewChange...")	
				
				//startviewchange
				node.IsViewChanging = true

				// Broadcast view change message.
				node.MsgError <- []error{ctx.Err()}
				fmt.Printf("&&&&&&&&&&&&&&&&&&& state.GetSequenceID %d &&&&&&&&&&&&&&&&&&\n",state.GetSequenceID())
				
				node.StartViewChange()
			}
			return
		
		}
	}
}
 
func (node *Node) GetPrepare(state consensus.PBFT, ReqPrePareMsgs *consensus.ReqPrePareMsgs) {
	prepareMsg := ReqPrePareMsgs.PrepareMsg
	requestMsg := ReqPrePareMsgs.RequestMsg

	node.EpochID = ReqPrePareMsgs.PrepareMsg.EpochID
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

	// if prepareMsg.SequenceID != 1 {
	// 	node.TimerStop(state, "Prepare")
	// } 
	// node.TimerStart(state, "Vote")

	atomic.AddInt64(&node.Prepared[prepareMsg.SequenceID],1)
	if node.Committed[prepareMsg.SequenceID] == 1 {
		node.MsgExecution <- state.GetPrepareMsg()
	} else {
		node.Broadcast(voteMsg, "/vote")
	}

	if !node.IsViewChanging {
		go node.startTransitionWithDeadline(nil)
	}
}
func (node *Node) GetVote(state consensus.PBFT, voteMsg *consensus.VoteMsg) {
	fmt.Println("[GetVote] to", node.MyInfo.NodeID ,"from", voteMsg.NodeID ,"sequenceID:", voteMsg.SequenceID, time.Now().UnixNano()) 

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
			node.Broadcast(collateMsg, "/collate")
			atomic.AddInt64(&node.Committed[voteMsg.SequenceID], 1)
			if node.Prepared[voteMsg.SequenceID] == 1 {
				node.MsgExecution <- state.GetPrepareMsg()
			}
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
	fmt.Printf("[GetCollate] to %s from %s sequenceID: %d TYPE : %d \n", 
					node.MyInfo.NodeID, collateMsg.NodeID, collateMsg.SequenceID, collateMsg.MsgType)
	switch collateMsg.MsgType {
		case consensus.COMMITTED:
			node.TimerStop(state, "Vote")

			if node.Committed[collateMsg.SequenceID] == 0 {
				/*
				for NodeID, VoteMsg := range state.GetVoteMsgs() {
					if  VoteMsg == collateMsg.ReceivedVoteMsg[NodeID] {
						continue
					}
					state.GetVoteMsgs()[NodeID] = collateMsg.ReceivedVoteMsg[NodeID]
				}
				*/

				atomic.AddInt64(&node.Committed[collateMsg.SequenceID], 1)
				if node.Prepared[collateMsg.SequenceID] == 1 {
					node.MsgExecution <- state.GetPrepareMsg()
				}

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
				atomic.AddInt64(&node.Committed[collateMsg.SequenceID], 1)
				node.Broadcast(newcollateMsg, "/collate")
				// ch2 := node.States[collateMsg.SequenceID].GetMsgExitSendChannel()
				// ch2 <- 0
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

	return consensus.CreateState(node.View.ID, node.MyInfo.NodeID, len(node.NodeTable), seqID)
}

func (node *Node) dispatchMsg() {
	for {
		select {
		case msg := <-node.MsgEntrance:
			if !node.IsViewChanging {
				node.MsgDelivery <- msg
			}
		case viewmsg := <-node.ViewMsgEntrance:
			node.MsgDelivery <- viewmsg
		}
	}
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
			if node.Committed[msg.SequenceID] == 1 {
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
		
		//case *consensus.CheckPointMsg:
		//	node.GetCheckPoint(msg)
		case *consensus.ViewChangeMsg:
			node.GetViewChange(msg)
		case *consensus.NewViewMsg:
			node.GetNewView(msg)
		}
 
		if err != nil {
			// Print error.
			node.MsgError <- []error{err}
			// Send message into dispatcher.
			node.MsgDelivery <- msgDelivered
		}
	}
}

func (node *Node) executeMsg() {
	pairs := make(map[int64]*consensus.PrepareMsg)
	for {
		prepareMsg := <- node.MsgExecution
		pairs[prepareMsg.SequenceID] = prepareMsg
		fmt.Println("[CommitMsg] sequenceID:", prepareMsg.SequenceID, time.Now().UnixNano())
		for {
			var lastSequenceID int64

			// Find the last committed message.
			msgTotalCnt := int64(len(node.CommittedMsgs))
			if msgTotalCnt > 0 {
				lastCommittedMsg := node.CommittedMsgs[msgTotalCnt]
				lastSequenceID = lastCommittedMsg.SequenceID
			} else {
				lastSequenceID = 0
			}
			//fmt.Println("[executeMsg] LastSequenceID : ", lastSequenceID)
			// Stop execution if the message for the
			// current sequence is not ready to execute.
			p := pairs[lastSequenceID + 1]
			
			if p == nil {
				break
			}
			fmt.Println("[Execute] sequenceID:",lastSequenceID + 1,",",time.Now().UnixNano())
			// Add the committed message in a private log queue
			// to print the orderly executed messages.
			node.CommittedMsgs[int64(lastSequenceID + 1)] = prepareMsg
			LogStage("Commit", true)

			node.StableCheckPoint = lastSequenceID + 1
			node.updateView(node.StableCheckPoint)
			node.updateEpochID(node.StableCheckPoint)
			if node.View.ID % 10 == 0 {
				node.VCStates = make(map[int64]*consensus.VCState)
				node.NextCandidateIdx = 10
			}
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
		case <-state.GetPhaseTimer(phase).C: //when timer is done


		case <-state.GetCancelTimerCh(phase): //when timer stop
		}
	}()
		*/
}
func (node *Node) TimerStop(state consensus.PBFT, phase string) {
	/*
	//log.Printf("%s Timer stop function.. phase is %s", node.MyInfo.NodeID,phase)
	state.GetPhaseTimer(phase).Stop()
	state.GetCancelTimerCh(phase) <- struct {}{}
	*/
}