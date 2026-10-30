// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

// Package store implementa la persistencia de metadatos de Sytue Drive sobre
// PostgreSQL (pgx). Todo acceso pasa por transacciones con `app.tenant` fijado
// (pg.InTenantTx) para que RLS aísle por tenant. Consultas SIEMPRE
// parametrizadas (sin SQL dinámico), conforme a CONVENTIONS §10.
package store

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"math"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"git.sytue.eu/sytue/one/platform/pkg/authn"
	"git.sytue.eu/sytue/one/platform/pkg/pg"
	"git.sytue.eu/sytue/one/platform/pkg/tenancy"
	"git.sytue.eu/sytue/one/services/drive/internal/domain"
)

type ownerCtxKey struct{}

// WithOwner fija el propietario (`owner_sub`) del ámbito personal («Mi unidad»)
// en el contexto cuando el llamador no entra por el middleware HTTP de authn
// o cuando resuelve deliberadamente el ámbito de otra persona (compartición,
// enlace público, sincronización interna de Docs o copia hacia la dueña).
func WithOwner(ctx context.Context, ownerSub string) context.Context {
	return context.WithValue(ctx, ownerCtxKey{}, ownerSub)
}

// OwnerFrom devuelve el identificador de la persona cuyo ámbito personal («Mi
// unidad») se consulta: primero el fijado con WithOwner y, si no lo hay, el
// Subject de authn.FromContext.
func OwnerFrom(ctx context.Context) string {
	if v, ok := ctx.Value(ownerCtxKey{}).(string); ok {
		return v
	}
	if c, ok := authn.FromContext(ctx); ok {
		return c.Subject
	}
	return ""
}

// Store agrupa el acceso a datos de Drive.
type Store struct {
	pool *pg.Pool
}

// New construye un Store sobre el pool dado.
func New(pool *pg.Pool) *Store { return &Store{pool: pool} }

// Healthz delega en el pool (para sondas /readyz).
func (s *Store) Healthz(ctx context.Context) error { return s.pool.Healthz(ctx) }

// inTx abre una transacción aislada por tenant (`app.tenant`) y, en el ámbito
// personal («Mi unidad»), fija también `app.owner_sub` con el propietario del
// contexto (`OwnerFrom(ctx)`).
func (s *Store) inTx(ctx context.Context, fn func(tx pgx.Tx) error) error {
	owner := OwnerFrom(ctx)
	return s.pool.InTenantTx(ctx, func(tx pgx.Tx) error {
		if owner != "" {
			if _, err := tx.Exec(ctx, "SELECT set_config('app.owner_sub', $1, true)", owner); err != nil {
				return fmt.Errorf("store: fijar app.owner_sub: %w", err)
			}
		}
		return fn(tx)
	})
}

// uniqueViolation indica si el error es una violación de unicidad (23505).
func uniqueViolation(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == "23505"
}

// ---------------------------------------------------------------------------
// Carpetas
// ---------------------------------------------------------------------------

const folderCols = `id, parent_id, name, is_root, is_group, owner_sub, created_at, updated_at, deleted_at`

func scanFolder(row pgx.Row, t string) (domain.Folder, error) {
	var f domain.Folder
	if err := row.Scan(&f.ID, &f.ParentID, &f.Name, &f.IsRoot, &f.IsGroup, &f.OwnerSub, &f.CreatedAt, &f.UpdatedAt, &f.DeletedAt); err != nil {
		return domain.Folder{}, err
	}
	f.Tenant = t
	return f, nil
}

// EnsureRoot devuelve la carpeta raíz del tenant, creándola si no existe.
func (s *Store) EnsureRoot(ctx context.Context) (domain.Folder, error) {
	t := tenancy.MustFrom(ctx)
	var f domain.Folder
	err := s.inTx(ctx, func(tx pgx.Tx) error {
		var e error
		f, e = ensureRootTx(ctx, tx, t.Slug)
		return e
	})
	return f, err
}

// ensureRootTx es el cuerpo de EnsureRoot dentro de una transacción YA ABIERTA.
func ensureRootTx(ctx context.Context, tx pgx.Tx, slug string) (domain.Folder, error) {
	row := tx.QueryRow(ctx, `SELECT `+folderCols+` FROM folders WHERE tenant=$1 AND `+ambitoSQL+` AND is_root`, slug)
	f, err := scanFolder(row, slug)
	if err == nil {
		return f, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return domain.Folder{}, fmt.Errorf("store: leer raíz: %w", err)
	}
	owner := OwnerFrom(ctx)
	insert := tx.QueryRow(ctx,
		`INSERT INTO folders (tenant, parent_id, name, is_root, owner_sub) VALUES ($1, NULL, '', true, $2)
		 ON CONFLICT (tenant, owner_sub) WHERE is_root DO UPDATE SET updated_at = folders.updated_at
		 RETURNING `+folderCols, slug, owner)
	f, err = scanFolder(insert, slug)
	if err != nil {
		return domain.Folder{}, fmt.Errorf("store: crear raíz: %w", err)
	}
	return f, nil
}

// EnsureBase deja listos cuota y raíz del tenant en UNA sola transacción.
//
// Eran dos —y en `EnsureTenant`, tres, porque además releía la cuota que
// `EnsureQuota` ya había devuelto—. `Service.Root()` pasa por aquí en CADA
// resolución de ruta de WebDAV: cada PROPFIND, cada GET y cada PUT pagaban dos
// BEGIN/COMMIT para dos SELECT que casi siempre aciertan.
//
// Idempotente igual que las dos por separado, y con la misma semántica de
// arranque: el primer acceso de un tenant sigue creando lo que falte.
func (s *Store) EnsureBase(ctx context.Context, limit int64, fijar bool) (domain.Folder, domain.Quota, error) {
	t := tenancy.MustFrom(ctx)
	var (
		f domain.Folder
		q domain.Quota
	)
	err := s.inTx(ctx, func(tx pgx.Tx) error {
		var e error
		if q, e = ensureQuotaTx(ctx, tx, t.Slug, limit, fijar); e != nil {
			return e
		}
		f, e = ensureRootTx(ctx, tx, t.Slug)
		return e
	})
	return f, q, err
}

// GetFolder devuelve una carpeta viva por ID del ámbito PERSONAL. La carpeta de un
// canal de Rooms no es direccionable aquí (channel_id IS NULL, ver GetFile): si no,
// su nombre y su contenido se alcanzarían conociendo el uuid.
func (s *Store) GetFolder(ctx context.Context, id uuid.UUID) (domain.Folder, error) {
	t := tenancy.MustFrom(ctx)
	var f domain.Folder
	err := s.inTx(ctx, func(tx pgx.Tx) error {
		row := tx.QueryRow(ctx,
			`SELECT `+folderCols+` FROM folders
			 WHERE tenant=$1 AND `+ambitoSQL+` AND id=$2 AND deleted_at IS NULL AND channel_id IS NULL`,
			t.Slug, id)
		var e error
		f, e = scanFolder(row, t.Slug)
		return e
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.Folder{}, domain.ErrNotFound
	}
	return f, err
}

// listSubfoldersTx lista las subcarpetas vivas de una carpeta dentro de una tx.
func listSubfoldersTx(ctx context.Context, tx pgx.Tx, tenant string, folderID uuid.UUID) ([]domain.Folder, error) {
	rows, err := tx.Query(ctx,
		`SELECT `+folderCols+` FROM folders
		 WHERE tenant=$1 AND `+ambitoSQL+` AND parent_id=$2 AND deleted_at IS NULL ORDER BY lower(name)`,
		tenant, folderID)
	if err != nil {
		return nil, fmt.Errorf("store: listar subcarpetas: %w", err)
	}
	defer rows.Close()
	var folders []domain.Folder
	for rows.Next() {
		f, err := scanFolder(rows, tenant)
		if err != nil {
			return nil, err
		}
		folders = append(folders, f)
	}
	return folders, rows.Err()
}

// ListSubfolders lista solo las subcarpetas vivas de una carpeta, ordenadas por
// nombre. Los ficheros se listan aparte con ListFilesPage (paginación keyset).
// ponytail: sin paginar — el nº de carpetas hermanas es humano (decenas), no
// crece como los ficheros; paginar si algún tenant lo desmiente.
func (s *Store) ListSubfolders(ctx context.Context, folderID uuid.UUID) ([]domain.Folder, error) {
	t := tenancy.MustFrom(ctx)
	var folders []domain.Folder
	err := s.inTx(ctx, func(tx pgx.Tx) error {
		var e error
		folders, e = listSubfoldersTx(ctx, tx, t.Slug, folderID)
		return e
	})
	return folders, err
}

// ListChildren lista subcarpetas y ficheros vivos de una carpeta.
func (s *Store) ListChildren(ctx context.Context, folderID uuid.UUID) ([]domain.Folder, []domain.File, error) {
	t := tenancy.MustFrom(ctx)
	var folders []domain.Folder
	var files []domain.File
	err := s.inTx(ctx, func(tx pgx.Tx) error {
		var err error
		folders, err = listSubfoldersTx(ctx, tx, t.Slug, folderID)
		if err != nil {
			return err
		}

		frows, err := tx.Query(ctx,
			// channel_id IS NULL: aunque se pase el uuid de la carpeta de un canal, sus
			// adjuntos no se listan por la ruta personal (ver GetFile).
			`SELECT `+fileCols+` FROM files
			 WHERE tenant=$1 AND `+ambitoSQL+` AND folder_id=$2 AND deleted_at IS NULL AND channel_id IS NULL
			 ORDER BY lower(name)`,
			t.Slug, folderID)
		if err != nil {
			return fmt.Errorf("store: listar ficheros: %w", err)
		}
		defer frows.Close()
		for frows.Next() {
			f, err := scanFile(frows, t.Slug)
			if err != nil {
				return err
			}
			files = append(files, f)
		}
		return frows.Err()
	})
	return folders, files, err
}

// ResolveUnder desciende por `segs` desde `root` y devuelve el último nodo de la
// ruta. Puede venir poblada la carpeta, el fichero o LOS DOS —nada impide que un
// fichero y una carpeta hermanos se llamen igual—, y ninguno si la ruta no
// existe. Con qué se queda el llamante cuando vienen los dos es decisión suya.
//
// UNA transacción y un número FIJO de consultas, en vez de una por segmento.
// Antes, resolver `/a/b/c/d.pdf` abría seis transacciones —EnsureQuota,
// EnsureRoot y una por tramo—, y cada tramo intermedio se traía TODAS las
// subcarpetas de su padre para quedarse con una. WebDAV resuelve por ruta en
// cada PROPFIND, GET y PUT, así que se pagaba en todas.
//
// El descenso lo hace Postgres con un WITH RECURSIVE, y no de casualidad:
// `folders_unique_sibling_name (tenant, parent_id, lower(name))` es exactamente
// el índice que necesita cada salto, y ese mismo índice es el que ya impone que
// los hermanos no puedan repetir nombre sin distinguir mayúsculas. O sea, el
// `lower()` de aquí es la MISMA regla que aplicaba `strings.EqualFold` en Go,
// no una aproximación.
func (s *Store) ResolveUnder(ctx context.Context, root uuid.UUID, segs []string) (*domain.Folder, *domain.File, error) {
	if len(segs) == 0 {
		return nil, nil, nil
	}
	t := tenancy.MustFrom(ctx)
	var (
		carpeta *domain.Folder
		fichero *domain.File
	)
	err := s.inTx(ctx, func(tx pgx.Tx) error {
		// El padre del último tramo. Los intermedios son carpetas por definición,
		// así que el descenso no mira la tabla de ficheros ni una vez.
		var padre uuid.UUID
		err := tx.QueryRow(ctx, `
			WITH RECURSIVE segmentos(nombre, nivel) AS (
				SELECT nombre, nivel
				  FROM unnest($2::text[]) WITH ORDINALITY AS t(nombre, nivel)
			),
			descenso(id, nivel) AS (
				SELECT $3::uuid, 0::bigint
				UNION ALL
				SELECT hijo.id, d.nivel + 1
				  FROM descenso d
				  JOIN segmentos s ON s.nivel = d.nivel + 1
				  JOIN folders hijo
				    ON hijo.tenant = $1
				   AND hijo.parent_id = d.id
				   AND hijo.deleted_at IS NULL
				   AND lower(hijo.name) = lower(s.nombre)
				 WHERE d.nivel + 1 < $4
			)
			SELECT id FROM descenso WHERE nivel = $4 - 1`,
			t.Slug, segs, root, len(segs)).Scan(&padre)
		if errors.Is(err, pgx.ErrNoRows) {
			return nil // algún tramo intermedio no existe
		}
		if err != nil {
			return fmt.Errorf("store: descender por la ruta: %w", err)
		}

		// El ÚLTIMO tramo puede ser fichero o carpeta, y pueden existir LOS DOS con
		// el mismo nombre: la unicidad es por tabla, no entre las dos. Así que aquí
		// se buscan ambos y se devuelven ambos; QUIÉN GANA lo decide el llamante,
		// porque no coinciden: resolver una ruta de WebDAV prefiere el fichero y
		// navegar un enlace público prefiere la carpeta. Eran dos recorridos
		// distintos y cada uno traía su preferencia; unificar el recorrido sin
		// unificar la preferencia es la única forma de que ninguno cambie.
		ultimo := segs[len(segs)-1]
		// channel_id IS NULL: los adjuntos de un canal no se resuelven por la ruta
		// personal, igual que no se listan (ver ListChildren).
		f, err := scanFile(tx.QueryRow(ctx,
			`SELECT `+fileCols+` FROM files
			 WHERE tenant=$1 AND `+ambitoSQL+` AND folder_id=$2 AND deleted_at IS NULL AND channel_id IS NULL
			   AND lower(name) = lower($3)`,
			t.Slug, padre, ultimo), t.Slug)
		switch {
		case err == nil:
			fichero = &f
		case !errors.Is(err, pgx.ErrNoRows):
			return fmt.Errorf("store: resolver fichero de la ruta: %w", err)
		}

		c, err := scanFolder(tx.QueryRow(ctx,
			`SELECT `+folderCols+` FROM folders
			 WHERE tenant=$1 AND `+ambitoSQL+` AND parent_id=$2 AND deleted_at IS NULL
			   AND lower(name) = lower($3)`,
			t.Slug, padre, ultimo), t.Slug)
		if errors.Is(err, pgx.ErrNoRows) {
			return nil
		}
		if err != nil {
			return fmt.Errorf("store: resolver carpeta de la ruta: %w", err)
		}
		carpeta = &c
		return nil
	})
	return carpeta, fichero, err
}

// CreateFolder crea una subcarpeta. parentID debe existir y estar viva.
func (s *Store) CreateFolder(ctx context.Context, parentID uuid.UUID, name, _ string, isGroup bool) (domain.Folder, error) {
	t := tenancy.MustFrom(ctx)
	if !domain.ValidName(name) {
		return domain.Folder{}, domain.ErrInvalidName
	}
	folderOwner := OwnerFrom(ctx)
	var f domain.Folder
	err := s.inTx(ctx, func(tx pgx.Tx) error {
		var exists bool
		if err := tx.QueryRow(ctx,
			`SELECT true FROM folders WHERE tenant=$1 AND `+ambitoSQL+` AND id=$2 AND deleted_at IS NULL`,
			t.Slug, parentID).Scan(&exists); err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return domain.ErrInvalidParent
			}
			return err
		}
		row := tx.QueryRow(ctx,
			`INSERT INTO folders (tenant, parent_id, name, is_group, owner_sub)
			 VALUES ($1, $2, $3, $4, $5) RETURNING `+folderCols,
			t.Slug, parentID, strings.TrimSpace(name), isGroup, folderOwner)
		var e error
		f, e = scanFolder(row, t.Slug)
		if uniqueViolation(e) {
			return domain.ErrConflict
		}
		return e
	})
	return f, err
}

