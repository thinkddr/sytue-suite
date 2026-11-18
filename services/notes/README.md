<a id="sytue-notepads-servicio-go"></a>

# Sytue Notepads (Go service)

Backend of **Sytue Notepads** (`notes.sytue.eu`, dev `:8089`): nested notebooks,
heterogeneous notes, tags with state, search and cross-references with the
rest of the suite. State in **PostgreSQL** (single source of truth); auth with
**Sytue ID** JWTs validated via JWKS.

**Clean-room** implementation: our own model. The functional inspiration
—nested containers of notes of different types, tags with state, filtering—
consists of ideas, and ideas are not copyrightable; no third-party code has been
consulted or ported (docs/CONVENTIONS.md §12).

<a id="modelo"></a>

## Model

| Table                                     | What it stores                                                                                                            |
| ----------------------------------------- | ------------------------------------------------------------------------------------------------------------------------- |
| `notebooks`                               | The notebook tree: title, icon, color, layout, visibility, indexing, encryption, linked Rooms room and public-link token. |
| `notes`                                   | The notes: type, content (**sanitized** HTML or ciphertext), position, grouping and the visibility/indexing overrides.    |
| `tags` / `tag_states` / `note_tag_states` | Tags with mutually exclusive states and their marks.                                                                      |
| `note_links`                              | Links to suite objects. It is the table that answers the REVERSE query (“which notes hang off this event?”).              |

<a id="las-tres-invariantes-que-sostiene-el-esquema"></a>

## The three invariants the schema upholds

1. **Isolation per organization**: RLS `ENABLE` + `FORCE` on the six tables.
   Separation between PEOPLE in the same organization is enforced by the visibility
   clauses in `internal/store`, on every query.
2. **Encryption and search are incompatible**: `notebook_cifrado_no_indexa`. There is no
   plaintext to index, and a check in the service can be forgotten.
3. **Either plaintext or encrypted, never both**: `note_o_claro_o_cifrado`. Storing
   the title “just in case” would publish the most revealing part of an encrypted note.

<a id="saneado-del-texto-enriquecido"></a>

## Sanitizing rich text

`internal/domain/html.go` reduces the HTML to a closed **allowlist** with
`golang.org/x/net/html` (already in the module: the Rooms unfurl uses it). It is done on
the server because the content of an organization notebook is rendered by ANOTHER
person's browser: client-side sanitizing is not sanitizing.

## Endpoints

```
GET|POST      /api/v1/notebooks                 árbol y creación
GET|PUT|DELETE/api/v1/notebooks/{id}            ajustes, visibilidad, cifrado, publicación
GET|POST      /api/v1/notebooks/{id}/notes      notas del cuaderno
PUT|DELETE    /api/v1/notes/{id}                editar / a la papelera
POST          /api/v1/notes/{id}/move           reubicar (arrastrar y soltar)
POST          /api/v1/notes/{id}/state          marcar un estado de etiqueta
POST          /api/v1/notes/{id}/restore        sacar de la papelera
DELETE        /api/v1/notes/{id}/purge          borrar definitivamente
GET           /api/v1/notes/search?q=           búsqueda (sin q: recientes)
GET           /api/v1/notes/linked?target_kind=&target_id=
                                                consulta INVERSA (la usa Calendar)
GET           /api/v1/notes/trash               papelera
GET|POST|PUT|DELETE /api/v1/tags[/{id}]         etiquetas con sus estados
GET           /public/notebooks/{token}         cuaderno publicado, SIN sesión
```

<a id="desarrollo"></a>

## Development

```bash
SYTUE_ENV=dev \
DATABASE_URL="postgres://postgres:postgres@127.0.0.1:5432/sytue_notes?sslmode=disable" \
OIDC_ISSUER="http://localhost:8081" \
go run ./services/notes/cmd/notes      # sirve :8089

go test ./services/notes/...                        # dominio y saneado
go test -tags integration ./services/notes/...      # RLS y visibilidad (necesita Docker)
```
