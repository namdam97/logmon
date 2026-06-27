# DDD & Bounded Contexts trong LogMon
> Module ARCH-2 · aggregate/VO/domain event + bản đồ BC · Độ khó: 🥉→🥇 · Prereqs: ARCH-1

## 1. Vì sao kỹ năng này quan trọng trong LogMon

LogMon không phải một service phẳng — nó là một **modular monolith** (`logmon-api`) gồm nhiều miền nghiệp vụ khác hẳn nhau: xác thực người dùng (`identity`), quản lý alert rule (`alerting`), truy vấn log (`logpipeline`), và sau này là SLO/incident/notification. Mỗi miền có "ngôn ngữ" riêng: chữ "rule" trong `alerting` là một biểu thức PromQL có severity và `for`; trong `logpipeline` không có khái niệm đó. Nếu nhồi tất cả vào một model chung, bạn sẽ có một `God object` mà sửa một chỗ vỡ ba chỗ.

Domain-Driven Design (DDD) cho bạn ngôn ngữ và ranh giới để chia hệ thống đúng theo nghiệp vụ. Cụ thể trong LogMon:

- **Quy tắc kiến trúc bắt buộc** (`CLAUDE.md:61`): "KHÔNG cross-BC imports — BCs giao tiếp qua domain events hoặc shared kernel". Hiểu DDD là điều kiện để không vi phạm luật này.
- **Layer direction** `adapters → ports ← app → domain`: tầng `domain/` chỉ được import Go stdlib. Muốn giữ luật đó, bạn phải biết cái gì là domain logic, cái gì là hạ tầng.
- **Cross-BC event** như `AlertFired → NotificationService.Send()` (`CLAUDE.md:159`) là cách duy nhất các BC nói chuyện với nhau. Không có DDD, bạn sẽ `import` thẳng và tạo coupling chết người.

Nắm DDD ở đây = đọc được tại sao `alerting` tách aggregate `AlertRule` khỏi entity `AlertInstance`, và viết được BC mới (`slo`, `incident`) đúng khuôn mà không phá kiến trúc.

## 2. Mô hình tư duy (first principles) — giải thích từ con số 0

Hãy quên framework đi. DDD trả lời một câu hỏi gốc: **"Code nên được tổ chức quanh cái gì?"** Câu trả lời của DDD: quanh **nghiệp vụ**, không quanh kỹ thuật (không phải "tất cả controller một chỗ, tất cả model một chỗ").

DDD có hai nửa:

**Strategic (chiến lược) — chia hệ thống thành các Bounded Context.** Một *bounded context* (BC) là một biên giới ngữ nghĩa: bên trong nó, mỗi thuật ngữ có đúng một nghĩa. "Rule" bên trong `alerting` luôn là alert rule; bạn không cần thêm tiền tố. *Ubiquitous language* (ngôn ngữ thống nhất) là từ vựng mà dev + chuyên gia nghiệp vụ dùng chung *trong một BC* — và nó xuất hiện thẳng trong tên code: `AlertRule`, `Severity`, `Silence`, `Fingerprint`.

**Tactical (chiến thuật) — bên trong một BC, model bằng các khối:**

- **Entity**: có *định danh* sống qua thời gian. Hai entity cùng ID là cùng một thứ dù thuộc tính đổi. Ví dụ: một user đổi email vẫn là user đó.
- **Value Object (VO)**: *không* có định danh, chỉ định nghĩa bằng giá trị, **bất biến**. Hai VO cùng giá trị là thay thế được cho nhau. Ví dụ: `Severity{"critical"}`.
- **Aggregate**: một cụm entity + VO có chung một **biên giới nhất quán** (consistency boundary). Một entity là *aggregate root* — cổng duy nhất để sửa cụm. Mọi invariant (luật bất biến) được bảo vệ qua root.
- **Domain Event**: một sự kiện nghiệp vụ đã xảy ra ("alert rule đã được tạo"), dùng để phối hợp giữa các aggregate/BC.

Nguyên lý nền: *một transaction chỉ sửa một aggregate*; mọi thứ vượt ra ngoài aggregate thì dùng **eventual consistency** qua domain event. Đó chính là lý do LogMon có transactional outbox.

## 3. Khái niệm cốt lõi (tăng dần độ khó)

### 3.1 Value Object — bất biến, định nghĩa bằng giá trị

VO là viên gạch nhỏ nhất. Nó tự validate khi sinh ra và không bao giờ rơi vào trạng thái sai. Trong LogMon, `Severity` là VO kinh điển:

