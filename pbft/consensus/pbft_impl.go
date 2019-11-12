package consensus

import (
	"errors"
	"fmt"
	"time"
	//"log"
	"sync"
	"sync/atomic"
)
const prepareSigma, voteSigma, collateSigma, vcSigma time.Duration = 3000, 3000, 3000, 10000
type State struct {
	ViewID          int64
	NodeID          string
	MsgLogs         *MsgLogs
	SequenceID      int64

	MsgState chan interface{}
	MsgExit		chan int64

	// f: the number of Byzantine faulty nodes
	// f = (n-1) / 3
	// e.g., n = 5, f = 1

	/* Adaptive BFT */
	// f: the number of Byzantine faulty nodes
	// AQ = f - b + floor(h+1/2)
	// e.g., n = 5, f = 2, h = 3, b=0 AQ=2-0+floor(3+1/2) = 4
	F int
	B int
	BNode map[string]int

	prepareTimer	*time.Timer
	voteTimer		*time.Timer
	collateTimer 	*time.Timer
	viewchangeTimer	*time.Timer
	prepareCanceled		chan struct {}
	voteCanceled		chan struct {}
	collateCanceled		chan struct {}
	viewchangeCanceled	chan struct {}

}

type MsgLogs struct {
	ReqMsg        *RequestMsg
	Digest 			string
	//PrePrepareMsg *PrePrepareMsg
	//PrepareMsgs   map[string]*VoteMsg
	//CommitMsgs    map[string]*VoteMsg
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
	//TotalPrepareMsg  int32 // atomic
	//TotalCommitMsg   int32 // atomic
	TotalVoteMsg int32
	TotalCollateMsg int32

	// Flags whether each message has broadcasted.
	// Its value is atomically swapped by CompareAndSwapInt32().
	commitMsgSent   int32 // atomic bool
	replyMsgSent    int32 // atomic bool
}

