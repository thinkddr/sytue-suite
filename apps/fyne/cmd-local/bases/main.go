// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

// Command bases es Sytue Bases de Sytue Office: las mismas tablas relacionales de la
// suite, para una persona, con sus datos en este equipo y en claro, sin cuenta y sin
// servidor (apps/fyne/PLAN-EDICION-LOCAL.md §6.12).
//
// Comparado con `cmd/bases`: no hay `sesion.Abrir` (ni OIDC ni llavero de sesión), ni
// `cifrado.NewPrivateApplication` (ni Matrix ni dispositivo verificado). El repositorio
// de dominio es el mismo (`apps/nucleo/bases`), sobre el almacén en claro de la cáscara
// (`l.Almacen`), y la fachada `api.Bases` va enchufada a él con `WithPrivateData`: lo que
// no tiene camino local (compartir, formularios públicos, tiempo real) no se ofrece y, si
// se llamara, falla cerrado sin abrir ni una conexión (R-RED). La actividad de cada base
// la anota el propio repositorio en `Datos/bases/actividad.jsonl` (B-06).
package main

import (
	"context"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"

	"fyne.io/fyne/v2"

	"git.sytue.eu/sytue/one/apps/fyne/apps/bases"
	"git.sytue.eu/sytue/one/apps/fyne/editor"
	"git.sytue.eu/sytue/one/apps/fyne/iconos"
	"git.sytue.eu/sytue/one/apps/fyne/local"
	"git.sytue.eu/sytue/one/apps/fyne/soltar"
	"git.sytue.eu/sytue/one/apps/fyne/tema"
	"git.sytue.eu/sytue/one/apps/fyne/ui"
	"git.sytue.eu/sytue/one/apps/nucleo/api"
	privateBases "git.sytue.eu/sytue/one/apps/nucleo/bases"
	"git.sytue.eu/sytue/one/apps/nucleo/ficheros"
	"git.sytue.eu/sytue/one/apps/nucleo/local/almacen"
	"git.sytue.eu/sytue/one/apps/nucleo/local/exportar"
)

// sinRed es la raíz que se le da a la fachada: nunca se llama, porque `WithPrivateData`
// cambia el transporte por uno que falla cerrado sin salir del proceso.
const sinRed = "https://sin-red.invalid"

func main() {
	a := local.NuevaApp("")
	l, err := local.Abrir(a, "bases")
	if err != nil {
		slog.Error("bases", "error", err)
		os.Exit(1)
	}
	tema.ElegirCromo(tema.CromoPapel)
	t, err := bases.Traductor(l.Idioma())
	if err != nil {
		slog.Error("no se pudo cargar el catálogo de idiomas", "error", err)
		os.Exit(1)
	}

	nombre := "Sytue Bases Office"
	if m := a.Metadata(); m.Name != "" {
		nombre = m.Name
	}
	w := a.NewWindow(nombre)
	w.SetPadded(false)
	w.Resize(fyne.NewSize(1280, 800))

	fachada, err := abrirBases(l)
	if err != nil {
		slog.Error("bases: no se pudo abrir el almacén", "error", err)
		w.SetContent(l.Montar(w, iconos.Bases, ui.NuevoVacioError(t.T("matrix.failed", nil), err.Error())))
		w.ShowAndRun()
		return
	}

	// «Exportar en formatos abiertos»: cada tabla en CSV, en la carpeta de su base.
	l.AlExportarTodo(func(ctx context.Context, carpeta string) error { return exportar.Bases(ctx, fachada, carpeta) })
	p := bases.NuevaPantalla(t, fachada, w)
	// Sin servicio: ni compartir, ni tiempo real, ni formularios públicos (B-05). La
	// actividad sí: la que anota el propio repositorio (B-06).
	p.SinServidor, p.SinFormularios, p.ActividadLocal = true, true, true
	// Una sola persona: no hay organización a la que compartir, y quien mira es siempre
	// la de esta instalación.
	p.Org = func() string { return "" }
	p.Yo = func() string { return l.Espacio.ID }
	p.Guardar = func(nombre string, r io.Reader) (string, error) {
		carpeta, err := ficheros.Carpeta()
		if err != nil {
			return "", err
		}
		return ficheros.Guardar(carpeta, nombre, r)
	}
	p.Elegir = editor.ElegirDelSistema

	// Adjuntos e importar un CSV: soltar los ficheros sobre la ventana, como en la
	// conectada. El texto soltado se pega en la celda sobre la que cae.
	w.SetOnDropped(p.Soltado)
	soltar.AlArrastrar(w, p.Arrastrando)
	soltar.AlSoltar(w, func(x soltar.Soltado) { p.SoltarTexto(x.Texto, x.Posicion) })

	w.SetContent(l.Montar(w, iconos.Bases, p.Objeto()))
	p.Arrancar()
	// Una base (`?clase=base&id=<base>`) o un registro (`?clase=registro&id=<base>/<tabla>/<registro>`).
	l.AlAbrirElemento(func(clase, id string) {
		switch clase {
		case "base":
			p.Abrir(id)
		case "tabla": // `<base>/<tabla>` (el Hub)
			if b, tabla, ok := strings.Cut(id, "/"); ok {
				p.AbrirTabla(b, tabla)
			}
		case "registro": // `<base>/<tabla>/<registro>`
			if b, resto, ok := strings.Cut(id, "/"); ok {
				tabla, registro, _ := strings.Cut(resto, "/")
				p.AbrirEnlace(b, tabla, registro)
			}
		}
	})
	w.ShowAndRun()
}

// abrirBases monta el repositorio de Bases sobre el almacén en claro y le pone delante
// la fachada que usan las pantallas. El repositorio se cierra con la app, antes que el
// almacén (AlCerrar va del último al primero).
func abrirBases(l *local.App) (*api.Bases, error) {
	ctx := context.Background()
	rec, err := l.Almacen(ctx, "bases")
	if err != nil {
		return nil, err
	}
	repo, err := privateBases.New(rec.Config)
	if err != nil {
		return nil, err
	}
	l.AlCerrar(repo.Close)
	// La actividad de cada base, en claro junto a su almacén (B-06).
	repo.Actividad = privateBases.NuevaActividad(filepath.Join(l.Espacio.Rutas.Producto("bases"), privateBases.FicheroActividad))
	// Un adjunto borrado no se recupera aquí: su fichero se suelta y la compactación del
	// almacén lo quita del disco.
	repo.SoltarAdjuntos = true
	if err := almacen.Preparar(ctx, repo); err != nil {
		return nil, err
	}
	return api.NewBases(sinRed, nil, nil).WithPrivateData(repo)
}
