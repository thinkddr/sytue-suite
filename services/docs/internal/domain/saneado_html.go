// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package domain

import (
	"bytes"
	"encoding/json"
	"html"
	"strings"

	xhtml "golang.org/x/net/html"
)

// SANEADO DEL HTML DE UN DOCUMENTO (XSS-01, auditoría de seguridad del 2026-10-06).
//
// El contenido de un documento es HTML que escribe quien lo edita —también una persona de
// otra organización que canjeó un enlace de editor— y que pinta el navegador de los demás.
// Hasta aquí el servicio lo guardaba tal cual: un `onerror` en una imagen se ejecutaba en
// la sesión de quien abriera el documento. El navegador sanea antes de pintar
// (`saneaHtmlDeDocumento`, en @sytue/ui); esto es la misma regla puesta donde no se puede
// saltar, para que lo guardado sea seguro para cualquier cliente.
//
// EL CONTRATO ES UNO PARA LOS DOS LADOS y está medido sobre lo que el editor escribe: cuatro
// reglas (qué se quita con su contenido, qué etiquetas se conservan, qué atributos y qué
// esquemas de URL). Si cambia aquí tiene que cambiar allí, y al revés.
//
// TRES COSAS QUE ROMPEN DOCUMENTOS SI SE HACEN COMO EN UN SANEADOR DE CORREO:
//
//   - `<foreignObject>` se conserva: lleva el texto de las formas y de SmartArt.
//   - El comentario `<!--sytue-track-->` al principio se conserva: es el interruptor del
//     control de cambios. Los demás comentarios se quitan.
//   - `style` no se reduce a una lista de propiedades ni rechaza `url(`: el editor escribe
//     con `styleWithCSS` y la marca de agua de imagen va ahí como `url(data:image/…)`.
//
// Y UNA CUARTA, QUE DECIDE LA FORMA DE ESTE CÓDIGO: para contenido legítimo el saneador es
// una NO-OPERACIÓN BYTE A BYTE. El documento se coedita además por carácter, y un saneador
// que reserializara cambiaría comillas, mayúsculas o entidades en cada guardado y haría
// divergir el registro de la secuencia. Por eso NO se construye un árbol: se recorre con el
// tokenizador y cada pieza que se conserva sale con sus bytes originales. Solo se reescribe
// la etiqueta a la que hay que quitarle un atributo.
//
// Lo que sale es, siempre: texto sin ningún `<` crudo, etiquetas de la lista con sus
// atributos filtrados y, como mucho, ese comentario al principio. No hay otra cosa que el
// navegador pueda leer de otra manera.

// marcaControlDeCambios es el único comentario que se conserva, y solo al principio.
const marcaControlDeCambios = "<!--sytue-track-->"

func conjunto(palabras string) map[string]bool {
	out := map[string]bool{}
	for _, p := range strings.Fields(palabras) {
		out[strings.ToLower(p)] = true
	}
	return out
}

var (
	// Regla 1: se eliminan CON su contenido.
	htmlQuitadas = conjunto(`script style iframe object embed base meta link noscript template
		frame frameset applet portal fencedframe xmp noembed noframes plaintext math
		animate animateMotion animateTransform set`)
	// De esas, las que no tienen contenido: se quita la etiqueta y nada más. Esperar su
	// cierre, que no existe, se llevaría el resto del documento.
	htmlQuitadasVacias = conjunto(`base meta link embed frame`)
	// Las que el tokenizador lee como texto crudo hasta su cierre aunque vengan «autocerradas».
	htmlCrudas = conjunto(`script style iframe noscript xmp noembed noframes plaintext`)

	// Regla 2: etiquetas que se conservan. Las demás se DESENVUELVEN: se quita la etiqueta
	// y se conservan sus hijos, ya saneados. Nunca se tira contenido por una etiqueta
	// desconocida.
	htmlPermitidas = conjunto(`
		a b blockquote br del div em figure font h1 h2 h3 h4 h5 h6 hr i img ins li mark nav
		ol p s section span strong sup table tbody td th tr u ul
		svg g path rect circle ellipse line polygon text tspan image defs title desc foreignObject
		strike sub pre code small big center thead tfoot caption colgroup col figcaption dl dt
		dd abbr cite q kbd samp var time wbr bdi bdo address article aside header footer main
		hgroup details summary label audio video source track picture
		polyline clipPath linearGradient radialGradient stop marker pattern mask symbol use`)

	// Regla 4: atributos que llevan una URL, y de qué clase.
	atributosDeEnlace  = conjunto(`href xlink:href action formaction ping`)
	atributosDeRecurso = conjunto(`src poster background srcset`)
	// En estos elementos `href` es un recurso (la marca de agua es `<image href="data:image/…">`).
	elementosDeRecurso = conjunto(`image feImage`)

	esquemasDeEnlace  = []string{"http:", "https:", "mailto:", "tel:", "cid:", "/", "./", "../", "#"}
	esquemasDeRecurso = []string{"blob:", "data:image/", "data:video/", "data:audio/", "data:text/vtt"}
	// Regla 3, `style`: se conserva entero salvo que lleve una de estas.
	estiloVetado = []string{"javascript:", "expression(", "-moz-binding", "behavior:"}
)