func CreateState(viewID int64, nodeID string, totNodes int,  seqID int64) *State {
	state := &State{
		ViewID: viewID,
		NodeID: nodeID,
		MsgLogs: &MsgLogs{
			ReqMsg:        nil,
			//PrePrepareMsg: nil,
			//PrepareMsgs:   make(map[string]*VoteMsg),
			//CommitMsgs:    make(map[string]*VoteMsg),
			PrepareMsg:    nil,
			SentVoteMsg: 	nil,
			VoteMsgs:	   make(map[string]*VoteMsg),
			CollateMsgs:   make(map[string]*CollateMsg),

			// Setting these counters during consensus is unsafe
			// because quorum condition check can be skipped.
			// TotalPrepareMsg: 0,
			// TotalCommitMsg: 0,
			TotalVoteMsg:		0,
			TotalCollateMsg:    0,

			commitMsgSent: 0,
			replyMsgSent: 0,
		},
		SequenceID: seqID,

		MsgState: make(chan interface{}, totNodes), // stack enough
		MsgExit: make(chan int64),

		//F: (totNodes - 1) / 3,
		F: (totNodes-1) / 3,
		B: 0,
		//succChkPointDelete: 0,
	}

	return state
}
/*
func (state *State) StartConsensus(request *RequestMsg, sequenceID int64) (*PrepareMsg, error) {
	// From TOCS: The primary picks the "ordering" for execution of
	// operations requested by clients. It does this by assigning
	// the next available `sequence number` to a request and sending
	// this assignment to the backups.
	state.SequenceID = sequenceID
	request.SequenceID = sequenceID

	// TODO: From TOCS: no sequence numbers are skipped but
	// when there are view changes some sequence numbers
	// may be assigned to null requests whose execution is a no-op.

	// Log REQUEST message.
	state.MsgLogs.ReqMsg = request

	// Get the digest of the request message
	digest, err := Digest(request)
	if err != nil {
		return nil, err
	}
	state.MsgLogs.Digest = digest
	
	// Create PREPREPARE message.
	//prePrepareMsg := &PrePrepareMsg{
	//	ViewID:     state.ViewID,
	//	SequenceID: request.SequenceID,
	//	Digest:     digest,
	//}
	
	// Create PREPARE message.
	prepareMsg := &PrepareMsg{
		//EpochID: state.EpochID
		ViewID: state.ViewID,
		SequenceID: request.SequenceID,
		Digest: digest,
		NodeID: "",
		Signature: "",
	}
	// Accessing to the message log without locking is safe because
	// nobody except for this node starts consensus at this point,
	// i.e., the node has not registered this state yet.
	//state.MsgLogs.PrePrepareMsg = prePrepareMsg
	state.MsgLogs.PrepareMsg = prepareMsg

	//return prePrepareMsg, nil
	return prepareMsg, nil
}
*/
func (state *State) Prepare(prepareMsg *PrepareMsg, requestMsg *RequestMsg) (VoteMsg, error) {
	var voteMsg VoteMsg
	// case1: Prepare Timer expires.. sending NULLMSG
	if prepareMsg == nil {	
		voteMsg = VoteMsg{
			ViewID: state.ViewID,
			Digest: state.MsgLogs.Digest,
			//PrepareMsg: prepareMsg,
			NodeID: "",
			SequenceID: state.SequenceID, 				//This sequence number is already known..
			MsgType: NULLMSG,
		}
		return voteMsg, nil
	}
	digest, err := Digest(state.MsgLogs.ReqMsg)
	if err != nil {
		return voteMsg, err
	}
	state.MsgLogs.Digest = digest
	state.MsgLogs.ReqMsg = requestMsg
	// case2: prepareMsg which arrived in right time
	// Log PREPARE message.
	state.MsgLogs.PrepareMsg = prepareMsg
	// From TOCS: The primary picks the "ordering" for execution of
	// operations requested by clients. It does this by assigning
	// the next available `sequence number` to a request and sending
	// this assignment to the backups.
	state.SequenceID = prepareMsg.SequenceID


	voteMsg = VoteMsg{
		ViewID: state.ViewID,
		Digest: state.MsgLogs.Digest,
		//PrepareMsg: prepareMsg,
		NodeID: "",
		SequenceID: state.SequenceID,
		MsgType: VOTE,
	}	
	state.MsgLogs.SentVoteMsg = &voteMsg

	// Verify if v, n(a.k.a. sequenceID), d are correct.
	if err := state.verifyMsg(prepareMsg.ViewID, prepareMsg.SequenceID, prepareMsg.Digest); err != nil {
		state.SetBizantine(prepareMsg.NodeID)
		//voteMsg.MsgType = REJECT 				
		//return nil, errors.New("pre-prepare message is corrupted: " + err.Error() + " (operation: " + prePrepareMsg.RequestMsg.Operation + ")")
	}
	return voteMsg, nil
}

