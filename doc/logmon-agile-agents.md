# LogMon AI Agent Team — Story-Driven Multi-Agent System

> **Dự án:** LogMon — Logging & Monitoring Platform
> **Inspired by:** [get-shit-done](https://github.com/gsd-build/get-shit-done), Luồng làm việc chuẩn BA/Dev/QC
> **Ngày cập nhật:** 2026-03-19
> **Status:** CONFIRMED

---

## 1. Tổng Quan

### 1.1 Triết Lý

**Story là đơn vị trung tâm**, không phải Sprint. Mỗi story là một feature/sub-feature có:
- `story.md` do BA viết — **single source of truth**
- `tech/tech-spec.md` do Dev-BE sinh ra từ story — bản cam kết kỹ thuật với BA
- `tech/openapi.yaml` do Dev-BE sinh — **API contract chung** giữa BE/FE/QC
- `tech/tech-spec-fe.md` do Dev-FE sinh — tham chiếu openapi.yaml
- `test/test-cases.md` do QC sinh ra từ story — **song song** với Dev, không cần chờ code
- `security/review.md` do DevSecOps sinh ra từ story + tech-spec

**5 role, mỗi role là một con người thật hoặc AI Agent** — có thể mix:

```
┌──────────────────────────────────────────────────────────────┐
│                     stories/ (Source of Truth)                 │
│                                                               │
│  stories/alerting/create-rule/                                │
│    ├── story.md              ← BA viết                        │
│    ├── tech/                                                  │
│    │   ├── openapi.yaml      ← Dev-BE sinh (SHARED CONTRACT) │
│    │   ├── tech-spec.md      ← Dev-BE: DDD layers, domain    │
│    │   └── tech-spec-fe.md   ← Dev-FE: components, pages     │
│    ├── test/                                                  │
│    │   ├── test-cases.md     ← QC sinh từ story (SONG SONG)  │
│    │   ├── scripts/          ← QC sinh sau khi code exists    │
│    │   └── bugs/             ← QC bug reports                 │
│    └── security/                                              │
│        └── review.md         ← DevSecOps audit                │
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
| 1 | **Story = Source of Truth** | Mọi artifacts (tech-spec, openapi, test-cases, security) sinh ra từ story.md |
| 2 | **OpenAPI = API Contract** | `openapi.yaml` là shared contract giữa BE, FE, QC. Machine-readable, gen code được |
| 3 | **QC song song với Dev** | QC gen test cases từ story, KHÔNG cần chờ code xong |
| 4 | **Tech Spec trước Code** | Dev gen tech-spec + openapi TRƯỚC khi code — cam kết kỹ thuật với BA |
| 5 | **Versioned stories** | Story có version (v1.0.0 → v1.1.0) qua git tags. Update → auto-diff → sync |
| 6 | **Sync on change** | Story thay đổi → Dev sync tech-spec, QC sync test-cases, Sec sync review |
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

# Release: commit + git tag (source of truth cho Dev & QC)
/ba:release alerting/create-rule v1.0.0
```

**Kết quả:** `stories/alerting/create-rule/story.md` + git tag `story/alerting/create-rule/v1.0.0`

BA viết trực tiếp trong `stories/`. Draft = uncommitted changes. Release = commit + git tag.

### Giai Đoạn 2 — Dev generate từ story

```bash
# Story → Tech Spec + OpenAPI (CHẠY TRƯỚC khi code)
/dev:gen-tech-spec alerting/create-rule
/dev:gen-openapi alerting/create-rule

# Implement code từ tech-spec (Dev tự code hoặc AI assist)
# ...

# Check implementation vs AC trong story
/dev:review alerting/create-rule
```

```bash
# Dev-FE đọc openapi.yaml → gen TypeScript client tự động
/dev-fe:gen-client alerting/create-rule

# Dev-FE gen tech-spec cho UI (tham chiếu openapi.yaml)
/dev-fe:gen-tech-spec alerting/create-rule

# Check UI vs AC
/dev-fe:review alerting/create-rule
```

**Output:**
- `stories/alerting/create-rule/tech/tech-spec.md` (BE architecture)
- `stories/alerting/create-rule/tech/openapi.yaml` (shared API contract)
- `stories/alerting/create-rule/tech/tech-spec-fe.md` (FE architecture)

> **Quy tắc:** Chạy `/dev:gen-tech-spec` + `/dev:gen-openapi` **trước** khi code. Tech spec + OpenAPI là bản cam kết với BA — nếu AI generate sai hướng, điều chỉnh trước khi viết code.

### Giai Đoạn 3 — QC generate từ story (SONG SONG với Giai Đoạn 2)

```bash
# Story → Test Cases + Coverage Matrix (SONG SONG với Dev, không cần code)
/qc:gen-test-cases alerting/create-rule

# Test Cases + actual code → Automation Scripts (SAU KHI code exists)
/qc:gen-scripts alerting/create-rule

# Viết bug report có cấu trúc theo Test Case ID
/qc:bug-report alerting/create-rule TC-003
```

**Output:** `stories/alerting/create-rule/test/test-cases.md`

> **Quy tắc:** `/qc:gen-test-cases` chạy **song song** với Dev (từ story, không cần code). `/qc:gen-scripts` chạy **sau** khi Dev implement (cần import types thật).

### Giai Đoạn 4 — DevSecOps review (SONG SONG với Giai Đoạn 2-3)

```bash
# Story + Tech Spec → Security Review
/sec:review alerting/create-rule

# Infra requirements từ story (chỉ khi story cần infra mới)
/sec:gen-infra alerting/create-rule

# Audit code sau khi Dev implement xong
/sec:audit alerting/create-rule
```

**Output:** `stories/alerting/create-rule/security/review.md`

### Giai Đoạn 5 — BA update story (vòng lặp)

```bash
# BA chỉnh sửa stories/alerting/create-rule/story.md
# Sau đó release version mới:
/ba:release alerting/create-rule v1.1.0
# → git tag mới, auto-diff vs v1.0.0, tóm tắt thay đổi
```

```bash
# Dev nhận thông báo, chạy:
/dev:sync alerting/create-rule
# → AI chỉ đúng phần tech-spec + openapi nào cần update

# QC nhận thông báo, chạy:
/qc:sync alerting/create-rule
# → AI chỉ test case nào bị invalid

# DevSecOps nhận thông báo, chạy:
/sec:sync alerting/create-rule
# → AI chỉ security review nào cần update
```

### Timeline Song Song

```
Thời gian ──────────────────────────────────────────────────────────▶

BA:       ██ new-story ██ review ██ release v1.0 ··········· ██ update ██ release v1.1
                                       │                           │
Dev-BE:                                ├── tech-spec + openapi ── code ── review · sync ── fix
Dev-FE:                                ├── gen-client + tech-spec-fe ── code ····· sync ── fix
QC:                                    ├── gen-test-cases ·· gen-scripts (sau code) sync ── update
DevSecOps:                             └── review ── gen-infra ·················· sync ── audit
                                       │
                                  Song song từ đây
```

---

## 3. Story Directory Structure

```
stories/
├── alerting/                                ← Bounded Context
│   ├── create-rule/                         ← Feature
│   │   ├── story.md                         ← BA output (versioned via git tags)
│   │   ├── tech/
│   │   │   ├── openapi.yaml                 ← Dev-BE: SHARED API CONTRACT
│   │   │   ├── tech-spec.md                 ← Dev-BE: DDD layers, domain model
│   │   │   └── tech-spec-fe.md              ← Dev-FE: components, pages, state
│   │   ├── test/
│   │   │   ├── test-cases.md                ← QC: coverage matrix, test scenarios
│   │   │   ├── scripts/                     ← QC: automation scripts (_test.go, .test.ts)
│   │   │   └── bugs/                        ← QC: bug reports (BUG-001.md, ...)
│   │   └── security/
│   │       └── review.md                    ← DevSecOps: security review + infra notes
│   │
│   ├── evaluate-rule/
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

**Versioning:** Git tags thay vì file copies. So sánh versions:
```bash
git diff story/alerting/create-rule/v1.0.0..story/alerting/create-rule/v1.1.0 \
  -- stories/alerting/create-rule/story.md
```

---

## 4. Slash Commands

### 4.1 BA Commands

| Command | Mô tả | Output |
|---------|-------|--------|
| `/ba:new-story <path>` | Tạo story mới từ template | `stories/{path}/story.md` |
| `/ba:review <path>` | AI review gaps, ambiguity, missing AC | Feedback inline |
| `/ba:release <path> <version>` | Commit + git tag, auto-diff nếu update | Git tag + change summary |

### 4.2 Dev-BE Commands

| Command | Mô tả | Output |
|---------|-------|--------|
| `/dev:gen-tech-spec <path>` | Story → tech-spec.md (DDD layer mapping) | `stories/{path}/tech/tech-spec.md` |
| `/dev:gen-openapi <path>` | Story + tech-spec → openapi.yaml | `stories/{path}/tech/openapi.yaml` |
| `/dev:review <path>` | Check implementation vs AC trong story | Pass/Fail report |
| `/dev:sync <path>` | Detect tech-spec + openapi changes needed after story update | Update plan |

### 4.3 Dev-FE Commands

| Command | Mô tả | Output |
|---------|-------|--------|
| `/dev-fe:gen-client <path>` | openapi.yaml → TypeScript API client | `frontend/services/{bc}-api.ts` |
| `/dev-fe:gen-tech-spec <path>` | Story + openapi → tech-spec-fe.md | `stories/{path}/tech/tech-spec-fe.md` |
| `/dev-fe:review <path>` | Check UI vs AC trong story | Pass/Fail report |
| `/dev-fe:sync <path>` | Detect changes needed after story update | UI update plan |

### 4.4 QC Commands

| Command | Mô tả | Output |
|---------|-------|--------|
| `/qc:gen-test-cases <path>` | Story → test-cases.md + coverage matrix (SONG SONG Dev) | `stories/{path}/test/test-cases.md` |
| `/qc:gen-scripts <path>` | test-cases + actual code → automation scripts (SAU code) | `stories/{path}/test/scripts/` |
| `/qc:bug-report <path> <TC-id>` | Tạo bug report có cấu trúc theo TC | `stories/{path}/test/bugs/BUG-{N}.md` |
| `/qc:sync <path>` | Detect test cases invalid sau khi story thay đổi | Invalid test case list |

### 4.5 DevSecOps Commands

| Command | Mô tả | Output |
|---------|-------|--------|
| `/sec:review <path>` | Security review từ story + tech-spec + openapi | `stories/{path}/security/review.md` |
| `/sec:gen-infra <path>` | Sinh infra requirements (chỉ khi story cần) | Infra notes trong review.md |
| `/sec:audit <path>` | Audit source code (OWASP checklist) | Security audit report |
| `/sec:sync <path>` | Detect security review nào cần update | Security update plan |

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
created: 2026-03-19
updated: 2026-03-19
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

### 5.2 tech-spec.md (Dev-BE Output)

```markdown
---
story: US-ALT-001
story_version: 1.0.0
author: DEV-BE
created: 2026-03-19
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
| `http/handler.go` | POST /api/v1/alert-rules (see openapi.yaml) |

## AC Mapping
| AC | Implementation |
|----|---------------|
| AC1 | CreateRuleHandler.Handle() → repo.Save() |
| AC2 | domain.NewAlertRule() validates PromQL syntax |
| AC3 | Out of scope (alerting/notify-slack story) |
| AC4 | repo.FindByName() check before save |
```

### 5.3 openapi.yaml (Dev-BE Output — Shared Contract)

```yaml
openapi: 3.1.0
info:
  title: LogMon Alerting API — create-rule
  version: 1.0.0
paths:
  /api/v1/alert-rules:
    post:
      summary: Create a new alert rule
      operationId: createAlertRule
      requestBody:
        required: true
        content:
          application/json:
            schema:
              $ref: '#/components/schemas/CreateAlertRuleRequest'
      responses:
        '201':
          description: Rule created
          content:
            application/json:
              schema:
                $ref: '#/components/schemas/AlertRule'
        '400':
          description: Validation error
        '409':
          description: Rule name already exists

components:
  schemas:
    CreateAlertRuleRequest:
      type: object
      required: [name, expression, severity, for_duration, service]
      properties:
        name:
          type: string
          minLength: 3
          maxLength: 100
        expression:
          type: string
          description: PromQL expression
        severity:
          type: string
          enum: [critical, warning, info]
        for_duration:
          type: string
          pattern: '^\d+[smh]$'
          example: '2m'
        service:
          type: string

    AlertRule:
      type: object
      properties:
        id:
          type: string
          format: uuid
        name:
          type: string
        status:
          type: string
          enum: [active, inactive]
```

> **Dev-FE** chạy `/dev-fe:gen-client` từ file này → tự động sinh TypeScript API client, không cần viết tay.

### 5.4 test-cases.md (QC Output)

```markdown
---
story: US-ALT-001
story_version: 1.0.0
author: QC
created: 2026-03-19
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
- Viết stories với AC (Given/When/Then) trực tiếp trong `stories/`
- Review stories trước release (gaps, ambiguity)
- Release: commit + git tag (v1.0.0, v1.1.0, ...)
- Auto-diff khi release update version

### 6.2 DEV-BE (Backend Developer)

| Thuộc tính | Giá trị |
|-----------|---------|
| **Model** | Sonnet (balanced cho code generation) |
| **Tools** | Read, Write, Edit, Bash, Glob, Grep |
| **KHÔNG có** | WebSearch, WebFetch |
| **Context** | `CLAUDE.md` full, `doc/logmon.md` Sections 7-9, story + tech-spec + openapi |

**Trách nhiệm:**
- Gen tech-spec từ story (DDD layer mapping)
- Gen openapi.yaml (API contract cho FE/QC consume)
- Implement code từ tech-spec
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
| **Context** | `CLAUDE.md`, story + openapi.yaml + tech-spec-fe, existing components |

**Trách nhiệm:**
- Gen TypeScript API client từ openapi.yaml (tự động, không viết tay)
- Gen tech-spec-fe (component tree, pages, state — tham chiếu openapi)
- Implement responsive UI
- TypeScript strict, no `any`

### 6.4 QC (Quality Assurance)

| Thuộc tính | Giá trị |
|-----------|---------|
| **Model** | Opus (reasoning sâu cho test design) |
| **Tools** | Read, Write, Bash, Glob, Grep |
| **KHÔNG có** | Edit source code (chỉ viết test files + reports) |
| **Context** | Story (AC), openapi.yaml, tech-spec, source code, `doc/logmon.md` Section 9 |

**Trách nhiệm:**
- Gen test cases từ story (SONG SONG với Dev, không chờ code)
- Gen automation scripts từ test-cases + actual code (SAU khi code exists)
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
| **Context** | `doc/logmon.md` Sections 9.7 + 13, story + tech-spec + openapi, infra configs |

**Trách nhiệm:**
- Security review từ story + tech-spec + openapi (OWASP checklist)
- Gen infra requirements (chỉ khi story cần infra mới)
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
  > git push

Terminal 2 — Dev-BE (người thật):
  $ claude
  > git pull  # lấy story mới từ BA
  > /dev:gen-tech-spec alerting/create-rule
  > /dev:gen-openapi alerting/create-rule
  > # ... implement from tech-spec ...
  > /dev:review alerting/create-rule
  > git push

Terminal 3 — Dev-FE (người thật):    ← SAU KHI openapi.yaml exists
  $ claude
  > git pull  # lấy openapi.yaml từ Dev-BE
  > /dev-fe:gen-client alerting/create-rule
  > /dev-fe:gen-tech-spec alerting/create-rule
  > # ... implement UI ...
  > /dev-fe:review alerting/create-rule
  > git push

Terminal 4 — QC (người thật):        ← SONG SONG với Terminal 2-3
  $ claude
  > git pull  # lấy story mới từ BA
  > /qc:gen-test-cases alerting/create-rule
  > # ... chờ code exists ...
  > /qc:gen-scripts alerting/create-rule
  > git push

Terminal 5 — DevSecOps (người thật):  ← SONG SONG với Terminal 2-4
  $ claude
  > git pull
  > /sec:review alerting/create-rule
  > # ... chờ code exists ...
  > /sec:audit alerting/create-rule
  > git push
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

```bash
# 1 người, chạy lần lượt
/ba:new-story alerting/create-rule
/ba:release alerting/create-rule v1.0.0

/dev:gen-tech-spec alerting/create-rule
/dev:gen-openapi alerting/create-rule
/qc:gen-test-cases alerting/create-rule       # parallel: from story, no code needed

# implement code ...

/dev-fe:gen-client alerting/create-rule        # after openapi exists
/qc:gen-scripts alerting/create-rule           # after code exists
/sec:review alerting/create-rule
/dev:review alerting/create-rule               # self-check
```

---

## 8. Sync Workflow (Khi Story Thay Đổi)

```
BA update story v1.0.0 → v1.1.0
         │
         ├──▶ /ba:release alerting/create-rule v1.1.0
         │         │
         │         ├── Git tag: story/alerting/create-rule/v1.1.0
         │         ├── Auto-diff: git diff v1.0.0..v1.1.0
         │         └── Change summary: "Added AC5, modified AC2 threshold"
         │
         ├──▶ Dev chạy /dev:sync alerting/create-rule
         │         └── AI output: "tech-spec.md cần update:
         │                          - Section 'AC Mapping': thêm AC5
         │                          openapi.yaml cần update:
         │                          - Thêm field 'labels' trong request schema"
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
start: 2026-03-19
end: 2026-04-02
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
  "openapi_as_contract": true,
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
  "qc_parallel_with_dev": true
}
```

### 10.2 Agent Context Injection

Mỗi agent khi spawn được inject context phù hợp với role:

| Agent | CLAUDE.md | logmon.md Sections | Story artifacts |
|-------|-----------|-------------------|-----------------|
| BA | Overview, Architecture Rules | 1-6 (system overview) | Existing stories, backlog |
| DEV-BE | Full (arch + style + security) | 7-9 (structure + backend + rules) | story.md → gen tech-spec.md + openapi.yaml |
| DEV-FE | Overview + frontend-relevant | 7, 9.1, 9.8 | story.md + openapi.yaml → gen tech-spec-fe.md + client |
| QC | Architecture Rules, Testing | 9.2, 9.8 (error handling + testing) | story.md + openapi.yaml → gen test-cases.md |
| DevSecOps | Security section | 9.7, 9.9, 13 (security + infra + deploy) | story.md + tech-spec.md + openapi.yaml |

---

## 11. Quick Start

```bash
# 1. Tạo directories
mkdir -p stories .agile/sprints

# 2. BA tạo story đầu tiên
/ba:new-story alerting/create-rule
# → stories/alerting/create-rule/story.md

# 3. BA review và release
/ba:review alerting/create-rule
/ba:release alerting/create-rule v1.0.0
# → git tag story/alerting/create-rule/v1.0.0

# 4. Dev-BE gen tech-spec + openapi (TRƯỚC khi code)
/dev:gen-tech-spec alerting/create-rule
/dev:gen-openapi alerting/create-rule

# 5. QC gen test cases SONG SONG (không cần chờ code)
/qc:gen-test-cases alerting/create-rule

# 6. Dev-FE gen client từ openapi
/dev-fe:gen-client alerting/create-rule
/dev-fe:gen-tech-spec alerting/create-rule

# 7. Dev implement code từ tech-spec ...

# 8. QC gen scripts SAU khi code exists
/qc:gen-scripts alerting/create-rule

# 9. DevSecOps review
/sec:review alerting/create-rule

# 10. Verify
/dev:review alerting/create-rule
/dev-fe:review alerting/create-rule

# 11. Nếu BA update story:
/ba:release alerting/create-rule v1.1.0
/dev:sync alerting/create-rule
/qc:sync alerting/create-rule
/sec:sync alerting/create-rule
```

---

*Tài liệu thiết kế dựa trên [get-shit-done](https://github.com/gsd-build/get-shit-done) architecture + Luồng làm việc chuẩn BA/Dev/QC, adapted cho 5-role team (BA, DEV-BE, DEV-FE, QC, DevSecOps) trên dự án LogMon.*
