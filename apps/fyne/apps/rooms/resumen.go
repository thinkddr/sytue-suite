// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package rooms

import (
	"context"
	"errors"
	"strings"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/widget"

	"git.sytue.eu/sytue/one/apps/fyne/tema"
	"git.sytue.eu/sytue/one/apps/fyne/ui"
	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/ia"
)

// En la web: apps/frontend/apps/rooms/src/pages/api/resumen.ts (el BFF), `ponmeAlDia`
// de ChatApp.tsx y apps/frontend/apps/rooms/src/components/TextoDelAsistente.tsx.
//
// «Ponme al día»: qué se ha hablado en el canal. En la web lo compone el BFF; en nativo
// no hay BFF, así que lo compone la app con el MISMO contexto —los últimos cien
// mensajes, «autor: texto», cortado a 12 000 caracteres por el final— y se lo pasa a
// Sytue AI con el token de la persona. Los mensajes no salen hacia ningún tercero: el
// servicio de IA es el de la casa, en la UE.

// Las constantes del BFF.
const (
	// MensajesResumen es `CUANTOS`: una tarde de canal activo.
	MensajesResumen = 100
	// TopeResumen es `MAX_CHARS`: se corta por caracteres, porque un canal con volcados
	// de registros llena cualquier ventana.
	TopeResumen = 12000
	// EsperaResumen es el `AbortSignal.timeout(70_000)` del BFF.
	EsperaResumen = 70 * time.Second
)

// InstruccionResumen es la del BFF, tal cual.
const InstruccionResumen = "Resume esta conversación de trabajo en cinco líneas como mucho. Di qué se ha " +
	"decidido, qué queda pendiente y de quién se espera algo. En el idioma de la " +
	"conversación. Sin preámbulo."

// ContextoResumen es el contexto del BFF: quién dijo qué, en orden de lectura, y lo
// último si pasa del tope. El nombre va delante porque un resumen sin autores no sirve
// para saber a quién preguntar.
func ContextoResumen(ms []api.RoomsMessage) string {
	lineas := make([]string, 0, len(ms))
	for _, m := range ms {
		lineas = append(lineas, nombreDe(m.AuthorName, m.AuthorSub)+": "+m.Body)
	}
	s := []rune(strings.Join(lineas, "\n"))
	return string(s[max(len(s)-TopeResumen, 0):])
}

// Asistente es lo que Rooms le pide a Sytue AI; lo cumple `(*api.IA).Asistir`.
type Asistente func(ctx context.Context, q api.PeticionIA) (string, error)

// errSinMensajes es el 422 `sin_mensajes` del BFF.
var errSinMensajes = errors.New("rooms: el canal no tiene mensajes que resumir")

// PonmeAlDia es `ponmeAlDia`: pide el resumen del canal abierto y lo enseña en su
// diálogo. No se guarda: lo interesante es lo de ahora. Mientras tanto, el botón de la
// cabecera se apaga.
func (sa *Sala) PonmeAlDia() {
	canal, cab, t := sa.abierto, sa.P.Canal.Cabecera, sa.P.T
	if canal == "" || cab.Resumiendo {
		return
	}
	cab.Resumiendo = true
	cab.Refresh()
	idioma := string(sa.P.Locale)
	pedir(sa, func(ctx context.Context) (string, error) {
		if sa.IA == nil {
			return "", &api.Error{Status: 503, Body: `{"code":"ai_not_configured"}`}
		}
		ms, err := sa.S.Messages(ctx, canal, time.Time{}, MensajesResumen)
		if err != nil {
			return "", err
		}
		if len(ms) == 0 {
			return "", errSinMensajes
		}
		ctx, cancelar := context.WithTimeout(ctx, EsperaResumen)
		defer cancelar()
		return sa.IA(ctx, api.PeticionIA{
			Accion: "resumir", Tipo: "chat", Instruccion: InstruccionResumen, Contexto: ContextoResumen(ms),
			Modo: "interactivo", Esfuerzo: "none", Idioma: idioma,
		})
	}, func(texto string, err error) {
		cab.Resumiendo = false
		cab.Refresh()
		switch {
		case api.IANoConfigurada(err):
			texto = t.T("channel.summaryOff", nil)
		case err != nil || strings.TrimSpace(texto) == "":
			texto = t.T("channel.summaryError", nil)
		}
		sa.enseñarResumen(texto)
	})
}

// enseñarResumen es el `<Modal title={channel.summary}>` con el texto dentro.
func (sa *Sala) enseñarResumen(texto string) {
	lienzo := fyne.CurrentApp().Driver().CanvasForObject(sa.P.Objeto())
	if lienzo == nil {
		return
	}
	t := sa.P.T
	d := ui.NuevoDialogo(t.T("channel.summary", nil), NuevoTextoAsistente(texto))
	d.RotuloCerrar = t.T("action.close", nil)
	sa.pregunta = d
	sa.resumen = texto
	d.Mostrar(lienzo)
}

