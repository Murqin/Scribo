.PHONY: build build-linux-amd64 build-linux-arm64 release run test clean

LDFLAGS=-ldflags "-s -w"

build:
	go build $(LDFLAGS) -o scribo main.go

build-linux-amd64:
	GOOS=linux GOARCH=amd64 go build $(LDFLAGS) -o scribo-linux-amd64 main.go

build-linux-arm64:
	GOOS=linux GOARCH=arm64 go build $(LDFLAGS) -o scribo-linux-arm64 main.go

release: build-linux-amd64 build-linux-arm64
	rm -rf dist && mkdir -p dist/scribo-linux-amd64/scribo dist/scribo-linux-arm64/scribo
	cp scribo-linux-amd64 dist/scribo-linux-amd64/scribo/scribo
	cp scribo-linux-arm64 dist/scribo-linux-arm64/scribo/scribo
	cp setup_service.sh README.md LICENSE dist/scribo-linux-amd64/scribo/
	cp setup_service.sh README.md LICENSE dist/scribo-linux-arm64/scribo/
	cp modes.example.json dist/scribo-linux-amd64/scribo/modes.json
	cp modes.example.json dist/scribo-linux-arm64/scribo/modes.json
	cp .env.example dist/scribo-linux-amd64/scribo/.env
	cp .env.example dist/scribo-linux-arm64/scribo/.env
	tar -czvf dist/scribo-linux-amd64.tar.gz -C dist/scribo-linux-amd64 scribo
	tar -czvf dist/scribo-linux-arm64.tar.gz -C dist/scribo-linux-arm64 scribo
	@echo "✅ Hazır yayın paketleri dist/ klasöründe oluşturuldu!"

run: build
	./scribo

test:
	go test -v ./...

clean:
	rm -f scribo scribo-linux-amd64 scribo-linux-arm64
	rm -rf dist
