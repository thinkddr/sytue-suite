// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package photos

import (
	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"

	"git.sytue.eu/sytue/one/apps/fyne/tema"
	"git.sytue.eu/sytue/one/apps/fyne/ui"
)

// LOS ESQUELETOS DE FOTOS: `Esqueletos.tsx`. Lo que se ve mientras llega la primera
// página de una pantalla, con la geometría de lo que va a llegar (design §3.18).
//
// Antes era la rueda suelta de CuerpoDeEstado, 18 px arriba a la izquierda: la
// pantalla estaba en blanco y al llegar las fotos todo bajaba de golpe. Un esqueleto
// con LA MISMA MAQUETA —el layout Filas con su alto de fila, o la tarjeta 4:3 de
// tarjetaAlbumLayout— reserva el sitio y la página no salta.
//
// La rueda se queda para lo que sí es «un poco más»: la página siguiente de la
// galería, en el centinela del final, cuando ya hay fotos encima.

// ProporcionesEsqueleto son las de `PROPORCIONES_ESQUELETO` de la web: las de una
// biblioteca corriente, variadas a propósito. Con todas iguales el esqueleto era una
// cuadrícula, y al llegar las fotos las filas se recomponían de otra forma.
var ProporcionesEsqueleto = []float32{
	1.3333, 0.75, 1.5, 1, 1.7778, 0.5625, 1.3333, 1.5, 0.75, 1.3333, 1, 1.5, 0.75, 1.7778, 1.3333,
	0.6667, 1.5, 1.3333,
}

// Cuántas piezas pinta cada esqueleto: las de la web.
const (
	TeselasEsqueleto         = 12 // `<EsqueletoFilas />`
	TeselasEsqueletoGaleria  = 18 // `<EsqueletoFilas mes cuantas={18} />`
	TarjetasEsqueleto        = 8  // `<EsqueletoAlbumes />`
	GruposEsqueletoRepetidas = 2  // `<EsqueletoDuplicadas />`
)

// AnchoMesEsqueleto es el `<Skeleton width="8rem">` de la cabecera de mes.
const AnchoMesEsqueleto = 8 * tema.Rem

// nuevaCarga es el `<div aria-busy="true">` con su único `role="status"` «Cargando»: un
// `ui.BloquesEsqueleto`, así que las teselas no dicen nada, el aviso es uno y no veinte
// y todas laten A UNA. Mide lo que mida su contenido al ancho que le den (AltoPara).
//
// Hasta el 2026-11-16 era un widget propio, `Ocupado`, con un `ui.Esqueleto` por
// tesela: el aviso ya era uno, pero cada tesela latía a su aire.
func nuevaCarga(v Vista) *ui.BloquesEsqueleto {
	return ui.NuevosBloquesEsqueleto().ConRotulo(v.Traduce("ui.loading"))
}

// teselaEsqueleto es un bloque gris del grupo con el `rounded-sm` de la miniatura.
func teselaEsqueleto(b *ui.BloquesEsqueleto) fyne.CanvasObject {
	r := b.Bloque(0, 0)
	r.CornerRadius = tema.RadioSM
	return r
}

// filasEsqueleto son `cuantas` teselas en el layout de la galería, con sus proporciones.
func filasEsqueleto(v Vista, b *ui.BloquesEsqueleto, cuantas int) *fyne.Container {
	r := make([]float32, cuantas)
	hijos := make([]fyne.CanvasObject, cuantas)
	for i := range r {
		r[i] = ProporcionesEsqueleto[i%len(ProporcionesEsqueleto)]
		hijos[i] = teselaEsqueleto(b)
	}
	return container.New(Filas{Razones: r, Ventana: v.Ventana}, hijos...)
}

