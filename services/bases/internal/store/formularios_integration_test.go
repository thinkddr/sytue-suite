// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

//go:build integration

package store

import (
	"encoding/json"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"git.sytue.eu/sytue/one/services/bases/internal/domain"
)

// formulario monta una tabla con tres campos y una vista de formulario con dos de ellos
// (el tercero, «Notas internas», no se enseña ni se admite desde fuera).
func formulario(t *testing.T) (*entorno, domain.Vista, domain.Campo) {
	t.Helper()
	e := nuevo(t)
	notas, err := e.s.CrearCampo(e.ctx, ana, e.tabl.ID, domain.EntradaCampo{Name: "Notas internas", Kind: domain.TipoTexto})
	e.ok(err)
	v, err := e.s.CrearVista(e.ctx, ana, e.tabl.ID, "Contacto", domain.VistaFormulario, domain.ConfigVista{Fields: []string{e.txt.ID, e.num.ID}})
	e.ok(err)
	return e, v, notas
}

func TestFormularioPublicoEndToEnd(t *testing.T) {
	e, v, notas := formulario(t)
	en, token, err := e.s.CrearEnlaceFormulario(e.ctx, ana, v.ID, 3, nil)
	e.ok(err)
	if len(token) != 43 || strings.Contains(string(jsonDe(en)), token) {
		t.Fatalf("el token (%d) nunca viaja en el enlace", len(token))
	}
	// El token no está en la base de datos en claro.
	var visto int
	e.ok(e.s.en(e.ctx, ana, func(tx pgx.Tx) error {
		return tx.QueryRow(e.ctx, `SELECT count(*) FROM base_form_links WHERE token_hash = $1 OR token_hash = $2`, token, hashToken(token)).Scan(&visto)
	}))
	if visto != 1 {
		t.Errorf("solo debe estar el hash: %d", visto)
	}

	f, err := e.s.FormularioPorToken(e.ctx, token)
	e.ok(err)
	if f.Name != "Contacto" || len(f.Fields) != 2 {
		t.Fatalf("definición pública: %+v", f)
	}
	for _, c := range f.Fields {
		if c.ID == notas.ID {
			t.Error("la definición pública enseña un campo que la vista no incluye")
		}
	}

	ok := map[string]json.RawMessage{e.txt.ID: json.RawMessage(`"Panadería"`), e.num.ID: json.RawMessage(`3`)}
	e.ok(e.s.EnviarFormulario(e.ctx, token, ok))
	pag, err := e.s.ListarRegistros(e.ctx, ana, e.tabl.ID, "", 0, "")
	e.ok(err)
	if len(pag.Records) != 1 || !strings.HasPrefix(pag.Records[0].CreatedBy, "form:") {
		t.Fatalf("el registro llega marcado como del formulario: %+v", pag.Records)
	}

	// Lo que el formulario no admite se rechaza (no se ignora).
	esperar(t, e.s.EnviarFormulario(e.ctx, token, map[string]json.RawMessage{notas.ID: json.RawMessage(`"x"`)}), domain.ErrInvalido, "campo fuera del formulario")
	esperar(t, e.s.EnviarFormulario(e.ctx, token, map[string]json.RawMessage{"inventado": json.RawMessage(`"x"`)}), domain.ErrInvalido, "campo inventado")
	esperar(t, e.s.EnviarFormulario(e.ctx, token, map[string]json.RawMessage{e.num.ID: json.RawMessage(`"no es número"`)}), domain.ErrInvalido, "tipo inválido")
	esperar(t, e.s.EnviarFormulario(e.ctx, token, map[string]json.RawMessage{}), domain.ErrInvalido, "formulario vacío")

	// La auditoría lleva a quien envía como actor «form:<enlace>».
	es, err := e.s.ListarAuditoria(e.ctx, ana, e.base.ID, 0, 200)
	e.ok(err)
	var auditado bool
	for _, x := range es {
		auditado = auditado || (x.Action == "record.create.form" && x.Actor == "form:"+en.ID)
	}
	if !auditado {
		t.Error("el envío no queda auditado con su enlace")
	}
}

