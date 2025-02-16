package shardctrler

//
// Shardctrler clerk.
//

import (
	"crypto/rand"
	"cs651/labrpc"
	"math/big"
)

type Clerk struct {
	servers []*labrpc.ClientEnd
	// Your data here.
	clientID int64
	seqNum   int
}

func nrand() int64 {
	max := big.NewInt(int64(1) << 62)
	bigx, _ := rand.Int(rand.Reader, max)
	x := bigx.Int64()
	return x
}

func MakeClerk(servers []*labrpc.ClientEnd) *Clerk {
	ck := new(Clerk)
	ck.servers = servers
	// Your code here.
	ck.clientID = nrand()
	ck.seqNum = 0
	return ck
}

func (ck *Clerk) Query(num int) Config {
	// args := &QueryArgs{}
	// // Your code here.
	// args.Num = num
	args := &QueryArgs{
		Num:      num,
		ClientID: ck.clientID,
		SeqNum:   ck.seqNum,
	}
	ck.seqNum++
	for {
		// try each known server.
		for _, srv := range ck.servers {
			var reply QueryReply
			ok := srv.Call("ShardCtrler.Query", args, &reply)
			if ok && reply.Err == "" {
				return reply.Config
			}
		}
		// time.Sleep(100 * time.Millisecond)
	}
}

func (ck *Clerk) Join(servers map[int][]string) {
	// args := &JoinArgs{}
	// // Your code here.
	// args.Servers = servers
	args := &JoinArgs{
		Servers:  servers,
		ClientID: ck.clientID,
		SeqNum:   ck.seqNum,
	}
	ck.seqNum++

	for {
		// try each known server.
		for _, srv := range ck.servers {
			var reply JoinReply
			ok := srv.Call("ShardCtrler.Join", args, &reply)
			if ok && reply.Err == "" {
				return
			}
		}
		// time.Sleep(100 * time.Millisecond)
	}
}

func (ck *Clerk) Leave(gids []int) {
	// args := &LeaveArgs{}
	// // Your code here.
	// args.GIDs = gids
	args := &LeaveArgs{
		GIDs:     gids,
		ClientID: ck.clientID,
		SeqNum:   ck.seqNum,
	}
	ck.seqNum++

	for {
		// try each known server.
		for _, srv := range ck.servers {
			var reply LeaveReply
			ok := srv.Call("ShardCtrler.Leave", args, &reply)
			if ok && reply.Err == "" {
				return
			}
		}
		// time.Sleep(100 * time.Millisecond)
	}
}

func (ck *Clerk) Move(shard int, gid int) {
	// args := &MoveArgs{}
	// // Your code here.
	// args.Shard = shard
	// args.GID = gid
	args := &MoveArgs{
		Shard:    shard,
		GID:      gid,
		ClientID: ck.clientID,
		SeqNum:   ck.seqNum,
	}
	ck.seqNum++

	for {
		// try each known server.
		for _, srv := range ck.servers {
			var reply MoveReply
			ok := srv.Call("ShardCtrler.Move", args, &reply)
			if ok && reply.Err == "" {
				return
			}
		}
		// time.Sleep(100 * time.Millisecond)
	}
}
