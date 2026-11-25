# @sytue/api-clients

Typed TypeScript SDKs for Sytue One's **own Go services**. One client per
service; each one speaks the open standard/protocol of the corresponding
Go service. No third-party AGPL engines (_clean-room_ implementation).

> Source of truth: [`docs/CONVENTIONS.md`](../../../../docs/CONVENTIONS.md) (v4).
> Code **Apache-2.0 © 2026 Daniel Delgado Ramos (thinkddr)**.

<a id="autorización"></a>
## Authorization

All clients receive an `AuthProvider` that returns the **Sytue ID access
token** (JWT). Each Go service validates it via JWKS. The SDK does **not** manage
credentials: the token is provided by the app/BFF (`@sytue/auth`, OIDC+PKCE).

```ts
import { bearer } from '@sytue/api-clients';
const auth = bearer(() => session.accessToken); // string | () => string | Promise<string>
```

<a id="clientes-subpath-exports"></a>
## Clients (subpath exports)

| Import                        | Go service        | Domain (dev)                | Protocol                  |
| ----------------------------- | ----------------- | --------------------------- | ------------------------- |
| `@sytue/api-clients/id`       | Sytue ID          | `id.sytue.eu` (:8081)       | OIDC/OAuth2, JWKS         |
| `@sytue/api-clients/drive`    | Sytue Drive       | `drive.sytue.eu` (:8082)    | REST + WebDAV             |
| `@sytue/api-clients/docs`     | Sytue Docs        | `docs.sytue.eu` (:8083)     | REST + colab WS (CRDT/OT) |
| `@sytue/api-clients/chat`     | Sytue Chat        | `rooms.sytue.eu` (:8085)    | REST + WebSocket          |
| `@sytue/api-clients/mail`     | Sytue Mail        | `mail.sytue.eu` (:8086)     | JMAP (RFC 8620/8621)      |
| `@sytue/api-clients/calendar` | Sytue Calendar    | `calendar.sytue.eu` (:8087) | REST + CalDAV (RFC 4791)  |
| `@sytue/api-clients/contacts` | Sytue Contacts    | `contacts.sytue.eu` (:8088) | REST + CardDAV (RFC 6352) |
| `@sytue/api-clients/meet`     | Sytue Meet        | `rooms.sytue.eu` (:8089)    | REST + LiveKit web SDK    |
| `@sytue/api-clients/cloud`    | Sytue DevX (PaaS) | `cloud.sytue.eu` (:8090)    | REST control plane        |
| `@sytue/api-clients/hub`      | Sytue Hub         | `hub.sytue.eu` (:8092)      | REST + SSE                |

All of them are also re-exported from the `@sytue/api-clients` root.

<a id="uso"></a>
## Usage

```ts
import { DriveClient, MailClient, MeetClient, bearer } from '@sytue/api-clients';

const auth = bearer(() => session.accessToken);

// Drive
const drive = new DriveClient({ baseUrl: 'https://drive.sytue.eu', auth });
const files = await drive.list('/');

// Mail (JMAP; deriva /.well-known/jmap del baseUrl)
const mail = new MailClient({ baseUrl: 'https://mail.sytue.eu', auth });
const mailboxes = await mail.listMailboxes();

// Meet: el BFF mintea el JWT de LiveKit; se pasa a livekit-client
const meet = new MeetClient({ baseUrl: 'https://rooms.sytue.eu', auth });
const conn = await meet.joinRoom('standup');
// new Room().connect(conn.serverUrl, conn.token)  // livekit-client (Apache-2.0)
```

<a id="tiempo-real-ws--sse"></a>
### Real time (WS / SSE)

The WebSocket (Docs collab, Chat) and SSE (Hub, Cloud logs) channels pass the token in the
query string (browsers do not allow headers on `WebSocket`/`EventSource`); the
Go service validates it via Sytue ID's JWKS.

```ts
const chat = new ChatClient({ baseUrl: 'https://rooms.sytue.eu', auth });
const ws = await chat.connect(); // WebSocket de eventos en vivo
```

<a id="dav-en-node-ssr"></a>
## DAV in Node (SSR)

The DAV clients (`CalDavClient`, `CardDavClient`) parse XML with `DOMParser`. In
the browser it works directly; in SSR (Node), inject a compatible parser:

```ts
import { setXmlParser } from '@sytue/api-clients';
import { DOMParser } from '@xmldom/xmldom';
setXmlParser(new DOMParser());
```

## Scripts

- `pnpm build` — `tsc -p tsconfig.lib.json`
- `pnpm typecheck` — typecheck without emitting
