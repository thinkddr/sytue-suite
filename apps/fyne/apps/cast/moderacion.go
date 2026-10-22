// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

// paridad-web: apps/frontend/apps/cast/src/pages/moderation.astro

package cast

import (
	"errors"
	"net/http"
	"slices"
	"strings"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"

	"git.sytue.eu/sytue/one/apps/fyne/tema"
	"git.sytue.eu/sytue/one/apps/fyne/ui"
	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/i18n"
)

// Denunciar y bloquear (ADR 0074), como en la web: `Moderacion.tsx` y `ReportDialog.tsx`
// (el diálogo de denuncia y el bloqueo, desde un vídeo, un comentario, un mensaje o un
// canal) y `ModerationApp.tsx` (la pantalla «Denuncias y bloqueos»).
//
// Quién puede qué lo decide el servicio. Aquí solo se elige qué se enseña: denunciar,
// siempre que no sea lo tuyo y también sin sesión; bloquear, con sesión; la cola, si el
// servicio dice que administras la organización (contesta 403 a quien no).

// Las tres pestañas de quien administra.
const (
	PestanaAbiertas = iota
	PestanaResueltas
	PestanaBloqueosOrg
)

// Moderacion son los datos de «Denuncias y bloqueos».
type Moderacion struct {
	// Mios son las personas que tiene bloqueadas quien mira.
	Mios []api.CastBlocked
	// Administra: además, la cola y los bloqueos de la organización.
	Administra bool
	Pestana    int
	Denuncias  []api.CastReport
	DeLaOrg    []api.CastBlocked
	// Cargando y ErrorCola son de la pestaña; EnCurso, la denuncia que se está resolviendo.
	Cargando  bool
	ErrorCola bool
	EnCurso   string

	AlPestana        func(int)
	AlDesbloquear    func(api.CastBlocked)
	AlDesbloquearOrg func(api.CastBlocked)
	AlResolver       func(api.CastReport, string)
	AlVer            func(video string)
	AlReintentarCola func()
}

// PantallaModeracion pinta «Denuncias y bloqueos».
func PantallaModeracion(v Vista, d Moderacion) *fyne.Container {
	bloques := []ui.Bloque{{Obj: Cabecera(v.Traduce("nav.moderation"), ""), Margen: HuecoSeccion}}
	if cuerpo := CuerpoDeEstado(v, nil); cuerpo != nil {
		return Pagina(v, append(bloques, ui.Bloque{Obj: cuerpo})...)
	}
	if d.Administra {
		tira := ui.NuevasPestanas([]string{
			v.Traduce("moderation.open"), v.Traduce("moderation.resolved"), v.Traduce("moderation.orgBlocks"),
		}, nil)
		tira.Nombre = v.Traduce("nav.moderation")
		tira.Activa = d.Pestana
		tira.AlCambiar = d.AlPestana
		bloques = append(bloques,
			ui.Bloque{Obj: tira, Margen: tema.Espacio(4)},
			ui.Bloque{Obj: panelModeracion(v, d), Margen: HuecoSeccion},
		)
	}
	bloques = append(bloques,
		ui.Bloque{Obj: TituloSeccion(v, v.Traduce("moderation.myBlocks")), Margen: tema.Espacio(3)},
		ui.Bloque{Obj: listaBloqueados(v, d.Mios, v.Traduce("moderation.myBlocks.empty"), d.AlDesbloquear)},
	)
	return Pagina(v, bloques...)
}

