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

export const DIAGRAM_COST = 15;
export const ZOOM_STEPS = [0.5, 0.75, 1, 1.5, 2] as const;
