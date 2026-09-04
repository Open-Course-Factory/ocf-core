# Publishing a Terminal Session Port (Traefik)

Configuration guide for the opt-in feature that lets a user publish a port
from inside their running terminal session to a public URL, served by a
dedicated Traefik instance.

Code: `src/terminalTrainer/services/exposedPortService.go`,
`src/terminalTrainer/routes/traefikConfigController.go`.
Reference infra: separate git repo `traefik/` (see its `README.md` for the
Traefik deployment itself — this document covers the `ocf-core` side of the
configuration).

**Current status: dev mode, plain HTTP, no TLS.** See the "Adding TLS
later" section of the `traefik/` repo's `README.md` to turn it on later —
no code change is needed on either side, only configuration.

Disabled by default at three independent levels: operator config
(`EXPOSE_DOMAIN`/`TRAEFIK_PROVIDER_SECRET` must both be explicitly set, or
the routes aren't even mounted), plan flag (`port_exposure_enabled`,
`false` by default on any new plan), and explicit user action (nothing is
exposed until the user calls `POST /terminals/:id/exposed-ports`).

## 1. Configure `ocf-core`

In `.env`:

```
EXPOSE_DOMAIN=expose.local          # domain you'll use (see step 2)
TRAEFIK_PROVIDER_SECRET=a-long-random-secret
EXPOSE_SCHEME=http                  # or leave empty, "http" is the default
TRAEFIK_CERT_RESOLVER=              # leave empty in dev
```

Generate the secret e.g. with `openssl rand -hex 32`. Restart `ocf-core` —
without both `EXPOSE_DOMAIN` and `TRAEFIK_PROVIDER_SECRET`, the routes
aren't even mounted (404), so that's the first thing to check if nothing
responds.

## 2. Resolve the domain to the Traefik machine

In dev, no need for a real public wildcard DNS record: add to `/etc/hosts`
(on the machine you'll test from in a browser):

```
<Traefik_machine_IP>  test.expose.local
```

One per exposure you want to test (the slug is random, generated on every
`POST`), or simpler: point a real wildcard DNS record at that IP if you
have a test domain available — saves editing `/etc/hosts` on every attempt.

## 3. Launch Traefik

See the `traefik/` repo's `README.md` (startup, networking — same machine
or a separate one). Both topologies are validated and documented there.

## 4. Enable the feature on a plan

`port_exposure_enabled` defaults to `false` on every plan. Flip it to
`true` on the plan your test user is on:

```
PATCH /api/v1/subscription-plans/:id
{"port_exposure_enabled": true}
```

(as an admin), or directly in the database for a quick dev shortcut:

```sql
UPDATE subscription_plans SET port_exposure_enabled = true WHERE id = '<test-plan-id>';
```

(or without the `WHERE` to enable it on every existing plan — dev only,
never in production: the flag still defaults to `false` for any plan
created afterward, this `UPDATE` only touches rows already in the database
at the time it runs).

## 5. Verify the internal endpoint responds

```
curl -H "X-Provider-Secret: <your-secret>" http://<ocf-core-host>:8080/internal/traefik/dynamic-config
```

→ should return `{"http":{"routers":{},"services":{}}}` as long as no
session is exposing a port.

## 6. End-to-end test

1. Launch a terminal session on that plan.
2. Inside it: `python3 -m http.server 8000 --bind 0.0.0.0`.
3. From the outside:
   ```
   curl -X POST https://<ocf-front-or-api>/api/v1/terminals/<session_id>/exposed-ports \
     -H "Authorization: Bearer <your-token>" -H "Content-Type: application/json" \
     -d '{"port": 8000}'
   ```
4. Grab the `url` from the response, open it in a browser (or `curl` it).
5. Stop the session — the URL should stop responding within ~5s (Traefik's
   poll interval).

## Environment variables — summary

| Variable | Default | Role |
|---|---|---|
| `EXPOSE_DOMAIN` | empty (disables the feature) | Domain under which public URLs are minted |
| `TRAEFIK_PROVIDER_SECRET` | empty (disables the feature) | Secret expected on the `X-Provider-Secret` header of the internal endpoint, must match the Traefik side |
| `EXPOSE_SCHEME` | `http` | Scheme minted into generated URLs (`http` in dev, `https` once TLS is configured) |
| `TRAEFIK_CERT_RESOLVER` | empty | Name of the Traefik ACME cert resolver; as long as empty, no `tls` block is generated in the dynamic config |

## Data model

- `SubscriptionPlan.PortExposureEnabled` (`src/payment/models/subscriptionPlan.go`) — plan flag, `false` by default. `CreateSubscriptionPlanInput.PortExposureEnabled` is a `*bool` (not a bare `bool`): the application-level `true` default only exists on that one creation path (see the comment on the model field) — any other direct construction of a `SubscriptionPlan{}` (seed, script, test) stays `false` unless set explicitly.
- `ExposedPort` (`src/terminalTrainer/models/exposedPort.go`) — one row per published port: `SessionID`, `ContainerPort`, `Slug` (random, never derived from guessable data), `ContainerIP` (resolved once at creation time via tt-backend, not re-resolved periodically), `ExpiresAt`. Cleaned up automatically when the session stops/is deleted, and by the expiry sweep.
- Cap of 3 active exposures per session (`maxExposedPortsPerSession`, `exposedPortService.go`) — a simple abuse guard, not a plan field.
