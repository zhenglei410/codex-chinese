VERSION ?= dev
OUT     := dist
LDFLAGS := -s -w -X main.version=$(VERSION)

.PHONY: all build vet test release clean doctor

all: build

build:
	VERSION=$(VERSION) bash build.sh

vet:
	go vet ./...

test:
	go test ./...

doctor: build
	./$(OUT)/codex-chinese-$(shell go env GOOS)-$(shell go env GOARCH) -doctor || true

release:
	@test -n "$(VERSION)" || (echo "usage: make release VERSION=v1.0.0"; exit 1)
	VERSION=$(VERSION) bash build.sh

clean:
	rm -rf $(OUT)
