# Common tasks. CI is optional: every check can be run locally with these.
GO ?= go
PREFIX ?= $(HOME)/.local/bin
# The repo ships the short name 502d (502 is left free for other tools).
# `make install ALIAS=502` if you prefer the plain number.
ALIAS ?= 502d
VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
LDFLAGS := -s -w -X github.com/itsgiahung/502Drive/internal/cli.Version=$(VERSION)

.PHONY: build check test docs install uninstall docker release-local clean

build:
	$(GO) build -trimpath -ldflags "$(LDFLAGS)" -o bin/502drive ./cmd/502drive
	$(GO) build -trimpath -ldflags "$(LDFLAGS)" -o bin/502drive-tray ./cmd/502drive-tray

check:
	@test -z "$$(gofmt -l cmd internal)" || { gofmt -l cmd internal; exit 1; }
	$(GO) vet ./...
	$(GO) test -race -count=1 ./...

test:
	$(GO) test -count=1 ./...

docs:
	python3 scripts/check-docs-links.py .
	python3 -c "import json; json.load(open('docs/contracts/openapi.json'))"
	git diff --check

# install puts the binary on PATH and adds the short `502` name scripts use.
install: build
	install -d "$(DESTDIR)$(PREFIX)"
	install -m 0755 bin/502drive "$(DESTDIR)$(PREFIX)/502drive"
	ln -sf 502drive "$(DESTDIR)$(PREFIX)/$(ALIAS)"
	@printf 'installed %s/502drive and the %s symlink\n' "$(DESTDIR)$(PREFIX)" "$(ALIAS)"

uninstall:
	rm -f "$(DESTDIR)$(PREFIX)/502drive" "$(DESTDIR)$(PREFIX)/$(ALIAS)"

docker:
	docker build --build-arg VERSION=$(VERSION) -t 502drive:latest .

release-local:
	scripts/release-local.sh

clean:
	rm -rf bin dist build
