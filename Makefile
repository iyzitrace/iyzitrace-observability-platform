# Makefile for Iyzitrace Observability Platform

.PHONY: all up up-build down restart logs dirs check-dirs gen-creds test validate

DATA_DIR ?= ./data
export DATA_DIR

# Default target
all: up

# Create necessary data directories for local bind mounts
dirs:
	@echo "Creating data directories under $(DATA_DIR)..."
	@mkdir -p "$(DATA_DIR)/auth"
	@mkdir -p "$(DATA_DIR)/lawrence"
	@mkdir -p "$(DATA_DIR)/loki"
	@mkdir -p "$(DATA_DIR)/prometheus"
	@mkdir -p "$(DATA_DIR)/seaweedfs"
	@mkdir -p "$(DATA_DIR)/tempo"
	@mkdir -p "$(DATA_DIR)/thanos-store"
	@mkdir -p "$(DATA_DIR)/thanos-compactor"
	@mkdir -p "$(DATA_DIR)/inventory"
	@mkdir -p "$(DATA_DIR)/alertmanager"
	@echo "Data directories created."

# Generate credentials and config files
gen-creds:
	@chmod +x setup/generate_credentials.py
	@python3 setup/generate_credentials.py

# Start the platform
# Usage:
#   make up         # start without rebuilding images
#   make up-build   # start and rebuild images
up: dirs gen-creds
	@echo "Starting platform..."
	@docker compose up -d --remove-orphans --force-recreate
	@echo "Platform started. Access services at:"
	@echo "  - Dashboard: http://localhost/console"

# Start the platform, rebuilding images first
up-build: dirs gen-creds
	@echo "Starting platform (with rebuild)..."
	@docker compose up -d --remove-orphans --force-recreate --build
	@echo "Platform started. Access services at:"
	@echo "  - Dashboard: http://localhost/console"


# Stop the platform (containers only — data volumes are preserved)
down:
	@echo "Stopping platform..."
	@docker compose down --remove-orphans
	@echo "Platform stopped. Data volumes preserved."

# Destroy everything including all data volumes (irreversible)
nuke:
	@echo "WARNING: This will permanently delete all data volumes (traces, logs, metrics, auth, agents)."
	@printf 'Type YES to confirm: ' && read ans && [ "$$ans" = "YES" ] || (echo "Aborted."; exit 1)
	@docker compose down --volumes --remove-orphans
	@echo "All data volumes removed."

# Restart the platform (preserves data)
restart: down up

# View logs for all services
logs:
	@docker-compose logs -f

# Check which directories exist (helper)
check-dirs:
	@ls -F "$(DATA_DIR)/" || echo "Data directory does not exist: $(DATA_DIR)"

test:
	$(MAKE) -C cli test
	cd inventory-service && go test ./...
	cd opamp && go test ./...
	cd auth-service && npm ci && npm run build

validate: gen-creds
	docker compose config --quiet
	helm dependency update helm/iyzitrace-platform
	helm lint helm/iyzitrace-platform
	helm template iyzitrace helm/iyzitrace-platform --namespace iyzitrace >/tmp/iyzitrace-platform.yaml
	sh scripts/check-binary-assets.sh
	$(MAKE) bundle

####################
# iyzitrace CLI    #
####################

BUNDLE_VERSION := $(shell cat bundle/BUNDLE_VERSION | tr -d ' \n\t')
BUNDLE_TARBALL := dist/iyzitrace-bundle-$(BUNDLE_VERSION).tar.gz

# Package bundle/ into a versioned tarball + sha256 sibling.
bundle: $(BUNDLE_TARBALL)

# Bundle = repo's config/ + docker-compose.yml + bundle/BUNDLE_VERSION + bundle/iyzitrace.yaml.default.
# Assembled into dist/bundle-staging/ then tar'd so the archive root mirrors
# the install dir layout (config/ at top level, docker-compose.yml at top level).
$(BUNDLE_TARBALL):
	@mkdir -p dist/bundle-staging
	@rm -rf dist/bundle-staging/*
	@cp -R config dist/bundle-staging/
	@cp docker-compose.yml dist/bundle-staging/
	@cp bundle/BUNDLE_VERSION dist/bundle-staging/
	@cp bundle/iyzitrace.yaml.default dist/bundle-staging/
	@find dist/bundle-staging -name '._*' -delete
	@find dist/bundle-staging -name '.DS_Store' -delete
	@find dist/bundle-staging/config/nginx/certs -type f ! -name '.gitkeep' -delete 2>/dev/null || true
	# Strip substituted secret-bearing files; only the .template siblings ship.
	@find dist/bundle-staging/config -type f -name '*.template' | sed 's/\.template$$//' | xargs -I{} rm -f {}
	@COPYFILE_DISABLE=1 tar --no-xattrs -czf $@ -C dist/bundle-staging .
	@shasum -a 256 $@ | awk '{print $$1}' > $@.sha256
	@rm -rf dist/bundle-staging
	@echo "wrote $@ ($$(wc -c < $@) bytes), sha256=$$(cat $@.sha256)"

bundle-clean:
	rm -rf dist/

# Build the CLI binary into ./cli/bin/iyzitrace.
cli:
	$(MAKE) -C cli build
