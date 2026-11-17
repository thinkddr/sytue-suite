<!--
SPDX-License-Identifier: Apache-2.0
SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
-->

# Sytue AI

Go microservice that powers the **editor's AI assistant** (Sytue
Docs). It is _stateless_: it uses no database or storage; it only authenticates the
request, builds a safe prompt and calls a configurable LLM provider.

<a id="contrato-http"></a>
## HTTP contract

```
POST /api/v1/assist        Authorization: Bearer <JWT de sesión de Sytue ID>
Content-Type: application/json

{ "action": "...", "instruction": "...", "context": "...", "kind": "..." }

200 OK  ->  { "text": "..." }
```

The BFF (`apps/frontend/packages/office/src/pages/api/ai.ts`) forwards to this endpoint
by pointing `SYTUE_AI_URL` at `http://ai:8084/api/v1/assist` and expects `{ text }`.
The BFF did not need to be touched: the contract already matched.

Probes: `GET /healthz` (liveness), `GET /readyz` (readiness, no dependencies).

<a id="idioma-de-las-respuestas"></a>
## Response language

The assistant writes in the language the person chose in their Sytue
ID profile: the `locale` claim of the access token (scope `profile`), and for a token without it,
the one negotiated by `Accept-Language`. With neither, the prompt carries no directive.
The exception is transforming an existing text (correct, rewrite, shorten…):
there the language of that text is preserved. See `provider.ConIdioma`.

<a id="seguridad"></a>
## Security

- **Server-only API key.** `AI_API_KEY` lives only in the server environment.
  It is NEVER logged or returned (see `Config.LogValue` and `provider.postJSON`,
  which on error only exposes the status code, never headers or body).
- **JWT validated** against Sytue ID's JWKS (issuer/audience/expiry),
  with an allowlist of first-party audiences (the suite shares the session).
- **Rate limit per user/tenant** (in-memory token bucket).
- **Context treated as DATA.** The document text is wrapped between
  markers with an unpredictable per-request _nonce_ and the system prompt instructs
  the model not to obey instructions embedded in it: a document that says “ignore the
  previous instructions” cannot escalate privileges.
- **Timeouts and size limits** at the boundary (instruction and context).
- Multi-tenant: the tenant comes from the JWT claims and goes into the
  rate-limit key.

<a id="proveedor-llm-configurable"></a>
## LLM provider (configurable)

Behind the `provider.Provider` interface (one call, one response; MVP without
streaming):

- **`scaleway`** (default) — Scaleway Generative APIs via Chat
  Completions (`POST /v1/chat/completions`). Default model
  `mistral-small-3.2-24b-instruct-2506`; French provider and inference in the EU.
- **`openai-compat`** — OpenAI Chat Completions endpoint
  (`POST /v1/chat/completions`, `Authorization: Bearer`). The way to plug in a
  European **sovereign/self-hosted model** (vLLM, llama.cpp, Ollama, TGI…).

<a id="configuración-entorno"></a>
## Configuration (environment)

| Variable | Default | Description |
|---|---|---|
| `PORT` / `HTTP_ADDR` | `:8084` | HTTP listener. |
| `OIDC_ISSUER` | `https://id.sytue.localhost` | Sytue ID issuer. |
| `OIDC_JWKS_URL` | `<issuer>/.well-known/jwks.json` | JWKS. |
| `OIDC_ACCEPTED_AUDIENCES` | first-party allowlist | Accepted audiences. |
| `AI_PROVIDER` | `scaleway` | `scaleway` \| `openai-compat`. |
| `AI_API_KEY` | _(empty)_ | **Server-only secret.** Without it, `/assist` responds 503. |
| `AI_MODEL` | `mistral-small-3.2-24b-instruct-2506` | Model. Required with `openai-compat`. |
| `AI_BASE_URL` | `https://api.scaleway.ai` | Provider root. Required with `openai-compat`. |
| `AI_MAX_TOKENS` | `1024` | Output token cap. |
| `AI_TIMEOUT` | `60s` | Deadline for the provider call. |
| `AI_MAX_INSTRUCTION_BYTES` | `8192` | Instruction limit. |
| `AI_MAX_CONTEXT_BYTES` | `262144` | Context (document) limit. |
| `AI_RATE_LIMIT_PER_MIN` | `20` | Sustained rate per user/tenant. |
| `AI_RATE_LIMIT_BURST` | `5` | Burst per user/tenant. |

<a id="ejecutar--verificar"></a>
## Run / verify

```sh
go build ./services/ai/...
go vet ./services/ai/...
go test ./services/ai/...
```

<a id="follow-ups-documentados"></a>
## Documented follow-ups

- SSE streaming (the BFF already anticipates forwarding a `ReadableStream`); the MVP returns
  the complete `{ text }`.
- Global rate limit across replicas (today it is per replica, in memory): migrate to a
  shared `httpx.RateLimiter` (e.g. `pg.RateLimiter`) if needed.
- Add the Sytue AI row (`:8084`) to the ports table in
  `docs/CONVENTIONS.md §6` (outside the scope of this change).
