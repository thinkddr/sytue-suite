// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package iconos

import (
	"encoding/xml"
	"math"
	"regexp"
	"strconv"
	"strings"
)

// Qué se rellena de un glifo: el espejo de `subtrazosCerrados` y de `contorno` de
// `apps/frontend/packages/ui/src/components/Glifo.tsx`. Es lógica y no dibujo: aquí
// no se reescribe ningún trazo, se ELIGE qué parte del que ya hay lleva la capa de
// acento. `TestLoQueSeRellenaEsLoDeLaWeb` pasa los mismos casos que el test de allí.

// inicioSub es el `M x y` (o `m dx dy`) con el que empieza cada subtrazo.
var inicioSub = regexp.MustCompile(`^([Mm])\s*(` + numeroSub + `)[\s,]*(` + numeroSub + `)[\s,]*`)

const numeroSub = `[-+]?(?:\d*\.\d+|\d+)`

// n3 escribe un número con tres decimales como mucho y sin ceros de cola.
func n3(v float64) string {
	return strconv.FormatFloat(math.Round(v*1000)/1000, 'f', -1, 64)
}

// subtrazosCerrados deja de un `d` solo los subtrazos que acaban en `Z`, unidos en un
// trazado: es lo que se rellena. Los abiertos no, porque SVG los cerraría por su
// cuenta y un chevrón saldría con un triángulo dentro.
//
// Al sacar un subtrazo de su sitio, una `m` RELATIVA deja de valer: su origen es donde
// quedó la pluma. Tras un `Z` eso se sabe sin interpretar nada —vuelve al inicio del
// subtrazo que cierra—, así que se reescribe como `M` absoluta.
//
// ponytail: tras un subtrazo ABIERTO la pluma está donde acabó su último tramo, y
// saberlo pide interpretar el trazado entero. Un cerrado que empiece con `m` relativa
// justo ahí se queda sin relleno, igual que en la web. Ningún catálogo tiene uno (lo
// vigila `glifos-al-comun.test.ts`); si aparece, se sigue el punto en los dos lados.
func subtrazosCerrados(d string) string {
	var cerrados []string
	pluma, sabida := [2]float64{}, true // dónde está la pluma al empezar el subtrazo
	for _, sub := range partirEnM(strings.TrimSpace(d)) {
		sub = strings.TrimSpace(sub)
		m := inicioSub.FindStringSubmatch(sub)
		var inicio [2]float64
		conInicio := m != nil
		relativa := conInicio && m[1] == "m"
		if conInicio {
			x, _ := strconv.ParseFloat(m[2], 64)
			y, _ := strconv.ParseFloat(m[3], 64)
			inicio = [2]float64{x, y}
			if relativa {
				inicio, conInicio = [2]float64{pluma[0] + x, pluma[1] + y}, sabida
			}
		}
		cerrado := strings.HasSuffix(sub, "Z") || strings.HasSuffix(sub, "z")
		if conInicio && cerrado {
			resto := sub[len(m[0]):]
			// Los pares que siguen a una `m` son líneas relativas implícitas: con la
			// `M` absoluta delante hay que decirlo, o pasarían a ser absolutas.
			sigue := ""
			if resto != "" && strings.ContainsRune("-+.0123456789", rune(resto[0])) {
				sigue = " "
				if relativa {
					sigue = "l"
				}
			}
			cerrados = append(cerrados, "M"+n3(inicio[0])+" "+n3(inicio[1])+sigue+resto)
		}
		pluma, sabida = inicio, cerrado && conInicio
	}
	return strings.Join(cerrados, " ")
}

// partirEnM corta un `d` delante de cada `M` o `m`, que son las únicas órdenes que
// abren subtrazo.
func partirEnM(d string) []string {
	var out []string
	desde := 0
	for i := 1; i < len(d); i++ {
		if d[i] == 'M' || d[i] == 'm' {
			out = append(out, d[desde:i])
			desde = i
		}
	}
	return append(out, d[desde:])
}

// radioPunto es `RADIO_PUNTO` de Glifo.tsx: el radio, en la rejilla, de un punto.
const radioPunto = 1.25

// unPunto es un subtrazo que es solo un punto: `M x y h.01`.
var unPunto = regexp.MustCompile(`^M\s*(` + numeroSub + `)[\s,]*(` + numeroSub + `)\s*h\.01$`)

