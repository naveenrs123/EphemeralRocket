.PHONY: client server coord clean all

all: server coord client

server:
	go build -o bin/server ./cmd/server

coord:
	go build -o bin/coord ./cmd/coord

client:
	go build -o bin/client ./cmd/client

clean:
	rm -f bin/*
