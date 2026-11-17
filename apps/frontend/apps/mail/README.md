# Sytue Mail

Email client of the **Sytue One** suite, custom-built with
**Astro 7 + Preact 10** on top of the open **JMAP** protocol of Sytue's **own Go
service Sytue Mail** (`mail.sytue.eu`; 100 % EU sovereignty, Scaleway). A
**Sytue-native**, original and **proprietary** product (© Sytue). Clean UX with the
**Sytue** visual language: conversation list, reader, compose/reply and
folders/labels.

It mounts the **Sytue Shell** (`@sytue/shell`) and uses only the `@sytue/ui` design
system. No ad-hoc UI. Light/dark mode and WCAG AA inherited from the design system.

<a id="arquitectura"></a>

## Architecture

- **BFF + OIDC/PKCE** (`@sytue/auth`): endpoints in `src/pages/auth/*`
  (`login`, `callback`, `logout`, `session`, `refresh`, `token`). httpOnly session
  cookie for the `.sytue.eu` domain. The confidential client is `sytue-mail`.
- **JMAP BFF** (`src/pages/api/*`): the browser calls same-origin with the cookie; the
  server forwards to the Sytue Mail Go service with the Sytue ID access token through
  the `@sytue/api-clients/jmap` SDK. For mail, the token never touches the browser.
  Endpoints: `mailboxes`, `emails`, `seen`, `send`, `intent`.
- **Preact islands** (`src/components/*`): `MailApp` (root, mounts `SytueShell` +
  `AppShell`), `MessageList`, `MessageReader`, `ComposeModal`.
- **Hub intents** (`@sytue/api-clients/hub`): _Create event from an email_
  (→ Sytue Calendar) and _Attach from Drive_ (→ Sytue Drive), via `POST /api/intent`.

<a id="desarrollo"></a>

## Development

Dev port `4325` (see `docs/CONVENTIONS.md` §6); domain `mail.sytue.localhost`.

```bash
bun run --filter @sytue/mail dev
```

Copy `.env.example` to `.env` (placeholders only; real secrets stay out of git).

<a id="producción"></a>

## Production

Multi-stage `Dockerfile` with **Bun** (Astro `standalone` output). Deployed by
the `sytue-one` umbrella chart on Kapsule (Scaleway, EU regions).
