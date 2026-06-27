// drawio-ai-kit — Diagram builder. Bundles all boilerplate: icon/box/group/panel/link
// + auto-routing by type + auto-size panel + validate + XML export. Goal: build
// a diagram with just a few lines of declaration (easy to use, easy to extend).
import { loadCatalog, styleForIcon, styleForGroup, validateDiagram } from "./core.mjs";
import { routeLR, routeTB, routeLRFan, routeTBFan, routeLRFanIn, routeTBFanIn, centerInBoxX, distributeY, centerInGapX, panelSize } from "./layout.mjs";
import { typePreset, edgeRounded } from "./types.mjs";
import { THEME } from "./theme.mjs";

const esc = (s) => String(s).replace(/&/g, "&amp;").replace(/</g, "&lt;").replace(/>/g, "&gt;").replace(/"/g, "&quot;");

export class Diagram {
  /** type: pipeline|hierarchy|network|hubspoke|hybrid|mesh|sequence */
  constructor(type = "pipeline", { title = "", page = [2000, 1200] } = {}) {
    this.c = loadCatalog();
    this.type = type;
    this.preset = typePreset(type);
    this.page = page;
    this.cells = [];
    this.R = {};
    this.eid = 0;
    this.edgeSpecs = [];        // edges recorded first, built later (to bundle fan-out 1→N)
    this._edgesBuilt = false;
    if (title) this.text("__title", [0, 24], page[0], title, { fs: 14 });
  }
  _put(id, parent, x, y, w, h, style, label) {
    this.R[id] = { x, y, w, h };
    const p = this.R[parent]; const ox = p ? p.x : 0, oy = p ? p.y : 0;   // layer parents ("1"/"boundaries") → offset 0
    this.cells.push(`<mxCell id="${id}" value="${esc(label)}" style="${style}" vertex="1" parent="${parent}"><mxGeometry x="${x - ox}" y="${y - oy}" width="${w}" height="${h}" as="geometry"/></mxCell>`);
    return this.R[id];
  }
  /** AWS icon by catalog name (verbatim style). [x,y] = top-left corner (48×48 icon). */
  icon(id, name, [x, y], { parent = "1", label = "" } = {}) {
    const s = styleForIcon(this.c, name);
    if (!s) throw new Error(`Icon not found in catalog: "${name}" — use search_icon to look up the correct name.`);
    return this._put(id, parent, x, y, 48, 48, s.style, label);
  }
  // Default SQUARE CORNERS — AWS diagrams rarely use rounded frames. (round:true if needed.)
  box(id, [x, y], [w, h], label = "", { parent = "1", fill = "#FFFFFF", stroke = "#5A6B7B", va = "middle", bold = false, fs = 11, round = false } = {}) {
    return this._put(id, parent, x, y, w, h, `rounded=${round ? 1 : 0};whiteSpace=wrap;html=1;fillColor=${fill};strokeColor=${stroke};fontColor=#1A1A1A;fontSize=${fs};fontStyle=${bold ? 1 : 0};verticalAlign=${va};`, label);
  }
  /** AWS group container (group_aws_cloud_alt, group_region, group_vpc, group_account, ...).
   *  fill/stroke (optional) override the stencil's colours by appending to the style. */
  group(id, gname, [x, y], [w, h], label = "", { parent = "1", fill = null, stroke = null } = {}) {
    const s = styleForGroup(this.c, gname);
    if (!s) throw new Error(`Group not found: "${gname}"`);
    let style = s.style;
    // convention colours (overridable): public subnet green / private subnet blue; Region teal; VPC purple.
    if (!fill && gname === "group_subnet") {
      const priv = /private/i.test(label);
      fill = priv ? THEME.subnetPrivate : THEME.subnetPublic;
      stroke = stroke || (priv ? THEME.subnetPrivateStroke : THEME.subnetPublicStroke);
    }
    if (!stroke && gname === "group_region") stroke = THEME.regionStroke;
    if (!stroke && gname === "group_vpc") stroke = THEME.vpcStroke;
    if (!stroke && gname === "group_account") stroke = THEME.accountStroke;
    if (!stroke && gname === "group_availability_zone") stroke = THEME.azStroke;
    if (fill) style += `fillColor=${fill};`;
    if (stroke) style += `strokeColor=${stroke};`;
    return this._put(id, parent, x, y, w, h, style, label);
  }
  /** Dashed "logical cluster" frame that SPANS already-placed children — call AFTER renderTree (it reads
   *  computed geometry from this.R). Draws a dashed, no-fill frame styled like the Region/AZ containers,
   *  with an icon + label at the TOP-LEFT corner. Use it for a boundary that CROSSES the real container
   *  nesting: an EKS cluster spanning the private subnets of several AZs, a service-mesh/trust boundary,
   *  a logical "platform" grouping, etc. Leave vertical room above the spanned children (a taller inter-tier
   *  gap) so the header strip (icon+label) sits clear of the children's own headers.
   *  opts: { icon (catalog name for the corner logo), stroke, dashed:true, pad, padTop, iconSize, fontColor }. */
  clusterBox(id, childIds, label = "", { icon = null, stroke = "#ED7100", dashed = true, pad = 14, padTop = 34, iconSize = 20, strokeWidth = 1, fontColor = null } = {}) {
    const rs = childIds.map((c) => this.R[c]).filter(Boolean);
    if (!rs.length) return null;
    const x = Math.min(...rs.map((r) => r.x)) - pad;
    const y = Math.min(...rs.map((r) => r.y)) - padTop;
    const w = Math.max(...rs.map((r) => r.x + r.w)) + pad - x;
    const h = Math.max(...rs.map((r) => r.y + r.h)) + pad - y;
    const fc = fontColor || stroke;
    const spacingLeft = icon ? iconSize + 6 : 6;
    const dash = dashed ? "dashed=1;" : "";
    // Put boundary frames on their OWN draw.io layer ("boundaries", locked by default) so they can be
    // toggled/locked while hand-editing the icons & containers. No fill → only the dashed border shows.
    this._put(id, "boundaries", x, y, w, h, `rounded=0;${dash}fillColor=none;strokeColor=${stroke};strokeWidth=${strokeWidth};verticalAlign=top;align=left;spacingLeft=${spacingLeft};spacingTop=5;fontColor=${fc};fontStyle=1;fontSize=11;`, label);
    if (icon) {
      const s = styleForIcon(this.c, icon);
      if (!s) throw new Error(`clusterBox icon not found in catalog: "${icon}"`);
      this._put(`${id}_icon`, "boundaries", x + 1, y + 1, iconSize, iconSize, s.style, "");   // flush to the top-left corner
    }
    return this.R[id];
  }
  /** Title centered across the page width (call after the page size is known). */
  title(label, { fs = 14 } = {}) { this.text("__title", [0, 24], this.page[0], label, { fs }); return this; }
  text(id, [x, y], w, label, { fs = 14, parent = "1" } = {}) {
    const ox = parent === "1" ? 0 : this.R[parent].x, oy = parent === "1" ? 0 : this.R[parent].y;
    this.R[id] = { x, y, w, h: 30 };
    this.cells.push(`<mxCell id="${id}" value="${esc(label)}" style="text;html=1;align=center;fontStyle=1;fontSize=${fs};fontColor=light-dark(#232F3E,#E8E8E8);" vertex="1" parent="${parent}"><mxGeometry x="${x - ox}" y="${y - oy}" width="${w}" height="30" as="geometry"/></mxCell>`);
  }
  /**
   * Panel that AUTO-SIZES to the icon count: draws a snug box, icons centered in columns + evenly distributed.
   * items = [[iconName, label], ...]. Returns the panel's rect.
   */
  panel(id, [x, y], title, items, { parent = "1", cols = 1, fill = "#F5F5F5", stroke = "#999999", itemW = 130, itemH = 84 } = {}) {
    const { w, h } = panelSize(items.length, { cols, itemW, itemH });
    this.box(id, [x, y], [w, h], title, { parent, fill, stroke, va: "top", bold: true });
    const pad = 20, header = 34, gap = 18;
    items.forEach(([name, label], i) => {
      const r = Math.floor(i / cols), col = i % cols;
      const ix = Math.round(x + pad + col * (itemW + gap) + (itemW - 48) / 2);
      const iy = Math.round(y + header + pad + r * (itemH + gap));
      this.icon(`${id}_${i}`, name, [ix, iy], { parent: id, label });
    });
    return this.R[id];
  }
  /** Edge: just provide source→target + label; the router goes straight/through-gap automatically; corners by type+role.
   *  Recorded first — toXML() bundles edges with the SAME SOURCE and same direction into a fan-out BUNDLE
   *  (comb/trunk sharing a lane) so 1→N edges don't overlap/break.
   *  opts: { dir: LR|TB (auto by position if omitted), role: flow|fanout|tree, dash: true (sync/DR),
   *          flow: true (animated moving-dash flow — shows in SVG / draw.io app, not in PNG) }. */
  link(src, tgt, label = "", opts = {}) {
    if (!this.R[src]) throw new Error(`link: source does not exist yet "${src}"`);
    if (!this.R[tgt]) throw new Error(`link: target does not exist yet "${tgt}"`);
    this.edgeSpecs.push({ src, tgt, label, opts });
    return this;
  }

  /** Build all edges. Detect FAN-OUT bundles (1 source → ≥2 same-direction targets) and
   *  FAN-IN bundles (≥2 same-direction sources → 1 target); each bundle SHARES one lane so
   *  collinear segments merge into a single trunk and arrowheads don't stack. Fan-out wins
   *  when an edge qualifies as both. */
  _buildEdges() {
    if (this._edgesBuilt) return;
    this._edgesBuilt = true;
    // Direction is AUTO-DETECTED from the nodes' relative position (vertical offset dominates → TB,
    // else LR), so a vertically-stacked pair routes straight down without the caller remembering.
    // opts.dir is an explicit override.
    const dirOf = (e) => {
      if (e.opts.dir) return e.opts.dir;
      const a = this.R[e.src], b = this.R[e.tgt];
      const dx = Math.abs((a.x + a.w / 2) - (b.x + b.w / 2));
      const dy = Math.abs((a.y + a.h / 2) - (b.y + b.h / 2));
      return dy > dx ? "TB" : "LR";
    };
    const R = (i, side) => this.R[this.edgeSpecs[i][side]];
    const clamp = (v) => Math.max(0.2, Math.min(0.8, v));
    const route = this.edgeSpecs.map(() => null);

    // FAN-OUT: group by (direction, source)
    const outG = {};
    this.edgeSpecs.forEach((e, i) => ((outG[`${dirOf(e)}|${e.src}`] ||= []).push(i)));
    for (const k in outG) {
      const idxs = outG[k];
      if (idxs.length < 2) continue;
      const axis = k.slice(0, 2), s = R(idxs[0], "src");
      let lane;
      if (axis === "LR") {
        const left = idxs.every((i) => R(i, "tgt").x + R(i, "tgt").w <= s.x + s.w / 2);  // targets on the left?
        lane = left ? Math.round((s.x + Math.max(...idxs.map((i) => R(i, "tgt").x + R(i, "tgt").w))) / 2)
                    : Math.round((s.x + s.w + Math.min(...idxs.map((i) => R(i, "tgt").x))) / 2);
      } else {
        const up = idxs.every((i) => R(i, "tgt").y + R(i, "tgt").h <= s.y + s.h / 2);     // targets above?
        lane = up ? Math.round((s.y + Math.max(...idxs.map((i) => R(i, "tgt").y + R(i, "tgt").h))) / 2)
                  : Math.round((s.y + s.h + Math.min(...idxs.map((i) => R(i, "tgt").y))) / 2);
      }
      idxs.forEach((i) => (route[i] = { kind: "fanout", axis, lane }));
    }

    // FAN-IN: group by (direction, target) — only for edges not already in a fan-out bundle
    const inG = {};
    this.edgeSpecs.forEach((e, i) => ((inG[`${dirOf(e)}|${e.tgt}`] ||= []).push(i)));
    for (const k in inG) {
      const idxs = inG[k].filter((i) => !route[i]);
      if (idxs.length < 2) continue;
      const axis = k.slice(0, 2), t = R(idxs[0], "tgt");
      // If the target SPANS all the sources (each source sits directly over/beside it), straight
      // drops read cleaner than a converging comb — leave them as plain edges (routeTB/LR straight).
      const spans = axis === "TB"
        ? idxs.every((i) => { const s = R(i, "src"), cx = s.x + s.w / 2; return cx > t.x + 4 && cx < t.x + t.w - 4; })
        : idxs.every((i) => { const s = R(i, "src"), cy = s.y + s.h / 2; return cy > t.y + 4 && cy < t.y + t.h - 4; });
      if (spans) continue;
      let lane;
      if (axis === "LR") {
        const left = idxs.every((i) => { const s = R(i, "src"); return s.x + s.w <= t.x + t.w / 2; });  // sources left of target?
        lane = left ? Math.round((Math.max(...idxs.map((i) => R(i, "src").x + R(i, "src").w)) + t.x) / 2)
                    : Math.round((t.x + t.w + Math.min(...idxs.map((i) => R(i, "src").x))) / 2);
      } else {
        const up = idxs.every((i) => { const s = R(i, "src"); return s.y + s.h <= t.y + t.h / 2; });
        lane = up ? Math.round((Math.max(...idxs.map((i) => R(i, "src").y + R(i, "src").h)) + t.y) / 2)
                  : Math.round((t.y + t.h + Math.min(...idxs.map((i) => R(i, "src").y))) / 2);
      }
      const ord = [...idxs].sort((a, b) => axis === "LR"
        ? R(a, "src").y - R(b, "src").y : R(a, "src").x - R(b, "src").x);
      ord.forEach((i, j) => (route[i] = { kind: "fanin", axis, lane, frac: clamp((j + 1) / (ord.length + 1)) }));
    }

    this.edgeSpecs.forEach((e, i) => this._emitEdge(e, route[i], dirOf(e)));
  }

  _emitEdge({ src, tgt, label = "", opts = {} }, ro, dir) {
    const { role = "flow", dash = false, flow = false, stroke = THEME.edge.stroke, laneX = null, laneY = null } = opts;
    const a = this.R[src], b = this.R[tgt];
    let r, fan = false;
    if (ro && ro.kind === "fanout") {
      fan = true;
      r = ro.axis === "LR" ? routeLRFan(a, b, { laneX: laneX ?? ro.lane }) : routeTBFan(a, b, { laneY: laneY ?? ro.lane });
    } else if (ro && ro.kind === "fanin") {
      fan = true;
      r = ro.axis === "LR" ? routeLRFanIn(a, b, { laneX: laneX ?? ro.lane, entryY: ro.frac })
                           : routeTBFanIn(a, b, { laneY: laneY ?? ro.lane, entryX: ro.frac });
    } else {
      // if a sibling box sits in the straight path (same-column back-edge etc.), route AROUND the
      // side instead of cutting through it (a clean C-bracket).
      const aroundX = this._aroundLaneX(a, b), aroundY = aroundX == null ? this._aroundLaneY(a, b) : null;
      if (aroundX != null) {
        const sy = Math.round(a.y + a.h / 2), ty = Math.round(b.y + b.h / 2);
        r = { pins: "exitX=1;exitY=0.5;exitDx=0;exitDy=0;entryX=1;entryY=0.5;entryDx=0;entryDy=0;", wp: [{ x: aroundX, y: sy }, { x: aroundX, y: ty }] };
      } else if (aroundY != null) {
        const sx = Math.round(a.x + a.w / 2), tx = Math.round(b.x + b.w / 2);
        r = { pins: "exitX=0.5;exitY=0;exitDx=0;exitDy=0;entryX=0.5;entryY=0;entryDx=0;entryDy=0;", wp: [{ x: sx, y: aroundY }, { x: tx, y: aroundY }] };
      } else if (dir === "TB") r = routeTB(a, b, { laneY: laneY != null ? laneY : (a.y + a.h + b.y) / 2 });
      else r = routeLR(a, b, { laneX: laneX != null ? laneX : (a.x + a.w + b.x) / 2 });
    }
    let st = `edgeStyle=orthogonalEdgeStyle;html=1;jettySize=auto;orthogonalLoop=1;fontSize=10;fontColor=${THEME.edge.fontColor};strokeColor=${stroke};strokeWidth=${THEME.edge.strokeWidth};rounded=${edgeRounded(this.preset, fan ? "fanout" : role)};`;
    if (dash) st += "dashed=1;";
    if (flow) st += "flowAnimation=1;";   // animated "moving dashes" flow (shows in SVG / draw.io app, not PNG)
    if (label) st += `labelBackgroundColor=${THEME.edge.labelBg};`;
    st += r.pins;
    const pts = r.wp.length ? `<Array as="points">${r.wp.map((p) => `<mxPoint x="${p.x}" y="${p.y}"/>`).join("")}</Array>` : "";
    this.cells.push(`<mxCell id="ed${++this.eid}" value="${esc(label)}" style="${st}" edge="1" parent="1" source="${src}" target="${tgt}"><mxGeometry relative="1" as="geometry">${pts}</mxGeometry></mxCell>`);
  }
  /** If a sibling box sits in the straight vertical path between two same-column nodes, return an
   *  x just past it to route AROUND (avoid cutting through). Else null. */
  _aroundLaneX(a, b) {
    const xr0 = Math.max(a.x, b.x), xr1 = Math.min(a.x + a.w, b.x + b.w);
    if (xr1 - xr0 < 12) return null;                 // not vertically stacked / same column
    const gTop = Math.min(a.y + a.h, b.y + b.h), gBot = Math.max(a.y, b.y);
    if (gBot - gTop < 8) return null;                // adjacent — nothing between them
    const holds = (p, q) => q.x >= p.x - 2 && q.y >= p.y - 2 && q.x + q.w <= p.x + p.w + 2 && q.y + q.h <= p.y + p.h + 2;
    let right = Math.max(a.x + a.w, b.x + b.w), blocked = false;
    for (const id in this.R) {
      const n = this.R[id];
      if (n === a || n === b || n.w <= 2 || n.h <= 2 || holds(n, a) || holds(n, b)) continue;
      const ov = Math.min(n.x + n.w, xr1) - Math.max(n.x, xr0);
      if (ov > 6 && n.y < gBot - 4 && n.y + n.h > gTop + 4) { blocked = true; right = Math.max(right, n.x + n.w); }
    }
    return blocked ? Math.round(right + 22) : null;
  }

  /** Horizontal analog of _aroundLaneX: a sibling box in the straight horizontal path between two
   *  same-row nodes → return a y above it to route around. Else null. */
  _aroundLaneY(a, b) {
    const yr0 = Math.max(a.y, b.y), yr1 = Math.min(a.y + a.h, b.y + b.h);
    if (yr1 - yr0 < 12) return null;                 // not in the same row
    const gL = Math.min(a.x + a.w, b.x + b.w), gR = Math.max(a.x, b.x);
    if (gR - gL < 8) return null;                    // adjacent — nothing between them
    const holds = (p, q) => q.x >= p.x - 2 && q.y >= p.y - 2 && q.x + q.w <= p.x + p.w + 2 && q.y + q.h <= p.y + p.h + 2;
    let top = Math.min(a.y, b.y), blocked = false;
    for (const id in this.R) {
      const n = this.R[id];
      if (n === a || n === b || n.w <= 2 || n.h <= 2 || holds(n, a) || holds(n, b)) continue;
      const ov = Math.min(n.y + n.h, yr1) - Math.max(n.y, yr0);
      if (ov > 6 && n.x < gR - 4 && n.x + n.w > gL + 4) { blocked = true; top = Math.min(top, n.y); }
    }
    return blocked ? Math.round(top - 22) : null;
  }

  // reusable layout helpers
  centerInGapX(a, b, w) { return centerInGapX(a, b, w); }
  rect(id) { return this.R[id]; }

  /**
   * Node that "spans vertically" (LB/bus/hub) across multiple rows — the kit computes the rect, no numbers/coords at the call site.
   *   spec: { icon, label, w, pad?, fill?, stroke? }
   *   at:   { lane }  (centered in a pre-reserved lane) OR { between:[idA,idB] } (in the gap between 2 nodes)
   *         + { from, to } (height from the top edge of `from` to the bottom edge of `to`)
   */
  spanV(id, { icon, label = "", w, pad = 16, fill = "#FFFFFF", stroke = "#5A6B7B" }, { lane, between, from, to }) {
    const F = this.R[from], T = this.R[to] || F;
    const x = lane ? Math.round(this.R[lane].x + (this.R[lane].w - w) / 2)
                   : centerInGapX(this.R[between[0]], this.R[between[1]], w);
    const y = Math.round(F.y - pad), h = Math.round(T.y + T.h - F.y + pad * 2);
    this.box(id, [x, y], [w, h], label, { fill, stroke, va: "bottom", fs: 10 });
    if (icon) this.icon(`${id}_ic`, icon, [Math.round(x + (w - 48) / 2), y + 12]);
    return this.R[id];
  }

  toXML() {
    this._buildEdges();
    const cellsXml = this.cells.join("");
    // emit a separate (locked) layer for the dashed boundary frames, so editing the content layer is easy.
    const boundsLayer = cellsXml.includes('parent="boundaries"') ? `<mxCell id="boundaries" value="Stack boundaries (locked)" parent="0" style="locked=1;"/>` : "";
    return `<mxGraphModel dx="1400" dy="900" grid="0" gridSize="10" guides="1" tooltips="1" connect="1" arrows="1" fold="1" page="1" pageScale="1" pageWidth="${this.page[0]}" pageHeight="${this.page[1]}" math="0" shadow="0"><root><mxCell id="0"/><mxCell id="1" parent="0"/>${boundsLayer}${cellsXml}</root></mxGraphModel>`;
  }
  validate(opts = { strict: true }) { return validateDiagram(this.c, this.toXML(), opts); }
  mxfile(name = "Diagram") { return `<mxfile host="app.diagrams.net"><diagram name="${esc(name)}" id="d">${this.toXML()}</diagram></mxfile>`; }
}
