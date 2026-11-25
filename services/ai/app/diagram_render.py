"""Draw an architecture graph as an SVG: layered layout, our own renderer, and a geometric lint.

Pure functions. The graph has already been validated (see app/diagram.py); nothing here trusts
text, and every string that reaches the SVG is escaped. The model never supplies coordinates,
colours or markup.
"""

# ruff: noqa: E501  (SVG templates read better on one line)
from __future__ import annotations

from collections import defaultdict
from dataclasses import dataclass, field
from itertools import pairwise
from xml.sax.saxutils import escape

FONT = "Inter, -apple-system, 'Segoe UI', Roboto, Helvetica, Arial, sans-serif"
LABEL_PX, SUB_PX, EDGE_PX, TITLE_PX = 14, 11, 11, 18
CHAR_W = {LABEL_PX: 7.8, SUB_PX: 6.3, EDGE_PX: 6.3, TITLE_PX: 9.6}  # generous average advance, px
NODE_H, PAD_X, MARGIN, TITLE_H, LEGEND_H = 56, 14, 32, 48, 36
NODE_H_DETAIL = 74  # a box with a third line saying what the part does
MIN_W, MAX_W = 120, 240
SLOT_H = 14  # the room a long edge takes in a layer it passes through
MAX_SVG_BYTES = 150_000

# kind -> (fill, stroke, text). One colour per kind; the model never chooses colours.
KIND_STYLE = {
    "client": ("#eef2ff", "#6366f1", "#1e1b4b"),
    "gateway": ("#f5f3ff", "#8b5cf6", "#2e1065"),
    "service": ("#eff6ff", "#3b82f6", "#172554"),
    "worker": ("#ecfeff", "#06b6d4", "#083344"),
    "datastore": ("#f0fdf4", "#22c55e", "#052e16"),
    "cache": ("#fefce8", "#eab308", "#422006"),
    "queue": ("#fff7ed", "#f97316", "#431407"),
    "external": ("#f4f4f5", "#71717a", "#18181b"),
}
KIND_NAME = {
    "client": "Client", "gateway": "Gateway", "service": "Service", "worker": "Worker",
    "datastore": "Data store", "cache": "Cache", "queue": "Queue", "external": "External",
}  # fmt: skip
EDGE_DASH = {"sync": "", "async": "6 4", "data": "2 4"}
EDGE_NAME = {"sync": "Request", "async": "Message or job", "data": "Data"}


@dataclass
class Node:
    id: str
    label: str
    sublabel: str
    kind: str
    detail: str = ""  # what the part does, a few words; drawn as a third line


@dataclass
class Edge:
    src: str
    dst: str
    label: str
    kind: str


@dataclass
class Box:
    x: float = 0
    y: float = 0
    w: float = 0
    h: float = 0

    @property
    def cx(self) -> float:
        return self.x + self.w / 2

    @property
    def cy(self) -> float:
        return self.y + self.h / 2


@dataclass
class Layout:
    boxes: dict[str, Box]
    # per edge index: the points the line passes through, left to right, and whether it was reversed
    routes: dict[int, tuple[list[tuple[float, float]], bool]]
    width: float
    height: float
    notes: list[str] = field(default_factory=list)


def text_w(text: str, px: int) -> float:
    return len(text) * CHAR_W[px]


def node_width(n: Node) -> float:
    widest = max(text_w(n.label, LABEL_PX), text_w(n.sublabel, SUB_PX), text_w(n.detail, SUB_PX))
    return min(MAX_W, max(MIN_W, widest + 2 * PAD_X))


def node_height(nodes: list[Node]) -> float:
    """Every box in a drawing is the same height: taller when any of them has a third line."""
    return NODE_H_DETAIL if any(n.detail for n in nodes) else NODE_H


def _layers(nodes: list[Node], edges: list[Edge]) -> tuple[dict[str, int], set[int]]:
    """Longest-path layers; edges that would close a cycle are set aside (returned by index)."""
    out: dict[str, list[tuple[str, int]]] = defaultdict(list)
    for i, e in enumerate(edges):
        out[e.src].append((e.dst, i))
    state: dict[str, int] = {}
    back: set[int] = set()

    def visit(u: str) -> None:
        state[u] = 1
        for v, i in out[u]:
            if state.get(v) == 1:
                back.add(i)
            elif v not in state:
                visit(v)
        state[u] = 2

    for n in nodes:  # input order: a stable, readable result
        if n.id not in state:
            visit(n.id)
    layer = {n.id: 0 for n in nodes}
    for _ in range(len(nodes)):
        changed = False
        for i, e in enumerate(edges):
            if i in back or e.src == e.dst:
                continue
            if layer[e.dst] < layer[e.src] + 1:
                layer[e.dst] = layer[e.src] + 1
                changed = True
        if not changed:
            break
    return layer, back


