// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package store

import (
	"context"
	"encoding/json"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"git.sytue.eu/sytue/one/platform/pkg/tenancy"
	"git.sytue.eu/sytue/one/services/notes/internal/domain"
)

// Las columnas de una nota, en el orden que espera `scanNote`.
//
// MISMO PAR QUE EN `notebooks.go`, y por la misma razón: los `SELECT` van sobre
// `FROM notes n` y necesitan el alias, pero un `INSERT … RETURNING` no tiene
// tabla que aliasar y Postgres contesta «missing FROM-clause entry for table
// "n"». Se usaba la aliasada en los dos sitios, así que CREAR y ACTUALIZAR una
// nota fallaban siempre contra una base de datos real.
//
// Este es el segundo de los dos: al arreglar el de los cuadernos no se miró el
// fichero hermano, y el rojo volvió con otro nombre de tabla. Lo cazaron otra vez
// los tests de integración, porque los de unidad usan un almacén de mentira que
// se traga cualquier SQL.
const noteColsBase = "id, notebook_id, owner_sub, kind, title, content, content_cipher, " +
	"data, x, y, col, position, width, group_id, collapsed, checked, pinned, " +
	"visibility, indexable, checked_at, cancelled_at, created_at, updated_at"

// La misma lista con el alias delante. En UNA línea la base para que este
// reemplazo sea evidente: no hay saltos ni tabuladores que esquivar.
var noteCols = "n." + strings.ReplaceAll(noteColsBase, ", ", ", n.")

// `extra` son destinos para las columnas que la consulta ponga DETRÁS de `noteCols`.
func scanNote(row pgx.Row, tenantSlug string, extra ...any) (domain.Note, error) {
	var n domain.Note
	var vis *string
	var data []byte
	dest := append([]any{
		&n.ID, &n.NotebookID, &n.OwnerSub, &n.Kind, &n.Title, &n.Content,
		&n.ContentCipher, &data, &n.X, &n.Y, &n.Col, &n.Position, &n.Width, &n.GroupID,
		&n.Collapsed, &n.Checked, &n.Pinned, &vis, &n.Indexable,
		&n.CheckedAt, &n.CancelledAt, &n.CreatedAt, &n.UpdatedAt,
	}, extra...)
	if err := row.Scan(dest...); err != nil {
		return domain.Note{}, err
	}
	n.Tenant = tenantSlug
	n.Data = json.RawMessage(data)
	if vis != nil {
		n.Visibility = domain.Visibility(*vis)
	}
	return n, nil
}

// visOrNil convierte la visibilidad de un input a lo que va a la columna: vacía
// significa «hereda del cuaderno», y eso en SQL es NULL, no cadena vacía.
func visOrNil(v domain.Visibility) *string {
	if v == "" {
		return nil
	}
	s := string(v)
	return &s
}

const noteCanvasOrder = "n.pinned DESC, n.col, n.position, n.created_at, n.id"

// ListNotes devuelve las notas vivas de un cuaderno visible para `sub`, con sus
// estados de etiqueta y sus enlaces ya adjuntos.
func (s *Store) ListNotes(ctx context.Context, notebookID uuid.UUID, sub string) ([]domain.Note, error) {
	return s.listNotes(ctx, notebookID, sub, nil, 0, noteCanvasOrder)
}

func (s *Store) listNotes(ctx context.Context, notebookID uuid.UUID, sub string, after *uuid.UUID, fetch int, order string) ([]domain.Note, error) {
	_ = tenancy.MustFrom(ctx)
	var out []domain.Note
	err := s.pool.InTenantTx(ctx, func(tx pgx.Tx) error {
		var err error
		out, err = s.listNotesPageTx(ctx, tx, notebookID, sub, after, fetch, order)
		return err
	})
	return out, err
}

// listNotesTx reads the same visible, ordered notes and related records in an
// existing tenant transaction. The caller owns transaction scope and isolation.
func (s *Store) listNotesTx(ctx context.Context, tx pgx.Tx, notebookID uuid.UUID, sub string) ([]domain.Note, error) {
	return s.listNotesPageTx(ctx, tx, notebookID, sub, nil, 0, noteCanvasOrder)
}

func (s *Store) listNotesPageTx(ctx context.Context, tx pgx.Tx, notebookID uuid.UUID, sub string, after *uuid.UUID, fetch int, order string) ([]domain.Note, error) {
	t := tenancy.MustFrom(ctx)
	var out []domain.Note
	rows, err := tx.Query(ctx, `
		SELECT `+noteCols+`
		  FROM notes n
		  JOIN notebooks nb ON nb.id = n.notebook_id AND nb.deleted_at IS NULL
		 WHERE n.tenant = $1 AND n.notebook_id = $2 AND n.deleted_at IS NULL
		   AND `+legiblePorCuaderno("$3")+`
		 AND ($4::uuid IS NULL OR n.id > $4)
		 ORDER BY `+order+` LIMIT NULLIF($5,0)`, t.Slug, notebookID, sub, after, fetch)
	if err != nil {
		return out, err
	}
	defer rows.Close()
	for rows.Next() {
		n, err := scanNote(rows, t.Slug)
		if err != nil {
			return out, err
		}
		out = append(out, n)
	}
	if err := rows.Err(); err != nil {
		return out, err
	}
	return out, s.adjuntarEnPaquete(ctx, tx, t.Slug, out)
}

