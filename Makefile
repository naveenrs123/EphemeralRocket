.PHONY: client server1 server2 server3 coord clean all

all: server1 coord client

server1:
	go build -o bin/server ./cmd/server

server2:
	go build -o bin/server2 ./cmd/server

server3:
	go build -o bin/server3 ./cmd/server

coord:
	go build -o bin/coord ./cmd/coord

client:
	go build -o bin/client ./cmd/client

clean:
	rm -f bin/*
