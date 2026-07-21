.PHONY: build run test clean

build:
	go build -o scribo main.go

run: build
	./scribo

test:
	go test -v ./...

clean:
	rm -f scribo
