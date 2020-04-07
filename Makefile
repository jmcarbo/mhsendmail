VERSION=0.1.9

all: release

release: 
	GOOS=linux GOARCH=amd64 go build -o build/mhsendmail .



.PNONY: all release release-deps
