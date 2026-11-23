import { failure, parsed, request, type Result } from "./browser-api";
import { projectAnalysisSchema, type ProjectAnalysis } from "./analysis-schema";
import {
  projectListSchema,
  projectSchema,
  repositoryPreviewSchema,
  type Project,
  type RepositoryPreview,
} from "./project-schema";

/** Calls the projects API from the browser, always through the same-origin /api bridge. */
export interface ProjectsApi {
  preview(csrf: string, url: string): Promise<Result<{ repository: RepositoryPreview }>>;
  create(csrf: string, url: string): Promise<Result<{ project: Project }>>;
  list(): Promise<Result<{ projects: Project[] }>>;
  get(id: string): Promise<Result<{ project: Project }>>;
  analyse(csrf: string, id: string): Promise<Result<{ project: Project }>>;
  analysis(id: string): Promise<Result<{ analysis: ProjectAnalysis }>>;
  remove(csrf: string, id: string): Promise<Result<object>>;
}

export function createProjectsApi(
  fetchImpl: typeof fetch = (...args) => fetch(...args),
): ProjectsApi {
  return {
    async preview(csrf, url) {
      const res = await request(fetchImpl, "POST", "/projects/preview", { body: { url }, csrf });
      if (res?.status !== 200) return failure(res);
      const repository = await parsed(res, repositoryPreviewSchema);
      return repository ? { ok: true, repository } : { ok: false, kind: "unavailable" };
    },
    async create(csrf, url) {
      const res = await request(fetchImpl, "POST", "/projects", { body: { url }, csrf });
      if (res?.status !== 201) return failure(res);
      const project = await parsed(res, projectSchema);
      return project ? { ok: true, project } : { ok: false, kind: "unavailable" };
    },
    async list() {
      const res = await request(fetchImpl, "GET", "/projects");
      if (res?.status !== 200) return failure(res);
      const list = await parsed(res, projectListSchema);
      return list ? { ok: true, projects: list.projects } : { ok: false, kind: "unavailable" };
    },
    async get(id) {
      const res = await request(fetchImpl, "GET", `/projects/${encodeURIComponent(id)}`);
      if (res?.status !== 200) return failure(res);
      const project = await parsed(res, projectSchema);
      return project ? { ok: true, project } : { ok: false, kind: "unavailable" };
    },
    async analyse(csrf, id) {
      const res = await request(fetchImpl, "POST", `/projects/${encodeURIComponent(id)}/analyse`, {
        csrf,
      });
      if (res?.status !== 202) return failure(res);
      const project = await parsed(res, projectSchema);
      return project ? { ok: true, project } : { ok: false, kind: "unavailable" };
    },
    async analysis(id) {
      const res = await request(fetchImpl, "GET", `/projects/${encodeURIComponent(id)}/analysis`);
      if (res?.status !== 200) return failure(res);
      const analysis = await parsed(res, projectAnalysisSchema);
      return analysis ? { ok: true, analysis } : { ok: false, kind: "unavailable" };
    },
    async remove(csrf, id) {
      const res = await request(fetchImpl, "DELETE", `/projects/${encodeURIComponent(id)}`, {
        csrf,
      });
      return res?.status === 204 ? { ok: true } : failure(res);
    },
  };
}
