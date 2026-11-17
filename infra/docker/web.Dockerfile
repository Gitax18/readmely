# Next.js web image. Build from the repository root:
#   docker build -f infra/docker/web.Dockerfile -t readmely/web:dev .

FROM node:26.13.0-alpine@sha256:8996bb5821d62a8fd9bb27c1cb07462f3fdce6e47d640c6a881ead0b064ce216 AS build
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
