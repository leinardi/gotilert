#
# MIT License
#
# Copyright (c) 2025 Roberto Leinardi
#
# Permission is hereby granted, free of charge, to any person obtaining a copy
# of this software and associated documentation files (the "Software"), to deal
# in the Software without restriction, including without limitation the rights
# to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
# copies of the Software, and to permit persons to whom the Software is
# furnished to do so, subject to the following conditions:
#
# The above copyright notice and this permission notice shall be included in all
# copies or substantial portions of the Software.
#
# THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
# IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
# FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
# AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
# LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
# OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE
# SOFTWARE.
#

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
