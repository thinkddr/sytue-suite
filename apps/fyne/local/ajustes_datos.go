// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package local

import (
	"context"
	"errors"
	"fmt"
	"runtime"
	"slices"
	"strings"
	"sync"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/widget"

	"git.sytue.eu/sytue/one/apps/fyne/ui"
	"git.sytue.eu/sytue/one/apps/nucleo/ficheros"
	"git.sytue.eu/sytue/one/apps/nucleo/i18n"
	"git.sytue.eu/sytue/one/apps/nucleo/local/espacio"
	"git.sytue.eu/sytue/one/apps/nucleo/local/traslado"
	"git.sytue.eu/sytue/one/apps/nucleo/sistema/discocifrado"
)

// # Ajustes → Datos (§5.5)
//
// La carpeta de datos: verla, abrirla y MOVERLA con todo lo que tiene. Moverla
// (`nucleo/local/traslado`) copia todo a la nueva, lo comprueba (tamaño y huella de cada
// fichero) y solo entonces la apunta en el perfil; la vieja se queda, y se ofrece
// borrarla. Antes se dice si la nueva no vale (la misma, dentro de la de ahora o con
// cosas), si hay otras apps de la edición abiertas (`Abiertas`) o si no cabe. Como al
// restaurar una copia, la app cierra lo que tiene abierto antes de copiar y se cierra al
// terminar, haya ido bien o no. En el móvil la carpeta es la de la app y no se mueve.
//
// Y si el disco de la carpeta está cifrado (X-10, `nucleo/sistema/discocifrado`): cifrado,
// sin cifrar o no se sabe; en los dos últimos, que los datos van en claro y quien acceda
// al equipo los lee. Y, si los hay, los datos de antes de la migración de Photos y
// Contacts a SQLite, para borrarlos (ajustes_antes.go).

var (
	// cifradoVisto guarda, por carpeta, lo que se supo de su disco: preguntarlo puede
	// lanzar una orden del sistema y no cambia con la app abierta.
	cifradoVisto sync.Map
	// mirarCifrado, abiertasDeLaEdicion y moverCarpeta son los del sistema; las pruebas
	// los cambian.
	mirarCifrado        = discocifrado.De
	abiertasDeLaEdicion = func(cs []string) ([]string, bool) { return Abiertas(cs, runtime.GOOS) }
	moverCarpeta        = traslado.Mover
)

func (l *App) ajustesDatos(w fyne.Window) fyne.CanvasObject {
	ruta := widget.NewLabel(l.Espacio.Rutas.Datos)
	ruta.Wrapping = fyne.TextWrapBreak
	cambiar := widget.NewButton(l.T("datos.cambiar"), func() {
		ElegirCarpeta(l.T("datos.carpeta"), func(elegida string) {
			l.moverDatos(w, traslado.Destino(elegida, espacio.CarpetaDatos))
		})
	})
	abrir := widget.NewButton(l.T("datos.abrir"), func() { l.Fallo(w, ficheros.Abrir(l.Espacio.Rutas.Datos)) })
	secretos := l.T("datos.secretos.no")
	if l.Secretos.EnLlavero() {
		secretos = l.T("datos.secretos.si")
	}
	caja := container.NewVBox(negrita(l.T("datos.carpeta")), ruta)
	if Tactil() {
		// En el teléfono la carpeta es la de la app: ni se abre ni se mueve.
		caja.Add(ayuda(l.T("datos.ayuda")))
	} else {
		caja.Add(container.NewHBox(cambiar, abrir))
		caja.Add(ayuda(l.T("datos.ayuda") + " " + l.T("datos.mover.ayuda")))
	}
	caja.Add(widget.NewSeparator())
	caja.Add(negrita(l.T("datos.cifrado")))
	caja.Add(l.estadoCifrado())
	caja.Add(widget.NewSeparator())
	caja.Add(ayuda(secretos))
	// Lo de antes de la migración de Photos y Contacts, si lo hay (ajustes_antes.go).
	caja.Add(l.datosDeAntesDeMigrar(w))
	return caja
}

// estadoCifrado es la línea del cifrado del disco de la carpeta de datos: «mirando…» y,
// en cuanto se sabe (fuera del hilo de la interfaz), lo que es.
func (l *App) estadoCifrado() *widget.Label {
	etiqueta := ayuda(l.T("datos.cifrado.mirando"))
	datos := l.Espacio.Rutas.Datos
	if v, ok := cifradoVisto.Load(datos); ok {
		if r, ok := v.(discocifrado.Resultado); ok {
			l.pintarCifrado(etiqueta, r)
			return etiqueta
		}
	}
	go func() {
		ctx, parar := context.WithTimeout(context.Background(), 30*time.Second)
		defer parar()
		r := mirarCifrado(ctx, datos)
		cifradoVisto.Store(datos, r)
		fyne.Do(func() { l.pintarCifrado(etiqueta, r) })
	}()
	return etiqueta
}

