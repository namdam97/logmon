// Command userservice là HTTP service quản lý user của LogMon. main() chỉ gọi
// run() và exit một lần — toàn bộ logic khởi tạo + graceful shutdown nằm trong run.
package main

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
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
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"go.opentelemetry.io/contrib/instrumentation/github.com/gin-gonic/gin/otelgin"

	"github.com/namdam97/logmon/backend/internal/alerting/adapters/alertmanager"
	alertinghttp "github.com/namdam97/logmon/backend/internal/alerting/adapters/http"
	alertingpg "github.com/namdam97/logmon/backend/internal/alerting/adapters/postgres"
	"github.com/namdam97/logmon/backend/internal/alerting/adapters/promfile"
	"github.com/namdam97/logmon/backend/internal/alerting/adapters/promql"
	alertingsys "github.com/namdam97/logmon/backend/internal/alerting/adapters/system"
	"github.com/namdam97/logmon/backend/internal/alerting/app/command"
	"github.com/namdam97/logmon/backend/internal/alerting/app/query"
	alertingdomain "github.com/namdam97/logmon/backend/internal/alerting/domain"
	"github.com/namdam97/logmon/backend/internal/shared/auth"
	"github.com/namdam97/logmon/backend/internal/shared/logger"
	"github.com/namdam97/logmon/backend/internal/shared/metrics"
	"github.com/namdam97/logmon/backend/internal/shared/middleware"
	"github.com/namdam97/logmon/backend/internal/shared/outbox"
	"github.com/namdam97/logmon/backend/internal/shared/tracing"
	userhttp "github.com/namdam97/logmon/backend/internal/user/adapters/http"
	userpg "github.com/namdam97/logmon/backend/internal/user/adapters/postgres"
	usersys "github.com/namdam97/logmon/backend/internal/user/adapters/system"
	userapp "github.com/namdam97/logmon/backend/internal/user/app"
)

const (
	_serviceName       = "userservice" // service.name trong span + OTEL_SERVICE_NAME default
	_defaultPort       = "8080"
	_shutdownTimeout   = 10 * time.Second
	_readHeaderTimeout = 5 * time.Second
	_dbConnectTimeout  = 10 * time.Second
	_defaultJWTTTL     = 15 * time.Minute    // access token ngắn (ADR-023)
	_defaultRefreshTTL = 14 * 24 * time.Hour // refresh token dài
	_authRatePerMinute = 10                  // mặc định: giới hạn login/register mỗi IP
	_authRateBurst     = 5

	// _defaultWorkspaceID là workspace mặc định GĐ2 (multi-tenancy đầy đủ ở GĐ3).
	_defaultWorkspaceID = "00000000-0000-0000-0000-000000000001"
	_defaultRulesDir    = "/etc/prometheus/generated" // Prometheus mount đọc rule file đã render
	_defaultPromURL     = "http://prometheus:9090"    // cần --web.enable-lifecycle cho /-/reload
	_defaultAlertmgrURL = "http://alertmanager:9093"  // proxy silence (GĐ2.4b)
)

func main() {
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
	alertmgrURL    string
	webhookToken   string
	csrfSecret     string
	otelEndpoint   string
	otelService    string
	otelInsecure   bool
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
		alertmgrURL:    envOr("ALERTMANAGER_URL", _defaultAlertmgrURL),
		webhookToken:   os.Getenv("ALERTMANAGER_WEBHOOK_TOKEN"),
		csrfSecret:     os.Getenv("CSRF_SECRET"),
		// OTLP gRPC endpoint (host:port). Rỗng → tracing tắt (dev stack nhẹ).
		otelEndpoint: os.Getenv("OTEL_EXPORTER_OTLP_ENDPOINT"),
		otelService:  envOr("OTEL_SERVICE_NAME", _serviceName),
		otelInsecure: envOr("OTEL_EXPORTER_OTLP_INSECURE", "true") != "false",
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
	relay           *outbox.Relay
}

// buildAlerting dựng alerting BC + rule-sync pipeline: rule CRUD ghi event vào
// outbox; relay quét outbox và resync toàn bộ rule sang Prometheus qua bus.
func buildAlerting(pool *pgxpool.Pool, cfg config) alertingDeps {
	store := outbox.NewStore(pool)
	repo := alertingpg.NewRuleRepository(pool)
	txm := alertingpg.NewTxManager(pool)
	publisher := alertingpg.NewEventPublisher(pool, store)
	validator := promql.NewValidator()
	clock := alertingsys.NewClock()
	syncer := promfile.NewSyncer(repo, repo, clock, cfg.rulesDir, cfg.promURL)

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

	bus := outbox.NewBus()
	resync := func(ctx context.Context, _ outbox.Event) error { return syncer.Sync(ctx) }
	bus.Subscribe(alertingdomain.EventAlertRuleCreated, resync)
	bus.Subscribe(alertingdomain.EventAlertRuleUpdated, resync)
	bus.Subscribe(alertingdomain.EventAlertRuleDeleted, resync)

	return alertingDeps{
		handler:         handler,
		instanceHandler: instanceHandler,
		silenceHandler:  silenceHandler,
		relay:           outbox.NewRelay(store, bus.Dispatch),
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

	alerting := buildAlerting(pool, cfg)
	go alerting.relay.Run(ctx)

	if cfg.webhookToken == "" {
		log.Info(context.Background(), "WARNING: ALERTMANAGER_WEBHOOK_TOKEN chưa set — webhook receiver fail-closed (mọi POST /alerts/webhook trả 401)")
	}

	router := buildRouter(log, mx, svc, refreshSvc, alerting, pool, jwtSvc, csrf, cfg.otelService, cfg.cookieSecure, cfg.allowedOrigin,
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
	alerting.relay.Wait() // ctx đã hủy → relay thoát; chờ goroutine kết thúc hẳn
	return nil
}

func buildRouter(
	log *logger.Logger,
	mx *metrics.Metrics,
	svc *userapp.Service,
	refreshSvc *userapp.RefreshService,
	alerting alertingDeps,
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

	r.GET("/healthz", func(c *gin.Context) {
		pingCtx, cancel := context.WithTimeout(c.Request.Context(), 2*time.Second)
		defer cancel()
		if err := pool.Ping(pingCtx); err != nil {
			c.JSON(http.StatusServiceUnavailable, gin.H{"status": "unhealthy"})
			return
		}
		c.JSON(http.StatusOK, gin.H{"status": "ok"})
	})
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
	handler := userhttp.NewHandler(svc, refreshSvc, csrf, userhttp.CookieConfig{
		Secure:               cookieSecure,
		MaxAgeSeconds:        int(_defaultJWTTTL.Seconds()),
		RefreshMaxAgeSeconds: int(_defaultRefreshTTL.Seconds()),
	})
	authRate := middleware.NewPerMinuteLimiter(authRatePerMin, authRateBurst)
	handler.Register(api, auth.RequireAuth(jwtSvc), authRate.Middleware())
	alerting.handler.Register(api, auth.RequireAuth(jwtSvc))
	alerting.instanceHandler.Register(api, auth.RequireAuth(jwtSvc), auth.RequireBearerToken(webhookToken))
	alerting.silenceHandler.Register(api, auth.RequireAuth(jwtSvc))
	return r
}

// shouldTrace báo otelgin có tạo span cho request không — bỏ qua probe/scrape
// (/healthz, /metrics) vì chúng tần suất cao và không mang giá trị chẩn đoán.
func shouldTrace(c *gin.Context) bool {
	switch c.Request.URL.Path {
	case "/healthz", "/metrics":
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
