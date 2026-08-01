.PHONY: build test viruses coverage plan run run-json clean

BIN := mutant

build:
	go build -o $(BIN) ./cmd/mutant

install: build
	mv ./mutant ~/dev/bin

test:
	go test ./... -count=1 -coverprofile=coverage.out
	go tool cover -func=coverage.out | tail -1
	@rm -f coverage.out

viruses: build
	./$(BIN) viruses
	@echo ""
	./$(BIN) viruses --mode text

coverage: build
	./$(BIN) coverage ./example/...

plan: build
	./$(BIN) plan --workers 4 ./example/...

run: build
	./$(BIN) run --workers 4 --mode table --output mutant.json ./example/...

run-json: build
	./$(BIN) run --viruses arithmetic --mode json ./example/...

run-table: build
	./$(BIN) run --mode table ./example/...

clean:
	rm -f $(BIN)

all: test viruses coverage plan run

fix:
	- golangci-lint run
	- govulncheck -show verbose ./...
	- staticcheck ./...
	- go fix ./...
	- go vet ./...
	- command rg -nU '[Ee]rr\s*:=.*\n.*[Ee]rr\s*:=' --glob '*.go' --glob '!*_test.go' .