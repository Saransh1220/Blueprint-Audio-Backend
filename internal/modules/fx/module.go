package fx

import (
	"context"
	"log"
	"time"

	"github.com/jmoiron/sqlx"
	"github.com/saransh1220/blueprint-audio/internal/modules/fx/application"
	"github.com/saransh1220/blueprint-audio/internal/modules/fx/domain"
	"github.com/saransh1220/blueprint-audio/internal/modules/fx/infrastructure/persistence/postgres"
	"github.com/saransh1220/blueprint-audio/internal/modules/fx/infrastructure/provider/frankfurter"
	fxHttp "github.com/saransh1220/blueprint-audio/internal/modules/fx/interfaces/http"
	paymentDomain "github.com/saransh1220/blueprint-audio/internal/modules/payment/domain"
	"github.com/saransh1220/blueprint-audio/internal/shared/infrastructure/config"
)

type Module struct {
	service     *application.Service
	refresher   *application.Refresher
	snapshotter *application.Snapshotter
	handler     *fxHttp.FXHandler
	cfg         config.FXConfig
}

func NewModule(db *sqlx.DB, cfg config.FXConfig, auditLogger fxHttp.AuditLogger) *Module {
	repo := postgres.NewRateRepository(db)

	var provider domain.RateProvider
	prov, err := frankfurter.NewProvider(frankfurter.Config{
		BaseURL: cfg.ProviderURL,
		APIKey:  cfg.APIKey,
	})
	if err != nil {
		log.Printf("[FX Module] Warning: failed to initialize frankfurter provider: %v", err)
	} else {
		provider = prov
	}

	appCfg := application.Config{
		Enabled:           cfg.Enabled,
		RefreshInterval:   cfg.RefreshInterval,
		StaleAfter:        cfg.StaleAfter,
		MaxAge:            cfg.MaxAge,
		MaxDailyMove:      cfg.MaxDailyMove,
		ReportingCurrency: cfg.ReportingCurrency,
	}

	refresher := application.NewRefresher(repo, repo, provider, appCfg)
	service := application.NewService(repo, refresher, appCfg)
	snapshotter := application.NewSnapshotter(repo, appCfg)
	handler := fxHttp.NewFXHandler(service, auditLogger)

	return &Module{
		service:     service,
		refresher:   refresher,
		snapshotter: snapshotter,
		handler:     handler,
		cfg:         cfg,
	}
}

func (m *Module) Handler() *fxHttp.FXHandler {
	return m.handler
}

func (m *Module) Service() *application.Service {
	return m.service
}

func (m *Module) Snapshotter() paymentDomain.ReportingRateSnapshotter {
	return m.snapshotter
}

func (m *Module) StartRefresher(ctx context.Context) {
	if m.cfg.Enabled && m.refresher != nil {
		go m.refresher.StartRefresher(ctx, 1*time.Hour)
	}
}
