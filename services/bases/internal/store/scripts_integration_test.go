// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

//go:build integration

package store

import (
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"testing"

	"github.com/jackc/pgx/v5"

	"git.sytue.eu/sytue/one/platform/pkg/guiones"
	"git.sytue.eu/sytue/one/services/bases/internal/domain"
)

// manifiesto devuelve el manifiesto y su forma canónica, como hace la capa HTTP.
func manifiesto(t *testing.T, crudo string) (domain.ManifiestoScript, []byte) {
	t.Helper()
	m, err := domain.ParseManifiesto([]byte(crudo))
	if err != nil {
		t.Fatal(err)
	}
	canon, _ := json.Marshal(m)
	return m, canon
}

const soloLeer = `{"v":1,"ops":["records.list"],"write":false}`

func (e *entorno) script(a domain.Actor, ambito, crudo string) (domain.Script, domain.VersionScript) {
	e.t.Helper()
	m, canon := manifiesto(e.t, crudo)
	sc, v, err := e.s.CrearScript(e.ctx, a, e.base.ID, "Guion", ambito, "sytue.v1.log(1)", m, canon)
	e.ok(err)
	return sc, v
}

// aprobado deja un script compartido de `autor` con su versión 1 aprobada por ana.
func (e *entorno) aprobado(autor domain.Actor, crudo string) domain.Script {
	e.t.Helper()
	sc, v := e.script(autor, domain.ScriptCompartido, crudo)
	e.ok(e.s.ProponerVersionScript(e.ctx, autor, e.base.ID, sc.ID, 1))
	e.ok(e.s.RevisarVersionScript(e.ctx, ana, e.base.ID, sc.ID, 1, v.CodeHash, true))
	return sc
}

func (e *entorno) runs() []domain.EjecucionScript {
	e.t.Helper()
	es, err := e.s.EjecucionesDeScripts(e.ctx, ana, e.base.ID, 0, 200)
	e.ok(err)
	return es
}

