go build mp2.go
sleep 1
trap "killall mp2" INT
./mp2 node15 172.22.158.114 5001 &
./mp2 node16 172.22.158.114 5002 &
sleep 5000
