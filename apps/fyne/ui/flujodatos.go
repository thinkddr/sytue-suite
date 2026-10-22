// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package ui

import (
	"image/color"
	"strings"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"

	"git.sytue.eu/sytue/one/apps/fyne/iconos"
	"git.sytue.eu/sytue/one/apps/fyne/tema"
	"git.sytue.eu/sytue/one/apps/nucleo/i18n"
)

// # «Qué pasa con los datos»
//
// `DataFlow` de @sytue/ui: al conectar Sytue One con un tercero se dice ANTES de conceder
// nada si los datos solo entran, salen o van en los dos sentidos, qué tipo de datos en
// cada dirección y qué política los cubre. La dirección NO se pasa: se deduce de las
// listas, así que nadie puede decir «solo entra» y enumerar algo que sale.
//
// Las dos formas de la web: la COMPLETA (`border bg-bg-subtle rounded-lg p-4`, con el
// círculo de 40, título, cuerpo, las píldoras de cada dirección y la política), y la
// COMPACTA de una línea para los bloques de un clic, como «Traer mis datos».
//
// Se construye AL ANCHO que se le da, como `ParrafoRico`: quien lo usa repinta al cambiar.
//
// ponytail: el Hub tiene su propia versión compacta de solo `both` (`apps/hub`,
// `flujoDatos`); se puede cambiar por esta cuando su agente toque esa pantalla.

// ModoFlujo es la dirección deducida de las dos listas: `in`, `out`, `both` o `none`.
func ModoFlujo(entra, sale []string) string {
	switch {
	case len(entra) > 0 && len(sale) > 0:
		return "both"
	case len(entra) > 0:
		return "in"
	case len(sale) > 0:
		return "out"
	}
	return "none"
}

// iconoFlujo y tonoFlujo son el `ICONO` y el `TONO` de la web: lo que entra se queda bajo
// nuestra política (verde); lo que sale ya no (ámbar).
var (
	iconoFlujo = map[string]iconos.Icono{"in": iconos.Descargar, "out": iconos.Subir, "both": iconos.Intercambiar, "none": iconos.Enlace}
	tonoFlujo  = map[string][2]string{
		"in": {"success-bg", "success-text"}, "out": {"warning-bg", "warning-text"},
		"both": {"info-bg", "info-text"}, "none": {"surface", "text-subtle"},
	}
)

// Medidas del `DataFlow`.
const (
	LadoCirculoFlujoCompacto float32 = 5 * 0.25 * tema.Rem  // `h-5 w-5`
	LadoCirculoFlujo         float32 = 10 * 0.25 * tema.Rem // `h-10 w-10`
	HuecoFlujoCompacto       float32 = 1.5 * 0.25 * tema.Rem
)

// PoliticaDePrivacidad es el enlace del `DataFlow`: la del sitio público, con el idioma
// delante salvo en inglés, que es el de la raíz.
func PoliticaDePrivacidad(apex string, idioma i18n.Locale) string {
	if idioma == i18n.Default {
		return "https://" + apex + "/privacy"
	}
	return "https://" + apex + "/" + string(idioma) + "/privacy"
}

