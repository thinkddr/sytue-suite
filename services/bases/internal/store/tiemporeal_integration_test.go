// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

//go:build integration

package store

import (
	"encoding/json"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"git.sytue.eu/sytue/one/platform/pkg/pg"
	"git.sytue.eu/sytue/one/services/bases/internal/domain"
)

// desdeAhora abre el flujo de `a` y lo deja AL DÍA: el cursor desde el que se le contará
// solo lo que pase a partir de aquí. Abrir empieza en la transacción más antigua aún en
// curso (de cualquier base del mismo Postgres: otros tests en paralelo), así que puede
// traer cambios de la preparación; se consumen hasta pasar una marca que no viaja por el flujo.
func (e *entorno) desdeAhora(a domain.Actor) domain.Cursor {
	e.t.Helper()
	n, err := e.s.Novedades(e.ctx, a, e.base.ID, nil)
	e.ok(err)
	if len(n.Cambios) != 0 {
		e.t.Fatalf("al abrir no se entrega nada en la primera tanda: %+v", n.Cambios)
	}
	cursor := n.Cursor
	var marca int64
	e.ok(e.s.en(e.ctx, ana, func(tx pgx.Tx) error {
		return tx.QueryRow(e.ctx, `INSERT INTO base_audit (tenant, base_id, actor, action) VALUES ('acme', $1, 'test', 'test.marca') RETURNING id`, e.base.ID).Scan(&marca)
	}))
	for limite := time.Now().Add(30 * time.Second); cursor.ID != marca; {
		if time.Now().After(limite) {
			e.t.Fatal("el flujo no llega a la marca")
		}
		time.Sleep(10 * time.Millisecond)
		n, err := e.s.Novedades(e.ctx, a, e.base.ID, &cursor)
		e.ok(err)
		cursor = n.Cursor
	}
	return cursor
}

// cambios lee el flujo hasta juntar `cuantos` cambios (los entrega cuando ninguna
// transacción anterior sigue abierta; otros tests del mismo Postgres pueden retrasarlo).
func (e *entorno) cambios(a domain.Actor, cursor *domain.Cursor, cuantos int) []string {
	e.t.Helper()
	var out []string
	limite := time.Now().Add(30 * time.Second)
	for {
		n, err := e.s.Novedades(e.ctx, a, e.base.ID, cursor)
		e.ok(err)
		*cursor = n.Cursor
		for _, c := range n.Cambios {
			out = append(out, c.Action+":"+c.Actor)
		}
		if len(out) >= cuantos || time.Now().After(limite) {
			return out
		}
		time.Sleep(20 * time.Millisecond)
	}
}

func (e *entorno) celda(a domain.Actor, registro, campo, valor string) ResultadoCelda {
	e.t.Helper()
	r, err := e.s.EscribirCelda(e.ctx, a, registro, Celda{FieldID: campo, Value: json.RawMessage(valor)})
	e.ok(err)
	return r
}

func TestFlujoDeCambiosEnOrdenYConReanudacion(t *testing.T) {
	e := nuevo(t)
	e.conceder(bea, domain.RolEditor)
	cursor := e.desdeAhora(ana)

	r := e.registro(ana, e.tabl.ID, dato(e.txt.ID, `"Ana"`))
	e.celda(bea, r.ID, e.txt.ID, `"Bea"`)
	e.comentar(bea, r.ID, "", "hola")
	// Lo que no cambia lo que otra persona tiene en pantalla no viaja por el flujo.
	e.ok(e.s.ExportarTabla(e.ctx, ana, e.tabl.ID, func(Exportacion) error { return nil }))

	inicio := cursor
	got := e.cambios(ana, &cursor, 3)
	if !slices.Equal(got, []string{"record.create:ana", "record.update:bea", "comment.create:bea"}) {
		t.Fatalf("cambios: %v", got)
	}
	if nada := e.cambios(ana, &cursor, 0); len(nada) != 0 {
		t.Errorf("con el cursor al día no hay nada que contar: %v", nada)
	}
	// Reanudar desde un cursor anterior vuelve a contar lo mismo, en el mismo orden.
	if otra := e.cambios(ana, &inicio, 3); !slices.Equal(otra, got) {
		t.Errorf("replay: %v", otra)
	}
	// Un cambio de permisos sale como «access», sin decir quién ni a quién.
	e.conceder(cleo, domain.RolLector)
	n, err := e.s.Novedades(e.ctx, bea, e.base.ID, &cursor)
	e.ok(err)
	for i := 0; len(n.Cambios) == 0 && i < 200; i++ {
		time.Sleep(20 * time.Millisecond)
		n, err = e.s.Novedades(e.ctx, bea, e.base.ID, &cursor)
		e.ok(err)
	}
	if len(n.Cambios) != 1 || n.Cambios[0].Action != "access" || n.Cambios[0].Actor != "" || n.Cambios[0].RecordID != "" {
		t.Errorf("cambio de acceso: %+v", n.Cambios)
	}
}

