go build mp2.go
go build mp2_logger.go 
./mp2_logger &
python3 mp2_service.py 4999 20 0.1
# sleep 2
# ./mp2 node1 172.22.156.112 5001 &
# ./mp2 node2 172.22.156.112 5002 &
# ./mp2 node3 172.22.156.112 5003 &
# ./mp2 node4 172.22.156.112 5004 &
# ./mp2 node5 172.22.156.112 5005 &
# ./mp2 node6 172.22.156.112 5006 &
# ./mp2 node7 172.22.156.112 5007 &
# ./mp2 node8 172.22.156.112 5008 &
# ./mp2 node9 172.22.156.112 5009 &
# ./mp2 node10 172.22.156.112 5010 &
