package main

import (
	"bufio"
	"container/heap"
	"fmt"
	"net"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"
    "os/signal"
    "syscall"
)

/**
	M [pid] [ord] [pri] msg
	P [pid] [ord] [pri]					//Propose
	F [pid]	[ord] [pri]					//Final
*/

/** Debug */
var countPyMsg int = 0
var countDeliveredMsg int = 0

const argNumNode int = 2
var path1 = "bandwidth.txt"
var path2 = "log.txt"
var file1, err_f1 = os.OpenFile(path1, os.O_RDWR|os.O_CREATE|os.O_TRUNC, 0666)
var file2, err_f2 = os.OpenFile(path2, os.O_RDWR|os.O_CREATE|os.O_TRUNC, 0666)

var interrupted bool = false

var nodeID int

const maxNodes int = 10
const releasedState int = 0

var ipAddr = [10]string{
	"172.22.156.112", "172.22.158.112", "172.22.94.112", "172.22.156.113",
	"172.22.158.113", "172.22.94.113", "172.22.156.114", "172.22.158.114",
	"172.22.94.114", "172.22.156.115"}

var m sync.Mutex
var mBal sync.Mutex


var conn [maxNodes]net.Conn
var isAlive [maxNodes]bool
var numAlive int

var numNodes int
var nodeState int

var port string

var nextOrd int = 0

/* Priority Queue */
type Item struct {
	msg string
	pid int
	ord int
	priority int
	index int // The index of the item in the heap.
	deliverable bool
	agreedNodes [maxNodes]bool
}

type PriorityQueue []*Item

var pq PriorityQueue

var balance map[string]int


var nextPriority int = 0

// testing

var timeBandwidth string
var timeLast string


func max(m1 int, m2 int) int{
	if m1>m2 {
		return m1
	}
	return m2
}

func getTimeString() string{
	return fmt.Sprintf("%f", float64(time.Now().UnixNano())/float64(time.Second))
}

func getIdByConn(c net.Conn) int {
	addr := c.RemoteAddr()
	ip := addr.String()
	for i:=0; i < maxNodes; i++{
		if strings.Contains(ip, ipAddr[i]) {
			return i
		}
	}
	return -1
}

var msgItems map[string]int

func (pq PriorityQueue) Len() int { return len(pq) }

func (pq PriorityQueue) Less(i, j int) bool {
	return (pq[i].priority < pq[j].priority) || 
			((pq[i].priority == pq[j].priority) && (pq[i].pid < pq[j].pid))
}

func (pq PriorityQueue) Swap(i, j int) {
	msgItems[fmt.Sprintf("%d:%d",pq[i].pid,pq[i].ord)] = j
	msgItems[fmt.Sprintf("%d:%d",pq[j].pid,pq[j].ord)] = i
	pq[i], pq[j] = pq[j], pq[i]
	pq[i].index = i
	pq[j].index = j
}

func (pq *PriorityQueue) Push(x interface{}) {
	n := len(*pq)
	item := x.(*Item)
	item.index = n
	*pq = append(*pq, item)
	msgItems[fmt.Sprintf("%d:%d",item.pid,item.ord)] = item.index
}

func (pq *PriorityQueue) Pop() interface{} {
	old := *pq
	n := len(old)
	item := old[n-1]
	old[n-1] = nil  // avoid memory leak
	item.index = -1 // for safety
	msgItems[fmt.Sprintf("%d:%d",item.pid,item.ord)] = -1
	*pq = old[0 : n-1]
	return item
}

// update modifies the priority and value of an Item in the queue.
func (pq *PriorityQueue) update(item *Item) {
	heap.Fix(pq, item.index)
}

// 0:false; 1:true; -1:error, just pop
func peekDeliverable() int{
	if (len(pq)==0){
		return 0
	}
	item := (pq)[0]
	if (item.deliverable){
		return 1
	}
	if (!isAlive[item.pid]){
		return -1
	}
	flag := true
	for i:=0; i < numNodes; i++{
		if (!item.agreedNodes[i] && isAlive[i] && i != nodeID){
			flag=false
			break
		}
	}
	if (flag){
		return 1
	}
	return 0
}

