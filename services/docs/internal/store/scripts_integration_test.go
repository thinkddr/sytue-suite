// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

//go:build integration

package store_test

import (
	"context"
	"errors"
	"testing"

	"github.com/jackc/pgx/v5"

	"git.sytue.eu/sytue/one/services/docs/internal/store"

	"git.sytue.eu/sytue/one/services/docs/internal/domain"
)

const manifiestoPrueba = `{"v":1,"commands":["bold"],"read":[],"write":false,"scope":{},"max_ops":1000,"max_ms":5000,"unattended":false}`

const docPrueba = "0b6f0000-0000-4000-8000-000000000001"

func TestScriptsVersionesInmutablesYAprobacionConHash(t *testing.T) {
	h := harnessComoEnProduccion(t)
	st := store.New(h.Pool)
	ctx := tenantCtx("acme")
	sc, v1, err := st.CrearScript(ctx, domain.Script{Name: "S", Kind: domain.KindSpreadsheet, Scope: domain.AmbitoOrg, AuthorSub: "ana"}, "run()", []byte(manifiestoPrueba))
	if err != nil {
		t.Fatal(err)
	}
	if v1.Version != 1 || v1.Status != domain.EstadoBorrador || v1.CodeHash != domain.HashCodigoScript("run()") {
		t.Fatalf("v1: %+v", v1)
	}
	// Aprobar sin proponer: no. Proponer: sí. Aprobar con un hash ajeno: no.
	if err := st.RevisarVersionScript(ctx, sc.ID, 1, v1.CodeHash, true, "root"); !errors.Is(err, domain.ErrConflictoEstadoScript) {
		t.Fatalf("aprobó un borrador: %v", err)
	}
	if err := st.ProponerVersionScript(ctx, sc.ID, 1); err != nil {
		t.Fatal(err)
	}
	otro := domain.HashCodigoScript("otro código")
	if err := st.RevisarVersionScript(ctx, sc.ID, 1, otro, true, "root"); !errors.Is(err, domain.ErrHashNoCoincide) {
		t.Fatalf("aprobó con el hash de otro código: %v", err)
	}
	if err := st.RevisarVersionScript(ctx, sc.ID, 1, v1.CodeHash, true, "root"); err != nil {
		t.Fatal(err)
	}
	// Ya aprobada no se vuelve a revisar ni a proponer.
	if err := st.RevisarVersionScript(ctx, sc.ID, 1, v1.CodeHash, false, "root"); !errors.Is(err, domain.ErrConflictoEstadoScript) {
		t.Fatalf("rechazó una aprobada: %v", err)
	}
	if err := st.ProponerVersionScript(ctx, sc.ID, 1); !errors.Is(err, domain.ErrConflictoEstadoScript) {
		t.Fatalf("propuso una aprobada: %v", err)
	}
	// Editar crea la versión 2 SIN aprobar.
	v2, err := st.NuevaVersionScript(ctx, sc.ID, "run2()", []byte(manifiestoPrueba), "ana")
	if err != nil || v2.Version != 2 || v2.Status != domain.EstadoBorrador {
		t.Fatalf("v2: %+v %v", v2, err)
	}
	got, _ := st.ObtenerScript(ctx, sc.ID)
	if got.LatestVersion != 2 || got.ApprovedVersion != 1 {
		t.Fatalf("resumen: %+v", got)
	}
	// Inmutabilidad a nivel de SQL: ni el código ni el manifiesto cambian.
	err = h.Pool.InTenantTx(ctx, func(tx pgx.Tx) error {
		_, err := tx.Exec(context.Background(), `UPDATE doc_script_versions SET code = 'troyano()' WHERE script_id = $1 AND version = 1`, sc.ID)
		return err
	})
	if err == nil {
		t.Fatal("el código de una versión aprobada se pudo reescribir")
	}
}

func TestScriptsVisibilidadYAislamientoPorOrganizacion(t *testing.T) {
	st := newStore(t)
	acme := tenantCtx("acme")
	priv, _, _ := st.CrearScript(acme, domain.Script{Name: "privado", Kind: domain.KindDocument, Scope: domain.AmbitoPrivado, AuthorSub: "ana"}, "a()", []byte(manifiestoPrueba))
	org, _, _ := st.CrearScript(acme, domain.Script{Name: "org", Kind: domain.KindDocument, Scope: domain.AmbitoOrg, AuthorSub: "ana"}, "b()", []byte(manifiestoPrueba))

	nombres := func(sub string, admin bool) map[string]bool {
		l, err := st.ListarScripts(acme, sub, admin, 50)
		if err != nil {
			t.Fatal(err)
		}
		m := map[string]bool{}
		for _, s := range l {
			m[s.Name] = true
		}
		return m
	}
	if m := nombres("beto", false); len(m) != 0 {
		t.Fatalf("beto ve %v: nada está aprobado", m)
	}
	if m := nombres("root", true); !m["org"] || m["privado"] {
		t.Fatalf("el administrador debe ver el de la organización y NO el privado: %v", m)
	}
	if m := nombres("ana", false); !m["org"] || !m["privado"] {
		t.Fatalf("la autora ve lo suyo: %v", m)
	}
	_ = st.ProponerVersionScript(acme, org.ID, 1)
	_ = st.RevisarVersionScript(acme, org.ID, 1, domain.HashCodigoScript("b()"), true, "root")
	if m := nombres("beto", false); !m["org"] || m["privado"] {
		t.Fatalf("beto ve lo aprobado y solo eso: %v", m)
	}
	// Otra organización no ve nada, ni por id.
	globex := tenantCtx("globex")
	if l, _ := st.ListarScripts(globex, "ana", true, 50); len(l) != 0 {
		t.Fatalf("fuga entre organizaciones: %v", l)
	}
	if _, err := st.ObtenerScript(globex, priv.ID); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("lee un script de otra organización: %v", err)
	}
}

