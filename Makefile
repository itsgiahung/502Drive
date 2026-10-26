# Common tasks. CI is optional: every check can be run locally with these.
GO ?= go

.PHONY: build check test docs release-local clean

build:
	$(GO) build -o bin/502drive ./cmd/502drive
	$(GO) build -o bin/502drive-tray ./cmd/502drive-tray

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

release-local:
	scripts/release-local.sh

clean:
	rm -rf bin dist build
