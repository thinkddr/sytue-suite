// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package rooms

import (
	"context"
	"image/color"
	"slices"
	"strconv"
	"strings"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/driver/desktop"
	"fyne.io/fyne/v2/widget"

	"git.sytue.eu/sytue/one/apps/fyne/iconos"
	"git.sytue.eu/sytue/one/apps/fyne/tema"
	"git.sytue.eu/sytue/one/apps/fyne/ui"
	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/i18n"
)

// En la web: apps/frontend/apps/rooms/src/components/CardView.tsx,
// CardComposerModal.tsx y lib/cards.ts.
//
// Un mensaje puede ser una ENCUESTA (la pregunta es el cuerpo y las opciones se votan)
// o una TAREA (el título es el cuerpo y se marca como hecha). El estado es de todos: el
// servicio devuelve el recuento agregado y la vista lo pinta.

// Tarjeta es la `MessageCard` de un mensaje, vista por quien mira.
type Tarjeta struct {
	// Tipo es "poll" o "task".
	Tipo     string
	Opciones []string
	Multi    bool
	Asignado string
	Vence    time.Time
	Votos    []Voto
}

// Voto es un `CardVote` con `mine`.
type Voto struct {
	Valor string
	Veces int
	Mio   bool
	Subs  []string
}

// tarjetaDe es la tarjeta del servicio vista por `yo`.
func tarjetaDe(c *api.RoomsCard, yo string) *Tarjeta {
	if c == nil || c.Kind == "" {
		return nil
	}
	out := &Tarjeta{Tipo: c.Kind, Opciones: c.Options, Multi: c.Multi, Asignado: c.AssigneeName}
	if c.Due != nil {
		out.Vence = *c.Due
	}
	for _, v := range c.Votes {
		out.Votos = append(out.Votos, Voto{Valor: v.Value, Veces: v.Count, Mio: slices.Contains(v.Subs, yo), Subs: v.Subs})
	}
	return out
}

// VotoDe es `voteOf`.
func (t *Tarjeta) VotoDe(valor string) Voto {
	for _, v := range t.Votos {
		if v.Valor == valor {
			return v
		}
	}
	return Voto{Valor: valor}
}

// Total es `totalVotes`.
func (t *Tarjeta) Total() int {
	n := 0
	for _, v := range t.Votos {
		n += v.Veces
	}
	return n
}

// CambiarMiVoto es `toggleMyCardVote`: el recuento tras marcar (poner) o retirar un
// valor. En una encuesta de respuesta única, marcar una retira mi voto de las demás; un
// valor que se queda sin votos desaparece.
func CambiarMiVoto(t *Tarjeta, valor string, poner bool, yo string) []api.RoomsCardVote {
	unica := t.Tipo == "poll" && !t.Multi
	var out []api.RoomsCardVote
	for _, v := range t.Votos {
		subs := slices.Clone(v.Subs)
		n := v.Veces
		if v.Mio && ((v.Valor == valor && !poner) || (v.Valor != valor && poner && unica)) {
			n = max(0, n-1)
			subs = slices.DeleteFunc(subs, func(s string) bool { return s == yo })
		}
		if v.Valor == valor && poner && !v.Mio {
			n++
			subs = append(subs, yo)
		}
		out = append(out, api.RoomsCardVote{Value: v.Valor, Count: n, Subs: subs})
	}
	if poner && !slices.ContainsFunc(t.Votos, func(v Voto) bool { return v.Valor == valor }) {
		out = append(out, api.RoomsCardVote{Value: valor, Count: 1, Subs: []string{yo}})
	}
	out = slices.DeleteFunc(out, func(v api.RoomsCardVote) bool { return v.Count <= 0 })
	slices.SortFunc(out, func(a, b api.RoomsCardVote) int { return strings.Compare(a.Value, b.Value) })
	return out
}

// ---- La vista ----

// Las medidas de CardView.tsx. HOJA.
var (
	// RellenoCajaTarjeta es el `p-2.5` de la caja, AireCajaTarjeta su `mt-1.5`.
	RellenoCajaTarjeta = tema.Espacio(2.5)
	AireCajaTarjeta    = tema.Espacio(1.5)
	// HuecoOpciones es el `gap-1.5` entre opciones; CanalonOpcionTarjeta y
	// RellenoOpcionTarjeta el `px-2.5 py-1.5` de cada una.
	HuecoOpciones        = tema.Espacio(1.5)
	CanalonOpcionTarjeta = tema.Espacio(2.5)
	RellenoOpcionTarjeta = tema.Espacio(1.5)
)

