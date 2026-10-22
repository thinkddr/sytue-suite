# Sytue Notepads (servicio Go)

Backend de **Sytue Notepads** (`notes.sytue.eu`, dev `:8089`): cuadernos anidados,
notas heterogéneas, etiquetas con estado, búsqueda y referencias cruzadas con el
resto de la suite. Estado en **PostgreSQL** (única fuente de verdad); auth con los
JWT de **Sytue ID** validados por JWKS.

Implementación **clean-room**: modelo propio. La inspiración funcional
—contenedores anidados de notas de distinto tipo, etiquetas con estado, filtro—
son ideas, y las ideas no tienen copyright; no se ha consultado ni portado código
de terceros (docs/CONVENTIONS.md §12).

## Modelo

| Tabla                                     | Qué guarda                                                                                                                                      |
| ----------------------------------------- | ----------------------------------------------------------------------------------------------------------------------------------------------- |
| `notebooks`                               | El árbol de cuadernos: título, icono, color, disposición, visibilidad, indexación, cifrado, sala de Rooms vinculada y token del enlace público. |
| `notes`                                   | Las notas: tipo, contenido (HTML **saneado** o criptograma), posición, agrupación y las sobreescrituras de visibilidad/indexación.              |
| `tags` / `tag_states` / `note_tag_states` | Etiquetas con estados excluyentes y sus marcas.                                                                                                 |
| `note_links`                              | Enlaces a objetos de la suite. Es la tabla que responde la consulta INVERSA («¿qué notas cuelgan de este evento?»).                             |

## Las tres invariantes que sostiene el esquema

1. **Aislamiento por organización**: RLS `ENABLE` + `FORCE` en las seis tablas.
   La separación entre PERSONAS de la misma organización la aplican las cláusulas
   de visibilidad de `internal/store`, en cada consulta.
2. **Cifrado y búsqueda son incompatibles**: `notebook_cifrado_no_indexa`. No hay
   texto en claro que indexar, y una comprobación en el servicio se puede olvidar.
3. **O en claro o cifrado, nunca las dos cosas**: `note_o_claro_o_cifrado`. Guardar
   el título «por si acaso» publicaría lo más revelador de una nota cifrada.

## Saneado del texto enriquecido

`internal/domain/html.go` reduce el HTML a una **allowlist** cerrada con
`golang.org/x/net/html` (ya en el módulo: lo usa el unfurl de Rooms). Se hace en
el servidor porque el contenido de un cuaderno de organización lo pinta el
navegador de OTRA persona: saneado en cliente no es saneado.

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

## Desarrollo

```bash
SYTUE_ENV=dev \
DATABASE_URL="postgres://postgres:postgres@127.0.0.1:5432/sytue_notes?sslmode=disable" \
OIDC_ISSUER="http://localhost:8081" \
go run ./services/notes/cmd/notes      # sirve :8089

go test ./services/notes/...                        # dominio y saneado
go test -tags integration ./services/notes/...      # RLS y visibilidad (necesita Docker)
```