// RenameFolder cambia el nombre de una carpeta (no la raíz).
func (s *Store) RenameFolder(ctx context.Context, id uuid.UUID, name string) (domain.Folder, error) {
	t := tenancy.MustFrom(ctx)
	if !domain.ValidName(name) {
		return domain.Folder{}, domain.ErrInvalidName
	}
	var f domain.Folder
	err := s.inTx(ctx, func(tx pgx.Tx) error {
		row := tx.QueryRow(ctx,
			`UPDATE folders SET name=$3, updated_at=now()
			 WHERE tenant=$1 AND `+ambitoSQL+` AND id=$2 AND deleted_at IS NULL AND NOT is_root
			 RETURNING `+folderCols,
			t.Slug, id, strings.TrimSpace(name))
		var e error
		f, e = scanFolder(row, t.Slug)
		if uniqueViolation(e) {
			return domain.ErrConflict
		}
		return e
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.Folder{}, domain.ErrNotFound
	}
	return f, err
}

// MoveFolder reubica una carpeta bajo newParentID, evitando ciclos.
func (s *Store) MoveFolder(ctx context.Context, id, newParentID uuid.UUID) (domain.Folder, error) {
	t := tenancy.MustFrom(ctx)
	if id == newParentID {
		return domain.Folder{}, domain.ErrCycle
	}
	var f domain.Folder
	err := s.inTx(ctx, func(tx pgx.Tx) error {
		// Detección de ciclo: newParent no puede ser descendiente de id.
		descendant, err := isDescendant(ctx, tx, t.Slug, newParentID, id)
		if err != nil {
			return err
		}
		if descendant {
			return domain.ErrCycle
		}
		row := tx.QueryRow(ctx,
			`UPDATE folders SET parent_id=$3, updated_at=now()
			 WHERE tenant=$1 AND `+ambitoSQL+` AND id=$2 AND deleted_at IS NULL AND NOT is_root
			 RETURNING `+folderCols,
			t.Slug, id, newParentID)
		var e error
		f, e = scanFolder(row, t.Slug)
		if uniqueViolation(e) {
			return domain.ErrConflict
		}
		return e
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.Folder{}, domain.ErrNotFound
	}
	return f, err
}

// isDescendant indica si candidate es candidate==ancestor o desciende de ancestor.
func isDescendant(ctx context.Context, tx pgx.Tx, tenant string, candidate, ancestor uuid.UUID) (bool, error) {
	const q = `
		WITH RECURSIVE up AS (
			SELECT id, parent_id FROM folders WHERE tenant=$1 AND ` + ambitoSQL + ` AND id=$2
			UNION ALL
			SELECT f.id, f.parent_id FROM folders f
			JOIN up ON f.id = up.parent_id WHERE f.tenant=$1
		)
		SELECT EXISTS (SELECT 1 FROM up WHERE id=$3)`
	var found bool
	if err := tx.QueryRow(ctx, q, tenant, candidate, ancestor).Scan(&found); err != nil {
		return false, fmt.Errorf("store: comprobar ciclo: %w", err)
	}
	return found, nil
}

// DeletedFile representa un fichero marcado como borrado, con su clave S3 y
// tamaño, para que el servicio libere el blob y la cuota.
type DeletedFile struct {
	ID         uuid.UUID
	StorageKey string
	SizeBytes  int64
	// BlobOrphaned indica que, tras purgar esta fila, NINGÚN otro metadato
	// (fichero vivo/en papelera o versión archivada) referencia ya su storage_key,
	// de modo que el servicio puede borrar el blob de S3 con seguridad. Falso
	// cuando una copia server-side (que comparte el MISMO blob por referencia) aún
	// lo necesita: borrarlo sería pérdida de datos de esa copia. Solo lo calculan
	// las rutas de purga (PurgeFile/PurgeFolder); en soft-delete queda en false y no
	// se usa (esas rutas no tocan blobs).
	BlobOrphaned bool
}

// DeleteFolder envía una carpeta y todo su subárbol a la papelera (soft delete):
// marca deleted_at pero CONSERVA los blobs en S3 y la cuota (siguen ocupando
// almacenamiento hasta que se purguen). Devuelve los ficheros afectados para que
// el servicio pueda contarlos en el evento; la purga real de blobs/cuota ocurre
// en PurgeFolder.
func (s *Store) DeleteFolder(ctx context.Context, id uuid.UUID) ([]DeletedFile, error) {
	t := tenancy.MustFrom(ctx)
	var deleted []DeletedFile
	err := s.inTx(ctx, func(tx pgx.Tx) error {
		var e error
		deleted, e = softDeleteFolderTx(ctx, tx, t.Slug, id)
		return e
	})
	return deleted, err
}

// softDeleteFolderTx envía una carpeta y su subárbol a la papelera (soft delete)
// dentro de la tx dada. Núcleo compartido por DeleteFolder («Mi unidad») y
// TeamDeleteFolder (unidad de equipo): el ámbito lo impone la RLS del llamador. NO
// libera cuota (el blob sigue en S3 hasta la purga). Devuelve los ficheros afectados.
func softDeleteFolderTx(ctx context.Context, tx pgx.Tx, tenant string, id uuid.UUID) ([]DeletedFile, error) {
	var isRoot bool
	if err := tx.QueryRow(ctx,
		`SELECT is_root FROM folders WHERE tenant=$1 AND `+ambitoSQL+` AND id=$2 AND deleted_at IS NULL`,
		tenant, id).Scan(&isRoot); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, domain.ErrNotFound
		}
		return nil, err
	}
	if isRoot {
		return nil, domain.ErrRootImmutable
	}

	// Subárbol completo de carpetas (incluida id).
	rows, err := tx.Query(ctx, `
		WITH RECURSIVE sub AS (
			SELECT id FROM folders WHERE tenant=$1 AND `+ambitoSQL+` AND id=$2
			UNION ALL
			SELECT f.id FROM folders f JOIN sub ON f.parent_id = sub.id WHERE f.tenant=$1
		)
		UPDATE files SET deleted_at=now(), updated_at=now()
		WHERE tenant=$1 AND `+ambitoSQL+` AND deleted_at IS NULL AND folder_id IN (SELECT id FROM sub)
		RETURNING id, storage_key, size_bytes`, tenant, id)
	if err != nil {
		return nil, fmt.Errorf("store: borrar ficheros del subárbol: %w", err)
	}
	var deleted []DeletedFile
	for rows.Next() {
		var df DeletedFile
		if err := rows.Scan(&df.ID, &df.StorageKey, &df.SizeBytes); err != nil {
			rows.Close()
			return nil, err
		}
		deleted = append(deleted, df)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, err
	}

	if _, err := tx.Exec(ctx, `
		WITH RECURSIVE sub AS (
			SELECT id FROM folders WHERE tenant=$1 AND `+ambitoSQL+` AND id=$2
			UNION ALL
			SELECT f.id FROM folders f JOIN sub ON f.parent_id = sub.id WHERE f.tenant=$1
		)
		UPDATE folders SET deleted_at=now(), updated_at=now()
		WHERE tenant=$1 AND `+ambitoSQL+` AND deleted_at IS NULL AND id IN (SELECT id FROM sub)`, tenant, id); err != nil {
		return nil, fmt.Errorf("store: borrar carpetas del subárbol: %w", err)
	}
	// La cuota NO se descuenta al enviar a la papelera: el blob sigue en S3.
	// Se libera en PurgeFolder (borrado definitivo).
	return deleted, nil
}

// ---------------------------------------------------------------------------
// Ficheros
// ---------------------------------------------------------------------------

// labelReadCols son las columnas del snapshot de etiqueta (mig 0013) tal y como se
// LEEN: rank casteado a int (la columna es smallint). Las comparten fileCols y las
// proyecciones planas de «Recientes»/«Favoritos»/«Compartido conmigo», que traen la
// etiqueta en la MISMA consulta (sin N+1) para poder avisar ANTES de intentar
// compartir, igual que el listado principal.
const labelReadCols = `label_id, label_name, label_rank::int, label_block_external_share`

const fileCols = `id, folder_id, name, storage_key, size_bytes, content_type, checksum, owner_sub, modified_by, version, created_at, updated_at,
	` + labelReadCols + `, deleted_at`

// labelSnapshotCols son SOLO las columnas del snapshot de etiqueta (mig 0013), en el
// mismo orden que en fileCols. Las usan las copias server-side para propagar la
// etiqueta al duplicado (copiar un fichero NO debe ser una forma de quitarle la
// etiqueta y, con ella, su restricción de compartición).
const labelSnapshotCols = `label_id, label_name, label_rank, label_block_external_share`

// labelPtr convierte el snapshot recién escaneado en el puntero que expone el
// dominio: nil cuando el fichero NO lleva etiqueta (label_id = ”).
func labelPtr(lb domain.Label) *domain.Label {
	if lb.ID == "" {
		return nil
	}
	return &lb
}

func scanFile(row pgx.Row, t string) (domain.File, error) {
	var f domain.File
	var lb domain.Label
	if err := row.Scan(&f.ID, &f.FolderID, &f.Name, &f.StorageKey, &f.SizeBytes,
		&f.ContentType, &f.Checksum, &f.OwnerSub, &f.ModifiedBy, &f.Version, &f.CreatedAt, &f.UpdatedAt,
		&lb.ID, &lb.Name, &lb.Rank, &lb.BlockExternalShare, &f.DeletedAt); err != nil {
		return domain.File{}, err
	}
	f.Tenant = t
	f.Label = labelPtr(lb)
	return f, nil
}

// SetFileLabel fija (o quita) el snapshot de etiqueta de sensibilidad de un fichero.
// Un label.ID vacío DESETIQUETA (limpia las cuatro columnas). El llamante ya resolvió
// la etiqueta contra la taxonomía de Sytue ID: aquí solo se persiste el snapshot.
// Acotado por tenant vía RLS + predicado explícito: un fichero de otro tenant no
// existe (ErrNotFound). No toca updated_at del contenido —etiquetar no es modificar el
// fichero— para no ensuciar «Recientes» ni el orden por modificación.
func (s *Store) SetFileLabel(ctx context.Context, id uuid.UUID, label domain.Label) (domain.File, error) {
	t := tenancy.MustFrom(ctx)
	if label.ID == "" {
		label = domain.Label{}
	}
	var f domain.File
	err := s.inTx(ctx, func(tx pgx.Tx) error {
		row := tx.QueryRow(ctx,
			`UPDATE files SET label_id=$3, label_name=$4, label_rank=$5, label_block_external_share=$6
			 WHERE tenant=$1 AND `+ambitoSQL+` AND id=$2 AND deleted_at IS NULL RETURNING `+fileCols,
			t.Slug, id, label.ID, label.Name, label.Rank, label.BlockExternalShare)
		var e error
		f, e = scanFile(row, t.Slug)
		return e
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.File{}, domain.ErrNotFound
	}
	return f, err
}

// GetFile devuelve un fichero vivo por ID DEL ÁMBITO PERSONAL («Mi unidad»).
//
// El predicado `channel_id IS NULL` es explícito a propósito, además de la RLS de
// ámbito (mig 0014): un ADJUNTO de un canal de Rooms no se sirve NUNCA por la ruta
// personal —que en Drive es un espacio compartido de toda la organización—, aunque
// se conozca su uuid. Es el mismo cinturón-y-tirantes que el aislamiento por tenant
// (RLS + predicado), y hace la verja verificable en el harness de integración, cuyo
// rol tiene BYPASSRLS. Los adjuntos se leen por store.ChannelFile (channelfiles.go).
func (s *Store) GetFile(ctx context.Context, id uuid.UUID) (domain.File, error) {
	t := tenancy.MustFrom(ctx)
	var f domain.File
	err := s.inTx(ctx, func(tx pgx.Tx) error {
		row := tx.QueryRow(ctx,
			`SELECT `+fileCols+` FROM files
			 WHERE tenant=$1 AND `+ambitoSQL+` AND id=$2 AND deleted_at IS NULL AND channel_id IS NULL`,
			t.Slug, id)
		var e error
		f, e = scanFile(row, t.Slug)
		return e
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.File{}, domain.ErrNotFound
	}
	return f, err
}

// GetFileByStorageKey devuelve el fichero VIVO que referencia una storage_key, o
// domain.ErrNotFound si ninguno la referencia. Sostiene la idempotencia de la
// finalización de subidas directas (un reintento de finalize del MISMO blob no
// debe recrear el metadato). Sirve por el índice (tenant, storage_key).
func (s *Store) GetFileByStorageKey(ctx context.Context, key string) (domain.File, error) {
	t := tenancy.MustFrom(ctx)
	var f domain.File
	err := s.inTx(ctx, func(tx pgx.Tx) error {
		row := tx.QueryRow(ctx,
			`SELECT `+fileCols+` FROM files WHERE tenant=$1 AND `+ambitoSQL+` AND storage_key=$2 AND deleted_at IS NULL`,
			t.Slug, key)
		var e error
		f, e = scanFile(row, t.Slug)
		return e
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.File{}, domain.ErrNotFound
	}
	return f, err
}

// NewFileRecord son los datos de creación/sustitución de un fichero.
type NewFileRecord struct {
	FolderID    uuid.UUID
	Name        string
	StorageKey  string
	SizeBytes   int64
	ContentType string
	Checksum    string
	OwnerSub    string
}

// CreateFile inserta el metadato de un fichero recién subido a S3, validando y
// reservando cuota atómicamente. Si ya existe un fichero con ese nombre en la
// carpeta, lo sustituye (nueva versión) ajustando la cuota por la diferencia.
func (s *Store) CreateFile(ctx context.Context, rec NewFileRecord) (domain.File, error) {
	t := tenancy.MustFrom(ctx)
	if !domain.ValidName(rec.Name) {
		return domain.File{}, domain.ErrInvalidName
	}
	if rec.ContentType == "" {
		rec.ContentType = "application/octet-stream"
	}
	var f domain.File
	err := s.inTx(ctx, func(tx pgx.Tx) error {
		var e error
		f, e = createFileTx(ctx, tx, t.Slug, nil, rec)
		return e
	})
	return f, err
}

// createFileTx es el núcleo de la creación/sustitución de un fichero dentro de la
// tx dada (RLS ya fijada por el llamador: `app.tenant`, y opcionalmente
// `app.team_drive` para una unidad de equipo). Lo comparten CreateFile («Mi
// unidad», teamDriveID nil) y la subida a una unidad de equipo (teamdrive.go,
// teamDriveID = la unidad): la MISMA lógica de cuota, sustitución-por-nombre (nueva
// versión) y conflicto, sin duplicar. teamDriveID marca la fila NUEVA con su ámbito
// para satisfacer el WITH CHECK de la política (en una sustitución la fila existente
// ya lleva el ámbito correcto). La cuota se reserva contra el tenant en ambos casos.
func createFileTx(ctx context.Context, tx pgx.Tx, tenant string, teamDriveID *uuid.UUID, rec NewFileRecord) (domain.File, error) {
	var f domain.File
	// Carpeta destino debe existir y estar viva (RLS acota el ámbito visible).
	var ok bool
	if err := tx.QueryRow(ctx,
		`SELECT true FROM folders WHERE tenant=$1 AND `+ambitoSQL+` AND id=$2 AND deleted_at IS NULL`,
		tenant, rec.FolderID).Scan(&ok); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return domain.File{}, domain.ErrInvalidParent
		}
		return domain.File{}, err
	}

	// Fichero existente con el mismo nombre (sustitución).
	var prevSize int64
	var prevKey string
	var prevVersion int
	var existingID uuid.UUID
	exists := false
	err := tx.QueryRow(ctx,
		`SELECT id, size_bytes, version, storage_key FROM files
		 WHERE tenant=$1 AND `+ambitoSQL+` AND folder_id=$2 AND lower(name)=lower($3) AND deleted_at IS NULL`,
		tenant, rec.FolderID, rec.Name).Scan(&existingID, &prevSize, &prevVersion, &prevKey)
	switch {
	case err == nil:
		exists = true
	case errors.Is(err, pgx.ErrNoRows):
	default:
		return domain.File{}, err
	}

	// Cuota por bytes lógicos, sin contar lo que vive en el bucket de terceros del
	// tenant (mig 0022): ni lo nuevo si va fuera, ni lo viejo si ya estaba fuera.
	delta := bytesEnCuota(rec.StorageKey, rec.SizeBytes)
	if exists {
		delta -= bytesEnCuota(prevKey, prevSize)
	}
	if err := reserveQuota(ctx, tx, tenant, delta); err != nil {
		return domain.File{}, err
	}

	fileOwner := rec.OwnerSub
	modifiedBy := rec.OwnerSub
	if teamDriveID == nil {
		fileOwner = OwnerFrom(ctx)
		if modifiedBy == "" {
			modifiedBy = fileOwner
		}
	}
	if exists {
		// Conserva la versión ANTERIOR (referencia su blob previo, que si no
		// quedaría huérfano) antes de sobrescribir el contenido.
		if err := snapshotVersionTx(ctx, tx, tenant, existingID); err != nil {
			return domain.File{}, err
		}
		row := tx.QueryRow(ctx,
			`UPDATE files SET storage_key=$3, size_bytes=$4, content_type=$5, checksum=$6, modified_by=$7,
			 version=version+1, updated_at=now()
			 WHERE tenant=$1 AND `+ambitoSQL+` AND id=$2 RETURNING `+fileCols,
			tenant, existingID, rec.StorageKey, rec.SizeBytes, rec.ContentType, rec.Checksum, modifiedBy)
		return scanFile(row, tenant)
	}
	row := tx.QueryRow(ctx,
		`INSERT INTO files (tenant, folder_id, name, storage_key, size_bytes, content_type, checksum, owner_sub, modified_by, team_drive_id)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10) RETURNING `+fileCols,
		tenant, rec.FolderID, strings.TrimSpace(rec.Name), rec.StorageKey,
		rec.SizeBytes, rec.ContentType, rec.Checksum, fileOwner, modifiedBy, teamDriveID)
	f, e := scanFile(row, tenant)
	if uniqueViolation(e) {
		return domain.File{}, domain.ErrConflict
	}
	return f, e
}

// RenameFile cambia el nombre de un fichero.
func (s *Store) RenameFile(ctx context.Context, id uuid.UUID, name string) (domain.File, error) {
	t := tenancy.MustFrom(ctx)
	if !domain.ValidName(name) {
		return domain.File{}, domain.ErrInvalidName
	}
	var f domain.File
	err := s.inTx(ctx, func(tx pgx.Tx) error {
		row := tx.QueryRow(ctx,
			`UPDATE files SET name=$3, updated_at=now()
			 WHERE tenant=$1 AND `+ambitoSQL+` AND id=$2 AND deleted_at IS NULL RETURNING `+fileCols,
			t.Slug, id, strings.TrimSpace(name))
		var e error
		f, e = scanFile(row, t.Slug)
		if uniqueViolation(e) {
			return domain.ErrConflict
		}
		return e
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.File{}, domain.ErrNotFound
	}
	return f, err
}

// MoveFile reubica un fichero en otra carpeta.
func (s *Store) MoveFile(ctx context.Context, id, newFolderID uuid.UUID) (domain.File, error) {
	t := tenancy.MustFrom(ctx)
	var f domain.File
	err := s.inTx(ctx, func(tx pgx.Tx) error {
		var ok bool
		if err := tx.QueryRow(ctx,
			`SELECT true FROM folders WHERE tenant=$1 AND `+ambitoSQL+` AND id=$2 AND deleted_at IS NULL`,
			t.Slug, newFolderID).Scan(&ok); err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return domain.ErrInvalidParent
			}
			return err
		}
		row := tx.QueryRow(ctx,
			`UPDATE files SET folder_id=$3, updated_at=now()
			 WHERE tenant=$1 AND `+ambitoSQL+` AND id=$2 AND deleted_at IS NULL RETURNING `+fileCols,
			t.Slug, id, newFolderID)
		var e error
		f, e = scanFile(row, t.Slug)
		if uniqueViolation(e) {
			return domain.ErrConflict
		}
		return e
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.File{}, domain.ErrNotFound
	}
	return f, err
}

// DeleteFile envía un fichero a la papelera (soft delete): marca deleted_at pero
// CONSERVA el blob en S3 y la cuota. Devuelve el metadato del fichero borrado; la
// liberación de blob/cuota ocurre en PurgeFile (borrado definitivo).
func (s *Store) DeleteFile(ctx context.Context, id uuid.UUID) (DeletedFile, error) {
	t := tenancy.MustFrom(ctx)
	var df DeletedFile
	err := s.inTx(ctx, func(tx pgx.Tx) error {
		row := tx.QueryRow(ctx,
			`UPDATE files SET deleted_at=now(), updated_at=now()
			 WHERE tenant=$1 AND `+ambitoSQL+` AND id=$2 AND deleted_at IS NULL
			 RETURNING id, storage_key, size_bytes`, t.Slug, id)
		return row.Scan(&df.ID, &df.StorageKey, &df.SizeBytes)
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return DeletedFile{}, domain.ErrNotFound
	}
	return df, err
}

// ---------------------------------------------------------------------------
// Copia server-side (por referencia al blob, sin duplicar bytes)
// ---------------------------------------------------------------------------

// maxCopyNodes acota el nº de ficheros que puede arrastrar la copia recursiva de
// una carpeta (defensa contra subárboles gigantes; paridad con maxZipFiles del
// servicio). ponytail: cota por NÚMERO de ficheros; añadir una por bytes totales si
// algún tenant la necesita.
const maxCopyNodes = 5000

// uniqueCopyName construye un nombre de copia libre en el destino: base («Copia de
// X»), y si choca, «Copia de X (2)», «(3)»… sondeando con `exists` hasta encontrar
// hueco (cota defensiva; la unicidad REAL la garantiza el índice único al insertar,
// que devuelve ErrConflict si una copia concurrente ganó la carrera). Sanea cada
// candidato con domain.ValidName (longitud/‘/’/control), así que el nombre resultante
// es seguro. Devuelve ErrInvalidName si «Copia de X» ya no cabe en 255 caracteres.
func uniqueCopyName(base string, exists func(name string) (bool, error)) (string, error) {
	for i := 0; i < 1000; i++ {
		cand := base
		if i > 0 {
			cand = fmt.Sprintf("%s (%d)", base, i+1)
		}
		if !domain.ValidName(cand) {
			return "", domain.ErrInvalidName
		}
		ex, err := exists(cand)
		if err != nil {
			return "", err
		}
		if !ex {
			return cand, nil
		}
	}
	return "", domain.ErrConflict
}

// copyBase antepone el prefijo de copia («Copia de …», paridad Google Drive) al
// nombre original.
func copyBase(name string) string { return "Copia de " + name }

// CopyFile crea una COPIA server-side de un fichero: una fila de metadatos NUEVA en
// destFolderID que REFERENCIA EL MISMO blob que el original (misma storage_key,
// size, content_type, checksum), sin descargar ni resubir ni duplicar bytes en S3
// —O(1) en bytes, como snapshotVersionTx/RestoreVersion—. El nombre es «Copia de X»
// único en el destino. La cuota se reserva por BYTES LÓGICOS (misma política que
// CreateFile: cada fichero VIVO cuenta su tamaño), aunque el blob no se duplique.
// RLS gatea origen y destino por tenant: un fichero de otro tenant no existe
// (ErrNotFound) y no se puede escribir en la carpeta de otro tenant (ErrInvalidParent).
func (s *Store) CopyFile(ctx context.Context, srcID, destFolderID uuid.UUID, ownerSub string) (domain.File, error) {
	t := tenancy.MustFrom(ctx)
	var out domain.File
	err := s.inTx(ctx, func(tx pgx.Tx) error {
		var e error
		out, e = copiarFichero(ctx, tx, t.Slug, srcID, destFolderID, ownerSub)
		return e
	})
	return out, err
}

// CopiaParaDuenya es CopyFile a nombre de OTRA persona —la dueña de un álbum de Photos
// al que alguien añade una foto suya (ver service.CopiaParaFotos)— sin duplicar lo que
// ya tiene: si la dueña tiene viva una copia con la misma huella (`checksum`), se
// devuelve esa y no se cobra cuota otra vez. Un cerrojo por (organización, dueña,
// huella) pone en fila dos añadidos a la vez de la misma foto. Solo se copia un fichero
// de `editora` —quien lo añade—: uno de otra persona es ErrNotFound.
func (s *Store) CopiaParaDuenya(ctx context.Context, srcID, destFolderID uuid.UUID, duenya, editora string) (out domain.File, reusada bool, err error) {
	t := tenancy.MustFrom(ctx)
	if editora == "" {
		return domain.File{}, false, domain.ErrNotFound
	}
	err = s.inTx(WithOwner(ctx, editora), func(tx pgx.Tx) error {
		src, e := scanFile(tx.QueryRow(ctx,
			`SELECT `+fileCols+` FROM files WHERE tenant=$1 AND `+ambitoSQL+` AND id=$2 AND owner_sub=$3 AND deleted_at IS NULL`,
			t.Slug, srcID, editora), t.Slug)
		if errors.Is(e, pgx.ErrNoRows) {
			return domain.ErrNotFound
		}
		if e != nil {
			return e
		}
		if _, e := tx.Exec(ctx, "SELECT set_config('app.owner_sub', $1, true)", duenya); e != nil {
			return fmt.Errorf("store: fijar app.owner_sub de la dueña: %w", e)
		}
		ctxDuenya := WithOwner(ctx, duenya)
		huella := src.Checksum
		if huella != "" {
			if _, e := tx.Exec(ctxDuenya, `SELECT pg_advisory_xact_lock(hashtextextended($1 || '|' || $2 || '|' || $3, 0))`,
				t.Slug, duenya, huella); e != nil {
				return e
			}
			ya, e := scanFile(tx.QueryRow(ctxDuenya,
				`SELECT `+fileCols+` FROM files
				  WHERE tenant=$1 AND `+ambitoSQL+` AND owner_sub=$2 AND checksum=$3 AND deleted_at IS NULL
				  ORDER BY created_at LIMIT 1`, t.Slug, duenya, huella), t.Slug)
			if e == nil {
				out, reusada = ya, true
				return nil
			}
			if !errors.Is(e, pgx.ErrNoRows) {
				return e
			}
		}
		out, e = insertarCopia(ctxDuenya, tx, t.Slug, src, destFolderID, duenya)
		return e
	})
	return out, reusada, err
}

// copiarFichero es el cuerpo de CopyFile dentro de una transacción ya abierta.
func copiarFichero(ctx context.Context, tx pgx.Tx, slug string, srcID, destFolderID uuid.UUID, ownerSub string) (domain.File, error) {
	// Origen VIVO (RLS): no se copia lo que no se puede ver.
	src, err := scanFile(tx.QueryRow(ctx,
		`SELECT `+fileCols+` FROM files WHERE tenant=$1 AND `+ambitoSQL+` AND id=$2 AND deleted_at IS NULL`,
		slug, srcID), slug)
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.File{}, domain.ErrNotFound
	}
	if err != nil {
		return domain.File{}, err
	}
	return insertarCopia(ctx, tx, slug, src, destFolderID, ownerSub)
}

// insertarCopia crea en destFolderID —en el ámbito de la transacción— una fila que
// referencia el mismo blob que `src`, ya leído por quien llama en SU ámbito.
func insertarCopia(ctx context.Context, tx pgx.Tx, slug string, src domain.File, destFolderID uuid.UUID, ownerSub string) (domain.File, error) {
	// Destino VIVO (RLS).
	var ok bool
	if err := tx.QueryRow(ctx,
		`SELECT true FROM folders WHERE tenant=$1 AND `+ambitoSQL+` AND id=$2 AND deleted_at IS NULL`,
		slug, destFolderID).Scan(&ok); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return domain.File{}, domain.ErrInvalidParent
		}
		return domain.File{}, err
	}
	// Cuota por bytes lógicos (aunque el blob se comparta por referencia); lo
	// que vive en el bucket de terceros no cuenta.
	if err := reserveQuota(ctx, tx, slug, bytesEnCuota(src.StorageKey, src.SizeBytes)); err != nil {
		return domain.File{}, err
	}
	name, err := uniqueCopyName(copyBase(src.Name), func(cand string) (bool, error) {
		var ex bool
		e := tx.QueryRow(ctx,
			`SELECT EXISTS (SELECT 1 FROM files
			 WHERE tenant=$1 AND `+ambitoSQL+` AND folder_id=$2 AND lower(name)=lower($3) AND deleted_at IS NULL)`,
			slug, destFolderID, cand).Scan(&ex)
		return ex, e
	})
	if err != nil {
		return domain.File{}, err
	}
	// La etiqueta de sensibilidad viaja con la copia: duplicar un fichero no puede
	// ser la forma fácil de quitarle la etiqueta (y su bloqueo de compartición).
	lb := domain.Label{}
	if src.Label != nil {
		lb = *src.Label
	}
	fileOwner := OwnerFrom(ctx)
	modifiedBy := ownerSub
	if modifiedBy == "" {
		modifiedBy = fileOwner
	}
	out, err := scanFile(tx.QueryRow(ctx,
		`INSERT INTO files (tenant, folder_id, name, storage_key, size_bytes, content_type, checksum, owner_sub, modified_by, `+labelSnapshotCols+`)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13) RETURNING `+fileCols,
		slug, destFolderID, name, src.StorageKey, src.SizeBytes, src.ContentType, src.Checksum, fileOwner, modifiedBy,
		lb.ID, lb.Name, lb.Rank, lb.BlockExternalShare), slug)
	if uniqueViolation(err) {
		return domain.File{}, domain.ErrConflict
	}
	return out, err
}