def _chains(
    edges: list[Edge], layer: dict[str, int], back: set[int], cols: list[list[str]]
) -> tuple[dict[int, list[str]], dict[int, bool]]:
    """Every edge as a chain src -> slot -> ... -> dst in forward (low to high layer) order.

    Slots are added to the columns an edge passes through, so the layout reserves room for it.
    """
    chains: dict[int, list[str]] = {}
    flipped: dict[int, bool] = {}
    for i, e in enumerate(edges):
        first, last = (e.dst, e.src) if i in back else (e.src, e.dst)
        flipped[i] = i in back
        if layer[last] < layer[first]:  # a set-aside edge whose ends sit the other way round
            first, last = last, first
            flipped[i] = not flipped[i]
        chain = [first]
        for k in range(layer[first] + 1, layer[last]):
            slot = f"~{i}.{k}"
            cols[k].append(slot)
            chain.append(slot)
        chain.append(last)
        chains[i] = chain
    return chains, flipped


def _order(cols: list[list[str]], chains: dict[int, list[str]]) -> None:
    """Reorder each column by the average position of its neighbours, to cut crossings."""
    neighbours: dict[str, list[str]] = defaultdict(list)
    for chain in chains.values():
        for u, v in pairwise(chain):
            neighbours[u].append(v)
            neighbours[v].append(u)

    def sweep(fixed: int, moving: int) -> None:
        pos = {nid: j for j, nid in enumerate(cols[fixed])}
        own = {nid: j for j, nid in enumerate(cols[moving])}  # sort() empties the list while it runs

        def key(nid: str) -> float:
            ps = [pos[m] for m in neighbours[nid] if m in pos]
            return sum(ps) / len(ps) if ps else float(own[nid])

        cols[moving].sort(key=key)

    for _ in range(3):  # a few sweeps are plenty for 9 boxes
        for c in range(1, len(cols)):
            sweep(c - 1, c)
        for c in range(len(cols) - 2, -1, -1):
            sweep(c + 1, c)


def _place(
    nodes: list[Node], cols: list[list[str]], spread: float, min_gap_x: float = 0.0
) -> tuple[dict[str, Box], float, float]:
    by_id = {n.id: n for n in nodes}
    node_h = node_height(nodes)
    gap_y, gap_x = 28 * spread, max(96 * spread, min_gap_x)
    col_w = [max((node_width(by_id[i]) for i in col if i in by_id), default=24.0) for col in cols]
    col_h = [sum(node_h if i in by_id else SLOT_H for i in col) + gap_y * max(len(col) - 1, 0) for col in cols]
    height = max(col_h)
    boxes: dict[str, Box] = {}
    x = float(MARGIN)
    for c, col in enumerate(cols):
        y = MARGIN + TITLE_H + (height - col_h[c]) / 2
        for nid in col:
            if nid in by_id:
                w = node_width(by_id[nid])
                boxes[nid] = Box(x + (col_w[c] - w) / 2, y, w, node_h)
                y += node_h + gap_y
            else:
                boxes[nid] = Box(x + col_w[c] / 2, y, 0, SLOT_H)
                y += SLOT_H + gap_y
        x += col_w[c] + gap_x
    return boxes, x - gap_x + MARGIN, MARGIN + TITLE_H + height + MARGIN + LEGEND_H


def _route(chain: list[str], boxes: dict[str, Box]) -> list[tuple[float, float]]:
    pts: list[tuple[float, float]] = []
    for k, nid in enumerate(chain):
        box = boxes[nid]
        if k == 0:
            pts.append((box.x + box.w, box.cy))
        elif k == len(chain) - 1:
            pts.append((box.x, box.cy))
        else:
            pts.append((box.cx, box.cy))
    return pts


