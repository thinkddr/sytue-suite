// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

// Command erp abre Sytue ERP en una ventana de verdad.
//
// Es SU PROPIA aplicación, como en la web es su propio subdominio: un binario
// instalable por producto (`docs/CONVENTIONS.md` §13.6). Su `client_id` en Sytue ID es
// `sytue-syerp` y su esquema de URL, el que declara `FyneApp.toml`.
//
// Para VERLA hace falta una máquina con pantalla; el piloto no la tiene. Lo que sí
// corre en cualquier sitio, sin CGO y sin servidor gráfico, es `go test ./apps/erp/...`,
// que pinta lo mismo con el pintor de software y compara los píxeles contra lo medido.
package main

import (
	"context"
	"errors"
	"log/slog"
	"net/url"
	"runtime"
	"slices"
	"strings"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/app"

	"git.sytue.eu/sytue/one/apps/fyne/apps/erp"
	"git.sytue.eu/sytue/one/apps/fyne/iconos"
	"git.sytue.eu/sytue/one/apps/fyne/sesion"
	"git.sytue.eu/sytue/one/apps/fyne/tema"
	"git.sytue.eu/sytue/one/apps/fyne/ui"
	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/ficheros"
)

func main() {
	a := app.New()
	a.Settings().SetTheme(tema.Sytue{})

	// La sesión, la apariencia y el idioma son TRANSVERSALES a las trece apps: esto lo
	// lee de donde lo dejó la última que lo tocó. `Abrir` no habla con la red.
	s := sesion.Abrir(a, "")

	// El idioma lo decide el SISTEMA, igual que en la web lo decide el middleware:
	// nunca se detecta dentro de una vista. Uno que la suite no sirva cae al inglés.
	l := s.Idioma()
	t, err := erp.Traductor(l)
	if err != nil {
		panic(err) // el catálogo va embebido: si falla, es un error de compilación
	}

	// El cliente va con la sesión compartida: si ya se entró desde otra de las trece
	// apps, el token está en el llavero y la primera carga ya trae datos. Sin nadie
	// dentro, la carga falla y se pinta el estado de error —con su «Reintentar»—, que
	// es lo honesto; en cuanto se entra desde el menú de cuenta, se vuelve a pedir todo.
	p := erp.NuevaPantalla(t, api.NewERP(s.URL("erp"), s.Sesion, nil, string(l)), nil)
	// Subir documentos e importar CSV usan el selector de ficheros DEL SISTEMA, como el
	// `<input type="file">` de la web.
	switch runtime.GOOS {
	case "js":
	default:
		p.ElegirFicheros = elegirFicheros
	}
	// El correo que pide una factura se escribe en Sytue Mail: por su esquema, o en la
	// web si la app no está instalada.
	p.AbrirApp = func(producto, ruta string) { _ = api.AbrirEn(a, s.Apex(), producto, s.Sesion.Organizacion(), ruta) }
	// El saludo de la portada lleva el nombre de Sytue ID, que el estado transversal no
	// trae (solo el `sub`): es la misma ficha que pide el menú de cuenta.
	saludar := func() {
		go func() {
			ctx, cancelar := context.WithTimeout(context.Background(), 15*time.Second)
			defer cancelar()
			if yo, err := api.NewAccount(s.URL("id"), s.Sesion, nil).Me(ctx); err == nil {
				nombre := yo.Name
				if nombre == "" {
					nombre = yo.Email
				}
				fyne.Do(func() { p.PonerNombre(nombre) })
			}
		}()
	}
	dentro := s.Estado().Dentro()
	s.AlCambiar(func(e sesion.Estado) {
		if e.Dentro() && !dentro {
			p.Arrancar()
			saludar()
		}
		dentro = e.Dentro()
	})

	// El título de la ventana es el NOMBRE DEL PRODUCTO y sale de `FyneApp.toml`, no
	// del catálogo: una marca no se traduce y no tiene clave en la web.
	nombre := "Sytue ERP"
	if m := fyne.CurrentApp().Metadata(); m.Name != "" {
		nombre = m.Name
	}
	w := a.NewWindow(nombre)
	w.SetPadded(false)
	barra := ui.NuevaBarraSuite(t, s, iconos.ERP)
	w.SetContent(barra.Montar(p.Objeto()))
	p.Atajos(w.Canvas(), barra)
	w.Resize(fyne.NewSize(1280, 744))
	p.Arrancar()
	if dentro {
		saludar()
	}
	// El aviso de una tarea asignada abre la vista de tareas (`?vista=tareas`), como la
	// web.
	s.AlAbrirURL(func(u *url.URL) {
		if m, ok := api.LeerPeticionERP(u); ok {
			fyne.Do(func() { p.AtenderPeticion(m) }) // el aviso de una petición de edición
		} else if v, ok := api.LeerVista(u); ok {
			if slices.Contains([]string{erp.VistaTareas, erp.VistaLibro, erp.VistaBalance, erp.VistaObligaciones, erp.VistaBanco}, v) {
				fyne.Do(func() { p.IrA(v) })
			}
		} else if strings.Trim(strings.TrimPrefix(u.Path, "/erp"), "/") != "" || strings.Trim(u.Opaque, "/") != "" {
			s.NoEncontrado() // con un receptor apuntado, la sesión ya no sabe qué no es de nadie
		}
	})
	s.Arrancar(context.Background())
	w.ShowAndRun()
}

// elegirFicheros abre el selector del sistema —bloquea, así que va aparte— y le pasa a
// `listo` lo elegido en el hilo de la interfaz. Cancelar no hace nada.
func elegirFicheros(listo func([]string)) {
	go func() {
		rutas, err := ficheros.Elegir()
		if err != nil {
			if !errors.Is(err, ficheros.ErrNoDisponible) {
				slog.Warn("erp: el selector de ficheros ha fallado", "error", err)
			}
			return
		}
		if len(rutas) > 0 {
			fyne.Do(func() { listo(rutas) })
		}
	}()
}
