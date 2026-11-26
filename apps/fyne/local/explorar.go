// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package local

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/storage"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"

	"git.sytue.eu/sytue/one/apps/fyne/ui"
	"git.sytue.eu/sytue/one/apps/nucleo/local/biblioteca"
	"git.sytue.eu/sytue/one/apps/nucleo/local/nube"
)

// ExplorarNube abre el explorador de la nube. Para abrir (guardar=false): lo elegido se
// descarga a `<Datos>/Nube/<cuenta>/` y se abre en esta app como cualquier fichero de la
// máquina. Para guardar: se elige carpeta y se sube nombre con datos.
//
// Las carpetas que ya sincroniza el sistema se ofrecen también, con el selector de
// ficheros del sistema: ahí no hay nada que descargar.
func (l *App) ExplorarNube(w fyne.Window, guardar bool, nombre string, datos []byte) {
	cuentas := l.Nube.Cuentas()
	sinc := nube.Sincronizadas()
	var d *ui.Dialogo
	lista := container.NewVBox(widget.NewLabel(l.T("nube.elige")))
	for _, c := range cuentas {
		lista.Add(widget.NewButtonWithIcon(c.Rotulo+"  "+c.Usuario, theme.StorageIcon(), func() {
			d.Cerrar()
			p, err := l.Nube.Proveedor(c.ID)
			if err != nil {
				l.Fallo(w, err)
				return
			}
			l.navegar(w, c, p, guardar, nombre, datos)
		}))
	}
	for _, s := range sinc {
		lista.Add(widget.NewButtonWithIcon(s.Nombre, theme.FolderIcon(), func() {
			d.Cerrar()
			l.enCarpeta(w, s.Ruta, guardar, nombre, datos)
		}))
	}
	if len(cuentas) == 0 && len(sinc) == 0 {
		lista.Add(ayuda(l.T("nube.ninguna")))
	}
	lista.Add(widget.NewButtonWithIcon(l.T("ajustes")+"…", theme.SettingsIcon(), func() { d.Cerrar(); l.MostrarAjustes(w) }))
	d = ui.NuevoDialogo(l.T("nube"), lista).Con(ui.DialogoSM, nil)
	d.Mostrar(w.Canvas())
}

// enCarpeta trabaja en una carpeta sincronizada: guardar escribe ahí el fichero, sin más
// (su cliente lo sube); abrir usa el selector del sistema.
func (l *App) enCarpeta(w fyne.Window, ruta string, guardar bool, nombre string, datos []byte) {
	if guardar {
		destino := filepath.Join(ruta, nombreSeguro(nombre))
		if err := biblioteca.EscribirAtomico(destino, datos); err != nil {
			l.Fallo(w, err)
			return
		}
		Info(w, l.T("nube"), l.T("nube.subido")+"\n"+destino)
		return
	}
	ElegirFichero(func(rutas []string) { l.AbrirLocal(rutas[0]) })
}

func (l *App) recibirTexto(s string) {
	if u, ok := uriDeArgumento(s, nil); ok {
		l.recibir(u)
	}
}

