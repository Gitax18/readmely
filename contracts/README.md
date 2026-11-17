# API contracts

The shape of every HTTP API in Readmely is written down here once, as OpenAPI 3.0.3, one file per server:

| File | Server | Reachable from |
|---|---|---|
| `core-public.yaml` | Go API, public listener | the internet |
| `core-internal.yaml` | Go API, internal listener (`/readyz`, `/api/v1/status`) | the internal network only |
| `ai.yaml` | Python AI service (bearer token required except `/healthz`) | the Go API, internal network only |

Why 3.0.3 and not 3.1: the Go validator (`kin-openapi`) has no confirmed 3.1 support and `oapi-codegen` only an initial one. Move up when both catch up.

## Rules

- Every object has `additionalProperties: false`, status values are enums, and each response documents `X-Request-ID`.
- Files are self-contained. The schemas `Error`, `Liveness`, `CheckState`, `CheckMap`, `Readiness` and the `RequestId` header are repeated in each file and must be identical; `make contracts-check` fails if they differ.
- OpenAPI 3.0 cannot say "property names must match a pattern", so the rule for dependency names (`^[a-z][a-z0-9_]{0,31}$`) is stated in the `CheckMap` description and enforced in code on both sides.
- Add examples to every response. They are checked against their schemas here, and reused by tests in the other languages.
- A change to an API starts here, in the same commit as the code that implements it.

## Checking

`make contracts-check` validates the specs, confirms the shared schemas match, and confirms every example fits its schema. It is tested against planted faults in `tests/`.
