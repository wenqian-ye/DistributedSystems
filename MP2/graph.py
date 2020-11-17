'''
 Logger
 		Part 1 evaluation
 			B timestamp length
 			T nodename timestamp TRANSACTION createtime
 		Part 2 evaluation
 			BLK nodename timestamp hash --- block creation
			TB nodename addTimestamp receiveTimestamp TRANSACTION... -- transaction to block
 			CS nodename timestamp length hash1 hash2 -- chain split
'''
import numpy as np
import matplotlib.pyplot as plt
reader = open("/Users/wenqian/Desktop/cs425-sp2020/MP2/mp2-data/log100-20-0.4.txt", "r")

bandwidth = {} # time -> bandwidth
transactions = {} # key -> (max propagation time, receive number, create time)
blocksCreation = {} # hash -> (max time, receive number, min time)  
transactionAppear = {} # receiveTimestamp -> (max propagation time, receive number, create time)
splitChain = {} # time -> (time, length)

bandwidthMaxTime = 0
bandwidthMinTime = float("inf")

blockIdx = 0

def plotPropagation(xArray, inputArray):
    n = np.ceil(np.max(xArray)-np.min(xArray))
    n = int(n)
    maxArr = np.full(n, np.max(inputArray))
    minArr = np.full(n, np.min(inputArray))
    medArr = np.full(n, np.median(inputArray))
    plt.plot(maxArr, label='Max')
    plt.plot(minArr, label='Min')
    plt.plot(medArr, label='Median')

blockCount=0
while True:
    logLine = reader.readline()
    if logLine == "" or logLine == "\n" or logLine is None:
        break 
    log = logLine.split(' ')
    if log[0] == "B":
        time = int(float(log[1]))
        length = int(float(log[2]))
        if time > bandwidthMaxTime:
            bandwidthMaxTime=time
        if time < bandwidthMinTime:
            bandwidthMinTime = time
        bandwidth[time] = length
    elif log[0] == "T":
        id = log[4]
        createTime = float(log[4])
        maxPropagationTime = 0
        receiveNumber = 0
        if (id in transactions):
            maxPropagationTime, receiveNumber, createTime = transactions[id]
        
        propDelay = float(log[2]) - createTime
        maxPropagationTime = max(propDelay, maxPropagationTime)
        receiveNumber += 1
        transactions[id] = (maxPropagationTime, receiveNumber, createTime)
    elif log[0] == "BLK": 
        blockCount +=1
        hash = log[3]
        createTime = float(log[2])
        if (hash in blocksCreation):
            maxTime, receiveNumberBlk, minTime = blocksCreation[hash]
            maxTime = max(createTime, maxTime)
            minTime = min(createTime, minTime)
            receiveNumberBlk += 1
            blocksCreation[hash] = (maxTime, receiveNumberBlk, minTime)
        else:
            blocksCreation[hash] = (createTime, 1, createTime)

        # receiveNumberBlk = 0
        # if (hash in blocksCreation):
        #     maxTime, receiveNumberBlk, createTime = blocksCreation[hash]
        # receiveNumberBlk += 1
        # blocksCreation[hash] = (createTime ,receiveNumberBlk, createTime)
        
    elif log[0] == "TB":
        id = log[3]
        createTime = float(log[3])
        maxAppearTime = 0
        receiveNumberTrans = 0
        if (id in transactionAppear):
            maxAppearTime, receiveNumberTrans, createTime = transactionAppear[id]

        propDelayTrans = float(log[2]) - createTime
        maxAppearTime = max(propDelayTrans, maxAppearTime)
        receiveNumberTrans += 1
        transactionAppear[id] = (maxAppearTime, receiveNumberTrans, createTime)
            
    elif log[0] == "CS":
        time = float(log[2])
        length = int(log[3])
        splitChain[time] = (time, length)

