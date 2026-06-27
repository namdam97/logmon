// doc_tech — Notification hub: events → hub (template/queue/dedup) → fan-out đa kênh
import { writeFileSync } from "node:fs";
import { Diagram } from "../src/builder.mjs";
import { frame, icon, ossBox, renderTree } from "../src/layout-engine.mjs";

const OUT = process.env.OUT ?? new URL("../out/notification-hub.drawio", import.meta.url);
const d = new Diagram("hubspoke");

const ev = (id, label) => ossBox(id, label, { fill: "#F3EEF8", stroke: "#8C4FFF", w: 178, h: 50, bold: true, fs: 10 });
const chan = (id, label) => ossBox(id, label, { fill: "#EAF2FB", stroke: "#3B6FB0", w: 150, h: 46, fs: 10 });

const events = frame("ev", "Domain events (nguồn)", { dir: "col", gap: 18, fill: "#FAFBFC", stroke: "#9AA7B4" }, [
  ev("fired", "AlertFired"),
  ev("resolved", "AlertResolved"),
  ev("escalated", "IncidentEscalated"),
]);
const hub = frame("hub", "Notification Hub (BC notification — planned)", { dir: "col", gap: 12, fill: "#FDEEE0", stroke: "#ED7100" }, [
  ossBox("tmpl", "Template engine\n(render theo kênh)", { fill: "#fff", stroke: "#ED7100", w: 196, h: 48, fs: 10 }),
  ossBox("queue", "Queue: Redis Streams\nworker + retry (at-least-once)", { fill: "#fff", stroke: "#ED7100", w: 196, h: 48, fs: 10 }),
  ossBox("dedup", "Dedup / idempotency\n(group, rate-limit)", { fill: "#fff", stroke: "#ED7100", w: 196, h: 48, fs: 10 }),
]);
const channels = frame("ch", "Kênh gửi (fan-out)", { dir: "col", gap: 12, fill: "#FAFBFC", stroke: "#9AA7B4" }, [
  chan("slack", "Slack"),
  chan("email", "Email (SMTP)"),
  icon("pd", "pagerduty", "PagerDuty"),
  chan("teams", "Teams"),
  chan("webhook", "Webhook"),
  chan("inapp", "In-app (UI)"),
]);

const tree = frame("root", "", { dir: "row", gap: 70, align: "center", header: 0, pad: 12, fill: "none", stroke: "none" }, [events, hub, channels]);
renderTree(d, tree, [40, 80]);
d.title("Notification Hub đa kênh — event → template/queue/dedup → fan-out (planned, doc_v2/06)");

d.link("fired", "tmpl", "", { dir: "LR" });
d.link("resolved", "tmpl", "", { dir: "LR" });
d.link("escalated", "tmpl", "", { dir: "LR" });
d.link("dedup", "slack", "", { role: "fanout" });
d.link("dedup", "email", "", { role: "fanout" });
d.link("dedup", "pd", "", { role: "fanout" });
d.link("dedup", "teams", "", { role: "fanout" });
d.link("dedup", "webhook", "", { role: "fanout" });
d.link("dedup", "inapp", "", { role: "fanout" });

const res = d.validate();
console.log("notification:", JSON.stringify({ ok: res.ok, advice: res.audit.advice }));
writeFileSync(OUT, d.mxfile("Notification hub"));
