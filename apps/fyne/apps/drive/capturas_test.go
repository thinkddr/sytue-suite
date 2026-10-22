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
	recientes.Fase, recientes.Ventana = Exito, 1280
	recientes.Elementos = []Elemento{
		{ID: "f1", Nombre: "Notas de la reunión.md", Tamano: 4096, Modificado: hoy, Ruta: "/Notas de la reunión.md"},
		{ID: "f2", Nombre: "informe.pdf", Tamano: 1536000, Modificado: hoy, Ruta: "/Contratos/informe.pdf"},
		{ID: "f3", Nombre: "playa.jpg", Tamano: 2048000, Modificado: hoy, Ruta: "/Fotos/playa.jpg"},
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