// SanearHTML devuelve `in` con el contrato aplicado. Si no hay nada que quitar devuelve
// `in` tal cual, sin reservar ni copiar.
//
//nolint:gocognit,gocyclo // un solo recorrido del tokenizador: partirlo separaría el estado de lo que se salta de quien lo consulta.
func SanearHTML(in string) string {
	if !strings.Contains(in, "<") {
		return in // sin etiquetas ni comentarios no hay nada que quitar
	}
	var (
		out      strings.Builder
		cambiado bool
		leido    int    // bytes de `in` ya recorridos
		saltando string // etiqueta que se está quitando con su contenido
		hondura  int    // cuántas de esas hay abiertas
	)
	// Mientras no cambie nada no se escribe: `out` solo se llena desde el primer cambio.
	emitir := func(pieza string, igual bool) {
		if igual && !cambiado {
			return
		}
		if !cambiado {
			cambiado = true
			out.Grow(len(in))
			out.WriteString(in[:leido])
		}
		out.WriteString(pieza)
	}
	z := xhtml.NewTokenizer(strings.NewReader(in))
	for {
		tt := z.Next()
		if tt == xhtml.ErrorToken {
			break
		}
		// Una copia: TagName y TagAttr pasan a minúsculas y resuelven entidades EN el búfer.
		crudo := string(z.Raw())
		switch tt {
		case xhtml.TextToken:
			switch {
			case saltando != "":
				emitir("", false)
			case strings.Contains(crudo, "<"):
				// El editor nunca deja un `<` crudo en el texto (el navegador lo serializa
				// como `&lt;`). Si lo hay es el contenido de un <title> o un <textarea>, o lo
				// que queda junto a una etiqueta quitada: como texto se queda, escapado.
				emitir(strings.ReplaceAll(crudo, "<", "&lt;"), false)
			default:
				emitir(crudo, true)
			}
		case xhtml.CommentToken:
			if leido == 0 && crudo == marcaControlDeCambios {
				emitir(crudo, true)
			} else {
				emitir("", false)
			}
		case xhtml.StartTagToken, xhtml.SelfClosingTagToken:
			nombreB, _ := z.TagName()
			nombre := string(nombreB)
			switch {
			case saltando != "":
				if nombre == saltando && tt == xhtml.StartTagToken {
					hondura++
				}
				emitir("", false)
			case htmlQuitadas[nombre]:
				if !htmlQuitadasVacias[nombre] && (tt == xhtml.StartTagToken || htmlCrudas[nombre]) {
					saltando, hondura = nombre, 1
				}
				emitir("", false)
			case !htmlPermitidas[nombre]:
				emitir("", false)
			default:
				if limpia, igual := etiquetaSaneada(z, nombre, tt == xhtml.SelfClosingTagToken); igual {
					emitir(crudo, true)
				} else {
					emitir(limpia, false)
				}
			}
		case xhtml.EndTagToken:
			nombreB, _ := z.TagName()
			nombre := string(nombreB)
			switch {
			case saltando != "":
				if nombre == saltando {
					hondura--
					if hondura == 0 {
						saltando = ""
					}
				}
				emitir("", false)
			case htmlPermitidas[nombre]:
				emitir(crudo, true)
			default:
				emitir("", false)
			}
		case xhtml.DoctypeToken, xhtml.ErrorToken:
			emitir("", false)
		}
		leido += len(crudo)
	}
	if leido != len(in) {
		// Una etiqueta sin cerrar al final: el navegador la descarta, y aquí también.
		emitir("", false)
	}
	if !cambiado {
		return in
	}
	return out.String()
}

// etiquetaSaneada mira los atributos de una etiqueta permitida. Si se conservan todos dice
// `igual` y quien llama emite los bytes originales; si no, devuelve la etiqueta reescrita
// sin los que sobran.
func etiquetaSaneada(z *xhtml.Tokenizer, nombre string, autocerrada bool) (limpia string, igual bool) {
	type atributo struct{ clave, valor string }
	var (
		conservados []atributo
		quitado     bool
	)
	for {
		k, v, mas := z.TagAttr()
		if clave := string(k); clave != "" {
			if atributoAdmitido(nombre, clave, string(v)) {
				conservados = append(conservados, atributo{clave, string(v)})
			} else {
				quitado = true
			}
		}
		if !mas {
			break
		}
	}
	if !quitado {
		return "", true
	}
	var b strings.Builder
	b.WriteString("<" + nombre)
	for _, a := range conservados {
		b.WriteString(" " + a.clave + `="` + html.EscapeString(a.valor) + `"`)
	}
	if autocerrada {
		b.WriteString("/")
	}
	b.WriteString(">")
	return b.String(), false
}