// CopyFolder copia RECURSIVAMENTE una carpeta y su subárbol. Crea «Copia de X» bajo
// destParentID (o, si es nil, el mismo padre del origen: copia hermana), replica la
// estructura de subcarpetas y, por cada fichero, inserta una fila NUEVA que
// REFERENCIA EL MISMO blob (no duplica bytes). Reserva cuota por los bytes lógicos
// TOTALES del subárbol. RLS gatea por tenant. Escala O(nodos) sin N+1 desmedido: el
// subárbol se lee en dos consultas (carpetas + ficheros, recursive CTE) y se inserta
// en dos más. Falla con ErrBatchTooLarge si el subárbol supera maxCopyNodes ficheros.
//
// La inserción en bloque va con `INSERT ... SELECT FROM unnest(...)`, NO con CopyFrom:
// PostgreSQL rechaza el protocolo COPY sobre una tabla con RLS activa para un rol sin
// BYPASSRLS —«COPY FROM not supported with row-level security», SQLSTATE 0A000—, así
// que la versión con CopyFrom fallaba SIEMPRE en producción. No se veía porque el
// harness de tests conectaba como superusuario, que sí puede. unnest se prefiere a un
// INSERT ... VALUES multi-fila porque pasa 10 arrays en vez de 13 parámetros por
// fichero: con maxCopyNodes=5000 el multi-fila rozaría el tope de 65535 de PostgreSQL.
//
// El orden de las subcarpetas ya no importa para la FK parent_id: dentro de UNA
// sentencia las comprobaciones de clave ajena se ejecutan al final, así que un hijo
// puede insertarse antes que su padre. (Con CopyFrom había que respetar el orden.)
//
//nolint:gocognit,gocyclo // copia recursiva en UNA tx: recorrido, cuota, colisiones de nombre y permisos comparten estado.
func (s *Store) CopyFolder(ctx context.Context, srcID uuid.UUID, destParentID *uuid.UUID, ownerSub string) (domain.Folder, error) {
	t := tenancy.MustFrom(ctx)
	var out domain.Folder
	err := s.inTx(ctx, func(tx pgx.Tx) error {
		// Origen VIVO y no raíz (RLS).
		src, err := scanFolder(tx.QueryRow(ctx,
			`SELECT `+folderCols+` FROM folders WHERE tenant=$1 AND `+ambitoSQL+` AND id=$2 AND deleted_at IS NULL AND NOT is_root`,
			t.Slug, srcID), t.Slug)
		if errors.Is(err, pgx.ErrNoRows) {
			return domain.ErrNotFound
		}
		if err != nil {
			return err
		}
		// Destino: el indicado o el padre del origen (copia hermana). Debe estar vivo.
		destParent := src.ParentID
		if destParentID != nil {
			destParent = destParentID
		}
		if destParent == nil { // defensivo: el origen no es raíz, siempre tiene padre
			return domain.ErrInvalidParent
		}
		// El destino no puede ser el propio subárbol que se copia (ciclo).
		if *destParent == srcID {
			return domain.ErrCycle
		}
		descendant, err := isDescendant(ctx, tx, t.Slug, *destParent, srcID)
		if err != nil {
			return err
		}
		if descendant {
			return domain.ErrCycle
		}
		var ok bool
		if err := tx.QueryRow(ctx,
			`SELECT true FROM folders WHERE tenant=$1 AND `+ambitoSQL+` AND id=$2 AND deleted_at IS NULL`,
			t.Slug, *destParent).Scan(&ok); err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return domain.ErrInvalidParent
			}
			return err
		}

		// Subcarpetas DESCENDIENTES (sin la raíz del subárbol), padres antes que hijos.
		subRows, err := tx.Query(ctx, `
			WITH RECURSIVE sub AS (
				SELECT id, parent_id, name, 0 AS depth
				FROM folders WHERE tenant=$1 AND `+ambitoSQL+` AND id=$2
				UNION ALL
				SELECT c.id, c.parent_id, c.name, sub.depth+1
				FROM folders c JOIN sub ON c.parent_id = sub.id
				WHERE c.tenant=$1 AND c.deleted_at IS NULL
			)
			SELECT id, parent_id, name FROM sub WHERE depth > 0 ORDER BY depth`, t.Slug, srcID)
		if err != nil {
			return fmt.Errorf("store: copiar carpeta (subárbol): %w", err)
		}
		type folderRow struct {
			id, parent uuid.UUID
			name       string
		}
		var subs []folderRow
		for subRows.Next() {
			var fr folderRow
			if err := subRows.Scan(&fr.id, &fr.parent, &fr.name); err != nil {
				subRows.Close()
				return err
			}
			subs = append(subs, fr)
		}
		subRows.Close()
		if err := subRows.Err(); err != nil {
			return err
		}

		// Ficheros del subárbol (incluida la raíz), con su blob a referenciar.
		fileRows, err := tx.Query(ctx, `
			WITH RECURSIVE sub AS (
				SELECT id FROM folders WHERE tenant=$1 AND `+ambitoSQL+` AND id=$2
				UNION ALL
				SELECT c.id FROM folders c JOIN sub ON c.parent_id = sub.id
				WHERE c.tenant=$1 AND c.deleted_at IS NULL
			)
			SELECT folder_id, name, storage_key, size_bytes, content_type, checksum,
			       label_id, label_name, label_rank::int, label_block_external_share
			FROM files WHERE tenant=$1 AND `+ambitoSQL+` AND deleted_at IS NULL AND folder_id IN (SELECT id FROM sub)`, t.Slug, srcID)
		if err != nil {
			return fmt.Errorf("store: copiar carpeta (ficheros): %w", err)
		}
		type fileRow struct {
			folderID           uuid.UUID
			name, key, ct, chk string
			size               int64
			// Snapshot de etiqueta a propagar a la copia (igual que en CopyFile).
			label domain.Label
		}
		var files []fileRow
		var totalBytes int64
		for fileRows.Next() {
			var fr fileRow
			if err := fileRows.Scan(&fr.folderID, &fr.name, &fr.key, &fr.size, &fr.ct, &fr.chk,
				&fr.label.ID, &fr.label.Name, &fr.label.Rank, &fr.label.BlockExternalShare); err != nil {
				fileRows.Close()
				return err
			}
			totalBytes += bytesEnCuota(fr.key, fr.size)
			files = append(files, fr)
		}
		fileRows.Close()
		if err := fileRows.Err(); err != nil {
			return err
		}
		if len(files) > maxCopyNodes {
			return domain.ErrBatchTooLarge
		}

		// Cuota por bytes lógicos totales (los blobs se comparten por referencia).
		if err := reserveQuota(ctx, tx, t.Slug, totalBytes); err != nil {
			return err
		}

		// Raíz de la copia: «Copia de X» única bajo el destino.
		rootName, err := uniqueCopyName(copyBase(src.Name), func(cand string) (bool, error) {
			var ex bool
			e := tx.QueryRow(ctx,
				`SELECT EXISTS (SELECT 1 FROM folders
				 WHERE tenant=$1 AND `+ambitoSQL+` AND parent_id=$2 AND lower(name)=lower($3) AND deleted_at IS NULL AND NOT is_root)`,
				t.Slug, *destParent, cand).Scan(&ex)
			return ex, e
		})
		if err != nil {
			return err
		}
		folderOwner := OwnerFrom(ctx)
		modifiedBy := ownerSub
		if modifiedBy == "" {
			modifiedBy = folderOwner
		}
		out, err = scanFolder(tx.QueryRow(ctx,
			`INSERT INTO folders (tenant, parent_id, name, owner_sub) VALUES ($1, $2, $3, $4)
			 RETURNING `+folderCols, t.Slug, *destParent, rootName, folderOwner), t.Slug)
		if uniqueViolation(err) {
			return domain.ErrConflict
		}
		if err != nil {
			return err
		}

		// Mapa origen→copia de IDs de carpeta. La raíz del subárbol mapea a la copia.
		idMap := map[uuid.UUID]uuid.UUID{srcID: out.ID}
		// Subcarpetas: UUID nuevo por nodo (padres ya mapeados por el orden de depth).
		subIDs := make([]uuid.UUID, 0, len(subs))
		subParents := make([]uuid.UUID, 0, len(subs))
		subNames := make([]string, 0, len(subs))
		for _, fr := range subs {
			newID := uuid.New()
			idMap[fr.id] = newID
			subIDs = append(subIDs, newID)
			subParents = append(subParents, idMap[fr.parent])
			subNames = append(subNames, fr.name)
		}
		if len(subIDs) > 0 {
			if _, err := tx.Exec(ctx, `
				INSERT INTO folders (tenant, id, parent_id, name, owner_sub)
				SELECT $1, u.id, u.parent_id, u.name, $2
				FROM unnest($3::uuid[], $4::uuid[], $5::text[]) AS u(id, parent_id, name)`,
				t.Slug, folderOwner, subIDs, subParents, subNames); err != nil {
				return fmt.Errorf("store: copiar carpeta (insertar subcarpetas): %w", err)
			}
		}
		// Ficheros: fila nueva por fichero, MISMO blob (storage_key), carpeta remapeada.
		fFolders := make([]uuid.UUID, 0, len(files))
		fNames := make([]string, 0, len(files))
		fKeys := make([]string, 0, len(files))
		fSizes := make([]int64, 0, len(files))
		fTypes := make([]string, 0, len(files))
		fSums := make([]string, 0, len(files))
		fLabelIDs := make([]string, 0, len(files))
		fLabelNames := make([]string, 0, len(files))
		fLabelRanks := make([]int, 0, len(files))
		fLabelBlocks := make([]bool, 0, len(files))
		for _, fr := range files {
			fFolders = append(fFolders, idMap[fr.folderID])
			fNames = append(fNames, fr.name)
			fKeys = append(fKeys, fr.key)
			fSizes = append(fSizes, fr.size)
			fTypes = append(fTypes, fr.ct)
			fSums = append(fSums, fr.chk)
			fLabelIDs = append(fLabelIDs, fr.label.ID)
			fLabelNames = append(fLabelNames, fr.label.Name)
			fLabelRanks = append(fLabelRanks, fr.label.Rank)
			fLabelBlocks = append(fLabelBlocks, fr.label.BlockExternalShare)
		}
		if len(fFolders) > 0 {
			if _, err := tx.Exec(ctx, `
				INSERT INTO files (tenant, folder_id, name, storage_key, size_bytes, content_type,
				                   checksum, owner_sub, modified_by,
				                   label_id, label_name, label_rank, label_block_external_share)
				SELECT $1, u.folder_id, u.name, u.storage_key, u.size_bytes, u.content_type,
				       u.checksum, $2, $13,
				       u.label_id, u.label_name, u.label_rank, u.label_block
				FROM unnest($3::uuid[], $4::text[], $5::text[], $6::bigint[], $7::text[], $8::text[],
				            $9::text[], $10::text[], $11::smallint[], $12::boolean[])
				     AS u(folder_id, name, storage_key, size_bytes, content_type, checksum,
				          label_id, label_name, label_rank, label_block)`,
				t.Slug, folderOwner, fFolders, fNames, fKeys, fSizes, fTypes, fSums,
				fLabelIDs, fLabelNames, fLabelRanks, fLabelBlocks, modifiedBy); err != nil {
				return fmt.Errorf("store: copiar carpeta (insertar ficheros): %w", err)
			}
		}
		return nil
	})
	return out, err
}

