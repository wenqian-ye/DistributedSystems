package main

import (
	"bufio"
	"container/list"
	"fmt"
	"net"
	"os"
	"os/signal"
	"strings"
	"sync"
	"syscall"
	"time"
)

/*
Note:
	no need to support BEGIN in the middle of a
	transaction
*/

/*
Usage:
	run branches first, each branch should use the ports
	as defined in @param defaultBranchAddr

Protocol:
	To client: read and write as described in doc
	To branch:
		transactionId BEGIN
		transactionId ABORT
		transactionId COMMIT
		... (DEPOSIT, BALANCE, WITHDRAW)
	From branch

Actions:
	Upon receiving an action:
		transaction ...
*/

// Transaction : as defined in doc
type Transaction struct {
	id          string
	state       int
	msgList     *list.List
	cmdList     *list.List
	commitCount int
	abortCount  int
	mtx         sync.Mutex
}

/*Resource : all transactions are executed chronologically
 */
type Resource struct {
	nRead  int
	nWrite int
}

// RagEdge : each node has 2 lists of RagEdges
type RagEdge struct {
	node  string
	write bool
}

// RagNode : a node in resource allocation graph
type RagNode struct {
	inList, outList *list.List // list of RagEdge
}

// Branch : branch
type Branch struct {
	id       string
	conn     net.Conn
	writeMtx sync.Mutex
}

// Client : client
type Client struct {
	id                   string
	currentTransactionID string
	conn                 net.Conn
	readMtx, writeMtx    sync.Mutex
	cmdList              *list.List
	state                int
	interrupted          bool
}

const (
	transactionIdle     = 0
	transactionWaitRes  = 1
	transactionWaitExec = 2
	transactionAbort    = -1
	transactionAborted  = -2
	transactionCommit   = 3
	transactionCommited = 4
)

var defaultBranchAddr = [5]string{
	"172.22.156.112:5001",
	"172.22.156.112:5002",
	"172.22.156.112:5003",
	"172.22.156.112:5004",
	"172.22.156.112:5005"}
var branchName = [5]string{"A", "B", "C", "D", "E"}

var ragMtx sync.Mutex
var ragSet map[string]bool
var rag map[string]*RagNode
var transactionMap map[string]*Transaction
var resourceMap map[string]*Resource

var branches map[string]*Branch
var clients map[string]*Client

var interrupted bool = false

func getTimeString() string {
	return fmt.Sprintf("%f", float64(time.Now().UnixNano())/float64(time.Second))
}

func (transaction *Transaction) isInProgress() bool {
	return transaction.state == transactionIdle ||
		transaction.state == transactionWaitRes ||
		transaction.state == transactionWaitExec
}

/* s: source; resourceName: current node*/
func ragDfs(s, u string) bool {
	ragSet[u] = true
	if rag[u].outList.Len() > 0 {
		for ele := rag[u].outList.Front(); ele != nil; ele = ele.Next() {
			if ele.Value.(RagEdge).node == s {
				return true
			} else if !ragSet[ele.Value.(RagEdge).node] {
				if ragDfs(s, ele.Value.(RagEdge).node) {
					return true
				}
			}
		}
	}
	return false
}

// search from the edge <from, to>, check whether there is cycle
func ragHasCycle(from, to string) bool {
	ragSet = make(map[string]bool)
	ragSet[from] = true
	return ragDfs(from, to)
}

func ragAddEdge(from, to string, write bool) {
	rag[from].outList.PushBack(RagEdge{to, write})
	rag[to].inList.PushBack(RagEdge{from, write})
}

func ragRemoveEdge(from, to string) {
	for ele := rag[from].outList.Front(); ele != nil; ele = ele.Next() {
		if ele.Value.(RagEdge).node == to {
			rag[from].outList.Remove(ele)
			break
		}
	}
	for ele := rag[to].inList.Front(); ele != nil; ele = ele.Next() {
		if ele.Value.(RagEdge).node == from {
			rag[to].inList.Remove(ele)
			break
		}
	}
}

/*
@return : returns acquired, isWrite
*/
func lockAcquired(transactionID, resourceID string) (bool, bool) {
	for ele := rag[transactionID].inList.Front(); ele != nil; ele = ele.Next() {
		if ele.Value.(RagEdge).node == resourceID {
			return true, ele.Value.(RagEdge).write
		}
	}
	return false, false
}

