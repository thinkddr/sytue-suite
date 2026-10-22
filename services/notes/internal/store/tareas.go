// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package store

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"git.sytue.eu/sytue/one/platform/pkg/tenancy"
	"git.sytue.eu/sytue/one/services/notes/internal/domain"
)

// Estados que admite GET /tasks.
const (
	TareaAbierta   = "open"
	TareaHecha     = "done"
	TareaCancelada = "cancelled"
)

// topeTareas es lo más que devuelve una consulta de tareas.
const topeTareas = 200

// Tarea es una tarea con lo justo de su cuaderno para pintarla fuera de él.
type Tarea struct {
	domain.Note
	NotebookTitle string `json:"notebook"`
	DailyDate     string `json:"daily_date,omitempty"`
}

// ListTasks devuelve las tareas (`kind='todo'`) de `sub` en todos los cuadernos que
// puede leer: las que escribió y las de sus cuadernos. Las de los cuadernos de
// organización de otras personas no son «sus tareas» y no entran.
//
// Las verjas de lectura son las de SearchNotes: ni cuadernos cifrados (no hay
// título que enseñar) ni notas privadas de otra persona. La de indexación no
// aplica, como en RecentNotes: son tus tareas, no un índice.
//
// Las casillas escritas DENTRO del HTML de una nota no salen: viven en `content`
// y no se pueden consultar.
//
// `desde` y `hasta` acotan por la fecha del estado: completada, cancelada o creada.
func (s *Store) ListTasks(ctx context.Context, sub, estado string, desde, hasta *time.Time) ([]Tarea, error) {
	if estado != TareaAbierta && estado != TareaHecha && estado != TareaCancelada {
		return nil, domain.ErrInvalidInput
	}
	t := tenancy.MustFrom(ctx)
	out := []Tarea{}
	err := s.pool.InTenantTx(ctx, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, `
			SELECT `+noteCols+`, nb.title, nb.daily_date
			  FROM notes n
			  JOIN notebooks nb ON nb.id = n.notebook_id AND nb.deleted_at IS NULL
			 WHERE n.tenant = $1 AND n.deleted_at IS NULL AND n.kind = 'todo'
			   AND NOT nb.encrypted AND n.content_cipher = ''
			   AND (n.owner_sub = $2 OR nb.owner_sub = $2)
			   AND (nb.owner_sub = $2 OR nb.visibility IN ('organization','public')
			        OR EXISTS (SELECT 1 FROM notebook_lectores l WHERE l.notebook_id = nb.id AND l.sujeto = $2)
			        OR EXISTS (SELECT 1 FROM notebook_editores e WHERE e.notebook_id = nb.id AND e.sujeto = $2))
			   AND (n.visibility IS NULL OR n.visibility <> 'private' OR n.owner_sub = $2)
			   AND CASE $3 WHEN 'open' THEN NOT n.checked AND n.cancelled_at IS NULL
			               WHEN 'done' THEN n.checked
			               ELSE n.cancelled_at IS NOT NULL END
			   AND ($4::timestamptz IS NULL OR COALESCE(n.checked_at, n.cancelled_at, n.created_at) >= $4)
			   AND ($5::timestamptz IS NULL OR COALESCE(n.checked_at, n.cancelled_at, n.created_at) < $5)
			 ORDER BY COALESCE(n.checked_at, n.cancelled_at, n.created_at) DESC, n.id
			 LIMIT $6`, t.Slug, sub, estado, desde, hasta, topeTareas)
		if err != nil {
			return err
		}
		var notas []domain.Note
		for rows.Next() {
			var x Tarea
			n, err := scanNote(rows, t.Slug, &x.NotebookTitle, &x.DailyDate)
			if err != nil {
				rows.Close()
				return err
			}
			notas = append(notas, n)
			out = append(out, x)
		}
		rows.Close()
		if err := rows.Err(); err != nil {
			return err
		}
		// Con estados y enlaces: marcar una tarea desde la vista reenvía la nota
		// entera, y sin sus enlaces el PUT los borraría.
		if err := s.adjuntarEnPaquete(ctx, tx, t.Slug, notas); err != nil {
			return err
		}
		for i := range out {
			out[i].Note = notas[i]
		}
		return nil
	})
	return out, err
}

// PostponeTask lleva una nota, con todo lo que cuelga de ella, al diario de `date`
// (creándolo si hace falta). Es «Posponer» de Twos: mañana o el día que se elija.
// Las reglas son las del traslado diario, así que una nota compartida o cifrada no
// se mueve: contesta ErrNotFound, igual que una que no existe.
func (s *Store) PostponeTask(ctx context.Context, id uuid.UUID, sub, date, title string) (domain.Notebook, error) {
	t := tenancy.MustFrom(ctx)
	var nb domain.Notebook
	err := s.inWriteTx(ctx, func(tx pgx.Tx) error {
		var err error
		if nb, err = abrirDiario(ctx, tx, t.Slug, sub, date, title); err != nil {
			return err
		}
		movidas, err := trasladar(ctx, tx, t.Slug, sub, nb, date, &id)
		if err == nil && movidas == 0 {
			err = domain.ErrNotFound
		}
		return err
	})
	return nb, err
}