// Ciclo entero: borrador → propuesta → aprobada, y quién ve y ejecuta qué en cada paso.
func TestScriptsCicloDeVidaYVisibilidad(t *testing.T) {
	e := nuevo(t)
	e.conceder(bea, domain.RolEditor)
	e.conceder(cleo, domain.RolLector)

	sc, v1 := e.script(bea, domain.ScriptPrivado, soloLeer)
	if v1.Version != 1 || v1.Status != guiones.EstadoBorrador || v1.CodeHash != guiones.HashCodigo("sytue.v1.log(1)") {
		t.Fatalf("versión 1: %+v", v1)
	}
	// Privado: no existe para nadie más, tampoco para la propietaria de la base.
	for _, otro := range []domain.Actor{ana, cleo} {
		_, _, _, err := e.s.VerScript(e.ctx, otro, e.base.ID, sc.ID)
		esperar(t, err, domain.ErrNoExiste, "ver un privado ajeno ("+otro.Sub+")")
		_, err = e.s.AutorizarEjecucion(e.ctx, otro, e.base.ID, sc.ID, 1)
		esperar(t, err, domain.ErrNoExiste, "ejecutar un privado ajeno ("+otro.Sub+")")
		esperar(t, e.s.BorrarScript(e.ctx, otro, e.base.ID, sc.ID), domain.ErrNoExiste, "borrar un privado ajeno ("+otro.Sub+")")
		if l, _, err := e.s.ListarScripts(e.ctx, otro, e.base.ID); err != nil || len(l) != 0 {
			t.Errorf("%s lista privados ajenos: %v %v", otro.Sub, l, err)
		}
	}
	// La autora prueba su borrador.
	aut, err := e.s.AutorizarEjecucion(e.ctx, bea, e.base.ID, sc.ID, 1)
	e.ok(err)
	if aut.Code != "sytue.v1.log(1)" || aut.CodeHash != v1.CodeHash || aut.RunID == 0 {
		t.Errorf("autorizada: %+v", aut)
	}
	// Un privado no se propone.
	esperar(t, e.s.ProponerVersionScript(e.ctx, bea, e.base.ID, sc.ID, 1), domain.ErrEstadoScript, "proponer un privado")

	// Compartido y sin aprobar: lo ve quien revisa (la propietaria), no un lector; y no lo ejecutan.
	e.ok(e.s.CambiarAmbitoScript(e.ctx, bea, e.base.ID, sc.ID, domain.ScriptCompartido))
	_, _, _, err = e.s.VerScript(e.ctx, cleo, e.base.ID, sc.ID)
	esperar(t, err, domain.ErrNoExiste, "ver un compartido sin aprobar")
	_, vs, revisa, err := e.s.VerScript(e.ctx, ana, e.base.ID, sc.ID)
	if err != nil || !revisa || len(vs) != 1 || vs[0].Code == "" {
		t.Errorf("la propietaria revisa con el código delante: %+v %v %v", vs, revisa, err)
	}
	_, err = e.s.AutorizarEjecucion(e.ctx, ana, e.base.ID, sc.ID, 1)
	esperar(t, err, domain.ErrScriptSinAprobar, "ni la propietaria ejecuta una versión sin aprobar")

	// Revisar: solo pendiente, solo quien revisa, y con el hash del código revisado.
	esperar(t, e.s.RevisarVersionScript(e.ctx, ana, e.base.ID, sc.ID, 1, v1.CodeHash, true), domain.ErrEstadoScript, "aprobar un borrador")
	esperar(t, e.s.ProponerVersionScript(e.ctx, ana, e.base.ID, sc.ID, 1), domain.ErrSinPermiso, "proponer lo de otra")
	e.ok(e.s.ProponerVersionScript(e.ctx, bea, e.base.ID, sc.ID, 1))
	esperar(t, e.s.RevisarVersionScript(e.ctx, bea, e.base.ID, sc.ID, 1, v1.CodeHash, true), domain.ErrSinPermiso, "la autora (editora) se aprueba a sí misma")
	esperar(t, e.s.RevisarVersionScript(e.ctx, ana, e.base.ID, sc.ID, 1, strings.Repeat("0", 64), true), domain.ErrHashNoCoincide, "aprobar con otro hash")
	e.ok(e.s.RevisarVersionScript(e.ctx, ana, e.base.ID, sc.ID, 1, v1.CodeHash, true))
	esperar(t, e.s.RevisarVersionScript(e.ctx, ana, e.base.ID, sc.ID, 1, v1.CodeHash, false), domain.ErrEstadoScript, "revisar dos veces")

	// Aprobada: la ejecuta un lector (solo lee) y ve SOLO la versión aprobada.
	_, err = e.s.AutorizarEjecucion(e.ctx, cleo, e.base.ID, sc.ID, 1)
	e.ok(err)
	m2, canon2 := manifiesto(t, `{"v":1,"ops":["records.create"],"write":true}`)
	v2, err := e.s.NuevaVersionScript(e.ctx, bea, e.base.ID, sc.ID, "sytue.v1.log(2)", m2, canon2)
	e.ok(err)
	if v2.Version != 2 || v2.Status != guiones.EstadoBorrador {
		t.Errorf("editar no hereda la aprobación: %+v", v2)
	}
	_, vs, _, err = e.s.VerScript(e.ctx, cleo, e.base.ID, sc.ID)
	if err != nil || len(vs) != 1 || vs[0].Version != 1 {
		t.Errorf("un lector solo ve la aprobada: %+v %v", vs, err)
	}
	_, err = e.s.AutorizarEjecucion(e.ctx, cleo, e.base.ID, sc.ID, 2)
	esperar(t, err, domain.ErrScriptSinAprobar, "ejecutar la versión nueva sin aprobar")
	_, err = e.s.NuevaVersionScript(e.ctx, cleo, e.base.ID, sc.ID, "x", m2, canon2)
	esperar(t, err, domain.ErrSinPermiso, "versión nueva de quien no es la autora")
	esperar(t, e.s.CambiarAmbitoScript(e.ctx, ana, e.base.ID, sc.ID, domain.ScriptPrivado), domain.ErrSinPermiso, "cambiar el ámbito de lo ajeno")

	// La propietaria puede aprobar el suyo (decisión del dueño, ADR 0069).
	propio := e.aprobado(ana, soloLeer)
	if _, err := e.s.AutorizarEjecucion(e.ctx, cleo, e.base.ID, propio.ID, 1); err != nil {
		t.Errorf("el script de la propietaria, aprobado por ella: %v", err)
	}

	// Borrar: un lector no; la propietaria, lo compartido. El registro sobrevive.
	esperar(t, e.s.BorrarScript(e.ctx, cleo, e.base.ID, sc.ID), domain.ErrSinPermiso, "un lector borra")
	antes := len(e.runs())
	e.ok(e.s.BorrarScript(e.ctx, ana, e.base.ID, sc.ID))
	_, _, _, err = e.s.VerScript(e.ctx, bea, e.base.ID, sc.ID)
	esperar(t, err, domain.ErrNoExiste, "ver tras borrar")
	if len(e.runs()) != antes || antes == 0 {
		t.Errorf("el registro no sobrevive al borrado: %d → %d", antes, len(e.runs()))
	}
}

