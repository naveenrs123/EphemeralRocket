.PHONY: client1 server1 server2 server3 coord clean all

all: server1 server2 server3 coord client1

server1:
	go build -o bin/server1 ./cmd/server

server2:
	go build -o bin/server2 ./cmd/server

server3:
	go build -o bin/server3 ./cmd/server

coord:
	go build -o bin/coord ./cmd/coord

client1:
	go build -o bin/client1 ./cmd/client

clean:
	rm -f bin/*
