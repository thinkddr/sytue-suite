// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package drive

import (
	"errors"
	"image/color"
	"net/http"
	"strings"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"

	"git.sytue.eu/sytue/one/apps/fyne/iconos"
	"git.sytue.eu/sytue/one/apps/fyne/tema"
	"git.sytue.eu/sytue/one/apps/fyne/ui"
	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/i18n"
)

// # Compartir
//
// `ShareDialog` de FileBrowser.tsx, talla lg, en su orden: con una PERSONA de la
// organización primero —es lo que se busca casi siempre—, con un GRUPO propio si se
// pertenece a alguno, y el ENLACE con su nivel de acceso por lo que permite hacer, su
// caducidad y su contraseña; debajo, lo que ya está compartido, con «Revocar».
//
// La etiqueta de sensibilidad que prohíbe compartir fuera se dice AL ABRIR, no al fallar:
// viaja con el fichero. Si el bloqueo aparece al intentarlo (un 403 con motivo), el
// motivo lo redacta el servicio y se queda escrito; un aviso se iría y dejaría a la
// persona sin saber qué hacer.
//
// La caducidad se teclea (dd/mm/aaaa) o se elige en el calendario que abre su icono
// (`ui.ConCalendario`): el de la web lo pinta el navegador y no tiene hoja que copiar.
//
// ponytail: los dos desplegables son el `ui.Selector`, que pinta `text-base px-3` donde
// el `<select>` suelto de la web es `text-sm px-2`.

// Medidas del diálogo.
const (
	// AnchoAccesoCompartir es el `w-44` del selector de acceso y AnchoCampoCompartir el
	// `w-56` de la caducidad y la contraseña.
	AnchoAccesoCompartir float32 = 44 * 0.25 * tema.Rem
	AnchoCampoCompartir  float32 = 56 * 0.25 * tema.Rem
	// MinCorreoCompartir es el `min-w-56` del campo de correo.
	MinCorreoCompartir float32 = 56 * 0.25 * tema.Rem
)

// dialogoCompartir es el estado del diálogo.
type dialogoCompartir struct {
	p  *Pantalla
	el Elemento
	d  *ui.Dialogo

	comparticiones []api.DriveShare
	personas       map[string]api.Persona
	falloLista     bool
	grupos         []string

	escribir       bool
	creado         string
	creadoEscribir bool
	creadoClave    bool
	creadoCaduca   time.Time
	bloqueo        string
	errorPersona   string
	revocando      string

	correo, clave, caduca                  *ui.Entrada
	accesoPersona, grupo, accesoGrupo      *ui.Selector
	anadirPersona, anadirGrupo, crear      *ui.Boton
	invitando, invitandoGrupo, creandoLink bool

	cuerpo *fyne.Container
	// anchoCuerpo y anchoPanel son los últimos anchos colocados: los de la pila de fuera
	// y la del «listo para enviar».
	anchoCuerpo, anchoPanel float32
}

// pedirCompartir abre el diálogo de un elemento y lo devuelve (para el guardián).
func (p *Pantalla) pedirCompartir(el Elemento) *dialogoCompartir {
	if p.win == nil || p.svc == nil {
		return nil
	}
	c := &dialogoCompartir{p: p, el: el, cuerpo: container.NewStack(), personas: map[string]api.Persona{}}
	c.anchoCuerpo = AnchoCuerpoDialogo(ui.DialogoLG, p.explor.Ventana)
	c.anchoPanel = c.anchoCuerpo - 2*(tema.Espacio(3)+tema.Borde)
	t := func(k string) string { return p.tr.T(k, nil) }
	accesos := []string{t("share.view.title"), t("share.edit.title")}

	c.correo = ui.NuevaEntrada(t("share.person.placeholder"))
	c.correo.AlCambiar(func(string) {
		if c.errorPersona != "" {
			c.errorPersona = ""
			c.pintar()
		}
		c.pintarBotones()
	})
	c.correo.AlTeclear = func(k *fyne.KeyEvent) bool {
		if k.Name == fyne.KeyReturn || k.Name == fyne.KeyEnter {
			c.invitarPersona()
			return true
		}
		return false
	}
	c.accesoPersona = ui.NuevoSelector(accesos, nil)
	c.anadirPersona = ui.NuevoBoton(t("share.person.add"), c.invitarPersona)

	c.grupo = ui.NuevoSelector(nil, func(int) { c.pintarBotones() })
	c.accesoGrupo = ui.NuevoSelector(accesos, nil)
	c.anadirGrupo = ui.NuevoBoton(t("share.group.add"), c.invitarGrupo)

	c.caduca = ui.ConCalendario(ui.CampoFecha(ui.CampoMD), p.tr.Locale(), time.Local)
	c.caduca.AlCambiar(func(string) { c.pintar() })
	c.clave = ui.NuevaEntrada(t("share.passwordPlaceholder"))
	c.clave.Contrasena(true)
	c.clave.AlCambiar(func(string) { c.pintar() })
	c.crear = ui.NuevoBoton(t("share.create"), c.crearEnlace)

	listo := ui.NuevoBoton(t("share.done"), func() { c.d.Cerrar() }).Con(ui.Primario, ui.TallaMD)
	c.d = ui.NuevoDialogo(p.tr.T("share.title", i18n.Vars{"name": el.Nombre}), c.cuerpo).Con(ui.DialogoLG, listo)
	c.d.RotuloCerrar = t("ui.close")
	c.pintar()
	c.d.Mostrar(p.win.Canvas())
	c.p.enSegundoPlano(func() { c.cargar() })
	if p.Grupos != nil {
		c.ponerGrupos(p.Grupos())
	} else {
		c.ponerGrupos(nil)
	}
	return c
}