// IDOR: un script de otra base, de otra organización o de alguien de fuera no existe,
// y las tablas de un manifiesto han de ser de SU base.
func TestScriptsIDOR(t *testing.T) {
	e := nuevo(t)
	sc := e.aprobado(ana, soloLeer)
	e.conceder(bea, domain.RolEditor)

	// Otra base de la MISMA organización, donde bea es la propietaria.
	otra, err := e.s.CrearBase(e.ctx, bea, "De bea", "")
	e.ok(err)
	tablaDeBea, err := e.s.CrearTabla(e.ctx, bea, otra.ID, "T")
	e.ok(err)
	m, canon := manifiesto(t, soloLeer)
	// El script de ana, pedido por la base de bea: no existe, ni para verlo, ni para
	// ejecutarlo, ni para revisarlo (bea es propietaria de ESA base, no de la del script).
	_, _, _, err = e.s.VerScript(e.ctx, bea, otra.ID, sc.ID)
	esperar(t, err, domain.ErrNoExiste, "ver por otra base")
	_, err = e.s.AutorizarEjecucion(e.ctx, bea, otra.ID, sc.ID, 1)
	esperar(t, err, domain.ErrNoExiste, "ejecutar por otra base")
	_, err = e.s.NuevaVersionScript(e.ctx, bea, otra.ID, sc.ID, "x", m, canon)
	esperar(t, err, domain.ErrNoExiste, "versión por otra base")
	esperar(t, e.s.BorrarScript(e.ctx, bea, otra.ID, sc.ID), domain.ErrNoExiste, "borrar por otra base")
	pend, vp := e.script(bea, domain.ScriptCompartido, soloLeer)
	e.ok(e.s.ProponerVersionScript(e.ctx, bea, e.base.ID, pend.ID, 1))
	esperar(t, e.s.RevisarVersionScript(e.ctx, bea, otra.ID, pend.ID, 1, vp.CodeHash, true), domain.ErrNoExiste, "aprobar por la base donde sí es propietaria")
	// El registro de una base no se lee por otra, ni lo lee quien no revisa.
	_, err = e.s.EjecucionesDeScripts(e.ctx, bea, e.base.ID, 0, 10)
	esperar(t, err, domain.ErrSinPermiso, "registro de una base ajena")
	// Una ejecución no se cierra por otra base ni por otra persona.
	aut, err := e.s.AutorizarEjecucion(e.ctx, bea, e.base.ID, sc.ID, 1)
	e.ok(err)
	esperar(t, e.s.CerrarEjecucion(e.ctx, bea, otra.ID, aut.RunID, "ok", "", 1, 1), domain.ErrNoExiste, "cerrar por otra base")
	esperar(t, e.s.CerrarEjecucion(e.ctx, ana, e.base.ID, aut.RunID, "ok", "", 1, 1), domain.ErrNoExiste, "cerrar la de otra persona")
	e.ok(e.s.CerrarEjecucion(e.ctx, bea, e.base.ID, aut.RunID, "ok", "", 12, 3))
	esperar(t, e.s.CerrarEjecucion(e.ctx, bea, e.base.ID, aut.RunID, "error", "x", 1, 1), domain.ErrNoExiste, "cerrar dos veces")

	// Un manifiesto no puede nombrar una tabla de otra base, aunque quien guarda la lea.
	mAjena, canonAjena := manifiesto(t, `{"v":1,"ops":["records.list"],"tables":["`+tablaDeBea.ID+`"],"write":false}`)
	_, _, err = e.s.CrearScript(e.ctx, bea, e.base.ID, "Guion", domain.ScriptPrivado, "x", mAjena, canonAjena)
	esperar(t, err, domain.ErrInvalido, "manifiesto con tabla de otra base")
	_, err = e.s.NuevaVersionScript(e.ctx, bea, e.base.ID, pend.ID, "x", mAjena, canonAjena)
	esperar(t, err, domain.ErrInvalido, "versión con tabla de otra base")

	// Otra organización: nada existe, tampoco con los ids en la mano.
	_, _, err = e.s.ListarScripts(e.ctx, gus, e.base.ID)
	esperar(t, err, domain.ErrNoExiste, "listar desde otra organización")
	_, _, _, err = e.s.VerScript(e.ctx, gus, e.base.ID, sc.ID)
	esperar(t, err, domain.ErrNoExiste, "ver desde otra organización")
	_, err = e.s.AutorizarEjecucion(e.ctx, gus, e.base.ID, sc.ID, 1)
	esperar(t, err, domain.ErrNoExiste, "ejecutar desde otra organización")
	_, _, err = e.s.CrearScript(e.ctx, gus, e.base.ID, "Guion", domain.ScriptPrivado, "x", m, canon)
	esperar(t, err, domain.ErrNoExiste, "crear desde otra organización")
	// Y con una base propia de gus: el script de acme sigue sin existir.
	suya, err := e.s.CrearBase(e.ctx, gus, "Suya", "")
	e.ok(err)
	_, err = e.s.AutorizarEjecucion(e.ctx, gus, suya.ID, sc.ID, 1)
	esperar(t, err, domain.ErrNoExiste, "script de otra organización por una base propia")

	// Invitada de otra organización (ADR 0040) con acceso de editora: los scripts no son para ella.
	_, err = e.s.ConcederPermiso(e.ctx, ana, e.base.ID, domain.Permiso{PrincipalKind: domain.PrincipalExterno, PrincipalID: "gus", Role: domain.RolEditor})
	e.ok(err)
	_, _, err = e.s.ListarScripts(e.ctx, gusEnAcme, e.base.ID)
	esperar(t, err, domain.ErrSinPermiso, "una invitada lista scripts")
	_, err = e.s.AutorizarEjecucion(e.ctx, gusEnAcme, e.base.ID, sc.ID, 1)
	esperar(t, err, domain.ErrSinPermiso, "una invitada ejecuta")
	_, _, err = e.s.CrearScript(e.ctx, gusEnAcme, e.base.ID, "Guion", domain.ScriptPrivado, "x", m, canon)
	esperar(t, err, domain.ErrSinPermiso, "una invitada crea")
}

