# Next.js web image. Build from the repository root:
#   docker build -f infra/docker/web.Dockerfile -t readmely/web:dev .

FROM node:24.22.2-alpine@sha256:a34ab6d7f6f4813f4d0c5469d74685604b962fbdbf5793a7bc7b4998ecb867c8 AS build
# pnpm is pinned to the version in package.json; the release-age and
# install-script policies in pnpm-workspace.yaml apply here too.
RUN npm install --global --ignore-scripts pnpm@12.10.1
ENV NEXT_TELEMETRY_DISABLED=1
WORKDIR /app
# Dependencies first, so code changes don't invalidate this layer.
# --frozen-lockfile fails if pnpm-lock.yaml is out of date.
COPY apps/web/package.json apps/web/pnpm-lock.yaml apps/web/pnpm-workspace.yaml ./
RUN pnpm install --frozen-lockfile
COPY apps/web/ ./
RUN pnpm run build

# Runtime: distroless Node. No shell, no npm, no package manager; non-root (65532).
FROM gcr.io/distroless/nodejs24-debian13:nonroot@sha256:6abfcb367ebc291572c774c1592a986be2ab26a2486f73093401327495a5962a
WORKDIR /app
ENV NODE_ENV=production \
    NEXT_TELEMETRY_DISABLED=1 \
    HOSTNAME=0.0.0.0 \
    PORT=3000
COPY --from=build --chown=root:root /app/.next/standalone ./
COPY --from=build --chown=root:root /app/.next/static ./.next/static
USER 65532:65532
EXPOSE 3000
# Distroless has no shell; this is the Node entrypoint with the script as argument.
CMD ["server.js"]
