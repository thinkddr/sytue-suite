// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

// Package store guarda el ÍNDICE de Sytue Photos en PostgreSQL: qué fotos hay, cómo
// se veían y qué receta tiene cada una. Los bytes no pasan por aquí —el original
// está en Drive y las derivadas en el almacén de objetos—, así que este paquete
// nunca abre un fichero.
package store

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"git.sytue.eu/sytue/one/platform/pkg/pg"
	"git.sytue.eu/sytue/one/platform/pkg/tenancy"
	"git.sytue.eu/sytue/one/services/syphotos/internal/domain"
)

// Store es el acceso a datos de Sytue Photos.
type Store struct {
	pool *pg.Pool
}

// New construye un Store sobre el pool dado.
func New(pool *pg.Pool) *Store { return &Store{pool: pool} }

// DB expone el pool para lo que necesita hablar con Postgres sin pasar por el
// store: el limitador del enlace público, que cuenta visitas de forma atómica.
func (s *Store) DB() *pg.Pool { return s.pool }

// Healthz delega en el pool (para sondas /readyz).
func (s *Store) Healthz(ctx context.Context) error { return s.pool.Healthz(ctx) }

// Las columnas de un activo, en el orden que espera `scanActivo`.
//
// DOS FORMAS DE LA MISMA LISTA, y no es duplicación: los `SELECT` van sobre
// `FROM assets a` y necesitan el alias, pero un `INSERT … RETURNING` no tiene
// tabla que aliasar y Postgres responde «missing FROM-clause entry for table
// "a"». En Sytue Notes eso hizo que crear y actualizar un cuaderno fallaran
// SIEMPRE contra una base real, sin que lo viera ningún test de unidad.
//
// La aliasada se DERIVA de la otra para que no puedan separarse.
const colsBase = "id, owner_sub, drive_file_id, name, media_type, bytes, " +
	"width, height, orientation, taken_at, uploaded_at, favorite, camera, lens, " +
	"latitude, longitude, description, tags"

// La misma lista con el alias delante. En UNA línea la base para que este
// reemplazo sea evidente: no hay saltos ni tabuladores que esquivar.
var cols = "a." + strings.ReplaceAll(colsBase, ", ", ", a.")

func scanActivo(row pgx.Row, tenant string) (domain.Activo, error) {
	var a domain.Activo
	var orient int16
	err := row.Scan(&a.ID, &a.Duenyo, &a.DriveID, &a.Nombre, &a.Tipo, &a.Bytes,
		&a.Ancho, &a.Alto, &orient, &a.TomadaEn, &a.SubidaEn, &a.Favorita, &a.Camara, &a.Objetivo, &a.Lat, &a.Lon, &a.Descripcion, &a.Etiquetas)
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.Activo{}, domain.ErrNotFound
	}
	if err != nil {
		return domain.Activo{}, err
	}
	a.Orientacion = domain.OrientacionDe(int(orient))
	a.Tenant = tenant
	return a, nil
}