func (c *dialogoCompartir) t(k string, v ...i18n.Vars) string {
	var vars i18n.Vars
	if len(v) > 0 {
		vars = v[0]
	}
	return c.p.tr.T(k, vars)
}

// ponerGrupos rellena el selector con la opción vacía delante, que es la de «elige uno».
func (c *dialogoCompartir) ponerGrupos(g []string) {
	c.grupos = g
	c.grupo.Opciones = append([]string{c.t("share.group.placeholder")}, g...)
	c.grupo.Elegida = 0
	c.pintar()
}

// bloqueaEtiqueta: la etiqueta del fichero prohíbe compartir fuera.
func (c *dialogoCompartir) bloqueaEtiqueta() bool {
	return c.el.Etiqueta != nil && c.el.Etiqueta.BlockExternalShare
}

// cargar trae las comparticiones vivas y los nombres de las personas con quien están.
func (c *dialogoCompartir) cargar() {
	ctx, cancelar := c.p.ctx()
	defer cancelar()
	lista, err := c.p.svc.Shares(ctx, claseDe(c.el), c.el.ID)
	personas := map[string]api.Persona{}
	if err == nil && c.p.Directorio != nil {
		for _, s := range lista {
			if s.Kind == "user" && s.Subject != "" {
				if per, e := c.p.Directorio.PorID(ctx, s.Subject); e == nil {
					personas[s.Subject] = per
				}
			}
		}
	}
	enHilo(func() {
		c.comparticiones, c.falloLista = lista, err != nil
		if c.comparticiones == nil && err == nil {
			c.comparticiones = []api.DriveShare{}
		}
		c.personas = personas
		c.pintar()
	})
}

func (c *dialogoCompartir) recargar() {
	enHilo(func() { c.comparticiones, c.falloLista = nil, false; c.pintar() })
	c.cargar()
}

// invitarPersona comparte con alguien de la organización por su correo. Un 404 es «esa
// dirección no es de nadie de aquí»: se queda escrito junto al campo, no en un aviso.
func (c *dialogoCompartir) invitarPersona() {
	correo := strings.TrimSpace(c.correo.Texto())
	if correo == "" || c.invitando || c.p.Directorio == nil {
		return
	}
	c.invitando, c.errorPersona = true, ""
	escribir := c.accesoPersona.Elegida == 1
	c.pintarBotones()
	c.p.enSegundoPlano(func() {
		ctx, cancelar := c.p.ctx()
		defer cancelar()
		per, err := c.p.Directorio.PorCorreo(ctx, correo)
		if err == nil {
			_, err = c.p.svc.ShareWith(ctx, claseDe(c.el), c.el.ID, "user", per.ID, escribir)
		}
		enHilo(func() {
			c.invitando = false
			switch {
			case err == nil:
				c.correo.PonerTexto("")
				c.p.avisar(ui.AvisoExito, c.t("share.person.done", i18n.Vars{"email": correo}), "")
			case codigo(err) == http.StatusNotFound:
				c.errorPersona = c.t("share.person.unknown")
			case codigo(err) == http.StatusForbidden && motivoLargo(err) != "":
				c.bloqueo = motivoLargo(err)
			default:
				c.p.avisar(ui.AvisoPeligro, c.t("share.person.failed"), "")
			}
			c.pintar()
		})
		if err == nil {
			c.recargar()
		}
	})
}

// invitarGrupo comparte con un grupo propio, por su nombre.
func (c *dialogoCompartir) invitarGrupo() {
	if c.grupo.Elegida <= 0 || c.invitandoGrupo {
		return
	}
	nombre := c.grupos[c.grupo.Elegida-1]
	escribir := c.accesoGrupo.Elegida == 1
	c.invitandoGrupo = true
	c.pintarBotones()
	c.p.enSegundoPlano(func() {
		ctx, cancelar := c.p.ctx()
		defer cancelar()
		_, err := c.p.svc.ShareWith(ctx, claseDe(c.el), c.el.ID, "group", nombre, escribir)
		enHilo(func() {
			c.invitandoGrupo = false
			switch {
			case err == nil:
				c.grupo.Elegida = 0
				c.p.avisar(ui.AvisoExito, c.t("share.group.done", i18n.Vars{"group": nombre}), "")
			case codigo(err) == http.StatusForbidden && motivoLargo(err) != "":
				c.bloqueo = motivoLargo(err)
			default:
				c.p.avisar(ui.AvisoPeligro, c.t("share.group.failed"), "")
			}
			c.pintar()
		})
		if err == nil {
			c.recargar()
		}
	})
}

// caducidad es el final del día LOCAL escrito en el campo, que es cuando el servicio
// hace vencer el enlace; cero si no hay fecha o no se entiende.
func (c *dialogoCompartir) caducidad() time.Time {
	d, vacia, ok := ui.LeerFecha(c.caduca.Texto(), time.Local)
	if vacia || !ok {
		return time.Time{}
	}
	return time.Date(d.Year(), d.Month(), d.Day(), 23, 59, 59, 0, time.Local)
}

