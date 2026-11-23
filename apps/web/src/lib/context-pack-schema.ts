import { z } from "zod";

/**
 * The context pack response (contracts/core-public.yaml). The compile-time check in
 * contract-check.ts keeps these in step with the generated types.
 */
export const contextPackFileSchema = z.object({
  path: z.string().min(1).max(200),
  content: z.string().max(200_000),
});

export const contextPackPresetSchema = z.object({
  id: z.enum(["claude", "agents", "cursor", "generic"]),
  label: z.string().max(60),
  description: z.string().max(200),
  tokens: z.number().int().min(0),
  files: z.array(contextPackFileSchema).max(8),
});

export const contextPackSchema = z.object({
  commit_sha: z.string().regex(/^[0-9a-f]{40}$/),
  with_summary: z.boolean(),
  presets: z.array(contextPackPresetSchema).max(8),
});

export type ContextPackFile = z.infer<typeof contextPackFileSchema>;
export type ContextPackPreset = z.infer<typeof contextPackPresetSchema>;
export type ContextPack = z.infer<typeof contextPackSchema>;
