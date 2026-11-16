// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package erp

import (
	"image/color"
	"strings"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"

	"git.sytue.eu/sytue/one/apps/fyne/iconos"
	"git.sytue.eu/sytue/one/apps/fyne/tema"
	"git.sytue.eu/sytue/one/apps/fyne/ui"
	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/i18n"
)

// # Aprobaciones (`Aprobaciones.tsx`)
//
// Los documentos que pasan del umbral de su módulo y esperan a una segunda persona, y
// debajo, para quien administra la contabilidad, los umbrales mismos. Es la pantalla de
// Tareas con otro contenido, y comparte con ella la cabecera, el grupo de filtro y la
// lista `rounded-card border divide-y`:
//
//	cabecera   título `text-lg`, frase `text-sm` `max-w-prose` y a la derecha
//	           «Pendientes · Aprobadas · Rechazadas»; `pb-4` y filete
//	fila       `px-4 py-3`: modelo (subrayado, abre el registro) · importe en mono, y
//	           debajo la fecha, «pedida por ti» y el motivo en `text-xs`; a la derecha
//	           «Aprobar» y «Rechazar», o la insignia del estado
//	umbrales   versalitas, frase, y una fila `px-4 py-2` por módulo con el campo `w-32`,
//	           la divisa y «Guardar»
//
// Los cuatro ojos los impone el servicio (403 `cuatro_ojos`); aquí solo se evita
// ofrecer «Aprobar» a quien lo pidió o creó el documento.

// Medidas de Aprobaciones.
const (
	// FilaUmbralPY es el `py-2` de la fila de un umbral.
	FilaUmbralPY float32 = 2 * 0.25 * tema.Rem
	// AnchoCampoUmbral es el `w-32` del importe.
	AnchoCampoUmbral float32 = 32 * 0.25 * tema.Rem
)

// Los tres estados del filtro, en su orden.
var estadosAprobacion = []string{"pendiente", "aprobada", "rechazada"}

// Aprobaciones es el estado de la pantalla.
type Aprobaciones struct {
	tr  *i18n.Translator
	loc i18n.Locale
	// Estado es el filtro: «pendiente» (por defecto), «aprobada» o «rechazada».
	Estado string
	// Lista son las de ese estado; nil, cargando.
	Lista []api.ERPAprobacion
	// Yo es el `sub` de quien mira: no se le ofrece aprobar lo suyo.
	Yo string
	// PuedeAprobar es el verbo `aprobar` en contabilidad (lo que exige la ruta) y
	// PuedeAdministrar, `administrar`: ve y cambia los umbrales.
	PuedeAprobar, PuedeAdministrar bool
	// Ocupada es la solicitud cuyo botón espera al servicio.
	Ocupada string
	// Umbrales son los puestos y los módulos; nil, cargando.
	Umbrales *api.ERPUmbrales
	// Guardando es el módulo cuyo umbral se está guardando.
	Guardando string
	// Ventana decide el reparto.
	Ventana float32

	// campos son los importes de los umbrales: el MISMO widget de una pintada a otra,
	// para que lo escrito y el foco sobrevivan a cada repintado.
	campos map[string]*ui.Entrada

	RotuloModelo func(clave string) string
	AlEstado     func(estado string)
	AlAbrir      func(modelo, id string)
	AlResolver   func(a api.ERPAprobacion, aprobar bool)
	// AlGuardarUmbral recibe lo escrito tal cual: quien llama lo valida y lo convierte.
	AlGuardarUmbral func(modulo, texto, moneda string)
}

// NuevaAprobaciones crea la pantalla con el filtro de pendientes.
func NuevaAprobaciones(tr *i18n.Translator) *Aprobaciones {
	return &Aprobaciones{tr: tr, loc: tr.Locale(), Estado: "pendiente", campos: map[string]*ui.Entrada{}}
}

func (a *Aprobaciones) t(clave string, vars ...i18n.Vars) string {
	var v i18n.Vars
	if len(vars) > 0 {
		v = vars[0]
	}
	return a.tr.T(clave, v)
}