// crearEnlace crea el enlace y enseña su dirección, que el servicio da una sola vez.
func (c *dialogoCompartir) crearEnlace() {
	if c.creandoLink || c.bloqueaEtiqueta() {
		return
	}
	clave := strings.TrimSpace(c.clave.Texto())
	caduca, escribir := c.caducidad(), c.escribir
	c.creandoLink = true
	c.pintarBotones()
	c.p.enSegundoPlano(func() {
		ctx, cancelar := c.p.ctx()
		defer cancelar()
		_, direccion, err := c.p.svc.ShareLink(ctx, claseDe(c.el), c.el.ID, escribir, clave, caduca)
		enHilo(func() {
			c.creandoLink = false
			switch {
			case err == nil:
				c.creado, c.creadoEscribir, c.creadoClave, c.creadoCaduca = direccion, escribir, clave != "", caduca
				c.bloqueo = ""
				c.clave.PonerTexto("")
			case codigo(err) == http.StatusForbidden && motivoLargo(err) != "":
				c.bloqueo = motivoLargo(err)
			default:
				c.p.avisar(ui.AvisoPeligro, c.t("share.createFailed"), "")
			}
			c.pintar()
		})
		if err == nil {
			c.recargar()
		}
	})
}

// revocar mata una compartición y la quita de la lista.
func (c *dialogoCompartir) revocar(s api.DriveShare) {
	if c.revocando != "" {
		return
	}
	c.revocando = s.ID
	c.pintar()
	c.p.enSegundoPlano(func() {
		ctx, cancelar := c.p.ctx()
		defer cancelar()
		err := c.p.svc.Revoke(ctx, s.ID)
		enHilo(func() {
			c.revocando = ""
			if err != nil {
				c.p.avisar(ui.AvisoPeligro, c.t("share.revokeFailed"), "")
			} else {
				quedan := c.comparticiones[:0:0]
				for _, x := range c.comparticiones {
					if x.ID != s.ID {
						quedan = append(quedan, x)
					}
				}
				c.comparticiones = quedan
				c.p.avisar(ui.AvisoExito, c.t("share.revoked"), "")
			}
			c.pintar()
		})
	})
}

// motivoLargo es el mensaje que el servicio da con un error —en compartir, un 403 que
// explica qué etiqueta lo bloquea—, o vacío si no dio ninguno.
func motivoLargo(err error) string {
	var e *api.Error
	if errors.As(err, &e) {
		return e.Message()
	}
	return ""
}

// FraseCaducidad es `expiryPhrase`: la caducidad contada en llano, por DÍA local, porque
// el servicio vence el enlace al final del día elegido.
func FraseCaducidad(tr *i18n.Translator, l i18n.Locale, cuando, ahora time.Time) string {
	dia := func(t time.Time) time.Time {
		t = t.In(time.Local)
		return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, time.Local)
	}
	dias := int(dia(cuando).Sub(dia(ahora)).Hours()/24 + 0.5)
	switch {
	case dias < 0:
		return tr.T("expiry.past", nil)
	case dias == 0:
		return tr.T("expiry.today", nil)
	case dias == 1:
		return tr.T("expiry.tomorrow", nil)
	}
	return tr.T("expiry.days", i18n.Vars{"count": dias, "date": Fecha(l, cuando)})
}

// pintarBotones pone los estados de los tres botones sin rehacer el diálogo.
func (c *dialogoCompartir) pintarBotones() {
	c.anadirPersona.Inerte = strings.TrimSpace(c.correo.Texto()) == ""
	c.anadirPersona.Cargando = c.invitando
	c.anadirGrupo.Inerte = c.grupo.Elegida <= 0
	c.anadirGrupo.Cargando = c.invitandoGrupo
	c.crear.Inerte = c.bloqueaEtiqueta()
	c.crear.Cargando = c.creandoLink
	for _, b := range []*ui.Boton{c.anadirPersona, c.anadirGrupo, c.crear} {
		b.Refresh()
	}
}

