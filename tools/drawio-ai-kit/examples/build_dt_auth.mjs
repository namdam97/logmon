// doc_tech — Auth flow: argon2id + JWT HttpOnly + refresh rotation (reuse detect) + CSRF
import { writeFileSync } from "node:fs";
import { Diagram } from "../src/builder.mjs";
import { frame, stage, endpoint, ossBox, renderTree } from "../src/layout-engine.mjs";

const OUT = process.env.OUT ?? new URL("../out/auth-flow.drawio", import.meta.url);
const d = new Diagram("pipeline");
const b = (id, label, fill, stroke) => ossBox(id, label, { fill, stroke, w: 184, h: 70, bold: true, fs: 10 });

const login = stage("l", 0, "1 · Đăng nhập", [
  b("verify", "POST /login\nverify argon2id\n(crypto/subtle so sánh)", "#FDEEE0", "#ED7100"),
]);
const issue = stage("i", 1, "2 · Cấp token", [
  b("jwt", "Access JWT\nHttpOnly+Secure+SameSite\ncookie (ngắn hạn)", "#EAF2FB", "#3B6FB0"),
  b("refresh", "Refresh token\nfamily_id + token_hash\n(rotation)", "#EAF2FB", "#3B6FB0"),
  b("csrf", "CSRF token\nsigned double-submit\n(lm_csrf + X-CSRF-Token)", "#F3EEF8", "#8C4FFF"),
]);
const use = stage("u", 2, "3 · Request được bảo vệ", [
  b("mw", "Middleware verify\nJWT cookie + CSRF header\n→ context user_id", "#EAF3EC", "#5E9B57"),
]);
const rot = stage("r", 3, "4 · Làm mới (rotation)", [
  b("refresh2", "POST /refresh\nrotate: used_at, family\nphát hiện reuse → thu hồi cả family", "#FDECEC", "#DD344C"),
]);

const tree = frame("root", "", { dir: "row", gap: 48, align: "center", header: 0, pad: 12, fill: "none", stroke: "none" }, [
  endpoint("client", "Client\n(Next.js / API)\n\nemail + password"),
  frame("core", "Auth (shared/auth · user BC) — ADR-022/023", { dir: "row", gap: 40, fill: "#FAFBFC", stroke: "#9AA7B4" }, [login, issue, use, rot]),
]);
renderTree(d, tree, [40, 80]);
d.title("Auth flow — argon2id + JWT HttpOnly + refresh rotation (reuse detection) + CSRF double-submit");
d.link("client", "verify", "credentials", { flow: true });
d.link("verify", "jwt", "OK → cấp", { flow: true });
d.link("verify", "refresh", "", { role: "fanout" });
d.link("verify", "csrf", "", { role: "fanout" });
d.link("jwt", "mw", "cookie kèm mỗi request", { flow: true });
d.link("mw", "refresh2", "401 → refresh", { dash: true });
d.link("refresh2", "jwt", "token mới", { dash: true });

const res = d.validate();
console.log("auth:", JSON.stringify({ ok: res.ok, advice: res.audit.advice }));
writeFileSync(OUT, d.mxfile("Auth flow"));
