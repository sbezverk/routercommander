.PHONY: all routercommander-mac routercommander clean test

ifdef V
TESTARGS = -v -args -alsologtostderr -v 5
else
TESTARGS =
endif

all: routercommander

routercommander:
	mkdir -p bin
	$(MAKE) -C ./cmd compile-routercommander

routercommander-mac:
	mkdir -p bin
	$(MAKE) -C ./cmd compile-routercommander-mac

routercommander-win:
	mkdir -p bin
	$(MAKE) -C ./cmd compile-routercommander-win
clean:
	rm -rf bin

test:
	go test `go list ./... | grep -v 'vendor'` $(TESTARGS)
	go vet `go list ./... | grep -v vendor`