// ---------------------------------------------------------------------------
// Historial de versiones
// ---------------------------------------------------------------------------

const fileVersionCols = `file_id, version, storage_key, size_bytes, content_type, checksum, author_sub, created_at`

func scanFileVersion(row pgx.Row, t string) (domain.FileVersion, error) {
	var v domain.FileVersion
	if err := row.Scan(&v.FileID, &v.Version, &v.StorageKey, &v.SizeBytes,
		&v.ContentType, &v.Checksum, &v.AuthorSub, &v.CreatedAt); err != nil {
		return domain.FileVersion{}, err
	}
	v.Tenant = t
	return v, nil
}

// snapshotVersionTx archiva la versión ACTUAL del fichero como una fila de
// file_versions REFERENCIANDO su blob (no lo copia: INSERT ... SELECT sin tocar
// el objeto en S3). Se llama ANTES de sobrescribir el contenido (subida de versión
// nueva o restauración), dentro de la misma tx para que el archivado y el
// reemplazo sean atómicos.
func snapshotVersionTx(ctx context.Context, tx pgx.Tx, tenant string, fileID uuid.UUID) error {
	if _, err := tx.Exec(ctx,
		`INSERT INTO file_versions (tenant, file_id, version, storage_key, size_bytes, content_type, checksum, author_sub, created_at)
		 SELECT tenant, id, version, storage_key, size_bytes, content_type, checksum, modified_by, updated_at
		 FROM files WHERE tenant=$1 AND `+ambitoSQL+` AND id=$2`, tenant, fileID); err != nil {
		return fmt.Errorf("store: archivar versión: %w", err)
	}
	return nil
}

// versionCeil es una cota superior a cualquier número de versión real (version es
// int4, máx 2147483647): para la primera página del keyset DESCENDENTE, version <
// versionCeil se cumple para todas las filas sin un segundo SQL (evita SQL
// dinámico, CONVENTIONS §10).
const versionCeil int64 = math.MaxInt32 + 1

// ListVersions lista las versiones ARCHIVADAS de un fichero (no la actual, que
// vive en files) más nueva primero (version DESC) con paginación keyset por
// version. before<=0 pide la primera página. Devuelve hasta `limit` versiones y,
// si hay más, el número de versión de continuación. Lo sirve el índice
// file_versions_file_version_idx por backward scan (sin OFFSET, sin sort).
func (s *Store) ListVersions(ctx context.Context, fileID uuid.UUID, before, limit int) ([]domain.FileVersion, *int, error) {
	t := tenancy.MustFrom(ctx)
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	bound := int64(before)
	if before <= 0 {
		bound = versionCeil
	}
	var out []domain.FileVersion
	var next *int
	err := s.inTx(ctx, func(tx pgx.Tx) error {
		// $3::bigint: la cota (versionCeil) desborda int4; el cast la pasa como int8
		// y el operador cruzado int4<int8 sigue usando el índice (sin castear la
		// columna, que sí lo impediría).
		rows, err := tx.Query(ctx,
			`SELECT `+fileVersionCols+` FROM file_versions
			 WHERE tenant=$1 AND file_id=$2 AND version < $3::bigint
			   AND EXISTS (SELECT 1 FROM files f WHERE f.tenant=$1 AND `+ambitoF+` AND f.id=file_versions.file_id)
			 ORDER BY version DESC LIMIT $4`,
			t.Slug, fileID, bound, limit+1)
		if err != nil {
			return fmt.Errorf("store: listar versiones: %w", err)
		}
		defer rows.Close()
		for rows.Next() {
			v, err := scanFileVersion(rows, t.Slug)
			if err != nil {
				return err
			}
			out = append(out, v)
		}
		if err := rows.Err(); err != nil {
			return err
		}
		if len(out) > limit {
			last := out[limit-1]
			out = out[:limit]
			next = &last.Version
		}
		return nil
	})
	if err != nil {
		return nil, nil, err
	}
	return out, next, nil
}

