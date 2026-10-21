// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package calcus

import (
	"context"
	"encoding/json"
	"errors"
	"strconv"
	"strings"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"

	"git.sytue.eu/sytue/one/apps/fyne/editor"
	"git.sytue.eu/sytue/one/apps/fyne/iconos"
	"git.sytue.eu/sytue/one/apps/fyne/tema"
	"git.sytue.eu/sytue/one/apps/fyne/ui"
	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/formula"
	"git.sytue.eu/sytue/one/apps/nucleo/formula/analisis"
	"git.sytue.eu/sytue/one/apps/nucleo/i18n"
)

// El RELLENO CON IA de Datos: `AiFillMenu` de Datos.tsx. Sytue AI ve las filas que ya
// tienen valor en la columna como ejemplos y propone el resto; lo propuesto se enseña
// fila a fila y no se escribe nada hasta pulsar «Escribir». Qué se manda y cómo se lee
// es `analisis.PedirRelleno`/`LeerRelleno`, con los vectores de la web.

// CmdRellenar es `fillcells`: JSON {values: {ref: texto}}, lo que la persona confirmó.
const CmdRellenar editor.Comando = "fillcells"

// plazoRelleno es el tope de la consulta: el `TIMEOUT_MS` del BFF de la web.
const plazoRelleno = 70 * time.Second

// ConsultaIA manda a Sytue AI una instrucción con su contexto y devuelve el texto.
type ConsultaIA func(ctx context.Context, instruccion, contexto string) (string, error)

// mandarRelleno ejecuta `fillcells`: escribe los valores como texto crudo, conservando
// el formato de cada celda. Nunca una fórmula, como la web. false si no es él o no
// cambia nada.
func (c *Lienzo) mandarRelleno(cmd editor.Comando, valor string) bool {
	if cmd != CmdRellenar {
		return false
	}
	var req struct {
		Values map[string]any `json:"values"`
	}
	_ = json.Unmarshal([]byte(valor), &req)
	vals := map[Celda]string{}
	for ref, v := range req.Values {
		s, ok := v.(string)
		if k, okc := celdaUnica(ref); okc && ok && strings.TrimSpace(s) != "" && !strings.HasPrefix(s, "=") {
			vals[k] = s
		}
	}
	return len(vals) > 0 && c.Libro.escribirCrudos(vals)
}

// claveErrorIA es `aiErrorKey` para lo que devuelve el cliente nativo.
func claveErrorIA(err error) string {
	var e *api.Error
	switch {
	case api.IANoConfigurada(err):
		return "ai.error.notConfigured"
	case errors.Is(err, context.DeadlineExceeded):
		return "ai.error.timeout"
	case errors.As(err, &e) && e.Status == 401:
		return "ai.error.unauthenticated"
	default:
		return "ai.error.unreachable"
	}
}

// botonRelleno es el botón del grupo Asistente de «Fórmulas»; nil sin servicio de IA, como la
// web sin `aiEnabled`.
func (lz *Lienzo) botonRelleno(c *editor.Contexto) fyne.CanvasObject {
	if lz.IA == nil {
		return nil
	}
	var b *editor.BotonCinta
	b = editor.NuevoBotonCinta(iconos.EdIA, c.Traducir("data.aiFill"), func() { lz.abrirRelleno(c, b) })
	b.Inerte = c.SoloLectura
	return b
}

// formRelleno es el estado del panel.
type formRelleno struct {
	rango, columna, pista string
	ocupado               bool
	fallo                 string
	propuesta             [][2]string // celda y valor, en el orden de las filas
	// vuelta cuenta las consultas: la respuesta de una que ya no es la última (se cerró
	// el panel o se cambió algo) no se enseña.
	vuelta int
	parar  context.CancelFunc
}

func (f *formRelleno) olvidar() {
	f.vuelta++
	if f.parar != nil {
		f.parar()
		f.parar = nil
	}
	f.ocupado, f.fallo, f.propuesta = false, "", nil
}