// PonerUmbrales guarda lo que contestó el servicio y escribe cada importe en su campo
// (en blanco, sin umbral).
func (a *Aprobaciones) PonerUmbrales(u api.ERPUmbrales) {
	a.Umbrales = &u
	for _, m := range u.Modulos {
		a.campo(m.Clave).PonerTexto("")
	}
	for _, x := range u.Umbrales {
		a.campo(x.Modulo).PonerTexto(DeCentimos(x.Importe))
	}
}

// Campo es el importe escrito para el umbral de un módulo.
func (a *Aprobaciones) Campo(modulo string) *ui.Entrada { return a.campo(modulo) }

func (a *Aprobaciones) campo(modulo string) *ui.Entrada {
	if e, ok := a.campos[modulo]; ok {
		return e
	}
	e := ui.NuevaEntrada(a.t("approvals.thresholdNone")).Con(ui.CampoSM)
	a.campos[modulo] = e
	return e
}

// ModulosConUmbral son los que admiten umbral. «Obligaciones» no es un módulo del
// catálogo: el servicio no le acepta umbral, aunque venga en la lista de rótulos.
func (a *Aprobaciones) ModulosConUmbral() []api.ERPModuloRotulado {
	if a.Umbrales == nil {
		return nil
	}
	var out []api.ERPModuloRotulado
	for _, m := range a.Umbrales.Modulos {
		if m.Clave != "obligaciones" {
			out = append(out, m)
		}
	}
	return out
}

func (a *Aprobaciones) rotuloModelo(clave string) string {
	if a.RotuloModelo != nil {
		return a.RotuloModelo(clave)
	}
	return clave
}

func (a *Aprobaciones) sm() bool { return a.Ventana == 0 || a.Ventana >= ui.CorteSM }

// Objeto arma la pantalla.
func (a *Aprobaciones) Objeto(v tema.Variante) fyne.CanvasObject {
	ancho := anchoUtilDe(a.Ventana, AnchoContenidoLista)
	p := &pila{}
	cab, altoCab := a.cabecera(v, ancho)
	p.add(cab, altoCab, 0)
	switch {
	case a.Lista == nil:
		alto := float32(2*12*0.25*tema.Rem + 24)
		p.add(centrado(ui.NuevaRueda(24, "text-muted", v), alto), alto, TareasGapGrupos)
	case len(a.Lista) == 0:
		titulo, frase := a.t("approvals.emptyResolved"), ""
		if a.Estado == "pendiente" {
			titulo, frase = a.t("approvals.empty"), a.t("approvals.emptyHint")
		}
		x := ui.NuevoVacio(titulo, frase)
		x.Ilustracion = iconos.Pintar(iconos.VacioCalendario, v).A(VacioTareasIlustracion).Objeto()
		alto := x.MedidaVacio(ancho)
		p.add(centrado(x, alto), alto, TareasGapGrupos)
	default:
		o, alto := a.lista(v, ancho)
		p.add(o, alto, TareasGapGrupos)
	}
	if a.PuedeAdministrar {
		a.umbrales(p, v, ancho)
	}
	o, alto := p.objeto()
	return enPagina(o, alto, AnchoContenidoLista)
}

// cabecera es la de Tareas con el grupo de los tres estados.
func (a *Aprobaciones) cabecera(v tema.Variante, ancho float32) (fyne.CanvasObject, float32) {
	elegida := 0
	for i, e := range estadosAprobacion {
		if e == a.Estado {
			elegida = i
		}
	}
	grupo := NuevoGrupoFiltro(a.t("approvals.title"),
		[]string{a.t("approvals.pending"), a.t("approvals.approved"), a.t("approvals.rejected")}, elegida,
		func(i int) {
			if a.AlEstado != nil {
				a.AlEstado(estadosAprobacion[i])
			}
		})
	return cabeceraConGrupo(v, a.Ventana, ancho, a.t("approvals.title"), a.t("approvals.subtitle"), grupo)
}