// VistaTarjeta es `CardView`: la encuesta con sus opciones votables y su barra de
// resultado, o la tarea con su casilla, su responsable y su fecha.
type VistaTarjeta struct {
	widget.BaseWidget
	T       *i18n.Translator
	Locale  i18n.Locale
	Tarjeta *Tarjeta
	// AlVotar marca o retira un valor; nil, la tarjeta es de solo lectura.
	AlVotar func(valor string, poner bool)
	// Ahora decide si la tarea va fuera de plazo.
	Ahora func() time.Time

	cuerpo *fyne.Container
	ancho  float32
}

// NuevaVistaTarjeta crea la vista.
func NuevaVistaTarjeta(t *i18n.Translator, l i18n.Locale, c *Tarjeta, alVotar func(string, bool)) *VistaTarjeta {
	v := &VistaTarjeta{T: t, Locale: l, Tarjeta: c, AlVotar: alVotar, Ahora: time.Now}
	v.ExtendBaseWidget(v)
	return v
}

// AccessibilityLabel es la pregunta o el título, que van en el cuerpo del mensaje.
func (v *VistaTarjeta) AccessibilityLabel() string { return v.T.T("card.options", nil) }

// AccessibilityRole: un grupo de controles.
func (v *VistaTarjeta) AccessibilityRole() fyne.AccessibleRole { return fyne.AccessibleRoleContainer }

// Visible dice si pinta algo: una encuesta sin opciones no (`CardView` devuelve null).
func (t *Tarjeta) Visible() bool {
	return t != nil && (t.Tipo == "task" || (t.Tipo == "poll" && len(t.Opciones) > 0))
}

// MedidaTarjeta es lo que ocupa a ese ancho, con su `mt-1.5`.
func MedidaTarjeta(v *VistaTarjeta, ancho float32) fyne.Size {
	c := v.construir()
	return fyne.NewSize(ancho, AireCajaTarjeta+c.MinSize().Height)
}

func (v *VistaTarjeta) CreateRenderer() fyne.WidgetRenderer {
	v.cuerpo = v.construir()
	return &vistaTarjetaRenderer{v: v}
}

type vistaTarjetaRenderer struct{ v *VistaTarjeta }

func (r *vistaTarjetaRenderer) MinSize() fyne.Size { return MedidaTarjeta(r.v, r.v.ancho) }
func (r *vistaTarjetaRenderer) Objects() []fyne.CanvasObject {
	return []fyne.CanvasObject{r.v.cuerpo}
}
func (r *vistaTarjetaRenderer) Destroy() {}
func (r *vistaTarjetaRenderer) Layout(t fyne.Size) {
	if t.Width != r.v.ancho {
		r.v.ancho = t.Width
		r.v.cuerpo = r.v.construir()
	}
	r.v.cuerpo.Move(fyne.NewPos(0, AireCajaTarjeta))
	r.v.cuerpo.Resize(fyne.NewSize(t.Width, max(t.Height-AireCajaTarjeta, 0)))
}

func (r *vistaTarjetaRenderer) Refresh() {
	r.v.cuerpo = r.v.construir()
	r.Layout(r.v.Size())
}

