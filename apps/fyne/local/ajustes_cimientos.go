// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package local

import (
	"context"
	"strconv"
	"strings"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/widget"

	"git.sytue.eu/sytue/one/apps/fyne/ui"
	"git.sytue.eu/sytue/one/apps/nucleo/ficheros"
	"git.sytue.eu/sytue/one/apps/nucleo/i18n"
	"git.sytue.eu/sytue/one/apps/nucleo/local/red"
)

// Las pestañas de Ajustes de los cimientos (§5.5): Cuentas, Red y Copia de seguridad.

func negrita(texto string) *widget.Label {
	return widget.NewLabelWithStyle(texto, fyne.TextAlignLeading, fyne.TextStyle{Bold: true})
}

// ajustesRed enseña a dónde puede conectarse la app ahora mismo y por qué, con el
// interruptor «Sin conexión».
func (l *App) ajustesRed(w fyne.Window) fyne.CanvasObject {
	lista := container.NewVBox()
	pintar := func() {
		lista.RemoveAll()
		ds := l.Red.Destinos()
		if len(ds) == 0 {
			lista.Add(ayuda(l.T("red.ninguno")))
		}
		for _, d := range ds {
			host := d.Host
			if strings.HasPrefix(host, ".") {
				host = "*" + host
			}
			if d.Puerto != 0 {
				host += ":" + strconv.Itoa(d.Puerto)
			}
			por := l.T("red.para." + string(d.Proposito))
			if d.Motivo != "" {
				por += " — " + d.Motivo
			}
			if d.Claro && !red.EsLocal(d.Host) {
				por += " · " + l.T("red.sinTLS")
			}
			if d.Temporal {
				por += " · " + l.T("red.temporal")
			}
			lista.Add(container.NewVBox(negrita(host), ayuda(por)))
		}
		lista.Refresh()
	}
	pintar()
	sin := widget.NewCheck(l.T("red.sinConexion"), func(si bool) {
		l.Fallo(w, l.Red.PonerSinConexion(si))
		pintar()
	})
	sin.SetChecked(l.Red.SinConexion())
	return container.NewVBox(ayuda(l.T("red.ayuda")), sin, ayuda(l.T("red.sinConexion.ayuda")), widget.NewSeparator(), negrita(l.T("red.destinos")), lista)
}

// ajustesCopia crea, lista y restaura copias de seguridad.
func (l *App) ajustesCopia(w fyne.Window, reabrir func()) fyne.CanvasObject {
	caja := container.NewVBox(ayuda(l.T("copia.ayuda")))
	crear := widget.NewButton(l.T("copia.crear"), func() {
		aviso := Espera(w, l.T("copia"), l.T("copia.creando"))
		go func() {
			ctx, parar := context.WithTimeout(context.Background(), time.Hour)
			defer parar()
			ruta, err := l.Copia.Crear(ctx)
			fyne.Do(func() {
				aviso.Cerrar()
				if err != nil {
					l.Fallo(w, err)
					return
				}
				Info(w, l.T("copia"), l.T("copia.hecha")+"\n\n"+ruta)
				reabrir()
			})
		}()
	})
	crear.Importance = widget.HighImportance
	restaurar := widget.NewButton(l.T("copia.restaurarOtra"), func() {
		ElegirFichero(func(rutas []string) {
			if len(rutas) > 0 {
				l.confirmarRestaurar(w, rutas[0])
			}
		})
	})
	abrir := widget.NewButton(l.T("datos.abrir"), func() { l.Fallo(w, ficheros.Abrir(l.Copia.CarpetaCopias())) })
	if Tactil() {
		abrir.Hide()
	}
	caja.Add(container.NewHBox(crear, restaurar, abrir))
	// Lo de esta app en formatos abiertos, para llevárselo a otro programa (AlExportarTodo).
	l.mu.Lock()
	exporta := l.exportarTodo != nil
	l.mu.Unlock()
	if exporta {
		caja.Add(widget.NewSeparator())
		caja.Add(negrita(l.T("copia.exportar")))
		caja.Add(ayuda(l.T("copia.exportar.ayuda")))
		caja.Add(container.NewHBox(widget.NewButton(l.T("copia.exportar")+"…", func() { l.exportarAbierto(w) })))
	}
	caja.Add(widget.NewSeparator())
	caja.Add(negrita(l.T("copia.lista")))
	lista, err := l.Copia.Lista()
	if err != nil {
		caja.Add(ayuda(err.Error()))
	}
	if len(lista) == 0 {
		caja.Add(ayuda(l.T("copia.ninguna")))
	}
	for _, c := range lista {
		ruta := c.Ruta
		detalle := c.Creada.Local().Format("2006-01-02 15:04") + " · " + i18n.FormatBytes(l.Idioma(), float64(c.Bytes))
		caja.Add(container.NewBorder(nil, nil, nil, widget.NewButton(l.T("copia.restaurar"), func() { l.confirmarRestaurar(w, ruta) }),
			container.NewVBox(widget.NewLabel(c.Nombre), ayuda(detalle))))
	}
	return caja
}

func (l *App) confirmarRestaurar(w fyne.Window, ruta string) {
	Confirmar(w, l.T("copia.restaurar"), l.T("copia.restaurar.pregunta"), l.T("cancelar"), l.T("copia.restaurar"), true, func() {
		aviso := Espera(w, l.T("copia"), l.T("copia.restaurando"))
		go func() {
			ctx, parar := context.WithTimeout(context.Background(), time.Hour)
			defer parar()
			// Lo que la app tiene abierto se cierra antes de sustituirlo.
			l.Cerrar()
			previa, err := l.Copia.Restaurar(ctx, ruta)
			fyne.Do(func() {
				aviso.Cerrar()
				if err != nil {
					l.Fallo(w, err)
				}
				texto := l.T("copia.restaurada")
				if previa != "" {
					texto += "\n\n" + l.T("copia.previa") + " " + previa
				}
				var d *ui.Dialogo
				d = Ventana(w, l.T("copia"), container.NewVBox(ayuda(texto), widget.NewButton(l.T("copia.salir"), func() {
					d.Cerrar()
					l.A.Quit()
				})), nil)
			})
		}()
	})
}