// adjuntarEnPaquete rellena estados y enlaces de TODAS las notas con dos consultas
// en total. Hacerlo nota a nota son 2N viajes para pintar un lienzo, que es el N+1
// clásico y se nota justo cuando el cuaderno crece.
func (s *Store) adjuntarEnPaquete(ctx context.Context, tx pgx.Tx, slug string, notas []domain.Note) error {
	if len(notas) == 0 {
		return nil
	}
	ids := make([]uuid.UUID, 0, len(notas))
	porID := make(map[uuid.UUID]*domain.Note, len(notas))
	for i := range notas {
		ids = append(ids, notas[i].ID)
		porID[notas[i].ID] = &notas[i]
	}

	rows, err := tx.Query(ctx, `
		SELECT nts.note_id, t.id, t.name, ts.id, ts.name, ts.emoji, ts.color
		  FROM note_tag_states nts
		  JOIN tags t ON t.id = nts.tag_id
		  JOIN tag_states ts ON ts.id = nts.tag_state_id
		 WHERE nts.tenant = $1 AND nts.note_id = ANY($2)
		 ORDER BY t.position, ts.position`, slug, ids)
	if err != nil {
		return err
	}
	for rows.Next() {
		var noteID uuid.UUID
		var st domain.NoteState
		if err := rows.Scan(&noteID, &st.TagID, &st.TagName, &st.StateID, &st.Name, &st.Emoji, &st.Color); err != nil {
			rows.Close()
			return err
		}
		if n := porID[noteID]; n != nil {
			n.States = append(n.States, st)
		}
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return err
	}

	lrows, err := tx.Query(ctx, `
		SELECT note_id, target_kind, target_id, label, url
		  FROM note_links WHERE tenant = $1 AND note_id = ANY($2) ORDER BY created_at`, slug, ids)
	if err != nil {
		return err
	}
	defer lrows.Close()
	for lrows.Next() {
		var noteID uuid.UUID
		var l domain.NoteLink
		if err := lrows.Scan(&noteID, &l.TargetKind, &l.TargetID, &l.Label, &l.URL); err != nil {
			return err
		}
		if n := porID[noteID]; n != nil {
			n.Links = append(n.Links, l)
		}
	}
	return lrows.Err()
}

// CreateNote inserta una nota en un cuaderno que `sub` pueda escribir.
func (s *Store) CreateNote(ctx context.Context, notebookID uuid.UUID, sub string, in domain.NoteInput) (domain.Note, error) {
	t := tenancy.MustFrom(ctx)
	var n domain.Note
	err := s.inWriteTx(ctx, func(tx pgx.Tx) error {
		if in.GroupID != nil {
			if err := lockNoteTree(ctx, tx, t.Slug); err != nil {
				return err
			}
		}
		if err := normalizeNewNoteWrite(ctx, tx, t.Slug, notebookID, sub, &in); err != nil {
			return err
		}
		if err := grupoAdmisible(ctx, tx, t.Slug, uuid.Nil, &notebookID, in.GroupID); err != nil {
			return err
		}
		if got, found, err := existingCapture(ctx, tx, t.Slug, notebookID, sub, in); err != nil {
			return err
		} else if found {
			n = got
			return nil
		}
		row := tx.QueryRow(ctx, `
			INSERT INTO notes (tenant, notebook_id, owner_sub, kind, title, content, content_cipher,
			                   data, x, y, col, position, width, group_id, collapsed, checked, pinned,
			                   visibility, indexable, checked_at, cancelled_at)
			SELECT $1, nb.id, $3, COALESCE(NULLIF($4, ''), 'text'), $5, $6, $7,
			       COALESCE($8::jsonb, '{}'::jsonb),
			       COALESCE($9,0), COALESCE($10,0), COALESCE($11,0),
			       COALESCE($12, (SELECT COALESCE(max(position),0)+1 FROM notes
			                       WHERE tenant=$1 AND notebook_id=nb.id AND col=COALESCE($11,0)
			                         AND deleted_at IS NULL)),
			       COALESCE($13,260), $14, COALESCE($15,false), COALESCE($16,false), COALESCE($17,false),
			       $18, $19,
			       CASE WHEN COALESCE($16, false) THEN now() END,
			       CASE WHEN COALESCE($20, false) THEN now() END
			  FROM notebooks nb
			 WHERE nb.tenant=$1 AND nb.id=$2 AND nb.deleted_at IS NULL
			   AND (nb.owner_sub = $3 OR nb.visibility = 'organization' OR EXISTS (SELECT 1 FROM notebook_editores e WHERE e.notebook_id = nb.id AND e.sujeto = $3))
			RETURNING `+noteColsBase,
			t.Slug, notebookID, sub, in.Kind, in.Title, in.Content, in.ContentCipher, []byte(in.Data),
			in.X, in.Y, in.Col, in.Position, in.Width, in.GroupID, in.Collapsed, in.Checked, in.Pinned,
			visOrNil(in.Visibility), in.Indexable, in.Cancelled)
		got, e := scanNote(row, t.Slug)
		if isNoRows(e) {
			// El INSERT ... SELECT no devuelve fila cuando el cuaderno no existe o no
			// es escribible: los dos casos se contestan igual para no revelar cuáles
			// existen.
			return domain.ErrNotFound
		}
		if checkViolation(e) {
			return domain.ErrInvalidInput
		}
		if e != nil {
			return e
		}
		if err := reemplazarEnlaces(ctx, tx, t.Slug, got.ID, in.Links); err != nil {
			return err
		}
		got.Links = in.Links
		n = got
		return nil
	})
	return n, err
}

