// Command userservice là HTTP service quản lý user của LogMon. main() chỉ gọi
// run() và exit một lần — toàn bộ logic khởi tạo + graceful shutdown nằm trong run.
package main

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	"github.com/exaring/otelpgx"
	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/redis/go-redis/v9"
	"go.opentelemetry.io/contrib/instrumentation/github.com/gin-gonic/gin/otelgin"

	"github.com/namdam97/logmon/backend/internal/alerting/adapters/alertmanager"
	alertinghttp "github.com/namdam97/logmon/backend/internal/alerting/adapters/http"
	alertingpg "github.com/namdam97/logmon/backend/internal/alerting/adapters/postgres"
	"github.com/namdam97/logmon/backend/internal/alerting/adapters/promfile"
	alertingpromk8s "github.com/namdam97/logmon/backend/internal/alerting/adapters/promk8s"
	"github.com/namdam97/logmon/backend/internal/alerting/adapters/promql"
	alertingsys "github.com/namdam97/logmon/backend/internal/alerting/adapters/system"
	"github.com/namdam97/logmon/backend/internal/alerting/app/command"
	"github.com/namdam97/logmon/backend/internal/alerting/app/query"
	alertingdomain "github.com/namdam97/logmon/backend/internal/alerting/domain"
	incidenthttp "github.com/namdam97/logmon/backend/internal/incident/adapters/http"
	incidentmetrics "github.com/namdam97/logmon/backend/internal/incident/adapters/metrics"
	incidentpg "github.com/namdam97/logmon/backend/internal/incident/adapters/postgres"
	incidentsys "github.com/namdam97/logmon/backend/internal/incident/adapters/system"
	incidentcommand "github.com/namdam97/logmon/backend/internal/incident/app/command"
	incidentquery "github.com/namdam97/logmon/backend/internal/incident/app/query"
	incidentdomain "github.com/namdam97/logmon/backend/internal/incident/domain"
	incidentports "github.com/namdam97/logmon/backend/internal/incident/ports"
	"github.com/namdam97/logmon/backend/internal/logpipeline/adapters/elasticsearch"
	loghttp "github.com/namdam97/logmon/backend/internal/logpipeline/adapters/http"
	logpg "github.com/namdam97/logmon/backend/internal/logpipeline/adapters/postgres"
	logcommand "github.com/namdam97/logmon/backend/internal/logpipeline/app/command"
	logquery "github.com/namdam97/logmon/backend/internal/logpipeline/app/query"
	logports "github.com/namdam97/logmon/backend/internal/logpipeline/ports"
	notifhttp "github.com/namdam97/logmon/backend/internal/notification/adapters/http"
	notifpg "github.com/namdam97/logmon/backend/internal/notification/adapters/postgres"
	"github.com/namdam97/logmon/backend/internal/notification/adapters/redisqueue"
	"github.com/namdam97/logmon/backend/internal/notification/adapters/sender"
	notifsys "github.com/namdam97/logmon/backend/internal/notification/adapters/system"
	notifcommand "github.com/namdam97/logmon/backend/internal/notification/app/command"
	"github.com/namdam97/logmon/backend/internal/notification/app/notify"
	notifworker "github.com/namdam97/logmon/backend/internal/notification/app/worker"
	notifdomain "github.com/namdam97/logmon/backend/internal/notification/domain"
	reporthttp "github.com/namdam97/logmon/backend/internal/reporting/adapters/http"
	reportpg "github.com/namdam97/logmon/backend/internal/reporting/adapters/postgres"
	reportsys "github.com/namdam97/logmon/backend/internal/reporting/adapters/system"
	reportcommand "github.com/namdam97/logmon/backend/internal/reporting/app/command"
	reportquery "github.com/namdam97/logmon/backend/internal/reporting/app/query"
	reportworker "github.com/namdam97/logmon/backend/internal/reporting/app/worker"
	"github.com/namdam97/logmon/backend/internal/shared/audit"
	"github.com/namdam97/logmon/backend/internal/shared/auth"
	"github.com/namdam97/logmon/backend/internal/shared/crypto"
	"github.com/namdam97/logmon/backend/internal/shared/health"
	"github.com/namdam97/logmon/backend/internal/shared/logger"
	"github.com/namdam97/logmon/backend/internal/shared/metrics"
	"github.com/namdam97/logmon/backend/internal/shared/middleware"
	"github.com/namdam97/logmon/backend/internal/shared/outbox"
	"github.com/namdam97/logmon/backend/internal/shared/promrule"
	"github.com/namdam97/logmon/backend/internal/shared/tracing"
	slohttp "github.com/namdam97/logmon/backend/internal/slo/adapters/http"
	slopg "github.com/namdam97/logmon/backend/internal/slo/adapters/postgres"
	sloprom "github.com/namdam97/logmon/backend/internal/slo/adapters/prometheus"
	slopromfile "github.com/namdam97/logmon/backend/internal/slo/adapters/promfile"
	slopromk8s "github.com/namdam97/logmon/backend/internal/slo/adapters/promk8s"
	slosys "github.com/namdam97/logmon/backend/internal/slo/adapters/system"
	slocommand "github.com/namdam97/logmon/backend/internal/slo/app/command"
	sloquery "github.com/namdam97/logmon/backend/internal/slo/app/query"
	slosnapshot "github.com/namdam97/logmon/backend/internal/slo/app/snapshot"
	slodomain "github.com/namdam97/logmon/backend/internal/slo/domain"
	topocache "github.com/namdam97/logmon/backend/internal/topology/adapters/cache"
	topoes "github.com/namdam97/logmon/backend/internal/topology/adapters/elasticsearch"
	topohttp "github.com/namdam97/logmon/backend/internal/topology/adapters/http"
	topoapp "github.com/namdam97/logmon/backend/internal/topology/app"
	topoports "github.com/namdam97/logmon/backend/internal/topology/ports"
	usagehttp "github.com/namdam97/logmon/backend/internal/usage/adapters/http"
	usagepg "github.com/namdam97/logmon/backend/internal/usage/adapters/postgres"
	usageprom "github.com/namdam97/logmon/backend/internal/usage/adapters/prometheus"
	usageapp "github.com/namdam97/logmon/backend/internal/usage/app"
	usageports "github.com/namdam97/logmon/backend/internal/usage/ports"
	userhttp "github.com/namdam97/logmon/backend/internal/user/adapters/http"
	userpg "github.com/namdam97/logmon/backend/internal/user/adapters/postgres"
	usersys "github.com/namdam97/logmon/backend/internal/user/adapters/system"
	userapp "github.com/namdam97/logmon/backend/internal/user/app"
)

