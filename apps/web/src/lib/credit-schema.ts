import { z } from "zod";

/**
 * The credits API's response shapes (contracts/core-public.yaml). The compile-time check in
 * contract-check.ts keeps these in step with the generated types.
 */
export const creditBalanceSchema = z.object({ balance: z.number().int() });

export const creditEntrySchema = z.object({
  id: z.uuid(),
  delta: z.number().int(),
  kind: z.enum(["grant", "reserve", "settle", "refund", "adjustment"]),
  note: z.string().max(500),
  created_at: z.string().min(1),
});

export const creditHistorySchema = z.object({ entries: z.array(creditEntrySchema).max(50) });

export type CreditBalance = z.infer<typeof creditBalanceSchema>;
export type CreditEntry = z.infer<typeof creditEntrySchema>;
export type CreditHistory = z.infer<typeof creditHistorySchema>;
