// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

//go:build integration

package store

import (
	"slices"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"

	"git.sytue.eu/sytue/one/services/bases/internal/domain"
)

func (e *entorno) conceder(quien domain.Actor, rol domain.Rol) {
	e.t.Helper()
	_, err := e.s.ConcederPermiso(e.ctx, ana, e.base.ID, domain.Permiso{PrincipalKind: "user", PrincipalID: quien.Sub, Role: rol})
	e.ok(err)
}

func (e *entorno) comentar(a domain.Actor, registro, padre, texto string) domain.Comentario {
	e.t.Helper()
	c, _, err := e.s.CrearComentario(e.ctx, a, registro, padre, texto)
	e.ok(err)
	return c
}

// El comentarista se distingue POR FIN del lector: comenta, pero no edita. Y borrar es
// de quien lo escribió (mientras pueda comentar) o de quien modera.
func TestComentariosMatrizDeRoles(t *testing.T) {
	e := nuevo(t)
	r := e.registro(ana, e.tabl.ID, dato(e.txt.ID, `"Ana"`))

	// Sin acceso: ni leer, ni comentar, ni se revela que existe.
	_, err := e.s.ListarComentarios(e.ctx, bea, r.ID)
	esperar(t, err, domain.ErrNoExiste, "sin acceso, listar")
	_, _, err = e.s.CrearComentario(e.ctx, bea, r.ID, "", "hola")
	esperar(t, err, domain.ErrNoExiste, "sin acceso, comentar")

	// Lector: lee el hilo, no comenta.
	deAna := e.comentar(ana, r.ID, "", "primero")
	e.conceder(bea, domain.RolLector)
	if cs, err := e.s.ListarComentarios(e.ctx, bea, r.ID); err != nil || len(cs) != 1 || cs[0].CanDelete {
		t.Errorf("el lector lee el hilo sin poder borrar: %+v %v", cs, err)
	}
	_, _, err = e.s.CrearComentario(e.ctx, bea, r.ID, "", "hola")
	esperar(t, err, domain.ErrSinPermiso, "lector comenta")
	esperar(t, e.s.BorrarComentario(e.ctx, bea, deAna.ID), domain.ErrSinPermiso, "lector borra")

	// Comentarista: comenta, no edita el registro ni borra lo ajeno; lo suyo sí.
	e.conceder(bea, domain.RolComentarista)
	e.conceder(cleo, domain.RolComentarista)
	deBea := e.comentar(bea, r.ID, "", "de bea")
	_, err = e.s.ActualizarRegistro(e.ctx, bea, r.ID, r.Rev, dato(e.txt.ID, `"x"`), nil)
	esperar(t, err, domain.ErrSinPermiso, "comentarista edita el registro")
	_, err = e.s.CrearRegistro(e.ctx, bea, e.tabl.ID, nil, nil)
	esperar(t, err, domain.ErrSinPermiso, "comentarista crea registros")
	esperar(t, e.s.BorrarComentario(e.ctx, cleo, deBea.ID), domain.ErrSinPermiso, "comentarista borra lo ajeno")
	esperar(t, e.s.BorrarComentario(e.ctx, bea, deAna.ID), domain.ErrSinPermiso, "comentarista borra lo de la propietaria")
	cs, err := e.s.ListarComentarios(e.ctx, bea, r.ID)
	e.ok(err)
	if len(cs) != 2 || cs[0].CanDelete || !cs[1].CanDelete {
		t.Errorf("can_delete del comentarista: %+v", cs)
	}

	// Degradada a lectora, ya no borra ni lo que escribió.
	e.conceder(bea, domain.RolLector)
	esperar(t, e.s.BorrarComentario(e.ctx, bea, deBea.ID), domain.ErrSinPermiso, "lectora borra lo que escribió de comentarista")
	e.conceder(bea, domain.RolComentarista)
	e.ok(e.s.BorrarComentario(e.ctx, bea, deBea.ID))

	// Editor: modera (borra lo de otros).
	deCleo := e.comentar(cleo, r.ID, "", "de cleo")
	e.conceder(bea, domain.RolEditor)
	e.ok(e.s.BorrarComentario(e.ctx, bea, deCleo.ID))
	esperar(t, e.s.BorrarComentario(e.ctx, bea, deCleo.ID), domain.ErrNoExiste, "borrar dos veces")

	// La traza registra el alta y la baja, y NO guarda el texto.
	es, err := e.s.ListarAuditoria(e.ctx, ana, e.base.ID, 0, 200)
	e.ok(err)
	var altas, bajas int
	for _, x := range es {
		switch x.Action {
		case "comment.create":
			altas++
		case "comment.delete":
			bajas++
		default:
			continue
		}
		if x.RecordID != r.ID || strings.Contains(string(x.After)+string(x.Before), "de cleo") {
			t.Errorf("entrada de traza de comentario: %+v", x)
		}
	}
	if altas != 3 || bajas != 2 {
		t.Errorf("traza: %d altas y %d bajas, esperadas 3 y 2", altas, bajas)
	}
}

