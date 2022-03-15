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
	# CGO_ENABLED=0 GOOS=linux go build -a -ldflags '-extldflags "-static"' -o ./bin/routercommander routercommander.go

routercommander-mac:
	mkdir -p bin
	$(MAKE) -C ./cmd compile-routercommander
	# CGO_ENABLED=0 GOOS=darwin go build -a -ldflags '-extldflags "-static"' -o ./bin/routercommander.mac routercommander.go

clean:
	rm -rf bin

test:
	go test `go list ./... | grep -v 'vendor'` $(TESTARGS)
	go vet `go list ./... | grep -v vendor`