func (state *State) Vote(voteMsg *VoteMsg) (CollateMsg, error){
	var collateMsg CollateMsg
	// case1: Vote Timer expires.. sending UNCOMMITTED
	if voteMsg == nil {		//Timeout.. sending null-msg
		collateMsg = CollateMsg{
	   		//ReceivedPrepare:	state.MsgLogs.PrepareMsg,
	   		ReceivedVoteMsg:	make(map[string]*VoteMsg),
	   		SentVoteMsg:    	state.MsgLogs.SentVoteMsg,
	   		ViewID:				state.ViewID,
	   		Digest:				state.MsgLogs.Digest,
	   		NodeID:				"",
	   		SequenceID:			state.SequenceID,
	   		MsgType:			UNCOMMITTED,
		}
		collateMsg.ReceivedVoteMsg = state.MsgLogs.VoteMsgs
		return collateMsg, nil
	}

	// case2: voteMsg which arrived in right time
	if err := state.verifyMsg(voteMsg.ViewID, voteMsg.SequenceID, voteMsg.Digest); err != nil {
		state.SetBizantine(voteMsg.NodeID)
		//return nil, errors.New("vote message is corrupted: " + err.Error() + " (nodeID: " + voteMsg.NodeID + ")")
	}
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
	newTotalVoteMsg := atomic.AddInt32(&state.MsgLogs.TotalVoteMsg, 1)

	// Return commit message only once.
	//if int(newTotalVoteMsg) >= 2*state.F && state.prepared() &&
	if int(newTotalVoteMsg) == 2*state.F + 1 && state.prepared() &&
	    atomic.CompareAndSwapInt32(&state.MsgLogs.commitMsgSent, 0, 1) {
	   	// Create COLLATE message.
	   	collateMsg := CollateMsg{
	   		//ReceivedPrepare: 	state.MsgLogs.PrepareMsg,
	   		ReceivedVoteMsg:	make(map[string]*VoteMsg),
	   		SentVoteMsg:        state.MsgLogs.SentVoteMsg,
	   		ViewID:		state.ViewID,
	   		Digest:		state.MsgLogs.Digest,
	   		NodeID:		"",
	   		SequenceID:	state.SequenceID,
	   		MsgType:	COMMITTED,
	   	}
		collateMsg.ReceivedVoteMsg = state.MsgLogs.VoteMsgs
		return collateMsg, nil
	}

	return collateMsg, nil
}

