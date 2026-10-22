// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package diapos

import (
	"context"
	"html"
	"regexp"
	"strings"
	"sync"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"

	"git.sytue.eu/sytue/one/apps/fyne/tema"
	"git.sytue.eu/sytue/one/apps/fyne/ui"
	"git.sytue.eu/sytue/one/apps/nucleo/api"
)

// «Presentar con Cast» (`slides/cast.ts` y `CastDialog.tsx` de la web): Diapos programa un
// directo en Sytue Cast, abre su página para emitir y, mientras se presenta, avisa de la
// diapositiva en la que va.
//
// SOLO SALE LA POSICIÓN: número, total y título. Ni el contenido ni las notas del ponente
// viajan a Cast; las notas siguen siendo del ponente, en su vista del moderador.

// MaxTituloCast es lo que se envía del título de una diapositiva (Cast guarda 120).
const MaxTituloCast = 120

var (
	reSaltoHTML   = regexp.MustCompile(`(?i)<br\s*/?>`)
	reEtiquetas   = regexp.MustCompile(`<[^>]*>`)
	reEncabezado  = regexp.MustCompile(`(?i)^\s*<h[1-6][\s>]`)
	reFinDeBloque = regexp.MustCompile(`(?i)</(?:p|div|li|h[1-6])>`)
	reRefCast     = regexp.MustCompile(`^[a-z][a-z0-9-]{0,19}:[A-Za-z0-9_-]{1,100}$`)
)

// textoPlano es el texto de un trozo de HTML del editor: sin etiquetas, con las entidades
// decodificadas y los blancos plegados (`textoPlano` de cast.ts).
func textoPlano(h string) string {
	h = reSaltoHTML.ReplaceAllString(h, " ")
	h = reEtiquetas.ReplaceAllString(h, " ")
	// Fields pliega también el espacio de no separación, que `\s` de Go no ve y el de JS sí.
	return strings.Join(strings.Fields(html.UnescapeString(h)), " ")
}

// TituloCast es el título de una diapositiva para Cast: el de su primera caja de texto que
// empieza por un encabezado o, si no hay, la primera línea de la primera caja con algo
// escrito. "" si no tiene texto.
func TituloCast(d Diapo) string {
	var cajas []Caja
	for _, c := range d.Cajas {
		if c.Tipo == "" || c.Tipo == "text" {
			cajas = append(cajas, c)
		}
	}
	var elegida *Caja
	for i := range cajas {
		if reEncabezado.MatchString(cajas[i].HTML) && textoPlano(cajas[i].HTML) != "" {
			elegida = &cajas[i]
			break
		}
	}
	if elegida == nil {
		for i := range cajas {
			if textoPlano(cajas[i].HTML) != "" {
				elegida = &cajas[i]
				break
			}
		}
	}
	if elegida == nil {
		return ""
	}
	primero, _, _ := splitBloque(elegida.HTML)
	r := []rune(textoPlano(primero))
	return string(r[:min(len(r), MaxTituloCast)])
}

// splitBloque corta en el primer cierre de bloque, como el `split(...)[0]` de la web.
func splitBloque(h string) (string, string, bool) {
	if loc := reFinDeBloque.FindStringIndex(h); loc != nil {
		return h[:loc[0]], h[loc[1]:], true
	}
	return h, "", false
}

// PosicionCast es la posición de la diapositiva `i` (desde 0); false si no existe.
func PosicionCast(m *Modelo, i int, docID string) (api.CastPosicion, bool) {
	if i < 0 || i >= len(m.Diapos) {
		return api.CastPosicion{}, false
	}
	p := api.CastPosicion{Index: i, Total: len(m.Diapos), Title: TituloCast(m.Diapos[i])}
	if ref := "docs:" + docID; reRefCast.MatchString(ref) {
		p.Ref = ref
	}
	return p, true
}

// NuevoAvisador manda a Cast la última posición, de una en una y sin amontonar: si se
// cambia de diapositiva mientras una petición va, solo se manda la más reciente al acabar
// esa. Los fallos van a `alFallar` una vez por racha y no paran la presentación: presentar
// no depende de que Cast conteste. `lanzar` corre la petición fuera del hilo de la
// interfaz.
func NuevoAvisador(enviar func(api.CastPosicion) error, alFallar func(), lanzar func(func())) func(api.CastPosicion) {
	var (
		mu         sync.Mutex
		enCurso    bool
		pendiente  *api.CastPosicion
		falloDicho bool
	)
	var bombear func()
	bombear = func() {
		mu.Lock()
		if enCurso || pendiente == nil {
			mu.Unlock()
			return
		}
		p := *pendiente
		pendiente, enCurso = nil, true
		mu.Unlock()
		lanzar(func() {
			err := enviar(p)
			mu.Lock()
			enCurso = false
			dice := err != nil && !falloDicho
			falloDicho = err != nil
			mu.Unlock()
			if dice {
				alFallar()
			}
			bombear()
		})
	}
	return func(p api.CastPosicion) {
		mu.Lock()
		pendiente = &p
		mu.Unlock()
		bombear()
	}
}

