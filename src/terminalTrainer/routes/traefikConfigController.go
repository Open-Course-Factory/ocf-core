package terminalController

import (
	"fmt"
	"net/http"
	"os"

	terminalMiddleware "soli/formations/src/terminalTrainer/middleware"
	services "soli/formations/src/terminalTrainer/services"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// traefikService is deliberately just the one method this controller needs
// from TerminalTrainerService, rather than the full interface — keeps the
// controller's dependency honest and makes it trivial to fake in tests.
type traefikService interface {
	GetActiveExposedPortsForTraefik() ([]exposedPortRow, error)
}

// exposedPortRow mirrors the fields of models.ExposedPort this controller
// reads. Declared narrowly here (see traefikConfigAdapter) so this file
// doesn't need to import terminalTrainer/models just to read four fields.
type exposedPortRow struct {
	Slug          string
	ContainerIP   string
	ContainerPort int
}

// traefikConfigAdapter bridges the real services.TerminalTrainerService
// (which returns []models.ExposedPort) to the narrow traefikService
// interface above.
type traefikConfigAdapter struct {
	svc services.TerminalTrainerService
}

func (a *traefikConfigAdapter) GetActiveExposedPortsForTraefik() ([]exposedPortRow, error) {
	exposedPorts, err := a.svc.GetActiveExposedPortsForTraefik()
	if err != nil {
		return nil, err
	}
	rows := make([]exposedPortRow, 0, len(exposedPorts))
	for _, ep := range exposedPorts {
		rows = append(rows, exposedPortRow{
			Slug:          ep.Slug,
			ContainerIP:   ep.ContainerIP,
			ContainerPort: ep.ContainerPort,
		})
	}
	return rows, nil
}

// traefikDynamicConfig is the subset of Traefik's dynamic configuration
// schema (https://doc.traefik.io/traefik/reference/dynamic-configuration/file/)
// this endpoint produces: one router+service pair per active exposed port.
type traefikDynamicConfig struct {
	HTTP traefikHTTPConfig `json:"http"`
}

type traefikHTTPConfig struct {
	Routers  map[string]traefikRouter   `json:"routers"`
	Services map[string]traefikService_ `json:"services"`
}

type traefikRouter struct {
	Rule    string           `json:"rule"`
	Service string           `json:"service"`
	TLS     traefikRouterTLS `json:"tls"`
}

type traefikRouterTLS struct {
	CertResolver string `json:"certResolver,omitempty"`
}

// traefikService_ avoids colliding with the traefikService interface name
// above while still matching Traefik's "services" JSON shape.
type traefikService_ struct {
	LoadBalancer traefikLoadBalancer `json:"loadBalancer"`
}

type traefikLoadBalancer struct {
	Servers []traefikServer `json:"servers"`
}

type traefikServer struct {
	URL string `json:"url"`
}

// TraefikConfigRoutes mounts the internal, non-JWT endpoint Traefik's HTTP
// provider polls for dynamic configuration. It is registered OUTSIDE the
// /api/v1 group (see main.go) and protected by RequireProviderSecret
// instead of AuthManagement — the caller is Traefik, not a logged-in user.
//
// Only mounted when services.IsExposedPortsFeatureEnabled() — the caller in
// main.go is responsible for that check, matching the "disabled unless
// explicitly configured" contract for this whole feature.
func TraefikConfigRoutes(router *gin.RouterGroup, db *gorm.DB) {
	svc := services.NewTerminalTrainerService(db)
	adapter := &traefikConfigAdapter{svc: svc}

	routes := router.Group("/traefik")
	routes.GET("/dynamic-config", terminalMiddleware.RequireProviderSecret(), func(ctx *gin.Context) {
		getDynamicConfig(ctx, adapter)
	})
}

func getDynamicConfig(ctx *gin.Context, svc traefikService) {
	domain := os.Getenv("EXPOSE_DOMAIN")
	if domain == "" {
		// Belt-and-suspenders: main.go only mounts this route when
		// IsExposedPortsFeatureEnabled() is true, which already implies
		// EXPOSE_DOMAIN is set. An empty config here still avoids
		// Traefik installing valid-looking-but-broken hostless routes.
		ctx.JSON(http.StatusOK, traefikDynamicConfig{HTTP: traefikHTTPConfig{
			Routers:  map[string]traefikRouter{},
			Services: map[string]traefikService_{},
		}})
		return
	}

	exposedPorts, err := svc.GetActiveExposedPortsForTraefik()
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	certResolver := os.Getenv("TRAEFIK_CERT_RESOLVER")
	if certResolver == "" {
		certResolver = "letsencrypt"
	}

	config := traefikDynamicConfig{HTTP: traefikHTTPConfig{
		Routers:  make(map[string]traefikRouter, len(exposedPorts)),
		Services: make(map[string]traefikService_, len(exposedPorts)),
	}}

	for _, ep := range exposedPorts {
		config.HTTP.Routers[ep.Slug] = traefikRouter{
			Rule:    fmt.Sprintf("Host(`%s.%s`)", ep.Slug, domain),
			Service: ep.Slug,
			TLS:     traefikRouterTLS{CertResolver: certResolver},
		}
		config.HTTP.Services[ep.Slug] = traefikService_{
			LoadBalancer: traefikLoadBalancer{
				Servers: []traefikServer{{URL: fmt.Sprintf("http://%s:%d", ep.ContainerIP, ep.ContainerPort)}},
			},
		}
	}

	ctx.JSON(http.StatusOK, config)
}
