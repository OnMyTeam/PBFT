package consensus

type PBFT interface {
	/*
	StartConsensus(request *RequestMsg, sequenceID int64) (*PrePrepareMsg, error)
	PrePrepare(prePrepareMsg *PrePrepareMsg) (*VoteMsg, error)
	Prepare(prepareMsg *VoteMsg) (*VoteMsg, error)
	Commit(commitMsg *VoteMsg) (*ReplyMsg, *RequestMsg, error)
	*/
	//StartConsensus(request *RequestMsg, sequenceID int64) (*PrepareMsg, error)
	Prepare(prepareMsg *PrepareMsg, requestMsg *RequestMsg) (VoteMsg, error)
	Vote(voteMsg *VoteMsg) (CollateMsg, error)
	Collate(collateMsg *CollateMsg) (CollateMsg, error)

	SetBizantine(nodeID string) bool
	GetSequenceID() int64
	GetF() int

	GetMsgReceiveChannel() <-chan interface{}
	GetMsgSendChannel() chan<- interface{}
	GetMsgExitReceiveChannel() <-chan int64
	GetMsgExitSendChannel() chan<- int64
	GetTimerStartReceiveChannel() <-chan string
	GetTimerStartSendChannel() chan<- string
	GetTimerStopReceiveChannel() <-chan string
	GetTimerStopSendChannel() chan<- string

	GetReqMsg() *RequestMsg
	GetPrepareMsg() *PrepareMsg
	GetVoteMsgs() map[string]*VoteMsg
	GetCollateMsgs() map[string]*CollateMsg

	//SetSuccChkPoint(int64)
	SetSequenceID(sequenceID int64)
	SetDigest(digest string)
	SetViewID(viewID int64)

	//setrequ
	ClearMsgLogs()
	Redo_SetState(viewID int64, nodeID string, totNodes int, prepareMsg *PrepareMsg, digest string) *State
}
