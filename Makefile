.PHONY: client server1 server2 server3 server4 server5 server6 coord clean all

all: server1 server2 server3 server4 server5 server6 coord client 

server1:
	go build -o bin/server1 ./cmd/server

server2:
	go build -o bin/server2 ./cmd/server

server3:
	go build -o bin/server3 ./cmd/server

server4:
	go build -o bin/server4 ./cmd/server

server5:
	go build -o bin/server5 ./cmd/server

server6:
	go build -o bin/server6 ./cmd/server	

coord:
	go build -o bin/coord ./cmd/coord

client:
	go build -o bin/client ./cmd/client

clean:
	rm -f bin/*
