.PHONY: build test lint clean install

build:
	go build -o builder ./cmd/builder

test:
	go test ./...

clean:
	rm -f builder

install: build
	cp builder /usr/local/bin/builder
