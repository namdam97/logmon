// doc_tech — Incident state machine (7 trạng thái) + MTTA/MTTR + escalation
import { writeFileSync } from "node:fs";
import { Diagram } from "../src/builder.mjs";
import { frame, ossBox, renderTree } from "../src/layout-engine.mjs";

const OUT = process.env.OUT ?? new URL("../out/incident-state-machine.drawio", import.meta.url);
const d = new Diagram("pipeline");

const st = (id, label, fill, stroke) => ossBox(id, label, { fill, stroke, w: 126, h: 58, bold: true, fs: 10 });
const states = frame("states", "", { dir: "row", gap: 88, align: "center", header: 0, fill: "none", stroke: "none" }, [
  st("detected", "1. Detected\n(mở)", "#FDECEC", "#DD344C"),
  st("ack", "2. Acknowledged\n(đã nhận)", "#FDEEE0", "#ED7100"),
  st("invest", "3. Investigating\n(điều tra)", "#FFF8E6", "#D6B656"),
  st("mitig", "4. Mitigating\n(khắc phục)", "#FFF8E6", "#D6B656"),
  st("monitor", "5. Monitoring\n(theo dõi)", "#EAF2FB", "#3B6FB0"),
  st("resolved", "6. Resolved\n(đã xử lý)", "#EAF3EC", "#5E9B57"),
  st("closed", "7. Closed\n(+ postmortem)", "#EEF1F5", "#5A6B7B"),
]);
const note = ossBox("note", "MTTA = Detected→Acknowledged   ·   MTTR = Detected→Resolved   ·   SEV1–4 quyết định tốc độ escalation & kênh báo   ·   Monitoring có thể quay lại Investigating nếu tái phát (reopen)   ·   (planned: BC incident, doc_v2/06)",
  { fill: "#FFF8E6", stroke: "#D6B656", w: 1180, h: 44, fs: 10 });
const esc = ossBox("esc", "On-call escalation\nSEV1 → L2/L3 (PagerDuty)", { fill: "#FDECEC", stroke: "#DD344C", w: 190, h: 50, fs: 10 });

const tree = frame("root", "", { dir: "col", gap: 34, align: "center", header: 0, pad: 12, fill: "none", stroke: "none" }, [states, esc, note]);
renderTree(d, tree, [40, 80]);
d.title("Incident lifecycle — state machine 7 trạng thái (MTTA/MTTR · escalation)");

d.link("detected", "ack", "ack");
d.link("ack", "invest", "triage");
d.link("invest", "mitig", "RCA");
d.link("mitig", "monitor", "fix");
d.link("monitor", "resolved", "ổn định");
d.link("resolved", "closed", "P.M.");
d.link("esc", "ack", "SEV1/2 → page", { dash: true });

const res = d.validate();
console.log("incident:", JSON.stringify({ ok: res.ok, advice: res.audit.advice }));
writeFileSync(OUT, d.mxfile("Incident state machine"));
