go build mp2.go
sleep 1
trap "killall mp2" INT
./mp2 node3 172.22.158.112 5001 &
./mp2 node4 172.22.158.112 5002 &
./mp2 node1 172.22.158.112 5003 &
sleep 5000