// pintar rehace el cuerpo con el estado de ahora. Los campos son los MISMOS de una vez
// a otra, así que no pierden ni el texto ni el foco.
func (c *dialogoCompartir) pintar() {
	v := c.p.variante()
	c.pintarBotones()
	var bloques []fyne.CanvasObject
	var separas []float32
	add := func(sep float32, o fyne.CanvasObject) {
		bloques, separas = append(bloques, o), append(separas, sep)
	}
	dir := c.el.Carpeta

	if c.bloqueo != "" || c.bloqueaEtiqueta() {
		titulo := "share.blockedFile"
		if dir {
			titulo = "share.blockedFolder"
		}
		motivo := c.bloqueo
		if motivo == "" {
			motivo = c.t("share.blockedByLabel", i18n.Vars{"name": c.el.Etiqueta.Name})
		}
		ayuda := ui.NuevaAyudaEnLinea(motivo + " " + c.t("share.blockedAdmin")).ConTono(ui.AyudaPeligro).ConTitulo(c.t(titulo))
		add(0, ayuda)
	}

	// Con una persona.
	pista := ui.NuevaPistaCampo(c.t("share.person.hint"))
	if c.errorPersona != "" {
		pista = ui.NuevaPistaCampo(c.errorPersona).ConTono(ui.AyudaPeligro)
	}
	add(0, leyenda(c.t("share.person.legend"), v))
	add(tema.Espacio(2), filaForm{
		campos: []fyne.CanvasObject{
			grupoCampo(c.t("share.person.email"), c.correo, pista),
			grupoCampo(c.t("share.person.access"), c.accesoPersona),
		},
		anchos: []float32{0, AnchoAccesoCompartir}, boton: c.anadirPersona, apilada: c.enTelefono(),
	}.objeto())

	// Con un grupo, si se pertenece a alguno.
	if len(c.grupos) > 0 {
		add(tema.Espacio(4), leyenda(c.t("share.group.legend"), v))
		add(tema.Espacio(2), filaForm{
			campos: []fyne.CanvasObject{
				grupoCampo(c.t("share.group.select"), c.grupo),
				grupoCampo(c.t("share.person.access"), c.accesoGrupo),
			},
			anchos: []float32{0, AnchoAccesoCompartir}, boton: c.anadirGrupo, apilada: c.enTelefono(),
		}.objeto())
	}

	// El enlace: el nivel por lo que PERMITE HACER.
	add(tema.Espacio(6), leyenda(c.t("share.legend"), v))
	sufijo := "file"
	if dir {
		sufijo = "folder"
	}
	ver := ui.NuevaTarjetaEleccion(c.t("share.view.title"), c.t("share.view."+sufijo), func() { c.escribir = false; c.pintar() })
	editar := ui.NuevaTarjetaEleccion(c.t("share.edit.title"), c.t("share.edit."+sufijo), func() { c.escribir = true; c.pintar() })
	ver.Icono, editar.Icono = iconos.Globo, iconos.Mas
	ver.Elegida, editar.Elegida = !c.escribir, c.escribir
	add(tema.Espacio(2), container.New(&dosColumnas{ventana: c.p.explor.Ventana}, ver, editar))

	caduca := c.caducidad()
	pistaCaduca := ui.NuevaPistaCampo(c.t("share.noExpiry"))
	if !caduca.IsZero() {
		pistaCaduca = ui.NuevaPistaCampo(FraseCaducidad(c.p.tr, c.p.tr.Locale(), caduca, time.Now())).ConTono(ui.AyudaExito)
	}
	pistaClave := ui.NuevaPistaCampo(c.t("share.passwordNone"))
	if strings.TrimSpace(c.clave.Texto()) != "" {
		pistaClave = ui.NuevaPistaCampo(c.t("share.passwordSet")).ConTono(ui.AyudaExito)
	}
	add(tema.Espacio(4), filaForm{
		campos: []fyne.CanvasObject{
			grupoCampo(c.t("share.until"), c.caduca, pistaCaduca),
			grupoCampo(c.t("share.password"), c.clave, pistaClave),
		},
		anchos: []float32{AnchoCampoCompartir, AnchoCampoCompartir}, hueco: tema.Espacio(3),
	}.objeto())
	crear := []fyne.CanvasObject{c.crear}
	if c.bloqueaEtiqueta() {
		crear = append(crear, ui.Texto(c.t("share.labelBlocksHint"), tema.TextoXS, ui.Normal, tema.Color("text-subtle", v)))
	}
	add(tema.Espacio(3), container.New(ui.Fila{Hueco: tema.Espacio(2)}, crear...))

	if c.creado != "" {
		add(tema.Espacio(4), c.panelCreado(v))
	}

	add(tema.Espacio(5), versalEnLinea(ui.NuevoVersal(c.t("share.liveHeading"), tema.TextoXS, ui.SemiNegrita, tema.Color("text-subtle", v))))
	add(tema.Espacio(2), c.lista(v))
	if len(c.comparticiones) > 0 {
		add(tema.Espacio(2), ui.Texto(c.t("share.revokeNote"), tema.TextoXS, ui.Normal, tema.Color("text-subtle", v)))
	}
	c.cuerpo.Objects = []fyne.CanvasObject{container.New(&pilaSeparada{separas: separas, ancho: &c.anchoCuerpo}, bloques...)}
	c.cuerpo.Refresh()
	c.d.Recolocar()
}

// leyenda es el `<legend>` de cada bloque: `text-base font-semibold`.
func leyenda(texto string, v tema.Variante) fyne.CanvasObject {
	return lineaTexto(ui.Texto(texto, tema.TextoBase, ui.SemiNegrita, tema.Color("text", v)), tema.TextoBase)
}

// enTelefono es el `max-md:` de la web: la ventana por debajo de 768.
func (c *dialogoCompartir) enTelefono() bool {
	return c.p.explor.Ventana > 0 && c.p.explor.Ventana < ui.Estrecho
}

