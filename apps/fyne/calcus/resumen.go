// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package calcus

import (
	"math"
	"slices"
	"strings"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"

	"git.sytue.eu/sytue/one/apps/fyne/iconos"
	"git.sytue.eu/sytue/one/apps/fyne/tema"
	"git.sytue.eu/sytue/one/apps/fyne/ui"
	"git.sytue.eu/sytue/one/apps/nucleo/formula"
	"git.sytue.eu/sytue/one/apps/nucleo/i18n"
)

// # El resumen a la carta
//
// Es `resumenBarra.ts` y `ResumenSeleccion.tsx`: el resumen del rango marcado enseña lo
// que cada cual elige —promedio, recuento, recuento numérico, mínimo, máximo y suma—,
// la suma sola por defecto, y se pulsa para elegir, como la barra de estado de Excel.
// Lo elegido es preferencia del DISPOSITIVO, no del libro: va a las preferencias de
// Fyne con la misma cadena que la web guarda en localStorage («sum,min»).

// Estadisticas es `SelectionStats` de `sheets/format.ts`.
type Estadisticas struct {
	// Recuento son las celdas no vacías, también el texto (criterio de Excel).
	Recuento int
	// Numeros son las numéricas («Recuento numérico»).
	Numeros int
	// Suma, Media, Min y Max son de las numéricas; la media va a seis decimales.
	Suma, Media, Min, Max float64
}

// resumenDe es `selectionStats`: ok es false si no hay ningún número, que es cuando la
// web no enseña nada. Los errores y los booleanos ocupan celda pero no entran en la
// aritmética.
func resumenDe(vals []formula.Value) (Estadisticas, bool) {
	e := Estadisticas{Min: math.Inf(1), Max: math.Inf(-1)}
	for _, v := range vals {
		if s, es := v.(string); es && s == "" {
			continue
		}
		e.Recuento++
		if n, es := v.(float64); es && !math.IsNaN(n) && !math.IsInf(n, 0) {
			e.Numeros++
			e.Suma += n
			e.Min = min(e.Min, n)
			e.Max = max(e.Max, n)
		}
	}
	if e.Numeros == 0 {
		return Estadisticas{}, false
	}
	// Seis decimales como en la web, y los números enormes tal cual: multiplicarlos por
	// 1e6 los sacaría del entero seguro. `Floor(x+0,5)` y no `Round`, que es como
	// redondea `Math.round` los negativos (−2,5 → −2).
	e.Media = e.Suma / float64(e.Numeros)
	if math.Abs(e.Media) < 1e9 {
		e.Media = math.Floor(e.Media*1e6+0.5) / 1e6
	}
	return e, true
}

// MedidaResumen es `MedidaResumen`: lo que puede enseñar el resumen.
type MedidaResumen string

// Las medidas, con los mismos nombres que guarda la web.
const (
	MedidaPromedio MedidaResumen = "average"
	MedidaRecuento MedidaResumen = "count"
	MedidaNumeros  MedidaResumen = "numbers"
	MedidaMin      MedidaResumen = "min"
	MedidaMax      MedidaResumen = "max"
	MedidaSuma     MedidaResumen = "sum"
)

// MedidasResumen son todas, en el orden en que se pintan y se ofrecen: el de Excel.
var MedidasResumen = []MedidaResumen{MedidaPromedio, MedidaRecuento, MedidaNumeros, MedidaMin, MedidaMax, MedidaSuma}

// ResumenPorDefecto es lo que se ve sin haber elegido nada.
var ResumenPorDefecto = []MedidaResumen{MedidaSuma}

// Las claves de las preferencias del dispositivo.
const (
	// Con guiones y no con puntos: con forma de clave i18n, el guardián de catálogos las
	// tomaría por textos que traducir (como `sytue-notes-orden` de Notes).
	prefResumen      = "sytue-calcus-resumen"
	prefFxDesplegada = "sytue-calcus-fx-desplegada"
	separadorMedidas = ","
	separadorResumen = " · "
)

// LeerMedidas es `leerMedidas`: lo que no se entiende se ignora, los repetidos cuentan
// una vez y el orden es el de MedidasResumen. Sin nada válido, el de por defecto: un
// resumen vacío no se podría pulsar para volver a elegir.
func LeerMedidas(s string) []MedidaResumen {
	pedidas := strings.Split(s, separadorMedidas)
	var out []MedidaResumen
	for _, m := range MedidasResumen {
		if slices.Contains(pedidas, string(m)) {
			out = append(out, m)
		}
	}
	if len(out) == 0 {
		return slices.Clone(ResumenPorDefecto)
	}
	return out
}

