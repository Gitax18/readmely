import { z } from "zod";

// The API's response is untrusted input: anything that does not match this
// exactly is rejected, including unknown keys and unexpected values.

const CHECK_NAME = /^[a-z][a-z0-9_]{0,31}$/;
const MAX_CHECKS = 8;

const checks = z
  .record(z.string().regex(CHECK_NAME), z.enum(["ok", "down"]))
  .refine((c) => Object.keys(c).length <= MAX_CHECKS);

const serviceStatus = z.enum(["ok", "down", "unreachable", "unconfigured"]);

const service = z.object({ status: serviceStatus, checks }).strict();

export const reportSchema = z
  .object({
    status: z.enum(["operational", "degraded", "unavailable"]),
    services: z.object({ api: service, ai: service }).strict(),
  })
  .strict();

export type Report = z.infer<typeof reportSchema>;
export type ServiceStatus = z.infer<typeof serviceStatus>;
export type Overall = Report["status"];
