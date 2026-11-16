# Entry point for every task. On the host only Docker and make are needed:
# each target re-runs itself inside the dev container (IN_DOCKER=1 there).
#
#   make build APP_NAME=x      build for one GOOS/GOARCH into dist/
#   make dist APP_NAME=x       cross-compile six binaries into dist/
#   make test | lint | tidy
#   make check-name APP_NAME=x fail if the name appears anywhere in the tree
#   make run APP_NAME=x ARGS="template list"
#   make shell

-include .env
export APP_NAME VERSION COMMIT ARGS

VERSION := $(or $(VERSION),dev)

.PHONY: build dist test golden lint tidy check-name run shell need-name

ifeq ($(IN_DOCKER),)

export HOST_UID := $(shell id -u)
export HOST_GID := $(shell id -g)
DC := docker compose run --rm dev

build dist test golden lint tidy check-name run:
	$(DC) make $@

shell:
	$(DC) sh

else

PKG       := app/internal/buildinfo
GOOS      := $(or $(GOOS),$(shell go env GOOS))
GOARCH    := $(or $(GOARCH),$(shell go env GOARCH))
COMMIT    := $(or $(COMMIT),$(shell git rev-parse --short HEAD 2>/dev/null))
EXT       := $(if $(filter windows,$(GOOS)),.exe)
LDFLAGS    = -s -w -X $(PKG).Name=$(APP_NAME) -X $(PKG).Version=$(VERSION) -X $(PKG).Commit=$(COMMIT)
PLATFORMS := linux/amd64 linux/arm64 darwin/amd64 darwin/arm64 windows/amd64 windows/arm64
HOME_VAR   = $(shell printf %s "$(APP_NAME)" | tr 'a-z' 'A-Z' | tr -c 'A-Z0-9' _)_HOME

need-name:
	@test -n "$(APP_NAME)" || { echo "APP_NAME is required (pass it or put it in .env)" >&2; exit 1; }

build: need-name
	GOOS=$(GOOS) GOARCH=$(GOARCH) go build -trimpath -ldflags "$(LDFLAGS)" -o dist/$(APP_NAME)$(EXT) ./cmd/app

dist: need-name
	@for p in $(PLATFORMS); do \
	  os=$${p%/*}; arch=$${p#*/}; ext=""; \
	  if [ "$$os" = windows ]; then ext=.exe; fi; \
	  echo "build $$os/$$arch"; \
	  GOOS=$$os GOARCH=$$arch go build -trimpath -ldflags "$(LDFLAGS)" \
	    -o "dist/$(APP_NAME)-$$os-$$arch$$ext" ./cmd/app || exit 1; \
	done

test:
	go test ./...

golden:
	go test ./internal/scaffold -run Golden -update

lint:
	@out="$$(gofmt -l .)"; if [ -n "$$out" ]; then echo "gofmt needed:" >&2; echo "$$out" >&2; exit 1; fi
	go vet ./...

tidy:
	go mod tidy

check-name: need-name
	@out="$$(find . \( -path ./.git -o -path ./dist -o -name .env \) -prune -o -type f -print0 \
	  | xargs -0 grep -Iil -e "$(APP_NAME)" || true)"; \
	if [ -n "$$out" ]; then echo "error: the app name appears in:" >&2; echo "$$out" >&2; exit 1; fi; \
	echo "check-name: ok"

run: need-name
	env $(HOME_VAR)=/tmp/store go run -ldflags "-X $(PKG).Name=$(APP_NAME)" ./cmd/app $(ARGS)

endif
