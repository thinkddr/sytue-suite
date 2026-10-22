// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package erp

import (
	"context"
	"fmt"
	"image/color"
	"math"
	"regexp"
	"strconv"
	"strings"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"

	"git.sytue.eu/sytue/one/apps/fyne/tema"
	"git.sytue.eu/sytue/one/apps/fyne/ui"
	"git.sytue.eu/sytue/one/apps/nucleo/api"
)

// # Los campos de la ficha (`CampoDeFormulario` y `formato.ts` de la web)
//
// El control sale del TIPO declarado en el catálogo, no de código por módulo. Y lo que
// viaja al servicio es su contrato: el dinero en CÉNTIMOS enteros, los decimales como
// cadena canónica con punto, las fechas en AAAA-MM-DD y las fechas con hora en RFC3339.
//
// Medido en la ficha del arnés (2026-09-23): cada campo es su rótulo (`text-xs
// font-medium`, 14), un `mb-1` y el control de 31,75 —49,25 en total—; el selector mide
// 32 y el área de notas (`rows=3`) 63,25. Entre campos, `space-y-3`.

// Medidas de un campo de la ficha.
const (
	// RotuloCampoMB es el `mb-1` del rótulo.
	RotuloCampoMB float32 = 1 * 0.25 * tema.Rem
	// SeparaCampos es el `space-y-3` entre campos.
	SeparaCampos float32 = 3 * 0.25 * tema.Rem
	// ErrorCampoMT es el `mt-1` del error de un campo.
	ErrorCampoMT float32 = 1 * 0.25 * tema.Rem
	// AsteriscoML es el `ml-0.5` del asterisco de obligatorio.
	AsteriscoML float32 = 0.5 * 0.25 * tema.Rem
	// FilasNotas es el `rows={3}` del área de texto largo.
	FilasNotas = 3
	// HuecoBajoArea es el descendente que deja el textarea en línea a text-xs:
	// Chromium remoto con Figtree y rem16 mide6px (Notas mide98px en total).
	HuecoBajoArea float32 = 6
)

// ACentimos es `aCentimos` de formato.ts: de lo que se escribe en un importe a
// céntimos, SIN pasar por un flotante —1234,56 × 100 da 123455,99999 en binario y
// truncar eso pierde un céntimo por factura—. Admite coma o punto, y el tercer
// decimal redondea al modo comercial. El segundo valor es falso si no es un importe.
func ACentimos(texto string) (int64, bool) {
	limpio := strings.Replace(strings.Join(strings.Fields(texto), ""), ",", ".", 1)
	if limpio == "" || limpio == "-" || limpio == "." || !reImporte.MatchString(limpio) {
		return 0, false
	}
	negativo := strings.HasPrefix(limpio, "-")
	entera, decimal, _ := strings.Cut(strings.TrimPrefix(limpio, "-"), ".")
	if entera == "" {
		entera = "0"
	}
	dos := (decimal + "00")[:2]
	arrastre := int64(0)
	if len(decimal) > 2 && decimal[2] >= '5' {
		arrastre = 1
	}
	e, err := strconv.ParseInt(entera, 10, 64)
	if err != nil {
		return 0, false
	}
	d, _ := strconv.ParseInt(dos, 10, 64)
	total := e*100 + d + arrastre
	if negativo {
		total = -total
	}
	return total, true
}

var reImporte = regexp.MustCompile(`^-?\d*(\.\d*)?$`)

// DeCentimos es `deCentimos`: de céntimos al texto que se edita, con dos decimales y
// PUNTO (el `<input type="number">` de la web no lleva otra cosa).
func DeCentimos(c int64) string {
	signo := ""
	if c < 0 {
		signo, c = "-", -c
	}
	return fmt.Sprintf("%s%d.%02d", signo, c/100, c%100)
}

