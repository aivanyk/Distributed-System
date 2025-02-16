package omnipaxos

//
// This is an outline of the API that OmniPaxos must expose to
// the service (or tester). See comments below for
// each of these functions for more details.
//
// rf = Make(...)
//   Create a new OmniPaxos server.
// op.Start(command interface{}) (index, ballot, isleader)
//   Start agreement on a new log entry
// op.GetState() (ballot, isLeader)
//   ask a OmniPaxos for its current ballot, and whether it thinks it is leader
// ApplyMsg
//   Each time a new entry is committed to the log, each OmniPaxos peer
//   should send an ApplyMsg to the service (or tester) in the same server.
//

import (
	"bytes"
	"sync"
	"sync/atomic"
	"time"

	"cs651/labgob"
	"cs651/labrpc"

	"github.com/rs/zerolog/log"
)

type Role int

const (
	FOLLOWER Role = iota
	LEADER
)

type Phase int

const (
	PREPARE Phase = iota
	ACCEPT
	RECOVER
)

// A Go object implementing a single OmniPaxos peer.
type OmniPaxos struct {
	mu            sync.Mutex          // Lock to protect shared access to this peer's state
	peers         []*labrpc.ClientEnd // RPC end points of all peers
	persister     *Persister          // Object to hold this peer's persisted state
	me            int                 // This peer's index into peers[]
	dead          int32               // Set by Kill()
	enableLogging int32
	// Your code here (2A, 2B).
	// (2A)
	// Persistent state on all servers
	L Ballot // ballot number of current leader
	// Volatile staste on all servers
	R       int                 // current heartbeat round
	B       Ballot              // ballot number
	QC      bool                // quorum-connected flag
	delay   time.Duration       // the duaration a server waits for heartbeat
	ballots map[int]BallotEntry // ballots received in the current round

	// (2B)
	// Persistent state on all servers
	Log         []LogEntry // log with entries
	PromisedRnd Ballot
	AcceptedRnd Ballot
	DecidedIdx  int

	// Volatile staste on all servers
	state State // the role and phase a server is in

	// Volatile state of leader
	currentRnd Ballot
	promises   map[int]PromiseEntry
	maxProm    PromiseEntry
	accepted   []int
	buffer     []LogEntry

	// applyChan for tester
	applyCh  chan ApplyMsg
	LinkDrop bool
}

type State struct {
	Role  Role
	Phase Phase
}

type LogEntry struct {
	Valid   bool
	Command interface{}
}

type Promise struct {
	N      Ballot     // promised round
	AccRnd Ballot     // the acceptedRnd of follower
	LogIdx int        // the log length of follower
	DecIdx int        // the decidedIdx of follower
	Sfx    []LogEntry // suffix of entries the leader might be missing
}

type PromiseEntry struct {
	AccRnd Ballot     // the acceptedRnd of follower
	LogIdx int        // the log length of follower
	F      int        // the follower PID
	DecIdx int        // the decidedIdx of follower
	Sfx    []LogEntry // suffix of entries the leader might be missing
}

type Prepare struct {
	N      Ballot // rnd of leader
	AccRnd Ballot // the accRnd of leader
	LogIdx int    // the length of the leader's log
	DecIdx int    // the decide Idx of leader
}

type Decide struct {
	N      Ballot // round of leader
	DecIdx int    // position in the log that has been decided
}

type Accept struct {
	N Ballot      // round of leader
	C interface{} // client request
}

type Accepted struct {
	N      Ballot // promised round
	LogIdx int    // the position in the log f has accepted up to
}

type AcceptSync struct {
	N       Ballot     // round of leader
	Sfx     []LogEntry // entries to be appended to the log
	SyncIdx int        // the position in the log where sfx should be appended at
}

// As each OmniPaxos peer becomes aware that successive log entries are
// committed, the peer should send an ApplyMsg to the service (or
// tester) on the same server, via the applyCh passed to Make(). Set
// CommandValid to true to indicate that the ApplyMsg contains a newly
// committed log entry.
type ApplyMsg struct {
	CommandValid bool
	Command      interface{}
	CommandIndex int
}

// Ensure that all your fields for a RCP start with an Upper Case letter
type HBRequest struct {
	// Your code here (2A).
	Rnd    int
	Ballot Ballot
	QC     bool
}