// lista es `ul.rounded-card.border.divide-y` con una fila por solicitud.
func (a *Aprobaciones) lista(v tema.Variante, ancho float32) (fyne.CanvasObject, float32) {
	filas := make([]bloque, 0, len(a.Lista))
	for _, x := range a.Lista {
		o, alto := a.fila(v, x, ancho-2*tema.Borde)
		filas = append(filas, bloque{o, alto, 0})
	}
	return enListaConFiletes(v, filas)
}

// enListaConFiletes mete filas ya armadas en la caja con borde y un filete entre cada
// dos, que es el `divide-y` de la web.
func enListaConFiletes(v tema.Variante, filas []bloque) (fyne.CanvasObject, float32) {
	caja := ui.NuevaCaja(tema.Color(ui.FondoHoja(), v), tema.Color("border", v), radioTarjeta())
	col := &columnaFilas{}
	var hijos []fyne.CanvasObject
	for i, f := range filas {
		col.filas = append(col.filas, f)
		hijos = append(hijos, f.obj)
		if i > 0 {
			d := canvas.NewRectangle(tema.Color("border", v))
			col.filetes = append(col.filetes, d)
			hijos = append(hijos, d)
		}
	}
	lista := container.New(col, hijos...)
	return container.New(&enCaja{caja: caja, hijo: lista}, append(caja.Objetos(), lista)...), col.alto() + 2*tema.Borde
}

// fila es una solicitud: el modelo (abre el registro) · el importe, y debajo cuándo se
// pidió, si fue quien mira y el motivo; a la derecha, lo que se puede hacer con ella.
func (a *Aprobaciones) fila(v tema.Variante, x api.ERPAprobacion, ancho float32) (fyne.CanvasObject, float32) {
	ap := x
	rotulo := a.rotuloModelo(x.Modelo)
	en := ui.NuevoEnlace(rotulo, tema.TextoSM, ui.Medio, "text", func() {
		if a.AlAbrir != nil {
			a.AlAbrir(ap.Modelo, ap.RegistroID)
		}
	})
	arriba := []pieza{
		piezaDe(en, fyne.NewSize(ui.Medir(rotulo, tema.TextoSM, ui.Medio).Width, tema.TextoSM.Linea)),
		textoPieza(" · ", tema.TextoSM, ui.Medio, "text", v),
		textoPieza(Dinero(a.loc, x.Importe, x.Moneda), tema.TextoSM, ui.Mono, "text", v),
	}
	sub := a.t("approvals.requestedOn", i18n.Vars{"date": i18n.FormatDateTime(a.loc, x.SolicitadaEn.Local())})
	if x.SolicitadaPor == a.Yo {
		sub += " · " + a.t("approvals.byYou")
	}
	if x.Motivo != "" {
		sub += " · " + a.t("approvals.reason", i18n.Vars{"reason": x.Motivo})
	}

	var der []pieza
	switch {
	case x.Estado != "pendiente":
		tono, clave := ui.InsigniaPeligro, "approvals.rejected"
		if x.Estado == "aprobada" {
			tono, clave = ui.InsigniaExito, "approvals.approved"
		}
		der = append(der, piezaDe(ui.NuevaInsignia(a.t(clave), tono), ui.MedidaInsignia(a.t(clave))))
	case a.PuedeAprobar && x.PuedeResolverla(a.Yo):
		ocupada := a.Ocupada == x.ID
		si := botonXS(a.t("approvals.approve"), ui.Primario, iconos.Tic, func() {
			if a.AlResolver != nil {
				a.AlResolver(ap, true)
			}
		})
		si.Cargando = ocupada
		no := ui.NuevoBoton(a.t("approvals.reject"), func() {
			if a.AlResolver != nil {
				a.AlResolver(ap, false)
			}
		}).Con(ui.Fantasma, ui.TallaXS)
		no.Inerte = ocupada
		der = append(der, piezaDe(si, si.MinSize()), piezaDe(no, no.MinSize()))
	default:
		der = append(der, piezaDe(ui.NuevaInsignia(a.t("approvals.waiting"), ui.InsigniaAviso), ui.MedidaInsignia(a.t("approvals.waiting"))))
	}
	return filaSolicitud(v, arriba, sub, der, ancho, a.sm())
}

