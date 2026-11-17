// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package ui

import (
	"strings"
	"unicode"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"

	"git.sytue.eu/sytue/one/apps/fyne/tema"
)

// Pregunta es usePrompt.tsx: `window.prompt` con el diálogo de la suite y un campo de
// verdad. El nombre de una hoja, la etiqueta de una versión, el de una categoría.
//
// Es una Confirmacion con un FieldGroup dentro —la etiqueta en `text-sm font-semibold`
// con el asterisco de obligatorio— y el campo con `valor` ya escrito. Lo que añade
// sobre la confirmación es lo que el diálogo del navegador no sabía hacer: el botón de
// aceptar se APAGA mientras el campo esté vacío, e Intro acepta como allí.
//
// `mensaje` hace de título y de etiqueta, como en la web cuando no se da `titulo`.
// `obligatorio` es «ui.required» YA traducido: el asterisco se pinta y no se lee, y lo
// que se anuncia es la palabra.
func Pregunta(mensaje, valor, rotuloCancelar, rotuloAceptar, obligatorio string, alAceptar func(string)) *Dialogo {
	return pregunta(mensaje, valor, rotuloCancelar, rotuloAceptar, obligatorio, false, alAceptar)
}

// PreguntaOpcional es la Pregunta con `permitirVacio`: el campo vacío también se acepta
// —el botón no se apaga e Intro acepta—, y la etiqueta va sin asterisco, como FieldGroup
// sin `required`. La etiqueta de una versión.
func PreguntaOpcional(mensaje, valor, rotuloCancelar, rotuloAceptar string, alAceptar func(string)) *Dialogo {
	return pregunta(mensaje, valor, rotuloCancelar, rotuloAceptar, "", true, alAceptar)
}

// OpcionesPregunta son los campos de PreguntarOpciones que Pregunta no recibe por
// argumento: `titulo` y `secreto`.
type OpcionesPregunta struct {
	// Titulo es el del diálogo cuando no es el propio mensaje: «Vista de Sytue Bases»
	// encima de la frase que explica qué token pegar.
	Titulo string
	// Secreto es `secreto`: el campo pinta puntos (`type="password"`).
	Secreto bool
	// Lienzo es el hueco donde se va a mostrar. Con él, una etiqueta que no cabe en una
	// línea se PARTE, como el `<label>` de la web; sin él, va en una sola línea (el
	// rótulo corto de siempre: «Nombre de la hoja»).
	Lienzo fyne.Size
	// Validar es `validar`: recibe lo escrito sin espacios a los lados y devuelve el
	// error que se pinta bajo el campo —la línea de error de GrupoCampo, con el campo en
	// rojo— y que apaga aceptar, o "" si vale. Con el campo vacío no se llama: eso ya
	// apaga aceptar. El nombre de una hoja que ya tiene otra.
	Validar func(string) string
}

// PreguntaCon es Pregunta con las opciones de PreguntarOpciones que no son de todos los
// días: un título propio, un campo secreto y una etiqueta larga que se parte.
func PreguntaCon(mensaje, valor, rotuloCancelar, rotuloAceptar, obligatorio string, o OpcionesPregunta,
	alAceptar func(string),
) *Dialogo {
	return preguntaCon(mensaje, valor, rotuloCancelar, rotuloAceptar, obligatorio, false, o, alAceptar)
}

func pregunta(mensaje, valor, rotuloCancelar, rotuloAceptar, obligatorio string, permitirVacio bool,
	alAceptar func(string),
) *Dialogo {
	return preguntaCon(mensaje, valor, rotuloCancelar, rotuloAceptar, obligatorio, permitirVacio, OpcionesPregunta{}, alAceptar)
}

func preguntaCon(mensaje, valor, rotuloCancelar, rotuloAceptar, obligatorio string, permitirVacio bool,
	o OpcionesPregunta, alAceptar func(string),
) *Dialogo {
	mensaje = SinDosPuntos(mensaje)
	titulo := mensaje
	if o.Titulo != "" {
		titulo = o.Titulo
	}
	campo := NuevaEntrada("")
	campo.PonerTexto(valor)
	campo.Contrasena(o.Secreto)
	var asterisco *asteriscoObligatorio
	if !permitirVacio {
		asterisco = &asteriscoObligatorio{
			Rotulo: Rotulo{Texto: "*", Escalon: tema.TextoSM, Peso: SemiNegrita, Token: "danger-text"},
			leido:  obligatorio,
		}
		asterisco.ExtendBaseWidget(asterisco)
	}
	etiqueta := etiquetaPregunta(mensaje, asterisco, o.Lienzo)
	cuerpo := container.New(Columna{Hueco: tema.Espacio(1.5)}, etiqueta, campo) // `gap-1.5`

	var d *Dialogo
	var aceptar *Boton
	// errorDe es lo que dice Validar de lo escrito ("" si vale o no hay que validar).
	errorDe := func() string {
		s := strings.TrimSpace(campo.Texto())
		if o.Validar == nil || s == "" {
			return ""
		}
		return o.Validar(s)
	}
	falta := func() bool {
		return (!permitirVacio && strings.TrimSpace(campo.Texto()) == "") || errorDe() != ""
	}
	// avisar pinta (o quita) la línea de error bajo el campo, como el `error` de
	// FieldGroup, y vuelve a medir el diálogo, que crece una línea.
	avisado := ""
	avisar := func() {
		e := errorDe()
		if e == avisado {
			return
		}
		avisado = e
		campo.Invalida = e != ""
		campo.Refresh()
		cuerpo.Objects = []fyne.CanvasObject{etiqueta, campo}
		if e != "" {
			// Partido al ancho del cuerpo: el aviso suele ser una frase larga.
			cuerpo.Objects = append(cuerpo.Objects, LineaErrorA(e, AnchoCuerpoDialogo(d, d.lienzo)))
		}
		cuerpo.Refresh()
		if d != nil {
			d.Recolocar()
		}
	}
	enviar := func() {
		if falta() {
			return
		}
		d.Cerrar()
		alAceptar(campo.Texto())
	}
	d, aceptar = confirmacion(titulo, rotuloCancelar, rotuloAceptar, false, cuerpo,
		func() { d.Cerrar() }, enviar)
	avisar()
	aceptar.Inerte = falta()
	campo.AlCambiar(func(string) {
		avisar()
		aceptar.Inerte = falta()
		aceptar.Refresh()
	})
	campo.AlTeclear = func(k *fyne.KeyEvent) bool {
		if k.Name != fyne.KeyReturn && k.Name != fyne.KeyEnter {
			return false
		}
		enviar()
		return true
	}
	d.Foco = campo
	d.clase = clasePreguntar // la nueva cancela la pendiente, como usePrompt
	return d
}

