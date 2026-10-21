# Sytue Notepads

Cuadernos y notas de la suite **Sytue One** en **Astro 7 + Preact 10** sobre el
servicio Go propio **Sytue Notepads** (API REST propia; estado en PostgreSQL;
soberanía 100 % UE, Scaleway). Lenguaje visual **Sytue**, modo claro/oscuro y
WCAG AA del design system.

Monta la **Sytue Shell** (`@sytue/shell`) y usa exclusivamente `@sytue/ui`.

## Qué es

Un **cuaderno** contiene **notas**, y una nota puede ser cualquier cosa: texto
enriquecido, una tarea, un enlace, un color, un fichero de Sytue Drive, un
documento de Sytue Docs (con su visor incrustado), un evento de Sytue Calendar o
una sala de Sytue Rooms. Los cuadernos **anidan** y se pintan en la disposición
que cada uno tenga: **lienzo libre** (se coloca donde se suelta), **columnas** o
**lista**.

Las **etiquetas con estado** son el eje de organización: «Prioridad» con estados
«Alta / Media / Baja», excluyentes entre sí, cada uno con su color y su atajo de
un carácter para marcar deprisa.

## Privacidad: dos perillas, y las dos importan

- **Visibilidad** (`private` · `personal` · `organization` · `public`). Se fija en
  el cuaderno y cada nota puede sobreescribirla: una nota privada dentro de un
  cuaderno de equipo la ve solo quien la escribió.
- **Índice de búsqueda.** Independiente de lo anterior. Lo que se deja fuera no
  aparece en la búsqueda unificada del Hub ni en la de esta app — el filtro lo
  aplica el servicio en la consulta, no la interfaz.
- **Cifrado extremo a extremo** por cuaderno (WebCrypto: PBKDF2-SHA256 + AES-256-GCM,
  `src/client/crypto.ts`). La contraseña NO sale del navegador; el servidor guarda
  el criptograma y los parámetros de derivación. Un cuaderno cifrado **nunca** se
  indexa, y el esquema lo garantiza con una constraint, no con una comprobación.

## Arquitectura

- **BFF + OIDC/PKCE** (`@sytue/auth`): `src/pages/auth/*` (client `sytue-notes`
  contra **Sytue ID**). Cookie de sesión httpOnly.
- **BFF de datos** (`src/pages/api/*`): proxy same-origin (cookie) → servicio Go
  **Sytue Notepads** con el access token Bearer, vía `NotesClient` de
  `@sytue/api-clients`. El token NUNCA llega al navegador.
- **`/api/suite`**: un solo endpoint para los cuatro selectores de incrustación
  (Drive, Docs, Calendar, Rooms). Lo que se guarda es una **referencia**, nunca una
  copia de bytes.
- **`/api/doc-preview`**: sirve el documento de Sytue Docs exportado a HTML desde
  el origen de ESTA app, para pintarlo en un `<iframe sandbox>`. El middleware
  relaja `frame-ancestors` **solo** para esa ruta.
- **Islas Preact** (`src/components/*`): `NotesApp` (raíz), `NotebookTree`,
  `NoteCanvas`, `NoteCard`, `RichText`, `NotebookSettings`, `TagsModal`,
  `SuitePicker`, `PassphraseDialog`, `PublicNotebook`.
- **Cuaderno publicado**: `/p/<token>` — sin sesión, solo lectura, `noindex`.

## Integración con el resto de la suite

- **Calendar (bidireccional).** Una nota puede colgar de un evento; la ficha del
  evento en Sytue Calendar lista las notas que cuelgan de él. **El vínculo vive una
  sola vez**, en `note_links` de este servicio: Calendar pregunta con el token de
  quien mira, así que la autorización sigue siendo la de Notes y no hay dos copias
  que se desincronicen. La identidad del evento es su **UID** (RFC 5545).
- **Rooms.** «Guardar en Notes» desde el menú de un mensaje abre una nota nueva ya
  vinculada; «Compartir en Rooms» publica el **enlace** de una nota en un canal
  (nunca su contenido).
- **Docs / Drive.** Selector en línea y, en el caso de Docs, visor incrustado.
- **Hub.** Conector de búsqueda federada (`services/hub/internal/connectors/notes.go`),
  que solo ve lo que su dueño ha dejado en el índice.

## Desarrollo

```bash
bun install                       # desde apps/frontend
bun run build                     # construye packages/* (obligatorio antes de typecheck)
bun run --filter '@sytue/notes' dev        # http://localhost:4331/notes
bun run --filter '@sytue/notes' typecheck
bun run --filter '@sytue/notes' test
```

Variables de entorno: ver `.env.example`. En compose, `docker compose --profile notes up`.
