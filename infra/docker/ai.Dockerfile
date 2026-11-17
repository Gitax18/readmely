# Python AI service image. Build from the repository root:
#   docker build -f infra/docker/ai.Dockerfile -t readmely/ai:dev .

FROM ghcr.io/astral-sh/uv:0.13.0@sha256:cdc6093146eb3ff6a40107b38f008b789e050e77ad87865e381d9917da55a168 AS uv

FROM python:3.13.16-slim-trixie@sha256:cf50a4f6ac4f026458b08d5222023d700ac806410dff5e9ca8728fa44f2b850f AS build
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
FROM python:3.13.16-slim-trixie@sha256:cf50a4f6ac4f026458b08d5222023d700ac806410dff5e9ca8728fa44f2b850f
# The service never installs anything at run time, so the package manager goes: it
# bundles its own copies of libraries (certifi, urllib3, msgpack) that scanners flag
# and that nothing here uses.
#
# Debian published security fixes (apt, zlib1g) that the newest base image does not have
# yet, so those packages alone are upgraded. Remove this once a base image includes them;
# CI's Trivy scan will keep passing either way.
RUN apt-get update \
 && apt-get install --yes --no-install-recommends --only-upgrade apt libapt-pkg7.0 zlib1g \
 && rm -rf /var/lib/apt/lists/* \
 && python -m pip uninstall --yes pip \
 && groupadd --system --gid 10001 app \
 && useradd --system --uid 10001 --gid app --no-create-home --shell /usr/sbin/nologin app
WORKDIR /app
COPY --from=build --chown=root:root /app /app
ENV PATH="/app/.venv/bin:$PATH" \
    PYTHONDONTWRITEBYTECODE=1 \
    PYTHONUNBUFFERED=1
USER 10001:10001
EXPOSE 8000
CMD ["python", "-m", "app.server"]
