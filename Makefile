clean:
	rm -rf bin/*
	
linux:
	env GOOS="linux" GOARCH="amd64" go build -o bin/conv

windows:
	env GOOS="windows" GOARCH="amd64" go build -o bin/conv.exe

all: linux windows