// construir arma la caja `border rounded-md bg-surface-sunken p-2.5` con lo de dentro.
func (v *VistaTarjeta) construir() *fyne.Container {
	tm := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	t, c := v.T, v.Tarjeta
	var dentro fyne.CanvasObject
	if c.Tipo == "task" {
		dentro = v.tarea()
	} else {
		total := c.Total()
		filas := make([]fyne.CanvasObject, 0, len(c.Opciones)+1)
		for i, o := range c.Opciones {
			valor := strconv.Itoa(i)
			voto := c.VotoDe(valor)
			pct := 0
			if total > 0 {
				pct = (voto.Veces*100 + total/2) / total // `Math.round`
			}
			filas = append(filas, NuevaOpcionEncuesta(o, t.T("card.pollOptionResult", i18n.Vars{"count": voto.Veces, "pct": pct}),
				voto.Mio, pct, v.AlVotar == nil, func() {
					if v.AlVotar != nil {
						v.AlVotar(valor, !voto.Mio)
					}
				}))
		}
		pie := t.T("card.pollTotal", i18n.Vars{"count": total})
		if c.Multi {
			pie += " · " + t.T("card.pollMulti", nil)
		}
		dentro = container.New(ui.Columna{Hueco: HuecoOpciones},
			append(filas, ui.NuevoRotulo(pie, escalonMeta, ui.Normal, "text-subtle"))...)
	}
	caja := ui.NuevaCaja(tema.Color("surface-sunken", tm), tema.Color("border", tm), tema.RadioMD)
	return container.NewStack(append(caja.Objetos(),
		container.New(ui.Margen{Izq: RellenoCajaTarjeta, Arr: RellenoCajaTarjeta, Der: RellenoCajaTarjeta, Abj: RellenoCajaTarjeta}, dentro))...)
}

// tarea es `Task`: la casilla-botón, «Para Ana» y «Para el 3 oct. 2026», en rojo y con
// «Fuera de plazo» si venció sin hacerse.
func (v *VistaTarjeta) tarea() fyne.CanvasObject {
	t, c := v.T, v.Tarjeta
	voto := c.VotoDe("done")
	hecha := voto.Veces > 0
	rotulo, icono, tinta := t.T("card.taskMarkDone", nil), iconos.Cuadrado, "text-subtle"
	if hecha {
		rotulo, icono, tinta = t.T("card.taskDone", nil), iconos.Tic, "primary-text"
	}
	boton := NuevaOpcionEncuesta("", "", false, 0, v.AlVotar == nil, func() {
		if v.AlVotar != nil {
			v.AlVotar("done", !voto.Mio)
		}
	})
	boton.Rotulo, boton.Icono, boton.TintaIcono, boton.Casilla = rotulo, icono, tinta, true
	objs := []fyne.CanvasObject{boton}
	if c.Asignado != "" {
		objs = append(objs, ui.NuevoRotulo(t.T("card.taskAssignee", i18n.Vars{"name": c.Asignado}), escalonMeta, ui.Normal, "text-subtle"))
	}
	if !c.Vence.IsZero() {
		fuera := !hecha && c.Vence.Before(v.Ahora())
		s := t.T("card.taskDue", i18n.Vars{"date": i18n.FormatDate(v.Locale, c.Vence.Local())})
		token := "text-subtle"
		if fuera {
			s, token = t.T("card.taskOverdue", nil)+" · "+s, "danger-text"
		}
		objs = append(objs, ui.NuevoRotulo(s, tema.TextoXS, ui.Normal, token))
	}
	return container.New(&envuelta{hueco: tema.Espacio(2)}, objs...)
}

// OpcionEncuesta es un botón de la tarjeta: una opción de la encuesta (`relative w-full
// rounded-md border px-2.5 py-1.5`, con la barra del resultado al 15 % del acento y el
// resultado a la derecha) o la casilla de la tarea.
type OpcionEncuesta struct {
	ui.Pulsable
	Rotulo, Resultado string
	Mia               bool
	Pct               int
	// Casilla es la de la tarea: el icono delante y sin barra.
	Casilla    bool
	Icono      iconos.Icono
	TintaIcono string
}

var _ desktop.Hoverable = (*OpcionEncuesta)(nil)

// NuevaOpcionEncuesta crea la opción; `soloLectura` la deja sin pulsar (`disabled`).
func NuevaOpcionEncuesta(rotulo, resultado string, mia bool, pct int, soloLectura bool, al func()) *OpcionEncuesta {
	o := &OpcionEncuesta{Rotulo: rotulo, Resultado: resultado, Mia: mia, Pct: pct}
	o.Nombre, o.AlPulsar, o.Inerte = rotulo, al, soloLectura
	o.ExtendBaseWidget(o)
	return o
}

