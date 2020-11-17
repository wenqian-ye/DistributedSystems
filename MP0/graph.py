import numpy as np
import matplotlib.pyplot as plt

'''
    Rename the log file
        of 3 nodes 0.5 hz to "3.txt"
        of 8 nodes 5 hz to "8.txt"
    Change the variable profileNum to 3 or 8 to choose the
    log file to process
'''

profileNum = 8
profiles = {3:{'name':'3 nodes 0.5 hz','file':'3.txt'},
            8:{'name':'8 nodes 5 hz','file':'8.txt'}}

f = open(profiles[profileNum]['file'], "r")
nodeList = []
dat = {}
npDat=np.zeros((0, 3))

while True:
    logLine = f.readline()
    if logLine == "" or logLine == "\n" or logLine is None:
        break    
    log = logLine.split(' ')
    delay = float(f.readline())
    bandwidth = int(f.readline())
    if(len(log) == 4): #connection
        nodeList.append(log[2])
        dat[log[2]] = {"delay": [], "bandwidth": []}
    else: #event
        dat[log[1]]['delay'].append(delay)
        dat[log[1]]['bandwidth'].append(bandwidth)
    time = float(log[0])
    npDat = np.append(npDat, [[time, delay, bandwidth]], axis = 0)

npDat[:,0] = npDat[:,0] - np.min(npDat[:,0])
n = npDat.shape[0]
seconds = int(np.max(npDat[:,0])) - int(np.min(npDat[:,0])) + 1
boxes = []

for i in range(seconds):
    boxes.append(np.zeros((0,2)))
for i in range(n):
    boxes[int(npDat[i,0])] = np.append(boxes[int(npDat[i,0])], [[npDat[i,1], npDat[i,2]]], axis = 0)

bandwidthAvg = np.zeros(seconds)
delayMin = np.zeros(seconds)
delayMax = np.zeros(seconds)
delayMed = np.zeros(seconds)
delay90 = np.zeros(seconds)

for i in range(seconds):
    if(boxes[i].shape[0] > 0):
        bandwidthAvg[i] = np.sum(boxes[i][:,1])
        delayMin[i] = np.min(boxes[i][:,0])
        delayMax[i] = np.max(boxes[i][:,0])
        delayMed[i] = np.median(boxes[i][:,0])
        delay90[i] = np.percentile(boxes[i][:,0], 90)

time = np.arange(0, seconds)

plt.figure(figsize=(12,8))
plt.plot(time, delayMin, label='Minimum Delay')
plt.plot(time, delayMax, label='Maximum Delay')
plt.plot(time, delayMed, label='Median Delay')
plt.plot(time, delay90, label='90 Percentile Delay')
plt.legend()
plt.xlabel('Time (seconds)')
plt.ylabel('Delay (seconds)')
plt.title('Delay versus time ('+profiles[profileNum]['name']+')')

plt.figure(figsize=(12,8))
plt.plot(time, bandwidthAvg)
plt.xlabel('Time (seconds)')
plt.ylabel('Bandwidth (message length in characters)')
plt.title('Total bandwidth in each second ('+profiles[profileNum]['name']+')')