func (l *App) pintarCifrado(etiqueta *widget.Label, r discocifrado.Resultado) {
	etiqueta.SetText(l.textoCifrado(r))
	if r.Estado != discocifrado.Cifrado {
		etiqueta.Importance = widget.WarningImportance
	}
	etiqueta.Refresh()
}

// textoCifrado es lo que se dice de un disco: con qué está cifrado o, si no lo está o no
// se sabe, el aviso de X-10.
func (l *App) textoCifrado(r discocifrado.Resultado) string {
	switch r.Estado {
	case discocifrado.Cifrado:
		return fmt.Sprintf(l.T("datos.cifrado.si"), r.Como)
	case discocifrado.SinCifrar:
		return l.T("datos.cifrado.no")
	case discocifrado.NoSeSabe:
	}
	return l.T("datos.cifrado.nose")
}

// carpetaPropia es la de esta app en `cmd-local` («docus»), por su identificador: el
// mismo con que escucha su copia única (instancia.go), que sin metadatos de empaquetado
// es el de la app.
func (l *App) carpetaPropia() string {
	return strings.TrimPrefix(idDeCopia(l.A), espacio.PrefijoID)
}

// otrasApps son las demás apps de la edición, por su carpeta.
func (l *App) otrasApps() []string {
	propia := l.carpetaPropia()
	var out []string
	for c := range AppsDeLaEdicion {
		if c != propia {
			out = append(out, c)
		}
	}
	slices.Sort(out)
	return out
}

// nombresDeApps son los nombres de esas apps, en una línea.
func nombresDeApps(carpetas []string) string {
	nombres := make([]string, 0, len(carpetas))
	for _, c := range carpetas {
		nombres = append(nombres, NombreDeApp(c))
	}
	return strings.Join(nombres, ", ")
}

// moverDatos lleva la carpeta de datos a destino: antes de tocar nada, dice si no vale,
// si hay otras apps abiertas o si no cabe, y pide confirmarlo.
func (l *App) moverDatos(w fyne.Window, destino string) {
	origen := l.Espacio.Rutas.Datos
	titulo := l.T("datos.mover")
	if err := traslado.Comprobar(origen, destino); err != nil {
		Info(w, titulo, l.textoTraslado(err, destino)+"\n\n"+destino)
		return
	}
	abiertas, sabido := abiertasDeLaEdicion(l.otrasApps())
	if len(abiertas) > 0 {
		Info(w, titulo, fmt.Sprintf(l.T("datos.mover.cerrar"), nombresDeApps(abiertas)))
		return
	}
	aviso := Espera(w, titulo, l.T("datos.mover.midiendo"))
	go func() {
		ctx, parar := context.WithTimeout(context.Background(), 10*time.Minute)
		defer parar()
		_, bytes, err := traslado.Medir(ctx, origen)
		libre, errLibre := traslado.LibreEn(destino)
		fyne.Do(func() {
			aviso.Cerrar()
			if err != nil {
				l.Fallo(w, err)
				return
			}
			if errLibre == nil && libre < bytes {
				Info(w, titulo, fmt.Sprintf(l.T("datos.mover.sinSitio"), i18n.FormatBytes(l.Idioma(), float64(libre))))
				return
			}
			texto := fmt.Sprintf(l.T("datos.mover.pregunta"), i18n.FormatBytes(l.Idioma(), float64(bytes))) + "\n\n" + origen + "\n→ " + destino
			if !sabido {
				texto += "\n\n" + l.T("datos.mover.cerrarTodas")
			}
			Confirmar(w, titulo, texto, l.T("cancelar"), l.T("datos.mover.verbo"), false, func() { l.hacerTraslado(w, origen, destino) })
		})
	}()
}

// hacerTraslado mueve, con el progreso a la vista y la posibilidad de cancelarlo.
func (l *App) hacerTraslado(w fyne.Window, origen, destino string) {
	ctx, cancelar := context.WithCancel(context.Background())
	fase := widget.NewLabel(l.T("datos.mover.midiendo"))
	barra := widget.NewProgressBar()
	cuanto := ayuda("")
	parar := widget.NewButton(l.T("cancelar"), nil)
	parar.OnTapped = func() {
		parar.Disable()
		cancelar()
	}
	d := ui.NuevoDialogo(l.T("datos.mover"), container.NewVBox(fase, barra, cuanto, ayuda(destino), container.NewHBox(parar)))
	// Ni el velo ni Escape lo quitan sin más: piden cancelar, como el botón.
	d.NoDescartable = true
	d.AlCerrar = parar.OnTapped
	d.Con(ui.DialogoSM, nil).Mostrar(w.Canvas())
	idioma := l.Idioma()
	go func() {
		defer cancelar()
		// Lo que esta app tiene abierto (sus almacenes, sus bases) se cierra antes: el
		// traslado lo toma entero. La app no sigue después: se vuelve a abrir.
		l.Cerrar()
		inf, err := moverCarpeta(ctx, traslado.Opciones{
			Origen: origen, Destino: destino,
			Apuntar: func(nueva string) error {
				var err error
				fyne.DoAndWait(func() {
					p := l.Espacio.Perfil
					p.Datos = nueva
					err = l.Espacio.GuardarPerfil(p)
				})
				return err
			},
			Progreso: func(p traslado.Progreso) {
				fyne.Do(func() { l.pintarProgreso(idioma, fase, barra, cuanto, p) })
			},
			Libre: traslado.LibreEn,
		})
		fyne.Do(func() {
			d.Cerrar()
			l.trasTraslado(w, inf, err)
		})
	}()
}

