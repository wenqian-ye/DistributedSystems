go build mp2.go
sleep 1
trap "killall mp2" INT
./mp2 node9 172.22.158.113 5001 &
./mp2 node10 172.22.158.113 5002 &
sleep 5000
