package plugin

import (
	"context"
	"net/http"

	"github.com/grafana/grafana-plugin-sdk-go/backend"
	"github.com/grafana/grafana-plugin-sdk-go/backend/instancemgmt"
)

// Make sure App implements required interfaces. This is important to do
// since otherwise we will only get a not implemented error response from plugin in
// runtime. Plugin should not implement all these interfaces - only those which are
// required for a particular task.
var (
	_ backend.CallResourceHandler   = (*App)(nil)
	_ instancemgmt.InstanceDisposer = (*App)(nil)
	_ backend.CheckHealthHandler    = (*App)(nil)
)

// App is an example app plugin with a backend which can respond to data queries.
type App struct {
	settings settings
	client   *http.Client
}

// NewApp creates a new example *App instance.
func NewApp(_ context.Context, appSettings backend.AppInstanceSettings) (instancemgmt.Instance, error) {
	cfg, err := loadSettings(appSettings)
	if err != nil {
		return nil, err
	}
	return &App{
		settings: cfg,
		client:   &http.Client{},
	}, nil
}

// Dispose here tells plugin SDK that plugin wants to clean up resources when a new instance
// created.
func (a *App) Dispose() {
	// cleanup
}

// CheckHealth handles health checks sent from Grafana to the plugin.
func (a *App) CheckHealth(_ context.Context, _ *backend.CheckHealthRequest) (*backend.CheckHealthResult, error) {
	if a.settings.ControlServerURL == "" {
		return &backend.CheckHealthResult{
			Status:  backend.HealthStatusError,
			Message: "controlServerUrl is not configured",
		}, nil
	}
	return &backend.CheckHealthResult{
		Status:  backend.HealthStatusOk,
		Message: "ok",
	}, nil
}
