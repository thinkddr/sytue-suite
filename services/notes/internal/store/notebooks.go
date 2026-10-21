// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package store

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"git.sytue.eu/sytue/one/platform/pkg/pg"
	"git.sytue.eu/sytue/one/platform/pkg/tenancy"
	"git.sytue.eu/sytue/one/services/notes/internal/domain"
)

// Las columnas de un cuaderno, en el orden que espera `scanNotebook`.
//
// DOS FORMAS DE LA MISMA LISTA, y no es duplicación: los `SELECT` van sobre
// `FROM notebooks nb` y necesitan el alias, pero un `INSERT … RETURNING` no
// tiene tabla que aliasar y Postgres responde «missing FROM-clause entry for
// table "nb"». Se usaba la aliasada en los dos sitios, así que CREAR y
// ACTUALIZAR un cuaderno fallaban siempre contra una base de datos real. No lo
// vio nadie porque los tests de unidad usan un almacén de mentira; lo cazaron
// los de integración, que es para lo que están.
//
// La aliasada se DERIVA de la otra para que no puedan separarse: añadir una
// columna en un sitio y olvidarla en el otro es cómo se vuelve aquí.
const nbColsBase = "id, owner_sub, parent_id, title, emoji, color, layout, " +
	"visibility, indexable, encrypted, crypto, room_id, public_token, " +
	"position, created_at, updated_at, daily_date, archived_at"

// La misma lista con el alias delante. En UNA línea la base para que este
// reemplazo sea evidente: no hay saltos ni tabuladores que esquivar.
var nbCols = "nb." + strings.ReplaceAll(nbColsBase, ", ", ", nb.")

func scanNotebook(row pgx.Row, tenantSlug, sub string) (domain.Notebook, error) {
	var nb domain.Notebook
	var crypto []byte
	var token string
	if err := row.Scan(&nb.ID, &nb.OwnerSub, &nb.ParentID, &nb.Title, &nb.Emoji, &nb.Color,
		&nb.Layout, &nb.Visibility, &nb.Indexable, &nb.Encrypted, &crypto, &nb.RoomID,
		&token, &nb.Position, &nb.CreatedAt, &nb.UpdatedAt, &nb.DailyDate, &nb.ArchivedAt); err != nil {
		return domain.Notebook{}, err
	}
	nb.Tenant = tenantSlug
	if nb.Encrypted && len(crypto) > 0 {
		var cp domain.CryptoParams
		if err := json.Unmarshal(crypto, &cp); err == nil && cp.V != 0 {
			nb.Crypto = &cp
		}
	}
	// El token del enlace público es una CREDENCIAL de lectura: solo lo ve su
	// dueño, que es quien puede repartirlo. Para el resto no existe.
	if nb.OwnerSub == sub {
		nb.PublicToken = token
	}
	return nb, nil
}

