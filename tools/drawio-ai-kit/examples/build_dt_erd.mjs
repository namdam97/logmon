// doc_tech — Database ERD (từ backend/migrations, nhóm theo BC)
import { writeFileSync } from "node:fs";
import { Diagram } from "../src/builder.mjs";
import { frame, ossBox, renderTree } from "../src/layout-engine.mjs";

const OUT = process.env.OUT ?? new URL("../out/db-erd.drawio", import.meta.url);
const d = new Diagram("mesh");
const tbl = (id, label, fill, stroke) => ossBox(id, label, { fill, stroke, w: 220, h: 132, bold: false, fs: 10 });

const identity = frame("idg", "identity (user→identity)", { dir: "col", gap: 20, fill: "#EAF2FB", stroke: "#3B6FB0" }, [
  tbl("users", "users\n──────\nid (PK, TEXT)\nemail (UNIQUE)\npassword_hash (argon2id)\ncreated_at", "#fff", "#3B6FB0"),
  tbl("rtok", "refresh_tokens\n──────\nid (PK)\nuser_id → users.id\nfamily_id · token_hash (UQ)\nused_at · expires_at", "#fff", "#3B6FB0"),
]);
const alerting = frame("alg", "alerting", { dir: "col", gap: 20, fill: "#F3EEF8", stroke: "#8C4FFF" }, [
  tbl("rules", "alert_rules\n──────\nid (PK) · workspace_id\nname · expression (PromQL)\nseverity · enabled\nsync_status", "#fff", "#8C4FFF"),
  tbl("inst", "alert_instances\n──────\nid (PK)\nrule_id → alert_rules.id (nullable)\nfingerprint · status\nfired/ack/resolved_at", "#fff", "#8C4FFF"),
]);
const slo = frame("slg", "slo", { dir: "col", gap: 20, fill: "#EAF3EC", stroke: "#5E9B57" }, [
  tbl("slos", "slos\n──────\nid (PK) · workspace_id\nname · service\nsli_type · window_days\nsync_status", "#fff", "#5E9B57"),
  tbl("snap", "slo_snapshots\n──────\nid (PK)\nslo_id → slos.id\nrecorded_at\n(error budget theo thời gian)", "#fff", "#5E9B57"),
]);
const shared = frame("shg", "shared kernel", { dir: "col", gap: 20, fill: "#FFF8E6", stroke: "#D6B656" }, [
  tbl("outbox", "outbox_events\n──────\nid (PK, identity)\naggregate_type/id · event_type\npayload (JSONB)\nstatus · retry_count", "#fff", "#D6B656"),
  ossBox("ws", "workspaces (planned)\nworkspace_id ở các bảng\n= khóa tenant\n(identity BC, GĐ3)", { fill: "#F2F2F2", stroke: "#999", w: 220, h: 80, fs: 10 }),
]);

const tree = frame("root", "", { dir: "row", gap: 50, align: "top", header: 0, pad: 12, fill: "none", stroke: "none" }, [identity, alerting, slo, shared]);
renderTree(d, tree, [40, 80]);
d.title("LogMon — Database ERD (backend/migrations; nhóm theo Bounded Context)");
d.link("rtok", "users", "user_id (FK, CASCADE)");
d.link("inst", "rules", "rule_id (FK, nullable)", { dash: true });
d.link("snap", "slos", "slo_id (FK, CASCADE)");

const res = d.validate();
console.log("erd:", JSON.stringify({ ok: res.ok, advice: res.audit.advice }));
writeFileSync(OUT, d.mxfile("DB ERD"));
