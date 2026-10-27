# Organization AI Assistant (bring-your-own-key scenario generation)

This feature lets an organization configure its own AI assistant endpoint
and use it to generate scenario content automatically, instead of the
manual copy-prompt / paste-answer workflow in the scenario editor.

## Overview

Before this feature, the scenario editor's "Create with AI" / "Improve with
AI" flow (`ScenarioAiModal`) only ever produced a prompt: the teacher had to
copy it into their own chat assistant (ChatGPT, Claude, Le Chat…), get an
answer, and paste it back. Nothing on the OCF side ever saw that exchange.

An organization can now configure an **OpenAI-compatible "chat completions"
endpoint** (its own OpenAI account, Mistral, a self-hosted Ollama/vLLM
instance, or any proxy exposing the same request/response shape). Once
configured and enabled, the scenario editor offers a one-click "Generate
automatically" button that calls this endpoint through the OCF backend —
the manual copy/paste flow remains available as a fallback, and is the only
option for an organization that leaves this unconfigured.

**Key properties:**
- **Per-organization**, not platform-wide: each organization brings its own
  key and billing.
- **Routed through `ocf-core`**: the frontend never talks to the AI
  provider directly, and the API key never reaches the browser.
- **API key encrypted at rest**, write-only from the frontend's point of
  view: a `GET` never returns it, only a `has_api_key` flag.
- **Fails closed, not open**: if the endpoint is unset, disabled, or the
  call fails, the user falls back to the manual flow — a scenario is never
  blocked on this feature being configured or working.

## Data model

`src/organizations/models/organizationAiConfig.go` — one row per
organization (`organization_id`, unique), created lazily on first save; an
organization that never configured anything behaves as disabled with empty
fields, not as a missing resource.

| Field | Notes |
|---|---|
| `enabled` | On/off switch, independent of whether the rest is actually usable. |
| `endpoint` | Full URL of an OpenAI-compatible chat-completions endpoint. |
| `model` | Sent as-is in the request body's `model` field. |
| `api_key` | Encrypted at rest the same way `SshKey.PrivateKey` already is (AES-256-GCM, `FIELD_ENCRYPTION_SECRET`). Never serialized (`json:"-"`). |
| `has_api_key` | Computed on read, never stored — the only signal the frontend gets that a key is set. |

## API

All three routes require authentication (`AuthManagement()`), scoped to the
organization by Layer-2 `OrgRole` permissions
(`src/organizations/routes/permissions.go`):

| Method | Path | Minimum role | Purpose |
|---|---|---|---|
| `GET` | `/organizations/:id/ai-config` | member | Read the config (never the key). |
| `PUT` | `/organizations/:id/ai-config` | manager | Create or update the config. |
| `POST` | `/organizations/:id/ai-config/generate` | manager | Send a prompt, get the assistant's raw answer back. |

### `GET /organizations/:id/ai-config`

```json
{ "enabled": true, "endpoint": "https://api.openai.com/v1/chat/completions", "model": "gpt-4o-mini", "has_api_key": true }
```

### `PUT /organizations/:id/ai-config`

```json
{ "enabled": true, "endpoint": "https://api.openai.com/v1/chat/completions", "model": "gpt-4o-mini", "api_key": "sk-..." }
```

`api_key` is optional and has three states:
- **omitted** (`null`/absent): the stored key, if any, is left untouched.
- **non-empty string**: replaces the stored key.
- **explicit empty string `""`**: clears the stored key.

This is why the frontend never pre-fills the key field — there is nothing
to pre-fill it with, and leaving it blank must mean "keep what's there",
not "clear it".

The endpoint must be an `https` URL to a public host: no credentials in the
URL, no `localhost`, no loopback, private, link-local (cloud metadata) or
shared-address IP. Changing the endpoint while a key is stored requires a
new `api_key` in the same request, so a stored key is never sent to a host
its owner didn't enter it for.

Errors (`error_message` carries the code):
- **400 `ai_endpoint_invalid`** — the endpoint fails the rules above.
- **400 `ai_key_required`** — the endpoint changed but no new key was sent.

### `POST /organizations/:id/ai-config/generate`

```json
// request
{ "prompt": "..." }
// success (200)
{ "answer": "..." }
```

Errors (`error_message` carries the code):
- **409 `ai_not_configured`** — the organization has no usable configuration
  (never set, or set but `enabled: false`, or missing endpoint/key).
