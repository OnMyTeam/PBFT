package consensus

import (
	"time"
)

type PBFT interface {
	/*
	StartConsensus(request *RequestMsg, sequenceID int64) (*PrePrepareMsg, error)
	PrePrepare(prePrepareMsg *PrePrepareMsg) (*VoteMsg, error)
	Prepare(prepareMsg *VoteMsg) (*VoteMsg, error)
	Commit(commitMsg *VoteMsg) (*ReplyMsg, *RequestMsg, error)
	*/
	//StartConsensus(request *RequestMsg, sequenceID int64) (*PrepareMsg, error)
	Prepare(prepareMsg *PrepareMsg) (*VoteMsg, error)
	Vote(voteMsg *VoteMsg) (*CollateMsg, error)
	Collate(collateMsg *CollateMsg) (*CollateMsg, bool, error)

	Commit() (*ReplyMsg, *PrepareMsg)
	Collating()
	SetBizantine(nodeID string) bool
	GetSequenceID() int64
	GetF() int

	GetMsgReceiveChannel() <-chan interface{}
	GetMsgSendChannel() chan<- interface{}
	GetMsgExitReceiveChannel() <-chan int64
	GetMsgExitSendChannel() chan<- int64

	GetReqMsg() *RequestMsg
	//GetPrePrepareMsg() 
	GetPrepareMsg() *PrepareMsg
	GetVoteMsgs() map[string]*VoteMsg
	GetCollateMsgs() map[string]*CollateMsg
	
	GetPhaseTimer(phase string) (*time.Timer)
	GetCancelTimerCh(phase string) (chan struct {})
	SetTimer(phase string)

	//SetSuccChkPoint(int64)
	SetSequenceID(sequenceID int64)
	SetDigest(digest string)
	SetViewID(viewID int64)

	//setrequ
	ClearMsgLogs()
	Redo_SetState(viewID int64, nodeID string, totNodes int, prepareMsg *PrepareMsg, digest string) *State
}
