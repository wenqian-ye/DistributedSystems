package main

import "fmt"
import "net"
import "os"
import "bufio"
import "strings"
import "time"

func main(){
	args := os.Args
	scanner := bufio.NewScanner(os.Stdin)
	conn , err := net.Dial("tcp", args[2] + ":" + args[3])
	_ =  err
	timeString := fmt.Sprintf("%f", float64(time.Now().UnixNano()) / float64(time.Second))
	fmt.Fprintf(conn, timeString + " - " + args[1] +" connected\n")
	for scanner.Scan() {
		dat := strings.Split(scanner.Text(), " ")
		fmt.Fprintf(conn, dat[0] + " " + args[1] + " " + dat[1] + "\n")
	}
}