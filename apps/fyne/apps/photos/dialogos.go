// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package photos

import (
	"sort"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"

	"git.sytue.eu/sytue/one/apps/fyne/iconos"
	"git.sytue.eu/sytue/one/apps/fyne/tema"
	"git.sytue.eu/sytue/one/apps/fyne/ui"
	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/i18n"
)

// ---- Los dos diálogos de la casa ----

// confirmar es `useConfirm()`: el ConfirmDialog con un párrafo `text-sm
// leading-relaxed` dentro, el cancelar de `ui.cancel` y el verbo que se pida.
// Todas las confirmaciones de Fotos son de peligro (`tone: 'danger'`): borran o
// dejan de compartir.
func (a *App) confirmar(titulo, mensaje, verbo string, alAceptar func()) {
	if a.w == nil {
		alAceptar() // sin ventana (los guardianes) no hay a quién preguntar
		return
	}
	var d *ui.Dialogo
	texto := ui.NuevoParrafo(mensaje, tema.Escalon{Tam: tema.TextoSM.Tam, Linea: 1.625 * tema.TextoSM.Tam},
		ui.Normal, "text", a.v.Variante)
	d = ui.Confirmacion(titulo, a.v.Traduce("ui.cancel"), verbo, true, ui.AlAncho(anchoCuerpoDialogo(a), texto),
		func() { d.Cerrar() }, func() { d.Cerrar(); alAceptar() })
	d.RotuloCerrar = a.v.Traduce("ui.close")
	d.Mostrar(a.w.Canvas())
}

// preguntar es `usePrompt()`: el mismo diálogo con un campo, su etiqueta con el
// asterisco de obligatorio y el botón de confirmar apagado mientras esté vacío.
func (a *App) preguntar(titulo, mensaje, verbo, valor string, alAceptar func(string)) {
	if a.w == nil {
		alAceptar(valor)
		return
	}
	campo := ui.NuevaEntrada("")
	campo.PonerTexto(valor)
	var d *ui.Dialogo
	aceptar := func() {
		if t := strings.TrimSpace(campo.Texto()); t != "" {
			d.Cerrar()
			alAceptar(t)
		}
	}
	etiqueta := container.New(ui.Fila{Hueco: tema.Espacio(0.5)},
		ParrafoEn(mensaje, tema.TextoSM, ui.SemiNegrita, "text", a.v.Variante),
		ParrafoEn("*", tema.TextoSM, ui.SemiNegrita, "danger-text", a.v.Variante))
	cuerpo := ui.PilaBloques(ui.Bloque{Obj: etiqueta, Margen: tema.Espacio(1.5)}, ui.Bloque{Obj: campo})
	d = ui.Confirmacion(titulo, a.v.Traduce("ui.cancel"), verbo, false, ui.AlAncho(anchoCuerpoDialogo(a), cuerpo),
		func() { d.Cerrar() }, aceptar)
	d.RotuloCerrar = a.v.Traduce("ui.close")
	d.Foco = campo
	confirmar := botonConfirmar(d)
	sincronizar := func(s string) {
		if confirmar != nil {
			confirmar.Inerte = strings.TrimSpace(s) == ""
			confirmar.Refresh()
		}
	}
	campo.AlCambiar(sincronizar)
	sincronizar(valor)
	campo.AlTeclear = func(e *fyne.KeyEvent) bool {
		if e.Name == fyne.KeyReturn || e.Name == fyne.KeyEnter {
			aceptar()
			return true
		}
		return false
	}
	d.Mostrar(a.w.Canvas())
}

// botonConfirmar es el último botón del pie de una Confirmacion: el del verbo.
func botonConfirmar(d *ui.Dialogo) *ui.Boton {
	pie, ok := d.Pie.(*fyne.Container)
	if !ok || len(pie.Objects) == 0 {
		return nil
	}
	b, _ := pie.Objects[len(pie.Objects)-1].(*ui.Boton)
	return b
}

// anchoCuerpoDialogo es lo que le queda al cuerpo de un diálogo `sm` en esta ventana:
// los 400 de `max-w-[400px]` menos el `px-6` y los bordes. El párrafo se reparte a ese
// ancho y no al mínimo de sus palabras.
func anchoCuerpoDialogo(a *App) float32 {
	var lienzo fyne.Size
	if a.w != nil {
		lienzo = a.w.Canvas().Size()
	}
	return ui.AnchoCuerpoTalla(ui.DialogoSM, lienzo)
}

// ---- Los recuerdos ----