// panelModeracion es el contenido de la pestaña activa.
func panelModeracion(v Vista, d Moderacion) fyne.CanvasObject {
	switch {
	case d.ErrorCola:
		return Fila(
			Parrafo(v, v.Traduce("errors.title"), tema.TextoSM, ui.Normal, "danger-text"),
			ui.NuevoBoton(v.Traduce("errors.retry"), d.AlReintentarCola).Con(ui.Secundario, ui.TallaSM))
	case d.Cargando:
		r := ui.NuevaRueda(ui.LadoRueda, "text-muted", v.Variante)
		r.Rotulo = v.Traduce("ui.loading")
		return container.New(ui.FilaAuto{Derecha: 1}, r)
	case d.Pestana == PestanaBloqueosOrg:
		return listaBloqueados(v, d.DeLaOrg, v.Traduce("moderation.orgBlocks.empty"), d.AlDesbloquearOrg)
	case len(d.Denuncias) == 0:
		return Parrafo(v, v.Traduce("moderation.empty"), tema.TextoSM, ui.Normal, "text-muted")
	}
	filas := make([]ui.Bloque, 0, len(d.Denuncias))
	for _, r := range d.Denuncias {
		filas = append(filas, ui.Bloque{Obj: fichaDenuncia(v, r, d), Margen: tema.Espacio(3)})
	}
	return ui.PilaBloques(filas...)
}

// listaBloqueados: una fila por persona, con su «Desbloquear a …». El rótulo lleva el
// nombre: con varias filas, un lector oiría «Desbloquear» tantas veces como personas.
func listaBloqueados(v Vista, lista []api.CastBlocked, vacio string, alQuitar func(api.CastBlocked)) fyne.CanvasObject {
	if len(lista) == 0 {
		return Parrafo(v, vacio, tema.TextoSM, ui.Normal, "text-muted")
	}
	filas := make([]ui.Bloque, 0, len(lista))
	for _, b := range lista {
		nombre := b.Name
		if nombre == "" {
			nombre = b.Sub
		}
		texto := ui.PilaBloques(
			ui.Bloque{Obj: Parrafo(v, nombre, tema.TextoSM, ui.SemiNegrita, "text")},
			ui.Bloque{Obj: Parrafo(v, Hace(v, b.CreatedAt), tema.TextoXS, ui.Normal, "text-subtle")},
		)
		quitar := ui.NuevoBoton(v.Traduce("moderation.unblock", i18n.Vars{"name": nombre}), func() {
			if alQuitar != nil {
				alQuitar(b)
			}
		}).Con(ui.Secundario, ui.TallaSM)
		filas = append(filas, ui.Bloque{
			Obj:    container.New(ui.FilaAuto{Hueco: HuecoFila, Derecha: 1, Elastico: 1, Partir: true}, texto, quitar),
			Margen: tema.Espacio(3),
		})
	}
	return ui.PilaBloques(filas...)
}

// claveTipoDenuncia y claveAccion son los rótulos de cada valor del servicio.
func claveTipoDenuncia(tipo string) string { return "moderation.kind." + tipo }

func claveAccion(accion string) string { return "moderation.action." + accion }

