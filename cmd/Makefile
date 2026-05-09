compile-routercommander:
	CGO_ENABLED=0 GOOS=linux GOARCH=amd64 GO111MODULE=on go build -a -ldflags '-extldflags "-static"' -o ../bin/routercommander ./routercommander.go ./ssh.go ./pipeline.go

compile-routercommander-mac:
	CGO_ENABLED=0 GOOS=darwin GO111MODULE=on go build -a -ldflags '-extldflags "-static"' -o ../bin/routercommander.mac ./routercommander.go ./ssh.go ./pipeline.go

compile-routercommander-win:
	CGO_ENABLED=0 GOOS=windows GOARCH=amd64 GO111MODULE=on go build -a -ldflags '-extldflags "-static"' -o ../bin/routercommander.win ./routercommander.go ./ssh.go ./pipeline.go

