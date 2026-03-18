# LogMon AI Agent Team — Story-Driven Multi-Agent System

> **Dự án:** LogMon — Logging & Monitoring Platform
> **Inspired by:** [get-shit-done](https://github.com/gsd-build/get-shit-done), Luồng làm việc chuẩn BA/Dev/QC
> **Ngày cập nhật:** 2026-03-18
> **Status:** DRAFT — Chờ Boss NamDam confirm

---

## 1. Tổng Quan

### 1.1 Triết Lý

**Story là đơn vị trung tâm**, không phải Sprint. Mỗi story là một feature/sub-feature có:
- `story.md` do BA viết — **single source of truth**
- `tech/tech-spec.md` do Dev sinh ra từ story — bản cam kết kỹ thuật với BA
- `test/test-cases.md` do QC sinh ra từ story — **song song** với Dev, không cần chờ code
- `security/review.md` do DevSecOps sinh ra từ story + tech-spec

**5 role, mỗi role là một con người thật hoặc AI Agent** — có thể mix:

```
┌──────────────────────────────────────────────────────────────┐
│                     stories/ (Source of Truth)                 │
│                                                               │
│  stories/alerting/create-rule/                                │
│    ├── story.md          ← BA viết                            │
│    ├── tech/tech-spec.md ← Dev sinh từ story                  │
│    ├── tech/scaffold/    ← Dev sinh code scaffold              │
│    ├── test/test-cases.md← QC sinh từ story (SONG SONG Dev)   │
│    ├── test/scripts/     ← QC sinh automation scripts          │
│    └── security/review.md← DevSecOps audit                    │
└──────────────────────────────────────────────────────────────┘
         ▲          ▲           ▲           ▲
         │          │           │           │
    ┌────┴───┐ ┌────┴───┐ ┌────┴───┐ ┌─────┴─────┐
    │   BA   │ │DEV-BE  │ │   QC   │ │ DevSecOps │
    │        │ │DEV-FE  │ │        │ │           │
    │ /ba:*  │ │ /dev:* │ │ /qc:*  │ │ /sec:*    │
    └────────┘ └────────┘ └────────┘ └───────────┘
     Human      Human       Human       Human
     + AI       + AI        + AI        + AI
```

### 1.2 Nguyên Tắc

| # | Nguyên tắc | Mô tả |
|---|-----------|-------|
| 1 | **Story = Source of Truth** | Mọi artifacts (tech-spec, test-cases, security) sinh ra từ story.md |
| 2 | **QC song song với Dev** | QC gen test cases từ story, KHÔNG cần chờ code xong |
| 3 | **Versioned stories** | Story có version (v1.0.0 → v1.1.0). Update → auto-diff → sync impact |
| 4 | **Draft → Release** | BA draft ở `ba/`, review xong mới release sang `stories/` |
| 5 | **Tech Spec trước Code** | Dev gen tech-spec TRƯỚC khi code — cam kết kỹ thuật với BA |
| 6 | **Sync on change** | Story thay đổi → Dev sync tech-spec, QC sync test-cases |
| 7 | **Multi-user capable** | Mỗi role có thể là người thật + AI agent riêng trên cùng repo |

---

## 2. Luồng Làm Việc Chuẩn

### Giai Đoạn 1 — BA viết và release story

```bash
# Tạo story mới (feature / sub-feature)
/ba:new-story alerting
/ba:new-story alerting/create-rule

# Review trước khi release (AI check gaps, ambiguity)
/ba:review alerting/create-rule

# Release draft → stories/ (đây mới là bản Dev & QC dùng)
/ba:release alerting/create-rule v1.0.0
```

**Kết quả:** `stories/alerting/create-rule/story.md` — source of truth cho Dev và QC.

**BA draft directory:** `ba/alerting/create-rule/story.md` (workspace riêng, chưa publish)

### Giai Đoạn 2 — Dev generate từ story

```bash
# Story → Tech Spec (CHẠY TRƯỚC khi code)
/dev:gen-tech-spec alerting/create-rule

# Tech Spec → Code Scaffold
/dev:gen-scaffold alerting/create-rule

# Implement code (Dev tự code hoặc AI assist)
# ...

# Check implementation vs AC trong story
/dev:review alerting/create-rule
```

**Output:** `stories/alerting/create-rule/tech/tech-spec.md`

> **Quy tắc:** Chạy `/dev:gen-tech-spec` **trước** khi code. Tech spec là bản cam kết với BA — nếu AI generate sai hướng, điều chỉnh agent config trước khi viết code.

### Giai Đoạn 3 — QC generate từ story (SONG SONG với Giai Đoạn 2)

```bash
# Story → Test Cases + Coverage Matrix
/qc:gen-test-cases alerting/create-rule

# Test Cases → Automation Scripts
/qc:gen-scripts alerting/create-rule

# Viết bug report có cấu trúc theo Test Case ID
/qc:bug-report alerting/create-rule TC-003
```

**Output:** `stories/alerting/create-rule/test/test-cases.md`

> **Quy tắc:** QC chạy `/qc:gen-test-cases` **song song** với Dev — không cần chờ code xong mới viết test.

### Giai Đoạn 4 — DevSecOps review (SONG SONG với Giai Đoạn 2-3)

```bash
# Story + Tech Spec → Security Review
/sec:review alerting/create-rule

# Infra requirements từ story
/sec:gen-infra alerting/create-rule

# Audit code sau khi Dev implement xong
/sec:audit alerting/create-rule
```

**Output:** `stories/alerting/create-rule/security/review.md`

### Giai Đoạn 5 — BA update story (vòng lặp)

```bash
# BA chỉnh sửa ba/alerting/create-rule/story.md
# Sau đó release version mới:
/ba:release alerting/create-rule v1.1.0
# → Auto-diff vs v1.0.0, tóm tắt thay đổi, ước lượng impact lên Dev & QC
```

```bash
# Dev nhận thông báo, chạy:
/dev:sync alerting/create-rule
# → AI chỉ đúng phần tech-spec nào cần update

# QC nhận thông báo, chạy:
/qc:sync alerting/create-rule
# → AI chỉ test case nào bị invalid

# DevSecOps nhận thông báo, chạy:
/sec:sync alerting/create-rule
# → AI chỉ security review nào cần update
```

### Timeline Song Song

```
Thời gian ──────────────────────────────────────────────────▶

BA:       ██ new-story ██ review ██ release v1.0 ·········· ██ update ██ release v1.1
                                       │                          │
Dev-BE:                                ├── gen-tech-spec ── code ── review ·· sync ── fix
Dev-FE:                                ├── gen-tech-spec ── code ── review ·· sync ── fix
QC:                                    ├── gen-test-cases ── gen-scripts ··· sync ── update
DevSecOps:                             └── review ── gen-infra ············ sync ── audit
                                       │
                                  Song song từ đây
```

---

## 3. Story Directory Structure

### 3.1 Stories (Published — Source of Truth)

```
stories/
├── alerting/                                ← Bounded Context
│   ├── create-rule/                         ← Feature
│   │   ├── story.md                         ← BA output (versioned)
│   │   ├── tech/
│   │   │   ├── tech-spec.md                 ← Dev-BE tech spec
│   │   │   ├── tech-spec-fe.md              ← Dev-FE tech spec (nếu có UI)
│   │   │   └── scaffold/                    ← Generated code scaffold
│   │   ├── test/
│   │   │   ├── test-cases.md                ← QC test cases + coverage matrix
│   │   │   ├── scripts/                     ← QC automation scripts
│   │   │   └── bugs/                        ← QC bug reports (BUG-001.md, ...)
│   │   └── security/
│   │       ├── review.md                    ← DevSecOps security review
│   │       └── infra.md                     ← DevSecOps infra requirements
│   │
│   ├── evaluate-rule/                       ← Another feature
│   │   └── ...
│   ├── silence-alert/
│   │   └── ...
│   └── notify-slack/
│       └── ...
│
├── slo/
│   ├── define-slo/
│   ├── error-budget/
│   └── burn-rate-alert/
│
├── logpipeline/
│   ├── switch-mode/
│   ├── retry-dlq/
│   └── manage-ilm/
│
├── order/
│   ├── create-order/
│   └── cancel-order/
│
└── user/
    ├── register/
    └── login/
```

### 3.2 BA Draft Space

```
ba/                                          ← BA workspace (chưa publish)
├── alerting/
│   └── create-rule/
│       └── story.md                         ← Draft, đang soạn
└── ...
```

### 3.3 Versions (tự động khi release)

```
.versions/                                   ← Auto-generated khi /ba:release
├── alerting/
│   └── create-rule/
│       ├── v1.0.0/story.md                  ← Snapshot v1.0.0
│       └── v1.1.0/story.md                  ← Snapshot v1.1.0
└── ...
```

---

## 4. Slash Commands

### 4.1 BA Commands

| Command | Mô tả | Input | Output |
|---------|-------|-------|--------|
| `/ba:new-story <path>` | Tạo story mới từ template | BC + feature name | `ba/{path}/story.md` |
| `/ba:review <path>` | AI review gaps, ambiguity, missing AC | `ba/{path}/story.md` | Feedback inline hoặc suggestions |
| `/ba:release <path> <version>` | Promote draft → `stories/`, auto-diff nếu update | `ba/{path}/story.md` | `stories/{path}/story.md` + version snapshot |
| `/ba:impact <path>` | Ước lượng impact thay đổi lên Dev & QC | Diff giữa versions | Impact report |

### 4.2 Dev-BE Commands

| Command | Mô tả | Input | Output |
|---------|-------|-------|--------|
| `/dev:gen-tech-spec <path>` | Đọc story → sinh tech-spec.md | `stories/{path}/story.md` | `stories/{path}/tech/tech-spec.md` |
| `/dev:gen-scaffold <path>` | Đọc tech-spec → sinh code scaffold | `tech-spec.md` | `backend/internal/{bc}/**/*.go` |
| `/dev:review <path>` | Kiểm tra implementation vs AC trong story | story.md + source code | Pass/Fail report |
| `/dev:sync <path>` | Xem phần tech-spec nào cần update sau khi story thay đổi | story.md diff | Tech-spec update plan |

### 4.3 Dev-FE Commands

| Command | Mô tả | Input | Output |
|---------|-------|-------|--------|
| `/dev-fe:gen-tech-spec <path>` | Đọc story → sinh tech-spec-fe.md | `stories/{path}/story.md` | `stories/{path}/tech/tech-spec-fe.md` |
| `/dev-fe:gen-scaffold <path>` | Đọc tech-spec → sinh UI scaffold | `tech-spec-fe.md` | `frontend/**/*.tsx` |
| `/dev-fe:review <path>` | Kiểm tra UI vs AC | story.md + source code | Pass/Fail report |
| `/dev-fe:sync <path>` | Detect changes needed after story update | story.md diff | UI update plan |

### 4.4 QC Commands

| Command | Mô tả | Input | Output |
|---------|-------|-------|--------|
| `/qc:gen-test-cases <path>` | Đọc story → sinh test-cases.md + coverage matrix | `stories/{path}/story.md` | `stories/{path}/test/test-cases.md` |
| `/qc:gen-scripts <path>` | Đọc test cases → sinh automation scripts | `test-cases.md` | `stories/{path}/test/scripts/*.go` hoặc `*.ts` |
| `/qc:bug-report <path> <TC-id>` | Tạo bug report có cấu trúc theo TC | Test case + evidence | `stories/{path}/test/bugs/BUG-{N}.md` |
| `/qc:sync <path>` | Xem test case nào invalid sau khi story thay đổi | story.md diff | Invalid test case list |

### 4.5 DevSecOps Commands

| Command | Mô tả | Input | Output |
|---------|-------|-------|--------|
| `/sec:review <path>` | Security review từ story + tech-spec | story.md + tech-spec.md | `stories/{path}/security/review.md` |
| `/sec:gen-infra <path>` | Sinh infra requirements từ story | story.md | `stories/{path}/security/infra.md` |
| `/sec:audit <path>` | Audit source code (OWASP checklist) | Source code | Security audit report |
| `/sec:sync <path>` | Detect security review nào cần update | story.md diff | Security update plan |

### 4.6 Sprint Commands (Orchestration)

| Command | Mô tả | Khi nào dùng |
|---------|-------|-------------|
| `/sprint:init <N>` | Tạo sprint mới, chọn stories từ backlog | Đầu sprint |
| `/sprint:status` | Xem progress tất cả stories trong sprint | Bất kỳ lúc nào |
| `/sprint:review` | Tổng hợp kết quả từ tất cả stories | Cuối sprint |
| `/sprint:retro` | Retrospective — lessons learned | Cuối sprint |

---

## 5. Story Format

### 5.1 story.md (BA Output)

```markdown
---
id: US-ALT-001
title: Tạo Alert Rule mới
bounded_context: alerting
version: 1.0.0
priority: high
story_points: 5
sprint: 1
status: released
created: 2026-03-18
updated: 2026-03-18
---

## User Story
As a **SRE**, I want to **create a new alert rule with PromQL expression**,
so that **I get notified when a service exceeds error threshold**.

## Acceptance Criteria

### AC1: Tạo rule thành công
- **Given** valid PromQL expression và severity level
- **When** SRE submit form tạo alert rule
- **Then** rule được lưu vào database và evaluation bắt đầu trong 15s

### AC2: Validate PromQL
- **Given** invalid PromQL expression (syntax error)
- **When** SRE submit form
- **Then** trả về validation error với lý do cụ thể

### AC3: Notify khi alert fires
- **Given** rule severity = critical
- **When** alert fires (metric vượt threshold liên tục >= for duration)
- **Then** Slack notification gửi trong vòng 5 phút

### AC4: Duplicate name
- **Given** rule name đã tồn tại
- **When** SRE submit form
- **Then** trả về conflict error

## Business Rules
- PromQL phải validate syntax trước khi save
- Support 3 severity levels: critical, warning, info
- Mỗi rule phải có: name, expression, severity, for_duration
- Rule name unique trong cùng service

## UI Requirements (nếu có)
- Form tạo alert rule với fields: name, expression, severity, for_duration, service
- PromQL syntax highlighting (nice-to-have)
- Preview: dry-run evaluation trước khi save

## Out of Scope
- Alert evaluation engine (story riêng: alerting/evaluate-rule)
- Notification channels setup (story riêng: alerting/notify-slack)
```

### 5.2 tech-spec.md (Dev Output)

```markdown
---
story: US-ALT-001
story_version: 1.0.0
author: DEV-BE
created: 2026-03-18
---

## Architecture Decision
- Bounded Context: alerting (Clean Architecture + DDD + CQRS)
- AlertRule = Aggregate Root
- CreateRule = Command (write side)

## Implementation Plan

### Layer: domain/
| File | Purpose |
|------|---------|
| `alert_rule.go` | Aggregate Root: NewAlertRule() constructor with validation |
| `severity.go` | Value Object: Critical/Warning/Info enum |
| `errors.go` | ErrRuleInvalid, ErrRuleNameExists |

### Layer: app/command/
| File | Purpose |
|------|---------|
| `create_rule.go` | CreateRuleHandler: validate + save + emit event |

### Layer: ports/
| File | Purpose |
|------|---------|
| `repository.go` | AlertRuleRepository interface (Save, FindByName) |

### Layer: adapters/
| File | Purpose |
|------|---------|
| `postgres/repo.go` | PostgreSQL implementation |
| `http/handler.go` | POST /api/v1/alert-rules |

## API Contract
```
POST /api/v1/alert-rules
Content-Type: application/json

{
  "name": "high-error-rate",
  "expression": "rate(logmon_http_requests_total{status=~\"5..\"}[5m]) > 0.05",
  "severity": "critical",
  "for_duration": "2m",
  "service": "order-service"
}

→ 201 Created: { "id": "uuid", "name": "...", "status": "active" }
→ 400 Bad Request: { "error": "invalid PromQL expression: ..." }
→ 409 Conflict: { "error": "rule name already exists" }
```

## AC Mapping
| AC | Implementation |
|----|---------------|
| AC1 | CreateRuleHandler.Handle() → repo.Save() |
| AC2 | domain.NewAlertRule() validates PromQL syntax |
| AC3 | Out of scope (alerting/notify-slack story) |
| AC4 | repo.FindByName() check before save |
```

### 5.3 test-cases.md (QC Output)

```markdown
---
story: US-ALT-001
story_version: 1.0.0
author: QC
created: 2026-03-18
total_cases: 8
---

## Coverage Matrix

| AC | Test Cases | Coverage |
|----|-----------|----------|
| AC1 | TC-001, TC-002 | Full |
| AC2 | TC-003, TC-004, TC-005 | Full |
| AC3 | TC-006 | Deferred (depends on alerting/notify-slack) |
| AC4 | TC-007, TC-008 | Full |

## Test Cases

### TC-001: Create rule with valid data
- **Type:** Happy path
- **Precondition:** Database empty
- **Input:** name="high-error-rate", expression="rate(...) > 0.05", severity="critical", for="2m"
- **Expected:** 201 Created, rule appears in GET /api/v1/alert-rules
- **Priority:** Critical

### TC-002: Create rule — evaluation starts within 15s
- **Type:** Timing
- **Precondition:** Rule created successfully
- **Input:** Metric matching expression pushed to Prometheus
- **Expected:** Alert status changes to "firing" within 15s
- **Priority:** High

### TC-003: Invalid PromQL — syntax error
- **Type:** Negative
- **Input:** expression="rate(invalid{{"
- **Expected:** 400, error message contains "syntax"
- **Priority:** Critical

### TC-004: Invalid PromQL — empty expression
- **Type:** Boundary
- **Input:** expression=""
- **Expected:** 400, error message contains "required"
- **Priority:** High

### TC-005: Invalid severity value
- **Type:** Negative
- **Input:** severity="urgent" (not in enum)
- **Expected:** 400, error message contains "oneof"
- **Priority:** Medium

### TC-006: Slack notification on critical alert
- **Type:** Integration (DEFERRED)
- **Depends on:** alerting/notify-slack story
- **Priority:** Critical

### TC-007: Duplicate rule name — exact match
- **Type:** Negative
- **Precondition:** Rule "high-error-rate" exists
- **Input:** name="high-error-rate" (same)
- **Expected:** 409 Conflict
- **Priority:** High

### TC-008: Duplicate rule name — case sensitivity
- **Type:** Edge case
- **Precondition:** Rule "High-Error-Rate" exists
- **Input:** name="high-error-rate" (different case)
- **Expected:** 409 Conflict (case-insensitive match)
- **Priority:** Medium
```

---

## 6. Năm AI Agents

### 6.1 BA (Business Analyst)

| Thuộc tính | Giá trị |
|-----------|---------|
| **Model** | Opus (reasoning sâu cho domain analysis) |
| **Tools** | Read, Write, Glob, Grep, WebSearch, WebFetch |
| **KHÔNG có** | Edit source code, Bash |
| **Context** | `doc/logmon.md` Sections 1-6, `CLAUDE.md` overview, existing stories |

**Trách nhiệm:**
- Viết stories với AC (Given/When/Then)
- Review stories trước release (gaps, ambiguity)
- Versioning: release v1.0.0, v1.1.0, ...
- Impact analysis khi update story

### 6.2 DEV-BE (Backend Developer)

| Thuộc tính | Giá trị |
|-----------|---------|
| **Model** | Sonnet (balanced cho code generation) |
| **Tools** | Read, Write, Edit, Bash, Glob, Grep |
| **KHÔNG có** | WebSearch, WebFetch |
| **Context** | `CLAUDE.md` full, `doc/logmon.md` Sections 7-9, story + tech-spec |

**Trách nhiệm:**
- Gen tech-spec từ story (architecture mapping to DDD layers)
- Gen code scaffold theo Clean Architecture
- Implement business logic
- Unit tests + integration tests
- Commit format: `feat({bc}/US-{id}): {description}`

**DDD/CQRS-specific rules:**
- Tech-spec PHẢI map AC → domain layer (aggregate, value object, event)
- PHẢI specify: domain/ → ports/ → adapters/ file breakdown
- CQRS: tách command (write) và query (read) trong tech-spec
- Domain events PHẢI list nếu có cross-BC impact

### 6.3 DEV-FE (Frontend Developer)

| Thuộc tính | Giá trị |
|-----------|---------|
| **Model** | Sonnet |
| **Tools** | Read, Write, Edit, Bash, Glob, Grep |
| **KHÔNG có** | WebSearch, WebFetch |
| **Context** | `CLAUDE.md`, story + tech-spec-fe, existing components |

**Trách nhiệm:**
- Gen tech-spec-fe (component tree, pages, API client)
- Gen UI scaffold (Next.js pages, shadcn/ui components)
- Implement responsive UI
- TypeScript strict, no `any`

### 6.4 QC (Quality Assurance)

| Thuộc tính | Giá trị |
|-----------|---------|
| **Model** | Opus (reasoning sâu cho test design) |
| **Tools** | Read, Write, Bash, Glob, Grep |
| **KHÔNG có** | Edit source code (chỉ viết test files + reports) |
| **Context** | Story (AC), tech-spec, source code, `doc/logmon.md` Section 9 |

**Trách nhiệm:**
- Gen test cases từ story (SONG SONG với Dev, không chờ code)
- Gen automation scripts (Go tests, E2E tests)
- Bug reports có cấu trúc (theo TC-id)
- Sync test cases khi story thay đổi

**Test case rules:**
- Mỗi AC phải có ≥ 1 test case
- Coverage matrix: AC → Test Cases mapping
- Test types: happy path, negative, boundary, edge case, integration
- Bug report PHẢI có: TC-id, severity, steps to reproduce, expected vs actual, file:line

### 6.5 DevSecOps

| Thuộc tính | Giá trị |
|-----------|---------|
| **Model** | Sonnet |
| **Tools** | Read, Write, Edit, Bash, Glob, Grep |
| **KHÔNG có** | WebSearch, WebFetch |
| **Context** | `doc/logmon.md` Sections 9.7 + 13, story + tech-spec, infra configs |

**Trách nhiệm:**
- Security review từ story + tech-spec (OWASP checklist)
- Gen infra requirements (Docker, CI/CD needs per story)
- Audit source code sau khi Dev implement
- Sync security review khi story thay đổi

---

## 7. Multi-User Parallel Mode

### 7.1 Cách Hoạt Động

Mỗi team member mở Claude Code terminal riêng, trên **cùng repo** (Git branching):

```
Terminal 1 — BA (người thật):
  $ claude
  > /ba:new-story alerting/create-rule
  > /ba:review alerting/create-rule
  > /ba:release alerting/create-rule v1.0.0
  > git add stories/ && git commit && git push

Terminal 2 — Dev-BE (người thật):
  $ claude
  > git pull  # lấy story mới từ BA
  > /dev:gen-tech-spec alerting/create-rule
  > /dev:gen-scaffold alerting/create-rule
  > # ... code implementation ...
  > /dev:review alerting/create-rule
  > git add . && git commit && git push

Terminal 3 — QC (người thật):        ← SONG SONG với Terminal 2
  $ claude
  > git pull  # lấy story mới từ BA
  > /qc:gen-test-cases alerting/create-rule
  > /qc:gen-scripts alerting/create-rule
  > git add stories/alerting/create-rule/test/ && git commit && git push

Terminal 4 — DevSecOps (người thật):  ← SONG SONG với Terminal 2-3
  $ claude
  > git pull
  > /sec:review alerting/create-rule
  > /sec:gen-infra alerting/create-rule
  > git add . && git commit && git push
```

### 7.2 Git Branching Strategy

```
main
 └── sprint-1
      ├── feature/alerting-create-rule        ← Dev-BE works here
      ├── feature/alerting-create-rule-fe     ← Dev-FE works here
      ├── test/alerting-create-rule           ← QC works here
      └── infra/alerting-create-rule          ← DevSecOps works here

Merge order:
1. stories/ (BA) → main (hoặc sprint branch)
2. test/ (QC) + security/ (DevSecOps) → sprint branch
3. backend/ + frontend/ (Dev) → sprint branch (PR with QC review)
4. sprint branch → main (sprint review done)
```

### 7.3 Single-User Mode

Một người điều khiển tất cả agents cũng hoạt động:

```bash
# Đội 1 người, chạy lần lượt
/ba:new-story alerting/create-rule
/ba:release alerting/create-rule v1.0.0
/dev:gen-tech-spec alerting/create-rule      # rồi implement
/qc:gen-test-cases alerting/create-rule      # rồi gen scripts
/sec:review alerting/create-rule
/dev:review alerting/create-rule             # self-check
```

---

## 8. Sync Workflow (Khi Story Thay Đổi)

```
BA update story v1.0.0 → v1.1.0
         │
         ├──▶ /ba:release alerting/create-rule v1.1.0
         │         │
         │         ├── Auto-diff: so sánh v1.0.0 vs v1.1.0
         │         ├── Change summary: "Added AC5, modified AC2 threshold"
         │         └── Impact estimate: "Dev: 2 files affected, QC: 3 test cases affected"
         │
         ├──▶ Dev chạy /dev:sync alerting/create-rule
         │         └── AI output: "tech-spec.md cần update:
         │                          - Section 'API Contract': thêm field 'labels'
         │                          - Section 'AC Mapping': thêm AC5 mapping"
         │
         ├──▶ QC chạy /qc:sync alerting/create-rule
         │         └── AI output: "test-cases.md impact:
         │                          - TC-002: INVALID (AC2 threshold changed)
         │                          - NEW: cần test case cho AC5
         │                          - TC-001, TC-003-008: unchanged"
         │
         └──▶ DevSecOps chạy /sec:sync alerting/create-rule
                   └── AI output: "security/review.md:
                                    - 'labels' field cần input validation review"
```

---

## 9. Sprint Integration

Sprints là cách **nhóm stories** để tracking tiến độ, không phải driver chính.

### 9.1 Sprint Config

```
.agile/
├── sprints/
│   ├── sprint-1.md                          ← Sprint metadata + story list
│   ├── sprint-2.md
│   └── ...
├── backlog.md                               ← Unprioritized stories
└── config.json                              ← Settings
```

### 9.2 sprint-1.md

```markdown
---
sprint: 1
goal: "Implement Alerting BC core + basic infrastructure"
start: 2026-03-18
end: 2026-04-01
status: in_progress
---

## Stories

| Story | BC | SP | BA | Dev | QC | Sec | Status |
|-------|----|----|-----|-----|-----|------|--------|
| alerting/create-rule | alerting | 5 | done | in_progress | done | done | implementing |
| alerting/list-rules | alerting | 3 | done | done | in_progress | pending | testing |
| alerting/evaluate-rule | alerting | 8 | in_progress | pending | pending | pending | drafting |
| infra/docker-compose | shared | 3 | done | n/a | n/a | done | done |
| infra/db-migrations | shared | 2 | done | n/a | n/a | done | done |

## Velocity
- Total SP: 21
- Completed: 5
- In Progress: 8
- Remaining: 8
```

### 9.3 LogMon Sprint Roadmap

| Sprint | Focus | Key Stories |
|--------|-------|-------------|
| **1** | Foundation + Alerting Core | `infra/docker-compose`, `infra/db-migrations`, `alerting/create-rule`, `alerting/list-rules`, `alerting/evaluate-rule` |
| **2** | Alerting Notifications + SLO | `alerting/notify-slack`, `alerting/notify-email`, `alerting/silence`, `slo/define-slo`, `slo/error-budget` |
| **3** | Log Pipeline + CRUD Services | `logpipeline/switch-mode`, `logpipeline/retry-dlq`, `order/create-order`, `order/list-orders`, `user/register`, `user/login` |
| **4** | Frontend Dashboard | `frontend/alert-management`, `frontend/slo-dashboard`, `frontend/log-viewer`, `frontend/service-overview` |
| **5** | Monitoring Stack Integration | `infra/prometheus-config`, `infra/elk-pipeline`, `infra/grafana-dashboards`, `infra/kafka-setup` |
| **6** | CI/CD + Production | `infra/github-actions`, `infra/nginx-ssl`, `security/penetration-test`, `security/hardening` |

---

## 10. Agent Configuration

### 10.1 config.json

```json
{
  "project": "logmon",
  "sprint_length_weeks": 2,
  "models": {
    "ba": "opus",
    "dev_be": "sonnet",
    "dev_fe": "sonnet",
    "qc": "opus",
    "devsecops": "sonnet"
  },
  "story_path": "stories",
  "draft_path": "ba",
  "versions_path": ".versions",
  "bounded_contexts": [
    "alerting", "slo", "logpipeline", "order", "user", "shared"
  ],
  "architecture": {
    "alerting": "clean-arch-ddd-cqrs",
    "slo": "clean-arch-ddd-cqrs",
    "logpipeline": "clean-arch-ddd-cqrs",
    "order": "clean-arch",
    "user": "clean-arch"
  },
  "tech_spec_required_before_code": true,
  "qc_parallel_with_dev": true,
  "auto_version_on_release": true
}
```

### 10.2 Agent Context Injection

Mỗi agent khi spawn được inject context phù hợp với role:

| Agent | CLAUDE.md | logmon.md Sections | Story artifacts |
|-------|-----------|-------------------|-----------------|
| BA | Overview, Architecture Rules | 1-6 (system overview) | Existing stories, backlog |
| DEV-BE | Full (arch + style + security) | 7-9 (structure + backend + rules) | story.md, tech-spec.md |
| DEV-FE | Overview + frontend-relevant | 7, 9.1, 9.8 | story.md, tech-spec-fe.md |
| QC | Architecture Rules, Testing | 9.2, 9.8 (error handling + testing) | story.md, test-cases.md |
| DevSecOps | Security section | 9.7, 9.9, 13 (security + infra + deploy) | story.md, tech-spec.md, security/review.md |

---

## 11. Quick Start

```bash
# 1. Tạo directories
mkdir -p ba stories .versions .agile/sprints

# 2. BA tạo story đầu tiên
/ba:new-story alerting/create-rule
# → ba/alerting/create-rule/story.md (draft)

# 3. BA review và release
/ba:review alerting/create-rule
/ba:release alerting/create-rule v1.0.0
# → stories/alerting/create-rule/story.md (published)

# 4. Dev và QC chạy SONG SONG
/dev:gen-tech-spec alerting/create-rule    # Dev terminal
/qc:gen-test-cases alerting/create-rule    # QC terminal (parallel!)

# 5. Dev implement
/dev:gen-scaffold alerting/create-rule
# ... code ...
/dev:review alerting/create-rule

# 6. DevSecOps review
/sec:review alerting/create-rule

# 7. QC gen scripts và test
/qc:gen-scripts alerting/create-rule

# 8. Nếu BA update story:
/ba:release alerting/create-rule v1.1.0
/dev:sync alerting/create-rule             # Dev chạy
/qc:sync alerting/create-rule              # QC chạy
/sec:sync alerting/create-rule             # DevSecOps chạy
```

---

*Tài liệu thiết kế dựa trên [get-shit-done](https://github.com/gsd-build/get-shit-done) architecture + Luồng làm việc chuẩn BA/Dev/QC, adapted cho 5-role team (BA, DEV-BE, DEV-FE, QC, DevSecOps) trên dự án LogMon.*