// peticion es lo que se pediría con lo escrito, y las cabeceras del rango.
func (lz *Lienzo) peticionRelleno(f *formRelleno) (analisis.PeticionRelleno, []string, formula.Rect, bool) {
	r, ok := formula.ParseRangeRef(f.rango)
	rejilla, _ := lz.Libro.TextosRango(f.rango)
	cab := cabecerasDe(rejilla)
	col, _ := strconv.Atoi(f.columna)
	if !ok || col >= len(cab) {
		return analisis.PeticionRelleno{}, cab, r, false
	}
	p, ok := analisis.PedirRelleno(cab, rejilla[1:], r.R1+2, col, f.pista)
	return p, cab, r, ok
}

// abrirRelleno es `AiFillMenu`.
func (lz *Lienzo) abrirRelleno(c *editor.Contexto, disparador *editor.BotonCinta) {
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	f := &formRelleno{rango: rangoTexto(lz.Rejilla)}
	if r, ok := formula.ParseRangeRef(f.rango); ok {
		f.columna = strconv.Itoa(r.C2 - r.C1) // la última: la que suele quedar por rellenar
	}
	var vivo *emergenteVivo
	proponer := func() {
		p, _, r, ok := lz.peticionRelleno(f)
		if !ok || f.ocupado {
			return
		}
		f.olvidar()
		f.ocupado = true
		vuelta, col := f.vuelta, r.C1
		if n, err := strconv.Atoi(f.columna); err == nil {
			col += n
		}
		ctx, parar := context.WithTimeout(context.Background(), plazoRelleno)
		f.parar = parar
		vivo.rehacer()
		consulta := lz.IA
		lz.lanzar(func() {
			texto, err := consulta(ctx, p.Instruccion, p.Contexto)
			fyne.Do(func() {
				if vuelta != f.vuelta {
					return
				}
				f.ocupado, f.parar = false, nil
				parar()
				leido := analisis.LeerRelleno(texto, p.Pendientes)
				switch {
				case err != nil:
					f.fallo = c.Traducir(claveErrorIA(err))
				case strings.TrimSpace(texto) == "":
					f.fallo = c.Traducir("ai.error.badResponse")
				case len(leido) == 0:
					f.fallo = c.Traducir("data.aiFill.none")
				}
				for _, n := range p.Pendientes {
					if x, ok := leido[n]; ok && err == nil {
						f.propuesta = append(f.propuesta, [2]string{Ref(col, n-1), x})
					}
				}
				vivo.rehacer()
			})
		})
	}
	cambia := func(p *string) func(string) {
		return func(s string) { *p = s; f.olvidar(); vivo.rehacer() }
	}
	eRango := entradaPanel("A1:C20", f.rango, cambia(&f.rango), nil)
	ePista := entradaPanel(c.Traducir("data.aiFill.instructionPlaceholder"), "", cambia(&f.pista), proponer)
	vivo = abrirVivo(disparador, anchoPanelAnalisis, func(e *emergenteVivo) []fyne.CanvasObject {
		return lz.cuerpoRelleno(c, f, e, v, eRango, ePista, proponer, cambia(&f.columna))
	})
	cerrar := vivo.em.AlRecogerse
	vivo.em.AlRecogerse = func() { f.olvidar(); cerrar() }
}

