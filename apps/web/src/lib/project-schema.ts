import { z } from "zod";

/**
 * The projects API's response shapes (contracts/core-public.yaml). The compile-time check in
 * contract-check.ts keeps these in step with the generated types.
 */
export const projectJobSchema = z.object({
  id: z.uuid(),
  status: z.enum(["queued", "running", "succeeded", "failed", "cancelled"]),
  step: z.string().max(40).nullable(),
  progress: z.number().int().min(0).max(100),
  error_code: z.string().max(64).nullable(),
  created_at: z.string().min(1),
  finished_at: z.string().min(1).nullable(),
});

export const projectSchema = z.object({
  id: z.uuid(),
  name: z.string().min(1).max(100),
  owner: z.string().max(39),
  repo: z.string().max(100),
  description: z.string().max(500),
  default_branch: z.string().max(255),
  size_kb: z.number().int().min(0),
  status: z.enum(["draft", "importing", "ready", "failed"]),
  created_at: z.string().min(1),
  updated_at: z.string().min(1),
  job: projectJobSchema.nullable(),
});

export const projectListSchema = z.object({
  projects: z.array(projectSchema).max(50),
});

export const repositoryPreviewSchema = z.object({
  owner: z.string().min(1).max(39),
  name: z.string().min(1).max(100),
  description: z.string().max(500),
  default_branch: z.string().max(255),
  size_kb: z.number().int().min(0),
});

export type ProjectJob = z.infer<typeof projectJobSchema>;
export type Project = z.infer<typeof projectSchema>;
export type RepositoryPreview = z.infer<typeof repositoryPreviewSchema>;

/** A project whose analysis is waiting or running: the pages poll until it settles. */
export function isWorking(p: Project): boolean {
  return (
    p.status === "importing" && (p.job === null || ["queued", "running"].includes(p.job.status))
  );
}
