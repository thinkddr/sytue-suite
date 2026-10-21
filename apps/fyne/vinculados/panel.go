// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

// Package vinculados muestra datos vivos de las cuentas externas de Mail en sus
// aplicaciones correspondientes. No copia registros a Sytue ni guarda credenciales.
package vinculados

import (
	"cmp"
	"context"
	"slices"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"

	"git.sytue.eu/sytue/one/apps/fyne/catalogos"
	"git.sytue.eu/sytue/one/apps/fyne/sesion"
	"git.sytue.eu/sytue/one/apps/fyne/tema"
	"git.sytue.eu/sytue/one/apps/fyne/ui"
	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/i18n"
)

// Panel es una vista por cuenta y colección; la identidad, los IDs opacos y los
// ETags se conservan hasta la escritura. Todo su estado se toca en el hilo UI.
type Panel struct {
	tr, dominio                                            *i18n.Translator
	svc                                                    *api.Integraciones
	servicio                                               string
	identidad                                              func() string
	autorizar                                              func(context.Context, string, string, ...string) (string, error)
	enFondo, enUI                                          func(func())
	lienzo                                                 fyne.Canvas
	dialogo                                                *ui.Dialogo
	ctx                                                    context.Context
	cancelar                                               context.CancelFunc
	actor                                                  string
	secuencia                                              uint64
	ocupado, escrituraIncierta                             bool
	reconectarNecesario                                    bool
	fallo                                                  string
	paso                                                   string
	cuentas                                                api.ExternalMailAccounts
	pendienteProvider, pendienteID, esperadaID             string
	consentProvider, consentInstance, volverConsentimiento string
	consentServices                                        []string
	cuenta                                                 api.ExternalMailAccount
	colecciones                                            []api.RemoteCollection
	coleccion                                              api.RemoteCollection
	eventos                                                []api.RemoteEvent
	contactos                                              []api.RemoteContact
	notas                                                  []api.RemoteNote
	cursor                                                 string
	desde, hasta                                           time.Time
	evento                                                 api.RemoteEvent
	contacto                                               api.RemoteContact
	nota                                                   api.RemoteNote
	formulario                                             fyne.CanvasObject
	guardarBoton                                           *ui.Boton
	campos                                                 map[string]*ui.Entrada
	inicio, fin                                            *ui.CampoFechaHora
	diaCompleto                                            *ui.Eleccion
	nuevoRegistro                                          bool
}

// Nuevo utiliza la misma cuenta y el mismo consentimiento sellado que Mail, con
// retorno a ESTA aplicación. El inventario nunca se consulta antes de abrirlo.
func Nuevo(s *sesion.Suite, w fyne.Window, dominio *i18n.Translator, servicio string) *Panel {
	if servicio != "calendar" && servicio != "contacts" && servicio != "notes" {
		panic("unsupported linked service")
	}
	tr, err := i18n.New(s.Idioma(), i18n.Common(), catalogos.Mail())
	if err != nil {
		panic(err)
	}
	svc := api.NewIntegraciones(s.URL("integrations"), s.Sesion, nil)
	p := &Panel{tr: tr, dominio: dominio, svc: svc, servicio: servicio, lienzo: w.Canvas(), enFondo: func(f func()) { go f() }, enUI: fyne.Do}
	p.identidad = func() string {
		e := s.Estado()
		if !e.Dentro() {
			return ""
		}
		return e.Sujeto + "\x00" + e.GeneracionSesion + "\x00" + s.Sesion.Organizacion()
	}
	p.autorizar = func(ctx context.Context, provider, instance string, services ...string) (string, error) {
		return s.AutorizarCorreoDesde(ctx, svc, provider, instance, "native-"+servicio, services...)
	}
	s.AlCambiar(func(e sesion.Estado) {
		if p.dialogo != nil && (!e.Dentro() || p.actor != p.identidad()) {
			p.Cerrar()
		}
	})
	return p
}

// Abrir ofrece cuentas existentes y una conexión nueva incluso si no hay ninguna.
func (p *Panel) Abrir() {
	if p.identidad() == "" {
		return
	}
	p.Cerrar()
	p.actor = p.identidad()
	p.ctx, p.cancelar = context.WithCancel(context.Background())
	p.paso = "cuentas"
	ahora := time.Now()
	p.desde = time.Date(ahora.Year(), ahora.Month(), 1, 0, 0, 0, 0, ahora.Location())
	p.hasta = p.desde.AddDate(0, 1, 0)
	p.dialogo = ui.NuevoDialogo(p.t("linkedServices.connected"), nil).Con(ui.DialogoLG, nil)
	p.dialogo.AlCerrar = p.Cerrar
	p.dialogo.Mostrar(p.lienzo)
	p.cargarCuentas("")
}

