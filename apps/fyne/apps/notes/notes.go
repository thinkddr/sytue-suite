// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

// Package notes es Sytue Notepads en Fyne: el árbol de cuadernos, el lienzo de notas,
// la búsqueda y la papelera, con los widgets propios de `apps/fyne/ui` y sin una sola
// medida inventada.
//
// # Qué es la referencia
//
// `apps/frontend/apps/notes/src/components/**`, y los números salen del barrido de
// producción (`test/e2e/medidas-web.mjs notes`, 1280×800, es-ES, DSF 1,
// corporate.sytue.eu v3.3.9, 2026-09-21). Cada constante lleva de dónde salió: «medido»
// es del volcado del DOM, «hoja» es la clase de Tailwind convertida con el rem de 14.
//
// # EL FORMATO NATIVO ES HTML, y los documentos no se corrompen
//
// Una nota se guarda en HTML: lo demás —md, txt, onex, pdf— son SALIDAS hacia fuera y
// no formatos de trabajo. Esta vista lee ese HTML con `documento.go` y aplica el
// contrato que ya usan Calcus y Diapos, con sus dos preguntas separadas:
//
//   - `Fiel()`  — ¿el viaje de ida y vuelta reproduce el original entero?
//   - `NoPintado()` — ¿qué trae que esta vista no sabe dibujar?
//
// Pueden discrepar, y por eso son dos: una tabla se reescribe byte a byte y aun así no
// se pinta con rejilla; un atributo que la web añada mañana no se conserva aunque no
// haya nada nuevo que dibujar. **Guardar solo se ofrece cuando `Fiel()` dice que sí.**
// Si no, la nota se abre en SOLO LECTURA y se dice por qué, con el mismo cartel que
// Diapos.
//
// El texto plano de una nota —el resumen de una lista, el cuerpo de la búsqueda— sale
// de `apps/nucleo/htmltext`, que es de donde sale en todas las apps.
//
// # La app es SUYA
//
// Un binario por producto, como manda `docs/CONVENTIONS.md` §13.6. «Compartir en una
// sala» o «abrir el evento en Calendar» no son pantallas de este proceso: son una
// llamada al sistema por el esquema de URL de la otra app.
//
// # Sin conexión
//
// Se lee lo ya cargado y TODA mutación se desactiva —crear, editar, mover, etiquetar,
// papelera—, que es el contrato de `docs/roadmap.md` §«Contrato offline de las apps
// nativas». No se encola nada: una edición que se reintenta sola sobre una nota que
// mientras tanto cambió en otro sitio es como se pierde texto.
package notes

import (
	"encoding/json"
	"errors"
	"slices"
	"strings"
	"time"

	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/buscar"
	"git.sytue.eu/sytue/one/apps/nucleo/htmltext"
	"git.sytue.eu/sytue/one/apps/nucleo/i18n"
)

// paridad-web: apps/frontend/apps/notes/src/pages/index.astro

// Fase es en cuál de los estados está una pantalla: la de `api`, con alias para que
// las firmas de este paquete se lean sin `api.` delante.
//
// Estaba escrita aquí entera, y también en `notes` —idéntica hasta la coma— y en `erp`.
// La cuenta que la produce mira códigos HTTP, así que vive donde viven los códigos
// (`docs/CONVENTIONS.md` §13.2).
type Fase = api.Fase

// Las fases, con el cero en «cargando»: una pantalla recién construida todavía no ha
// pedido nada, y ese es el estado honesto.
const (
	Cargando    = api.FaseCargando
	Exito       = api.FaseExito
	Vacia       = api.FaseVacia
	Fallo       = api.FaseFallo
	SinSesion   = api.FaseSinSesion
	Prohibida   = api.FaseProhibida
	SinConexion = api.FaseSinConexion
)

// FaseDe traduce el resultado de una carga a la fase que toca pintar. Es `api.FaseDe`:
// el orden de las comprobaciones es el de la web y se decide una sola vez.
var FaseDe = api.FaseDe

// Vista es cuál de las tres del panel principal se está mirando (`type Vista` de
// NotesApp.tsx).
type Vista uint8

// Las cuatro vistas, con el cuaderno de primera porque es la de arranque.
const (
	VistaCuaderno Vista = iota
	VistaBusqueda
	VistaPapelera
	// VistaCompartidas es «Compartido conmigo»: las notas sueltas que me han dado.
	VistaCompartidas
	// VistaTareas son las tareas de todos los cuadernos (tareas.go).
	VistaTareas
	// VistaFijadas y VistaArchivo son las de archivo.go.
	VistaFijadas
	VistaArchivo
)

// Disposicion es el `layout` de un cuaderno: dónde se pinta cada nota.
type Disposicion uint8

