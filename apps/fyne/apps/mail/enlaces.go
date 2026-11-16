// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package mail

import (
	"image/color"
	"strings"
	"unicode"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/driver/desktop"

	"git.sytue.eu/sytue/one/apps/fyne/tema"
	"git.sytue.eu/sytue/one/apps/fyne/ui"
	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/htmltext"
)

// Los enlaces del cuerpo de un mensaje: `TextoEnlazado` de `TextoConCita.tsx`.
//
// El cuerpo se pinta en líneas de `canvas.Text` (ver `TarjetaMensaje`), así que un
// enlace es un TRAMO de una línea con la tinta `primary-text` y subrayado, y la
// tarjeta sabe dónde cae cada uno para abrirlo al pulsar. Las direcciones salen de
// `htmltext.Enlaces`, que recorre los mismos vectores que `enlazarTexto` de la web.
//
// Es una superficie de phishing, así que: solo se abre con un gesto (pulsar), solo
// `http(s)` —por `api.AbrirFuera`, la última puerta antes del sistema— y `mailto`,
// que se escribe desde Sytue; y lo que se ve es la dirección real: en un correo que
// solo trae HTML, `htmltext.TextoConEnlaces` deja el destino de cada enlace escrito
// junto a su texto, que es lo que hace de pista. Un extracto no se enlaza: está
// cortado, y una dirección a medias abriría otro sitio.

// trozoPintado es dónde cae un tramo del cuerpo: su línea, su x desde el margen y su
// ancho. Destino vacío, es texto.
type trozoPintado struct {
	linea    int
	x, ancho float32
	destino  string
}

// zonaEnlace es la diana de un enlace en coordenadas de la tarjeta.
type zonaEnlace struct {
	pos, tam fyne.Position
	destino  string
}

func (z zonaEnlace) contiene(p fyne.Position) bool {
	return p.X >= z.pos.X && p.X < z.pos.X+z.tam.X && p.Y >= z.pos.Y && p.Y < z.pos.Y+z.tam.Y
}

// tramosDeLineas reparte los enlaces de `texto` en las líneas que salieron de
// `repartirPre(texto, …)`. El reparto junta blancos y parte palabras largas, pero no
// cambia ni el orden ni las letras que no son blanco, y un enlace nunca lleva blancos:
// basta con contar esas letras para saber a qué enlace pertenece cada una. Así una
// dirección partida en dos líneas abre ENTERA desde cualquiera de sus dos trozos.
func tramosDeLineas(texto string, lineas []string, enlazar bool) [][]htmltext.Tramo {
	out := make([][]htmltext.Tramo, len(lineas))
	var destinos []string
	if enlazar {
		destinos = destinosPorLetra(texto)
	}
	k := 0
	for i, l := range lineas {
		var tramos []htmltext.Tramo
		var b strings.Builder
		actual := ""
		cerrar := func() {
			if b.Len() > 0 {
				tramos = append(tramos, htmltext.Tramo{Texto: b.String(), Destino: actual})
				b.Reset()
			}
		}
		for _, c := range l {
			d := ""
			if !unicode.IsSpace(c) {
				if k < len(destinos) {
					d = destinos[k]
				}
				k++
			}
			if d != actual {
				cerrar()
				actual = d
			}
			b.WriteRune(c)
		}
		cerrar()
		if len(tramos) == 0 {
			tramos = []htmltext.Tramo{{}}
		}
		out[i] = tramos
	}
	return out
}

// destinosPorLetra es el destino de cada letra que no es blanco de `texto`, en orden;
// vacío, la letra es texto.
func destinosPorLetra(texto string) []string {
	var destinos []string
	tramos := htmltext.Enlaces(texto)
	// Un cuerpo demasiado largo llega cortado con «…» (`recortar`), y el «…» no
	// termina una dirección: la última, pegada al corte, sería una a medias que
	// abriría otro sitio, como en el extracto. Esa se queda en texto.
	if n := len(tramos); n > 0 && tramos[n-1].Destino != "" && strings.HasSuffix(tramos[n-1].Texto, "…") {
		tramos[n-1].Destino = ""
	}
	for _, tr := range tramos {
		for _, c := range tr.Texto {
			if !unicode.IsSpace(c) {
				destinos = append(destinos, tr.Destino)
			}
		}
	}
	return destinos
}