// Ingerir da de alta un fichero de Drive en la galería.
//
// IDEMPOTENTE POR (organización, fichero de Drive). Reingerir el mismo fichero
// —porque se reindexa una carpeta, o porque dos clientes suben a la vez— tiene
// que devolver el activo que ya existe, no crear un segundo: la línea de tiempo
// saldría doble y la persona no tendría forma de saber cuál borrar. El UNIQUE
// del esquema lo garantiza; el `ON CONFLICT` lo convierte en «devuélveme el
// que hay» en vez de en un error que quien llama tendría que distinguir.
//
// Se actualizan los metadatos al reingerir: si la primera pasada no supo leer
// el EXIF y la segunda sí, la fecha de captura mejora sin duplicar la foto.
func (s *Store) Ingerir(ctx context.Context, a domain.Activo) (domain.Activo, error) {
	if a.Ancho <= 0 || a.Alto <= 0 {
		return domain.Activo{}, fmt.Errorf("%w: un activo sin dimensiones no se puede colocar en la cuadrícula", domain.ErrInvalidInput)
	}
	if a.DriveID == "" || a.Duenyo == "" {
		return domain.Activo{}, fmt.Errorf("%w: falta el fichero de Drive o el dueño", domain.ErrInvalidInput)
	}
	var out domain.Activo
	err := s.pool.InTenantTx(ctx, func(tx pgx.Tx) error {
		tenant, err := tenantDe(ctx)
		if err != nil {
			return err
		}
		row := tx.QueryRow(ctx, `
			INSERT INTO assets (tenant, owner_sub, drive_file_id, name, media_type,
			                    bytes, width, height, orientation, taken_at, camera, lens,
			                    latitude, longitude)
			VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14)
			ON CONFLICT (tenant, drive_file_id) DO UPDATE SET
			    name        = EXCLUDED.name,
			    media_type  = EXCLUDED.media_type,
			    bytes       = EXCLUDED.bytes,
			    width       = EXCLUDED.width,
			    height      = EXCLUDED.height,
			    orientation = EXCLUDED.orientation,
			    -- La fecha de captura solo MEJORA: si ya se leyó una y esta pasada
			    -- no trae ninguna, no se pierde.
			    taken_at    = COALESCE(EXCLUDED.taken_at, assets.taken_at),
			    camera      = EXCLUDED.camera,
			    lens        = EXCLUDED.lens,
			    -- El lugar, como la fecha, solo MEJORA: una segunda pasada que
			    -- no supo leer el GPS no borra el que ya se leyó.
			    latitude    = COALESCE(EXCLUDED.latitude, assets.latitude),
			    longitude   = COALESCE(EXCLUDED.longitude, assets.longitude),
			    deleted_at  = NULL,
			    updated_at  = now()
			-- Solo la PROPIA: un fichero que ya está en Photos a nombre de otra persona
			-- no se le quita, ni se le reescriben los datos, ni se le saca de la papelera.
			WHERE assets.owner_sub = EXCLUDED.owner_sub
			RETURNING `+colsBase,
			tenant, a.Duenyo, a.DriveID, a.Nombre, a.Tipo, a.Bytes,
			a.Ancho, a.Alto, int16(a.Orientacion), a.TomadaEn, a.Camara, a.Objetivo,
			a.Lat, a.Lon)
		// Sin fila (el fichero es de otra persona) scanActivo da ErrNotFound.
		out, err = scanActivo(row, tenant)
		return err
	})
	return out, err
}

// AltaDeCopia da de alta en Photos, a nombre de su dueña, la copia de Drive de una foto
// que alguien con edición le añade a un álbum (edicion.go). A diferencia de Ingerir, NO
// toca lo que ya hubiera: si ese fichero ya estaba —la dueña ya tenía esa foto—, devuelve
// su activo solo si es de ella y está vivo; nada se sobrescribe ni sale de la papelera.
func (s *Store) AltaDeCopia(ctx context.Context, a domain.Activo) (domain.Activo, error) {
	var out domain.Activo
	err := s.pool.InTenantTx(ctx, func(tx pgx.Tx) error {
		tenant, err := tenantDe(ctx)
		if err != nil {
			return err
		}
		out, err = scanActivo(tx.QueryRow(ctx, `
			INSERT INTO assets (tenant, owner_sub, drive_file_id, name, media_type,
			                    bytes, width, height, orientation, taken_at, camera, lens)
			VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12)
			ON CONFLICT (tenant, drive_file_id) DO NOTHING
			RETURNING `+colsBase,
			tenant, a.Duenyo, a.DriveID, a.Nombre, a.Tipo, a.Bytes,
			a.Ancho, a.Alto, int16(a.Orientacion), a.TomadaEn, a.Camara, a.Objetivo), tenant)
		if !errors.Is(err, domain.ErrNotFound) {
			return err
		}
		out, err = scanActivo(tx.QueryRow(ctx, `
			SELECT `+cols+` FROM assets a
			 WHERE a.tenant = $1 AND a.drive_file_id = $2 AND a.owner_sub = $3 AND a.deleted_at IS NULL`,
			tenant, a.DriveID, a.Duenyo), tenant)
		return err
	})
	return out, err
}

// Cursor es la posición en la línea de tiempo. Se pagina por (fecha, id) y no
// por OFFSET: con OFFSET, subir una foto mientras alguien se desplaza le
// desplaza todo y le repite o le salta una fila. El id desempata las fotos con
// la misma fecha, que son muchas —una ráfaga comparte segundo—.
type Cursor struct {
	Fecha time.Time
	ID    uuid.UUID
}

