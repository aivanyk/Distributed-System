package kvpaxos

import (
	"crypto/rand"
	"fmt"
	"math/big"

	"cs651/labrpc"
)

type Clerk struct {
	servers []*labrpc.ClientEnd
	// You will have to modify this struct.
	lastLeader int
	Cid        int64
	Rid        int64
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
	// You'll have to add code here.
	ck.lastLeader = -1
	ck.Cid = nrand()
	ck.Rid = 1
	return ck
}

// fetch the current value for a key.
// returns "" if the key does not exist.
// keeps trying forever in the face of all other errors.
//
// you can send an RPC with code like this:
// ok := ck.servers[i].Call("KVServer.Get", &args, &reply)
//
// the types of args and reply (including whether they are pointers)
// must match the declared types of the RPC handler function's
// arguments. and reply must be passed as a pointer.
func (ck *Clerk) Get(key string) string {

	// You will have to modify this function.
	len := len(ck.servers)
	args := &GetArgs{key, ck.Cid, ck.Rid}
	targetS := ck.lastLeader
	for {
		targetS = (targetS + 1) % len

		fmt.Printf("Clerk | try sending %v to %d ... \n", args, targetS)
		reply := &GetReply{}
		ok := ck.servers[targetS].Call("KVServer.Get", args, reply)
		if !ok {
			// fmt.Printf("Clerk | send Get to %d failed.", targetS)
			continue
		}
		if reply.Err != "" {
			fmt.Printf("Clerk | send Get to %d err: %s \n", targetS, reply.Err)
			continue
		}
		ck.lastLeader = targetS
		ck.Rid++
		return reply.Value
	}
}

// shared by Put and Append.
//
// you can send an RPC with code like this:
// ok := ck.servers[i].Call("KVServer.PutAppend", &args, &reply)
//
// the types of args and reply (including whether they are pointers)
// must match the declared types of the RPC handler function's
// arguments. and reply must be passed as a pointer.
func (ck *Clerk) PutAppend(key string, value string, op string) {
	// You will have to modify this function.
	len := len(ck.servers)
	args := &PutAppendArgs{key, value, op, ck.Cid, ck.Rid}
	targetS := ck.lastLeader
	for {
		targetS = (targetS + 1) % len

		// fmt.Printf("Clerk | try sending %v to %d ... \n", args, targetS)
		reply := &PutAppendReply{}
		ok := ck.servers[targetS].Call("KVServer.PutAppend", args, reply)
		if !ok {
			// fmt.Printf("Clerk | send PutAppend to %d failed.", targetS)
			continue
		}
		if reply.Err != "" {
			// fmt.Printf("Clerk | send PutAppend to %d err: %s \n", targetS, reply.Err)
			continue
		}
		ck.lastLeader = targetS
		ck.Rid++
		return
	}
}

func (ck *Clerk) Put(key string, value string) {
	ck.PutAppend(key, value, "Put")
}
func (ck *Clerk) Append(key string, value string) {
	ck.PutAppend(key, value, "Append")
}
