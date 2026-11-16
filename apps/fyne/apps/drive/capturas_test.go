// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package drive

import (
	"fmt"
	"image/png"
	"os"
	"path/filepath"
	"testing"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/test"

	"git.sytue.eu/sytue/one/apps/fyne/tema"
	"git.sytue.eu/sytue/one/apps/fyne/ui"
	"git.sytue.eu/sytue/one/apps/nucleo/api"
)

// TestCapturasDeLasVistas deja en SYTUE_CAPTURAS un PNG por vista, a 1280×744 con el
// carril, con los MISMOS datos que el caso de la web (`caso-drive.ts` del arnés), para
// compararlas a ojo y por píxel. Sin la variable no hace nada.
func TestCapturasDeLasVistas(t *testing.T) {
	dir := os.Getenv("SYTUE_CAPTURAS")
	if dir == "" {
		t.Skip("sin SYTUE_CAPTURAS")
	}
	tr := traductor(t)
	hoy := time.Date(2026, 9, 6, 9, 0, 0, 0, time.UTC)
	favs := map[string]bool{"f2": true}
	recientes := NuevaVistaPlana(tr, Recientes, favs)
	// El 6 de septiembre de 2026 es domingo: los tres tramos de «Recientes» salen.
	recientes.Fase, recientes.Ventana, recientes.Ahora = Exito, 1280, hoy
	recientes.Elementos = []Elemento{
		{ID: "f1", Nombre: "Notas de la reunión.md", Tamano: 4096, Modificado: hoy, Ruta: "/Notas de la reunión.md"},
		{ID: "f2", Nombre: "informe.pdf", Tamano: 1536000, Modificado: hoy.AddDate(0, 0, -1), Ruta: "/Contratos/informe.pdf"},
		{ID: "f3", Nombre: "playa.jpg", Tamano: 2048000, Modificado: hoy.AddDate(0, 0, -3), Ruta: "/Fotos/playa.jpg"},
	}
	compartido := NuevaVistaCompartido(tr)
	compartido.Fase, compartido.Ventana = Exito, 1280
	compartido.Raices = []api.Compartido{
		{File: api.File{ID: "s1", Name: "Proyecto Norte", Path: "/Proyecto Norte"}, IsDir: true, Permission: "edit", SharedBy: "Ana García"},
		{File: api.File{ID: "s2", Name: "presupuesto.xlsx", Path: "/Finanzas/presupuesto.xlsx", SizeBytes: 20480}, Permission: "view", SharedBy: "Luis Pérez"},
	}
	equipos := NuevaVistaEquipos(tr)
	equipos.Fase, equipos.Ventana = Exito, 1280
	archivada := time.Date(2026, 9, 1, 10, 0, 0, 0, time.UTC)
	equipos.Unidades = []api.UnidadEquipo{
		{ID: "u1", Name: "Ventas", Role: api.RolGestionar},
		{ID: "u2", Name: "Legal", Role: api.RolEditar, ArchivedAt: &archivada},
		{ID: "u3", Name: "Dirección", Role: api.RolVer},
	}
	documentos := NuevaVistaDocumentos(tr)
	documentos.Fase, documentos.Ventana = Exito, 1280
	documentos.Ahora = time.Date(2026, 9, 22, 10, 0, 0, 0, time.UTC)
	documentos.Documentos = []Documento{
		{ID: "d1", Titulo: "Acta de septiembre", Tipo: api.DocText, Cambiado: time.Date(2026, 9, 22, 8, 0, 0, 0, time.UTC)},
		{ID: "d2", Titulo: "Cuentas 2026", Tipo: api.DocSheet, Cambiado: time.Date(2026, 9, 20, 8, 0, 0, 0, time.UTC)},
	}

	explorador := NuevoExplorador(tr)
	explorador.Fase = Exito
	for _, n := range []string{"Attachments", "Contratos", "Diseño", "Documentación", "Fotografías"} {
		explorador.Elementos = append(explorador.Elementos, Elemento{ID: n, Nombre: n, Carpeta: true, Tamano: -1, Modificado: hoy})
	}
	explorador.Elementos = append(explorador.Elementos, recientes.Elementos...)
	// Como en la web fuera de una búsqueda: las columnas ordenan, y la del nombre lleva
	// la flecha del orden de siempre.
	explorador.AlOrdenar = func(CampoOrden) {}

	for _, ancho := range []float32{1280, 360} {
		alto := float32(744)
		if ancho < 1280 {
			alto = 740
		}
		recientes.Ventana, compartido.Ventana, equipos.Ventana, documentos.Ventana, explorador.Ventana = ancho, ancho, ancho, ancho, ancho
		for nombre, vista := range map[string]func() fyne.CanvasObject{
			"recientes":  func() fyne.CanvasObject { return recientes.Objeto(tema.Claro) },
			"compartido": func() fyne.CanvasObject { return compartido.Objeto(tema.Claro) },
			"equipos":    func() fyne.CanvasObject { return equipos.Objeto(tema.Claro) },
			"documentos": func() fyne.CanvasObject { return documentos.Objeto(tema.Claro) },
			"explorador": func() fyne.CanvasObject { return explorador.Objeto(tema.Claro) },
			// La cuadrícula, con «Ordenar» delante del conmutador (en el teléfono, en la
			// primera fila) y ordenada por tamaño, de lo más grande a lo más pequeño.
			// Uno nuevo y no el de arriba: la rejilla recuerda el ancho del pintado
			// anterior, y el de la lista ancha la descuadraba a 360.
			"explorador-rejilla": func() fyne.CanvasObject {
				rej := NuevoExplorador(tr)
				rej.Fase, rej.Ventana, rej.Elementos, rej.AlOrdenar = Exito, ancho, explorador.Elementos, explorador.AlOrdenar
				rej.Rejilla, rej.Orden = true, Orden{Campo: OrdenTamano, Desc: true}
				return rej.Objeto(tema.Claro)
			},
			"explorador-soltar": func() fyne.CanvasObject {
				explorador.Arrastrando = true
				defer func() { explorador.Arrastrando = false }()
				return explorador.Objeto(tema.Claro)
			},
		} {
			lat := NuevoLateral(tr, MiUnidad, nil)
			arm := ui.NuevoArmazon(vista())
			arm.Menu = lat.Objeto(tema.Claro)
			w := test.NewWindow(arm)
			w.SetPadded(false)
			w.Resize(fyne.NewSize(ancho, alto))
			img := w.Canvas().Capture()
			w.Close()
			f, err := os.Create(filepath.Join(dir, fmt.Sprintf("%s-%d.png", nombre, int(ancho))))
			if err != nil {
				t.Fatal(err)
			}
			if err := png.Encode(f, img); err != nil {
				t.Fatal(err)
			}
			_ = f.Close()
		}
	}
}
