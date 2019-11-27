package consensus

import (
	"errors"
	"fmt"
	// "log"
	"sync"
	"sync/atomic"
	"time"
)
type State struct {
	ViewID          int64
	NodeID          string
	MsgLogs         *MsgLogs
	SequenceID      int64

	MsgState 	chan interface{}
	MsgExit		chan int64
	TimerStartCh	chan string
	TimerStopCh		chan string

	// f: the number of Byzantine faulty nodes
	// f = (n-1) / 3
	// e.g., n = 5, f = 1

	F int
	B int
	BNode map[string]int

	ReceivedPrepareTime time.Time
}

type MsgLogs struct {
	ReqMsg        *RequestMsg
	Digest		  string

	PrepareMsg    *PrepareMsg
	SentVoteMsg	  *VoteMsg
	VoteMsgs       map[string]*VoteMsg
	CollateMsgs    map[string]*CollateMsg

	//PrepareMsgsMutex sync.RWMutex
	//CommitMsgsMutex  sync.RWMutex
	VoteMsgsMutex sync.RWMutex
	CollateMsgsMutex sync.RWMutex
	BNodeMutex sync.RWMutex

	// Count PREPARE message created from the current node
	// as one PREPARE message. PRE-PREPARE message from
	// primary node is also regarded as PREPARE message but
	// do not count it, because it is not real PREPARE message.
	TotalVoteMsg int32
	TotalVoteOKMsg int32
	TotalCollateMsg int32
}

func CreateState(viewID int64, nodeID string, totNodes int,  seqID int64) *State {
	state := &State{
		ViewID: viewID,
		NodeID: nodeID,
		MsgLogs: &MsgLogs{
			ReqMsg:        nil,
			PrepareMsg:    nil,
			SentVoteMsg:   nil,
			VoteMsgs:	   make(map[string]*VoteMsg),
			CollateMsgs:   make(map[string]*CollateMsg),

			// Setting these counters during consensus is unsafe
			// because quorum condition check can be skipped.
			TotalVoteMsg:		0,
			TotalVoteOKMsg:    0,			
			TotalCollateMsg:    0,
		},
		SequenceID: seqID,

		MsgState: make(chan interface{}, totNodes * 100), // stack enough
		MsgExit: make(chan int64, totNodes * 100),
		TimerStartCh: make(chan string, totNodes * 100),
		TimerStopCh: make(chan string, totNodes * 100),

		F: (totNodes-1) / 3,
		B: 0,
		//succChkPointDelete: 0,
	}
	return state
}

