package kvpaxos

import (
	"fmt"
	"log"
	"sync"
	"sync/atomic"
	"time"

	omnipaxos "cs651/a2-omnipaxos"
	"cs651/labgob"
	"cs651/labrpc"
)

const Debug = false

func DPrintf(format string, a ...interface{}) (n int, err error) {
	if Debug {
		log.Printf(format, a...)
	}
	return
}

type Op struct {
	// Your definitions here.
	// Field names must start with capital letters,
	// otherwise RPC will break.
	Key   string
	Value string
	Op    string
	Cid   int64
	Rid   int64
}

type KVServer struct {
	mu      sync.Mutex
	me      int
	rf      *omnipaxos.OmniPaxos
	applyCh chan omnipaxos.ApplyMsg
	dead    int32 // set by Kill()

	enableLogging int32

	// Your definitions here.
	store             map[string]string
	getReplyMsg       chan string
	putAppendReplyMsg chan string
	lastRid           map[int64]int64
	waitTimeout       time.Duration
}

func (kv *KVServer) Get(args *GetArgs, reply *GetReply) {
	// Your code here.
	kv.mu.Lock()
	defer kv.mu.Unlock()
	// kv.mu.Lock()
	// if kv.lastTid == args.Tid {
	// 	reply.Err = "Duplicated operation!"
	// 	kv.mu.Unlock()
	// 	return
	// }
	// kv.mu.Unlock()

	command := Op{Key: args.Key, Op: "Get", Cid: args.Cid, Rid: args.Rid}
	_, _, isLeader := kv.rf.Proposal(command)
	// kv.mu.Lock()
	// defer kv.mu.Unlock()
	if !isLeader {
		reply.Err = "Not Leader!"
	} else {
		// m := <-kv.replyMsg
		// if m != "ok" {
		// 	reply.Err = Err(m)
		// } else {
		// 	_, isLeader := kv.rf.GetState()
		// 	if !isLeader {
		// 		reply.Err = "Lose leadership before committed!"
		// 	} else {
		// 		reply.Value = kv.store[args.Key]
		// 	}
		// }
		select {
		case m := <-kv.getReplyMsg:
			if m != "ok" {
				reply.Err = Err(m)
			} else {
				_, isLeader := kv.rf.GetState()
				// fmt.Printf("Server %d get state \n", kv.me)
				if !isLeader {
					reply.Err = "Lose leadership before committed!"
				} else {
					reply.Value = kv.store[args.Key]
					fmt.Printf("Server %d store get %s is %s \n", kv.me, args.Key, reply.Value)
				}
			}
		case <-time.After(kv.waitTimeout):
			reply.Err = "Timeout while waiting for commit!"
		}
	}
}

func (kv *KVServer) PutAppend(args *PutAppendArgs, reply *PutAppendReply) {
	// Your code here.
	kv.mu.Lock()
	defer kv.mu.Unlock()
	// if kv.lastTid == args.Tid {
	// 	reply.Err = "Duplicated operation!"
	// 	kv.mu.Unlock()
	// 	return
	// }
	// kv.mu.Unlock()

	command := Op{args.Key, args.Value, args.Op, args.Cid, args.Rid}
	_, _, isLeader := kv.rf.Proposal(command)
	// kv.mu.Lock()
	// defer kv.mu.Unlock()
	if !isLeader {
		reply.Err = "Not Leader!"
	} else {
		select {
		case m := <-kv.putAppendReplyMsg:
			if m != "ok" {
				reply.Err = Err(m)
			} else {
				_, isLeader := kv.rf.GetState()
				// fmt.Printf("Server %d get state \n", kv.me)
				if !isLeader {
					reply.Err = "Lose leadership before committed!"
				}
			}
		case <-time.After(kv.waitTimeout):
			reply.Err = "Timeout while waiting for commit!"
		}
	}
}