def layout(nodes: list[Node], edges: list[Edge], spread: float = 1.0) -> Layout:
    """Place nodes in columns by layer and route every edge through reserved slots."""
    edges = [e for e in edges if e.src != e.dst]
    layer, back = _layers(nodes, edges)
    cols: list[list[str]] = [[] for _ in range(max(layer.values()) + 1)]
    for n in nodes:
        cols[layer[n.id]].append(n.id)
    chains, flipped = _chains(edges, layer, back, cols)
    _order(cols, chains)
    # An edge label sits in the gap between two columns, so the gap must be wider than the longest one.
    widest = max((text_w(e.label, EDGE_PX) + 10 + 8 + 16 for e in edges if e.label), default=0.0)
    boxes, width, height = _place(nodes, cols, spread, widest)
    # Edges between the same two boxes (a call and its reply) would lie on top of each other.
    groups: dict[tuple[str, str], list[int]] = defaultdict(list)
    for i, chain in chains.items():
        groups[(chain[0], chain[-1])].append(i)
    routes: dict[int, tuple[list[tuple[float, float]], bool]] = {}
    for members in groups.values():
        for rank, i in enumerate(members):
            shift = (rank - (len(members) - 1) / 2) * 24
            routes[i] = ([(x, y + shift) for x, y in _route(chains[i], boxes)], flipped[i])
    return Layout(boxes, routes, width, height)


def _path(pts: list[tuple[float, float]]) -> str:
    d = f"M {pts[0][0]:.1f} {pts[0][1]:.1f}"
    for (x0, y0), (x1, y1) in pairwise(pts):
        mx = (x0 + x1) / 2
        d += f" C {mx:.1f} {y0:.1f} {mx:.1f} {y1:.1f} {x1:.1f} {y1:.1f}"
    return d


def _edge_label_box(pts: list[tuple[float, float]], label: str) -> Box:
    k = (len(pts) - 1) // 2  # the middle segment
    (x0, y0), (x1, y1) = pts[k], pts[k + 1]
    w = text_w(label, EDGE_PX) + 10
    return Box((x0 + x1) / 2 - w / 2, (y0 + y1) / 2 - 9, w, 18)


def _hit(a: Box, b: Box) -> bool:
    return a.x < b.x + b.w and b.x < a.x + a.w and a.y < b.y + b.h and b.y < a.y + a.h


def _label_problems(edges: list[Edge], lay: Layout, real: list[tuple[Node, Box]]) -> list[str]:
    problems: list[str] = []
    labelled = [(i, e) for i, e in enumerate(edges) if e.src != e.dst and e.label]
    boxes = [(i, _edge_label_box(lay.routes[i][0], e.label)) for i, e in labelled]
    for i, lb in boxes:
        pts = lay.routes[i][0]
        problems += [f"edge label {i} covers {n.id}" for n, bn in real if _hit(lb, bn)]
        k = (len(pts) - 1) // 2  # a label wider than the gap it sits in would run into the boxes either side
        if abs(pts[k][0] - pts[k + 1][0]) < lb.w + 8:
            problems.append(f"edge label {i} does not fit its gap")
    for k, (i, la) in enumerate(boxes):
        problems += [f"edge labels {i} and {j} overlap" for j, lb in boxes[k + 1 :] if _hit(la, lb)]
    return problems


def lint(nodes: list[Node], edges: list[Edge], lay: Layout, svg: str) -> list[str]:
    """Geometric checks. An empty list means the drawing is usable."""
    problems: list[str] = []
    real = [(n, lay.boxes[n.id]) for n in nodes]
    for i, (a, ba) in enumerate(real):
        problems += [f"boxes overlap: {a.id}, {b.id}" for b, bb in real[i + 1 :] if _hit(ba, bb)]
        room = ba.w - 2 * PAD_X + 1
        if text_w(a.label, LABEL_PX) > room or max(text_w(a.sublabel, SUB_PX), text_w(a.detail, SUB_PX)) > room:
            problems.append(f"text overflows: {a.id}")
    problems += _label_problems(edges, lay, real)
    if lay.width > 2400 or lay.height > 1800:
        problems.append("drawing too large")
    if len(svg.encode()) > MAX_SVG_BYTES:
        problems.append("svg too large")
    return problems