// filaSolicitud es la fila `px-4 py-3` de Tareas con lo de arriba hecho de piezas y
// una segunda línea de texto que se recorta si no cabe; lo de la derecha baja en el
// móvil (`flex-col sm:flex-row`).
func filaSolicitud(v tema.Variante, arriba []pieza, sub string, der []pieza, ancho float32, sm bool) (fyne.CanvasObject, float32) {
	var anchoDer float32
	for i, p := range der {
		anchoDer += p.ancho
		if i > 0 {
			anchoDer += HuecoAccionesTarea
		}
	}
	cabe := ancho - 2*FilaTareaPX
	if sm {
		cabe -= anchoDer + HuecoCabeceraFila
	}
	st := ui.Texto(sub, tema.TextoXS, ui.Normal, tema.Color("text-subtle", v))
	ui.Truncar(st, sub, tema.TextoXS, ui.Normal, max(cabe, 0))
	pos := make([]fyne.Position, 0, len(arriba))
	x := float32(0)
	for _, p := range arriba {
		pos = append(pos, fyne.NewPos(x, 0))
		x += p.ancho
	}
	alto := AltoFilaTarea
	if !sm {
		alto += HuecoCabecera + ui.LadoIconoSM
	}
	l := &filaPiezas{arriba: arriba, pos: pos, abajo: st, der: der, anchoDer: anchoDer, sm: sm, fondo: canvas.NewRectangle(color.NRGBA{})}
	objs := append([]fyne.CanvasObject{l.fondo, st}, objetosDe(arriba, der)...)
	return container.New(l, objs...), alto
}

type filaPiezas struct {
	fondo       *canvas.Rectangle
	arriba, der []pieza
	pos         []fyne.Position
	abajo       *canvas.Text
	anchoDer    float32
	sm          bool
}

func (f *filaPiezas) MinSize([]fyne.CanvasObject) fyne.Size { return fyne.NewSize(0, 0) }
func (f *filaPiezas) Layout(_ []fyne.CanvasObject, t fyne.Size) {
	f.fondo.Resize(t)
	x0, y0 := FilaTareaPX, FilaTareaPY
	colocarPiezas(f.arriba, f.pos, x0, y0)
	ui.ColocarTexto(f.abajo, tema.TextoXS, x0, y0+tema.TextoSM.Linea)
	xd, yd := t.Width-FilaTareaPX-f.anchoDer, (t.Height-ui.LadoIconoSM)/2
	if !f.sm {
		xd, yd = x0, y0+tema.TextoSM.Linea+tema.TextoXS.Linea+HuecoCabecera
	}
	for _, p := range f.der {
		colocarPieza(p, xd, yd+(ui.LadoIconoSM-p.alto)/2)
		xd += p.ancho + HuecoAccionesTarea
	}
}

// umbrales es la sección de abajo: el título en versalitas, la frase y una fila por
// módulo.
func (a *Aprobaciones) umbrales(p *pila, v tema.Variante, ancho float32) {
	titulo := ui.NuevoVersal(a.t("approvals.thresholds"), tema.TextoXS, ui.SemiNegrita, tema.Color("text-subtle", v))
	p.add(titulo.Objeto(), tema.TextoXS.Linea, TareasGapGrupos)
	frase := ui.NuevoParrafo(a.t("approvals.thresholdsHint"), tema.TextoSM, ui.Normal, "text-muted", v)
	anchoFrase := min(ancho, anchoProsa()) + HolguraReparto
	altoFrase := ui.AltoParrafo(frase, anchoFrase)
	p.add(container.New(&anchoFijo{ancho: anchoFrase}, frase), altoFrase, SeccionGap)
	if a.Umbrales == nil {
		alto := float32(2*6*0.25*tema.Rem + 20)
		p.add(centrado(ui.NuevaRueda(20, "text-muted", v), alto), alto, SeccionGap)
		return
	}
	var filas []bloque
	for _, m := range a.ModulosConUmbral() {
		o, alto := a.filaUmbral(v, m, ancho-2*tema.Borde)
		filas = append(filas, bloque{o, alto, 0})
	}
	if len(filas) == 0 {
		return
	}
	o, alto := enListaConFiletes(v, filas)
	p.add(o, alto, SeccionGap)
}

