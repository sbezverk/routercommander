compile-routercommander:
	CGO_ENABLED=0 GOOS=linux GO111MODULE=on go build -a -ldflags '-extldflags "-static"' -o ../bin/routercommander ./routercommander.go

compile-routercommander-mac:
	CGO_ENABLED=0 GOOS=darwin GO111MODULE=on go build -a -ldflags '-extldflags "-static"' -o ../bin/routercommander.mac ./routercommander.go

compile-routercommander-win:
	CGO_ENABLED=0 GOOS=windows GO111MODULE=on go build -a -ldflags '-extldflags "-static"' -o ../bin/routercommander.win ./routercommander.go

