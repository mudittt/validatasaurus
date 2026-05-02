.PHONY: build run tidy clean

build:
	CGO_ENABLED=0 go build -ldflags="-s -w" -o validatasaurus ./cmd

run:
	go run ./cmd $(ARGS)

tidy:
	GOPROXY=direct GONOSUMDB='*' go mod tidy

clean:
	rm -f validatasaurus && rm -rf /tmp/validatasaurus