// Las cuatro de `layout.*` en el catálogo de la web.
const (
	// Lista es el valor por defecto y también el que se usa para pintar UNA nota a
	// todo el ancho en la vista de árbol.
	Lista Disposicion = iota
	Columnas
	Libre
	VistaArbol
)

// Columnas de la disposición en columnas: el `COLUMNAS` de NoteCanvas.tsx.
const NumColumnas = 3

// DisposicionDe traduce el `layout` que manda el servicio. Lo que no reconozca cae en
// lista, que es lo que hace la web con su `default`.
func DisposicionDe(s string) Disposicion {
	switch s {
	case "columns":
		return Columnas
	case "free":
		return Libre
	case "tree":
		return VistaArbol
	}
	return Lista
}

// Clave es el `layout` tal como lo escribe el servicio.
func (d Disposicion) Clave() string {
	switch d {
	case Columnas:
		return "columns"
	case Libre:
		return "free"
	case VistaArbol:
		return "tree"
	case Lista:
	}
	return "list"
}

// ClaveI18n es la clave de catálogo de su nombre (`layout.list`, …).
func (d Disposicion) ClaveI18n() string { return "layout." + d.Clave() }

// Cuaderno es lo que la vista necesita de un `api.Notebook`, con lo derivado ya hecho.
type Cuaderno struct {
	ID       string
	Padre    string
	Titulo   string
	Emoji    string
	Color    string
	Vista    Disposicion
	Publico  string
	Cifrado  bool
	Notas    int
	Posicion int
	// Archivado: está en el Archivo, fuera del árbol (archivo.go).
	Archivado bool
	// Crudo es el cuaderno tal como llegó: los ajustes se guardan desde aquí
	// (`Crudo.Input()`), que es lo que conserva su cifrado —ver `api.NotebookInput`—.
	Crudo api.Notebook
}

// DeCuaderno convierte uno del cliente.
func DeCuaderno(n api.Notebook) Cuaderno {
	c := Cuaderno{
		ID: n.ID, Titulo: n.Title, Emoji: n.Emoji, Color: n.Color,
		Vista: DisposicionDe(n.Layout), Publico: n.Visibility,
		Cifrado: n.Encrypted, Notas: n.NoteCount, Posicion: n.Position, Crudo: n,
		Archivado: n.ArchivedAt != nil,
	}
	if n.ParentID != nil {
		c.Padre = *n.ParentID
	}
	return c
}

// DeCuadernos convierte la lista entera conservando el orden del servicio.
func DeCuadernos(ns []api.Notebook) []Cuaderno {
	out := make([]Cuaderno, len(ns))
	for i, n := range ns {
		out[i] = DeCuaderno(n)
	}
	return out
}

// Nota es lo que la vista necesita de una `api.Note`.
//
// `Doc` se calcula UNA vez al convertir y no en cada pintado: interpretar el HTML de
// cuarenta notas en el `Layout` de la pantalla es cuarenta análisis por cada repintado,
// y de ahí salen las dos preguntas del contrato, que la barra necesita para decidir si
// deja escribir.
type Nota struct {
	ID        string
	Cuaderno  string
	Tipo      string
	Titulo    string
	Contenido string
	Datos     json.RawMessage
	// Cifrada dice que el cuerpo viaja en `content_cipher` y aquí no hay texto.
	Cifrada  bool
	Doc      *Documento
	Icono    string
	URL      string
	Grupo    string
	X, Y     int
	Col      int
	Posicion int
	Ancho    int
	Plegada  bool
	Marcada  bool
	// Cancelada: tarea tachada y fuera del traslado (`cancelled_at`).
	Cancelada bool
	// Traslados es cuántas veces ha pasado de un día al siguiente (`data.carry_count`).
	Traslados int
	Fijada    bool
	Estados   []api.NoteState
	Enlaces   []api.NoteLink
	Adjuntos  []Adjunto
	Tocada    time.Time
	Creada    time.Time
	// Crudo es la nota TAL COMO LLEGÓ del servicio. Toda escritura parte de aquí
	// (`Crudo.Input()`) y no de los campos de arriba: el PUT del servicio SUSTITUYE, y
	// armar el cuerpo con lo que la vista usa perdía `data`, los enlaces, la
	// visibilidad y el ancho de la nota —le pasaba a la casilla de una tarea—.
	Crudo api.Note
}

