/** Level-2 headings of a markdown document, ignoring any inside fenced code. Mirrors the API's parser. */
export function sectionHeadings(content: string): string[] {
  const out: string[] = [];
  let fenced = false;
  for (const line of content.split("\n")) {
    const t = line.trim();
    if (t.startsWith("```") || t.startsWith("~~~")) {
      fenced = !fenced;
      continue;
    }
    if (!fenced && line.startsWith("## ")) out.push(line.slice(3).trim());
  }
  return out;
}

export const TEMPLATE_LABELS = {
  standard: "Standard",
  minimal: "Minimal",
  detailed: "Detailed",
} as const;

export const TEMPLATE_HINTS = {
  standard:
    "Badges and a quote, then overview, features, tech stack, setup, usage, configuration, file tree, contributing, licence.",
  minimal: "Badges and a quote, then overview, features, setup, usage, licence.",
  detailed: "Standard plus an architecture diagram, testing and deployment.",
} as const;

export const SOURCE_LABELS = {
  generated: "Written by the model",
  edited: "Edited",
  regenerated: "Section rewritten",
  restored: "Restored",
} as const;

/** Credits held, at most charged (see services/core/internal/readme and services/ai/app/meter.py). */
export const GENERATE_COST = 27;
export const SECTION_COST = 4;