// the tester calls Kill() when a KVServer instance won't
// be needed again. for your convenience, we supply
// code to set rf.dead (without needing a lock),
// and a killed() method to test rf.dead in
// long-running loops. you can also add your own
// code to Kill(). you're not required to do anything
// about this, but it may be convenient (for example)
// to suppress debug output from a Kill()ed instance.
func (kv *KVServer) Kill() {
	atomic.StoreInt32(&kv.dead, 1)
	kv.rf.Kill()
	// Your code here, if desired.
}

func (kv *KVServer) killed() bool {
	z := atomic.LoadInt32(&kv.dead)
	return z == 1
}

// servers[] contains the ports of the set of
// servers that will cooperate via OmniPaxos to
// form the fault-tolerant key/value service.
// me is the index of the current server in servers[].
// the k/v server should store snapshots through the underlying OmniPaxos
// implementation, which should call persister.SaveStateAndSnapshot() to
// atomically save the OmniPaxos state along with the snapshot.
// the k/v server should snapshot when OmniPaxos's saved state exceeds maxomnipaxosstate bytes,
// in order to allow OmniPaxos to garbage-collect its log. if maxomnipaxosstate is -1,
// you don't need to snapshot.
// StartKVServer() must return quickly, so it should start goroutines
// for any long-running work.
func StartKVServer(servers []*labrpc.ClientEnd, me int, persister *omnipaxos.Persister, maxomnipaxosstate int) *KVServer {
	// call labgob.Register on structures you want
	// Go's RPC library to marshall/unmarshall.
	labgob.Register(Op{})

	kv := new(KVServer)
	kv.me = me
	// to enable logging
	// set to 0 to disable
	kv.enableLogging = 1

	// You may need initialization code here.

	kv.applyCh = make(chan omnipaxos.ApplyMsg, 500)
	kv.putAppendReplyMsg = make(chan string)
	kv.getReplyMsg = make(chan string)
	kv.rf = omnipaxos.Make(servers, me, persister, kv.applyCh)

	// You may need initialization code here.
	kv.store = make(map[string]string)
	kv.lastRid = make(map[int64]int64)
	kv.waitTimeout = 150 * time.Millisecond

	go func() {
		for {
			m, ok := <-kv.applyCh
			args := m.Command.(Op)
			ops := args.Op
			fmt.Printf("Server %d read from applyCh %v \n", kv.me, m)
			_, isLeader := kv.rf.GetState()
			fmt.Printf("Server %d get state \n", kv.me)
			if !ok {
				if isLeader {
					if ops == "Get" {
						kv.getReplyMsg <- "Read from applyCh fail!"
					} else {
						kv.putAppendReplyMsg <- "Read from applyCh fail!"
					}
				}
			} else {
				fmt.Printf("Server %d commit %v into log \n", kv.me, m)
				args, ok := m.Command.(Op)

				if !ok {
					fmt.Println("m.Command is not of type Op")
				} else {
					// kv.mu.Lock()
					if kv.lastRid[args.Cid] >= args.Rid {
						fmt.Printf("Server %d Duplicated operation \n", kv.me)
						if isLeader {
							if ops == "Get" {
								kv.getReplyMsg <- "ok"
							} else {
								kv.putAppendReplyMsg <- "ok"
							}
						}
						continue
					} else {
						kv.lastRid[args.Cid] = args.Rid
					}
					// kv.mu.Unlock()
					// kv.mu.Lock()
					if args.Op == "Put" {
						kv.store[args.Key] = args.Value
						fmt.Printf("Server %d store after %s into %s is %s \n", kv.me, args.Op, args.Key, kv.store[args.Key])
					} else if args.Op == "Append" {
						kv.store[args.Key] = kv.store[args.Key] + args.Value
						fmt.Printf("Server %d store after %s into %s is %s \n", kv.me, args.Op, args.Key, kv.store[args.Key])
					}
					// kv.mu.Unlock()
				}
				if isLeader {
					if ops == "Get" {
						kv.getReplyMsg <- "ok"
					} else {
						kv.putAppendReplyMsg <- "ok"
					}
				}
			}
		}
	}()

	return kv
}
