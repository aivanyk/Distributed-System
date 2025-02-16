package shardctrler

import (
	omnipaxos "cs651/a2-omnipaxos"
	"cs651/labgob"
	"cs651/labrpc"
	"encoding/gob"
	"sort"
	"sync"
)

type ShardCtrler struct {
	mu      sync.Mutex
	me      int
	op      *omnipaxos.OmniPaxos
	applyCh chan omnipaxos.ApplyMsg

	// Your data here.

	configs       []Config // indexed by config num
	clientLastSeq map[int64]int
	joinReplyMsg  chan string
	leaveReplyMsg chan string
	moveReplyMsg  chan string
	queryReplyMsg chan string
}

type Op struct {
	// Your data here.
	OpType   string
	ClientID int64
	SeqNum   int
	Args     interface{}
}

func (sc *ShardCtrler) Join(args *JoinArgs, reply *JoinReply) {
	// Your code here.
	sc.handleRequest("Join", args, reply)
}

func (sc *ShardCtrler) Leave(args *LeaveArgs, reply *LeaveReply) {
	// Your code here.
	sc.handleRequest("Leave", args, reply)
}

func (sc *ShardCtrler) Move(args *MoveArgs, reply *MoveReply) {
	// Your code here.
	sc.handleRequest("Move", args, reply)
}

func (sc *ShardCtrler) Query(args *QueryArgs, reply *QueryReply) {
	// Your code here.
	sc.handleRequest("Query", args, reply)
}

func (sc *ShardCtrler) handleRequest(opType string, args interface{}, reply interface{}) {
	sc.mu.Lock()
	defer sc.mu.Unlock()

	var clientID int64
	var seqNum int

	if r, ok := args.(ArgsInterface); ok {
		clientID = r.GetClientID()
		seqNum = r.GetSeqNum()
	}

	op := Op{
		OpType:   opType,
		ClientID: clientID,
		SeqNum:   seqNum,
		Args:     args,
	}

	lastSeq, ok := sc.clientLastSeq[op.ClientID]
	if ok && lastSeq >= op.SeqNum {
		return
	}

	_, _, isLeader := sc.op.Proposal(op)

	if !isLeader {
		if reply, ok := reply.(ReplyInterface); ok {
			reply.SetErr("Not Leader!")
		}
	} else {
		var readingCh chan string
		switch opType {
		case "Join":
			readingCh = sc.joinReplyMsg
		case "Move":
			readingCh = sc.moveReplyMsg
		case "Leave":
			readingCh = sc.leaveReplyMsg
		case "Query":
			readingCh = sc.queryReplyMsg
		}
		opReplyMsg := <-readingCh
		if opReplyMsg != "ok" {
			if reply, ok := reply.(ReplyInterface); ok {
				reply.SetErr(Err(opReplyMsg))
			}
		} else {
			sc.clientLastSeq[op.ClientID] = op.SeqNum
			_, isLeader := sc.op.GetState()
			// fmt.Printf("Server %d get state \n", kv.me)
			if !isLeader {
				if reply, ok := reply.(ReplyInterface); ok {
					reply.SetErr(Err("Lose leadership before committed!"))
				}
				return
			}

			if req, ok := op.Args.(*QueryArgs); ok {
				if reply, ok := reply.(*QueryReply); ok {
					if req.Num == -1 || req.Num >= len(sc.configs) {
						reply.Config = sc.configs[len(sc.configs)-1]
					} else {
						reply.Config = sc.configs[req.Num]
					}
				}
			}
		}
	}
}

// the tester calls Kill() when a ShardCtrler instance won't
// be needed again. you are not required to do anything
// in Kill(), but it might be convenient to (for example)
// turn off debug output from this instance.
func (sc *ShardCtrler) Kill() {
	sc.op.Kill()
	// Your code here, if desired.
}

// needed by shardkv tester
func (sc *ShardCtrler) OmniPaxos() *omnipaxos.OmniPaxos {
	return sc.op
}

