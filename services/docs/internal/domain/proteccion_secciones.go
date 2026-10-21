// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package domain

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"regexp"
	"slices"
	"strings"

	"golang.org/x/net/html"
	"golang.org/x/net/html/atom"
)

// Secciones protegidas de un documento de texto (ROADMAP-OFFICE permisos-3): regiones
// del HTML de Docus que solo pueden editar ciertas personas o grupos.
//
// MARCADO. Un elemento cualquiera del registro `doc:html` con tres atributos:
//
//	<section data-sytue-seccion="firmas" data-sytue-puede="<sub> <sub>" data-sytue-grupos="<grupo>"><p>…</p></section>
//
// (se recomienda `<section>` con párrafos dentro: al pulsar Intro el navegador parte el
// párrafo y no clona el elemento marcado, que repetiría el identificador.)
//
//   - data-sytue-seccion: identificador estable de la sección (letras, cifras, «_» y «-»,
//     hasta 64). Debe ser único en el documento.
//   - data-sytue-puede: `sub` de las personas que pueden editarla, separados por espacios.
//   - data-sytue-grupos: grupos de Sytue ID que pueden editarla, separados por espacios.
//
// REGLAS (las cumple el servidor al aplicar un set de `doc:html`, dentro de la misma
// transacción que bloquea el documento; la comparación es contra el último valor EFECTIVO
// del registro, no contra lo que dice el cliente):
//   - El propietario puede todo, también crear, cambiar o quitar secciones.
//   - Cualquier otra persona NO puede crear, quitar ni cambiar el marcado de una sección
//     (identificador, personas, grupos): sería abrirse un hueco o cerrárselo a otro.
//   - Una persona que no figura (ni por `sub` ni por grupo) en una sección no puede
//     cambiar su contenido; las que figuran, sí. Fuera de las secciones se edita como
//     siempre.
//
// El contenido se compara canónicamente (se parsea y se vuelve a serializar), así que
// las formas equivalentes de escribir el mismo HTML no cuentan como cambio. El estado
// que se guarda por documento es solo una huella por sección (no el HTML entero).
//
// Las operaciones por carácter de la secuencia RGA (insert/delete) se validan con la misma
// regla: ver proteccion_secuencia.go.

const (
	atrSeccion = "data-sytue-seccion"
	atrPuede   = "data-sytue-puede"
	atrGrupos  = "data-sytue-grupos"
	claveHTML  = "doc:html"
)

var reIDSeccion = regexp.MustCompile(`^[A-Za-z0-9_-]{1,64}$`)

// TocaHTML indica si el lote puede cambiar el contenido del documento de texto.
func TocaHTML(ops []Op) bool {
	for i := range ops {
		if ops[i].Kind == OpSet && ops[i].Key == claveHTML {
			return true
		}
	}
	return false
}

// seccion es la huella de una sección en un instante.
type seccion struct {
	ID     string   `json:"id"`
	Puede  []string `json:"puede,omitempty"`
	Grupos []string `json:"grupos,omitempty"`
	Marco  string   `json:"marco"`  // etiqueta y atributos del elemento, canónicos
	Huella string   `json:"huella"` // sha256 del contenido canónico
}

// marca es el marcado sin el contenido.
func (s seccion) marca() string {
	return s.ID + "|" + strings.Join(s.Puede, " ") + "|" + strings.Join(s.Grupos, " ") + "|" + s.Marco
}

func (s seccion) permite(quien Subject) bool {
	if quien.Sub != "" && slices.Contains(s.Puede, quien.Sub) {
		return true
	}
	for _, g := range s.Grupos {
		if slices.Contains(quien.Groups, g) {
			return true
		}
	}
	return false
}

// seccionesDe extrae las secciones de un HTML. ok=false si el marcado no es válido
// (identificador ilegible o repetido): quien no es propietario no puede dejarlo así.
func seccionesDe(contenido string) (out []seccion, ok bool) {
	if !strings.Contains(contenido, atrSeccion) {
		return nil, true
	}
	cuerpo := &html.Node{Type: html.ElementNode, DataAtom: atom.Body, Data: "body"}
	nodos, err := html.ParseFragment(strings.NewReader(contenido), cuerpo)
	if err != nil {
		return nil, false
	}
	ok = true
	vistos := map[string]bool{}
	var recorre func(n *html.Node)
	recorre = func(n *html.Node) {
		if n.Type == html.ElementNode {
			if id, hay := valorAtr(n, atrSeccion); hay {
				if !reIDSeccion.MatchString(id) || vistos[id] {
					ok = false
				}
				vistos[id] = true
				s := seccion{ID: id, Puede: fichas(n, atrPuede), Grupos: fichas(n, atrGrupos)}
				s.Marco = marco(n)
				s.Huella = huella(n)
				out = append(out, s)
			}
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			recorre(c)
		}
	}
	for _, n := range nodos {
		recorre(n)
	}
	return out, ok
}

