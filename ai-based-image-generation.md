# AI architecture diagram generator: build guide

Created by Gitanshu Sankhla · @Gitanshu Sankhla · Nov 15, 2026

> **Architecture note (Readmely):** this guide describes the pipeline in Go for illustration. In Readmely, all LLM orchestration (prompting, structured output, the repair loop) runs in the **Python AI service** (decision D1 in `plans/00-architecture.md`). The Go core provides the deterministic parts: graph extraction, layout, SVG rendering and validation. The Go snippets and tech-stack rows below will be rewritten when the diagram phase starts.

## Overview

The tool reads a codebase, works out its architecture, and outputs a clean, coloured SVG diagram. The core rule: **the LLM decides structure and meaning; deterministic code decides geometry.** Claude's diagrams look good because a design system, layout discipline and a render-and-check loop sit around the model, not because of a special drawing ability. This guide rebuilds that scaffolding for your own model.

What it should produce:

- An overview diagram of the whole system (5 to 9 nodes), plus one detail diagram per interesting sub-flow
- Consistent colours that mean something (a category per colour), two font sizes, thin strokes, no overlaps
- SVG output (vector, editable, themable), with PNG and D2/Mermaid exports as extras

Why not ask the LLM for Mermaid? Mermaid fixes the look and gives you little control over layout. Why not ask the LLM to write raw SVG in one shot? Models are unreliable at coordinate arithmetic on graphs bigger than about 6 nodes: boxes overlap, text overflows, arrows cut through boxes. The middle path is in this guide: the LLM emits a structured graph, a layout engine places it, a template renderer draws it, and a validator plus a vision model check it.

## Pipeline at a glance

&#91;embedded content: diagram pipeline · 7 stages, 1 retry loop\]

Only two stages need LLM judgement; the other five are ordinary code you can unit-test. When validation fails, the fix goes back to the stage that caused it, not into the SVG.

## Stage 1: Ingest the project context

Goal: turn a repo into a compact, high-signal context pack (aim for under 30k tokens) without sending the whole codebase to the model. Hard facts come from parsers; the LLM only fills the gaps.

1. **Walk the repo.** Respect `.gitignore` using `go-git` or `github.com/sabhiram/go-gitignore`. Skip `vendor/`, `node_modules/`, build output, lockfiles and binaries. Clone remote repos with `go-git`.
2. **Rank files by architectural signal.** Highest: infra and contracts. Then entrypoints (`cmd/*/main.go`, `main.py`, `index.ts`), then router and handler files, then everything else.
3. **Parse deterministic sources** (these give exact edges and need no LLM):
   - `docker-compose.yml`: services, `depends_on`, ports, networks (`gopkg.in/yaml.v3`)
   - Kubernetes manifests and Helm charts: Deployments, Services, Ingress (`k8s.io/apimachinery` or plain YAML)
   - Terraform: resources and references (`hashicorp/hcl/v2`, or `terraform graph` output)
   - `.proto` files: gRPC services and RPCs (`github.com/bufbuild/protocompile`)
   - OpenAPI/Swagger specs: endpoints (`github.com/getkin/kin-openapi`)
   - `go.mod`, `package.json`, `requirements.txt`: which client libraries are used (pgx means Postgres, go-redis means Redis, sarama or franz-go means Kafka)
   - `.env.example` and config files: hostnames and URLs of external dependencies
4. **Parse source code into a symbol skeleton** using **tree-sitter** (`github.com/smacker/go-tree-sitter`, which covers 40+ languages) or `go/ast` + `golang.org/x/tools/go/packages` for Go. Extract packages, exported types, functions, HTTP route registrations, DB queries, outbound HTTP/gRPC calls, and queue publish/subscribe calls. Keep signatures and drop function bodies.
5. **Build an import graph** between internal packages. This becomes the module-level dependency view.
6. **Summarise per directory with a cheap model.** Send each package's skeleton (not raw code) and ask for 2 to 3 lines: responsibility, inputs, outputs, external calls. Cache by content hash so re-runs are cheap.
7. **Add the README and docs**: README, `ARCHITECTURE.md`, ADRs. These carry the developer's own names for things, and the diagram should use those names.