// DeNota convierte una del cliente y le interpreta el HTML.
func DeNota(n api.Note) Nota {
	o := Nota{
		ID: n.ID, Cuaderno: n.NotebookID, Tipo: n.Kind, Titulo: n.Title,
		Contenido: n.Content, Cifrada: n.ContentCipher != "",
		X: n.X, Y: n.Y, Col: n.Col, Posicion: n.Position, Ancho: n.Width,
		Plegada: n.Collapsed, Marcada: n.Checked, Fijada: n.Pinned,
		Estados: n.States, Enlaces: n.Links, Tocada: n.UpdatedAt, Creada: n.CreatedAt,
		Crudo: n, Datos: n.Data,
	}
	if n.GroupID != nil {
		o.Grupo = *n.GroupID
	}
	o.Icono, o.URL = datos(n.Data)
	o.Cancelada = n.CancelledAt != nil
	o.Traslados = trasladosDe(n.Data)
	o.Adjuntos = adjuntosDe(n.Data)
	o.Doc = Leer(o.Contenido)
	return o
}

// Terminada es una tarea hecha o cancelada: las dos se pintan apagadas.
func (n Nota) Terminada() bool { return n.Tipo == "todo" && (n.Marcada || n.Cancelada) }

// trasladosDe es `vecesTrasladada` de client/diario.ts: un entero positivo o 0.
func trasladosDe(crudo json.RawMessage) int {
	var m struct {
		N any `json:"carry_count"`
	}
	if len(crudo) == 0 || json.Unmarshal(crudo, &m) != nil {
		return 0
	}
	f, ok := m.N.(float64)
	if !ok || f < 1 || f != float64(int(f)) {
		return 0
	}
	return int(f)
}

// DeNotas convierte la lista entera.
func DeNotas(ns []api.Note) []Nota {
	out := make([]Nota, len(ns))
	for i, n := range ns {
		out[i] = DeNota(n)
	}
	return out
}

// datos saca de `data` los dos campos que la vista mira, sin confiar en su forma: es el
// `texto(data, clave)` de NoteCard.tsx. Un `data` que no sea un objeto no es un error,
// es una nota de un tipo que esta versión no conoce.
func datos(crudo json.RawMessage) (icono, url string) {
	if len(crudo) == 0 {
		return "", ""
	}
	var m map[string]any
	if json.Unmarshal(crudo, &m) != nil {
		return "", ""
	}
	cadena := func(k string) string {
		s, _ := m[k].(string)
		return s
	}
	return cadena("icon"), cadena("url")
}

// datoCadena es un campo de texto de `data`, vacío si no está o no es texto.
func datoCadena(crudo json.RawMessage, clave string) string {
	var m map[string]any
	if json.Unmarshal(crudo, &m) != nil {
		return ""
	}
	s, _ := m[clave].(string)
	return s
}

// Resumen es la primera línea de la nota para una lista: `extracto()` de NoteTree.tsx.
// Una nota cifrada y cerrada no enseña nada, que es lo que se pretende.
func (n Nota) Resumen(limite int) string {
	if n.Cifrada {
		return ""
	}
	return htmltext.Summary(n.Contenido, limite)
}

// Rotulo es el título que se pinta, o la clave de «nota nueva» si no tiene.
func (n Nota) Rotulo(tr *i18n.Translator) string {
	if n.Cifrada {
		return tr.T("crypto.locked", nil)
	}
	if n.Titulo != "" {
		return n.Titulo
	}
	return tr.T("note.untitled", nil)
}

// Editable dice si esta vista deja escribir en la nota.
//
// Son las TRES condiciones juntas, y la tercera es la que manda sobre las otras dos: la
// web deja editar texto y tareas (`note.kind === 'text' || 'todo'`), aquí además hace
// falta que el HTML vuelva a salir entero. `Doc.Fiel()` es lo que lo dice.
func (n Nota) Editable(soloLectura, bloqueado bool) bool {
	if soloLectura || bloqueado || n.Cifrada {
		return false
	}
	if n.Tipo != "text" && n.Tipo != "todo" {
		return false
	}
	return n.Doc != nil && n.Doc.Fiel()
}

// Orden es cómo se leen las notas de un cuaderno (`client/orden.ts`).
type Orden string

// Los cuatro de `ORDENES`, en el orden en que la web los ofrece.
const (
	OrdenManual Orden = "manual"
	OrdenTitulo Orden = "title"
	OrdenTocada Orden = "updated"
	OrdenCreada Orden = "created"
)

// Ordenes son los cuatro, para pintar el menú.
var Ordenes = []Orden{OrdenManual, OrdenTitulo, OrdenTocada, OrdenCreada}

// EsOrden dice si la cadena guardada es uno de los cuatro.
func EsOrden(s string) bool { return slices.Contains(Ordenes, Orden(s)) }

// ClaveI18n es la clave de catálogo de su nombre (`orden.manual`, …).
func (o Orden) ClaveI18n() string { return "orden." + string(o) }