// filaUmbral es un módulo: su nombre y, debajo, desde cuánto pide aprobación; a la
// derecha el importe, la divisa y «Guardar».
func (a *Aprobaciones) filaUmbral(v tema.Variante, m api.ERPModuloRotulado, ancho float32) (fyne.CanvasObject, float32) {
	var puesto *api.ERPUmbral
	for i := range a.Umbrales.Umbrales {
		if a.Umbrales.Umbrales[i].Modulo == m.Clave {
			puesto = &a.Umbrales.Umbrales[i]
		}
	}
	moneda := a.Umbrales.MonedaPorDefecto
	sub := a.t("approvals.thresholdNone")
	if puesto != nil {
		moneda = puesto.Moneda
		sub = a.t("approvals.thresholdFrom", i18n.Vars{"amount": Dinero(a.loc, puesto.Importe, puesto.Moneda)})
	}
	clave := m.Clave
	campo := a.campo(clave)
	campo.Rotulo = m.Rotulo // el `<label for>`
	altoCampo := ui.AltoCampo(ui.CampoSM)
	guardar := ui.NuevoBoton(a.t("action.save"), func() {
		if a.AlGuardarUmbral != nil {
			a.AlGuardarUmbral(clave, campo.Texto(), moneda)
		}
	}).Con(ui.Secundario, ui.TallaXS)
	guardar.Cargando = a.Guardando == clave
	der := []pieza{
		piezaDe(campo, fyne.NewSize(AnchoCampoUmbral, altoCampo)),
		textoPieza(moneda, tema.TextoXS, ui.Normal, "text-subtle", v),
		piezaDe(guardar, guardar.MinSize()),
	}
	var anchoDer float32
	for i, p := range der {
		anchoDer += p.ancho
		if i > 0 {
			anchoDer += HuecoAccionesTarea
		}
	}
	nombre := ui.Texto(m.Rotulo, tema.TextoSM, ui.Normal, tema.Color("text", v))
	st := ui.Texto(sub, tema.TextoXS, ui.Normal, tema.Color("text-subtle", v))
	sm := a.sm()
	altoIzq := tema.TextoSM.Linea + tema.TextoXS.Linea
	alto := max(altoIzq, altoCampo) + 2*FilaUmbralPY
	if !sm {
		alto = altoIzq + HuecoCabecera + altoCampo + 2*FilaUmbralPY
	}
	// El nombre y lo de debajo se recortan antes de chocar con el campo.
	cabe := ancho - 2*FilaTareaPX
	if sm {
		cabe -= anchoDer + HuecoCabeceraFila
	}
	ui.Truncar(nombre, m.Rotulo, tema.TextoSM, ui.Normal, max(cabe, 0))
	ui.Truncar(st, sub, tema.TextoXS, ui.Normal, max(cabe, 0))
	l := &filaUmbral{nombre: nombre, sub: st, der: der, anchoDer: anchoDer, altoIzq: altoIzq, altoDer: altoCampo, sm: sm}
	return container.New(l, append([]fyne.CanvasObject{nombre, st}, objetosDe(der)...)...), alto
}

type filaUmbral struct {
	nombre, sub      *canvas.Text
	der              []pieza
	anchoDer         float32
	altoIzq, altoDer float32
	sm               bool
}

