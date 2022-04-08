#!/bin/bash

test="$1"
for pid in $(ps -ef | grep $test| awk '{print $2}'); do kill -9 $pid; done



