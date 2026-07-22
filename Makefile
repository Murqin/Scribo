.PHONY: build build-linux-amd64 build-linux-arm64 build-windows-amd64 build-windows-arm64 build-darwin-amd64 build-darwin-arm64 release run test clean

LDFLAGS=-ldflags "-s -w"

build:
	CGO_ENABLED=0 go build $(LDFLAGS) -o scribo main.go

build-linux-amd64:
	CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build $(LDFLAGS) -o scribo-linux-amd64 main.go

build-linux-arm64:
	CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go build $(LDFLAGS) -o scribo-linux-arm64 main.go

build-windows-amd64:
	CGO_ENABLED=0 GOOS=windows GOARCH=amd64 go build $(LDFLAGS) -o scribo-windows-amd64.exe main.go

build-windows-arm64:
	CGO_ENABLED=0 GOOS=windows GOARCH=arm64 go build $(LDFLAGS) -o scribo-windows-arm64.exe main.go

build-darwin-amd64:
	CGO_ENABLED=0 GOOS=darwin GOARCH=amd64 go build $(LDFLAGS) -o scribo-darwin-amd64 main.go

build-darwin-arm64:
	CGO_ENABLED=0 GOOS=darwin GOARCH=arm64 go build $(LDFLAGS) -o scribo-darwin-arm64 main.go

release: build-linux-amd64 build-linux-arm64 build-windows-amd64 build-windows-arm64 build-darwin-amd64 build-darwin-arm64
	rm -rf dist && mkdir -p dist/scribo-linux-amd64/scribo dist/scribo-linux-arm64/scribo dist/scribo-windows-amd64/scribo dist/scribo-windows-arm64/scribo dist/scribo-darwin-amd64/scribo dist/scribo-darwin-arm64/scribo
	cp scribo-linux-amd64 dist/scribo-linux-amd64/scribo/scribo
	cp scribo-linux-arm64 dist/scribo-linux-arm64/scribo/scribo
	cp scribo-windows-amd64.exe dist/scribo-windows-amd64/scribo/scribo.exe
	cp scribo-windows-arm64.exe dist/scribo-windows-arm64/scribo/scribo.exe
	cp scribo-darwin-amd64 dist/scribo-darwin-amd64/scribo/scribo
	cp scribo-darwin-arm64 dist/scribo-darwin-arm64/scribo/scribo
	cp setup_service.sh restart.sh stop.sh uninstall.sh README.md LICENSE dist/scribo-linux-amd64/scribo/
	cp setup_service.sh restart.sh stop.sh uninstall.sh README.md LICENSE dist/scribo-linux-arm64/scribo/
	cp README.md LICENSE dist/scribo-windows-amd64/scribo/
	cp README.md LICENSE dist/scribo-windows-arm64/scribo/
	cp README.md LICENSE dist/scribo-darwin-amd64/scribo/
	cp README.md LICENSE dist/scribo-darwin-arm64/scribo/
	cp modes.example.json dist/scribo-linux-amd64/scribo/modes.json
	cp modes.example.json dist/scribo-linux-arm64/scribo/modes.json
	cp modes.example.json dist/scribo-windows-amd64/scribo/modes.json
	cp modes.example.json dist/scribo-windows-arm64/scribo/modes.json
	cp modes.example.json dist/scribo-darwin-amd64/scribo/modes.json
	cp modes.example.json dist/scribo-darwin-arm64/scribo/modes.json
	cp .env.example dist/scribo-linux-amd64/scribo/.env
	cp .env.example dist/scribo-linux-arm64/scribo/.env
	cp .env.example dist/scribo-windows-amd64/scribo/.env
	cp .env.example dist/scribo-windows-arm64/scribo/.env
	cp .env.example dist/scribo-darwin-amd64/scribo/.env
	cp .env.example dist/scribo-darwin-arm64/scribo/.env
	tar -czvf dist/scribo-linux-amd64.tar.gz -C dist/scribo-linux-amd64 scribo
	tar -czvf dist/scribo-linux-arm64.tar.gz -C dist/scribo-linux-arm64 scribo
	python3 -m zipfile -c dist/scribo-windows-amd64.zip dist/scribo-windows-amd64/scribo
	python3 -m zipfile -c dist/scribo-windows-arm64.zip dist/scribo-windows-arm64/scribo
	tar -czvf dist/scribo-darwin-amd64.tar.gz -C dist/scribo-darwin-amd64 scribo
	tar -czvf dist/scribo-darwin-arm64.tar.gz -C dist/scribo-darwin-arm64 scribo
	@echo "✅ Hazır yayın paketleri dist/ klasöründe oluşturuldu!"

run: build
	./scribo

test:
	go test -race -v ./...

clean:
	rm -f scribo scribo-linux-amd64 scribo-linux-arm64 scribo-windows-amd64.exe scribo-windows-arm64.exe scribo-darwin-amd64 scribo-darwin-arm64
	rm -rf dist