func valorAtr(n *html.Node, nombre string) (string, bool) {
	for _, a := range n.Attr {
		if a.Key == nombre {
			return a.Val, true
		}
	}
	return "", false
}

// fichas: los valores separados por espacios de un atributo, ordenados y sin repetir.
func fichas(n *html.Node, nombre string) []string {
	v, _ := valorAtr(n, nombre)
	f := strings.Fields(v)
	slices.Sort(f)
	return slices.Compact(f)
}

// marco: la etiqueta y todos los atributos del elemento, en orden canónico. Que un editor
// no pueda cambiarlos evita ocultar una sección (display:none) o cambiarle el aspecto.
func marco(n *html.Node) string {
	attrs := make([]string, 0, len(n.Attr))
	for _, a := range n.Attr {
		attrs = append(attrs, a.Key+"="+a.Val)
	}
	slices.Sort(attrs)
	return n.Data + "?" + strings.Join(attrs, "&")
}

// huella del CONTENIDO canónico de un nodo (sin su propia etiqueta).
func huella(n *html.Node) string {
	h := sha256.New()
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		_ = html.Render(h, c) // sha256.Write nunca falla
	}
	return hex.EncodeToString(h.Sum(nil))
}

// AplicarSecciones valida un lote contra las secciones protegidas del documento y
// devuelve el estado resultante, o ErrProtegido sin efecto alguno. Pura, como
// AplicarProteccion; Estado.Modelo guarda las huellas (JSON) o "" si no hay secciones.
func AplicarSecciones(est EstadoProteccion, ops []Op, esDueno bool, quien Subject) (ResultadoProteccion, error) {
	res := ResultadoProteccion{Ops: append([]Op(nil), ops...), Estado: est}
	for i := range res.Ops {
		op := res.Ops[i]
		if op.Kind != OpSet || op.Key != claveHTML || !ganaRegistro(res.Estado, op) {
			continue // lo que el registro descarta no cambia nada
		}
		var contenido string
		if !isNullJSON(op.Value) && json.Unmarshal(op.Value, &contenido) != nil {
			if !esDueno && res.Estado.Modelo != "" {
				return ResultadoProteccion{}, ErrProtegido // valor ilegible: no se puede comprobar
			}
			contenido = ""
		}
		nuevas, validas := seccionesDe(contenido)
		antes := leerHuellas(res.Estado.Modelo)
		if !esDueno && !seccionesPermitidas(antes, nuevas, validas, quien) {
			return ResultadoProteccion{}, ErrProtegido
		}
		res.Estado.HayRegistro, res.Estado.Lamport, res.Estado.Dot = true, op.Lamport, op.Dot
		res.Estado.Modelo = ""
		if len(nuevas) > 0 {
			b, _ := json.Marshal(nuevas)
			res.Estado.Modelo = string(b)
		}
	}
	// Las operaciones por carácter de la secuencia RGA, contra las mismas secciones.
	if err := secuenciaDelLote(&res, est, ops, esDueno, quien); err != nil {
		return ResultadoProteccion{}, err
	}
	return res, nil
}

func leerHuellas(modelo string) []seccion {
	var out []seccion
	if modelo != "" {
		_ = json.Unmarshal([]byte(modelo), &out)
	}
	return out
}

// seccionesPermitidas: ¿puede `quien`, que no es propietario, pasar de `antes` a `nuevas`?
func seccionesPermitidas(antes, nuevas []seccion, validas bool, quien Subject) bool {
	if len(antes) == 0 && len(nuevas) == 0 {
		return true // lo habitual: un documento sin secciones, nada que mirar
	}
	if !validas || len(antes) != len(nuevas) {
		return false
	}
	porID := make(map[string]seccion, len(nuevas))
	for _, n := range nuevas {
		porID[n.ID] = n
	}
	for _, a := range antes {
		n, hay := porID[a.ID]
		if !hay || n.marca() != a.marca() {
			return false // creada, quitada o con otro marcado
		}
		if n.Huella != a.Huella && !a.permite(quien) {
			return false
		}
	}
	return true
}

// EstadoInicialSecciones deduce el estado de un documento de texto que ya existe.
func EstadoInicialSecciones(st *State) (EstadoProteccion, error) {
	est := EstadoProteccion{Claves: map[string]ClaveHash{}}
	r, ok := st.Registers[claveHTML]
	if !ok {
		return est, nil
	}
	est.HayRegistro, est.Lamport, est.Dot = true, r.Lamport, r.Dot
	var contenido string
	// Un valor ilegible, o un documento heredado con marcado inválido, queda sin secciones:
	// lo que había antes de esta función no se podía proteger, y no se bloquea a nadie por ello.
	if err := json.Unmarshal(r.Value, &contenido); err == nil {
		if secs, ok := seccionesDe(contenido); ok && len(secs) > 0 {
			b, _ := json.Marshal(secs)
			est.Modelo = string(b)
			est.Secuencia = CopiaSecuencia(st) // el texto por carácter se valida contra estas secciones
		}
	}
	return est, nil
}
