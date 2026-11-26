// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package erp

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"github.com/google/uuid"

	"git.sytue.eu/sytue/one/apps/nucleo/erp/catalogo"
	"git.sytue.eu/sytue/one/apps/nucleo/erp/importar"
)

// LA IMPORTACIÓN COMO LOTE, con simulacro y marcha atrás (entrega 3): `store/importacion.go`
// del servidor sobre SQLite. La lectura del CSV es la misma (apps/nucleo/erp/importar);
// cada fila se escribe en su punto de guardado, así que una mala no tira el lote, y cada
// fila escrita queda apuntada con lo que había antes para poder deshacerla. Simular es
// importar de verdad dentro de la transacción y deshacerla entera.

const esquemaImportacion = `
CREATE TABLE IF NOT EXISTS importacion (
  id           TEXT PRIMARY KEY,
  modelo       TEXT NOT NULL,
  fichero      TEXT NOT NULL DEFAULT '',
  filas        INTEGER NOT NULL DEFAULT 0,
  creados      INTEGER NOT NULL DEFAULT 0,
  actualizados INTEGER NOT NULL DEFAULT 0,
  fallos       INTEGER NOT NULL DEFAULT 0,
  creada       TEXT NOT NULL,
  revertida    TEXT
);
CREATE TABLE IF NOT EXISTS importacion_fila (
  id             INTEGER PRIMARY KEY AUTOINCREMENT,
  importacion    TEXT NOT NULL REFERENCES importacion(id),
  registro       TEXT NOT NULL,
  accion         TEXT NOT NULL CHECK (accion IN ('creado', 'actualizado')),
  datos_antes    TEXT,
  etiqueta_antes TEXT NOT NULL DEFAULT '',
  estado_antes   TEXT NOT NULL DEFAULT '',
  editado_tras   TEXT NOT NULL
);
CREATE INDEX IF NOT EXISTS importacion_fila_lote ON importacion_fila(importacion);
`

// ErrYaRevertida: el lote ya se deshizo.
var ErrYaRevertida = errors.New("erp: esa importación ya se revirtió")

// ResultadoImportacion es lo que pasó con un lote (la forma de `store.ResultadoImportacion`).
type ResultadoImportacion struct {
	ID           string           `json:"id,omitempty"`
	Filas        int              `json:"filas"`
	Creados      int              `json:"creados"`
	Actualizados int              `json:"actualizados"`
	Fallos       []importar.Fallo `json:"fallos"`
}

// Reversion es lo que pasó al deshacer un lote.
type Reversion struct {
	Borrados    int `json:"borrados"`
	Restaurados int `json:"restaurados"`
	Intactos    int `json:"intactos"`
}

// LoteImportado es una importación de la lista.
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

var errSimulacion = errors.New("erp: simulación, se deshace a propósito")

// Importar escribe el lote entero; con `simular`, lo escribe y lo deshace.
func (a *Almacen) Importar(ctx context.Context, m *catalogo.Modelo, filas []importar.Fila, fichero string, simular bool) (ResultadoImportacion, error) {
	res := ResultadoImportacion{Filas: len(filas), Fallos: []importar.Fallo{}}
	if simular {
		fichero = "(simulación)"
	}
	err := a.tx(ctx, func(tx *sql.Tx) error {
		id := uuid.NewString()
		if _, err := tx.ExecContext(ctx, `INSERT INTO importacion (id, modelo, fichero, creada) VALUES (?, ?, ?, ?)`,
			id, m.Clave, fichero, a.marcaAhora()); err != nil {
			return err
		}
		for _, f := range filas {
			if _, err := tx.ExecContext(ctx, `SAVEPOINT fila`); err != nil {
				return err
			}
			creado, err := a.aplicarFila(ctx, tx, m, id, f)
			if err != nil {
				if _, e := tx.ExecContext(ctx, `ROLLBACK TO fila`); e != nil {
					return e
				}
				_, _ = tx.ExecContext(ctx, `RELEASE fila`)
				res.Fallos = append(res.Fallos, importar.Fallo{Fila: f.Linea, Error: mensajeDe(err)})
				continue
			}
			if _, err := tx.ExecContext(ctx, `RELEASE fila`); err != nil {
				return err
			}
			if creado {
				res.Creados++
			} else {
				res.Actualizados++
			}
		}
		if simular {
			return errSimulacion
		}
		res.ID = id
		_, err := tx.ExecContext(ctx, `UPDATE importacion SET filas = ?, creados = ?, actualizados = ?, fallos = ? WHERE id = ?`,
			res.Filas, res.Creados, res.Actualizados, len(res.Fallos), id)
		return err
	})
	if err != nil && !errors.Is(err, errSimulacion) {
		return ResultadoImportacion{}, err
	}
	return res, nil
}

// mensajeDe es el error de una fila como lo lee quien importa.
func mensajeDe(err error) string {
	if strings.Contains(err.Error(), "UNIQUE") {
		return "A document with that reference already exists."
	}
	return err.Error()
}

