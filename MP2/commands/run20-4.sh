go build mp2.go
sleep 1
trap "killall mp2" INT
./mp2 node7 172.22.156.113 5001 &
./mp2 node8 172.22.156.113 5002 &
sleep 5000