- **413 `ai_request_too_large`** — the request body is over 512 KiB.
- **502 `ai_provider_error`** — the configured endpoint failed, timed out,
  redirected, answered a non-200 status, answered more than 2 MiB or
  returned nothing usable. This is a problem with the organization's own
  endpoint, not with OCF.

The request sent to the configured endpoint is the standard chat-completions
shape: `{"model": "...", "messages": [{"role": "user", "content": "<prompt>"}]}`,
with `Authorization: Bearer <api_key>`, and a 90-second timeout (scenario
generation prompts are long, and so are some answers). The client re-checks
every resolved address when it connects (so DNS rebinding can't reach an
internal host), never follows redirects and uses no proxy.

## Frontend usage

### Configuring it — Organization Settings

An organization manager or owner opens the organization's **Settings**
tab and finds the **"AI assistant"** section (between the general settings
and the danger zone). From there they can:
1. Paste the endpoint URL (e.g. `https://api.openai.com/v1/chat/completions`
   for OpenAI itself, or the equivalent URL for Mistral/a self-hosted
   Ollama/vLLM instance).
2. Enter the model name (e.g. `gpt-4o-mini`).
3. Enter the API key (left blank on every subsequent visit — the page
   only shows "a key is already configured" text, never the key itself).
4. Tick "Use this endpoint instead of the manual copy/paste flow" and save.

Any member who can see the Settings tab at all already has at least manager
access (the tab itself is `v-if="canManage"` in the parent page), so no
extra admin gating was added on top — this mirrors the backend's own
`manager`-minimum requirement on the `PUT`.

### Using it — Scenario creation

In the scenario editor's "Create with AI" / "Improve with AI" modal
(`ScenarioAiModal.vue`), once the destination organization (or the
organization owning the destination class/group) has AI configured and
enabled, step 2 shows a **"Generate automatically"** button above the usual
"copy the prompt" instructions. Clicking it:
1. Sends the already-built prompt to
   `POST /organizations/:id/ai-config/generate`.
2. On success, jumps straight to step 3 with the answer pre-filled, exactly
   as if it had been pasted in manually.
3. On failure, shows an inline error and leaves the manual flow available
   right below it — nothing is lost, the teacher can still copy the prompt
   and do it by hand.

If the destination has no AI configured (or the config is disabled), the
button simply doesn't appear; the manual flow is unchanged and is all the
teacher ever sees, exactly like before this feature existed.

## Security notes

- The API key never leaves `ocf-core`: it is read from the database,
  decrypted in memory, used once to build the `Authorization` header for
  the outbound call, and never appears in any response sent back to the
  frontend.
- Encryption follows the existing field-encryption pattern
  (`src/utils/crypto`, `FIELD_ENCRYPTION_SECRET` env var) already used for
  SSH private keys — no new secret-management mechanism was introduced.
- A manager is required both to change the configuration and to trigger a
  generation: every call spends the organization's own provider credit, so
  students (plain members) cannot reach it.
- The key never reaches a log: errors from the provider call name the
  failure (status code, timeout, size) but never carry the request, its
  headers or the provider's body, which some providers use to echo the key.

## Files

**Backend (`ocf-core`):**
- `src/organizations/models/organizationAiConfig.go`
- `src/organizations/dto/organizationAiConfigDto.go`
- `src/organizations/services/organizationAiService.go`
- `src/organizations/controller/organizationAiConfigController.go`
- `src/organizations/routes/organizationRoutes.go`, `permissions.go`
- `src/initialization/database.go` (migration)
- `src/organizations/services/organizationAiProviderClient.go`
- `tests/organizations/organizationAiConfig_test.go`,
  `tests/organizations/organizationAiSecurity_test.go`,
  `tests/authorization/all_permissions_test.go`

**Frontend (`ocf-front`):**
- `src/services/domain/organization/organizationService.ts`
- `src/components/Organizations/OrganizationSettingsTab.vue`
- `src/components/ScenarioEditor/ScenarioAiModal.vue`
- `src/composables/useScenarioAiTranslations.ts`
- `tests/components/OrganizationSettingsTab-aiConfig.test.ts`,
  `tests/components/ScenarioAiModal.test.ts`,
  `tests/components/OrganizationSettingsTab-retention.test.ts`