const (
	_serviceName             = "userservice" // service.name trong span + OTEL_SERVICE_NAME default
	_defaultPort             = "8080"
	_shutdownTimeout         = 10 * time.Second
	_readHeaderTimeout       = 5 * time.Second
	_dbConnectTimeout        = 10 * time.Second
	_escalationSweepInterval = time.Minute         // chu kỳ quét escalation (GĐ3.4)
	_postmortemSweepInterval = time.Hour           // chu kỳ quét reminder postmortem (GĐ3.5)
	_exportWorkerInterval    = 10 * time.Second    // chu kỳ tiêu thụ export job (GĐ4.3)
	_reportSchedulerInterval = time.Minute         // chu kỳ chạy report schedule (GĐ4.3)
	_defaultJWTTTL           = 15 * time.Minute    // access token ngắn (ADR-023)
	_defaultRefreshTTL       = 14 * 24 * time.Hour // refresh token dài
	_authRatePerMinute       = 10                  // mặc định: giới hạn login/register mỗi IP
	_authRateBurst           = 5
	_wsRatePerMinute         = 1000 // mặc định: giới hạn request mỗi workspace (doc_v2/07 §3)
	_wsRateBurst             = 100

	// _defaultWorkspaceID là workspace mặc định GĐ2 (multi-tenancy đầy đủ ở GĐ3).
	_defaultWorkspaceID = "00000000-0000-0000-0000-000000000001"
	_defaultRulesDir    = "/etc/prometheus/generated" // Prometheus mount đọc rule file đã render
	_defaultPromURL     = "http://prometheus:9090"    // cần --web.enable-lifecycle cho /-/reload
	_defaultAlertmgrURL = "http://alertmanager:9093"  // proxy silence (GĐ2.4b)

	// Rule sync mode (ADR-024): "file" = render rule file + reload (Compose);
	// "k8s" = apply PrometheusRule CR (Prometheus Operator, Phase III C3).
	_ruleSyncFile = "file"
	_ruleSyncK8s  = "k8s"
	// Tên PrometheusRule CR sinh ở mode k8s (1 cho alerting, 1 cho slo).
	_alertingRuleCRName = "logmon-alerting-rules"
	_sloRuleCRName      = "logmon-slo-rules"
	_defaultCRNamespace = "logmon"
)

func main() {
	// Subcommand healthcheck cho Docker HEALTHCHECK (image distroless không có wget).
	if len(os.Args) > 1 && os.Args[1] == "healthcheck" {
		healthCheck()
		return
	}
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "userservice:", err)
		os.Exit(1)
	}
}

type config struct {
	port           string
	databaseURL    string
	logLevel       string
	jwtSecret      string
	cookieSecure   bool
	allowedOrigin  string
	authRatePerMin int
	authRateBurst  int
	rulesDir       string
	promURL        string
	ruleSyncMode   string
	ruleCRNS       string
	alertmgrURL    string
	webhookToken   string
	csrfSecret     string
	otelEndpoint   string
	otelService    string
	otelInsecure   bool
	esURL          string
	esUsername     string
	esPassword     string
	exportDir      string
	exportBaseURL  string
	redisAddr      string
	redisPassword  string
	notifKeyID     string
	notifEncKey    string
}

func loadConfig() config {
	return config{
		port:           envOr("PORT", _defaultPort),
		databaseURL:    os.Getenv("DATABASE_URL"),
		logLevel:       envOr("LOG_LEVEL", "info"),
		jwtSecret:      os.Getenv("JWT_SECRET"),
		cookieSecure:   envOr("COOKIE_SECURE", "true") != "false",
		allowedOrigin:  os.Getenv("ALLOWED_ORIGIN"),
		authRatePerMin: envIntOr("AUTH_RATE_PER_MINUTE", _authRatePerMinute),
		authRateBurst:  envIntOr("AUTH_RATE_BURST", _authRateBurst),
		rulesDir:       envOr("RULES_DIR", _defaultRulesDir),
		promURL:        envOr("PROMETHEUS_URL", _defaultPromURL),
		// RULE_SYNC_MODE=k8s khi chạy trên Kubernetes (apply PrometheusRule CR);
		// mặc định "file" giữ nguyên hành vi Compose. RULE_CR_NAMESPACE: ns đặt CR.
		ruleSyncMode: envOr("RULE_SYNC_MODE", _ruleSyncFile),
		ruleCRNS:     envOr("RULE_CR_NAMESPACE", _defaultCRNamespace),
		alertmgrURL:  envOr("ALERTMANAGER_URL", _defaultAlertmgrURL),
		webhookToken: os.Getenv("ALERTMANAGER_WEBHOOK_TOKEN"),
		csrfSecret:   os.Getenv("CSRF_SECRET"),
		// OTLP gRPC endpoint (host:port). Rỗng → tracing tắt (dev stack nhẹ).
		otelEndpoint: os.Getenv("OTEL_EXPORTER_OTLP_ENDPOINT"),
		otelService:  envOr("OTEL_SERVICE_NAME", _serviceName),
		otelInsecure: envOr("OTEL_EXPORTER_OTLP_INSECURE", "true") != "false",
		// Elasticsearch log search (GĐ2.8). Rỗng → tắt /logs (stack nhẹ không có ES).
		esURL:         os.Getenv("ELASTICSEARCH_URL"),
		exportDir:     envOr("EXPORT_DIR", "/tmp/logmon-exports"),
		exportBaseURL: envOr("EXPORT_BASE_URL", "/exports"),
		esUsername:    envOr("ELASTICSEARCH_USERNAME", "elastic"),
		esPassword:    os.Getenv("ELASTICSEARCH_PASSWORD"),
		// Notification delivery (GĐ3.2). REDIS_ADDR rỗng → delivery worker tắt
		// (CRUD kênh vẫn chạy). notifEncKey rỗng → derive từ JWT secret (HMAC).
		redisAddr:     os.Getenv("REDIS_ADDR"),
		redisPassword: os.Getenv("REDIS_PASSWORD"),
		notifKeyID:    envOr("NOTIFICATION_KEY_ID", "v1"),
		notifEncKey:   os.Getenv("NOTIFICATION_ENCRYPTION_KEY"),
	}
}

// csrfSecret trả về khoá CSRF: ưu tiên CSRF_SECRET, nếu rỗng thì derive từ JWT
// secret qua HMAC (tách miền khỏi việc ký JWT). Luôn trả chuỗi đủ dài (hex 64 ký tự).
func csrfSecret(cfg config) string {
	if cfg.csrfSecret != "" {
		return cfg.csrfSecret
	}
	mac := hmac.New(sha256.New, []byte(cfg.jwtSecret))
	mac.Write([]byte("logmon-csrf-v1"))
	return hex.EncodeToString(mac.Sum(nil))
}

