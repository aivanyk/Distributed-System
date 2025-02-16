package shardkv

import (
	"bytes"
	omnipaxos "cs651/a2-omnipaxos"
	"cs651/a3b-sharding/shardctrler"
	"cs651/labgob"
	"cs651/labrpc"
	"encoding/gob"
	"fmt"
	"os"
	"sync"
	"sync/atomic"
	"time"
)

type Op struct {
	// Your definitions here.
	// Field names must start with capital letters,
	// otherwise RPC will break.
	OpType   string
	ClientID int64
	SeqNum   int
	Args     interface{}
}

type ShardKV struct {
	mu            sync.Mutex
	me            int
	op            *omnipaxos.OmniPaxos
	applyCh       chan omnipaxos.ApplyMsg
	make_end      func(string) *labrpc.ClientEnd
	gid           int
	ctrlers       []*labrpc.ClientEnd
	maxpaxosstate int // snapshot if log grows this big

	// Your definitions here.
	mck               *shardctrler.Clerk
	config            shardctrler.Config
	clientLastSeq     map[int64]int
	shardData         map[int]map[string]string
	getReplyMsg       chan string
	putAppendReplyMsg chan string
	persister         *omnipaxos.Persister
	reconfiguring     int

	// measure
	totalOps            int64
	throughputStart     time.Time
	latencyMeasurements []int64
}

func (kv *ShardKV) Get(args *GetArgs, reply *GetReply) {
	// Your code here.
	// kv.handleRequest("Get", args, reply)
	start := time.Now()
	kv.handleRequest("Get", args, reply)
	elapsed := time.Since(start)

	if reply.Err == OK {
		kv.recordLatency(elapsed)
		atomic.AddInt64(&kv.totalOps, 1)
	}

}

func (kv *ShardKV) PutAppend(args *PutAppendArgs, reply *PutAppendReply) {
	// Your code here.
	// kv.handleRequest(args.Op, args, reply)
	start := time.Now()
	kv.handleRequest("Get", args, reply)
	elapsed := time.Since(start)

	if reply.Err == OK {
		kv.recordLatency(elapsed)
		atomic.AddInt64(&kv.totalOps, 1)
	}
}

func (kv *ShardKV) handleRequest(opType string, args interface{}, reply interface{}) {
	kv.mu.Lock()
	defer kv.mu.Unlock()
	if kv.reconfiguring > 0 {
		if reply, ok := reply.(ReplyInterface); ok {
			reply.SetErr(ErrReConfiguring)
		}
		return
	}
	// fmt.Printf("GID %v ShardKV %v have sharddata: %v \n", kv.gid, kv.me, kv.shardData[8]["0"])
	var clientID int64
	var seqNum int

	if r, ok := args.(ArgsInterface); ok {
		if !kv.validShard(r.GetKey()) {
			if reply, ok := reply.(ReplyInterface); ok {
				reply.SetErr(ErrWrongGroup)
				return
			}
		}
		clientID = r.GetClientID()
		seqNum = r.GetSeqNum()
	}

	op := Op{
		OpType:   opType,
		ClientID: clientID,
		SeqNum:   seqNum,
		Args:     args,
	}

	lastSeq, ok := kv.clientLastSeq[op.ClientID]
	if ok && lastSeq >= op.SeqNum {
		if reply, ok := reply.(ReplyInterface); ok {
			reply.SetErr(OK)
		}
		return
	}

	_, _, isLeader := kv.op.Proposal(op)

	// kv.mu.Unlock()
	// kv.mu.Lock()
	// defer kv.mu.Unlock()
	if !isLeader {
		if reply, ok := reply.(ReplyInterface); ok {
			reply.SetErr("Not Leader!")
		}
	} else {
		// fmt.Printf("Gid %v ShardKV %v handles %v \n", kv.gid, kv.me, args)
		var readingCh chan string
		switch opType {
		case "Get":
			readingCh = kv.getReplyMsg
		default:
			readingCh = kv.putAppendReplyMsg
		}
		opReplyMsg := <-readingCh

		// kv.mu.Lock()
		if opReplyMsg != "ok" {
			if reply, ok := reply.(ReplyInterface); ok {
				reply.SetErr(Err(opReplyMsg))
			}
		} else {
			kv.clientLastSeq[op.ClientID] = op.SeqNum
			_, isLeader := kv.op.GetState()
			// fmt.Printf("ShardKV %d get state \n", kv.me)
			if !isLeader {
				if reply, ok := reply.(ReplyInterface); ok {
					reply.SetErr(Err("Lose leadership before committed!"))
				}
				return
			}

			if req, ok := op.Args.(*GetArgs); ok {
				// fmt.Print("hihihi \n")
				if reply, ok := reply.(*GetReply); ok {
					// fmt.Print("hihihi222 \n")
					sha := key2shard(req.Key)
					value, ok := kv.shardData[sha][req.Key]
					if !ok {
						reply.Err = ErrNoKey
						return
					}

					reply.Err = OK
					reply.Value = value
					// fmt.Printf("ShardKV %d store get %s is %s \n", kv.me, req.Key, reply.Value)
					// fmt.Printf("Gid %v ShardKV %v: Get shard: %v key: %v result shardData: %v \n", kv.gid, kv.me, sha, req.Key, value)
				}
			} else {
				if reply, ok := reply.(*PutAppendReply); ok {
					reply.Err = OK
				}
			}
		}
		// kv.mu.Unlock()
	}

}

