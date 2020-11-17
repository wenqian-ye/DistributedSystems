go build mp2.go
sleep 1
trap "killall mp2" INT
./mp2 node11 172.22.94.113 5001 &
./mp2 node12 172.22.94.113 5002 &
sleep 5000
