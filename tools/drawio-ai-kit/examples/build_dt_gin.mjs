// doc_tech — Gin request lifecycle: middleware chain → handler → use case → repo → response
import { writeFileSync } from "node:fs";
import { Diagram } from "../src/builder.mjs";
import { frame, stage, endpoint, ossBox, renderTree } from "../src/layout-engine.mjs";

const OUT = process.env.OUT ?? new URL("../out/gin-request-lifecycle.drawio", import.meta.url);
const d = new Diagram("pipeline");
const b = (id, label, fill, stroke) => ossBox(id, label, { fill, stroke, w: 176, h: 56, fs: 10, bold: true });

const mw = stage("m", 0, "Middleware chain (thứ tự quan trọng)", [
  b("rec", "Recovery (panic→500)", "#FDECEC", "#DD344C"),
  b("log", "RequestID + Logger (zerolog)", "#EEF1F5", "#5A6B7B"),
  b("otel", "OTel (otelgin) — span", "#EAF3EC", "#5E9B57"),
  b("rl", "RateLimit (x/time/rate, in-mem)", "#FDEEE0", "#ED7100"),
  b("auth", "Auth (JWT cookie) + CSRF", "#EAF2FB", "#3B6FB0"),
]);
const handle = stage("h", 1, "Handler (adapters/http)", [
  b("bind", "bind + validate\n(validator/v10)", "#F3EEF8", "#8C4FFF"),
]);
const app = stage("a", 2, "App use case", [
  b("uc", "use case\n(business logic)", "#EAF2FB", "#3B6FB0"),
]);
const data = stage("d", 3, "Adapters (out)", [
  b("repo", "Repository (pgx)\n/ Cache (Redis)", "#EEF1F5", "#5A6B7B"),
]);

const tree = frame("root", "", { dir: "row", gap: 44, align: "center", header: 0, pad: 12, fill: "none", stroke: "none" }, [
  endpoint("req", "HTTP request\n(client)"),
  frame("core", "Gin engine — vòng đời 1 request (shared/httpx · middleware · adapters)", { dir: "row", gap: 36, fill: "#FAFBFC", stroke: "#9AA7B4" }, [mw, handle, app, data]),
  endpoint("resp", "Response envelope\n{success,data,error}\n(JSON nhất quán)"),
]);
renderTree(d, tree, [40, 80]);
d.title("Vòng đời 1 HTTP request trong Gin — middleware → handler → use case → repo → response");
d.link("req", "rec", "", { flow: true });
d.link("auth", "bind", "next()", { flow: true });
d.link("bind", "uc", "gọi", { flow: true });
d.link("uc", "repo", "qua port", { flow: true });
d.link("repo", "resp", "kết quả → render", { flow: true });

const res = d.validate();
console.log("gin:", JSON.stringify({ ok: res.ok, advice: res.audit.advice }));
writeFileSync(OUT, d.mxfile("Gin request lifecycle"));
