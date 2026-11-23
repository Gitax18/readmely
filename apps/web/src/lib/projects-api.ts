import { failure, parsed, request, type Result } from "./browser-api";
import {
  readmeSchema,
  readmeStateSchema,
  readmeVersionsSchema,
  type Readme,
  type ReadmeState,
  type ReadmeTemplate,
  type ReadmeVersions,
} from "./readme-schema";
import { contextPackSchema, type ContextPack } from "./context-pack-schema";
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
  contextPack(id: string): Promise<Result<{ pack: ContextPack }>>;
  readme(id: string): Promise<Result<{ state: ReadmeState }>>;
  generateReadme(csrf: string, id: string, template: ReadmeTemplate): Promise<Result<object>>;
  saveReadme(
    csrf: string,
    id: string,
    content: string,
    base: number,
  ): Promise<Result<{ readme: Readme }>>;
  rewriteSection(
    csrf: string,
    id: string,
    heading: string,
    instruction: string,
  ): Promise<Result<object>>;
  readmeVersions(id: string): Promise<Result<{ versions: ReadmeVersions["versions"] }>>;
  restoreReadme(csrf: string, id: string, version: number): Promise<Result<{ readme: Readme }>>;
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
    async contextPack(id) {
      const res = await request(
        fetchImpl,
        "GET",
        `/projects/${encodeURIComponent(id)}/context-pack`,
      );
      if (res?.status !== 200) return failure(res);
      const pack = await parsed(res, contextPackSchema);
      return pack ? { ok: true, pack } : { ok: false, kind: "unavailable" };
    },
    async readme(id) {
      const res = await request(fetchImpl, "GET", `/projects/${encodeURIComponent(id)}/readme`);
      if (res?.status !== 200) return failure(res);
      const state = await parsed(res, readmeStateSchema);
      return state ? { ok: true, state } : { ok: false, kind: "unavailable" };
    },
    async generateReadme(csrf, id, template) {
      const res = await request(fetchImpl, "POST", `/projects/${encodeURIComponent(id)}/readme`, {
        body: { template },
        csrf,
      });
      return res?.status === 202 ? { ok: true } : failure(res);
    },
    async saveReadme(csrf, id, content, base) {
      const res = await request(fetchImpl, "PUT", `/projects/${encodeURIComponent(id)}/readme`, {
        body: { content, base_version: base },
        csrf,
      });
      if (res?.status !== 200) return failure(res);
      const readme = await parsed(res, readmeSchema);
      return readme ? { ok: true, readme } : { ok: false, kind: "unavailable" };
    },
    async rewriteSection(csrf, id, heading, instruction) {
      const res = await request(
        fetchImpl,
        "POST",
        `/projects/${encodeURIComponent(id)}/readme/sections`,
        { body: { heading, instruction }, csrf },
      );
      return res?.status === 202 ? { ok: true } : failure(res);
    },
    async readmeVersions(id) {
      const res = await request(
        fetchImpl,
        "GET",
        `/projects/${encodeURIComponent(id)}/readme/versions`,
      );
      if (res?.status !== 200) return failure(res);
      const list = await parsed(res, readmeVersionsSchema);
      return list ? { ok: true, versions: list.versions } : { ok: false, kind: "unavailable" };
    },
    async restoreReadme(csrf, id, version) {
      const res = await request(
        fetchImpl,
        "POST",
        `/projects/${encodeURIComponent(id)}/readme/versions/${version}/restore`,
        { csrf },
      );
      if (res?.status !== 200) return failure(res);
      const readme = await parsed(res, readmeSchema);
      return readme ? { ok: true, readme } : { ok: false, kind: "unavailable" };
    },
    async remove(csrf, id) {
      const res = await request(fetchImpl, "DELETE", `/projects/${encodeURIComponent(id)}`, {
        csrf,
      });
      return res?.status === 204 ? { ok: true } : failure(res);
    },
  };
}