// Un script nunca puede más que quien lo ejecuta: el servidor no autoriza una versión
// que escribe a quien no escribe, y lo anota; perder el acceso corta también ejecutar.
func TestScriptsPermisoDelActorYDenegadas(t *testing.T) {
	e := nuevo(t)
	otraTabla, err := e.s.CrearTabla(e.ctx, ana, e.base.ID, "Pedidos")
	e.ok(err)
	escribe := e.aprobado(ana, `{"v":1,"ops":["records.create"],"write":true}`)
	acotado := e.aprobado(ana, `{"v":1,"ops":["records.create"],"tables":["`+otraTabla.ID+`"],"write":true}`)
	lee := e.aprobado(ana, soloLeer)

	_, err = e.s.AutorizarEjecucion(e.ctx, bea, e.base.ID, escribe.ID, 1)
	esperar(t, err, domain.ErrNoExiste, "sin acceso a la base")
	e.conceder(bea, domain.RolLector)
	_, err = e.s.AutorizarEjecucion(e.ctx, bea, e.base.ID, escribe.ID, 1)
	esperar(t, err, domain.ErrSinPermiso, "una lectora ejecuta un script que escribe")
	_, err = e.s.AutorizarEjecucion(e.ctx, bea, e.base.ID, acotado.ID, 1)
	esperar(t, err, domain.ErrSinPermiso, "una lectora ejecuta un script que escribe en una tabla")
	if _, err := e.s.AutorizarEjecucion(e.ctx, bea, e.base.ID, lee.ID, 1); err != nil {
		t.Errorf("una lectora ejecuta un script que solo lee: %v", err)
	}
	// Editora SOLO de esa tabla: el script acotado a ella sí; el que escribe en toda la base, no.
	_, err = e.s.ConcederPermiso(e.ctx, ana, e.base.ID, domain.Permiso{TableID: otraTabla.ID, PrincipalKind: "user", PrincipalID: "bea", Role: domain.RolEditor})
	e.ok(err)
	if _, err := e.s.AutorizarEjecucion(e.ctx, bea, e.base.ID, acotado.ID, 1); err != nil {
		t.Errorf("editora de la tabla ejecuta el script acotado a ella: %v", err)
	}
	_, err = e.s.AutorizarEjecucion(e.ctx, bea, e.base.ID, escribe.ID, 1)
	esperar(t, err, domain.ErrSinPermiso, "editora de una tabla ejecuta un script que escribe en toda la base")

	// Los rechazos quedan anotados con su código y sin gastar cuota; los intentos sin
	// acceso a la base (404) no dejan rastro.
	var denegadas, corriendo int
	for _, r := range e.runs() {
		switch {
		case r.Outcome == guiones.ResDenegado && r.ErrorCode == "forbidden" && r.Sub == "bea":
			denegadas++
		case r.Outcome == guiones.ResEjecutando:
			corriendo++
		default:
			t.Errorf("línea inesperada: %+v", r)
		}
	}
	if denegadas != 3 || corriendo != 2 {
		t.Errorf("registro: %d denegadas y %d en curso, esperadas 3 y 2", denegadas, corriendo)
	}
	uso, err := e.s.UsoCuotaScripts(e.ctx, bea)
	e.ok(err)
	if uso.EjecucionesUsuario != 2 || uso.EjecucionesOrg != 2 {
		t.Errorf("las denegadas gastan cuota: %+v", uso)
	}

	// Borrar la tabla de un manifiesto deja la versión sin poder ejecutarse.
	e.ok(e.s.BorrarTabla(e.ctx, ana, otraTabla.ID))
	_, err = e.s.AutorizarEjecucion(e.ctx, ana, e.base.ID, acotado.ID, 1)
	esperar(t, err, domain.ErrInvalido, "manifiesto con una tabla que ya no existe")
	// Quitar el acceso corta ejecutar en el acto.
	ps, err := e.s.ListarPermisos(e.ctx, ana, e.base.ID)
	e.ok(err)
	for _, p := range ps {
		e.ok(e.s.RevocarPermiso(e.ctx, ana, e.base.ID, p.ID))
	}
	_, err = e.s.AutorizarEjecucion(e.ctx, bea, e.base.ID, lee.ID, 1)
	esperar(t, err, domain.ErrNoExiste, "ejecutar tras perder el acceso")
}