// panelCreado es el «listo para enviar»: `border-success bg-success-bg rounded-md p-3`,
// con la dirección de solo lectura, «Copiar» y el resumen en frases completas.
func (c *dialogoCompartir) panelCreado(v tema.Variante) fyne.CanvasObject {
	campo := ui.NuevaEntrada("")
	campo.PonerTexto(c.creado)
	campo.SoloLectura(true)
	copiar := ui.NuevoBoton(c.t("share.copy"), func() {
		fyne.CurrentApp().Clipboard().SetContent(c.creado)
		c.p.avisar(ui.AvisoExito, c.t("share.copied"), "")
	}).Con(ui.Secundario, ui.TallaMD)
	resumen := c.t("share.summaryView")
	if c.creadoEscribir {
		resumen = c.t("share.summaryEdit")
	}
	if c.creadoCaduca.IsZero() {
		resumen += " " + c.t("share.summaryNoExpiry")
	} else {
		resumen += " " + FraseCaducidad(c.p.tr, c.p.tr.Locale(), c.creadoCaduca, time.Now())
	}
	if c.creadoClave {
		resumen += " " + c.t("share.summaryPassword")
	} else {
		resumen += " " + c.t("share.summaryNoPassword")
	}
	titulo := lineaTexto(ui.Texto(c.t("share.readyTitle"), tema.TextoSM, ui.SemiNegrita, tema.Color("success-text", v)), tema.TextoSM)
	var fila fyne.CanvasObject = container.New(&campoYBoton{}, campo, copiar)
	if c.enTelefono() {
		// `max-md:flex-col max-md:items-stretch`: la dirección a todo el ancho y
		// «Copiar» debajo; al lado le quedaban 229 para 394 de enlace.
		fila = container.New(ui.Columna{Hueco: tema.Espacio(2)}, campo, copiar)
	}
	texto := ui.NuevoParrafo(resumen, tema.Escalon{Tam: tema.TextoSM.Tam, Linea: 1.625 * tema.TextoSM.Tam}, ui.Normal, "text-muted", v)
	nota := ui.Texto(c.t("share.copyNow"), tema.TextoXS, ui.Normal, tema.Color("text-subtle", v))
	dentro := container.New(&pilaSeparada{separas: []float32{0, tema.Espacio(2), tema.Espacio(2), tema.Espacio(1.5)}, ancho: &c.anchoPanel},
		titulo, fila, texto, lineaTexto(nota, tema.TextoXS))
	caja := ui.NuevaCaja(tema.Color("success-bg", v), tema.Color("success", v), tema.RadioMD)
	return container.New(&conMargen{m: tema.Espacio(3) + tema.Borde}, append(caja.Objetos(), dentro)...)
}

// lista son las comparticiones vivas, o lo que haya en su lugar.
func (c *dialogoCompartir) lista(v tema.Variante) fyne.CanvasObject {
	borde := ui.NuevaCaja(color.NRGBA{}, tema.Color("border", v), tema.RadioMD)
	switch {
	case c.comparticiones == nil && !c.falloLista:
		r := ui.NuevaRueda(18, "text", v)
		r.Rotulo = c.t("share.loadingLinks")
		return container.New(&altoFijo{alto: tema.Espacio(16)}, append(borde.Objetos(), container.NewCenter(r))...)
	case c.falloLista:
		fondo := ui.NuevaCaja(tema.Color("bg-subtle", v), tema.Color("border", v), tema.RadioMD)
		texto := lineaTexto(ui.Texto(c.t("share.loadFailed"), tema.TextoSM, ui.Normal, tema.Color("text-muted", v)), tema.TextoSM)
		reintentar := ui.NuevoBoton(c.t("files.retry"), func() { c.p.enSegundoPlano(func() { c.recargar() }) }).Con(ui.Secundario, ui.TallaSM)
		return container.New(&conMargenXY{x: tema.Espacio(3), y: tema.Espacio(2)},
			append(fondo.Objetos(), container.NewBorder(nil, nil, texto, reintentar))...)
	case len(c.comparticiones) == 0:
		clave := "share.emptyFile"
		if c.el.Carpeta {
			clave = "share.emptyFolder"
		}
		return lineaTexto(ui.Texto(c.t(clave), tema.TextoSM, ui.Normal, tema.Color("text-muted", v)), tema.TextoSM)
	}
	filas := make([]fyne.CanvasObject, 0, len(c.comparticiones))
	for i, s := range c.comparticiones {
		filas = append(filas, c.filaCompartida(s, i > 0, v))
	}
	return container.New(&conMargen{m: tema.Borde}, append(borde.Objetos(), container.New(ui.Columna{}, filas...))...)
}

// filaCompartida es una compartición viva: `px-3 py-2 gap-3`, el glifo, a quién o qué
// permite, la línea de meta y «Revocar».
func (c *dialogoCompartir) filaCompartida(s api.DriveShare, filete bool, v tema.Variante) fyne.CanvasObject {
	permiso := c.t("share.permView")
	if s.Permission == "write" || s.Permission == "edit" {
		permiso = c.t("share.permEdit")
	}
	var nombre, meta string
	glifo := glifoCon(glifoEnlace, 16, tema.Color("text-muted", v))
	if s.Kind == "link" {
		caduca := c.t("share.noExpiryShort")
		if s.ExpiresAt != nil {
			caduca = FraseCaducidad(c.p.tr, c.p.tr.Locale(), *s.ExpiresAt, time.Now())
		}
		acceso := c.t("share.anyoneWithLink")
		if s.HasPassword {
			acceso = c.t("share.asksPassword")
		}
		nombre, meta = permiso, caduca+" · "+acceso
	} else {
		glifo = iconos.Trazo(iconos.Usuario, 16, "text-muted", v)
		per, ok := c.personas[s.Subject]
		switch {
		case s.Kind == "group":
			nombre = s.Subject
		case ok && per.Nombre != "":
			nombre = per.Nombre
		case ok:
			nombre = per.Email
		default:
			nombre = c.t("share.person.someone")
		}
		meta = permiso
		if ok && per.Nombre != "" && per.Email != "" {
			meta += " · " + per.Email
		}
	}
	revocar := ui.NuevoBoton(c.t("share.revoke"), func() { c.revocar(s) }).Con(ui.Fantasma, ui.TallaSM)
	revocar.Cargando = c.revocando == s.ID
	revocar.Inerte = c.revocando != "" && c.revocando != s.ID
	f := &filaShare{
		glifo:   glifo,
		nombre:  ui.Texto(nombre, tema.TextoBase, ui.Medio, tema.Color("text", v)),
		meta:    ui.Texto(meta, tema.TextoXS, ui.Normal, tema.Color("text-subtle", v)),
		revocar: revocar,
		linea:   canvas.NewRectangle(conAlfaBorde(v)),
	}
	f.linea.Hidden = !filete
	hijos := []fyne.CanvasObject{f.linea, glifo.Objeto(), f.nombre, f.meta, revocar}
	if s.HasPassword {
		f.candado = glifoCon(glifoCandado, 13, tema.Color("text-subtle", v))
		hijos = append(hijos, f.candado.Objeto())
	}
	return container.New(f, hijos...)
}

