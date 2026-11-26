// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package local

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/widget"

	"git.sytue.eu/sytue/one/apps/nucleo/i18n"
	"git.sytue.eu/sytue/one/apps/nucleo/local/almacen"
	"git.sytue.eu/sytue/one/apps/nucleo/local/fototeca"
	"git.sytue.eu/sytue/one/apps/nucleo/local/libreta"
)

// # Ajustes → Datos: los datos de antes de la migración
//
// Photos y Contacts pasaron su almacén de registros a SQLite (el plan B de §5.6) y lo
// dejaron en su carpeta, sin tocar: nunca se borra solo. Aquí se ofrece borrarlo, app
// por app, SOLO si su migración está hecha y comprobada (su marca) y queda algo: se dice
// cuánto ocupa, se pide confirmarlo y se borra solo lo del almacén viejo
// (`fototeca.BorrarAntes`: de `objetos/`, que es de las dos, lo que la fototeca ya no
// cita, con su cerrojo de escritura; `libreta.BorrarAntes`: el almacén entero, que la
// libreta no usa). Antes, las demás apps que usan esos datos tienen que estar cerradas
// (`Abiertas`); la propia no hace falta, porque lo vivo de su base no se toca. Y si una
// copia de antes tiene el almacén abierto, su cerrojo lo dice y no se toca nada.

// datosDeAntes es lo de antes de la migración de una app.
type datosDeAntes struct {
	// app es su carpeta de `cmd-local` y producto, el de su carpeta de datos.
	app, producto string
	medir, borrar func(ctx context.Context, dir string) (almacen.Antes, error)
	// usan son las apps que leen o escriben esos datos: cerradas al borrar.
	usan []string
}

// antesDeLaMigracion son las apps que pasaron a SQLite y dejaron su almacén al lado.
var antesDeLaMigracion = []datosDeAntes{
	// El Hub trae a la fototeca, la busca y la exporta.
	{app: "photos", producto: "photos", medir: fototeca.MedirAntes, borrar: fototeca.BorrarAntes, usan: []string{"photos", "hub"}},
	// Mail y Calendar leen la libreta (`contacts/lector`); el Hub trae a ella y la busca.
	{app: "contacts", producto: "contacts", medir: libreta.MedirAntes, borrar: libreta.BorrarAntes, usan: []string{"contacts", "hub", "mail", "calendar"}},
}

// datosDeAntesDeMigrar es la sección de Ajustes → Datos con lo de antes de la migración
// de cada app: vacía hasta que se sabe (medirlo recorre `objetos/`), y si no hay nada
// que borrar se queda así.
func (l *App) datosDeAntesDeMigrar(w fyne.Window) *fyne.Container {
	caja := container.NewVBox()
	filas := container.NewVBox()
	go func() {
		ctx, parar := context.WithTimeout(context.Background(), 5*time.Minute)
		defer parar()
		for _, d := range antesDeLaMigracion {
			dir := l.Espacio.Rutas.Producto(d.producto)
			a, err := d.medir(ctx, dir)
			if err != nil || !a.Migrada || a.Ficheros == 0 {
				continue
			}
			fyne.Do(func() {
				if len(caja.Objects) == 0 {
					caja.Add(widget.NewSeparator())
					caja.Add(negrita(l.T("datos.antes")))
					caja.Add(ayuda(l.T("datos.antes.ayuda")))
					caja.Add(filas)
				}
				filas.Add(l.filaDeAntes(w, d, dir, a))
			})
		}
	}()
	return caja
}

// filaDeAntes es lo de una app: cuánto ocupa y el botón de borrarlo.
func (l *App) filaDeAntes(w fyne.Window, d datosDeAntes, dir string, a almacen.Antes) fyne.CanvasObject {
	nombre := NombreDeApp(d.app)
	cuanto := ayuda(fmt.Sprintf(l.T("datos.antes.ocupa"), nombre, i18n.FormatBytes(l.Idioma(), float64(a.Bytes))))
	borrar := widget.NewButton(l.T("datos.antes.borrar"), nil)
	borrar.Importance = widget.DangerImportance
	fila := container.NewVBox(cuanto, container.NewHBox(borrar))
	borrar.OnTapped = func() { l.borrarDeAntes(w, d, dir, a, cuanto, borrar) }
	return fila
}

// borrarDeAntes borra lo de antes de la migración de una app: antes dice si hay apps que
// cerrar y pide confirmarlo; después, cuánto se fue (o qué quedó).
func (l *App) borrarDeAntes(w fyne.Window, d datosDeAntes, dir string, a almacen.Antes, cuanto *widget.Label, boton *widget.Button) {
	titulo := l.T("datos.antes.borrar")
	propia := l.carpetaPropia()
	otras := slices.DeleteFunc(slices.Clone(d.usan), func(c string) bool { return c == propia })
	abiertas, sabido := abiertasDeLaEdicion(otras)
	if len(abiertas) > 0 {
		Info(w, titulo, fmt.Sprintf(l.T("datos.mover.cerrar"), nombresDeApps(abiertas)))
		return
	}
	nombre := NombreDeApp(d.app)
	idioma := l.Idioma()
	texto := fmt.Sprintf(l.T("datos.antes.pregunta"), nombre, i18n.FormatBytes(idioma, float64(a.Bytes)))
	if !sabido && len(otras) > 0 {
		texto += "\n\n" + fmt.Sprintf(l.T("datos.antes.cerrarTodas"), nombresDeApps(otras))
	}
	Confirmar(w, titulo, texto, l.T("cancelar"), l.T("datos.antes.verbo"), true, func() {
		boton.Disable()
		aviso := Espera(w, titulo, l.T("datos.antes.borrando"))
		go func() {
			ctx, parar := context.WithTimeout(context.Background(), 30*time.Minute)
			defer parar()
			hecho, err := d.borrar(ctx, dir)
			queda, errQueda := d.medir(ctx, dir)
			fyne.Do(func() {
				aviso.Cerrar()
				switch {
				case errors.Is(err, fototeca.ErrEnUso), errors.Is(err, libreta.ErrEnUso):
					boton.Enable()
					Info(w, titulo, l.T("datos.mover.enUso"))
					return
				case err != nil || errQueda != nil || queda.Ficheros > 0:
					boton.Enable()
					cuanto.SetText(fmt.Sprintf(l.T("datos.antes.ocupa"), nombre, i18n.FormatBytes(idioma, float64(queda.Bytes))))
					mensaje := fmt.Sprintf(l.T("datos.antes.quedan"), nombre)
					if err != nil {
						mensaje += "\n\n" + err.Error()
					}
					Info(w, titulo, mensaje)
					return
				}
				boton.Hide()
				cuanto.SetText(fmt.Sprintf(l.T("datos.antes.hecho"), nombre, i18n.FormatBytes(idioma, float64(hecho.Bytes))))
			})
		}()
	})
}
