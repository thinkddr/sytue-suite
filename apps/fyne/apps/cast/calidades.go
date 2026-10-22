// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

// Pieza de pantalla (Subir, Desde Drive, Gestionar): QualityPicker.tsx de la web.

package cast

import (
	"slices"
	"strconv"
	"strings"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"

	"git.sytue.eu/sytue/one/apps/fyne/tema"
	"git.sytue.eu/sytue/one/apps/fyne/ui"
	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/i18n"
)

// # Elegir calidades
//
// En qué calidades se podrá ver el vídeo, con lo que ocupa cada una ANTES de elegir:
// en Subir, en «Subir desde Drive» y en Gestionar, como QualityPicker.tsx. Los números
// son del servicio (`GET /videos:estimate`), la misma estimación que ve la web; aquí
// solo se combinan (lib/calidades.ts).

// SelCalidades son los datos del selector.
type SelCalidades struct {
	// Estimacion es nil mientras se calcula; Error, si no se pudo.
	Estimacion *api.CastEstimate
	Error      bool
	Elegidas   []int
	// Libres es lo que queda en el plan; nil si no se sabe.
	Libres *int64
	// AOcupar es lo que tiene que caber en Libres; nil = el total al subir (el
	// original más lo elegido). En Gestionar, solo lo que se añade.
	AOcupar *int64
	// Avisar1080: al subir aún no se sabe qué da el original.
	Avisar1080    bool
	Deshabilitado bool
	AlCambiar     func([]int)
}

// Etiqueta es lo que se enseña de una altura: «1080p».
func Etiqueta(q int) string { return strconv.Itoa(q) + "p" }

// Ordenadas de mayor a menor, como las guarda el servicio.
func Ordenadas(qs []int) []int {
	out := slices.Clone(qs)
	slices.SortFunc(out, func(a, b int) int { return b - a })
	return out
}

// BytesDe es lo que ocupan esas calidades (sin el original): solo las permitidas.
func BytesDe(e api.CastEstimate, elegidas []int) int64 {
	var n int64
	for _, o := range e.Options {
		if o.Allowed && slices.Contains(elegidas, o.Quality) {
			n += o.Bytes
		}
	}
	return n
}

// Recomendada es la elección de partida: todas las que el original permite.
func Recomendada(e api.CastEstimate) []int { return Ordenadas(e.Recommended) }

// EsRecomendada dice si esa elección es la recomendada.
func EsRecomendada(e api.CastEstimate, elegidas []int) bool {
	return slices.Equal(Ordenadas(elegidas), Recomendada(e))
}

// Alternar marca o desmarca una calidad sin dejar nunca la elección vacía: el
// servicio exige al menos una.
func Alternar(elegidas []int, q int) []int {
	if slices.Contains(elegidas, q) {
		resto := slices.DeleteFunc(slices.Clone(elegidas), func(x int) bool { return x == q })
		if len(resto) == 0 {
			return slices.Clone(elegidas)
		}
		return resto
	}
	return Ordenadas(append(slices.Clone(elegidas), q))
}

// Quitadas son las de `antes` que ya no están en `despues`: lo que se borra.
func Quitadas(antes, despues []int) []int {
	var out []int
	for _, q := range Ordenadas(antes) {
		if !slices.Contains(despues, q) {
			out = append(out, q)
		}
	}
	return out
}

// Generadas son las que existen en alguna de las dos escaleras.
func Generadas(c api.CastVideoQualities) []int {
	var out []int
	for _, q := range append(slices.Clone(c.HLSQualities), c.WebMQualities...) {
		if !slices.Contains(out, q) {
			out = append(out, q)
		}
	}
	return Ordenadas(out)
}