// puntos es `puntos` de Glifo.tsx: los puntos de un `d` —los de «…», las viñetas— como
// círculos, para pintarlos macizos. Con el trazo de 1,5 px un tramo de longitud cero se
// queda en una mota; así el punto tiene tamaño propio y no sale del grosor.
func puntos(d string) string {
	var out []string
	for _, sub := range partirEnM(strings.TrimSpace(d)) {
		m := unPunto.FindStringSubmatch(strings.TrimSpace(sub))
		if m == nil {
			continue
		}
		x, _ := strconv.ParseFloat(m[1], 64)
		r := n3(radioPunto)
		out = append(out, "M"+n3(x-radioPunto)+" "+m[2]+"a"+r+" "+r+" 0 1 0 "+n3(2*radioPunto)+" 0a"+r+" "+r+" 0 1 0 "+n3(-2*radioPunto)+" 0Z")
	}
	return strings.Join(out, " ")
}

// puntosDeCuerpo son los puntos de los `<path>` de un glifo escrito con figuras, con las
// mismas reglas que rellenoDeCuerpo: ni lo que lleva `fill` o `stroke` propios ni lo de
// dentro de un `<g>`.
func puntosDeCuerpo(cuerpo string) string {
	return deCuerpo(cuerpo, func(tipo string, at map[string]string) string {
		if tipo != "path" {
			return ""
		}
		return puntos(at["d"])
	})
}

// rellenoDeCuerpo es el relleno de las figuras hijas de Glifo.tsx: lo que se rellena de un glifo
// escrito con figuras (`TrazoDeCuerpo`), como UN solo trazado para que el par-impar
// abra huecos entre figuras distintas (el reloj no, el ojo y su pupila sí).
//
// Las reglas son las de allí: una figura con `fill` o `stroke` propios ya dice cómo se
// pinta y no lleva relleno debajo; `line` y `polyline` son abiertas; y lo que va
// dentro de un `<g>` tampoco, porque está en otro sistema de coordenadas (el sello de
// `GlifoPDF`).
func rellenoDeCuerpo(cuerpo string) string { return deCuerpo(cuerpo, contorno) }

// deCuerpo recorre las figuras de primer nivel de un cuerpo y junta lo que `de` saque
// de cada una.
func deCuerpo(cuerpo string, de func(tipo string, at map[string]string) string) string {
	var partes []string
	dec := xml.NewDecoder(strings.NewReader("<r>" + cuerpo + "</r>"))
	for {
		tok, err := dec.Token()
		if err != nil {
			break // fin, o un cuerpo que `oksvg` tampoco va a entender: él dirá lo suyo
		}
		e, ok := tok.(xml.StartElement)
		if !ok || e.Name.Local == "r" {
			continue
		}
		at := map[string]string{}
		for _, a := range e.Attr {
			at[a.Name.Local] = a.Value
		}
		_, conFill := at["fill"]
		_, conStroke := at["stroke"]
		if c := de(e.Name.Local, at); c != "" && !conFill && !conStroke {
			partes = append(partes, c)
		}
		// Sin bajar a los hijos: ni a los de un `<g>` ni a nada que cuelgue de una figura.
		if dec.Skip() != nil {
			break
		}
	}
	return strings.Join(partes, " ")
}

// contorno es el de Glifo.tsx: el borde de una figura como `d`.
func contorno(tipo string, at map[string]string) string {
	n := func(k string) float64 { v, _ := strconv.ParseFloat(at[k], 64); return v }
	switch tipo {
	case "path":
		return subtrazosCerrados(at["d"])
	case "circle", "ellipse":
		rx, ry := n("rx"), n("ry")
		if tipo == "circle" {
			rx, ry = n("r"), n("r")
		}
		return "M" + n3(n("cx")-rx) + " " + n3(n("cy")) +
			"a" + n3(rx) + " " + n3(ry) + " 0 1 0 " + n3(2*rx) + " 0" +
			"a" + n3(rx) + " " + n3(ry) + " 0 1 0 " + n3(-2*rx) + " 0Z"
	case "rect":
		w, h := n("width"), n("height")
		// `rx` sin `ry` (y al revés) vale para los dos, como dice SVG.
		rx, ry := n("rx"), n("ry")
		if _, hay := at["rx"]; !hay {
			rx = ry
		}
		if _, hay := at["ry"]; !hay {
			ry = rx
		}
		rx, ry = math.Min(rx, w/2), math.Min(ry, h/2)
		esquina := func(dx, dy float64) string {
			if rx == 0 || ry == 0 {
				return ""
			}
			return "a" + n3(rx) + " " + n3(ry) + " 0 0 1 " + n3(dx) + " " + n3(dy)
		}
		return "M" + n3(n("x")+rx) + " " + n3(n("y")) + "h" + n3(w-2*rx) + esquina(rx, ry) +
			"v" + n3(h-2*ry) + esquina(-rx, ry) + "h" + n3(2*rx-w) + esquina(-rx, -ry) +
			"v" + n3(2*ry-h) + esquina(rx, -ry) + "Z"
	case "polygon":
		return "M" + strings.TrimSpace(at["points"]) + "Z"
	}
	return ""
}