// Cerrar invalida también respuestas pendientes y elimina todo contenido remoto.
func (p *Panel) Cerrar() {
	p.secuencia++
	if p.cancelar != nil {
		p.cancelar()
	}
	if p.dialogo != nil {
		p.dialogo.Cerrar()
	}
	p.dialogo, p.cancelar, p.ctx = nil, nil, nil
	p.cuentas = api.ExternalMailAccounts{}
	p.pendienteProvider, p.pendienteID, p.esperadaID = "", "", ""
	p.consentProvider, p.consentInstance, p.volverConsentimiento = "", "", ""
	p.consentServices = nil
	p.cuenta, p.coleccion = api.ExternalMailAccount{}, api.RemoteCollection{}
	p.colecciones, p.eventos, p.contactos, p.notas = nil, nil, nil, nil
	p.evento, p.contacto, p.nota = api.RemoteEvent{}, api.RemoteContact{}, api.RemoteNote{}
	p.formulario, p.campos, p.guardarBoton = nil, nil, nil
	p.inicio, p.fin = nil, nil
	p.diaCompleto, p.nuevoRegistro = nil, false
	p.actor, p.paso, p.cursor = "", "", ""
	p.ocupado, p.escrituraIncierta, p.fallo = false, false, ""
	p.reconectarNecesario = false
}

func (p *Panel) t(k string) string  { return p.tr.T(k, nil) }
func (p *Panel) dt(k string) string { return p.dominio.T(k, nil) }
func (p *Panel) texto(s string) fyne.CanvasObject {
	return ui.NuevoParrafo(s, tema.TextoSM, ui.Normal, "text", tema.De(fyne.CurrentApp().Settings().ThemeVariant()))
}

func (p *Panel) boton(s string, f func()) *ui.Boton {
	b := ui.NuevoBoton(s, f).Con(ui.Secundario, ui.TallaMD)
	b.Inerte = p.ocupado
	return b
}

func (p *Panel) eleccion(titulo, descripcion string, f func()) *ui.TarjetaEleccion {
	c := ui.NuevaTarjetaEleccion(titulo, descripcion, f)
	c.Inerte, c.Densa, c.Ancho = p.ocupado, true, max(180, min(584, p.lienzo.Size().Width-48))
	return c
}

func (p *Panel) columna(o ...fyne.CanvasObject) *fyne.Container {
	return container.New(ui.Columna{Hueco: tema.Espacio(3)}, o...)
}

// trabajar tiene una sola operación en vuelo. Nunca publica una respuesta que
// pertenezca a una ventana cerrada, otra cuenta de Sytue u otra selección.
func (p *Panel) trabajar(timeout time.Duration, tarea func(context.Context) func()) {
	if p.ocupado || p.dialogo == nil || p.identidad() != p.actor {
		return
	}
	p.ocupado, p.fallo = true, ""
	p.secuencia++
	seq, actor, parent := p.secuencia, p.actor, p.ctx
	p.pintar()
	p.enFondo(func() {
		ctx, cancel := context.WithTimeout(parent, timeout)
		defer cancel()
		aplicar := tarea(ctx)
		p.enUI(func() {
			if p.dialogo == nil || p.secuencia != seq || p.actor != actor || p.identidad() != actor {
				return
			}
			p.ocupado = false
			aplicar()
			p.pintar()
		})
	})
}

func (p *Panel) cargarCuentas(elegida string) {
	p.esperadaID = elegida
	p.trabajar(30*time.Second, func(ctx context.Context) func() {
		cuentas, err := p.svc.MailAccounts(ctx)
		return func() {
			if err != nil {
				p.fallar(err, false)
				return
			}
			p.cuentas = cuentas
			if elegida != "" {
				for _, c := range cuentas.Items {
					if c.InstanceID == elegida && c.Connected && slices.Contains(c.Services, p.servicio) {
						p.esperadaID = ""
						p.elegirCuenta(c)
						return
					}
				}
				p.fallo = p.t("linkedServices.notGranted")
			}
		}
	})
}

func capacidad(provider api.ExternalMailProvider, servicio string) (bool, string) {
	for _, c := range provider.Services {
		if c.Service == servicio {
			return provider.Available && c.Available, c.Reason
		}
	}
	return false, ""
}

func (p *Panel) conectar(provider, instance string) {
	if instance == "" && p.pendienteProvider == provider {
		instance = p.pendienteID
	}
	services := p.serviciosConsentir(provider, instance)
	p.trabajar(10*time.Minute, func(ctx context.Context) func() {
		id, err := p.autorizar(ctx, provider, instance, services...)
		return func() {
			if err != nil {
				if id != "" {
					p.pendienteProvider, p.pendienteID = provider, id
				}
				p.fallo = p.t("externalMail.authError")
				return
			}
			if id == "" {
				p.fallo = p.t("externalMail.authError")
				return
			}
			p.paso = "cuentas"
			p.pendienteProvider, p.pendienteID = "", ""
			p.cargarCuentas(id)
		}
	})
}

