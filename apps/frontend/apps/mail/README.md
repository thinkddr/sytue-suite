# Sytue Mail

Cliente de correo de la suite **Sytue One**, construido a medida en
**Astro 7 + Preact 10** sobre el protocolo abierto **JMAP** del servicio **Go
propio Sytue Mail** (`mail.sytue.eu`; soberanía 100 % UE, Scaleway). Producto
**Sytue-native**, original y **propietario** (© Sytue). UX limpia con el lenguaje
visual **Sytue**: lista de conversaciones, lector, redacción/respuesta y
carpetas/etiquetas.

Monta la **Sytue Shell** (`@sytue/shell`) y usa exclusivamente el design system
`@sytue/ui`. Sin UI ad-hoc. Modo claro/oscuro y WCAG AA heredados del design system.

## Arquitectura

- **BFF + OIDC/PKCE** (`@sytue/auth`): endpoints en `src/pages/auth/*`
  (`login`, `callback`, `logout`, `session`, `refresh`, `token`). Cookie de sesión
  httpOnly de dominio `.sytue.eu`. El client confidencial es `sytue-mail`.
- **BFF JMAP** (`src/pages/api/*`): el navegador llama same-origin con la cookie; el
  servidor reenvía al servicio Go Sytue Mail con el access token de Sytue ID vía el
  SDK `@sytue/api-clients/jmap`. El token nunca toca el navegador para el correo.
  Endpoints: `mailboxes`, `emails`, `seen`, `send`, `intent`.
- **Islas Preact** (`src/components/*`): `MailApp` (raíz, monta `SytueShell` +
  `AppShell`), `MessageList`, `MessageReader`, `ComposeModal`.
- **Intents del Hub** (`@sytue/api-clients/hub`): _Crear evento desde un correo_
  (→ Sytue Calendar) y _Adjuntar de Drive_ (→ Sytue Drive), vía `POST /api/intent`.

## Desarrollo

Puerto dev `4325` (ver `docs/CONVENTIONS.md` §6); dominio `mail.sytue.localhost`.

```bash
bun run --filter @sytue/mail dev
```

Copia `.env.example` a `.env` (sólo placeholders; secretos reales fuera de git).

## Producción

`Dockerfile` multi-stage con **Bun** (salida `standalone` de Astro). Desplegado por
el umbrella chart `sytue-one` en Kapsule (Scaleway, regiones UE).