type HBReply struct {
	// Your code here (2A).
	Rnd    int
	Ballot Ballot
	QC     bool
}

type DummyReply struct{}

type LeaderRequest struct {
	PID int
	N   Ballot
}

type PrepareRequest struct {
	FollowerId int
}

type Ballot struct {
	Seq int
	PID int
}

type BallotEntry struct {
	B  Ballot
	QC bool
}

// Sample code for sending an RPC to another server
func (op *OmniPaxos) sendHeartBeats(server int, args *HBRequest, reply *HBReply) bool {
	ok := op.peers[server].Call("OmniPaxos.HeartBeatHandler", args, reply)
	return ok
}

func (op *OmniPaxos) HeartBeatHandler(args *HBRequest, reply *HBReply) {
	// Your code here (2A).
	op.mu.Lock()
	defer op.mu.Unlock()
	// // fmt.Printf("Receiver: %d, Rnd: %d, QC: %v \n", op.B.PID, args.Rnd, op.QC)
	reply.Rnd = args.Rnd
	reply.Ballot = op.B
	reply.QC = op.QC
}

func (op *OmniPaxos) sendAccept(server int, args *Accept, reply *Accepted) bool {
	ok := op.peers[server].Call("OmniPaxos.AcceptHandler", args, reply)
	return ok
}

func (op *OmniPaxos) AcceptHandler(args *Accept, reply *Accepted) {
	op.mu.Lock()
	defer op.mu.Unlock()
	if op.PromisedRnd != args.N || !op.checkState(FOLLOWER, ACCEPT) {
		return
	}
	op.Log = append(op.Log, LogEntry{Valid: true, Command: args.C})
	op.persist()
	reply.N = args.N
	reply.LogIdx = len(op.Log)
	// fmt.Printf("Accept | Server: %d receive: %v, reply: %v \n", op.B.PID, args, reply)
}

func (op *OmniPaxos) sendDecide(server int, args *Decide, reply *DummyReply) bool {
	ok := op.peers[server].Call("OmniPaxos.DecideHandler", args, reply)
	return ok
}

func (op *OmniPaxos) DecideHandler(args *Decide, reply *DummyReply) {
	op.mu.Lock()
	// fmt.Printf("Decide | Server: %d Log Length: %d, old DecIdx: %d, new DecIDx: %d \n", op.B.PID, len(op.Log), op.DecidedIdx, args.DecIdx)
	if op.PromisedRnd == args.N && op.checkState(FOLLOWER, ACCEPT) && args.DecIdx > op.DecidedIdx {
		// send logs to applyChan
		// fmt.Printf("Decide | Server: %d Log: %v, Log Length: %d, old DecIdx: %d, new DecIDx: %d \n", op.B.PID, op.Log, len(op.Log), op.DecidedIdx, args.DecIdx)

		preDec := op.DecidedIdx
		op.sendApplyMsg(preDec, args.DecIdx)
		// go func() {
		// 	// op.mu.Lock()
		// 	// defer op.mu.Unlock()

		// }()
		op.DecidedIdx = args.DecIdx
		op.persist()
	}
	op.mu.Unlock()
	// fmt.Printf("Decide | Server: %d lock released \n", op.B.PID)
}

func (op *OmniPaxos) sendPrepare(server int, args *Prepare, reply *Promise) bool {
	ok := op.peers[server].Call("OmniPaxos.PrepareHandler", args, reply)
	return ok
}

func (op *OmniPaxos) PrepareHandler(args *Prepare, reply *Promise) {
	op.mu.Lock()
	defer op.mu.Unlock()
	// fmt.Printf("Prepare | Server: %d received prepare, PromisedRnd: %d, leader rnd: %d, AcceptedRnd: %d, AccRnd: %d \n", op.B.PID, op.PromisedRnd, args.N, op.AcceptedRnd, args.AccRnd)
	if compareBallot(op.PromisedRnd, args.N) > 0 {
		return
	}
	op.L = args.N
	op.LinkDrop = false
	op.state = State{FOLLOWER, PREPARE}
	op.PromisedRnd = args.N
	op.persist()
	if compareBallot(op.AcceptedRnd, args.AccRnd) > 0 {
		reply.Sfx = op.suffix(args.DecIdx)
	} else if op.AcceptedRnd == args.AccRnd {
		reply.Sfx = op.suffix(args.LogIdx)
	} else {
		reply.Sfx = []LogEntry{}
	}
	reply.N = args.N
	reply.AccRnd = op.AcceptedRnd
	reply.LogIdx = len(op.Log)
	reply.DecIdx = op.DecidedIdx
}