// MinSize es `py-1.5` y la línea de `text-base`; la casilla, lo que mide su texto.
func (o *OpcionEncuesta) MinSize() fyne.Size {
	alto := 2*RellenoOpcionTarjeta + tema.TextoBase.Linea + 2*tema.Borde
	if o.Casilla {
		return fyne.NewSize(2*CanalonOpcionTarjeta+2*tema.Borde+iconos.TallaXS+tema.Espacio(2)+
			ui.Medir(o.Rotulo, tema.TextoBase, ui.Normal).Width, alto)
	}
	return fyne.NewSize(0, alto)
}

func (o *OpcionEncuesta) CreateRenderer() fyne.WidgetRenderer { return &opcionEncuestaRenderer{o: o} }

type opcionEncuestaRenderer struct {
	o    *OpcionEncuesta
	objs []fyne.CanvasObject
}

func (r *opcionEncuestaRenderer) MinSize() fyne.Size           { return r.o.MinSize() }
func (r *opcionEncuestaRenderer) Objects() []fyne.CanvasObject { return r.objs }
func (r *opcionEncuestaRenderer) Destroy()                     {}
func (r *opcionEncuestaRenderer) Refresh()                     { r.Layout(r.o.Size()) }

func (r *opcionEncuestaRenderer) Layout(t fyne.Size) {
	o := r.o
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	borde := "border"
	if o.Mia {
		borde = "border-strong"
	}
	fondo := color.NRGBA{}
	if o.Encima() && !o.Inerte {
		fondo = tema.Color("bg-subtle", v)
	}
	caja := ui.NuevaCaja(fondo, tema.Color(borde, v), tema.RadioMD)
	ui.MoverCaja(caja, fyne.Position{}, t)
	r.objs = append([]fyne.CanvasObject{}, caja.Objetos()...)
	alfa := float32(1)
	if o.Inerte {
		alfa = 0.7 // `disabled:opacity-70`
	}
	x := tema.Borde + CanalonOpcionTarjeta
	y := (t.Height - tema.TextoBase.Linea) / 2
	if o.Casilla {
		g := iconos.Trazo(o.Icono, iconos.TallaXS, o.TintaIcono, v).Objeto()
		g.Move(fyne.NewPos(x, (t.Height-iconos.TallaXS)/2))
		g.Resize(fyne.NewSize(iconos.TallaXS, iconos.TallaXS))
		tx := ui.Texto(o.Rotulo, tema.TextoBase, ui.Normal, tokenConAlfa("text", alfa, v))
		ui.ColocarTexto(tx, tema.TextoBase, x+iconos.TallaXS+tema.Espacio(2), y)
		r.objs = append(r.objs, g, tx)
		return
	}
	// La barra del resultado: decorativa, al 15 % del acento, bajo el texto.
	if o.Pct > 0 {
		barra := canvas.NewRectangle(tokenConAlfa("primary", 0.15, v))
		barra.CornerRadius = tema.RadioMD
		barra.Move(fyne.NewPos(tema.Borde, tema.Borde))
		barra.Resize(fyne.NewSize((t.Width-2*tema.Borde)*float32(o.Pct)/100, t.Height-2*tema.Borde))
		r.objs = append(r.objs, barra)
	}
	der := t.Width - tema.Borde - CanalonOpcionTarjeta
	res := ui.Texto(o.Resultado, escalonMeta, ui.Normal, tokenConAlfa("text-subtle", alfa, v))
	anchoRes := ui.Medir(o.Resultado, escalonMeta, ui.Normal).Width
	ui.ColocarTexto(res, escalonMeta, der-anchoRes, (t.Height-escalonMeta.Linea)/2)
	der -= anchoRes + tema.Espacio(2)
	r.objs = append(r.objs, res)
	if o.Mia {
		g := iconos.Trazo(iconos.Tic, iconos.TallaXS, "primary-text", v).Objeto()
		der -= iconos.TallaXS
		g.Move(fyne.NewPos(der, (t.Height-iconos.TallaXS)/2))
		g.Resize(fyne.NewSize(iconos.TallaXS, iconos.TallaXS))
		der -= tema.Espacio(2)
		r.objs = append(r.objs, g)
	}
	tx := ui.Texto("", tema.TextoBase, ui.Normal, tokenConAlfa("text", alfa, v))
	ui.Truncar(tx, o.Rotulo, tema.TextoBase, ui.Normal, max(der-x, 0))
	ui.ColocarTexto(tx, tema.TextoBase, x, y)
	r.objs = append(r.objs, tx)
}

