#!/bin/bash

# trap 'kill $(jobs -p)' EXIT
bin/coord &
sleep 1
export SERVER_CONFIG=config/server_config2.json
bin/server2 &
sleep 1
export SERVER_CONFIG=config/server_config3.json
bin/server3 &
sleep 1
export SERVER_CONFIG=config/server_config4.json
bin/server4 &
sleep 1
export SERVER_CONFIG=config/server_config5.json
bin/server5 &
sleep 1
export SERVER_CONFIG=config/server_config6.json
bin/server6
