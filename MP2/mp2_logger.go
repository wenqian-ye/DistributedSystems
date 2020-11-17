package main

import (
	"bufio"
	"fmt"
	"net"
	"os"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
)

// Logger
// 		Part 1 evaluation
// 			B timestamp length
// 			T nodename timestamp TRANSACTION createtime
// 		Part 2 evaluation
// 			BLK nodename timestamp hash --- block creation
// 			TB nodename addTimestamp receiveTimestamp TRANSACTION... -- transaction to block
// 			CS nodename timestamp length hash1 hash2 -- chain split

var path = "log.txt"
var m, bandwidthMtx sync.Mutex

var cumulativeConnect, cumulativeDisconnect int32

var bandwidthMap map[int64]int64

var file *(os.File)

var exitCond *sync.Cond
var exitMtx sync.Mutex

func handleConn(conn net.Conn) {
	reader := bufio.NewReader(conn)
	for {
		msg, err := reader.ReadString('\n')
		if err != nil {
			break
		}
		dat := strings.Split(msg[:len(msg)-1], " ")
		if dat[0] == "B" {
			// parse
			time, eTime := strconv.ParseFloat(dat[1], 64)
			if eTime != nil {
				fmt.Fprintf(os.Stderr, "Logger: PART1 Timestamp parse failed\n")
				continue
			}
			bandwidth, eBandwidth := strconv.ParseInt(dat[2], 10, 64)
			if eBandwidth != nil {
				fmt.Fprintf(os.Stderr, "Logger: PART1 Bandwidth parse failed\n")
				continue
			}
			// save to map
			id := int64(time)
			bandwidthMtx.Lock()
			totalBandwidth, exist := bandwidthMap[id]
			if !exist {
				totalBandwidth = 0
			}
			bandwidthMap[id] = totalBandwidth + bandwidth
			bandwidthMtx.Unlock()
		} else {
			m.Lock()
			file.WriteString(msg)
			m.Unlock()
		}
	}
	atomic.AddInt32(&cumulativeDisconnect, 1)
	exitCond.Signal()
	m.Lock()
	//file.Sync()
	m.Unlock()
}

func saveBandwidth() {
	m.Lock()
	for timeInt := range bandwidthMap {
		file.WriteString(fmt.Sprintf("B %d %d\n", timeInt, bandwidthMap[timeInt]))
	}
	//file.Sync()
	m.Unlock()
}

func listen(listener net.Listener) {
	for {
		conn, _ := listener.Accept()
		atomic.AddInt32(&cumulativeConnect, 1)
		go handleConn(conn)
	}
}

func main() {
	exitCond = sync.NewCond(&exitMtx)
	bandwidthMap = make(map[int64]int64)

	ln, e := net.Listen("tcp", ":4998")
	if e != nil {
		fmt.Fprintf(os.Stderr, "Mp2 Logger: cannot listen\n")
		os.Exit(1)
	}

	var errF error
	file, errF = os.OpenFile(path, os.O_RDWR|os.O_CREATE|os.O_TRUNC, 0666)
	if errF != nil {
		fmt.Fprintf(os.Stderr, "Mp2 Logger: cannot init file\n")
		os.Exit(1)
	}

	defer file.Close()
	go listen(ln)
	exitCond.L.Lock()
	for cumulativeConnect == 0 || cumulativeConnect != cumulativeDisconnect {
		exitCond.Wait()
	}
	exitCond.L.Unlock()
	saveBandwidth()
	//file.Close()
	//os.Exit(0)
}