// filaShare coloca una compartición viva. El filete va ARRIBA (`divide-y`).
type filaShare struct {
	glifo, candado *iconos.Dibujo
	nombre, meta   *canvas.Text
	revocar        *ui.Boton
	linea          *canvas.Rectangle
}

var _ fyne.Layout = (*filaShare)(nil)

func (f *filaShare) alto() float32 {
	return 2*tema.Espacio(2) + max(tema.TextoBase.Linea+tema.TextoXS.Linea, f.revocar.MinSize().Height)
}

func (f *filaShare) MinSize([]fyne.CanvasObject) fyne.Size { return fyne.NewSize(0, f.alto()) }

func (f *filaShare) Layout(_ []fyne.CanvasObject, t fyne.Size) {
	f.linea.Move(fyne.NewPos(0, 0))
	f.linea.Resize(fyne.NewSize(t.Width, tema.Borde))
	x := tema.Espacio(3)
	f.glifo.Colocar(x, (t.Height-16)/2, 16)
	x += 16 + tema.Espacio(3)
	m := f.revocar.MinSize()
	derecha := t.Width - tema.Espacio(3) - m.Width
	f.revocar.Move(fyne.NewPos(derecha, (t.Height-m.Height)/2))
	f.revocar.Resize(m)
	cabe := max(derecha-tema.Espacio(3)-x, 0)
	y := (t.Height - tema.TextoBase.Linea - tema.TextoXS.Linea) / 2
	candado := float32(0)
	if f.candado != nil {
		candado = 13 + tema.Espacio(1.5)
	}
	ancho := ui.Truncar(f.nombre, f.nombre.Text, tema.TextoBase, ui.Medio, max(cabe-candado, 0))
	ui.ColocarTexto(f.nombre, tema.TextoBase, x, y)
	if f.candado != nil {
		f.candado.Colocar(x+ancho+tema.Espacio(1.5), y+(tema.TextoBase.Linea-13)/2, 13)
	}
	ui.Truncar(f.meta, f.meta.Text, tema.TextoXS, ui.Normal, cabe)
	ui.ColocarTexto(f.meta, tema.TextoXS, x, y+tema.TextoBase.Linea)
}

// ---- Piezas de maquetación del formulario ----

// filaForm es la fila `flex flex-wrap items-start gap-2` de los formularios de compartir:
// grupos de campo (el de ancho 0 crece) y, al final, el botón con `mt-6` para quedar a
// la altura del campo y no de la etiqueta.
type filaForm struct {
	campos []fyne.CanvasObject
	anchos []float32
	boton  fyne.CanvasObject
	hueco  float32
	// apilada es el teléfono (por debajo de `md`): cada campo y el botón a todo el
	// ancho, uno debajo de otro (`max-md:w-full max-md:mt-0` de la web). En una fila,
	// el acceso de 176 cortaba «Solo mirar y descargar».
	apilada bool
}

func (f filaForm) objeto() fyne.CanvasObject {
	hijos := append([]fyne.CanvasObject{}, f.campos...)
	if f.boton != nil {
		hijos = append(hijos, f.boton)
	}
	if f.hueco == 0 {
		f.hueco = tema.Espacio(2)
	}
	return container.New(&f, hijos...)
}

// hijos son los campos y el botón, en orden.
func (f *filaForm) hijos() []fyne.CanvasObject {
	if f.boton == nil {
		return f.campos
	}
	return append(append([]fyne.CanvasObject{}, f.campos...), f.boton)
}

func (f *filaForm) MinSize(objs []fyne.CanvasObject) fyne.Size {
	if f.apilada {
		return ui.Columna{Hueco: f.hueco}.MinSize(f.hijos())
	}
	var ancho, alto float32
	for i, c := range f.campos {
		m := c.MinSize()
		w := f.anchos[i]
		if w == 0 {
			w = MinCorreoCompartir
		}
		ancho += w + f.hueco
		alto = max(alto, m.Height)
	}
	if f.boton != nil {
		m := f.boton.MinSize()
		ancho += m.Width
		alto = max(alto, tema.Espacio(6)+m.Height)
	}
	return fyne.NewSize(ancho, alto)
}