// ValoresIniciales es `valoresIniciales`: del registro a los textos que edita la ficha.
// Con `r` nil, todo en blanco. Las fechas se escriben como las lee `ui.LeerFecha`
// (día delante), que es lo que se teclea en los 26 idiomas.
func ValoresIniciales(m Modelo, r *Registro) map[string]string {
	out := make(map[string]string, len(m.Campos))
	for _, c := range m.Campos {
		out[c.Clave] = ""
		if r == nil {
			continue
		}
		v, hay := r.Data[c.Clave]
		if !hay || v == nil {
			continue
		}
		out[c.Clave] = textoEditable(c, v)
	}
	return out
}

func textoEditable(c Campo, v any) string {
	switch c.Tipo {
	case api.ERPDinero:
		if n, ok := v.(float64); ok {
			return DeCentimos(int64(math.Round(n)))
		}
	case api.ERPFecha:
		if t, ok := fechaISO(texto(v)); ok {
			return ui.EscribirFecha(t)
		}
	case api.ERPFechaHora:
		if t, err := time.Parse(time.RFC3339, texto(v)); err == nil {
			// `datetime-local` no lleva zona: se enseña en la hora de quien mira.
			t = t.Local()
			return ui.EscribirFecha(t) + " " + ui.EscribirHora(t)
		}
	case api.ERPBooleano:
		if b, ok := v.(bool); ok {
			return strconv.FormatBool(b)
		}
	case api.ERPEntero:
		if n, ok := entero(v); ok {
			return strconv.FormatInt(n, 10)
		}
	}
	return texto(v)
}

// ACuerpo es `aCuerpo`: de los textos de la ficha al cuerpo que espera el servicio. Lo
// vacío no viaja, y lo que no se puede leer TAMPOCO —como en la web, un importe que no
// es un importe se queda fuera y el servicio dice si faltaba—.
func ACuerpo(m Modelo, valores map[string]string, zona *time.Location) map[string]any {
	data := map[string]any{}
	for _, c := range m.Campos {
		bruto := strings.TrimSpace(valores[c.Clave])
		if bruto == "" {
			continue
		}
		switch c.Tipo {
		case api.ERPDinero:
			if n, ok := ACentimos(bruto); ok {
				data[c.Clave] = n
			}
		case api.ERPEntero:
			if n, ok := enteroTecleado(bruto); ok {
				data[c.Clave] = n
			}
		case api.ERPDecimal:
			data[c.Clave] = strings.Replace(bruto, ",", ".", 1)
		case api.ERPBooleano:
			data[c.Clave] = bruto == "true"
		case api.ERPFecha:
			if t, _, ok := ui.LeerFecha(bruto, time.UTC); ok {
				data[c.Clave] = t.Format(time.DateOnly)
			}
		case api.ERPFechaHora:
			if t, ok := leerFechaHora(bruto, zona); ok {
				data[c.Clave] = t.UTC().Format(time.RFC3339)
			}
		default:
			data[c.Clave] = bruto
		}
	}
	return data
}

// enteroTecleado es el `parseInt(bruto, 10)` de la web: los dígitos del principio.
func enteroTecleado(s string) (int64, bool) {
	fin := 0
	for i, r := range s {
		if (r == '-' || r == '+') && i == 0 {
			fin = i + 1
			continue
		}
		if r < '0' || r > '9' {
			break
		}
		fin = i + 1
	}
	n, err := strconv.ParseInt(s[:fin], 10, 64)
	return n, err == nil
}

// leerFechaHora lee «31/12/2026 09:30» en la zona de quien mira.
func leerFechaHora(s string, zona *time.Location) (time.Time, bool) {
	fecha, hora, _ := strings.Cut(strings.TrimSpace(s), " ")
	d, _, ok := ui.LeerFecha(fecha, zona)
	if !ok {
		return time.Time{}, false
	}
	h, m, vacia, ok := ui.LeerHora(strings.TrimSpace(hora))
	if !vacia && !ok {
		return time.Time{}, false
	}
	return d.Add(time.Duration(h)*time.Hour + time.Duration(m)*time.Minute), true
}

// --- El control ---------------------------------------------------------------