// SumarEstimaciones es la de varios ficheros con una sola elección: cada calidad suma
// lo de los ficheros que la permiten y se permite si la permite alguno.
func SumarEstimaciones(es []api.CastEstimate) *api.CastEstimate {
	if len(es) == 0 {
		return nil
	}
	por := map[int]api.CastQualityOption{}
	var orden []int
	out := api.CastEstimate{}
	for _, e := range es {
		out.OriginalBytes += e.OriginalBytes
		out.DurationMS += e.DurationMS
		out.Approximate = out.Approximate || e.Approximate
		for _, o := range e.Options {
			a, ok := por[o.Quality]
			if !ok {
				orden = append(orden, o.Quality)
				por[o.Quality] = o
				continue
			}
			a.Allowed, a.Recommended, a.Bytes = a.Allowed || o.Allowed, a.Recommended || o.Recommended, a.Bytes+o.Bytes
			por[o.Quality] = a
		}
	}
	for _, q := range Ordenadas(orden) {
		out.Options = append(out.Options, por[q])
		if por[q].Recommended {
			out.Recommended = append(out.Recommended, q)
		}
	}
	return &out
}

// SelectorCalidades es QualityPicker.tsx.
func SelectorCalidades(v Vista, d SelCalidades) fyne.CanvasObject {
	bloques := []ui.Bloque{
		{Obj: ui.Texto(v.Traduce("quality.title"), tema.TextoSM, ui.SemiNegrita, tema.Color("text", v.Variante)), Margen: tema.Espacio(1)},
		{Obj: Parrafo(v, v.Traduce("quality.hint"), tema.TextoXS, ui.Normal, "text-muted"), Margen: tema.Espacio(2)},
	}
	switch {
	case d.Error:
		bloques = append(bloques, ui.Bloque{Obj: Parrafo(v, v.Traduce("quality.estimateError"), tema.TextoSM, ui.Normal, "text-muted")})
	case d.Estimacion == nil:
		for i := range 4 {
			bloques = append(bloques, ui.Bloque{Obj: ui.NuevoEsqueleto(0, []float32{tema.Espacio(10), tema.Espacio(5), tema.Espacio(5), tema.Espacio(5)}[i]), Margen: tema.Espacio(2)})
		}
	default:
		bloques = append(bloques, cuerpoCalidades(v, d, *d.Estimacion)...)
	}
	return ui.PilaBloques(bloques...)
}

func cuerpoCalidades(v Vista, d SelCalidades, e api.CastEstimate) []ui.Bloque {
	tam := func(n int64) string { return v.Traduce("quality.size", i18n.Vars{"size": Tamano(v, n)}) }
	cambiar := func(qs []int) {
		if d.AlCambiar != nil && !d.Deshabilitado {
			d.AlCambiar(qs)
		}
	}
	rec := Recomendada(e)
	enLaRec := EsRecomendada(e, d.Elegidas)
	etiquetas := make([]string, len(rec))
	for i, q := range rec {
		etiquetas[i] = Etiqueta(q)
	}
	// La recomendada es una tarjeta de elección (ChoiceCard): marcada si es la
	// elección de ahora y, si no, pulsarla vuelve a ella.
	recomendada := ui.NuevaTarjetaEleccion(v.Traduce("quality.recommended"),
		strings.Join(etiquetas, " · ")+" · "+tam(BytesDe(e, rec)), func() { cambiar(rec) })
	recomendada.Nota, recomendada.Elegida, recomendada.Densa = v.Traduce("quality.recommendedHint"), enLaRec, true
	recomendada.Inerte = d.Deshabilitado

	out := []ui.Bloque{{Obj: container.New(&unaTarjeta{}, recomendada), Margen: tema.Espacio(2)}}
	for _, o := range e.Options {
		q := o.Quality
		marcada := o.Allowed && slices.Contains(d.Elegidas, q)
		c := ui.NuevaCasilla(Etiqueta(q), func(bool) { cambiar(Alternar(d.Elegidas, q)) })
		c.Marcada, c.Escalon = marcada, tema.TextoSM
		if !o.Allowed || d.Deshabilitado || (marcada && len(d.Elegidas) == 1) {
			c.Disable()
		}
		texto := v.Traduce("quality.unavailable")
		if o.Allowed {
			texto = tam(o.Bytes)
		}
		out = append(out, ui.Bloque{Obj: container.New(&filaCalidad{}, c, Parrafo(v, texto, tema.TextoSM, ui.Normal, "text-muted")), Margen: tema.Espacio(1)})
	}
	total := e.OriginalBytes + BytesDe(e, d.Elegidas)
	necesita := total
	if d.AOcupar != nil {
		necesita = *d.AOcupar
	}
	out = append(out, ui.Bloque{Obj: Parrafo(v, v.Traduce("quality.total", i18n.Vars{"size": Tamano(v, total)}), tema.TextoSM, ui.Medio, "text"), Margen: tema.Espacio(1)})
	if d.Libres != nil {
		if necesita > *d.Libres {
			out = append(out, ui.Bloque{Obj: Parrafo(v, v.Traduce("quality.overQuota"), tema.TextoXS, ui.Normal, "danger-text"), Margen: tema.Espacio(1)})
		} else {
			out = append(out, ui.Bloque{Obj: Parrafo(v, v.Traduce("quality.available", i18n.Vars{"available": Tamano(v, *d.Libres)}), tema.TextoXS, ui.Normal, "text-muted"), Margen: tema.Espacio(1)})
		}
	}
	if e.Approximate {
		out = append(out, ui.Bloque{Obj: Parrafo(v, v.Traduce("quality.approximate"), tema.TextoXS, ui.Normal, "text-muted"), Margen: tema.Espacio(1)})
	}
	if d.Avisar1080 {
		out = append(out, ui.Bloque{Obj: Parrafo(v, v.Traduce("quality.onlyIfOriginal"), tema.TextoXS, ui.Normal, "text-muted")})
	}
	return out
}

