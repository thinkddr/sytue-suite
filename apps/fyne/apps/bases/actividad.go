// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package bases

// paridad-web: apps/frontend/apps/bases/src/pages/base/[id].astro

import (
	"encoding/json"
	"strings"
	"time"
	"unicode/utf8"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"

	"git.sytue.eu/sytue/one/apps/fyne/tema"
	"git.sytue.eu/sytue/one/apps/fyne/ui"
	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/buscar"
	"git.sytue.eu/sytue/one/apps/nucleo/i18n"
)

// La actividad de una base (su traza, solo para el propietario) contada para personas,
// y la búsqueda de «Mis bases». Es el espejo de `lib/actividad.ts` de la web: la misma
// entrada se cuenta con la misma frase en las dos.

// paginaActividad es cuántas entradas se piden cada vez (PAGINA_ACTIVIDAD).
const paginaActividad = 50

// maxDescripcion es el tope de la descripción de una base: el mismo que el servicio.
const maxDescripcion = 1000

// frasesDeAccion: varias acciones técnicas comparten frase (cambiar la fórmula o las
// opciones de un campo es «cambiar un campo»). Va por la acción con ESPACIOS en vez de
// puntos (`record update` es `record.update`): con puntos, el guardián de catálogos las
// leería como claves de i18n que no existen.
var frasesDeAccion = map[string]string{
	"base create":        "activity.do.baseCreate",
	"base update":        "activity.do.baseUpdate",
	"base transfer":      "activity.do.baseTransfer",
	"table create":       "activity.do.tableCreate",
	"table rename":       "activity.do.tableRename",
	"table delete":       "activity.do.tableDelete",
	"table import":       "activity.do.tableImport",
	"table export":       "activity.do.tableExport",
	"field create":       "activity.do.fieldCreate",
	"field rename":       "activity.do.fieldRename",
	"field delete":       "activity.do.fieldDelete",
	"field options":      "activity.do.fieldChange",
	"field formula":      "activity.do.fieldChange",
	"field reorder":      "activity.do.fieldReorder",
	"record create":      "activity.do.recordCreate",
	"record create form": "activity.do.recordForm",
	"record update":      "activity.do.recordUpdate",
	"record link":        "activity.do.recordUpdate",
	"record delete":      "activity.do.recordDelete",
	"view create":        "activity.do.viewCreate",
	"view update":        "activity.do.viewUpdate",
	"view delete":        "activity.do.viewDelete",
	"permission grant":   "activity.do.permissionGrant",
	"permission revoke":  "activity.do.permissionRevoke",
	"comment create":     "activity.do.commentCreate",
	"comment delete":     "activity.do.commentDelete",
	"attachment add":     "activity.do.attachmentAdd",
	"attachment delete":  "activity.do.attachmentDelete",
	"form link create":   "activity.do.formLink",
	"form link revoke":   "activity.do.formLink",
	"connector grant":    "activity.do.connection",
	"connector revoke":   "activity.do.connection",
	"connector rotate":   "activity.do.connection",
	"connector read":     "activity.do.connectionUse",
	"connector denied":   "activity.do.connectionUse",
}

// contextoActividad es lo que ya se sabe de la base para contar sus entradas.
type contextoActividad struct {
	yo       string
	tablas   []api.BasesTable
	permisos []api.BasesPermission
}

// entradaContada es una entrada lista para pintar.
type entradaContada struct {
	Quien, Que string
	// Sobre es de qué trata («Importe · Pedidos»), o vacío si la entrada no lo dice.
	Sobre string
	At    time.Time
}

// textoDe lee `clave` de un objeto JSON si es una cadena.
func textoDe(crudo json.RawMessage, clave string) string {
	var m map[string]any
	if json.Unmarshal(crudo, &m) != nil {
		return ""
	}
	s, _ := m[clave].(string)
	return strings.TrimSpace(s)
}

// nombreDePersona es «Tú», el nombre del directorio de quien tiene permiso o «Otra
// persona»: un `sub` no se enseña nunca.
func (p *Pantalla) nombreDePersona(sub string, c contextoActividad) string {
	if sub != "" && sub == c.yo {
		return p.t("activity.you")
	}
	for _, x := range c.permisos {
		if (x.PrincipalKind == "user" || x.PrincipalKind == "external") && x.PrincipalID == sub && x.PrincipalLabel != "" {
			return x.PrincipalLabel
		}
	}
	return p.t("activity.someone")
}

// destinatario es a quién se dio o quitó acceso, con las palabras de compartir.
func (p *Pantalla) destinatario(e api.BasesAuditEntry, c contextoActividad) string {
	datos := e.After
	if len(datos) == 0 {
		datos = e.Before
	}
	quien := textoDe(datos, "principal")
	switch textoDe(datos, "kind") {
	case "tenant":
		return p.t("share.kind.tenant")
	case "group":
		return p.t("share.groupLabel", i18n.Vars{"name": quien})
	}
	if quien == "" {
		return ""
	}
	return p.nombreDePersona(quien, c)
}