// Una transacción lenta no deja un hueco: lo que confirma tarde, con un id menor que
// otros ya escritos, se entrega igualmente y ANTES que lo posterior.
func TestFlujoDeCambiosSinHuecosConTransaccionesLentas(t *testing.T) {
	e := nuevo(t)
	r := e.registro(ana, e.tabl.ID, nil)
	cursor := e.desdeAhora(ana)

	lenta, err := e.h.Pool.Begin(e.ctx)
	e.ok(err)
	defer func() { _ = lenta.Rollback(e.ctx) }()
	e.ok(pg.SetTenant(e.ctx, lenta, "acme"))
	_, err = lenta.Exec(e.ctx, `INSERT INTO base_audit (tenant, base_id, table_id, record_id, actor, action) VALUES ('acme', $1, $2, $3, 'lenta', 'record.update')`,
		e.base.ID, e.tabl.ID, r.ID)
	e.ok(err)

	// Mientras sigue abierta, otra escritura confirma.
	e.celda(ana, r.ID, e.txt.ID, `"rápida"`)
	time.Sleep(150 * time.Millisecond)
	n, err := e.s.Novedades(e.ctx, ana, e.base.ID, &cursor)
	e.ok(err)
	if len(n.Cambios) != 0 {
		t.Fatalf("se entregó un cambio con una transacción anterior aún abierta (dejaría un hueco): %+v", n.Cambios)
	}
	e.ok(lenta.Commit(e.ctx))
	if got := e.cambios(ana, &cursor, 2); !slices.Equal(got, []string{"record.update:lenta", "record.update:ana"}) {
		t.Errorf("tras confirmar la lenta: %v", got)
	}
}

func TestFlujoDeCambiosSeAutorizaEnCadaTanda(t *testing.T) {
	e := nuevo(t)
	t2, err := e.s.CrearTabla(e.ctx, ana, e.base.ID, "Segunda")
	e.ok(err)
	c2, err := e.s.CrearCampo(e.ctx, ana, t2.ID, domain.EntradaCampo{Name: "N", Kind: domain.TipoTexto})
	e.ok(err)
	// cleo solo accede a la segunda tabla; bea a ninguna; gus es de otra organización.
	_, err = e.s.ConcederPermiso(e.ctx, ana, e.base.ID, domain.Permiso{TableID: t2.ID, PrincipalKind: "user", PrincipalID: "cleo", Role: domain.RolLector})
	e.ok(err)
	for nombre, a := range map[string]domain.Actor{"sin acceso": bea, "otra organización": gus, "de fuera sin invitación": gusEnAcme} {
		_, err := e.s.Novedades(e.ctx, a, e.base.ID, nil)
		esperar(t, err, domain.ErrNoExiste, nombre)
		_, err = e.s.Novedades(e.ctx, a, e.base.ID, &domain.Cursor{})
		esperar(t, err, domain.ErrNoExiste, nombre+", con cursor")
	}

	cursor := e.desdeAhora(cleo)
	r1 := e.registro(ana, e.tabl.ID, dato(e.txt.ID, `"secreto de la primera"`))
	r2 := e.registro(ana, t2.ID, dato(c2.ID, `"de la segunda"`))
	e.celda(ana, r1.ID, e.txt.ID, `"x"`)
	e.celda(ana, r2.ID, c2.ID, `"y"`)
	e.ok(e.s.ActualizarBase(e.ctx, ana, e.base.ID, "Renombrada", ""))
	var n Novedades
	var vistos []domain.Cambio
	for i := 0; len(vistos) < 2 && i < 300; i++ {
		n, err = e.s.Novedades(e.ctx, cleo, e.base.ID, &cursor)
		e.ok(err)
		cursor = n.Cursor
		vistos = append(vistos, n.Cambios...)
		time.Sleep(20 * time.Millisecond)
	}
	if len(vistos) != 2 {
		t.Fatalf("cleo debe ver solo los dos cambios de SU tabla (ni los de la otra, ni el de la base): %+v", vistos)
	}
	for _, c := range vistos {
		if c.TableID != t2.ID || c.RecordID != r2.ID {
			t.Errorf("cleo ve un cambio de otra tabla: %+v", c)
		}
	}
	// Por el flujo no viaja ningún dato: ni el valor, ni el antes ni el después.
	if b, _ := json.Marshal(vistos); strings.Contains(string(b), "segunda") || strings.Contains(string(b), `"y"`) {
		t.Errorf("el flujo lleva datos: %s", b)
	}

	// Se le retira el acceso: la tanda siguiente ya no existe para ella.
	ps, err := e.s.ListarPermisos(e.ctx, ana, e.base.ID)
	e.ok(err)
	e.ok(e.s.RevocarPermiso(e.ctx, ana, e.base.ID, ps[0].ID))
	_, err = e.s.Novedades(e.ctx, cleo, e.base.ID, &cursor)
	esperar(t, err, domain.ErrNoExiste, "tras retirar el acceso")
}

