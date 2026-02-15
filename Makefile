BINARY  := clog
MODULE  := github.com/thinkwright/claude-chronicle
CMD     := ./cmd/clog

.PHONY: build run test lint clean release

build:
	go build -o $(BINARY) $(CMD)

run: build
	./$(BINARY)

test:
	go test ./...

lint:
	go vet ./...

clean:
	rm -f $(BINARY)
	rm -rf dist/

release: clean
	GOOS=darwin  GOARCH=arm64 go build -o dist/$(BINARY)-darwin-arm64  $(CMD)
	GOOS=darwin  GOARCH=amd64 go build -o dist/$(BINARY)-darwin-amd64  $(CMD)
	GOOS=linux   GOARCH=amd64 go build -o dist/$(BINARY)-linux-amd64   $(CMD)
	GOOS=linux   GOARCH=arm64 go build -o dist/$(BINARY)-linux-arm64   $(CMD)
	GOOS=windows GOARCH=amd64 go build -o dist/$(BINARY)-windows-amd64.exe $(CMD)