// etiquetaPregunta es la `<label>` de FieldGroup: el rótulo y, si es obligatorio, el
// asterisco a `ml-0.5`. Si no cabe en una línea del cuerpo del diálogo, se parte y el
// asterisco va detrás de la última palabra, que es donde lo deja el navegador.
func etiquetaPregunta(mensaje string, asterisco *asteriscoObligatorio, lienzo fyne.Size) fyne.CanvasObject {
	var cola float32
	if asterisco != nil {
		cola = tema.Espacio(0.5) + MedidaRotulo(&asterisco.Rotulo).Width
	}
	if ancho := AnchoCuerpoTalla(DialogoSM, lienzo); lienzo.Width > 0 &&
		Medir(mensaje, tema.TextoSM, SemiNegrita).Width+cola > ancho {
		v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
		texto := NuevoParrafo(mensaje, tema.TextoSM, SemiNegrita, "text", v).A(ancho)
		objs := []fyne.CanvasObject{texto}
		if asterisco != nil {
			objs = append(objs, asterisco)
		}
		return container.New(&etiquetaPartida{texto: texto, ancho: ancho, cola: cola}, objs...)
	}
	etiqueta := container.New(Fila{Hueco: tema.Espacio(0.5)}, // `ml-0.5`
		NuevoRotulo(mensaje, tema.TextoSM, SemiNegrita, "text"))
	if asterisco != nil {
		etiqueta.Add(asterisco)
	}
	return etiqueta
}

// etiquetaPartida coloca el párrafo de la etiqueta y, detrás de su última línea, el
// asterisco. Si el asterisco ya no cabe en esa línea, baja a la siguiente, como un
// `<span>` en línea.
type etiquetaPartida struct {
	texto *Parrafo
	ancho float32
	cola  float32 // `ml-0.5` más el asterisco; 0 sin asterisco
}

func (e *etiquetaPartida) colaBaja() bool {
	if e.cola == 0 {
		return false
	}
	lineas := LineasDe(e.texto, e.ancho)
	if len(lineas) == 0 {
		return false
	}
	return Medir(lineas[len(lineas)-1], e.texto.Escalon, e.texto.Peso).Width+e.cola > e.ancho
}

func (e *etiquetaPartida) MinSize([]fyne.CanvasObject) fyne.Size {
	alto := AltoParrafo(e.texto, e.ancho)
	if e.colaBaja() {
		alto += e.texto.Escalon.Linea
	}
	return fyne.NewSize(e.ancho, alto)
}

func (e *etiquetaPartida) Layout(objs []fyne.CanvasObject, _ fyne.Size) {
	e.texto.Move(fyne.NewPos(0, 0))
	e.texto.Resize(MedidaParrafo(e.texto, e.ancho))
	if len(objs) < 2 {
		return
	}
	lineas := LineasDe(e.texto, e.ancho)
	x, y := float32(0), float32(0)
	if n := len(lineas); n > 0 {
		x = Medir(lineas[n-1], e.texto.Escalon, e.texto.Peso).Width + tema.Espacio(0.5)
		y = float32(n-1) * e.texto.Escalon.Linea
	}
	if e.colaBaja() {
		x, y = 0, AltoParrafo(e.texto, e.ancho)
	}
	objs[1].Move(fyne.NewPos(x, y))
	objs[1].Resize(objs[1].MinSize())
}

// asteriscoObligatorio es el `<span aria-hidden>*</span><span class="sr-only">` de
// FieldGroup: se ve el asterisco y se anuncia la palabra.
type asteriscoObligatorio struct {
	Rotulo
	leido string
}

func (a *asteriscoObligatorio) AccessibilityLabel() string { return a.leido }

// SinDosPuntos quita los dos puntos del final: las preguntas heredadas de
// `window.prompt` («Nombre de la macro:», «Etiqueta de la versión (opcional):») hacen
// de título y de etiqueta del campo, y ahí no llevan. Lo mismo hace usePrompt.tsx.
func SinDosPuntos(s string) string {
	s = strings.TrimRight(strings.TrimRightFunc(s, unicode.IsSpace), ":：")
	return strings.TrimRightFunc(s, unicode.IsSpace) // «Nom de la macro :», con su espacio fino
}
