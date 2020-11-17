go build mp2.go
sleep 1
trap "killall mp2" INT
./mp2 node5 172.22.94.112 5001 &
./mp2 node6 172.22.94.112 5002 &
./mp2 node2 172.22.94.112 5003 &
sleep 5000
