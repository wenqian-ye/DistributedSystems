go build mp2.go
sleep 1
trap "killall mp2" INT
./mp2 node61 172.22.156.114 5001 &
./mp2 node62 172.22.156.114 5002 &
./mp2 node63 172.22.156.114 5003 &
./mp2 node64 172.22.156.114 5004 &
./mp2 node65 172.22.156.114 5005 &
./mp2 node66 172.22.156.114 5006 &
./mp2 node67 172.22.156.114 5007 &
./mp2 node68 172.22.156.114 5008 &
./mp2 node69 172.22.156.114 5009 &
./mp2 node70 172.22.156.114 5010 &
./mp2 node6 172.22.156.114 5011 &
sleep 5000