// nuevoToken genera el secreto del enlace público (32 bytes, base64url).
func nuevoToken() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("notes: generar token público: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}

// ListNotebooks devuelve el árbol completo visible para `sub`, con el recuento de
// notas de cada cuaderno. Una sola consulta: el árbol de la barra lateral se pinta
// entero en cada carga y hacerlo por niveles serían N viajes para nada.
func (s *Store) ListNotebooks(ctx context.Context, sub string) ([]domain.Notebook, error) {
	t := tenancy.MustFrom(ctx)
	var out []domain.Notebook
	err := s.pool.InTenantTx(ctx, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, `
			SELECT `+nbCols+`,
			       (SELECT count(*) FROM notes n
			         WHERE n.notebook_id = nb.id AND n.deleted_at IS NULL
			           AND (n.visibility IS NULL OR n.visibility <> 'private' OR n.owner_sub = $2)),
			       NOT (nb.owner_sub = $2 OR nb.visibility = 'organization'
			            OR EXISTS (SELECT 1 FROM notebook_editores e WHERE e.notebook_id = nb.id AND e.sujeto = $2)),
			       EXISTS (SELECT 1 FROM notebook_peticiones_edicion p WHERE p.notebook_id = nb.id AND p.sujeto = $2)
			  FROM notebooks nb
			 WHERE nb.tenant = $1 AND nb.deleted_at IS NULL
			   AND (nb.owner_sub = $2 OR nb.visibility IN ('organization','public')
			        OR EXISTS (SELECT 1 FROM notebook_lectores l WHERE l.notebook_id = nb.id AND l.sujeto = $2)
			        OR EXISTS (SELECT 1 FROM notebook_editores e WHERE e.notebook_id = nb.id AND e.sujeto = $2))
			 ORDER BY nb.position, nb.created_at`, t.Slug, sub)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var nb domain.Notebook
			var crypto []byte
			var token string
			if err := rows.Scan(&nb.ID, &nb.OwnerSub, &nb.ParentID, &nb.Title, &nb.Emoji,
				&nb.Color, &nb.Layout, &nb.Visibility, &nb.Indexable, &nb.Encrypted, &crypto,
				&nb.RoomID, &token, &nb.Position, &nb.CreatedAt, &nb.UpdatedAt, &nb.DailyDate, &nb.ArchivedAt, &nb.NoteCount,
				&nb.ReadOnly, &nb.EditRequested); err != nil {
				return err
			}
			nb.Tenant = t.Slug
			if nb.Encrypted && len(crypto) > 0 {
				var cp domain.CryptoParams
				if err := json.Unmarshal(crypto, &cp); err == nil && cp.V != 0 {
					nb.Crypto = &cp
				}
			}
			if nb.OwnerSub == sub {
				nb.PublicToken = token
			}
			out = append(out, nb)
		}
		return rows.Err()
	})
	sinPadreAjeno(out)
	return out, err
}

// sinPadreAjeno cuelga de la raíz los cuadernos cuyo padre no ve quien pregunta: uno
// compartido con él (0010) dentro del árbol de otra persona, o uno de la organización
// dentro de uno personal. Los clientes arman el árbol desde la raíz, y con el padre
// puesto no saldrían en ningún sitio; tampoco hay por qué decir cuál es.
func sinPadreAjeno(nbs []domain.Notebook) {
	vistos := make(map[uuid.UUID]bool, len(nbs))
	for _, nb := range nbs {
		vistos[nb.ID] = true
	}
	for i := range nbs {
		if p := nbs[i].ParentID; p != nil && !vistos[*p] {
			nbs[i].ParentID = nil
		}
	}
}

// GetNotebook devuelve un cuaderno si `sub` puede verlo.
func (s *Store) GetNotebook(ctx context.Context, id uuid.UUID, sub string) (domain.Notebook, error) {
	t := tenancy.MustFrom(ctx)
	var nb domain.Notebook
	err := s.pool.InTenantTx(ctx, func(tx pgx.Tx) error {
		row := tx.QueryRow(ctx, `
			SELECT `+nbCols+`
			  FROM notebooks nb
			 WHERE nb.tenant = $1 AND nb.id = $2 AND nb.deleted_at IS NULL
			   AND (nb.owner_sub = $3 OR nb.visibility IN ('organization','public')
			        OR EXISTS (SELECT 1 FROM notebook_lectores l WHERE l.notebook_id = nb.id AND l.sujeto = $3)
			        OR EXISTS (SELECT 1 FROM notebook_editores e WHERE e.notebook_id = nb.id AND e.sujeto = $3))`,
			t.Slug, id, sub)
		got, err := scanNotebook(row, t.Slug, sub)
		if isNoRows(err) {
			return domain.ErrNotFound
		}
		nb = got
		return err
	})
	return nb, err
}

