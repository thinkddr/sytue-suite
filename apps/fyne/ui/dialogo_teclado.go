// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package ui

import (
	"runtime"
	"sync"
	"time"

	"fyne.io/fyne/v2"

	"git.sytue.eu/sytue/one/apps/fyne/sesion"
)

// El teclado del teléfono y la hoja de un diálogo.
//
// En la web la capa del Modal mide el viewport VISUAL (`--sytue-vvh`), así que al salir el
// teclado la hoja se apoya encima de él, el cuerpo encoge y el campo se desplaza a la
// vista. En Fyne nada de eso pasaba: en Android ni el lienzo ni su zona interactiva
// encogen, y el campo en el que se escribía —el asunto de un evento, el nombre de una
// carpeta— quedaba DEBAJO del teclado, con «Guardar» también tapado. Solo Cast lo
// arreglaba, y solo para su página.

// AltoTeclado dice lo que el teclado en pantalla tapa AHORA desde abajo de la pantalla,
// en unidades del lienzo `c` (0 si no está). Solo se pregunta en el teléfono; un
// guardián lo sustituye por su propio teclado.
//
// sesion.AltoTeclado contesta en PÍXELES en Android y en PUNTOS en iOS, y las unidades de
// Fyne en iOS ya son puntos: dividir también allí por la escala dejaba el teclado en un
// tercio en un iPhone de @3x.
var AltoTeclado = func(c fyne.Canvas) float32 {
	alto := sesion.AltoTeclado()
	if runtime.GOOS == "android" && c != nil && c.Scale() > 0 {
		alto /= c.Scale()
	}
	return alto
}

// tapadoPorTeclado es lo que el teclado se come de la capa del diálogo por abajo. La capa
// vive en `InteractiveArea`, que ya deja fuera la barra de inicio —y, en iOS, el propio
// teclado—: solo cuenta lo que pisa DENTRO de ella, así que no se descuenta dos veces.
func (d *Dialogo) tapadoPorTeclado(lienzo fyne.Size) float32 {
	if d.teclado <= 0 {
		return 0
	}
	tapa := d.teclado
	if c := lienzoDe(d.capa); c != nil {
		debajo := c.Size().Height - d.capa.Position().Y - lienzo.Height
		tapa -= max(debajo, 0)
	}
	// Siempre queda sitio para la cabecera: un teclado que lo tapara todo dejaría la
	// hoja sin aspa.
	return min(max(tapa, 0), max(lienzo.Height-HuecoHoja-AltoCabeceraDialogo(), 0))
}

// ponerTeclado apoya la hoja sobre un teclado que tapa `alto` y deja a la vista el campo
// enfocado. Con 0, la hoja vuelve abajo.
func (d *Dialogo) ponerTeclado(c fyne.Canvas, alto float32) {
	if d.capa == nil {
		return
	}
	if d.teclado != alto {
		d.teclado = alto
		d.capa.Refresh()
	}
	if alto > 0 {
		MostrarFoco(d.Desplazador(), c, 0)
	}
}

// dialogoArriba es el diálogo de la capa de más arriba de `c`, si lo hay.
func dialogoArriba(c fyne.Canvas) *Dialogo {
	lista := c.Overlays().List()
	for i := len(lista) - 1; i >= 0; i-- {
		if cp, es := lista[i].(*capa); es {
			if d, es := cp.panel.(*Dialogo); es {
				return d
			}
			return nil
		}
	}
	return nil
}

var (
	tecladoMu   sync.Mutex
	tecladoAlto = map[fyne.Canvas]float32{} // los lienzos que se siguen, con su último alto
)

// seguirTeclado mide el teclado cuando termina de salir —su animación tarda, más de
// 400 ms en el emulador: se espera a que el alto deje de crecer— y lo sigue hasta que se
// va. Si ya se seguía, solo deja a la vista el campo nuevo.
func seguirTeclado(c fyne.Canvas) {
	medir := AltoTeclado
	if medir == nil {
		return
	}
	tecladoMu.Lock()
	alto, siguiendo := tecladoAlto[c]
	if !siguiendo {
		tecladoAlto[c] = 0
	}
	tecladoMu.Unlock()
	if siguiendo {
		if d := dialogoArriba(c); d != nil && alto > 0 {
			d.ponerTeclado(c, alto)
		}
		return
	}
	aplicar := func(alto float32) {
		tecladoMu.Lock()
		tecladoAlto[c] = alto
		tecladoMu.Unlock()
		fyne.Do(func() {
			if d := dialogoArriba(c); d != nil {
				d.ponerTeclado(c, alto)
			}
		})
	}
	go func() {
		defer func() {
			tecladoMu.Lock()
			delete(tecladoAlto, c)
			tecladoMu.Unlock()
		}()
		antes := float32(-1)
		for range 20 {
			time.Sleep(100 * time.Millisecond)
			alto := medir(c)
			if alto > 0 && alto == antes {
				aplicar(alto)
				break
			}
			antes = alto
		}
		if antes <= 0 {
			return // no ha salido: un campo de solo lectura, o un teclado físico
		}
		// Mientras siga fuera, atento a que cambie de alto (la fila de sugerencias, otro
		// idioma) y, al irse, la hoja vuelve abajo.
		ultimo := antes
		for {
			time.Sleep(300 * time.Millisecond)
			alto := medir(c)
			if alto <= 0 {
				aplicar(0)
				return
			}
			if alto != ultimo {
				ultimo = alto
				aplicar(alto)
			}
		}
	}()
}