// Las cuotas son fijas y no se cuelan dos ejecuciones por el último hueco.
func TestScriptsCuotas(t *testing.T) {
	e := nuevo(t)
	sc := e.aprobado(ana, `{"v":1,"ops":["records.list"],"write":false,"max_ms":100}`)

	// Tiempo: lo reservado cuenta hasta que se cierra con lo medido.
	largo := e.aprobado(ana, `{"v":1,"ops":["records.list"],"write":false,"max_ms":30000}`)
	caben := int(guiones.CuotaMsUsuarioHora / 30000)
	abiertas := make([]int64, 0, caben)
	for i := range caben {
		aut, err := e.s.AutorizarEjecucion(e.ctx, ana, e.base.ID, largo.ID, 1)
		if err != nil {
			t.Fatalf("ejecución %d: %v", i, err)
		}
		abiertas = append(abiertas, aut.RunID)
	}
	_, err := e.s.AutorizarEjecucion(e.ctx, ana, e.base.ID, sc.ID, 1)
	esperar(t, err, domain.ErrCuotaScripts, "tiempo reservado agotado")
	for _, id := range abiertas {
		e.ok(e.s.CerrarEjecucion(e.ctx, ana, e.base.ID, id, "ok", "", 10, 1))
	}
	// Cerradas con 10 ms cada una, vuelve a haber tiempo; quedan 60 − 20 ejecuciones.
	quedan := guiones.CuotaEjecucionesUsuarioHora - len(abiertas)
	var wg sync.WaitGroup
	var mu sync.Mutex
	pasan, cuota := 0, 0
	for range quedan + 8 {
		wg.Go(func() {
			_, err := e.s.AutorizarEjecucion(e.ctx, ana, e.base.ID, sc.ID, 1)
			mu.Lock()
			defer mu.Unlock()
			switch {
			case err == nil:
				pasan++
			case strings.Contains(fmt.Sprint(err), "cuota"):
				cuota++
			default:
				t.Errorf("error inesperado: %v", err)
			}
		})
	}
	wg.Wait()
	if pasan != quedan || cuota != 8 {
		t.Errorf("a la vez: pasan %d (esperadas %d) y %d por cuota (esperadas 8)", pasan, quedan, cuota)
	}
	// La cuota de una persona no es la de otra; la de la organización es de todos.
	e.conceder(bea, domain.RolLector)
	if _, err := e.s.AutorizarEjecucion(e.ctx, bea, e.base.ID, sc.ID, 1); err != nil {
		t.Errorf("bea tiene su propia cuota: %v", err)
	}
	uso, err := e.s.UsoCuotaScripts(e.ctx, bea)
	e.ok(err)
	if uso.EjecucionesUsuario != 1 || uso.EjecucionesOrg != guiones.CuotaEjecucionesUsuarioHora+1 {
		t.Errorf("uso: %+v", uso)
	}
	// Y la de otra organización no cuenta aquí.
	if uso, err := e.s.UsoCuotaScripts(e.ctx, gus); err != nil || uso.EjecucionesOrg != 0 {
		t.Errorf("la cuota de otra organización: %+v %v", uso, err)
	}
}