// GetNotebookByPublicToken resuelve un cuaderno por el secreto de su enlace
// público. NO exige sesión: es la puerta de las notas públicas.
func (s *Store) GetNotebookByPublicToken(ctx context.Context, token string) (domain.Notebook, error) {
	var nb domain.Notebook
	// Sin tenant en el contexto (nadie ha iniciado sesión): la consulta va por el
	// token, que es único en toda la instalación y de 32 bytes aleatorios.
	//
	// `app.public_link='on'` es lo que la hace posible: con RLS forzada y sin
	// organización fijada, la política de aislamiento no devuelve NI UNA fila —ni
	// siquiera la del token correcto—. La política que abre esta puerta (migración
	// 0003) solo ve cuadernos publicados, con token y vivos, así que la credencial
	// sigue siendo el token.
	err := s.pool.InTx(ctx, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, "SELECT set_config('app.public_link', 'on', true)"); err != nil {
			return fmt.Errorf("notes: abrir la lectura del enlace público: %w", err)
		}
		row := tx.QueryRow(ctx, `
			SELECT nb.tenant, `+nbCols+`
			  FROM notebooks nb
			 WHERE nb.public_token = $1 AND nb.visibility = 'public' AND nb.deleted_at IS NULL`,
			token)
		var slug string
		var crypto []byte
		var tok string
		if err := row.Scan(&slug, &nb.ID, &nb.OwnerSub, &nb.ParentID, &nb.Title, &nb.Emoji,
			&nb.Color, &nb.Layout, &nb.Visibility, &nb.Indexable, &nb.Encrypted, &crypto,
			&nb.RoomID, &tok, &nb.Position, &nb.CreatedAt, &nb.UpdatedAt, &nb.DailyDate, &nb.ArchivedAt); err != nil {
			if isNoRows(err) {
				return domain.ErrNotFound
			}
			return err
		}
		nb.Tenant = slug
		return nil
	})
	return nb, err
}

// CreateNotebook inserta un cuaderno del que `sub` es dueño.
func (s *Store) CreateNotebook(ctx context.Context, sub string, in domain.NotebookInput) (domain.Notebook, error) {
	t := tenancy.MustFrom(ctx)
	crypto := []byte(`{}`)
	if in.Crypto != nil {
		b, err := json.Marshal(in.Crypto)
		if err != nil {
			return domain.Notebook{}, domain.ErrInvalidInput
		}
		crypto = b
	}
	token := ""
	if in.Visibility == domain.VisPublic {
		tk, err := nuevoToken()
		if err != nil {
			return domain.Notebook{}, err
		}
		token = tk
	}
	var nb domain.Notebook
	err := s.pool.InTenantTx(ctx, func(tx pgx.Tx) error {
		// El padre tiene que existir y ser escribible por quien crea; sin esta
		// comprobación se podría colgar un cuaderno del árbol de otra persona.
		if in.ParentID != nil {
			var ok bool
			if err := tx.QueryRow(ctx, `
				SELECT true FROM notebooks nb
				 WHERE nb.tenant=$1 AND nb.id=$2 AND nb.deleted_at IS NULL
				   AND (nb.owner_sub=$3 OR nb.visibility='organization')`,
				t.Slug, *in.ParentID, sub).Scan(&ok); err != nil {
				if isNoRows(err) {
					return domain.ErrForbidden
				}
				return err
			}
		}
		row := tx.QueryRow(ctx, `
			INSERT INTO notebooks (tenant, owner_sub, parent_id, title, emoji, color, layout,
			                       visibility, indexable, encrypted, crypto, room_id, public_token, position)
			VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,
			        COALESCE($14, (SELECT COALESCE(max(position),0)+1 FROM notebooks
			                        WHERE tenant=$1 AND owner_sub=$2 AND deleted_at IS NULL)))
			RETURNING `+nbColsBase,
			t.Slug, sub, in.ParentID, in.Title, in.Emoji, in.Color, in.Layout,
			in.Visibility, *in.Indexable, in.Encrypted, crypto, in.RoomID, token, in.Position)
		var e error
		nb, e = scanNotebook(row, t.Slug, sub)
		if checkViolation(e) {
			return domain.ErrInvalidInput
		}
		return e
	})
	return nb, err
}