// EscrituraDeNota es contra lo que se valida lo que llega para guardar una nota que
// `sub` puede escribir: si su cuaderno está cifrado, su tipo ACTUAL y si la escribe por
// el cuaderno o solo porque se la compartieron (0010). Se lee de la NOTA y no del
// cuaderno que diga el cliente, que podría nombrar otro. ErrNotFound si no la escribe,
// para que un 400 de validación no delate notas ajenas.
//
// El tipo importa porque el saneado del HTML depende de él: quien la tiene solo por
// compartida no cambia el tipo (UpdateNote lo conserva), así que su contenido se valida
// con el que la nota TIENE y no con el que diga el cuerpo —con `link` se saltaba el
// saneado y el HTML acababa en una nota de texto—.
func (s *Store) EscrituraDeNota(ctx context.Context, id uuid.UUID, sub string) (cifrado bool, tipo domain.Kind, porCuaderno bool, err error) {
	t := tenancy.MustFrom(ctx)
	err = s.pool.InTenantTx(ctx, func(tx pgx.Tx) error {
		e := tx.QueryRow(ctx, `
			SELECT nb.encrypted, n.kind,
			       (nb.owner_sub = $3 OR nb.visibility = 'organization' OR EXISTS (SELECT 1 FROM notebook_editores e WHERE e.notebook_id = nb.id AND e.sujeto = $3))
			       AND (n.visibility IS NULL OR n.visibility <> 'private' OR n.owner_sub = $3)
			  FROM notes n
			  JOIN notebooks nb ON nb.id = n.notebook_id AND nb.deleted_at IS NULL
			 WHERE n.tenant = $1 AND n.id = $2 AND n.deleted_at IS NULL
			   AND (((nb.owner_sub = $3 OR nb.visibility = 'organization' OR EXISTS (SELECT 1 FROM notebook_editores e WHERE e.notebook_id = nb.id AND e.sujeto = $3))
			         AND (n.visibility IS NULL OR n.visibility <> 'private' OR n.owner_sub = $3))
			        OR (EXISTS (SELECT 1 FROM note_personas c WHERE c.note_id = n.id AND c.sujeto = $3 AND c.puede_editar)
			             AND (nb.owner_sub = n.owner_sub OR nb.visibility = 'organization' OR EXISTS (SELECT 1 FROM notebook_editores e WHERE e.notebook_id = nb.id AND e.sujeto = n.owner_sub))))`,
			t.Slug, id, sub).Scan(&cifrado, &tipo, &porCuaderno)
		if isNoRows(e) {
			return domain.ErrNotFound
		}
		return e
	})
	return cifrado, tipo, porCuaderno, err
}

// UpdateNote actualiza una nota. Los punteros nil significan «no lo toques», que
// es lo que permite mover una nota por el lienzo sin reenviar su contenido.
func (s *Store) UpdateNote(ctx context.Context, id uuid.UUID, sub string, in domain.NoteInput) (domain.Note, error) {
	n, _, err := s.ActualizarNota(ctx, id, sub, in)
	return n, err
}

// Antes es lo que la nota tenía ANTES de actualizarla, que es lo que hace falta para
// actuar solo en el PASO: de sin hacer a hecha (repetir la tarea), de no mencionar a
// alguien a mencionarlo (avisarle una vez, no en cada guardado).
type Antes struct {
	Checked           bool
	Titulo, Contenido string
}

// admiteEscritura son las comprobaciones de un guardado con la nota ya bloqueada: las del
// cifrado y, después, la de la versión esperada (conflicto entre dispositivos). Normaliza
// `in` si el cuaderno está cifrado.
func admiteEscritura(state noteWriteState, in *domain.NoteInput) error {
	if state.encrypted || in.ContentCipher != "" {
		if err := in.Normalize(state.encrypted); err != nil {
			return err
		}
	}
	if state.encrypted && !state.cryptoMatches {
		return domain.ErrNotebookCryptoChanged
	}
	if state.encrypted && (state.note.Title != "" || state.note.Content != "" || !domain.EncryptedPublicData(state.note.Data, true)) {
		return domain.ErrEncryptedMigrationRequired
	}
	// Después de las comprobaciones del cifrado (que piden otra cosa al cliente) y
	// con la fila bloqueada (FOR UPDATE): dos escrituras con la misma versión
	// esperada no pueden pasar las dos.
	if in.ChocaCon(state.note.UpdatedAt) {
		return domain.ErrNoteEditConflict
	}
	return nil
}