// contarEntrada cuenta una entrada de la traza.
func (p *Pantalla) contarEntrada(e api.BasesAuditEntry, c contextoActividad) entradaContada {
	que := p.t("activity.do.other", i18n.Vars{"action": e.Action})
	if clave, ok := frasesDeAccion[strings.ReplaceAll(e.Action, ".", " ")]; ok {
		que = p.t(clave)
	}
	tabla := ""
	for _, x := range c.tablas {
		if e.TableID != "" && x.ID == e.TableID {
			tabla = x.Name
		}
	}
	propio := ""
	switch {
	case strings.HasPrefix(e.Action, "permission."):
		propio = p.destinatario(e, c)
	case !strings.HasPrefix(e.Action, "base."):
		if propio = textoDe(e.After, "name"); propio == "" {
			propio = textoDe(e.Before, "name")
		}
	}
	// En las de tabla el nombre propio YA es el de la tabla: no se repite.
	partes := []string{propio, tabla}
	if strings.HasPrefix(e.Action, "table.") {
		partes = []string{propio}
		if propio == "" {
			partes = []string{tabla}
		}
	}
	var sobre []string
	for _, x := range partes {
		if x != "" {
			sobre = append(sobre, x)
		}
	}
	return entradaContada{Quien: p.nombreDePersona(e.Actor, c), Que: que, Sobre: strings.Join(sobre, " · "), At: e.At}
}

// filtrarBases son las bases cuyo nombre o descripción contienen lo escrito, con la
// regla de `plegar` («cafe» encuentra «Café»). Sin texto, todas.
func filtrarBases(bases []api.BasesBase, q string) []api.BasesBase {
	q = strings.TrimSpace(q)
	if q == "" {
		return bases
	}
	var out []api.BasesBase
	for _, b := range bases {
		if buscar.Contiene(b.Name, q) || buscar.Contiene(b.Description, q) {
			out = append(out, b)
		}
	}
	return out
}

// panelActividad es el panel lateral con la actividad de la base abierta.
type panelActividad struct {
	p    *Pantalla
	base string
	ctx  contextoActividad
	// entradas es nil mientras llega la primera página.
	entradas []api.BasesAuditEntry
	hayMas   bool
	cargando bool
	fallo    string
	d        *ui.Dialogo
}

// abrirActividad abre el panel de la base abierta y pide la primera página con los
// permisos (para ponerle nombre a quien hizo cada cosa; si fallan, se ve igual).
func (p *Pantalla) abrirActividad() *panelActividad {
	b := p.abierta
	a := &panelActividad{p: p, base: b.ID, ctx: contextoActividad{tablas: b.Tablas}}
	if p.Yo != nil {
		a.ctx.yo = p.Yo()
	}
	nombre := b.Datos.Name
	if nombre == "" {
		nombre = p.t("nav.title")
	}
	a.d = ui.NuevoDialogo(p.t("activity.title", i18n.Vars{"name": nombre}), container.New(ui.Columna{})).ALaDerecha()
	a.d.Con(ui.DialogoMD, container.New(ui.Fila{Hueco: ui.HuecoPieDialogo}))
	a.d.RotuloCerrar = p.t("ui.close")
	a.rehacer()
	a.d.Mostrar(p.ventana.Canvas())
	if p.svc != nil {
		a.cargando = true
		p.fondo(func() {
			ctx, cancelar := p.ctx()
			defer cancelar()
			permisos, _ := p.svc.ListPermissions(ctx, a.base)
			es, err := p.svc.ListAudit(ctx, a.base, 0, paginaActividad)
			enHilo(func() { a.llegan(es, err, permisos) })
		})
	}
	return a
}

// llegan añade una página (o su fallo) y repinta.
func (a *panelActividad) llegan(es []api.BasesAuditEntry, err error, permisos []api.BasesPermission) {
	a.cargando = false
	if permisos != nil {
		a.ctx.permisos = permisos
	}
	if a.entradas == nil {
		a.entradas = []api.BasesAuditEntry{}
	}
	if err != nil {
		a.fallo = a.p.mensajeDeError(err)
	} else {
		a.fallo = ""
		a.entradas = append(a.entradas, es...)
		a.hayMas = len(es) >= paginaActividad
	}
	a.rehacer()
}

// cargarMas pide las anteriores a la última que se ve.
func (a *panelActividad) cargarMas() {
	p := a.p
	if a.cargando || len(a.entradas) == 0 || p.svc == nil {
		return
	}
	antes := a.entradas[len(a.entradas)-1].ID
	a.cargando = true
	a.rehacer()
	p.fondo(func() {
		ctx, cancelar := p.ctx()
		defer cancelar()
		es, err := p.svc.ListAudit(ctx, a.base, antes, paginaActividad)
		enHilo(func() { a.llegan(es, err, nil) })
	})
}