// ---- En la Sala: votar y crear ----

// Votar es `handleCardVote`: el recuento se pinta ya con mi voto y se ajusta al que dé
// el servicio; si falla, vuelve el de antes.
func (sa *Sala) Votar(m Mensaje, valor string, poner bool) {
	if m.Tarjeta == nil || m.Enviando || m.Fallido {
		return
	}
	antes := slices.Clone(m.Tarjeta.Votos)
	aplicar := func(vs []api.RoomsCardVote) {
		if i := slices.IndexFunc(sa.mensajes, func(x api.RoomsMessage) bool { return x.ID == m.ID }); i >= 0 && sa.mensajes[i].Card != nil {
			sa.mensajes[i].Card.Votes = vs
			sa.pintarMensajes()
			sa.P.Canal.Historial.Refresh()
		}
	}
	aplicar(CambiarMiVoto(m.Tarjeta, valor, poner, sa.Yo))
	pedir(sa, func(ctx context.Context) ([]api.RoomsCardVote, error) { return sa.S.VoteCard(ctx, m.ID, valor, poner) },
		func(vs []api.RoomsCardVote, err error) {
			if err == nil {
				aplicar(vs)
				return
			}
			var vuelta []api.RoomsCardVote
			for _, v := range antes {
				vuelta = append(vuelta, api.RoomsCardVote{Value: v.Valor, Count: v.Veces, Subs: v.Subs})
			}
			aplicar(vuelta)
		})
}

// Los límites de CardComposerModal.
const (
	MinOpciones      = 2
	MaxOpciones      = 10
	TopeOpcion       = 120
	HoraFechaTarjeta = 12 // `${due}T12:00:00Z`: el mediodía UTC cae el mismo día en toda Europa
)

