# livestream-viewer Makefile

GO_PKGS = ./cmd/... ./internal/... ./api/...
GO_FILES = $(shell find . -name '*.go' -not -path './frontend/node_modules/*' -not -path './research/*')

.PHONY: frontend build build-server test vet fmt fmt-check check

fmt:
	gofmt -w $(GO_FILES)

fmt-check:
	@unformatted=$$(gofmt -l $(GO_FILES)); \
	if [ -n "$$unformatted" ]; then echo "gofmt needed for:"; echo "$$unformatted"; exit 1; fi

vet:
	go vet $(GO_PKGS)

test:
	CGO_ENABLED=1 go test -timeout=30s $(GO_PKGS)

check: fmt-check vet test

frontend:
	cd frontend && yarn install --frozen-lockfile && yarn build
	rm -rf cmd/livestream-viewer/frontend-dist
	cp -r frontend/dist cmd/livestream-viewer/frontend-dist

build-server:
	SKIP_FRONTEND=true ./scripts/build.sh

build:
	./scripts/build.sh