// UpdateNotebook actualiza un cuaderno que `sub` pueda escribir, sin administrar la
// organización. Ver ActualizarCuaderno.
func (s *Store) UpdateNotebook(ctx context.Context, id uuid.UUID, sub string, in domain.NotebookInput) (domain.Notebook, error) {
	return s.ActualizarCuaderno(ctx, id, sub, false, in)
}

// ActualizarCuaderno actualiza un cuaderno que `sub` pueda escribir.
//
// MOVERLO, PUBLICARLO, ESCONDERLO Y CIFRARLO SON DE SU DUEÑO —y, en un cuaderno de
// organización, de quien administra la organización (`administra`)—, igual que
// borrarlo. Que un cuaderno de organización se escriba entre varios es escribir sus
// notas y su rótulo, no decidir quién lo ve: con el permiso de escribir bastaba para
// publicarlo hacia fuera, quitárselo a todos o colgarlo de un cuaderno propio y
// llevárselo al borrar ese. Quien no puede y lo intenta recibe ErrSoloDueno.
//
// Cambiar la visibilidad a pública ACUÑA el token si aún no lo tiene, y dejar de
// serlo lo borra: un enlace que sigue vivo tras despublicar es la fuga clásica.
func (s *Store) ActualizarCuaderno(ctx context.Context, id uuid.UUID, sub string, administra bool, in domain.NotebookInput) (domain.Notebook, error) {
	t := tenancy.MustFrom(ctx)
	crypto := []byte(`{}`)
	if in.Crypto != nil {
		b, err := json.Marshal(in.Crypto)
		if err != nil {
			return domain.Notebook{}, domain.ErrInvalidInput
		}
		crypto = b
	}
	nuevo := ""
	if in.Visibility == domain.VisPublic {
		tk, err := nuevoToken()
		if err != nil {
			return domain.Notebook{}, err
		}
		nuevo = tk
	}
	var nb domain.Notebook
	err := s.pool.InTenantTx(ctx, func(tx pgx.Tx) error {
		visAntes, err := ajustesAdmisibles(ctx, tx, t.Slug, id, sub, administra, in)
		if err != nil {
			return err
		}

		row := tx.QueryRow(ctx, `
			UPDATE notebooks nb
			   SET parent_id = $4, title = $5, emoji = $6, color = $7, layout = $8,
			       visibility = $9, indexable = $10, encrypted = $11, crypto = $12,
			       room_id = $13, position = COALESCE($14, nb.position),
			       public_token = CASE WHEN $9 = 'public'
			                           THEN CASE WHEN nb.public_token <> '' THEN nb.public_token ELSE $15 END
			                           ELSE '' END,
			       updated_at = now()
			 WHERE nb.tenant = $1 AND nb.id = $2 AND nb.deleted_at IS NULL
			   AND (nb.owner_sub = $3 OR nb.visibility = 'organization')
			RETURNING `+nbColsBase,
			t.Slug, id, sub, in.ParentID, in.Title, in.Emoji, in.Color, in.Layout,
			in.Visibility, *in.Indexable, in.Encrypted, crypto, in.RoomID, in.Position, nuevo)
		got, e := scanNotebook(row, t.Slug, sub)
		if isNoRows(e) {
			return domain.ErrNotFound
		}
		if checkViolation(e) {
			return domain.ErrInvalidInput
		}
		nb = got
		if e != nil {
			return e
		}

		// SOLO SI CAMBIA LA EXPOSICIÓN. Renombrar un cuaderno público no vuelve a
		// publicarlo, y anotarlo llenaría la traza de líneas que no son lo que
		// alguien busca cuando la abre.
		switch {
		case visAntes != domain.VisPublic && nb.Visibility == domain.VisPublic:
			return writeAuditTx(ctx, tx, t.Slug, sub, domain.AuditNotebookPublish, id.String(),
				map[string]any{"titulo": nb.Title, "visibilidad_anterior": string(visAntes)})
		case visAntes == domain.VisPublic && nb.Visibility != domain.VisPublic:
			return writeAuditTx(ctx, tx, t.Slug, sub, domain.AuditNotebookUnpublish, id.String(),
				map[string]any{"titulo": nb.Title, "visibilidad_nueva": string(nb.Visibility)})
		}
		return nil
	})
	return nb, err
}

