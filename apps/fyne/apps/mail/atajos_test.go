// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package mail

import (
	"os"
	"path/filepath"
	"slices"
	"testing"
	"time"

	"fyne.io/fyne/v2"

	"git.sytue.eu/sytue/one/apps/nucleo/api"
)

// TestLosAtajosSonLosDeLaWeb: j abre la siguiente, k la anterior, e archiva la abierta,
// s la destaca, u vuelve a la lista, c redacta y «?» abre la hoja. Con un diálogo
// abierto, el teclado es suyo y los atajos no hacen nada.
//
// VISTO EN ROJO (2026-09-23): sin la comprobación de capas, pulsar «e» con el redactor
// abierto archivaba el hilo de detrás.
func TestLosAtajosSonLosDeLaWeb(t *testing.T) {
	s := servicioConTodas()
	b, w, _ := buzonConVentana(t, s, fyne.NewSize(1280, 744))
	c := w.Canvas()
	convs := b.P.Bandeja.Conversaciones

	b.Atajo(c, 'j')
	if b.hiloID != convs[0].HiloID {
		t.Fatalf("j sin nada abierto abre %q y tocaba la primera", b.hiloID)
	}
	b.Atajo(c, 'j')
	if b.hiloID != convs[1].HiloID {
		t.Errorf("j abre %q y tocaba la segunda", b.hiloID)
	}
	b.Atajo(c, 'k')
	if b.hiloID != convs[0].HiloID {
		t.Errorf("k abre %q y tocaba volver a la primera", b.hiloID)
	}
	b.Atajo(c, 's')
	if !slices.Contains(s.llamadas, "SetFlagMany(\\Flagged)") {
		t.Errorf("s no destaca: %v", s.llamadas)
	}
	b.Atajo(c, 'u')
	if b.hiloID != "" {
		t.Error("u no vuelve a la lista")
	}
	// Con un hilo abierto detrás, para que «e» tenga algo que archivar si se colara.
	b.Atajo(c, 'j')
	b.Atajo(c, 'c')
	if b.redaccion == nil || b.hiloID == "" {
		t.Fatalf("c no abre el redactor encima del hilo (redactor %v, hilo %q)", b.redaccion != nil, b.hiloID)
	}
	antes := len(s.llamadas)
	b.Atajo(c, 'e')
	if len(s.llamadas) != antes {
		t.Errorf("con el redactor abierto, «e» ha hecho algo: %v", s.llamadas[antes:])
	}
	b.cerrar(b.redaccion)
	if !b.Atajo(c, '?') || !hayDialogo(c, traductor(t).T("shortcuts.title", nil)) {
		t.Error("«?» no abre la hoja de atajos")
	}
}

// TestLaHojaDeAtajosSeGuarda y el móvil: capturas de la hoja, de la barra móvil y del
// botón flotante (SYTUE_CAPTURAS).
func TestLaHojaDeAtajosSeGuarda(t *testing.T) {
	dir := os.Getenv("SYTUE_CAPTURAS")
	if dir == "" {
		t.Skip("sin SYTUE_CAPTURAS")
	}
	b, w, _ := buzonConVentana(t, servicioConTodas(), fyne.NewSize(1280, 800))
	b.verAtajos()
	guardar(t, filepath.Join(dir, "atajos.png"), w.Canvas().Capture())

	m, wm, _ := buzonConVentana(t, servicioConTodas(), fyne.NewSize(360, 740))
	guardar(t, filepath.Join(dir, "movil-lista.png"), wm.Canvas().Capture())
	m.Abrir(m.P.Bandeja.Conversaciones[0])
	guardar(t, filepath.Join(dir, "movil-hilo.png"), wm.Canvas().Capture())
	m.editarFirma()
	guardar(t, filepath.Join(dir, "movil-firma.png"), wm.Canvas().Capture())
}

// TestLaCapturaDeEtiquetasYProgramadosSeGuarda (SYTUE_CAPTURAS).
func TestLaCapturaDeEtiquetasYProgramadosSeGuarda(t *testing.T) {
	dir := os.Getenv("SYTUE_CAPTURAS")
	if dir == "" {
		t.Skip("sin SYTUE_CAPTURAS")
	}
	s := servicioConTodas()
	s.programados = []api.MailScheduled{{ID: "p1", Subject: "Informe trimestral", Recipients: []string{"grace@example.org"}, ReleaseAt: time.Now().Add(26 * time.Hour)}}
	b, w, _ := buzonConVentana(t, s, fyne.NewSize(1280, 800))
	b.verEtiquetas()
	guardar(t, filepath.Join(dir, "etiquetas.png"), w.Canvas().Capture())
	for _, o := range w.Canvas().Overlays().List() {
		w.Canvas().Overlays().Remove(o)
	}
	b.verProgramados()
	guardar(t, filepath.Join(dir, "programados.png"), w.Canvas().Capture())
	for _, o := range w.Canvas().Overlays().List() {
		w.Canvas().Overlays().Remove(o)
	}
	b.abrirRedactor(Borrador{Para: "grace@example.org", Asunto: "Mañana"})
	b.redaccion.r.AlternarProgramar()
	guardar(t, filepath.Join(dir, "redactar-programar.png"), w.Canvas().Capture())
}
