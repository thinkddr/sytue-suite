// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package calendario

import (
	"encoding/json"
	"regexp"
	"strings"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"

	"git.sytue.eu/sytue/one/apps/nucleo/api"

	"git.sytue.eu/sytue/one/apps/fyne/iconos"
	"git.sytue.eu/sytue/one/apps/fyne/tema"
	"git.sytue.eu/sytue/one/apps/fyne/ui"
)

// El asistente del formulario: AsistenteEvento.tsx y lib/ai.ts. Arriba del todo, una
// frase («Comida con Ana el jueves a las 14:00 en Casa Lucio») y «Rellenar»: el
// servicio de IA propone un evento y el formulario se rellena. NO guarda nada: quien
// firma es quien pulsa «Guardar».

// maxFraseAsistente es el `MAX` de la web.
const maxFraseAsistente = 2000

// Propuesta es `EventoPropuesto`. Lo que el modelo no sepa, se queda vacío.
type Propuesta struct {
	Titulo, Lugar, Descripcion string
	Inicio, Fin                time.Time
}

// InstruccionEvento es `instruccionEvento`: lleva la fecha de hoy dentro —sin ella «el
// jueves» no tiene referencia— y pide ISO 8601, que es lo único que se acepta.
func InstruccionEvento(ahora time.Time, zona string) string {
	return "Extrae un evento de agenda del texto. Devuelve SOLO un objeto JSON con las claves " +
		"{titulo, inicio, fin, lugar, descripcion}. Las fechas en ISO 8601 con zona horaria. " +
		"Omite las claves que el texto no diga; no inventes ni la hora ni el lugar. " +
		"Hoy es " + ahora.UTC().Format("2006-01-02T15:04:05.000Z") + " y la zona horaria es " + zona + "."
}

var (
	cercaAbre   = regexp.MustCompile("(?i)^\\s*```(?:json)?\\s*")
	cercaCierra = regexp.MustCompile("\\s*```\\s*$")
	// formaFecha es la de `fechaHora`: la forma se comprueba A MANO, porque un parser
	// generoso convierte una frase sin resolver en una cita.
	formaFecha = regexp.MustCompile(`^\d{4}-\d{2}-\d{2}[T ]\d{2}:\d{2}(:\d{2})?(\.\d+)?(Z|[+-]\d{2}:?\d{2})?$`)
)

// Las formas que admite formaFecha, con zona y sin ella.
var (
	conZona = []string{
		"2006-01-02T15:04:05.999999999Z07:00", "2006-01-02T15:04Z07:00",
		"2006-01-02T15:04:05.999999999-0700", "2006-01-02T15:04-0700",
	}
	sinZona = []string{"2006-01-02T15:04:05.999999999", "2006-01-02T15:04"}
)

// objetoDe es el de `@sytue/ai`: el objeto JSON de la respuesta, aunque venga entre
// cercas de código o con prosa alrededor.
func objetoDe(texto string) map[string]any {
	limpio := strings.TrimSpace(cercaCierra.ReplaceAllString(cercaAbre.ReplaceAllString(texto, ""), ""))
	intentos := []string{limpio}
	if i, j := strings.Index(limpio, "{"), strings.LastIndex(limpio, "}"); i >= 0 && j > i {
		intentos = append(intentos, limpio[i:j+1])
	}
	for _, c := range intentos {
		var v map[string]any
		if json.Unmarshal([]byte(c), &v) == nil && v != nil {
			return v
		}
	}
	return nil
}

// cadena es la de `@sytue/ai`: una cadena no vacía, recortada a `tope`.
//
// ponytail: recorta por runas y la web por unidades UTF-16; solo difiere con emojis y
// letras fuera del plano básico, en el carácter del corte.
func cadena(obj map[string]any, clave string, tope int) string {
	s, _ := obj[clave].(string)
	return recortar(strings.TrimSpace(s), tope)
}

// fechaHora es la de `@sytue/ai`. Sin zona, la hora es de la zona de visualización, que
// es lo que hace `new Date(…)` en el navegador.
func fechaHora(obj map[string]any, clave string, loc *time.Location) time.Time {
	s := cadena(obj, clave, 40)
	if !formaFecha.MatchString(s) {
		return time.Time{}
	}
	s = s[:10] + "T" + s[11:]
	for _, f := range conZona {
		if t, err := time.Parse(f, s); err == nil {
			return t
		}
	}
	for _, f := range sinZona {
		if t, err := time.ParseInLocation(f, s, loc); err == nil {
			return t
		}
	}
	return time.Time{}
}

// EventoDe es `eventoDe`: valida la respuesta. Sin título no hay propuesta; un fin
// anterior al inicio se descarta, él solo.
func EventoDe(bruto string, loc *time.Location) (Propuesta, bool) {
	obj := objetoDe(bruto)
	if obj == nil {
		return Propuesta{}, false
	}
	p := Propuesta{Titulo: cadena(obj, "titulo", 200)}
	if p.Titulo == "" {
		return Propuesta{}, false
	}
	p.Inicio = fechaHora(obj, "inicio", loc)
	if fin := fechaHora(obj, "fin", loc); !fin.IsZero() && (p.Inicio.IsZero() || fin.After(p.Inicio)) {
		p.Fin = fin
	}
	p.Lugar, p.Descripcion = cadena(obj, "lugar", 200), cadena(obj, "descripcion", 2000)
	return p, true
}