func (state *State) Prepare(prepareMsg *PrepareMsg, requestMsg *RequestMsg) (VoteMsg, error) {
	var voteMsg VoteMsg
	// case1: Making NULL Msg
	if requestMsg == nil {
		voteMsg = VoteMsg{
			ViewID: state.ViewID,
			Digest: "NULL",
			PrepareMsg: prepareMsg,
			NodeID: "",
			SequenceID: state.SequenceID, 	//This sequence number is already known..
			MsgType: NULLMSG,
		}
		return voteMsg, nil
	}

	// case2: Making VoteMsg
	digest, err := Digest(state.MsgLogs.ReqMsg)
	if err != nil {
		return voteMsg, err
	}
	state.MsgLogs.Digest = digest
	state.MsgLogs.ReqMsg = requestMsg
	state.MsgLogs.PrepareMsg = prepareMsg

	state.SequenceID = prepareMsg.SequenceID
	voteMsg = VoteMsg{
		ViewID: state.ViewID,
		Digest: state.MsgLogs.Digest,
		PrepareMsg: prepareMsg,
		NodeID: "",
		SequenceID: state.SequenceID,
		MsgType: VOTE,
	}	
	state.MsgLogs.SentVoteMsg = &voteMsg

	// Verify if v, n(a.k.a. sequenceID), d are correct.
	if err := state.verifyMsg(prepareMsg.ViewID, prepareMsg.SequenceID, prepareMsg.Digest); err != nil {
		state.SetBizantine(prepareMsg.NodeID)
		voteMsg.MsgType = REJECT
		//return nil, errors.New("pre-prepare message is corrupted: " + err.Error() + " (operation: " + prePrepareMsg.RequestMsg.Operation + ")")
	}
	return voteMsg, nil
}
func (state *State) Vote(voteMsg *VoteMsg) (CollateMsg, error){
	var collateMsg CollateMsg
	var newTotalVoteOKMsg int32
	// case1: Making UNCOMMITTED Msg
	if voteMsg == nil {
		collateMsg = CollateMsg{
	   		//ReceivedPrepare:	state.MsgLogs.PrepareMsg,
	   		ReceivedVoteMsg:	state.MsgLogs.VoteMsgs,
	   		SentVoteMsg:    	state.MsgLogs.SentVoteMsg,
	   		ViewID:				state.ViewID,
	   		Digest:				state.MsgLogs.Digest,
	   		NodeID:				"",
	   		SequenceID:			state.SequenceID,
	   		MsgType:			UNCOMMITTED,
		}
		return collateMsg, nil
	}

	// case2: Making COMMITTED Msg

	// Append msg to its logs
	state.MsgLogs.VoteMsgsMutex.Lock()
	if _, ok := state.MsgLogs.VoteMsgs[voteMsg.NodeID]; ok {
		state.SetBizantine(voteMsg.NodeID)
		fmt.Printf("Vote message from %s is already received, sequence number=%d\n",
		           voteMsg.NodeID, state.SequenceID)
		state.MsgLogs.VoteMsgsMutex.Unlock()
		return collateMsg, nil
	}
	state.MsgLogs.VoteMsgs[voteMsg.NodeID] = voteMsg
	state.MsgLogs.VoteMsgsMutex.Unlock()
	atomic.AddInt32(&state.MsgLogs.TotalVoteMsg, 1)
	if voteMsg.MsgType == VOTE {
		newTotalVoteOKMsg = atomic.AddInt32(&state.MsgLogs.TotalVoteOKMsg, 1)
		
	}
	
	// Verify Message
	if err := state.verifyMsg(voteMsg.ViewID, voteMsg.SequenceID, voteMsg.Digest); err != nil {
		state.SetBizantine(voteMsg.NodeID)
		return collateMsg, errors.New("vote message is corrupted: " + err.Error() + " (nodeID: " + voteMsg.NodeID + ")")
	}
	// If Committed, make CollateMsg
	if int(newTotalVoteOKMsg) == 2*state.F + 1 {
	   	collateMsg := CollateMsg{
	   		//ReceivedPrepare: 	state.MsgLogs.PrepareMsg,
	   		ReceivedVoteMsg:	state.MsgLogs.VoteMsgs,
	   		SentVoteMsg:        state.MsgLogs.SentVoteMsg,
	   		ViewID:		state.ViewID,
	   		Digest:		state.MsgLogs.Digest,
	   		NodeID:		"",
	   		SequenceID:	state.SequenceID,
	   		MsgType:	COMMITTED,
	   	}
		return collateMsg, nil
	}

	return collateMsg, nil
}
func (state *State) VoteAQ(TotalNode int32) (CollateMsg, error){

	newTotalVoteOKMsg := state.MsgLogs.TotalVoteOKMsg
	newTotalVoteMsg := state.MsgLogs.TotalVoteMsg
	// byzantine length
	byzantine := TotalNode - newTotalVoteMsg
	fmt.Println("TotalNode :",TotalNode, "newTotalVoteMsg :",newTotalVoteMsg, "byzantine length :", byzantine, "state.SequenceID :", state.SequenceID)
	collateMsg := CollateMsg{
		//ReceivedPrepare: 	state.MsgLogs.PrepareMsg,
		ReceivedVoteMsg:	state.MsgLogs.VoteMsgs,
		SentVoteMsg:        state.MsgLogs.SentVoteMsg,
		ViewID:		state.ViewID,
		Digest:		state.MsgLogs.Digest,
		NodeID:		"",
		SequenceID:	state.SequenceID,
		MsgType:	0,
	}	
	// If Committed, make CollateMsg
	if int(newTotalVoteOKMsg) >= (2*state.F - int(byzantine) + 1) && (2*state.F - int(byzantine) + 1) >= 1{
		collateMsg.MsgType = COMMITTED

	}else {
		collateMsg.MsgType = UNCOMMITTED	
	}

	return collateMsg, nil
}
func (state *State) Collate(collateMsg *CollateMsg) (CollateMsg, error) {
	var newcollateMsg CollateMsg

	// Append msg to its logs
	state.MsgLogs.CollateMsgsMutex.Lock()
	if _, ok := state.MsgLogs.CollateMsgs[collateMsg.NodeID]; ok {
		state.SetBizantine(collateMsg.NodeID)
		fmt.Printf("Commit message from %s is already received, sequence number=%d\n",
		           collateMsg.NodeID, state.SequenceID)
		state.MsgLogs.CollateMsgsMutex.Unlock()
		return newcollateMsg,nil
	}
	state.MsgLogs.CollateMsgs[collateMsg.NodeID] = collateMsg
	state.MsgLogs.CollateMsgsMutex.Unlock()

	// Verify Message
	if err := state.verifyMsg(collateMsg.ViewID, collateMsg.SequenceID, collateMsg.Digest); err != nil {
		state.SetBizantine(collateMsg.NodeID)
		return newcollateMsg, errors.New("collate message is corrupted: " + err.Error() + " (nodeID: " + collateMsg.NodeID + ")")
	}

	// Append VoteMsgs of CollateMsg to My VoteMsgs
	switch collateMsg.MsgType {
	case COMMITTED:
		for NodeID, VoteMsg := range state.GetVoteMsgs() {
			if  VoteMsg == collateMsg.ReceivedVoteMsg[NodeID] {
				continue
			}
			state.GetVoteMsgs()[NodeID] = collateMsg.ReceivedVoteMsg[NodeID]
		}
	// If Committed, make CollateMsg
		if int(state.MsgLogs.TotalVoteMsg) >= 2*state.F + 1 {
			return CollateMsg{
				//ReceivedPrepare: 	state.MsgLogs.PrepareMsg,
				ReceivedVoteMsg:	state.MsgLogs.VoteMsgs,
				SentVoteMsg:        state.MsgLogs.SentVoteMsg,
				ViewID:		state.ViewID,
				Digest:		state.MsgLogs.Digest,
				NodeID:		"",
				SequenceID:	state.SequenceID,
				MsgType:	COMMITTED,
			}, nil
		}
	case UNCOMMITTED:
		// TODO implement this
	}
	return newcollateMsg, nil
}
func (state *State) SetBizantine(nodeID string) bool {
	/*
	state.MsgLogs.BNodeMutex.Lock()
	if _, ok := state.BNode[nodeID]; !ok {
		state.BNode[nodeID] = 1
		state.B += 1
		state.MsgLogs.BNodeMutex.Unlock()
		return true
	} else {
		state.MsgLogs.BNodeMutex.Unlock()
		return false
	}
	*/
	return false

}
func (state *State) GetSequenceID() int64 {
	return state.SequenceID
}
func (state *State) GetF() int {
	return state.F
}
func (state *State) GetMsgReceiveChannel() <-chan interface{} {
	return state.MsgState
}
func (state *State) GetMsgSendChannel() chan<- interface{} {
	return state.MsgState
}
func (state *State) GetMsgExitReceiveChannel() <-chan int64 {
	return state.MsgExit
}
func (state *State) GetMsgExitSendChannel() chan<- int64 {
	return state.MsgExit
}
func (state *State) GetTimerStartReceiveChannel() <-chan string {
	return state.TimerStartCh
}
func (state *State) GetTimerStartSendChannel() chan<- string {
	return state.TimerStartCh
}
func (state *State) GetTimerStopReceiveChannel() <-chan string {
	return state.TimerStopCh
}
func (state *State) GetTimerStopSendChannel() chan<- string {
	return state.TimerStopCh
}
func (state *State) GetReceivePrepareTime() time.Time {
	return state.ReceivedPrepareTime
}
func (state *State) GetReqMsg() *RequestMsg {
	//return state.MsgLogs.ReqMsg
	return state.MsgLogs.ReqMsg
}
func (state *State) GetPrepareMsg() *PrepareMsg {
	return state.MsgLogs.PrepareMsg
}
func (state *State) GetVoteMsgs() map[string]*VoteMsg{
	newMap := make(map[string]*VoteMsg)

	state.MsgLogs.VoteMsgsMutex.RLock()
	for k, v := range state.MsgLogs.VoteMsgs {
		newMap[k] = v
	}
	state.MsgLogs.VoteMsgsMutex.RUnlock()

	return newMap
}
func (state *State) GetCollateMsgs() map[string]*CollateMsg {
	newMap := make(map[string]*CollateMsg)

	state.MsgLogs.CollateMsgsMutex.RLock()
	for k, v := range state.MsgLogs.CollateMsgs {
		newMap[k] = v
	}
	state.MsgLogs.CollateMsgsMutex.RUnlock()

	return newMap
}
func (state *State) FillHoleVoteMsgs(collateMsg *CollateMsg) {

	for NodeID, VoteMsg := range state.GetVoteMsgs() {
			if  VoteMsg == collateMsg.ReceivedVoteMsg[NodeID] {
				continue
			}
			state.GetVoteMsgs()[NodeID] = collateMsg.ReceivedVoteMsg[NodeID]
	}
}

