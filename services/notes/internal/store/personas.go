// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package store

import (
	"context"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"git.sytue.eu/sytue/one/platform/pkg/tenancy"
	"git.sytue.eu/sytue/one/services/notes/internal/domain"
)

// # Compartir con una persona (0010)
//
// Un cuaderno lo comparte SU DUEÑO: para leer (`notebook_lectores`) o para editar sus
// notas (`notebook_editores`, la tabla de «Pedir acceso de edición»). Una persona está
// en una de las dos o en ninguna: cambiar el nivel la mueve de tabla.
//
// Una nota suelta la comparte QUIEN LA ESCRIBIÓ (`note_personas`). No se comparte una
// nota de un cuaderno cifrado: su clave vive en el navegador de quien lo abre, y quien
// recibe la nota sin el cuaderno no tendría con qué descifrarla.
//
// Compartir con uno mismo es entrada inválida; lo que no es de quien comparte, no
// existe (ErrNotFound), igual que en el resto del paquete.

// PersonasDeCuaderno son las personas con quien se ha compartido un cuaderno. Solo su
// dueño.
func (s *Store) PersonasDeCuaderno(ctx context.Context, nb uuid.UUID, sub string) ([]domain.Persona, error) {
	t := tenancy.MustFrom(ctx)
	var out []domain.Persona
	err := s.pool.InTenantTx(ctx, func(tx pgx.Tx) error {
		if e := esDueno(ctx, tx, t.Slug, nb, sub); e != nil {
			return e
		}
		rows, e := tx.Query(ctx, `
			SELECT sujeto, 'read', concedido_por, creado FROM notebook_lectores WHERE tenant = $1 AND notebook_id = $2
			UNION ALL
			SELECT sujeto, 'write', concedido_por, creado FROM notebook_editores WHERE tenant = $1 AND notebook_id = $2
			ORDER BY 4, 1`, t.Slug, nb)
		if e != nil {
			return e
		}
		out, e = scanPersonas(rows)
		return e
	})
	return out, err
}

// CompartirCuaderno da a `sujeto` el cuaderno para leer o para editar. `nueva` dice si
// antes no lo tenía por ninguna de las dos (a quien avisar); `titulo` es el del cuaderno.
// Dar edición retira además su petición pendiente, que ya no tiene nada que pedir.
func (s *Store) CompartirCuaderno(ctx context.Context, nb uuid.UUID, sub, sujeto string, editar bool) (nueva bool, titulo string, err error) {
	if sujeto == "" || sujeto == sub {
		return false, "", domain.ErrInvalidInput
	}
	t := tenancy.MustFrom(ctx)
	err = s.pool.InTenantTx(ctx, func(tx pgx.Tx) error {
		// FOR UPDATE: dos «compartir» a la vez con la misma persona van en fila; si no,
		// cada uno borraría cero filas y la dejarían en las DOS tablas.
		if e := tx.QueryRow(ctx, `SELECT title FROM notebooks WHERE tenant = $1 AND id = $2 AND owner_sub = $3 AND deleted_at IS NULL FOR UPDATE`,
			t.Slug, nb, sub).Scan(&titulo); e != nil {
			if isNoRows(e) {
				return domain.ErrNotFound
			}
			return e
		}
		var antes int64
		for _, q := range []string{
			`DELETE FROM notebook_lectores WHERE tenant = $1 AND notebook_id = $2 AND sujeto = $3`,
			`DELETE FROM notebook_editores WHERE tenant = $1 AND notebook_id = $2 AND sujeto = $3`,
		} {
			tag, e := tx.Exec(ctx, q, t.Slug, nb, sujeto)
			if e != nil {
				return e
			}
			antes += tag.RowsAffected()
		}
		nueva = antes == 0
		if !editar {
			_, e := tx.Exec(ctx, `INSERT INTO notebook_lectores (notebook_id, tenant, sujeto, concedido_por) VALUES ($1, $2, $3, $4)`,
				nb, t.Slug, sujeto, sub)
			return e
		}
		if _, e := tx.Exec(ctx, `INSERT INTO notebook_editores (notebook_id, tenant, sujeto, concedido_por) VALUES ($1, $2, $3, $4)`,
			nb, t.Slug, sujeto, sub); e != nil {
			return e
		}
		_, e := tablaPeticiones.Quitar(ctx, tx, nb.String(), sujeto)
		return e
	})
	return nueva, titulo, err
}