// alertingDeps gom các thành phần alerting BC dựng ở composition root.
type alertingDeps struct {
	handler         *alertinghttp.Handler
	instanceHandler *alertinghttp.InstanceHandler
	silenceHandler  *alertinghttp.SilenceHandler
}

// buildAlerting dựng alerting BC + rule-sync pipeline: rule CRUD ghi event vào
// outbox; relay (dùng chung) quét outbox và resync rule sang Prometheus qua bus.
// bus được chia sẻ với các BC khác (SLO) — CHỈ một relay quét outbox để tránh
// claim chéo event (SKIP LOCKED) làm handler BC kia bỏ lỡ event.
func buildAlerting(pool *pgxpool.Pool, cfg config, bus *outbox.Bus, ruleApplier *promrule.Applier) alertingDeps {
	store := outbox.NewStore(pool)
	repo := alertingpg.NewRuleRepository(pool)
	txm := alertingpg.NewTxManager(pool)
	publisher := alertingpg.NewEventPublisher(pool, store)
	validator := promql.NewValidator()
	clock := alertingsys.NewClock()
	// Mode k8s: apply PrometheusRule CR (Operator nạp); mode file: render + reload.
	var syncer interface{ Sync(context.Context) error }
	if ruleApplier != nil {
		syncer = alertingpromk8s.NewSyncer(repo, repo, clock, ruleApplier, _alertingRuleCRName, cfg.ruleCRNS)
	} else {
		syncer = promfile.NewSyncer(repo, repo, clock, cfg.rulesDir, cfg.promURL)
	}

	ids := alertingsys.NewUUIDGenerator()
	createRule := command.NewCreateRuleHandler(txm, repo, publisher, validator, ids, clock)
	updateRule := command.NewUpdateRuleHandler(txm, repo, repo, publisher, validator, clock)
	deleteRule := command.NewDeleteRuleHandler(txm, repo, repo, publisher)
	enableRule := command.NewSetRuleEnabledHandler(txm, repo, repo, publisher, clock)
	handler := alertinghttp.NewHandler(createRule, updateRule, deleteRule, enableRule, query.NewRuleQueries(repo), _defaultWorkspaceID)

	// Webhook receiver: Alertmanager → upsert alert_instances (GĐ2.3) + ack (GĐ2.4).
	instanceRepo := alertingpg.NewInstanceRepository(pool)
	ingest := command.NewIngestWebhookHandler(txm, instanceRepo, ids, clock)
	acknowledge := command.NewAcknowledgeHandler(txm, instanceRepo, instanceRepo, clock)
	instanceHandler := alertinghttp.NewInstanceHandler(ingest, acknowledge, instanceRepo, _defaultWorkspaceID)

	// Silence proxy (GĐ2.4b): tạo/xoá/liệt kê silence trên Alertmanager — AM là
	// source of truth, LogMon không lưu silence và không reimplement matcher/expiry.
	silenceGW := alertmanager.NewClient(cfg.alertmgrURL)
	createSilence := command.NewCreateSilenceHandler(silenceGW, clock)
	deleteSilence := command.NewDeleteSilenceHandler(silenceGW)
	silenceHandler := alertinghttp.NewSilenceHandler(createSilence, deleteSilence, query.NewSilenceQueries(silenceGW))

	resync := func(ctx context.Context, _ outbox.Event) error { return syncer.Sync(ctx) }
	bus.Subscribe(alertingdomain.EventAlertRuleCreated, resync)
	bus.Subscribe(alertingdomain.EventAlertRuleUpdated, resync)
	bus.Subscribe(alertingdomain.EventAlertRuleDeleted, resync)

	return alertingDeps{
		handler:         handler,
		instanceHandler: instanceHandler,
		silenceHandler:  silenceHandler,
	}
}

// sloDeps gom các thành phần slo BC dựng ở composition root.
type sloDeps struct {
	handler     *slohttp.Handler
	snapshotJob *slosnapshot.Job
}

// buildSLO dựng slo BC (GĐ3.1): SLO CRUD ghi event vào outbox; relay dùng chung
// resync recording + MWMB rules sang Prometheus (file riêng). Budget snapshot job
// query Prometheus định kỳ ghi slo_snapshots + phát BudgetExhausted.
func buildSLO(pool *pgxpool.Pool, cfg config, bus *outbox.Bus, log *logger.Logger, ruleApplier *promrule.Applier) sloDeps {
	store := outbox.NewStore(pool)
	repo := slopg.NewSLORepository(pool)
	snapRepo := slopg.NewSnapshotRepository(pool)
	txm := slopg.NewTxManager(pool)
	publisher := slopg.NewEventPublisher(pool, store)
	clock := slosys.NewClock()
	ids := slosys.NewUUIDGenerator()
	var syncer interface{ Sync(context.Context) error }
	if ruleApplier != nil {
		syncer = slopromk8s.NewSyncer(repo, repo, clock, ruleApplier, _sloRuleCRName, cfg.ruleCRNS)
	} else {
		syncer = slopromfile.NewSyncer(repo, repo, clock, cfg.rulesDir, cfg.promURL)
	}

	createSLO := slocommand.NewCreateSLOHandler(txm, repo, publisher, ids, clock)
	updateSLO := slocommand.NewUpdateSLOHandler(txm, repo, repo, publisher, clock)
	deleteSLO := slocommand.NewDeleteSLOHandler(txm, repo, repo, publisher)
	queries := sloquery.NewSLOQueries(repo, snapRepo)
	handler := slohttp.NewHandler(createSLO, updateSLO, deleteSLO, queries, _defaultWorkspaceID)

	resync := func(ctx context.Context, _ outbox.Event) error { return syncer.Sync(ctx) }
	bus.Subscribe(slodomain.EventSLODefined, resync)
	bus.Subscribe(slodomain.EventSLOUpdated, resync)
	bus.Subscribe(slodomain.EventSLODeleted, resync)

	querier := sloprom.NewClient(cfg.promURL)
	job := slosnapshot.NewJob(repo, querier, snapRepo, txm, publisher, clock, sloLog{log})

	return sloDeps{handler: handler, snapshotJob: job}
}

// sloLog adapt shared logger sang slosnapshot.Logger (msg, kv...) — gộp kv vào
// message vì shared logger chỉ nhận 1 cặp key/value.
type sloLog struct{ log *logger.Logger }

func (l sloLog) Info(msg string, kv ...any) { l.log.Info(context.Background(), joinKV(msg, kv...)) }
func (l sloLog) Error(msg string, kv ...any) {
	l.log.Error(context.Background(), nil, joinKV(msg, kv...))
}