// trasladar mueve al diario `destino` las tareas pendientes de días anteriores
// (`raiz` nil) o la nota `raiz` (posponer), cada una con TODO su subárbol.
//
// UN TRASLADO NUNCA AMPLÍA QUIÉN PUEDE LEER UNA NOTA: origen y destino tienen que
// ser cuadernos personales, sin cifrar y sin compartir, y si un solo nodo del
// subárbol es de otra persona, está cifrado, tiene visibilidad propia más abierta
// o está compartido con alguien (`note_personas`), el subárbol entero se queda
// donde está. Mover solo una parte rompería el grupo; moverlo entero lo
// enseñaría donde no se enseñaba.
//
// Las raíces del traslado diario son las tareas pendientes sueltas y las que
// cuelgan de una tarea terminada (hecha o cancelada) de primer nivel: el padre se
// queda en su día y los hijos suben al nivel superior del día nuevo. Una tarea
// cancelada no se traslada nunca. Cada raíz trasladada suma uno a
// `data.carry_count` y recuerda el primer día del que salió en `data.carried_from`.
func trasladar(ctx context.Context, tx pgx.Tx, slug, sub string, destino domain.Notebook, date string, raiz *uuid.UUID) (int64, error) {
	if destino.Encrypted || (destino.Visibility != domain.VisPersonal && destino.Visibility != domain.VisPrivate) {
		return 0, nil
	}
	tag, err := tx.Exec(ctx, `WITH RECURSIVE raices AS (
            SELECT n.id, old.daily_date AS origen_dia,
                old.visibility AS origen_visibilidad, old.indexable AS origen_indexable,
                row_number() OVER (ORDER BY old.daily_date, n.position, n.created_at, n.id) AS orden
            FROM notes n JOIN notebooks old ON old.id=n.notebook_id
            WHERE n.tenant=$1 AND n.owner_sub=$2 AND old.tenant=$1 AND old.owner_sub=$2
                AND old.id<>$4 AND old.deleted_at IS NULL AND n.deleted_at IS NULL
                AND old.visibility IN ('personal','private') AND NOT old.encrypted
                AND NOT EXISTS (SELECT 1 FROM notebook_lectores l WHERE l.notebook_id=old.id)
                AND NOT EXISTS (SELECT 1 FROM notebook_editores e WHERE e.notebook_id=old.id)
                AND CASE WHEN $5::uuid IS NOT NULL THEN n.id=$5 ELSE
                    old.daily_date<>'' AND old.daily_date<$3
                    AND n.kind='todo' AND NOT n.checked AND n.cancelled_at IS NULL
                    AND (n.group_id IS NULL OR EXISTS (
                        SELECT 1 FROM notes padre WHERE padre.id=n.group_id AND padre.group_id IS NULL
                            AND padre.kind='todo' AND (padre.checked OR padre.cancelled_at IS NOT NULL)))
                END
        ), arbol AS (
            SELECT id AS raiz, id, 0 AS nivel FROM raices
            UNION ALL
            SELECT a.raiz, c.id, a.nivel+1 FROM notes c JOIN arbol a ON c.group_id=a.id
             WHERE c.tenant=$1 AND a.nivel<64
        ), vetadas AS (
            SELECT DISTINCT a.raiz FROM arbol a JOIN notes m ON m.id=a.id
             WHERE m.owner_sub<>$2 OR m.content_cipher<>''
                OR (m.visibility IS NOT NULL AND m.visibility NOT IN ('personal','private'))
                OR EXISTS (SELECT 1 FROM note_personas p WHERE p.note_id=m.id)
        ), final AS (
            SELECT COALESCE(max(position),0) AS posicion FROM notes WHERE notebook_id=$4 AND deleted_at IS NULL
        ) UPDATE notes n SET notebook_id=$4,
            visibility=COALESCE(n.visibility, r.origen_visibilidad),
            indexable=COALESCE(n.indexable, r.origen_indexable),
            group_id=CASE WHEN n.id=r.id THEN NULL ELSE n.group_id END,
            col=CASE WHEN n.id=r.id THEN 0 ELSE n.col END,
            position=CASE WHEN n.id=r.id THEN (final.posicion+r.orden)::integer ELSE n.position END,
            data=CASE WHEN n.id=r.id AND $5::uuid IS NULL THEN n.data || jsonb_build_object(
                    'carried_from', COALESCE(n.data->>'carried_from', r.origen_dia),
                    'carry_count', CASE WHEN n.data->>'carry_count' ~ '^[0-9]{1,6}$'
                                        THEN (n.data->>'carry_count')::int ELSE 0 END + 1)
                 ELSE n.data END,
            updated_at=now()
        FROM arbol a JOIN raices r ON r.id=a.raiz, final
        WHERE n.id=a.id AND a.raiz NOT IN (SELECT raiz FROM vetadas)
            AND NOT EXISTS (SELECT 1 FROM notebook_lectores l WHERE l.notebook_id=$4)
            AND NOT EXISTS (SELECT 1 FROM notebook_editores e WHERE e.notebook_id=$4)`,
		slug, sub, date, destino.ID, raiz)
	return tag.RowsAffected(), err
}
