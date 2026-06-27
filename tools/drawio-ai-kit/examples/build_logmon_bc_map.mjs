// LogMon — Bounded Contexts context map (DDD). 6 BCs + Shared Kernel + GĐ5 AI layer.
// Cross-BC giao tiếp qua DOMAIN EVENTS (CLAUDE.md), KHÔNG direct import.
// alerting là HUB (4 quan hệ) → hub 3 cột: nguồn (trái) → alerting (giữa) → đích (phải),
// spoke xếp theo CỘT (aligned) nên fan-in/fan-out route thành comb sạch.
import { writeFileSync } from "node:fs";
import { Diagram } from "../src/builder.mjs";
import { frame, band, ossBox, renderTree } from "../src/layout-engine.mjs";

const OUT = process.env.OUT ?? new URL("../out/logmon_bc_map.drawio", import.meta.url);
const d = new Diagram("hubspoke");

// pattern-based tints: purple = DDD+CQRS (complex), blue = Clean Arch (simple)
const DDD = { fill: "#F3EEFF", stroke: "#8C4FFF" };
const CA  = { fill: "#EAF2FB", stroke: "#3B6FB0" };
const bc = (id, label, kind) => ossBox(id, label, { ...kind, w: 198, h: 108, bold: true, fs: 11 });

const identity     = bc("identity",     "IDENTITY\n— Clean Arch —\nauth · users · workspaces\nRBAC / policy", CA);
const logpipeline  = bc("logpipeline",  "LOGPIPELINE\n— DDD + CQRS —\nmode A/B · DLQ\nILM policy", DDD);
const slo          = bc("slo",          "SLO\n— DDD + CQRS —\nerror budget\nburn rate · compliance", DDD);
const alerting     = bc("alerting",     "ALERTING · hub\n— DDD + CQRS —\nthreshold · inhibition\nrouting · escalation", DDD);
const incident     = bc("incident",     "INCIDENT\n— DDD + CQRS —\nstate machine (7) · SEV1–4\nMTTA/MTTR · on-call", DDD);
const notification = bc("notification", "NOTIFICATION\n— Clean Arch —\nSlack · Email · PagerDuty\ntemplate · retry / queue", CA);

// columns: identity (foundational) | sources | hub | sinks  — spokes aligned in columns
// align "top" → logpipeline · alerting · incident sit on one row = a straight top spine;
// slo & notification hang below; incident→notification becomes a clean vertical.
const identityCol = frame("idcol",  "", { dir: "col", header: 0, fill: "none", stroke: "none" }, [identity]);
const inCol       = frame("incol",  "", { dir: "col", gap: 72, header: 0, fill: "none", stroke: "none" }, [logpipeline, slo]);
const hubCol      = frame("hubcol", "", { dir: "col", header: 0, fill: "none", stroke: "none" }, [alerting]);
const outCol      = frame("outcol", "", { dir: "col", gap: 72, header: 0, fill: "none", stroke: "none" }, [incident, notification]);
const bcArea = frame("bcarea", "", { dir: "row", gap: 112, align: "top", header: 0, fill: "none", stroke: "none" },
  [identityCol, inCol, hubCol, outCol]);

// shared kernel = FOUNDATION band (dependency implied by position; only identity anchored for auth)
const shared = band("shared", "Shared Kernel · internal/shared — dùng bởi MỌI BC (no cross-BC import)", [
  ossBox("k_auth",  "auth / JWT"),
  ossBox("k_err",   "errors"),
  ossBox("k_log",   "logger (zerolog)"),
  ossBox("k_metr",  "metrics middleware"),
  ossBox("k_bus",   "domain event bus"),
]);

const core = frame("core", "Go Core · internal/ — Bounded Contexts (Clean Arch / +DDD+CQRS)",
  { dir: "col", gap: 50, align: "center", fill: "#FAFBFC", stroke: "#9AA7B4" }, [bcArea, shared]);

// GĐ5 AI layer: Python service NGOÀI Go core (MCP/webhook/event), ADR-032
const ai = frame("ai", "GĐ5 · AI Incident Automation\n(Python — ngoài Go core, ADR-032)",
  { dir: "col", gap: 16, fill: "#E8F7F1", stroke: "#01A88D" }, [
    ossBox("holmes",  "HolmesGPT\n(RCA / investigate)", { w: 190, h: 58 }),
    ossBox("weknora", "WeKnora\n(RAG · runbooks/retros)", { w: 190, h: 58 }),
    ossBox("hil",     "human-in-loop (L2)", { w: 190, h: 44 }),
  ]);

const tree = frame("root", "", { dir: "row", gap: 64, align: "center", header: 0, pad: 12, fill: "none", stroke: "none" },
  [core, ai]);

renderTree(d, tree, [40, 80]);
d.title("LogMon — Bản đồ Bounded Contexts: domain events + shared kernel (no cross-BC import)");

// --- domain events (CLAUDE.md §Domain Events). solid = event flow; dashed = back-edge ---
d.link("logpipeline", "alerting",     "ModeChanged");        // top spine (left)
d.link("alerting",    "incident",     "AlertFired");         // top spine (right)
d.link("slo",         "alerting",     "BudgetExhausted · RecordFailure"); // two-way (see patch below)
d.link("alerting",    "notification", "Fired / Resolved");   // down into sink
d.link("incident",    "notification", "escalate / page");    // clean vertical

// --- foundation + external (dashed) ---
d.link("identity", "shared", "auth / JWT", { dash: true });   // anchor identity to the kernel
d.link("incident", "ai",     "MCP / webhook", { dash: true });

const res = d.validate();
console.log("VALIDATE:", JSON.stringify({ ok: res.ok, errors: res.errors, warnings: res.warnings, advice: res.audit.advice }, null, 2));

// SLO ⇄ Alerting is a TWO-WAY relationship (slo→alerting: BudgetExhausted, alerting→slo: AlertFired→RecordFailure).
// Render as ONE double-headed edge → avoids two near-collinear lines with overlapping labels. Patch after
// validate (edges are built during validate()); mxfile() reuses the cached cells.
d.cells = d.cells.map((c) =>
  c.includes('source="slo"') && c.includes('target="alerting"')
    ? c.replace("edgeStyle=orthogonalEdgeStyle;", "edgeStyle=orthogonalEdgeStyle;startArrow=block;startFill=1;")
    : c);
writeFileSync(OUT, d.mxfile("LogMon Bounded Contexts map"));
console.log("WROTE:", OUT.pathname ?? OUT);