func TestFormularioCupoRevocacionYCaducidad(t *testing.T) {
	e, v, _ := formulario(t)
	valores := map[string]json.RawMessage{e.txt.ID: json.RawMessage(`"x"`)}

	_, token, err := e.s.CrearEnlaceFormulario(e.ctx, ana, v.ID, 2, nil)
	e.ok(err)
	e.ok(e.s.EnviarFormulario(e.ctx, token, valores))
	e.ok(e.s.EnviarFormulario(e.ctx, token, valores))
	esperar(t, e.s.EnviarFormulario(e.ctx, token, valores), domain.ErrNoExiste, "cupo agotado")
	_, err = e.s.FormularioPorToken(e.ctx, token)
	esperar(t, err, domain.ErrNoExiste, "definición con cupo agotado")

	// Revocar: efecto inmediato en lectura y envío.
	en, tok2, err := e.s.CrearEnlaceFormulario(e.ctx, ana, v.ID, 10, nil)
	e.ok(err)
	e.ok(e.s.EnviarFormulario(e.ctx, tok2, valores))
	e.ok(e.s.RevocarEnlaceFormulario(e.ctx, ana, en.ID))
	esperar(t, e.s.EnviarFormulario(e.ctx, tok2, valores), domain.ErrNoExiste, "enlace revocado")
	_, err = e.s.FormularioPorToken(e.ctx, tok2)
	esperar(t, err, domain.ErrNoExiste, "definición con enlace revocado")

	// Caducado: se fuerza la fecha en la base.
	en3, tok3, err := e.s.CrearEnlaceFormulario(e.ctx, ana, v.ID, 10, nil)
	e.ok(err)
	e.ok(e.s.en(e.ctx, ana, func(tx pgx.Tx) error {
		_, err := tx.Exec(e.ctx, `UPDATE base_form_links SET expires_at = now() - interval '1 minute' WHERE id = $1`, en3.ID)
		return err
	}))
	esperar(t, e.s.EnviarFormulario(e.ctx, tok3, valores), domain.ErrNoExiste, "enlace caducado")

	// Tokens que no son tokens.
	for _, malo := range []string{"", "corto", strings.Repeat("a", 43), strings.Repeat("a", 44), "../" + strings.Repeat("a", 40), token + "x"} {
		esperar(t, e.s.EnviarFormulario(e.ctx, malo, valores), domain.ErrNoExiste, "token «"+malo+"»")
	}
}

func TestFormularioEnviosConcurrentesNoPasanDelCupo(t *testing.T) {
	e, v, _ := formulario(t)
	_, token, err := e.s.CrearEnlaceFormulario(e.ctx, ana, v.ID, 5, nil)
	e.ok(err)
	var wg sync.WaitGroup
	var mu sync.Mutex
	aceptados := 0
	for i := 0; i < 12; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if e.s.EnviarFormulario(e.ctx, token, map[string]json.RawMessage{e.txt.ID: json.RawMessage(`"x"`)}) == nil {
				mu.Lock()
				aceptados++
				mu.Unlock()
			}
		}()
	}
	wg.Wait()
	if aceptados != 5 {
		t.Errorf("con cupo 5 se aceptaron %d envíos", aceptados)
	}
}

func TestFormularioGestionSoloDelPropietarioYSoloFormularios(t *testing.T) {
	e, v, _ := formulario(t)
	_, err := e.s.ConcederPermiso(e.ctx, ana, e.base.ID, domain.Permiso{PrincipalKind: "user", PrincipalID: "bea", Role: domain.RolEditor})
	e.ok(err)
	_, _, err = e.s.CrearEnlaceFormulario(e.ctx, bea, v.ID, 5, nil)
	esperar(t, err, domain.ErrSinPermiso, "un editor no publica formularios")
	_, _, err = e.s.CrearEnlaceFormulario(e.ctx, gus, v.ID, 5, nil)
	esperar(t, err, domain.ErrNoExiste, "otra organización")
	_, err = e.s.ListarEnlacesFormulario(e.ctx, bea, v.ID)
	esperar(t, err, domain.ErrSinPermiso, "un editor no lista enlaces")

	rejilla, err := e.s.CrearVista(e.ctx, ana, e.tabl.ID, "Rejilla", domain.VistaRejilla, domain.ConfigVista{})
	e.ok(err)
	_, _, err = e.s.CrearEnlaceFormulario(e.ctx, ana, rejilla.ID, 5, nil)
	esperar(t, err, domain.ErrInvalido, "una rejilla no es un formulario")
	_, _, err = e.s.CrearEnlaceFormulario(e.ctx, ana, v.ID, 99999, nil)
	esperar(t, err, domain.ErrInvalido, "cupo fuera de rango")
	ayer := time.Now().Add(-time.Hour)
	_, _, err = e.s.CrearEnlaceFormulario(e.ctx, ana, v.ID, 5, &ayer)
	esperar(t, err, domain.ErrInvalido, "caducidad pasada")

	en, _, err := e.s.CrearEnlaceFormulario(e.ctx, ana, v.ID, 5, nil)
	e.ok(err)
	esperar(t, e.s.RevocarEnlaceFormulario(e.ctx, bea, en.ID), domain.ErrSinPermiso, "un editor no revoca")
	esperar(t, e.s.RevocarEnlaceFormulario(e.ctx, gus, en.ID), domain.ErrNoExiste, "otra organización no revoca")
	for i := 0; i < MaxEnlacesPorFormulario; i++ {
		if _, _, err := e.s.CrearEnlaceFormulario(e.ctx, ana, v.ID, 5, nil); err != nil {
			if i < MaxEnlacesPorFormulario-1 {
				t.Fatalf("enlace %d: %v", i, err)
			}
			esperar(t, err, domain.ErrLimite, "demasiados enlaces vivos")
		}
	}
}

func TestAltchaSoloSeAceptaUnaVez(t *testing.T) {
	e := nuevo(t)
	firma := "firma-" + time.Now().String()
	nueva, err := e.s.RegistrarAltcha(e.ctx, firma, time.Now().Add(time.Minute))
	if err != nil || !nueva {
		t.Fatalf("primera: %v %v", nueva, err)
	}
	nueva, err = e.s.RegistrarAltcha(e.ctx, firma, time.Now().Add(time.Minute))
	if err != nil || nueva {
		t.Fatalf("repetida: %v %v", nueva, err)
	}
}