// FlujoDatos compone el `DataFlow` a `ancho`. `socio` es la MARCA del otro lado
// («Google Drive», «S3»), que la forma completa exige porque se declina dentro de la
// frase; la compacta no la nombra. `alPolitica` abre la política; nil, sin enlace.
func FlujoDatos(tr *i18n.Translator, socio string, entra, sale []string, compacto bool, alPolitica func(), ancho float32) fyne.CanvasObject {
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	modo := ModoFlujo(entra, sale)
	vars := i18n.Vars{"partner": socio}
	enlace := NuevoEnlace(tr.T("dataflow.policy.link", nil), tema.TextoXS, SemiNegrita, "primary-text", alPolitica)
	circulo := func(lado, icono float32) []fyne.CanvasObject {
		c := canvas.NewCircle(tema.Color(tonoFlujo[modo][0], v))
		c.Resize(fyne.NewSize(lado, lado))
		d := iconos.Trazo(iconoFlujo[modo], icono, tonoFlujo[modo][1], v)
		d.Colocar((lado-icono)/2, (lado-icono)/2, icono)
		return []fyne.CanvasObject{c, d.Objeto()}
	}

	if compacto {
		// `flex items-center gap-1.5 text-xs text-muted`: el círculo, y el título en
		// `text-text font-medium` con « · » y el enlace detrás.
		clave := "dataflow." + modo + ".title"
		if modo == "out" {
			clave = "dataflow.out.short"
		}
		piezas := PalabrasFlujo(tr.T(clave, vars), tema.TextoXS, Medio, tema.Color("text", v))
		if modo != "none" {
			piezas = append(piezas, PalabrasFlujo("·", tema.TextoXS, Normal, tema.Color("text-muted", v))...)
			piezas = append(piezas, PiezaFlujo{O: enlace, Ancho: MedidaEnlace(enlace).Width, Alto: tema.TextoXS.Linea, Sep: anchoEspacio(tema.TextoXS, Normal)})
		}
		x := LadoCirculoFlujoCompacto + HuecoFlujoCompacto
		texto, alto := Fluir(piezas, ancho-x, tema.TextoXS.Linea, 0)
		alto = max(alto, LadoCirculoFlujoCompacto)
		objs := circulo(LadoCirculoFlujoCompacto, 12)
		for _, o := range objs {
			o.Move(o.Position().AddXY(0, (alto-LadoCirculoFlujoCompacto)/2))
		}
		texto.Move(fyne.NewPos(x, (alto-texto.MinSize().Height)/2))
		return container.New(tamFijo{fyne.NewSize(ancho, alto)}, append(objs, texto)...)
	}

	// La completa: `flex gap-3 rounded-lg border p-4` y dentro, `space-y-2`.
	caja := NuevaCaja(tema.Color("bg-subtle", v), tema.Color("border", v), tema.RadioLG)
	relleno := tema.Borde + tema.Espacio(4)
	x := relleno + LadoCirculoFlujo + tema.Espacio(3)
	col := max(ancho-x-relleno, 0)
	relajado := tema.Escalon{Tam: tema.TextoSM.Tam, Linea: 1.625 * tema.TextoSM.Tam}
	relajadoXS := tema.Escalon{Tam: tema.TextoXS.Tam, Linea: 1.625 * tema.TextoXS.Tam}
	var bloques []fyne.CanvasObject
	var altos []float32
	poner := func(o fyne.CanvasObject, alto float32) { bloques, altos = append(bloques, o), append(altos, alto) }

	cabeza := []*Parrafo{
		NuevoParrafo(tr.T("dataflow.label", nil), tema.TextoXS, Normal, "text-muted", v),
		NuevoParrafo(tr.T("dataflow."+modo+".title", vars), tema.TextoBase, SemiNegrita, "text", v),
		NuevoParrafo(tr.T("dataflow."+modo+".body", vars), relajado, Normal, "text-muted", v),
	}
	var altoCabeza float32
	for _, p := range cabeza {
		altoCabeza += AltoParrafo(p, col)
	}
	poner(container.New(&apilarParrafos{ps: cabeza}, cabeza[0], cabeza[1], cabeza[2]), altoCabeza)

	fila := func(icono iconos.Icono, rotulo string, cosas []string) {
		// `flex flex-wrap items-center gap-1.5`: el rótulo con su icono de 14, y las
		// píldoras `rounded-full border px-2 py-0.5 text-xs` con `gap-1`.
		piezas := make([]PiezaFlujo, 0, 1+len(cosas))
		piezas = append(piezas, PiezaFlujo{O: rotuloFlujo(icono, rotulo, v), Ancho: 14 + tema.Espacio(1) + Medir(rotulo, tema.TextoXS, Medio).Width, Alto: max(14, tema.TextoXS.Linea)})
		for i, c := range cosas {
			p := pildoraFlujo(tr.T("dataflow.item."+c, nil), v)
			sep := tema.Espacio(1)
			if i == 0 {
				sep = HuecoFlujoCompacto
			}
			piezas = append(piezas, PiezaFlujo{O: p, Ancho: p.MinSize().Width, Alto: p.MinSize().Height, Sep: sep})
		}
		o, alto := Fluir(piezas, col, AltoPildoraFlujo, HuecoFlujoCompacto)
		poner(o, alto)
	}
	if len(entra) > 0 {
		fila(iconos.Descargar, tr.T("dataflow.incoming", nil), entra)
	}
	if len(sale) > 0 {
		fila(iconos.Subir, tr.T("dataflow.outgoing", vars), sale)
	}
	if modo != "none" {
		var frase []string
		if len(entra) > 0 {
			frase = append(frase, tr.T("dataflow.policy.ours", nil))
		}
		if len(sale) > 0 {
			frase = append(frase, tr.T("dataflow.policy.theirs", vars))
		}
		piezas := PalabrasFlujo(strings.Join(frase, " "), relajadoXS, Normal, tema.Color("text-muted", v))
		piezas = append(piezas, PiezaFlujo{O: enlace, Ancho: MedidaEnlace(enlace).Width, Alto: tema.TextoXS.Linea, Sep: anchoEspacio(tema.TextoXS, Normal)})
		o, alto := Fluir(piezas, col, relajadoXS.Linea, 0)
		poner(o, alto)
	}

	objs := append([]fyne.CanvasObject{}, caja.Objetos()...)
	objs = append(objs, circulo(LadoCirculoFlujo, 20)...)
	for _, o := range objs[len(caja.Objetos()):] {
		o.Move(o.Position().AddXY(relleno, relleno))
	}
	y := relleno
	for i, b := range bloques {
		if i > 0 {
			y += tema.Espacio(2)
		}
		b.Move(fyne.NewPos(x, y))
		b.Resize(fyne.NewSize(col, altos[i]))
		y += altos[i]
		objs = append(objs, b)
	}
	alto := max(y, relleno+LadoCirculoFlujo) + relleno
	caja.Redimensionar(fyne.NewSize(ancho, alto))
	return container.New(tamFijo{fyne.NewSize(ancho, alto)}, objs...)
}

