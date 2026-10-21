// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package notes

import (
	"context"
	"strings"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"

	"git.sytue.eu/sytue/one/apps/fyne/iconos"
	"git.sytue.eu/sytue/one/apps/fyne/tema"
	"git.sytue.eu/sytue/one/apps/fyne/ui"
	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/ia"
)

// Paridad web: EL ASISTENTE DEL EDITOR —`aplicarIA` de RichText.tsx y `lib/ai.ts`—. Va
// al final de la barra y aparte («no es formato, reescribe lo que has apuntado»), con las
// siete acciones de un cuaderno: ordenar, sacar las tareas y proponer un título; y
// resumir, desarrollar, corregir y traducir. La inferencia la hace Sytue AI
// (`api.IA.Asistir`), en directo y con el token de la persona.
//
// Lo que vuelve se aplica como en la web: con formato (varios bloques, títulos, listas,
// negrita) y sin selección, se escribe en el cursor con el formato de la nota; con
// selección, la sustituye; y si no, sustituye la nota entera. Siempre en un paso de
// deshacer. Si falla, la nota no se toca y se dice debajo de la barra.

// AccionesIA son las del menú, en su orden; la vacía es el separador.
var AccionesIA = []struct{ Accion, Clave string }{
	{"ordenar", "editor.ai.tidy"},
	{"tareas", "editor.ai.tasks"},
	{"titular", "editor.ai.title"},
	{"", ""},
	{"resumir", "editor.ai.summarize"},
	{"ampliar", "editor.ai.expand"},
	{"corregir", "editor.ai.fix"},
	{"traducir", "editor.ai.translate"},
}

// MaxContextoIA es el `MAX_CONTEXTO`: lo que se manda, como mucho, en letras.
const MaxContextoIA = 12000

// directivasIA son las `DIRECTIVAS` de lib/ai.ts, tal cual: todas acaban pidiendo SOLO
// el texto, o el preámbulo del modelo acaba en la nota.
var directivasIA = map[string]string{
	"ordenar":  "Ordena estas notas en apartados con títulos y viñetas, sin añadir ni quitar información. Usa Markdown. Devuelve solo el texto.",
	"corregir": "Corrige la ortografía, la gramática y la puntuación. No reescribas ni cambies el estilo. Devuelve solo el texto corregido.",
	"resumir":  "Resume estas notas conservando los datos concretos (fechas, cifras, nombres). Devuelve solo el texto.",
	"tareas":   "Saca las tareas pendientes que haya en estas notas, una por línea y empezando por un verbo. Si se dice de quién se espera algo, ponlo. Devuelve solo la lista.",
	"ampliar":  "Desarrolla estas notas en un texto completo, sin inventar datos que no estén. Devuelve solo el texto.",
	"titular":  "Escribe un título de menos de sesenta caracteres para estas notas. Devuelve solo el título, sin comillas ni punto final.",
	// La web no deja elegir el idioma: traduce al de por defecto, el inglés.
	"traducir": "Traduce estas notas a inglés. Devuelve solo la traducción.",
}

// PeticionIA es `construirPeticion`: la acción con su directiva y el texto recortado.
func PeticionIA(accion, texto string) api.PeticionIA {
	r := []rune(strings.TrimSpace(texto))
	return api.PeticionIA{
		Accion: accion, Tipo: "note", Instruccion: directivasIA[accion],
		Contexto: string(r[:min(len(r), MaxContextoIA)]),
	}
}

// PlazoIA es lo que se espera al asistente.
const PlazoIA = 90 * time.Second

// ConAsistente pone el asistente al final de la barra: `asistir` es Sytue AI y `idioma`
// la lengua en que contesta. Sin él (Notepads sin sesión, o quien no lo quiera) no sale.
func (e *EditorNota) ConAsistente(asistir func(context.Context, api.PeticionIA) (string, error), idioma string) {
	e.Asistir, e.Idioma = asistir, idioma
	if e.botones["ia"] != nil {
		return
	}
	b := nuevoBotonBarra(ChispasD, iconos.Ninguno, e.tr.T("editor.ai", nil), nil)
	b.AlPulsar = func() { e.menuIA(b) }
	e.botones["ia"] = b
	e.barra.Objects = append(e.barra.Objects, b)
	e.barra.Refresh()
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	e.aviso = container.New(ui.Margen{Izq: 0.25 * tema.Rem, Der: 0.25 * tema.Rem, Arr: 0.125 * tema.Rem, Abj: 0.125 * tema.Rem},
		lineaTok(e.tr.T("editor.aiError", nil), tema.TextoXS, ui.Normal, "danger-text", v))
}

// menuIA abre el desplegable de las siete acciones.
func (e *EditorNota) menuIA(disparador fyne.CanvasObject) {
	var elementos []fyne.CanvasObject
	for _, a := range AccionesIA {
		if a.Accion == "" {
			elementos = append(elementos, ui.NuevoSeparadorMenu())
			continue
		}
		elementos = append(elementos, ui.NuevoElementoMenu(e.tr.T(a.Clave, nil), iconos.Ninguno, func() {
			e.PedirIA(func(texto string) api.PeticionIA { return PeticionIA(a.Accion, texto) })
			e.reenfocar()
		}))
	}
	m := ui.NuevoMenu(elementos...)
	m.Rotulo = e.tr.T("editor.ai", nil)
	m.Mostrar(disparador)
}

