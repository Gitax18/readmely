import type { components } from "@/contract/core-internal";
import type { Report } from "./status-schema";

/**
 * Compile-time proof that the hand-written Zod schema accepts exactly the shape
 * the contract (contracts/core-internal.yaml) describes: every value either
 * type allows must be allowed by the other. If the spec or the schema changes
 * on its own, this stops type-checking and `make lint` fails.
 *
 * Types cannot express every rule (dependency-name pattern, entry limit); the
 * examples test in contract-examples.test.ts covers those at runtime.
 */
export type SameShape<A, B> = [A] extends [B] ? ([B] extends [A] ? true : false) : false;

export type GeneratedReport = components["schemas"]["StatusReport"];

export const reportMatchesContract: SameShape<Report, GeneratedReport> = true;