func joinKV(msg string, kv ...any) string {
	out := msg
	for i := 0; i+1 < len(kv); i += 2 {
		out += fmt.Sprintf(" %v=%v", kv[i], kv[i+1])
	}
	return out
}

// notifDeps gom các thành phần notification BC dựng ở composition root. worker +
// queue nil khi REDIS_ADDR rỗng (delivery tắt, CRUD vẫn chạy).
type notifDeps struct {
	handler *notifhttp.Handler
	worker  *notifworker.Worker
	queue   *redisqueue.Queue
	send    *notify.SendHandler // nil khi delivery tắt; dùng wire incident→notification
}

// notifEncKey trả key mã hóa config kênh: ưu tiên NOTIFICATION_ENCRYPTION_KEY,
// rỗng thì derive từ JWT secret qua HMAC (tách miền khỏi JWT/CSRF).
func notifEncKey(cfg config) []byte {
	if cfg.notifEncKey != "" {
		return crypto.KeyFromPassphrase(cfg.notifEncKey)
	}
	mac := hmac.New(sha256.New, []byte(cfg.jwtSecret))
	mac.Write([]byte("logmon-notif-enc-v1"))
	return mac.Sum(nil)
}

// buildNotification dựng notification BC (GĐ3.2): CRUD kênh (config mã hóa
// AES-GCM at-rest) + delivery qua Redis Streams. Subscribe BudgetExhausted (SLO)
// → render template → enqueue; worker tiêu thụ → Sender gửi → ghi history.
// rdb nil → worker/queue nil (delivery tắt). Trả lỗi nếu cipher init lỗi.
func buildNotification(pool *pgxpool.Pool, cfg config, bus *outbox.Bus, log *logger.Logger, rdb *redis.Client) (notifDeps, error) {
	cipher, err := crypto.NewCipher(cfg.notifKeyID, notifEncKey(cfg))
	if err != nil {
		return notifDeps{}, fmt.Errorf("init notification cipher: %w", err)
	}
	repo := notifpg.NewChannelRepository(pool, cipher)
	histRepo := notifpg.NewHistoryRepository(pool)
	txm := notifpg.NewTxManager(pool)
	ids := notifsys.NewUUIDGenerator()
	clock := notifsys.NewClock()
	senders := sender.Registry()

	create := notifcommand.NewCreateChannelHandler(txm, repo, ids, clock)
	update := notifcommand.NewUpdateChannelHandler(txm, repo, repo, clock)
	del := notifcommand.NewDeleteChannelHandler(txm, repo)
	tester := notify.NewTestHandler(repo, senders)
	handler := notifhttp.NewHandler(create, update, del, tester, repo, histRepo, _defaultWorkspaceID)

	if rdb == nil {
		log.Info(context.Background(), "REDIS_ADDR chưa set — notification delivery worker tắt (CRUD kênh vẫn chạy)")
		return notifDeps{handler: handler}, nil
	}

	queue := redisqueue.New(rdb, _serviceName, nil)
	sendH, err := notify.NewSendHandler(repo, queue, notifLog{log})
	if err != nil {
		return notifDeps{}, fmt.Errorf("init send handler: %w", err)
	}
	worker := notifworker.NewWorker(queue, queue, senders, histRepo, clock, log)
	bus.Subscribe(slodomain.EventBudgetExhausted, budgetExhaustedToSend(sendH))

	return notifDeps{handler: handler, worker: worker, queue: queue, send: sendH}, nil
}

// budgetExhaustedToSend map outbox event BudgetExhausted → SendInput (slo_budget_warning).
func budgetExhaustedToSend(sendH *notify.SendHandler) outbox.Handler {
	return func(ctx context.Context, e outbox.Event) error {
		var p slodomain.BudgetExhaustedPayload
		if err := json.Unmarshal(e.Payload, &p); err != nil {
			return fmt.Errorf("unmarshal budget exhausted payload: %w", err)
		}
		return sendH.Handle(ctx, notify.SendInput{
			WorkspaceID: p.WorkspaceID,
			EventType:   notifdomain.EventSLOBudgetWarning,
			EventRef:    p.SLOID,
			Data: map[string]string{
				"sloName":         p.SLOID,
				"service":         p.Service,
				"budgetRemaining": fmt.Sprintf("%.2f%%", p.BudgetRemainingPercent),
			},
			DedupKey: "slo-budget-" + p.SLOID,
		})
	}
}

// notifLog adapt shared logger sang notify.Logger (Warn) — không có Warn riêng,
// map sang Info có tiền tố.
type notifLog struct{ log *logger.Logger }

func (l notifLog) Warn(ctx context.Context, msg string) { l.log.Info(ctx, "WARN: "+msg) }

// incidentDeps gom các thành phần incident BC dựng ở composition root.
type incidentDeps struct {
	handler           *incidenthttp.Handler
	oncallHandler     *incidenthttp.OnCallHandler
	postmortemHandler *incidenthttp.PostmortemHandler
	escalation        *incidentcommand.EscalationService         // nil khi delivery tắt
	reminder          *incidentcommand.PostmortemReminderService // auto 24h Resolved→PMP
}

