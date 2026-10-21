// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package mail

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"fyne.io/fyne/v2"

	"git.sytue.eu/sytue/one/apps/fyne/ui"
	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/eml"
)

// driveNavegable es Drive con carpetas: guarda (como driveFalso) y navega.
type driveNavegable struct {
	driveFalso
	errRaiz error
	pedidas []string
}

func (d *driveNavegable) Root(context.Context) (api.Folder, error) {
	return api.Folder{ID: "r", Name: "Mi unidad", IsRoot: true}, d.errRaiz
}

func (d *driveNavegable) Children(_ context.Context, carpeta, cursor string) (api.Children, error) {
	d.pedidas = append(d.pedidas, carpeta+"/"+cursor)
	switch {
	case carpeta == "r" && cursor == "":
		return api.Children{
			Folders:    []api.Folder{{ID: "p1", Name: "Proyectos"}},
			Files:      []api.File{{ID: "f1", Name: "Contrato firmado.pdf", SizeBytes: 1348213}},
			NextCursor: "c2",
		}, nil
	case carpeta == "r" && cursor == "c2":
		return api.Children{Files: []api.File{{ID: "f2", Name: "Presupuesto obra.xlsx", SizeBytes: 48213}}}, nil
	case carpeta == "p1":
		return api.Children{Files: []api.File{{ID: "f9", Name: "Plano.pdf", SizeBytes: 482133}}}, nil
	}
	return api.Children{}, nil
}

// TestAdjuntarDelEquipoLeeLosFicheros: cada fichero elegido entra entero con su nombre
// y su tipo; cancelar no hace nada y un fallo se dice.
func TestAdjuntarDelEquipoLeeLosFicheros(t *testing.T) {
	dir := t.TempDir()
	pdf, raro := filepath.Join(dir, "informe.pdf"), filepath.Join(dir, "datos.zzz")
	_ = os.WriteFile(pdf, []byte("%PDF-1.7"), 0o600)
	_ = os.WriteFile(raro, []byte{1, 2, 3}, 0o600)
	b, _, _ := buzonConVentana(t, servicioConTodas(), fyne.NewSize(1280, 800))
	b.Redactar()
	rd := b.redaccion

	b.ElegirFicheros = func() ([]string, error) { return nil, nil }
	b.adjuntarDelEquipo(rd)
	if len(rd.r.B.Adjuntos) != 0 || len(b.P.Avisos.Avisos()) != 0 {
		t.Fatal("cancelar el panel ha hecho algo")
	}
	b.ElegirFicheros = func() ([]string, error) { return []string{pdf, raro}, nil }
	b.adjuntarDelEquipo(rd)
	as := rd.r.B.Adjuntos
	if len(as) != 2 || as[0].Nombre != "informe.pdf" || as[0].Tipo != "application/pdf" || string(as[0].Datos) != "%PDF-1.7" ||
		as[1].Tipo != "application/octet-stream" {
		t.Fatalf("adjuntos: %+v", as)
	}
	b.ElegirFicheros = func() ([]string, error) { return []string{filepath.Join(dir, "no-está")}, nil }
	b.adjuntarDelEquipo(rd)
	if _, titulo := ultimoAviso(b); titulo != traductor(t).T("compose.attachFailed", nil) || len(rd.r.B.Adjuntos) != 2 {
		t.Errorf("un fichero ilegible: aviso %q, adjuntos %d", titulo, len(rd.r.B.Adjuntos))
	}
}

// TestElMenuDeAdjuntarOfreceDriveSoloConDrive: sin Drive que navegar, solo «del equipo».
func TestElMenuDeAdjuntarOfreceDriveSoloConDrive(t *testing.T) {
	b, _, _ := buzonConVentana(t, servicioConTodas(), fyne.NewSize(1280, 800))
	b.Redactar()
	if n := len(b.menuAdjuntar(b.redaccion).Elementos); n != 1 {
		t.Errorf("sin Drive el menú tiene %d entradas", n)
	}
	b.ConDrive(&driveNavegable{})
	if n := len(b.menuAdjuntar(b.redaccion).Elementos); n != 2 {
		t.Errorf("con Drive el menú tiene %d entradas", n)
	}
}

// TestAdjuntarDeDriveNavegaYDejaElEnlace: Mi unidad, «Cargar más», entrar en una
// carpeta y elegir: el enlace va al borrador una sola vez y al .eml con su encabezado.
func TestAdjuntarDeDriveNavegaYDejaElEnlace(t *testing.T) {
	s := servicioConTodas()
	b, w, _ := buzonConVentana(t, s, fyne.NewSize(1280, 800))
	d := &driveNavegable{}
	b.ConDrive(d)
	b.URLDrive = "https://drive.example/"
	b.abrirRedactor(Borrador{Para: "grace@example.org", Asunto: "Plano", Cuerpo: "Te lo paso."})
	rd := b.redaccion
	tr := traductor(t)
	g := b.elegirDeDrive(rd)
	if !hayDialogo(w.Canvas(), tr.T("drive.title", nil)) || g.estado != Listo || len(g.ruta) != 1 || len(g.ficheros) != 1 {
		t.Fatalf("Mi unidad: %v, ruta %d, ficheros %d", g.estado, len(g.ruta), len(g.ficheros))
	}
	g.masFicheros()
	if len(g.ficheros) != 2 || g.cursor != "" {
		t.Fatalf("cargar más: %d ficheros, cursor %q", len(g.ficheros), g.cursor)
	}
	g.abrir(append(g.ruta, g.carpetas[0]))
	if len(g.ruta) != 2 || len(g.ficheros) != 1 || g.ficheros[0].ID != "f9" {
		t.Fatalf("Proyectos: ruta %d, %+v", len(g.ruta), g.ficheros)
	}
	g.elegir(g.ficheros[0])
	want := EnlaceDrive{Nombre: "Plano.pdf", URL: "https://drive.example/files/f9"}
	if len(rd.r.B.Drive) != 1 || rd.r.B.Drive[0] != want || hayDialogo(w.Canvas(), tr.T("drive.title", nil)) {
		t.Fatalf("elegir: %+v", rd.r.B.Drive)
	}
	rd.r.AdjuntarDrive(want)
	if len(rd.r.B.Drive) != 1 {
		t.Error("el mismo enlace se ha adjuntado dos veces")
	}
	_, s2, _, err := b.componer(rd.r.B, false)
	if err != nil || !strings.HasSuffix(s2.Texto, tr.T("compose.driveAttachmentsHeading", nil)+"\nPlano.pdf: https://drive.example/files/f9") {
		t.Errorf("el cuerpo no lleva el enlace: %q %v", s2.Texto, err)
	}
}