Output of this stage: `context_pack.json` containing parsed facts (exact), directory summaries (LLM), and doc excerpts.

## Stage 2: Extract an architecture IR

The model never jumps straight from code to pixels. It first writes a typed JSON graph, the intermediate representation (IR). Every later stage reads only this graph, which makes the pipeline testable, cacheable and model-agnostic.

```json
{
  "title": "Orders are processed asynchronously via Kafka",
  "nodes": [
    {"id": "api", "label": "API gateway", "sublabel": "Go, chi router",
     "kind": "service", "group": "backend", "evidence": ["cmd/api/main.go"]},
    {"id": "pg", "label": "Orders DB", "sublabel": "PostgreSQL 16",
     "kind": "datastore", "group": "data", "evidence": ["docker-compose.yml"]}
  ],
  "edges": [
    {"from": "api", "to": "pg", "label": "SQL", "kind": "sync",
     "evidence": ["internal/store/orders.go:42"]}
  ],
  "groups": [{"id": "backend", "label": "Backend services"}]
}
```

- **Closed vocabularies.** `kind` is one of: `client`, `service`, `worker`, `datastore`, `cache`, `queue`, `external`, `gateway`. Edge `kind` is one of: `sync`, `async`, `data`. Each kind maps to a fixed style in Stage 5, so the model never chooses colours.
- **Evidence required.** Every node and edge cites a file or line. Drop anything without evidence; this is your main defence against hallucinated components.
- **Structured output.** Enforce the schema with your provider's JSON-schema / structured-output mode, or with function calling. In Go, validate with `github.com/santhosh-tekuri/jsonschema`.
- **Merge, don't trust.** Deterministic facts from Stage 1 (compose services, proto RPCs, imports) are inserted first and locked. The LLM may add nodes, rename for clarity, group, and add edges it can justify, but it may not delete parsed facts.
- **Label rules in the prompt.** Labels at most 3 words, sublabels at most 5 words, sentence case, using the project's own names.

## Stage 3: Plan the diagram

This is where Claude-style diagrams separate from cluttered ones: decide **what to leave out** before drawing anything. The planner is a second LLM call that reads the IR and the user's request and returns a `ViewPlan`.

1. **Pick the diagram type from the request's verb, not its noun.**
   - "what's the architecture", "how is it organised" → structural (containers with services inside)
   - "what happens when a user places an order" → flowchart or sequence (left to right, one path)
   - "show the database schema" → ERD (hand this to Mermaid `erDiagram` or D2 `sql_table`; hand-placed tables fail)
   - "how does X work" for a concept → illustrative (out of scope for v1)
2. **Enforce a complexity budget.** Overview: at most 9 nodes, at most 4 in a row, at most 3 groups. Detail views: 3 to 5 nodes each. Count nodes before drawing.
3. **Decompose when over budget.** A 25-service repo becomes one overview (collapse each group to a single node) plus one detail view per group or per key flow. Use the C4 model levels as a guide: context → containers → components.
4. **Write a title that states a finding, not a topic.** "Orders are processed asynchronously via Kafka" beats "System architecture".
5. **Assign colour by category, at most 3 hues per diagram.** Gray for neutral or external, one hue for services, one for data stores. Colour never encodes order (no rainbows).
6. **Choose direction.** Request flows go left to right; layered systems (client → API → services → data) go top to bottom.

The output `ViewPlan` lists node ids to include, groups to collapse, direction, title, and the colour role per kind. It is still pure data, with no coordinates.

## Stage 4: Layout

Use a real layout engine for coordinates and edge routing. Let the LLM place things by hand only for small diagrams (5 nodes or fewer) or illustrative ones, and even then validate the result.