// buildIncident dựng incident BC (GĐ3.3-3.4): state machine 7 trạng thái + timeline
// (transactional outbox) + metrics MTTA/MTTR (registry dùng chung) + on-call &
// escalation (doc_v2/06 §1.4). Subscribe BudgetExhausted (SLO) → auto-create SEV2
// incident (dedup theo sloId). IncidentCreated/Resolved phát ra outbox để
// notification gửi (wire ở run()). Escalation runner chỉ bật khi sendH != nil
// (cần Notification Hub để giao thông báo escalation).
func buildIncident(pool *pgxpool.Pool, bus *outbox.Bus, reg prometheus.Registerer, sendH *notify.SendHandler) incidentDeps {
	store := outbox.NewStore(pool)
	repo := incidentpg.NewIncidentRepository(pool)
	tlRepo := incidentpg.NewTimelineRepository(pool)
	txm := incidentpg.NewTxManager(pool)
	publisher := incidentpg.NewEventPublisher(pool, store)
	mx := incidentmetrics.New(reg)
	ids := incidentsys.NewUUIDGenerator()
	clock := incidentsys.NewClock()

	create := incidentcommand.NewCreateIncidentHandler(txm, repo, repo, tlRepo, publisher, mx, ids, clock)
	trans := incidentcommand.NewTransitionHandler(txm, repo, repo, tlRepo, publisher, mx, ids, clock)
	queries := incidentquery.NewIncidentQueries(repo, tlRepo)
	handler := incidenthttp.NewHandler(create, trans, queries, _defaultWorkspaceID)

	// On-call & escalation (GĐ3.4): schedule/override/policy CRUD + "ai on-call".
	schedRepo := incidentpg.NewScheduleRepo(pool)
	ovRepo := incidentpg.NewOverrideRepo(pool)
	polRepo := incidentpg.NewEscalationPolicyRepo(pool)
	stateRepo := incidentpg.NewEscalationStateRepo(pool)
	createSched := incidentcommand.NewCreateScheduleHandler(schedRepo, ids, clock)
	createOv := incidentcommand.NewCreateOverrideHandler(ovRepo, schedRepo, ids, clock)
	createPol := incidentcommand.NewCreateEscalationPolicyHandler(polRepo, ids)
	oncallQ := incidentquery.NewOnCallQueries(schedRepo, ovRepo)
	oncallHandler := incidenthttp.NewOnCallHandler(createSched, createOv, createPol, oncallQ, _defaultWorkspaceID)

	// Postmortem & action items (GĐ3.5): submit/publish + reminder auto 24h.
	pmRepo := incidentpg.NewPostmortemRepo(pool)
	aiRepo := incidentpg.NewActionItemRepo(pool)
	pmHandler := incidentcommand.NewPostmortemHandler(repo, pmRepo, pmRepo, aiRepo, aiRepo, ids, clock)
	pmQueries := incidentquery.NewPostmortemQueries(repo, pmRepo, aiRepo)
	postmortemHandler := incidenthttp.NewPostmortemHandler(pmHandler, pmQueries, _defaultWorkspaceID)
	reminder := incidentcommand.NewPostmortemReminderService(repo, trans, clock, 0)

	deps := incidentDeps{
		handler:           handler,
		oncallHandler:     oncallHandler,
		postmortemHandler: postmortemHandler,
		reminder:          reminder,
	}
	if sendH != nil {
		deps.escalation = incidentcommand.NewEscalationService(
			repo, polRepo, schedRepo, ovRepo, stateRepo, stateRepo,
			escalationNotifier{send: sendH}, clock)
	}

	bus.Subscribe(slodomain.EventBudgetExhausted, budgetExhaustedToIncident(create))
	return deps
}

// escalationNotifier adapt notification SendHandler sang ports.EscalationNotifier
// (giao escalation qua Notification Hub, event type incident_escalated).
type escalationNotifier struct{ send *notify.SendHandler }

var _ incidentports.EscalationNotifier = escalationNotifier{}

func (n escalationNotifier) Notify(ctx context.Context, notice incidentports.EscalationNotice) error {
	return n.send.Handle(ctx, notify.SendInput{
		WorkspaceID: notice.WorkspaceID,
		EventType:   notifdomain.EventIncidentEscalated,
		EventRef:    notice.IncidentID,
		Data: map[string]string{
			"title":     notice.Title,
			"service":   notice.Service,
			"severity":  notice.Severity,
			"target":    notice.Target,
			"recipient": notice.Recipient,
			"level":     strconv.Itoa(notice.Level),
		},
		DedupKey: fmt.Sprintf("escalation-%s-%d", notice.IncidentID, notice.Level),
	})
}

// runEscalation quét escalation định kỳ (stop qua ctx) — escalate incident chưa
// ack tới bậc kế khi quá timeout (doc_v2/06 §1.4).
func runEscalation(ctx context.Context, svc *incidentcommand.EscalationService, log *logger.Logger) {
	ticker := time.NewTicker(_escalationSweepInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			res, err := svc.Sweep(ctx)
			if err != nil {
				log.Error(ctx, err, "escalation sweep")
				continue
			}
			if res.Escalated > 0 {
				log.Infof(ctx, "escalation sweep", "escalated", strconv.Itoa(res.Escalated))
			}
		}
	}
}

// runPostmortemReminder quét định kỳ (stop qua ctx) — auto chuyển incident SEV1/2
// đã Resolved quá 24h sang PostmortemPending (doc_v2/06 §1.5).
func runPostmortemReminder(ctx context.Context, svc *incidentcommand.PostmortemReminderService, log *logger.Logger) {
	ticker := time.NewTicker(_postmortemSweepInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			flagged, err := svc.Sweep(ctx)
			if err != nil {
				log.Error(ctx, err, "postmortem reminder sweep")
				continue
			}
			if flagged > 0 {
				log.Infof(ctx, "postmortem reminder sweep", "flagged", strconv.Itoa(flagged))
			}
		}
	}
}

// reportingDeps gom thành phần reporting BC (GĐ4.3).
type reportingDeps struct {
	handler   *reporthttp.Handler
	worker    *reportworker.ExportWorker
	scheduler *reportworker.ReportScheduler
}

// buildReporting dựng reporting BC: scheduled reports + async export. Generator/
// exporter/delivery là adapter dev (CSV/local FS) — bản PDF/S3/real-query là nợ GĐ4.
func buildReporting(pool *pgxpool.Pool, exportDir, exportBaseURL string) reportingDeps {
	ids := usersys.NewUUIDGenerator()
	clock := usersys.NewClock()
	schedRepo := reportpg.NewScheduleRepository(pool)
	jobRepo := reportpg.NewExportJobRepository(pool)
	blobs := reportsys.NewLocalBlobStore(exportDir, exportBaseURL)

	schedSvc := reportcommand.NewScheduleService(schedRepo, reportsys.Cron{}, ids, clock)
	exportSvc := reportcommand.NewExportService(jobRepo, ids, clock)
	queries := reportquery.NewQueries(schedRepo, jobRepo)

	return reportingDeps{
		handler:   reporthttp.NewHandler(schedSvc, exportSvc, queries, blobs),
		worker:    reportworker.NewExportWorker(jobRepo, reportsys.CSVExporter{}, blobs, clock),
		scheduler: reportworker.NewReportScheduler(schedRepo, schedRepo, reportsys.Cron{}, reportsys.CSVGenerator{}, reportsys.LogDelivery{}, clock),
	}
}

// runExportWorker tiêu thụ export job pending định kỳ (stop qua ctx). Mỗi tick
// xử lý liên tục đến khi hàng đợi trống.
func runExportWorker(ctx context.Context, w *reportworker.ExportWorker, log *logger.Logger) {
	ticker := time.NewTicker(_exportWorkerInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			for {
				processed, err := w.ProcessOne(ctx)
				if err != nil {
					log.Error(ctx, err, "export worker")
				}
				if !processed {
					break
				}
			}
		}
	}
}

