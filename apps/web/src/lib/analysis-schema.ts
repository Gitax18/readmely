import { z } from "zod";

/**
 * The analysis response (contracts/core-public.yaml). The compile-time check in contract-check.ts
 * keeps these in step with the generated types.
 */
const count = z.number().int().min(0);

export const projectFactsSchema = z.object({
  languages: z
    .array(
      z.object({
        name: z.string().max(40),
        files: count,
        lines: count,
        percent: z.number().min(0).max(100),
      }),
    )
    .max(8),
  frameworks: z.array(z.string().max(60)).max(24),
  package_managers: z.array(z.string().max(30)).max(12),
  entry_points: z.array(z.string().max(400)).max(10),
  commands: z.array(z.object({ name: z.string().max(200), source: z.string().max(400) })).max(20),
  env_vars: z.array(z.string().max(100)).max(60),
  services: z.array(z.string().max(100)).max(40),
  license: z.string().max(40).nullable(),
  has_readme: z.boolean(),
  has_tests: z.boolean(),
  has_ci: z.boolean(),
  has_docker: z.boolean(),
  totals: z.object({ files: count, source_files: count, lines: count, bytes: count }),
  directories: z.array(z.object({ path: z.string().max(400), files: count })).max(15),
  secrets: z.object({
    count,
    by_rule: z.record(z.string(), count),
    locations: z
      .array(z.object({ path: z.string().max(400), line: count, rule: z.string().max(40) }))
      .max(50),
  }),
  skipped: z.object({ count, by_reason: z.record(z.string(), count) }),
});

export const hubSchema = z.object({
  kind: z.enum(["file", "package", "external"]),
  key: z.string().max(400),
  imported_by: count,
});

export const projectAnalysisSchema = z.object({
  commit_sha: z.string().regex(/^[0-9a-f]{40}$/),
  analysed_at: z.string().min(1),
  facts: projectFactsSchema,
  hubs: z.array(hubSchema).max(10),
});

export type ProjectFacts = z.infer<typeof projectFactsSchema>;
export type Hub = z.infer<typeof hubSchema>;
export type ProjectAnalysis = z.infer<typeof projectAnalysisSchema>;
