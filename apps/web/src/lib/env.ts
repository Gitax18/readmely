import "server-only";

export type StatusDetail = "full" | "coarse";

export interface WebEnv {
  /** Base URL of the Go API's internal listener. Server-only, never sent to the browser. */
  apiInternalUrl: string;
  statusDetail: StatusDetail;
}

/**
 * Reads and validates the server environment. Every problem is reported at
 * once, by variable name only: values can hold addresses or secrets, so they
 * never appear in an error.
 */
export function loadEnv(source: Record<string, string | undefined> = process.env): WebEnv {
  const problems: string[] = [];

  let apiInternalUrl = (source.API_INTERNAL_URL ?? "").trim();
  if (apiInternalUrl === "") {
    problems.push("API_INTERNAL_URL: is required");
  } else {
    try {
      const u = new URL(apiInternalUrl);
      if (u.protocol !== "http:" && u.protocol !== "https:") {
        problems.push("API_INTERNAL_URL: scheme must be http or https");
      } else if (u.username !== "" || u.password !== "" || u.search !== "" || u.hash !== "") {
        problems.push("API_INTERNAL_URL: must not contain credentials, a query or a fragment");
      }
      apiInternalUrl = apiInternalUrl.replace(/\/+$/, "");
    } catch {
      problems.push("API_INTERNAL_URL: must be a valid URL");
    }
  }

  const raw = (source.STATUS_DETAIL ?? "").trim();
  // Production defaults to the coarse view: naming internal dependencies on a
  // public page helps attackers. Development defaults to the full view.
  let statusDetail: StatusDetail = source.NODE_ENV === "production" ? "coarse" : "full";
  if (raw !== "") {
    if (raw === "full" || raw === "coarse") {
      statusDetail = raw;
    } else {
      problems.push("STATUS_DETAIL: must be full or coarse");
    }
  }

  if (problems.length > 0) {
    throw new Error(`Invalid environment:\n${problems.join("\n")}`);
  }
  return { apiInternalUrl, statusDetail };
}
