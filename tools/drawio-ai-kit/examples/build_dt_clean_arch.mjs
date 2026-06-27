// doc_tech — Clean Architecture layer direction:  adapters → ports ← app → domain
import { writeFileSync } from "node:fs";
import { Diagram } from "../src/builder.mjs";
import { frame, ossBox, renderTree } from "../src/layout-engine.mjs";

const OUT = process.env.OUT ?? new URL("../out/clean-arch-layers.drawio", import.meta.url);
const d = new Diagram("mesh");

const ADAPT = { fill: "#FDEEE0", stroke: "#ED7100" };
const PORT  = { fill: "#EEF1F5", stroke: "#5A6B7B" };
const APP   = { fill: "#EAF2FB", stroke: "#3B6FB0" };
const DOM   = { fill: "#EAF3EC", stroke: "#5E9B57" };
const card = (id, label, kind) => ossBox(id, label, { ...kind, w: 210, h: 132, bold: true, fs: 11 });

const adapters = card("adapters", "ADAPTERS\n(adapters/)\n\nHTTP handlers (Gin)\nPostgres repo (pgx)\nRedis · Prometheus\nSlack notifier", ADAPT);
const ports    = card("ports",    "PORTS\n(ports/) — interfaces\n\nRepository · Cache\nNotifier\nEventPublisher", PORT);
const app      = card("app",      "APP\n(app/) — use cases\n\nLoginUser\nFireAlert\nRotateRefreshToken", APP);
const domain   = card("domain",   "DOMAIN\n(domain/) — core\n\nentities · value objects\ndomain errors · events\nimport: chỉ stdlib", DOM);

const row = frame("row", "", { dir: "row", gap: 70, align: "center", header: 0, fill: "none", stroke: "none" },
  [adapters, ports, app, domain]);
const note = ossBox("note", "Dependency Rule: phụ thuộc CHỈ hướng vào trong (domain). domain không biết gì về adapters.\nports do app/domain định nghĩa (DIP); adapters implement ports → dễ thay thế & test bằng mock.",
  { fill: "#FFF8E6", stroke: "#D6B656", w: 980, h: 56, fs: 11 });
const tree = frame("root", "", { dir: "col", gap: 30, align: "center", header: 0, pad: 12, fill: "none", stroke: "none" }, [row, note]);

renderTree(d, tree, [40, 80]);
d.title("Clean Architecture — adapters → ports ← app → domain (một chiều)");
d.link("adapters", "ports", "implements");
d.link("app", "ports", "depends on");
d.link("app", "domain", "uses");

const res = d.validate();
console.log("clean-arch:", JSON.stringify({ ok: res.ok, advice: res.audit.advice }));
writeFileSync(OUT, d.mxfile("Clean Architecture layers"));