// servers[] contains the ports of the set of
// servers that will cooperate via OmniPaxos to
// form the fault-tolerant shardctrler service.
// me is the index of the current server in servers[].
func StartServer(servers []*labrpc.ClientEnd, me int, persister *omnipaxos.Persister) *ShardCtrler {
	sc := new(ShardCtrler)
	sc.me = me

	sc.configs = make([]Config, 1)
	sc.configs[0].Groups = map[int][]string{}

	labgob.Register(Op{})
	sc.applyCh = make(chan omnipaxos.ApplyMsg, 500)
	sc.op = omnipaxos.Make(servers, me, persister, sc.applyCh)

	// Your code here.
	sc.clientLastSeq = make(map[int64]int)
	sc.joinReplyMsg = make(chan string)
	sc.moveReplyMsg = make(chan string)
	sc.leaveReplyMsg = make(chan string)
	sc.queryReplyMsg = make(chan string)

	go func() {
		for {
			m, ok := <-sc.applyCh
			ops := m.Command.(Op)
			opType := ops.OpType
			var readingCh chan string
			switch opType {
			case "Join":
				readingCh = sc.joinReplyMsg
			case "Move":
				readingCh = sc.moveReplyMsg
			case "Leave":
				readingCh = sc.leaveReplyMsg
			case "Query":
				readingCh = sc.queryReplyMsg
			}
			// fmt.Printf("Server %d read from applyCh %v \n", sc.me, m)
			_, isLeader := sc.op.GetState()

			if !ok {
				if isLeader {
					readingCh <- "Read from applyCh fail!"
				}
			} else {
				// fmt.Printf("Server %d commit %v into log, ops.Args: %v \n", sc.me, m, ops.Args)
				// sc.clientLastSeq[ops.ClientID] = ops.SeqNum

				if req, ok := ops.Args.(*JoinArgs); ok {
					// fmt.Printf("Server %d req after %v is: %v \n", sc.me, ops, req)
					newConfig := sc.makeNewConfig()
					for gid, servers := range req.Servers {
						newConfig.Groups[gid] = servers
					}
					sc.rebalanceShards(&newConfig)
					sc.configs = append(sc.configs, newConfig)
				} else if req, ok := ops.Args.(*LeaveArgs); ok {
					newConfig := sc.makeNewConfig()
					for _, gid := range req.GIDs {
						delete(newConfig.Groups, gid)
					}
					sc.rebalanceShards(&newConfig)
					sc.configs = append(sc.configs, newConfig)
				} else if req, ok := ops.Args.(*MoveArgs); ok {
					newConfig := sc.makeNewConfig()
					newConfig.Shards[req.Shard] = req.GID
					sc.configs = append(sc.configs, newConfig)
				} else if req, ok := ops.Args.(JoinArgs); ok {
					// fmt.Printf("Server %d req after %v is: %v \n", sc.me, ops, req)
					newConfig := sc.makeNewConfig()
					for gid, servers := range req.Servers {
						newConfig.Groups[gid] = servers
					}
					sc.rebalanceShards(&newConfig)
					sc.configs = append(sc.configs, newConfig)
				} else if req, ok := ops.Args.(LeaveArgs); ok {
					newConfig := sc.makeNewConfig()
					for _, gid := range req.GIDs {
						delete(newConfig.Groups, gid)
					}
					sc.rebalanceShards(&newConfig)
					sc.configs = append(sc.configs, newConfig)
				} else if req, ok := ops.Args.(MoveArgs); ok {
					newConfig := sc.makeNewConfig()
					newConfig.Shards[req.Shard] = req.GID
					sc.configs = append(sc.configs, newConfig)
				}
				// fmt.Printf("Server %d config after %v is: %v \n", sc.me, ops, sc.configs[len(sc.configs)-1])
				if isLeader {
					readingCh <- "ok"
				}
			}
		}
	}()

	return sc
}

func (sc *ShardCtrler) makeNewConfig() Config {
	lastConfig := sc.configs[len(sc.configs)-1]
	newConfig := Config{
		Num:    lastConfig.Num + 1,
		Shards: lastConfig.Shards,
		Groups: make(map[int][]string),
	}
	// Copy the group mappings
	for gid, servers := range lastConfig.Groups {
		newConfig.Groups[gid] = append([]string{}, servers...)
	}
	return newConfig
}

func (sc *ShardCtrler) rebalanceShards(config *Config) {
	if len(config.Groups) == 0 {
		// No groups available, all shards should belong to GID 0
		for i := 0; i < NShards; i++ {
			config.Shards[i] = 0
		}
		return
	}

	// List of GIDs (sorted for deterministic behavior)
	gids := make([]int, 0, len(config.Groups))
	for gid := range config.Groups {
		gids = append(gids, gid)
	}
	sort.Ints(gids)

	// Reset shard assignments and calculate the ideal distribution
	shardCount := make(map[int]int) // GID -> number of assigned shards
	for _, gid := range gids {
		shardCount[gid] = 0
	}

	// Ideal number of shards per group (balanced)
	idealShardCount := NShards / len(gids)
	extraShards := NShards % len(gids) // Some groups may get one extra shard

	// Prepare a list of unassigned shards
	unassignedShards := []int{}
	for i, gid := range config.Shards {
		if _, ok := config.Groups[gid]; !ok {
			// If the current shard's GID is invalid, mark it as unassigned
			unassignedShards = append(unassignedShards, i)
			config.Shards[i] = 0
		} else {
			shardCount[gid]++
		}
	}

	// Gather shards from over-represented groups
	for i := 0; i < NShards; i++ {
		gid := config.Shards[i]
		if gid != 0 && shardCount[gid] > idealShardCount {
			unassignedShards = append(unassignedShards, i)
			shardCount[gid]--
			config.Shards[i] = 0
		}
	}

	// Redistribute shards among groups
	shardIndex := 0
	for _, gid := range gids {
		// Determine how many shards this group should have
		targetShards := idealShardCount
		if extraShards > 0 {
			targetShards++
			extraShards--
		}

		// Assign shards to this group
		for shardCount[gid] < targetShards && shardIndex < len(unassignedShards) {
			shard := unassignedShards[shardIndex]
			config.Shards[shard] = gid
			shardCount[gid]++
			shardIndex++
		}
	}
}

func init() {
	gob.Register(QueryArgs{})
	gob.Register(QueryReply{})
	gob.Register(JoinArgs{})
	gob.Register(JoinReply{})
	gob.Register(LeaveArgs{})
	gob.Register(LeaveReply{})
	gob.Register(MoveArgs{})
	gob.Register(MoveReply{})
	gob.Register(Config{})
}
