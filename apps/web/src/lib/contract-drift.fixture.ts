// Type-check only (never imported). Each schema below has drifted from the
// contract in a different way, and each line is expected to FAIL the check.
// `@ts-expect-error` turns "no error" into an error, so if SameShape ever stops
// noticing one of these drifts, `tsc` fails on the unused directive.
import { z } from "zod";
import type { GeneratedReport, SameShape } from "./contract-check";

const checks = z.record(z.string(), z.enum(["ok", "down"]));
const serviceStatus = ["ok", "down", "unreachable", "unconfigured"] as const;
const overallStatus = ["operational", "degraded", "unavailable"] as const;

const service = z.object({ status: z.enum(serviceStatus), checks }).strict();
const build = <S extends z.ZodType, V extends z.ZodType>(status: S, svc: V) =>
  z.object({ status, services: z.object({ api: svc, ai: svc }).strict() }).strict();

// Sanity: the undrifted schema matches, so a failure below is caused by the drift.
export const good = build(z.enum(overallStatus), service);
export const sanity: SameShape<z.infer<typeof good>, GeneratedReport> = true;

// 1. Zod accepts an overall status the contract does not allow.
export const extraOverall = build(z.enum([...overallStatus, "maintenance"]), service);
// @ts-expect-error drift: extra value in Zod
export const d1: SameShape<z.infer<typeof extraOverall>, GeneratedReport> = true;

// 2. Zod rejects an overall status the contract allows.
export const missingOverall = build(z.enum(["operational", "degraded"]), service);
// @ts-expect-error drift: value missing in Zod
export const d2: SameShape<z.infer<typeof missingOverall>, GeneratedReport> = true;

// 3. A service state is missing in Zod.
const fewerStates = z.object({ status: z.enum(["ok", "down"]), checks }).strict();
export const missingState = build(z.enum(overallStatus), fewerStates);
// @ts-expect-error drift: service state missing in Zod
export const d3: SameShape<z.infer<typeof missingState>, GeneratedReport> = true;

// 4. Zod drops a required service.
export const missingService = z
  .object({ status: z.enum(overallStatus), services: z.object({ api: service }).strict() })
  .strict();
// @ts-expect-error drift: ai service missing in Zod
export const d4: SameShape<z.infer<typeof missingService>, GeneratedReport> = true;

// 5. Zod has an extra field the contract does not.
export const extraField = z
  .object({
    status: z.enum(overallStatus),
    services: z.object({ api: service, ai: service }).strict(),
    debug: z.string(),
  })
  .strict();
// @ts-expect-error drift: extra field in Zod
export const d5: SameShape<z.infer<typeof extraField>, GeneratedReport> = true;

// 6. A check state is retyped (free text instead of ok/down).
const looseChecks = z
  .object({ status: z.enum(serviceStatus), checks: z.record(z.string(), z.string()) })
  .strict();
export const loose = build(z.enum(overallStatus), looseChecks);
// @ts-expect-error drift: check state is a free string in Zod
export const d6: SameShape<z.infer<typeof loose>, GeneratedReport> = true;