// GetVersion devuelve una versión archivada concreta (para descargar su blob).
// RLS aísla por tenant: una versión de un fichero ajeno no existe (ErrNotFound).
func (s *Store) GetVersion(ctx context.Context, fileID uuid.UUID, version int) (domain.FileVersion, error) {
	t := tenancy.MustFrom(ctx)
	var v domain.FileVersion
	err := s.inTx(ctx, func(tx pgx.Tx) error {
		row := tx.QueryRow(ctx,
			`SELECT `+fileVersionCols+` FROM file_versions
			 WHERE tenant=$1 AND file_id=$2 AND version=$3
			   AND EXISTS (SELECT 1 FROM files f WHERE f.tenant=$1 AND `+ambitoF+` AND f.id=file_versions.file_id)`,
			t.Slug, fileID, version)
		var e error
		v, e = scanFileVersion(row, t.Slug)
		return e
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.FileVersion{}, domain.ErrNotFound
	}
	return v, err
}

// RestoreVersion hace ACTUAL una versión archivada de forma NO DESTRUCTIVA y
// atómica: la versión actual se archiva como una entrada más del historial y el
// fichero pasa a apuntar (por REFERENCIA, sin copiar ni traer el blob a memoria)
// al blob de la versión objetivo, con un número de versión NUEVO (version+1). La
// cuota se ajusta por la diferencia de tamaño. Devuelve el fichero ya restaurado.
func (s *Store) RestoreVersion(ctx context.Context, fileID uuid.UUID, version int, actorSub string) (domain.File, error) {
	t := tenancy.MustFrom(ctx)
	var f domain.File
	err := s.inTx(ctx, func(tx pgx.Tx) error {
		// Contenido de la versión objetivo (referencia a su blob existente).
		var vKey, vCT, vChk string
		var vSize int64
		err := tx.QueryRow(ctx,
			`SELECT storage_key, size_bytes, content_type, checksum FROM file_versions
			 WHERE tenant=$1 AND file_id=$2 AND version=$3`, t.Slug, fileID, version).
			Scan(&vKey, &vSize, &vCT, &vChk)
		if errors.Is(err, pgx.ErrNoRows) {
			return domain.ErrNotFound
		}
		if err != nil {
			return err
		}
		// Tamaño actual para el delta de cuota (y que el fichero siga vivo).
		var curSize int64
		var curKey string
		err = tx.QueryRow(ctx,
			`SELECT size_bytes, storage_key FROM files WHERE tenant=$1 AND `+ambitoSQL+` AND id=$2 AND deleted_at IS NULL`,
			t.Slug, fileID).Scan(&curSize, &curKey)
		if errors.Is(err, pgx.ErrNoRows) {
			return domain.ErrNotFound
		}
		if err != nil {
			return err
		}
		if err := reserveQuota(ctx, tx, t.Slug, bytesEnCuota(vKey, vSize)-bytesEnCuota(curKey, curSize)); err != nil {
			return err
		}
		// La versión ACTUAL pasa a archivarse (no se pierde).
		if err := snapshotVersionTx(ctx, tx, t.Slug, fileID); err != nil {
			return err
		}
		// El fichero apunta ahora al blob de la versión objetivo, como versión NUEVA.
		row := tx.QueryRow(ctx,
			`UPDATE files SET storage_key=$3, size_bytes=$4, content_type=$5, checksum=$6, modified_by=$7,
			 version=version+1, updated_at=now()
			 WHERE tenant=$1 AND `+ambitoSQL+` AND id=$2 AND deleted_at IS NULL RETURNING `+fileCols,
			t.Slug, fileID, vKey, vSize, vCT, vChk, actorSub)
		var e error
		f, e = scanFile(row, t.Slug)
		return e
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.File{}, domain.ErrNotFound
	}
	return f, err
}

// ---------------------------------------------------------------------------
// Papelera: listar, restaurar y purgar (borrado definitivo)
// ---------------------------------------------------------------------------

// ListTrash lista los elementos en la papelera del tenant de NIVEL SUPERIOR: las
// carpetas borradas cuyo padre NO está borrado y los ficheros borrados cuya
// carpeta NO está borrada. Así una carpeta borrada con su contenido aparece como
// una sola entrada (restaurarla recupera todo el subárbol), igual que en Drive.
// Los índices parciales (tenant, deleted_at) WHERE deleted_at IS NOT NULL de la
// migración 0004 cubren estos barridos (orden deleted_at DESC incluido).
// ponytail: sin paginar — la papelera de un tenant es pequeña frente a sus
// ficheros vivos; keyset por (deleted_at, id) si algún tenant lo desmiente.
func (s *Store) ListTrash(ctx context.Context) ([]domain.Folder, []domain.File, error) {
	t := tenancy.MustFrom(ctx)
	var folders []domain.Folder
	var files []domain.File
	err := s.inTx(ctx, func(tx pgx.Tx) error {
		var e error
		folders, files, e = listTrashTx(ctx, tx, t.Slug)
		return e
	})
	return folders, files, err
}

// listTrashTx lista los elementos de papelera de NIVEL SUPERIOR (carpetas/ficheros
// borrados cuyo contenedor NO está borrado) dentro de la tx dada. Núcleo compartido
// por ListTrash («Mi unidad») y TeamListTrash (unidad de equipo): el ámbito lo
// impone la RLS del llamador.
func listTrashTx(ctx context.Context, tx pgx.Tx, tenant string) ([]domain.Folder, []domain.File, error) {
	var folders []domain.Folder
	var files []domain.File
	rows, err := tx.Query(ctx,
		`SELECT `+folderCols+` FROM folders f
		 WHERE f.tenant=$1 AND `+ambitoF+` AND f.deleted_at IS NOT NULL AND NOT f.is_root
		   AND NOT EXISTS (
		     SELECT 1 FROM folders p
		     WHERE p.tenant=$1 AND p.id=f.parent_id AND p.deleted_at IS NOT NULL)
		 ORDER BY f.deleted_at DESC, lower(f.name)`, tenant)
	if err != nil {
		return nil, nil, fmt.Errorf("store: listar papelera (carpetas): %w", err)
	}
	for rows.Next() {
		f, err := scanFolder(rows, tenant)
		if err != nil {
			rows.Close()
			return nil, nil, err
		}
		folders = append(folders, f)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, nil, err
	}

	frows, err := tx.Query(ctx,
		`SELECT `+fileCols+` FROM files fi
		 WHERE fi.tenant=$1 AND `+ambitoFi+` AND fi.deleted_at IS NOT NULL
		   AND NOT EXISTS (
		     SELECT 1 FROM folders p
		     WHERE p.tenant=$1 AND p.id=fi.folder_id AND p.deleted_at IS NOT NULL)
		 ORDER BY fi.deleted_at DESC, lower(fi.name)`, tenant)
	if err != nil {
		return nil, nil, fmt.Errorf("store: listar papelera (ficheros): %w", err)
	}
	defer frows.Close()
	for frows.Next() {
		f, err := scanFile(frows, tenant)
		if err != nil {
			return nil, nil, err
		}
		files = append(files, f)
	}
	return folders, files, frows.Err()
}

// RestoreFile devuelve un fichero de la papelera a su carpeta (deleted_at=NULL).
// Si en esa carpeta ya vive un fichero con el mismo nombre, el índice único
// parcial dispara ErrConflict (el usuario debe renombrar/eliminar el otro).
// ponytail: no re-parenta a la raíz si la carpeta original está borrada; la
// papelera solo lista ficheros de nivel superior, así que por UI no ocurre.
func (s *Store) RestoreFile(ctx context.Context, id uuid.UUID) (domain.File, error) {
	t := tenancy.MustFrom(ctx)
	var f domain.File
	err := s.inTx(ctx, func(tx pgx.Tx) error {
		row := tx.QueryRow(ctx,
			`UPDATE files SET deleted_at=NULL, updated_at=now()
			 WHERE tenant=$1 AND `+ambitoSQL+` AND id=$2 AND deleted_at IS NOT NULL
			 RETURNING `+fileCols, t.Slug, id)
		var e error
		f, e = scanFile(row, t.Slug)
		if uniqueViolation(e) {
			return domain.ErrConflict
		}
		return e
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.File{}, domain.ErrNotFound
	}
	return f, err
}

// RestoreFolder devuelve una carpeta y todo su subárbol de la papelera. Restaura
// primero la carpeta (que puede chocar con un hermano vivo del mismo nombre →
// ErrConflict) y después sus subcarpetas y ficheros descendientes.
// ponytail: restaura todo lo que esté borrado bajo el subárbol; si algún
// descendiente se borró por separado ANTES que la carpeta, también revive.
func (s *Store) RestoreFolder(ctx context.Context, id uuid.UUID) (domain.Folder, error) {
	t := tenancy.MustFrom(ctx)
	var f domain.Folder
	err := s.inTx(ctx, func(tx pgx.Tx) error {
		var e error
		f, e = restoreFolderTx(ctx, tx, t.Slug, id)
		return e
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.Folder{}, domain.ErrNotFound
	}
	return f, err
}

// restoreFolderTx restaura una carpeta y su subárbol de la papelera dentro de la tx
// dada. Núcleo compartido por RestoreFolder («Mi unidad») y TeamRestoreFolder
// (unidad de equipo): el ámbito lo impone la RLS del llamador. Devuelve pgx.ErrNoRows
// si la carpeta no está en papelera (el llamante lo traduce a ErrNotFound).
func restoreFolderTx(ctx context.Context, tx pgx.Tx, tenant string, id uuid.UUID) (domain.Folder, error) {
	row := tx.QueryRow(ctx,
		`UPDATE folders SET deleted_at=NULL, updated_at=now()
		 WHERE tenant=$1 AND `+ambitoSQL+` AND id=$2 AND deleted_at IS NOT NULL AND NOT is_root
		 RETURNING `+folderCols, tenant, id)
	f, e := scanFolder(row, tenant)
	if uniqueViolation(e) {
		return domain.Folder{}, domain.ErrConflict
	}
	if e != nil {
		return domain.Folder{}, e
	}
	if _, err := tx.Exec(ctx, `
		WITH RECURSIVE sub AS (
			SELECT id FROM folders WHERE tenant=$1 AND `+ambitoSQL+` AND id=$2
			UNION ALL
			SELECT c.id FROM folders c JOIN sub ON c.parent_id = sub.id WHERE c.tenant=$1
		)
		UPDATE folders SET deleted_at=NULL, updated_at=now()
		WHERE tenant=$1 AND `+ambitoSQL+` AND deleted_at IS NOT NULL AND id IN (SELECT id FROM sub)`, tenant, id); err != nil {
		return domain.Folder{}, fmt.Errorf("store: restaurar subcarpetas: %w", err)
	}
	if _, err := tx.Exec(ctx, `
		WITH RECURSIVE sub AS (
			SELECT id FROM folders WHERE tenant=$1 AND `+ambitoSQL+` AND id=$2
			UNION ALL
			SELECT c.id FROM folders c JOIN sub ON c.parent_id = sub.id WHERE c.tenant=$1
		)
		UPDATE files SET deleted_at=NULL, updated_at=now()
		WHERE tenant=$1 AND `+ambitoSQL+` AND deleted_at IS NOT NULL AND folder_id IN (SELECT id FROM sub)`, tenant, id); err != nil {
		return domain.Folder{}, fmt.Errorf("store: restaurar ficheros del subárbol: %w", err)
	}
	return f, nil
}

// blobReferenced indica si ALGÚN metadato del tenant (fichero vivo o en papelera,
// o versión archivada) referencia storageKey. Se llama al purgar, DESPUÉS de haber
// borrado la(s) fila(s) objetivo (y su cascada a file_versions), para decidir si el
// blob puede borrarse de S3: una copia server-side comparte el MISMO blob por
// referencia, así que un blob todavía referenciado NO debe borrarse (sería pérdida
// de datos). Lo sirve el índice (tenant, storage_key) de la migración 0008.
//
// TODA LA ORGANIZACIÓN, A PROPÓSITO: un blob lo pueden compartir un fichero personal,
// uno de una unidad y un adjunto de canal («Guardar en mi Drive» copia por
// referencia), así que la pregunta no cabe en el ámbito de quien purga. Por eso no
// lleva ambitoSQL y se hace con la puerta de mantenimiento abierta (ver
// conTodaLaOrganizacion): sin ella, con la RLS evaluada, una purga manual solo
// vería su ámbito y daría por huérfano un blob que otro sigue usando.
func blobReferenced(ctx context.Context, tx pgx.Tx, tenant, storageKey string) (bool, error) {
	var referenced bool
	err := conTodaLaOrganizacion(ctx, tx, func() error {
		return tx.QueryRow(ctx, `
			SELECT EXISTS (SELECT 1 FROM files         WHERE tenant=$1 AND storage_key=$2)
			    OR EXISTS (SELECT 1 FROM file_versions WHERE tenant=$1 AND storage_key=$2)
			    OR EXISTS (SELECT 1 FROM cipher_blobs WHERE tenant=$1 AND storage_key=$2)`,
			tenant, storageKey).Scan(&referenced)
	})
	if err != nil {
		return false, fmt.Errorf("store: comprobar referencia de blob: %w", err)
	}
	return referenced, nil
}

// conTodaLaOrganizacion ejecuta fn con la puerta de mantenimiento (mig 0009)
// abierta y la deja como estaba: cerrada en una purga manual, abierta en la
// auto-purga, que ya viene con ella.
//
// Es el ÚNICO sitio del paquete donde una transacción de una persona mira fuera de
// su ámbito, y solo para una pregunta: «¿sigue alguien usando este blob?». La
// puerta abre también las demás organizaciones, así que lo que corra dentro lleva
// SIEMPRE `tenant=$1` escrito, y no devuelve filas a quien llama: solo si existen.
func conTodaLaOrganizacion(ctx context.Context, tx pgx.Tx, fn func() error) error {
	var antes string
	if err := tx.QueryRow(ctx,
		`SELECT coalesce(current_setting('`+pg.MaintenanceSettingKey+`', true), '')`).Scan(&antes); err != nil {
		return fmt.Errorf("store: leer %s: %w", pg.MaintenanceSettingKey, err)
	}
	if antes == "on" {
		return fn()
	}
	if err := pg.SetMaintenance(ctx, tx); err != nil {
		return err
	}
	errFn := fn()
	// Se cierra también si fn falló. Si no se puede cerrar, la transacción no sigue.
	_, errCierre := tx.Exec(ctx, `SELECT set_config('`+pg.MaintenanceSettingKey+`', '', true)`)
	if errFn != nil {
		return errFn
	}
	if errCierre != nil {
		return fmt.Errorf("store: cerrar %s: %w", pg.MaintenanceSettingKey, errCierre)
	}
	return nil
}

// PurgeFile borra DEFINITIVAMENTE un fichero que está en la papelera: elimina la
// fila, libera la cuota y devuelve la clave S3 para que el servicio borre el
// blob. Solo actúa sobre ficheros ya borrados (deleted_at IS NOT NULL). Marca
// BlobOrphaned solo si ninguna otra fila (copia por referencia o versión) usa ya
// el blob, para que el servicio no borre bytes que una copia aún necesita.
func (s *Store) PurgeFile(ctx context.Context, id uuid.UUID) (DeletedFile, error) {
	t := tenancy.MustFrom(ctx)
	var df DeletedFile
	err := s.inTx(ctx, func(tx pgx.Tx) error {
		var e error
		df, e = purgeFileTx(ctx, tx, t.Slug, id)
		return e
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return DeletedFile{}, domain.ErrNotFound
	}
	return df, err
}

// purgeFileTx es el núcleo de la purga de un fichero, dentro de la tx dada y bajo
// un tenant EXPLÍCITO (no el del contexto): así lo comparten la purga MANUAL
// (PurgeFile, InTenantTx con el tenant del contexto) y la AUTO-PURGA cross-tenant
// del worker (InMaintenanceTx con el tenant de la fila reclamada). Devuelve
// pgx.ErrNoRows si el fichero no está en papelera (el llamante lo traduce). La
// guarda de blob compartido (blobReferenced) se evalúa TRAS el DELETE.
func purgeFileTx(ctx context.Context, tx pgx.Tx, tenant string, id uuid.UUID) (DeletedFile, error) {
	// Legal hold (compliance NO destructivo): un fichero de un sujeto bajo hold
	// activo NUNCA se borra físicamente (preservación DINÁMICA; sus versiones caen
	// por cascada FK, así que preservarlo las preserva). Refuerza la purga MANUAL
	// (Mi unidad + unidades de equipo); la auto-purga ya salta los preservados en el
	// claim (gc.go), esto es defensa en profundidad ante una carrera (rollback seguro).
	if held, err := fileUnderHoldTx(ctx, tx, tenant, id); err != nil {
		return DeletedFile{}, err
	} else if held {
		return DeletedFile{}, domain.ErrLegalHold
	}
	var df DeletedFile
	row := tx.QueryRow(ctx,
		`DELETE FROM files
		 WHERE tenant=$1 AND `+ambitoSQL+` AND id=$2 AND deleted_at IS NOT NULL
		 RETURNING id, storage_key, size_bytes`, tenant, id)
	if err := row.Scan(&df.ID, &df.StorageKey, &df.SizeBytes); err != nil {
		return DeletedFile{}, err
	}
	// Tras el DELETE (y su cascada a las versiones de ESTE fichero), ¿queda algún
	// otro metadato apuntando al blob? Si no, es huérfano y puede borrarse.
	referenced, err := blobReferenced(ctx, tx, tenant, df.StorageKey)
	if err != nil {
		return DeletedFile{}, err
	}
	df.BlobOrphaned = !referenced
	if liberados := bytesEnCuota(df.StorageKey, df.SizeBytes); liberados > 0 {
		if err := addUsage(ctx, tx, tenant, -liberados); err != nil {
			return DeletedFile{}, err
		}
	}
	return df, nil
}

// PurgeFolder borra DEFINITIVAMENTE una carpeta de la papelera y todo su subárbol:
// recolecta los ficheros (para borrar sus blobs y liberar cuota), elimina la fila
// de la carpeta —ON DELETE CASCADE arrastra subcarpetas, ficheros y shares— y
// devuelve las claves S3 a purgar. Solo actúa sobre carpetas ya borradas.
func (s *Store) PurgeFolder(ctx context.Context, id uuid.UUID) ([]DeletedFile, error) {
	t := tenancy.MustFrom(ctx)
	var deleted []DeletedFile
	err := s.inTx(ctx, func(tx pgx.Tx) error {
		var e error
		deleted, e = purgeFolderTx(ctx, tx, t.Slug, id)
		return e
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, domain.ErrNotFound
	}
	return deleted, err
}

// purgeFolderTx es el núcleo de la purga de una carpeta y su subárbol, dentro de la
// tx dada y bajo un tenant EXPLÍCITO (ver purgeFileTx): lo comparten la purga
// MANUAL y la AUTO-PURGA cross-tenant del worker. Recolecta los ficheros del
// subárbol (blobs+cuota), borra la carpeta (ON DELETE CASCADE arrastra subcarpetas,
// ficheros y shares) y marca huérfanos los blobs que ya no referencia nadie vivo.
// Devuelve pgx.ErrNoRows si la carpeta no está en papelera (el llamante lo traduce).
func purgeFolderTx(ctx context.Context, tx pgx.Tx, tenant string, id uuid.UUID) ([]DeletedFile, error) {
	var isRoot bool
	if err := tx.QueryRow(ctx,
		`SELECT is_root FROM folders WHERE tenant=$1 AND `+ambitoSQL+` AND id=$2 AND deleted_at IS NOT NULL`,
		tenant, id).Scan(&isRoot); err != nil {
		return nil, err // pgx.ErrNoRows => ErrNotFound arriba
	}
	if isRoot {
		return nil, domain.ErrRootImmutable
	}
	// Legal hold: si el subárbol guarda algún fichero de un sujeto bajo hold activo,
	// la cascada (ON DELETE CASCADE) lo borraría físicamente; se niega la purga de
	// toda la carpeta (preserva también los hermanos no retenidos —no destructivo—).
	if held, err := subtreeUnderHoldTx(ctx, tx, tenant, id); err != nil {
		return nil, err
	} else if held {
		return nil, domain.ErrLegalHold
	}
	rows, err := tx.Query(ctx, `
		WITH RECURSIVE sub AS (
			SELECT id FROM folders WHERE tenant=$1 AND `+ambitoSQL+` AND id=$2
			UNION ALL
			SELECT c.id FROM folders c JOIN sub ON c.parent_id = sub.id WHERE c.tenant=$1
		)
		SELECT id, storage_key, size_bytes FROM files
		WHERE tenant=$1 AND `+ambitoSQL+` AND folder_id IN (SELECT id FROM sub)`, tenant, id)
	if err != nil {
		return nil, fmt.Errorf("store: recolectar ficheros a purgar: %w", err)
	}
	var deleted []DeletedFile
	var freed int64
	for rows.Next() {
		var df DeletedFile
		if err := rows.Scan(&df.ID, &df.StorageKey, &df.SizeBytes); err != nil {
			rows.Close()
			return nil, err
		}
		freed += bytesEnCuota(df.StorageKey, df.SizeBytes)
		deleted = append(deleted, df)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if _, err := tx.Exec(ctx,
		`DELETE FROM folders WHERE tenant=$1 AND `+ambitoSQL+` AND id=$2`, tenant, id); err != nil {
		return nil, fmt.Errorf("store: purgar carpeta: %w", err)
	}
	// Tras la cascada, marca huérfanos los blobs que ya no referencia ninguna fila
	// superviviente (una copia server-side fuera del subárbol comparte el MISMO
	// blob por referencia y debe conservarlo). Un único barrido por-clave.
	if err := markOrphanedBlobs(ctx, tx, tenant, deleted); err != nil {
		return nil, err
	}
	if freed > 0 {
		if err := addUsage(ctx, tx, tenant, -freed); err != nil {
			return nil, err
		}
	}
	return deleted, nil
}

// markOrphanedBlobs fija BlobOrphaned en cada DeletedFile cuyo storage_key ya NO
// referencia ninguna fila superviviente (fichero o versión) del tenant. Resuelve
// las referencias de TODAS las claves en UNA consulta (id = ANY), sin N+1; se llama
// tras borrar las filas objetivo. Claves repetidas (dos copias del mismo blob en el
// subárbol) comparten el mismo veredicto (el servicio deduplica el borrado en S3 de
// forma best-effort).
func markOrphanedBlobs(ctx context.Context, tx pgx.Tx, tenant string, deleted []DeletedFile) error {
	if len(deleted) == 0 {
		return nil
	}
	keys := make([]string, 0, len(deleted))
	for _, df := range deleted {
		keys = append(keys, df.StorageKey)
	}
	stillReferenced := make(map[string]bool, len(keys))
	// Como blobReferenced: toda la organización, no el ámbito de quien purga.
	err := conTodaLaOrganizacion(ctx, tx, func() error {
		rows, err := tx.Query(ctx, `
			SELECT storage_key FROM files         WHERE tenant=$1 AND storage_key = ANY($2)
			UNION
			SELECT storage_key FROM file_versions WHERE tenant=$1 AND storage_key = ANY($2)
			UNION SELECT storage_key FROM cipher_blobs WHERE tenant=$1 AND storage_key = ANY($2)`,
			tenant, keys)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var k string
			if err := rows.Scan(&k); err != nil {
				return err
			}
			stillReferenced[k] = true
		}
		return rows.Err()
	})
	if err != nil {
		return fmt.Errorf("store: referencias de blobs a purgar: %w", err)
	}
	for i := range deleted {
		deleted[i].BlobOrphaned = !stillReferenced[deleted[i].StorageKey]
	}
	return nil
}

// FileCursor es la clave de continuación de una paginación keyset de ficheros.
// Ordena por (lower(name), id) ascendente, estable y sin OFFSET (CONVENTIONS
// §0.8: paginación keyset, sin N+1). El cero-valor significa "primera página".
type FileCursor struct {
	// NameLower es lower(name) del último fichero de la página anterior.
	NameLower string
	// ID desempata nombres iguales (case-insensitive) de forma determinista.
	ID uuid.UUID
}

// IsZero indica que el cursor apunta al inicio (primera página).
func (c FileCursor) IsZero() bool { return c.ID == uuid.Nil && c.NameLower == "" }

// FilePage es una página de ficheros con el cursor para pedir la siguiente.
type FilePage struct {
	Files []domain.File
	// Next es el cursor de continuación; nil si no hay más resultados.
	Next *FileCursor
}

// ListFilesPage lista ficheros vivos de una carpeta con paginación keyset por
// (lower(name), id). Devuelve hasta `limit` ficheros y, si hay más, el cursor de
// continuación. Evita OFFSET (coste O(n)) usando la clave compuesta indexada.
func (s *Store) ListFilesPage(ctx context.Context, folderID uuid.UUID, after FileCursor, limit int) (FilePage, error) {
	t := tenancy.MustFrom(ctx)
	if limit <= 0 || limit > 500 {
		limit = 100
	}
	var page FilePage
	err := s.inTx(ctx, func(tx pgx.Tx) error {
		var e error
		page, e = listFilesPageTx(ctx, tx, t.Slug, folderID, after, limit)
		return e
	})
	if err != nil {
		return FilePage{}, err
	}
	return page, nil
}

// listFilesPageTx es el núcleo del listado keyset de ficheros de una carpeta,
// dentro de la tx dada (RLS ya fijada por el llamador). Lo comparten ListFilesPage
// («Mi unidad») y el listado dentro de una unidad de equipo (teamdrive.go): el
// ámbito lo impone la RLS.
//
// El predicado `channelScopeSQL` REPITE en SQL el ámbito de canal de la política
// (mig 0014) para que el listado no dependa SOLO de la RLS: sin él, pasar el uuid de
// la carpeta de un canal listaría sus adjuntos. Se escribe genérico (contra la GUC,
// no `IS NULL`) para que siga valiendo si algún día se lista DENTRO de un canal.
func listFilesPageTx(ctx context.Context, tx pgx.Tx, tenant string, folderID uuid.UUID, after FileCursor, limit int) (FilePage, error) {
	var page FilePage
	// Carpeta destino debe existir, estar viva y ser DEL ÁMBITO en curso (evita
	// filtrar ausencia como página vacía).
	var ok bool
	if err := tx.QueryRow(ctx,
		`SELECT true FROM folders
		 WHERE tenant=$1 AND `+ambitoSQL+` AND id=$2 AND deleted_at IS NULL AND `+channelScopeSQL,
		tenant, folderID).Scan(&ok); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return FilePage{}, domain.ErrNotFound
		}
		return FilePage{}, err
	}
	// Predicado keyset: (lower(name), id) > (cursor.NameLower, cursor.ID).
	// El comparador de tuplas de PostgreSQL da un recorrido estable del índice
	// folders/files (tenant, folder_id, lower(name)). Pedimos limit+1 para
	// saber si existe una página siguiente sin un COUNT extra.
	rows, err := tx.Query(ctx,
		`SELECT `+fileCols+` FROM files
		 WHERE tenant=$1 AND `+ambitoSQL+` AND folder_id=$2 AND deleted_at IS NULL AND `+channelScopeSQL+`
		   AND (lower(name), id) > ($3, $4)
		 ORDER BY lower(name), id
		 LIMIT $5`,
		tenant, folderID, after.NameLower, after.ID, limit+1)
	if err != nil {
		return FilePage{}, fmt.Errorf("store: listar ficheros paginado: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		f, err := scanFile(rows, tenant)
		if err != nil {
			return FilePage{}, err
		}
		page.Files = append(page.Files, f)
	}
	if err := rows.Err(); err != nil {
		return FilePage{}, err
	}
	if len(page.Files) > limit {
		last := page.Files[limit-1]
		page.Files = page.Files[:limit]
		page.Next = &FileCursor{NameLower: strings.ToLower(last.Name), ID: last.ID}
	}
	return page, nil
}

// SearchFiles busca ficheros vivos por subcadena de nombre (case-insensitive).
// FolderPath resuelve la ruta absoluta de una carpeta («/a/b», «/» si es la raíz).
//
// Existe para que un id se pueda convertir en algo navegable: el explorador se mueve por
// RUTA, no por id, así que un enlace canónico /files/{id} no lleva a ninguna parte sin
// esto. Reusa el mismo resolvedor recursivo que «Recientes», «Favoritos» y la búsqueda.
func (s *Store) FolderPath(ctx context.Context, id uuid.UUID) (string, error) {
	t := tenancy.MustFrom(ctx)
	var path string
	err := s.inTx(ctx, func(tx pgx.Tx) error {
		paths, err := folderPaths(ctx, tx, t.Slug, []uuid.UUID{id})
		if err != nil {
			return err
		}
		p, ok := paths[id]
		if !ok {
			// Sin fila visible: no existe, o RLS la oculta a este tenant. Se trata igual
			// —no decir cuál de las dos es— que el resto del store.
			return domain.ErrNotFound
		}
		path = p
		return nil
	})
	return path, err
}

// SearchFiles busca por nombre y devuelve cada resultado CON SU RUTA resuelta.
//
// La ruta no es un adorno: una búsqueda cruza todo el árbol, así que sin ella un
// resultado no se puede abrir ni situar —solo enseñar—. Es el mismo criterio y el mismo
// resolvedor (folderPaths) que ya usan «Recientes» y «Favoritos», que cruzan el árbol
// igual. Antes esto devolvía []domain.File, que lleva folder_id pero no ruta.
func (s *Store) SearchFiles(ctx context.Context, query string, limit int) ([]Entry, error) {
	t := tenancy.MustFrom(ctx)
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	pattern := "%" + strings.TrimSpace(query) + "%"
	var entries []Entry
	err := s.inTx(ctx, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx,
			// channel_id IS NULL: los adjuntos de canal no se buscan desde «Mi unidad»
			// (ver GetFile). Sin esto, el buscador filtraría el NOMBRE de un fichero de
			// un canal privado a toda la organización.
			`SELECT `+fileCols+` FROM files
			 WHERE tenant=$1 AND `+ambitoSQL+` AND deleted_at IS NULL AND channel_id IS NULL AND plegar(name) LIKE plegar($2)
			 ORDER BY updated_at DESC LIMIT $3`,
			t.Slug, pattern, limit)
		if err != nil {
			return fmt.Errorf("store: búsqueda: %w", err)
		}
		// Las filas se consumen y se CIERRAN antes de resolver rutas: folderPaths lanza
		// otra consulta en la misma transacción y pgx no admite dos cursores a la vez.
		var files []domain.File
		scanErr := func() error {
			defer rows.Close()
			for rows.Next() {
				f, err := scanFile(rows, t.Slug)
				if err != nil {
					return err
				}
				files = append(files, f)
			}
			return rows.Err()
		}()
		if scanErr != nil {
			return scanErr
		}

		// Una sola consulta recursiva para TODAS las carpetas implicadas, no una por
		// resultado: con 50 resultados en 50 carpetas eso serían 50 viajes a la base.
		ids := make([]uuid.UUID, 0, len(files))
		seen := make(map[uuid.UUID]struct{}, len(files))
		for _, f := range files {
			if _, ok := seen[f.FolderID]; ok {
				continue
			}
			seen[f.FolderID] = struct{}{}
			ids = append(ids, f.FolderID)
		}
		paths, err := folderPaths(ctx, tx, t.Slug, ids)
		if err != nil {
			return err
		}
		entries = make([]Entry, 0, len(files))
		for _, f := range files {
			entries = append(entries, Entry{
				ID: f.ID, Name: f.Name, IsDir: false,
				Path:      joinFilePath(paths[f.FolderID], f.Name),
				SizeBytes: f.SizeBytes, ContentType: f.ContentType,
				UpdatedAt: f.UpdatedAt, Label: f.Label,
			})
		}
		return nil
	})
	return entries, err
}

// ---------------------------------------------------------------------------
// Cuotas
// ---------------------------------------------------------------------------

// EnsureQuota crea la fila de cuota del tenant con el límite por defecto si no
// existe, y devuelve la cuota vigente.
func (s *Store) EnsureQuota(ctx context.Context, defaultLimit int64) (domain.Quota, error) {
	t := tenancy.MustFrom(ctx)
	var q domain.Quota
	err := s.inTx(ctx, func(tx pgx.Tx) error {
		var e error
		q, e = ensureQuotaTx(ctx, tx, t.Slug, defaultLimit, false)
		return e
	})
	return q, err
}

// ensureQuotaTx es el cuerpo de EnsureQuota dentro de una transacción YA ABIERTA.
// Existe para que quien necesita cuota y raíz a la vez no pague dos.
//
// `fijar` distingue dos orígenes del límite. Falso: `defaultLimit` es el valor
// por defecto del servicio y solo vale para un tenant NUEVO —una organización
// puede haber ajustado su cuota con SetQuotaLimit y eso no se pisa—. Verdadero:
// el límite lo dicta el plan (viene en el token, cuentas personales) y manda
// sobre lo que haya: al pasar a 1 TB, o al caducar el pago, la fila se actualiza
// en la siguiente petición. Solo se escribe si el valor CAMBIA, así que la
// garantía de «leer no escribe» se conserva; la vigila el test de xmin.
func ensureQuotaTx(ctx context.Context, tx pgx.Tx, slug string, defaultLimit int64, fijar bool) (domain.Quota, error) {
	// Leer ANTES de escribir. Service.Root() pasa por aquí en CADA resolución de
	// ruta de WebDAV y en cada GET /folders/root (el arranque de la app y el de
	// Rooms), y `DO UPDATE SET tenant = EXCLUDED.tenant` es una escritura de
	// verdad aunque no cambie ningún valor: tupla muerta, WAL y un bloqueo sobre
	// la ÚNICA fila del tenant, que serializaba todas sus peticiones a la vez.
	q, err := scanQuota(tx.QueryRow(ctx, `SELECT `+quotaCols+` FROM quotas q WHERE q.tenant=$1`, slug))
	if err == nil && fijar && defaultLimit > 0 && q.LimitBytes != defaultLimit {
		return scanQuota(tx.QueryRow(ctx, `UPDATE quotas AS q SET limit_bytes = $2, updated_at = now()
			WHERE q.tenant = $1 RETURNING `+quotaCols, slug, defaultLimit))
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return q, err
	}
	// Primer contacto del tenant. Sigue siendo DO UPDATE y no DO NOTHING: si
	// otra transacción crea la fila entremedias, DO NOTHING no devolvería
	// ninguna y el alta fallaría por ErrNoRows.
	return scanQuota(tx.QueryRow(ctx, `
			INSERT INTO quotas AS q (tenant, limit_bytes) VALUES ($1, $2)
			ON CONFLICT (tenant) DO UPDATE SET tenant = EXCLUDED.tenant
			RETURNING `+quotaCols, slug, defaultLimit))
}

// quotaCols son las columnas con las que se lee una cuota (la tabla va con alias
// `q`), con lo declarado por las otras apps de la suite agregado EN LA MISMA
// consulta (mig 0026): una petición, una transacción, y ningún camino que lea
// la cuota puede olvidarse del sumando. `q.used_bytes` es solo lo de Drive; el
// total lo compone scanQuota.
const quotaCols = `q.tenant, q.limit_bytes, q.used_bytes, q.updated_at,
	COALESCE((SELECT jsonb_object_agg(e.app, e.bytes) FROM quota_external_usage e WHERE e.tenant = q.tenant), '{}'::jsonb)`

// scanQuota lee una fila de quotaCols y devuelve la cuota como la ve la suite:
// UsedBytes = lo de Drive + lo externo, con el desglose en ExternalUsage.
func scanQuota(row pgx.Row) (domain.Quota, error) {
	var q domain.Quota
	if err := row.Scan(&q.Tenant, &q.LimitBytes, &q.UsedBytes, &q.UpdatedAt, &q.ExternalUsage); err != nil {
		return domain.Quota{}, err
	}
	q.UsedBytes += q.ExternalBytes()
	return q, nil
}

// GetQuota devuelve la cuota del tenant.
func (s *Store) GetQuota(ctx context.Context) (domain.Quota, error) {
	t := tenancy.MustFrom(ctx)
	var q domain.Quota
	err := s.inTx(ctx, func(tx pgx.Tx) error {
		var e error
		q, e = scanQuota(tx.QueryRow(ctx, `SELECT `+quotaCols+` FROM quotas q WHERE q.tenant=$1`, t.Slug))
		return e
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.Quota{}, domain.ErrNotFound
	}
	return q, err
}

// SetQuotaLimit fija el límite de cuota del tenant (acción de admin).
func (s *Store) SetQuotaLimit(ctx context.Context, limit int64) (domain.Quota, error) {
	t := tenancy.MustFrom(ctx)
	var q domain.Quota
	err := s.inTx(ctx, func(tx pgx.Tx) error {
		var e error
		q, e = scanQuota(tx.QueryRow(ctx, `
			INSERT INTO quotas AS q (tenant, limit_bytes) VALUES ($1, $2)
			ON CONFLICT (tenant) DO UPDATE SET limit_bytes=EXCLUDED.limit_bytes, updated_at=now()
			RETURNING `+quotaCols, t.Slug, limit))
		return e
	})
	return q, err
}

// SetExternalUsage fija lo que otra app de la suite ocupa en la organización del
// contexto (mig 0026): la cuota del plan es una y Drive es su dueño. Es el TOTAL
// de esa app, no un delta, así que repetir la llamada no cambia nada y una que se
// pierda la corrige la siguiente. Solo escribe si el valor cambia: Cast lo
// declara tras cada subida, transcodificación y borrado, y un UPDATE al mismo
// valor sigue costando una tupla muerta y WAL.
func (s *Store) SetExternalUsage(ctx context.Context, app string, bytes int64) error {
	t := tenancy.MustFrom(ctx)
	return s.pool.InTenantTx(ctx, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, `
			INSERT INTO quota_external_usage (tenant, app, bytes) VALUES ($1, $2, $3)
			ON CONFLICT (tenant, app) DO UPDATE SET bytes = EXCLUDED.bytes, updated_at = now()
			WHERE quota_external_usage.bytes <> EXCLUDED.bytes`, t.Slug, app, bytes); err != nil {
			return fmt.Errorf("store: declarar uso externo de %s: %w", app, err)
		}
		return nil
	})
}

// reserveQuota intenta sumar delta bytes al uso del tenant, fallando con
// ErrQuotaExceeded si se supera el límite. delta puede ser negativo (libera).
func reserveQuota(ctx context.Context, tx pgx.Tx, tenant string, delta int64) error {
	if delta == 0 {
		return nil
	}
	var limit, used int64
	// Bloqueo de fila para evitar carreras de cuota entre subidas concurrentes.
	// El límite se compara con el uso de TODA la suite: lo que Cast ha declarado
	// (mig 0026) también está gastado, aunque viva en otro bucket. Solo se
	// bloquea la fila de quotas (FOR UPDATE OF q): lo externo se lee, no se toca.
	err := tx.QueryRow(ctx, `
		SELECT q.limit_bytes,
		       q.used_bytes + COALESCE((SELECT sum(e.bytes) FROM quota_external_usage e WHERE e.tenant = q.tenant), 0)::bigint
		  FROM quotas q WHERE q.tenant=$1 FOR UPDATE OF q`, tenant).
		Scan(&limit, &used)
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.ErrNotFound
	}
	if err != nil {
		return fmt.Errorf("store: leer cuota: %w", err)
	}
	if delta > 0 && used+delta > limit {
		return domain.ErrQuotaExceeded
	}
	return addUsage(ctx, tx, tenant, delta)
}

// bytesEnCuota son los bytes de un blob que cuentan en la cuota de Sytue: todos,
// salvo que viva en el bucket de terceros del tenant (mig 0023), que no gasta
// nada de aquí. Es el ÚNICO sitio que sabe esa regla; los que reservan o
// liberan cuota pasan por él.
func bytesEnCuota(storageKey string, size int64) int64 {
	if EsExterno(storageKey) {
		return 0
	}
	return size
}

// addUsage actualiza used_bytes (sin comprobar límite; usar tras reserveQuota o
// para liberar). Nunca baja de 0.
func addUsage(ctx context.Context, tx pgx.Tx, tenant string, delta int64) error {
	if _, err := tx.Exec(ctx,
		`UPDATE quotas SET used_bytes = GREATEST(0, used_bytes + $2), updated_at=now()
		 WHERE tenant=$1`, tenant, delta); err != nil {
		return fmt.Errorf("store: actualizar uso de cuota: %w", err)
	}
	return nil
}

// ---------------------------------------------------------------------------
// Compartición
// ---------------------------------------------------------------------------

// hashShareToken devuelve el SHA-256 hex del token de un enlace público. La base
// guarda SOLO este hash: el token ES la credencial del enlace y en claro se lo
// lleva cualquiera que lea la base (una copia extraviada, un volcado de soporte).
// SHA-256 a secas basta porque el token trae 32 bytes de entropía del servicio —
// no hay diccionario que probar—, y Argon2 solo añadiría coste al camino más
// caliente del servicio. Mismo criterio que `mfa_continuations` en Sytue ID.
func hashShareToken(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}

// shareCols acaba con la expresión booleana (password_hash IS NOT NULL) para que
// TODA lectura de share (crear/listar/resolver) rellene HasPassword sin exponer el
// hash. El hash en claro solo lo lee ResolveLink aparte, para verificar el prompt.
const shareCols = `id, file_id, folder_id, kind, subject, permission, created_by, expires_at, created_at, (password_hash IS NOT NULL)`

func scanShare(row pgx.Row, t string) (domain.Share, error) {
	var sh domain.Share
	if err := row.Scan(&sh.ID, &sh.FileID, &sh.FolderID, &sh.Kind, &sh.Subject,
		&sh.Permission, &sh.CreatedBy, &sh.ExpiresAt, &sh.CreatedAt, &sh.HasPassword); err != nil {
		return domain.Share{}, err
	}
	sh.Tenant = t
	return sh, nil
}

// NewShareRecord son los datos para crear una compartición.
type NewShareRecord struct {
	FileID     *uuid.UUID
	FolderID   *uuid.UUID
	Kind       domain.ShareKind
	Subject    string
	Permission domain.Permission
	Token      string // solo kind=link; secreto generado por el servicio
	// PasswordHash es el hash Argon2id PHC de la contraseña del enlace (vacío = sin
	// contraseña). Se persiste como NULL cuando está vacío.
	PasswordHash string
	CreatedBy    string
	ExpiresAt    *time.Time
}

// CreateShare inserta una compartición. Para enlaces (link) devuelve el token
// en el campo Token del resultado (única vez que se expone).
func (s *Store) CreateShare(ctx context.Context, rec NewShareRecord) (domain.Share, error) {
	t := tenancy.MustFrom(ctx)
	if !domain.ValidShareKind(rec.Kind) || !domain.ValidPermission(rec.Permission) {
		return domain.Share{}, domain.ErrInvalidName
	}
	var sh domain.Share
	err := s.inTx(ctx, func(tx pgx.Tx) error {
		if rec.FileID != nil {
			var ok bool
			if err := tx.QueryRow(ctx,
				`SELECT true FROM files WHERE tenant=$1 AND `+ambitoSQL+` AND id=$2 AND deleted_at IS NULL AND channel_id IS NULL`,
				t.Slug, *rec.FileID).Scan(&ok); err != nil {
				if errors.Is(err, pgx.ErrNoRows) {
					return domain.ErrNotFound
				}
				return err
			}
		}
		if rec.FolderID != nil {
			var ok bool
			if err := tx.QueryRow(ctx,
				`SELECT true FROM folders WHERE tenant=$1 AND `+ambitoSQL+` AND id=$2 AND deleted_at IS NULL AND channel_id IS NULL`,
				t.Slug, *rec.FolderID).Scan(&ok); err != nil {
				if errors.Is(err, pgx.ErrNoRows) {
					return domain.ErrNotFound
				}
				return err
			}
		}
		var tokenHash *string
		if rec.Kind == domain.ShareLink {
			h := hashShareToken(rec.Token)
			tokenHash = &h
		}
		var passwordHash *string
		if rec.PasswordHash != "" {
			passwordHash = &rec.PasswordHash
		}
		row := tx.QueryRow(ctx,
			`INSERT INTO shares (tenant, file_id, folder_id, kind, subject, permission, token_hash, password_hash, created_by, expires_at)
			 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
			 RETURNING `+shareCols,
			t.Slug, rec.FileID, rec.FolderID, string(rec.Kind), rec.Subject,
			string(rec.Permission), tokenHash, passwordHash, rec.CreatedBy, rec.ExpiresAt)
		var e error
		sh, e = scanShare(row, t.Slug)
		if e != nil {
			return e
		}
		if rec.Kind == domain.ShareLink {
			sh.Token = rec.Token
		}
		return nil
	})
	return sh, err
}

// ListSharesForFile lista las comparticiones vivas de un fichero.
func (s *Store) ListSharesForFile(ctx context.Context, fileID uuid.UUID) ([]domain.Share, error) {
	return s.listShares(ctx, `file_id=$2 AND EXISTS (SELECT 1 FROM files f WHERE f.tenant=$1 AND `+ambitoF+` AND f.id=shares.file_id)`, fileID)
}

// ListSharesForFolder lista las comparticiones vivas de una carpeta.
func (s *Store) ListSharesForFolder(ctx context.Context, folderID uuid.UUID) ([]domain.Share, error) {
	return s.listShares(ctx, `folder_id=$2 AND EXISTS (SELECT 1 FROM folders f WHERE f.tenant=$1 AND `+ambitoF+` AND f.id=shares.folder_id)`, folderID)
}

func (s *Store) listShares(ctx context.Context, cond string, id uuid.UUID) ([]domain.Share, error) {
	t := tenancy.MustFrom(ctx)
	var out []domain.Share
	err := s.inTx(ctx, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx,
			`SELECT `+shareCols+` FROM shares
			 WHERE tenant=$1 AND `+cond+` AND revoked_at IS NULL
			 ORDER BY created_at DESC`, t.Slug, id)
		if err != nil {
			return fmt.Errorf("store: listar shares: %w", err)
		}
		defer rows.Close()
		for rows.Next() {
			sh, err := scanShare(rows, t.Slug)
			if err != nil {
				return err
			}
			out = append(out, sh)
		}
		return rows.Err()
	})
	return out, err
}

