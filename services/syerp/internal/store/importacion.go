// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package store

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"

	"git.sytue.eu/sytue/one/services/syerp/internal/catalogo"
	"git.sytue.eu/sytue/one/services/syerp/internal/domain"
)

// IMPORTACIÓN EN LOTE, REVERSIBLE (migración 0093).
//
// Antes, importar era un bucle de `Crear`/`UpsertPorExterno`: una transacción
// POR FILA, cinco mil transacciones por petición, y nada que dijera después qué
// entró en esa tanda. Aquí va todo en UNA transacción con un savepoint por fila
// —así una fila mala sigue sin abortar el resto, que es lo que se quiere de una
// importación— y queda apuntado qué se creó, qué se actualizó y qué había antes.

// FilaImportada es una fila ya leída y validada, lista para escribir.
type FilaImportada struct {
	Linea    int
	Data     map[string]any
	Etiqueta string
	// Externo es el `externo_id`: con él la fila se crea o se actualiza (así
	// importar dos veces no duplica); vacío, siempre se crea.
	Externo string
	Estado  string
}

// FalloDeFila es una fila que no entró, con su número de línea.
type FalloDeFila struct {
	Fila  int    `json:"fila"`
	Error string `json:"error"`
}

// ResultadoImportacion es lo que pasó con el lote.
type ResultadoImportacion struct {
	// ID del lote. Vacío en una simulación: no se ha escrito nada que revertir.
	ID           string        `json:"id,omitempty"`
	Filas        int           `json:"filas"`
	Creados      int           `json:"creados"`
	Actualizados int           `json:"actualizados"`
	Fallos       []FalloDeFila `json:"fallos"`
}

// Importar escribe un lote entero y lo deja revertible.
//
// Una fila que falla se anota y NO tumba a las demás: cada una va en su
// savepoint. Eso conserva el comportamiento que tenía la importación fila a
// fila y añade lo que le faltaba —que el lote exista como una cosa—, sin pagar
// una transacción por fila.
func (s *Store) Importar(ctx context.Context, m *catalogo.Modelo, filas []FilaImportada, fichero, por string) (ResultadoImportacion, error) {
	res := ResultadoImportacion{Filas: len(filas), Fallos: []FalloDeFila{}}
	err := s.escritura(ctx, func(tx pgx.Tx) error {
		if err := tx.QueryRow(ctx, `
			INSERT INTO importacion (tenant, modelo, fichero, por)
			VALUES (current_setting('app.tenant', true), $1, $2, $3)
			RETURNING id::text`, m.Clave, fichero, por).Scan(&res.ID); err != nil {
			return err
		}
		for _, f := range filas {
			creado, err := escribirFila(ctx, tx, m, res.ID, f, por)
			if err != nil {
				res.Fallos = append(res.Fallos, FalloDeFila{f.Linea, traducirError(err).Error()})
				continue
			}
			if creado {
				res.Creados++
			} else {
				res.Actualizados++
			}
		}
		_, err := tx.Exec(ctx, `
			UPDATE importacion SET filas = $2, creados = $3, actualizados = $4, fallos = $5
			 WHERE tenant = current_setting('app.tenant', true) AND id = $1::uuid`, res.ID, res.Filas, res.Creados, res.Actualizados, len(res.Fallos))
		return err
	})
	if err != nil {
		return ResultadoImportacion{}, err
	}
	return res, nil
}

// escribirFila mete UNA fila dentro de su propio savepoint y deja apuntado qué
// le pasó al registro. Devuelve si se creó (frente a actualizarse).
func escribirFila(ctx context.Context, tx pgx.Tx, m *catalogo.Modelo, lote string, f FilaImportada, por string) (bool, error) {
	sp, err := tx.Begin(ctx) // savepoint
	if err != nil {
		return false, err
	}
	creado, err := aplicarFila(ctx, sp, m, lote, f, por)
	if err != nil {
		_ = sp.Rollback(ctx)
		return false, err
	}
	return creado, sp.Commit(ctx)
}