```go
// backend/internal/alerting/domain/severity.go
type Severity struct{ value string } // field không export → bất biến

func NewSeverity(raw string) (Severity, error) {
    switch raw {
    case "critical", "warning", "info":
        return Severity{raw}, nil
    default:
        return Severity{}, newValidationError("severity", "must be one of critical|warning|info")
    }
}
```

Không ai tạo được `Severity` sai vì constructor là cổng duy nhất. VO còn mang **hành vi miền**: `Severity.MinForDuration()` trả về `for` tối thiểu (critical ≥ 1m) — logic nghiệp vụ sống *trong* VO, không phải trong if/else rải rác.

| Đặc điểm | Entity | Value Object |
|---|---|---|
| Định danh | Có (ID) | Không |
| So sánh | Theo ID | Theo giá trị |
| Mutable? | Trạng thái đổi theo thời gian | Bất biến — đổi = tạo mới |
| Ví dụ LogMon | `AlertRule`, `User`, `AlertInstance` | `Severity`, `Fingerprint`, `RuleID`, `SearchCriteria` |

### 3.2 Aggregate Root — cổng vào duy nhất, bảo vệ invariant

`AlertRule` là aggregate root của `alerting`. Mọi field **không export** — không sửa trực tiếp được từ ngoài:

```go
// backend/internal/alerting/domain/rule.go:48
type AlertRule struct {
    id          RuleID
    name        string
    expression  string
    severity    Severity
    forDuration time.Duration
    // ...
}
```

Bạn chỉ tạo được qua `NewAlertRule(...)`, và nó *bắt buộc* qua `validateRuleFields` (`rule.go:94`) kiểm mọi invariant: name không rỗng và ≤ 100 ký tự, expression không rỗng, annotation `summary` + `runbook_url` bắt buộc, và `forDuration ≥ severity.MinForDuration()`. Đây là *true invariant* — luật phải đúng trong cùng transaction với thao tác sửa.

### 3.3 Bất biến qua copy-on-write (immutability)

Mọi chuyển trạng thái của `AlertRule` **trả về bản copy mới**, không mutate tại chỗ — đúng nguyên tắc immutability:

```go
// backend/internal/alerting/domain/rule.go:245
func (r AlertRule) Disabled(now time.Time) AlertRule {
    c := r.clone()           // copy sâu cả map labels/annotations
    c.enabled = false
    c.syncStatus = SyncPending
    c.updatedAt = now
    return c
}
```

`copyMap` (`rule.go:329`) đảm bảo không rò rỉ reference map ra ngoài boundary — tránh side effect ẩn.

### 3.4 State machine trong aggregate — `AlertInstance`

`AlertInstance` (`backend/internal/alerting/domain/instance.go:38`) là entity mô hình vòng đời một lần firing: `firing → acknowledged → resolved`. Hành vi kiểm transition hợp lệ:

```go
// instance.go:115
func (i AlertInstance) Acknowledge(by string, at time.Time) (AlertInstance, error) {
    if i.status != InstanceFiring {
        return AlertInstance{}, ErrInstanceNotAcknowledgeable // chỉ firing mới ack được
    }
    // ...
}
```

Luật "chỉ instance đang firing mới ack được" sống trong domain, không ở handler — không thể bypass.

### 3.5 Domain Event + biên giới nhất quán

Khi `CreateRuleHandler` lưu rule, nó phát `AlertRuleCreated` **trong cùng một TX** với INSERT rule (`backend/internal/alerting/app/command/create_rule.go:79`). Payload tối thiểu (chỉ `ruleId` + `workspaceId`, `events.go:15`) — subscriber tự đọc chi tiết từ DB. Đây là cách LogMon vượt biên giới một aggregate mà vẫn nhất quán: ghi event + state atomically, rồi xử lý phần còn lại bất đồng bộ.

## 4. LogMon dùng nó thế nào (bám code thật)

![DDD & Bounded Contexts trong LogMon](../../doc_v2/diagrams/logmon_bc_map.png)

LogMon dùng **hai pattern** tùy độ phức tạp domain (`CLAUDE.md:63`, `doc_v2/02-backend-architecture.md:9`):