func (f *filaUmbral) MinSize([]fyne.CanvasObject) fyne.Size { return fyne.NewSize(0, 0) }
func (f *filaUmbral) Layout(_ []fyne.CanvasObject, t fyne.Size) {
	x0 := FilaTareaPX
	yIzq := (t.Height - f.altoIzq) / 2
	xd, yd := t.Width-FilaTareaPX-f.anchoDer, (t.Height-f.altoDer)/2
	if !f.sm {
		yIzq = FilaUmbralPY
		xd, yd = x0, FilaUmbralPY+f.altoIzq+HuecoCabecera
	}
	ui.ColocarTexto(f.nombre, tema.TextoSM, x0, yIzq)
	ui.ColocarTexto(f.sub, tema.TextoXS, x0, yIzq+tema.TextoSM.Linea)
	for _, p := range f.der {
		colocarPieza(p, xd, yd+(f.altoDer-p.alto)/2)
		xd += p.ancho + HuecoAccionesTarea
	}
}

// anchoFijo deja a su único hijo un ancho fijo (el `max-w-prose` de una frase).
type anchoFijo struct{ ancho float32 }

func (a *anchoFijo) MinSize(objs []fyne.CanvasObject) fyne.Size { return fyne.NewSize(0, 0) }
func (a *anchoFijo) Layout(objs []fyne.CanvasObject, t fyne.Size) {
	for _, o := range objs {
		o.Move(fyne.NewPos(0, 0))
		o.Resize(fyne.NewSize(min(a.ancho, t.Width), t.Height))
	}
}

// --- Lo que hace la pantalla con el servicio ---------------------------------------

// PuedeAprobar es el verbo `aprobar` en contabilidad, que es lo que exige la ruta de
// resolver; PuedeUmbrales, `administrar`, lo que exige la de los umbrales.
func (p *Pantalla) PuedeAprobar() bool  { return p.PuedeAccion("contabilidad", "aprobar") }
func (p *Pantalla) PuedeUmbrales() bool { return p.PuedeAccion("contabilidad", "administrar") }

func (p *Pantalla) yo() string {
	if p.Yo == nil {
		return ""
	}
	return p.Yo()
}

func (p *Pantalla) engancharAprobaciones() {
	a := p.aprob
	a.RotuloModelo = func(clave string) string {
		if m, ok := p.modeloPorClave(clave); ok {
			return m.Etiqueta
		}
		return clave
	}
	a.AlEstado = func(e string) {
		a.Estado, a.Lista = e, nil
		p.Repintar()
		p.cargarAprobaciones()
	}
	a.AlAbrir = p.abrirRegistro
	a.AlResolver = p.resolverAprobacion
	a.AlGuardarUmbral = p.guardarUmbral
}

// contarAprobaciones pide las pendientes y cuenta las que quien mira puede resolver:
// el número del menú. Su fallo es silencioso, como en la web.
func (p *Pantalla) contarAprobaciones() {
	if p.svc == nil || !p.PuedeAprobar() {
		return
	}
	yo := p.yo()
	p.fondo(func() {
		ctx, cancelar := p.ctx()
		defer cancelar()
		as, err := p.svc.Aprobaciones(ctx, "pendiente")
		if err != nil {
			return
		}
		n := 0
		for _, x := range as {
			if x.PuedeResolverla(yo) {
				n++
			}
		}
		enHilo(func() { p.AprobacionesPendientes = n; p.Repintar() })
	})
}

// cargarAprobaciones pide las del estado elegido y, a quien administra, los umbrales.
func (p *Pantalla) cargarAprobaciones() {
	if p.svc == nil {
		return
	}
	a := p.aprob
	estado, umbrales := a.Estado, p.PuedeUmbrales() && a.Umbrales == nil
	p.fondo(func() {
		ctx, cancelar := p.ctx()
		defer cancelar()
		as, err := p.svc.Aprobaciones(ctx, estado)
		var u api.ERPUmbrales
		var errU error
		if umbrales {
			u, errU = p.svc.Umbrales(ctx)
		}
		enHilo(func() {
			if p.Vista != VistaAprobaciones || a.Estado != estado {
				return
			}
			a.Lista = vacioSiFalla(as, err)
			if umbrales {
				if errU != nil {
					u = api.ERPUmbrales{}
				}
				a.PonerUmbrales(u)
			}
			p.Repintar()
		})
	})
}

