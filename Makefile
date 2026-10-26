# NEON//COUP
VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo 1.0.0)
LDFLAGS := -s -w -X main.version=$(VERSION)
TARGETS := linux/amd64 linux/arm64 darwin/amd64 darwin/arm64 windows/amd64

.PHONY: build test test-short race lint vet dist screenshots wav clean

build:
	CGO_ENABLED=0 go build -trimpath -ldflags "$(LDFLAGS)" -o neoncoup ./cmd/neoncoup

test:
	go test ./...

test-short:
	go test -short ./...

race:
	go test -race -short ./...

vet:
	go vet ./...

lint:
	golangci-lint run ./...

# Static binaries for every platform in dist/.
dist:
	@mkdir -p dist
	@for t in $(TARGETS); do \
		os=$${t%/*}; arch=$${t#*/}; ext=""; [ $$os = windows ] && ext=.exe; \
		echo "building $$os/$$arch"; \
		CGO_ENABLED=0 GOOS=$$os GOARCH=$$arch go build -trimpath -ldflags "$(LDFLAGS)" \
			-o dist/neoncoup-$$os-$$arch$$ext ./cmd/neoncoup || exit 1; \
	done

# Regenerate the text screenshots and re-embed them in the README.
screenshots:
	NEONCOUP_SCREENSHOTS=$(CURDIR)/docs/screens NEONCOUP_README=$(CURDIR)/README.md go test ./client -run TestScreenshots -count=1

# Render every song and effect to WAV files in dist/wav.
wav:
	@mkdir -p dist/wav
	NEONCOUP_WAV=$(CURDIR)/dist/wav go test ./audio -run TestWriteWAV -count=1

clean:
	rm -rf neoncoup dist