// ActualizarNota es UpdateNote diciendo además cómo estaba ANTES: repetir una tarea es
// cosa del paso de sin hacer a hecha, no de reenviar `checked: true` al cambiarle el
// texto. La fila se bloquea al leerla, así que dos guardados a la vez no ven los dos
// el paso.
func (s *Store) ActualizarNota(ctx context.Context, id uuid.UUID, sub string, in domain.NoteInput) (domain.Note, Antes, error) {
	t := tenancy.MustFrom(ctx)
	var n domain.Note
	var antes Antes
	err := s.inWriteTx(ctx, func(tx pgx.Tx) error {
		var err error
		n, antes, err = s.actualizarNotaTx(ctx, tx, t.Slug, id, sub, in)
		return err
	})
	return n, antes, err
}

func (s *Store) actualizarNotaTx(ctx context.Context, tx pgx.Tx, tenant string, id uuid.UUID, sub string, in domain.NoteInput) (domain.Note, Antes, error) {
	if in.GroupID != nil {
		if err := lockNoteTree(ctx, tx, tenant); err != nil {
			return domain.Note{}, Antes{}, err
		}
	}
	// POR DÓNDE ESCRIBE: por el cuaderno (dueño, organización o editora) o SOLO
	// porque le compartieron la nota con edición (0010). Quien llega por lo
	// segundo cambia su contenido y nada más: ni la visibilidad —abrir a todo el
	// cuaderno una nota privada que le enseñaron es justo lo que no se le dio—,
	// ni si se indexa, ni el grupo, que es colgarla de otra nota del cuaderno.
	state, err := lockNoteWrite(ctx, tx, tenant, id, sub, in.ExpectedCrypto)
	if err != nil {
		return domain.Note{}, Antes{}, err
	}
	porCuaderno := state.byNotebook
	antes := Antes{Checked: state.note.Checked, Titulo: state.note.Title, Contenido: state.note.Content}
	if err := admiteEscritura(state, &in); err != nil {
		return domain.Note{}, Antes{}, err
	}
	if !porCuaderno {
		restringirEntradaCompartida(&in, state.note)
	}
	if err := grupoAdmisible(ctx, tx, tenant, id, nil, in.GroupID); err != nil {
		return domain.Note{}, Antes{}, err
	}
	// Cómo estaba antes, al historial (versiones.go). Con la fila ya bloqueada y
	// después de admitir la escritura: un guardado rechazado no deja versión.
	if err := guardarVersion(ctx, tx, tenant, sub, state.note, in); err != nil {
		return domain.Note{}, Antes{}, err
	}
	got, err := ejecutarUpdateNota(ctx, tx, tenant, id, sub, in)
	if err != nil {
		return domain.Note{}, Antes{}, err
	}
	if !porCuaderno {
		una := []domain.Note{got}
		err := s.adjuntarEnPaquete(ctx, tx, tenant, una)
		return una[0], antes, err
	}
	if err := reemplazarEnlaces(ctx, tx, tenant, got.ID, in.Links); err != nil {
		return domain.Note{}, Antes{}, err
	}
	got.Links = in.Links
	return got, antes, nil
}

func restringirEntradaCompartida(in *domain.NoteInput, note domain.Note) {
	in.Visibility, in.Indexable, in.GroupID = note.Visibility, note.Indexable, nil
	// Ni el tipo ni el sitio en el lienzo: son de quien ordena el cuaderno. Ni sus
	// datos ni sus enlaces, que la cuelgan de otras cosas de la suite y la sacan en
	// sus retroenlaces a quien lee el cuaderno.
	in.Kind, in.Data = "", nil
	in.X, in.Y, in.Col, in.Position, in.Width = nil, nil, nil, nil, nil
	in.Collapsed, in.Pinned = nil, nil
}