// Control es un campo de la ficha: el widget de su tipo con su rótulo. Se crea UNA vez
// al abrir la ficha y sobrevive a los repintados, como el buscador de la lista: si no,
// escribir la segunda letra perdería el foco.
type Control struct {
	Campo Campo
	// obj es el control en sí; alto, lo que mide.
	obj  fyne.CanvasObject
	alto float32

	entrada  *ui.Entrada
	area     *ui.AreaTexto
	selector *ui.Selector
	casilla  *ui.Eleccion
	relacion *SelectorRelacion
}

// nuevoControl crea el control del tipo del campo. `cambio` recibe el valor nuevo y,
// en las relaciones, el nombre legible del elegido.
func nuevoControl(c Campo, valor string, f *Ficha) *Control {
	k := &Control{Campo: c}
	cambio := func(v string) { f.fijar(c.Clave, v) }
	switch c.Tipo {
	case api.ERPTextoLargo:
		a := ui.NuevaAreaTexto("")
		a.Filas = FilasNotas
		a.Propia = &CampoFicha
		a.PonerTexto(valor)
		a.AlCambiar(cambio)
		k.area, k.obj, k.alto = a, a, a.MinSize().Height
	case api.ERPSeleccion:
		opciones := make([]string, 1, 1+len(c.Opciones))
		elegida := 0
		for i, o := range c.Opciones {
			opciones = append(opciones, o.Etiqueta)
			if o.Valor == valor {
				elegida = i + 1
			}
		}
		s := ui.NuevoSelector(opciones, func(i int) {
			v := ""
			if i > 0 {
				v = c.Opciones[i-1].Valor
			}
			cambio(v)
		})
		s.Propio = selectorPropio(CampoFicha, AltoSelectNativo)
		s.Elegida = elegida
		k.selector, k.obj, k.alto = s, s, AltoSelectNativo
	case api.ERPBooleano:
		cs := ui.NuevaCasilla(c.Etiqueta, func(si bool) { cambio(strconv.FormatBool(si)) })
		cs.SinEtiqueta, cs.Marcada = true, valor == "true"
		k.casilla, k.obj, k.alto = cs, cs, ui.LadoEleccion
	case api.ERPRelacion:
		r := NuevoSelectorRelacion(f.tr, c.Modelo, valor, f.Rotulos[valor], func(v, rotulo string) {
			if rotulo != "" {
				f.Rotulos[v] = rotulo
			}
			cambio(v)
		})
		r.Buscar = func(ctx context.Context, modelo, q string) ([]Registro, error) {
			if f.BuscarRelacion == nil {
				return nil, nil
			}
			return f.BuscarRelacion(ctx, modelo, q)
		}
		k.relacion, k.obj, k.alto = r, r, CampoFicha.Alto()
	default:
		var e *ui.Entrada
		switch c.Tipo {
		case api.ERPFecha:
			e = ui.CampoFecha(ui.CampoMD).ConPropia(CampoFicha)
		case api.ERPFechaHora:
			e = ui.NuevaEntrada("31/12/2026 09:30").ConPropia(CampoFicha)
			e.Icono = ui.CampoFecha(ui.CampoMD).Icono
		default:
			e = ui.NuevaEntrada("").ConPropia(CampoFicha)
		}
		e.PonerTexto(valor)
		e.AlCambiar(cambio)
		k.entrada, k.obj, k.alto = e, e, CampoFicha.Alto()
	}
	return k
}

