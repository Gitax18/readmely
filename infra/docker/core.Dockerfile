# Builds one of the Go binaries (api, ingest or render) into a minimal image.
# Build from the repository root:
#   docker build -f infra/docker/core.Dockerfile --build-arg CMD=api -t readmely/api:dev .

FROM golang:1.27.2-alpine AS build
WORKDIR /src
COPY services/core/go.mod services/core/go.sum ./
RUN go mod download
COPY services/core/ ./
ARG CMD=api
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/app ./cmd/${CMD}

# Distroless static: no shell, no package manager, runs as a non-root user.
FROM gcr.io/distroless/static-debian13:nonroot
COPY --from=build /out/app /app
USER nonroot:nonroot
ENTRYPOINT ["/app"]
