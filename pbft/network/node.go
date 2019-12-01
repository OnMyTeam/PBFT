package network

import (
	"encoding/json"
	"fmt"
	"github.com/bigpicturelabs/consensusPBFT/pbft/consensus"
	"time"
	// "context"
	"crypto/ecdsa"
	"log"
	"sync"
	"sync/atomic"
	//"runtime"
)

type Node struct {
	MyInfo          *NodeInfo
	PrivKey         *ecdsa.PrivateKey
	NodeTable       []*NodeInfo
	SeedNodeTables	[20][]*NodeInfo
	View            *View
	EpochID			int64
	
	States          map[int64]consensus.PBFT // key: sequenceID, value: state
	VCStates		map[int64]*consensus.VCState
	CommittedMsgs   map[int64]*consensus.PrepareMsg // kinda block.

	Committed		[1000]int64
	Prepared 		[1000]int64

	//ViewChangeState *consensus.ViewChangeState
	TotalConsensus  int64 // atomic. number of consensus started so far.
	IsViewChanging  bool
	NextCandidateIdx int64

	// Channels
	MsgEntrance   chan interface{}
	MsgSend       chan interface{}
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
// const ConsensusDeadline = time.Millisecond * 5000

// Cooling time to escape frequent error, or message sending retry.
const CoolingTime = time.Millisecond * 2

// Number of error messages to start cooling.
const CoolingTotalErrMsg = 30

// Number of outbound connection for a node.
const MaxOutboundConnection = 3000

func NewNode(myInfo *NodeInfo, nodeTable []*NodeInfo, seedNodeTables [20][]*NodeInfo,
			viewID int64, decodePrivKey *ecdsa.PrivateKey) *Node {
	node := &Node{
		MyInfo:    myInfo,
		PrivKey: decodePrivKey,
		NodeTable: nodeTable,
		SeedNodeTables: seedNodeTables,
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
		MsgExecution: make(chan *consensus.PrepareMsg, len(nodeTable) * 100),
		MsgOutbound: make(chan *MsgOut, len(nodeTable)),
		MsgError: make(chan []error, len(nodeTable)),
		ViewMsgEntrance: make(chan interface{}, len(nodeTable)*3),
	}

	atomic.StoreInt64(&node.TotalConsensus, 0)
	node.updateViewID(viewID)

	// Start message dispatcher
	for i:=0; i < 19; i++ {
		go node.dispatchMsg()
	}

	for i := 0; i < 19; i++ {
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
	node.MsgOutbound <- &MsgOut{IP: node.MyInfo.Url, Msg: jsonMsg, Path: path}
}

func (node *Node) startTransitionWithDeadline(seqID int64, state consensus.PBFT) {

	var sigma	[4]time.Duration
	sigma[consensus.NumOfPhase("Prepare")] = 450
	sigma[consensus.NumOfPhase("Vote")] = 150
	sigma[consensus.NumOfPhase("Collate")] = 50000
	sigma[consensus.NumOfPhase("ViewChange")] = 500000

	var timerArr			[4]*time.Timer
	var cancelCh			[4]chan struct {}

	MsgCh := state.GetMsgReceiveChannel()
	TimerStartCh := state.GetTimerStartReceiveChannel()
	TimerStopCh := state.GetTimerStopReceiveChannel()
	ExitCh := state.GetMsgExitReceiveChannel()
	ExitCh1 := state.GetMsgExitReceiveChannel1()
	go func(){
		for {
			select {
			case msgState := <-MsgCh:

				switch msg := msgState.(type) {
				case *consensus.ReqPrePareMsgs:
					node.GetPrepare(state, msg)
				case *consensus.VoteMsg:
					node.GetVote(state, msg)
				case *consensus.CollateMsg:
					node.GetCollate(state, msg)
				case *consensus.ViewChangeMsg:
					node.GetViewChange(msg)
				case *consensus.NewViewMsg:
					node.GetNewView(msg)
				}
			case <-ExitCh1:
				fmt.Printf("[Terminate Thread] seqId %d finished!!!\n", state.GetSequenceID())
				return
			}

		}
	}()	
	go func() {

		for {
			select {
			// case msgState := <-MsgCh:

			// 	switch msg := msgState.(type) {
			// 	case *consensus.ReqPrePareMsgs:
			// 		node.GetPrepare(state, msg)
			// 	case *consensus.VoteMsg:
			// 		node.GetVote(state, msg)
			// 	case *consensus.CollateMsg:
			// 		node.GetCollate(state, msg)
			// 	}
			case phaseName := <- TimerStartCh:
				phase:=consensus.NumOfPhase(phaseName)
				if timerArr[phase] == nil {
					timerArr[phase] = time.NewTimer(time.Millisecond*sigma[phase])
					cancelCh[phase] = make(chan struct {}, 10)
					fmt.Printf("[Seq %d Thread] Start %s Timer\n", seqID, phaseName)
				}

				go func(phase int64, phaseName string) {
					select {
					case <-timerArr[phase].C: //when timer is done
						switch phaseName{
							case "Prepare":
								state.GetTimerStartSendChannel() <- "Vote"
								fmt.Println("Not Receive Preparemsg... BroadCast NULL VOTE ",seqID)
								var PrepareMsg consensus.PrepareMsg
								PrepareMsg.ViewID = 0
								PrepareMsg.SequenceID = seqID
								PrepareMsg.Digest = ""
								PrepareMsg.EpochID = 0
								PrepareMsg.NodeID = ""
								PrepareMsg.Seed= 0
															
								// NULL Vote
								voteMsg, _:= state.Prepare(&PrepareMsg, nil)
								voteMsg.NodeID = node.MyInfo.NodeID
								node.Broadcast(voteMsg, "/vote")
								
								
							case "Vote":
								fmt.Println("Vote finished....", seqID)
								collateMsg, _ := state.VoteAQ(int32(len(node.NodeTable)))
								collateMsg.NodeID = node.MyInfo.NodeID
								

								switch collateMsg.MsgType {
								// Stop vote phase and start collate phase if it is not committed
									case consensus.UNCOMMITTED:
										fmt.Println("==== ADAPTIVE VOTE QUORUM UNCOMMITED====")
										node.Broadcast(collateMsg, "/collate")
									// Stop vote phase and execute the sequence if it is committed
									case consensus.COMMITTED:
										//state.GetTimerStopSendChannel() <- "Vote"
										node.CommittedMutex.Lock()
										if node.Committed[collateMsg.SequenceID] == 0 {
											fmt.Println("==== ADAPTIVE VOTE QUORUM COMMITED====")
											if state.GetPrepareMsg() == nil {
												for _, vote := range collateMsg.ReceivedVoteMsg {
													node.MsgExecution <- vote.PrepareMsg
													log.Printf("vote.PrepareMsg: %s",vote.PrepareMsg)
													break
												}
											} else {
												node.MsgExecution <- state.GetPrepareMsg()
											}
											node.Broadcast(collateMsg, "/collate")
										} else {
											fmt.Println("Already Commit and Execute SequenceID :", collateMsg.SequenceID)
										}

										node.CommittedMutex.Unlock()
										// Log last sequence id for checkpointing

								}	
							case "Collate":
								fmt.Println("Collate finished....", seqID)
								newcollateMsg, _ := state.VoteAQ(int32(len(node.NodeTable)))
								newcollateMsg.NodeID = node.MyInfo.NodeID
								switch newcollateMsg.MsgType {
									case consensus.UNCOMMITTED:

									// Stop vote phase and execute the sequence if it is committed
									case consensus.COMMITTED:
										//state.GetTimerStopSendChannel() <- "Vote"
										node.CommittedMutex.Lock()
										if node.Committed[newcollateMsg.SequenceID] == 0 {
											fmt.Println("==== ADAPTIVE COLLATE QUORUM COMMITED====")
											if state.GetPrepareMsg() == nil {
												for _, vote := range newcollateMsg.ReceivedVoteMsg {
													node.MsgExecution <- vote.PrepareMsg
													log.Printf("vote.PrepareMsg: %s",vote.PrepareMsg)
													break
												}
											} else {
												node.MsgExecution <- state.GetPrepareMsg()
											}
											node.Broadcast(newcollateMsg, "/collate")
										} else {
											fmt.Println("Already Commit and Execute SequenceID :", newcollateMsg.SequenceID)
										}

										node.CommittedMutex.Unlock()
										// Log last sequence id for checkpointing
								}

							case "ViewChange":
								fmt.Println("Start ViewChange...")
								fmt.Printf("&&&&&&&&&&&&&&&&&&& state.GetSequenceID %d &&&&&&&&&&&&&&&&&&\n",state.GetSequenceID())
								node.StartViewChange(state.GetSequenceID())										

						}
					case <-cancelCh[phase]: //when timer stop
					}
				}(phase, phaseName)
			case phaseName := <-TimerStopCh:
				phase := consensus.NumOfPhase(phaseName)
				if timerArr[phase] != nil {
					switch phaseName{
						case "Prepare":
							fmt.Println("Prepare Timer Stop....")

						case "Vote":
							fmt.Println("Vote Stop....")
						
						case "ViewChange":
							fmt.Println("ViewChange Stop...")

					}					
					timerArr[phase].Stop()
				}
				
				cancelCh[phase] <- struct {}{}
			case <-ExitCh:
				// //fmt.Printf("[Terminate Thread] seqId %d finished!!!\n", state.GetSequenceID())
				// node.StatesMutex.Lock()
				// node.States[seqID] = nil
				// node.StatesMutex.Unlock()
				return
			}

		}
	}()
}
func (node *Node) BroadCastNextPrepareMsgIfPrimary(sequenceID int64){
	//var epoch int64 = 0
	var seed int64 = -1

	data := make([]byte, 1 << 20)
	for i := range data {
		data[i] = 'A'
	}
	data[len(data)-1]=0


	node.updateViewID(sequenceID-1)
	// if (sequenceID-1) % 10 == 0 {
	// 	node.updateEpochID(sequenceID-1)		
	// 	//node.NextCandidateIdx = 11
	// }
	primaryNode := node.getPrimaryInfoByID(node.View.ID)

	fmt.Printf("server.node.MyInfo.NodeID: %s\n", node.MyInfo.NodeID)
	fmt.Printf("primaryNode.NodeID: %s\n", primaryNode.NodeID)

	// if sequenceID % 10 == 1 && sequenceID != 1{
	// 	epoch += 1
	// 	seed = epoch % 19+1
	// 	//server.node.setNewSeedList(int(seed))
	// } else {
	// 	seed = -1
	// }
	//errCh := make(chan error, 1)
	if primaryNode.NodeID != node.MyInfo.NodeID {
		return
	}

	prepareMsg := PrepareMsgMaking("Op1", "Client1", data, 
		node.View.ID,int64(sequenceID),
		node.MyInfo.NodeID, int(seed), node.EpochID)

	log.Printf("Broadcasting dummy message from %s, sequenceId: %d, epochId: %d, viewId: %d",
		node.MyInfo.NodeID, sequenceID, node.EpochID, node.View.ID)

	fmt.Println("[StartPrepare]", "seqID / ",sequenceID,"/", time.Now().UnixNano())
	time.Sleep(time.Millisecond * 40)
	node.Broadcast(prepareMsg, "/prepare")
	fmt.Println("[StartPrepare] After Broadcast!")
	//broadcast(errCh, node.MyInfo.Url, dummy, "/prepare", node.PrivKey)
	// err := <-errCh
	// if err != nil {
	// 	log.Println(err)
	// }
}

func (node *Node) GetPrepare(state consensus.PBFT, ReqPrePareMsgs *consensus.ReqPrePareMsgs) {
	prepareMsg := ReqPrePareMsgs.PrepareMsg
	requestMsg := ReqPrePareMsgs.RequestMsg
	//fmt.Println("[PrepareMsg]",prepareMsg.SequenceID,"/",time.Now().UnixNano())
	fmt.Printf("[GetPrepare] to %s from %s sequenceID: %d\n", 
						node.MyInfo.NodeID, prepareMsg.NodeID, prepareMsg.SequenceID)
	// When receive Prepare, save current time
	state.SetReceivePrepareTime(time.Now())
	voteMsg, err := state.Prepare(prepareMsg, requestMsg)
	if err != nil {
		node.MsgError <- []error{err}
	}

	//Check VoteMsg created
	if voteMsg.SequenceID == 0 {
		return
	}
	node.BroadCastNextPrepareMsgIfPrimary(prepareMsg.SequenceID + 1)
	// Log last sequence id for checkpointing
	atomic.AddInt64(&node.Prepared[prepareMsg.SequenceID],1)
	// Start next sequence thread if does not exists
	node.StartThreadIfNotExists(prepareMsg.SequenceID + 1)

	// Attach node ID to the message and broadcast voteMsg..
	voteMsg.NodeID = node.MyInfo.NodeID
	node.Broadcast(voteMsg, "/vote")



	if prepareMsg.Seed != -1 {
		//log.Println("Prepare for next Epoch",prepareMsg.Seed)
		//node.setNewSeedList(prepareMsg.Seed)
	}
	// Stop prepare phase and start vote phase if it is not committed
	// fmt.Println("[Lock-resolve Collate Lock Try]")
	node.CommittedMutex.Lock()
	// fmt.Println("[Lock-resolve Collate Lock Release]")
	if node.Committed[prepareMsg.SequenceID] == 1 {
		node.CommittedMutex.Unlock()
		// Stop prepare phase and execute the sequence if it is committed
		state.GetTimerStopSendChannel() <- "Prepare"
		node.MsgExecution <- prepareMsg
	} else {
		node.CommittedMutex.Unlock()
		state.GetTimerStopSendChannel() <- "Prepare"
		state.GetTimerStartSendChannel() <- "Vote"
	}


}

func (node *Node) GetVote(state consensus.PBFT, voteMsg *consensus.VoteMsg) {
	fmt.Println("[GetVote] to",node.MyInfo.NodeID, "from",voteMsg.NodeID, "SeqID:", voteMsg.SequenceID,"MsgType : ",voteMsg.MsgType,"/", time.Now().UnixNano())
	// if voteMsg.SequenceID >= 1 && voteMsg.SequenceID <= 10 {
	// 	var PrepareMsg consensus.PrepareMsg
	// 	PrepareMsg.ViewID = 0
	// 	PrepareMsg.SequenceID = voteMsg.SequenceID
	// 	PrepareMsg.Digest = ""
	// 	PrepareMsg.EpochID = 0
	// 	PrepareMsg.NodeID = ""
	// 	PrepareMsg.Seed= 0
	// 	node.CommittedMsgs[voteMsg.SequenceID] = &PrepareMsg
	// }
	collateMsg, err := state.Vote(voteMsg, int64(len(node.NodeTable)))
	fmt.Println("Node VoteLength : ", len(state.GetVoteMsgs()))
	if err != nil {
		node.MsgError <- []error{err}
	}

	// Check COLLATE message created.
	if collateMsg.SequenceID == 0 {
		return
	}

	switch collateMsg.MsgType {


	// Stop vote phase and execute the sequence if it is committed
	case consensus.COMMITTED:
		state.GetTimerStopSendChannel() <- "Vote"
			// fmt.Println("[EXECUTECOMMIT] ","/",voteMsg.SequenceID,"/",time.Since(state.GetReceivePrepareTime()))
		
		if state.GetPrepareMsg() == nil {
			for _, vote := range collateMsg.ReceivedVoteMsg {
				node.MsgExecution <- vote.PrepareMsg
				log.Println("vote.PrepareMsg: ",vote.PrepareMsg)
				break
			}
		} else {
			node.MsgExecution <- state.GetPrepareMsg()
		}
		
		// atomic.AddInt64(&node.Committed[voteMsg.SequenceID], 1)
		collateMsg.NodeID = node.MyInfo.NodeID
		node.Broadcast(collateMsg, "/collate")		
		// Log last sequence id for checkpointing
	case consensus.UNCOMMITTED:
		state.GetTimerStartSendChannel() <- "Collate"
		collateMsg.NodeID = node.MyInfo.NodeID
		node.Broadcast(collateMsg, "/collate")		
	}

	// Attach node ID to the message

}
func (node *Node) GetCollate(state consensus.PBFT, collateMsg *consensus.CollateMsg) {
	fmt.Printf("[GetCollate] to %s from %s sequenceID: %d TYPE : %d \n", 
	 				node.MyInfo.NodeID, collateMsg.NodeID, collateMsg.SequenceID, collateMsg.MsgType)


	


	// Check COLLATE message created

	

	switch collateMsg.MsgType {
	// Stop vote phase and start collate phase if it is not committed
		case consensus.UNCOMMITTED:
			
			fmt.Println("1111111111111111")
			state.FillHoleVoteMsgs(collateMsg)
			fmt.Println("2222222222222222")
			newCollateMsg, err := state.Collate(collateMsg)
			fmt.Println("33333333333333333")
			atomic.AddInt64(&node.Committed[collateMsg.SequenceID], 1)
			fmt.Println("4444444444444444444")
			if err != nil {
				node.MsgError <- []error{err}
			}
			if newCollateMsg.SequenceID == 0 {
				return
			}
			
			switch newCollateMsg.MsgType {
				case consensus.UNCOMMITTED:
					fmt.Println("UNCOMMITTED newCollateMsg.MsgType : ", newCollateMsg.MsgType)
					newCollateMsg.NodeID = node.MyInfo.NodeID
					node.Broadcast(newCollateMsg, "/collate")
					// Try to stop current phase timer
					// state.GetTimerStopSendChannel() <- "Collate"

				case consensus.COMMITTED:
					fmt.Println("COMMITTED newCollateMsg.MsgType : ", newCollateMsg.MsgType)
					newCollateMsg.NodeID = node.MyInfo.NodeID
					node.Broadcast(newCollateMsg, "/collate")
					// Try to stop current phase timer
					

					// Log last sequence id for checkpointing
					node.PreparedMutex.Lock()
					if node.Prepared[collateMsg.SequenceID] == 1 {
						// fmt.Println("[EXECUTECOMMIT]","/",collateMsg.SequenceID,"/",time.Since(state.GetReceivePrepareTime()))
						node.PreparedMutex.Unlock()
						if node.Committed[collateMsg.SequenceID] == 0 {
							fmt.Println("========= Collate UNCOMMITED ==> Collate COMMITED ==============")
							node.MsgExecution <- state.GetPrepareMsg()
						}		

					}
					state.GetTimerStopSendChannel() <- "Collate"
					node.PreparedMutex.Unlock()				
			}
		// Stop vote phase and execute the sequence if it is committed
		case consensus.COMMITTED:
			state.FillHoleVoteMsgs(collateMsg)
			newCollateMsg, err := state.Collate(collateMsg)
			atomic.AddInt64(&node.Committed[collateMsg.SequenceID], 1)
			if err != nil {
				node.MsgError <- []error{err}
			}			
			// Attach node ID to the message and broadcast collateMsg..
			newCollateMsg.NodeID = node.MyInfo.NodeID
			node.Broadcast(newCollateMsg, "/collate")
			// Try to stop current phase timer
			state.GetTimerStopSendChannel() <- "Collate"

			// Log last sequence id for checkpointing
			node.PreparedMutex.Lock()
			if node.Prepared[collateMsg.SequenceID] == 1 {
				// fmt.Println("[EXECUTECOMMIT]","/",collateMsg.SequenceID,"/",time.Since(state.GetReceivePrepareTime()))
				node.PreparedMutex.Unlock()
				state.GetTimerStopSendChannel() <- "Vote"
				if node.Committed[collateMsg.SequenceID] == 0 {
					fmt.Println("========= Collate COMMITED ==============")
					node.MsgExecution <- state.GetPrepareMsg()
				}		

			}
			node.PreparedMutex.Unlock()

	}


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
func (node *Node) StartThreadIfNotExists(seqID int64) consensus.PBFT {
	node.StatesMutex.Lock()
	state := node.States[seqID]
	if state == nil {
		node.States[seqID] = node.createState(seqID)
		state = node.States[seqID]
		node.StatesMutex.Unlock()
		newTotalConsensus := atomic.AddInt64(&node.TotalConsensus, 1)
		fmt.Printf("Consensus Process.. newTotalConsensus num is %d\n", newTotalConsensus)	
		node.startTransitionWithDeadline(seqID, state)
		state.GetTimerStartSendChannel() <- "ViewChange"
		state.GetTimerStartSendChannel() <- "Prepare"
		//state.GetTimerStartSendChannel() <- "Total"
		
	}else {
		node.StatesMutex.Unlock()
	}
	return node.States[seqID]
}
func (node *Node) resolveMsg() {
	for {
		var state consensus.PBFT
		var err string = ""
		msgDelivered := <-node.MsgDelivery
		//fmt.Println("Message came in..")
		// Resolve the message.
		switch msg := msgDelivered.(type) {
		// Signature check is already done at proxyserver receiveloop..
		// Do we have to check bizantine check at receiveloop, too??
		//if node.Prepared[msg.PrepareMsg.SequenceID] == 1 &&
		// 			node.isBizantine(msg.PrepareMsg.NodeID) {
		case *consensus.ReqPrePareMsgs:
			//node.PreparedMutex.Lock()
			// if node.Prepared[msg.PrepareMsg.SequenceID] == 1{
			// 	node.PreparedMutex.Unlock()
			// 	continue
			// }
			//node.PreparedMutex.Unlock()
			//fmt.Println(msg.PrepareMsg.SequenceID,"came in!!")
			state = node.StartThreadIfNotExists(msg.PrepareMsg.SequenceID)
			state.GetMsgSendChannel() <- msg

		case *consensus.VoteMsg:
			// fmt.Println("[Lock-resolve Collate Lock Try]")
			node.CommittedMutex.Lock()
			// fmt.Println("[Lock-resolve Collate Lock Release]")
			if node.Committed[msg.SequenceID] == 1 {
				node.CommittedMutex.Unlock()
				continue
			}
			node.StatesMutex.Lock()
			state = node.States[msg.SequenceID]
			node.StatesMutex.Unlock()
			if state == nil && msg.SequenceID != 1 {
				state = node.StartThreadIfNotExists(msg.SequenceID)
				fmt.Println("StartThreadIfNotExists ",msg.SequenceID," make!!")
				state.GetMsgSendChannel() <- msg
			} else if state == nil && msg.SequenceID == 1 {
				//err = "Genesis message is not came in.."
			} else if state != nil {
				state.GetMsgSendChannel() <- msg
			}
			node.CommittedMutex.Unlock()
		case *consensus.CollateMsg:
			// fmt.Println("[Lock-resolve Collate Lock Try]")
			node.CommittedMutex.Lock()
			// fmt.Println("[Lock-resolve Collate Lock Release]")
			// fmt.Println("node.Committed[msg.SequenceID] ", node.Committed[msg.SequenceID]," from ",msg.NodeID)
			if node.Committed[msg.SequenceID] == 1 {
				node.CommittedMutex.Unlock()
			 	continue
			}
			node.StatesMutex.Lock()
			state = node.States[msg.SequenceID]
			node.StatesMutex.Unlock()
			if state == nil && msg.SequenceID != 1 {
				state = node.StartThreadIfNotExists(msg.SequenceID)
				state.GetMsgSendChannel() <- msg
			} else if state == nil && msg.SequenceID == 1 {
				//err = "Genesis message is not came in.."
			} else if state != nil {
				// fmt.Println("Collate Msg!!!!!", msg.SequenceID," /",msg.ReceivedVoteMsg," from",msg.NodeID)
				state.GetMsgSendChannel() <- msg
			}
			node.CommittedMutex.Unlock()

		//case *consensus.CheckPointMsg:
		//	node.GetCheckPoint(msg)
		case *consensus.ViewChangeMsg:
			state = node.StartThreadIfNotExists(msg.SequenceID)
			state.GetMsgSendChannel() <- msg

			//node.GetViewChange(msg)
		case *consensus.NewViewMsg:
			state = node.StartThreadIfNotExists(msg.SequenceID)
			state.GetMsgSendChannel() <- msg

			//node.GetNewView(msg)
		}
		if err != "" {
			// Print error.
			//node.MsgError <- []error{err}
			// Send message into dispatcher.
			//fmt.Println(err)
			node.MsgDelivery <- msgDelivered
			time.Sleep(time.Millisecond * 50)
		}
		//runtime.Gosched()
	}
}
func (node *Node) executeMsg() {
	pairs := make(map[int64]*consensus.PrepareMsg)
	for {
		prepareMsg := <- node.MsgExecution
		fmt.Println("prepareMsg : ", prepareMsg)
		pairs[prepareMsg.SequenceID] = prepareMsg
		fmt.Println("[CommitMsg]",prepareMsg.SequenceID,"/",time.Now().UnixNano())
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
			// Stop execution if the message for the
			// current sequence is not ready to execute.
			p := pairs[lastSequenceID + 1]
			
			if p == nil {
				//fmt.Println("[STAGE-DONE11] Commit SequenceID : ", int64(len(node.CommittedMsgs)))
				break
			}

			node.States[prepareMsg.SequenceID].GetTimerStopSendChannel() <- "ViewChange"

			fmt.Println("[Execute] /", lastSequenceID + 1,"/", time.Now().UnixNano())
			// Add the committed message in a private log queue
			// to print the orderly executed messages.
			node.CommittedMsgs[int64(lastSequenceID + 1)] = prepareMsg
			atomic.AddInt64(&node.Committed[int64(lastSequenceID + 1)], 1)
			//fmt.Println("[STAGE-DONE] Commit SequenceID : ",lastSequenceID + 1)
			node.StableCheckPoint = lastSequenceID + 1
			node.StatesMutex.Lock()
			// ch := node.States[node.StableCheckPoint].GetMsgExitSendChannel()
			// ch1 := node.States[node.StableCheckPoint].GetMsgExitSendChannel1()
			// ch <- 0
			// ch1 <- 0

			node.StatesMutex.Unlock()
			// TODO: execute appropriate operation.

			delete(pairs, lastSequenceID + 1)
			// fmt.Println("[Execute] sequenceID:",lastSequenceID + 1,",",time.Now().UnixNano())
			// // Add the committed message in a private log queue
			// // to print the orderly executed messages.
			// node.CommittedMsgs[int64(lastSequenceID + 1)] = prepareMsg
			// LogStage("Commit", true)

			node.StableCheckPoint = lastSequenceID + 1
			node.updateViewID(node.StableCheckPoint)
			node.updateEpochID(node.StableCheckPoint)
			if node.StableCheckPoint % 10 == 0 {
				//ode.VCStates = make(map[int64]*consensus.VCState)
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
			errCh := make(chan error, 10)

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

	// if state == nil {
	// 	return nil, fmt.Errorf("State for sequence number %d has not created yet.", sequenceID)
	// }

	return state, nil
}