// Layout reparte: los fijos con su ancho, el botón con el suyo, y el resto al flexible.
//
// ponytail: sin `flex-wrap`. A 360 la web baja el botón y el acceso a otra línea; aquí
// el campo flexible se estrecha hasta su `min-w-56` y lo que no cabe se recorta.
func (f *filaForm) Layout(_ []fyne.CanvasObject, t fyne.Size) {
	if f.apilada {
		ui.Columna{Hueco: f.hueco}.Layout(f.hijos(), t)
		return
	}
	fijo := float32(0)
	flexibles := 0
	for _, w := range f.anchos {
		if w == 0 {
			flexibles++
		}
		fijo += w
	}
	var mb fyne.Size
	if f.boton != nil {
		mb = f.boton.MinSize()
		fijo += mb.Width
	}
	huecos := float32(len(f.campos)) * f.hueco
	if f.boton == nil {
		huecos -= f.hueco
	}
	libre := float32(0)
	if flexibles > 0 {
		libre = max((t.Width-fijo-huecos)/float32(flexibles), MinCorreoCompartir)
	}
	x := float32(0)
	for i, c := range f.campos {
		w := f.anchos[i]
		if w == 0 {
			w = libre
		}
		c.Move(fyne.NewPos(x, 0))
		c.Resize(fyne.NewSize(w, c.MinSize().Height))
		x += w + f.hueco
	}
	if f.boton != nil {
		f.boton.Move(fyne.NewPos(x, tema.Espacio(6)))
		f.boton.Resize(mb)
	}
}

// pilaSeparada apila bloques a todo el ancho con la separación de cada uno por encima.
//
// Lo que reparte líneas —una ayuda en línea, un párrafo, las dos tarjetas del enlace—
// mide su alto AL ANCHO, y `fyne.Layout.MinSize` no lo recibe: se usa el último con el
// que se colocó, que vive en `ancho` —del diálogo, así que sobrevive a cada repintado—.
type pilaSeparada struct {
	separas []float32
	ancho   *float32
}

func (p *pilaSeparada) altoDe(o fyne.CanvasObject, ancho float32) float32 {
	if ancho > 0 {
		switch x := o.(type) {
		case *ui.AyudaEnLinea:
			return ui.AltoAyudaEnLinea(x, ancho)
		case *ui.Parrafo:
			return ui.AltoParrafo(x, ancho)
		case *fyne.Container:
			if d, es := x.Layout.(*dosColumnas); es {
				return d.alto(x.Objects, ancho)
			}
		}
	}
	return o.MinSize().Height
}

func (p *pilaSeparada) MinSize(objs []fyne.CanvasObject) fyne.Size {
	var ancho float32
	if p.ancho != nil {
		ancho = *p.ancho
	}
	var alto, minimo float32
	for i, o := range objs {
		minimo = max(minimo, o.MinSize().Width)
		alto += p.separas[i] + p.altoDe(o, ancho)
	}
	return fyne.NewSize(minimo, alto)
}

func (p *pilaSeparada) Layout(objs []fyne.CanvasObject, t fyne.Size) {
	// Un ancho de cero es «todavía sin colocar» —el cuerpo de un diálogo que se pinta
	// antes de mostrarse—, no el ancho de verdad: apuntarlo dejaba el siguiente pintado
	// repartiendo el texto a cero, una palabra por línea y el nombre recortado entero.
	if p.ancho != nil && t.Width > 0 {
		*p.ancho = t.Width
	}
	y := float32(0)
	for i, o := range objs {
		y += p.separas[i]
		h := p.altoDe(o, t.Width)
		o.Move(fyne.NewPos(0, y))
		o.Resize(fyne.NewSize(t.Width, h))
		y += h
	}
}

// AnchoCuerpoDialogo es el ancho útil del cuerpo de un diálogo de esa talla en una
// ventana así: el `max-w` de la talla, el `p-4` de alrededor y el `px-6` de dentro. Es la
// primera estimación; en cuanto el cuerpo se coloca, manda el ancho de verdad.
func AnchoCuerpoDialogo(t ui.TallaDialogo, ventana float32) float32 {
	if ventana <= 0 {
		ventana = 1280
	}
	return ui.AnchoCuerpoTalla(t, fyne.NewSize(ventana, 0))
}

// dosColumnas es el `grid gap-2 sm:grid-cols-2` de las dos opciones del enlace: una
// columna por debajo de `sm`.
type dosColumnas struct {
	ventana float32
	ancho   float32
}

func (d *dosColumnas) columnas() int {
	if d.ventana > 0 && d.ventana < ui.CorteSM {
		return 1
	}
	return 2
}

func (d *dosColumnas) alto(objs []fyne.CanvasObject, ancho float32) float32 {
	cols := d.columnas()
	col := (ancho - float32(cols-1)*tema.Espacio(2)) / float32(cols)
	var alto, fila float32
	for i, o := range objs {
		if tj, ok := o.(*ui.TarjetaEleccion); ok {
			fila = max(fila, ui.AltoTarjetaEleccion(tj, col))
		}
		if (i+1)%cols == 0 || i == len(objs)-1 {
			if alto > 0 {
				alto += tema.Espacio(2)
			}
			alto += fila
			fila = 0
		}
	}
	return alto
}