func TestFlujoDeCambiosQuienSeQuedaAtrasRecarga(t *testing.T) {
	e := nuevo(t)
	cursor := e.desdeAhora(ana)
	e.ok(e.s.en(e.ctx, ana, func(tx pgx.Tx) error {
		_, err := tx.Exec(e.ctx, `INSERT INTO base_audit (tenant, base_id, table_id, actor, action)
			SELECT 'acme', $1, $2, 'ana', 'record.update' FROM generate_series(1, $3)`, e.base.ID, e.tabl.ID, domain.MaxCambiosPorTanda+1)
		return err
	}))
	var n Novedades
	var err error
	for i := 0; !n.Atras && i < 300; i++ {
		n, err = e.s.Novedades(e.ctx, ana, e.base.ID, &cursor)
		e.ok(err)
		time.Sleep(20 * time.Millisecond)
	}
	if !n.Atras || len(n.Cambios) != 0 {
		t.Fatalf("quien se queda atrás recibe resync y ningún cambio: atrás=%v, %d cambios", n.Atras, len(n.Cambios))
	}
	// Y sigue desde ahora: lo que venga después sí se le cuenta.
	cursor = n.Cursor
	e.registro(ana, e.tabl.ID, nil)
	if got := e.cambios(ana, &cursor, 1); !slices.Equal(got, []string{"record.create:ana"}) {
		t.Errorf("tras el resync: %v", got)
	}
	for _, malo := range []string{"", "1", "a-b", "-1-2", "1--2", "1-2-3", strings.Repeat("9", 30) + "-1"} {
		if _, err := domain.LeerCursor(malo); err == nil {
			t.Errorf("cursor %q debería rechazarse", malo)
		}
	}
}

