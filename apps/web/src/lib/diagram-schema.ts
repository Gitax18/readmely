import { z } from "zod";
import { projectJobSchema } from "./project-schema";

/** The diagram API's response shapes (contracts/core-public.yaml), kept in step by contract-check.ts. */
export const diagramSchema = z.object({
  title: z.string().max(120),
  svg: z.string().max(200_000),
  node_count: z.number().int().min(1),
  created_at: z.string().min(1),
});

export const diagramStateSchema = z.object({
  diagram: diagramSchema.nullable(),
  job: projectJobSchema.nullable(),
});

export type Diagram = z.infer<typeof diagramSchema>;
export type DiagramState = z.infer<typeof diagramStateSchema>;
