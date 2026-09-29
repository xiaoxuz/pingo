.PHONY: all build-hub build-sidecar build-pingo build-admin package-pingo run-hub run-sidecar run-admin migrate test docker-up docker-down clean help

all: build-hub build-sidecar build-pingo

build-hub:
	@echo "Building Hub..."
	@cd src/hub && go build -o ../../bin/pingo-hub ./cmd/hub
	@echo "Hub binary built: bin/pingo-hub"

build-sidecar:
	@echo "Building Sidecar..."
	@set -a; . "$${PINGO_RELEASE_CONFIG:-./release/pingo-package.conf}"; set +a; cd src/sidecar && go build -ldflags "-X github.com/pingo/sidecar/internal/config.BuiltinHubEndpoint=$$PINGO_HUB_WS_URL -X github.com/pingo/sidecar/internal/config.Version=$$PINGO_VERSION -X github.com/pingo/sidecar/internal/config.Channel=$$PINGO_CHANNEL -X github.com/pingo/sidecar/internal/config.UpdateManifestURL=$$PINGO_UPDATE_MANIFEST_URL -X github.com/pingo/sidecar/internal/config.UpdateCheckInterval=$$PINGO_UPDATE_CHECK_INTERVAL" -o ../../bin/pingo-sidecar ./cmd/sidecar
	@echo "Sidecar binary built: bin/pingo-sidecar"

build-pingo:
	@echo "Building Pingo..."
	@cd src/pingo && go build -o ../../bin/pingo ./cmd/pingo
	@echo "Pingo binary built: bin/pingo"

build-admin:
	@echo "Building Admin Web..."
	@cd src/admin-web && pnpm install --frozen-lockfile && pnpm build
	@echo "Admin Web built: src/admin-web/dist"

package-pingo: build-sidecar build-pingo
	@echo "Packaging Pingo Sidecar..."
	@./scripts/package-pingo.sh

run-hub: build-hub
	@echo "Starting Hub..."
	@./bin/pingo-hub src/hub/configs/hub.yaml

run-sidecar: build-sidecar
	@echo "Starting Sidecar..."
	@./bin/pingo-sidecar

run-admin:
	@echo "Starting Admin Web..."
	@cd src/admin-web && pnpm dev --host 0.0.0.0

migrate:
	@echo "Running migrations..."
	@echo "Make sure PostgreSQL is running and migrations/001_init.sql is applied"
	@echo "Usage: psql -h localhost -U pingo -d pingo -f migrations/001_init.sql"

test:
	@echo "Running Hub tests..."
	@cd src/hub && go test ./...
	@echo "Running Sidecar tests..."
	@cd src/sidecar && go test ./...

docker-up:
	@echo "Starting Docker services..."
	docker-compose up -d

docker-down:
	@echo "Stopping Docker services..."
	docker-compose down

clean:
	@echo "Cleaning..."
	@rm -rf bin/ dist/
	@cd src/hub && go clean
	@cd src/sidecar && go clean

help:
	@echo "Pingo Makefile"
	@echo ""
	@echo "Targets:"
	@echo "  all           - Build hub and sidecar"
	@echo "  build-hub     - Build hub binary"
	@echo "  build-sidecar - Build sidecar binary"
	@echo "  build-pingo   - Build Pingo native CLI bridge"
	@echo "  build-admin   - Build admin web"
	@echo "  package-pingo - Build the standalone Sidecar installation package"
	@echo "  run-hub       - Build and run hub locally"
	@echo "  run-sidecar   - Build and run sidecar locally"
	@echo "  run-admin     - Start admin web locally"
	@echo "  migrate       - Run database migrations"
	@echo "  test          - Run all tests"
	@echo "  docker-up     - Start Docker services"
	@echo "  docker-down   - Stop Docker services"
	@echo "  clean         - Clean build artifacts"
