/** An SVG as a data: address an img element can show. An img never runs scripts, whatever the SVG holds. */
export function svgDataUrl(svg: string): string {
  const bytes = new TextEncoder().encode(svg);
  let binary = "";
  for (const b of bytes) binary += String.fromCharCode(b);
  return `data:image/svg+xml;base64,${btoa(binary)}`;
}

/** The drawing's own size, read from the viewBox our renderer writes; null when it is not there. */
export function svgSize(svg: string): { width: number; height: number } | null {
  const m = /viewBox="0 0 (\d{1,5}) (\d{1,5})"/.exec(svg);
  return m ? { width: Number(m[1]), height: Number(m[2]) } : null;
}

/** Credits held for a diagram, at most charged (see services/core/internal/diagram). */
export const DIAGRAM_COST = 38;

/** The two diagrams a project can have, one of each. */
export type DiagramKind = "architecture" | "database";

export const DIAGRAM_KINDS: readonly DiagramKind[] = ["architecture", "database"];

/** Everything on the diagram card that differs between the two kinds. */
export const KIND_TEXT: Record<
  DiagramKind,
  {
    tab: string;
    region: string;
    file: string;
    unit: string;
    title: string;
    again: string;
    about: string;
    hint: string;
    field: string;
    button: string;
  }
> = {
  architecture: {
    tab: "Architecture",
    region: "Architecture diagram",
    file: "architecture",
    unit: "parts",
    title: "Draw your architecture diagram",
    again: "Draw a new diagram",
    about: "A picture of the main parts and how they talk to each other, drawn from the analysis.",
    hint: 'Optional, for example "the background worker".',
    field: "Anything it should emphasise?",
    button: "Draw my diagram",
  },
  database: {
    tab: "Database (ER)",
    region: "Database diagram",
    file: "database",
    unit: "tables",
    title: "Draw your database diagram",
    again: "Draw a new database diagram",
    about:
      "An entity relationship diagram: the tables and how they point at each other, read from the SQL, migration and schema files in the repository.",
    hint: 'Optional, for example "users and billing".',
    field: "Which tables matter most?",
    button: "Draw my database",
  },
};
export const ZOOM_STEPS = [0.5, 0.75, 1, 1.5, 2] as const;