// NuevaTarjeta es `CardComposerModal`: la pregunta y las opciones de una encuesta
// —de dos a diez, «Permitir varias respuestas»— o el título, el responsable y la fecha
// de una tarea. «Enviar» publica el mensaje con su tarjeta por el envío de siempre.
//
//nolint:gocognit,gocyclo // El formulario alterna todos los controles específicos de encuesta y tarea.
func (sa *Sala) NuevaTarjeta(tipo string) {
	lienzo := fyne.CurrentApp().Driver().CanvasForObject(sa.P.Objeto())
	if lienzo == nil {
		return
	}
	t := sa.P.T
	poll := tipo == "poll"
	titulo, rotuloCuerpo, marca := t.T("card.newTask", nil), t.T("card.taskTitle", nil), t.T("card.taskTitlePlaceholder", nil)
	if poll {
		titulo, rotuloCuerpo, marca = t.T("card.newPoll", nil), t.T("card.question", nil), t.T("card.questionPlaceholder", nil)
	}
	cuerpo := ui.NuevaEntrada(marca)
	var opciones []*ui.Entrada
	multi := ui.NuevaCasilla(t.T("card.allowMulti", nil), nil)
	multi.AlCambiar = func(si bool) { multi.Marcada = si; multi.Refresh() }
	fecha := ui.CampoFecha(ui.CampoMD)
	var responsable *ui.Selector
	roster := sa.roster
	col := container.New(ui.Columna{Hueco: tema.Espacio(4)})
	var d *ui.Dialogo
	enviar := ui.NuevoBoton(t.T("composer.send", nil), nil).Con(ui.Primario, ui.TallaMD)
	llenas := func() []string {
		var out []string
		for _, o := range opciones {
			if s := strings.TrimSpace(o.Texto()); s != "" {
				out = append(out, s)
			}
		}
		return out
	}
	valida := func() bool { return TarjetaValida(tipo, cuerpo.Texto(), llenas()) }
	revisar := func() {
		enviar.Inerte = !valida()
		enviar.Refresh()
	}
	cuerpo.AlCambiar(func(string) { revisar() })
	var pintar func()
	nuevaOpcion := func() {
		e := ui.NuevaEntrada(t.T("card.optionN", i18n.Vars{"n": len(opciones) + 1}))
		e.AlCambiar(func(s string) {
			if r := []rune(s); len(r) > TopeOpcion {
				e.PonerTexto(string(r[:TopeOpcion]))
			}
			revisar()
		})
		opciones = append(opciones, e)
	}
	pintar = func() {
		bloques := []fyne.CanvasObject{ui.GrupoCampo(rotuloCuerpo, true, cuerpo, "", "")}
		if poll {
			filas := []fyne.CanvasObject{container.New(ui.Margen{Abj: tema.Espacio(1.5)},
				ui.NuevoRotulo(t.T("card.options", nil), tema.TextoSM, ui.SemiNegrita, "text"))}
			for i, o := range opciones {
				fila := fyne.CanvasObject(o)
				if len(opciones) > MinOpciones {
					quitar := ui.NuevoBotonIcono(iconos.Cerrar, t.T("card.removeOption", i18n.Vars{"n": i + 1}), func() {
						opciones = slices.Delete(opciones, i, i+1)
						pintar()
						revisar()
					})
					fila = container.NewBorder(nil, nil, nil, quitar, o)
				}
				filas = append(filas, fila)
			}
			if len(opciones) < MaxOpciones {
				filas = append(filas, container.New(alPrincipio{}, ui.NuevoBoton(t.T("card.addOption", nil), func() {
					nuevaOpcion()
					pintar()
				}).Con(ui.Secundario, ui.TallaSM)))
			}
			filas = append(filas, container.New(ui.Margen{Arr: tema.Espacio(1)}, multi))
			bloques = append(bloques, container.New(ui.Columna{Hueco: tema.Espacio(2)}, filas...))
		} else {
			if len(roster) > 0 && responsable == nil {
				ops := make([]string, 0, 1+len(roster))
				ops = append(ops, t.T("card.assigneeNone", nil))
				for _, m := range roster {
					ops = append(ops, m.Nombre)
				}
				responsable = ui.NuevoSelector(ops, nil)
			}
			if responsable != nil {
				bloques = append(bloques, ui.GrupoCampo(t.T("card.assignee", nil), false, responsable, "", ""))
			}
			bloques = append(bloques, ui.GrupoCampo(t.T("card.due", nil), false, fecha, t.T("card.dueHelp", nil), ""))
		}
		col.Objects = bloques
		col.Refresh()
		if d != nil {
			d.Recolocar()
		}
	}
	if poll {
		nuevaOpcion()
		nuevaOpcion()
	}
	pintar()
	cancelar := ui.NuevoBoton(t.T("action.cancel", nil), func() { d.Cerrar() }).Con(ui.Fantasma, ui.TallaMD)
	d = ui.NuevoDialogo(titulo, col).Con(ui.DialogoMD, container.New(ui.Fila{Hueco: ui.HuecoPieDialogo}, cancelar, enviar))
	d.RotuloCerrar, d.Foco = t.T("action.close", nil), cuerpo
	enviar.AlPulsar = func() {
		if !valida() {
			return
		}
		card := &api.RoomsCard{Kind: tipo}
		if poll {
			card.Options, card.Multi = llenas(), multi.Marcada
		} else {
			if responsable != nil && responsable.Elegida > 0 && responsable.Elegida <= len(roster) {
				m := roster[responsable.Elegida-1]
				card.AssigneeSub, card.AssigneeName = m.ID, m.Nombre
			}
			if dia, vacia, ok := ui.LeerFecha(fecha.Texto(), time.UTC); ok && !vacia {
				v := dia.Add(HoraFechaTarjeta * time.Hour)
				card.Due = &v
			}
		}
		d.Cerrar()
		sa.EnviarTarjeta(strings.TrimSpace(cuerpo.Texto()), card)
	}
	revisar()
	sa.pregunta = d
	d.Mostrar(lienzo)
}

// TarjetaValida es el `valid` de CardComposerModal: con pregunta o título y, en una
// encuesta, al menos dos opciones escritas.
func TarjetaValida(tipo, cuerpo string, opciones []string) bool {
	n := 0
	for _, o := range opciones {
		if strings.TrimSpace(o) != "" {
			n++
		}
	}
	return strings.TrimSpace(cuerpo) != "" && (tipo != "poll" || n >= MinOpciones)
}