// RevokeShare revoca (soft) una compartición.
func (s *Store) RevokeShare(ctx context.Context, id uuid.UUID) error {
	t := tenancy.MustFrom(ctx)
	return s.inTx(ctx, func(tx pgx.Tx) error {
		tag, err := tx.Exec(ctx,
			`UPDATE shares s SET revoked_at=now()
			 WHERE s.tenant=$1 AND s.id=$2 AND s.revoked_at IS NULL
			   AND (
			     (s.file_id IS NOT NULL AND EXISTS (SELECT 1 FROM files f WHERE f.tenant=$1 AND `+ambitoF+` AND f.id=s.file_id))
			     OR (s.folder_id IS NOT NULL AND EXISTS (SELECT 1 FROM folders f WHERE f.tenant=$1 AND `+ambitoF+` AND f.id=s.folder_id))
			   )`, t.Slug, id)
		if err != nil {
			return fmt.Errorf("store: revocar share: %w", err)
		}
		if tag.RowsAffected() == 0 {
			return domain.ErrNotFound
		}
		return nil
	})
}

// ResolveLink resuelve un token de enlace público a su share (vivo y no
// expirado). NO usa RLS por tenant porque el visitante anónimo no porta tenant;
// la consulta lleva el tenant en la propia fila y el token es un secreto
// inadivinable. Devuelve el slug del tenant (para acceder al blob) y, si el enlace
// está protegido, su hash Argon2id (para verificar el prompt en el servicio).
func (s *Store) ResolveLink(ctx context.Context, token string) (domain.Share, string, string, error) {
	var sh domain.Share
	var tenantSlug, passwordHash string
	// El visitante anónimo no porta tenant, así que la RLS por `app.tenant`
	// bloquearía la lectura directa. Resolvemos el tenant del token vía la
	// función SECURITY DEFINER acotada drive_resolve_share_tenant (solo tokens
	// vivos), fijamos `app.tenant` y releemos bajo RLS para devolver el share.
	// El token en claro NO viaja al motor: se compara por hash, así que tampoco
	// acaba en `pg_stat_statements` ni en el log de consultas lentas.
	tokenHash := hashShareToken(token)
	err := s.pool.InTx(ctx, func(tx pgx.Tx) error {
		if err := tx.QueryRow(ctx,
			`SELECT drive_resolve_share_tenant($1)`, tokenHash).Scan(&tenantSlug); err != nil {
			return err
		}
		if tenantSlug == "" {
			return pgx.ErrNoRows
		}
		if err := pg.SetTenant(ctx, tx, tenantSlug); err != nil {
			return err
		}
		row := tx.QueryRow(ctx,
			`SELECT `+shareCols+` FROM shares
			 WHERE tenant=$1 AND token_hash=$2 AND revoked_at IS NULL
			 AND (expires_at IS NULL OR expires_at > now())`, tenantSlug, tokenHash)
		var e error
		sh, e = scanShare(row, tenantSlug)
		if e != nil {
			return e
		}
		// El hash en claro solo se lee cuando el enlace está protegido (minoría), con
		// un lookup por PK dentro de la misma tx: barato y fuera del camino común.
		if sh.HasPassword {
			return tx.QueryRow(ctx,
				`SELECT password_hash FROM shares WHERE tenant=$1 AND id=$2`,
				tenantSlug, sh.ID).Scan(&passwordHash)
		}
		return nil
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.Share{}, "", "", domain.ErrShareExpired
	}
	return sh, tenantSlug, passwordHash, err
}

// GetFileByName devuelve un fichero vivo por (carpeta, nombre) case-insensitive.
// Aprovecha el índice único files_unique_name_in_folder (tenant, folder_id,
// lower(name)) WHERE deleted_at IS NULL: lookup O(1), no barre la carpeta. Lo usa
// la navegación pública para resolver un fichero por su nombre dentro del subárbol
// compartido sin paginar todos los ficheros de la carpeta.
func (s *Store) GetFileByName(ctx context.Context, folderID uuid.UUID, name string) (domain.File, error) {
	t := tenancy.MustFrom(ctx)
	var f domain.File
	err := s.inTx(ctx, func(tx pgx.Tx) error {
		row := tx.QueryRow(ctx,
			`SELECT `+fileCols+` FROM files
			 WHERE tenant=$1 AND `+ambitoSQL+` AND folder_id=$2 AND lower(name)=lower($3) AND deleted_at IS NULL`,
			t.Slug, folderID, name)
		var e error
		f, e = scanFile(row, t.Slug)
		return e
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.File{}, domain.ErrNotFound
	}
	return f, err
}

// ---------------------------------------------------------------------------
// Recientes y Favoritos (paridad Google Drive/Dropbox)
// ---------------------------------------------------------------------------

// Entry es un fichero o carpeta con su RUTA absoluta ya resuelta. Es la vista
// plana que consumen «Recientes» y «Favoritos» (listados que cruzan todo el
// árbol, no navegables por carpeta): la ruta permite abrir/descargar/compartir
// el elemento con la misma maquinaria por-ruta del explorador.
type Entry struct {
	ID          uuid.UUID
	Name        string
	IsDir       bool
	Path        string
	SizeBytes   int64
	ContentType string
	UpdatedAt   time.Time
	// Label es el snapshot de etiqueta de sensibilidad del fichero (nil = sin
	// etiqueta; las carpetas nunca la llevan). Viaja en la MISMA consulta que el
	// resto de la fila para que estas vistas puedan avisar de una restricción de
	// compartición antes de intentarla, como el listado principal.
	Label *domain.Label
}

// RecentCursor es la clave de continuación keyset de «Recientes», ordenada por
// (updated_at, id) DESCENDENTE (reciente-primero). El cero-valor = primera
// página. La sirve el índice files_recent_idx (tenant, updated_at DESC, id DESC).
type RecentCursor struct {
	UpdatedAt time.Time
	ID        uuid.UUID
}

// IsZero indica que el cursor apunta al inicio (primera página).
func (c RecentCursor) IsZero() bool { return c.ID == uuid.Nil && c.UpdatedAt.IsZero() }

// Cotas superiores para la primera página del keyset DESCENDENTE: una fecha
// posterior a cualquier fila real y el uuid máximo, de modo que
// (updated_at, id) < (recentUpperTime, recentUpperID) se cumple para todas las
// filas sin necesitar un segundo SQL (evita SQL dinámico, CONVENTIONS §10).
var (
	recentUpperTime = time.Date(9999, 12, 31, 23, 59, 59, 0, time.UTC)
	recentUpperID   = uuid.MustParse("ffffffff-ffff-ffff-ffff-ffffffffffff")
)

// ListRecent lista los ficheros vivos del tenant por reciente-primero
// (updated_at DESC, id DESC) con paginación keyset. Devuelve hasta `limit`
// entradas con su ruta resuelta y, si hay más, el cursor de continuación.
// ponytail: alcance por TENANT (como el resto del explorador, que es una unidad
// compartida del tenant, no por-usuario); añadir un filtro por owner_sub si el
// Drive pasa a ser por-usuario.
func (s *Store) ListRecent(ctx context.Context, after RecentCursor, limit int) ([]Entry, *RecentCursor, error) {
	t := tenancy.MustFrom(ctx)
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	upTime, upID := after.UpdatedAt, after.ID
	if after.IsZero() {
		upTime, upID = recentUpperTime, recentUpperID
	}
	var entries []Entry
	var next *RecentCursor
	err := s.inTx(ctx, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx,
			// channel_id IS NULL: un adjunto de canal no aparece en «Recientes» del
			// tenant (ver GetFile).
			`SELECT id, folder_id, name, size_bytes, content_type, updated_at, `+labelReadCols+` FROM files
			 WHERE tenant=$1 AND `+ambitoSQL+` AND deleted_at IS NULL AND channel_id IS NULL
			   AND (updated_at, id) < ($2, $3)
			 ORDER BY updated_at DESC, id DESC
			 LIMIT $4`,
			t.Slug, upTime, upID, limit+1)
		if err != nil {
			return fmt.Errorf("store: listar recientes: %w", err)
		}
		defer rows.Close()
		type rec struct {
			e        Entry
			folderID uuid.UUID
		}
		var recs []rec
		for rows.Next() {
			var r rec
			var lb domain.Label
			if err := rows.Scan(&r.e.ID, &r.folderID, &r.e.Name, &r.e.SizeBytes, &r.e.ContentType, &r.e.UpdatedAt,
				&lb.ID, &lb.Name, &lb.Rank, &lb.BlockExternalShare); err != nil {
				return err
			}
			r.e.Label = labelPtr(lb)
			recs = append(recs, r)
		}
		if err := rows.Err(); err != nil {
			return err
		}
		if len(recs) > limit {
			last := recs[limit-1].e
			recs = recs[:limit]
			next = &RecentCursor{UpdatedAt: last.UpdatedAt, ID: last.ID}
		}
		// Rutas de las carpetas contenedoras en UNA consulta (sin N+1).
		paths, err := folderPaths(ctx, tx, t.Slug, distinctFolderIDs(recs, func(r rec) uuid.UUID { return r.folderID }))
		if err != nil {
			return err
		}
		entries = make([]Entry, len(recs))
		for i, r := range recs {
			r.e.Path = joinFilePath(paths[r.folderID], r.e.Name)
			entries[i] = r.e
		}
		return nil
	})
	if err != nil {
		return nil, nil, err
	}
	return entries, next, nil
}

