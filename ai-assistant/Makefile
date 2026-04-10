.PHONY: install install-dev run run-debug test clean docker-build docker-run help

SHELL := /bin/bash
PORT ?= 3000
HOST ?= 0.0.0.0
IMAGE_NAME ?= ai-assistant

help: ## Show this help
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | sort | \
		awk 'BEGIN {FS = ":.*?## "}; {printf "  \033[36m%-18s\033[0m %s\n", $$1, $$2}'

install: ## Install runtime dependencies
	uv sync

install-dev: ## Install all dependencies (runtime + dev)
	uv sync --all-extras

run: install ## Run the service
	uv run uvicorn app:app --host $(HOST) --port $(PORT) --reload

run-debug: install ## Run with DEBUG logging
	LOG_LEVEL=DEBUG uv run uvicorn app:app --host $(HOST) --port $(PORT) --reload --log-level debug

test: install-dev ## Run tests
	uv run python -m pytest tests/ -v --tb=short

clean: ## Remove caches
	rm -rf __pycache__ tools/__pycache__ tests/__pycache__
	rm -rf .pytest_cache

docker-build: ## Build Docker image
	docker build -t $(IMAGE_NAME) .

docker-run: ## Run Docker container
	docker run --rm -p $(PORT):3000 --env-file .env $(IMAGE_NAME)

lock: ## Update uv lockfile
	uv lock