// pintarTramos ajusta los textos a los tramos de las líneas, con la tinta del cuerpo
// y, en los enlaces, la de `primary-text` subrayada (`underline underline-offset-2`).
// Dice dónde cae cada tramo y si ha cambiado el número de textos.
func pintarTramos(l []*canvas.Text, lineas [][]htmltext.Tramo, tinta color.NRGBA, v tema.Variante) ([]*canvas.Text, []trozoPintado, bool) {
	var trozos []trozoPintado
	n := 0
	for _, tr := range lineas {
		n += len(tr)
	}
	l, cambio := textos(l, n, tema.TextoBase, tinta)
	i := 0
	for li, tr := range lineas {
		x := float32(0)
		for j, tramo := range tr {
			t := l[i]
			t.Text, t.Color, t.TextStyle = tramo.Texto, tinta, fyne.TextStyle{}
			ancho := float32(0)
			if len(tr) > 1 {
				// Solo se mide lo que hace falta: una línea de un solo tramo va en x = 0.
				ancho = ui.Medir(tramo.Texto, tema.TextoBase, ui.Normal).Width
				if j > 0 {
					x = ui.Medir(textoHasta(tr, j), tema.TextoBase, ui.Normal).Width
				}
			}
			if tramo.Destino != "" {
				t.Color, t.TextStyle = tema.Color("primary-text", v), fyne.TextStyle{Underline: true}
				if ancho == 0 {
					ancho = ui.Medir(tramo.Texto, tema.TextoBase, ui.Normal).Width
				}
			}
			trozos = append(trozos, trozoPintado{linea: li, x: x, ancho: ancho, destino: tramo.Destino})
			i++
		}
	}
	return l, trozos, cambio
}

// textoHasta es la línea hasta el tramo j, sin él: lo que mide su x.
func textoHasta(tr []htmltext.Tramo, j int) string {
	var b strings.Builder
	for _, t := range tr[:j] {
		b.WriteString(t.Texto)
	}
	return b.String()
}

// colocarTramos pone los textos de los tramos a partir de (x, y), apunta las dianas de
// los enlaces y devuelve dónde acaba la última de `lineas` líneas.
func colocarTramos(l []*canvas.Text, trozos []trozoPintado, lineas int, x, y float32, zonas []zonaEnlace) (float32, []zonaEnlace) {
	e := tema.Escalon{Tam: tema.TextoBase.Tam, Linea: LineaCuerpo}
	for i, tp := range trozos {
		if i >= len(l) {
			break
		}
		ty := y + float32(tp.linea)*LineaCuerpo
		ui.ColocarTexto(l[i], e, x+tp.x, ty)
		if tp.destino != "" {
			zonas = append(zonas, zonaEnlace{pos: fyne.NewPos(x+tp.x, ty), tam: fyne.NewPos(tp.ancho, LineaCuerpo), destino: tp.destino})
		}
	}
	return y + float32(lineas)*LineaCuerpo, zonas
}

// enlaceEn es el destino del enlace bajo `p`, o vacío.
func (t *TarjetaMensaje) enlaceEn(p fyne.Position) string {
	if !t.Expandida {
		return ""
	}
	for _, z := range t.zonas {
		if z.contiene(p) {
			return z.destino
		}
	}
	return ""
}

// Cursor es la mano sobre un enlace del cuerpo, como el `cursor: pointer` de un `<a>`.
func (t *TarjetaMensaje) Cursor() desktop.Cursor {
	if t.sobreEnlace {
		return desktop.PointerCursor
	}
	return desktop.DefaultCursor
}

// leido dice si lo que enseña el cuerpo es el mensaje leído y no el extracto o un aviso:
// solo eso se enlaza.
func (t *TarjetaMensaje) leido() bool {
	c, ya := t.L.cuerpoDe(t.Mensaje.ID)
	return ya && c.Texto != ""
}

// abrirEnlace es lo que hace pulsar un enlace del cuerpo: una web va al navegador del
// sistema por `api.AbrirFuera`, que no deja pasar más que http(s) con su máquina, y un
// `mailto:` abre el redactor de Sytue con esa dirección en «Para», como «Redactar».
func (b *Buzon) abrirEnlace(destino string) {
	if len(destino) > 7 && strings.EqualFold(destino[:7], "mailto:") {
		b.abrirRedactor(Borrador{De: b.deCompartido(), Para: destino[7:], Cuerpo: ConFirma("", b.firma, false)})
		return
	}
	if b.Abridor != nil {
		_ = api.AbrirFuera(b.Abridor, destino)
		return
	}
	_ = api.AbrirFuera(fyne.CurrentApp(), destino)
}
