VERSION ?= dev
OUT     := dist
LDFLAGS := -s -w -X main.version=$(VERSION)

.PHONY: all build vet test test-go test-js release clean doctor

all: build

build:
	VERSION=$(VERSION) bash build.sh

vet:
	go vet ./...

test: test-go test-js

test-go:
	go test ./...

# prelude 是内联 JS 字符串，go vet 看不到它的逻辑错误，必须单独跑。
test-js:
	node test/prelude.test.mjs

doctor: build
	./$(OUT)/codex-chinese-$(shell go env GOOS)-$(shell go env GOARCH) -doctor || true

release:
	@test -n "$(VERSION)" || (echo "usage: make release VERSION=v1.0.0"; exit 1)
	VERSION=$(VERSION) bash build.sh

clean:
	rm -rf $(OUT)