func (op *OmniPaxos) sendAcceptSync(server int, args *AcceptSync, reply *Accepted) bool {
	ok := op.peers[server].Call("OmniPaxos.AcceptSyncHandler", args, reply)
	return ok
}

func (op *OmniPaxos) AcceptSyncHandler(args *AcceptSync, reply *Accepted) {
	op.mu.Lock()
	defer op.mu.Unlock()
	if op.PromisedRnd != args.N || !op.checkState(FOLLOWER, PREPARE) {
		return
	}
	op.AcceptedRnd = args.N
	op.state = State{FOLLOWER, ACCEPT}
	op.Log = op.prefix(args.SyncIdx)
	op.Log = append(op.Log, args.Sfx...)
	op.persist()
	reply.N = args.N
	reply.LogIdx = len(op.Log)
	// // fmt.Printf("AcceptSync | Server: %d received: %v, reply %v \n", op.B.PID, args, reply)
	// fmt.Printf("AcceptSync | Server: %d received, reply %v \n", op.B.PID, reply)
}

func (op *OmniPaxos) promiseHandler(reply *Promise, followerId int) {
	// // fmt.Printf("Promise | Server: %d received promise: %v from %d \n", op.B.PID, reply, followerId)
	if op.currentRnd != reply.N {
		return
	}
	fPromise := PromiseEntry{
		AccRnd: reply.AccRnd,
		LogIdx: reply.LogIdx,
		F:      followerId,
		DecIdx: reply.DecIdx,
		Sfx:    append([]LogEntry(nil), reply.Sfx...),
	}
	op.promises[followerId] = fPromise

	if op.checkState(LEADER, PREPARE) {
		if len(op.promises) < len(op.peers)/2+1 {
			return
		}
		op.maxProm = op.promises[0]
		for _, pr := range op.promises {
			if compareBallot(pr.AccRnd, op.maxProm.AccRnd) > 0 {
				op.maxProm = pr
			} else if pr.AccRnd == op.maxProm.AccRnd && pr.LogIdx > op.maxProm.LogIdx {
				op.maxProm = pr
			}
		}

		if op.maxProm.AccRnd != op.AcceptedRnd {
			op.Log = op.prefix(op.DecidedIdx)
		}
		op.Log = append(op.Log, op.maxProm.Sfx...)
		if op.stopped() {
			op.buffer = []LogEntry{}
		} else {
			op.Log = append(op.Log, op.buffer...)
		}
		op.AcceptedRnd = op.currentRnd
		op.persist()
		for _, pr := range op.promises {
			if pr.F == op.me {
				selfPromise := PromiseEntry{
					AccRnd: op.AcceptedRnd,
					LogIdx: len(op.Log),
					F:      op.B.PID,
					DecIdx: op.DecidedIdx,
					Sfx:    op.suffix(op.DecidedIdx),
				}
				op.promises[op.B.PID] = selfPromise
				break
			}
		}
		op.maxProm = op.promises[0]
		for _, pr := range op.promises {
			if compareBallot(pr.AccRnd, op.maxProm.AccRnd) > 0 {
				op.maxProm = pr
			} else if pr.AccRnd == op.maxProm.AccRnd && pr.LogIdx > op.maxProm.LogIdx {
				op.maxProm = pr
			}
		}

		op.accepted[op.B.PID] = len(op.Log)
		op.state = State{LEADER, ACCEPT}
		// fmt.Printf("Promise | Server: %d state ACCEPT, log length: %d \n", op.B.PID, len(op.Log))

		for _, pr := range op.promises {
			syncIdx := -1
			if pr.AccRnd == op.maxProm.AccRnd {
				syncIdx = pr.LogIdx
			} else {
				syncIdx = pr.DecIdx
			}
			go func(server int) {
				op.mu.Lock()
				syncArgs := AcceptSync{op.currentRnd, op.suffix(syncIdx), syncIdx}
				op.mu.Unlock()
				acceptedReply := &Accepted{}
				ok := op.sendAcceptSync(server, &syncArgs, acceptedReply)
				if ok {
					op.acceptedHandler(acceptedReply, server)
				}
			}(pr.F)
		}
	} else if op.checkState(LEADER, ACCEPT) {
		syncIdx := -1
		if reply.AccRnd == op.maxProm.AccRnd {
			syncIdx = op.maxProm.LogIdx
		} else {
			syncIdx = reply.DecIdx
		}
		// fmt.Printf("Promise | Server: %d maxProm: %v, received from %d, promise: %v \n", op.B.PID, op.maxProm, followerId, reply)
		// fmt.Printf("Promise | Server: %d send to %d syncIdx is: %d \n", op.B.PID, followerId, syncIdx)
		syncArgs := AcceptSync{op.currentRnd, op.suffix(syncIdx), syncIdx}

		// fmt.Printf("Promise | Server: %d send to %d syncArgs is: %v \n", op.B.PID, followerId, syncArgs)
		acceptedReply := &Accepted{}
		op.sendAcceptSync(followerId, &syncArgs, acceptedReply)
		// go func(server int) {
		// 	// op.mu.Lock()
		// 	syncArgs := AcceptSync{op.currentRnd, op.suffix(syncIdx), syncIdx}
		// 	// op.mu.Unlock()
		// 	// fmt.Printf("Promise | Server: %d send to %d syncArgs is: %v \n", op.B.PID, server, syncArgs)
		// 	acceptedReply := &Accepted{}
		// 	ok := op.sendAcceptSync(server, &syncArgs, acceptedReply)
		// 	if ok {
		// 		// op.mu.Lock()
		// 		// defer op.mu.Unlock()
		// 		// op.acceptedHandler(acceptedReply, server)
		// 	}
		// }(followerId)
		if op.DecidedIdx > reply.DecIdx {
			go func(server int) {
				op.mu.Lock()
				decideArgs := Decide{op.currentRnd, op.DecidedIdx}
				op.mu.Unlock()
				// fmt.Printf("Promise | Server: %d send to %d decideArgs is: %v \n", op.B.PID, server, decideArgs)
				ok := op.sendDecide(server, &decideArgs, &DummyReply{})
				if !ok {
					log.Error().Msgf("promiseHandler | Server: %d send decide to Server: %d fail", op.B.PID, server)
				}
			}(followerId)
		}
	}
}