// ajustesAdmisibles lee CÓMO ESTABA el cuaderno, con la fila bloqueada, y dice si `sub`
// puede dejarlo como pide `in`. Devuelve la visibilidad de antes, que es lo que
// distingue publicar de despublicar y de un cambio que no toca la exposición: el
// `UPDATE ... RETURNING` solo devuelve la nueva y las tres cosas se parecen. Busca por
// clave primaria: cuesta lo que cuesta un índice.
func ajustesAdmisibles(ctx context.Context, tx pgx.Tx, tenant string, id uuid.UUID, sub string, administra bool, in domain.NotebookInput) (domain.Visibility, error) {
	var (
		visAntes    domain.Visibility
		dueno       string
		padreAntes  *uuid.UUID
		cifAntes    bool
		cryptoAntes []byte
	)
	err := tx.QueryRow(ctx, `
		SELECT visibility, owner_sub, parent_id, encrypted, crypto FROM notebooks
		 WHERE tenant = $1 AND id = $2 AND deleted_at IS NULL
		   FOR UPDATE`,
		tenant, id).Scan(&visAntes, &dueno, &padreAntes, &cifAntes, &cryptoAntes)
	if isNoRows(err) {
		return "", domain.ErrNotFound
	}
	if err != nil {
		return "", err
	}
	mueve := !mismoID(in.ParentID, padreAntes)
	if dueno != sub && (!administra || visAntes != domain.VisOrganization) {
		// Ni suyo ni de la organización: para quien pregunta no existe, y el UPDATE
		// diría lo mismo.
		if visAntes != domain.VisOrganization {
			return "", domain.ErrNotFound
		}
		var antes, ahora domain.CryptoParams
		_ = json.Unmarshal(cryptoAntes, &antes)
		if in.Crypto != nil {
			ahora = *in.Crypto
		}
		if mueve || in.Visibility != visAntes || in.Encrypted != cifAntes || ahora != antes {
			return "", domain.ErrSoloDueno
		}
	}
	if mueve && in.ParentID != nil {
		return visAntes, padreAdmisible(ctx, tx, tenant, id, *in.ParentID, sub)
	}
	return visAntes, nil
}

func mismoID(a, b *uuid.UUID) bool {
	return (a == nil) == (b == nil) && (a == nil || *a == *b)
}

// padreAdmisible dice si `id` se puede colgar de `padre`: el padre existe, lo puede
// escribir quien mueve (la misma regla que al crear) y no es el propio cuaderno ni
// cuelga de él. Colgarlo de un descendiente cierra un ciclo, y las consultas que bajan
// por el árbol (borrar, archivar) no acabarían. Mismo recorrido y mismo tope de 64
// niveles que grupoAdmisible: con un ciclo ya existente la subida no encuentra `id`,
// así que llegar al tope cuenta como inadmisible.
func padreAdmisible(ctx context.Context, tx pgx.Tx, tenant string, id, padre uuid.UUID, sub string) error {
	var ok bool
	err := tx.QueryRow(ctx, `
		WITH RECURSIVE arriba AS (
			SELECT nb.id, nb.parent_id, 0 AS nivel
			  FROM notebooks nb
			 WHERE nb.tenant = $1 AND nb.id = $2 AND nb.deleted_at IS NULL
			   AND (nb.owner_sub = $4 OR nb.visibility = 'organization')
			UNION ALL
			SELECT p.id, p.parent_id, a.nivel + 1
			  FROM notebooks p JOIN arriba a ON p.id = a.parent_id
			 WHERE p.tenant = $1 AND a.nivel < 64
		)
		SELECT EXISTS (SELECT 1 FROM arriba WHERE nivel = 0)
		   AND NOT EXISTS (SELECT 1 FROM arriba WHERE id = $3)
		   AND NOT EXISTS (SELECT 1 FROM arriba WHERE nivel = 64)`,
		tenant, padre, id, sub).Scan(&ok)
	if err != nil {
		return err
	}
	if !ok {
		return domain.ErrInvalidInput
	}
	return nil
}

