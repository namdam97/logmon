// LogMon — Logs pipeline (type "pipeline"): zerolog → OTel Collector (agent → gateway)
//   → Elasticsearch (data streams + ILM); Kafka buffer CHỈ ở Mode B (ADR-018).
// Dùng layout-engine + themed creators (pale per-stage tint, theme-aware). KHÔNG hardcode toạ độ.
import { writeFileSync } from "node:fs";
import { Diagram } from "../src/builder.mjs";
import { group, frame, icon, stage, band, endpoint, ossBox, renderTree } from "../src/layout-engine.mjs";
import { GRAFANA_ICON } from "./_grafana_icon.mjs";

const OUT = process.env.OUT ?? new URL("../out/logmon_logs_pipeline.drawio", import.meta.url);

const d = new Diagram("pipeline");
// Grafana has no AWS/lobe stencil → register the official brand logo so icon("grafana") resolves.
d.c.byName.set(GRAFANA_ICON.name, { ...GRAFANA_ICON, kind: "icon" });
d.c.validNames.add(GRAFANA_ICON.name);

// --- pipeline stages (left → right), spine = first icon of each stage (cùng hàng → thẳng) ---
const collect = stage("collect", 0, "1 · Collect (per-node)", [
  icon("otel_agent", "opentelemetry", "OTel Collector\n(agent · DaemonSet)"),
]);
const aggregate = stage("agg", 1, "2 · Aggregate / buffer", [
  icon("otel_gw", "opentelemetry", "OTel Collector\n(gateway · batch)"),
  icon("kafka", "kafka", "Kafka (KRaft)\nbuffer — Mode B"),
]);
const store = stage("store", 2, "3 · Store", [
  icon("es", "elasticsearch", "Elasticsearch\ndata streams + ILM"),
]);
const visualize = stage("viz", 3, "4 · Visualize", [
  icon("kibana", "kibana", "Kibana\n(ES-native)"),
  icon("grafana", "grafana", "Grafana 13.1\nExplore (logs)"),
]);

// --- cross-cutting band: logpipeline BC control-plane (no AWS icon → boxes) ---
const xcut = band("band", "logpipeline BC · cross-cutting (ADR-018)", [
  ossBox("mode", "Mode switch\nA: direct · B: Kafka"),
  ossBox("dlq", "DLQ + retry\n(reprocess)"),
  ossBox("ilm", "ILM policy\nhot→warm→cold→delete"),
  ossBox("mtls", "OTLP over mTLS\n(TLS ≥ 1.2)"),
]);

// Flat neutral platform frame (no AWS corner icon — this is a K8s platform, not an AWS account).
const cloud = frame("plat", "LogMon Observability Platform (K8s)", { dir: "col", gap: 36, fill: "#F7F9FB", stroke: "#9AA7B4" }, [
  frame("pipe", "", { dir: "row", gap: 50, align: "top", header: 0, fill: "none", stroke: "none" }, [collect, aggregate, store, visualize]),
  xcut,
]);

const tree = frame("root", "", { dir: "row", gap: 50, align: "center", header: 0, pad: 10, fill: "none", stroke: "none" }, [
  endpoint("src", "GO MICROSERVICES\n\nidentity · alerting\nslo · logpipeline\n\nzerolog → OTLP"),
  cloud,
  endpoint("cons", "SRE / DEVS\n\nsearch · dashboards\nlog correlation"),
]);

renderTree(d, tree, [40, 80]);
d.title("LogMon — Logs pipeline: zerolog → OTel Collector → Elasticsearch (ADR-018)");

// --- edges: solid = data flow (spine animated), dashed = Mode-B / lifecycle / policy ---
d.link("src", "otel_agent", "logs (OTLP/stdout)", { flow: true });
d.link("otel_agent", "otel_gw", "forward", { flow: true });
d.link("otel_gw", "es", "Mode A · direct", { flow: true });          // spine
d.link("otel_gw", "kafka", "Mode B", { dash: true });
d.link("kafka", "es", "consume", { dash: true });
d.link("es", "kibana", "query", { role: "fanout" });
d.link("es", "grafana", "query", { role: "fanout" });
d.link("kibana", "cons", "view");
d.link("grafana", "cons", "view");
d.link("es", "ilm", "lifecycle", { dash: true });                    // pipeline → band
d.link("kafka", "dlq", "on failure", { dash: true });

const res = d.validate();
console.log("VALIDATE:", JSON.stringify({ ok: res.ok, errors: res.errors, warnings: res.warnings, advice: res.audit.advice }, null, 2));
writeFileSync(OUT, d.mxfile("LogMon logs pipeline"));
console.log("WROTE:", OUT.pathname ?? OUT);
