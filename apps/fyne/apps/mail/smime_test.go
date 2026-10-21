// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package mail

import (
	"errors"
	"io"
	"os"
	"path/filepath"
	"slices"
	"testing"

	"fyne.io/fyne/v2"

	"git.sytue.eu/sytue/one/apps/fyne/ui"
	"git.sytue.eu/sytue/one/apps/nucleo/api"
)

// TestElCertificadoPropioSeInstala: vacío no se deja; elegido de un fichero entra en
// el área; el motivo del servicio sale en la pista y el diálogo se queda; bien, avisa
// y se cierra.
func TestElCertificadoPropioSeInstala(t *testing.T) {
	s := servicioConTodas()
	b, w, _ := buzonConVentana(t, s, fyne.NewSize(1280, 800))
	tr := traductor(t)
	g := b.verSmime()
	if !g.instalar.Inerte {
		t.Fatal("sin PEM se deja instalar")
	}
	if h := g.pem.MinSize().Height; h != 146 {
		t.Errorf("el área mide %g de alto y en la web 146 (`rows=8`, línea16 y relleno16 + bordes2)", h)
	}
	// El panel mide lo que pide su cuerpo YA colocado (la ayuda en sus líneas): si no,
	// el cuerpo se desplaza y la pista queda fuera.
	if got, want := g.d.Size().Height, ui.MedidaDialogo(g.d, w.Canvas().Size()).Height; got < want-0.5 {
		t.Errorf("el diálogo mide %g y su contenido pide %g", got, want)
	}
	fich := filepath.Join(t.TempDir(), "yo.pem")
	_ = os.WriteFile(fich, []byte("-----BEGIN CERTIFICATE-----\nX\n"), 0o600)
	b.ElegirFicheros = func() ([]string, error) { return []string{fich}, nil }
	g.elegirFichero()
	if g.pem.Texto() != "-----BEGIN CERTIFICATE-----\nX\n" || g.instalar.Inerte {
		t.Fatalf("el fichero no entra: %q", g.pem.Texto())
	}

	s.errSmime = &api.Error{Status: 400, Body: `{"error":{"message":"The certificate does not match the key."}}`}
	g.enviar()
	pista, _ := g.caja.Objects[2].(*ui.PistaCampo)
	if pista == nil || pista.Tono != ui.AyudaPeligro || pista.Texto != "The certificate does not match the key." ||
		!hayDialogo(w.Canvas(), tr.T("smime.installTitle", nil)) {
		t.Fatalf("el rechazo: %+v", pista)
	}
	s.errSmime = nil
	g.enviar()
	if !slices.Contains(s.llamadas, "SetSmimeIdentity(-----BEGIN CERTIFICATE-----\nX\n)") || hayDialogo(w.Canvas(), tr.T("smime.installTitle", nil)) {
		t.Errorf("instalar: %v", s.llamadas)
	}
	if _, titulo := ultimoAviso(b); titulo != tr.T("smime.installed", nil) {
		t.Errorf("aviso: %q", titulo)
	}
}

// TestLaCapturaDelCertificadoSeGuarda (SYTUE_CAPTURAS).
func TestLaCapturaDelCertificadoSeGuarda(t *testing.T) {
	dir := os.Getenv("SYTUE_CAPTURAS")
	if dir == "" {
		t.Skip("sin SYTUE_CAPTURAS")
	}
	b, w, _ := buzonConVentana(t, servicioConTodas(), fyne.NewSize(1280, 800))
	b.verSmime()
	guardar(t, filepath.Join(dir, "smime.png"), w.Canvas().Capture())
}

// TestElAnclaQueNoSeBajaLoDice: sin autoridad todavía (404) avisa de que se crea con
// el primer correo firmado; sin red, de que no se pudo bajar; bien, se guarda con su
// nombre y no avisa.
func TestElAnclaQueNoSeBajaLoDice(t *testing.T) {
	s := servicioConTodas()
	b, _, _ := buzonConVentana(t, s, fyne.NewSize(1280, 800))
	tr := traductor(t)
	var guardado string
	antes := Guardar
	Guardar = func(nombre string, _ io.Reader) error { guardado = nombre; return nil }
	t.Cleanup(func() { Guardar = antes })

	s.errTriaje = &api.Error{Status: 404}
	b.bajarAncla()
	if tono, titulo := ultimoAviso(b); tono != ui.AvisoAdvertencia || titulo != tr.T("smime.anchorMissing", nil) {
		t.Errorf("sin autoridad: %v «%s»", tono, titulo)
	}
	s.errTriaje = errors.New("sin red")
	b.bajarAncla()
	if tono, titulo := ultimoAviso(b); tono != ui.AvisoPeligro || titulo != tr.T("smime.anchorFailed", nil) {
		t.Errorf("sin red: %v «%s»", tono, titulo)
	}
	s.errTriaje = nil
	n := len(b.P.Avisos.Avisos())
	b.bajarAncla()
	if guardado != NombreAnclaSmime || len(b.P.Avisos.Avisos()) != n {
		t.Errorf("bien: guardado «%s», %d avisos nuevos", guardado, len(b.P.Avisos.Avisos())-n)
	}
}