func aplicarFila(ctx context.Context, tx pgx.Tx, m *catalogo.Modelo, lote string, f FilaImportada, por string) (bool, error) {
	if err := bloquearOrigenCompra(ctx, tx, m.Clave, f.Externo); err != nil {
		return false, err
	}
	if err := comprobarRelaciones(ctx, tx, m, f.Data); err != nil {
		return false, err
	}
	crudo, err := json.Marshal(f.Data)
	if err != nil {
		return false, err
	}

	// ¿Existe ya por su id de origen? Sin `externo_id` no hay a qué agarrarse:
	// se crea.
	var id, etiquetaAntes, estadoAntes string
	var datosAntes []byte
	if f.Externo != "" {
		filtro, err := json.Marshal(map[string]string{"externo_id": f.Externo})
		if err != nil {
			return false, err
		}
		err = tx.QueryRow(ctx, `
			SELECT id::text, data, etiqueta, estado FROM records
			 WHERE tenant = current_setting('app.tenant', true) AND model = $1 AND data @> $2::jsonb AND deleted_at IS NULL
			 LIMIT 1
			 FOR UPDATE`, m.Clave, filtro).Scan(&id, &datosAntes, &etiquetaAntes, &estadoAntes)
		if err != nil && !errors.Is(err, pgx.ErrNoRows) {
			return false, err
		}
	}

	var tras time.Time
	if id == "" {
		ref := ""
		if m.Secuencia != "" {
			if ref, err = reservarRef(ctx, tx, "doc:"+m.Clave, m.Secuencia, periodoDe(f.Data)); err != nil {
				return false, err
			}
		}
		estado := f.Estado
		if estado == "" {
			estado = domain.EstadoInicial(m)
		}
		if err := tx.QueryRow(ctx, `
			INSERT INTO records (tenant, model, ref, etiqueta, estado, data, owner_sub)
			VALUES (current_setting('app.tenant', true), $1, $2, $3, $4, $5, $6)
			RETURNING id::text, updated_at`,
			m.Clave, ref, f.Etiqueta, estado, crudo, por).Scan(&id, &tras); err != nil {
			return false, err
		}
		return true, apuntarFila(ctx, tx, lote, id, "creado", nil, "", "", tras)
	}

	// Actualizar NO pisa el estado si el origen no trae ninguno: el flujo local
	// puede ir más adelantado que el del sistema de origen.
	if err := tx.QueryRow(ctx, `
		UPDATE records
		   SET data = $2,
		       etiqueta = CASE WHEN $3 <> '' THEN $3 ELSE etiqueta END,
		       estado = CASE WHEN $4 <> '' THEN $4 ELSE estado END,
		       updated_at = now()
		 WHERE tenant = current_setting('app.tenant', true) AND id = $1::uuid
		RETURNING updated_at`, id, crudo, f.Etiqueta, f.Estado).Scan(&tras); err != nil {
		return false, err
	}
	return false, apuntarFila(ctx, tx, lote, id, "actualizado", datosAntes, etiquetaAntes, estadoAntes, tras)
}

func apuntarFila(ctx context.Context, tx pgx.Tx, lote, registro, accion string, antes []byte, etiquetaAntes, estadoAntes string, tras time.Time) error {
	_, err := tx.Exec(ctx, `
		INSERT INTO importacion_fila (tenant, importacion_id, registro_id, accion,
		                              datos_antes, etiqueta_antes, estado_antes, updated_at_tras)
		VALUES (current_setting('app.tenant', true), $1::uuid, $2::uuid, $3, $4, $5, $6, $7)`,
		lote, registro, accion, antes, etiquetaAntes, estadoAntes, tras)
	return err
}

// --- Vuelta atrás -------------------------------------------------------------

// ErrYaRevertida es un lote que ya se deshizo. No se deshace dos veces.
var ErrYaRevertida = errors.New("esa importación ya se revirtió")

// Reversion es lo que se deshizo y, sobre todo, lo que NO.
type Reversion struct {
	Borrados    int `json:"borrados"`
	Restaurados int `json:"restaurados"`
	// Intactos son los registros que alguien tocó DESPUÉS de la importación.
	// No se revierten: el trabajo de otra persona no se pisa para deshacer un
	// error propio. Van contados y aparte para que se sepa que quedan ahí.
	Intactos int `json:"intactos"`
}

// Revertir deshace un lote entero en UNA transacción.
//
// Lo creado se manda a la papelera (borrado lógico, como cualquier borrado del
// ERP: el historial lo registra y se puede rescatar). Lo actualizado vuelve a
// los datos y al estado que tenía. Lo que alguien cambió después de la
// importación se queda como está y se cuenta aparte.
//
// ponytail: «cambió después» incluye que lo cambiara OTRA importación, así que
// revertir un lote viejo después de que uno nuevo tocara sus mismas filas no
// deshace esas filas — las cuenta como intactas y lo dice. Es el lado seguro:
// deshacerlas se llevaría por delante el lote nuevo sin avisar. Si algún día
// hace falta encadenar reversiones, el camino es revertir en orden inverso y
// que cada reversión reescriba el `updated_at_tras` del lote anterior.
func (s *Store) Revertir(ctx context.Context, importacionID, por string) (Reversion, error) {
	var out Reversion
	err := s.escritura(ctx, func(tx pgx.Tx) error {
		var revertida *time.Time
		if err := tx.QueryRow(ctx, `
			SELECT revertida_en FROM importacion WHERE tenant = current_setting('app.tenant', true) AND id = $1::uuid FOR UPDATE`,
			importacionID).Scan(&revertida); err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return ErrNoEncontrado
			}
			return err
		}
		if revertida != nil {
			return ErrYaRevertida
		}

		// Lo creado: a la papelera, salvo lo que alguien haya tocado después.
		etiq, err := tx.Exec(ctx, `
			UPDATE records r SET deleted_at = now(), updated_at = now()
			  FROM importacion_fila f
			 WHERE r.tenant = current_setting('app.tenant', true) AND f.tenant = r.tenant
			   AND f.importacion_id = $1::uuid AND f.accion = 'creado'
			   AND r.id = f.registro_id AND r.deleted_at IS NULL
			   AND r.updated_at = f.updated_at_tras`, importacionID)
		if err != nil {
			return err
		}
		out.Borrados = int(etiq.RowsAffected())

		// Lo actualizado: vuelve a lo que había, mismo criterio.
		etiq, err = tx.Exec(ctx, `
			UPDATE records r
			   SET data = f.datos_antes,
			       etiqueta = CASE WHEN f.etiqueta_antes <> '' THEN f.etiqueta_antes ELSE r.etiqueta END,
			       estado = CASE WHEN f.estado_antes <> '' THEN f.estado_antes ELSE r.estado END,
			       updated_at = now()
			  FROM importacion_fila f
			 WHERE r.tenant = current_setting('app.tenant', true) AND f.tenant = r.tenant
			   AND f.importacion_id = $1::uuid AND f.accion = 'actualizado'
			   AND r.id = f.registro_id AND r.deleted_at IS NULL
			   AND r.updated_at = f.updated_at_tras
			   AND f.datos_antes IS NOT NULL`, importacionID)
		if err != nil {
			return err
		}
		out.Restaurados = int(etiq.RowsAffected())

		var total int
		if err := tx.QueryRow(ctx, `
			SELECT count(*) FROM importacion_fila WHERE tenant = current_setting('app.tenant', true) AND importacion_id = $1::uuid`,
			importacionID).Scan(&total); err != nil {
			return err
		}
		out.Intactos = total - out.Borrados - out.Restaurados

		_, err = tx.Exec(ctx, `
			UPDATE importacion SET revertida_en = now(), revertida_por = $2
			 WHERE tenant = current_setting('app.tenant', true) AND id = $1::uuid`, importacionID, por)
		return err
	})
	return out, err
}