// fichaDenuncia es una fila de la cola: qué se denunció, por qué y cuándo; de quién es
// y qué decía; lo que añadió quien denuncia; y las acciones que el servicio admite. No
// dice quién denunció: el servicio no lo manda.
func fichaDenuncia(v Vista, r api.CastReport, d Moderacion) fyne.CanvasObject {
	cabecera := []fyne.CanvasObject{
		ui.NuevaInsignia(v.Traduce(claveTipoDenuncia(r.Kind)), ui.InsigniaNeutra),
		ui.Texto(v.Traduce("report.reason."+r.Reason), tema.TextoSM, ui.SemiNegrita, tema.Color("text", v.Variante)),
		ui.Texto(Hace(v, r.CreatedAt), tema.TextoXS, ui.Normal, tema.Color("text-subtle", v.Variante)),
	}
	if r.Anonymous {
		cabecera = append(cabecera, ui.Texto(v.Traduce("moderation.anonymous"), tema.TextoXS, ui.Normal, tema.Color("text-subtle", v.Variante)))
	}
	if r.Resolution != "" {
		cabecera = append(cabecera, ui.NuevaInsignia(v.Traduce(claveAccion(r.Resolution)), ui.InsigniaInfo))
	}
	bloques := []ui.Bloque{{
		Obj:    container.New(ui.FilaAuto{Hueco: HuecoFila, Derecha: len(cabecera), Partir: true, SaltoFila: tema.Espacio(1)}, cabecera...),
		Margen: tema.Espacio(2),
	}}
	if r.TargetName != "" {
		bloques = append(bloques, ui.Bloque{Obj: Parrafo(v, r.TargetName, tema.TextoSM, ui.SemiNegrita, "text"), Margen: tema.Espacio(1)})
	}
	if r.Excerpt != "" {
		bloques = append(bloques, ui.Bloque{Obj: Parrafo(v, r.Excerpt, tema.TextoSM, ui.Normal, "text"), Margen: tema.Espacio(1)})
	}
	if r.Details != "" {
		bloques = append(bloques, ui.Bloque{Obj: Parrafo(v, r.Details, tema.TextoSM, ui.Normal, "text-muted"), Margen: tema.Espacio(1)})
	}
	var botones []fyne.CanvasObject
	if r.VideoID != "" && d.AlVer != nil {
		botones = append(botones, ui.NuevoBoton(v.Traduce("manage.view"), func() { d.AlVer(r.VideoID) }).Con(ui.Fantasma, ui.TallaSM))
	}
	for _, accion := range r.Actions {
		variante := ui.Secundario
		if accion == api.CastActionDismiss {
			variante = ui.Fantasma
		}
		b := ui.NuevoBoton(v.Traduce(claveAccion(accion)), func() {
			if d.AlResolver != nil {
				d.AlResolver(r, accion)
			}
		}).Con(variante, ui.TallaSM)
		b.Inerte = d.EnCurso == r.ID
		botones = append(botones, b)
	}
	if len(botones) > 0 {
		bloques[len(bloques)-1].Margen = tema.Espacio(3)
		bloques = append(bloques, ui.Bloque{Obj: Fila(botones...)})
	}
	caja := ui.NuevaCaja(tema.Color("surface", v.Variante), tema.Color("border", v.Variante), tema.RadioMD)
	return container.New(&recuadroFino{caja: caja}, append(caja.Objetos(), ui.PilaBloques(bloques...))...)
}

// ---- El diálogo de denuncia ----

// TituloDenuncia es el del diálogo, según qué se denuncia.
func TituloDenuncia(v Vista, tipo, nombre string) string {
	if tipo == api.CastReportPerson {
		return v.Traduce("report.title.person", i18n.Vars{"name": nombre})
	}
	return v.Traduce("report.title." + tipo)
}

// FormularioDenuncia es el cuerpo del diálogo: el motivo, de la lista cerrada, y un
// texto opcional. `AlElegir` avisa del primer motivo elegido: hasta entonces no se
// puede enviar.
type FormularioDenuncia struct {
	Motivo   string
	Detalle  *ui.AreaTexto
	Radios   []*ui.Eleccion
	AlElegir func()
}

// maxDetalleDenuncia es el tope del servicio para el texto libre.
const maxDetalleDenuncia = 1000

// Objeto arma el formulario.
func (f *FormularioDenuncia) Objeto(v Vista) fyne.CanvasObject {
	filas := make([]fyne.CanvasObject, 0, len(api.CastMotivos)+4)
	filas = append(filas, ui.NuevoParrafo(v.Traduce("report.reason"), tema.TextoSM, ui.SemiNegrita, "text", v.Variante))
	f.Radios = f.Radios[:0]
	for _, motivo := range api.CastMotivos {
		r := ui.NuevoRadio(v.Traduce("report.reason."+motivo), nil)
		r.Escalon = tema.TextoSM
		r.AlCambiar = func(bool) { f.Elegir(motivo) }
		f.Radios = append(f.Radios, r)
		filas = append(filas, r)
	}
	f.Detalle = ui.NuevaAreaTexto("")
	f.Detalle.Filas = 3
	filas = append(filas,
		ui.NuevoParrafo(v.Traduce("report.details"), tema.TextoSM, ui.SemiNegrita, "text", v.Variante), f.Detalle,
		ui.NuevoParrafo(v.Traduce("report.privacy"), tema.TextoXS, ui.Normal, "text-muted", v.Variante),
	)
	return container.New(ui.Columna{Hueco: tema.Espacio(2)}, filas...)
}