func release(transaction *Transaction) {
	id := transaction.id
	for rag[id].outList.Len() > 0 { // quit wait
		ragRemoveEdge(id, rag[id].outList.Front().Value.(RagEdge).node)
	}
	for rag[id].inList.Len() > 0 { // release resources
		res := rag[id].inList.Front().Value.(RagEdge).node
		write := rag[id].inList.Front().Value.(RagEdge).write
		ragRemoveEdge(res, id)
		if write {
			resourceMap[res].nWrite--
		} else {
			resourceMap[res].nRead--
		}
		// check res state, wait list
		if rag[res].outList.Len() == 0 {
			// move in list to out list and notify
			if rag[res].inList.Len() > 0 {
				write = rag[res].inList.Front().Value.(RagEdge).write // next isWrite
				if write {
					resourceMap[res].nWrite++
				} else {
					resourceMap[res].nRead++
				}
				if write {
					nextTransaction := rag[res].inList.Front().Value.(RagEdge).node
					ragRemoveEdge(nextTransaction, res)
					ragAddEdge(res, nextTransaction, write)
					//rag[res].outList.PushBack(rag[res].inList.Front().Value)
					//rag[res].inList.Remove(rag[res].inList.Front())
					if transactionMap[nextTransaction].state == transactionWaitRes {
						transactionMap[nextTransaction].state = transactionWaitExec
					}
				} else { // read
					ele := rag[res].inList.Front()
					for ele != nil {
						next := ele.Next()
						nextTransaction := ele.Value.(RagEdge).node
						if !ele.Value.(RagEdge).write {
							//rag[res].outList.PushBack(ele.Value)
							//rag[res].inList.Remove(ele)
							ragRemoveEdge(nextTransaction, res)
							ragAddEdge(res, nextTransaction, write)
							if transactionMap[nextTransaction].state == transactionWaitRes {
								transactionMap[nextTransaction].state = transactionWaitExec
							}
						}
						ele = next
					}
				}
			}
		}
	}
	delete(rag, id)
	delete(transactionMap, id)
}

/*
@return:
	0 for lock success
	1 for wait
	-1 for deadlock
*/
func tryLock(transactionID, resourceID string, isWrite bool) int {
	ret := 0
	ragMtx.Lock()
	lastAcquired, lastIsWrite := lockAcquired(transactionID, resourceID)
	if lastAcquired && (lastIsWrite || !isWrite) {
		ragMtx.Unlock()
		return 0
	} else if lastAcquired && isWrite { // if has read access and need write access
		ragRemoveEdge(resourceID, transactionID)
		resourceMap[resourceID].nRead--
	}
	if !isWrite { // try read
		if resourceMap[resourceID].nWrite == 0 { // no need to wait
			ret = 0
			ragAddEdge(resourceID, transactionID, isWrite)
			resourceMap[resourceID].nRead++
		} else {
			ragAddEdge(transactionID, resourceID, isWrite)
			if ragHasCycle(transactionID, resourceID) {
				ret = -1
				ragRemoveEdge(transactionID, resourceID)
			} else {
				ret = 1
			}
		}
	} else { //try write
		if rag[resourceID].outList.Len() == 0 {
			resourceMap[resourceID].nWrite++
			ret = 0
			ragAddEdge(resourceID, transactionID, isWrite)
		} else {
			ragAddEdge(transactionID, resourceID, isWrite)
			if ragHasCycle(transactionID, resourceID) {
				ret = -1
				ragRemoveEdge(transactionID, resourceID)
			} else {
				ret = 1
			}
		}
	}
	ragMtx.Unlock()
	return ret
}

func tryAddNewResource(resourceName string) {
	_, exist := resourceMap[resourceName]
	if !exist {
		resourceMap[resourceName] = &Resource{
			nRead:  0,
			nWrite: 0,
		}
		rag[resourceName] = &RagNode{
			inList:  list.New(),
			outList: list.New(),
		}
	}
}

func addNewTransaction(transactionID string) {
	transactionMap[transactionID] = &Transaction{
		id:          transactionID,
		state:       transactionIdle,
		msgList:     list.New(),
		cmdList:     list.New(),
		commitCount: 0,
		abortCount:  0,
		mtx:         sync.Mutex{},
	}
	rag[transactionID] = &RagNode{
		inList:  list.New(),
		outList: list.New(),
	}
}