// construirAsistente crea el campo y «Rellenar» una sola vez.
func (f *formulario) construirAsistente() {
	p := f.p
	f.ia = ui.NuevaEntrada(p.t("event.aiPlaceholder", nil))
	f.ia.Rotulo = p.t("event.ai", nil) // `aria-label`: el campo no lleva `<label>` a la vista
	f.iaBoton = ui.NuevoBoton(p.t("event.aiFill", nil), f.pedirAsistente).Con(ui.Secundario, ui.TallaSM)
	f.iaBoton.Icono, f.iaBoton.TamIcono, f.iaBoton.Inerte = iconos.IA, 16, true
	f.ia.AlCambiar(func(s string) {
		f.iaBoton.Inerte = strings.TrimSpace(s) == ""
		f.iaBoton.Refresh()
		if f.iaError {
			f.iaError = false
			f.rehacer()
		}
	})
	// Intro RELLENA y no envía: dentro del formulario, un Intro que guarda a medias es
	// justo lo que no se quiere.
	f.ia.AlTeclear = intro(f.pedirAsistente)
}

// pedirAsistente es `pedir`.
func (f *formulario) pedirAsistente() {
	frase := recortar(strings.TrimSpace(f.ia.Texto()), maxFraseAsistente)
	if frase == "" || f.iaBoton.Cargando || f.a.Asistente == nil {
		return
	}
	f.iaBoton.Cargando, f.iaError = true, false
	f.iaBoton.Refresh()
	f.rehacer()
	// Lo mismo que manda la web: la frase va en el CONTEXTO, nunca en la instrucción.
	peticion := api.PeticionIA{
		Accion: "evento", Instruccion: InstruccionEvento(time.Now(), f.p.nombreZona()),
		Contexto: frase, Tipo: "calendar", Modo: "calidad",
	}
	f.a.Asistente(peticion, func(texto string, err error) {
		f.iaBoton.Cargando = false
		f.iaBoton.Refresh()
		prop, ok := EventoDe(texto, f.p.zona())
		if err != nil || !ok {
			f.iaError = true
			f.rehacer()
			return
		}
		f.aplicarPropuesta(prop)
		f.ia.PonerTexto("")
	})
}

// aplicarPropuesta es la de EventModal: lo que no venga se queda como estaba, y el fin
// ARRASTRA al inicio conservando la duración, como al mover la hora a mano.
func (f *formulario) aplicarPropuesta(p Propuesta) {
	f.leerFechas()
	f.titulo.PonerTexto(p.Titulo)
	f.errTitulo, f.titulo.Invalida = "", false
	switch {
	case !p.Inicio.IsZero():
		fin := p.Fin
		if fin.IsZero() {
			fin = p.Inicio.Add(f.e.Fin.Sub(f.e.Inicio))
		}
		f.e.Inicio, f.e.Fin = p.Inicio, fin
		f.errFin, f.fin.Invalida = "", false
	case !p.Fin.IsZero():
		f.e.Fin = p.Fin
		f.errFin, f.fin.Invalida = "", false
	}
	f.escribirFechas()
	if p.Lugar != "" {
		f.lugar.PonerTexto(p.Lugar)
	}
	if p.Descripcion != "" {
		f.descripcion.PonerTexto(p.Descripcion)
	}
	f.rehacer()
}

// bloqueAsistente es `flex flex-col gap-1`: la fila `items-end gap-2` con el campo que
// crece y el botón, y debajo el error.
func (f *formulario) bloqueAsistente() fyne.CanvasObject {
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	hijos := []fyne.CanvasObject{container.New(filaAsistente{}, f.ia, f.iaBoton)}
	if f.iaError {
		hijos = append(hijos, ui.NuevoParrafo(f.p.t("event.aiError", nil), tema.TextoXS, ui.Normal, "danger-text", v).A(f.util()))
	}
	return container.New(ui.Columna{Hueco: espacio1}, hijos...)
}

// filaAsistente es `flex items-end gap-2` con el campo en `flex-1`.
type filaAsistente struct{}

func (filaAsistente) MinSize(objs []fyne.CanvasObject) fyne.Size {
	a, b := objs[0].MinSize(), objs[1].MinSize()
	return fyne.NewSize(a.Width+espacio2+b.Width, max(a.Height, b.Height))
}

func (filaAsistente) Layout(objs []fyne.CanvasObject, t fyne.Size) {
	b := objs[1].MinSize()
	objs[1].Move(fyne.NewPos(t.Width-b.Width, t.Height-b.Height))
	objs[1].Resize(b)
	a := objs[0].MinSize()
	objs[0].Move(fyne.NewPos(0, t.Height-a.Height))
	objs[0].Resize(fyne.NewSize(max(t.Width-b.Width-espacio2, 0), a.Height))
}