// runReportScheduler chạy report schedule đến hạn định kỳ (stop qua ctx).
func runReportScheduler(ctx context.Context, s *reportworker.ReportScheduler, log *logger.Logger) {
	ticker := time.NewTicker(_reportSchedulerInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			res, err := s.Sweep(ctx)
			if err != nil {
				log.Error(ctx, err, "report scheduler sweep")
			}
			if res.Ran > 0 {
				log.Infof(ctx, "report scheduler sweep", "ran", strconv.Itoa(res.Ran))
			}
		}
	}
}

// budgetExhaustedToIncident map outbox event BudgetExhausted → auto-create SEV2
// incident (doc_v2/06 §1.1-1.2). Idempotent: handler dedup theo source+sloId.
func budgetExhaustedToIncident(create *incidentcommand.CreateIncidentHandler) outbox.Handler {
	return func(ctx context.Context, e outbox.Event) error {
		var p slodomain.BudgetExhaustedPayload
		if err := json.Unmarshal(e.Payload, &p); err != nil {
			return fmt.Errorf("unmarshal budget exhausted payload: %w", err)
		}
		_, err := create.Handle(ctx, incidentcommand.CreateIncidentInput{
			WorkspaceID: p.WorkspaceID,
			Title:       fmt.Sprintf("SLO budget exhausted: %s", p.Service),
			Description: fmt.Sprintf("Error budget còn %.2f%% cho service %s", p.BudgetRemainingPercent, p.Service),
			Service:     p.Service,
			Severity:    incidentdomain.SEV2.String(),
			Source:      incidentdomain.SourceSLOBudget.String(),
			SourceRef:   p.SLOID,
			Actor:       "system",
		})
		return err
	}
}

// incidentToSend map outbox event incident (Created/Resolved) → notification send
// với event type tương ứng (đóng vòng incident → thông báo đa kênh).
func incidentToSend(sendH *notify.SendHandler, notifEventType string) outbox.Handler {
	return func(ctx context.Context, e outbox.Event) error {
		var p incidentdomain.IncidentPayload
		if err := json.Unmarshal(e.Payload, &p); err != nil {
			return fmt.Errorf("unmarshal incident payload: %w", err)
		}
		return sendH.Handle(ctx, notify.SendInput{
			WorkspaceID: p.WorkspaceID,
			EventType:   notifEventType,
			EventRef:    p.IncidentID,
			Data: map[string]string{
				"title":    p.Title,
				"service":  p.Service,
				"severity": p.Severity,
				"status":   p.Status,
			},
			DedupKey: "incident-" + p.IncidentID + "-" + p.Status,
		})
	}
}

// envIntOr đọc một biến môi trường số nguyên dương; rỗng/không hợp lệ → fallback.
func envIntOr(key string, fallback int) int {
	if v, err := strconv.Atoi(os.Getenv(key)); err == nil && v > 0 {
		return v
	}
	return fallback
}

