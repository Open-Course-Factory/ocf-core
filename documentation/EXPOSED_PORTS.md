# Exposed ports — publishing a session port at a public URL

Opt-in feature: a learner publishes a port of their running terminal container at
`https://<slug>.<EXPOSE_DOMAIN>`. ocf-core owns the policy and the registry; a Traefik
instance run by the operator owns the traffic.

Code: `src/terminalTrainer/services/exposedPortService.go` (policy, registry),
`src/terminalTrainer/routes/traefikConfigController.go` (the endpoint Traefik polls),
`src/terminalTrainer/routes/exposedPortController.go` (learner + admin routes).

## How it fits together

```
learner ─► POST /terminals/:id/exposed-ports {port}   (plan + scenario gate, max 3 per session)
                │ row in exposed_ports: slug, container IP (from tt-backend /info), backend, expires_at
                ▼
Traefik ─► GET /internal/traefik/dynamic-config  every 5 s, header X-Provider-Secret
                │ one router Host(`<slug>.<EXPOSE_DOMAIN>`) + one service http://<container ip>:<port>
                ▼
visitor ─► https://<slug>.<EXPOSE_DOMAIN> ─► Traefik ─► container ip:port
```

Traefik must be able to reach the container IPs. On one host that is the Incus bridge; in
production the Traefik lives in the cluster and reaches each host's bridge over an overlay
through a gateway instance — that topology, the wildcard certificate and the second load
balancer are documented in `deploy-to-k8s` (`docs/EXPOSED_PORTS_RUNBOOK.md`), not here.

Rows are deleted when the session stops, is deleted or expires; the Traefik endpoint only
publishes exposures whose terminal is live (`RunningDisplayScope`), so a dead session never
keeps a route alive.

## Four gates, all off by default

1. **Operator env**: `EXPOSE_DOMAIN` and `TRAEFIK_PROVIDER_SECRET` must both be set, or none of
   the routes are mounted (404).
2. **Platform feature flag** `port_exposure` (admin → Platform settings, declared in
   `src/terminalTrainer/moduleConfig.go`, seeded disabled): off means 403 on create and list,
   and an empty Traefik config — switching it off kills every published route within one poll.
3. **Plan**: `SubscriptionPlan.PortExposureEnabled` — `false` on every new plan, toggled by an
   admin in the plan form (`PATCH /subscription-plans/:id {"port_exposure_enabled": true}`).
4. **Scenario**: a terminal running a scenario is refused unless `Scenario.PortExposureAllowed`
   is on. Plain terminals skip this gate.

The list endpoint runs the same gate as the create endpoint, so the frontend hides the panel
on a 403 from `GET /terminals/:id/exposed-ports` instead of re-deriving the rule.

## Environment variables

| Variable | Default | Role |
|---|---|---|
| `EXPOSE_DOMAIN` | empty (feature off) | Domain under which public URLs are minted |
| `TRAEFIK_PROVIDER_SECRET` | empty (feature off) | Value Traefik must send in `X-Provider-Secret`; compared constant-time, never fails open |
| `EXPOSE_SCHEME` | `http` | `https` once Traefik carries a certificate: URLs are minted https and every generated router gets a `tls: {}` block, so Traefik terminates with its default TLS store |

## Routes

| Method | Path | Who |
|---|---|---|
| `POST` | `/api/v1/terminals/:id/exposed-ports` `{"port": 8000}` | session owner |
| `GET` | `/api/v1/terminals/:id/exposed-ports` | session owner |
| `DELETE` | `/api/v1/terminals/:id/exposed-ports/:portId` | session owner |
| `GET` | `/api/v1/terminals/admin/exposed-ports` | platform admin — every active exposure with user, session, backend |
| `DELETE` | `/api/v1/terminals/admin/exposed-ports/:portId` | platform admin — kill any exposure |
| `GET` | `/internal/traefik/dynamic-config` | Traefik, `X-Provider-Secret` (outside `/api/v1`, no JWT) |

Ports must be in `1024–65535`. A session holds at most 3 exposures
(`maxExposedPortsPerSession`). An exposure lives `port_exposure_ttl_minutes` of the plan (default 60,
edited in the plan admin form) or until the session ends, whichever comes first: a URL is for looking
at one's own work during the training, not for sharing. Expired exposures are neither listed nor
counted against the cap.

A reconcile job (`cron.StartExposedPortReconcileJob`, every minute) asks tt-backend `/info` about
every active exposure and withdraws those whose container is gone, stopped or re-addressed, so a
freed bridge address is never routed to a newer container; an unreachable tt-backend changes
nothing. The plan TTL is capped at 3 hours (`maxExposeTTL`). The platform flag fails closed: no
`port_exposure` feature row means disabled.

Retention (RGPD register): an exposure row is hard-deleted after its expiry plus the number of
days set in Platform Settings → "Exposed ports: row retention (days)" (`exposed_port_retention_days`,
30 by default; blank or unreadable means 30), whether it ended by withdrawal, reconcile or expiry
(`cron.StartExposedPortRetentionJob`); the
audit trail keeps who/what/when under the audit-log retention. Visitor access logs live on the
Traefik pod's stdout only (node log rotation, no log stack), so a complaint's evidence must be
copied when it arrives.

Every exposure and withdrawal lands in `audit_logs` (`terminal.port.exposed` /
`terminal.port.unexposed`, target `exposed_port`, metadata: slug, URL, port, container IP,
backend, session, owner, expiry); the admin kill switch is recorded with the admin as actor and
the owner as on-behalf-of.

**The session needs the `network` feature.** The `ocf-base` profile is NIC-less, so a session
started without network has no address for Traefik to reach: `POST` answers 400 with
"this session has no network interface". Exposure is therefore only possible on plans that
also grant `network_access_enabled`.

## Local end-to-end

1. `.env`: `EXPOSE_DOMAIN=expose.local`, `TRAEFIK_PROVIDER_SECRET=$(openssl rand -hex 32)`;
   restart ocf-core.
2. Run the reference Traefik from the `ocf-exposed-ports-traefik` repo (joins the `ocf-shared`
   Docker network, polls `http://ocf-core:8080/internal/traefik/dynamic-config`).
3. `curl -H "X-Provider-Secret: <secret>" http://localhost:8080/internal/traefik/dynamic-config`
   → `{}` while nothing is exposed (the only idle payload Traefik v3 accepts).
4. Enable the flag on the test plan, start a session **with the network feature**, inside it
   `python3 -m http.server 8000 --bind 0.0.0.0`, expose 8000 from the panel, add
   `<traefik ip> <slug>.expose.local` to `/etc/hosts`, open the URL.
5. Stop the session: the URL stops answering within one poll interval.

## Data model

`ExposedPort` (`src/terminalTrainer/models/exposedPort.go`): `TerminalID`, `SessionID`,
`UserID`, `Backend`, `ContainerPort`, `Slug` (random, 10 chars, never derived from anything
guessable), `ContainerIP` (resolved once at creation from tt-backend `/info`; a resumed session
gets a fresh IP, which is why stop/resume clears exposures), `ExpiresAt` (the terminal's).