// DejarDeCompartirCuaderno le quita a `sujeto` el cuaderno, lo tuviera para leer o para
// editar. ErrNotFound si no lo tenía.
func (s *Store) DejarDeCompartirCuaderno(ctx context.Context, nb uuid.UUID, sub, sujeto string) error {
	t := tenancy.MustFrom(ctx)
	return s.pool.InTenantTx(ctx, func(tx pgx.Tx) error {
		if e := esDueno(ctx, tx, t.Slug, nb, sub); e != nil {
			return e
		}
		var quitadas int64
		for _, q := range []string{
			`DELETE FROM notebook_lectores WHERE tenant = $1 AND notebook_id = $2 AND sujeto = $3`,
			`DELETE FROM notebook_editores WHERE tenant = $1 AND notebook_id = $2 AND sujeto = $3`,
		} {
			tag, e := tx.Exec(ctx, q, t.Slug, nb, sujeto)
			if e != nil {
				return e
			}
			quitadas += tag.RowsAffected()
		}
		if quitadas == 0 {
			return domain.ErrNotFound
		}
		return nil
	})
}

// autoraDeNota comprueba que `sub` escribió la nota (viva, en un cuaderno vivo) y que
// SIGUE pudiendo escribir en su cuaderno —quien perdió el cuaderno no reparte sus
// notas—, y devuelve su título y si el cuaderno está cifrado. ErrNotFound si no.
func autoraDeNota(ctx context.Context, tx pgx.Tx, tenant string, nota uuid.UUID, sub string) (titulo string, cifrado bool, err error) {
	err = tx.QueryRow(ctx, `
		SELECT n.title, nb.encrypted
		  FROM notes n
		  JOIN notebooks nb ON nb.id = n.notebook_id AND nb.deleted_at IS NULL
		 WHERE n.tenant = $1 AND n.id = $2 AND n.owner_sub = $3 AND n.deleted_at IS NULL
		   AND (nb.owner_sub = $3 OR nb.visibility = 'organization' OR EXISTS (SELECT 1 FROM notebook_editores e WHERE e.notebook_id = nb.id AND e.sujeto = $3))`,
		tenant, nota, sub).Scan(&titulo, &cifrado)
	if isNoRows(err) || (err == nil && sub == "") {
		return "", false, domain.ErrNotFound
	}
	return titulo, cifrado, err
}

// PersonasDeNota son las personas con quien se ha compartido una nota. Solo quien la
// escribió.
func (s *Store) PersonasDeNota(ctx context.Context, nota uuid.UUID, sub string) ([]domain.Persona, error) {
	t := tenancy.MustFrom(ctx)
	var out []domain.Persona
	err := s.pool.InTenantTx(ctx, func(tx pgx.Tx) error {
		if _, _, e := autoraDeNota(ctx, tx, t.Slug, nota, sub); e != nil {
			return e
		}
		rows, e := tx.Query(ctx, `
			SELECT sujeto, CASE WHEN puede_editar THEN 'write' ELSE 'read' END, concedido_por, creado
			  FROM note_personas WHERE tenant = $1 AND note_id = $2
			 ORDER BY creado, sujeto`, t.Slug, nota)
		if e != nil {
			return e
		}
		out, e = scanPersonas(rows)
		return e
	})
	return out, err
}

// CompartirNota da a `sujeto` la nota para leer o para editar (o le cambia el nivel).
// `nueva` dice si antes no la tenía; `titulo` es el de la nota.
func (s *Store) CompartirNota(ctx context.Context, nota uuid.UUID, sub, sujeto string, editar bool) (nueva bool, titulo string, err error) {
	if sujeto == "" || sujeto == sub {
		return false, "", domain.ErrInvalidInput
	}
	t := tenancy.MustFrom(ctx)
	err = s.pool.InTenantTx(ctx, func(tx pgx.Tx) error {
		var cifrado bool
		var e error
		if titulo, cifrado, e = autoraDeNota(ctx, tx, t.Slug, nota, sub); e != nil {
			return e
		}
		if cifrado {
			return domain.ErrInvalidInput
		}
		// `xmax = 0` distingue la fila recién insertada de la que el ON CONFLICT
		// actualizó: es la forma de Postgres de decir «esto es nuevo» en una sola ida.
		return tx.QueryRow(ctx, `
			INSERT INTO note_personas (note_id, tenant, sujeto, puede_editar, concedido_por)
			VALUES ($1, $2, $3, $4, $5)
			ON CONFLICT (note_id, sujeto) DO UPDATE SET puede_editar = EXCLUDED.puede_editar
			RETURNING xmax = 0`, nota, t.Slug, sujeto, editar, sub).Scan(&nueva)
	})
	return nueva, titulo, err
}

// DejarDeCompartirNota le quita a `sujeto` la nota. ErrNotFound si no la tenía.
func (s *Store) DejarDeCompartirNota(ctx context.Context, nota uuid.UUID, sub, sujeto string) error {
	t := tenancy.MustFrom(ctx)
	return s.pool.InTenantTx(ctx, func(tx pgx.Tx) error {
		if _, _, e := autoraDeNota(ctx, tx, t.Slug, nota, sub); e != nil {
			return e
		}
		tag, e := tx.Exec(ctx, `DELETE FROM note_personas WHERE tenant = $1 AND note_id = $2 AND sujeto = $3`,
			t.Slug, nota, sujeto)
		if e != nil {
			return e
		}
		if tag.RowsAffected() == 0 {
			return domain.ErrNotFound
		}
		return nil
	})
}