// filaCalidad: la casilla con su «1080p» en una columna fija (`w-14` tras el
// control) y lo que ocupa a continuación, para que los tamaños queden en columna.
type filaCalidad struct{}

func (filaCalidad) columna() float32 { return ui.LadoEleccion + ui.HuecoEleccion + tema.Espacio(14) }

func (f filaCalidad) MinSize(objs []fyne.CanvasObject) fyne.Size {
	return fyne.NewSize(f.columna()+objs[1].MinSize().Width, max(objs[0].MinSize().Height, tema.TextoSM.Linea)+tema.Espacio(3))
}

// AltoCon es el de la línea con su `py-1.5`.
func (f filaCalidad) AltoCon(objs []fyne.CanvasObject, ancho float32) float32 {
	return max(objs[0].MinSize().Height, ui.AltoA(objs[1], ancho-f.columna()-tema.Espacio(3))) + tema.Espacio(3)
}

func (f filaCalidad) Layout(objs []fyne.CanvasObject, t fyne.Size) {
	y := tema.Espacio(1.5)
	m := objs[0].MinSize()
	objs[0].Move(fyne.NewPos(tema.Espacio(2), y))
	objs[0].Resize(fyne.NewSize(f.columna()-tema.Espacio(2), m.Height))
	x := f.columna() + tema.Espacio(3)
	w := max(t.Width-x, 0)
	objs[1].Move(fyne.NewPos(x, y+(m.Height-tema.TextoSM.Linea)/2))
	objs[1].Resize(fyne.NewSize(w, ui.AltoA(objs[1], w)))
}

// unaTarjeta es una tarjeta de elección a todo el ancho, del alto que pide a ese ancho.
type unaTarjeta struct{}

func (unaTarjeta) MinSize(objs []fyne.CanvasObject) fyne.Size { return objs[0].MinSize() }

// AltoCon es el de la tarjeta a ese ancho.
func (unaTarjeta) AltoCon(objs []fyne.CanvasObject, ancho float32) float32 {
	if c, ok := objs[0].(*ui.TarjetaEleccion); ok {
		return ui.AltoTarjetaEleccion(c, ancho)
	}
	return objs[0].MinSize().Height
}

func (u unaTarjeta) Layout(objs []fyne.CanvasObject, t fyne.Size) {
	if c, ok := objs[0].(*ui.TarjetaEleccion); ok {
		c.Ancho = t.Width
	}
	objs[0].Move(fyne.NewPos(0, 0))
	objs[0].Resize(fyne.NewSize(t.Width, u.AltoCon(objs, t.Width)))
}
