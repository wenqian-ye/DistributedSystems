package main

import (
	"bufio"
	"bytes"
	"container/list"
	"crypto/sha256"
	"encoding/gob"
	"encoding/hex"
	"fmt"
	"math/rand"
	"net"
	"os"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

/*
	We use lowercase for user defined commands
	Define message: connect ip:port
		the sender sends "connect" to receiver if intro service
		introduces receiver to sender
	Define message: dial ip:port
		upon receiving "connect" msg, the ipaddr of sender of "connect" is
		forwarded to the receiver's out nodes, and then the out nodes dial
		the sender

	Define message: request
	Define message: introduce ip:port

	Define message: block prevblock solution height hash TRANSACTION... TRANSACTION...

	Logger
		Part 1 evaluation
			B timestamp length
			T nodename timestamp TRANSACTION createtime
		Part 2 evaluation
			BLK nodename timestamp hash
			TB nodename addTimestamp receiveTimestamp TRANSACTION...
			CS nodename timestamp length hash1 hash2
*/

//introServiceAddr : IP address and port of intro service
const introServiceAddr string = "172.22.156.112:4999"
const loggingServiceAddr string = "172.22.156.112:4998"

// maxConnIn : max nodes accepted (receive msg)
// maxConnOut : max nodes dialed (send msg)
const maxConnIn, maxConnOut int = 127, 14
const miningSolutionTimeout float64 = 120
const acceptBlockTimeout float64 = 1

type Connection struct {
	conn net.Conn
	addr string
}

type Transaction struct {
	msg            string
	receivedTime   float64
	putInBlockTime string
	shouldFlood    bool
	used           bool
}

type BlockHead struct {
	PrevHash     string
	Transactions []string
}

type Block struct {
	Head      BlockHead
	solution  string
	height    int
	received  bool
	verified  bool
	accepted  bool
	hash      string
	timestamp string
}

//node info and stats
var nodeName, nodeListenIPAddr, nodeListenPort string
var interrupted bool = false

// log service
var logServiceConn net.Conn
var logMtx sync.Mutex

// intro service
var introServiceConnMtx sync.Mutex
var introServiceConn net.Conn
var introServiceReader *bufio.Reader

//connections
var connListIn, connListOut *list.List
var connInMtx, connOutMtx sync.Mutex

// request new node vars
var requestNodeCond *sync.Cond

// multicast (broadcast)
var multicastMsgList *list.List
var multicastCond *sync.Cond
var multicastMtx sync.Mutex

// transaction
var transMap map[string]Transaction // key -- ID, val -- msg
var transMapMtx sync.Mutex

// balance
var account map[string]int

// block chain
var maxBlockChainHeight int = 0
var maxLenBlockHash string = ""
var blockMap map[string]Block // key -- block hash, val -- block
var blockMapMtx sync.Mutex
var acceptCond *sync.Cond
var zeroBlock Block

var currentSolvingHash, currentSolution string

func generateHash(blk Block) string {
	b := bytes.Buffer{}
	e := gob.NewEncoder(&b)
	err := e.Encode(blk.Head)
	if err != nil {
		panic(err)
	}
	h := sha256.New()
	h.Write(b.Bytes())
	return hex.EncodeToString(h.Sum(nil))
}

func serializeBlock(blk Block) []byte {
	var b bytes.Buffer
	e := gob.NewEncoder(&b)
	err := e.Encode(blk)
	if err != nil {
		panic(err)
	}
	return b.Bytes()
}

// block prevhash solution height hash tranlist[]
func blockToMsg(blk Block) string {
	msg := "block " + blk.Head.PrevHash + " " + blk.solution + " " + strconv.Itoa(blk.height) + " " + blk.hash
	transLen := len(blk.Head.Transactions)
	i := 0
	for i < transLen {
		msg += " " + blk.Head.Transactions[i]
		i++
	}
	return msg
}

func msgToBlock(msg string) Block {
	dat := strings.Split(msg, " ")
	heightInt, _ := strconv.Atoi(dat[3])
	i := 5
	var s []string
	for i < len(dat) {
		transStr := "TRANSACTION " + dat[i+1] + " " + dat[i+2] + " " + dat[i+3] + " " + dat[i+4] + " " + dat[i+5]
		s = append(s, transStr)
		i += 6
	}
	blk := Block{Head: BlockHead{PrevHash: dat[1], Transactions: s}, solution: dat[2], height: heightInt, verified: false, hash: dat[4]}

	return blk
}

func updateAccount(trans string) bool {
	dat := strings.Split(trans, " ")
	src := dat[3]
	dst := dat[4]
	amountInt, _ := strconv.Atoi(dat[5])
	if src == "0" {
		account[dst] += amountInt
		return true
	}
	if account[src]-amountInt < 0 {
		return false
	}
	account[src] -= amountInt
	account[src] += amountInt
	return true
}

func recursiveUpdateAccount(curBlock Block) bool {
	// trace back to last block
	if curBlock.Head.PrevHash != "" {
		res := recursiveUpdateAccount(blockMap[curBlock.Head.PrevHash])
		if res == false {
			return false
		}
	}
	// calculate
	i := 0
	for i < len(curBlock.Head.Transactions) {
		if !updateAccount(curBlock.Head.Transactions[i]) {
			return false
		}
		i++
	}
	return true
}

func updateAccountFromChain(curBlock Block) bool {
	account = make(map[string]int)
	return recursiveUpdateAccount(curBlock)
}

func getUsableTransactions() []string {
	transMapMtx.Lock()
	trans := make([]Transaction, 0)
	for _, transaction := range transMap {
		if !transaction.used {
			trans = append(trans, transaction)
		}
	}
	sort.Slice(trans, func(p, q int) bool { return trans[p].receivedTime < trans[q].receivedTime })
	transArr := make([]string, len(trans))
	i := 0
	for i < len(trans) {
		transArr[i] = trans[i].msg
		i++
	}
	transMapMtx.Unlock()
	return transArr
}

func askSolveBlock(block Block) {
	introServiceConnMtx.Lock()
	fmt.Fprintf(introServiceConn, "SOLVE %s\n", block.hash)
	introServiceConnMtx.Unlock()
}

func askVerifyBlock(block Block) {
	puzzle := block.hash
	solution := block.solution
	introServiceConnMtx.Lock()
	fmt.Fprintf(introServiceConn, "VERIFY %s %s\n", puzzle, solution)
	introServiceConnMtx.Unlock()
}

//BLK nodename timestamp hash
func logBlock(block Block) {
	logToService("BLK %s %s %s\n", nodeName, block.timestamp, block.hash)
}

func miningService() {
	for !interrupted {
		blockTransactions := make([]string, 0)
		// get usable transactions
		transactions := getUsableTransactions()
		if len(transactions) == 0 {
			time.Sleep(100 * time.Millisecond)
			continue
		}
		blockMapMtx.Lock()
		if maxLenBlockHash == "" {
			updateAccountFromChain(zeroBlock)
		} else {
			lastBlock := blockMap[maxLenBlockHash]
			updateAccountFromChain(lastBlock)
		}
		for i := 0; i < len(transactions) && i < 2000; i++ {
			if updateAccount(transactions[i]) {
				blockTransactions = append(blockTransactions, transactions[i])
				transMapMtx.Lock()
				id := strings.Split(transactions[i], " ")[2]
				trans := transMap[id]
				trans.used = true
				transMap[id] = trans
				transMapMtx.Unlock()
			}
		}
		if len(blockTransactions) == 0 {
			blockMapMtx.Unlock()
			time.Sleep(100 * time.Millisecond)
			continue
		}
		// create new block
		block := Block{}
		block.Head.PrevHash = maxLenBlockHash
		block.Head.Transactions = blockTransactions
		block.height = maxBlockChainHeight + 1
		block.timestamp = getTimeString()
		block.hash = generateHash(block)
		block.solution = ""
		currentSolvingHash = block.hash
		currentSolution = ""
		blockMapMtx.Unlock()
		askSolveBlock(block)
		fmt.Fprintf(os.Stderr, "%s--- Waiting for solution %s\n", nodeName, getTimeString())
		//wait for solution
		start := getTime()
		canCommit := true

		for getTime()-start < miningSolutionTimeout {
			blockMapMtx.Lock()
			if block.height != maxBlockChainHeight+1 {
				canCommit = false
				blockMapMtx.Unlock()
				fmt.Fprintf(os.Stderr, "%s--- discarded due to higher chain %s\n", nodeName, getTimeString())
				break
			} else if currentSolvingHash == block.hash && currentSolution != "" {
				block.solution = currentSolution
				blockMapMtx.Unlock()
				break
			}
			blockMapMtx.Unlock()
			time.Sleep(time.Millisecond * 100)
		}
		if block.solution == "" {
			canCommit = false
			fmt.Fprintf(os.Stderr, "%s--- discarded due to timeout %s\n", nodeName, getTimeString())

		}
		block.received = true
		block.verified = true
		block.accepted = true
		if canCommit {
			/*
				fmt.Println("New block hash and solution")
				fmt.Println(block.hash)
				fmt.Println(block.solution)
			*/
			blockMapMtx.Lock()
			blockMap[block.hash] = block
			if maxBlockChainHeight <= block.height {
				maxBlockChainHeight = block.height
				maxLenBlockHash = block.hash
			}
			blockMapMtx.Unlock()
			transMapMtx.Lock()
			for i := range block.Head.Transactions {
				id := strings.Split(block.Head.Transactions[i], " ")[2]
				trans := transMap[id]
				trans.putInBlockTime = getTimeString()
				logToService("TB %s %s %f\n", nodeName, trans.putInBlockTime, trans.receivedTime)
				trans.used = true
				transMap[id] = trans
			}
			transMapMtx.Unlock()
			block.timestamp = getTimeString()
			logBlock(block)
			multicast(blockToMsg(block))
			fmt.Fprintf(os.Stderr, "%s--- Created block with %d transactions, height = %d\n", nodeName, len(block.Head.Transactions), block.height)
		}
		time.Sleep(100 * time.Millisecond)
	}
}

// go routine
func chainSplit(hash1 string, hash2 string, timeStamp string) {
	var block1, block2, nextBlock1, nextBlock2 Block
	var exist1, exist2 bool
	length := 0
	if hash1 != "" {
		length = 1
		blockMapMtx.Lock()
		block1, exist1 = blockMap[hash1]
		block2, exist2 = blockMap[hash2]
		blockMapMtx.Unlock()
		for block1.Head.PrevHash != "" {
			blockMapMtx.Lock()
			nextBlock1, exist1 = blockMap[block1.Head.PrevHash]
			nextBlock2, exist2 = blockMap[block2.Head.PrevHash]
			blockMapMtx.Unlock()
			length++
			if !exist1 {
				waitAccept(block1, acceptBlockTimeout)
				blockMapMtx.Lock()
				nextBlock1, exist1 = blockMap[block1.Head.PrevHash]
				blockMapMtx.Unlock()
			}
			if !exist2 {
				waitAccept(block2, acceptBlockTimeout)
				blockMapMtx.Lock()
				nextBlock2, exist2 = blockMap[block2.Head.PrevHash]
				blockMapMtx.Unlock()
			}
			if !exist1 || !exist2 {
				break
			}
			block1 = nextBlock1
			block2 = nextBlock2
		}
	}
	logToService("CS %s %s %d %s %s", nodeName, timeStamp, length, hash1, hash2)
}

func updateTransactionUsedFromBlock(block Block) {
	n := len(block.Head.Transactions)
	transMapMtx.Lock()
	for i := 0; i < n; i++ {
		msg := block.Head.Transactions[i]
		id := strings.Split(msg, " ")[2]
		t, exists := transMap[id]
		if exists {
			t.used = true
			transMap[id] = t
		} else {
			transMap[id] = Transaction{msg, 0, "", true, true}
		}
	}
	transMapMtx.Unlock()
}

func waitAccept(block Block, timeOut float64) bool {
	var lastBlock Block
	var exists bool
	start := getTime()
	acceptCond.L.Lock()
	if blockMap[block.hash].accepted {
		acceptCond.L.Unlock()
		acceptCond.Broadcast()
		return true
	}
	if block.Head.PrevHash == "" {
		block.accepted = true
		updateTransactionUsedFromBlock(block)
		blockMap[block.hash] = block
		acceptCond.L.Unlock()
		acceptCond.Broadcast()
		return true
	}
	for getTime()-start < timeOut {
		lastBlock, exists = blockMap[block.Head.PrevHash]
		if !exists || lastBlock.accepted == false {
			acceptCond.Wait()
		} else {
			break
		}
	}
	lastBlock, exists = blockMap[block.Head.PrevHash]
	if exists && lastBlock.accepted {
		block.accepted = true
		updateTransactionUsedFromBlock(block)
		blockMap[block.hash] = block
		acceptCond.Broadcast()
	}
	acceptCond.L.Unlock()
	return block.accepted
}

func initServices(nodeName string, addr string, port string) {
	// important messages for evaluation are sent to logging service
	var e error
	logServiceConn, e = net.Dial("tcp", loggingServiceAddr)
	if e != nil {
		fmt.Fprintf(os.Stderr, "%s--- Warning: cannot connect to log service\n", nodeName)
	} else {
		//logToService("CONNECT %s %s %s\n", nodeName, addr, port)
	}
	// connect to intro service
	introServiceConn, e = net.Dial("tcp", introServiceAddr)
	if e != nil {
		fmt.Fprintf(os.Stderr, "%s--- Error connecting intro service\n", nodeName)
		os.Exit(1)
	}
	introServiceReader = bufio.NewReader(introServiceConn)
	fmt.Fprintf(introServiceConn, "CONNECT %s %s %s\n", nodeName, addr, port)
}

func handleConn(conn net.Conn, element *list.Element) {
	reader := bufio.NewReader(conn)
	for !interrupted {
		msg, err := reader.ReadString('\n')
		if err != nil {
			connInMtx.Lock()
			connListIn.Remove(element)
			fmt.Fprintf(os.Stderr, "%s--- Connection to node lost\n", nodeName)
			connInMtx.Unlock()
			return
		}
		msg = msg[:len(msg)-1]
		logToService("B %s %d\n", getTimeString(), len(msg)+1)
		//fmt.Fprintf(os.Stderr, "%s received msg: %s\n", nodeName, msg)
		if strings.HasPrefix(msg, "connect ") {
			addr := strings.Split(msg, " ")[1]
			// save addr
			connInMtx.Lock()
			element.Value = Connection{element.Value.(Connection).conn, addr}
			connInMtx.Unlock()
			// ask out nodes to dial connect
			connOutMtx.Lock()
			if connListOut.Len() == 0 {
				go tryDial(addr, true, false)
			} else {
				multicast("dial " + addr)
			}
			connOutMtx.Unlock()
		} else if strings.HasPrefix(msg, "TRANSACTION ") {
			// if has been received then ignore
			// else add to dict and multicast transaction
			hash := strings.Split(msg, " ")[2]
			transMapMtx.Lock()
			trans, exist := transMap[hash]
			if !exist || trans.shouldFlood {
				transMap[hash] = Transaction{msg, getTime(), "", false, false}
				transMapMtx.Unlock()
				logToService("T %s %s TRANSACTION %s\n", nodeName, getTimeString(), strings.Split(msg, " ")[1])
				multicast(msg)
			} else {
				transMapMtx.Unlock()
			}
		} else if strings.HasPrefix(msg, "dial ") {
			go tryDial(strings.Split(msg, " ")[1], false, false)
		} else if msg == "request" {
			// randomly select a node
			connOutMtx.Lock()
			connInMtx.Lock()
			totalNodeNum := connListIn.Len() + connListOut.Len()
			var addr string = ""
			var ele *list.Element
			if totalNodeNum != 0 {
				randNum := rand.Intn(totalNodeNum)
				if randNum < connListOut.Len() {
					ele = connListOut.Front()
				} else {
					randNum -= connListOut.Len()
					ele = connListIn.Front()
				}
				for randNum != 0 {
					ele = ele.Next()
					randNum--
				}
				addr = ele.Value.(Connection).addr
			}
			connInMtx.Unlock()
			fmt.Fprintf(conn, "introduce %s\n", addr)
			connOutMtx.Unlock()
		} else if strings.HasPrefix(msg, "block ") {
			blk := msgToBlock(msg)
			blk.accepted = false
			blk.verified = false
			blk.timestamp = getTimeString()
			// if has been received then ignore
			// else add to dict and multicast transaction
			blockMapMtx.Lock()
			_, exist := blockMap[blk.hash]
			if !exist {
				blockMap[blk.hash] = blk
				blockMapMtx.Unlock()
				logBlock(blk)
				askVerifyBlock(blk)
				logToService("B %s %s %s\n", nodeName, getTimeString(), blk.hash)
				multicast(msg)
			} else {
				blockMapMtx.Unlock()
			}
		}
	}
}

func listenService(serverConn net.Listener) {
	for !interrupted {
		conn, e := serverConn.Accept()
		if e != nil {
			fmt.Fprintf(os.Stderr, "%s--- Error accepting client\n", nodeName)
			os.Exit(1)
		}
		connInMtx.Lock()
		if connListIn.Len() < maxConnIn {
			element := connListIn.PushBack(Connection{conn, ""})
			go handleConn(conn, element)
		} else {
			conn.Close() //this will never happen in our tests
		}
		connInMtx.Unlock()
	}
}

func multicastService() {
	for !interrupted {
		multicastCond.L.Lock()
		for multicastMsgList.Len() == 0 {
			multicastCond.Wait()
		}
		msg := multicastMsgList.Front().Value.(string)
		multicastMsgList.Remove(multicastMsgList.Front())
		multicastCond.L.Unlock()
		connOutMtx.Lock()
		ele := connListOut.Front()
		for ele != nil {
			next := ele.Next()
			_, e := fmt.Fprintf(ele.Value.(Connection).conn, msg)
			if e != nil {
				connListOut.Remove(ele)
				fmt.Fprintf(os.Stderr, "%s--- connection lost, out degree: %d\n", nodeName, connListOut.Len())
				requestNodeCond.Signal()
			}
			ele = next
		}
		connOutMtx.Unlock()
	}
}

func multicast(msg string) {
	if len(msg) > 0 && msg[len(msg)-1] != '\n' {
		msg = msg + "\n"
	}
	multicastCond.L.Lock()
	multicastMsgList.PushBack(msg)
	multicastCond.L.Unlock()
	multicastCond.Signal()
}

func requestNewNodeService() {
	for !interrupted {
		requestNodeCond.L.Lock() // requestNodeCond.L == connOutMtx
		for connListOut.Len() == maxConnOut {
			requestNodeCond.Wait()
		}
		// send request to random node
		nodeNum := connListOut.Len()
		var conn net.Conn = nil
		var ele *list.Element
		if nodeNum != 0 {
			randNum := rand.Intn(nodeNum)
			ele = connListOut.Front()
			for randNum != 0 {
				ele = ele.Next()
				randNum--
			}
			conn = ele.Value.(Connection).conn
			fmt.Fprintf(conn, "request\n")
		}
		requestNodeCond.L.Unlock()
		// read "introduction" and dial
		if conn != nil {
			reader := bufio.NewReader(conn)
			msg, err := reader.ReadString('\n')
			if err != nil {
				fmt.Fprintf(os.Stderr, "%s--- receive introduce from peer failed\n", nodeName)
				//time.Sleep(3 * time.Millisecond)
			} else if strings.HasPrefix(msg, "introduce ") {
				msg = msg[:len(msg)-1]
				addr := strings.Split(msg, " ")[1]
				if addr != "" { // the node being requested cannot return connection
					tryDial(strings.Split(msg, " ")[1], false, false)
				} else {
					time.Sleep(3000 * time.Microsecond)
				}
			}
			time.Sleep(100 * time.Microsecond)
		} else {
			time.Sleep(100 * time.Microsecond)
		}
	}
}

/* addr has the form "ip:port"*/
func tryDial(addr string, forceDial bool, fromIntroServer bool) {
	// check if addr already dialed
	connOutMtx.Lock()
	for ele := connListOut.Front(); ele != nil; ele = ele.Next() {
		if addr == ele.Value.(Connection).addr {
			connOutMtx.Unlock()
			return
		}
	}
	connOutMtx.Unlock()
	// dial
	conn, e := net.Dial("tcp", addr)
	if e != nil {
		fmt.Fprintf(os.Stderr, "%s--- Cannot connect to %s\n", nodeName, addr)
	} else {
		connOutMtx.Lock()
		if connListOut.Len() < maxConnOut {
			connListOut.PushBack(Connection{conn, addr})
		} else {
			if forceDial {
				// close and delete random conn
				randNum := rand.Intn(connListOut.Len())
				ele := connListOut.Front()
				for randNum != 0 {
					ele = ele.Next()
					randNum--
				}
				ele.Value.(Connection).conn.Close()
				connListOut.Remove(ele)
				connListOut.PushBack(Connection{conn, addr})
			} else {
				conn.Close()
			}
		}
		connOutMtx.Unlock()
		if fromIntroServer { // then conn is not closed
			fmt.Fprintf(conn, "connect %s:%s\n", nodeListenIPAddr, nodeListenPort)
		}
	}
}

func quit() {
	interrupted = true
}

func die() {
	os.Exit(2)
}

func logToService(format string, a ...interface{}) {
	logMtx.Lock()
	if logServiceConn != nil {
		fmt.Fprintf(logServiceConn, format, a...)
	}
	logMtx.Unlock()
}

func getTimeString() string {
	return fmt.Sprintf("%f", float64(time.Now().UnixNano())/float64(time.Second))
}

func getTime() float64 {
	return float64(time.Now().UnixNano()) / float64(time.Second)
}

func main() {
	// args
	argv := os.Args
	if len(argv) < 4 {
		fmt.Fprintf(os.Stderr, "Usage: ./mp2 <node name> <ipAddr> <port>\n")
		os.Exit(1)
	}
	nodeName = argv[1]
	nodeListenIPAddr = argv[2]
	nodeListenPort = argv[3]
	// init
	connListIn = list.New()
	connListOut = list.New()
	multicastMsgList = list.New()
	multicastCond = sync.NewCond(&multicastMtx)
	requestNodeCond = sync.NewCond(&connOutMtx)
	acceptCond = sync.NewCond(&blockMapMtx)
	transMap = make(map[string]Transaction)
	blockMap = make(map[string]Block)
	zeroBlock.Head.PrevHash = ""
	zeroBlock.accepted = true
	zeroBlock.height = 0
	// create TCP server
	server, e := net.Listen("tcp", ":"+argv[3])
	if e != nil {
		fmt.Fprintf(os.Stderr, "%s--- Error creating TCP server", nodeName)
	}
	initServices(argv[1], argv[2], argv[3])
	go listenService(server)
	// start services
	// start go routine
	go multicastService()
	go requestNewNodeService()
	go miningService()
	// handle messages from introService
	for !interrupted {
		msg, err := introServiceReader.ReadString('\n')
		if err != nil {
			fmt.Fprintf(os.Stderr, "%s--- Connection to intro service lost\n", nodeName)
			os.Exit(1)
		}
		if len(msg) == 0 {
			continue
		}
		msg = msg[:len(msg)-1]
		logToService("B %s %d\n", getTimeString(), len(msg)+1)
		//fmt.Fprintf(os.Stderr, "%s received msg from intro: %s\n", nodeName, msg)
		if msg == "QUIT" {
			quit()
		} else if msg == "DIE" {
			die()
		} else {
			dat := strings.Split(msg, " ")
			if dat[0] == "INTRODUCE" {
				go tryDial(dat[2]+":"+dat[3], true, true)
			} else if dat[0] == "SOLVED" {
				blockMapMtx.Lock()
				if currentSolvingHash == dat[1] {
					currentSolution = dat[2]
				}
				blockMapMtx.Unlock()
			} else if dat[0] == "VERIFY" {
				blockHash := dat[2]
				if dat[1] == "OK" {
					// compare length of block chain
					blockMapMtx.Lock()
					height := blockMap[blockHash].height
					if height > maxBlockChainHeight {
						maxBlockChainHeight = height
						maxLenBlockHash = blockHash
					} else if height == maxBlockChainHeight {
						hash1 := maxLenBlockHash
						hash2 := blockMap[blockHash].hash
						go chainSplit(hash1, hash2, getTimeString())
					}
					block := blockMap[blockHash]
					block.verified = true
					blockMap[blockHash] = block
					go waitAccept(block, acceptBlockTimeout)
					blockMapMtx.Unlock()
				} else if dat[1] == "FAIL" {
					// do nothing
				}
			} else if dat[0] == "TRANSACTION" {
				transMapMtx.Lock()
				transMap[dat[2]] = Transaction{msg, getTime(), "", false, false}
				transMapMtx.Unlock()
				logToService("T %s %s TRANSACTION %s\n", nodeName, getTimeString(), strings.Split(msg, " ")[1])
				multicast(msg)
			}
		}
	}
}
