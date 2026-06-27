// doc_tech — Traces: OpenTelemetry SDK → Collector (tail sampling) → Jaeger v2 (ES storage)
import { writeFileSync } from "node:fs";
import { Diagram } from "../src/builder.mjs";
import { frame, stage, icon, endpoint, ossBox, renderTree } from "../src/layout-engine.mjs";
import { GRAFANA_ICON } from "./_grafana_icon.mjs";

const OUT = process.env.OUT ?? new URL("../out/traces-otel-jaeger.drawio", import.meta.url);
const d = new Diagram("pipeline");
d.c.byName.set(GRAFANA_ICON.name, { ...GRAFANA_ICON, kind: "icon" });
d.c.validNames.add(GRAFANA_ICON.name);

const inst = stage("i", 0, "1 · Instrument (in-process)", [
  ossBox("sdk", "OTel SDK (Go)\notelgin (HTTP)\notelpgx (DB)\ncontext propagation", { fill: "#FFF8E6", stroke: "#D6B656", w: 196, h: 84 }),
]);
const collect = stage("c", 1, "2 · Collect", [
  icon("otel", "opentelemetry", "OTel Collector\nagent → gateway\ntail sampling"),
]);
const storeS = stage("st", 2, "3 · Store", [
  icon("jaeger", "jaeger", "Jaeger v2"),
  icon("es", "elasticsearch", "Elasticsearch\n(span storage)"),
]);
const view = stage("v", 3, "4 · Explore", [
  ossBox("jui", "Jaeger UI\n(trace timeline)", { fill: "#EEF1F5", stroke: "#5A6B7B", w: 168, h: 56 }),
  icon("grafana", "grafana", "Grafana\n(trace↔log↔metric)"),
]);

const tree = frame("root", "", { dir: "row", gap: 56, align: "center", header: 0, pad: 12, fill: "none", stroke: "none" }, [
  endpoint("app", "GO MICROSERVICES\n\nrequest tạo 1 trace\n= nhiều span\n(trace_id lan truyền\nqua mọi service)"),
  frame("core", "Tracing pipeline (OTLP/gRPC)", { dir: "row", gap: 46, fill: "#FAFBFC", stroke: "#9AA7B4" }, [inst, collect, storeS, view]),
]);

renderTree(d, tree, [40, 80]);
d.title("Distributed Tracing — OpenTelemetry → Collector (tail sampling) → Jaeger v2");
d.link("app", "sdk", "start/end span", { flow: true });
d.link("sdk", "otel", "OTLP export", { flow: true });
d.link("otel", "jaeger", "sampled traces", { flow: true });
d.link("jaeger", "es", "persist");
d.link("jaeger", "jui", "query");
d.link("jaeger", "grafana", "datasource");

const res = d.validate();
console.log("traces:", JSON.stringify({ ok: res.ok, advice: res.audit.advice }));
writeFileSync(OUT, d.mxfile("Traces OTel Jaeger"));
