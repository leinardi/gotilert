IMAGE_NAME ?= $(BIN_NAME)

# App runtime args (drop-in defaults)
DOCKER_RUN_PORT ?= 8008
DOCKER_RUN_ARGS ?= --config.file=/config/gotilert.yaml --log-level=info --log-format=plain

# Local config file to mount into the container
CONFIG_FILE ?= examples/gotilert.yaml
CONFIG_FILE_ABS := $(abspath $(CONFIG_FILE))

.PHONY: docker-run
docker-run: ## Run the image locally publishing HTTP port and mounting the config file
	@if [ ! -f "$(CONFIG_FILE_ABS)" ]; then \
		echo "ERROR: Config file not found at '$(CONFIG_FILE_ABS)'."; \
		echo "       Override with: make docker-run CONFIG_FILE=/path/to/gotilert.yaml"; \
		exit 1; \
	fi
	docker run --rm \
		--name "$(IMAGE_NAME)" \
		-p "$(DOCKER_RUN_PORT):8008" \
		--read-only \
		--security-opt no-new-privileges:true \
		--tmpfs /tmp \
		-v "$(CONFIG_FILE_ABS):/config/gotilert.yaml:ro" \
		"$(IMAGE_REPO):$(IMAGE_TAG)" \
		$(DOCKER_RUN_ARGS)
