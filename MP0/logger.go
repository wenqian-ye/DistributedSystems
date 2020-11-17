package main

import (
	"bufio"
	"fmt"
	"net"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"
)

var path = "log.txt"
var m sync.Mutex

func handleConn(conn net.Conn, file *(os.File), err_f error) {
	reader := bufio.NewReader(conn)
	dat, err := reader.ReadString('\n')
	_ = err
	fmt.Fprintf(os.Stdout, dat)

	nodeName := strings.Split(dat, " ")[2]
	m.Lock()
	file.WriteString(dat)
	m.Unlock()
	timestamp_c, err := strconv.ParseFloat(strings.Split(dat, " ")[0], 64) // client time
	timestamp_s := float64(time.Now().UnixNano()) / float64(time.Second)
	time_delay_str := fmt.Sprintf("%f", timestamp_s-timestamp_c) + "\n"
	bandwidth_str := fmt.Sprintf("%d", len(dat)) + "\n"
	// file.WriteString(dat)
	file.WriteString(time_delay_str)
	file.WriteString(bandwidth_str)

	for {
		dat, err := bufio.NewReader(conn).ReadString('\n')
		if err != nil {
			timeString := fmt.Sprintf("%f", float64(time.Now().UnixNano())/float64(time.Second))
			out := timeString + " - " + nodeName + " disconnected\n"
			fmt.Fprintf(os.Stdout, out)
			m.Lock()
			m.Unlock()
			break
		}

		timestamp_c, err := strconv.ParseFloat(strings.Split(dat, " ")[0], 64) // client time
		timestamp_s := float64(time.Now().UnixNano()) / float64(time.Second)
		fmt.Fprintf(os.Stdout, dat)

		m.Lock()
		time_delay_str := fmt.Sprintf("%f", timestamp_s-timestamp_c) + "\n"
		bandwidth_str := fmt.Sprintf("%d", len(dat)) + "\n"
		file.WriteString(dat)
		file.WriteString(time_delay_str)
		file.WriteString(bandwidth_str)
		m.Unlock()
	}
}

func main() {
	args := os.Args
	port := ":" + args[1]
	ln, e := net.Listen("tcp", port)
	if isError(e) {
		return
	}

	file, err_f := os.OpenFile(path, os.O_RDWR|os.O_CREATE|os.O_TRUNC, 0666)
	if isError(err_f) {
		return
	}
	defer file.Close()

	for true {
		conn, err := ln.Accept()
		_ = err
		go handleConn(conn, file, err_f)
	}
}

func isError(err error) bool {
	if err != nil {
		fmt.Println(err.Error())
	}
	return (err != nil)
}
