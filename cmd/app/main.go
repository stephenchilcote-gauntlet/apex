package main

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"os"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"

	"github.com/apex-checkout/mobile-check-deposit/internal/config"
	appmiddleware "github.com/apex-checkout/mobile-check-deposit/internal/http/middleware"
	"github.com/apex-checkout/mobile-check-deposit/internal/deposits"
	"github.com/apex-checkout/mobile-check-deposit/internal/funding"
	"github.com/apex-checkout/mobile-check-deposit/internal/http/api"
	uihandlers "github.com/apex-checkout/mobile-check-deposit/internal/http/ui"
	"github.com/apex-checkout/mobile-check-deposit/internal/ledger"
	"github.com/apex-checkout/mobile-check-deposit/internal/repository"
	"github.com/apex-checkout/mobile-check-deposit/internal/returns"
	"github.com/apex-checkout/mobile-check-deposit/internal/settlement"
	"github.com/apex-checkout/mobile-check-deposit/internal/transfers"
	vendorclient "github.com/apex-checkout/mobile-check-deposit/internal/vendorsvc/client"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		slog.Error("invalid configuration", "err", err)
		os.Exit(1)
	}

	db, err := repository.InitDB(cfg.DBPath)
	if err != nil {
		slog.Error("failed to initialize database", "err", err)
		os.Exit(1)
	}
	defer db.Close()

	if err := repository.RunMigrations(db, "db/migrations"); err != nil {
		slog.Error("failed to run migrations", "err", err)
		os.Exit(1)
	}

	transferSvc := &transfers.TransferService{}
	ledgerSvc := &ledger.LedgerService{}
	fundingSvc := &funding.FundingService{DB: db}
	vendorClient := vendorclient.New(cfg.VendorStubURL)

	depositSvc := &deposits.DepositService{
		DB:           db,
		TransferSvc:  transferSvc,
		FundingSvc:   fundingSvc,
		LedgerSvc:    ledgerSvc,
		VendorClient: vendorClient,
		ImageDir:     cfg.ImageStoragePath,
	}

	settlementSvc := &settlement.SettlementService{
		DB:          db,
		OutputPath:  cfg.SettlementOutputPath,
		TransferSvc: transferSvc,
	}

	returnsSvc := &returns.ReturnsService{
		DB:          db,
		TransferSvc: transferSvc,
		LedgerSvc:   ledgerSvc,
	}

	apiHandlers := &api.Handlers{
		DB:            db,
		DepositSvc:    depositSvc,
		TransferSvc:   transferSvc,
		LedgerSvc:     ledgerSvc,
		SettlementSvc: settlementSvc,
		ReturnsSvc:    returnsSvc,
	}

	r := chi.NewRouter()
	r.Use(middleware.Logger)
	r.Use(middleware.Recoverer)
	r.Use(middleware.RequestID)
	r.Use(appmiddleware.SecurityHeaders())
	r.Use(appmiddleware.RateLimit(cfg.RateLimitRPM))

	r.Handle("/static/*", http.StripPrefix("/static/", http.FileServer(http.Dir("web/static"))))
	r.Get("/", func(w http.ResponseWriter, r *http.Request) { http.Redirect(w, r, "/ui", http.StatusFound) })

	r.Get("/healthz", func(w http.ResponseWriter, r *http.Request) {
		status := "ok"
		httpStatus := http.StatusOK

		dbStatus := "ok"
		if err := db.Ping(); err != nil {
			slog.Error("healthz db ping failed", "err", err)
			dbStatus = "unavailable"
			status = "degraded"
			httpStatus = http.StatusServiceUnavailable
		}

		vendorStatus := "ok"
		resp, err := http.Get(cfg.VendorStubURL + "/health")
		if err != nil {
			slog.Error("healthz vendor check failed", "err", err)
			vendorStatus = "unavailable"
			status = "degraded"
			httpStatus = http.StatusServiceUnavailable
		} else {
			resp.Body.Close()
			if resp.StatusCode != http.StatusOK {
				vendorStatus = fmt.Sprintf("HTTP %d", resp.StatusCode)
				status = "degraded"
				httpStatus = http.StatusServiceUnavailable
			}
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(httpStatus)
		json.NewEncoder(w).Encode(map[string]string{
			"status": status,
			"db":     dbStatus,
			"vendor": vendorStatus,
		})
	})

	// API routes — protected by API key auth
	r.Group(func(r chi.Router) {
		r.Use(appmiddleware.APIKeyAuth(cfg.APIKey))
		apiHandlers.RegisterRoutes(r)
	})

	// Login page — must be outside the auth group
	r.Get("/ui/login", appmiddleware.UILoginHandler(cfg.UIUsername, cfg.UIPassword, cfg.SessionSecret))
	r.Post("/ui/login", appmiddleware.UILoginHandler(cfg.UIUsername, cfg.UIPassword, cfg.SessionSecret))

	// UI routes — protected by session cookie auth
	uiH := &uihandlers.UIHandlers{
		DB:            db,
		TemplateDir:   "web/templates",
		ImageDir:      cfg.ImageStoragePath,
		DepositSvc:    depositSvc,
		TransferSvc:   transferSvc,
		LedgerSvc:     ledgerSvc,
		SettlementSvc: settlementSvc,
		ReturnsSvc:    returnsSvc,
	}
	if err := uiH.Init(); err != nil {
		slog.Error("failed to initialize UI templates", "err", err)
		os.Exit(1)
	}
	r.Group(func(r chi.Router) {
		r.Use(appmiddleware.UIAuth(cfg.UIUsername, cfg.UIPassword, cfg.SessionSecret))
		uiH.RegisterRoutes(r)
	})

	addr := fmt.Sprintf(":%s", cfg.AppPort)
	slog.Info("Starting Mobile Check Deposit server", "addr", addr)
	if err := http.ListenAndServe(addr, r); err != nil {
		slog.Error("server error", "err", err)
		os.Exit(1)
	}
}