// the tester calls Kill() when a ShardKV instance won't
// be needed again. you are not required to do anything
// in Kill(), but it might be convenient to (for example)
// turn off debug output from this instance.
func (kv *ShardKV) Kill() {
	kv.op.Kill()
	// Your code here, if desired.
}

// servers[] contains the ports of the servers in this group.
//
// me is the index of the current server in servers[].
//
// the k/v server should store snapshots through the underlying OmniPaxos
// implementation, which should call persister.SaveStateAndSnapshot() to
// atomically save the OmniPaxos state along with the snapshot.
//
// the k/v server should snapshot when OmniPaxos's saved state exceeds
// maxOmniPaxosstate bytes, in order to allow OmniPaxos to garbage-collect its
// log. if maxOmniPaxosstate is -1, you don't need to snapshot.
//
// gid is this group's GID, for interacting with the shardctrler.
//
// pass ctrlers[] to shardctrler.MakeClerk() so you can send
// RPCs to the shardctrler.
//
// make_end(servername) turns a server name from a
// Config.Groups[gid][i] into a labrpc.ClientEnd on which you can
// send RPCs. You'll need this to send RPCs to other groups.
//
// look at client.go for examples of how to use ctrlers[]
// and make_end() to send RPCs to the group owning a specific shard.
//
// StartServer() must return quickly, so it should start goroutines
// for any long-running work.
func StartServer(servers []*labrpc.ClientEnd, me int, persister *omnipaxos.Persister, maxOmniPaxosstate int, gid int, ctrlers []*labrpc.ClientEnd, make_end func(string) *labrpc.ClientEnd) *ShardKV {
	// call labgob.Register on structures you want
	// Go's RPC library to marshall/unmarshall.

	kv := new(ShardKV)
	kv.me = me
	kv.maxpaxosstate = maxOmniPaxosstate
	kv.make_end = make_end
	kv.gid = gid
	kv.ctrlers = ctrlers

	// Your initialization code here.

	// Use something like this to talk to the shardctrler:
	kv.mck = shardctrler.MakeClerk(kv.ctrlers)
	kv.persister = persister
	kv.clientLastSeq = make(map[int64]int)
	kv.applyCh = make(chan omnipaxos.ApplyMsg, 500)
	kv.putAppendReplyMsg = make(chan string, 500)
	kv.getReplyMsg = make(chan string, 500)
	kv.op = omnipaxos.Make(servers, me, persister, kv.applyCh)
	kv.shardData = make(map[int]map[string]string)
	// kv.config = make(shardctrler.Config)
	kv.reconfiguring = 0
	for i := 0; i < shardctrler.NShards; i++ {
		kv.shardData[i] = make(map[string]string)
	}

	// measure
	kv.totalOps = 0
	kv.throughputStart = time.Now()
	kv.latencyMeasurements = make([]int64, 0)
	kv.StartPeriodicMetricsWriter("../metrics/metrics_log.txt")

	go func() {
		for {
			m, ok := <-kv.applyCh
			ops := m.Command.(Op)
			opType := ops.OpType
			var readingCh chan string
			switch opType {
			case "Get":
				readingCh = kv.getReplyMsg
			default:
				readingCh = kv.putAppendReplyMsg
			}
			// fmt.Printf("Gid %v ShardKV %d read from applyCh %v \n", kv.gid, kv.me, m)
			_, isLeader := kv.op.GetState()
			// fmt.Printf("ShardKV %d get state \n", kv.me)
			if !ok {
				if isLeader {
					readingCh <- "Read from applyCh fail!"
				}
			} else {
				// fmt.Printf("Gid %v ShardKV %d commit %v into log \n", kv.gid, kv.me, m)

				// kv.mu.Unlock()
				// kv.mu.Lock()
				if req, ok := ops.Args.(*PutAppendArgs); ok {
					sha := key2shard(req.Key)
					if opType == "Put" {
						kv.shardData[sha][req.Key] = req.Value
					} else if opType == "Append" {
						kv.shardData[sha][req.Key] += req.Value
					}
					// fmt.Printf("Gid %v ShardKV %v: %s shard: %v key: %v result shardData: %v \n", kv.gid, kv.me, opType, sha, req.Key, kv.shardData[sha][req.Key])
				} else if req, ok := ops.Args.(PutAppendArgs); ok {
					sha := key2shard(req.Key)
					if opType == "Put" {
						kv.shardData[sha][req.Key] = req.Value //???
					} else if opType == "Append" {
						kv.shardData[sha][req.Key] += req.Value
					}
					// fmt.Printf("Gid %v ShardKV %v: %s shard: %v key: %v result shardData: %v \n", kv.gid, kv.me, opType, sha, req.Key, kv.shardData[sha][req.Key])
				} else if req, ok := ops.Args.(RePutArgs); ok {
					sha := key2shard(req.Key)
					kv.shardData[sha][req.Key] = req.Value //???
					kv.mu.Lock()
					kv.reconfiguring -= 1

					// fmt.Printf("Gid %v ShardKV %v: %s shard: %v key: %v result shardData: %v reconfiguring: %v \n", kv.gid, kv.me, opType, sha, req.Key, kv.shardData[sha][req.Key], kv.reconfiguring)
					kv.mu.Unlock()
				}
				// kv.mu.Unlock()

				if isLeader && opType != "RePut" {
					readingCh <- "ok"
				}
			}
		}
	}()

	go func() {
		for {
			newConfig := kv.mck.Query(-1) // Fetch latest configuration
			kv.mu.Lock()
			if newConfig.Num > kv.config.Num {
				// fmt.Printf("Gid %v ShardKV %d start reconfigure \n", kv.gid, kv.me)
				kv.updateConfig(newConfig)
				// fmt.Printf("Gid %v ShardKV %d reconfigure lock release, value %v, shards: %v \n", kv.gid, kv.me, kv.reconfiguring, kv.config.Shards)
			}
			kv.mu.Unlock()
			time.Sleep(100 * time.Millisecond)
		}
	}()

	return kv
}