| Engine | Language | Strengths | Use it for |
| --- | --- | --- | --- |
| [ELK.js](https://github.com/kieler/elkjs) (`layered` algorithm) | JS (runs in Node or Bun) | Nested groups, orthogonal edge routing, port constraints | **Default choice** for architecture diagrams with containers |
| [dagre](https://github.com/dagrejs/dagre) | JS | Simple and fast, layered DAGs | Flat flowcharts without nesting |
| [Graphviz](https://graphviz.org) `dot` | C (call the CLI, or `goccy/go-graphviz`) | Battle-tested, handles 100+ nodes | Big dependency graphs, import graphs |
| [D2](https://d2lang.com) (`oss.terrastruct.com/d2`) | Go library | Native Go, ELK and dagre built in, themes, SVG output | A quick end-to-end v1 without writing a renderer |

**Recommended setup for a Go backend:** keep the pipeline in Go and run ELK.js in a small Node sidecar, or embed it with `github.com/dop251/goja` / `rogchap/v8go`. Send `{nodes with width/height, edges, nesting}` and get back `x, y` for every node and bend points for every edge.

**Measure text before layout.** The engine needs real box sizes. Box width = longest line width + 2 × 24px padding. Measure with your actual font using `golang.org/x/image/font` + `opentype` (load the TTF and sum glyph advances). A rough fallback: about 8px per character at 14px medium weight, 7px at 12px regular. Two-line boxes are 56px tall, one-line boxes 44px.

**Engine settings that match the look:** node spacing 40 to 60px, layer spacing 60 to 80px, orthogonal edge routing, group padding 20px plus 32px on top for the group label.

## Stage 5: Render SVG with a design system

The polished look comes from a small, strict design system applied by code. Write the renderer with Go's `text/template` (or `github.com/a-h/templ`) and give it components, not freedom.

**Design tokens (write them once and reuse everywhere):**

| Token | Value | Rule |
| --- | --- | --- |
| Canvas width | 680 to 760 px viewBox, height computed | Height = lowest element + 40px; nothing outside the viewBox |
| Fonts | Inter or IBM Plex Sans | Two sizes only: 14px titles (weight 500), 12px sublabels (400) |
| Strokes | 0.5 to 1px borders, 1.25px connectors | Never 2px+ except one accent box |
| Corners | `rx=8` on boxes, `rx=16` on groups | Pills only on purpose |
| Palette | 9 ramps × 7 stops (50 → 900), e.g. teal, purple, coral, gray | Fill = 50 stop, stroke = 600, title = 800, sublabel = 600 |
| Colour count | At most 3 hues per diagram | Gray = neutral and external |
| Text case | Sentence case | No Title Case, no ALL CAPS |
| Effects | None | No gradients, shadows or glow |

**Map IR kinds to styles:** `service` → purple, `datastore`/`cache` → teal, `queue` → coral, `external`/`client` → gray. Async edges are dashed, sync edges solid.

**Components to implement:**

- `Node(x, y, w, h, title, sublabel, ramp)`: rect + centred text with `dominant-baseline="central"`
- `Group(x, y, w, h, label, ramp)`: a larger rounded rect, label top-left, lighter fill
- `Edge(points[], kind, label?)`: a polyline from the layout engine's bend points, `fill="none"`, one shared arrow marker that uses `stroke="context-stroke"` so the arrowhead matches the line colour
- `Legend(items)`: only when colour carries meaning the labels don't already show
- `Title(text)`: the finding sentence at the top-left

**Dark mode:** emit colours as CSS custom properties inside a `<style>` block with a `@media (prefers-color-scheme: dark)` override. Dark mode uses the 800 stop for fill, 200 for stroke, 100 for title text.

**Accessibility:** `role="img"`, plus `<title>` and `<desc>` as the first children of the `<svg>`. Generate the desc from the IR.

For reference, a single rendered node looks like this:

```xml
<g class="node service">
  <rect x="200" y="40" width="200" height="56" rx="8"
        fill="#EEEDFE" stroke="#534AB7" stroke-width="0.5"/>
  <text x="300" y="58" text-anchor="middle" dominant-baseline="central"
        font-size="14" font-weight="500" fill="#3C3489">Order service</text>
  <text x="300" y="78" text-anchor="middle" dominant-baseline="central"
        font-size="12" fill="#534AB7">Go, gRPC</text>
</g>
```

## Stage 6: Validate and self-correct

A model can't see the SVG it writes, so the pipeline has to look for it. Use two layers of checks: cheap geometric checks on every render, then a vision-model critique on the rasterised image.

**Layer 1: geometric lint (deterministic, in Go, a few milliseconds):**

- [ ] No two node rects intersect; boxes in the same row are at least 20px apart
- [ ] Every text fits inside its box: measured width + 2 × padding ≤ box width
- [ ] No edge segment passes through the interior of a node other than its source and target (segment-vs-rect intersection test)
- [ ] No label overlaps an edge or another label
- [ ] Everything sits inside the viewBox, with no negative coordinates
- [ ] Every IR node in the `ViewPlan` was drawn and no extra nodes appear
- [ ] Colour count ≤ 3 hues; font sizes ∈ {12, 14}
- [ ] The SVG parses (`encoding/xml`) and every connector `<path>` has `fill="none"`

**Layer 2: visual critique:**

1. Rasterise the SVG to PNG with [resvg](https://github.com/linebender/resvg) (fast CLI, faithful text rendering). A headless browser via `chromedp` or Playwright is the fallback when you need exact web-font rendering.
2. Send the PNG plus the `ViewPlan` to a vision-capable model with a rubric: overlaps, unreadable text, arrows that cut through boxes, misleading colours, missing components, and whether the title matches what's drawn. Ask for JSON: `{"pass": bool, "issues": [{"node": "...", "problem": "...", "fix": "..."}]}`.
3. Apply fixes at the right layer. Layout issues go back to Stage 4 with adjusted spacing or a different direction. Content issues go to Stage 3 (split the view, shorten labels). Never patch raw SVG by hand.

**Retry policy:** at most 3 loops. If it still fails, decompose the view into smaller diagrams. Log every failure with its IR, which becomes your eval set.

**Eval set:** keep 20 to 30 real repos (your Conveyor project, popular Go services, a microservices demo) with golden IRs. Score node recall, edge precision and lint pass rate on every prompt change.

## Stage 7: Export and deliver

Ship the SVG as the main artefact, and generate the other formats from the same IR so they never drift apart.

- **Optimised SVG:** run [SVGO](https://github.com/svg/svgo) to cut 30 to 60% of the size. Embed the font with `@font-face` (base64 WOFF2 subset) or convert text to paths for portability.
- **PNG at 2×:** `resvg --zoom 2` for READMEs, slides and chat.
- **Editable formats:** emit D2 or Mermaid source from the IR so users can tweak the diagram in tools they already know. Excalidraw JSON or draw.io XML are good stretch goals.
- **Interactive HTML:** wrap the SVG in a page that adds hover highlights and click-to-show evidence (the file and line behind each node), which works well for code review.
- **Caching:** key every stage on a content hash (repo commit + stage inputs). Only changed packages are re-summarised and only affected views re-render, which matters for a CI job that regenerates diagrams on every merge.
- **Delivery surfaces:** CLI (`archdiag ./repo -o out/`), a GitHub Action that commits `docs/architecture.svg`, an HTTP API, and an MCP server so other AI agents can call it as a tool.

## System prompt for the model

Use this for the agent that orchestrates Stages 2, 3 and 6. Keep the design tokens in code; the prompt only needs to teach judgement.

```text
You are an architecture-diagram planner. You turn a project's context pack
into clear, minimal, accurate diagrams. You never write coordinates or SVG;
you produce structured data that a layout engine and renderer draw.

PRINCIPLES
1. Accuracy over completeness. Every node and edge must cite evidence
   (file or file:line) from the context pack. No evidence, no node.
2. Intent decides diagram type. Architecture/organisation -> structural.
   "What happens when..." -> flow, left to right. Schema -> ERD.
3. Budget. Overview <= 9 nodes, <= 4 per row, <= 3 groups. If the system
   is bigger, collapse groups in the overview and plan detail views.
4. Names. Use the project's own names. Labels <= 3 words, sublabels
   <= 5 words, sentence case. No jargon the repo doesn't use.
5. Title states a finding ("Orders are processed async via Kafka"),
   not a topic ("Architecture").
6. Colour is assigned by node kind, never by you. Pick the correct kind
   from: client, gateway, service, worker, datastore, cache, queue, external.
7. Edges: sync (request/response), async (queue/event), data (read/write).
   Label an edge only if source + target don't already make it obvious.

PROCESS
1. Call get_context_pack. Read parsed facts first; they are ground truth.
2. Call submit_ir with the full architecture graph.
3. Call plan_views with an overview and up to 4 detail views.
4. For each view call render_view. Read the lint report and critique.
5. If issues exist, fix at the right level (shorten labels, split the
   view, change direction, regroup) and re-render. Max 3 attempts.
6. Finish with a 2-sentence summary per view and any components you
   suspected but could not evidence.

NEVER
- Invent services, databases or queues not supported by evidence.
- Put more than 9 nodes in one view.
- Use colour to show order or sequence.
- Draw an edge between every pair of related nodes; show the main flow.
```

## Tool-calling interface

The agent gets five tools. Each one wraps a deterministic stage, so the model reasons while code does the heavy lifting. Expose them via your provider's function calling, or as an MCP server so any agent can use them.

| Tool | Input | Returns | Backed by |
| --- | --- | --- | --- |
| `get_context_pack` | `repo_path`, `focus?` (a directory or flow name) | Parsed facts, directory summaries, doc excerpts | Stage 1 |
| `read_file` | `path`, `start_line?`, `end_line?` | File contents, to confirm evidence | Filesystem, sandboxed to the repo |
| `submit_ir` | Full IR JSON (schema in Stage 2) | Validation errors, or `ok` + merged IR with locked facts | JSON schema + merge logic |
| `plan_views` | `[{view_id, type, node_ids, collapse_groups, direction, title}]` | Budget violations, or `ok` | Stage 3 rules in code |
| `render_view` | `view_id` | SVG path, PNG path, lint report, vision critique JSON | Stages 4 to 6 |

Example `render_view` response the model reacts to:

```json
{
  "view_id": "overview",
  "svg": "out/overview.svg",
  "lint": {"pass": false, "errors": [
    {"rule": "text_overflow", "node": "notif", "detail": "'Notification dispatcher' 186px > box 160px"}
  ]},
  "critique": {"pass": true, "issues": []}
}
```

The model's correct response is to shorten the label to "Notifier" via `submit_ir`, then call `render_view` again, not to ask for a wider box.

## Tech stack and build roadmap

| Layer | Pick | Alternatives |
| --- | --- | --- |
| Orchestrator | Go service | Python + LangGraph |
| Repo access | `go-git`, `go-gitignore` | GitHub API |
| Code parsing | tree-sitter (`smacker/go-tree-sitter`), `go/ast` | ctags, LSP servers |
| Infra parsing | `yaml.v3`, `hcl/v2`, `protocompile`, `kin-openapi` | `terraform graph` |
| LLM (extraction, planning) | Any strong tool-calling model with JSON-schema output | Smaller local model for summaries via Ollama |
| Vision critic | Any vision-capable model | Skip it in v1 and rely on lint |
| Layout | ELK.js `layered` (Node sidecar) | D2, Graphviz, dagre |
| Text measuring | `golang.org/x/image/font/opentype` | Character-width table |
| Rendering | Go `text/template` SVG components | `templ`, D2 themes |
| Rasterising | resvg | `chromedp`, Playwright |
| Optimising | SVGO | scour |
| Delivery | CLI, GitHub Action, MCP server | VS Code extension |

**Roadmap:**

1. **Phase 1, about a week: D2 shortcut.** Stage 1 (compose + `go.mod` only) → LLM writes the IR → convert the IR to D2 → render with D2's ELK layout and a custom theme. This proves the extraction quality before you invest in rendering. Exit when it's right on 5 test repos.
2. **Phase 2, about 2 weeks: own renderer.** Replace D2 with ELK.js + your SVG components + design tokens. Add the geometric lint. Exit when lint passes on 20 repos without manual fixes.
3. **Phase 3, about 2 weeks: deep context.** tree-sitter skeletons, import graph, per-package summaries with caching, evidence links. Add the planner with view decomposition.
4. **Phase 4, about a week: self-correction.** resvg + vision critique loop, eval set with scores tracked on every prompt change.
5. **Phase 5: ship.** CLI, GitHub Action, MCP server, interactive HTML with click-through evidence. Try it on Conveyor first; it makes a good demo project for placements.

The single most important habit across all phases: when a diagram looks wrong, fix the stage that caused it (context, IR, plan, layout or tokens), and never patch the SVG by hand.