func broadcast(msg string) {
	for _, branch := range branches {
		branch.writeMtx.Lock()
		fmt.Fprintf(branch.conn, msg)
		branch.writeMtx.Unlock()
	}
}

func handleBranch(branch *Branch) {
	reader := bufio.NewReader(branch.conn)
	for !interrupted {
		msg, e := reader.ReadString('\n')
		if e != nil {
			fmt.Fprintf(os.Stderr, "Server: Branch disconnected\n")
			break
		}
		msg = msg[:len(msg)-1]
		if msg == "" {
			continue
		}
		dat := strings.SplitN(msg, " ", 2)
		id := dat[0]
		msg = dat[1]
		if _, exists := transactionMap[id]; exists {
			transactionMap[id].mtx.Lock()
			if msg == "COMMIT OK" {
				transactionMap[id].commitCount++
			} else if msg == "ABORTED" {
				transactionMap[id].abortCount++
				if transactionMap[id].state != transactionAbort {
					go abort(transactionMap[id])
				}
			} else {
				if _, exist := transactionMap[id]; exist {
					transactionMap[id].msgList.PushBack(msg)
				}
			}
			transactionMap[id].mtx.Unlock()
		}
	}
}

/*  */
func abort(transaction *Transaction) {
	transaction.mtx.Lock()
	transaction.state = transactionAbort
	transaction.mtx.Unlock()
	broadcast(fmt.Sprintf("%s ABORT\n", transaction.id))
	client := clients[strings.Split(transaction.id, "-")[0]]
	client.writeMtx.Lock()
	fmt.Fprintf(client.conn, "ABORTED\n")
	client.writeMtx.Unlock()
	ragMtx.Lock()
	release(transaction)
	ragMtx.Unlock()
}

/*
	Broadcast commit msg, wait for response,
	and decide whether to abort
*/
func commit(transaction *Transaction) {
	transaction.mtx.Lock()
	transaction.state = transactionCommit
	transaction.mtx.Unlock()
	broadcast(fmt.Sprintf("%s COMMIT\n", transaction.id))
	for transaction.abortCount == 0 && transaction.commitCount != 5 {
		time.Sleep(1000 * time.Microsecond)
	}
	if transaction.abortCount == 0 {
		client := clients[strings.Split(transaction.id, "-")[0]]
		client.writeMtx.Lock()
		fmt.Fprintf(client.conn, "COMMIT OK\n")
		client.writeMtx.Unlock()
		ragMtx.Lock()
		release(transaction)
		ragMtx.Unlock()
	}
}

func execCmd(transactionID, cmd string) {
	dat := strings.Split(cmd, " ")
	branchName := strings.Split(dat[1], ".")[0]
	branches[branchName].writeMtx.Lock()
	fmt.Fprintf(branches[branchName].conn, "%s %s\n", transactionID, cmd)
	branches[branchName].writeMtx.Unlock()
}

func (client *Client) getTransactionID() string {
	return fmt.Sprintf("%s-%s", client.id, client.currentTransactionID)
}

// ReadMsg, if read abort, then delete all msg before next begin
func clientMsgReader(client *Client) {
	reader := bufio.NewReader(client.conn)
	for !interrupted && !client.interrupted {
		cmd, e := reader.ReadString('\n')
		if e != nil {
			fmt.Fprintf(os.Stderr, "Server: Client disconnected\n")
			client.interrupted = true
			id := client.getTransactionID()
			if transactionMap[id].state != transactionAbort {
				go abort(transactionMap[id])
			}
			break
		}
		cmd = cmd[:len(cmd)-1]
		if cmd == "" {
			continue
		}
		client.readMtx.Lock()
		if cmd == "ABORT" {
			tid := client.getTransactionID()
			if transactionMap[tid].isInProgress() {
				for client.cmdList.Len() > 0 &&
					client.cmdList.Front().Value != "BEGIN" {
					client.cmdList.Remove(client.cmdList.Front())
				}
				go abort(transactionMap[tid])
			}
		} else {
			client.cmdList.PushBack(cmd)
		}
		client.readMtx.Unlock()
	}
}

