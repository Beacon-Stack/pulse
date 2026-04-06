.PHONY: build run dev sqlc test clean docker

# Build the binary
build:
	go build -o bin/configurarr ./cmd/configurarr

# Run the server
run: build
	./bin/configurarr

# Run with hot reload (requires air: go install github.com/air-verse/air@latest)
dev:
	air -- -config config.yaml

# Regenerate SQLC code from queries
sqlc:
	sqlc generate

# Run tests
test:
	go test ./... -v -count=1

# Clean build artifacts
clean:
	rm -rf bin/ data/

# Build Docker image
docker:
	docker build -t configurarr:latest .

# Run in Docker
docker-run:
	docker run -d \
		--name configurarr \
		-p 9696:9696 \
		-v configurarr-data:/app/data \
		configurarr:latest

# Tidy Go modules
tidy:
	go mod tidy
