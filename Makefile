.PHONY: build build-linux-amd64 build-linux-arm64 run test clean

LDFLAGS=-ldflags "-s -w"

build:
	go build $(LDFLAGS) -o scribo main.go

build-linux-amd64:
	GOOS=linux GOARCH=amd64 go build $(LDFLAGS) -o scribo-linux-amd64 main.go

build-linux-arm64:
	GOOS=linux GOARCH=arm64 go build $(LDFLAGS) -o scribo-linux-arm64 main.go

run: build
	./scribo

test:
	go test -v ./...

clean:
	rm -f scribo scribo-linux-amd64 scribo-linux-arm64