// Operaciones de celda: último escritor gana y se avisa, sin estorbarse entre celdas
// distintas, y la misma operación repetida no se aplica dos veces.
func TestEscribirCelda(t *testing.T) {
	e := nuevo(t)
	r := e.registro(ana, e.tabl.ID, dato(e.txt.ID, `"uno"`, e.num.ID, `1`))
	e.conceder(bea, domain.RolEditor)
	e.conceder(cleo, domain.RolComentarista)

	// Dos personas, dos celdas del mismo registro: ninguna pisa a la otra ni hay conflicto de versión.
	e.celda(ana, r.ID, e.txt.ID, `"de ana"`)
	out := e.celda(bea, r.ID, e.num.ID, `2`)
	if string(out.Record.Data[e.txt.ID]) != `"de ana"` || string(out.Record.Data[e.num.ID]) != `2` || out.Conflict || out.Record.Rev != r.Rev+2 {
		t.Errorf("celdas distintas: %+v", out)
	}

	// La misma celda: gana quien escribe después; si lo que veía ya no estaba, se le avisa.
	visto := json.RawMessage(`"de ana"`)
	e.celda(bea, r.ID, e.txt.ID, `"de bea"`)
	out, err := e.s.EscribirCelda(e.ctx, ana, r.ID, Celda{FieldID: e.txt.ID, Value: json.RawMessage(`"otra vez ana"`), Expected: visto})
	e.ok(err)
	if !out.Conflict || string(out.Record.Data[e.txt.ID]) != `"otra vez ana"` {
		t.Errorf("último escritor gana y avisa: %+v", out)
	}
	out, err = e.s.EscribirCelda(e.ctx, bea, r.ID, Celda{FieldID: e.txt.ID, Value: json.RawMessage(`"bea al día"`), Expected: json.RawMessage(`"otra vez ana"`)})
	e.ok(err)
	if out.Conflict {
		t.Error("quien veía el valor vigente no pisa nada")
	}
	// Vaciar una celda que se veía vacía tampoco es un conflicto.
	out, err = e.s.EscribirCelda(e.ctx, bea, r.ID, Celda{FieldID: e.num.ID, Value: json.RawMessage(`null`), Expected: json.RawMessage(`2`)})
	e.ok(err)
	if out.Conflict || len(out.Record.Data[e.num.ID]) != 0 {
		t.Errorf("vaciar: %+v", out)
	}

	// Idempotencia: la operación repetida no se vuelve a aplicar, ni aunque otro haya escrito después.
	op := uuid.NewString()
	uno, err := e.s.EscribirCelda(e.ctx, ana, r.ID, Celda{FieldID: e.txt.ID, Value: json.RawMessage(`"con op"`), OpID: op})
	e.ok(err)
	e.celda(bea, r.ID, e.txt.ID, `"posterior de bea"`)
	dos, err := e.s.EscribirCelda(e.ctx, ana, r.ID, Celda{FieldID: e.txt.ID, Value: json.RawMessage(`"con op"`), OpID: op})
	e.ok(err)
	if uno.Replayed || !dos.Replayed || string(dos.Record.Data[e.txt.ID]) != `"posterior de bea"` {
		t.Errorf("la repetición reaplicó la operación: %+v / %+v", uno, dos)
	}
	// Un identificador ya usado en OTRO registro no es una repetición.
	r2 := e.registro(ana, e.tabl.ID, nil)
	_, err = e.s.EscribirCelda(e.ctx, ana, r2.ID, Celda{FieldID: e.txt.ID, Value: json.RawMessage(`"x"`), OpID: op})
	esperar(t, err, domain.ErrInvalido, "operación reutilizada en otro registro")
	// A la vez: la misma operación desde ocho sitios se aplica UNA vez.
	op2 := uuid.NewString()
	antes, err := e.s.ObtenerRegistro(e.ctx, ana, r.ID)
	e.ok(err)
	var wg sync.WaitGroup
	for range 8 {
		wg.Go(func() {
			_, _ = e.s.EscribirCelda(e.ctx, ana, r.ID, Celda{FieldID: e.txt.ID, Value: json.RawMessage(`"a la vez"`), OpID: op2})
		})
	}
	wg.Wait()
	despues, err := e.s.ObtenerRegistro(e.ctx, ana, r.ID)
	e.ok(err)
	if despues.Rev != antes.Rev+1 {
		t.Errorf("la operación concurrente se aplicó %d veces", despues.Rev-antes.Rev)
	}

	// Permisos: escribe el editor; ni el comentarista, ni quien no tiene acceso, ni otra organización.
	_, err = e.s.EscribirCelda(e.ctx, cleo, r.ID, Celda{FieldID: e.txt.ID, Value: json.RawMessage(`"x"`)})
	esperar(t, err, domain.ErrSinPermiso, "comentarista escribe una celda")
	_, err = e.s.EscribirCelda(e.ctx, gus, r.ID, Celda{FieldID: e.txt.ID, Value: json.RawMessage(`"x"`)})
	esperar(t, err, domain.ErrNoExiste, "otra organización escribe una celda")
	_, err = e.s.EscribirCelda(e.ctx, domain.Actor{Sub: "dani", Tenant: "acme"}, r.ID, Celda{FieldID: e.txt.ID, Value: json.RawMessage(`"x"`)})
	esperar(t, err, domain.ErrNoExiste, "sin acceso escribe una celda")
	_, err = e.s.ObtenerRegistro(e.ctx, gus, r.ID)
	esperar(t, err, domain.ErrNoExiste, "otra organización lee un registro")
	if _, err := e.s.ObtenerRegistro(e.ctx, cleo, r.ID); err != nil {
		t.Errorf("quien lee la tabla lee el registro: %v", err)
	}

	// Entradas hostiles: un campo de otra tabla, un valor que no encaja, un campo inventado.
	t2, _ := e.s.CrearTabla(e.ctx, ana, e.base.ID, "Segunda")
	c2, _ := e.s.CrearCampo(e.ctx, ana, t2.ID, domain.EntradaCampo{Name: "N", Kind: domain.TipoTexto})
	for nombre, c := range map[string]Celda{
		"campo de otra tabla": {FieldID: c2.ID, Value: json.RawMessage(`"x"`)},
		"texto en un número":  {FieldID: e.num.ID, Value: json.RawMessage(`"no soy un número"`)},
		"campo inventado":     {FieldID: uuid.NewString(), Value: json.RawMessage(`"x"`)},
	} {
		_, err := e.s.EscribirCelda(e.ctx, ana, r.ID, c)
		esperar(t, err, domain.ErrInvalido, nombre)
	}

	// La operación queda en la traza con su identificador.
	var n int
	e.ok(e.s.en(e.ctx, ana, func(tx pgx.Tx) error {
		return tx.QueryRow(e.ctx, `SELECT count(*) FROM base_audit WHERE op_id = $1 AND action = 'record.update' AND actor = 'ana'`, op).Scan(&n)
	}))
	if n != 1 {
		t.Errorf("filas de traza de la operación: %d", n)
	}
}