| BC | Pattern | Trạng thái |
|---|---|---|
| `internal/user/` → `identity/` | Clean Architecture | **Implemented** (còn tên `user/`, đổi tên ở GĐ3 — ADR-029) |
| `internal/alerting/` | Clean Arch + DDD + CQRS | **Implemented** (rule + instance + silence) |
| `internal/logpipeline/` | Clean Arch + DDD + CQRS | **Implemented một phần** — chỉ read side (query log) |
| `internal/slo/` | Clean Arch + DDD + CQRS | **Mới có tầng domain** — `slo/domain/` đã có aggregate `SLO` + VO + events; `app/ports/adapters` *chưa có* (planned) |
| `internal/incident/`, `notification/` | (theo bảng) | **Planned** — thư mục *chưa tồn tại* trong repo |
| `internal/shared/` | Shared Kernel | **Implemented** (outbox, errors, auth, metrics, tracing...) |

**Aggregate đã có (implemented):**

- `AlertRule` (aggregate root) + `Severity`, `RuleID` (VO) — `backend/internal/alerting/domain/rule.go`, `severity.go`.
- `AlertInstance` (entity, state machine) + `Fingerprint` (VO ≤ 64 ký tự khớp cột DB) — `backend/internal/alerting/domain/instance.go:20`.
- `Silence` + `SilenceMatcher` (VO) — `backend/internal/alerting/domain/silence.go`. Chú ý ranh giới trách nhiệm: LogMon **không** lưu silence; Alertmanager là source of truth, `Silence` chỉ validate + proxy (`silence.go:49`).
- `User` (aggregate root) + `Email`, `UserID` (VO) — `backend/internal/user/domain/user.go:29`.
- `SearchCriteria` (VO query đã validate) — `backend/internal/logpipeline/domain/search.go:43`. Lưu ý `logpipeline` hiện **chỉ có read side**: `LogEntry`/`SearchResult` là read model (`log.go:10`), write side (Mode switch, DLQ, ILM) còn **planned**.

**CQRS — tách write/read:** `alerting` có `app/command/` (write: `create_rule.go`, `acknowledge.go`...) và `app/query/` (read: `queries.go`) riêng biệt, vì monitoring có tỷ lệ read:write ~100:1 (`doc_v2/02-backend-architecture.md:65`). Ports tách rõ `RuleRepository` (write) vs `RuleReader` (read) — `backend/internal/alerting/ports/ports.go:21,66`.

**Cross-BC qua event, không import:** Bằng chứng kiến trúc là `alerting/domain/errors.go:5-8` — package `domain` chỉ import `errors` + `fmt` (stdlib). Event đi qua **shared kernel outbox**: `Bus` in-process dispatch đồng bộ theo event type (`backend/internal/shared/outbox/bus.go:34`), `Relay` background worker quét outbox table bằng `FOR UPDATE SKIP LOCKED` (SQL ở `outbox/store.go:110`, `doc_v2/02-backend-architecture.md:165`). Handler **phải idempotent** vì at-least-once (`bus.go:9`). Đây là transactional outbox (ADR-016) — chưa có BC consumer thật vì `notification`/`incident` còn planned; hiện tại event `AlertRuleCreated` được `alerting` tự subscribe để chạy rule sync sang Prometheus.

## 5. Best practices (mỗi mục kèm 1 nguồn)

