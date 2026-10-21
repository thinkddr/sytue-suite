// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package office

import (
	"encoding/json"
	"net/url"
	"regexp"
	"slices"
	"strings"
	"unicode"
	"unicode/utf8"
)

// Validación de las macros: la misma regla que `editor/macros.ts` en la web, escrita en
// Go para la cinta nativa. Una macro es texto guardado y reproducido más tarde, así que
// reproducir no se fía de lo guardado: antes de despachar NADA se comprueba cada paso
// contra el registro de comandos, el tipo de documento, el permiso de ahora y el valor
// (que sea de los que la cinta puede producir).
//
// Son DOS copias de una regla y las sujeta una sola tabla de casos,
// `apps/frontend/packages/office/test/vectores/macros.json`, que exigen la web
// (`macros.test.ts`) y este paquete (`macros_test.go`). Un caso que cambie, cambia en las
// dos; una regla que se toque en una sola, se pone en rojo en la otra.

// Los techos de una macro; los mismos que en la web.
const (
	MaxPasos  = 200       // pasos de una macro
	MaxMacros = 50        // macros por usuario
	MaxValor  = 64 * 1024 // caracteres de un valor
	MaxNombre = 80        // caracteres del nombre de una macro
)

// Paso es un paso grabado: el comando, su valor opcional y el editor del que salió.
type Paso struct {
	Editor string
	Cmd    string
	Valor  string
	// ConValor distingue «sin valor» de «valor vacío», como `undefined` y `''` en la web.
	ConValor bool
}

// Origenes son los sitios de los que puede venir una imagen o un medio sin salir de la casa.
type Origenes struct {
	Propio string   // el origen de la página o de la aplicación
	Drive  []string // los orígenes de Sytue Drive
}

// Entorno es dónde se reproduce: el tipo de documento abierto y el permiso de ahora.
type Entorno struct {
	Kind        string
	SoloLectura bool
	Origenes    Origenes
}

// Rechazo es por qué un paso no se reproduce. Vacío: se puede.
type Rechazo string

// Los motivos, con los mismos nombres que en la web.
const (
	Valido      Rechazo = ""
	Excluido    Rechazo = "excluido"    // de los que nunca se reproducen (contraseñas, deshacer…)
	NoConocido  Rechazo = "comando"     // no está en el registro
	OtroTipo    Rechazo = "tipo"        // no existe en este tipo de documento
	SoloLectura Rechazo = "soloLectura" // edita y ahora no se puede editar
	ValorMalo   Rechazo = "valor"       // su valor no es de los que la cinta produce
	Limite      Rechazo = "limite"      // más pasos de los admitidos
)

// NoReproducibles son los comandos que nunca se reproducen: deshacer y rehacer
// corromperían la reproducción, «presentar» no es una edición, los de protección llevan la
// CONTRASEÑA tecleada en su valor, y `extdata` lleva el id de una conexión de datos.
var NoReproducibles = map[string]bool{
	"undo": true, "redo": true, "presentar": true,
	"protect": true, "unprotect": true, "protectbook": true, "unprotectbook": true,
	"extdata": true,
}

var (
	fuentes        = []string{"Inter", "Arial", "Georgia", "Times New Roman", "Courier New", "JetBrains Mono"}
	tallas         = []string{"1", "2", "3", "4", "5", "6", "7"}
	interlineados  = []string{"1", "1.15", "1.5", "2"}
	bloques        = []string{"p", "h1", "h2", "h3", "blockquote"}
	alineacionVert = []string{"top", "middle", "bottom"}
	disposiciones  = []string{"titulo", "titulo-cuerpo", "dos-columnas"}

	reColor      = regexp.MustCompile(`^#(?:[0-9a-fA-F]{3}|[0-9a-fA-F]{6})$`)
	reDataImagen = regexp.MustCompile(`(?i)^data:image/(?:png|jpe?g|gif|webp|avif|bmp);base64,[a-z0-9+/=\s]*$`)
	reDataMedio  = regexp.MustCompile(`(?i)^data:(?:image|audio|video)/[a-z0-9.+-]+;base64,[a-z0-9+/=\s]*$`)
	reEsquema    = regexp.MustCompile(`(?i)^([a-z][a-z0-9+.-]*):`)
	reClaveColor = regexp.MustCompile(`(?i)colou?r|fill|background|^bg`)
	reTabla      = regexp.MustCompile(`(?i)^\d{1,2}\s*[x×]\s*\d{1,2}$`)
	reNumFmt     = regexp.MustCompile(`(?i)^(?:custom:.{1,128}|[a-z0-9_-]{1,32})$`)
	reEntero     = regexp.MustCompile(`^\d{1,6}$`)
	reCondMove   = regexp.MustCompile(`^\d{1,6},(?:-1|1)$`)
	reCongelar   = regexp.MustCompile(`^\d{1,3},\d{1,6}$`)
	reCelda      = regexp.MustCompile(`(?i)^[A-Z]{1,3}\d{1,7}$`)
	reEstilo     = regexp.MustCompile(`^[\w-]{1,64}$`)
)