func TestEscribirCeldaDeEnlace(t *testing.T) {
	e := nuevo(t)
	pedidos, _ := e.s.CrearTabla(e.ctx, ana, e.base.ID, "Pedidos")
	cli, err := e.s.CrearCampo(e.ctx, ana, pedidos.ID, domain.EntradaCampo{Name: "C", Kind: domain.TipoRelacion, LinkTableID: e.tabl.ID})
	e.ok(err)
	c1 := e.registro(ana, e.tabl.ID, nil)
	c2 := e.registro(ana, e.tabl.ID, nil)
	p := e.registro(ana, pedidos.ID, nil)

	out, err := e.s.EscribirCelda(e.ctx, ana, p.ID, Celda{FieldID: cli.ID, Links: []string{c1.ID}})
	e.ok(err)
	if !slices.Equal(out.Record.Links[cli.ID], []string{c1.ID}) {
		t.Fatalf("enlace escrito: %+v", out.Record.Links)
	}
	// Quien veía la celda vacía pisa el enlace que otra persona puso: gana y se le avisa.
	out, err = e.s.EscribirCelda(e.ctx, ana, p.ID, Celda{FieldID: cli.ID, Links: []string{c2.ID}, Expected: json.RawMessage(`[]`)})
	e.ok(err)
	if !out.Conflict || !slices.Equal(out.Record.Links[cli.ID], []string{c2.ID}) {
		t.Errorf("enlace con conflicto: %+v", out)
	}
	// Un enlace a un registro de OTRA tabla, o que no existe, no entra.
	_, err = e.s.EscribirCelda(e.ctx, ana, p.ID, Celda{FieldID: cli.ID, Links: []string{p.ID}})
	esperar(t, err, domain.ErrInvalido, "enlace a un registro de otra tabla")
	_, err = e.s.EscribirCelda(e.ctx, ana, p.ID, Celda{FieldID: cli.ID, Links: []string{uuid.NewString()}})
	esperar(t, err, domain.ErrInvalido, "enlace a un registro que no existe")
}

