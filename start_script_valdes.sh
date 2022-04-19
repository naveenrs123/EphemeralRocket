#!/bin/bash

# trap 'kill $(jobs -p)' EXIT
export SERVER_CONFIG=config/server_config1.json
bin/server1 &
sleep 1
export SERVER_CONFIG=config/server_config3.json
bin/server3