func handleErr(err error) {
	if err != nil {
		fmt.Fprintf(os.Stderr, "%s\n", err)
		os.Exit(1)
	}
}

func isError(err error) bool {
	if err != nil {
		fmt.Fprintf(os.Stderr, err.Error(), "\n")
	}
	return (err != nil)
}

func getNodeID() {
	interfaces, err := net.Interfaces()
	handleErr(err)
	var ip net.IP
	for _, i := range interfaces {
		addrs, e := i.Addrs()
		handleErr(e)
		for _, addr := range addrs {
			switch j := addr.(type) {
			case *net.IPNet:
				ip = j.IP
			case *net.IPAddr:
				ip = j.IP
			}
			for index := 0; index < maxNodes; index++ {
				if ipAddr[index] == ip.String() {
					nodeID = index
					return
				}
			}
		}
	}
}

func getNextOrd() int{
	m.Lock()
	ret:=nextOrd
	nextOrd+=1
	m.Unlock()
	return ret
}

func deliverMsg(message string) {
	mBal.Lock()
	countDeliveredMsg++
	mBal.Unlock()
	fmt.Fprintf(os.Stderr,"Delivering transaction: " + message + "\n")
	dat := strings.Split(message, " ")
	if dat[0] == "TRANSFER" {
		// TRANSFER a -> c 13
		usr1 := dat[1]
		usr2 := dat[3]
		amount, _ := strconv.Atoi(dat[4])
		if (amount == 0) {
			return
		}
		mBal.Lock()
		if _, ok := balance[usr1]; ok {
			if balance[usr1]-amount >= 0 {
				balance[usr1] -= amount
				if _, ok2 := balance[usr2]; ok2 {
					balance[usr2] += amount
				} else {
					balance[usr2] = amount
				}
			} else {
				fmt.Fprintf(os.Stderr,"Illegal transaction: " + message + "\n")
			}
		}		
		mBal.Unlock()
	} else if dat[0] == "DEPOSIT" {
		// DEPOSIT a 75
		usr := dat[1]
		dMoney := dat[2]
		amount, _ := strconv.Atoi(dMoney)
		if (amount == 0) { 
			return 
		}
		mBal.Lock()
		if _, ok := balance[usr]; ok {
			balance[usr] += amount
		} else {
			balance[usr] = amount
		}
		mBal.Unlock()
	}
}

func setNodeUnavailable(id int){
	if (isAlive[id]){
		numAlive--
		isAlive[id] = false
	}
}

func broadcast(message string) {
	// fmt.Print("Connections:")
	for i := 0; i < numNodes; i++ {
		// fmt.Print(" ", conn[i])
		if i != nodeID && isAlive[i] {
			_, e := fmt.Fprintf(conn[i], message+"\n")
			
			if (e!=nil) {
				setNodeUnavailable(i)
			} else {
				// bandwidth
				timeBandwidth = getTimeString()
				// fmt.Println(timeBandwidth, "Bandwidth: ", len(message + "\n"))
				file1.WriteString(timeBandwidth + " " +  fmt.Sprintf("%d", len(message + "\n")) + "\n")
			} 
		}
	}
	//fmt.Println()
}