func (kv *ShardKV) validShard(key string) bool {
	shard := key2shard(key)
	gid := kv.config.Shards[shard]
	return gid == kv.gid
}

func init() {
	// Register all types used in RPCs
	gob.Register(PutAppendArgs{})
	gob.Register(PutAppendReply{})
	gob.Register(GetArgs{})
	gob.Register(GetReply{})
	gob.Register(RePutArgs{})
	gob.Register(RePutReply{})
	gob.Register(Op{})
}

func (kv *ShardKV) updateConfig(newConfig shardctrler.Config) {
	oldShards := kv.config.Shards
	newShards := newConfig.Shards
	// fmt.Printf("Gid %v ShardKV %v oldShards %v newShards %v \n", kv.gid, kv.me, oldShards, newShards)

	for shard, newGID := range newShards {
		if newGID == kv.gid && oldShards[shard] != kv.gid {
			// if newGID == kv.gid {
			kv.acquireShard(shard, oldShards[shard])
		}
		// if oldShards[shard] == kv.gid && newGID != kv.gid {
		// 	kv.transferShard(shard, newGID)
		// }
		// kv.wg.Done()
	}

	kv.config = newConfig
}

func (kv *ShardKV) acquireShard(shard int, oldGID int) {
	if oldGID == 0 {
		kv.shardData[shard] = make(map[string]string) //???
		return
	}
	// kv.reconfiguring += 1
	// _, isLeader := kv.op.GetState()
	// if !isLeader {
	// 	time.Sleep(50 * time.Millisecond)
	// }
	// _, isLeader = kv.op.GetState()
	// if !isLeader {
	// 	// fmt.Printf("acquireShard | Gid %v ShardKV %v not leader \n", kv.gid, kv.me)
	// 	return
	// }

	servers := kv.config.Groups[oldGID]
	// fmt.Printf("Gid %v ShardKV %v groups: %v, oldGID: %v, servers: %v \n", kv.gid, kv.me, kv.config.Groups, oldGID, servers)
	for {
		for _, srv := range servers {
			args := ShardTransferArgs{Shard: shard}
			reply := ShardTransferReply{}
			srv := kv.make_end(srv)
			// fmt.Printf("Gid %v ShardKV %v call TransferShard \n", kv.gid, kv.me)
			ok := srv.Call("ShardKV.TransferShard", &args, &reply)
			if ok && reply.Err == OK {
				// fmt.Printf("Gid %v ShardKV %v from Gid %v ShardKV %v received shard from acquire: %v \n", kv.gid, kv.me, reply.Gid, reply.Sid, reply.Data)
				for key, value := range reply.Data {
					kv.reconfiguring += 1
					// if !isLeader {
					// 	continue
					// }
					op := Op{
						OpType: "RePut",
						Args: RePutArgs{
							Key:   key,
							Value: value,
						},
					}
					kv.op.Proposal(op)
				}

				// kv.shardData[shard] = make(map[string]string)
				// for key, value := range reply.Data {
				// 	kv.shardData[shard][key] = value
				// }
				// fmt.Printf("Gid %v ShardKV %v from Gid %v ShardKV %v received shard from acquire %v: %v \n", kv.gid, kv.me, reply.Gid, reply.Sid, shard, kv.shardData[shard])
				return
			}
		}
	}

}

