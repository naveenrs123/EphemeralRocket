#!/bin/bash

# trap 'kill $(jobs -p)' EXIT
bin/coord &
sleep 2
export SERVER_CONFIG=config/server_config1.json
bin/server1 &
sleep 2
export SERVER_CONFIG=config/server_config2.json
bin/server2 &
sleep 2
export SERVER_CONFIG=config/server_config3.json
bin/server3 &