// LoteImportado es una importación tal como se lista.
type LoteImportado struct {
	ID           string     `json:"id"`
	Modelo       string     `json:"modelo"`
	Fichero      string     `json:"fichero,omitempty"`
	Filas        int        `json:"filas"`
	Creados      int        `json:"creados"`
	Actualizados int        `json:"actualizados"`
	Fallos       int        `json:"fallos"`
	Por          string     `json:"por,omitempty"`
	CreadaEn     time.Time  `json:"creada_en"`
	RevertidaEn  *time.Time `json:"revertida_en,omitempty"`
}

// Importaciones lista los lotes recientes, el más nuevo primero. Sin esto la
// vuelta atrás sería inalcanzable: haría falta saberse el id del lote.
func (s *Store) Importaciones(ctx context.Context, limite int) ([]LoteImportado, error) {
	if limite <= 0 || limite > 200 {
		limite = 50
	}
	out := make([]LoteImportado, 0, limite)
	err := s.pool.InTenantTx(ctx, func(tx pgx.Tx) error {
		filas, err := tx.Query(ctx, `
			SELECT id::text, modelo, fichero, filas, creados, actualizados, fallos,
			       por, creada_en, revertida_en
			  FROM importacion
			 WHERE tenant = current_setting('app.tenant', true)
			 ORDER BY creada_en DESC
			 LIMIT $1`, limite)
		if err != nil {
			return err
		}
		defer filas.Close()
		for filas.Next() {
			var l LoteImportado
			if err := filas.Scan(&l.ID, &l.Modelo, &l.Fichero, &l.Filas, &l.Creados,
				&l.Actualizados, &l.Fallos, &l.Por, &l.CreadaEn, &l.RevertidaEn); err != nil {
				return err
			}
			out = append(out, l)
		}
		return filas.Err()
	})
	return out, err
}

// SimularImportacion comprueba un lote SIN ESCRIBIR NADA.
//
// Lo hace dentro de una transacción que SIEMPRE se deshace, y no con una
// validación paralela en memoria: así lo que se comprueba es lo mismo que
// pasaría de verdad —las relaciones contra las filas que hay, los CHECK de la
// base, los disparadores— y no una aproximación que se desalinea a la primera.
func (s *Store) SimularImportacion(ctx context.Context, m *catalogo.Modelo, filas []FilaImportada, por string) (ResultadoImportacion, error) {
	res := ResultadoImportacion{Filas: len(filas), Fallos: []FalloDeFila{}}
	errPrueba := errors.New("simulación: se deshace a propósito")
	err := s.escritura(ctx, func(tx pgx.Tx) error {
		var lote string
		if err := tx.QueryRow(ctx, `
			INSERT INTO importacion (tenant, modelo, fichero, por)
			VALUES (current_setting('app.tenant', true), $1, '(simulación)', $2)
			RETURNING id::text`, m.Clave, por).Scan(&lote); err != nil {
			return err
		}
		for _, f := range filas {
			creado, err := escribirFila(ctx, tx, m, lote, f, por)
			if err != nil {
				res.Fallos = append(res.Fallos, FalloDeFila{f.Linea, traducirError(err).Error()})
				continue
			}
			if creado {
				res.Creados++
			} else {
				res.Actualizados++
			}
		}
		// Y aquí se cae a propósito: la transacción entera se deshace.
		return errPrueba
	})
	if err != nil && !errors.Is(err, errPrueba) {
		return ResultadoImportacion{}, err
	}
	return res, nil
}
