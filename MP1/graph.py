import numpy as np
import matplotlib.pyplot as plt

ROOT_DIR = "MP1-data"

profiles = {
    1:{"n":3,"dir":"3-nofailure"},
    2:{"n":8,"dir":"8-nofailure"},
    3:{"n":3,"dir":"3-failure"},
    4:{"n":8,"dir":"8-failure"},
}

profileNum = 4

nNodes = profiles[profileNum]["n"]

bandwidthFiles = []

logFiles = []

for i in range(nNodes):
    bandwidthFiles.append(open(ROOT_DIR+"/"+ \
            profiles[profileNum]["dir"]+"/bandwidth"+str(i+1)+".txt","r"))
    logFiles.append(open(ROOT_DIR+"/"+ \
            profiles[profileNum]["dir"]+"/log"+str(i+1)+".txt","r"))            

msgTimestamp = {}

bandwidthDat = []

for nodeIndex in range(nNodes):
    # Bandwidth
    bandwidthList = np.zeros((0, 2))
    while True:
        bandwidthLine = bandwidthFiles[nodeIndex].readline()
        if bandwidthLine == "" or bandwidthLine == "\n" or bandwidthLine is None:
            break    
        bandwidthLog = bandwidthLine.split(' ')
        time = float(bandwidthLog[0])
        length = float(bandwidthLog[1])
        bandwidthList = np.append(bandwidthList, [[time, length]], axis = 0)
    n = bandwidthList.shape[0]
    print(bandwidthList.shape)
    bandwidthList[:,0] = bandwidthList[:,0]-np.min(bandwidthList[:,0])
    seconds = int(np.max(bandwidthList[:,0])) - int(np.min(bandwidthList[:,0])) + 1
    boxes = []
    for i in range(seconds):
        boxes.append(np.zeros(0))
    for i in range(n):
        boxes[int(bandwidthList[i,0])] = np.append(boxes[int(bandwidthList[i,0])], \
                [bandwidthList[i,1]])
    bandwidthAvg = np.zeros(seconds)
    for i in range(seconds):
        if(boxes[i].shape[0] > 0):
            bandwidthAvg[i] = np.sum(boxes[i])
    bandwidthDat.append(bandwidthAvg)

    # Delay
    while True:
        logLine = logFiles[nodeIndex].readline()
        if logLine == "" or logLine == "\n" or logLine is None:
            break    
        log = logLine.split(' ')
        time = float(log[0])
        key = log[2]+":"+log[3]
        isFirst = (log[1] == "FirstMessage")
        if(not (key in msgTimestamp)):
            msgTimestamp[key] = [-1, -1]
        index = 0
        if(not isFirst):
            index = 1
        msgTimestamp[key][index] = max(time, msgTimestamp[key][index])

# format bandwidth
time = 0
for i in range(nNodes):
    if bandwidthDat[i].size > time:
        time = bandwidthDat[i].size
for i in range(nNodes):
    tmp = np.zeros(time)
    tmp[:bandwidthDat[i].size] = bandwidthDat[i]
    bandwidthDat[i] = tmp
time = np.arange(0,time)

# plot bandwidth
plt.figure(figsize=(12,8))
for i in range(nNodes):
    plt.plot(time, bandwidthDat[i], label="node "+str(i+1))
plt.legend()
plt.xlabel('Time (seconds)')
plt.ylabel('Bandwidth (bytes per second)')
plt.title('Bandwidth versus time')

#plot delay
delay = np.zeros(0)
for msg in msgTimestamp:
    val = msgTimestamp[msg]
    if (val[0] >= 0) and (val[1] >= 0):
        diff = val[1]-val[0]
        #diff = np.log(diff)
        delay = np.append(delay,[diff])

plt.figure(figsize=(12,6))
ax = plt.subplot(3, 1, 1)
#plt.tight_layout()
ax.boxplot(delay,vert=False)
ax.set_xscale('log')
ax.set_xlabel('Second (s)')
ax.set_title('Distribution of delay (log plot)')

ax = plt.subplot(3, 1, 3)
ax.boxplot(delay,vert=False,showfliers=False)
#ax.set_xscale('log')
ax.set_xlabel('Second (s)')
ax.set_title('Distribution of delay without outliers')

plt.show()