func (op *OmniPaxos) acceptedHandler(reply *Accepted, followerId int) {
	if op.currentRnd != reply.N || !op.checkState(LEADER, ACCEPT) {
		return
	}
	// // fmt.Printf("Accepted | Server: %d received from %d LogIdx: %d \n", op.B.PID, followerId, reply.LogIdx)
	op.mu.Lock()
	op.accepted[followerId] = reply.LogIdx
	majority := op.acceptMajority(reply.LogIdx)
	if reply.LogIdx > op.DecidedIdx && majority {
		preDecIDx := op.DecidedIdx
		op.DecidedIdx = reply.LogIdx
		op.persist()
		// fmt.Printf("Proposal | Server: %d DecidedIdx after Accepted from %d is %d \n", op.B.PID, followerId, op.DecidedIdx)

		// time.Sleep(10 * time.Millisecond)
		for _, proE := range op.promises {
			followerId := proE.F
			if followerId == op.B.PID {
				continue
			}
			decArgs := Decide{N: op.currentRnd, DecIdx: op.DecidedIdx}
			// // fmt.Printf("Accepted | Server: %d send decide to %d, %v \n", op.B.PID, followerId, decArgs)
			go func(server int) {
				ok := op.sendDecide(server, &decArgs, &DummyReply{})
				if !ok {
					log.Error().Msgf("Proposal | Server: %d send decide to Server: %d fail", op.B.PID, server)
				}
			}(followerId)
		}
		// send logs to applyChan
		op.sendApplyMsg(preDecIDx, reply.LogIdx)
	}
	op.mu.Unlock()
}

func (op *OmniPaxos) sendPrepareRequest(server int, args *PrepareRequest, reply *DummyReply) bool {
	ok := op.peers[server].Call("OmniPaxos.PrepareRequestHandler", args, reply)
	return ok
}

