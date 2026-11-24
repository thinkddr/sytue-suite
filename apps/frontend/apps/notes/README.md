# Sytue Notepads

Notebooks and notes of the **Sytue One** suite in **Astro 7 + Preact 10** on top of
Sytue's own Go service **Sytue Notepads** (own REST API; state in PostgreSQL;
100 % EU sovereignty, Scaleway). **Sytue** visual language, light/dark mode and
WCAG AA from the design system.

It mounts the **Sytue Shell** (`@sytue/shell`) and uses only `@sytue/ui`.

<a id="qué-es"></a>

## What it is

A **notebook** contains **notes**, and a note can be anything: rich text, a task,
a link, a color, a Sytue Drive file, a Sytue Docs document (with its embedded
viewer), a Sytue Calendar event or a Sytue Rooms room. Notebooks **nest** and are
drawn in whatever layout each one has: **free canvas** (things stay where they are
dropped), **columns** or **list**.

**Tags with states** are the organizing axis: “Priority” with the states
“High / Medium / Low”, mutually exclusive, each with its own color and a
one-character shortcut for quick marking.

<a id="privacidad-dos-perillas-y-las-dos-importan"></a>

## Privacy: two knobs, and both matter

- **Visibility** (`private` · `personal` · `organization` · `public`). It is set on
  the notebook and each note can override it: a private note inside a team notebook
  is seen only by whoever wrote it.
- **Search index.** Independent of the above. Whatever is left out does not appear
  in the Hub's unified search nor in this app's — the filter is applied by the
  service in the query, not by the interface.
- **End-to-end encryption** per notebook (WebCrypto: PBKDF2-SHA256 + AES-256-GCM,
  `src/client/crypto.ts`). The password does NOT leave the browser; the server stores
  the ciphertext and the derivation parameters. An encrypted notebook is **never**
  indexed, and the schema guarantees it with a constraint, not with a check.

<a id="arquitectura"></a>

## Architecture

- **BFF + OIDC/PKCE** (`@sytue/auth`): `src/pages/auth/*` (client `sytue-notes`
  against **Sytue ID**). httpOnly session cookie.
- **Data BFF** (`src/pages/api/*`): same-origin proxy (cookie) → Go service
  **Sytue Notepads** with the Bearer access token, via `NotesClient` from
  `@sytue/api-clients`. The token NEVER reaches the browser.
- **`/api/suite`**: a single endpoint for the four embedding pickers
  (Drive, Docs, Calendar, Rooms). What is stored is a **reference**, never a
  copy of the bytes.
- **`/api/doc-preview`**: serves the Sytue Docs document exported to HTML from
  THIS app's origin, to render it in an `<iframe sandbox>`. The middleware
  relaxes `frame-ancestors` **only** for that route.
- **Preact islands** (`src/components/*`): `NotesApp` (root), `NotebookTree`,
  `NoteCanvas`, `NoteCard`, `RichText`, `NotebookSettings`, `TagsModal`,
  `SuitePicker`, `PassphraseDialog`, `PublicNotebook`.
- **Published notebook**: `/p/<token>` — no session, read-only, `noindex`.

<a id="integración-con-el-resto-de-la-suite"></a>

## Integration with the rest of the suite

- **Calendar (bidirectional).** A note can hang off an event; the event's page
  in Sytue Calendar lists the notes that hang off it. **The link lives only
  once**, in this service's `note_links`: Calendar asks with the viewer's token,
  so authorization is still Notes' own and there are no two copies to drift
  out of sync. The event's identity is its **UID** (RFC 5545).
- **Rooms.** “Save to Notes” from a message's menu opens a new note that is already
  linked; “Share in Rooms” posts a note's **link** to a channel
  (never its content).
- **Docs / Drive.** Inline picker and, in the case of Docs, an embedded viewer.
- **Hub.** Federated search connector (`services/hub/internal/connectors/notes.go`),
  which only sees what its owner has left in the index.

<a id="desarrollo"></a>

## Development

```bash
bun install                       # from apps/frontend
bun run build                     # builds packages/* (required before typecheck)
bun run --filter '@sytue/notes' dev        # http://localhost:4331/notes
bun run --filter '@sytue/notes' typecheck
bun run --filter '@sytue/notes' test
```

Environment variables: see `.env.example`. In compose, `docker compose --profile notes up`.