// Elegir marca un motivo y desmarca los demás: es un grupo de una sola opción.
func (f *FormularioDenuncia) Elegir(motivo string) {
	f.Motivo = motivo
	for i, r := range f.Radios {
		r.Marcada = api.CastMotivos[i] == motivo
		r.Refresh()
	}
	if f.AlElegir != nil {
		f.AlElegir()
	}
}

// TextoDetalle es lo escrito, recortado al tope del servicio.
func (f *FormularioDenuncia) TextoDetalle() string {
	if f.Detalle == nil {
		return ""
	}
	texto := []rune(strings.TrimSpace(f.Detalle.Texto()))
	return string(texto[:min(len(texto), maxDetalleDenuncia)])
}

// AbrirDenuncia abre el diálogo de una denuncia. `nombre` es el de la persona, cuando
// se denuncia a una. Con sesión va por la ruta con sesión; sin ella, por la pública,
// con su prueba de trabajo.
func (a *App) AbrirDenuncia(r api.CastNewReport, nombre string) {
	if a.w == nil || a.cli == nil {
		return
	}
	f := &FormularioDenuncia{}
	var d *ui.Dialogo
	f.AlElegir = func() { d.SinConfirmar(false) }
	d = ui.Confirmacion(TituloDenuncia(a.v, r.Kind, nombre), a.v.Traduce("ui.cancel"), a.v.Traduce("report.send"), false,
		ui.AlAncho(anchoDialogo(a), f.Objeto(a.v)), nil, func() {
			d.Ocupado(true)
			a.enviarDenuncia(r, f, func(err error) {
				d.Ocupado(false)
				if err != nil {
					a.avisar(ui.AvisoPeligro, a.textoDenunciaFallida(err))
					return
				}
				d.Cerrar()
				a.avisar(ui.AvisoExito, a.v.Traduce("report.sent"))
			})
		})
	d.RotuloCerrar = a.v.Traduce("ui.close")
	d.SinConfirmar(true)
	d.Mostrar(a.w.Canvas())
}

// enviarDenuncia manda lo elegido en el formulario y avisa con el resultado.
func (a *App) enviarDenuncia(r api.CastNewReport, f *FormularioDenuncia, hecho func(error)) {
	r.Reason, r.Details = f.Motivo, f.TextoDetalle()
	gen, conSesion := a.gen, a.dentro()
	a.lanzar(func() {
		c, fin := ctx()
		defer fin()
		var err error
		if conSesion {
			err = a.cli.Report(c, r)
		} else {
			err = a.cli.PublicReport(c, r)
		}
		a.hilo(gen, func() { hecho(err) })
	})
}

func (a *App) textoDenunciaFallida(err error) string {
	var e *api.Error
	if errors.As(err, &e) && e.Status == http.StatusTooManyRequests {
		return a.v.Traduce("report.tooMany")
	}
	return a.v.Traduce("errors.title")
}

// ---- Bloquear ----

// bloqueado dice si quien mira ha bloqueado a esa persona en esta sesión: lo suyo se
// quita de la pantalla en el acto (el servicio deja de mandarlo en la carga siguiente).
func (a *App) bloqueado(sub string) bool { return a.bloqueados[sub] }