func (p *Panel) elegirCuenta(c api.ExternalMailAccount) {
	p.reconectarNecesario, p.escrituraIncierta = false, false
	p.esperadaID = ""
	p.cuenta, p.coleccion = c, api.RemoteCollection{}
	p.eventos, p.contactos, p.notas, p.cursor = nil, nil, nil, ""
	p.colecciones = nil
	p.formulario, p.campos = nil, nil
	p.paso = "colecciones"
	p.trabajar(30*time.Second, func(ctx context.Context) func() {
		colecciones, err := p.svc.LinkedCollections(ctx, c, p.servicio)
		return func() {
			if err != nil {
				p.fallar(err, false)
				return
			}
			p.colecciones = colecciones
		}
	})
}

func (p *Panel) cargarItems(mas bool) {
	account, collection, cursor, desde, hasta := p.cuenta, p.coleccion.ID, "", p.desde, p.hasta
	if mas {
		cursor = p.cursor
	}
	p.trabajar(30*time.Second, func(ctx context.Context) func() {
		var eventos api.RemotePage[api.RemoteEvent]
		var contactos api.RemotePage[api.RemoteContact]
		var notas api.RemotePage[api.RemoteNote]
		var err error
		switch p.servicio {
		case "calendar":
			eventos, err = p.svc.LinkedEvents(ctx, account, collection, desde, hasta, cursor)
		case "contacts":
			contactos, err = p.svc.LinkedContacts(ctx, account, collection, cursor)
		case "notes":
			notas, err = p.svc.LinkedNotes(ctx, account, collection, cursor)
		}
		return func() {
			if err != nil {
				p.fallar(err, false)
				return
			}
			if !mas {
				p.eventos, p.contactos, p.notas = nil, nil, nil
			}
			p.eventos = unicos(p.eventos, eventos.Items, func(e api.RemoteEvent) string { return e.ID })
			p.contactos = unicos(p.contactos, contactos.Items, func(c api.RemoteContact) string { return c.ID })
			p.notas = unicos(p.notas, notas.Items, func(n api.RemoteNote) string { return n.ID })
			p.cursor = cmp.Or(eventos.NextCursor, contactos.NextCursor, notas.NextCursor)
			if p.cursor == cursor {
				p.cursor = ""
			}
			p.paso = "items"
		}
	})
}

func (p *Panel) pintar() {
	if p.dialogo == nil {
		return
	}
	var cuerpo []fyne.CanvasObject
	if p.fallo != "" {
		cuerpo = append(cuerpo, p.texto(p.fallo))
	}
	if p.ocupado {
		cuerpo = append(cuerpo, p.texto(p.t("ui.loading")))
	}
	if p.cuenta.InstanceID != "" {
		cuerpo = append(cuerpo, p.texto(api.MailProviderNames[p.cuenta.Provider]+" · "+p.cuenta.Address))
	}
	cuerpo = append(cuerpo, p.texto(p.t("linkedServices.providerData")))
	if p.reconectarNecesario && p.cuenta.InstanceID != "" && p.paso != "consentimiento" {
		cuerpo = append(cuerpo, p.boton(p.t("externalMail.connect"), func() { p.prepararConsentimiento(p.cuenta.Provider, p.cuenta.InstanceID) }))
	}
	switch p.paso {
	case "cuentas":
		cuerpo = append(cuerpo, p.cuerpoCuentas()...)
	case "proveedores":
		cuerpo = append(cuerpo, p.cuerpoProveedores()...)
	case "colecciones":
		cuerpo = append(cuerpo, p.cuerpoColecciones()...)
	case "items":
		cuerpo = append(cuerpo, p.cuerpoItems()...)
	case "detalle":
		cuerpo = append(cuerpo, p.cuerpoDetalle()...)
	case "consentimiento":
		cuerpo = append(cuerpo, p.cuerpoConsentimiento()...)
	}
	var pie []fyne.CanvasObject
	if p.paso != "cuentas" {
		pie = append(pie, p.boton(p.t("ui.wizard.back"), p.volver))
	}
	pie = append(pie, ui.NuevoBoton(p.t("ui.close"), p.Cerrar).Con(ui.Fantasma, ui.TallaMD))
	p.dialogo.CambiarCuerpo(p.columna(cuerpo...))
	p.dialogo.CambiarPie(container.New(ui.PieAcciones{Hueco: tema.Espacio(2)}, pie...))
}

func (p *Panel) volver() {
	switch p.paso {
	case "proveedores":
		p.paso = "cuentas"
	case "colecciones":
		p.cuenta = api.ExternalMailAccount{}
		p.paso = "cuentas"
	case "items":
		p.paso = "colecciones"
	case "detalle":
		p.formulario = nil
		p.paso = "items"
	case "consentimiento":
		p.paso = p.volverConsentimiento
	}
	p.pintar()
}

func unicos[T any](previos, nuevos []T, id func(T) string) []T {
	vistos := make(map[string]bool, len(previos)+len(nuevos))
	for _, item := range previos {
		vistos[id(item)] = true
	}
	for _, item := range nuevos {
		key := id(item)
		if key != "" && !vistos[key] {
			previos = append(previos, item)
			vistos[key] = true
		}
	}
	return previos
}
