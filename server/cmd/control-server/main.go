package main

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/gin-gonic/gin"
)

type server struct {
	cfg    Config
	db     database
	router *gin.Engine
	log    *slog.Logger
}

func main() {
	cfg := loadConfig()
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	sqlDB, err := openDatabase(ctx, cfg.DatabaseURL)
	if err != nil {
		logger.Error("database initialization failed", "error", err)
		os.Exit(1)
	}
	defer sqlDB.Close()

	s := &server{
		cfg: cfg,
		db:  database{db: sqlDB},
		log: logger,
	}
	s.router = s.routes()

	if cfg.OperatorEnabled {
		go s.runOperator(ctx)
	}

	httpServer := &http.Server{
		Addr:              ":" + cfg.Port,
		Handler:           s.router,
		ReadHeaderTimeout: 10 * time.Second,
	}
	go func() {
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		_ = httpServer.Shutdown(shutdownCtx)
	}()
	logger.Info("control-server listening", "port", cfg.Port, "operator_enabled", cfg.OperatorEnabled)
	if err := httpServer.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		logger.Error("http server failed", "error", err)
		os.Exit(1)
	}
}

func (s *server) routes() *gin.Engine {
	r := gin.New()
	r.Use(gin.Recovery())
	r.Use(gin.Logger())

	r.GET("/healthz", s.healthz)
	r.GET("/readyz", s.readyz)

	api := r.Group("/api/v1")
	api.POST("/policies", s.createPolicy)
	api.POST("/policies/render-preview", s.renderPolicyPreview)
	api.GET("/policies", s.listPolicies)
	api.GET("/policies/:id", s.getPolicy)
	api.PUT("/policies/:id", s.updatePolicy)
	api.DELETE("/policies/:id", s.deletePolicy)
	api.GET("/policies/:id/revisions", s.listRevisions)
	api.POST("/policies/:id/rollback", s.rollbackPolicy)
	api.GET("/agents", s.listAgents)
	api.GET("/agents/:id", s.getAgent)
	api.GET("/cluster/options", s.clusterOptions)

	api.GET("/policy-by-id", s.policyByIDAlias)
	api.PUT("/policy-by-id", s.policyByIDAlias)
	api.DELETE("/policy-by-id", s.policyByIDAlias)
	api.GET("/policy-revisions", s.policyRevisionsAlias)
	api.POST("/policy-rollback", s.policyRollbackAlias)

	agent := api.Group("/agent", s.agentAuth)
	agent.POST("/register", s.registerAgent)
	agent.POST("/heartbeat", s.heartbeatAgent)
	agent.GET("/config", s.getAgentConfig)
	agent.GET("/watch", s.watchAgentConfig)
	agent.POST("/apply-result", s.applyResult)
	return r
}

func (s *server) healthz(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{"status": "ok"})
}

func (s *server) readyz(c *gin.Context) {
	if err := s.db.db.PingContext(c.Request.Context()); err != nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"status": "failed", "error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": "ok"})
}