func (op *OmniPaxos) PrepareRequestHandler(args *PrepareRequest, reply *DummyReply) {
	op.mu.Lock()
	defer op.mu.Unlock()
	if op.state.Role == LEADER {
		prepare := &Prepare{
			op.currentRnd,
			op.AcceptedRnd,
			len(op.Log),
			op.DecidedIdx,
		}
		go func() {
			promise := &Promise{}
			ok := op.sendPrepare(args.FollowerId, prepare, promise)
			if ok {
				op.mu.Lock()
				defer op.mu.Unlock()
				// // fmt.Printf("Leader: %d send prepare to %d success \n", op.B.PID, server)
				// fmt.Printf("PrepareRequest | Server: %d received promise: %v from %d \n", op.B.PID, promise, args.FollowerId)
				op.promiseHandler(promise, args.FollowerId)
			}
		}()
	}
}

// GetState Return the current leader's ballot and whether this server
// believes it is the leader.
func (op *OmniPaxos) GetState() (int, bool) {
	var ballot int
	var isleader bool

	// Your code here (2A).
	op.mu.Lock()
	defer op.mu.Unlock()
	ballot = op.L.Seq
	isleader = op.state.Role == LEADER

	return ballot, isleader
}

// Called by the tester to submit a log to your OmniPaxos server
// Implement this as described in Figure 3
func (op *OmniPaxos) Proposal(command interface{}) (int, int, bool) {
	// fmt.Printf("Proposal | Server: %d start Proposal %v \n", op.B.PID, command)
	op.mu.Lock()
	index := -1
	ballot := -1
	isLeader := false

	// Your code here (2B).

	if op.stopped() {
		return index, ballot, isLeader
	}
	// ballot, isLeader = op.GetState()
	// // fmt.Printf("Proposal | Server: %d ballot: %d, isLeader: %v \n", op.B.PID, ballot, isLeader)
	ballot = op.B.Seq
	isLeader = op.state.Role == LEADER
	if op.checkState(LEADER, PREPARE) {
		op.buffer = append(op.buffer, LogEntry{Valid: true, Command: command})
	} else if op.checkState(LEADER, ACCEPT) {
		op.Log = append(op.Log, LogEntry{Valid: true, Command: command})
		op.persist()
		// // fmt.Printf("Proposal | Server: %d insert log: %v, Log Length: %d \n", op.B.PID, op.Log[len(op.Log)-1], len(op.Log))
		op.accepted[op.B.PID] = len(op.Log)

		args := Accept{
			N: op.currentRnd,
			C: command,
		}
		var wg sync.WaitGroup
		for _, proE := range op.promises {
			followerId := proE.F
			if followerId == op.B.PID {
				continue
			}
			// fmt.Printf("Proposal | Server: %d send accept to %d, %v \n", op.B.PID, followerId, args)
			wg.Add(1)
			go func(server int) {
				reply := Accepted{}
				ok := op.sendAccept(server, &args, &reply)
				wg.Done()
				if ok {
					// op.mu.Lock()
					op.acceptedHandler(&reply, server)
					// op.mu.Unlock()
				}

			}(followerId)
		}
		wg.Wait()
	}
	// op.mu.Unlock()
	// fmt.Printf("Proposal | Server: %d DecidedIdx in Proposal is %d \n", op.B.PID, op.DecidedIdx)
	// op.mu.Lock()
	index = len(op.Log)
	op.mu.Unlock()
	// time.Sleep(100 * time.Millisecond)

	return index - 1, ballot, isLeader
}

// The service using OmniPaxos (e.g. a k/v server) wants to start
// agreement on the next command to be appended to OmniPaxos's log. If this
// server isn't the leader, returns false. Otherwise start the
// agreement and return immediately. There is no guarantee that this
// command will ever be committed to the OmniPaxos log, since the leader
// may fail or lose an election. Even if the OmniPaxos instance has been killed,
// this function should return gracefully.
//
// The first return value is the index that the command will appear at
// if it's ever committed. The second return value is the current
// ballot. The third return value is true if this server believes it is
// the leader.

// The tester doesn't halt goroutines created by OmniPaxos after each test,
// but it does call the Kill() method. Your code can use killed() to
// check whether Kill() has been called. The use of atomic avoids the
// need for a lock.
//
// The issue is that long-running goroutines use memory and may chew
// up CPU time, perhaps causing later tests to fail and generating
// confusing debug output. Any goroutine with a long-running loop
// should call killed() to check whether it should stop.
func (op *OmniPaxos) Kill() {
	atomic.StoreInt32(&op.dead, 1)
	// Your code here, if desired.
	// you may set a variable to false to
	// disable logs as soon as a server is killed
	atomic.StoreInt32(&op.enableLogging, 0)
}

