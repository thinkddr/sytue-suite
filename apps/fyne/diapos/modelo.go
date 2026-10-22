// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

// Package diapos es Sytue Diapos —las presentaciones— en Fyne: el lienzo de
// diapositiva, el carril de miniaturas, el modo presentación y las notas del ponente.
//
// # El documento de la gente no se corrompe
//
// El contenido de una presentación es UN registro LWW, `doc:slides`, con el
// `JSON.stringify(SlidesModel)` de la web dentro (`editor/types.ts`, CONTENT_KEY). El
// servicio Go lo trata como un blob opaco: nadie más lo valida. Así que el único
// guardián del documento es el cliente que lo abre.
//
// Por eso este paquete hace DOS cosas distintas y no las mezcla:
//
//  1. `Leer` obtiene la vista tipada que hace falta para PINTAR.
//  2. `Fiel` responde si ese modelo tipado, vuelto a serializar, reproduce el JSON de
//     origen ENTERO. Si no —un campo que esta versión no conoce, una forma nueva, un
//     incrustado que llegó después—, el documento se enseña en SOLO LECTURA y se dice
//     por qué. No hay tercer camino: guardar un modelo que no reproduce su origen es
//     borrar el trabajo de alguien.
//
// `Fiel` es literalmente la definición del viaje de ida y vuelta, no un proxy de ella:
// serializa y compara los ÁRBOLES de valores, no los bytes (el orden de las claves y
// `1` contra `1.0` son del codificador, no del documento).
//
// Hoy este paquete no escribe nunca —es un visor—, así que la pregunta que `Fiel`
// contesta es la del cartel: «entiendo esto entero» o «hay cosas que no pinto». El día
// que se añada edición, es la puerta que hay que cruzar antes de habilitar Guardar.
//
//nolint:tagliatelle // las claves son las de `SlidesModel` en la web: renombrarlas rompe el documento
package diapos

import (
	"encoding/json"
	"fmt"
	"reflect"
	"slices"
	"strings"
)

// STAGE_W de SlidesCanvas.tsx: el ancho del sistema de coordenadas interno del
// escenario. Las cajas van en % del lienzo, así que este número no es un tamaño en
// pantalla: es la unidad en la que la web declara los tamaños de letra de dentro
// (`h1` 44 px, `p` 20 px…). El escenario entero se escala a lo que haya de hueco.
const AnchoEscenario float32 = 960

// Proporciones de los dos presets, y el rango en el que se acota una personalizada
// (MIN_ASPECT/MAX_ASPECT de SlidesCanvas.tsx).
const (
	proporcion169 = 16.0 / 9.0
	proporcion43  = 4.0 / 3.0
	proporcionMin = 0.2
	proporcionMax = 5
)

// Modelo es `SlidesModel`.
type Modelo struct {
	Diapos []Diapo `json:"slides"`
	// Tema de TODA la presentación: un preset o el id de un tema propio (`Temas`); ausente
	// = Papel.
	Tema string `json:"theme,omitempty"`
	// Temas propios (`SlideCustomTheme`): colores y fuente que fija quien hace la
	// presentación.
	Temas []TemaPropio `json:"themes,omitempty"`
	// Patrón: cajas comunes que se pintan DETRÁS de las de cada diapositiva.
	Patron *Patron `json:"master,omitempty"`
	// Tamaño (proporción) del lienzo; ausente = 16:9.
	Tamano *Tamano `json:"size,omitempty"`
}

// Patron es `SlideMaster`.
type Patron struct {
	Cajas []Caja `json:"boxes"`
	// Diseños de la presentación (`SlideLayout`): aquí solo se conservan, no se editan.
	Disenos []Diseno `json:"layouts,omitempty"`
}

// Diseno es `SlideLayout`: un nombre y los marcadores de posición —cajas de texto con su
// muestra— de los que parte una diapositiva; y, de un .pptx o un .odp importado, sus formas
// propias (Decor) y sus líneas, que se pintan debajo de cada diapositiva que lo lleva y
// encima del patrón, y si oculta el patrón (SinPatron).
type Diseno struct {
	ID         string     `json:"id"`
	Nombre     string     `json:"name"`
	Cajas      []Caja     `json:"boxes"`
	Decor      []Caja     `json:"decor,omitempty"`
	Conectores []Conector `json:"connectors,omitempty"`
	SinPatron  bool       `json:"hideMaster,omitempty"`
}

// TemaPropio es `SlideCustomTheme`: fondo y tinta (#RRGGBB) y, si la fija, la fuente.
type TemaPropio struct {
	ID     string `json:"id"`
	Nombre string `json:"name"`
	Fondo  string `json:"bg"`
	Tinta  string `json:"ink"`
	Fuente string `json:"font,omitempty"`
}

// Tamano es `SlideSize`: preset y, solo en 'custom', ancho y alto cuyo cociente da la
// proporción.
type Tamano struct {
	Preset string   `json:"preset"`
	Ancho  *float64 `json:"w,omitempty"`
	Alto   *float64 `json:"h,omitempty"`
	Unidad string   `json:"unit,omitempty"`
}

