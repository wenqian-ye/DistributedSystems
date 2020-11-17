go build mp2.go
sleep 1
trap "killall mp2" INT
./mp2 node13 172.22.156.114 5001 &
./mp2 node14 172.22.156.114 5002 &
sleep 5000