func (op *OmniPaxos) Killed() bool {
	z := atomic.LoadInt32(&op.dead)
	return z == 1
}

func (op *OmniPaxos) increment() {
	op.B.Seq += 1
}

func (op *OmniPaxos) maxBallot() Ballot {
	max := Ballot{-2, -2}
	for _, ballotE := range op.ballots {
		if !ballotE.QC {
			continue
		}
		if ballotE.B.Seq > max.Seq {
			max = ballotE.B
		} else if ballotE.B.Seq == max.Seq && ballotE.B.PID > max.PID {
			max = ballotE.B
		}
	}
	return max
}

func (op *OmniPaxos) checkLeader() {
	maxBallot := op.maxBallot()
	// // fmt.Printf("Server: %d, maxBallot.Seq: %d, L.Seq: %d \n", op.B.PID, maxBallot.Seq, op.L.Seq)

	if compareBallot(maxBallot, op.L) < 0 {
		// op.increment()
		op.B.Seq = op.L.Seq + 1
		op.QC = true
		// // fmt.Printf("BLE | Server: %d QC=true \n", op.B.PID)
	} else if compareBallot(maxBallot, op.L) > 0 {
		op.L = maxBallot
		op.startLeader(maxBallot.PID, maxBallot)
		// op.startLeader(maxBallot.PID, Ballot{op.R, maxBallot.PID})
	}
}

func (op *OmniPaxos) startLeader(pid int, maxS Ballot) {
	// op.mu.Lock()
	// defer op.mu.Unlock()
	// op.state.Role = FOLLOWER
	if op.B.PID == pid && compareBallot(maxS, op.PromisedRnd) > 0 {
		// fmt.Printf("Leader | Server: %d is new Leader! \n", op.B.PID)
		op.resetLeader()
		op.state = State{LEADER, PREPARE}
		op.currentRnd = maxS
		op.PromisedRnd = maxS
		selfPromise := PromiseEntry{
			AccRnd: op.AcceptedRnd,
			LogIdx: len(op.Log),
			F:      op.B.PID,
			DecIdx: op.DecidedIdx,
			Sfx:    op.suffix(op.DecidedIdx),
		}
		op.promises[op.B.PID] = selfPromise
		op.persist()

		args := Prepare{
			N:      op.currentRnd,
			AccRnd: op.AcceptedRnd,
			LogIdx: len(op.Log),
			DecIdx: op.DecidedIdx,
		}
		for i := range op.peers {
			if i != op.me {
				// // fmt.Printf("peer: %v \n", i)
				go func(server int) {
					reply := &Promise{}
					ok := op.sendPrepare(server, &args, reply)
					if ok {
						op.mu.Lock()
						defer op.mu.Unlock()
						// fmt.Printf("Leader: %d send prepare %v to %d, receive %v \n", op.B.PID, args, server, reply)
						op.promiseHandler(reply, server)
					}
				}(i)
			}
		}
	}
	if op.state.Role == LEADER && op.B.PID != pid {
		op.Reconnect()
	}
}

func (op *OmniPaxos) Reconnect() {
	if op.state.Role == LEADER {
		op.state = State{FOLLOWER, RECOVER}
		// fmt.Printf("Reconnct | Server: %d state to FOLLOWER, RECOVER \n", op.B.PID)
	}
	for i := range op.peers {
		if i != op.me {
			go func(server int) {
				// // fmt.Printf("Server: %d, Rnd: %d, Reply: peer.PID: %d, peer.QC: %v \n", op.B.PID, op.R, reply.Ballot.PID, reply.QC)
				ok := op.sendPrepareRequest(server, &PrepareRequest{op.B.PID}, &DummyReply{})
				if !ok {
					log.Error().Msgf("Reconnect | Server: %d send PrepareRequest fail \n", op.B.PID)
				}
			}(i)
		}
	}
}

