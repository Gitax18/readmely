import { z } from "zod";
import { projectJobSchema } from "./project-schema";

/**
 * The README API's response shapes (contracts/core-public.yaml). The compile-time check in
 * contract-check.ts keeps these in step with the generated types.
 */
const source = z.enum(["generated", "edited", "regenerated", "restored"]);
const template = z.enum(["standard", "minimal", "detailed"]);

export const readmeSchema = z.object({
  version: z.number().int().min(1),
  content: z.string().max(102_400),
  source,
  template: template.nullable(),
  created_at: z.string().min(1),
});

export const readmeScoreSchema = z.object({
  score: z.number().int().min(0).max(100),
  items: z
    .array(
      z.object({
        key: z.string().max(40),
        label: z.string().max(80),
        ok: z.boolean(),
        hint: z.string().max(200),
      }),
    )
    .max(20),
});

export const readmeStateSchema = z.object({
  readme: readmeSchema.nullable(),
  score: readmeScoreSchema.nullable(),
  job: projectJobSchema.nullable(),
});

export const readmeVersionsSchema = z.object({
  versions: z
    .array(
      z.object({
        version: z.number().int().min(1),
        source,
        template: template.nullable(),
        created_at: z.string().min(1),
      }),
    )
    .max(50),
});

export type ReadmeTemplate = z.infer<typeof template>;
export type Readme = z.infer<typeof readmeSchema>;
export type ReadmeScore = z.infer<typeof readmeScoreSchema>;
export type ReadmeState = z.infer<typeof readmeStateSchema>;
export type ReadmeVersions = z.infer<typeof readmeVersionsSchema>;