# bandwidth
# bandwidth = bandwidth.values()
bandwidthDat = np.zeros(bandwidthMaxTime-bandwidthMinTime+1)
for k,v in bandwidth.items():
    bandwidthDat[k-bandwidthMinTime] = v

plt.plot(bandwidthDat)
plt.title("Bandwidth versus time")
plt.xlabel("Time (second)")
plt.ylabel("Bandwidth (bytes per second)")
plt.show()

# transactions (max propagation time, receive number, create time)
transactions = list(transactions.values())
transactions = sorted(transactions, key=lambda x:x[2])
transactionDat = np.zeros((0,3))
for transaction in transactions:
    transactionDat = np.append(transactionDat, [[transaction[0], transaction[1], transaction[2]]], axis = 0)
transactionDat[:,2] -= np.min(transactionDat[:,2])

print(np.unique(transactionDat[:,1]))


plt.scatter(transactionDat[:,2], transactionDat[:,1])
plt.title("Number of nodes received for transactions")
plt.xlabel("Transaction create time(second)")
plt.ylabel("Number of nodes received")
plt.show()

plt.scatter(transactionDat[:,2], transactionDat[:,0])
plt.title("Propagation time for transactions")
plt.xlabel("Transaction create time(second)")
plt.ylabel("Propagation time(second)")
plotPropagation(transactionDat[:,2],transactionDat[:,0])
plt.legend()
plt.show()

# transaction appear in a block (max propagation time, receive number, create time)
transactionAppear = list(transactionAppear.values())
transactionAppear = sorted(transactionAppear, key=lambda x:x[2])
transactionAppearDat = np.zeros((0,3))
for transaction in transactionAppear:
    transactionAppearDat = np.append(transactionAppearDat, [[transaction[0], transaction[1], transaction[2]]], axis = 0)
transactionAppearDat[:,2] -= np.min(transactionAppearDat[:,2])

# plt.scatter(transactionAppearDat[:,2], transactionAppearDat[:,1])
# plt.title("Number of transactions appear in a block")
# plt.xlabel("Transaction create time(second)")
# plt.ylabel("Number of transactions appear in a block")
# plt.show()

plt.scatter(transactionAppearDat[:,2], transactionAppearDat[:,0])

plt.title("Congestion delays of transactions appearing in a block")
plt.xlabel("Transaction create time (second)")
plt.ylabel("Propagation time (second)")
plotPropagation(transactionAppearDat[:,2], transactionAppearDat[:,0])
plt.legend()
plt.show()

numBlocks = len(blocksCreation)
# block propagation (max time, receive number, min time) 
blocksCreation = list(blocksCreation.values())
blocksCreation = sorted(blocksCreation, key=lambda x:x[2])
blocksCreationDat = np.zeros((0,3))
for block in blocksCreation:
    blocksCreationDat = np.append(blocksCreationDat, [[(block[0] - block[2]), block[1], block[2]]], axis = 0)

# plt.scatter(blocksCreationDat[:,2], blocksCreationDat[:,1])
# plt.title("Number of blocks")
# plt.xlabel("Block create time(second)")
# plt.ylabel("Number of blocks")
# plt.show()
plt.scatter(blocksCreationDat[:,2], blocksCreationDat[:,0])
plt.title("Propagation time of blocks")
plt.xlabel("Block create time(second)")
plt.ylabel("Propagation time(second)")
plt.show()


# chain split (time, length)
splitChain = list(splitChain.values())
splitChainDat = np.zeros((0,2))
for split in splitChain:
    splitChainDat = np.append(splitChainDat, [[split[0], split[1]]], axis = 0)

maxHeight = 0
if splitChainDat[:,1].size != 0:
    maxHeight = np.max(splitChainDat[:,1])

print(maxHeight, splitChainDat[:,1].size/blockCount)

# plt.scatter(splitChainDat[:,0], splitChainDat[:,1])
# plt.title("Length of Split Chains")
# plt.xlabel("Time(second)")
# plt.ylabel("Length of Split Chains")
# plt.show()