// Ordenar devuelve las notas en el orden pedido, SIN tocar la lista de entrada.
//
// Las FIJADAS van siempre primero, en los cuatro órdenes: es lo que hace `ordenarNotas`
// y es lo que espera quien fija una nota —fijar es decir «esta primero»—. El orden
// «propio» no reordena nada más: la lista ya viene como la coloca el cuaderno.
//
// ponytail: el orden por título compara en minúsculas y sin acentos en vez de con un
// colador ICU (`localeCompare(..., {sensitivity:'base'})`). Da lo mismo en los alfabetos
// latinos de la suite y se separa en el sueco, donde «ä» va DETRÁS de «z». Cuando haya
// un colador en `apps/nucleo/i18n` —hoy no hay ninguno, lo he mirado—, esta función es
// la única que lo llama.
func Ordenar(notas []Nota, o Orden, _ i18n.Locale) []Nota {
	out := slices.Clone(notas)
	slices.SortStableFunc(out, func(a, b Nota) int {
		if a.Fijada != b.Fijada {
			if a.Fijada {
				return -1
			}
			return 1
		}
		switch o {
		case OrdenTitulo:
			// Sin título, al final: una nota vacía arriba del todo no dice nada.
			ta, tb := strings.TrimSpace(a.Titulo), strings.TrimSpace(b.Titulo)
			if (ta == "") != (tb == "") {
				if ta != "" {
					return -1
				}
				return 1
			}
			return strings.Compare(buscar.Plegar(ta), buscar.Plegar(tb))
		case OrdenTocada:
			return b.Tocada.Compare(a.Tocada)
		case OrdenCreada:
			return b.Creada.Compare(a.Creada)
		case OrdenManual:
		}
		return 0
	})
	return out
}

// Filtrar es el `notasVisibles` de NotesApp.tsx: el texto se busca en título y cuerpo,
// sin distinguir mayúsculas, y el estado de etiqueta es un filtro aparte.
func Filtrar(notas []Nota, texto, estado string) []Nota {
	q := buscar.Plegar(strings.TrimSpace(texto))
	var out []Nota
	for _, n := range notas {
		if estado != "" && !slices.ContainsFunc(n.Estados, func(s api.NoteState) bool {
			return s.StateID == estado
		}) {
			continue
		}
		if q != "" && !strings.Contains(buscar.Plegar(n.Titulo+" "+htmltext.Text(n.Contenido)), q) {
			continue
		}
		out = append(out, n)
	}
	return out
}

// Nodo es una nota con las que cuelgan de ella (`client/arbol.ts`).
type Nodo struct {
	Nota  Nota
	Hijos []Nodo
}

// Arbol agrupa las notas por `group_id` conservando el orden de entrada.
//
// Una nota cuyo padre NO está en la lista cuelga de la raíz en vez de desaparecer: es
// lo que hace `construirArbol`, y el caso ocurre de verdad —un filtro que se lleva al
// padre y deja al hijo—.
func Arbol(notas []Nota) []Nodo {
	hijos := map[string][]Nota{}
	presente := make(map[string]bool, len(notas))
	for _, n := range notas {
		presente[n.ID] = true
	}
	var raiz []Nota
	for _, n := range notas {
		if n.Grupo != "" && presente[n.Grupo] {
			hijos[n.Grupo] = append(hijos[n.Grupo], n)
			continue
		}
		raiz = append(raiz, n)
	}
	// La profundidad está acotada por el número de notas, así que no hay ciclo posible
	// entre las que entran aquí: un ciclo exigiría que dos se tuvieran mutuamente como
	// padre, y el servicio no lo deja escribir.
	var construir func([]Nota) []Nodo
	construir = func(ns []Nota) []Nodo {
		out := make([]Nodo, 0, len(ns))
		for _, n := range ns {
			out = append(out, Nodo{Nota: n, Hijos: construir(hijos[n.ID])})
		}
		return out
	}
	return construir(raiz)
}

// Fecha es el `formatDate(locale, n.updated_at, {day:'numeric', month:'short'})` del
// árbol de notas.
func Fecha(l i18n.Locale, t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return i18n.FormatDayMonth(l, t)
}

// Motivo es lo que el servicio dijo, si dijo algo útil. No se enseña la URL ni el
// cuerpo entero: un error de API no es un mensaje para una persona.
func Motivo(err error) string {
	var e *api.Error
	if errors.As(err, &e) {
		return e.Code()
	}
	return ""
}

// motivo traduce el código del servicio cuando tiene mensaje propio en el catálogo.
func (p *Pantalla) motivo(err error) string {
	switch m := Motivo(err); m {
	case "notebook_owner_only":
		return p.tr.T("notebook.ownerOnly", nil)
	default:
		return m
	}
}
