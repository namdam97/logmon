// doc_tech — Kubernetes architecture: control plane + worker nodes + ingress/service/pods
import { writeFileSync } from "node:fs";
import { Diagram } from "../src/builder.mjs";
import { frame, icon, endpoint, ossBox, renderTree } from "../src/layout-engine.mjs";

const OUT = process.env.OUT ?? new URL("../out/kubernetes-architecture.drawio", import.meta.url);
const d = new Diagram("hubspoke");

const comp = (id, label) => ossBox(id, label, { fill: "#EAF2FB", stroke: "#326CE5", w: 150, h: 56, bold: true, fs: 10 });
const pod  = (id, label) => ossBox(id, label, { fill: "#E8F0FE", stroke: "#5B8DEF", w: 150, h: 44, fs: 10 });

// Control plane (master)
const cp = frame("cp", "Control Plane (master)", { dir: "row", gap: 22, fill: "#EEF3FF", stroke: "#326CE5" }, [
  comp("api", "kube-apiserver\n(REST, mọi thứ đi qua đây)"),
  icon("etcd", "etcd", "etcd\n(key-value state)"),
  comp("sched", "kube-scheduler\n(gán Pod → Node)"),
  comp("cm", "controller-manager\n(reconcile loops)"),
]);

// Worker nodes
const node = (id, n) => frame(id, `Worker Node ${n}`, { dir: "col", gap: 12, fill: "#F5F8FF", stroke: "#5B8DEF" }, [
  comp(`${id}_kubelet`, "kubelet\n(chạy & báo cáo Pod)"),
  comp(`${id}_proxy`, "kube-proxy\n(network/Service)"),
  icon(`${id}_rt`, "containerd", "containerd (runtime)"),
  pod(`${id}_pods`, "Pods\n(logmon containers)"),
]);
const nodes = frame("nodes", "Data Plane — Worker Nodes", { dir: "row", gap: 40, fill: "#FAFBFC", stroke: "#9AA7B4" },
  [node("n1", 1), node("n2", 2)]);

const cluster = frame("cluster", "Kubernetes Cluster", { dir: "col", gap: 40, align: "center", fill: "#FCFDFF", stroke: "#326CE5" }, [cp, nodes]);

const tree = frame("root", "", { dir: "row", gap: 56, align: "center", header: 0, pad: 12, fill: "none", stroke: "none" }, [
  frame("left", "", { dir: "col", gap: 30, header: 0, fill: "none", stroke: "none" }, [
    endpoint("user", "Users / Dev\nkubectl · CI/CD\n(qua API server)"),
    icon("ingress", "nginx", "Ingress\n(L7 vào cluster)"),
  ]),
  cluster,
]);

renderTree(d, tree, [40, 80]);
d.title("Kubernetes — Architecture, Components & Concepts (control plane vs data plane)");
d.link("user", "api", "kubectl / API (HTTPS)", { flow: true });
d.link("api", "etcd", "đọc/ghi state");
d.link("api", "n1", "watch · gán Pod");
d.link("api", "n2", "watch · gán Pod");
d.link("ingress", "n1", "Ingress→Service→Pod");

const res = d.validate();
console.log("k8s:", JSON.stringify({ ok: res.ok, advice: res.audit.advice }));
writeFileSync(OUT, d.mxfile("Kubernetes architecture"));
