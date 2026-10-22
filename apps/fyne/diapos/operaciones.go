// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package diapos

import (
	"bytes"
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
)

// # Lo que se le puede hacer al modelo
//
// Son las operaciones de `SlidesCanvas.tsx` que tocan el DOCUMENTO —añadir, duplicar,
// borrar, reordenar, cambiar tema, fondo, transición, notas, tamaño—, escritas sobre el
// modelo y sin pintar nada. Cada una deja el JSON exactamente como lo deja la web
// («sin tema = sin campo», «notas vacías = sin campo»…): el mismo documento lo abren
// las dos, y un campo vacío que una escribe y la otra no es un documento que cambia
// solo de un guardado a otro.
//
// La web las hace inmutables porque React compara identidades; aquí se muta el modelo
// y quien pinta se entera por su `Refresh`.

// MaxCaracteres es MODEL_MAX_CHARS: el tope del modelo serializado. El cuerpo que
// acepta el servicio corta en 1 MiB y por encima el guardado se pierde ENTERO, así que
// una operación que lo supere no se aplica.
const MaxCaracteres = 900_000

// ErrDemasiadoGrande: la operación dejaría el modelo por encima de MaxCaracteres.
var ErrDemasiadoGrande = errors.New("diapos: la presentación no cabría en un guardado")

// Disposición de una diapositiva nueva (SLIDE_LAYOUTS).
const (
	DispTitulo       = "titulo"
	DispTituloCuerpo = "titulo-cuerpo"
	DispDosColumnas  = "dos-columnas"
)

// Disposiciones son las tres plantillas, con su clave de catálogo, en el orden del
// selector de la web.
var Disposiciones = []struct{ ID, Clave string }{
	{DispTitulo, "slides.layout.title"},
	{DispTituloCuerpo, "slides.layout.titleBody"},
	{DispDosColumnas, "slides.layout.twoColumns"},
}

// uuid es `crypto.randomUUID()`: un v4 en minúsculas con guiones. Es el identificador
// que la web da a cajas y diapositivas, y los dos clientes escriben en el mismo
// documento.
func uuid() string {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		panic("diapos: sin azar del sistema: " + err.Error()) // crypto/rand no falla en los cinco destinos
	}
	b[6] = b[6]&0x0f | 0x40
	b[8] = b[8]&0x3f | 0x80
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:16])
}

func nuevaCaja(x, y, w, h float64, html string) Caja {
	return Caja{ID: uuid(), X: x, Y: y, W: w, H: h, HTML: html}
}

// cajasDe es `layoutBoxes`: las cajas de texto de una plantilla, con el texto de
// ejemplo en el idioma de la sesión. El texto va sin escapar, como en la web: es
// nuestro catálogo, no lo que teclea nadie.
func cajasDe(disp string, t func(string) string) []Caja {
	titulo := "<h1>" + t("slides.placeholder.title") + "</h1>"
	cuerpo := "<p>" + t("slides.placeholder.body") + "</p>"
	switch disp {
	case DispTituloCuerpo:
		return []Caja{nuevaCaja(8, 8, 84, 16, titulo), nuevaCaja(8, 30, 84, 60, cuerpo)}
	case DispDosColumnas:
		return []Caja{
			nuevaCaja(8, 8, 84, 16, titulo),
			nuevaCaja(8, 30, 40, 60, cuerpo),
			nuevaCaja(52, 30, 40, 60, cuerpo),
		}
	}
	return []Caja{nuevaCaja(10, 35, 80, 20, titulo)} // igual que el modelo vacío
}

// NuevaDiapo es `newSlide`.
func NuevaDiapo(disp string, t func(string) string) Diapo {
	return Diapo{ID: uuid(), Cajas: cajasDe(disp, t)}
}

// ModeloVacio es `emptySlidesModel`: una diapositiva de título.
func ModeloVacio(t func(string) string) *Modelo {
	return &Modelo{Diapos: []Diapo{NuevaDiapo(DispTitulo, t)}}
}

