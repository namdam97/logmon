// doc_tech — Observability 3 pillars as 3 PARALLEL horizontal lanes: app → lane → unified view
import { writeFileSync } from "node:fs";
import { Diagram } from "../src/builder.mjs";
import { frame, stage, icon, endpoint, renderTree } from "../src/layout-engine.mjs";
import { GRAFANA_ICON } from "./_grafana_icon.mjs";

const OUT = process.env.OUT ?? new URL("../out/observability-3-pillars.drawio", import.meta.url);
const d = new Diagram("pipeline");
d.c.byName.set(GRAFANA_ICON.name, { ...GRAFANA_ICON, kind: "icon" });
d.c.validNames.add(GRAFANA_ICON.name);

// each pillar = one horizontal LANE (dir:row) so flows are straight & never cross another pillar
const metrics = stage("m", 0, "METRICS — “cái gì sai, mức độ”", [
  icon("prom", "prometheus", "Prometheus"),
  icon("thanos", "thanos", "Thanos (long-term)"),
], { dir: "row", gap: 40 });
const logs = stage("l", 1, "LOGS — “chi tiết sự kiện”", [
  icon("otelL", "opentelemetry", "OTel Collector"),
  icon("es", "elasticsearch", "Elasticsearch"),
], { dir: "row", gap: 40 });
const traces = stage("t", 2, "TRACES — “ở đâu chậm / lỗi”", [
  icon("otelT", "opentelemetry", "OTel (tail sampling)"),
  icon("jaeger", "jaeger", "Jaeger v2"),
], { dir: "row", gap: 40 });

const lanes = frame("lanes", "Thu thập & lưu trữ theo TỪNG trụ cột", { dir: "col", gap: 30, fill: "#FAFBFC", stroke: "#9AA7B4" },
  [metrics, logs, traces]);
const view = frame("view", "Khung nhìn HỢP NHẤT", { dir: "col", gap: 26, fill: "#F3EEF8", stroke: "#9673A6" }, [
  icon("grafana", "grafana", "Grafana\n(metrics·logs·traces)"),
  icon("kibana", "kibana", "Kibana (logs)"),
]);

const tree = frame("root", "", { dir: "row", gap: 64, align: "center", header: 0, pad: 12, fill: "none", stroke: "none" }, [
  endpoint("src", "GO MICROSERVICES\n(đã instrument)\n\n1 request →\nmetric + log + trace\nCÙNG trace_id"),
  lanes, view,
]);

renderTree(d, tree, [40, 80]);
d.title("Observability — 3 trụ cột song song: Metrics · Logs · Traces (liên kết qua trace_id)");
d.link("src", "prom", "metrics", { flow: true });
d.link("src", "otelL", "logs", { flow: true });
d.link("src", "otelT", "traces", { flow: true });
d.link("prom", "thanos", "");
d.link("otelL", "es", "");
d.link("otelT", "jaeger", "");
d.link("thanos", "grafana", "query");
d.link("es", "grafana", "query");
d.link("jaeger", "grafana", "query");
d.link("es", "kibana", "");

const res = d.validate();
console.log("3pillars:", JSON.stringify({ ok: res.ok, advice: res.audit.advice }));
writeFileSync(OUT, d.mxfile("Observability 3 pillars"));