// TextoAsistente es `TextoDelAsistente`: la respuesta del modelo pintada desde los
// bloques de `ia.BloquesDe` —nunca desde su HTML—: párrafos `text-sm`, viñetas con
// `ml-4 list-disc space-y-1` y los tres niveles de título igual, en seminegrita y con
// `mt-2`. Entre bloques, `gap-2`. Sin bloques, el texto crudo.
type TextoAsistente struct {
	widget.BaseWidget
	Texto string

	objs  []fyne.CanvasObject
	ancho float32
	alto  float32
}

// NuevoTextoAsistente crea el texto.
func NuevoTextoAsistente(texto string) *TextoAsistente {
	x := &TextoAsistente{Texto: texto}
	x.ExtendBaseWidget(x)
	return x
}

// AccessibilityLabel es el texto tal cual.
func (x *TextoAsistente) AccessibilityLabel() string { return x.Texto }

// AccessibilityRole: es texto.
func (x *TextoAsistente) AccessibilityRole() fyne.AccessibleRole { return fyne.AccessibleRoleText }

func (x *TextoAsistente) CreateRenderer() fyne.WidgetRenderer { return &textoAsistenteRenderer{x: x} }

type textoAsistenteRenderer struct{ x *TextoAsistente }

// AnchoTextoAsistente es el que se reparte antes de tener sitio: el cuerpo de un
// diálogo `md` (512 − 2 × 21).
var AnchoTextoAsistente float32 = 512 - 2*tema.Espacio(6)

func (r *textoAsistenteRenderer) MinSize() fyne.Size {
	ancho := r.x.ancho
	if ancho <= 0 {
		ancho = AnchoTextoAsistente
	}
	_, alto := maquetarAsistente(r.x.Texto, ancho)
	return fyne.NewSize(0, alto)
}

func (r *textoAsistenteRenderer) Objects() []fyne.CanvasObject { return r.x.objs }
func (r *textoAsistenteRenderer) Destroy()                     {}

func (r *textoAsistenteRenderer) Layout(t fyne.Size) {
	r.x.ancho = t.Width
	r.x.objs, r.x.alto = maquetarAsistente(r.x.Texto, t.Width)
}

func (r *textoAsistenteRenderer) Refresh() { r.Layout(r.x.Size()) }

// maquetarAsistente reparte los bloques a ese ancho y dice cuánto miden. Cada bloque es
// un `ui.ParrafoRico` a `text-sm`: `<strong>` en negrita (700) también dentro de un
// título, que es `font-semibold`.
func maquetarAsistente(texto string, ancho float32) ([]fyne.CanvasObject, float32) {
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	esc := tema.TextoSM
	bloques := ia.BloquesDe(texto)
	if len(bloques) == 0 {
		lineas := ui.Repartir(texto, esc, ui.Normal, ancho)
		objs := make([]fyne.CanvasObject, 0, len(lineas))
		for i, l := range lineas {
			tx := ui.Texto(l, esc, ui.Normal, tema.Color("text", v))
			ui.ColocarTexto(tx, esc, 0, float32(i)*esc.Linea)
			objs = append(objs, tx)
		}
		return objs, float32(len(lineas)) * esc.Linea
	}
	var objs []fyne.CanvasObject
	y := float32(0)
	anterior := ia.Tipo("")
	for i, b := range bloques {
		if i > 0 {
			// `gap-2` entre bloques; dentro de una lista, `space-y-1`.
			if b.Tipo == ia.Punto && anterior == ia.Punto {
				y += tema.Espacio(1)
			} else {
				y += tema.Espacio(2)
			}
		}
		x, cabe, cara := float32(0), ancho, tema.Regular
		switch b.Tipo {
		case ia.Punto:
			x = tema.Espacio(4) // `ml-4`, con la viñeta fuera como `list-disc`
			vi := ui.Texto("•", esc, ui.Normal, tema.Color("text", v))
			ui.ColocarTexto(vi, esc, x-ui.Medir("• ", esc, ui.Normal).Width, y)
			objs = append(objs, vi)
			cabe = ancho - x
		case ia.H1, ia.H2, ia.H3:
			if i > 0 {
				y += tema.Espacio(2) // `mt-2 first:mt-0`
			}
			cara = tema.SemiNegra
		case ia.Parrafo:
		}
		p := ui.ParrafoRico(b.Trozos, cara, esc, cabe)
		p.Move(fyne.NewPos(x, y))
		p.Resize(p.MinSize())
		objs = append(objs, p)
		y += p.MinSize().Height
		anterior = b.Tipo
	}
	return objs, y
}
