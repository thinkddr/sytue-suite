// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package rooms

import (
	"fyne.io/fyne/v2"

	"git.sytue.eu/sytue/one/apps/fyne/tema"
	"git.sytue.eu/sytue/one/apps/fyne/ui"
)

// Los dos esqueletos de Rooms: el del carril mientras llegan los canales y el del
// historial mientras llegan los mensajes. Los dos son `ui.FilasEsqueleto`, con un solo
// latido y un solo anuncio (el `aria-busy` con su `sr-only` de la web); lo que es de
// Rooms es CUÁNTAS filas hay y con qué geometría, que es la de la fila real a la que
// sustituyen —sin eso el layout salta al llegar los datos—.

// Medidas del esqueleto del carril (`ChatApp.tsx`): tres `h-9 rounded-md
// bg-sidebar-hover` en una columna `gap-1 px-0.5`. HOJA.
var (
	AltoEsqueletoCanal   = tema.Espacio(9)
	HuecoEsqueletoCanal  = tema.Espacio(1)
	MargenEsqueletoCanal = tema.Espacio(0.5)
	// CuantosEsqueletoCanal son los tres de `[0, 1, 2].map`.
	CuantosEsqueletoCanal = 3
)

// Medidas del esqueleto del historial (`MessageView.tsx`): `px-4 py-3` y cinco filas
// `mt-5 flex gap-3`, cada una con un cuadro de `2rem` y una columna `gap-2 pt-1` con
// dos barras de `0.75rem`: la del nombre de `7rem` y la del texto al 70, 48, 62, 40 y
// 55 % del ancho. HOJA.
var (
	ArribaEsqueletoMensajes          = tema.Espacio(3)
	AireEsqueletoMensaje             = tema.Espacio(5)
	LadoEsqueletoAvatar      float32 = 2 * tema.Rem
	HuecoEsqueletoMensaje            = tema.Espacio(3)
	ArribaEsqueletoTexto             = tema.Espacio(1)
	HuecoEsqueletoTexto              = tema.Espacio(2)
	AltoEsqueletoTexto       float32 = 0.75 * tema.Rem
	AnchoEsqueletoNombre     float32 = 7 * tema.Rem
	PartesEsqueletoMensajes          = []float32{0.70, 0.48, 0.62, 0.40, 0.55}
	altoFilaEsqueletoMensaje         = max(LadoEsqueletoAvatar,
		ArribaEsqueletoTexto+2*AltoEsqueletoTexto+HuecoEsqueletoTexto)
)

// NuevoEsqueletoCanales son los tres bloques del carril: filas sin delantero de una
// sola barra a todo el ancho, teñidas para el cromo oscuro.
func NuevoEsqueletoCanales(nombre string) *ui.FilasEsqueleto {
	return ui.NuevasFilasEsqueleto(CuantosEsqueletoCanal, ui.SinDelantero, 1).ConReceta(ui.RecetaEsqueleto{
		Alto:  AltoEsqueletoCanal,
		Hueco: HuecoEsqueletoCanal,
		Izq:   MargenEsqueletoCanal,
		Der:   MargenEsqueletoCanal,
		Rayas: []ui.RayaEsqueleto{{Alto: AltoEsqueletoCanal}},
	}).ConRotulo(nombre).SobreCromo()
}

// MedidaEsqueletoCanales es lo que ocupa de alto.
func MedidaEsqueletoCanales() fyne.Size { return NuevoEsqueletoCanales("").MinSize() }

// RecetaMensajes es la geometría de las filas del historial: el cuadro, y a su derecha
// (`flex-1`) la barra del nombre, de ancho fijo, y la del texto, que es una parte de esa
// columna. Arriba es el aire de encima de todo, Aire el de antes de cada fila y Canalon
// el de los lados; SinAireArriba quita el Aire de la PRIMERA fila, que es lo que hace un
// `gap` frente a un `mt`.
type RecetaMensajes struct {
	Partes                []float32
	AnchoNombre           float32
	Arriba, Aire, Canalon float32
	SinAireArriba         bool
}

// Filas monta la receta como `ui.FilasEsqueleto`, una fila por parte.
func (m RecetaMensajes) Filas(nombre string) *ui.FilasEsqueleto {
	encima := m.Arriba
	if !m.SinAireArriba {
		encima += m.Aire
	}
	return ui.NuevasFilasEsqueleto(len(m.Partes), ui.DelanteroTesela, 2).ConReceta(ui.RecetaEsqueleto{
		Alto:           altoFilaEsqueletoMensaje,
		Hueco:          m.Aire,
		Encima:         encima,
		Izq:            m.Canalon,
		Der:            m.Canalon,
		Delantero:      fyne.NewSize(LadoEsqueletoAvatar, LadoEsqueletoAvatar),
		HuecoDelantero: HuecoEsqueletoMensaje,
		Rayas: []ui.RayaEsqueleto{
			{Alto: AltoEsqueletoTexto, Ancho: m.AnchoNombre},
			{Alto: AltoEsqueletoTexto, Partes: m.Partes},
		},
		HuecoRayas: HuecoEsqueletoTexto,
		Arriba:     true,
		BajaRayas:  ArribaEsqueletoTexto,
	}).ConRotulo(nombre)
}

// recetaHistorial son las cinco filas del historial.
func recetaHistorial() RecetaMensajes {
	return RecetaMensajes{
		Partes: PartesEsqueletoMensajes, AnchoNombre: AnchoEsqueletoNombre,
		Arriba: ArribaEsqueletoMensajes, Aire: AireEsqueletoMensaje,
	}
}

// NuevoEsqueletoMensajes son las cinco filas del historial. El hilo usa la misma
// receta con otras filas y otros aires (`NuevoEsqueletoHilo`).
func NuevoEsqueletoMensajes(nombre string) *ui.FilasEsqueleto {
	return recetaHistorial().Filas(nombre)
}

// MedidaEsqueletoMensajes es lo que ocupa de alto el del historial, sin el `py-3` de
// abajo, que no empuja nada.
func MedidaEsqueletoMensajes() fyne.Size { return NuevoEsqueletoMensajes("").MinSize() }
