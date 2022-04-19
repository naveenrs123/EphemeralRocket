#!/bin/bash

# trap 'kill $(jobs -p)' EXIT
make all
bin/coord &
sleep 1
export SERVER_CONFIG=config/server_config6.json
bin/server6