func (l *App) pintarProgreso(idioma i18n.Locale, fase *widget.Label, barra *widget.ProgressBar, cuanto *widget.Label, p traslado.Progreso) {
	switch p.Fase {
	case traslado.Midiendo:
		fase.SetText(l.T("datos.mover.midiendo"))
	case traslado.Copiando:
		fase.SetText(l.T("datos.mover.copiando"))
	default:
		fase.SetText(l.T("datos.mover.comprobando"))
	}
	if p.Total <= 0 || p.Fase == traslado.Midiendo {
		barra.SetValue(0)
		cuanto.SetText("")
		return
	}
	barra.SetValue(min(1, float64(p.Hecho)/float64(p.Total)))
	cuanto.SetText(i18n.FormatBytes(idioma, float64(p.Hecho)) + " / " + i18n.FormatBytes(idioma, float64(p.Total)))
}

// trasTraslado dice cómo ha ido. Bien: dónde están los datos, y ofrece borrar la carpeta
// vieja. Mal o cancelado: que no ha cambiado nada. En los dos casos la app se cierra.
func (l *App) trasTraslado(w fyne.Window, inf traslado.Informe, err error) {
	if err != nil {
		texto := l.T("datos.mover.nada")
		if m := l.textoTraslado(err, inf.Destino); m != "" {
			texto += "\n\n" + m
		}
		l.salirCon(w, ayuda(texto))
		return
	}
	principal := ayuda(l.T("datos.mover.hecho") + "\n\n" + inf.Destino)
	resultado := ayuda("")
	resultado.Hide()
	borrar := widget.NewButton(l.T("datos.mover.borrar"), nil)
	borrar.Importance = widget.DangerImportance
	borrar.OnTapped = func() {
		Confirmar(w, l.T("datos.mover.borrar"), l.T("datos.mover.borrar.pregunta")+"\n\n"+inf.Origen, l.T("cancelar"), l.T("datos.mover.borrar"), true, func() {
			borrar.Disable()
			go func() {
				quedan, err := traslado.BorrarOrigen(inf)
				fyne.Do(func() {
					texto := l.T("datos.mover.borrada")
					if quedan > 0 || err != nil {
						texto = l.T("datos.mover.quedan") + "\n" + inf.Origen
					} else {
						// La de antes ya no está: lo que queda por decir es dónde están.
						principal.SetText(l.T("datos.carpeta") + ": " + inf.Destino)
					}
					borrar.Hide()
					resultado.SetText(texto)
					resultado.Show()
				})
			}()
		})
	}
	l.salirCon(w, container.NewVBox(principal, resultado, container.NewHBox(borrar)))
}

// salirCon enseña el final de un traslado con «Cerrar la app»: lo que la app tenía
// abierto ya está cerrado, así que cerrarlo (el aspa, Escape) también cierra la app.
func (l *App) salirCon(w fyne.Window, contenido fyne.CanvasObject) {
	cuerpo := container.NewVBox(contenido)
	salir := widget.NewButton(l.T("copia.salir"), l.A.Quit)
	salir.Importance = widget.HighImportance
	cuerpo.Add(container.NewHBox(salir))
	d := ui.NuevoDialogo(l.T("datos.mover"), cuerpo)
	d.NoDescartable = true
	d.AlCerrar = l.A.Quit
	d.Con(ui.DialogoSM, nil).Mostrar(w.Canvas())
}

// textoTraslado es lo que se le dice a la persona de un error del traslado; "" si no hay
// nada que añadir (lo canceló ella).
func (l *App) textoTraslado(err error, destino string) string {
	switch {
	case errors.Is(err, context.Canceled):
		return ""
	case errors.Is(err, traslado.ErrMisma):
		return l.T("datos.mover.misma")
	case errors.Is(err, traslado.ErrDentro):
		return l.T("datos.mover.dentro")
	case errors.Is(err, traslado.ErrNoVacia):
		return l.T("datos.mover.noVacia")
	case errors.Is(err, traslado.ErrEnUso):
		return l.T("datos.mover.enUso")
	case errors.Is(err, traslado.ErrCopiaMala):
		return l.T("datos.mover.malo") + "\n" + err.Error()
	case errors.Is(err, traslado.ErrSinSitio):
		libre, _ := traslado.LibreEn(destino)
		return fmt.Sprintf(l.T("datos.mover.sinSitio"), i18n.FormatBytes(l.Idioma(), float64(libre)))
	}
	return err.Error()
}