func (kv *ShardKV) transferShard(shard int, newGID int) {
	data := kv.shardData[shard]
	// delete(kv.shardData, shard)
	_, isLeader := kv.op.GetState()
	if !isLeader {
		// fmt.Printf("transferShard | Gid %v ShardKV %v is not leader \n", kv.gid, kv.me)
		return
	}

	// Send data to the new group (you might need to retry if it fails)
	servers := kv.config.Groups[newGID]
	for _, srv := range servers {
		args := ShardTransferArgs{Sid: kv.me, Gid: kv.gid, Shard: shard, Data: data}
		reply := ShardTransferReply{}
		srv := kv.make_end(srv)
		ok := srv.Call("ShardKV.ReceiveShard", &args, &reply)
		if ok && reply.Err == OK {
			return
		}
	}

	// Retry logic can be added here if necessary
}

type ShardTransferArgs struct {
	Gid   int
	Sid   int
	Shard int
	Data  map[string]string
}

type ShardTransferReply struct {
	Gid  int
	Sid  int
	Err  Err
	Data map[string]string
}

func (kv *ShardKV) TransferShard(args *ShardTransferArgs, reply *ShardTransferReply) {
	kv.mu.Lock()
	defer kv.mu.Unlock()
	// fmt.Printf("TransferShard | Gid %v ShardKV %v received \n", kv.gid, kv.me)
	_, isLeader := kv.op.GetState()
	if !isLeader {
		reply.Err = ErrWrongLeader
		// fmt.Printf("TransferShard | Gid %v ShardKV %v is not leader \n", kv.gid, kv.me)
		return
	}

	if _, ok := kv.shardData[args.Shard]; ok {
		reply.Sid = kv.me
		reply.Gid = kv.gid
		reply.Data = kv.shardData[args.Shard]
		reply.Err = OK
	} else {
		reply.Err = ErrNoKey
	}
}

