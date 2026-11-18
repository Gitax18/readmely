import { proxyToApi } from "@/lib/api-proxy";
import { loadApiPublicEnv } from "@/lib/env";

// Same-origin bridge to the API (see lib/api-proxy.ts). Only these methods exist; Next.js
// answers 405 for the rest.
export const dynamic = "force-dynamic";

type Context = { params: Promise<{ path: string[] }> };

async function handle(request: Request, context: Context): Promise<Response> {
  const { path } = await context.params;
  return proxyToApi(request, path, loadApiPublicEnv());
}

export const GET = handle;
export const POST = handle;
export const PATCH = handle;
export const DELETE = handle;