// EnviarTarjeta publica la pregunta o el título con su tarjeta en el canal abierto.
func (sa *Sala) EnviarTarjeta(texto string, card *api.RoomsCard) {
	canal := sa.abierto
	if canal == "" || texto == "" {
		return
	}
	pedir(sa, func(ctx context.Context) (api.RoomsMessage, error) {
		return sa.S.PostWith(ctx, canal, texto, api.RoomsSend{Card: card})
	}, func(m api.RoomsMessage, err error) {
		if err != nil {
			sa.P.Avisar(ui.AvisoPeligro, sa.P.T.T("message.sendFailed", nil))
			return
		}
		if canal == sa.abierto && !slices.ContainsFunc(sa.mensajes, func(x api.RoomsMessage) bool { return x.ID == m.ID }) {
			sa.mensajes = append(sa.mensajes, m)
			sa.pintarMensajes()
			sa.P.Canal.Historial.Refresh()
		}
	})
}

// alPrincipio deja al hijo a su tamaño y a la izquierda, como un `<div>` con un botón.
type alPrincipio struct{}

func (alPrincipio) MinSize(objs []fyne.CanvasObject) fyne.Size { return objs[0].MinSize() }
func (alPrincipio) Layout(objs []fyne.CanvasObject, _ fyne.Size) {
	objs[0].Move(fyne.Position{})
	objs[0].Resize(objs[0].MinSize())
}

// MenuMas es el menú «+» del compositor del canal: los objetos ricos, encuesta y tarea
// (solo en el del canal: el del hilo no lleva tarjetas).
//
// Con Drive y el selector del sistema, «Subir desde tu equipo» va delante.
//
// Con el selector de la suite, «Elegir de Sytue Drive» detrás. Después, «GIF» (el
// catálogo de la organización) y, tras su separador, «Apps e integraciones», cada uno
// solo si el servicio que lo sirve está.
func (sa *Sala) MenuMas(disparador fyne.CanvasObject) {
	t := sa.P.T
	var es []fyne.CanvasObject
	if sa.Ficheros != nil && sa.ElegirFicheros != nil {
		subir := ui.NuevoElementoMenu(t.T("composer.uploadFile", nil), iconos.Ninguno, sa.SubirDelEquipo)
		subir.Glifo = dibujo(glifoSubir, iconos.TallaSM, "text-muted", tema.De(fyne.CurrentApp().Settings().ThemeVariant()))
		es = append(es, subir)
	}
	if sa.ListarDrive != nil {
		// Nube = el fichero YA vive en Drive (frente a la flecha de «subir»).
		es = append(es, ui.NuevoElementoMenu(t.T("drive.pickTitle", nil), iconos.Nube, sa.ElegirDeDrive))
	}
	if len(es) > 0 {
		es = append(es, ui.NuevoSeparadorMenu())
	}
	es = append(es,
		ui.NuevoElementoMenu(t.T("card.newPoll", nil), iconos.Grafico, func() { sa.NuevaTarjeta("poll") }),
		ui.NuevoElementoMenu(t.T("card.newTask", nil), iconos.Tic, func() { sa.NuevaTarjeta("task") }),
	)
	_, conEmoji := sa.S.(EmojiPropio)
	if _, conDrive := sa.Ficheros.(DriveOrganizacion); conEmoji && conDrive {
		gif := ui.NuevoElementoMenu(t.T("gif.title", nil), iconos.Ninguno, sa.AbrirGif)
		gif.Glifo = dibujo(glifoImagen, iconos.TallaSM, "text-muted", tema.De(fyne.CurrentApp().Settings().ThemeVariant()))
		es = append(es, gif)
	}
	if _, ok := sa.S.(Integrador); ok {
		es = append(es, ui.NuevoSeparadorMenu(),
			ui.NuevoElementoMenu(t.T("integrations.apps", nil), iconos.Rejilla, sa.AbrirIntegraciones))
	}
	m := ui.NuevoMenu(es...)
	m.Rotulo, m.AlInicio, m.Arriba = t.T("composer.messageActions", nil), true, true
	for _, e := range m.Elementos {
		if it, ok := e.(*ui.ElementoMenu); ok {
			it.Denso = true
		}
	}
	sa.menuMas = m
	m.Mostrar(disparador)
}
