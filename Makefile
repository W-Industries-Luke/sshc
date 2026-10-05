PREFIX  ?= $(HOME)/.local
GOFLAGS := -trimpath -ldflags=-s\ -w
export CGO_ENABLED = 0

.PHONY: build test e2e install dist clean

build:
	go build $(GOFLAGS) -o sshc .

test:
	go vet ./...
	go test ./...

# Needs Docker and a local OpenSSH client.
e2e: build
	test/e2e.sh ./sshc

install: build
	install -d $(PREFIX)/bin
	install -m 755 sshc $(PREFIX)/bin/sshc

dist:
	GOOS=linux   GOARCH=amd64 go build $(GOFLAGS) -o dist/sshc-linux-amd64 .
	GOOS=linux   GOARCH=arm64 go build $(GOFLAGS) -o dist/sshc-linux-arm64 .
	GOOS=darwin  GOARCH=amd64 go build $(GOFLAGS) -o dist/sshc-darwin-amd64 .
	GOOS=darwin  GOARCH=arm64 go build $(GOFLAGS) -o dist/sshc-darwin-arm64 .
	GOOS=windows GOARCH=amd64 go build $(GOFLAGS) -o dist/sshc-windows-amd64.exe .
	GOOS=windows GOARCH=arm64 go build $(GOFLAGS) -o dist/sshc-windows-arm64.exe .

clean:
	rm -rf sshc sshc.exe dist