func (state *State) Collate(collateMsg *CollateMsg) (CollateMsg, bool, error) {
	var newcollateMsg CollateMsg
	if err := state.verifyMsg(collateMsg.ViewID, collateMsg.SequenceID, collateMsg.Digest); err != nil {
		state.SetBizantine(collateMsg.NodeID)
		//return nil, nil, errors.New("commit message is corrupted: " + err.Error() + " (nodeID: " + commitMsg.NodeID + ")")
		return newcollateMsg, false, errors.New("collate message is corrupted: " + err.Error() + " (nodeID: " + collateMsg.NodeID + ")")
	}

	// Append msg to its logs
	state.MsgLogs.CollateMsgsMutex.Lock()
	if _, ok := state.MsgLogs.CollateMsgs[collateMsg.NodeID]; ok {
		state.SetBizantine(collateMsg.NodeID)
		fmt.Printf("Commit message from %s is already received, sequence number=%d\n",
		           collateMsg.NodeID, state.SequenceID)
		state.MsgLogs.CollateMsgsMutex.Unlock()
		return newcollateMsg, false,nil
	}

	state.MsgLogs.CollateMsgs[collateMsg.NodeID] = collateMsg
	state.MsgLogs.CollateMsgsMutex.Unlock()
	newTotalCollateMsg := atomic.AddInt32(&state.MsgLogs.TotalCollateMsg, 1)

	// Print current voting status
	fmt.Printf("[Collate-Vote]: %d, from %s, sequence number: %d\n",
	           newTotalCollateMsg, collateMsg.NodeID, collateMsg.SequenceID)

	commitFlag:=false;
	switch collateMsg.MsgType {
	case COMMITTED:		// Received Committed Msg from Other.. Reply the Msg
		commitFlag = true;
	case UNCOMMITTED:
		state.Collating()
		if int(state.MsgLogs.TotalVoteMsg) == 2*state.F - state.B + 1 {
			commitFlag = true;
		}
	}
	if commitFlag == true {
		return CollateMsg{
	   		//ReceivedPrepare: 	state.MsgLogs.PrepareMsg,
	   		ReceivedVoteMsg:	state.MsgLogs.VoteMsgs,
	   		SentVoteMsg:        state.MsgLogs.SentVoteMsg,
	   		ViewID:		state.ViewID,
	   		Digest:		state.MsgLogs.Digest,
	   		NodeID:		"",
	   		SequenceID:	state.SequenceID,
	   		MsgType:	COMMITTED,
		}, state.collateTimer == nil, nil
	} else {
		return newcollateMsg, false, nil
	}
}
func (state *State) Commit()(*ReplyMsg, *PrepareMsg) {
	return &ReplyMsg{
		ViewID:    state.ViewID,
		//Timestamp: state.MsgLogs.ReqMsg.Timestamp,
		//ClientID:  state.MsgLogs.ReqMsg.ClientID,
		Timestamp: 0,
		ClientID: "",
		// Nodes must execute the requested operation
		// locally and assign the result into reply message,
		// with considering their operation ordering policy.
		Result: "EXCUTE",
	}, state.MsgLogs.PrepareMsg
}
func (State *State) Collating(){

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
/*
func (state *State) GetPrepareMsgs() map[string]*VoteMsg {
	newMap := make(map[string]*VoteMsg)

	state.MsgLogs.PrepareMsgsMutex.RLock()
	for k, v := range state.MsgLogs.PrepareMsgs {
		newMap[k] = v
	}
	state.MsgLogs.PrepareMsgsMutex.RUnlock()

	return newMap
}
*/
func (state *State) GetCollateMsgs() map[string]*CollateMsg {
	newMap := make(map[string]*CollateMsg)

	state.MsgLogs.CollateMsgsMutex.RLock()
	for k, v := range state.MsgLogs.CollateMsgs {
		newMap[k] = v
	}
	state.MsgLogs.CollateMsgsMutex.RUnlock()

	return newMap
}
/*
func (state *State) GetCommitMsgs() map[string]*VoteMsg {
	newMap := make(map[string]*VoteMsg)

	state.MsgLogs.CommitMsgsMutex.RLock()
	for k, v := range state.MsgLogs.CommitMsgs {
		newMap[k] = v
	}
	state.MsgLogs.CommitMsgsMutex.RUnlock()

	return newMap
}
*/
func (state *State) SetTimer(phase string) {
	switch phase {
	case "Prepare":
		state.prepareTimer = time.NewTimer(time.Millisecond * prepareSigma)
		state.prepareCanceled = make(chan struct {})
	case "Vote":
		state.voteTimer = time.NewTimer(time.Millisecond * voteSigma)
		state.voteCanceled = make(chan struct {})
	case "Collate":
		state.collateTimer = time.NewTimer(time.Millisecond * collateSigma)
		state.collateCanceled = make(chan struct {})
	case "ViewChange":
		state.viewchangeTimer = time.NewTimer(time.Millisecond * vcSigma)
		state.viewchangeCanceled = make(chan struct {})
	}
}
func (state *State) GetPhaseTimer(phase string) (*time.Timer){
	switch phase {
	case "Prepare":
		return state.prepareTimer
	case "Vote":
		return state.voteTimer
	case "Collate":
		return state.collateTimer
	case "ViewChange":
		return state.viewchangeTimer
	}
	return nil
}
func (state *State) GetCancelTimerCh(phase string) (chan struct {}){
	switch phase {
	case "Prepare":
		return state.prepareCanceled
	case "Vote":
		return state.voteCanceled
	case "Collate":
		return state.collateCanceled
	case "ViewChange":
		return state.viewchangeCanceled
	}
	return nil
}

func (state *State) verifyMsg(viewID int64, sequenceID int64, digestGot string) error {
	// Wrong view. That is, wrong configurations of peers to start the consensus.
	if state.ViewID != viewID {
		return fmt.Errorf("verifyMsg ERROR state.ViewID = %d, viewID = %d", state.ViewID, viewID)
	}

	if state.SequenceID != sequenceID {
		return fmt.Errorf("verifyMsg ERROR state.SequenceID = %d, sequenceID = %d", state.SequenceID, sequenceID)
	}

	//digest, err := Digest(state.MsgLogs.ReqMsg)
	//if err != nil {
	//	return err
	//}
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