// Cast es lo que Diapos necesita para presentar con Cast; lo pone quien monta la ventana.
// Con Cliente nil la orden no se ofrece (sin Cast detrás la web tampoco la tiene).
type Cast struct {
	Cliente *api.Cast
	// DocID es el documento que se presenta (la referencia de origen `docs:<id>`).
	DocID string
	// Abrir enseña la página de Cast desde la que se emite el directo `videoID`.
	Abrir func(videoID string)
	// Lanzar corre una petición fuera del hilo de la interfaz; nil, una goroutine.
	Lanzar func(func())
}

// PresentarConCast pide el título de la emisión; con él programa el directo, abre su
// página y presenta con la vista del moderador.
func (e *Editor) PresentarConCast() {
	if e.Cast == nil || e.Cast.Cliente == nil {
		return
	}
	c := fyne.CurrentApp().Driver().CanvasForObject(e.raiz)
	if c == nil {
		return
	}
	campo := ui.NuevaEntrada("")
	campo.PonerTexto(e.ctx.Titulo)
	// El fallo es la ayuda de peligro de la suite (`InlineHelp tone="danger"`), como la
	// web; era un párrafo rojo suelto. Vacía mientras no haya fallo.
	fallo := container.New(ui.Columna{})
	fallo.Hide()
	var (
		dlg    *ui.Dialogo
		enviar *ui.Boton
	)
	mandar := func() {
		titulo := strings.Join(strings.Fields(campo.Texto()), " ")
		if titulo == "" || enviar.Cargando {
			return
		}
		enviar.Cargando = true
		enviar.Refresh()
		lanzar := e.lanzar()
		lanzar(func() {
			ctx, cancelar := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancelar()
			v, _, err := e.Cast.Cliente.ScheduleLive(ctx, api.CastNew{Title: titulo, Visibility: api.CastOrg, Source: "webrtc"})
			cancelar()
			fyne.Do(func() {
				enviar.Cargando = false
				enviar.Refresh()
				if err != nil {
					fallo.Objects = []fyne.CanvasObject{ui.NuevaAyudaEnLinea(e.t("slides.cast.failed")).ConTono(ui.AyudaPeligro)}
					fallo.Show()
					dlg.Recolocar()
					return
				}
				dlg.Cerrar()
				e.empezarCast(v.ID)
			})
		})
	}
	cancelar := ui.NuevoBoton(e.t("action.cancel"), func() { dlg.Cerrar() }).Con(ui.Fantasma, ui.TallaMD)
	enviar = ui.NuevoBoton(e.t("slides.cast.start"), mandar)
	campo.AlCambiar(func(s string) { enviar.Inerte = strings.TrimSpace(s) == ""; enviar.Refresh() })
	enviar.Inerte = strings.TrimSpace(e.ctx.Titulo) == ""
	campo.AlTeclear = func(k *fyne.KeyEvent) bool {
		if k.Name == fyne.KeyReturn || k.Name == fyne.KeyEnter {
			mandar()
			return true
		}
		return false
	}
	intro := ui.NuevoParrafo(e.t("slides.cast.intro"), tema.TextoSM, ui.Normal, "text-muted", e.variante())
	rotulo := ui.NuevoRotulo(e.t("slides.cast.name"), tema.TextoSM, ui.SemiNegrita, "text")
	cuerpo := container.New(ui.Columna{Hueco: tema.Espacio(1.5)}, intro, rotulo, campo, fallo)
	pie := container.New(ui.Fila{Hueco: ui.HuecoPieDialogo}, cancelar, enviar)
	dlg = ui.NuevoDialogo(e.t("slides.cast.title"), cuerpo).Con(ui.DialogoSM, pie)
	dlg.RotuloCerrar = e.t("ui.close")
	dlg.Mostrar(c)
}

func (e *Editor) lanzar() func(func()) {
	if e.Cast.Lanzar != nil {
		return e.Cast.Lanzar
	}
	return func(f func()) { go f() }
}

// empezarCast abre la página de emisión y presenta con el moderador, avisando de cada
// diapositiva.
func (e *Editor) empezarCast(videoID string) *Presentacion {
	if e.Cast.Abrir != nil {
		e.Cast.Abrir(videoID)
	}
	avisar := NuevoAvisador(
		func(p api.CastPosicion) error {
			ctx, cancelar := context.WithTimeout(context.Background(), 8*time.Second)
			defer cancelar()
			_, err := e.Cast.Cliente.AvanzarDiapositiva(ctx, videoID, p)
			return err
		},
		func() {
			fyne.Do(func() {
				e.avisos.Notificar(ui.NuevoAviso(ui.AvisoAdvertencia, e.t("slides.cast.lost"), ""))
			})
		},
		e.lanzar())
	enviar := func(i int) {
		if p, ok := PosicionCast(e.m, i, e.Cast.DocID); ok {
			avisar(p)
		}
	}
	p := Presentar(e.m, e.activa, true, e.Idioma, e.ctx.T)
	p.AlCambiarDiapo = enviar
	enviar(p.Diapo)
	return p
}