// Color dice si es un color de contenido: `#rgb` o `#rrggbb`; con transparente, también «transparent».
func Color(v string, transparente bool) bool {
	return reColor.MatchString(v) || (transparente && v == "transparent")
}

func longitud(v string) int { return utf8.RuneCountInString(v) }

// origen es `URL.origin` de la web: esquema y host, sin el puerto por defecto.
func origen(u *url.URL) string {
	host := strings.ToLower(u.Hostname())
	puerto := u.Port()
	if (u.Scheme == "https" && puerto == "443") || (u.Scheme == "http" && puerto == "80") {
		puerto = ""
	}
	if puerto != "" {
		host += ":" + puerto
	}
	return strings.ToLower(u.Scheme) + "://" + host
}

// URLPermitida dice si esta URL es una fuente permitida para una imagen (o, con medio, un
// audio o vídeo): `data:` del tipo que toca, `blob:` de esta página, una ruta del propio
// origen o un origen de Drive. Una dirección externa sería una petición a un tercero que
// nadie ve. SVG queda fuera a propósito: es código.
func URLPermitida(valor string, o Origenes, medio bool) bool {
	if valor == "" {
		return false
	}
	if strings.HasPrefix(valor, "data:") {
		if medio {
			return reDataMedio.MatchString(valor)
		}
		return reDataImagen.MatchString(valor)
	}
	if strings.HasPrefix(valor, "blob:") {
		u, err := url.Parse(valor[5:])
		return err == nil && u.Host != "" && origen(u) == o.Propio
	}
	// «//otro-sitio» es una URL con esquema heredado y empieza por «/».
	if strings.HasPrefix(valor, "/") && !strings.HasPrefix(valor, "//") {
		return true
	}
	u, err := url.Parse(valor)
	if err != nil || u.Host == "" || (u.Scheme != "https" && u.Scheme != "http") {
		return false
	}
	og := origen(u)
	return og == o.Propio || slices.Contains(o.Drive, og)
}

type validador func(v string, conValor bool, e Entorno) bool

func unoDe(lista ...string) validador {
	return func(v string, conValor bool, _ Entorno) bool { return conValor && slices.Contains(lista, v) }
}

func opcional(f validador) validador {
	return func(v string, conValor bool, e Entorno) bool { return !conValor || v == "" || f(v, true, e) }
}

func conforme(re *regexp.Regexp) validador {
	return func(v string, conValor bool, _ Entorno) bool { return conValor && re.MatchString(v) }
}

// texto es un texto corto, no vacío salvo que se diga: un nombre, una fórmula, un delimitador.
func texto(tope int, vacio bool) validador {
	return func(v string, conValor bool, _ Entorno) bool {
		return conValor && longitud(v) <= tope && (vacio || v != "")
	}
}

func color(v string, conValor bool, _ Entorno) bool { return conValor && Color(v, false) }
func colorOTransparente(v string, conValor bool, _ Entorno) bool {
	return conValor && Color(v, true)
}

// jsonSano: de poca hondura y con un color válido en cada clave que lo pide, a cualquier profundidad.
func jsonSano(v any, hondo int) bool {
	if hondo > 8 {
		return false
	}
	switch x := v.(type) {
	case []any:
		for _, e := range x {
			if !jsonSano(e, hondo+1) {
				return false
			}
		}
	case map[string]any:
		for k, e := range x {
			if s, ok := e.(string); ok && reClaveColor.MatchString(k) {
				if !Color(s, true) {
					return false
				}
				continue
			}
			if !jsonSano(e, hondo+1) {
				return false
			}
		}
	}
	return true
}

func leerJSON(v string, conValor bool) (any, bool) {
	if !conValor || v == "" || longitud(v) > MaxValor {
		return nil, false
	}
	var d any
	if err := json.Unmarshal([]byte(v), &d); err != nil {
		return nil, false
	}
	return d, true
}

// esJSON es el JSON de una regla, un gráfico, una tabla…: objeto o lista, de poca hondura y con colores válidos.
func esJSON(v string, conValor bool, _ Entorno) bool {
	d, ok := leerJSON(v, conValor)
	if !ok {
		return false
	}
	switch d.(type) {
	case map[string]any, []any:
		return jsonSano(d, 0)
	}
	return false
}

func enlace(v string, conValor bool, _ Entorno) bool {
	if !conValor || v == "" || longitud(v) > 2048 || strings.IndexFunc(v, unicode.IsSpace) >= 0 {
		return false
	}
	m := reEsquema.FindStringSubmatch(v)
	if m == nil {
		return true // un dominio pelado: `exec` lo pasa a https
	}
	switch strings.ToLower(m[1]) {
	case "http", "https", "mailto":
		return true
	}
	return false
}

