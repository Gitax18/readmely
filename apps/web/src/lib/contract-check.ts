import type { components } from "@/contract/core-internal";
import type { components as publicComponents } from "@/contract/core-public";
import type {
  AuthorizationUrl,
  AuthProviders,
  Identity,
  Me,
  Session,
  User,
} from "./account-schema";
import type { ProjectAnalysis, ProjectFacts, ProjectSummary } from "./analysis-schema";
import type { ContextPack, ContextPackPreset } from "./context-pack-schema";
import type { CreditBalance, CreditEntry } from "./credit-schema";
import type { Project, ProjectJob, RepositoryPreview } from "./project-schema";
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

// The account endpoints (contracts/core-public.yaml): the Zod schemas the browser and the
// server pages use must accept exactly what the contract describes.

export const userMatchesContract: SameShape<User, publicComponents["schemas"]["User"]> = true;
export const meMatchesContract: SameShape<Me, publicComponents["schemas"]["Me"]> = true;
export const sessionMatchesContract: SameShape<Session, publicComponents["schemas"]["Session"]> =
  true;

export const identityMatchesContract: SameShape<Identity, publicComponents["schemas"]["Identity"]> =
  true;
export const providersMatchContract: SameShape<
  AuthProviders,
  publicComponents["schemas"]["AuthProviders"]
> = true;
export const authorizationUrlMatchesContract: SameShape<
  AuthorizationUrl,
  publicComponents["schemas"]["AuthorizationUrl"]
> = true;

// The project endpoints.
export const projectJobMatchesContract: SameShape<
  ProjectJob,
  publicComponents["schemas"]["ProjectJob"]
> = true;
export const projectMatchesContract: SameShape<Project, publicComponents["schemas"]["Project"]> =
  true;
export const repositoryPreviewMatchesContract: SameShape<
  RepositoryPreview,
  publicComponents["schemas"]["RepositoryPreview"]
> = true;
export const projectFactsMatchContract: SameShape<
  ProjectFacts,
  publicComponents["schemas"]["ProjectFacts"]
> = true;
export const projectAnalysisMatchesContract: SameShape<
  ProjectAnalysis,
  publicComponents["schemas"]["ProjectAnalysis"]
> = true;

// The credits endpoints.
export const creditBalanceMatchesContract: SameShape<
  CreditBalance,
  publicComponents["schemas"]["CreditBalance"]
> = true;
export const creditEntryMatchesContract: SameShape<
  CreditEntry,
  publicComponents["schemas"]["CreditEntry"]
> = true;
export const projectSummaryMatchesContract: SameShape<
  ProjectSummary,
  publicComponents["schemas"]["ProjectSummary"]
> = true;
export const contextPackMatchesContract: SameShape<
  ContextPack,
  publicComponents["schemas"]["ContextPack"]
> = true;
export const contextPackPresetMatchesContract: SameShape<
  ContextPackPreset,
  publicComponents["schemas"]["ContextPackPreset"]
> = true;