// aplicarFila crea la fila o, si trae `externo_id` y ya existe, la actualiza; y la apunta.
func (a *Almacen) aplicarFila(ctx context.Context, tx *sql.Tx, m *catalogo.Modelo, lote string, f importar.Fila) (bool, error) {
	if err := comprobarRelaciones(ctx, tx, m, f.Data); err != nil {
		return false, err
	}
	var previo *struct {
		id, datos, etiqueta, estado string
	}
	if f.Externo != "" {
		r, ok, err := a.buscarPor(ctx, tx, m.Clave, "externo_id", f.Externo)
		if err != nil {
			return false, err
		}
		if ok {
			crudo, _ := json.Marshal(r.Data)
			previo = &struct{ id, datos, etiqueta, estado string }{r.ID, string(crudo), r.Etiqueta, r.Estado}
		}
	}
	if previo == nil {
		r, err := a.insertarEn(ctx, tx, m, f.Data, f.Etiqueta, f.Estado)
		if err != nil {
			return false, err
		}
		return true, apuntarFila(ctx, tx, lote, r.ID, "creado", nil, "", "", r.EditadoEn.UTC().Format(marca))
	}
	crudo, err := json.Marshal(f.Data)
	if err != nil {
		return false, err
	}
	ahora := a.marcaAhora()
	if _, err := tx.ExecContext(ctx, `UPDATE registro SET data = ?, etiqueta = CASE WHEN ? <> '' THEN ? ELSE etiqueta END,
		estado = CASE WHEN ? <> '' THEN ? ELSE estado END, editado = ? WHERE id = ?`,
		string(crudo), f.Etiqueta, f.Etiqueta, f.Estado, f.Estado, ahora, previo.id); err != nil {
		return false, err
	}
	if err := anotar(ctx, tx, m.Clave, previo.id, "cambio", nil, "", ahora); err != nil {
		return false, err
	}
	return false, apuntarFila(ctx, tx, lote, previo.id, "actualizado", &previo.datos, previo.etiqueta, previo.estado, ahora)
}

func apuntarFila(ctx context.Context, tx *sql.Tx, lote, registro, accion string, antes *string, etiqueta, estado, tras string) error {
	_, err := tx.ExecContext(ctx, `INSERT INTO importacion_fila (importacion, registro, accion, datos_antes, etiqueta_antes, estado_antes, editado_tras)
		VALUES (?, ?, ?, ?, ?, ?, ?)`, lote, registro, accion, antes, etiqueta, estado, tras)
	return err
}

// Revertir deshace un lote: borra (a la papelera) lo creado y devuelve lo actualizado a
// como estaba, salvo lo que alguien tocó después, que no se pisa (Intactos).
func (a *Almacen) Revertir(ctx context.Context, id string) (Reversion, error) {
	var out Reversion
	err := a.tx(ctx, func(tx *sql.Tx) error {
		var revertida sql.NullString
		err := tx.QueryRowContext(ctx, `SELECT revertida FROM importacion WHERE id = ?`, id).Scan(&revertida)
		if errors.Is(err, sql.ErrNoRows) {
			return ErrNoEncontrado
		}
		if err != nil {
			return err
		}
		if revertida.Valid {
			return ErrYaRevertida
		}
		ahora := a.marcaAhora()
		res, err := tx.ExecContext(ctx, `UPDATE registro SET borrado = ?, editado = ?
			WHERE borrado IS NULL AND id IN (SELECT f.registro FROM importacion_fila f
			  WHERE f.importacion = ? AND f.accion = 'creado' AND f.editado_tras = registro.editado)`, ahora, ahora, id)
		if err != nil {
			return err
		}
		n, _ := res.RowsAffected()
		out.Borrados = int(n)
		res, err = tx.ExecContext(ctx, `UPDATE registro SET
			  data = (SELECT f.datos_antes FROM importacion_fila f WHERE f.importacion = ? AND f.registro = registro.id AND f.accion = 'actualizado'),
			  etiqueta = COALESCE(NULLIF((SELECT f.etiqueta_antes FROM importacion_fila f WHERE f.importacion = ? AND f.registro = registro.id AND f.accion = 'actualizado'), ''), etiqueta),
			  estado = COALESCE(NULLIF((SELECT f.estado_antes FROM importacion_fila f WHERE f.importacion = ? AND f.registro = registro.id AND f.accion = 'actualizado'), ''), estado),
			  editado = ?
			WHERE borrado IS NULL AND id IN (SELECT f.registro FROM importacion_fila f
			  WHERE f.importacion = ? AND f.accion = 'actualizado' AND f.datos_antes IS NOT NULL AND f.editado_tras = registro.editado)`,
			id, id, id, ahora, id)
		if err != nil {
			return err
		}
		n, _ = res.RowsAffected()
		out.Restaurados = int(n)
		var total int
		if err := tx.QueryRowContext(ctx, `SELECT count(*) FROM importacion_fila WHERE importacion = ?`, id).Scan(&total); err != nil {
			return err
		}
		out.Intactos = total - out.Borrados - out.Restaurados
		_, err = tx.ExecContext(ctx, `UPDATE importacion SET revertida = ? WHERE id = ?`, ahora, id)
		return err
	})
	return out, err
}

// Importaciones son los últimos lotes, el último primero.
func (a *Almacen) Importaciones(ctx context.Context) ([]LoteImportado, error) {
	filas, err := a.db.QueryContext(ctx, `SELECT id, modelo, fichero, filas, creados, actualizados, fallos, creada, revertida
		FROM importacion ORDER BY creada DESC LIMIT 50`)
	if err != nil {
		return nil, err
	}
	defer func() { _ = filas.Close() }()
	out := []LoteImportado{}
	for filas.Next() {
		var l LoteImportado
		var creada string
		var revertida sql.NullString
		if err := filas.Scan(&l.ID, &l.Modelo, &l.Fichero, &l.Filas, &l.Creados, &l.Actualizados, &l.Fallos, &creada, &revertida); err != nil {
			return nil, err
		}
		l.Por = Persona
		l.CreadaEn, _ = time.Parse(marca, creada)
		if revertida.Valid {
			t, _ := time.Parse(marca, revertida.String)
			l.RevertidaEn = &t
		}
		out = append(out, l)
	}
	return out, filas.Err()
}