func TestEjecucionesCuotasYAuditoria(t *testing.T) {
	st := newStore(t)
	ctx := tenantCtx("acme")
	sc, v, _ := st.CrearScript(ctx, domain.Script{Name: "S", Kind: domain.KindDocument, Scope: domain.AmbitoPrivado, AuthorSub: "ana"}, "x()", []byte(manifiestoPrueba))
	e := domain.EjecucionScript{ScriptID: sc.ID, Version: 1, CodeHash: v.CodeHash, DocumentID: docPrueba, Sub: "ana"}

	// Cuota por persona: la 61ª se frena y las demás personas siguen pudiendo.
	var primera int64
	for i := 0; i < domain.CuotaEjecucionesUsuarioHora; i++ {
		id, err := st.ReservarEjecucion(ctx, e, 1000)
		if err != nil {
			t.Fatalf("ejecución %d: %v", i, err)
		}
		if i == 0 {
			primera = id
		}
	}
	if _, err := st.ReservarEjecucion(ctx, e, 1000); !errors.Is(err, domain.ErrCuotaScripts) {
		t.Fatalf("pasó la cuota por persona: %v", err)
	}
	beto := e
	beto.Sub = "beto"
	if _, err := st.ReservarEjecucion(ctx, beto, 1000); err != nil {
		t.Fatalf("la cuota de Ana frena a Beto: %v", err)
	}
	// Los intentos denegados no gastan cuota.
	if _, err := st.ReservarDenegada(ctx, beto, "forbidden"); err != nil {
		t.Fatal(err)
	}
	uso, _ := st.UsoCuotaScripts(ctx, "beto")
	if uso.EjecucionesUsuario != 1 {
		t.Fatalf("los denegados cuentan para la cuota: %+v", uso)
	}
	// Cerrar libera tiempo reservado y solo lo hace quien la abrió, una vez.
	if err := st.CerrarEjecucion(ctx, primera, "beto", domain.ResOK, "", 10, 3); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("beto cierra la ejecución de Ana: %v", err)
	}
	if err := st.CerrarEjecucion(ctx, primera, "ana", domain.ResTiempo, "timeout", 5000, 3); err != nil {
		t.Fatal(err)
	}
	if err := st.CerrarEjecucion(ctx, primera, "ana", domain.ResOK, "", 1, 1); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("se cerró dos veces: %v", err)
	}
	// Cuota de tiempo: 600 s por hora; con 30 s reservados por ejecución caben 20.
	t2 := tenantCtx("globex")
	sc2, v2, _ := st.CrearScript(t2, domain.Script{Name: "T", Kind: domain.KindDocument, Scope: domain.AmbitoPrivado, AuthorSub: "ana"}, "x()", []byte(manifiestoPrueba))
	e2 := domain.EjecucionScript{ScriptID: sc2.ID, Version: 1, CodeHash: v2.CodeHash, DocumentID: docPrueba, Sub: "ana"}
	for i := 0; i < 20; i++ {
		if _, err := st.ReservarEjecucion(t2, e2, domain.TopeMsScript); err != nil {
			t.Fatalf("tiempo %d: %v", i, err)
		}
	}
	if _, err := st.ReservarEjecucion(t2, e2, domain.TopeMsScript); !errors.Is(err, domain.ErrCuotaScripts) {
		t.Fatalf("pasó la cuota de tiempo: %v", err)
	}

	// Auditoría: paginada, con tope, y por documento.
	pag, err := st.EjecucionesScript(ctx, "", 0, 10)
	if err != nil || len(pag) != 10 {
		t.Fatalf("página: %d %v", len(pag), err)
	}
	sig, _ := st.EjecucionesScript(ctx, "", pag[len(pag)-1].ID, 10)
	if len(sig) != 10 || sig[0].ID >= pag[len(pag)-1].ID {
		t.Fatalf("la paginación repite o salta: %d", len(sig))
	}
	if l, _ := st.EjecucionesScript(ctx, "0b6f0000-0000-4000-8000-0000000000aa", 0, 10); len(l) != 0 {
		t.Fatalf("el filtro por documento no filtra: %d", len(l))
	}
	// 60 de Ana, 1 de Beto y 1 denegada: todas entran bajo el tope, que además acota el límite pedido.
	if l, _ := st.EjecucionesScript(ctx, docPrueba, 0, 1_000_000); len(l) != 62 {
		t.Fatalf("página con límite desmedido: %d", len(l))
	}
}
