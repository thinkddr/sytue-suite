// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package notes

import (
	"regexp"
	"slices"
	"strings"
	"unicode/utf8"

	"fyne.io/fyne/v2"

	"git.sytue.eu/sytue/one/apps/fyne/tema"
	"git.sytue.eu/sytue/one/apps/nucleo/buscar"
)

// LOS EMOJI DE UNA NOTA: los del selector de símbolos (su apartado «Emoji») y los de «:»
// al escribir —«:son» ofrece los emoji cuyas palabras empiezan así, como en Confluence o
// en Slack—. Es la lista de `client/simbolos.ts` de la web, en el mismo orden y con las
// mismas palabras, en castellano y en inglés.
//
// Se pintan con la cara de color portable de la suite (`tema.Emoji`, la de Rooms): la
// letra de reserva del sistema no dibuja los de macOS ni los de Windows (sbix y COLR),
// y un emoji que en un equipo es un dibujo y en otro una caja no se puede ofrecer. La
// cara se carga la primera vez que hay un emoji que pintar.

// simboloEmoji es un emoji con las palabras con que se encuentra.
type simboloEmoji struct{ s, claves string }

// Emoji son los de la web (`CATEGORIAS`, id «emoji»), en su orden.
var Emoji = []simboloEmoji{
	{"😀", "sonrisa feliz grin smile happy"},
	{"😂", "risa lagrimas joy laugh"},
	{"🙂", "sonrisa leve slight smile"},
	{"😉", "guino wink"},
	{"😍", "enamorado corazones heart eyes love"},
	{"🤔", "pensando thinking"},
	{"😅", "sudor alivio sweat smile"},
	{"😎", "gafas sol cool sunglasses"},
	{"😢", "triste llorar cry sad"},
	{"😡", "enfadado angry rage"},
	{"😴", "dormir sueno sleep"},
	{"🤯", "explota cabeza mind blown"},
	{"🥳", "fiesta celebrar party"},
	{"🙏", "gracias por favor pray thanks please"},
	{"👍", "bien pulgar arriba ok thumbs up like"},
	{"👎", "mal pulgar abajo thumbs down dislike"},
	{"👏", "aplauso clap"},
	{"🙌", "manos arriba celebrar raised hands"},
	{"👋", "hola adios saludo wave hello"},
	{"💪", "fuerza musculo strong muscle"},
	{"👀", "ojos mirar eyes look"},
	{"🤝", "acuerdo apreton handshake deal"},
	{"✅", "hecho completado check done"},
	{"❌", "no cruz error cross"},
	{"❓", "pregunta duda question"},
	{"❗", "importante exclamacion exclamation"},
	{"⭐", "estrella star"},
	{"🔥", "fuego fire hot"},
	{"✨", "brillos chispas sparkles"},
	{"💡", "idea bombilla bulb"},
	{"🎉", "fiesta confeti tada party"},
	{"🎯", "objetivo diana target goal"},
	{"🚀", "cohete lanzamiento rocket launch"},
	{"📌", "chincheta fijar pin"},
	{"📎", "clip adjunto paperclip attach"},
	{"📝", "nota apunte memo note"},
	{"📅", "calendario fecha calendar date"},
	{"📈", "grafico sube chart up"},
	{"📉", "grafico baja chart down"},
	{"📊", "grafico barras bar chart"},
	{"📚", "libros books"},
	{"📞", "telefono llamada phone call"},
	{"📧", "correo email mail"},
	{"🔒", "candado cerrado lock"},
	{"🔑", "llave key"},
	{"🔗", "enlace link"},
	{"🔍", "buscar lupa search"},
	{"⏰", "alarma despertador alarm clock"},
	{"⏳", "reloj arena espera hourglass wait"},
	{"🕒", "reloj hora clock time"},
	{"💬", "comentario mensaje comment speech"},
	{"💭", "pensamiento thought"},
	{"⚡", "rayo rapido lightning fast"},
	{"🐛", "bicho fallo bug"},
	{"🛠", "herramientas tools"},
	{"⚙", "ajustes engranaje settings gear"},
	{"💻", "portatil ordenador laptop computer"},
	{"📱", "movil telefono phone mobile"},
	{"🏠", "casa hogar home house"},
	{"🏢", "oficina empresa office building"},
	{"✈", "avion viaje plane travel"},
	{"🚗", "coche car"},
	{"☕", "cafe coffee"},
	{"🍕", "pizza"},
	{"🎂", "tarta cumpleanos cake birthday"},
	{"🎁", "regalo gift present"},
	{"❤", "corazon amor heart love"},
	{"💔", "corazon roto broken heart"},
	{"☀", "sol sun"},
	{"🌧", "lluvia rain"},
	{"❄", "nieve frio snow cold"},
	{"🌱", "planta brote plant sprout"},
	{"🌍", "mundo tierra world earth"},
	{"🇪🇺", "europa union europea eu europe flag"},
	{"🏁", "meta bandera fin finish flag"},
	{"🚧", "obras en construccion construction wip"},
	{"🛑", "parar stop"},
	{"🟢", "verde circulo green circle"},
	{"🟡", "amarillo circulo yellow circle"},
	{"🔴", "rojo circulo red circle"},
	{"🔵", "azul circulo blue circle"},
	{"➕", "mas sumar plus add"},
	{"➖", "menos restar minus"},
	{"💯", "cien perfecto hundred"},
	{"🤖", "robot bot"},
	{"🧠", "cerebro brain"},
	{"🎵", "musica nota music"},
	{"📷", "camara foto camera photo"},
	{"🎨", "arte paleta art palette"},
	{"🏆", "trofeo premio trophy award"},
	{"🤷", "no se encogerse shrug"},
	{"🙈", "no mirar mono see no evil"},
}