func run() error {
	cfg := loadConfig()
	log := logger.New(os.Stdout, cfg.logLevel)

	if cfg.databaseURL == "" {
		return errors.New("DATABASE_URL not configured")
	}

	jwtSvc, err := auth.NewJWTService(cfg.jwtSecret, _defaultJWTTTL)
	if err != nil {
		return fmt.Errorf("init jwt: %w", err)
	}

	// CSRF dùng khoá riêng tách miền với JWT (domain separation): CSRF_SECRET nếu
	// có, ngược lại derive từ JWT secret qua HMAC để không cần thêm secret bắt buộc.
	csrf, err := auth.NewCSRFProtector(csrfSecret(cfg))
	if err != nil {
		return fmt.Errorf("init csrf: %w", err)
	}

	if !cfg.cookieSecure {
		log.Info(context.Background(), "WARNING: COOKIE_SECURE=false — chỉ dùng cho local dev, KHÔNG dùng production")
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	// Tracing: xuất span qua OTLP gRPC sang OTel agent (doc_v2/04 §2). Endpoint
	// rỗng → no-op. Shutdown flush span còn lại trước khi thoát.
	tp, err := tracing.New(ctx, tracing.Config{
		ServiceName: cfg.otelService,
		Endpoint:    cfg.otelEndpoint,
		Insecure:    cfg.otelInsecure,
	})
	if err != nil {
		return fmt.Errorf("init tracing: %w", err)
	}
	defer func() {
		shutdownCtx, cancel := context.WithTimeout(context.Background(), _shutdownTimeout)
		defer cancel()
		if err := tp.Shutdown(shutdownCtx); err != nil {
			log.Error(context.Background(), err, "tracer shutdown")
		}
	}()
	if tp.Enabled() {
		log.Infof(context.Background(), "tracing enabled", "endpoint", cfg.otelEndpoint)
	}

	connectCtx, cancel := context.WithTimeout(ctx, _dbConnectTimeout)
	defer cancel()
	poolCfg, err := pgxpool.ParseConfig(cfg.databaseURL)
	if err != nil {
		return fmt.Errorf("parse database url: %w", err)
	}
	// otelpgx: span cho mỗi query (dùng global TracerProvider — no-op khi tracing tắt).
	poolCfg.ConnConfig.Tracer = otelpgx.NewTracer()
	pool, err := pgxpool.NewWithConfig(connectCtx, poolCfg)
	if err != nil {
		return fmt.Errorf("connect postgres: %w", err)
	}
	defer pool.Close()
	if err := pool.Ping(connectCtx); err != nil {
		return fmt.Errorf("ping postgres: %w", err)
	}

	mx := metrics.New()
	userIDs := usersys.NewUUIDGenerator()
	userClock := usersys.NewClock()
	svc := userapp.NewService(
		userpg.NewRepository(pool),
		usersys.NewArgon2idHasher(),
		userIDs,
		userClock,
		jwtSvc,
		userapp.WithLogger(log),
	)
	refreshSvc := userapp.NewRefreshService(
		userpg.NewRefreshRepository(pool),
		usersys.NewRefreshCodec(),
		jwtSvc,
		userIDs,
		userClock,
		_defaultRefreshTTL,
		log,
	)

	// Rule sync mode k8s (Phase III C3): apply PrometheusRule CR thay vì render
	// file + reload. Applier dùng chung cho alerting + slo (1 in-cluster client).
	var ruleApplier *promrule.Applier
	if cfg.ruleSyncMode == _ruleSyncK8s {
		ruleApplier, err = promrule.NewInClusterApplier()
		if err != nil {
			return fmt.Errorf("init rule-sync applier (k8s): %w", err)
		}
		log.Infof(ctx, "rule sync mode=k8s (PrometheusRule CR)", "namespace", cfg.ruleCRNS)
	}

	// Một bus + một relay dùng chung cho mọi BC (alerting + slo) — chỉ một relay
	// quét outbox để tránh claim chéo event (SKIP LOCKED) làm BC kia bỏ lỡ sync.
	bus := outbox.NewBus()
	alerting := buildAlerting(pool, cfg, bus, ruleApplier)
	slo := buildSLO(pool, cfg, bus, log, ruleApplier)

	// Notification delivery (GĐ3.2): Redis Streams queue + worker. REDIS_ADDR rỗng
	// → delivery tắt (CRUD kênh vẫn chạy). Đăng ký BC TRƯỚC khi relay.Run để
	// subscription (BudgetExhausted → send) sẵn sàng khi event đầu tiên dispatch.
	var rdb *redis.Client
	if cfg.redisAddr != "" {
		rdb = redis.NewClient(&redis.Options{Addr: cfg.redisAddr, Password: cfg.redisPassword})
		defer func() {
			if err := rdb.Close(); err != nil {
				log.Error(context.Background(), err, "close redis")
			}
		}()
	}
	notif, err := buildNotification(pool, cfg, bus, log, rdb)
	if err != nil {
		return err
	}

	// Incident BC (GĐ3.3): state machine + MTTA/MTTR. Subscribe BudgetExhausted
	// → auto-create SEV2. Đăng ký TRƯỚC relay.Run để subscription sẵn sàng.
	incident := buildIncident(pool, bus, mx.Registry(), notif.send)
	// Đóng vòng incident → notification: IncidentCreated/Resolved phát ra outbox,
	// relay dispatch → send qua kênh đã đăng ký (chỉ khi delivery bật).
	if notif.send != nil {
		bus.Subscribe(incidentdomain.EventIncidentCreated, incidentToSend(notif.send, notifdomain.EventIncidentCreated))
		bus.Subscribe(incidentdomain.EventIncidentResolved, incidentToSend(notif.send, notifdomain.EventIncidentResolved))
	}

	relay := outbox.NewRelay(outbox.NewStore(pool), bus.Dispatch)
	go relay.Run(ctx)
	go slo.snapshotJob.Run(ctx) // budget snapshot định kỳ (stop qua ctx)
	if incident.escalation != nil {
		go runEscalation(ctx, incident.escalation, log) // escalation sweep định kỳ
	}
	go runPostmortemReminder(ctx, incident.reminder, log) // auto 24h Resolved→PMP
	if notif.worker != nil {
		if err := notif.queue.EnsureGroup(ctx); err != nil {
			return fmt.Errorf("ensure delivery group: %w", err)
		}
		go notif.worker.Run(ctx) // tiêu thụ delivery queue (stop qua ctx)
	}

	// Reporting (GĐ4.3): scheduled reports + async export. Runner nền tiêu thụ
	// export job + chạy schedule đến hạn (stop qua ctx).
	reporting := buildReporting(pool, cfg.exportDir, cfg.exportBaseURL)
	go runExportWorker(ctx, reporting.worker, log)
	go runReportScheduler(ctx, reporting.scheduler, log)

	// Usage + quota (GĐ4.5): usage thực tế đọc từ Prometheus (rỗng → usage = 0,
	// degrade an toàn). Quota lưu Postgres.
	var usageReader usageports.UsageReader
	if cfg.promURL != "" {
		usageReader = usageprom.NewReader(cfg.promURL)
	}
	usageHandler := usagehttp.NewHandler(
		usageapp.NewService(usagepg.NewQuotaRepository(pool), usageReader, usersys.NewClock()),
	)

	// Service topology (GĐ4.4): cạnh phụ thuộc suy từ traces (ES). Cache graph
	// materialize: Redis (multi-instance) nếu có rdb, ngược lại in-memory. esURL
	// rỗng → reader nil → graph rỗng (degrade an toàn).
	var topoReader topoports.DependencyReader
	if cfg.esURL != "" {
		topoReader = topoes.NewReader(cfg.esURL).WithBasicAuth(cfg.esUsername, cfg.esPassword)
	}
	var topoCache topoports.GraphCache
	if rdb != nil {
		topoCache = topocache.NewRedis(rdb)
	} else {
		topoCache = topocache.NewMemory(nil)
	}
	topologyHandler := topohttp.NewHandler(
		topoapp.NewService(topoReader, topoCache, usersys.NewClock()),
	)

	// Log search (GĐ2.8): truy vấn data stream logs-* trên Elasticsearch. Optional —
	// ELASTICSEARCH_URL rỗng (stack nhẹ) → không đăng ký /logs.
	var logHandler *loghttp.LogHandler
	if cfg.esURL != "" {
		esClient := elasticsearch.NewClient(cfg.esURL, cfg.esUsername, cfg.esPassword)
		logHandler = loghttp.NewLogHandler(logquery.NewLogQueries(esClient))
	} else {
		log.Info(context.Background(), "ELASTICSEARCH_URL chưa set — log search API (/api/v1/logs) tắt")
	}

	// Pipeline management (GĐ3.7): config + DLQ luôn bật (Postgres); ILM/health/
	// datastreams chỉ khi có ES. DLQ replayer chưa wire (retry đánh dấu state — nợ).
	pipeClock := usersys.NewClock()
	pipeCfgRepo := logpg.NewConfigRepository(pool)
	dlqRepo := logpg.NewDLQRepository(pool)
	var (
		ilmApplier logports.ILMApplier
		pipeHealth logports.PipelineHealth
		dsReader   logports.DataStreamReader
	)
	if cfg.esURL != "" {
		mgmt := elasticsearch.NewManagement(cfg.esURL, cfg.esUsername, cfg.esPassword)
		ilmApplier, pipeHealth, dsReader = mgmt, mgmt, mgmt
	}
	pipelineHandler := loghttp.NewPipelineHandler(
		logcommand.NewPipelineCommands(pipeCfgRepo, ilmApplier, pipeClock),
		logcommand.NewDLQCommands(dlqRepo, nil, pipeClock),
		logquery.NewPipelineQueries(pipeCfgRepo, dlqRepo, pipeHealth, dsReader, pipeClock),
	)

	if cfg.webhookToken == "" {
		log.Info(context.Background(), "WARNING: ALERTMANAGER_WEBHOOK_TOKEN chưa set — webhook receiver fail-closed (mọi POST /alerts/webhook trả 401)")
	}

	router := buildRouter(log, mx, svc, refreshSvc, alerting, slo, notif, incident, logHandler, pipelineHandler, reporting.handler, usageHandler, topologyHandler, pool, jwtSvc, csrf, cfg.otelService, cfg.cookieSecure, cfg.allowedOrigin,
		cfg.authRatePerMin, cfg.authRateBurst, cfg.webhookToken)

	srv := &http.Server{
		Addr:              ":" + cfg.port,
		Handler:           router,
		ReadHeaderTimeout: _readHeaderTimeout,
	}

	errCh := make(chan error, 1)
	go func() {
		log.Infof(ctx, "userservice listening", "addr", srv.Addr)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			errCh <- err
		}
	}()

	select {
	case err := <-errCh:
		return fmt.Errorf("http server: %w", err)
	case <-ctx.Done():
		log.Info(context.Background(), "shutdown signal received")
	}

	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), _shutdownTimeout)
	defer shutdownCancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		return fmt.Errorf("graceful shutdown: %w", err)
	}
	relay.Wait() // ctx đã hủy → relay thoát; chờ goroutine kết thúc hẳn
	return nil
}

