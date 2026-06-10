// Command userservice là HTTP service quản lý user của LogMon. main() chỉ gọi
// run() và exit một lần — toàn bộ logic khởi tạo + graceful shutdown nằm trong run.
package main

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/prometheus/client_golang/prometheus/promhttp"

	"github.com/namdam97/logmon/backend/internal/shared/auth"
	"github.com/namdam97/logmon/backend/internal/shared/logger"
	"github.com/namdam97/logmon/backend/internal/shared/metrics"
	"github.com/namdam97/logmon/backend/internal/shared/middleware"
	userhttp "github.com/namdam97/logmon/backend/internal/user/adapters/http"
	userpg "github.com/namdam97/logmon/backend/internal/user/adapters/postgres"
	usersys "github.com/namdam97/logmon/backend/internal/user/adapters/system"
	userapp "github.com/namdam97/logmon/backend/internal/user/app"
)

const (
	_defaultPort       = "8080"
	_shutdownTimeout   = 10 * time.Second
	_readHeaderTimeout = 5 * time.Second
	_dbConnectTimeout  = 10 * time.Second
	_defaultJWTTTL     = 24 * time.Hour
	_authRatePerMinute = 10 // mặc định: giới hạn login/register mỗi IP
	_authRateBurst     = 5
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
	bcryptCost     int
	jwtSecret      string
	cookieSecure   bool
	allowedOrigin  string
	authRatePerMin int
	authRateBurst  int
}

func loadConfig() config {
	cost, _ := strconv.Atoi(os.Getenv("BCRYPT_COST"))
	return config{
		port:           envOr("PORT", _defaultPort),
		databaseURL:    os.Getenv("DATABASE_URL"),
		logLevel:       envOr("LOG_LEVEL", "info"),
		bcryptCost:     cost,
		jwtSecret:      os.Getenv("JWT_SECRET"),
		cookieSecure:   envOr("COOKIE_SECURE", "true") != "false",
		allowedOrigin:  os.Getenv("ALLOWED_ORIGIN"),
		authRatePerMin: envIntOr("AUTH_RATE_PER_MINUTE", _authRatePerMinute),
		authRateBurst:  envIntOr("AUTH_RATE_BURST", _authRateBurst),
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

	if !cfg.cookieSecure {
		log.Info(context.Background(), "WARNING: COOKIE_SECURE=false — chỉ dùng cho local dev, KHÔNG dùng production")
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	connectCtx, cancel := context.WithTimeout(ctx, _dbConnectTimeout)
	defer cancel()
	pool, err := pgxpool.New(connectCtx, cfg.databaseURL)
	if err != nil {
		return fmt.Errorf("connect postgres: %w", err)
	}
	defer pool.Close()
	if err := pool.Ping(connectCtx); err != nil {
		return fmt.Errorf("ping postgres: %w", err)
	}

	mx := metrics.New()
	svc := userapp.NewService(
		userpg.NewRepository(pool),
		usersys.NewBcryptHasher(cfg.bcryptCost),
		usersys.NewUUIDGenerator(),
		usersys.NewClock(),
		jwtSvc,
	)

	router := buildRouter(log, mx, svc, pool, jwtSvc, cfg.cookieSecure, cfg.allowedOrigin,
		cfg.authRatePerMin, cfg.authRateBurst)

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
	return nil
}

func buildRouter(
	log *logger.Logger,
	mx *metrics.Metrics,
	svc *userapp.Service,
	pool *pgxpool.Pool,
	jwtSvc *auth.JWTService,
	cookieSecure bool,
	allowedOrigin string,
	authRatePerMin, authRateBurst int,
) *gin.Engine {
	gin.SetMode(gin.ReleaseMode)
	r := gin.New()
	r.Use(
		middleware.Recovery(log),
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
	handler := userhttp.NewHandler(svc, userhttp.CookieConfig{
		Secure:        cookieSecure,
		MaxAgeSeconds: int(_defaultJWTTTL.Seconds()),
	})
	authRate := middleware.NewPerMinuteLimiter(authRatePerMin, authRateBurst)
	handler.Register(api, auth.RequireAuth(jwtSvc), authRate.Middleware())
	return r
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