// EsqueletoFilas es `<EsqueletoFilas>`: el de un álbum, la papelera, «parecidas», «por
// aquí» y un álbum compartido o público. Con `mes`, el de la galería: encima, la
// cabecera de un mes con la caja de cabeceraMes (`py-2` y una línea de `text-sm`).
func EsqueletoFilas(v Vista, cuantas int, mes bool) *ui.BloquesEsqueleto {
	b := nuevaCarga(v)
	filas := filasEsqueleto(v, b, cuantas)
	if !mes {
		return b.Con(filas)
	}
	cab := container.New(ui.Margen{Arr: tema.Espacio(2), Abj: tema.Espacio(2)}, b.BloqueFijo(AnchoMesEsqueleto, tema.TextoSM.Linea))
	return b.Con(ui.PilaBloques(ui.Bloque{Obj: cab}, ui.Bloque{Obj: filas}))
}

// EsqueletoAlbumes es `<EsqueletoAlbumes>`: tarjetas con la portada 4:3, el título y la
// cuenta de tarjetaAlbumLayout, en la rejilla de «Álbumes» (`gap-6`) o en la de
// «Compartidos conmigo» (la de serie, `gap-3`).
func EsqueletoAlbumes(v Vista, cuantas int, hueco float32) *ui.BloquesEsqueleto {
	b := nuevaCarga(v)
	tarjetas := make([]fyne.CanvasObject, cuantas)
	for i := range tarjetas {
		portada := teselaEsqueleto(b)
		// Las líneas, centradas en la caja de su texto como el `flex items-center` de
		// la web: un bloque de 1rem en la línea de 20 del título y uno de 0,75rem en la
		// de 16 de la cuenta.
		titulo := b.BloqueFraccion(0.6, 0)
		cuenta := b.BloqueFraccion(0.35, 0)
		margenT := (tema.TextoSM.Linea - tema.Rem) / 2
		margenC := (tema.TextoXS.Linea - 0.75*tema.Rem) / 2
		tarjetas[i] = container.New(tarjetaAlbumLayout{}, portada,
			container.New(ui.Margen{Arr: margenT, Abj: margenT}, titulo),
			container.New(ui.Margen{Arr: margenC, Abj: margenC}, cuenta))
	}
	r := ui.RejillaAlbumes()
	if hueco > 0 {
		r.Hueco = hueco
	}
	return b.Con(container.New(r, tarjetas...))
}

// EsqueletoRepetidas es `<EsqueletoDuplicadas>`: grupos con su borde y miniaturas de
// 160 con el botón debajo, como grupoRepetido.
func EsqueletoRepetidas(v Vista, grupos int) *ui.BloquesEsqueleto {
	b := nuevaCarga(v)
	bloques := make([]ui.Bloque, grupos)
	for g := range bloques {
		n := 2
		if g == 0 {
			n = 3
		}
		hijos := make([]fyne.CanvasObject, n)
		for i := range hijos {
			hijos[i] = container.New(columnaRepetida{}, teselaEsqueleto(b), b.Bloque(ladoRepetida, 2*tema.Rem))
		}
		fila := container.New(ui.Fila{Hueco: tema.Espacio(3)}, hijos...)
		bloques[g] = ui.Bloque{
			Obj: recuadro(v, "", "border", tema.RadioMD,
				ui.Margen{Izq: tema.Espacio(3), Der: tema.Espacio(3), Arr: tema.Espacio(3), Abj: tema.Espacio(3)}, fila),
			Margen: tema.Espacio(6),
		}
	}
	return b.Con(ui.PilaBloques(bloques...))
}

// CuerpoConEsqueleto es CuerpoDeEstado con el esqueleto de la pantalla en la PRIMERA
// carga (Cargando solo llega sin datos: vistaDe). Los demás estados, los de siempre.
func CuerpoConEsqueleto(v Vista, vacio *ui.Vacio, esqueleto func() *ui.BloquesEsqueleto) fyne.CanvasObject {
	if v.Estado == Cargando && esqueleto != nil {
		return esqueleto()
	}
	return CuerpoDeEstado(v, vacio)
}

// esqueletoDeFilas es el `<EsqueletoFilas />` de serie, para CuerpoConEsqueleto.
func esqueletoDeFilas(v Vista) func() *ui.BloquesEsqueleto {
	return func() *ui.BloquesEsqueleto { return EsqueletoFilas(v, TeselasEsqueleto, false) }
}
