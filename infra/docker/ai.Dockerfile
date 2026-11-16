# Python AI service image. Build from the repository root:
#   docker build -f infra/docker/ai.Dockerfile -t readmely/ai:dev .

FROM ghcr.io/astral-sh/uv:0.13.0 AS uv

FROM python:3.13.16-slim-trixie AS build
COPY --from=uv /uv /usr/local/bin/uv
ENV UV_COMPILE_BYTECODE=1 \
    UV_LINK_MODE=copy \
    UV_PYTHON_DOWNLOADS=never \
    UV_PROJECT_ENVIRONMENT=/app/.venv
WORKDIR /app
# Dependencies first, so code changes don't invalidate this layer. --frozen fails if uv.lock is stale.
COPY services/ai/pyproject.toml services/ai/uv.lock services/ai/.python-version ./
RUN uv sync --frozen --no-dev --no-install-project
COPY services/ai/app ./app
RUN python -m compileall -q app

# Runtime: same base, no uv, no build tools, non-root.
FROM python:3.13.16-slim-trixie
RUN groupadd --system --gid 10001 app \
 && useradd --system --uid 10001 --gid app --no-create-home --shell /usr/sbin/nologin app
WORKDIR /app
COPY --from=build --chown=root:root /app /app
ENV PATH="/app/.venv/bin:$PATH" \
    PYTHONDONTWRITEBYTECODE=1 \
    PYTHONUNBUFFERED=1
USER 10001:10001
EXPOSE 8000
CMD ["python", "-m", "app.server"]