func (d *dosColumnas) MinSize(objs []fyne.CanvasObject) fyne.Size {
	ancho := d.ancho
	if ancho <= 0 {
		ancho = 40 * tema.Rem
	}
	return fyne.NewSize(0, d.alto(objs, ancho))
}

func (d *dosColumnas) Layout(objs []fyne.CanvasObject, t fyne.Size) {
	d.ancho = t.Width
	cols := d.columnas()
	col := (t.Width - float32(cols-1)*tema.Espacio(2)) / float32(cols)
	alto := d.alto(objs, t.Width)
	filas := (len(objs) + cols - 1) / cols
	h := (alto - float32(filas-1)*tema.Espacio(2)) / float32(filas)
	for i, o := range objs {
		o.Move(fyne.NewPos(float32(i%cols)*(col+tema.Espacio(2)), float32(i/cols)*(h+tema.Espacio(2))))
		o.Resize(fyne.NewSize(col, h))
	}
}

// lineaTexto envuelve un texto en su caja de LÍNEA, que es lo que ocupa en la web.
func lineaTexto(t *canvas.Text, e tema.Escalon) fyne.CanvasObject {
	return container.New(&enLinea{t: t, e: e}, t)
}

type enLinea struct {
	t *canvas.Text
	e tema.Escalon
}

func (l *enLinea) MinSize([]fyne.CanvasObject) fyne.Size {
	return fyne.NewSize(ui.Medir(l.t.Text, l.e, ui.Normal).Width, l.e.Linea)
}

func (l *enLinea) Layout(_ []fyne.CanvasObject, t fyne.Size) { ui.ColocarTexto(l.t, l.e, 0, 0) }

// campoYBoton es `flex items-center gap-2` con el campo creciendo y el botón a su medida.
type campoYBoton struct{}

func (campoYBoton) MinSize(objs []fyne.CanvasObject) fyne.Size {
	a, b := objs[0].MinSize(), objs[1].MinSize()
	return fyne.NewSize(a.Width+tema.Espacio(2)+b.Width, max(a.Height, b.Height))
}

func (campoYBoton) Layout(objs []fyne.CanvasObject, t fyne.Size) {
	b := objs[1].MinSize()
	a := max(t.Width-b.Width-tema.Espacio(2), 0)
	objs[0].Move(fyne.NewPos(0, (t.Height-objs[0].MinSize().Height)/2))
	objs[0].Resize(fyne.NewSize(a, objs[0].MinSize().Height))
	objs[1].Move(fyne.NewPos(a+tema.Espacio(2), (t.Height-b.Height)/2))
	objs[1].Resize(b)
}

// conMargen pone los primeros objetos (la caja) a tamaño completo y el último con un
// margen igual por los cuatro lados.
type conMargen struct{ m float32 }

func (c *conMargen) MinSize(objs []fyne.CanvasObject) fyne.Size {
	m := objs[len(objs)-1].MinSize()
	return fyne.NewSize(m.Width+2*c.m, m.Height+2*c.m)
}

func (c *conMargen) Layout(objs []fyne.CanvasObject, t fyne.Size) {
	for i, o := range objs {
		if i == len(objs)-1 {
			o.Move(fyne.NewPos(c.m, c.m))
			o.Resize(fyne.NewSize(max(t.Width-2*c.m, 0), max(t.Height-2*c.m, 0)))
			continue
		}
		o.Move(fyne.NewPos(0, 0))
		o.Resize(t)
	}
}

// conMargenXY es conMargen con el `px`/`py` distintos.
type conMargenXY struct{ x, y float32 }

func (c *conMargenXY) MinSize(objs []fyne.CanvasObject) fyne.Size {
	m := objs[len(objs)-1].MinSize()
	return fyne.NewSize(m.Width+2*c.x, m.Height+2*c.y)
}

func (c *conMargenXY) Layout(objs []fyne.CanvasObject, t fyne.Size) {
	for i, o := range objs {
		if i == len(objs)-1 {
			o.Move(fyne.NewPos(c.x, c.y))
			o.Resize(fyne.NewSize(max(t.Width-2*c.x, 0), max(t.Height-2*c.y, 0)))
			continue
		}
		o.Move(fyne.NewPos(0, 0))
		o.Resize(t)
	}
}

// altoFijo es un contenedor de un alto dado (`h-16`) con todo a tamaño completo.
type altoFijo struct{ alto float32 }

func (a *altoFijo) MinSize([]fyne.CanvasObject) fyne.Size { return fyne.NewSize(0, a.alto) }

func (a *altoFijo) Layout(objs []fyne.CanvasObject, t fyne.Size) {
	for _, o := range objs {
		o.Move(fyne.NewPos(0, 0))
		o.Resize(t)
	}
}

// versalEnLinea es el `sytue-overline` de un encabezado, con su caja de línea de `text-xs`.
func versalEnLinea(v *ui.Versal) fyne.CanvasObject {
	return container.New(&enVersal{v: v}, v.Objetos()...)
}

type enVersal struct{ v *ui.Versal }

func (e *enVersal) MinSize([]fyne.CanvasObject) fyne.Size {
	return fyne.NewSize(e.v.Ancho(), tema.TextoXS.Linea)
}

func (e *enVersal) Layout([]fyne.CanvasObject, fyne.Size) { e.v.Colocar(0, 0) }