// save OmniPaxos's persistent state to stable storage,
// where it can later be retrieved after a crash and restart.
// see paper's Figure 3 &4 for a description of what should be persistent.
func (op *OmniPaxos) persist() {
	// Your code here (2C).
	// Example:
	w := new(bytes.Buffer)
	e := labgob.NewEncoder(w)
	e.Encode(op.L)
	e.Encode(op.Log)
	e.Encode(op.PromisedRnd)
	e.Encode(op.AcceptedRnd)
	e.Encode(op.DecidedIdx)
	data := w.Bytes()
	op.persister.SaveOmnipaxosState(data)
}

// restore previously persisted state.
func (op *OmniPaxos) readPersist(data []byte) bool {
	if data == nil || len(data) < 1 {
		return false
	}
	// Your code here (2C).
	// Example:
	r := bytes.NewBuffer(data)
	d := labgob.NewDecoder(r)
	var L Ballot
	var Log []LogEntry
	var PromisedRnd Ballot
	var AcceptedRnd Ballot
	var DecidedIdx int
	if d.Decode(&L) != nil ||
		d.Decode(&Log) != nil ||
		d.Decode(&PromisedRnd) != nil ||
		d.Decode(&AcceptedRnd) != nil ||
		d.Decode(&DecidedIdx) != nil {
		// fmt.Printf("readPersist FAILED!")
		return false
	} else {
		op.L = L
		op.Log = Log
		op.PromisedRnd = PromisedRnd
		op.AcceptedRnd = AcceptedRnd
		op.DecidedIdx = DecidedIdx
		op.sendApplyMsg(0, op.DecidedIdx)
		return true
	}
}

// The service or tester wants to create a OmniPaxos server. The ports
// of all the OmniPaxos servers (including this one) are in peers[]. This
// server's port is peers[me]. All the servers' peers[] arrays
// have the same order. persister is a place for this server to
// save its persistent state, and also initially holds the most
// recent saved state, if any. applyCh is a channel on which the
// tester or service expects OmniPaxos to send ApplyMsg messages.
// Make() must return quickly, so it should start goroutines
// for any long-running work.
func Make(peers []*labrpc.ClientEnd, me int,
	persister *Persister, applyCh chan ApplyMsg) *OmniPaxos {

	op := &OmniPaxos{}
	op.peers = peers
	op.persister = persister
	op.me = me
	op.applyCh = applyCh
	op.LinkDrop = false

	// Your initialization code here (2A, 2B).
	op.L = Ballot{Seq: -1, PID: -1}
	op.R = 0
	op.B = Ballot{Seq: 0, PID: me}
	op.QC = true
	op.delay = 100 * time.Millisecond
	op.ballots = make(map[int]BallotEntry)

	op.Log = []LogEntry{}
	op.PromisedRnd = Ballot{-1, -1}
	op.AcceptedRnd = Ballot{-1, -1}
	op.DecidedIdx = 0
	op.state = State{Role: FOLLOWER, Phase: PREPARE}
	needRecover := op.readPersist(persister.omnipaxosstate)
	op.persist()

	go func() {
		disconnectedRnd := 0
		for {
			// op.mu.Lock()

			if op.Killed() {
				// op.mu.Unlock()
				return
			}

			args := &HBRequest{
				Rnd: op.R,
			}

			disconnected := true
			var wg sync.WaitGroup
			for i := range op.peers {
				if i != op.me {
					wg.Add(1)
					go func(server int) {
						reply := &HBReply{}
						// // fmt.Printf("Server: %d, Rnd: %d, Reply: peer.PID: %d, peer.QC: %v \n", op.B.PID, op.R, reply.Ballot.PID, reply.QC)
						ok := op.sendHeartBeats(server, args, reply)
						if ok {
							// // fmt.Printf("HB | Server: %d, Rnd: %d, peer.PID: %d success, peer.QC: %v  \n", op.B.PID, op.R, server, reply.QC)
							op.mu.Lock()
							defer op.mu.Unlock()

							disconnected = false
							disconnectedRnd = 0
							if op.LinkDrop {
								op.LinkDrop = false
								// fmt.Printf("HB | Server: %d Reconnect, decIdx %d \n", op.B.PID, op.DecidedIdx)
								op.Reconnect()
							}
							if needRecover {
								// fmt.Printf("HB | Server: %d Recover, decIdx %d \n", op.B.PID, op.DecidedIdx)
								op.Reconnect()
								needRecover = false
							}

							if reply.Rnd == op.R {
								op.ballots[reply.Ballot.PID] = BallotEntry{B: reply.Ballot, QC: reply.QC}
							}
						} else {
							// // fmt.Printf("HB | Server: %d, Rnd: %d, peer.PID: %d failed \n", op.B.PID, op.R, server)
						}
						wg.Done()
					}(i)
				}
			}
			// op.mu.Unlock()
			wg.Wait()
			// op.startTimer(op.delay)

			op.mu.Lock()
			if disconnected {
				disconnectedRnd++
			}
			if disconnectedRnd >= 3 && !op.LinkDrop {
				// fmt.Printf("HB | Server: %d disconnected \n", op.B.PID)
				op.LinkDrop = true
				op.state.Role = FOLLOWER
			}

			op.ballots[op.B.PID] = BallotEntry{B: op.B, QC: op.QC}
			// // fmt.Printf("PID: %d, Rnd: %d, votes: %d/%d \n", op.B.PID, op.R, len(op.ballots), len(op.peers)/2+1)
			// // fmt.Printf("BLE | Server: %d ballots are: %v \n", op.B.PID, op.ballots)
			if len(op.ballots) >= len(op.peers)/2+1 {
				op.checkLeader()
			} else {
				op.QC = false
				// op.state.Role = FOLLOWER
				// // fmt.Printf("BLE | Server: %d QC=false \n", op.B.PID)
			}
			op.ballots = make(map[int]BallotEntry) // Clear ballots
			op.R += 1

			op.mu.Unlock()
			op.startTimer(op.delay)
			// // fmt.Printf("BLE | Server: %d timer end \n", op.B.PID)
		}
	}()

	return op
}