// Diapo es `Slide`.
type Diapo struct {
	ID         string     `json:"id"`
	Cajas      []Caja     `json:"boxes"`
	Conectores []Conector `json:"connectors,omitempty"`
	Transicion string     `json:"transition,omitempty"`
	Notas      string     `json:"notes,omitempty"`
	Fondo      string     `json:"bg,omitempty"`
	SinPatron  *bool      `json:"hideMaster,omitempty"`
	// Id del diseño (de `Patron.Disenos`) con el que está hecha.
	Diseno string `json:"layout,omitempty"`
}

// Caja es `SlideBox`: x/y/w/h en % del lienzo (0..100).
//
// Los booleanos opcionales van por PUNTERO a propósito. La web solo persiste el valor
// que no es el de por defecto (`controls` solo cuando es `false`, `autoplay` solo
// cuando es `true`), pero un documento que traiga el otro explícitamente existe, y con
// un `bool` + `omitempty` se perdería al volver a serializar: `Fiel` lo marcaría, y el
// documento se quedaría en solo lectura sin motivo real.
type Caja struct {
	ID string  `json:"id"`
	X  float64 `json:"x"`
	Y  float64 `json:"y"`
	W  float64 `json:"w"`
	H  float64 `json:"h"`
	// Ausente o "text" = caja de texto.
	Tipo string `json:"kind,omitempty"`
	// HTML del texto; "" en imagen/forma/gráfico/diagrama/medio. NO lleva omitempty:
	// la web lo emite siempre, también vacío.
	HTML      string `json:"html"`
	Src       string `json:"src,omitempty"`
	TipoMedio string `json:"mediaType,omitempty"`
	Cartel    string `json:"poster,omitempty"`
	AutoPlay  *bool  `json:"autoplay,omitempty"`
	Bucle     *bool  `json:"loop,omitempty"`
	Controles *bool  `json:"controls,omitempty"`
	// Subtítulos WebVTT del medio (`SlideTrack`): aquí solo se conservan, no se pintan.
	Pistas   []Pista   `json:"tracks,omitempty"`
	Forma    string    `json:"shape,omitempty"`
	Relleno  string    `json:"fill,omitempty"`
	Trazo    string    `json:"stroke,omitempty"`
	Grafico  *Grafico  `json:"chart,omitempty"`
	Diagrama *Diagrama `json:"smartart,omitempty"`
	Anims    []Anim    `json:"anims,omitempty"`
	// Modelo ANTIGUO: UNA animación de entrada. Se conserva para no perderlo al
	// releer un documento viejo.
	Anim *Anim `json:"anim,omitempty"`
	// Destino de un enlace de acción: 'first'|'last'|'next'|'prev' o un número. Se
	// guarda crudo porque es una unión de tipos y aquí solo hay que conservarla.
	Accion json.RawMessage `json:"action,omitempty"`
	Grupo  string          `json:"groupId,omitempty"`
}

// Pista es `SlideTrack`: un subtítulo WebVTT de un vídeo o un audio, con su idioma (BCP 47)
// y si es el de por defecto. `default` va por puntero por lo mismo que los booleanos de
// Caja: la web solo persiste el `true`, pero un documento con `false` explícito existe.
type Pista struct {
	Idioma   string `json:"lang"`
	Etiqueta string `json:"label,omitempty"`
	VTT      string `json:"vtt"`
	Defecto  *bool  `json:"default,omitempty"`
}

// Conector es `SlideConnector`.
type Conector struct {
	ID          string  `json:"id"`
	Trazado     string  `json:"kind"`
	Desde       Extremo `json:"from"`
	Hasta       Extremo `json:"to"`
	PuntaInicio *bool   `json:"startArrow,omitempty"`
	PuntaFin    *bool   `json:"endArrow,omitempty"`
	Trazo       string  `json:"stroke,omitempty"`
}

// Extremo es `ConnectorEnd`: anclado a una caja o punto libre en % del lienzo.
type Extremo struct {
	Caja    string   `json:"boxId,omitempty"`
	Anclaje string   `json:"anchor,omitempty"`
	X       *float64 `json:"x,omitempty"`
	Y       *float64 `json:"y,omitempty"`
}

// Anim es `SlideAnim`.
type Anim struct {
	Tipo    string  `json:"type"`
	Orden   float64 `json:"order"`
	Disparo string  `json:"trigger,omitempty"`
	Puntos  []Punto `json:"points,omitempty"`
}

// Punto es `MotionPoint`: desfase en puntos porcentuales del lienzo.
type Punto struct {
	DX float64 `json:"dx"`
	DY float64 `json:"dy"`
}

// Diagrama es `SlideSmartArt`.
type Diagrama struct {
	Disposicion string     `json:"layout"`
	Elementos   []Elemento `json:"items"`
}

// Elemento es `SmartArtItem`.
type Elemento struct {
	Texto string   `json:"text"`
	Nivel *float64 `json:"level,omitempty"`
}

// Grafico es `SlideChart`: tipo y rejilla de texto crudo por filas.
type Grafico struct {
	Tipo    string     `json:"type"`
	Rejilla [][]string `json:"grid"`
}