// Nada de un comentario cruza registros, tablas, bases ni organizaciones.
func TestComentariosIDOR(t *testing.T) {
	e := nuevo(t)
	r := e.registro(ana, e.tabl.ID, nil)
	r2 := e.registro(ana, e.tabl.ID, nil)
	c := e.comentar(ana, r.ID, "", "raíz")
	resp := e.comentar(ana, r.ID, c.ID, "respuesta")

	// Otra organización: nada existe, ni con los ids en la mano.
	_, err := e.s.ListarComentarios(e.ctx, gus, r.ID)
	esperar(t, err, domain.ErrNoExiste, "otra organización lista")
	_, _, err = e.s.CrearComentario(e.ctx, gus, r.ID, "", "x")
	esperar(t, err, domain.ErrNoExiste, "otra organización comenta")
	esperar(t, e.s.BorrarComentario(e.ctx, gus, c.ID), domain.ErrNoExiste, "otra organización borra")
	_, err = e.s.RolEnRegistro(e.ctx, gus, r.ID)
	esperar(t, err, domain.ErrNoExiste, "otra organización, rol")

	// gus, propietario de SU base, no cuelga una respuesta de un hilo de acme.
	bg, _ := e.s.CrearBase(e.ctx, gus, "Suya", "")
	tg, _ := e.s.CrearTabla(e.ctx, gus, bg.ID, "T")
	rg := e.registro(gus, tg.ID, nil)
	_, _, err = e.s.CrearComentario(e.ctx, gus, rg.ID, c.ID, "x")
	esperar(t, err, domain.ErrInvalido, "respuesta a un hilo de otra organización")

	// El hilo es del registro: ni desde otro registro, ni una respuesta a una respuesta.
	_, _, err = e.s.CrearComentario(e.ctx, ana, r2.ID, c.ID, "x")
	esperar(t, err, domain.ErrInvalido, "respuesta a un hilo de otro registro")
	_, _, err = e.s.CrearComentario(e.ctx, ana, r.ID, resp.ID, "x")
	esperar(t, err, domain.ErrInvalido, "respuesta a una respuesta")

	// Acceso a UNA tabla: cleo comenta en la suya y no ve los comentarios de la otra.
	t2, err := e.s.CrearTabla(e.ctx, ana, e.base.ID, "Segunda")
	e.ok(err)
	_, err = e.s.ConcederPermiso(e.ctx, ana, e.base.ID, domain.Permiso{TableID: t2.ID, PrincipalKind: "user", PrincipalID: "cleo", Role: domain.RolComentarista})
	e.ok(err)
	rt2 := e.registro(ana, t2.ID, nil)
	e.comentar(cleo, rt2.ID, "", "en mi tabla")
	_, err = e.s.ListarComentarios(e.ctx, cleo, r.ID)
	esperar(t, err, domain.ErrNoExiste, "comentarista de otra tabla lista")
	_, _, err = e.s.CrearComentario(e.ctx, cleo, r.ID, "", "x")
	esperar(t, err, domain.ErrNoExiste, "comentarista de otra tabla comenta")
	esperar(t, e.s.BorrarComentario(e.ctx, cleo, c.ID), domain.ErrNoExiste, "comentarista de otra tabla borra")

	// Y aunque el código se equivocara: la clave compuesta impide el comentario cruzado.
	err = e.h.Pool.InMergeTx(e.ctx, func(tx pgx.Tx) error {
		_, err := tx.Exec(e.ctx, `INSERT INTO base_comments (tenant, base_id, table_id, record_id, author_sub, body)
			VALUES ('globex', $1, $2, $3, 'gus', 'colado')`, e.base.ID, e.tabl.ID, r.ID)
		return err
	})
	if !esFK(err) {
		t.Errorf("un comentario de globex sobre un registro de acme debe violar la clave foránea: %v", err)
	}
}

