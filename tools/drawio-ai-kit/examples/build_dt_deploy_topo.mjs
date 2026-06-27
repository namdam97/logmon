// doc_tech — Deploy topology: LogMon trên Kubernetes (namespaces, ingress, svc, data, observability)
import { writeFileSync } from "node:fs";
import { Diagram } from "../src/builder.mjs";
import { frame, icon, endpoint, ossBox, renderTree } from "../src/layout-engine.mjs";
import { GRAFANA_ICON } from "./_grafana_icon.mjs";

const OUT = process.env.OUT ?? new URL("../out/deploy-topology.drawio", import.meta.url);
const d = new Diagram("hubspoke");
d.c.byName.set(GRAFANA_ICON.name, { ...GRAFANA_ICON, kind: "icon" });
d.c.validNames.add(GRAFANA_ICON.name);
const k = (id, label) => ossBox(id, label, { fill: "#EAF2FB", stroke: "#326CE5", w: 152, h: 50, bold: true, fs: 10 });

const appns = frame("app", "namespace: app", { dir: "col", gap: 12, fill: "#EEF3FF", stroke: "#326CE5" }, [
  k("svc", "Service (ClusterIP)\nlogmon-api"),
  k("deploy", "Deployment\nuserservice pods (×N)\nliveness/readiness probe"),
  k("hpa", "HPA (autoscale theo CPU/RPS)"),
  ossBox("cfg", "ConfigMap + Secret\n(env, JWT/CSRF key)", { fill: "#FFF8E6", stroke: "#D6B656", w: 152, h: 48, fs: 10 }),
]);
const datans = frame("data", "namespace: data", { dir: "col", gap: 14, fill: "#F5F8FF", stroke: "#5B8DEF" }, [
  icon("pg", "postgres", "PostgreSQL\n(StatefulSet / managed)"),
  icon("redis", "redis", "Redis"),
]);
const obsns = frame("obs", "namespace: observability", { dir: "col", gap: 12, fill: "#F3EEF8", stroke: "#8C4FFF" }, [
  icon("prom", "prometheus", "Prometheus"),
  icon("otel", "opentelemetry", "OTel Collector"),
  icon("es", "elasticsearch", "Elasticsearch"),
  icon("jaeger", "jaeger", "Jaeger"),
  icon("grafana", "grafana", "Grafana"),
]);
const cluster = frame("cluster", "Kubernetes Cluster", { dir: "row", gap: 40, align: "top", fill: "#FCFDFF", stroke: "#326CE5" }, [appns, datans, obsns]);

const tree = frame("root", "", { dir: "row", gap: 50, align: "center", header: 0, pad: 12, fill: "none", stroke: "none" }, [
  frame("left", "", { dir: "col", gap: 28, header: 0, fill: "none", stroke: "none" }, [
    endpoint("user", "Users / SRE\nHTTPS"),
    icon("ingress", "nginx", "Ingress\n(TLS, routing)"),
  ]),
  cluster,
]);
renderTree(d, tree, [40, 80]);
d.title("Triển khai LogMon trên Kubernetes — namespaces, Ingress→Service→Deployment, data & observability (planned)");
d.link("user", "ingress", "HTTPS", { flow: true });
d.link("ingress", "svc", "route /api", { flow: true });
d.link("svc", "deploy", "");
d.link("deploy", "pg", "pgx", { dir: "LR" });
d.link("deploy", "otel", "OTLP", { dir: "LR" });
d.link("prom", "deploy", "scrape /metrics", { dash: true });

const res = d.validate();
console.log("deploy:", JSON.stringify({ ok: res.ok, advice: res.audit.advice }));
writeFileSync(OUT, d.mxfile("Deploy topology"));
