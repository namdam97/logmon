// doc_tech — CI/CD + supply-chain security gates
import { writeFileSync } from "node:fs";
import { Diagram } from "../src/builder.mjs";
import { frame, stage, icon, endpoint, ossBox, renderTree } from "../src/layout-engine.mjs";

const OUT = process.env.OUT ?? new URL("../out/cicd-supply-chain.drawio", import.meta.url);
const d = new Diagram("pipeline");

const src = stage("s", 0, "1 · Source", [
  icon("gh", "github", "GitHub\n(PR / push)"),
]);
const ci = stage("c", 1, "2 · CI (GitHub Actions)", [
  icon("gha", "githubactions", "Actions runner"),
  ossBox("build", "golangci-lint\ngo test -race (≥80%)\ngo build · pnpm build", { fill: "#EAF2FB", stroke: "#3B6FB0", w: 188, h: 64 }),
]);
const sec = stage("g", 2, "3 · Security gates (fail = block)", [
  icon("trivy", "trivy", "Trivy\n(image/IaC CVE)"),
  ossBox("gov", "govulncheck\n(Go CVE)", { fill: "#FDECEC", stroke: "#DD344C", w: 160, h: 48 }),
  ossBox("leaks", "gitleaks\n(secret scan)", { fill: "#FDECEC", stroke: "#DD344C", w: 160, h: 48 }),
  ossBox("sbom", "SBOM (syft)\n+ sign (cosign)", { fill: "#FDECEC", stroke: "#DD344C", w: 160, h: 48 }),
]);
const art = stage("a", 3, "4 · Artifact", [
  icon("reg", "docker", "Registry\n(signed image)"),
]);
const dep = stage("d", 4, "5 · Deploy", [
  icon("argo", "argocd", "Argo CD\n(GitOps)"),
  icon("k8s", "kubernetes", "Kubernetes"),
]);

const tree = frame("root", "", { dir: "row", gap: 44, align: "center", header: 0, pad: 12, fill: "none", stroke: "none" }, [
  endpoint("dev", "Developer\n\ncommit theo\nConventional Commits"),
  frame("pipe", "Pipeline (ADR-044: govulncheck · gitleaks · Trivy)", { dir: "row", gap: 40, fill: "#FAFBFC", stroke: "#9AA7B4" }, [src, ci, sec, art, dep]),
]);

renderTree(d, tree, [40, 80]);
d.title("DevSecOps — CI/CD pipeline + supply-chain security gates");
d.link("dev", "gh", "git push", { flow: true });
d.link("gh", "gha", "trigger", { flow: true });
d.link("gha", "trivy", "scan", { flow: true });
d.link("trivy", "reg", "push nếu PASS", { flow: true });
d.link("reg", "argo", "image update", { flow: true });

const res = d.validate();
console.log("cicd:", JSON.stringify({ ok: res.ok, advice: res.audit.advice }));
writeFileSync(OUT, d.mxfile("CICD supply chain"));