func buildRouter(
	log *logger.Logger,
	mx *metrics.Metrics,
	svc *userapp.Service,
	refreshSvc *userapp.RefreshService,
	alerting alertingDeps,
	slo sloDeps,
	notif notifDeps,
	incident incidentDeps,
	logHandler *loghttp.LogHandler,
	pipelineHandler *loghttp.PipelineHandler,
	reportingHandler *reporthttp.Handler,
	usageHandler *usagehttp.Handler,
	topologyHandler *topohttp.Handler,
	pool *pgxpool.Pool,
	jwtSvc *auth.JWTService,
	csrf *auth.CSRFProtector,
	serviceName string,
	cookieSecure bool,
	allowedOrigin string,
	authRatePerMin, authRateBurst int,
	webhookToken string,
) *gin.Engine {
	gin.SetMode(gin.ReleaseMode)
	r := gin.New()
	r.Use(
		middleware.Recovery(log),
		// otelgin: tạo server span cho mỗi request (#2 trong chain, doc_v2/04 §2.1).
		// Bỏ qua /healthz + /metrics để không nhiễu trace probe/scrape.
		otelgin.Middleware(serviceName, otelgin.WithGinFilter(shouldTrace)),
		middleware.TraceID(),
		middleware.CORS(allowedOrigin),
		middleware.SecurityHeaders(),
		middleware.Metrics(mx),
		middleware.Logging(log),
	)

	// Liveness: process còn sống (KHÔNG ping dep — tránh restart oan). Readiness:
	// ping Postgres (dep tới hạn) để LB/K8s ngừng route traffic khi DB chưa sẵn sàng.
	r.GET("/healthz", health.Liveness())
	r.GET("/readyz", health.Readiness(2*time.Second,
		health.Check{Name: "postgres", Ping: pool.Ping},
	))
	r.GET("/metrics", gin.WrapH(promhttp.HandlerFor(mx.Registry(), promhttp.HandlerOpts{})))

	api := r.Group("/api/v1")
	// CSRF double-submit cho mọi route mutating dùng cookie session. Miễn trừ:
	// register/login (chưa có session), refresh (bảo vệ bằng refresh cookie HttpOnly
	// + SameSite), webhook (xác thực bearer token, không dùng cookie).
	api.Use(csrf.Middleware(
		"/api/v1/users",
		"/api/v1/auth/login",
		"/api/v1/auth/refresh",
		"/api/v1/alerts/webhook",
	))
	// Rate limit per workspace (GĐ4.5, doc_v2/07 §3): khóa theo header X-Workspace-ID
	// (rỗng → bỏ qua, vd login/me). In-memory single-instance; prod→redis_rate (nợ).
	wsRate := middleware.NewPerMinuteLimiter(_wsRatePerMinute, _wsRateBurst)
	api.Use(wsRate.KeyedMiddleware(func(c *gin.Context) string {
		return c.GetHeader(auth.WorkspaceHeader)
	}))
	handler := userhttp.NewHandler(svc, refreshSvc, csrf, userhttp.CookieConfig{
		Secure:               cookieSecure,
		MaxAgeSeconds:        int(_defaultJWTTTL.Seconds()),
		RefreshMaxAgeSeconds: int(_defaultRefreshTTL.Seconds()),
	})
	authRate := middleware.NewPerMinuteLimiter(authRatePerMin, authRateBurst)
	handler.Register(api, auth.RequireAuth(jwtSvc), authRate.Middleware())

	// Identity multi-tenancy (GĐ3.6): workspace + member + RBAC. Stateless ids/clock
	// dựng tại chỗ từ pool. resolver được dùng cho cả route quản lý workspace lẫn
	// middleware tenant của các BC khác.
	wsRepo := userpg.NewWorkspaceRepository(pool)
	memRepo := userpg.NewMembershipRepository(pool)
	wsSvc := userapp.NewWorkspaceService(wsRepo, wsRepo, memRepo, usersys.NewUUIDGenerator(), usersys.NewClock())
	memSvc := userapp.NewMemberService(memRepo, memRepo, userpg.NewRepository(pool), usersys.NewClock())
	resolver := userapp.NewMembershipResolver(memRepo)
	auditor := audit.NewPostgresRecorder(pool)
	userhttp.NewWorkspaceHandler(wsSvc, memSvc, resolver, auditor).Register(api, auth.RequireAuth(jwtSvc))

	// tenantMW = xác thực + validate membership từ header X-Workspace-ID, gắn
	// workspace+role vào context. Mọi BC tenant-scoped dùng làm authMW (isolation).
	tenantMW := auth.RequireAuthWorkspace(jwtSvc, resolver)
	alerting.handler.Register(api, tenantMW)
	// instanceHandler: route người dùng (ack/list) → tenantMW; webhook ingest dùng
	// bearer token (machine-auth, không có header workspace → wsID fallback default).
	alerting.instanceHandler.Register(api, tenantMW, auth.RequireBearerToken(webhookToken))
	alerting.silenceHandler.Register(api, tenantMW)
	slo.handler.Register(api, tenantMW)
	notif.handler.Register(api, tenantMW)
	incident.handler.Register(api, tenantMW)
	incident.oncallHandler.Register(api, tenantMW)
	incident.postmortemHandler.Register(api, tenantMW)
	if logHandler != nil {
		logHandler.Register(api, tenantMW)
	}
	pipelineHandler.Register(api, tenantMW)
	reportingHandler.Register(api, tenantMW)
	usageHandler.Register(api, tenantMW)
	topologyHandler.Register(api, tenantMW)
	return r
}

// shouldTrace báo otelgin có tạo span cho request không — bỏ qua probe/scrape
// (/healthz, /readyz, /metrics) vì chúng tần suất cao và không mang giá trị chẩn đoán.
func shouldTrace(c *gin.Context) bool {
	switch c.Request.URL.Path {
	case "/healthz", "/readyz", "/metrics":
		return false
	default:
		return true
	}
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