// Handles connection
func handleConn(id int) {
	reader := bufio.NewReader(conn[id])
	for !interrupted{
		msg, err := reader.ReadString('\n')
		// bandwidth
		timeBandwidth = getTimeString()
		// fmt.Println(timeBandwidth, "Bandwidth: ", len(msg))
		m.Lock()
		file1.WriteString(timeBandwidth + " " +  fmt.Sprintf("%d", len(msg)) + "\n")
		m.Unlock()
		if err != nil {
			setNodeUnavailable(id)
			break
		}
		if (msg == "" || msg == "\n"){
			continue
		}
		if last := len(msg) - 1; last >= 0 && msg[last] == '\n' {
			msg = msg[:last]
		}
		// fmt.Println(getTimeString(), " Raw Msg from ",id,": ", msg)
		
		dat := strings.SplitN(msg, " ", 5)
		if (msg[0] == 'M'){
			msgPid, _ := strconv.Atoi(dat[1])
			msgOrd, _ := strconv.Atoi(dat[2])
			item := &Item{
				msg: dat[4],
				pid: msgPid,
				ord: msgOrd,
				priority: nextPriority,
				deliverable: false,
			}
			// fmt.Println("Received message: ", item)
			m.Lock()
			fmt.Fprintf(conn[id], fmt.Sprintf("P %s %s %d\n",dat[1],dat[2],nextPriority))

			// bandwidth
			timeBandwidth = getTimeString()
			// fmt.Println(timeBandwidth, "Bandwidth: ", len(fmt.Sprintf("P %s %s %d\n",dat[1],dat[2],nextPriority)))
			file1.WriteString(timeBandwidth + " " +  fmt.Sprintf("%d", len(fmt.Sprintf("P %s %s %d\n",dat[1],dat[2],nextPriority))) + "\n")

			nextPriority += 1
			heap.Push(&pq, item)
			m.Unlock()
		} else if (msg[0] == 'P'){
			m.Lock()
			proposed, _ := strconv.Atoi(dat[3])
			item := pq[msgItems[dat[1]+":"+dat[2]]]
			// fmt.Println("Map result: ", item.msg)
			item.agreedNodes[id] = true
			item.priority = max(proposed, item.priority)
			pq.update(item)
			nextPriority = item.priority + 1
			flag := true
			for i:=0; i<numNodes; i++{
				if (!item.agreedNodes[i] && isAlive[i] && i != nodeID){
					flag=false
					break
				}
			}
			if (flag){
				item.deliverable = true
				broadcast(fmt.Sprintf("F %s %s %d", dat[1], dat[2], item.priority))
				peekVal := 1
				for peekVal != 0{
					peekVal = peekDeliverable()
					if peekVal == 0{
						break
					}
					head := heap.Pop(&pq).(*Item)
					// Last message processed
					timeLast = getTimeString()
					// 
					//fmt.Println(timeLast, "LastMessage",  head.pid ,head.ord , head.msg)
					file2.WriteString(timeLast + " LastMessage " + fmt.Sprintf("%d %d ", head.pid ,head.ord ) + head.msg + "\n")
					// fmt.Println("Trace pop item from msg:", msg)
					if (peekVal == 1) {
						deliverMsg(head.msg)
					}
				}
			}	
			m.Unlock()
		} else if (msg[0] == 'F'){
			m.Lock()
			// fmt.Println("id:",msgItems[dat[1]+":"+dat[2]], " pq:",len(pq))
			item := pq[msgItems[dat[1]+":"+dat[2]]]
			// fmt.Println("Map result: ", item.msg)
			item.deliverable = true
			peekVal := 1
			for peekVal != 0{
				peekVal = peekDeliverable()
				if peekVal == 0{
					break
				}
				head := heap.Pop(&pq).(*Item)
				// fmt.Println("Trace pop item from msg:", msg)
				// Last message processed
				timeLast = getTimeString()
				//fmt.Println(timeLast, "LastMessage", head.pid ,head.ord , head.msg)
				file2.WriteString(timeLast + " LastMessage " + fmt.Sprintf("%d %d ", head.pid ,head.ord ) + head.msg + "\n")
				if (peekVal == 1) {
					deliverMsg(head.msg)
				}
			}
			m.Unlock()
		} 
	}
}

