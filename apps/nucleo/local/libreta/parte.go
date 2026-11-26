// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package libreta

import (
	"context"
	"database/sql"
	"strings"

	"git.sytue.eu/sytue/one/apps/nucleo/contacts"
)

// # La parte de la libreta que mira una escritura
//
// Una escritura no lee la libreta entera: lee, por índice, las filas que la función de
// Contacts que la prepara mira y toca, y hace con ellas una instantánea (parte). Siempre
// van ENTERAS las cosas que entran: un contacto con su tarjeta, correos y teléfonos; una
// etiqueta con su tarjeta, todos sus miembros y los contactos que estos nombran; y todas
// las libretas (son pocas). Así la instantánea es válida por sí sola, y lo que Contacts
// calcula de cada cosa que hay en ella (su vista, sus miembros) es lo mismo que en la
// libreta entera. Leer de más no cambia nada; leer de menos, sí: cada escritura dice qué
// necesita (escrituras.go).

// maxVariables es cuántos valores se pasan como mucho en un IN (…).
const maxVariables = 500

// parte son las filas leídas para una escritura, sin repetir.
type parte struct {
	ctx       context.Context
	q         consulta
	filas     map[string]contacts.Row
	contactos map[string]bool // ya leídos enteros (o que no existen)
	grupos    map[string]bool
}

func nuevaParte(ctx context.Context, q consulta) *parte {
	return &parte{ctx: ctx, q: q, filas: map[string]contacts.Row{}, contactos: map[string]bool{}, grupos: map[string]bool{}}
}

func clave(kind, id string) string { return kind + "\x00" + id }

// Las consultas de las filas de un contacto y de una etiqueta enteros, cada una de una
// vez y por índice (TestLasConsultasVanPorIndice).
const (
	sqlLibretas       = `SELECT clase, id, json FROM fila WHERE clase = 'book'`
	sqlContactoEntero = `SELECT clase, id, json FROM fila WHERE clase = 'contact' AND id = ?1
		UNION ALL SELECT f.clase, f.id, f.json FROM fila c JOIN fila f ON f.clase = 'card' AND f.id = c.objeto WHERE c.clase = 'contact' AND c.id = ?1
		UNION ALL SELECT clase, id, json FROM fila WHERE contacto = ?1 AND clase IN ('email', 'phone')`
	sqlGrupoEntero = `SELECT clase, id, json FROM fila WHERE clase = 'group' AND id = ?1
		UNION ALL SELECT f.clase, f.id, f.json FROM fila g JOIN fila f ON f.clase = 'card' AND f.id = g.objeto WHERE g.clase = 'group' AND g.id = ?1
		UNION ALL SELECT clase, id, json FROM fila WHERE grupo = ?1 AND clase = 'member'`
)

// leerFilas añade las filas que da la consulta q (con sus args).
func (p *parte) leerFilas(q string, args ...any) ([]contacts.Row, error) {
	rs, err := p.q.QueryContext(p.ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rs.Close() }()
	var out []contacts.Row
	for rs.Next() {
		var r contacts.Row
		var datos string
		if err := rs.Scan(&r.Kind, &r.ID, &datos); err != nil {
			return nil, err
		}
		r.JSON = []byte(datos)
		p.filas[clave(r.Kind, r.ID)] = r
		out = append(out, r)
	}
	return out, rs.Err()
}

// libretas añade todas las libretas.
func (p *parte) libretas() error {
	_, err := p.leerFilas(sqlLibretas)
	return err
}

// contacto añade el contacto id entero: su fila, su tarjeta, sus correos y teléfonos.
func (p *parte) contacto(id string) error {
	if p.contactos[id] {
		return nil
	}
	p.contactos[id] = true
	_, err := p.leerFilas(sqlContactoEntero, id)
	return err
}

// Los de muchos contactos enteros, de maxVariables en maxVariables.
const (
	sqlContactosFilas    = `SELECT clase, id, json FROM fila WHERE clase = 'contact' AND id IN (`
	sqlContactosTarjetas = `SELECT f.clase, f.id, f.json FROM fila c JOIN fila f ON f.clase = 'card' AND f.id = c.objeto WHERE c.clase = 'contact' AND c.id IN (`
	sqlContactosCanales  = `SELECT clase, id, json FROM fila WHERE clase IN ('email', 'phone') AND contacto IN (`
)

// contactosEnteros añade enteros los contactos ids (como contacto, pero de tres
// consultas por cada maxVariables: una etiqueta puede nombrar a miles).
func (p *parte) contactosEnteros(ids []string) error {
	var nuevos []string
	for _, id := range ids {
		if !p.contactos[id] {
			p.contactos[id] = true
			nuevos = append(nuevos, id)
		}
	}
	return enTrozos(nuevos, func(trozo []string) error {
		for _, q := range []string{sqlContactosFilas, sqlContactosTarjetas, sqlContactosCanales} {
			if _, err := p.leerFilas(q+marcas(len(trozo))+`)`, argumentos(trozo)...); err != nil {
				return err
			}
		}
		return nil
	})
}

