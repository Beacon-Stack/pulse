.PHONY: build run dev sqlc test clean docker prowlarr-snapshot

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

# Refresh the embedded Prowlarr definitions snapshot (offline fallback for
# the scraper engine). Repacks definitions/v11 from the current zipball.
prowlarr-snapshot:
	rm -rf /tmp/prowlarr-snapshot && mkdir -p /tmp/prowlarr-snapshot/definitions/v11
	curl -sfL -o /tmp/prowlarr-snapshot/full.zip \
		'https://codeload.github.com/Prowlarr/Indexers/legacy.zip/refs/heads/master'
	unzip -q -j /tmp/prowlarr-snapshot/full.zip '*/definitions/v11/*.yml' \
		-d /tmp/prowlarr-snapshot/definitions/v11
	cd /tmp/prowlarr-snapshot && zip -q -9 -X -r snapshot.zip definitions
	mv /tmp/prowlarr-snapshot/snapshot.zip internal/core/indexer/prowlarr_snapshot.zip
	rm -rf /tmp/prowlarr-snapshot