// atributoAdmitido es la regla 3: se conserva todo atributo salvo los manejadores de
// evento (`on…`), `srcdoc`, los de URL que no pasan la regla 4 y un `style` con una de las
// cuatro cosas vetadas. Es regla y no lista porque el editor usa más de cien `data-…` y
// siguen creciendo.
func atributoAdmitido(elemento, clave, valor string) bool {
	switch {
	case strings.HasPrefix(clave, "on"), clave == "srcdoc":
		return false
	case clave == "style":
		bajo := strings.ToLower(valor)
		for _, v := range estiloVetado {
			if strings.Contains(bajo, v) {
				return false
			}
		}
		return true
	case atributosDeRecurso[clave], atributosDeEnlace[clave] && elementosDeRecurso[elemento]:
		return urlAdmitida(valor, true)
	case atributosDeEnlace[clave]:
		return urlAdmitida(valor, false)
	}
	return true
}

// urlAdmitida es la regla 4: lista blanca por prefijo sobre el valor recortado y en
// minúsculas, así que `javascript:`, `vbscript:`, `data:text/html` y sus variantes con
// espacios, tabuladores, mayúsculas o entidades caen solas.
func urlAdmitida(valor string, recurso bool) bool {
	v := strings.ToLower(strings.TrimFunc(valor, func(r rune) bool { return r <= ' ' }))
	if v == "" {
		return true
	}
	for _, p := range esquemasDeEnlace {
		if strings.HasPrefix(v, p) {
			return true
		}
	}
	if recurso {
		for _, p := range esquemasDeRecurso {
			if strings.HasPrefix(v, p) {
				return true
			}
		}
	}
	return false
}

// SanearContenido sanea el valor del registro de contenido `clave` de un documento: el HTML
// de un documento de texto, o el JSON de una hoja (el HTML de cada celda) o de una
// presentación (el `html` de cada caja). Lo demás de esos modelos no es HTML y no se toca.
// Si no hay nada que quitar devuelve `contenido` tal cual.
func SanearContenido(clave, contenido string) string {
	switch clave {
	case "doc:html":
		return SanearHTML(contenido)
	case claveHoja, claveDiapos:
	default:
		return contenido
	}
	// Sin un `<` —ni escrito como escape de JSON— no hay HTML en ninguna cadena del modelo:
	// lo normal en una hoja de números, que así no paga ni la lectura del JSON.
	if !strings.Contains(contenido, "<") && !strings.Contains(contenido, `\u003c`) && !strings.Contains(contenido, `\u003C`) {
		return contenido
	}
	dec := json.NewDecoder(strings.NewReader(contenido))
	dec.UseNumber() // los números vuelven a salir como se escribieron
	var modelo any
	if dec.Decode(&modelo) != nil {
		return contenido // no es un modelo: ningún cliente saca de él una celda ni una caja
	}
	var cambiado bool
	if clave == claveHoja {
		cambiado = sanearCeldas(modelo)
	} else {
		cambiado = sanearCampoHTML(modelo)
	}
	if !cambiado {
		return contenido
	}
	var b bytes.Buffer
	enc := json.NewEncoder(&b)
	enc.SetEscapeHTML(false)
	if enc.Encode(modelo) != nil {
		return contenido
	}
	return strings.TrimSuffix(b.String(), "\n")
}

// sanearCeldas sanea `sheets[].cells[ref]`, que es donde una hoja lleva HTML.
func sanearCeldas(modelo any) bool {
	m, _ := modelo.(map[string]any)
	hojas, _ := m["sheets"].([]any)
	cambiado := false
	for _, h := range hojas {
		hoja, _ := h.(map[string]any)
		celdas, _ := hoja["cells"].(map[string]any)
		for ref, v := range celdas {
			if txt, ok := v.(string); ok {
				if limpio := SanearHTML(txt); limpio != txt {
					celdas[ref], cambiado = limpio, true
				}
			}
		}
	}
	return cambiado
}

// sanearCampoHTML sanea todo campo `html` del modelo de una presentación: el de las cajas
// de las diapositivas, del patrón y de los diseños, sin la lista de sitios donde hoy hay cajas.
func sanearCampoHTML(v any) bool {
	cambiado := false
	switch x := v.(type) {
	case []any:
		for _, e := range x {
			cambiado = sanearCampoHTML(e) || cambiado
		}
	case map[string]any:
		for k, e := range x {
			if txt, ok := e.(string); ok && k == "html" {
				if limpio := SanearHTML(txt); limpio != txt {
					x[k], cambiado = limpio, true
				}
				continue
			}
			cambiado = sanearCampoHTML(e) || cambiado
		}
	}
	return cambiado
}

// SanearOps sanea, en sitio, los `set` de los registros de contenido de un lote. Es lo que
// llama el almacén antes de guardar.
func SanearOps(ops []Op) {
	for i := range ops {
		if ops[i].Kind != OpSet {
			continue
		}
		switch ops[i].Key {
		case "doc:html", claveHoja, claveDiapos:
		default:
			continue
		}
		var contenido string
		if json.Unmarshal(ops[i].Value, &contenido) != nil {
			continue
		}
		if limpio := SanearContenido(ops[i].Key, contenido); limpio != contenido {
			ops[i].Value = jsonString(limpio)
		}
	}
}
