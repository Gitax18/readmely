import "server-only";

export type StatusDetail = "full" | "coarse";

export interface WebEnv {
  /** Base URL of the Go API's internal listener. Server-only, never sent to the browser. */
  apiInternalUrl: string;
  statusDetail: StatusDetail;
}

/** Checks a base URL setting; returns the URL without trailing slashes, or null after recording why not. */
function checkBaseUrl(name: string, raw: string, problems: string[]): string | null {
  if (raw === "") {
    problems.push(`${name}: is required`);
    return null;
  }
  try {
    const u = new URL(raw);
    if (u.protocol !== "http:" && u.protocol !== "https:") {
      problems.push(`${name}: scheme must be http or https`);
      return null;
    }
    if (u.username !== "" || u.password !== "" || u.search !== "" || u.hash !== "") {
      problems.push(`${name}: must not contain credentials, a query or a fragment`);
      return null;
    }
    return raw.replace(/\/+$/, "");
  } catch {
    problems.push(`${name}: must be a valid URL`);
    return null;
  }
}

/**
 * Reads and validates the server environment. Every problem is reported at
 * once, by variable name only: values can hold addresses or secrets, so they
 * never appear in an error.
 */
export function loadEnv(source: Record<string, string | undefined> = process.env): WebEnv {
  const problems: string[] = [];

  const apiInternalUrl = checkBaseUrl(
    "API_INTERNAL_URL",
    (source.API_INTERNAL_URL ?? "").trim(),
    problems,
  );

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
  return { apiInternalUrl: apiInternalUrl ?? "", statusDetail };
}

export interface ApiPublicEnv {
  /** Base URL of the Go API's PUBLIC listener. Never the internal one (audit F14). Server-only. */
  apiPublicUrl: string;
  /**
   * True only when a trusted proxy or load balancer in front of this server sets
   * X-Forwarded-For. Then the header is passed on so the API can rate limit each visitor
   * separately; otherwise it is dropped, because anyone could forge it.
   */
  behindProxy: boolean;
}

/** Reads the settings the account pages and the /api proxy need. Names only in errors. */
export function loadApiPublicEnv(
  source: Record<string, string | undefined> = process.env,
): ApiPublicEnv {
  const problems: string[] = [];
  const apiPublicUrl = checkBaseUrl(
    "API_PUBLIC_URL",
    (source.API_PUBLIC_URL ?? "").trim(),
    problems,
  );

  const raw = (source.WEB_BEHIND_PROXY ?? "").trim();
  if (raw !== "" && raw !== "true" && raw !== "false") {
    problems.push("WEB_BEHIND_PROXY: must be true or false");
  }

  if (problems.length > 0) {
    throw new Error(`Invalid environment:\n${problems.join("\n")}`);
  }
  return { apiPublicUrl: apiPublicUrl ?? "", behindProxy: raw === "true" };
}