// navegar recorre las carpetas de una cuenta.
func (l *App) navegar(w fyne.Window, c nube.Cuenta, p nube.Proveedor, guardar bool, nombre string, datos []byte) {
	type nivel struct{ id, nombre string }
	pila := []nivel{{"", c.Rotulo}}
	titulo := widget.NewLabelWithStyle("", fyne.TextAlignLeading, fyne.TextStyle{Bold: true})
	cuerpo := container.NewVBox()
	var d *ui.Dialogo
	var cargar func()
	cargar = func() {
		actual := pila[len(pila)-1]
		partes := make([]string, len(pila))
		for i, n := range pila {
			partes[i] = n.nombre
		}
		titulo.SetText(strings.Join(partes, " / "))
		cuerpo.Objects = []fyne.CanvasObject{widget.NewProgressBarInfinite()}
		cuerpo.Refresh()
		go func() {
			ctx, parar := context.WithTimeout(context.Background(), time.Minute)
			defer parar()
			es, err := p.Listar(ctx, actual.id)
			fyne.Do(func() {
				cuerpo.Objects = nil
				if err != nil {
					cuerpo.Add(ayuda(err.Error()))
				}
				if len(pila) > 1 {
					cuerpo.Add(widget.NewButtonWithIcon(l.T("nube.atras"), theme.NavigateBackIcon(), func() { pila = pila[:len(pila)-1]; cargar() }))
				}
				if err == nil && len(es) == 0 {
					cuerpo.Add(ayuda(l.T("nube.vacia")))
				}
				for _, e := range es {
					cuerpo.Add(botonDeNube(e, guardar,
						func() { pila = append(pila, nivel{e.ID, e.Nombre}); cargar() },
						func() { d.Cerrar(); l.descargar(w, c, p, e) }))
				}
				cuerpo.Refresh()
			})
		}()
	}
	acciones := container.NewHBox()
	if guardar {
		nom := widget.NewEntry()
		nom.SetText(nombre)
		acciones.Add(container.NewGridWrap(fyne.NewSize(240, nom.MinSize().Height), nom))
		acciones.Add(widget.NewButtonWithIcon(l.T("nube.guardarAqui"), theme.UploadIcon(), func() {
			l.subir(w, p, pila[len(pila)-1].id, strings.TrimSpace(nom.Text), datos, func() { d.Cerrar() })
		}))
	} else {
		acciones.Add(widget.NewButtonWithIcon(l.T("nube.subir"), theme.UploadIcon(), func() {
			ElegirFichero(func(rutas []string) {
				nombre, contenido, err := LeerLocal(storage.NewFileURI(rutas[0]).String())
				if err != nil {
					l.Fallo(w, err)
					return
				}
				l.subir(w, p, pila[len(pila)-1].id, nombre, contenido, cargar)
			})
		}))
	}
	d = ui.NuevoDialogo(l.T("nube"), container.NewVBox(titulo, cuerpo)).Con(ui.DialogoLG, acciones)
	d.Mostrar(w.Canvas())
	cargar()
}

// botonDeNube es la fila de un elemento de la nube: una carpeta entra en ella (entrar) y
// un fichero se descarga y se abre (abrir); al guardar, los ficheros se ven sin pulsarse.
func botonDeNube(e nube.Elemento, guardar bool, entrar, abrir func()) fyne.CanvasObject {
	switch {
	case e.Carpeta:
		return widget.NewButtonWithIcon(e.Nombre, theme.FolderIcon(), entrar)
	case !guardar:
		return widget.NewButtonWithIcon(e.Nombre, theme.FileIcon(), abrir)
	default:
		b := widget.NewButtonWithIcon(e.Nombre, theme.FileIcon(), nil)
		b.Disable()
		return b
	}
}

func (l *App) subir(w fyne.Window, p nube.Proveedor, carpeta, nombre string, datos []byte, listo func()) {
	if nombre == "" {
		return
	}
	go func() {
		ctx, parar := context.WithTimeout(context.Background(), 5*time.Minute)
		defer parar()
		_, err := p.Subir(ctx, carpeta, nombre, datos)
		fyne.Do(func() {
			if err != nil {
				l.Fallo(w, err)
				return
			}
			Info(w, l.T("nube"), l.T("nube.subido"))
			listo()
		})
	}()
}

// descargar trae el fichero a `<Datos>/Nube/<cuenta>/` y lo abre en esta app.
func (l *App) descargar(w fyne.Window, c nube.Cuenta, p nube.Proveedor, e nube.Elemento) {
	espera := Espera(w, l.T("nube"), l.T("nube.descargando"))
	go func() {
		ctx, parar := context.WithTimeout(context.Background(), 5*time.Minute)
		defer parar()
		datos, nombre, err := p.Descargar(ctx, e)
		var ruta string
		if err == nil {
			ruta = filepath.Join(l.Espacio.Rutas.Datos, "Nube", nombreSeguro(c.Rotulo), nombreSeguro(nombre))
			if err = os.MkdirAll(filepath.Dir(ruta), 0o700); err == nil {
				err = biblioteca.EscribirAtomico(ruta, datos)
			}
		}
		fyne.Do(func() {
			espera.Cerrar()
			if err != nil {
				l.Fallo(w, err)
				return
			}
			l.AbrirLocal(ruta)
		})
	}()
}

// nombreSeguro quita lo que no puede ir en un nombre de fichero en ningún sistema.
func nombreSeguro(s string) string {
	s = strings.Map(func(r rune) rune {
		if strings.ContainsRune(`/\:*?"<>|`, r) || r < 32 {
			return '_'
		}
		return r
	}, strings.TrimSpace(s))
	if s == "" || s == "." || s == ".." {
		return "fichero"
	}
	return s
}
