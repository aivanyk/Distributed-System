package shardctrler

//
// Shard controler: assigns shards to replication groups.
//
// RPC interface:
// Join(servers) -- add a set of groups (gid -> server-list mapping).
// Leave(gids) -- delete a set of groups.
// Move(shard, gid) -- hand off one shard from current owner to gid.
// Query(num) -> fetch Config # num, or latest config if num==-1.
//
// A Config (configuration) describes a set of replica groups, and the
// replica group responsible for each shard. Configs are numbered. Config
// #0 is the initial configuration, with no groups and all shards
// assigned to group 0 (the invalid group).
//
// You will need to add fields to the RPC argument structs.
//

// The number of shards.
const NShards = 10

// A configuration -- an assignment of shards to groups.
// Please don't change this.
type Config struct {
	Num    int              // config number
	Shards [NShards]int     // shard -> gid
	Groups map[int][]string // gid -> servers[]
}

const (
	OK = "OK"
)

type Err string

type JoinArgs struct {
	Servers  map[int][]string // new GID -> servers mappings
	ClientID int64
	SeqNum   int
}

type JoinReply struct {
	WrongLeader bool
	Err         Err
}

type LeaveArgs struct {
	GIDs     []int
	ClientID int64
	SeqNum   int
}

type LeaveReply struct {
	WrongLeader bool
	Err         Err
}

type MoveArgs struct {
	Shard    int
	GID      int
	ClientID int64
	SeqNum   int
}

type MoveReply struct {
	WrongLeader bool
	Err         Err
}

type QueryArgs struct {
	Num      int // desired config number
	ClientID int64
	SeqNum   int
}

type QueryReply struct {
	WrongLeader bool
	Err         Err
	Config      Config
}

type ReplyInterface interface {
	SetWrongLeader(bool)
	SetErr(Err)
}

func (r *QueryReply) SetWrongLeader(wrongLeader bool) {
	r.WrongLeader = wrongLeader
}

func (r *QueryReply) SetErr(err Err) {
	r.Err = err
}

func (r *JoinReply) SetWrongLeader(wrongLeader bool) {
	r.WrongLeader = wrongLeader
}

func (r *JoinReply) SetErr(err Err) {
	r.Err = err
}

func (r *LeaveReply) SetWrongLeader(wrongLeader bool) {
	r.WrongLeader = wrongLeader
}

func (r *LeaveReply) SetErr(err Err) {
	r.Err = err
}

func (r *MoveReply) SetWrongLeader(wrongLeader bool) {
	r.WrongLeader = wrongLeader
}

func (r *MoveReply) SetErr(err Err) {
	r.Err = err
}

type ArgsInterface interface {
	GetClientID() int64
	GetSeqNum() int
}

func (a *JoinArgs) GetClientID() int64 {
	return a.ClientID
}

func (a *JoinArgs) GetSeqNum() int {
	return a.SeqNum
}

func (a *MoveArgs) GetClientID() int64 {
	return a.ClientID
}

func (a *MoveArgs) GetSeqNum() int {
	return a.SeqNum
}

func (a *LeaveArgs) GetClientID() int64 {
	return a.ClientID
}

func (a *LeaveArgs) GetSeqNum() int {
	return a.SeqNum
}

func (a *QueryArgs) GetClientID() int64 {
	return a.ClientID
}

func (a *QueryArgs) GetSeqNum() int {
	return a.SeqNum
}