// AltoPildoraFlujo es lo que mide una píldora: `py-0.5`, la línea de `text-xs` y el borde.
var AltoPildoraFlujo = 2*0.5*0.25*tema.Rem + tema.TextoXS.Linea + 2*tema.Borde

// pildoraFlujo es una categoría de datos: `border-border bg-surface text-text
// rounded-full border px-2 py-0.5 text-xs`.
func pildoraFlujo(texto string, v tema.Variante) fyne.CanvasObject {
	t := Texto(texto, tema.TextoXS, Normal, tema.Color("text", v))
	margen := tema.Espacio(2)
	ancho := Medir(texto, tema.TextoXS, Normal).Width + 2*margen + 2*tema.Borde
	caja := NuevaCaja(tema.Color("surface", v), tema.Color("border", v), AltoPildoraFlujo/2)
	caja.Redimensionar(fyne.NewSize(ancho, AltoPildoraFlujo))
	ColocarTexto(t, tema.TextoXS, tema.Borde+margen, tema.Borde+0.5*0.25*tema.Rem)
	return container.New(tamFijo{fyne.NewSize(ancho, AltoPildoraFlujo)}, append(caja.Objetos(), t)...)
}

// rotuloFlujo es `flex items-center gap-1 text-xs font-medium text-muted` con su icono.
func rotuloFlujo(icono iconos.Icono, texto string, v tema.Variante) fyne.CanvasObject {
	alto := max(14, tema.TextoXS.Linea)
	d := iconos.Trazo(icono, 14, "text-muted", v)
	d.Colocar(0, (alto-14)/2, 14)
	t := Texto(texto, tema.TextoXS, Medio, tema.Color("text-muted", v))
	ColocarTexto(t, tema.TextoXS, 14+tema.Espacio(1), (alto-tema.TextoXS.Linea)/2)
	return container.New(tamFijo{fyne.NewSize(14+tema.Espacio(1)+Medir(texto, tema.TextoXS, Medio).Width, alto)}, d.Objeto(), t)
}