// Los campos de la ficha no son `<Input>` de Form.tsx: son `<input>` escritos a mano con
// clases (`border-border bg-bg px-3 py-2 text-sm`), así que van con `ui.CampoPropio`.
var (
	// CampoFicha es el de los campos de la ficha: 31,75 de alto, medido.
	CampoFicha = ui.CampoPropio{
		MargenX: tema.Espacio(3), MargenY: tema.Espacio(2), Texto: tema.TextoSM, Borde: "border", Relleno: "bg",
	}
	// CampoBuscadorLista es el buscador de la lista: `bg-surface px-3 py-1.5`, 28,25.
	CampoBuscadorLista = ui.CampoPropio{
		MargenX: tema.Espacio(3), MargenY: tema.Espacio(1.5), Texto: tema.TextoSM, Borde: "border", Relleno: "surface",
	}
	// CampoLote es el `<select>` de «Cambiar estado» de la barra de lote: `bg-surface
	// rounded px-2 py-1 text-xs` y sin borde (el color del borde es el del fondo).
	CampoLote = ui.CampoPropio{
		MargenX: tema.Espacio(2), MargenY: tema.Espacio(1), Texto: tema.TextoXS, Borde: "surface",
		Relleno: "surface", Radio: tema.RadioSM,
	}
	// CampoTarea es el del editor de tareas y la nota del historial: `px-2 py-1.5`, 28,25.
	CampoTarea = ui.CampoPropio{
		MargenX: tema.Espacio(2), MargenY: tema.Espacio(1.5), Texto: tema.TextoSM, Borde: "border", Relleno: "bg",
	}
)

// AltoSelectNativo es el `<select>` de la ficha: con `line-height: normal` mide 32 y
// no 31,75. Medido.
const AltoSelectNativo float32 = 32

// AltoSelectLote es el select de la barra de lote: Chromium remoto mide24px
// con Figtree12 y padding vertical4px, sin borde (2026-10-04).
const AltoSelectLote float32 = 24

// AnchoSelectLote es el ancho de ese `<select>`: su opción más ancha con el `px-2` y el
// hueco de la flecha del navegador, sin borde. Con los estados de una factura, 90.
func AnchoSelectLote(opciones []string) float32 {
	var mayor float32
	for _, o := range opciones {
		mayor = max(mayor, ui.Medir(o, tema.TextoXS, ui.Normal).Width)
	}
	return mayor + 2*CampoLote.MargenX + ui.FlechaSelectNativo
}

// selectorPropio es un `<select>` de la clase `c` con la flecha del navegador.
func selectorPropio(c ui.CampoPropio, alto float32) *ui.SelectorPropio {
	return &ui.SelectorPropio{Campo: c, Alto: alto}
}

// enfocable es lo que recibe el foco al abrir la ficha (el `querySelector('input,
// select, textarea').focus()` de la web): el primer control que se teclea.
func (k *Control) enfocable() fyne.Focusable {
	switch {
	case k.entrada != nil:
		return k.entrada
	case k.area != nil:
		return k.area
	case k.selector != nil:
		return k.selector
	case k.casilla != nil:
		return k.casilla
	}
	return nil
}

// marcarError pinta el borde en `danger` del control que falló.
func (k *Control) marcarError(si bool) {
	switch {
	case k.entrada != nil:
		k.entrada.Invalida = si
		k.entrada.Refresh()
	case k.area != nil:
		k.area.Invalida = si
		k.area.Refresh()
	case k.selector != nil:
		k.selector.Invalida = si
		k.selector.Refresh()
	}
}

// ponerValor escribe un valor desde fuera (Sytue AI rellenando la ficha).
func (k *Control) ponerValor(v string) {
	switch {
	case k.entrada != nil:
		k.entrada.PonerTexto(v)
	case k.area != nil:
		k.area.PonerTexto(v)
	case k.casilla != nil:
		k.casilla.Marcada = v == "true"
		k.casilla.Refresh()
	case k.selector != nil:
		k.selector.Elegida = 0
		for i, o := range k.Campo.Opciones {
			if o.Valor == v {
				k.selector.Elegida = i + 1
			}
		}
		k.selector.Refresh()
	}
}