// AlternarMedida es `alternarMedida`: pone o quita m; quitar la última no hace nada.
func AlternarMedida(actual []MedidaResumen, m MedidaResumen) []MedidaResumen {
	quedan := slices.DeleteFunc(slices.Clone(actual), func(x MedidaResumen) bool { return x == m })
	if len(quedan) == len(actual) {
		quedan = append(quedan, m)
	}
	if len(quedan) == 0 {
		return slices.Clone(actual)
	}
	var out []MedidaResumen
	for _, x := range MedidasResumen {
		if slices.Contains(quedan, x) {
			out = append(out, x)
		}
	}
	return out
}

func unirMedidas(m []MedidaResumen) string {
	s := make([]string, len(m))
	for i, x := range m {
		s[i] = string(x)
	}
	return strings.Join(s, separadorMedidas)
}

// prefsCalcus da el almacén del dispositivo. Es una variable para que las pruebas no se
// dejen nada recordado entre ellas: comparten una sola app.
var prefsCalcus = func() fyne.Preferences {
	if a := fyne.CurrentApp(); a != nil {
		return a.Preferences()
	}
	return nil
}

func medidasRecordadas() []MedidaResumen {
	if p := prefsCalcus(); p != nil {
		return LeerMedidas(p.String(prefResumen))
	}
	return slices.Clone(ResumenPorDefecto)
}

func recordarMedidas(m []MedidaResumen) {
	p := prefsCalcus()
	switch {
	case p == nil:
	case slices.Equal(m, ResumenPorDefecto):
		p.RemoveValue(prefResumen)
	default:
		p.SetString(prefResumen, unirMedidas(m))
	}
}

func fxDesplegadaRecordada() bool {
	p := prefsCalcus()
	return p != nil && p.Bool(prefFxDesplegada)
}

func recordarFxDesplegada(v bool) {
	p := prefsCalcus()
	switch {
	case p == nil:
	case v:
		p.SetBool(prefFxDesplegada, true)
	default:
		p.RemoveValue(prefFxDesplegada)
	}
}

// textoMedida es `textoMedida`: «Suma: 6», «Recuento numérico: 3».
func (c *Lienzo) textoMedida(m MedidaResumen, e Estadisticas) string {
	num := formula.FormatNumber
	switch m {
	case MedidaPromedio:
		return c.t("sheet.stats.average", i18n.Vars{"value": num(e.Media)})
	case MedidaRecuento:
		return c.t("sheet.stats.count", i18n.Vars{"count": e.Recuento})
	case MedidaNumeros:
		return c.t("sheet.stats.numbers", i18n.Vars{"count": e.Numeros})
	case MedidaMin:
		return c.t("sheet.stats.min", i18n.Vars{"value": num(e.Min)})
	case MedidaMax:
		return c.t("sheet.stats.max", i18n.Vars{"value": num(e.Max)})
	default:
		return c.t("sheet.stats.sum", i18n.Vars{"value": num(e.Suma)})
	}
}

// estadisticas son las del rango marcado; ok es false sin rango o sin números.
func (p *Pestanas) estadisticas() (Estadisticas, bool) {
	r := p.lz.Rejilla
	if r.ext == nil {
		return Estadisticas{}, false
	}
	return p.lz.Libro.Resumen(r.Rango())
}

// Medidas son las que enseña ahora el resumen.
func (p *Pestanas) Medidas() []MedidaResumen { return slices.Clone(p.medidas) }

// AlternarMedida pone o quita una medida del resumen, la recuerda en el dispositivo y
// repinta la barra y el menú si está abierto.
func (p *Pestanas) AlternarMedida(m MedidaResumen) {
	p.medidas = AlternarMedida(p.medidas, m)
	recordarMedidas(p.medidas)
	p.Refresh()
	if p.menu != nil && p.menu.Abierto() {
		if e, ok := p.estadisticas(); ok {
			p.menu.Reemplazar(p.elementosResumen(e)...)
		}
	}
}

// AbrirMenuResumen despliega las seis medidas con su cifra y su tic, hacia arriba y
// pegado a la derecha del resumen (`<Dropdown align="end" side="top">`).
func (p *Pestanas) AbrirMenuResumen() {
	e, ok := p.estadisticas()
	if !ok || (p.menu != nil && p.menu.Abierto()) {
		return
	}
	m := ui.NuevoMenu(p.elementosResumen(e)...)
	m.Arriba, m.Rotulo = true, p.lz.t("sheet.stats.menu", nil)
	p.menu = m
	m.Mostrar(p.resumen)
}