def render(title: str, nodes: list[Node], edges: list[Edge], lay: Layout) -> str:
    """The SVG document. All text is escaped; there are no scripts, links or external references."""
    w, h = lay.width, lay.height
    out = [
        f'<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 {w:.0f} {h:.0f}" width="{w:.0f}" height="{h:.0f}" '
        f'font-family="{FONT}" role="img" aria-label="{escape(title, {chr(34): "&quot;"})}">',
        f"<title>{escape(title)}</title>",
        "<defs>"
        '<marker id="arrow" viewBox="0 0 10 10" refX="9" refY="5" markerWidth="8" markerHeight="8" '
        'orient="auto-start-reverse"><path d="M 0 0 L 10 5 L 0 10 z" fill="#52525b"/></marker>'
        "</defs>",
        f'<rect width="{w:.0f}" height="{h:.0f}" fill="#ffffff"/>',
        f'<text x="{MARGIN}" y="{MARGIN + 6}" font-size="{TITLE_PX}" font-weight="600" fill="#18181b">{escape(title)}</text>',
    ]
    for i, e in enumerate(edges):
        if i not in lay.routes:
            continue
        pts, rev = lay.routes[i]
        dash = EDGE_DASH.get(e.kind, "")
        marker = "marker-start" if rev else "marker-end"
        out.append(
            f'<path d="{_path(pts)}" fill="none" stroke="#52525b" stroke-width="1.5"'
            + (f' stroke-dasharray="{dash}"' if dash else "")
            + f' {marker}="url(#arrow)"/>'
        )
    for i, e in enumerate(edges):
        if i in lay.routes and e.label:
            pts, _ = lay.routes[i]
            lb = _edge_label_box(pts, e.label)
            out.append(
                f'<rect x="{lb.x:.1f}" y="{lb.y:.1f}" width="{lb.w:.1f}" height="{lb.h:.1f}" rx="4" fill="#ffffff" fill-opacity="0.92"/>'
                f'<text x="{lb.cx:.1f}" y="{lb.cy + 4:.1f}" font-size="{EDGE_PX}" text-anchor="middle" fill="#3f3f46">{escape(e.label)}</text>'
            )
    for n in nodes:
        b = lay.boxes[n.id]
        fill, stroke, ink = KIND_STYLE[n.kind]
        out.append(
            f'<rect x="{b.x:.1f}" y="{b.y:.1f}" width="{b.w:.1f}" height="{b.h:.1f}" rx="8" fill="{fill}" stroke="{stroke}" stroke-width="1.5"/>'
        )
        # The lines are centred together in the box: the label, then the technology, then what it does.
        lines = [(n.label, LABEL_PX, ' font-weight="600"', 1.0)]
        if n.sublabel:
            lines.append((n.sublabel, SUB_PX, "", 0.75))
        if n.detail:
            lines.append((n.detail, SUB_PX, ' font-style="italic"', 0.6))
        step = 17
        top = b.cy - step * (len(lines) - 1) / 2 + 5
        for k, (text, px, extra, opacity) in enumerate(lines):
            out.append(
                f'<text x="{b.cx:.1f}" y="{top + k * step:.1f}" font-size="{px}"{extra} text-anchor="middle" fill="{ink}"'
                + (f' fill-opacity="{opacity}"' if opacity < 1 else "")
                + f">{escape(text)}</text>"
            )
    # Legend: only the kinds that appear.
    x = float(MARGIN)
    y = h - LEGEND_H / 2 - 4
    for kind in dict.fromkeys(n.kind for n in nodes):
        fill, stroke, _ = KIND_STYLE[kind]
        out.append(
            f'<rect x="{x:.1f}" y="{y - 8:.1f}" width="14" height="14" rx="3" fill="{fill}" stroke="{stroke}" stroke-width="1.5"/>'
            f'<text x="{x + 20:.1f}" y="{y + 3:.1f}" font-size="{EDGE_PX}" fill="#3f3f46">{escape(KIND_NAME[kind])}</text>'
        )
        x += 20 + text_w(KIND_NAME[kind], EDGE_PX) + 18
    # And the line styles, when more than one is used.
    styles = list(dict.fromkeys(e.kind for e in edges))
    if len(styles) > 1:
        x += 10
        for kind in styles:
            dash = EDGE_DASH.get(kind, "")
            out.append(
                f'<line x1="{x:.1f}" y1="{y - 1:.1f}" x2="{x + 26:.1f}" y2="{y - 1:.1f}" stroke="#52525b" stroke-width="1.5"'
                + (f' stroke-dasharray="{dash}"' if dash else "")
                + "/>"
                f'<text x="{x + 32:.1f}" y="{y + 3:.1f}" font-size="{EDGE_PX}" fill="#3f3f46">{escape(EDGE_NAME.get(kind, kind))}</text>'
            )
            x += 32 + text_w(EDGE_NAME.get(kind, kind), EDGE_PX) + 18
    out.append("</svg>")
    return "".join(out)