// apilarParrafos pone los párrafos uno debajo de otro al ancho del contenedor.
type apilarParrafos struct{ ps []*Parrafo }

func (a *apilarParrafos) MinSize([]fyne.CanvasObject) fyne.Size { return fyne.NewSize(0, 0) }

func (a *apilarParrafos) Layout(_ []fyne.CanvasObject, t fyne.Size) {
	y := float32(0)
	for _, p := range a.ps {
		h := AltoParrafo(p, t.Width)
		p.Move(fyne.NewPos(0, y))
		p.Resize(fyne.NewSize(t.Width, h))
		y += h
	}
}

// PiezaFlujo es un trozo de una línea que se parte como el texto en línea de CSS: una
// palabra, un enlace, una píldora. `Sep` es el hueco que lleva DELANTE si no empieza
// línea (el espacio entre palabras, el `gap` entre píldoras).
type PiezaFlujo struct {
	O             fyne.CanvasObject
	Ancho, Alto   float32
	Sep           float32
	esc           tema.Escalon
	esTextoSuelto bool
}

// PalabrasFlujo parte un texto en palabras que se pueden llevar a la línea siguiente.
func PalabrasFlujo(texto string, e tema.Escalon, p Peso, c color.NRGBA) []PiezaFlujo {
	sep := anchoEspacio(e, p)
	palabras := strings.Fields(texto)
	out := make([]PiezaFlujo, 0, len(palabras))
	for _, w := range palabras {
		out = append(out, PiezaFlujo{O: Texto(w, e, p, c), Ancho: Medir(w, e, p).Width, Alto: e.Linea, Sep: sep, esc: e, esTextoSuelto: true})
	}
	return out
}

// anchoEspacio es lo que mide un espacio en ese escalón y ese peso.
func anchoEspacio(e tema.Escalon, p Peso) float32 {
	return Medir("a a", e, p).Width - Medir("aa", e, p).Width
}

// Fluir coloca las piezas en líneas de `ancho`, cada una centrada en su línea de
// `altoLinea` con `huecoFilas` entre líneas (el `gap` vertical de un `flex-wrap`), y
// devuelve el grupo y su alto.
func Fluir(piezas []PiezaFlujo, ancho, altoLinea, huecoFilas float32) (fyne.CanvasObject, float32) {
	objs := make([]fyne.CanvasObject, 0, len(piezas))
	var x, y float32
	for i, p := range piezas {
		sep := p.Sep
		if i == 0 || x == 0 {
			sep = 0
		}
		if x > 0 && x+sep+p.Ancho > ancho {
			x, y, sep = 0, y+altoLinea+huecoFilas, 0
		}
		cy := y + (altoLinea-p.Alto)/2
		if t, es := p.O.(*canvas.Text); es && p.esTextoSuelto {
			ColocarTexto(t, p.esc, x+sep, cy)
		} else {
			p.O.Move(fyne.NewPos(x+sep, cy))
			p.O.Resize(fyne.NewSize(p.Ancho, p.Alto))
		}
		objs = append(objs, p.O)
		x += sep + p.Ancho
	}
	alto := float32(0)
	if len(piezas) > 0 {
		alto = y + altoLinea
	}
	return container.New(tamFijo{fyne.NewSize(ancho, alto)}, objs...), alto
}
