<!--
SPDX-License-Identifier: Apache-2.0
SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
-->

# Sytue AI

Microservicio Go que hace funcionar el **asistente de IA del editor** (Sytue
Docs). Es _stateless_: no usa base de datos ni almacenamiento; solo autentica la
petición, construye un prompt seguro y llama a un proveedor LLM configurable.

## Contrato HTTP

```
POST /api/v1/assist        Authorization: Bearer <JWT de sesión de Sytue ID>
Content-Type: application/json

{ "action": "...", "instruction": "...", "context": "...", "kind": "..." }

200 OK  ->  { "text": "..." }
```

El BFF (`apps/frontend/packages/office/src/pages/api/ai.ts`) reenvía a este endpoint
apuntando `SYTUE_AI_URL` a `http://ai:8084/api/v1/assist` y espera `{ text }`.
No hubo que tocar el BFF: el contrato ya coincidía.

Sondas: `GET /healthz` (liveness), `GET /readyz` (readiness, sin dependencias).

## Seguridad

- **API key server-only.** `AI_API_KEY` vive solo en el entorno del servidor.
  NUNCA se registra ni se devuelve (ver `Config.LogValue` y `provider.postJSON`,
  que en error solo expone el código de estado, jamás cabeceras ni cuerpo).
- **JWT validado** contra el JWKS de Sytue ID (issuer/audiencia/expiración),
  con allowlist de audiencias first-party (la suite comparte sesión).
- **Rate-limit por usuario/tenant** (token bucket en memoria).
- **Contexto tratado como DATOS.** El texto del documento se envuelve entre
  marcadores con un _nonce_ impredecible por petición y el system prompt ordena
  no obedecer instrucciones incrustadas en él: un documento que diga «ignora las
  instrucciones anteriores» no puede escalar privilegios.
- **Timeouts y límites de tamaño** en la frontera (instrucción y contexto).
- Multi-tenant: el tenant sale de los claims del JWT y entra en la clave de
  rate-limit.

## Proveedor LLM (configurable)

Detrás de la interfaz `provider.Provider` (una llamada, una respuesta; MVP sin
streaming):

- **`scaleway`** (por defecto) — Generative APIs de Scaleway mediante Chat
  Completions (`POST /v1/chat/completions`). Modelo por defecto
  `mistral-small-3.2-24b-instruct-2506`; proveedor francés e inferencia en la UE.
- **`openai-compat`** — endpoint OpenAI Chat Completions
  (`POST /v1/chat/completions`, `Authorization: Bearer`). Vía para enchufar un
  **modelo soberano/self-hosted** europeo (vLLM, llama.cpp, Ollama, TGI…).

## Configuración (entorno)

| Variable | Por defecto | Descripción |
|---|---|---|
| `PORT` / `HTTP_ADDR` | `:8084` | Escucha HTTP. |
| `OIDC_ISSUER` | `https://id.sytue.localhost` | Issuer de Sytue ID. |
| `OIDC_JWKS_URL` | `<issuer>/.well-known/jwks.json` | JWKS. |
| `OIDC_ACCEPTED_AUDIENCES` | allowlist first-party | Audiencias aceptadas. |
| `AI_PROVIDER` | `scaleway` | `scaleway` \| `openai-compat`. |
| `AI_API_KEY` | _(vacío)_ | **Secreto server-only.** Sin ella, `/assist` responde 503. |
| `AI_MODEL` | `mistral-small-3.2-24b-instruct-2506` | Modelo. Obligatorio en `openai-compat`. |
| `AI_BASE_URL` | `https://api.scaleway.ai` | Raíz del proveedor. Obligatoria en `openai-compat`. |
| `AI_MAX_TOKENS` | `1024` | Tope de tokens de salida. |
| `AI_TIMEOUT` | `60s` | Plazo de la llamada al proveedor. |
| `AI_MAX_INSTRUCTION_BYTES` | `8192` | Límite de la instrucción. |
| `AI_MAX_CONTEXT_BYTES` | `262144` | Límite del contexto (documento). |
| `AI_RATE_LIMIT_PER_MIN` | `20` | Tasa sostenida por usuario/tenant. |
| `AI_RATE_LIMIT_BURST` | `5` | Ráfaga por usuario/tenant. |

## Ejecutar / verificar

```sh
go build ./services/ai/...
go vet ./services/ai/...
go test ./services/ai/...
```

## Follow-ups documentados

- Streaming SSE (el BFF ya prevé reenviar un `ReadableStream`); el MVP devuelve
  `{ text }` completo.
- Rate-limit global entre réplicas (hoy es por réplica, en memoria): migrar a un
  `httpx.RateLimiter` compartido (p. ej. `pg.RateLimiter`) si hace falta.
- Añadir la fila de Sytue AI (`:8084`) a la tabla de puertos de
  `docs/CONVENTIONS.md §6` (fuera del alcance de este cambio).
