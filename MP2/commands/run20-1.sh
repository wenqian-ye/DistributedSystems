go build mp2.go
go build mp2_logger.go 
./mp2_logger &
python3 mp2_service.py 4999 1
# sleep 2 
# ./mp2 node1 172.22.156.112 5001 &
# ./mp2 node2 172.22.156.112 5002 &