// Serializar es `JSON.stringify(model)` para guardar. Sin el escapado de HTML de
// `json.Marshal`: cada `<` del texto de una caja serían seis bytes, y el guardado tiene
// tope (MaxCaracteres).
func (m *Modelo) Serializar() (string, error) {
	var b bytes.Buffer
	enc := json.NewEncoder(&b)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(m); err != nil {
		return "", err
	}
	return string(bytes.TrimRight(b.Bytes(), "\n")), nil
}

func (m *Modelo) cabe() bool {
	s, err := m.Serializar()
	return err == nil && len(s) <= MaxCaracteres
}

func (m *Modelo) valida(i int) bool { return i >= 0 && i < len(m.Diapos) }

// InsertarTras mete una diapositiva DETRÁS de la `i` (o la primera, con i = −1) y
// devuelve su índice. Es `addSlide`, que nace tras la activa.
func (m *Modelo) InsertarTras(i int, d Diapo) int {
	pos := min(max(i+1, 0), len(m.Diapos))
	m.Diapos = slices.Insert(m.Diapos, pos, d)
	return pos
}

// Duplicar es `duplicateSlide`: copia la diapositiva `i` detrás de ella con
// identificadores nuevos para ella y sus cajas, y los conectores remapeados a esas
// cajas —si no, apuntarían a las de la original—. Conserva la transición, las notas y
// cualquier campo que la diapositiva traiga. No se aplica si no cabría en un guardado.
func (m *Modelo) Duplicar(i int) (int, error) {
	if !m.valida(i) {
		return i, nil
	}
	src := m.Diapos[i]
	copia := src
	copia.ID = uuid()
	ids := make(map[string]string, len(src.Cajas))
	copia.Cajas = make([]Caja, len(src.Cajas))
	for k, c := range src.Cajas {
		nuevo := uuid()
		ids[c.ID] = nuevo
		c.ID = nuevo
		copia.Cajas[k] = c
	}
	if len(src.Conectores) > 0 {
		copia.Conectores = make([]Conector, len(src.Conectores))
		for k, c := range src.Conectores {
			c.Desde, c.Hasta = remapear(c.Desde, ids), remapear(c.Hasta, ids)
			copia.Conectores[k] = c
		}
	}
	antes := m.Diapos
	pos := m.InsertarTras(i, copia)
	if !m.cabe() {
		m.Diapos = antes
		return i, ErrDemasiadoGrande
	}
	return pos, nil
}

// remapear es `remapConnectorIds` para un extremo.
func remapear(e Extremo, ids map[string]string) Extremo {
	if nuevo, ok := ids[e.Caja]; ok && e.Caja != "" {
		e.Caja = nuevo
	}
	return e
}

// Eliminar quita la diapositiva `i`. Siempre queda al menos una (§7.2): con una sola
// no hace nada y devuelve false.
func (m *Modelo) Eliminar(i int) bool {
	if len(m.Diapos) <= 1 || !m.valida(i) {
		return false
	}
	m.Diapos = slices.Delete(m.Diapos, i, i+1)
	return true
}

// Mover es `reorderSlides`: lleva la diapositiva `desde` a la posición `hasta`.
// Devuelve si ha cambiado algo.
func (m *Modelo) Mover(desde, hasta int) bool {
	if !m.valida(desde) || !m.valida(hasta) || desde == hasta {
		return false
	}
	d := m.Diapos[desde]
	m.Diapos = slices.Delete(m.Diapos, desde, desde+1)
	m.Diapos = slices.Insert(m.Diapos, hasta, d)
	return true
}

// AplicarDisposicion es `applyLayout` con `mergeLayoutBoxes`: el TEXTO entra por orden
// en las cajas de la plantilla, y lo que no es texto (imagen, forma, gráfico…) se
// conserva tal cual detrás. Cambiar la disposición no puede destruir una imagen.
func (m *Modelo) AplicarDisposicion(i int, disp string, t func(string) string) {
	if !m.valida(i) {
		return
	}
	var textos, resto []Caja
	for _, c := range m.Diapos[i].Cajas {
		if c.Tipo == "" || c.Tipo == "text" {
			textos = append(textos, c)
		} else {
			resto = append(resto, c)
		}
	}
	nuevas := cajasDe(disp, t)
	for k := range nuevas {
		if k < len(textos) {
			nuevas[k].HTML = textos[k].HTML
		}
	}
	// El reflujo puede tirar cajas de texto animadas: se renumera como la web.
	m.Diapos[i].Cajas = NormalizarOrden(append(nuevas, resto...))
}

