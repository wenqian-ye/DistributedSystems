# MP3: Distributed transactions

## Info

### Cluster Name

g34


### Authors

Wenqian Ye(wenqian3)
Yunqian Bao(yunqian4)

### Revision number

da95b689ae74d00146a47bd18883a648c06fc828

### URL and revision number
https://gitlab.engr.illinois.edu/wenqian3/cs425-sp2020/tree/master/MP3



## Usage

The branches and server are run on VM1. We should first run the program on VM1. After 2 seconds, run the clients on other VMs.

To run the coordinator server, 5 branches and one client on VM1, type the following command and type return.
```
sh run.sh
```
To run the clients on other VMs, type this command and type return:
```
sh build.sh
```



## Description of design


### A walk-through of a simple transation
We have 5 branches, a coordinator server, and multiple clients.

The client is simply an agent that sends and receives messages from the coordinator server.

Each branch receives and executes commands from the coordinator service. 

The coordinator service takes records of all resources (accounts), transactions, and locks. It receives commands from clients and decide whether and when to ask the branches to execute the commands, judging from the status of locks and resource allocation.

A client starts a transaction with "BEGIN"

When a client sends a command like "DEPOSIT", the coordinator service checks whether the lock for the account can be acquired. If yes, then acquire the lock and ask the cooresponding branch to execute the command; otherwise, if waiting for the lock to be released would not generate deadlock, then the client shall wait for the account become available (unless the client abort the transaction during wait). If a branch receives a command, then it exceutes the command and stores the command in a list cooresponding to the transaction (which makes rollback possible).

If the client sends "COMMIT", the coordinator service would perform 2-phase commit: it asks each branch whether each of them can commit the transation (whether there is no negative balance), if all branches replies yes, then ask all branches to commit; otherwise ask all branches to abort. After committing or aborting, the transaction no longer acquire any locks.

If the client sends "ABORT", the coordinator service abort the transaction immediately by asking all branches to abort the transaction and rollback.

### Concurrency control
We use two phase locking for each account. A lock is released when a transaction is aborted or committed (we use a two phase commit). And a lock is acquired when: no other transaction is writing into the account or all transaction acquiring the account is simply reading from it. 


### Aborted transaction and roll back
When a transaction is aborted, the coordinator server sends an "ABORT" message to all branches, asking them to abort the transaction. When a branch receives an "ABORT" message, it undos all the executed commands in the transaction. After all branches abort the transaction, the transaction no longer acquire the locks. Edge case: if an account is created in an aborted transaction, then after the abort operation, the account would not exist in the branch.


### Deadlock

We use a resource allocation graph to record the information of which processes are waiting for a resource and which processes have acquired the resource. If a process acquired a lock for a resource, then we add a directed edge from resource to process; if a process is waiting for a resource to be released, then we add a directed edge from process to resource. 

Deadlock occurs if and only if there is a cycle in the resource allocation graph. When a process (in this MP, transaction) is trying to acquire a resource, we edit the resource allocation graph and check whether there is a cycle by DFS. If there is a cycle, we abort the transaction.