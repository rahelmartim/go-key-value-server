SITE := site
PORT ?= 8080

.PHONY: test build web serve clean

## test: run the tests with the race detector
test:
	go vet ./...
	go test -race ./...

## build: build the kvserver and kvclient commands into bin/
build:
	go build -o bin/ ./cmd/kvserver ./cmd/kvclient

## web: build the browser demo into site/
web:
	rm -rf $(SITE) && mkdir -p $(SITE)
	cp web/* $(SITE)/
	cp "$$(go env GOROOT)/lib/wasm/wasm_exec.js" $(SITE)/
	GOOS=js GOARCH=wasm go build -trimpath -ldflags="-s -w" -o $(SITE)/kv.wasm ./cmd/kvwasm

## serve: build the browser demo and serve it on http://localhost:$(PORT)
serve: web
	python3 -m http.server $(PORT) --directory $(SITE)

clean:
	rm -rf bin $(SITE)