func (state *State) verifyMsg(viewID int64, sequenceID int64, digestGot string) error {
	// Wrong view. That is, wrong configurations of peers to start the consensus.
	if state.ViewID != viewID {
		return fmt.Errorf("verifyMsg ERROR state.ViewID = %d, viewID = %d", state.ViewID, viewID)
	}

	if state.SequenceID != sequenceID {
		return fmt.Errorf("verifyMsg ERROR state.SequenceID = %d, sequenceID = %d", state.SequenceID, sequenceID)
	}

	digest := state.MsgLogs.Digest

	// Check digest.
	if digestGot != digest {
		//fmt.Printf("???\n")
		//return fmt.Errorf("digest = %s, digestGot = %s", digest, digestGot)
	}

	return nil
}

// From TOCS: Each replica collects messages until it has a quorum certificate
// with the PRE-PREPARE and 2*f matching PREPARE messages for sequence number n,
// view v, and request m. We call this certificate the prepared certificate
// and we say that the replica "prepared" the request.
func (state *State) prepared() bool {
	// Condition is that previous sequence thread has received prepare message...
	// I think this can be proved by checking if the sequence thread is exists...
	// So just pass!

	//if state.MsgLogs.ReqMsg == nil || state.MsgLogs.PrePrepareMsg == nil {
	//if state.MsgLogs.PrepareMsg == nil {
	//	return false
	//}

	//if int(atomic.LoadInt32(&state.MsgLogs.TotalPrepareMsg)) < 2*state.F {
	//if int(atomic.LoadInt32(&state.MsgLogs.TotalVoteMsg)) <= state.F - state.B {
	//	return false
	//}

	return true
}

