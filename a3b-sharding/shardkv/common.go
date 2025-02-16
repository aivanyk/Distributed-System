package shardkv

//
// Sharded key/value server.
// Lots of replica groups, each running OmniPaxos.
// Shardctrler decides which group serves each shard.
// Shardctrler may change shard assignment from time to time.
//
// You will have to modify these definitions.
//

const (
	OK               = "OK"
	ErrNoKey         = "ErrNoKey"
	ErrWrongGroup    = "ErrWrongGroup"
	ErrWrongLeader   = "ErrWrongLeader"
	ErrReConfiguring = "ErrReConfiguring"
)

type Err string

// Put or Append
type PutAppendArgs struct {
	// You'll have to add definitions here.
	Key   string
	Value string
	Op    string // "Put" or "Append"
	// You'll have to add definitions here.
	// Field names must start with capital letters,
	// otherwise RPC will break.
	ClientID int64
	SeqNum   int
}

type PutAppendReply struct {
	Err Err
}

type GetArgs struct {
	Key string
	// You'll have to add definitions here.
	ClientID int64
	SeqNum   int
}

type GetReply struct {
	Err   Err
	Value string
}

type RePutArgs struct {
	// You'll have to add definitions here.
	Key   string
	Value string
	// You'll have to add definitions here.
	// Field names must start with capital letters,
	// otherwise RPC will break.
	ClientID int64
	SeqNum   int
}

type RePutReply struct {
	Err Err
}

type ReplyInterface interface {
	SetErr(Err)
}

func (r *GetReply) SetErr(err Err) {
	r.Err = err
}

func (r *PutAppendReply) SetErr(err Err) {
	r.Err = err
}

type ArgsInterface interface {
	GetClientID() int64
	GetSeqNum() int
	GetKey() string
}

func (a *GetArgs) GetClientID() int64 {
	return a.ClientID
}

func (a *GetArgs) GetSeqNum() int {
	return a.SeqNum
}

func (a *GetArgs) GetKey() string {
	return a.Key
}

func (a *PutAppendArgs) GetClientID() int64 {
	return a.ClientID
}

func (a *PutAppendArgs) GetSeqNum() int {
	return a.SeqNum
}

func (a *PutAppendArgs) GetKey() string {
	return a.Key
}