// La presencia la autoriza el servicio: quién es lo pone él, la celda solo de una tabla
// que se puede leer, y cada cual ve la celda de los demás solo en las tablas que lee.
func TestPresencia(t *testing.T) {
	e := nuevo(t)
	r := e.registro(ana, e.tabl.ID, nil)
	t2, _ := e.s.CrearTabla(e.ctx, ana, e.base.ID, "Segunda")
	e.conceder(bea, domain.RolLector)
	_, err := e.s.ConcederPermiso(e.ctx, ana, e.base.ID, domain.Permiso{TableID: t2.ID, PrincipalKind: "user", PrincipalID: "cleo", Role: domain.RolLector})
	e.ok(err)
	enCelda := domain.Presente{Conn: "pestana-de-ana", Name: "Ana", TableID: e.tabl.ID, RecordID: r.ID, FieldID: e.txt.ID}
	e.ok(e.s.Latir(e.ctx, ana, e.base.ID, enCelda))

	presentes := func(a domain.Actor) []domain.Presente {
		t.Helper()
		n, err := e.s.Novedades(e.ctx, a, e.base.ID, nil)
		e.ok(err)
		return n.Presentes
	}
	// bea lee la tabla: ve a ana en su celda.
	if ps := presentes(bea); len(ps) != 1 || ps[0].Sub != "ana" || ps[0].Name != "Ana" || ps[0].FieldID != e.txt.ID {
		t.Errorf("bea ve: %+v", ps)
	}
	// cleo no lee esa tabla: sabe que ana está, no en qué celda.
	if ps := presentes(cleo); len(ps) != 1 || ps[0].Sub != "ana" || ps[0].TableID != "" || ps[0].RecordID != "" || ps[0].FieldID != "" {
		t.Errorf("cleo ve la celda de una tabla que no lee: %+v", ps)
	}

	// Nadie se hace pasar por otra pestaña, ni dice estar donde no puede leer.
	esperar(t, e.s.Latir(e.ctx, bea, e.base.ID, domain.Presente{Conn: "pestana-de-ana"}), domain.ErrInvalido, "usar la pestaña de otra persona")
	esperar(t, e.s.Latir(e.ctx, cleo, e.base.ID, domain.Presente{Conn: "pestana-de-cleo", TableID: e.tabl.ID}), domain.ErrNoExiste, "presencia en una tabla que no lee")
	esperar(t, e.s.Latir(e.ctx, gus, e.base.ID, domain.Presente{Conn: "pestana-de-gus"}), domain.ErrNoExiste, "presencia desde otra organización")
	esperar(t, e.s.Latir(e.ctx, domain.Actor{Sub: "dani", Tenant: "acme"}, e.base.ID, domain.Presente{Conn: "pestana-de-dani"}), domain.ErrNoExiste, "presencia sin acceso")
	otra, _ := e.s.CrearBase(e.ctx, ana, "Otra", "")
	esperar(t, e.s.Latir(e.ctx, ana, otra.ID, domain.Presente{Conn: "pestana-de-ana2", TableID: e.tabl.ID}), domain.ErrNoExiste, "tabla de otra base")
	for _, malo := range []string{"", "corta", "con espacio dentro", strings.Repeat("x", 65), "<script>alert(1)</script>"} {
		esperar(t, e.s.Latir(e.ctx, ana, e.base.ID, domain.Presente{Conn: malo}), domain.ErrInvalido, "pestaña no válida")
	}

	// Caduca sola y se va al cerrar.
	e.ok(e.s.Latir(e.ctx, bea, e.base.ID, domain.Presente{Conn: "pestana-de-bea"}))
	e.ok(e.s.Irse(e.ctx, ana, e.base.ID, "pestana-de-bea")) // no es suya: no la quita
	if ps := presentes(ana); len(ps) != 2 {
		t.Errorf("presentes: %+v", ps)
	}
	e.ok(e.s.Irse(e.ctx, bea, e.base.ID, "pestana-de-bea"))
	e.ok(e.s.en(e.ctx, ana, func(tx pgx.Tx) error {
		_, err := tx.Exec(e.ctx, `UPDATE base_presence SET seen = now() - interval '1 minute'`)
		return err
	}))
	if ps := presentes(ana); len(ps) != 0 {
		t.Errorf("una presencia sin latido sigue saliendo: %+v", ps)
	}

	// Tope de pestañas por base.
	e.ok(e.s.en(e.ctx, ana, func(tx pgx.Tx) error {
		_, err := tx.Exec(e.ctx, `INSERT INTO base_presence (tenant, base_id, conn, sub) SELECT 'acme', $1, 'relleno-' || lpad(g::text, 4, '0'), 'ana' FROM generate_series(1, $2) g`,
			e.base.ID, domain.MaxPresentes)
		return err
	}))
	esperar(t, e.s.Latir(e.ctx, ana, e.base.ID, domain.Presente{Conn: "una-mas-de-la-cuenta"}), domain.ErrLimite, "tope de presentes")
}

// Una operación de celda recalcula las fórmulas que dependen de ella, como cualquier
// otra escritura, y una fórmula no se escribe desde fuera tampoco por aquí.
func TestEscribirCeldaRecalculaLasFormulas(t *testing.T) {
	e := nuevo(t)
	doble := e.formula(ana, e.tabl.ID, "Doble", "{"+e.num.ID+"} * 2", domain.TipoNumero)
	r := e.registro(ana, e.tabl.ID, dato(e.num.ID, `2`))
	out := e.celda(ana, r.ID, e.num.ID, `21`)
	if string(out.Record.Data[doble.ID]) != `42` {
		t.Errorf("la fórmula no se recalculó con la celda: %s", out.Record.Data[doble.ID])
	}
	_, err := e.s.EscribirCelda(e.ctx, ana, r.ID, Celda{FieldID: doble.ID, Value: json.RawMessage(`7`)})
	esperar(t, err, domain.ErrInvalido, "escribir una fórmula por una operación de celda")
}