func handleLocal() {
	scanner := bufio.NewReader(os.Stdin)
	for !interrupted{
		countPyMsg++
		messageLocal, err := scanner.ReadString('\n')
		timeFirst := getTimeString()
		//handleErr(err)
		if (err != nil){
			interrupted = true
			break
		}
		if (messageLocal == "" || messageLocal == "\n"){
			continue
		}
		if last := len(messageLocal) - 1; last >= 0 && messageLocal[last] == '\n' {
			messageLocal = messageLocal[:last]
		}
		
		item := &Item{
			msg:    messageLocal,
			pid: nodeID,
			ord: getNextOrd(),
			priority: nextPriority,
			deliverable: false,
		}
		m.Lock()
		// First message processed
		//fmt.Println(timeFirst, "FirstMessage", item.pid ,item.ord , messageLocal)
		file2.WriteString(timeFirst + " FirstMessage " + fmt.Sprintf("%d %d ", item.pid ,item.ord) + messageLocal + "\n")
		// fmt.Println(getTimeString(), "pid: " , item.pid ,  " ord: ", item.ord , " Python: ", messageLocal)
		for i := 0; i < maxNodes; i++{
			item.agreedNodes[i] = false
		}
		if numAlive > 1{
			broadcast(fmt.Sprintf("M %d %d %d %s", item.pid, item.ord, item.priority, item.msg))
			nextPriority += 1
			heap.Push(&pq, item)
		} else {
			deliverMsg(messageLocal)
		}
		m.Unlock()
	}
}

func printBalance(){
	for !interrupted {
		time.Sleep(5*time.Second)
		m.Lock()
		fmt.Print("BALANCES")
		for k, v := range balance{
			fmt.Print(" ", k, ":", v)
		}
		fmt.Println()
		// fmt.Fprintf(os.Stderr, "Python generated %d\n", countPyMsg)
		// fmt.Fprintf(os.Stderr, "Delivered %d\n", countDeliveredMsg)
		m.Unlock()
	}
}


func main() {
	// Get node id
	getNodeID()
	// Sigint
	sigs := make(chan os.Signal, 1)
	signal.Notify(sigs, syscall.SIGINT)
	go func() {
		_ = <-sigs
        interrupted = true
	}()
	
	// File
	if isError(err_f1) {
		return
	}
	defer file1.Close()
	if isError(err_f2) {
		return
	}
	defer file2.Close()
	// Arguments
	argv := os.Args[1:]
	if len(argv) != argNumNode {
		fmt.Fprintf(os.Stderr, "Usage: ./mp1_node <NODE NUMBER> <PORT_NUMBER>\n")
		os.Exit(1)
	}
	numNodes, _ = strconv.Atoi(argv[0])
	port = argv[1]
	// Initialize data
	pq = make(PriorityQueue, 0)
	heap.Init(&pq)
	balance = make(map[string]int)
	msgItems = make(map[string](int))
	for i := 0; i < numNodes; i++{
		isAlive[i] = true
	}
	numAlive = numNodes
	// TCP server
	ln, e := net.Listen("tcp", ":"+port)
	handleErr(e)
	for i := nodeID + 1; i < numNodes; i++ {
		c, e := ln.Accept()
		handleErr(e)
		index:=getIdByConn(c)
		conn[index] = c
		go handleConn(index)
	}
	// TCP client
	for i := 0; i < nodeID; i++ {
		for {
			conn[i], e = net.Dial("tcp", ipAddr[i]+":"+port)
			if e == nil {break}
		}
		go handleConn(i)
	}
	go printBalance()
	handleLocal()
	defer ln.Close()
	// fmt.Fprintf(os.Stderr,"Last BALANCES")
	// 	for k, v := range balance{
	// 		fmt.Fprintf(os.Stderr, " %d:%d", k, v)
	// 	}
	// fmt.Fprintf(os.Stderr,"\n")
	// fmt.Fprintf(os.Stderr, "Python generated %d\n", countPyMsg)
	// fmt.Fprintf(os.Stderr, "Delivered %d\n", countDeliveredMsg)
}