// PedirIA manda al asistente la petición que `pedir` componga con el texto —lo
// seleccionado, o la nota entera— y aplica lo que vuelva. Es la entrada del menú y la de
// quien trae sus propias acciones (el redactor de Mail). Con una en marcha, o sin texto,
// no hace nada.
func (e *EditorNota) PedirIA(pedir func(texto string) api.PeticionIA) {
	if e.Asistir == nil || e.iaTrabajando {
		return
	}
	sel := e.haySeleccion()
	texto := e.Seleccionado()
	if !sel && len(e.doc.Bloques) > 0 {
		texto = e.doc.TextoEntre(Pos{}, e.fin())
	}
	if strings.TrimSpace(texto) == "" {
		return
	}
	q := pedir(texto)
	q.Idioma = e.Idioma
	e.ponerIA(true, false)
	asistir := e.Asistir
	go func() {
		ctx, cancelar := context.WithTimeout(context.Background(), PlazoIA)
		defer cancelar()
		res, err := asistir(ctx, q)
		fyne.Do(func() {
			res = strings.TrimSpace(res)
			if err != nil || res == "" {
				e.ponerIA(false, true)
				return
			}
			e.ponerIA(false, false)
			e.aplicarIA(res, sel)
		})
	}()
}

// ponerIA cambia el estado del asistente: en marcha (el botón, apagado) o con el aviso
// de que ha fallado.
func (e *EditorNota) ponerIA(trabajando, fallo bool) {
	antes := e.iaFallo
	e.iaTrabajando, e.iaFallo = trabajando, fallo
	if e.AlAsistir != nil {
		e.AlAsistir(trabajando, fallo)
	}
	e.repintar()
	if antes != fallo {
		e.Refresh() // el aviso entra o sale de debajo de la barra
		llamar0(e.AlCrecer)
	}
}

// aplicarIA escribe la respuesta como la web (ver la cabecera).
func (e *EditorNota) aplicarIA(res string, conSeleccion bool) {
	bloques := ia.BloquesDe(res)
	formato := len(bloques) > 1
	for _, b := range bloques {
		for _, t := range b.Trozos {
			formato = formato || b.Tipo != ia.Parrafo || t.Negrita || t.Cursiva
		}
	}
	e.hacer(func() {
		a, b := e.seleccion()
		switch {
		case formato && !conSeleccion:
			if e.haySeleccion() {
				e.cur = e.doc.Borrar(a, b)
			}
			e.cur = e.doc.InsertarDocumento(e.cur, documentoDeIA(bloques, e.TitulosEnNegrita))
		case conSeleccion && e.haySeleccion():
			e.cur = e.doc.Borrar(a, b)
			e.cur = e.doc.Insertar(e.cur, res, estiloEn(e.doc.Bloques[e.cur.B], e.cur.O))
		case conSeleccion:
			e.cur = e.doc.Insertar(e.cur, res, estiloEn(e.bloqueActual(), e.cur.O))
		default:
			// Sin selección y en texto llano, la nota entera (`selectNodeContents`).
			e.doc.Bloques = nil
			e.cur = e.doc.Insertar(Pos{}, res, Tramo{})
		}
		e.ancla = e.cur
	})
}

// documentoDeIA son los bloques del asistente con el formato de la nota: párrafos,
// títulos y viñetas (`insertUnorderedList`, también si venían numeradas), con su
// negrita y su cursiva. Con `titulosEnNegrita` (el correo, `escribirBloques` de Mail),
// un título es un párrafo entero en negrita, que es como se titula un apartado en un
// correo y lo que entiende cualquier cliente.
func documentoDeIA(bloques []ia.Bloque, titulosEnNegrita bool) *Documento {
	d := &Documento{}
	for _, b := range bloques {
		bl := Bloque{Tipo: Parrafo, Etiqueta: "p"}
		titulo := false
		switch b.Tipo {
		case ia.H1, ia.H2, ia.H3:
			titulo = titulosEnNegrita
			if !titulo {
				bl = Bloque{Tipo: Titulo, Nivel: int(b.Tipo[1] - '0'), Etiqueta: string(b.Tipo)}
			}
		case ia.Punto:
			bl = Bloque{Tipo: Vineta, Nivel: 1, Etiqueta: "li"}
		case ia.Parrafo:
		}
		for _, t := range b.Trozos {
			tr := Tramo{Texto: t.Texto}
			if t.Negrita || titulo {
				tr.Marcas = append(tr.Marcas, "b")
			}
			if t.Cursiva {
				tr.Marcas = append(tr.Marcas, "i")
			}
			bl.Tramos = append(bl.Tramos, tr)
		}
		d.Bloques = append(d.Bloques, bl)
	}
	return d
}