func (op *OmniPaxos) checkState(role Role, phase Phase) bool {
	return op.state.Role == role && op.state.Phase == phase
}

func (op *OmniPaxos) acceptMajority(logIdx int) bool {
	count := 0
	for _, value := range op.accepted {
		if value == logIdx {
			count++
		}
	}
	return count >= len(op.peers)/2+1
}

func (op *OmniPaxos) resetLeader() {
	op.currentRnd = Ballot{-1, -1}
	op.promises = map[int]PromiseEntry{}
	op.maxProm = PromiseEntry{}
	op.accepted = make([]int, len(op.peers))
	op.buffer = []LogEntry{}
}

func (op *OmniPaxos) stopped() bool {
	if op.Killed() {
		// fmt.Printf("Server: %d stopped! \n", op.B.PID)
	}
	return op.Killed()
}

func min(a int, b int) int {
	if a < b {
		return a
	}
	return b
}

func (op *OmniPaxos) prefix(idx int) []LogEntry {
	if idx == -1 {
		return []LogEntry{}
	}
	// end := idx
	// if len(op.Log) < end {
	// 	end = len(op.Log)
	// }
	return append([]LogEntry(nil), op.Log[:min(idx, len(op.Log))]...)
}

func (op *OmniPaxos) suffix(idx int) []LogEntry {
	if idx == -1 || idx >= len(op.Log) {
		return []LogEntry{}
	}
	return append([]LogEntry(nil), op.Log[idx:]...)
}

func (op *OmniPaxos) startTimer(delay time.Duration) {
	time.Sleep(delay)
}

func (op *OmniPaxos) sendApplyMsg(preDec int, newIdx int) {
	for i := preDec + 1; i <= newIdx; i++ {
		if i > len(op.Log) {
			break
		}
		applyMsg := ApplyMsg{
			CommandValid: true,
			Command:      op.Log[i-1].Command,
			CommandIndex: i - 1,
		}
		// fmt.Printf("Server %d send ApplyMsg: %v \n", op.B.PID, applyMsg)
		// Send ApplyMsg to applyCh
		op.applyCh <- applyMsg
	}
	// fmt.Printf("Server %d send ApplyMsg ended \n", op.B.PID)
}

func compareBallot(b1 Ballot, b2 Ballot) int {
	if b1.Seq < b2.Seq || (b1.Seq == b2.Seq && b1.PID < b2.PID) {
		return -1
	} else if b1.Seq > b2.Seq || (b1.Seq == b2.Seq && b1.PID > b2.PID) {
		return 1
	}
	return 0
}