1. **Thiết kế aggregate nhỏ — chỉ gói dữ liệu cần nhất quán trong một transaction.** LogMon tách `AlertRule` và `AlertInstance` thành hai aggregate riêng vì vòng đời độc lập. Vernon: "Limit the Aggregate to just the Root Entity and a minimal number of attributes" ([InformIT — Rule: Design Small Aggregates](https://www.informit.com/articles/article.aspx?p=2020371&seqNum=3)).

2. **Tham chiếu aggregate khác bằ̀ng ID, không bằng object.** `RulePayload` chỉ mang `ruleId`/`workspaceId` (`events.go:15`), không nhồi cả state. Microsoft Learn: "The `Delivery` aggregate stores a `DroneId`... not direct references. This decoupling maps directly to microservice boundaries" ([Azure — Tactical DDD](https://learn.microsoft.com/en-us/azure/architecture/microservices/model/tactical-ddd)).

3. **Eventual consistency giữa aggregate bằng domain event, không one-big-transaction.** Đúng như outbox + bus của LogMon. ([Azure — Tactical DDD](https://learn.microsoft.com/en-us/azure/architecture/microservices/model/tactical-ddd)).

4. **VO bất biến, đổi = tạo mới.** `AlertRule.Update()` trả copy (`rule.go:178`). "Value objects are immutable. To update, you create a new instance" ([Azure — Tactical DDD](https://learn.microsoft.com/en-us/azure/architecture/microservices/model/tactical-ddd)).

5. **Nhét hành vi vào entity, tránh anemic domain model.** `Acknowledge`/`Disabled` sống trong domain, không trong handler. ([Azure — Tactical DDD](https://learn.microsoft.com/en-us/azure/architecture/microservices/model/tactical-ddd)).

6. **Transactional outbox để ghi state + event atomically (giải dual-write).** ([microservices.io / AWS Prescriptive Guidance — Transactional Outbox](https://docs.aws.amazon.com/prescriptive-guidance/latest/cloud-design-patterns/transactional-outbox.html)).

7. **Ubiquitous language + context map giữa các BC.** Tên code = tên nghiệp vụ; quan hệ BC khai báo rõ (Shared Kernel cho `internal/shared/`). ([Microsoft Learn — Domain Analysis](https://learn.microsoft.com/en-us/azure/architecture/microservices/model/domain-analysis)).

## 6. Lỗi thường gặp & anti-patterns

- **Anemic domain model**: aggregate chỉ là struct getter/setter, logic nằm hết ở service. LogMon tránh bằng cách đặt invariant trong `validateRuleFields` và transition trong method aggregate.
- **Aggregate khổng lồ**: gộp `AlertRule` + tất cả `AlertInstance` của nó vào một aggregate → lock cạnh tranh, không scale. Hai aggregate riêng là đúng.
- **Cross-BC import**: `import ".../notification/domain"` từ `alerting` → vi phạm `CLAUDE.md:61`. Dùng domain event.
- **Domain import hạ tầng**: `import "github.com/jackc/pgx"` trong `domain/` → vỡ layer direction. Domain chỉ stdlib (`doc_v2/02-backend-architecture.md:30`).
- **Mutate aggregate tại chỗ**: sửa field rồi save → mất immutability, side effect ẩn. Luôn `clone()` + trả copy.
- **Event payload béo**: nhồi cả aggregate state vào outbox → coupling schema. Chỉ mang ID, subscriber đọc lại.
- **Handler không idempotent**: bus là at-least-once (`bus.go:9`); subscriber không dedup → side effect lặp.
- **VO không validate ở constructor**: tạo `Severity{"foo"}` trực tiếp được → invariant vỡ. Field unexported + constructor là cổng duy nhất.

## 7. Lộ trình luyện tập NGAY trong repo LogMon

### 🥉 Cơ bản
- Đọc `backend/internal/alerting/domain/severity.go` rồi viết test case mới trong `rule_test.go` xác nhận `NewSeverity("debug")` trả `*ValidationError`.
- Map từng field của `AlertRule` (`rule.go:48`) vào cột tương ứng trong `doc_v2/08-database-schema.md` — nhận diện cái nào là VO, cái nào primitive.
- Thêm một accessor mới `IsCritical() bool` cho `AlertRule` (so `r.severity == SeverityCritical`) + test, để quen pattern method-trên-aggregate.
- Vẽ lại state machine của `AlertInstance` (`instance.go`) thành 3 ô + mũi tên, đối chiếu với method `Acknowledge`/`Resolve`.

### 🥈 Trung cấp
- Thêm một VO mới `Workspace` (hoặc `WorkspaceID`) bất biến trong `alerting/domain` theo khuôn `RuleID` (`rule.go:30`), có constructor validate + `String()` + test bảng.
- Thêm domain event `AlertInstanceAcknowledged` vào `events.go` và phát nó trong command `acknowledge.go` đúng pattern transactional outbox (publish trong `WithinTx`).
- Viết một subscriber idempotent đăng ký qua `Bus.Subscribe` (`bus.go:26`) chỉ log ra `ruleId` khi nhận `AlertRuleCreated`, kèm test dùng `outbox.Bus`.
- So sánh `ports.RuleRepository` (write) vs `ports.RuleReader` (read) trong `ports.go` và viết một note giải thích vì sao CQRS tách hai interface.

### 🥇 Nâng cao
- Đọc `slo/domain/` (đã có: aggregate `SLO`, VO `SLIType`/`SLOID`, event `BudgetExhausted` — `slo.go`, `events.go`) rồi bổ sung **tầng còn thiếu**: phác thảo `slo/ports/` (interface `SLORepository` + `SLOReader` theo khuôn `alerting/ports/ports.go`) và một use case `app/command/define_slo.go` rỗng, KHÔNG import BC khác. (VO `ErrorBudget`/`BurnRate` ở `doc_v2/02-backend-architecture.md:106` hiện mới có method `SLO.ErrorBudget()`, chưa tách thành VO riêng.)
- Thiết kế cross-BC event `BudgetExhausted → alerting.CreateCriticalAlert` (`doc_v2/02-backend-architecture.md:139`): viết payload tối thiểu + đăng ký handler qua bus, chứng minh không có import vòng.
- Chạy `golangci-lint run` ở `backend/` và kiểm tra rằng không có import nào từ `domain/` ra ngoài stdlib; thêm depguard rule chặn cross-BC import nếu chưa có (`doc_v2/02-backend-architecture.md:38`).
- Thêm write side đầu tiên cho `logpipeline` (hiện chỉ read): aggregate `Pipeline` với VO `PipelineMode` (A/B) + event `PipelineModeChanged`, theo `doc_v2/02-backend-architecture.md:111`.

## 8. Skill/agent ECC nên dùng khi luyện

- **`ecc:architect`** — khi quyết định BC mới (`slo`/`incident`) thuộc Clean Arch hay DDD+CQRS, hoặc khi vẽ context map / quan hệ event giữa BC. Dùng *trước* khi viết code.
- **`ecc:code-explorer`** (hoặc `ecc:codebase-onboarding`) — khi cần lần theo một aggregate qua các tầng (domain → command → ports → adapter) để hiểu luồng, ví dụ truy `AlertRuleCreated` đi đâu.
- **`ecc:type-design-analyzer`** — khi review một VO/aggregate mới: kiểm field unexported, constructor validate, immutability, tránh anemic model.
- Bổ trợ: **`ecc:go-review`** sau khi viết domain code (idiomatic + concurrency của outbox), và **`ecc:go-test`** để giữ TDD table-driven ≥ 80% như chuẩn repo.

## 9. Tài nguyên học thêm

- [Azure Architecture Center — Use Tactical DDD to Design Microservices](https://learn.microsoft.com/en-us/azure/architecture/microservices/model/tactical-ddd) — chuẩn về entity/VO/aggregate/domain event, ví dụ Drone Delivery rất sát LogMon.
- [Azure — Use Domain Analysis to Model Microservices](https://learn.microsoft.com/en-us/azure/architecture/microservices/model/domain-analysis) — phần strategic: subdomain, ubiquitous language, bounded context.
- [InformIT — Vaughn Vernon, Rule: Design Small Aggregates](https://www.informit.com/articles/article.aspx?p=2020371&seqNum=3) — luật aggregate nhỏ + tham chiếu theo ID, kèm khi nào được phá luật.
- [AWS Prescriptive Guidance — Transactional Outbox Pattern](https://docs.aws.amazon.com/prescriptive-guidance/latest/cloud-design-patterns/transactional-outbox.html) — nền tảng của outbox/relay trong LogMon, giải dual-write.
- [Microsoft Learn — Identify microservice boundaries](https://learn.microsoft.com/en-us/azure/architecture/microservices/model/microservice-boundaries) — "no smaller than an aggregate, no larger than a bounded context".
- LogMon nội bộ: `doc_v2/01-kien-truc-tong-the.md`, `doc_v2/02-backend-architecture.md` (mục 4–5), `doc_v2/13-adr.md` (ADR-016 outbox, ADR-024 alerting boundary, ADR-029 identity rename).

## 10. Checklist "đã hiểu"

- [ ] Phân biệt được Entity vs Value Object và chỉ ra ví dụ thật trong LogMon (`AlertRule` vs `Severity`).
- [ ] Giải thích được vì sao `AlertRule` và `AlertInstance` là **hai** aggregate riêng, không phải một.
- [ ] Chỉ ra invariant nào được bảo vệ trong `validateRuleFields` và tại sao nó nằm ở `domain/` chứ không ở handler.
- [ ] Mô tả được layer direction `adapters → ports ← app → domain` và lý do `domain/` chỉ import stdlib.
- [ ] Trình bày luồng transactional outbox: state + event ghi chung TX, relay drain, bus dispatch, handler idempotent.
- [ ] Phân biệt write side (`app/command`) và read side (`app/query`) trong CQRS và vì sao monitoring cần tách.
- [ ] Nêu đúng cái gì **implemented** (alerting đầy đủ, user, logpipeline read-only, `slo/domain/`) vs **planned** (slo app/ports/adapters, incident, notification).
- [ ] Biết hai cách hợp pháp để hai BC giao tiếp (domain event + shared kernel) và vì sao cross-BC import bị cấm.
