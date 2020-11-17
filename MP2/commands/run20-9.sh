go build mp2.go
sleep 1
trap "killall mp2" INT
./mp2 node17 172.22.94.114 5001 &
./mp2 node18 172.22.94.114 5002 &
sleep 5000