// resolverAprobacion aprueba con un clic; rechazar pide antes el motivo, que el
// servicio exige: sin él, quien lo pidió no sabe qué arreglar.
func (p *Pantalla) resolverAprobacion(x api.ERPAprobacion, aprobar bool) {
	if aprobar {
		p.enviarResolucion(x, true, "")
		return
	}
	if p.lienzo == nil {
		return
	}
	d := ui.Pregunta(p.t("approvals.rejectReason"), "", p.tr.T("ui.cancel", nil), p.t("approvals.reject"),
		p.tr.T("ui.required", nil), func(motivo string) {
			if motivo = strings.TrimSpace(motivo); motivo != "" {
				p.enviarResolucion(x, false, motivo)
			}
		})
	d.RotuloCerrar = p.tr.T("ui.close", nil)
	d.Mostrar(p.lienzo)
}

func (p *Pantalla) enviarResolucion(x api.ERPAprobacion, aprobar bool, motivo string) {
	if p.svc == nil {
		return
	}
	a := p.aprob
	a.Ocupada = x.ID
	p.Repintar()
	p.fondo(func() {
		ctx, cancelar := p.ctx()
		defer cancelar()
		err := p.svc.ResolverAprobacion(ctx, x.ID, aprobar, motivo)
		enHilo(func() {
			a.Ocupada = ""
			switch {
			case err != nil:
				p.avisar(ui.AvisoPeligro, MensajeDeError(err, p.tr))
			case aprobar:
				p.avisar(ui.AvisoExito, p.t("approvals.approvedToast"))
			default:
				p.avisar(ui.AvisoExito, p.t("approvals.rejectedToast"))
			}
			p.contarAprobaciones()
			p.cargarAprobaciones()
		})
	})
}

// guardarUmbral pone el umbral de un módulo. En blanco lo quita: el servicio lo
// entiende como un importe negativo.
func (p *Pantalla) guardarUmbral(modulo, texto, moneda string) {
	if p.svc == nil {
		return
	}
	texto = strings.TrimSpace(texto)
	centimos := int64(-1)
	if texto != "" {
		c, ok := ACentimos(texto)
		if !ok || c <= 0 {
			p.avisar(ui.AvisoPeligro, p.t("approvals.thresholdInvalid"))
			return
		}
		centimos = c
	}
	a := p.aprob
	a.Guardando = modulo
	p.Repintar()
	p.fondo(func() {
		ctx, cancelar := p.ctx()
		defer cancelar()
		err := p.svc.GuardarUmbral(ctx, modulo, centimos, moneda)
		var u api.ERPUmbrales
		var errU error
		if err == nil {
			u, errU = p.svc.Umbrales(ctx)
		}
		enHilo(func() {
			a.Guardando = ""
			if err != nil {
				p.avisar(ui.AvisoPeligro, MensajeDeError(err, p.tr))
			} else {
				p.avisar(ui.AvisoExito, p.t("approvals.thresholdSaved"))
				if errU == nil {
					a.PonerUmbrales(u)
				}
			}
			p.Repintar()
		})
	})
}

// pedirAprobacion abre la solicitud del documento de la ficha, después de que asentar
// contestara 409 `requiere_aprobacion`. El importe lo calcula el servicio.
func (p *Pantalla) pedirAprobacion(f *Ficha) {
	if p.svc == nil || f.Registro == nil {
		return
	}
	f.Guardando, f.Fallo = true, ""
	p.Repintar()
	m, id := f.Modelo, f.Registro.ID
	p.fondo(func() {
		ctx, cancelar := p.ctx()
		defer cancelar()
		_, err := p.svc.PedirAprobacion(ctx, m.Clave, id)
		enHilo(func() {
			f.Guardando = false
			if err != nil {
				f.Fallo = MensajeDeError(err, p.tr)
			} else {
				f.FaltaAprobacion, f.Aviso = false, p.t("form.approvalRequested")
			}
			p.Repintar()
		})
	})
}