func handleClient(client *Client) {
	go clientMsgReader(client)
	addNewTransaction(client.getTransactionID())
	for !interrupted && !client.interrupted {
		transaction, exists := transactionMap[client.getTransactionID()]
		state := 0
		if exists {
			state = transaction.state
		}
		if !exists || (state == transactionIdle || !transaction.isInProgress()) {
			client.readMtx.Lock()
			if client.cmdList.Len() == 0 {
				client.readMtx.Unlock()
				continue
			}
			cmd := client.cmdList.Front().Value.(string)
			client.cmdList.Remove(client.cmdList.Front())
			client.readMtx.Unlock()
			if cmd == "BEGIN" {
				client.currentTransactionID = getTimeString()
				addNewTransaction(client.getTransactionID())
			} else if cmd == "COMMIT" && exists {
				go commit(transactionMap[client.getTransactionID()])
			} else if exists {
				transaction := transactionMap[client.getTransactionID()]
				transaction.cmdList.PushBack(cmd)
				dat := strings.Split(cmd, " ")
				isWrite := dat[0] != "BALANCE"
				tryAddNewResource(dat[1])
				ret := tryLock(transaction.id, dat[1], isWrite)
				if ret == -1 { //deadlock
					go abort(transaction)
				} else if ret == 0 {
					//exec
					execCmd(transaction.id, cmd)
					for {
						transaction.mtx.Lock()
						if !transaction.isInProgress() {
							transaction.mtx.Unlock()
							break
						}
						if transaction.msgList.Len() != 0 {
							client.writeMtx.Lock()
							fmt.Fprintf(client.conn, "%s\n", transaction.msgList.Front().Value.(string))
							client.writeMtx.Unlock()
							transaction.msgList.Remove(transaction.msgList.Front())
							transaction.mtx.Unlock()
							break
						}
						time.Sleep(10 * time.Microsecond)
						transaction.mtx.Unlock()
					}
				} else if ret == 1 { //wait for resource
					transaction.state = transactionWaitRes
				}
			}
		} else if state == transactionWaitRes {
			time.Sleep(10 * time.Microsecond)
		} else if state == transactionWaitExec {
			cmd := transaction.cmdList.Back().Value.(string)
			execCmd(transaction.id, cmd)
			for {
				transaction.mtx.Lock()
				if !transaction.isInProgress() {
					transaction.mtx.Unlock()
					break
				}
				if transaction.msgList.Len() != 0 {
					client.writeMtx.Lock()
					fmt.Fprintf(client.conn, "%s\n", transaction.msgList.Front().Value.(string))
					client.writeMtx.Unlock()
					transaction.msgList.Remove(transaction.msgList.Front())
					transaction.mtx.Unlock()
					break
				}
				time.Sleep(10 * time.Microsecond)
				transaction.mtx.Unlock()
			}
			transaction.state = transactionIdle
		}
	}
}

func main() {
	// init
	ragSet = make(map[string]bool)
	rag = make(map[string]*RagNode)
	transactionMap = make(map[string]*Transaction)
	resourceMap = make(map[string]*Resource)
	branches = make(map[string]*Branch)
	clients = make(map[string]*Client)
	// signal
	sig := make(chan os.Signal, 1)
	signal.Notify(sig, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-sig
		interrupted = true
	}()
	// connect to branches

	for i, addr := range defaultBranchAddr {
		newConn, dialErr := net.Dial("tcp", addr)
		if dialErr != nil {
			fmt.Fprintf(os.Stderr, "Server: Error connecting to branch\n")
			os.Exit(1)
		}
		branch := &Branch{
			id:       branchName[i],
			conn:     newConn,
			writeMtx: sync.Mutex{},
		}
		branches[branchName[i]] = branch
		go handleBranch(branches[branchName[i]])
	}
	// accept clients
	listener, listenErr := net.Listen("tcp", ":4999")
	if listenErr != nil {
		fmt.Fprintf(os.Stderr, "Server: Error listening\n")
		os.Exit(1)
	}
	for !interrupted {
		newConn, acceptErr := listener.Accept()
		if acceptErr != nil {
			fmt.Fprintf(os.Stderr, "Server: Error accepting\n")
			os.Exit(1)
		}
		client := &Client{
			id:                   newConn.RemoteAddr().String(),
			conn:                 newConn,
			currentTransactionID: getTimeString(),
			readMtx:              sync.Mutex{},
			writeMtx:             sync.Mutex{},
			cmdList:              list.New(),
			interrupted:          false,
		}
		clients[client.id] = client
		ragMtx.Lock()
		rag[client.id] = &RagNode{list.New(), list.New()}
		ragMtx.Unlock()
		go handleClient(client)
	}
}
