go build client.go
go build server.go
go build branch.go
./branch 5001 &
./branch 5002 &
./branch 5003 &
./branch 5004 &
./branch 5005 &
sleep 1
./server &
sleep 1
./client
