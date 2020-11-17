go build mp2.go
sleep 1
trap "killall mp2" INT
./mp2 node19 172.22.156.115 5001 &
./mp2 node20 172.22.156.115 5002 &
sleep 5000
