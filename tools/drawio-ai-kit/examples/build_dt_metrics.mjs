// doc_tech — Metrics: Prometheus (PULL) + Thanos + Alertmanager → channels + Grafana
import { writeFileSync } from "node:fs";
import { Diagram } from "../src/builder.mjs";
import { frame, stage, icon, endpoint, ossBox, renderTree } from "../src/layout-engine.mjs";
import { GRAFANA_ICON } from "./_grafana_icon.mjs";

const OUT = process.env.OUT ?? new URL("../out/metrics-prometheus-alertmanager.drawio", import.meta.url);
const d = new Diagram("pipeline");
d.c.byName.set(GRAFANA_ICON.name, { ...GRAFANA_ICON, kind: "icon" });
d.c.validNames.add(GRAFANA_ICON.name);

const scrape = stage("s", 0, "1 · Scrape (PULL)", [
  icon("prom", "prometheus", "Prometheus\nscrape /metrics"),
]);
const store = stage("st", 1, "2 · Store + Rule eval", [
  icon("thanos", "thanos", "Thanos\n(long-term, Mode B)"),
  ossBox("am", "Alertmanager\ndedup · group · route\n· inhibit · silence", { fill: "#FDECEC", stroke: "#DD344C", w: 188, h: 84 }),
]);
const out = stage("o", 2, "3 · Visualize + Notify", [
  icon("grafana", "grafana", "Grafana\n(dashboards)"),
  icon("pd", "pagerduty", "PagerDuty\n(on-call)"),
  ossBox("ch", "Slack · Email\n· webhook", { fill: "#EEF1F5", stroke: "#5A6B7B", w: 168, h: 56 }),
]);

const tree = frame("root", "", { dir: "row", gap: 60, align: "center", header: 0, pad: 12, fill: "none", stroke: "none" }, [
  endpoint("app", "GO TARGETS\n\n/metrics (text)\nCounter · Gauge\nHistogram · Summary\nlabels: prefix logmon_"),
  frame("core", "Metrics platform", { dir: "row", gap: 50, fill: "#FAFBFC", stroke: "#9AA7B4" }, [scrape, store, out]),
]);

renderTree(d, tree, [40, 80]);
d.title("Metrics — Prometheus (PULL) + Thanos + Alertmanager → Grafana / on-call");
d.link("app", "prom", "scrape mỗi 15s", { dash: true });
d.link("prom", "thanos", "remote-write");
d.link("prom", "grafana", "PromQL query", { flow: true });
d.link("prom", "am", "firing alerts", { flow: true });
d.link("am", "pd", "SEV1/2", { role: "fanout" });
d.link("am", "ch", "SEV3+", { role: "fanout" });

const res = d.validate();
console.log("metrics:", JSON.stringify({ ok: res.ok, advice: res.audit.advice }));
writeFileSync(OUT, d.mxfile("Metrics Prometheus Alertmanager"));