// FiltroLinea acota la línea de tiempo.
type FiltroLinea struct {
	Duenyo string
	Limite int
	Desde  *Cursor
	// Texto filtra por nombre de fichero, sin distinguir mayúsculas. Vacío es
	// «todas». ponytail: ILIKE sobre la línea de una persona; si una biblioteca
	// de cientos de miles lo nota, un índice trigram (pg_trgm) sobre name.
	Texto string
	// Tambien son términos ALTERNATIVOS: una foto entra si casa con `Texto` o
	// con cualquiera de estos. Los propone Sytue AI a partir de lo que se
	// escribió, para que «playa» encuentre también lo descrito como «orilla del
	// mar». Vacío es la búsqueda literal de siempre.
	Tambien []string
	// SoloFavoritas es la vista de favoritas: la misma línea con un filtro, el
	// mismo cursor y el mismo orden, servida por el índice `assets_favoritas`.
	SoloFavoritas bool
}

// LineaDeTiempo devuelve las fotos de una persona, de la más reciente a la más
// antigua, y el cursor para pedir la página siguiente (nulo si no hay más).
//
// El ORDEN es `coalesce(taken_at, uploaded_at) DESC, id DESC`, exactamente la
// expresión del índice `assets_timeline`: cualquier variación —un `taken_at
// DESC NULLS LAST`, por ejemplo— deja de poder usarlo y Postgres ordena en
// memoria toda la colección en cada desplazamiento.
// patronILike escapa lo que ILIKE interpreta, para buscar el texto literal.
var patronILike = strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`)

func (s *Store) LineaDeTiempo(ctx context.Context, f FiltroLinea) ([]domain.Activo, *Cursor, error) {
	limite := f.Limite
	if limite <= 0 || limite > 200 {
		limite = 100
	}
	var activos []domain.Activo
	var siguiente *Cursor
	err := s.pool.InTenantTx(ctx, func(tx pgx.Tx) error {
		tenant, err := tenantDe(ctx)
		if err != nil {
			return err
		}
		// Se piden `limite+1` para saber si hay más SIN una segunda consulta de
		// conteo: contar toda la colección en cada página es el otro modo de que
		// esto se degrade con treinta mil fotos.
		args := []any{tenant, f.Duenyo, limite + 1}
		cond := ""
		if f.Desde != nil {
			cond = ` AND (coalesce(a.taken_at, a.uploaded_at), a.id) < ($4, $5)`
			args = append(args, f.Desde.Fecha, f.Desde.ID)
		}
		if f.SoloFavoritas {
			cond += ` AND a.favorite`
		}
		if texto := strings.TrimSpace(f.Texto); texto != "" {
			// Los comodines del propio texto se escapan: buscar «100%» es
			// buscar ese nombre, no todo lo que empiece por 100.
			patron := "%" + patronILike.Replace(texto) + "%"
			args = append(args, patron)
			// El nombre, lo que SALE en la foto y sus etiquetas. Buscar «playa»
			// encontraba las que alguien se acordó de llamar «playa», no las que
			// tienen una playa.
			// UN SOLO PATRÓN POR TÉRMINO Y TODO EN UN `OR`. Los alternativos se
			// añaden a la misma condición en vez de en otra `AND`: son formas de
			// decir lo mismo, así que casar con cualquiera basta. Con dos `AND`
			// habría que casar con todos, que es justo lo contrario.
			donde := []string{fmt.Sprintf(`(
				plegar(a.name) LIKE plegar($%[1]d) ESCAPE '\'
				OR plegar(a.description) LIKE plegar($%[1]d) ESCAPE '\'
				OR EXISTS (SELECT 1 FROM unnest(a.tags) tg WHERE plegar(tg) LIKE plegar($%[1]d) ESCAPE '\')
			)`, len(args))}
			for _, otro := range f.Tambien {
				otro = strings.TrimSpace(otro)
				if otro == "" {
					continue
				}
				args = append(args, "%"+patronILike.Replace(otro)+"%")
				donde = append(donde, fmt.Sprintf(`(
					plegar(a.name) LIKE plegar($%[1]d) ESCAPE '\'
					OR plegar(a.description) LIKE plegar($%[1]d) ESCAPE '\'
					OR EXISTS (SELECT 1 FROM unnest(a.tags) tg WHERE plegar(tg) LIKE plegar($%[1]d) ESCAPE '\')
				)`, len(args)))
			}
			cond += ` AND (` + strings.Join(donde, " OR ") + `)`
		}
		rows, err := tx.Query(ctx, `
			SELECT `+cols+`
			FROM assets a
			WHERE a.tenant = $1 AND a.owner_sub = $2 AND a.deleted_at IS NULL`+cond+`
			ORDER BY coalesce(a.taken_at, a.uploaded_at) DESC, a.id DESC
			LIMIT $3`, args...)
		if err != nil {
			return err
		}
		defer rows.Close()
		activos = activos[:0]
		for rows.Next() {
			a, err := scanActivo(rows, tenant)
			if err != nil {
				return err
			}
			activos = append(activos, a)
		}
		if err := rows.Err(); err != nil {
			return err
		}
		if len(activos) > limite {
			activos = activos[:limite]
			ultima := activos[len(activos)-1]
			cuando, _ := ultima.FechaDeGaleria()
			siguiente = &Cursor{Fecha: cuando, ID: ultima.ID}
		}
		return nil
	})
	return activos, siguiente, err
}

// Activo devuelve una foto por id, SI es de esa persona.
//
// El dueño va en la consulta y no en una comprobación posterior porque la
// respuesta tiene que ser la misma —«no existe»— para una foto que no está y
// para una que es de otra persona: con dos respuestas distintas, preguntar por
// identificadores dice qué fotos hay en la organización.
//
// La RLS solo aísla por ORGANIZACIÓN: dentro de una, el filtro por `owner_sub`
// es lo único que separa a dos personas. Todo lo que lea o toque un activo por
// id tiene que llevarlo.
func (s *Store) Activo(ctx context.Context, id uuid.UUID, duenyo string) (domain.Activo, error) {
	var a domain.Activo
	err := s.pool.InTenantTx(ctx, func(tx pgx.Tx) error {
		tenant, err := tenantDe(ctx)
		if err != nil {
			return err
		}
		row := tx.QueryRow(ctx, `
			SELECT `+cols+` FROM assets a
			WHERE a.tenant = $1 AND a.id = $2 AND a.owner_sub = $3 AND a.deleted_at IS NULL`,
			tenant, id, duenyo)
		a, err = scanActivo(row, tenant)
		return err
	})
	return a, err
}

// ALaPapelera marca una foto como borrada. NO toca Drive: quitar una foto de la
// galería no puede borrar el fichero de quien lo tuviera en una carpeta
// compartida. Vaciar la papelera de verdad es otra operación, y la que borra en
// Drive tiene que pedirlo explícitamente.
func (s *Store) ALaPapelera(ctx context.Context, id uuid.UUID, duenyo string) error {
	return s.pool.InTenantTx(ctx, func(tx pgx.Tx) error {
		tenant, err := tenantDe(ctx)
		if err != nil {
			return err
		}
		tag, err := tx.Exec(ctx, `
			UPDATE assets SET deleted_at = now(), updated_at = now()
			WHERE tenant = $1 AND id = $2 AND owner_sub = $3 AND deleted_at IS NULL`,
			tenant, id, duenyo)
		if err != nil {
			return err
		}
		if tag.RowsAffected() == 0 {
			return domain.ErrNotFound
		}
		return nil
	})
}

// GuardarReceta fija la edición vigente de una foto para una persona.
//
// UNA POR ACTIVO Y PERSONA, no un historial: el historial ya lo da la propia
// receta —deshacer es quitar un paso— y guardar cada estado intermedio
// multiplicaría las filas por cada arrastre de un deslizador.
//
// Devuelve la huella, que es con lo que se nombran las derivadas.
func (s *Store) GuardarReceta(ctx context.Context, activo uuid.UUID, duenyo string, r domain.Receta) (string, error) {
	if err := r.Valida(); err != nil {
		return "", err
	}
	huella, err := r.Huella()
	if err != nil {
		return "", err
	}
	crudo, err := json.Marshal(r)
	if err != nil {
		return "", err
	}
	err = s.pool.InTenantTx(ctx, func(tx pgx.Tx) error {
		tenant, err := tenantDe(ctx)
		if err != nil {
			return err
		}
		_, err = tx.Exec(ctx, `
			INSERT INTO edits (tenant, asset_id, owner_sub, recipe, fingerprint)
			VALUES ($1,$2,$3,$4,$5)
			ON CONFLICT (tenant, asset_id, owner_sub) DO UPDATE SET
			    recipe = EXCLUDED.recipe, fingerprint = EXCLUDED.fingerprint,
			    updated_at = now()`,
			tenant, activo, duenyo, crudo, huella)
		return err
	})
	return huella, err
}

// Receta devuelve la edición vigente. Sin fila, la receta VACÍA y no un error:
// una foto sin editar no es un caso de excepción, es el estado normal, y
// tratarlo como error obliga a quien llama a distinguir dos nadas.
func (s *Store) Receta(ctx context.Context, activo uuid.UUID, duenyo string) (domain.Receta, error) {
	r := domain.RecetaVacia()
	err := s.pool.InTenantTx(ctx, func(tx pgx.Tx) error {
		tenant, err := tenantDe(ctx)
		if err != nil {
			return err
		}
		var crudo []byte
		err = tx.QueryRow(ctx, `
			SELECT recipe FROM edits
			WHERE tenant = $1 AND asset_id = $2 AND owner_sub = $3`,
			tenant, activo, duenyo).Scan(&crudo)
		if errors.Is(err, pgx.ErrNoRows) {
			return nil
		}
		if err != nil {
			return err
		}
		return json.Unmarshal(crudo, &r)
	})
	return r, err
}

// tenantDe saca el slug de la organización del contexto.
//
// Se pide APARTE de `InTenantTx` —que ya lo usa para fijar `app.tenant`— porque
// las consultas también filtran por la columna `tenant`. Podría fiarse solo de
// la RLS, y no se hace: el filtro explícito es lo que permite que los índices
// empiecen por `tenant` y que el planificador acote antes de aplicar la
// política. La RLS es la red, no el camino.
func tenantDe(ctx context.Context) (string, error) {
	t, err := tenancy.From(ctx)
	if err != nil {
		return "", err
	}
	return t.Slug, nil
}

// Papelera lista las fotos borradas de esa persona, la última borrada primero.
func (s *Store) Papelera(ctx context.Context, duenyo string) ([]domain.Activo, error) {
	var out []domain.Activo
	err := s.pool.InTenantTx(ctx, func(tx pgx.Tx) error {
		tenant, err := tenantDe(ctx)
		if err != nil {
			return err
		}
		rows, err := tx.Query(ctx, `
			SELECT `+cols+` FROM assets a
			WHERE a.tenant = $1 AND a.owner_sub = $2 AND a.deleted_at IS NOT NULL
			ORDER BY a.deleted_at DESC, a.id DESC LIMIT 500`, tenant, duenyo)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			a, err := scanActivo(rows, tenant)
			if err != nil {
				return err
			}
			out = append(out, a)
		}
		return rows.Err()
	})
	return out, err
}

// Restaurar saca una foto de la papelera. Las pertenencias a álbumes se
// conservaron —borrar no las tocó—, así que vuelve donde estaba.
func (s *Store) Restaurar(ctx context.Context, id uuid.UUID, duenyo string) error {
	return s.pool.InTenantTx(ctx, func(tx pgx.Tx) error {
		tenant, err := tenantDe(ctx)
		if err != nil {
			return err
		}
		tag, err := tx.Exec(ctx, `
			UPDATE assets SET deleted_at = NULL, updated_at = now()
			WHERE tenant = $1 AND id = $2 AND owner_sub = $3 AND deleted_at IS NOT NULL`,
			tenant, id, duenyo)
		if err != nil {
			return err
		}
		if tag.RowsAffected() == 0 {
			return domain.ErrNotFound
		}
		return nil
	})
}

// MarcarFavorita pone o quita la marca de favorita.
//
// Devuelve `ErrNotFound` si la foto no es de esa persona o está en la papelera:
// marcar algo que no se ve no tendría dónde notarse.
func (s *Store) MarcarFavorita(ctx context.Context, id uuid.UUID, duenyo string, favorita bool) error {
	return s.pool.InTenantTx(ctx, func(tx pgx.Tx) error {
		tenant, err := tenantDe(ctx)
		if err != nil {
			return err
		}
		tag, err := tx.Exec(ctx, `
			UPDATE assets SET favorite = $4, updated_at = now()
			WHERE tenant = $1 AND id = $2 AND owner_sub = $3 AND deleted_at IS NULL`,
			tenant, id, duenyo, favorita)
		if err != nil {
			return err
		}
		if tag.RowsAffected() == 0 {
			return domain.ErrNotFound
		}
		return nil
	})
}

// Purgada identifica una foto que se ha borrado DE VERDAD, con la huella de su
// receta: es lo que hace falta para nombrar las derivadas que se quedan sin
// dueño en el almacén de objetos.
type Purgada struct {
	ID uuid.UUID
	// Tenant: el worker de retención recorre todas las organizaciones, así que
	// la clave del objeto no se puede componer sin saber de cuál es la foto.
	Tenant string
	// Huella de la receta vigente cuando se borró. Vacía si nunca se editó.
	Huella string
}

// Purgar borra de verdad lo que está EN LA PAPELERA. Con `id` nulo la vacía
// entera; con un id, borra esa foto y solo si está dentro.
//
// NO TOCA DRIVE. El original es de Drive y puede estar compartido: aquí se
// borra el índice —la foto deja de estar en la galería— y quien quiera borrar
// el fichero lo hace en Drive. Reindexar esa carpeta la traería de vuelta, que
// es exactamente lo que tiene que pasar.
//
// Se lee ANTES de borrar y en dos sentencias dentro de la misma transacción, no
// con un `DELETE ... RETURNING` y un JOIN: las recetas se van en cascada con la
// foto, y depender de si el disparador de la cascada ya corrió dentro de la
// misma sentencia es apostar sobre un detalle del motor.
func (s *Store) Purgar(ctx context.Context, duenyo string, id *uuid.UUID) ([]Purgada, error) {
	var out []Purgada
	err := s.pool.InTenantTx(ctx, func(tx pgx.Tx) error {
		tenant, err := tenantDe(ctx)
		if err != nil {
			return err
		}
		cond := ""
		args := []any{tenant, duenyo}
		if id != nil {
			cond = " AND a.id = $3"
			args = append(args, *id)
		}
		rows, err := tx.Query(ctx, `
			SELECT a.id, coalesce(e.fingerprint, '')
			FROM assets a
			LEFT JOIN edits e
			  ON e.tenant = a.tenant AND e.asset_id = a.id AND e.owner_sub = a.owner_sub
			WHERE a.tenant = $1 AND a.owner_sub = $2 AND a.deleted_at IS NOT NULL`+cond, args...)
		if err != nil {
			return err
		}
		defer rows.Close()
		out = out[:0]
		for rows.Next() {
			var p Purgada
			if err := rows.Scan(&p.ID, &p.Huella); err != nil {
				return err
			}
			out = append(out, p)
		}
		if err := rows.Err(); err != nil {
			return err
		}
		if len(out) == 0 {
			if id != nil {
				return domain.ErrNotFound
			}
			return nil
		}
		condDel := ""
		if id != nil {
			condDel = " AND id = $3"
		}
		_, err = tx.Exec(ctx, `
			DELETE FROM assets
			WHERE tenant = $1 AND owner_sub = $2 AND deleted_at IS NOT NULL`+condDel, args...)
		return err
	})
	return out, err
}

// MaxEtiquetas y MaxDescripcion acotan lo que se guarda. El texto viene de un
// modelo, o sea de fuera: sin tope, una respuesta larga se guarda entera y se
// pinta entera en la ficha.
const (
	MaxEtiquetas   = 12
	MaxDescripcion = 500
)

// GuardarDescripcion fija qué sale en la foto.
//
// Recorta en vez de rechazar: quien pulsó «describir» quiere su descripción, y
// que el modelo se haya pasado de largo no es un error suyo. Lo que no se
// admite es una foto que no sea de esa persona.
func (s *Store) GuardarDescripcion(ctx context.Context, id uuid.UUID, duenyo, descripcion string, etiquetas []string) error {
	descripcion = strings.TrimSpace(descripcion)
	if len(descripcion) > MaxDescripcion {
		descripcion = descripcion[:MaxDescripcion]
	}
	limpias := make([]string, 0, MaxEtiquetas)
	vistas := map[string]bool{}
	for _, e := range etiquetas {
		e = strings.TrimSpace(e)
		if e == "" || len(e) > 60 || vistas[strings.ToLower(e)] {
			continue
		}
		vistas[strings.ToLower(e)] = true
		limpias = append(limpias, e)
		if len(limpias) == MaxEtiquetas {
			break
		}
	}
	return s.pool.InTenantTx(ctx, func(tx pgx.Tx) error {
		tenant, err := tenantDe(ctx)
		if err != nil {
			return err
		}
		tag, err := tx.Exec(ctx, `
			UPDATE assets SET description = $4, tags = $5, updated_at = now()
			WHERE tenant = $1 AND id = $2 AND owner_sub = $3 AND deleted_at IS NULL`,
			tenant, id, duenyo, descripcion, limpias)
		if err != nil {
			return err
		}
		if tag.RowsAffected() == 0 {
			return domain.ErrNotFound
		}
		return nil
	})
}