func ejecutarUpdateNota(ctx context.Context, tx pgx.Tx, tenant string, id uuid.UUID, sub string, in domain.NoteInput) (domain.Note, error) {
	row := tx.QueryRow(ctx, `
		UPDATE notes n
		   SET kind = COALESCE(NULLIF($4, ''), n.kind), title = $5, content = $6, content_cipher = $7,
		       -- Task controls are authored by the service and survive content writes.
		       data = CASE WHEN $8::jsonb IS NULL THEN n.data
		                   WHEN jsonb_typeof($8::jsonb) = 'object'
		                   THEN $8::jsonb || `+taskControls+`
		                   ELSE $8::jsonb END,
		       x = COALESCE($9, n.x), y = COALESCE($10, n.y), col = COALESCE($11, n.col),
		       position = COALESCE($12, n.position), width = COALESCE($13, n.width),
		       group_id = COALESCE($14, n.group_id), collapsed = COALESCE($15, n.collapsed),
		       checked = COALESCE($16, n.checked), pinned = COALESCE($17, n.pinned),
		       -- La fecha se guarda al marcar y se conserva al reeditar; desmarcar la borra.
		       checked_at = CASE WHEN COALESCE($16, n.checked) THEN COALESCE(n.checked_at, now()) END,
		       cancelled_at = CASE WHEN $20::boolean IS NULL THEN n.cancelled_at
		                           WHEN $20 THEN COALESCE(n.cancelled_at, now()) END,
		       visibility = $18, indexable = $19, updated_at = now()
		  FROM notebooks nb
		 WHERE n.tenant = $1 AND n.id = $2 AND n.deleted_at IS NULL
		   AND nb.id = n.notebook_id AND nb.deleted_at IS NULL
		   AND (((nb.owner_sub = $3 OR nb.visibility = 'organization' OR EXISTS (SELECT 1 FROM notebook_editores e WHERE e.notebook_id = nb.id AND e.sujeto = $3))
		         AND (n.visibility IS NULL OR n.visibility <> 'private' OR n.owner_sub = $3))
		        OR (EXISTS (SELECT 1 FROM note_personas c WHERE c.note_id = n.id AND c.sujeto = $3 AND c.puede_editar)
		             AND (nb.owner_sub = n.owner_sub OR nb.visibility = 'organization' OR EXISTS (SELECT 1 FROM notebook_editores e WHERE e.notebook_id = nb.id AND e.sujeto = n.owner_sub))))
		RETURNING `+noteCols, // con alias: hay un FROM notebooks y `id`, `title`… existen en las dos tablas
		tenant, id, sub, in.Kind, in.Title, in.Content, in.ContentCipher, []byte(in.Data),
		in.X, in.Y, in.Col, in.Position, in.Width, in.GroupID, in.Collapsed, in.Checked, in.Pinned,
		visOrNil(in.Visibility), in.Indexable, in.Cancelled)
	got, err := scanNote(row, tenant)
	if isNoRows(err) {
		return domain.Note{}, domain.ErrNotFound
	}
	if checkViolation(err) {
		return domain.Note{}, domain.ErrInvalidInput
	}
	return got, err
}

// MoveNote reubica una nota (lienzo libre o columnas) sin tocar su contenido. Es
// la escritura del arrastrar y soltar, que ocurre muchas veces por minuto.
func (s *Store) MoveNote(ctx context.Context, id uuid.UUID, sub string, x, y, col, position int, groupID *uuid.UUID) error {
	t := tenancy.MustFrom(ctx)
	return s.inWriteTx(ctx, func(tx pgx.Tx) error {
		if err := lockNoteTree(ctx, tx, t.Slug); err != nil {
			return err
		}
		if err := grupoAdmisible(ctx, tx, t.Slug, id, nil, groupID); err != nil {
			return err
		}
		tag, err := tx.Exec(ctx, `
			UPDATE notes n
			   SET x=$4, y=$5, col=$6, position=$7, group_id=$8, updated_at=now()
			  FROM notebooks nb
			 WHERE n.tenant=$1 AND n.id=$2 AND n.deleted_at IS NULL
			   AND nb.id = n.notebook_id AND nb.deleted_at IS NULL
			   AND (nb.owner_sub = $3 OR nb.visibility = 'organization' OR EXISTS (SELECT 1 FROM notebook_editores e WHERE e.notebook_id = nb.id AND e.sujeto = $3))`,
			t.Slug, id, sub, x, y, col, position, groupID)
		if err != nil {
			return err
		}
		if tag.RowsAffected() == 0 {
			return domain.ErrNotFound
		}
		return nil
	})
}

// MoveNoteToNotebook cambia una nota de cuaderno, con todo lo que cuelga de ella
// (sus notas agrupadas, en la papelera o no: la familia va junta). Exige poder
// escribir en los DOS cuadernos —dueño u organización— y que ninguno esté
// cifrado: la clave de un cuaderno cifrado vive solo en el navegador y el
// servidor no puede recifrar. La nota llega al final de la primera columna del
// destino y fuera de cualquier grupo, porque los grupos son por cuaderno.
func (s *Store) MoveNoteToNotebook(ctx context.Context, id uuid.UUID, sub string, destino uuid.UUID) error {
	t := tenancy.MustFrom(ctx)
	return s.inWriteTx(ctx, func(tx pgx.Tx) error {
		return moveNoteToNotebookTx(ctx, tx, t.Slug, id, sub, destino)
	})
}

func moveNoteToNotebookTx(ctx context.Context, tx pgx.Tx, tenant string, id uuid.UUID, sub string, destino uuid.UUID) error {
	if err := lockNoteTree(ctx, tx, tenant); err != nil {
		return err
	}
	origen, err := resolveMoveOrigin(ctx, tx, tenant, id, sub, destino)
	if err != nil {
		return err
	}
	if err := lockAndVerifyMoveNotebooks(ctx, tx, tenant, sub, origen, destino); err != nil {
		return err
	}
	if err := lockMoveRootNote(ctx, tx, tenant, id, origen); err != nil {
		return err
	}
	familia, err := lockMoveFamily(ctx, tx, tenant, id, origen)
	if err != nil {
		return err
	}
	return executeMoveFamily(ctx, tx, tenant, id, destino, origen, familia)
}

