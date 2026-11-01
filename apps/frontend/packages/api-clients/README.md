# @sytue/api-clients

SDKs TypeScript tipados a los **servicios Go propios** de Sytue One. Un cliente por
servicio; cada uno habla el estándar/protocolo abierto del servicio Go
correspondiente. Sin motores AGPL de terceros (implementación _clean-room_).

> Fuente de verdad: [`docs/CONVENTIONS.md`](../../../../docs/CONVENTIONS.md) (v4).
> Código **Apache-2.0 © 2026 Daniel Delgado Ramos (thinkddr)**.

## Autorización

Todos los clientes reciben un `AuthProvider` que devuelve el **access token de
Sytue ID** (JWT). Cada servicio Go lo valida vía JWKS. El SDK **no** gestiona
credenciales: el token lo provee la app/BFF (`@sytue/auth`, OIDC+PKCE).

```ts
import { bearer } from '@sytue/api-clients';
const auth = bearer(() => session.accessToken); // string | () => string | Promise<string>
```

## Clientes (subpath exports)

| Import                        | Servicio Go       | Dominio (dev)               | Protocolo                 |
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

Todos están también re-exportados desde la raíz `@sytue/api-clients`.

## Uso

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

### Tiempo real (WS / SSE)

Los canales WebSocket (Docs colab, Chat) y SSE (Hub, Cloud logs) pasan el token por
query (los navegadores no permiten cabeceras en `WebSocket`/`EventSource`); el
servicio Go lo valida vía JWKS de Sytue ID.

```ts
const chat = new ChatClient({ baseUrl: 'https://rooms.sytue.eu', auth });
const ws = await chat.connect(); // WebSocket de eventos en vivo
```

## DAV en Node (SSR)

Los clientes DAV (`CalDavClient`, `CardDavClient`) parsean XML con `DOMParser`. En
el navegador funciona directamente; en SSR (Node) inyecta un parser compatible:

```ts
import { setXmlParser } from '@sytue/api-clients';
import { DOMParser } from '@xmldom/xmldom';
setXmlParser(new DOMParser());
```

## Scripts

- `pnpm build` — `tsc -p tsconfig.lib.json`
- `pnpm typecheck` — typecheck sin emitir