// ListFavorites lista los favoritos VIVOS de un usuario (carpetas primero, luego
// ficheros; reciente-marcado primero), cada uno con su ruta resuelta. Los
// favoritos en la papelera se omiten (JOIN a deleted_at IS NULL) y reaparecen al
// restaurar; al purgar el objetivo, el ON DELETE CASCADE elimina la marca.
//
//nolint:gocognit // listado con filtros combinables (tipo, propietario, papelera) resueltos en una sola consulta.
func (s *Store) ListFavorites(ctx context.Context, subject string) ([]Entry, error) {
	t := tenancy.MustFrom(ctx)
	var entries []Entry
	err := s.inTx(ctx, func(tx pgx.Tx) error {
		type folderRow struct {
			id  uuid.UUID
			nm  string
			upd time.Time
		}
		type fileRow struct {
			id, folderID uuid.UUID
			nm, ct       string
			size         int64
			upd          time.Time
			label        *domain.Label
		}
		var folders []folderRow
		var files []fileRow

		// Carpetas favoritas vivas (una tanda: leer y cerrar antes del siguiente
		// Query, pgx sólo admite un cursor abierto por transacción).
		frows, err := tx.Query(ctx,
			`SELECT fo.id, fo.name, fo.updated_at
			 FROM favorites fv JOIN folders fo ON fo.id = fv.folder_id AND fo.tenant = fv.tenant
			 WHERE fv.tenant=$1 AND fv.subject=$2 AND fv.folder_id IS NOT NULL
			   AND `+ambitoFo+`
			   AND fo.deleted_at IS NULL AND NOT fo.is_root
			 ORDER BY fv.created_at DESC`, t.Slug, subject)
		if err != nil {
			return fmt.Errorf("store: favoritos (carpetas): %w", err)
		}
		for frows.Next() {
			var r folderRow
			if err := frows.Scan(&r.id, &r.nm, &r.upd); err != nil {
				frows.Close()
				return err
			}
			folders = append(folders, r)
		}
		frows.Close()
		if err := frows.Err(); err != nil {
			return err
		}

		// Ficheros favoritos vivos.
		filerows, err := tx.Query(ctx,
			`SELECT fi.id, fi.folder_id, fi.name, fi.size_bytes, fi.content_type, fi.updated_at,
			        fi.label_id, fi.label_name, fi.label_rank::int, fi.label_block_external_share
			 FROM favorites fv JOIN files fi ON fi.id = fv.file_id AND fi.tenant = fv.tenant
			 WHERE fv.tenant=$1 AND fv.subject=$2 AND fv.file_id IS NOT NULL
			   AND `+ambitoFi+`
			   AND fi.deleted_at IS NULL
			 ORDER BY fv.created_at DESC`, t.Slug, subject)
		if err != nil {
			return fmt.Errorf("store: favoritos (ficheros): %w", err)
		}
		for filerows.Next() {
			var r fileRow
			var lb domain.Label
			if err := filerows.Scan(&r.id, &r.folderID, &r.nm, &r.size, &r.ct, &r.upd,
				&lb.ID, &lb.Name, &lb.Rank, &lb.BlockExternalShare); err != nil {
				filerows.Close()
				return err
			}
			r.label = labelPtr(lb)
			files = append(files, r)
		}
		filerows.Close()
		if err := filerows.Err(); err != nil {
			return err
		}

		// Rutas: la propia de cada carpeta favorita + la contenedora de cada
		// fichero favorito, todo en UNA consulta.
		ids := make([]uuid.UUID, 0, len(folders)+len(files))
		seen := make(map[uuid.UUID]bool, len(folders)+len(files))
		add := func(id uuid.UUID) {
			if !seen[id] {
				seen[id] = true
				ids = append(ids, id)
			}
		}
		for _, f := range folders {
			add(f.id)
		}
		for _, f := range files {
			add(f.folderID)
		}
		paths, err := folderPaths(ctx, tx, t.Slug, ids)
		if err != nil {
			return err
		}

		entries = make([]Entry, 0, len(folders)+len(files))
		for _, f := range folders {
			entries = append(entries, Entry{ID: f.id, Name: f.nm, IsDir: true, Path: paths[f.id], UpdatedAt: f.upd})
		}
		for _, f := range files {
			entries = append(entries, Entry{
				ID: f.id, Name: f.nm, IsDir: false, Path: joinFilePath(paths[f.folderID], f.nm),
				SizeBytes: f.size, ContentType: f.ct, UpdatedAt: f.upd, Label: f.label,
			})
		}
		return nil
	})
	return entries, err
}

// SetFavorite marca (on=true) o desmarca (on=false) un fichero o carpeta como
// favorito del usuario. Exactamente uno de fileID/folderID debe ser no-nil (lo
// valida el servicio). Marcar es idempotente (ON CONFLICT DO NOTHING) y exige que
// el objetivo exista y esté vivo; desmarcar es idempotente (DELETE sin fila = no-op).
func (s *Store) SetFavorite(ctx context.Context, subject string, fileID, folderID *uuid.UUID, on bool) error {
	t := tenancy.MustFrom(ctx)
	return s.inTx(ctx, func(tx pgx.Tx) error {
		if !on {
			if fileID != nil {
				_, err := tx.Exec(ctx,
					`DELETE FROM favorites WHERE tenant=$1 AND subject=$2 AND file_id=$3`, t.Slug, subject, *fileID)
				return err
			}
			_, err := tx.Exec(ctx,
				`DELETE FROM favorites WHERE tenant=$1 AND subject=$2 AND folder_id=$3`, t.Slug, subject, *folderID)
			return err
		}
		if fileID != nil {
			var ok bool
			// channel_id IS NULL: un adjunto de canal no se puede marcar como favorito
			// (si no, su NOMBRE aparecería en «Favoritos» de quien acierte el uuid).
			if err := tx.QueryRow(ctx,
				`SELECT true FROM files
				 WHERE tenant=$1 AND `+ambitoSQL+` AND id=$2 AND deleted_at IS NULL AND channel_id IS NULL`,
				t.Slug, *fileID).Scan(&ok); err != nil {
				if errors.Is(err, pgx.ErrNoRows) {
					return domain.ErrNotFound
				}
				return err
			}
			_, err := tx.Exec(ctx,
				`INSERT INTO favorites (tenant, subject, file_id) VALUES ($1, $2, $3)
				 ON CONFLICT (tenant, subject, file_id) WHERE file_id IS NOT NULL DO NOTHING`,
				t.Slug, subject, *fileID)
			return err
		}
		var ok bool
		if err := tx.QueryRow(ctx,
			`SELECT true FROM folders
			 WHERE tenant=$1 AND `+ambitoSQL+` AND id=$2 AND deleted_at IS NULL AND NOT is_root AND channel_id IS NULL`,
			t.Slug, *folderID).Scan(&ok); err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return domain.ErrNotFound
			}
			return err
		}
		_, err := tx.Exec(ctx,
			`INSERT INTO favorites (tenant, subject, folder_id) VALUES ($1, $2, $3)
			 ON CONFLICT (tenant, subject, folder_id) WHERE folder_id IS NOT NULL DO NOTHING`,
			t.Slug, subject, *folderID)
		return err
	})
}

// distinctFolderIDs extrae los folder_id únicos de un lote (para resolver rutas
// en una sola consulta, sin repetir carpetas).
func distinctFolderIDs[T any](rows []T, key func(T) uuid.UUID) []uuid.UUID {
	seen := make(map[uuid.UUID]bool, len(rows))
	out := make([]uuid.UUID, 0, len(rows))
	for _, r := range rows {
		id := key(r)
		if !seen[id] {
			seen[id] = true
			out = append(out, id)
		}
	}
	return out
}

// joinFilePath compone la ruta de un fichero a partir de la de su carpeta.
// JoinFilePath compone la ruta de un fichero a partir de la de su carpeta. Exportada
// para que el servicio arme la ruta de un id sin duplicar el criterio de la raíz.
func JoinFilePath(folderPath, name string) string { return joinFilePath(folderPath, name) }

func joinFilePath(folderPath, name string) string {
	if folderPath == "" || folderPath == "/" {
		return "/" + name
	}
	return folderPath + "/" + name
}

// ---------------------------------------------------------------------------
// Compartido conmigo + verja de acceso por share (kind=user|group)
// ---------------------------------------------------------------------------

// recipientPred filtra shares VIGENTES (no revocados, no caducados) dirigidos al
// usuario: kind='user' con subject = su sub ($2), o kind='group' con subject en
// uno de sus grupos ($3, un text[]). Deny-by-default: sin coincidencia no
// devuelve nada. Lo sirve shares_subject_idx (tenant, subject) WHERE revoked_at
// IS NULL (un recorrido indexado por sub y por cada grupo; sin barrer la tabla,
// sin N+1). $1=tenant, $2=subject, $3=groups.
const recipientPred = `revoked_at IS NULL
	AND (expires_at IS NULL OR expires_at > now())
	AND ((kind = 'user' AND subject = $2) OR (kind = 'group' AND subject = ANY ($3)))`

// SharedEntry es un recurso (fichero o carpeta) compartido CON el usuario vía un
// share kind=user|group vigente: el nodo raíz del share con su ruta resuelta, el
// permiso que concede (viewer/editor) y quién lo compartió (created_by del share).
type SharedEntry struct {
	Entry
	Permission domain.Permission
	SharedBy   string
}

// sharesForSubjectTx lee, dentro de una tx, los shares vigentes dirigidos al
// usuario (por su sub o sus grupos), reciente-primero.
func sharesForSubjectTx(ctx context.Context, tx pgx.Tx, tenant, subject string, groups []string) ([]domain.Share, error) {
	if groups == nil {
		groups = []string{}
	}
	rows, err := tx.Query(ctx,
		`SELECT `+shareCols+` FROM shares
		 WHERE tenant = $1 AND `+recipientPred+`
		 ORDER BY created_at DESC, id`, tenant, subject, groups)
	if err != nil {
		return nil, fmt.Errorf("store: shares por sujeto: %w", err)
	}
	defer rows.Close()
	var out []domain.Share
	for rows.Next() {
		sh, err := scanShare(rows, tenant)
		if err != nil {
			return nil, err
		}
		out = append(out, sh)
	}
	return out, rows.Err()
}

// ListSharesForSubject lista los shares VIGENTES kind=user|group dirigidos al
// usuario (subject = su sub) o a alguno de sus grupos, reciente-primero.
// Deny-by-default: sin coincidencia, lista vacía (nunca expone shares ajenos).
func (s *Store) ListSharesForSubject(ctx context.Context, subject string, groups []string) ([]domain.Share, error) {
	t := tenancy.MustFrom(ctx)
	var out []domain.Share
	err := s.inTx(ctx, func(tx pgx.Tx) error {
		var e error
		out, e = sharesForSubjectTx(ctx, tx, t.Slug, subject, groups)
		return e
	})
	return out, err
}