// Entradas hostiles y límites.
func TestComentariosEntradaYLimites(t *testing.T) {
	e := nuevo(t)
	r := e.registro(ana, e.tabl.ID, nil)
	for _, malo := range []string{"", "  \n ", "a\x00b", strings.Repeat("a", domain.MaxLongComentario+1)} {
		_, _, err := e.s.CrearComentario(e.ctx, ana, r.ID, "", malo)
		esperar(t, err, domain.ErrInvalido, "texto no válido")
	}
	e.ok(e.s.en(e.ctx, ana, func(tx pgx.Tx) error {
		_, err := tx.Exec(e.ctx, `INSERT INTO base_comments (tenant, base_id, table_id, record_id, author_sub, body)
			SELECT 'acme', $1, $2, $3, 'ana', 'x' FROM generate_series(1, $4)`, e.base.ID, e.tabl.ID, r.ID, domain.MaxComentariosPorRegistro)
		return err
	}))
	_, _, err := e.s.CrearComentario(e.ctx, ana, r.ID, "", "uno más")
	esperar(t, err, domain.ErrLimite, "tope de comentarios por registro")
	if cs, err := e.s.ListarComentarios(e.ctx, ana, r.ID); err != nil || len(cs) != domain.MaxComentariosPorRegistro {
		t.Errorf("el listado va acotado: %d %v", len(cs), err)
	}
}

// Borrar una raíz con respuestas deja la lápida (sin texto ni autor); al irse la última
// respuesta, la lápida también. Y borrar el registro se lleva su hilo.
func TestComentariosLapidaYCascada(t *testing.T) {
	e := nuevo(t)
	r := e.registro(ana, e.tabl.ID, nil)
	e.conceder(bea, domain.RolComentarista)
	raiz := e.comentar(bea, r.ID, "", "secreto de bea")
	resp := e.comentar(ana, r.ID, raiz.ID, "respuesta de ana")

	e.ok(e.s.BorrarComentario(e.ctx, bea, raiz.ID))
	cs, err := e.s.ListarComentarios(e.ctx, ana, r.ID)
	e.ok(err)
	if len(cs) != 2 || !cs[0].Deleted || cs[0].Body != "" || cs[0].AuthorSub != "" || cs[0].CanDelete || cs[1].ID != resp.ID {
		t.Fatalf("lápida: %+v", cs)
	}
	// A una lápida no se le responde ni se la borra otra vez.
	_, _, err = e.s.CrearComentario(e.ctx, ana, r.ID, raiz.ID, "x")
	esperar(t, err, domain.ErrInvalido, "respuesta a una lápida")
	esperar(t, e.s.BorrarComentario(e.ctx, ana, raiz.ID), domain.ErrNoExiste, "borrar una lápida")
	e.ok(e.s.BorrarComentario(e.ctx, ana, resp.ID))
	if cs, _ := e.s.ListarComentarios(e.ctx, ana, r.ID); len(cs) != 0 {
		t.Errorf("la lápida debe irse con su última respuesta: %+v", cs)
	}

	e.comentar(ana, r.ID, "", "se va con el registro")
	e.ok(e.s.BorrarRegistro(e.ctx, ana, r.ID, false))
	var n int
	e.ok(e.s.en(e.ctx, ana, func(tx pgx.Tx) error {
		return tx.QueryRow(e.ctx, `SELECT count(*) FROM base_comments`).Scan(&n)
	}))
	if n != 0 {
		t.Errorf("quedan %d comentarios de un registro borrado", n)
	}
}

// Los avisos solo salen hacia quien AHORA puede leer el registro, y nunca a quien comenta.
func TestComentariosDestinatarios(t *testing.T) {
	e := nuevo(t)
	r := e.registro(ana, e.tabl.ID, nil)
	e.conceder(bea, domain.RolComentarista)
	e.conceder(cleo, domain.RolComentarista)

	raiz, av, err := e.s.CrearComentario(e.ctx, bea, r.ID, "", "hola")
	e.ok(err)
	if !slices.Equal(av.Destinatarios, []string{"ana"}) || av.BaseID != e.base.ID || av.TableID != e.tabl.ID || av.BaseName != "Clientes" {
		t.Errorf("comentario nuevo avisa a la propietaria: %+v", av)
	}
	e.comentar(cleo, r.ID, raiz.ID, "yo también")
	_, av, err = e.s.CrearComentario(e.ctx, ana, r.ID, raiz.ID, "respuesta")
	e.ok(err)
	slices.Sort(av.Destinatarios)
	if !slices.Equal(av.Destinatarios, []string{"bea", "cleo"}) {
		t.Errorf("la respuesta avisa al hilo, no a quien escribe: %+v", av.Destinatarios)
	}

	// A bea se le retira el acceso: sigue en el hilo, pero ya no se le avisa.
	ps, err := e.s.ListarPermisos(e.ctx, ana, e.base.ID)
	e.ok(err)
	for _, p := range ps {
		if p.PrincipalID == "bea" {
			e.ok(e.s.RevocarPermiso(e.ctx, ana, e.base.ID, p.ID))
		}
	}
	_, av, err = e.s.CrearComentario(e.ctx, ana, r.ID, raiz.ID, "sin bea")
	e.ok(err)
	if !slices.Equal(av.Destinatarios, []string{"cleo"}) {
		t.Errorf("se avisa a quien ya no puede leer el registro: %+v", av.Destinatarios)
	}
}
