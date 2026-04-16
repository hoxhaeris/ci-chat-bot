FROM python:3.12-slim

WORKDIR /app

# Install git for workspace tools (clone and browse repos like openshift/release)
RUN apt-get update && apt-get install -y --no-install-recommends git && rm -rf /var/lib/apt/lists/*

# Install uv for fast dependency resolution
COPY --from=ghcr.io/astral-sh/uv:latest /uv /usr/local/bin/uv

# Install Python dependencies from lockfile
COPY pyproject.toml uv.lock ./
RUN uv sync --frozen --no-dev --no-editable

# Copy application code
COPY . .

# Expose the service port
EXPOSE 3000

# Run the service (binds to 0.0.0.0 for pod networking)
CMD ["uv", "run", "uvicorn", "app:app", "--host", "0.0.0.0", "--port", "3000"]