// temasValidos son los `id` de SLIDE_THEMES.
var temasValidos = []string{"carbon", "pizarra", "arena", "menta", "cielo"}

// PonerTema es `setTheme`: un tema válido o, con cualquier otra cosa, Papel (sin campo).
func (m *Modelo) PonerTema(id string) {
	if _, propio := m.temaPropio(id); propio || slices.Contains(temasValidos, id) {
		m.Tema = id
		return
	}
	m.Tema = ""
}

// PonerFondo es `setSlideBg`: 'transparent' = el del tema (sin campo).
func (m *Modelo) PonerFondo(i int, color string) {
	if !m.valida(i) {
		return
	}
	if color == "transparent" {
		color = ""
	}
	m.Diapos[i].Fondo = color
}

// PonerTransicion es `setTransition`. La web escribe también 'none' explícito cuando
// se elige, así que aquí igual: no es lo mismo que no haber elegido nunca.
func (m *Modelo) PonerTransicion(i int, tr string) {
	if !m.valida(i) {
		return
	}
	switch tr {
	case "fade", "slide", "morph":
	default:
		tr = "none"
	}
	m.Diapos[i].Transicion = tr
}

// PonerNotas es `setNotes`: vacías = sin campo.
func (m *Modelo) PonerNotas(i int, texto string) {
	if m.valida(i) {
		m.Diapos[i].Notas = texto
	}
}

// AlternarPatron es `toggleHideMaster`: enseña u oculta el patrón en esa diapositiva.
// Quitarlo borra el campo; ponerlo escribe `true`.
func (m *Modelo) AlternarPatron(i int) {
	if !m.valida(i) {
		return
	}
	if d := &m.Diapos[i]; d.SinPatron != nil && *d.SinPatron {
		d.SinPatron = nil
	} else {
		si := true
		d.SinPatron = &si
	}
}

// PonerTamano es `setSlideSize` con `sanitizeSlideSize`: 16:9 (o cualquier cosa no
// válida) = sin campo.
func (m *Modelo) PonerTamano(t *Tamano) { m.Tamano = sanearTamano(t) }

func sanearTamano(t *Tamano) *Tamano {
	switch {
	case t == nil:
		return nil
	case t.Preset == "4:3":
		return &Tamano{Preset: "4:3"}
	case t.Preset == "custom" && t.Ancho != nil && t.Alto != nil && *t.Ancho > 0 && *t.Alto > 0:
		unidad := "px"
		if t.Unidad == "cm" {
			unidad = "cm"
		}
		w, h := *t.Ancho, *t.Alto
		return &Tamano{Preset: "custom", Ancho: &w, Alto: &h, Unidad: unidad}
	}
	return nil
}

// MaxCajasBloque es el tope de cajas de un bloque (`MAX_CAJAS_BLOQUE`).
const MaxCajasBloque = 200

// DiapoDeBloque es `diapositivaDeBloque`: la diapositiva de un bloque reutilizable, con
// identificadores nuevos para ella y sus cajas. false si el contenido no es un bloque: sin
// cajas, con demasiadas o con ninguna que tenga su sitio (x, y, w, h).
func DiapoDeBloque(crudo string) (Diapo, bool) {
	var b struct {
		Boxes []json.RawMessage `json:"boxes"`
		Notes string            `json:"notes"`
	}
	if json.Unmarshal([]byte(crudo), &b) != nil || len(b.Boxes) > MaxCajasBloque {
		return Diapo{}, false
	}
	d := Diapo{ID: uuid(), Notas: b.Notes}
	for _, cr := range b.Boxes {
		var sitio struct{ X, Y, W, H *float64 }
		var c Caja
		if json.Unmarshal(cr, &sitio) != nil || sitio.X == nil || sitio.Y == nil || sitio.W == nil || sitio.H == nil ||
			json.Unmarshal(cr, &c) != nil {
			continue
		}
		c.ID = uuid()
		d.Cajas = append(d.Cajas, c)
	}
	return d, len(d.Cajas) > 0
}