// LadoRecuerdo es el `h-24` de cada foto de la tira. El ancho es el que le toque por su
// proporción (`aspect-ratio`, la misma `proporcion()` de la galería): eran cuadrados
// `w-24` que recortaban la foto.
const LadoRecuerdo = 24 * 0.25 * tema.Rem

// SeccionRecuerdos es Recuerdos.tsx: `border rounded-md p-4 mb-6`, la cabecera con el
// reloj, «Un día como hoy» y «Ocultar», y una tira por año que se desplaza de lado
// (`overflow-x-auto`) cuando no cabe, que en un teléfono es siempre.
func SeccionRecuerdos(v Vista, lista []api.Memory, hoy time.Time, alAbrir func(id string), alOcultar func()) fyne.CanvasObject {
	cabecera := container.New(ui.FilaAuto{Hueco: tema.Espacio(2), Derecha: 2},
		iconos.Trazo(iconos.Reloj, iconos.TallaSM, "text", v.Variante).Objeto(),
		ParrafoEn(v.Traduce("memories.title"), tema.TextoSM, ui.SemiNegrita, "text", v.Variante),
		ui.NuevoBoton(v.Traduce("memories.hide"), alOcultar).Con(ui.Fantasma, ui.TallaXS),
	)
	bloques := make([]ui.Bloque, 0, 1+2*len(lista))
	bloques = append(bloques, ui.Bloque{Obj: cabecera, Margen: tema.Espacio(3)})
	for _, r := range lista {
		clave := v.T.T("memories.years", i18n.Vars{"count": hoy.Year() - r.Year})
		if hoy.Year()-r.Year == 1 {
			clave = v.Traduce("memories.oneYear")
		}
		fotos := make([]fyne.CanvasObject, 0, len(r.Items))
		for _, f := range r.Items {
			m := NuevaMiniatura(f, v.Teca)
			m.SinMarcas, m.Rotulo = true, textoAlternativo(f)
			id := f.ID
			m.AlPulsar = func() {
				if alAbrir != nil {
					alAbrir(id)
				}
			}
			fotos = append(fotos, container.New(fijo{ancho: LadoRecuerdo * Proporcion(f), alto: LadoRecuerdo}, m))
		}
		bloques = append(bloques,
			ui.Bloque{Obj: ParrafoEn(clave, tema.TextoXS, ui.Normal, "text-muted", v.Variante), Margen: tema.Espacio(1)},
			// `pb-1` de la tira y `mb-3` entre años (`last:mb-0`).
			ui.Bloque{Obj: deslizable(container.New(ui.Fila{Hueco: tema.Espacio(2)}, fotos...)), Margen: tema.Espacio(1) + tema.Espacio(3)})
	}
	bloques[len(bloques)-1].Margen = tema.Espacio(1)
	return recuadro(v, "", "border", tema.RadioMD,
		ui.Margen{Izq: tema.Espacio(4), Der: tema.Espacio(4), Arr: tema.Espacio(4), Abj: tema.Espacio(4)}, ui.PilaBloques(bloques...))
}

// textoAlternativo es el de photosApi.ts: la descripción si la hay, y si no el nombre.
func textoAlternativo(f api.Photo) string {
	if strings.TrimSpace(f.Description) != "" {
		return f.Description
	}
	return f.Name
}

// ---- El título propuesto ----

// TituloSugerido es `tituloSugerido()` de lib/tituloAlbum.ts: las etiquetas que
// comparten al menos la mitad de las fotos (y al menos dos), las tres más repetidas,
// unidas con « · » y con la primera letra en mayúscula. Vacío si no hay nada claro.
func TituloSugerido(fotos []api.Photo) string {
	if len(fotos) < 2 {
		return ""
	}
	cuenta := map[string]int{}
	for _, f := range fotos {
		vistas := map[string]bool{}
		for _, e := range f.Tags {
			e = strings.ToLower(strings.TrimSpace(e))
			if e != "" && !vistas[e] {
				vistas[e] = true
				cuenta[e]++
			}
		}
	}
	minimo := max(2, (len(fotos)+1)/2)
	var elegidas []string
	for e, n := range cuenta {
		if n >= minimo {
			elegidas = append(elegidas, e)
		}
	}
	sort.Slice(elegidas, func(i, j int) bool {
		if cuenta[elegidas[i]] != cuenta[elegidas[j]] {
			return cuenta[elegidas[i]] > cuenta[elegidas[j]]
		}
		return elegidas[i] < elegidas[j]
	})
	if len(elegidas) == 0 {
		return ""
	}
	nombre := strings.Join(elegidas[:min(len(elegidas), 3)], " · ")
	r, n := utf8.DecodeRuneInString(nombre)
	return string(unicode.ToUpper(r)) + nombre[n:]
}
