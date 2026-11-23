import { z } from "zod";

/**
 * The account API's response shapes (contracts/core-public.yaml). The compile-time check in
 * contract-check.ts keeps these in step with the generated types.
 */
export const userSchema = z.object({
  id: z.uuid(),
  email: z.string().min(1),
  display_name: z.string(),
  created_at: z.string().min(1),
});

export const meSchema = z.object({
  user: userSchema,
  csrf_token: z.string().regex(/^[0-9a-f]{64}$/),
});

export const sessionSchema = z.object({
  id: z.uuid(),
  created_at: z.string().min(1),
  last_seen_at: z.string().min(1),
  device: z.string().max(100),
  current: z.boolean(),
});

export const sessionListSchema = z.object({
  sessions: z.array(sessionSchema).max(100),
});

export const providerSchema = z.enum(["google", "github"]);

export const providersSchema = z.object({
  providers: z.array(providerSchema).max(2),
});

export const identitySchema = z.object({
  provider: providerSchema,
  email: z.string(),
  created_at: z.string().min(1),
});

export const identityListSchema = z.object({
  identities: z.array(identitySchema).max(2),
});

export const authorizationUrlSchema = z.object({
  authorization_url: z.string().min(1).max(2048),
});

export type User = z.infer<typeof userSchema>;
export type Me = z.infer<typeof meSchema>;
export type Session = z.infer<typeof sessionSchema>;
export type Identity = z.infer<typeof identitySchema>;
export type AuthProviders = z.infer<typeof providersSchema>;
export type AuthorizationUrl = z.infer<typeof authorizationUrlSchema>;