// confirmarBloqueo pregunta y bloquea a una persona que aparece en el vídeo abierto.
func (a *App) confirmarBloqueo(sub, nombre string) {
	vars := i18n.Vars{"name": nombre}
	// El botón, «Bloquear» a secas (como la web): el nombre ya va en el título.
	a.confirmar(a.v.Traduce("block.action", vars), a.v.Traduce("block.confirm", vars), a.v.Traduce("block.short"),
		func() { a.bloquear(sub, nombre) })
}

func (a *App) bloquear(sub, nombre string) {
	if a.cli == nil {
		return
	}
	gen, id := a.gen, a.ver.id
	a.lanzar(func() {
		c, fin := ctx()
		defer fin()
		err := a.cli.Block(c, id, sub)
		a.hilo(gen, func() {
			if err != nil {
				a.avisar(ui.AvisoPeligro, a.v.Traduce("errors.title"))
				return
			}
			if a.bloqueados == nil {
				a.bloqueados = map[string]bool{}
			}
			a.bloqueados[sub] = true
			a.avisar(ui.AvisoExito, a.v.Traduce("block.done", i18n.Vars{"name": nombre}))
			a.pintar()
		})
	})
}

// ---- La pantalla ----

type estadoModeracion struct {
	estadoCarga
	pedidos    bool
	mios       []api.CastBlocked
	administra bool
	pestana    int
	denuncias  []api.CastReport
	deLaOrg    []api.CastBlocked
	// cargandoCola y errorCola son de la pestaña de quien administra.
	cargandoCola bool
	errorCola    bool
	enCurso      string
}

// esProhibido: el servicio dice que quien pregunta no administra la organización.
func esProhibido(err error) bool {
	var e *api.Error
	return errors.As(err, &e) && e.Status == http.StatusForbidden
}

// pedirAbiertas pregunta cuántas denuncias hay sin resolver, para el contador del
// menú. De paso el servicio apunta a quien pregunta como moderador, que es a quien
// avisa de las nuevas. A quien no administra le contesta 403 y aquí no pasa nada.
func (a *App) pedirAbiertas() {
	if !a.dentro() || a.cli == nil {
		return
	}
	a.lanzar(func() {
		c, fin := ctx()
		defer fin()
		n, err := a.cli.OpenReports(c)
		a.enLaInterfaz(func() {
			if err != nil {
				n = 0
			}
			if a.v.Denuncias != n {
				a.v.Denuncias = n
				a.pintar()
			}
		})
	})
}

// pedirModeracion trae los bloqueos de quien mira y, si administra, su pestaña.
func (a *App) pedirModeracion() {
	if !a.dentro() || a.cli == nil {
		return
	}
	a.mod.estadoCarga = estadoCarga{cargando: true}
	gen := a.gen
	a.lanzar(func() {
		c, fin := ctx()
		defer fin()
		mios, err := a.cli.Blocks(c)
		abiertas, errCola := 0, error(nil)
		if err == nil {
			abiertas, errCola = a.cli.OpenReports(c)
		}
		a.hilo(gen, func() {
			a.mod.estadoCarga, a.mod.pedidos = estadoCarga{fallo: err}, a.mod.pedidos || err == nil
			if err == nil {
				a.mod.mios = mios
				a.mod.administra = errCola == nil
				a.v.Denuncias = abiertas
			}
			a.pintar()
			if a.mod.administra {
				a.pedirCola()
			}
		})
	})
}

// pedirCola trae la pestaña activa de quien administra.
func (a *App) pedirCola() {
	if a.cli == nil {
		return
	}
	a.mod.cargandoCola, a.mod.errorCola = true, false
	a.pintar()
	gen, pestana := a.gen, a.mod.pestana
	a.lanzar(func() {
		c, fin := ctx()
		defer fin()
		var ds []api.CastReport
		var bs []api.CastBlocked
		var err error
		if pestana == PestanaBloqueosOrg {
			bs, err = a.cli.OrgBlocks(c)
		} else {
			ds, err = a.cli.Reports(c, pestana == PestanaResueltas)
		}
		a.hilo(gen, func() {
			if pestana != a.mod.pestana {
				return // se cambió de pestaña mientras llegaba
			}
			a.mod.cargandoCola, a.mod.errorCola = false, err != nil
			a.mod.administra = a.mod.administra && !esProhibido(err)
			if err == nil {
				a.mod.denuncias, a.mod.deLaOrg = ds, bs
			}
			a.pintar()
		})
	})
}