// bloqueCampo es un campo entero —rótulo, asterisco, control y error— con su alto.
func (f *Ficha) bloqueCampo(v tema.Variante, k *Control) (fyne.CanvasObject, float32) {
	control, altoControl := k.obj, k.alto
	if k.relacion != nil && f.CrearRelacion != nil && f.PuedeCrearRelacion != nil && f.PuedeCrearRelacion(k.Campo.Modelo) {
		nuevo := ui.NuevoBoton(f.t("action.new"), func() {
			k.relacion.cerrar()
			f.CrearRelacion(k.Campo.Modelo, func(r Registro) { k.relacion.elegir(r) })
		}).Con(ui.Fantasma, ui.TallaSM)
		nuevo.Inerte = f.Guardando
		control = container.New(&relacionConAlta{alto: k.alto}, k.obj, nuevo)
		altoControl += nuevo.MinSize().Height
	}
	b := &campoFicha{control: control, altoControl: altoControl}
	b.rotulo = ui.Texto(k.Campo.Etiqueta, tema.TextoXS, ui.Medio, tema.Color("text-muted", v))
	b.anchoRotulo = ui.Medir(k.Campo.Etiqueta, tema.TextoXS, ui.Medio).Width
	objs := []fyne.CanvasObject{b.rotulo}
	if k.Campo.Requerido {
		b.asterisco = ui.Texto("*", tema.TextoXS, ui.Medio, tema.Color("danger-text", v))
		objs = append(objs, b.asterisco)
	}
	objs = append(objs, control)
	alto := tema.TextoXS.Linea + RotuloCampoMB + altoControl
	if k.area != nil {
		// El `<textarea>` es `inline-block` y va en la línea base: deja debajo el
		// descendente de la línea. Medido: 86 el campo de notas y no 80,75.
		alto += HuecoBajoArea
	}
	if e := f.Errores[k.Campo.Clave]; e != "" {
		b.error = ui.NuevoParrafo(e, tema.TextoXS, ui.Normal, "danger-text", v)
		b.altoError = ui.AltoParrafo(b.error, f.anchoCampos())
		objs = append(objs, b.error)
		alto += ErrorCampoMT + b.altoError
	}
	k.marcarError(f.Errores[k.Campo.Clave] != "")
	b.alto = alto
	return container.New(b, objs...), alto
}

// campoFicha coloca un campo: rótulo arriba, control a todo el ancho y el error debajo.
type campoFicha struct {
	rotulo, asterisco *canvas.Text
	anchoRotulo       float32
	control           fyne.CanvasObject
	altoControl       float32
	error             *ui.Parrafo
	altoError         float32
	alto              float32
}

func (c *campoFicha) MinSize([]fyne.CanvasObject) fyne.Size { return fyne.NewSize(0, c.alto) }

func (c *campoFicha) Layout(_ []fyne.CanvasObject, t fyne.Size) {
	ui.ColocarTexto(c.rotulo, tema.TextoXS, 0, 0)
	if c.asterisco != nil {
		ui.ColocarTexto(c.asterisco, tema.TextoXS, c.anchoRotulo+AsteriscoML, 0)
	}
	y := tema.TextoXS.Linea + RotuloCampoMB
	ancho := t.Width
	if _, casilla := c.control.(*ui.Eleccion); casilla {
		ancho = ui.LadoEleccion
	}
	c.control.Resize(fyne.NewSize(ancho, c.altoControl))
	c.control.Move(fyne.NewPos(0, y))
	if c.error != nil {
		c.error.Resize(fyne.NewSize(t.Width, c.altoError))
		c.error.Move(fyne.NewPos(0, y+c.altoControl+ErrorCampoMT))
	}
}

// sinColor es el transparente, para las cajas que solo llevan borde o nada.
var sinColor = color.NRGBA{}

// relacionConAlta mantiene el selector completo y la acción ligera bajo él.
type relacionConAlta struct{ alto float32 }

func (r *relacionConAlta) MinSize(obj []fyne.CanvasObject) fyne.Size {
	return fyne.NewSize(obj[1].MinSize().Width, r.alto+obj[1].MinSize().Height)
}

func (r *relacionConAlta) Layout(obj []fyne.CanvasObject, t fyne.Size) {
	obj[0].Move(fyne.NewPos(0, 0))
	obj[0].Resize(fyne.NewSize(t.Width, r.alto))
	obj[1].Move(fyne.NewPos(0, r.alto))
	obj[1].Resize(obj[1].MinSize())
}