// DeleteNotebook lo manda a la papelera (borrado suave), con sus descendientes.
func (s *Store) DeleteNotebook(ctx context.Context, id uuid.UUID, sub string) error {
	t := tenancy.MustFrom(ctx)
	return s.pool.InTenantTx(ctx, func(tx pgx.Tx) error {
		// Solo el DUEÑO borra un cuaderno entero. Que un cuaderno de organización se
		// escriba entre varios no significa que cualquiera pueda hacerlo desaparecer.
		//
		// UNION y no UNION ALL: descarta lo ya visto, así que un ciclo de `parent_id`
		// guardado antes de padreAdmisible se recorre una vez y la consulta acaba.
		tag, err := tx.Exec(ctx, `
			WITH RECURSIVE rama AS (
			    SELECT id FROM notebooks
			     WHERE tenant=$1 AND id=$2 AND owner_sub=$3 AND deleted_at IS NULL
			    UNION
			    SELECT h.id FROM notebooks h JOIN rama r ON h.parent_id = r.id
			     WHERE h.tenant=$1 AND h.deleted_at IS NULL
			)
			UPDATE notebooks SET deleted_at = now()
			 WHERE tenant=$1 AND id IN (SELECT id FROM rama)`, t.Slug, id, sub)
		if err != nil {
			return err
		}
		if tag.RowsAffected() == 0 {
			return domain.ErrNotFound
		}
		// `ramas` y no «1»: el borrado arrastra los descendientes, y saber que se
		// fueron siete cuadernos y no uno es la diferencia entre entender el
		// incidente y tener que reconstruirlo a mano.
		return writeAuditTx(ctx, tx, t.Slug, sub, domain.AuditNotebookDelete, id.String(),
			map[string]any{"ramas": tag.RowsAffected()})
	})
}

// PublicNotebook resuelve el cuaderno de un enlace público y sus notas, SIN
// sesión. Se sirve entero y de una vez: es una página de lectura, no una app.
func (s *Store) PublicNotebook(ctx context.Context, token string) (domain.Notebook, []domain.Note, error) {
	nb, err := s.GetNotebookByPublicToken(ctx, token)
	if err != nil {
		return domain.Notebook{}, nil, err
	}
	var notes []domain.Note
	err = s.pool.InTx(ctx, func(tx pgx.Tx) error {
		if err := pg.SetTenant(ctx, tx, nb.Tenant); err != nil {
			return err
		}
		rows, err := tx.Query(ctx, `
			SELECT `+noteCols+`
			  FROM notes n
			 WHERE n.tenant = $1 AND n.notebook_id = $2 AND n.deleted_at IS NULL
			   -- Una nota marcada privada NO sale por el enlace público aunque el
			   -- cuaderno lo sea: la excepción de la nota manda sobre el contenedor.
			   AND (n.visibility IS NULL OR n.visibility <> 'private')
			 ORDER BY n.pinned DESC, n.col, n.position, n.created_at
			 LIMIT 500`, nb.Tenant, nb.ID)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			n, err := scanNote(rows, nb.Tenant)
			if err != nil {
				return err
			}
			notes = append(notes, n)
		}
		if err := rows.Err(); err != nil {
			return err
		}
		return s.adjuntarEnPaquete(ctx, tx, nb.Tenant, notes)
	})
	if notes == nil {
		notes = []domain.Note{}
	}
	// El dueño no se publica: es un dato personal que no aporta nada a quien lee.
	nb.OwnerSub = ""
	return nb, notes, err
}