// TestElSelectorDeDriveDistingueLaSesionCaducada: un 401 no es «comprueba tu conexión».
func TestElSelectorDeDriveDistingueLaSesionCaducada(t *testing.T) {
	b, _, _ := buzonConVentana(t, servicioConTodas(), fyne.NewSize(1280, 800))
	d := &driveNavegable{errRaiz: &api.Error{Status: 401}}
	b.ConDrive(d)
	if g := b.verSelectorDrive(func(EnlaceDrive) {}); g.estado != Error || !g.sinSesion {
		t.Errorf("401: estado %v, sin sesión %v", g.estado, g.sinSesion)
	}
	d.errRaiz = errors.New("caído")
	if g := b.verSelectorDrive(func(EnlaceDrive) {}); g.estado != Error || g.sinSesion {
		t.Errorf("caído: estado %v, sin sesión %v", g.estado, g.sinSesion)
	}
}

// TestLaCapturaDeAdjuntarSeGuarda (SYTUE_CAPTURAS): el selector y el redactor con un
// adjunto del equipo y otro de Drive.
func TestLaCapturaDeAdjuntarSeGuarda(t *testing.T) {
	dir := os.Getenv("SYTUE_CAPTURAS")
	if dir == "" {
		t.Skip("sin SYTUE_CAPTURAS")
	}
	b, w, _ := buzonConVentana(t, servicioConTodas(), fyne.NewSize(1280, 800))
	b.ConDrive(&driveNavegable{})
	b.URLDrive = "https://drive.example"
	b.abrirRedactor(Borrador{
		Para: "grace@example.org", Asunto: "Plano", Cuerpo: "Te lo paso.",
		Adjuntos: []eml.Adjunto{{Nombre: "informe.pdf", Tipo: "application/pdf", Datos: make([]byte, 48213)}},
	})
	b.redaccion.r.AdjuntarDrive(EnlaceDrive{Nombre: "Plano.pdf", URL: "https://drive.example/files/f9"})
	guardar(t, filepath.Join(dir, "redactar-adjuntos.png"), w.Canvas().Capture())
	b.elegirDeDrive(b.redaccion)
	guardar(t, filepath.Join(dir, "adjuntar-drive.png"), w.Canvas().Capture())
}

// TestLaSesionCaducadaOfreceVolverAEntrar: con la sesión caducada el selector ofrece
// «Volver a iniciar sesión» (la web recarga); al volver de Sytue ID pide otra vez la
// carpeta y ya la enseña. Sin quien sepa entrar, el estado lo dice sin botón.
func TestLaSesionCaducadaOfreceVolverAEntrar(t *testing.T) {
	b, w, _ := buzonConVentana(t, servicioConTodas(), fyne.NewSize(1280, 800))
	tr := traductor(t)
	d := &driveNavegable{errRaiz: &api.Error{Status: 401}}
	b.ConDrive(d)
	boton := func() *ui.Boton {
		var hay *ui.Boton
		var mirar func(fyne.CanvasObject)
		mirar = func(o fyne.CanvasObject) {
			if x, ok := o.(*ui.Boton); ok && x.Texto == tr.T("drive.signInAgain", nil) {
				hay = x
			}
			for _, h := range hijos(o) {
				mirar(h)
			}
		}
		for _, o := range w.Canvas().Overlays().List() {
			mirar(o)
		}
		return hay
	}
	if g := b.verSelectorDrive(func(EnlaceDrive) {}); !g.sinSesion || boton() != nil {
		t.Fatal("sin quien sepa entrar se ofrece un botón que no hace nada")
	}
	b.P.Lienzo().Overlays().Remove(b.P.Lienzo().Overlays().Top())

	entradas := 0
	b.Entrar = func(context.Context) error { entradas++; d.errRaiz = nil; return nil }
	g := b.verSelectorDrive(func(EnlaceDrive) {})
	bt := boton()
	if bt == nil {
		t.Fatal("con la sesión caducada no se ofrece volver a entrar")
	}
	bt.AlPulsar()
	if entradas != 1 || g.estado != Listo || len(g.carpetas) != 1 {
		t.Errorf("tras volver a entrar: %d entradas, estado %v, %d carpetas", entradas, g.estado, len(g.carpetas))
	}
}