func resolveMoveOrigin(ctx context.Context, tx pgx.Tx, tenant string, id uuid.UUID, sub string, destino uuid.UUID) (uuid.UUID, error) {
	// Find the candidate origin, then lock both notebooks in a stable order.
	// The candidate is rechecked below after waiting: a concurrent move may
	// have changed it before these locks were acquired.
	var origen uuid.UUID
	err := tx.QueryRow(ctx, `SELECT n.notebook_id FROM notes n
		JOIN notebooks o ON o.id=n.notebook_id AND o.tenant=n.tenant AND o.deleted_at IS NULL
		JOIN notebooks d ON d.tenant=$1 AND d.id=$4 AND d.deleted_at IS NULL
		WHERE n.tenant=$1 AND n.id=$2 AND n.deleted_at IS NULL AND d.id<>o.id
		  AND (o.owner_sub=$3 OR o.visibility='organization' OR EXISTS
		       (SELECT 1 FROM notebook_editores e WHERE e.notebook_id=o.id AND e.sujeto=$3))
		  AND (d.owner_sub=$3 OR d.visibility='organization' OR EXISTS
		       (SELECT 1 FROM notebook_editores e WHERE e.notebook_id=d.id AND e.sujeto=$3))`,
		tenant, id, sub, destino).Scan(&origen)
	if isNoRows(err) {
		return uuid.Nil, domain.ErrNotFound
	}
	if err != nil {
		return uuid.Nil, err
	}
	if origen == destino {
		return uuid.Nil, domain.ErrNotFound
	}
	return origen, nil
}

type moveNotebookAccess struct {
	owner      string
	visibility domain.Visibility
	encrypted  bool
}

func lockAndVerifyMoveNotebooks(ctx context.Context, tx pgx.Tx, tenant, sub string, origen, destino uuid.UUID) error {
	locked, err := queryLockedMoveNotebooks(ctx, tx, tenant, origen, destino)
	if err != nil {
		return err
	}
	if len(locked) != 2 {
		return domain.ErrNotFound
	}
	editors, err := queryLockedMoveGrants(ctx, tx, tenant, sub, origen, destino)
	if err != nil {
		return err
	}
	for bookID, book := range locked {
		if book.owner != sub && book.visibility != domain.VisOrganization && !editors[bookID] {
			return domain.ErrNotFound
		}
	}
	if locked[origen].encrypted || locked[destino].encrypted {
		return domain.ErrInvalidInput
	}
	return nil
}

func queryLockedMoveNotebooks(ctx context.Context, tx pgx.Tx, tenant string, origen, destino uuid.UUID) (map[uuid.UUID]moveNotebookAccess, error) {
	books, err := tx.Query(ctx, `SELECT id,owner_sub,visibility,encrypted FROM notebooks /* notes_move_lock_books */
		WHERE tenant=$1 AND id=ANY($2::uuid[]) AND deleted_at IS NULL
		ORDER BY id FOR UPDATE`, tenant, []uuid.UUID{origen, destino})
	if err != nil {
		return nil, err
	}
	defer books.Close()
	locked := make(map[uuid.UUID]moveNotebookAccess, 2)
	for books.Next() {
		var bookID uuid.UUID
		var book moveNotebookAccess
		if err := books.Scan(&bookID, &book.owner, &book.visibility, &book.encrypted); err != nil {
			return nil, err
		}
		locked[bookID] = book
	}
	return locked, books.Err()
}

func queryLockedMoveGrants(ctx context.Context, tx pgx.Tx, tenant, sub string, origen, destino uuid.UUID) (map[uuid.UUID]bool, error) {
	// A grant used for this move must survive until commit. Read and lock
	// existing editor rows, rather than re-reading an unlocked EXISTS later.
	grants, err := tx.Query(ctx, `SELECT notebook_id FROM notebook_editores /* notes_move_lock_grants */
		WHERE tenant=$1 AND sujeto=$2 AND notebook_id=ANY($3::uuid[])
		ORDER BY notebook_id FOR SHARE`, tenant, sub, []uuid.UUID{origen, destino})
	if err != nil {
		return nil, err
	}
	defer grants.Close()
	editors := make(map[uuid.UUID]bool, 2)
	for grants.Next() {
		var bookID uuid.UUID
		if err := grants.Scan(&bookID); err != nil {
			return nil, err
		}
		editors[bookID] = true
	}
	return editors, grants.Err()
}

func lockMoveRootNote(ctx context.Context, tx pgx.Tx, tenant string, id, origen uuid.UUID) error {
	var actual uuid.UUID
	err := tx.QueryRow(ctx, `
		SELECT n.notebook_id FROM notes n
		 WHERE n.tenant=$1 AND n.id=$2 AND n.deleted_at IS NULL AND n.notebook_id=$3
		 FOR UPDATE OF n`, tenant, id, origen).Scan(&actual)
	if isNoRows(err) {
		// Nota, cuaderno de destino o permiso que faltan: se contestan igual
		// para no revelar cuáles existen.
		return domain.ErrNotFound
	}
	return err
}