// ShareForResource es la VERJA de acceso: devuelve el share VIGENTE que concede al
// usuario (por su sub o grupos) acceso al recurso dado (fichero o carpeta), con el
// permiso MÁS FUERTE si hay varios (write > read). Sin share vigente devuelve
// ErrNotFound —deny-by-default, sin distinguir «no existe» de «sin permiso» para
// no filtrar la existencia del recurso—. Los shares caducados no cuentan (los
// excluye recipientPred con expires_at).
func (s *Store) ShareForResource(ctx context.Context, subject string, groups []string, resourceID uuid.UUID) (domain.Share, error) {
	if groups == nil {
		groups = []string{}
	}
	t := tenancy.MustFrom(ctx)
	var sh domain.Share
	err := s.inTx(ctx, func(tx pgx.Tx) error {
		row := tx.QueryRow(ctx,
			`SELECT `+shareCols+` FROM shares
			 WHERE tenant = $1 AND `+recipientPred+`
			   AND (file_id = $4 OR folder_id = $4)
			 ORDER BY (permission = 'write') DESC, created_at DESC
			 LIMIT 1`, t.Slug, subject, groups, resourceID)
		var e error
		sh, e = scanShare(row, t.Slug)
		return e
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.Share{}, domain.ErrNotFound
	}
	return sh, err
}

// ListSharedWithMe lista los recursos compartidos CON el usuario por otros vía
// shares kind=user|group vigentes: el nodo raíz de cada share, DEDUPLICADO (un
// recurso compartido por varias vías aparece una vez, con el permiso más fuerte y
// el share más reciente como «compartido por»), con su ruta resuelta, el permiso y
// quién lo compartió. Reciente-primero por la fecha del share. Sin N+1: shares en
// un recorrido indexado, metadatos de destino en dos consultas por lote y rutas en
// una (folderPaths). Los destinos en la papelera se omiten (reaparecen al restaurar).
//
//nolint:gocognit,gocyclo // «compartido conmigo»: une shares directos, por grupo y por enlace, deduplicando en una pasada.
func (s *Store) ListSharedWithMe(ctx context.Context, subject string, groups []string) ([]SharedEntry, error) {
	t := tenancy.MustFrom(ctx)
	var out []SharedEntry
	err := s.inTx(ctx, func(tx pgx.Tx) error {
		shares, err := sharesForSubjectTx(ctx, tx, t.Slug, subject, groups)
		if err != nil {
			return err
		}
		type ref struct {
			isDir    bool
			perm     domain.Permission
			sharedBy string
		}
		var order []uuid.UUID
		refs := make(map[uuid.UUID]*ref, len(shares))
		for _, sh := range shares {
			var id uuid.UUID
			var isDir bool
			switch {
			case sh.FolderID != nil:
				id, isDir = *sh.FolderID, true
			case sh.FileID != nil:
				id, isDir = *sh.FileID, false
			default:
				continue
			}
			if r, ok := refs[id]; ok {
				if sh.Permission == domain.PermWrite {
					r.perm = domain.PermWrite
				}
				continue
			}
			refs[id] = &ref{isDir: isDir, perm: sh.Permission, sharedBy: sh.CreatedBy}
			order = append(order, id)
		}
		if len(order) == 0 {
			return nil
		}

		var folderIDs, fileIDs []uuid.UUID
		for _, id := range order {
			if refs[id].isDir {
				folderIDs = append(folderIDs, id)
			} else {
				fileIDs = append(fileIDs, id)
			}
		}
		type folderMeta struct {
			name string
			upd  time.Time
		}
		type fileMeta struct {
			folderID uuid.UUID
			name, ct string
			size     int64
			upd      time.Time
			label    *domain.Label
		}
		folderMetas := make(map[uuid.UUID]folderMeta, len(folderIDs))
		fileMetas := make(map[uuid.UUID]fileMeta, len(fileIDs))
		pathIDs := make([]uuid.UUID, 0, len(order))

		var paths map[uuid.UUID]string
		if err := conTodaLaOrganizacion(ctx, tx, func() error {
			// Carpetas destino VIVAS (una tanda: leer y cerrar antes del siguiente Query).
			if len(folderIDs) > 0 {
				rows, err := tx.Query(ctx,
					`SELECT id, name, updated_at FROM folders
					 WHERE tenant=$1 AND team_drive_id IS NULL AND channel_id IS NULL AND id = ANY($2) AND deleted_at IS NULL AND NOT is_root`,
					t.Slug, folderIDs)
				if err != nil {
					return fmt.Errorf("store: compartido conmigo (carpetas): %w", err)
				}
				for rows.Next() {
					var id uuid.UUID
					var m folderMeta
					if err := rows.Scan(&id, &m.name, &m.upd); err != nil {
						rows.Close()
						return err
					}
					folderMetas[id] = m
					pathIDs = append(pathIDs, id) // la ruta de la propia carpeta
				}
				rows.Close()
				if err := rows.Err(); err != nil {
					return err
				}
			}
			// Ficheros destino VIVOS.
			if len(fileIDs) > 0 {
				rows, err := tx.Query(ctx,
					`SELECT id, folder_id, name, size_bytes, content_type, updated_at, `+labelReadCols+` FROM files
					 WHERE tenant=$1 AND team_drive_id IS NULL AND channel_id IS NULL AND id = ANY($2) AND deleted_at IS NULL`,
					t.Slug, fileIDs)
				if err != nil {
					return fmt.Errorf("store: compartido conmigo (ficheros): %w", err)
				}
				for rows.Next() {
					var id uuid.UUID
					var m fileMeta
					var lb domain.Label
					if err := rows.Scan(&id, &m.folderID, &m.name, &m.size, &m.ct, &m.upd,
						&lb.ID, &lb.Name, &lb.Rank, &lb.BlockExternalShare); err != nil {
						rows.Close()
						return err
					}
					m.label = labelPtr(lb)
					fileMetas[id] = m
					pathIDs = append(pathIDs, m.folderID) // la ruta de la carpeta contenedora
				}
				rows.Close()
				if err := rows.Err(); err != nil {
					return err
				}
			}

			var err error
			paths, err = folderPaths(ctx, tx, t.Slug, pathIDs)
			return err
		}); err != nil {
			return err
		}
		out = make([]SharedEntry, 0, len(order))
		for _, id := range order {
			r := refs[id]
			if r.isDir {
				m, ok := folderMetas[id]
				if !ok {
					continue // destino borrado/inaccesible: se omite
				}
				out = append(out, SharedEntry{
					Entry:      Entry{ID: id, Name: m.name, IsDir: true, Path: paths[id], UpdatedAt: m.upd},
					Permission: r.perm, SharedBy: r.sharedBy,
				})
				continue
			}
			m, ok := fileMetas[id]
			if !ok {
				continue
			}
			out = append(out, SharedEntry{
				Entry: Entry{
					ID: id, Name: m.name, IsDir: false, Path: joinFilePath(paths[m.folderID], m.name),
					SizeBytes: m.size, ContentType: m.ct, UpdatedAt: m.upd, Label: m.label,
				},
				Permission: r.perm, SharedBy: r.sharedBy,
			})
		}
		return nil
	})
	return out, err
}

// folderPaths resuelve la RUTA absoluta ("/", "/A/B") de cada carpeta dada,
// caminando hacia la raíz en UNA sola consulta (recursive CTE), sin N+1. Las
// carpetas ausentes o de otro tenant (RLS) no aparecen en el mapa (el llamador
// trata la ausencia como raíz). El comparador acumula los nombres de padre a hijo
// y para al llegar a la raíz (name=”).
func folderPaths(ctx context.Context, tx pgx.Tx, tenant string, ids []uuid.UUID) (map[uuid.UUID]string, error) {
	out := make(map[uuid.UUID]string, len(ids))
	if len(ids) == 0 {
		return out, nil
	}
	rows, err := tx.Query(ctx, `
		WITH RECURSIVE walk AS (
			SELECT id AS target, parent_id AS cur_parent, is_root AS cur_is_root,
			       CASE WHEN is_root THEN ''::text ELSE name END AS acc
			FROM folders WHERE tenant=$1 AND `+ambitoSQL+` AND id = ANY($2)
			UNION ALL
			SELECT w.target, p.parent_id, p.is_root,
			       CASE WHEN p.is_root THEN w.acc ELSE p.name || '/' || w.acc END
			FROM walk w JOIN folders p ON p.id = w.cur_parent AND p.tenant=$1
			WHERE NOT w.cur_is_root
		)
		SELECT target, acc FROM walk WHERE cur_is_root`, tenant, ids)
	if err != nil {
		return nil, fmt.Errorf("store: resolver rutas de carpetas: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var id uuid.UUID
		var acc string
		if err := rows.Scan(&id, &acc); err != nil {
			return nil, err
		}
		out[id] = "/" + acc
	}
	return out, rows.Err()
}

// ---------------------------------------------------------------------------
// Operaciones en lote (mover/papelera N) y ZIP en streaming
// ---------------------------------------------------------------------------

// BatchResult cuenta los elementos realmente afectados por una operación en lote.
// El total puede ser menor que el pedido: los ítems ajenos (RLS), inexistentes o
// ya borrados no casan con el filtro `id = ANY(...)` y se omiten silenciosamente
// (deny-by-default, mismo criterio por-ítem que las operaciones unitarias).
type BatchResult struct {
	Files   int
	Folders int
}

// idsOrEmpty normaliza un slice nil a vacío para que `id = ANY($1)` sea válido
// (pgx codifica el vacío como '{}', que no casa con nada) sin ramas especiales.
func idsOrEmpty(ids []uuid.UUID) []uuid.UUID {
	if ids == nil {
		return []uuid.UUID{}
	}
	return ids
}

// BatchMove reubica bajo targetFolderID, en UNA operación por tabla (WHERE id =
// ANY), los ficheros (fileIDs) y las carpetas (folderIDs) indicados —sin N+1, sin
// N round-trips—. GATEADO por ítem: el `tenant=$1` (RLS) + `id = ANY(...)` limita
// el efecto a lo del propio tenant y vivo; lo ajeno/inexistente/borrado se omite y
// no cuenta. La carpeta raíz nunca se mueve (NOT is_root). Un choque de nombre en
// el destino aborta TODA la operación (unique index → ErrConflict, atómico). El
// destino debe existir y estar vivo (si no, ErrInvalidParent) y no puede ser una de
// las carpetas movidas ni descender de ninguna (ErrCycle) —una sola consulta de
// ciclo que camina del destino hacia la raíz y cruza con ANY(folderIDs)—.
func (s *Store) BatchMove(ctx context.Context, fileIDs, folderIDs []uuid.UUID, targetFolderID uuid.UUID) (BatchResult, error) {
	t := tenancy.MustFrom(ctx)
	fileIDs, folderIDs = idsOrEmpty(fileIDs), idsOrEmpty(folderIDs)
	var res BatchResult
	err := s.inTx(ctx, func(tx pgx.Tx) error {
		var ok bool
		if err := tx.QueryRow(ctx,
			`SELECT true FROM folders WHERE tenant=$1 AND `+ambitoSQL+` AND id=$2 AND deleted_at IS NULL`,
			t.Slug, targetFolderID).Scan(&ok); err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return domain.ErrInvalidParent
			}
			return err
		}
		if len(folderIDs) > 0 {
			// Ciclo: el destino no puede ser una de las carpetas movidas ni descender
			// de ninguna. Caminamos del destino hacia la raíz y cruzamos con ANY.
			var cycle bool
			if err := tx.QueryRow(ctx, `
				WITH RECURSIVE up AS (
					SELECT id, parent_id FROM folders WHERE tenant=$1 AND `+ambitoSQL+` AND id=$2
					UNION ALL
					SELECT f.id, f.parent_id FROM folders f JOIN up ON f.id = up.parent_id WHERE f.tenant=$1
				)
				SELECT EXISTS (SELECT 1 FROM up WHERE id = ANY($3))`,
				t.Slug, targetFolderID, folderIDs).Scan(&cycle); err != nil {
				return fmt.Errorf("store: comprobar ciclo en lote: %w", err)
			}
			if cycle {
				return domain.ErrCycle
			}
		}
		tag, err := tx.Exec(ctx,
			`UPDATE files SET folder_id=$3, updated_at=now()
			 WHERE tenant=$1 AND `+ambitoSQL+` AND id = ANY($2) AND deleted_at IS NULL`,
			t.Slug, fileIDs, targetFolderID)
		if err != nil {
			if uniqueViolation(err) {
				return domain.ErrConflict
			}
			return fmt.Errorf("store: mover ficheros en lote: %w", err)
		}
		res.Files = int(tag.RowsAffected())
		tag, err = tx.Exec(ctx,
			`UPDATE folders SET parent_id=$3, updated_at=now()
			 WHERE tenant=$1 AND `+ambitoSQL+` AND id = ANY($2) AND deleted_at IS NULL AND NOT is_root`,
			t.Slug, folderIDs, targetFolderID)
		if err != nil {
			if uniqueViolation(err) {
				return domain.ErrConflict
			}
			return fmt.Errorf("store: mover carpetas en lote: %w", err)
		}
		res.Folders = int(tag.RowsAffected())
		return nil
	})
	return res, err
}

// scanDeletedFiles drena y cierra un cursor de `UPDATE ... RETURNING id,
// storage_key, size_bytes`, devolviendo los ficheros afectados.
func scanDeletedFiles(rows pgx.Rows) ([]DeletedFile, error) {
	defer rows.Close()
	var out []DeletedFile
	for rows.Next() {
		var df DeletedFile
		if err := rows.Scan(&df.ID, &df.StorageKey, &df.SizeBytes); err != nil {
			return nil, err
		}
		out = append(out, df)
	}
	return out, rows.Err()
}

// BatchTrash envía a la papelera, en UNA operación por consulta (WHERE id = ANY /
// recursive CTE seeded con ANY), N ficheros sueltos (fileIDs) y los subárboles
// completos de N carpetas (folderIDs). Soft delete idéntico a DeleteFile/
// DeleteFolder: marca deleted_at, CONSERVA blobs y cuota (recuperable; la purga
// libera). GATEADO por ítem vía RLS + ANY (lo ajeno/inexistente/borrado se omite);
// la raíz nunca se borra (NOT is_root). Devuelve los ficheros afectados (para el
// evento) y el nº de carpetas borradas. Sin N+1 por carpeta.
func (s *Store) BatchTrash(ctx context.Context, fileIDs, folderIDs []uuid.UUID) ([]DeletedFile, int, error) {
	t := tenancy.MustFrom(ctx)
	fileIDs, folderIDs = idsOrEmpty(fileIDs), idsOrEmpty(folderIDs)
	var deleted []DeletedFile
	var folderCount int
	err := s.inTx(ctx, func(tx pgx.Tx) error {
		if len(fileIDs) > 0 {
			rows, err := tx.Query(ctx,
				`UPDATE files SET deleted_at=now(), updated_at=now()
				 WHERE tenant=$1 AND `+ambitoSQL+` AND id = ANY($2) AND deleted_at IS NULL
				 RETURNING id, storage_key, size_bytes`, t.Slug, fileIDs)
			if err != nil {
				return fmt.Errorf("store: papelera ficheros en lote: %w", err)
			}
			deleted, err = scanDeletedFiles(rows)
			if err != nil {
				return err
			}
		}
		if len(folderIDs) > 0 {
			// Ficheros del subárbol de todas las carpetas seleccionadas (una pasada).
			rows, err := tx.Query(ctx, `
				WITH RECURSIVE sub AS (
					SELECT id FROM folders WHERE tenant=$1 AND `+ambitoSQL+` AND id = ANY($2) AND NOT is_root
					UNION ALL
					SELECT f.id FROM folders f JOIN sub ON f.parent_id = sub.id WHERE f.tenant=$1
				)
				UPDATE files SET deleted_at=now(), updated_at=now()
				WHERE tenant=$1 AND `+ambitoSQL+` AND deleted_at IS NULL AND folder_id IN (SELECT id FROM sub)
				RETURNING id, storage_key, size_bytes`, t.Slug, folderIDs)
			if err != nil {
				return fmt.Errorf("store: papelera subárbol (ficheros) en lote: %w", err)
			}
			more, err := scanDeletedFiles(rows)
			if err != nil {
				return err
			}
			deleted = append(deleted, more...)
			// Las carpetas del subárbol (misma derivación por parent_id, intacto).
			tag, err := tx.Exec(ctx, `
				WITH RECURSIVE sub AS (
					SELECT id FROM folders WHERE tenant=$1 AND `+ambitoSQL+` AND id = ANY($2) AND NOT is_root
					UNION ALL
					SELECT f.id FROM folders f JOIN sub ON f.parent_id = sub.id WHERE f.tenant=$1
				)
				UPDATE folders SET deleted_at=now(), updated_at=now()
				WHERE tenant=$1 AND `+ambitoSQL+` AND deleted_at IS NULL AND id IN (SELECT id FROM sub)`, t.Slug, folderIDs)
			if err != nil {
				return fmt.Errorf("store: papelera subárbol (carpetas) en lote: %w", err)
			}
			folderCount = int(tag.RowsAffected())
		}
		return nil
	})
	if err != nil {
		return nil, 0, err
	}
	return deleted, folderCount, nil
}

// ZipEntry es un fichero a incluir en un ZIP: su clave S3 (para abrir el blob), su
// ruta relativa DENTRO del zip y su tamaño. La ruta se compone de nombres validados
// (domain.ValidName, sin '/'/'..'); el servicio la re-sanea contra path traversal
// antes de escribirla.
type ZipEntry struct {
	StorageKey string
	Path       string
	Size       int64
}

// CollectZip reúne, SIN N+1, los ficheros a incluir en un ZIP de una selección:
//   - Por cada carpeta de folderIDs, todos los ficheros vivos de su subárbol, con
//     su ruta relativa prefijada por el nombre de la carpeta raíz (un recursive CTE
//     seeded con ANY; no una consulta por carpeta).
//   - Cada fichero suelto de fileIDs, en la raíz del zip por su nombre.
//
// GATEADO por ítem: `tenant=$1` (RLS) + `deleted_at IS NULL` + `channel_id IS NULL`
// → solo se incluye lo accesible y vivo del propio tenant; lo ajeno/borrado nunca
// aparece, y un ADJUNTO de canal tampoco se puede meter en un ZIP conociendo su uuid
// (deny-by-default). Si el total supera maxFiles (>0), devuelve ErrBatchTooLarge.
func (s *Store) CollectZip(ctx context.Context, fileIDs, folderIDs []uuid.UUID, maxFiles int) ([]ZipEntry, error) {
	t := tenancy.MustFrom(ctx)
	fileIDs, folderIDs = idsOrEmpty(fileIDs), idsOrEmpty(folderIDs)
	var out []ZipEntry
	err := s.inTx(ctx, func(tx pgx.Tx) error {
		if len(folderIDs) > 0 {
			rows, err := tx.Query(ctx, `
				WITH RECURSIVE tree AS (
					SELECT id, name AS relpath
					FROM folders WHERE tenant=$1 AND `+ambitoSQL+` AND id = ANY($2) AND deleted_at IS NULL AND NOT is_root
					UNION ALL
					SELECT c.id, tr.relpath || '/' || c.name
					FROM folders c JOIN tree tr ON c.parent_id = tr.id
					WHERE c.tenant=$1 AND c.deleted_at IS NULL
				)
				SELECT f.storage_key, tr.relpath || '/' || f.name AS zip_path, f.size_bytes
				FROM tree tr JOIN files f ON f.folder_id = tr.id
				WHERE f.tenant=$1 AND f.deleted_at IS NULL
				ORDER BY zip_path`, t.Slug, folderIDs)
			if err != nil {
				return fmt.Errorf("store: recolectar zip (subárbol): %w", err)
			}
			out, err = scanZipEntries(rows, out)
			if err != nil {
				return err
			}
		}
		if len(fileIDs) > 0 {
			rows, err := tx.Query(ctx,
				`SELECT storage_key, name AS zip_path, size_bytes FROM files
				 WHERE tenant=$1 AND `+ambitoSQL+` AND id = ANY($2) AND deleted_at IS NULL AND `+channelScopeSQL+`
				 ORDER BY lower(name)`, t.Slug, fileIDs)
			if err != nil {
				return fmt.Errorf("store: recolectar zip (sueltos): %w", err)
			}
			out, err = scanZipEntries(rows, out)
			if err != nil {
				return err
			}
		}
		if maxFiles > 0 && len(out) > maxFiles {
			return domain.ErrBatchTooLarge
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return out, nil
}

// scanZipEntries drena y cierra un cursor de (storage_key, zip_path, size_bytes),
// acumulando en acc (para encadenar subárbol + sueltos en un único slice).
func scanZipEntries(rows pgx.Rows, acc []ZipEntry) ([]ZipEntry, error) {
	defer rows.Close()
	for rows.Next() {
		var e ZipEntry
		if err := rows.Scan(&e.StorageKey, &e.Path, &e.Size); err != nil {
			return nil, err
		}
		acc = append(acc, e)
	}
	return acc, rows.Err()
}