// From TOCS: Each replica collects messages until it has a quorum certificate
// with 2*f+1 COMMIT messages for the same sequence number n and view v
// from different replicas (including itself). We call this certificate
// the committed certificate and say that the request is "committed"
// by the replica when it has both the prepared and committed certificates.
func (state *State) committed() bool {
	if !state.prepared() {
		return false
	}

	//if int(atomic.LoadInt32(&state.MsgLogs.TotalCommitMsg)) < 2*state.F + 1 {
	if int(atomic.LoadInt32(&state.MsgLogs.TotalCollateMsg)) <= 2*state.F + 1 {
		return false
	}

	return true
}

func (state *State) SetReqMsg(request *RequestMsg) {
	state.MsgLogs.ReqMsg = request
}

func (state *State) SetPrepareMsg(prepareMsg *PrepareMsg) {
	state.MsgLogs.PrepareMsg = prepareMsg
}

func (state *State) SetSequenceID(sequenceID int64) {
	state.SequenceID = sequenceID
}

func (state *State) SetDigest(digest string) {
	state.MsgLogs.Digest = digest
}

func (state *State) SetViewID(viewID int64) {
	state.ViewID = viewID
}

func (state *State) SetReceivePrepareTime(time time.Time) {
	state.ReceivedPrepareTime = time
}