// rehacer vuelve a armar la lista y el pie con el estado de ahora.
func (a *panelActividad) rehacer() {
	p := a.p
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	ancho := p.anchoDialogo(560)
	var hijos []fyne.CanvasObject
	switch {
	case a.entradas == nil:
		hijos = append(hijos, ui.NuevoEsqueleto(0, 6*tema.Rem))
	case len(a.entradas) == 0 && a.fallo == "":
		hijos = append(hijos, ui.NuevoParrafo(p.t("activity.empty"), tema.TextoSM, ui.Normal, "text-muted", v).A(ancho))
	default:
		filas := make([]fyne.CanvasObject, 0, 2*len(a.entradas))
		for i, e := range a.entradas {
			if i > 0 {
				// `border-b border-border-subtle` entre entradas.
				raya := canvas.NewRectangle(tema.Color("border-subtle", v))
				raya.SetMinSize(fyne.NewSize(ancho, tema.Borde))
				filas = append(filas, raya)
			}
			filas = append(filas, a.fila(p.contarEntrada(e, a.ctx), v, ancho))
		}
		hijos = append(hijos, container.New(ui.Columna{Hueco: tema.Espacio(3)}, filas...))
	}
	if a.fallo != "" {
		hijos = append(hijos, ui.NuevoParrafo(a.fallo, tema.TextoSM, ui.Normal, "danger-text", v).A(ancho))
	}
	a.d.CambiarCuerpo(container.New(ui.Columna{Hueco: tema.Espacio(4)}, hijos...))
	pie := []fyne.CanvasObject{}
	if a.hayMas {
		mas := ui.NuevoBoton(p.t("activity.more"), a.cargarMas).Con(ui.Secundario, ui.TallaMD)
		mas.Cargando = a.cargando
		pie = append(pie, mas)
	}
	pie = append(pie, ui.NuevoBoton(p.t("ui.close"), func() { a.d.Cerrar() }))
	a.d.CambiarPie(container.New(ui.Fila{Hueco: ui.HuecoPieDialogo}, pie...))
}

// fila es una entrada: «Quién · Qué», de qué trata y cuándo.
func (a *panelActividad) fila(c entradaContada, v tema.Variante, ancho float32) fyne.CanvasObject {
	p := a.p
	partes := []fyne.CanvasObject{
		ui.NuevoParrafo(c.Quien+" · "+c.Que, tema.TextoSM, ui.Normal, "text", v).A(ancho),
	}
	if c.Sobre != "" {
		sobre := ui.NuevoRotulo(c.Sobre, tema.TextoSM, ui.Normal, "text-muted")
		sobre.Truncar = true
		partes = append(partes, sobre)
	}
	partes = append(partes, ui.NuevoRotulo(i18n.FormatDateTime(p.tr.Locale(), c.At.Local()), tema.TextoXS, ui.Normal, "text-muted"))
	return container.New(ui.Columna{Hueco: tema.Espacio(0.5)}, partes...)
}

// editarDescripcion abre el diálogo de la descripción de la base. Vacía es válida: se
// quita. Se guarda con el nombre de ahora: el servicio cambia los dos.
func (p *Pantalla) editarDescripcion() {
	b := p.abierta
	area := ui.NuevaAreaTexto("")
	area.Rotulo, area.Ancho = p.t("base.descriptionLabel"), p.anchoDialogo(480)
	area.PonerTexto(b.Datos.Description)
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	fallo := ui.NuevoParrafo("", tema.TextoSM, ui.Normal, "danger-text", v).A(area.Ancho)
	cuerpo := container.New(ui.Columna{Hueco: tema.Espacio(2)},
		ui.GrupoCampo(p.t("base.descriptionLabel"), false, area, "", ""),
		ui.NuevoParrafo(p.t("base.descriptionHelp"), tema.TextoXS, ui.Normal, "text-muted", v).A(area.Ancho),
		fallo)
	d := ui.NuevoDialogo(p.t("base.editDescription"), cuerpo)
	var guardar *ui.Boton
	guardar = ui.NuevoBoton(p.t("ui.save"), func() {
		nueva := strings.TrimSpace(area.Texto())
		guardar.Cargando = true
		guardar.Refresh()
		p.fondo(func() {
			ctx, cancelar := p.ctx()
			defer cancelar()
			err := p.svc.UpdateBase(ctx, b.ID, b.Datos.Name, nueva)
			enHilo(func() {
				guardar.Cargando = false
				guardar.Refresh()
				if err != nil {
					fallo.Texto = p.mensajeDeError(err)
					fallo.Refresh()
					return
				}
				d.Cerrar()
				if p.abierta == b {
					p.cargarEstructura()
				}
			})
		})
	})
	ajustar := func() {
		guardar.Inerte = strings.TrimSpace(area.Texto()) == strings.TrimSpace(b.Datos.Description) || p.svc == nil
		guardar.Refresh()
	}
	area.AlCambiar(func(s string) {
		if utf8.RuneCountInString(s) > maxDescripcion { // el `maxLength` del textarea
			area.PonerTexto(string([]rune(s)[:maxDescripcion]))
		}
		ajustar()
	})
	ajustar()
	cancelar := ui.NuevoBoton(p.t("ui.cancel"), func() { d.Cerrar() }).Con(ui.Fantasma, ui.TallaMD)
	d.Con(ui.DialogoMD, container.New(ui.Fila{Hueco: ui.HuecoPieDialogo}, cancelar, guardar))
	d.RotuloCerrar = p.t("ui.close")
	d.Foco = area
	d.Mostrar(p.ventana.Canvas())
}
