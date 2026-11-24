.PHONY: build run dev sqlc test lint deadcode deadcode/go web check clean docker docker-run tidy prowlarr-snapshot

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
	go test ./... -count=1

# Lint
lint:
	golangci-lint run

# Fail on unreachable Go code or unused frontend files, exports and packages.
# pkg/ is skipped: the other Beacon apps import it, so a function Pulse
# itself never calls is not dead.
deadcode: deadcode/go
	cd web/ui && npm run deadcode

deadcode/go:
	@out="$$(go run golang.org/x/tools/cmd/deadcode@v0.53.0 -test ./... | grep -v -e node_modules -e '^pkg/')"; echo "$$out"; test -z "$$out"

# Build the frontend into web/static
web:
	cd web/ui && npm run build

# Everything CI checks
check: lint deadcode test web

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