// maxSugerenciasEmoji son los que ofrece «:»: ocho, que es lo que se lee de un vistazo.
const maxSugerenciasEmoji = 8

// reEmoji es `consultaDeEmoji` de la web: «:» al principio o tras un blanco y de dos a
// veinte letras o cifras. «a las 10:30» o una URL no abren nada.
var reEmoji = regexp.MustCompile(`(?:^|\s):([\p{L}\p{N}_]{2,20})$`)

// consultaDeEmoji es, si el texto antes del cursor acaba en `:palabra`, la palabra.
func consultaDeEmoji(antes string) (string, bool) {
	m := reEmoji.FindStringSubmatch(strings.ReplaceAll(antes, " ", " "))
	if len(m) < 2 {
		return "", false
	}
	return m[1], true
}

// buscarEmoji son los emoji cuyas palabras empiezan por cada término de lo buscado, sin
// acentos ni mayúsculas (`buscarSimbolos` de la web), hasta ocho.
func buscarEmoji(q string) []simboloEmoji {
	terminos := strings.Fields(buscar.Plegar(q))
	if len(terminos) == 0 {
		return nil
	}
	var out []simboloEmoji
	for _, x := range Emoji {
		palabras := strings.Fields(buscar.Plegar(x.claves))
		todas := x.s == strings.TrimSpace(q)
		if !todas {
			todas = true
			for _, t := range terminos {
				todas = todas && slices.ContainsFunc(palabras, func(p string) bool { return strings.HasPrefix(p, t) })
			}
		}
		if todas {
			out = append(out, x)
			if len(out) == maxSugerenciasEmoji {
				break
			}
		}
	}
	return out
}

// ponerEmoji cambia «:son» por el emoji elegido, en un paso de deshacer, y lo apunta en
// los recientes del selector de símbolos.
func (e *EditorNota) ponerEmoji(s string) {
	e.tomarDisparo(func() {
		e.cur = e.doc.Insertar(e.cur, s, estiloEn(e.bloqueActual(), e.cur.O))
		e.ancla = e.cur
	})
	recordarSimbolo(s)
}

// emojiBMP son los emoji de la lista que viven por debajo de U+1F000: sin ellos en la
// cara de color, «✅» o «☕» saldrían con la letra del sistema.
var emojiBMP = func() map[rune]bool {
	m := map[rune]bool{}
	for _, x := range Emoji {
		if r, _ := utf8.DecodeRuneInString(x.s); r < 0x1F000 {
			m[r] = true
		}
	}
	return m
}()

// esEmoji dice si una letra abre un emoji: los pictogramas de U+1F000 en adelante (caras,
// objetos, banderas) y los de la lista que están más abajo. Las marcas del selector de
// símbolos (✓, ★, ⚠) NO: son tipografía y se ven en la letra del texto.
func esEmoji(r rune) bool { return r >= 0x1F000 && r <= 0x1FAFF || emojiBMP[r] }

// pegaAlEmoji dice si una letra va con el emoji de delante: el selector de presentación
// (U+FE0F), el que une dos (U+200D), los tonos de piel, la tecla y las etiquetas de
// una bandera regional.
func pegaAlEmoji(r rune) bool {
	return r == 0xFE0F || r == 0x200D || r == 0x20E3 || r >= 0x1F3FB && r <= 0x1F3FF || r >= 0xE0020 && r <= 0xE007F
}

// trozosEmoji parte un texto en tramos alternos de texto y de emoji (el emoji con lo que
// se le pega, y lo que se une con U+200D), sin perder nada. Sin emoji, el texto entero.
func trozosEmoji(s string) (trozos []string, emoji []bool) {
	if !strings.ContainsFunc(s, esEmoji) {
		return []string{s}, []bool{false}
	}
	inicio, enEmoji, unir := 0, false, false
	for i, r := range s {
		es := esEmoji(r) || enEmoji && (pegaAlEmoji(r) || unir)
		if i > 0 && es != enEmoji {
			trozos, emoji = append(trozos, s[inicio:i]), append(emoji, enEmoji)
			inicio = i
		}
		enEmoji, unir = es, r == 0x200D
	}
	return append(trozos, s[inicio:]), append(emoji, enEmoji)
}

// caraEmoji es la cara de color de los emoji (la de Rooms).
func caraEmoji() fyne.Resource { return tema.Emoji() }
