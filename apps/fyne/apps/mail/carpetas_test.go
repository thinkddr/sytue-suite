// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package mail

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"fyne.io/fyne/v2"

	"git.sytue.eu/sytue/one/apps/fyne/ui"
	"git.sytue.eu/sytue/one/apps/nucleo/api"
)

// TestLosMotivosDeCarpetaSonLosDeLaWeb: `FAIL_KEYS`, y lo que no está ahí es una avería.
func TestLosMotivosDeCarpetaSonLosDeLaWeb(t *testing.T) {
	for motivo, clave := range map[api.MailboxSetError]string{
		"alreadyExists": "folderNew.exists", "invalidProperties": "folderNew.invalid", "notFound": "folderNew.invalid",
		"forbidden": "folders.systemFolder", "mailboxHasEmail": "folders.notEmpty", "serverFail": "",
	} {
		if got := ClaveRechazoCarpeta(motivo); got != clave {
			t.Errorf("%s dice %q y la web %q", motivo, got, clave)
		}
	}
}

// ultimoAviso es el título del aviso más reciente, o "".
func ultimoAviso(b *Buzon) (ui.TonoAviso, string) {
	av := b.P.Avisos.Avisos()
	if len(av) == 0 {
		return 0, ""
	}
	return av[len(av)-1].Tono, av[len(av)-1].Titulo
}

// TestLasCarpetasSeCreanRenombranYBorran: solo las propias; crear limpia el campo y
// la lista la trae; el nombre repetido (también el de una de sistema) no se deja
// crear; renombrar rechazado vuelve al nombre de antes y dice por qué; borrar una
// con correo lo explica y una vacía desaparece.
func TestLasCarpetasSeCreanRenombranYBorran(t *testing.T) {
	s := servicioConTodas()
	b, w, _ := buzonConVentana(t, s, fyne.NewSize(1280, 800))
	tr := traductor(t)
	g := b.verCarpetas()
	if !hayDialogo(w.Canvas(), tr.T("folders.title", nil)) || len(g.propias()) != 1 || g.propias()[0].ID != "pr" {
		t.Fatalf("propias: %+v", g.propias())
	}

	g.nombre.PonerTexto("inbox")
	g.crear()
	if s.pidio("CreateMailbox") != 0 {
		t.Fatal("se ha creado una carpeta con el nombre de una de sistema")
	}
	g.nombre.PonerTexto("Facturas")
	g.crear()
	if s.pidio("CreateMailbox") != 1 || g.nombre.Texto() != "" || len(g.propias()) != 2 {
		t.Fatalf("crear: %v, campo %q, propias %d", s.llamadas, g.nombre.Texto(), len(g.propias()))
	}
	if tono, titulo := ultimoAviso(b); tono != ui.AvisoExito || titulo != "Carpeta creada: Facturas" {
		t.Errorf("aviso al crear: %v %q", tono, titulo)
	}

	s.motivoCarpeta = "forbidden"
	campo := ui.NuevaEntrada("")
	campo.PonerTexto("Obra")
	g.renombrar(g.propias()[0], campo)
	if campo.Texto() != g.propias()[0].Name {
		t.Errorf("el renombrado rechazado deja %q en el campo", campo.Texto())
	}
	if _, titulo := ultimoAviso(b); titulo != tr.T("folders.systemFolder", nil) {
		t.Errorf("el rechazo dice %q", titulo)
	}

	s.motivoCarpeta = "mailboxHasEmail"
	g.borrar(g.propias()[0])
	if _, titulo := ultimoAviso(b); titulo != tr.T("folders.notEmpty", nil) || len(g.propias()) != 2 {
		t.Errorf("borrar con correo: %q, propias %d", titulo, len(g.propias()))
	}
	s.motivoCarpeta = ""
	g.borrar(g.propias()[0])
	if len(g.propias()) != 1 {
		t.Errorf("tras borrar quedan %d propias", len(g.propias()))
	}

	s.errCarpeta = errors.New("caído")
	g.borrar(g.propias()[0])
	if _, titulo := ultimoAviso(b); titulo != tr.T("folders.deleteFailed", nil) {
		t.Errorf("la avería dice %q", titulo)
	}
}

// TestLaCapturaDeCarpetasSeGuarda (SYTUE_CAPTURAS).
func TestLaCapturaDeCarpetasSeGuarda(t *testing.T) {
	dir := os.Getenv("SYTUE_CAPTURAS")
	if dir == "" {
		t.Skip("sin SYTUE_CAPTURAS")
	}
	s := servicioConTodas()
	s.carpetas = append(s.carpetas, api.Mailbox{ID: "fa", Name: "Facturas", TotalEmails: 12})
	b, w, _ := buzonConVentana(t, s, fyne.NewSize(1280, 800))
	g := b.verCarpetas()
	g.borrando = "fa"
	g.pintar()
	guardar(t, filepath.Join(dir, "carpetas.png"), w.Canvas().Capture())
}
