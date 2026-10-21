// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package mail

import (
	"errors"
	"os"
	"path/filepath"
	"slices"
	"testing"

	"fyne.io/fyne/v2"

	"git.sytue.eu/sytue/one/apps/fyne/ui"
	"git.sytue.eu/sytue/one/apps/nucleo/api"
)

// TestLaPistaDelAliasEsLaDeLaWeb: `draftHint`, en su orden (la repetida gana).
func TestLaPistaDelAliasEsLaDeLaWeb(t *testing.T) {
	alias := []api.MailAlias{{ID: "a1", Address: "ventas@sytue.eu"}}
	for _, c := range []struct {
		borrador string
		tono     ui.TonoAyuda
		clave    string
	}{
		{"  ", ui.AyudaNeutra, "aliases.hintEmpty"},
		{"VENTAS@sytue.eu", ui.AyudaPeligro, "aliases.hintDuplicate"},
		{"ada@sytue.eu", ui.AyudaPeligro, "aliases.hintDuplicate"},
		{"hola", ui.AyudaPeligro, "aliases.hintNoDomain"},
		{"hola@sytue", ui.AyudaPeligro, "aliases.hintInvalid"},
		{"hola@sytue.eu", ui.AyudaExito, "aliases.hintValid"},
	} {
		if tono, clave := PistaAlias(c.borrador, "ada@sytue.eu", alias); tono != c.tono || clave != c.clave {
			t.Errorf("%q: %v %s y la web %v %s", c.borrador, tono, clave, c.tono, c.clave)
		}
	}
}

// TestLosAliasSeAnadenYSeBorran: la principal arriba; añadir limpia el campo, avisa
// con la dirección y refresca el «De» del redactor; el rechazo con código lo explica;
// borrar es optimista y vuelve si falla.
func TestLosAliasSeAnadenYSeBorran(t *testing.T) {
	s := servicioConTodas()
	s.alias = []string{"ventas@example.org"}
	b, w, _ := buzonConVentana(t, s, fyne.NewSize(1280, 800))
	tr := traductor(t)
	g := b.verAlias()
	if !hayDialogo(w.Canvas(), tr.T("aliases.title", nil)) || g.estado != Listo || g.principal != "ada@example.org" || len(g.alias) != 1 {
		t.Fatalf("alias: %v %q %+v", g.estado, g.principal, g.alias)
	}

	g.borrador.PonerTexto("Ventas@example.org")
	g.anadir()
	if s.pidio("CreateAlias") != 0 {
		t.Fatal("se ha pedido un alias repetido")
	}
	antes := s.pidio("Aliases")
	g.borrador.PonerTexto("hola@example.org")
	g.anadir()
	if s.pidio("CreateAlias") != 1 || g.borrador.Texto() != "" || len(g.alias) != 2 {
		t.Fatalf("añadir: %v, campo %q, alias %+v", s.llamadas, g.borrador.Texto(), g.alias)
	}
	if s.pidio("Aliases") < antes+2 || !slices.Contains(b.opcionesDe(), "hola@example.org") {
		t.Errorf("el «De» del redactor no se ha refrescado: %v", b.opcionesDe())
	}
	if tono, titulo := ultimoAviso(b); tono != ui.AvisoExito || titulo != tr.T("aliases.added", nil) {
		t.Errorf("aviso al añadir: %v %q", tono, titulo)
	}

	s.errAlias = &api.Error{Status: 409, Body: `{"error":{"code":"alias_limit"}}`}
	g.borrador.PonerTexto("otra@example.org")
	g.anadir()
	if av := b.P.Avisos.Avisos(); av[len(av)-1].Descripcion != tr.T("aliases.limitDescription", nil) {
		t.Errorf("el límite dice %q", av[len(av)-1].Descripcion)
	}

	s.errAlias = errors.New("caído")
	g.borrar(g.alias[0])
	if len(g.alias) != 2 {
		t.Errorf("el borrado rechazado deja %d alias", len(g.alias))
	}
	if _, titulo := ultimoAviso(b); titulo != tr.T("aliases.deleteFailed", nil) {
		t.Errorf("el borrado rechazado dice %q", titulo)
	}
	s.errAlias = nil
	g.borrar(g.alias[0])
	if len(g.alias) != 1 || s.pidio("DeleteAlias") != 2 {
		t.Errorf("borrar: %+v %v", g.alias, s.llamadas)
	}
}

// TestLaCapturaDeAliasSeGuarda (SYTUE_CAPTURAS): con un alias, y sin ninguno.
func TestLaCapturaDeAliasSeGuarda(t *testing.T) {
	dir := os.Getenv("SYTUE_CAPTURAS")
	if dir == "" {
		t.Skip("sin SYTUE_CAPTURAS")
	}
	for nombre, alias := range map[string][]string{"alias.png": {"ventas@example.org"}, "alias-vacio.png": nil} {
		s := servicioConTodas()
		s.alias = alias
		b, w, _ := buzonConVentana(t, s, fyne.NewSize(1280, 800))
		g := b.verAlias()
		g.borrador.PonerTexto("hola@example.org")
		guardar(t, filepath.Join(dir, nombre), w.Canvas().Capture())
	}
}