// Una versión no se reescribe, tampoco por SQL directo; el estado sí cambia.
func TestScriptsVersionInmutablePorSQL(t *testing.T) {
	e := nuevo(t)
	sc, _ := e.script(ana, domain.ScriptCompartido, soloLeer)
	for _, sql := range []string{
		`UPDATE base_script_versions SET code = 'otro' WHERE script_id = $1`,
		`UPDATE base_script_versions SET code_hash = repeat('0', 64) WHERE script_id = $1`,
		`UPDATE base_script_versions SET manifest = '{"v":1,"ops":["records.delete"],"write":true}' WHERE script_id = $1`,
		`UPDATE base_script_versions SET version = 9 WHERE script_id = $1`,
	} {
		err := e.s.en(e.ctx, ana, func(tx pgx.Tx) error {
			_, err := tx.Exec(e.ctx, sql, sc.ID)
			return err
		})
		if err == nil || !strings.Contains(err.Error(), "inmutables") {
			t.Errorf("%s: %v", sql, err)
		}
	}
	e.ok(e.s.ProponerVersionScript(e.ctx, ana, e.base.ID, sc.ID, 1))
}

// Los límites y el borrado de la base.
func TestScriptsLimitesYCascada(t *testing.T) {
	e := nuevo(t)
	sc, _ := e.script(ana, domain.ScriptPrivado, soloLeer)
	aut, err := e.s.AutorizarEjecucion(e.ctx, ana, e.base.ID, sc.ID, 1)
	e.ok(err)
	m, canon := manifiesto(t, soloLeer)
	e.ok(e.s.en(e.ctx, ana, func(tx pgx.Tx) error {
		_, err := tx.Exec(e.ctx, `INSERT INTO base_scripts (tenant, base_id, name, author_sub) SELECT 'acme', $1, 'relleno', 'ana' FROM generate_series(2, $2)`, e.base.ID, domain.MaxScriptsPorBase)
		return err
	}))
	_, _, err = e.s.CrearScript(e.ctx, ana, e.base.ID, "Uno más", domain.ScriptPrivado, "x", m, canon)
	esperar(t, err, domain.ErrLimite, "más scripts de la cuenta")
	e.ok(e.s.en(e.ctx, ana, func(tx pgx.Tx) error {
		_, err := tx.Exec(e.ctx, `INSERT INTO base_script_versions (tenant, script_id, version, code, code_hash, manifest, created_by)
			SELECT 'acme', $1, n, 'x', repeat('0', 64), '{}', 'ana' FROM generate_series(2, $2) n`, sc.ID, domain.MaxVersionesDeScript)
		return err
	}))
	_, err = e.s.NuevaVersionScript(e.ctx, ana, e.base.ID, sc.ID, "x", m, canon)
	esperar(t, err, domain.ErrLimite, "más versiones de la cuenta")

	// Borrar la base se lleva scripts y versiones; el registro de ejecuciones se queda.
	e.ok(e.s.BorrarBase(e.ctx, ana, e.base.ID))
	var scripts, versiones, runs int
	e.ok(e.s.en(e.ctx, ana, func(tx pgx.Tx) error {
		return tx.QueryRow(e.ctx, `SELECT (SELECT count(*) FROM base_scripts), (SELECT count(*) FROM base_script_versions),
			(SELECT count(*) FROM base_script_runs WHERE id = $1)`, aut.RunID).Scan(&scripts, &versiones, &runs)
	}))
	if scripts != 0 || versiones != 0 || runs != 1 {
		t.Errorf("tras borrar la base: %d scripts, %d versiones, %d líneas del registro", scripts, versiones, runs)
	}
}