// LecturaDeNota dice por dónde LEE `sujeto` una nota: por su cuaderno (es suyo, de la
// organización, o está entre sus lectores o editores, y la nota no es privada de otra
// persona) o solo porque se la compartieron a ella. Las dos en falso: no la lee. Un
// cuaderno cifrado no cuenta: quien avisa de una mención no tiene el texto.
//
// Son las verjas de SearchNotes y de NotasCompartidas juntas, para una nota y una
// persona: lo pregunta el aviso de mención antes de nombrarle la nota a nadie.
func (s *Store) LecturaDeNota(ctx context.Context, nota uuid.UUID, sujeto string) (porCuaderno, porNota bool, err error) {
	t := tenancy.MustFrom(ctx)
	err = s.pool.InTenantTx(ctx, func(tx pgx.Tx) error {
		e := tx.QueryRow(ctx, `
			SELECT (nb.owner_sub = $3 OR nb.visibility IN ('organization','public')
			        OR EXISTS (SELECT 1 FROM notebook_lectores l WHERE l.notebook_id = nb.id AND l.sujeto = $3)
			        OR EXISTS (SELECT 1 FROM notebook_editores e WHERE e.notebook_id = nb.id AND e.sujeto = $3))
			       AND (n.visibility IS NULL OR n.visibility <> 'private' OR n.owner_sub = $3),
			       EXISTS (SELECT 1 FROM note_personas c WHERE c.note_id = n.id AND c.sujeto = $3)
			       AND (nb.owner_sub = n.owner_sub OR nb.visibility = 'organization'
			            OR EXISTS (SELECT 1 FROM notebook_editores e WHERE e.notebook_id = nb.id AND e.sujeto = n.owner_sub))
			  FROM notes n
			  JOIN notebooks nb ON nb.id = n.notebook_id AND nb.deleted_at IS NULL
			 WHERE n.tenant = $1 AND n.id = $2 AND n.deleted_at IS NULL AND NOT nb.encrypted`,
			t.Slug, nota, sujeto).Scan(&porCuaderno, &porNota)
		if isNoRows(e) {
			return nil
		}
		return e
	})
	return porCuaderno, porNota, err
}

// NotasCompartidas es «Compartido conmigo»: las notas vivas que otras personas han
// compartido con `sub`, la más reciente primero, con su nivel y quién se la dio.
//
// ponytail: hasta 200, sin paginar. Son las que alguien ha compartido a mano con una
// persona, no un volcado; si alguna vez se pasan, cursor por `creado`.
func (s *Store) NotasCompartidas(ctx context.Context, sub string) ([]domain.NotaCompartida, error) {
	t := tenancy.MustFrom(ctx)
	var out []domain.NotaCompartida
	err := s.pool.InTenantTx(ctx, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, `
			SELECT `+noteCols+`, c.puede_editar, c.concedido_por
			  FROM note_personas c
			  JOIN notes n ON n.id = c.note_id AND n.deleted_at IS NULL
			  JOIN notebooks nb ON nb.id = n.notebook_id AND nb.deleted_at IS NULL
			 WHERE c.tenant = $1 AND c.sujeto = $2
			   -- Lo que compartió quien ya no escribe en ese cuaderno deja de valer.
			   AND (nb.owner_sub = n.owner_sub OR nb.visibility = 'organization'
			        OR EXISTS (SELECT 1 FROM notebook_editores e WHERE e.notebook_id = nb.id AND e.sujeto = n.owner_sub))
			 ORDER BY c.creado DESC
			 LIMIT 200`, t.Slug, sub)
		if err != nil {
			return err
		}
		defer rows.Close()
		var notas []domain.Note
		var extra []domain.NotaCompartida
		for rows.Next() {
			var editar bool
			var por string
			n, err := scanNote(rows, t.Slug, &editar, &por)
			if err != nil {
				return err
			}
			permiso := domain.PermisoLeer
			if editar {
				permiso = domain.PermisoEditar
			}
			notas = append(notas, n)
			extra = append(extra, domain.NotaCompartida{Permiso: permiso, ConcedidoPor: por})
		}
		if err := rows.Err(); err != nil {
			return err
		}
		rows.Close()
		if err := s.adjuntarEnPaquete(ctx, tx, t.Slug, notas); err != nil {
			return err
		}
		for i := range notas {
			extra[i].Note = notas[i]
		}
		out = extra
		return nil
	})
	return out, err
}

func scanPersonas(rows pgx.Rows) ([]domain.Persona, error) {
	defer rows.Close()
	out := []domain.Persona{}
	for rows.Next() {
		var p domain.Persona
		if err := rows.Scan(&p.Sujeto, &p.Permiso, &p.ConcedidoPor, &p.Creado); err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}