func (p *Pestanas) elementosResumen(e Estadisticas) []fyne.CanvasObject {
	out := make([]fyne.CanvasObject, 0, len(MedidasResumen))
	for _, m := range MedidasResumen {
		el := ui.NuevoElementoMenu(p.lz.textoMedida(m, e), iconos.Ninguno, func() { p.AlternarMedida(m) })
		el.Denso, el.MantieneAbierto = true, true
		el.Marcada = slices.Contains(p.medidas, m)
		// La última que queda no se puede quitar (el resumen se quedaría sin nada que
		// pulsar): pulsarla no hace nada (`AlternarMedida`), pero se pinta con la tinta
		// de las demás. Apagada era la ÚNICA elegida la que salía en gris, a ~2,5:1
		// (auditoría del 2026-11-24).
		out = append(out, el)
	}
	return out
}

// MenuResumen es el menú del resumen si está abierto (para las pruebas).
func (p *Pestanas) MenuResumen() *ui.Menu {
	if p.menu == nil || !p.menu.Abierto() || p.menu.Rotulo != p.lz.t("sheet.stats.menu", nil) {
		return nil
	}
	return p.menu
}

// ---- el disparador ----

// botonResumen es el `<button>` del resumen: el texto en `text-muted` (`text` al pasar
// por encima) y el ▾ de 12, con `flex h-6 items-center gap-1 rounded-sm px-1`.
type botonResumen struct {
	ui.Pulsable

	texto string
}

var _ fyne.Widget = (*botonResumen)(nil)

func nuevoBotonResumen(alPulsar func()) *botonResumen {
	b := &botonResumen{}
	b.AlPulsar = alPulsar
	b.ExtendBaseWidget(b)
	return b
}

// MedidaBotonResumen es lo que ocupa con ese texto: `px-1`, el texto, `gap-1` y el ▾, de
// alto `h-6` (44 con el dedo).
func MedidaBotonResumen(texto string) fyne.Size {
	ancho := 2*tema.Espacio(1) + ui.Medir(texto, EscalonPestanas, ui.Normal).Width + tema.Espacio(1) + LadoIconoChevron
	return fyne.NewSize(ancho, ui.SueloTactil(tema.Espacio(6)))
}

func (b *botonResumen) MinSize() fyne.Size {
	b.ExtendBaseWidget(b)
	return MedidaBotonResumen(b.texto)
}

func (b *botonResumen) CreateRenderer() fyne.WidgetRenderer {
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	r := &botonResumenRenderer{
		b:     b,
		aro:   ui.NuevoAro(tema.RadioSM, ui.SinHueco, v),
		texto: ui.Texto("", EscalonPestanas, ui.Normal, tema.Color("text-muted", v)),
		icono: iconos.Trazo(iconos.ChevronAbajo, LadoIconoChevron, "text-muted", v),
	}
	r.objetos = r.aro.Antes(r.texto, r.icono.Objeto())
	r.Refresh()
	return r
}

type botonResumenRenderer struct {
	b       *botonResumen
	aro     *ui.Aro
	texto   *canvas.Text
	icono   *iconos.Dibujo
	objetos []fyne.CanvasObject
}

func (r *botonResumenRenderer) MinSize() fyne.Size           { return r.b.MinSize() }
func (r *botonResumenRenderer) Objects() []fyne.CanvasObject { return r.objetos }
func (r *botonResumenRenderer) Destroy()                     {}

// Layout recorta el texto con elipsis si no cabe (`truncate`) y deja el ▾ a su derecha.
func (r *botonResumenRenderer) Layout(t fyne.Size) {
	r.aro.Colocar(t)
	m := tema.Espacio(1)
	cabe := max(t.Width-2*m-m-LadoIconoChevron, 0)
	ancho := ui.Truncar(r.texto, r.b.texto, EscalonPestanas, ui.Normal, cabe)
	ui.ColocarTexto(r.texto, EscalonPestanas, m, (t.Height-EscalonPestanas.Linea)/2)
	r.icono.Colocar(m+ancho+m, (t.Height-LadoIconoChevron)/2, LadoIconoChevron)
}

func (r *botonResumenRenderer) Refresh() {
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	token := "text-muted"
	if r.b.Encima() {
		token = "text"
	}
	r.texto.Color = tema.Color(token, v)
	r.icono.Tenir(token, v)
	r.aro.Pintar(v)
	r.aro.Ver(r.b.Enfocado())
	r.Layout(r.b.Size())
	canvas.Refresh(r.texto)
	canvas.Refresh(r.icono.Objeto())
}