func (kv *ShardKV) ReceiveShard(args *ShardTransferArgs, reply *ShardTransferReply) {
	kv.mu.Lock()
	defer kv.mu.Unlock()
	// fmt.Printf("Gid %v ShardKV %v from Gid %v ShardKV %v received shard \n", kv.gid, kv.me, args.Gid, args.Sid)
	for key, value := range args.Data {
		op := Op{
			OpType: "RePut",
			Args: RePutArgs{
				Key:   key,
				Value: value,
			},
		}
		kv.op.Proposal(op)
	}
	// kv.shardData[args.Shard] = make(map[string]string)
	// for key, value := range args.Data {
	// 	kv.shardData[args.Shard][key] = value
	// }
	// fmt.Printf("Gid %v ShardKV %v from Gid %v ShardKV %v received shard %v: %v \n", kv.gid, kv.me, args.Gid, args.Sid, args.Shard, kv.shardData[args.Shard])
	reply.Err = OK
}

func (kv *ShardKV) persist() {
	// Your code here (2C).
	// Example:
	w := new(bytes.Buffer)
	e := labgob.NewEncoder(w)
	e.Encode(kv.config)
	e.Encode(kv.shardData)

	data := w.Bytes()
	kv.persister.SaveOmnipaxosState(data)
}

// restore previously persisted state.
func (kv *ShardKV) readPersist(data []byte) bool {
	if data == nil || len(data) < 1 {
		return false
	}
	r := bytes.NewBuffer(data)
	d := labgob.NewDecoder(r)
	var config shardctrler.Config
	var shardData map[int]map[string]string
	if d.Decode(&config) != nil ||
		d.Decode(&shardData) != nil {
		// fmt.Printf("readPersist FAILED!")
		return false
	} else {
		kv.config = config
		kv.shardData = shardData
		return true
	}
}

func (kv *ShardKV) GetThroughput() float64 {
	elapsed := time.Since(kv.throughputStart).Seconds() // Time in seconds
	ops := atomic.LoadInt64(&kv.totalOps)               // Safely read totalOps
	if elapsed == 0 {
		return 0
	}
	return float64(ops) / elapsed // Operations per second
}

func (kv *ShardKV) recordLatency(latency time.Duration) {
	atomic.AddInt64(&kv.totalOps, 1)
	kv.mu.Lock()
	kv.latencyMeasurements = append(kv.latencyMeasurements, latency.Nanoseconds())
	kv.mu.Unlock()
}

func (kv *ShardKV) StartPeriodicMetricsWriter(filename string) {
	go func() {
		ticker := time.NewTicker(30 * time.Second) // Write every minute
		defer ticker.Stop()

		for range ticker.C {
			kv.mu.Lock()

			file, err := os.OpenFile(filename, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
			if err != nil {
				fmt.Printf("Failed to open metrics file: %v\n", err)
				return
			}
			fmt.Printf("Success to open metrics file: %v\n", filename)

			ops := atomic.LoadInt64(&kv.totalOps)
			elapsed := time.Since(kv.throughputStart).Seconds()
			throughput := float64(ops) / elapsed

			file.WriteString(fmt.Sprintf("Throughput: %.2f ops/sec\n", throughput))

			file.WriteString("Latency Measurements (ns):\n")
			for _, latency := range kv.latencyMeasurements {
				file.WriteString(fmt.Sprintf("%.2f\n", float64(latency))) // Convert ns to ms
			}

			// Clear latencyMeasurements after writing
			kv.latencyMeasurements = []int64{}

			file.Close()
			kv.mu.Unlock()
		}
	}()
}