func lockMoveFamily(ctx context.Context, tx pgx.Tx, tenant string, id, origen uuid.UUID) ([]uuid.UUID, error) {
	// Freeze and inspect every row we will move. An orphaned ciphertext in an
	// otherwise unencrypted notebook cannot safely be copied to another one.
	rows, err := tx.Query(ctx, `
		WITH RECURSIVE familia AS (
			SELECT id, 0 AS nivel FROM notes WHERE tenant = $1 AND id = $2
			UNION ALL
			SELECT c.id, f.nivel + 1 FROM notes c JOIN familia f ON c.group_id = f.id
			 WHERE c.tenant = $1 AND f.nivel < 64
		)
		SELECT n.id,n.notebook_id,n.content_cipher,
		       (f.nivel=64 AND EXISTS(SELECT 1 FROM notes child WHERE child.tenant=$1 AND child.group_id=n.id))
		FROM notes n
		JOIN familia f ON f.id=n.id WHERE n.tenant=$1
		ORDER BY n.id FOR UPDATE OF n`, tenant, id)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var familia []uuid.UUID
	for rows.Next() {
		var noteID, notebookID uuid.UUID
		var cipher string
		var exceedsDepth bool
		if err := rows.Scan(&noteID, &notebookID, &cipher, &exceedsDepth); err != nil {
			return nil, err
		}
		if notebookID != origen || cipher != "" || exceedsDepth {
			return nil, domain.ErrInvalidInput
		}
		familia = append(familia, noteID)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if len(familia) == 0 {
		return nil, domain.ErrNotFound
	}
	return familia, nil
}

func executeMoveFamily(ctx context.Context, tx pgx.Tx, tenant string, id, destino, origen uuid.UUID, familia []uuid.UUID) error {
	tag, err := tx.Exec(ctx, `
		UPDATE notes n
		   SET notebook_id = $3,
		       group_id = CASE WHEN n.id = $2 THEN NULL ELSE n.group_id END,
		       col = CASE WHEN n.id = $2 THEN 0 ELSE n.col END,
		       position = CASE WHEN n.id = $2
		                       THEN (SELECT COALESCE(max(position), 0) + 1 FROM notes
		                              WHERE tenant = $1 AND notebook_id = $3 AND col = 0
		                                AND deleted_at IS NULL)
		                       ELSE n.position END,
		       updated_at = now()
		 WHERE n.tenant = $1 AND n.notebook_id=$4 AND n.content_cipher=''
		   AND n.id=ANY($5::uuid[])`,
		tenant, id, destino, origen, familia)
	if err != nil {
		return err
	}
	if tag.RowsAffected() != int64(len(familia)) {
		return domain.ErrInvalidInput
	}
	return nil
}

// DeleteNote manda una nota a la papelera (borrado suave).
func (s *Store) DeleteNote(ctx context.Context, id uuid.UUID, sub string) error {
	return s.marcarBorrado(ctx, id, sub, true)
}

// RestoreNote la saca de la papelera.
func (s *Store) RestoreNote(ctx context.Context, id uuid.UUID, sub string) error {
	return s.marcarBorrado(ctx, id, sub, false)
}

func (s *Store) marcarBorrado(ctx context.Context, id uuid.UUID, sub string, borrar bool) error {
	t := tenancy.MustFrom(ctx)
	return s.inWriteTx(ctx, func(tx pgx.Tx) error {
		if err := lockNoteTree(ctx, tx, t.Slug); err != nil {
			return err
		}
		tag, err := tx.Exec(ctx, `
			UPDATE notes n
			   SET deleted_at = CASE WHEN $4 THEN now() ELSE NULL END, updated_at = now()
			  FROM notebooks nb
			 WHERE n.tenant=$1 AND n.id=$2
			   AND nb.id = n.notebook_id AND nb.deleted_at IS NULL
			   AND (nb.owner_sub = $3 OR nb.visibility = 'organization' OR EXISTS (SELECT 1 FROM notebook_editores e WHERE e.notebook_id = nb.id AND e.sujeto = $3))
			   AND (n.deleted_at IS NULL) = $4`, t.Slug, id, sub, borrar)
		if err != nil {
			return err
		}
		if tag.RowsAffected() == 0 {
			return domain.ErrNotFound
		}
		return nil
	})
}

// PurgeNote la borra de verdad (vaciar papelera). Solo su dueño.
func (s *Store) PurgeNote(ctx context.Context, id uuid.UUID, sub string) error {
	t := tenancy.MustFrom(ctx)
	return s.inWriteTx(ctx, func(tx pgx.Tx) error {
		if err := lockNoteTree(ctx, tx, t.Slug); err != nil {
			return err
		}
		tag, err := tx.Exec(ctx,
			`DELETE FROM notes WHERE tenant=$1 AND id=$2 AND owner_sub=$3 AND deleted_at IS NOT NULL`,
			t.Slug, id, sub)
		if err != nil {
			return err
		}
		if tag.RowsAffected() == 0 {
			return domain.ErrNotFound
		}
		// Sin título ni cuerpo: la fila ya no existe y guardarlos aquí sería
		// conservar en la traza justo lo que se acaba de pedir destruir.
		return writeAuditTx(ctx, tx, t.Slug, sub, domain.AuditNotePurge, id.String(), nil)
	})
}

// lockNoteTree serializes only structural changes inside one tenant. Acquire
// it before notebook/note rows or daily-notebook locks, so a family snapshot
// cannot miss a concurrent insertion, reparent or another family move.
func lockNoteTree(ctx context.Context, tx pgx.Tx, tenant string) error {
	_, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtextextended('sytue:notes:tree:' || $1, 0)) /* notes_tree_guard */`, tenant)
	return err
}

// ListTrash lista las notas que `sub` tiene en la papelera, con cuándo se borraron, el
// título de su cuaderno y si una retención legal activa las cubre.
//
// El hold se evalúa con LA MISMA condición que la purga (retencion.go): si no casaran,
// la papelera prometería «se borra en N días» de algo que el recolector no tocará, o
// al revés, que es peor.
func (s *Store) ListTrash(ctx context.Context, sub string, limit int) ([]domain.TrashedNote, error) {
	t := tenancy.MustFrom(ctx)
	var out []domain.TrashedNote
	err := s.pool.InTenantTx(ctx, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, `
			SELECT `+noteCols+`, n.deleted_at, COALESCE(nb.title, ''),
			       EXISTS (
			         SELECT 1 FROM legal_holds h
			          WHERE h.tenant = n.tenant AND h.released_at IS NULL
			            AND (h.subject IS NULL OR h.subject = n.owner_sub))
			  FROM notes n
			  LEFT JOIN notebooks nb ON nb.tenant = n.tenant AND nb.id = n.notebook_id
			 WHERE n.tenant=$1 AND n.owner_sub=$2 AND n.deleted_at IS NOT NULL
			 ORDER BY n.deleted_at DESC LIMIT $3`, t.Slug, sub, limit)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var tn domain.TrashedNote
			n, err := scanNote(rows, t.Slug, &tn.DeletedAt, &tn.NotebookTitle, &tn.Held)
			if err != nil {
				return err
			}
			tn.Note = n
			out = append(out, tn)
		}
		return rows.Err()
	})
	return out, err
}

// reemplazarEnlaces deja los enlaces de la nota EXACTAMENTE como los manda el
// cliente (borra y reinserta). Es una lista corta y así no hay que diferenciar.
func reemplazarEnlaces(ctx context.Context, tx pgx.Tx, slug string, noteID uuid.UUID, links []domain.NoteLink) error {
	if _, err := tx.Exec(ctx, `DELETE FROM note_links WHERE tenant=$1 AND note_id=$2`, slug, noteID); err != nil {
		return err
	}
	for _, l := range links {
		if _, err := tx.Exec(ctx, `
			INSERT INTO note_links (tenant, note_id, target_kind, target_id, label, url)
			VALUES ($1,$2,$3,$4,$5,$6)
			ON CONFLICT (note_id, target_kind, target_id) DO UPDATE SET label = EXCLUDED.label, url = EXCLUDED.url`,
			slug, noteID, l.TargetKind, l.TargetID, l.Label, l.URL); err != nil {
			return err
		}
	}
	return nil
}

// grupoAdmisible decide si `grupo` puede ser el padre de la nota `id`: existe,
// está viva, es del MISMO cuaderno y no cuelga —directa ni indirectamente— de
// la propia nota.
//
// Sin esto, anidar A dentro de B cuando B ya cuelga de A deja un ciclo en
// `group_id` que ninguna consulta recursiva termina y que el árbol de la
// interfaz solo puede esconder. Y una nota de otro cuaderno como padre sería una
// nota que se ve en un sitio y cuelga de otro. `id` vale uuid.Nil al crear: una
// nota que aún no existe no puede ser antepasada de nadie. `notebookID` se pasa
// al crear —la nota no tiene fila de la que sacarlo— y se deduce al editar.
//
// El tope de 64 niveles no es un límite del producto: es el seguro de que un
// ciclo ya existente —de antes de esta guardia— no convierta la comprobación en
// un bucle. Con el ciclo la subida no encuentra `id` y no hay tope que valga, así
// que se cuenta como inadmisible.
func grupoAdmisible(ctx context.Context, tx pgx.Tx, tenant string, id uuid.UUID, notebookID, grupo *uuid.UUID) error {
	if grupo == nil {
		return nil
	}
	if *grupo == id {
		return domain.ErrInvalidInput
	}
	var ok bool
	err := tx.QueryRow(ctx, `
		WITH RECURSIVE arriba AS (
			SELECT n.id, n.group_id, n.notebook_id, 0 AS nivel
			  FROM notes n
			 WHERE n.tenant = $1 AND n.id = $2 AND n.deleted_at IS NULL
			UNION ALL
			SELECT p.id, p.group_id, p.notebook_id, a.nivel + 1
			  FROM notes p JOIN arriba a ON p.id = a.group_id
			 WHERE p.tenant = $1 AND a.nivel < 64
		)
		SELECT EXISTS (
		         SELECT 1 FROM arriba
		          WHERE nivel = 0
		            AND notebook_id = COALESCE($4::uuid,
		                  (SELECT notebook_id FROM notes WHERE tenant = $1 AND id = $3)))
		   AND NOT EXISTS (SELECT 1 FROM arriba WHERE id = $3)
		   AND NOT EXISTS (SELECT 1 FROM arriba WHERE nivel = 64)`,
		tenant, *grupo, id, notebookID).Scan(&ok)
	if err != nil {
		return err
	}
	if !ok {
		return domain.ErrInvalidInput
	}
	return nil
}
