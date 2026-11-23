import type { CreditEntry } from "./credit-schema";
import type { StatusTone } from "./project-text";

/** What a ledger entry says in plain words. */
export function creditEntryLabel(e: CreditEntry): string {
  switch (e.kind) {
    case "grant":
      return e.note || "Credits added";
    case "reserve":
      return "Held for a job";
    case "settle":
      return "Returned after a job (the hold was more than it used)";
    case "refund":
      return "Refunded: the job did not finish";
    default:
      return e.note || "Adjustment";
  }
}

export function creditEntryTone(e: CreditEntry): StatusTone {
  return e.delta > 0 ? "ok" : "neutral";
}