func imagen(v string, conValor bool, e Entorno) bool {
	return conValor && URLPermitida(v, e.Origenes, false)
}

// medio es el audio o vídeo de una diapositiva: viaja como JSON `{ src, mediaType… }`.
func medio(v string, conValor bool, e Entorno) bool {
	d, ok := leerJSON(v, conValor)
	m, esObjeto := d.(map[string]any)
	if !ok || !esObjeto || !jsonSano(d, 0) {
		return false
	}
	src, ok := m["src"].(string)
	return ok && URLPermitida(src, e.Origenes, true)
}

// valores es lo que cada comando admite como valor. Los que no figuran aceptan nada o un
// texto que no desborde. La mantiene a la vista de `types.ts`, `SheetCanvas.tsx` y
// `SlidesCanvas.tsx` de la web, cuyos comentarios describen cada valor.
var valores = map[string]validador{
	// Documento y diapositiva
	"font": unoDe(fuentes...), "size": unoDe(tallas...), "color": color, "highlight": colorOTransparente,
	"lineHeight": unoDe(interlineados...), "heading": unoDe(bloques...), "insertTable": conforme(reTabla),
	"insertImage": imagen, "insertLink": enlace, "tableCellBg": colorOTransparente,
	"tableCellValign": unoDe(alineacionVert...),
	// Hoja
	"align": unoDe("left", "center", "right"), "numfmt": conforme(reNumFmt), "decimals": unoDe("+1", "-1"),
	"border": unoDe("all", "outer", "bottom", "none"), "style": esJSON, "condadd": esJSON,
	"conddel": conforme(reEntero), "condmove": conforme(reCondMove), "validadd": esJSON,
	"validdel": conforme(reEntero), "copy": opcional(unoDe("cut")), "paste": opcional(unoDe("values")),
	"freeze": conforme(reCongelar), "sort": unoDe("asc", "desc"), "filterset": esJSON, "filtercond": esJSON, "filtersort": esJSON,
	"text2cols": texto(8, false), "insrow": unoDe("above", "below"), "inscol": unoDe("left", "right"),
	"chartadd": esJSON, "sparkadd": esJSON, "sparkdel": conforme(reCelda), "pivot": esJSON,
	"pivotrefresh": texto(255, true), "pivotdel": texto(255, false), "sliceradd": esJSON,
	"aifill": texto(MaxValor, false), "goalseek": esJSON, "solverapply": esJSON, "fillcells": esJSON,
	"scenadd": esJSON, "scenupdate": texto(255, false), "scenapply": texto(255, false),
	"scendel": texto(255, false), "scenreport": esJSON, "datatable": esJSON, "tableadd": esJSON,
	"tablestyle": conforme(reEstilo), "tabletotalfn": esJSON, "outline": esJSON, "subtotal": esJSON,
	// Diapositivas
	"nueva": opcional(unoDe(disposiciones...)), "disposicion": opcional(unoDe(disposiciones...)),
	"imagen": imagen, "media": medio, "forma": texto(64, false), "smartart": texto(64, false),
	"organizar": texto(64, false), "conector": opcional(unoDe("straight", "elbow", "curved")),
	"tamano": opcional(esJSON), "iafill": texto(MaxValor, false),
}

// ValidarPaso dice por qué NO se reproduce este paso aquí y ahora; [Valido] si se puede.
func ValidarPaso(p Paso, e Entorno) Rechazo {
	if NoReproducibles[p.Cmd] {
		return Excluido
	}
	def, ok := BuscarComando(p.Cmd)
	if !ok {
		return NoConocido
	}
	if p.Editor != e.Kind || !def.Existe(p.Editor) {
		return OtroTipo
	}
	// «Cortar» es `copy` con valor: el registro lo da por lectura y mueve celdas.
	edita := def.Edits || (p.Cmd == "copy" && p.ConValor && p.Valor == "cut")
	if edita && e.SoloLectura {
		return SoloLectura
	}
	valido, ok := valores[p.Cmd]
	if !ok {
		valido = func(v string, conValor bool, _ Entorno) bool { return !conValor || longitud(v) <= MaxValor }
	}
	if !valido(p.Valor, p.ConValor, e) {
		return ValorMalo
	}
	return Valido
}

// ValidarMacro valida la macro ENTERA antes de despachar nada: el primer paso que falla
// (con su posición, desde 0) o [Valido].
func ValidarMacro(pasos []Paso, e Entorno) (Rechazo, int) {
	if len(pasos) > MaxPasos {
		return Limite, MaxPasos
	}
	for i, p := range pasos {
		if r := ValidarPaso(p, e); r != Valido {
			return r, i
		}
	}
	return Valido, 0
}