// cuerpoRelleno es lo que se rehace al cambiar algo.
func (lz *Lienzo) cuerpoRelleno(c *editor.Contexto, f *formRelleno, e *emergenteVivo, v tema.Variante,
	eRango, ePista *ui.Entrada, proponer func(), alColumna func(string),
) []fyne.CanvasObject {
	p, cab, _, ok := lz.peticionRelleno(f)
	filas := []fyne.CanvasObject{
		rotuloPanel(c.Traducir("data.aiFill"), v),
		conPie(conEtiqueta(c.Traducir("data.rangeWithHeaders"), eRango), tema.Espacio(1.5)),
	}
	if len(cab) > 0 {
		campos := make([]editor.Opcion, len(cab))
		for i, h := range cab {
			campos[i] = editor.Opcion{Valor: strconv.Itoa(i), Rotulo: h}
			if strings.TrimSpace(h) == "" {
				campos[i].Rotulo = c.Traducir("common.columnN", i18n.Vars{"n": i + 1})
			}
		}
		// El `<select>` de PivotField mide 116 y no se estira: va a la izquierda.
		sel := editor.NuevoSelectorCinta(c.Traducir("data.aiFill.column"), anchoSelectorCampo, campos, f.columna, alColumna)
		filas = append(filas, conPie(conEtiqueta(c.Traducir("data.aiFill.column"), container.NewBorder(nil, nil, sel, nil)), tema.Espacio(1.5)))
	}
	aviso := "data.aiFill.privacy"
	if !ok && len(cab) > 0 {
		aviso = "data.aiFill.nothing"
	}
	filas = append(filas,
		conPie(conEtiqueta(c.Traducir("data.aiFill.instruction"), ePista), tema.Espacio(1.5)),
		conPie(meta(c.Traducir(aviso), v), tema.Espacio(2)))
	rotulo := "data.aiFill.propose"
	if f.ocupado {
		rotulo = "data.aiFill.busy"
	}
	pedir := botonPop(c.Traducir(rotulo), false, proponer)
	pedir.Inerte = !ok || f.ocupado || c.SoloLectura
	filas = append(filas, anchoEntero(pedir))
	if f.fallo != "" {
		filas = append(filas, conArriba(errorPanel(f.fallo, v), tema.Espacio(2)))
	}
	if len(f.propuesta) == 0 {
		return filas
	}
	lineas := make([]fyne.CanvasObject, len(f.propuesta))
	for i, pr := range f.propuesta {
		ref := ui.NuevoParrafo(pr[0], tema.TextoXS, ui.Normal, "text-muted", v)
		val := ui.NuevoParrafo(pr[1], tema.TextoXS, ui.Normal, "text", v)
		val.Lineas = 1
		lineas[i] = container.New(filaPropuesta{}, ref, val)
	}
	propuesta := f.propuesta
	escribir := botonPop(c.Traducir("data.aiFill.write"), false, func() {
		vals := map[string]string{}
		for _, pr := range propuesta {
			vals[pr[0]] = pr[1]
		}
		b, _ := json.Marshal(map[string]any{"values": vals})
		c.Mandar(CmdRellenar, string(b))
		e.em.Cerrar()
	})
	bloque := []fyne.CanvasObject{
		conPie(ui.NuevoParrafo(c.Traducir("data.aiFill.count", i18n.Vars{"count": len(f.propuesta), "total": len(p.Pendientes)}),
			tema.Texto2XS, ui.Normal, "text", v), tema.Espacio(1)),
		recuadro(topeAlto(altoListaRelleno-2*(tema.Borde+tema.Espacio(1.5)), container.New(ui.Columna{}, lineas...))),
		conArriba(rejillaBotones(2, escribir, botonPop(c.Traducir("action.cancel"), false, func() { e.em.Cerrar() })), tema.Espacio(2)),
	}
	return append(filas, resultadoPanel(container.New(ui.Columna{}, bloque...), v))
}

// altoListaRelleno es el `max-h-40` de la lista de lo propuesto.
var altoListaRelleno = tema.Espacio(40)

// filaPropuesta es `flex gap-2 py-px`: la celda en `w-10` y el valor, truncado, en lo
// que queda.
type filaPropuesta struct{}

func (filaPropuesta) MinSize(objs []fyne.CanvasObject) fyne.Size {
	return fyne.NewSize(tema.Espacio(12), max(objs[0].MinSize().Height, objs[1].MinSize().Height)+2)
}

func (filaPropuesta) Layout(objs []fyne.CanvasObject, t fyne.Size) {
	izq := tema.Espacio(10) + tema.Espacio(2)
	objs[0].Move(fyne.NewPos(0, 1))
	objs[0].Resize(fyne.NewSize(tema.Espacio(10), t.Height-2))
	objs[1].Move(fyne.NewPos(izq, 1))
	objs[1].Resize(fyne.NewSize(max(t.Width-izq, 0), t.Height-2))
}