func (a *App) resolverDenuncia(r api.CastReport, accion string) {
	if a.cli == nil || a.mod.enCurso != "" {
		return
	}
	a.mod.enCurso = r.ID
	a.pintar()
	gen := a.gen
	a.lanzar(func() {
		c, fin := ctx()
		defer fin()
		err := a.cli.ResolveReport(c, r.ID, accion)
		a.hilo(gen, func() {
			a.mod.enCurso = ""
			if err != nil {
				a.avisar(ui.AvisoPeligro, a.v.Traduce("errors.title"))
			} else {
				// Actuar cierra todas las denuncias sobre lo mismo; descartar, solo esta.
				a.mod.denuncias = slices.DeleteFunc(a.mod.denuncias, func(x api.CastReport) bool {
					if accion == api.CastActionDismiss {
						return x.ID == r.ID
					}
					return x.Kind == r.Kind && x.TargetID == r.TargetID
				})
				a.v.Denuncias = len(a.mod.denuncias)
			}
			a.pintar()
		})
	})
}

// quitarBloqueo deshace un bloqueo, propio o de la organización.
func (a *App) quitarBloqueo(b api.CastBlocked, deLaOrg bool) {
	if a.cli == nil {
		return
	}
	gen := a.gen
	a.lanzar(func() {
		c, fin := ctx()
		defer fin()
		var err error
		if deLaOrg {
			err = a.cli.UnblockOrg(c, b.Sub)
		} else {
			err = a.cli.Unblock(c, b.Sub)
		}
		a.hilo(gen, func() {
			if err != nil {
				a.avisar(ui.AvisoPeligro, a.v.Traduce("errors.title"))
				return
			}
			fuera := func(x api.CastBlocked) bool { return x.Sub == b.Sub }
			if deLaOrg {
				a.mod.deLaOrg = slices.DeleteFunc(a.mod.deLaOrg, fuera)
			} else {
				a.mod.mios = slices.DeleteFunc(a.mod.mios, fuera)
				delete(a.bloqueados, b.Sub)
			}
			nombre := b.Name
			if nombre == "" {
				nombre = b.Sub
			}
			a.avisar(ui.AvisoExito, a.v.Traduce("moderation.unblocked", i18n.Vars{"name": nombre}))
			a.pintar()
		})
	})
}

func (a *App) pantallaModeracion() fyne.CanvasObject {
	if !a.dentro() {
		return PantallaModeracion(a.sinSesion(), Moderacion{})
	}
	e := a.mod
	v := a.vistaDe(e.estadoCarga, !e.pedidos, false, a.pedirModeracion)
	return PantallaModeracion(v, Moderacion{
		Mios: e.mios, Administra: e.administra, Pestana: e.pestana,
		Denuncias: e.denuncias, DeLaOrg: e.deLaOrg, Cargando: e.cargandoCola, ErrorCola: e.errorCola, EnCurso: e.enCurso,
		AlPestana: func(i int) {
			a.mod.pestana, a.mod.denuncias, a.mod.deLaOrg = i, nil, nil
			a.pedirCola()
		},
		AlDesbloquear:    func(b api.CastBlocked) { a.quitarBloqueo(b, false) },
		AlDesbloquearOrg: func(b api.CastBlocked) { a.quitarBloqueo(b, true) },
		AlResolver:       a.resolverDenuncia,
		AlVer:            a.AbrirVideo,
		AlReintentarCola: a.pedirCola,
	})
}
