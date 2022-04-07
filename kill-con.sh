#!/bin/bash

for pid in $(ps -ef | grep "server1" | awk '{print $2}'); do kill -9 $pid; done
for pid in $(ps -ef | grep "server2" | awk '{print $2}'); do kill -9 $pid; done
for pid in $(ps -ef | grep "server3" | awk '{print $2}'); do kill -9 $pid; done
for pid in $(ps -ef | grep "coord" | awk '{print $2}'); do kill -9 $pid; done
for pid in $(ps -ef | grep "client1" | awk '{print $2}'); do kill -9 $pid; done