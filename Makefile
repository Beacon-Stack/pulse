.PHONY: build run dev sqlc test clean docker

# Build the binary
build:
	go build -o bin/pulse ./cmd/pulse

# Run the server
run: build
	./bin/pulse

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
	docker build -t pulse:latest .

# Run in Docker
docker-run:
	docker run -d \
		--name pulse \
		-p 9696:9696 \
		-v pulse-data:/app/data \
		pulse:latest

# Tidy Go modules
tidy:
	go mod tidy