// grupo añade la etiqueta id entera: su fila, su tarjeta, todos sus miembros y, entero,
// cada contacto que nombran.
func (p *parte) grupo(id string) error {
	if p.grupos[id] {
		return nil
	}
	p.grupos[id] = true
	filas, err := p.leerFilas(sqlGrupoEntero, id)
	if err != nil {
		return err
	}
	var nombrados []string
	for _, r := range filas {
		if r.Kind != "member" {
			continue
		}
		if c, err := columnasDe(r); err == nil && c.contacto != "" {
			nombrados = append(nombrados, c.contacto)
		}
	}
	return p.contactosEnteros(nombrados)
}

// contactosDonde añade enteros los contactos de la tabla contacto que cumplen donde.
func (p *parte) contactosDonde(donde string, args ...any) error {
	//nolint:gosec // G202: donde son condiciones fijas de este paquete, con `?`; los valores van en args
	rs, err := p.q.QueryContext(p.ctx, `SELECT id FROM contacto WHERE `+donde, args...)
	if err != nil {
		return err
	}
	ids, err := columna(rs)
	if err != nil {
		return err
	}
	return p.contactosEnteros(ids)
}

// contactosCon añade enteros los contactos cuya columna col (de la tabla contacto) es
// uno de valores, y que además cumplen y (si no es "").
func (p *parte) contactosCon(col string, valores []string, y string, args ...any) error {
	return enTrozos(valores, func(trozo []string) error {
		donde := col + ` IN (` + marcas(len(trozo)) + `)`
		if y != "" {
			donde += ` AND ` + y
		}
		return p.contactosDonde(donde, append(argumentos(trozo), args...)...)
	})
}

// gruposDonde añade enteras las etiquetas de la tabla grupo que cumplen donde.
func (p *parte) gruposDonde(donde string, args ...any) error {
	//nolint:gosec // G202: donde son condiciones fijas de este paquete, con `?`; los valores van en args
	rs, err := p.q.QueryContext(p.ctx, `SELECT id FROM grupo WHERE `+donde, args...)
	if err != nil {
		return err
	}
	ids, err := columna(rs)
	if err != nil {
		return err
	}
	for _, id := range ids {
		if err := p.grupo(id); err != nil {
			return err
		}
	}
	return nil
}

// gruposQueNombran añade enteras las etiquetas con algún miembro que nombra a uno de
// esos contactos (lo que cambia al eliminarlos para siempre).
func (p *parte) gruposQueNombran(ids []string) error {
	return enTrozos(ids, func(trozo []string) error {
		//nolint:gosec // G202: solo las marcas `?` del IN; los valores van en args
		rs, err := p.q.QueryContext(p.ctx, `SELECT DISTINCT grupo FROM fila WHERE clase = 'member' AND contacto IN (`+marcas(len(trozo))+`)`, argumentos(trozo)...)
		if err != nil {
			return err
		}
		grupos, err := columna(rs)
		if err != nil {
			return err
		}
		for _, g := range grupos {
			if err := p.grupo(g); err != nil {
				return err
			}
		}
		return nil
	})
}

// instantanea es la instantánea de Contacts de lo leído.
func (p *parte) instantanea(x *Libreta) (*contacts.Snapshot, error) {
	rows := make([]contacts.Row, 0, len(p.filas))
	for _, r := range p.filas {
		rows = append(rows, r)
	}
	return contacts.NewSnapshot(x.scope, rows)
}

// columna lee la primera columna de cada fila de rs (y lo cierra).
func columna(rs *sql.Rows) ([]string, error) {
	defer func() { _ = rs.Close() }()
	var out []string
	for rs.Next() {
		var v string
		if err := rs.Scan(&v); err != nil {
			return nil, err
		}
		out = append(out, v)
	}
	return out, rs.Err()
}

// enTrozos llama a f con valores en trozos de maxVariables como mucho.
func enTrozos(valores []string, f func([]string) error) error {
	for i := 0; i < len(valores); i += maxVariables {
		if err := f(valores[i:min(i+maxVariables, len(valores))]); err != nil {
			return err
		}
	}
	return nil
}

// marcas son n marcas «?» separadas por comas.
func marcas(n int) string { return strings.TrimSuffix(strings.Repeat("?,", n), ",") }

func argumentos(valores []string) []any {
	out := make([]any, len(valores))
	for i, v := range valores {
		out[i] = v
	}
	return out
}