// Leer decodifica el contenido del registro `doc:slides`.
//
// No sanea nada ni rellena valores por defecto: eso es trabajo del que pinta
// (Proporcion, CajasResueltas), y hacerlo aquí sería exactamente lo que rompe el viaje
// de ida y vuelta.
func Leer(crudo []byte) (*Modelo, error) {
	var m Modelo
	if err := json.Unmarshal(crudo, &m); err != nil {
		return nil, fmt.Errorf("diapos: doc:slides no es un modelo de presentación: %w", err)
	}
	return &m, nil
}

// Fiel dice si volver a serializar este modelo reproduce el JSON de origen entero.
//
// Compara los ÁRBOLES de valores y no los bytes: el orden de las claves de un objeto,
// los espacios y si un entero se escribió `1` o `1.0` son decisiones del codificador,
// no del documento, y exigir igualdad de bytes daría rojo por ellas. Lo que sí detecta
// —que es lo único que importa— es que FALTE algo: una clave que esta versión no
// conoce, un campo opcional que se comió un `omitempty`, un elemento perdido.
func (m *Modelo) Fiel(crudo []byte) bool {
	vuelta, err := json.Marshal(m)
	if err != nil {
		return false
	}
	var antes, despues any
	if json.Unmarshal(crudo, &antes) != nil || json.Unmarshal(vuelta, &despues) != nil {
		return false
	}
	return reflect.DeepEqual(antes, despues)
}

// Proporcion es el ancho/alto efectivo del lienzo, acotado al rango sano. Es
// `slideAspect` de la web, con sus mismos topes.
func (m *Modelo) Proporcion() float32 {
	t := m.Tamano
	switch {
	case t == nil:
		return proporcion169
	case t.Preset == "4:3":
		return proporcion43
	case t.Preset == "custom" && t.Ancho != nil && t.Alto != nil && *t.Alto != 0:
		p := float32(*t.Ancho / *t.Alto)
		return min(max(p, proporcionMin), proporcionMax)
	}
	return proporcion169
}

// AltoEscenario es el alto del escenario en las coordenadas internas: 16:9 → 540,
// 4:3 → 720. Es `stageHeight`.
func (m *Modelo) AltoEscenario() float32 { return AnchoEscenario / m.Proporcion() }

// CajasResueltas son las cajas que se pintan en la diapositiva `i`: primero las del
// patrón (con sus campos {n} y {total} sustituidos) y encima las suyas. Es
// `resolveSlideBoxes`.
func (m *Modelo) CajasResueltas(i int) []Caja {
	if i < 0 || i >= len(m.Diapos) {
		return nil
	}
	d := m.Diapos[i]
	patron := m.cajasPatron(i)
	fuera := make([]Caja, 0, len(patron)+len(d.Cajas))
	return append(append(fuera, patron...), d.Cajas...)
}

// cajasPatron es `masterBoxesFor`: las del patrón (salvo que su diseño lo oculte) y, encima,
// las formas de su diseño; nada si la diapositiva oculta el patrón, que oculta las dos.
func (m *Modelo) cajasPatron(i int) []Caja {
	if m.Patron == nil {
		return nil
	}
	if d := m.Diapos[i]; d.SinPatron != nil && *d.SinPatron {
		return nil
	}
	diseno := m.disenoDe(i)
	var cajas []Caja
	if diseno == nil || !diseno.SinPatron {
		cajas = m.Patron.Cajas
	}
	if diseno != nil {
		cajas = append(slices.Clone(cajas), diseno.Decor...)
	}
	fuera := make([]Caja, len(cajas))
	for j, c := range cajas {
		c.HTML = SustituirCampos(c.HTML, i+1, len(m.Diapos))
		fuera[j] = c
	}
	return fuera
}

// disenoDe es el diseño de la diapositiva `i`, si lo lleva y existe.
func (m *Modelo) disenoDe(i int) *Diseno {
	if m.Patron == nil || m.Diapos[i].Diseno == "" {
		return nil
	}
	for j := range m.Patron.Disenos {
		if m.Patron.Disenos[j].ID == m.Diapos[i].Diseno {
			return &m.Patron.Disenos[j]
		}
	}
	return nil
}

// ConectoresDiseno son las líneas de las formas del diseño de la diapositiva `i` (con los
// extremos sueltos), que se pintan con el patrón: `layoutConnectorsFor`.
func (m *Modelo) ConectoresDiseno(i int) []Conector {
	if i < 0 || i >= len(m.Diapos) {
		return nil
	}
	if d := m.Diapos[i]; d.SinPatron != nil && *d.SinPatron {
		return nil
	}
	if diseno := m.disenoDe(i); diseno != nil {
		return diseno.Conectores
	}
	return nil
}

// SustituirCampos cambia {n} por el número de diapositiva y {total} por el total. Es
// `substituteMasterFields`.
func SustituirCampos(html string, n, total int) string {
	return strings.NewReplacer(
		"{n}", fmt.Sprint(n),
		"{total}", fmt.Sprint(total),
	).Replace(html)
}
