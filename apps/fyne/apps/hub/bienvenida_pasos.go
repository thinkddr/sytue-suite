// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package hub

import (
	"context"
	"image/color"
	"math"
	"slices"
	"strconv"
	"strings"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"

	"git.sytue.eu/sytue/one/apps/fyne/iconos"
	"git.sytue.eu/sytue/one/apps/fyne/tema"
	"git.sytue.eu/sytue/one/apps/fyne/ui"
	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/i18n"
)

// Los seis pasos del alta, cada uno con lo que pinta la web. Las medidas son las de sus
// clases: `max-w-2xl gap-3` en las opciones, `max-w-xl gap-5` en los formularios,
// `gap-4` dentro de los paneles y `p-4 rounded-lg border bg-surface-raised` en ellos.
const (
	AnchoFormAlta    float32 = 36 * tema.Rem // `max-w-xl`
	HuecoFormAlta    float32 = 5 * 0.25 * tema.Rem
	HuecoPanelAlta   float32 = 4 * 0.25 * tema.Rem
	RellenoPanelAlta float32 = 4 * 0.25 * tema.Rem
	HuecoFilaAlta    float32 = 3 * 0.25 * tema.Rem
	HuecoCasillas    float32 = 2 * 0.25 * tema.Rem
)

// controlesAlta son los controles que sobreviven a los cambios de estado.
type controlesAlta struct {
	pasos                *ui.Pasos
	atras, siguiente     *ui.Boton
	reintentar           *ui.Boton
	modoPersona, modoOrg *ui.TarjetaEleccion

	nombre, slug   *ui.Entrada
	opcDominio     []*ui.TarjetaEleccion
	host           *ui.Entrada
	opcDNS         []*ui.TarjetaEleccion
	token          *ui.Entrada
	correo, record *ui.Eleccion
	cfMirar, cfApl *ui.Boton
	comprobar      *ui.Boton
	manual         *Detalles
	buscar         *ui.Entrada
	buscarBtn      *ui.Boton
	usarBtn        *ui.Boton

	correos *ui.AreaTexto
	rol     *ui.Selector
	enviar  *ui.Boton

	legal, nif, email, asientos *ui.Entrada
	direccion                   *ui.AreaTexto

	casillasRol map[string]*ui.Eleccion
	sustituir   map[string]*ui.Eleccion
	permisos    map[string]*ui.Enlace
	editar      map[string]*ui.Enlace
	siguientes  []*enlaceSiguiente
}

// valoresRol son los del selector de la invitación; su nombre sale del catálogo.
var valoresRol = []string{"member", "admin", "billing"}

// montar crea los controles UNA vez, con lo que cada uno cambia en el estado.
func (r *bienvenidaRenderer) montar() {
	b, t := r.b, r.b.T
	c := &r.c
	tt := func(k string) string { return t.T(k, nil) }
	cambia := func(f func()) {
		if b.sincronizando.Load() {
			return
		}
		b.mu.Lock()
		f()
		b.mu.Unlock()
		b.Refresh()
	}

	c.pasos = ui.NuevosPasos()
	c.pasos.RotuloIr = func(n int) string { return t.T("ui.wizard.goToStep", i18n.Vars{"n": n}) }
	c.atras = ui.NuevoBoton(tt("ui.wizard.back"), func() { b.volverPaso() }).Con(ui.Secundario, ui.TallaMD)
	c.atras.Icono, c.atras.TamIcono = iconos.ChevronIzquierda, 16
	c.siguiente = ui.NuevoBoton("", b.avanzar).Con(ui.Primario, ui.TallaMD)
	c.reintentar = ui.NuevoBoton(tt("ob.retry"), func() { b.lanzar(func() { b.Cargar(context.Background(), false) }) }).Con(ui.Primario, ui.TallaMD)

	c.modoPersona = ui.NuevaTarjetaEleccion(tt("ob.mode.individual.title"), tt("ob.mode.individual.body"),
		func() { cambia(func() { b.modo = api.AltaIndividual }) })
	c.modoPersona.Icono = iconos.Usuario
	c.modoOrg = ui.NuevaTarjetaEleccion(tt("ob.mode.org.title"), tt("ob.mode.org.body"),
		func() { cambia(func() { b.modo = api.AltaOrganizacion }) })
	c.modoOrg.Icono, c.modoOrg.Insignia = iconos.Edificio, tt("ob.mode.org.badge")

	c.nombre = ui.NuevaEntrada("")
	c.nombre.AlCambiar(func(s string) {
		if b.sincronizando.Load() {
			return
		}
		b.mu.Lock()
		b.nombre = s
		propone := !b.slugEditado && b.org() == nil
		b.mu.Unlock()
		if propone {
			c.slug.PonerTexto(Slugify(s))
			return
		}
		b.Refresh()
	})
	c.slug = ui.NuevaEntrada(tt("ob.identity.slugPlaceholder"))
	c.slug.AlCambiar(func(s string) {
		if b.sincronizando.Load() {
			return
		}
		b.mu.Lock()
		if low := strings.ToLower(s); low != s {
			b.mu.Unlock()
			c.slug.PonerTexto(low)
			return
		}
		if b.nombre == "" || Slugify(b.nombre) != s {
			b.slugEditado = true
		}
		b.slug = s
		b.mu.Unlock()
		b.comprobarSlug()
		b.Refresh()
	})
	for _, o := range []struct {
		valor string
		icono iconos.Icono
		desc  string
	}{{"subdomain", iconos.Cohete, "ob.domain.keep.body"}, {"own", iconos.Globo, "ob.domain.own.body"}, {"search", iconos.Buscar, "ob.domain.search.body"}} {
		op := ui.NuevaTarjetaEleccion("", tt(o.desc), func() { cambia(func() { b.eleccion = o.valor }) })
		op.Icono = o.icono
		c.opcDominio = append(c.opcDominio, op)
	}
	c.opcDominio[1].Titulo, c.opcDominio[2].Titulo = tt("ob.domain.own.title"), tt("ob.domain.search.title")

	c.host = ui.NuevaEntrada(tt("ob.domain.hostPlaceholder"))
	c.host.AlCambiar(func(s string) { cambia(func() { b.dominio = s }) })
	cf := ui.NuevaTarjetaEleccion(tt("ob.dns.cf.title"), tt("ob.dns.cf.body"), func() { cambia(func() { b.dnsHost = "cloudflare" }) })
	cf.Icono, cf.Insignia, cf.TonoInsignia = iconos.Nube, tt("ob.dns.cf.badge"), ui.InsigniaExito
	manual := ui.NuevaTarjetaEleccion(tt("ob.dns.manual.title"), tt("ob.dns.manual.body"), func() { cambia(func() { b.dnsHost = "manual" }) })
	manual.Icono = iconos.Globo
	c.opcDNS = []*ui.TarjetaEleccion{cf, manual}
	c.token = ui.NuevaEntrada(tt("ob.cf.tokenPlaceholder"))
	c.token.Contrasena(true)
	c.token.AlCambiar(func(s string) { cambia(func() { b.cfToken = s }) })
	c.correo = ui.NuevaCasilla("", func(si bool) { cambia(func() { b.cfCorreo = si }) })
	c.record = ui.NuevaCasilla(tt("ob.cf.remember"), func(si bool) { cambia(func() { b.cfRecordar = si }) })
	c.cfMirar = ui.NuevoBoton("", func() { b.hacer(b.cloudflare(false)) }).Con(ui.Primario, ui.TallaMD)
	c.cfApl = ui.NuevoBoton("", func() { b.hacer(b.cloudflare(true)) }).Con(ui.Primario, ui.TallaMD)
	c.comprobar = ui.NuevoBoton("", func() { b.hacer(b.comprobarDominio) }).Con(ui.Primario, ui.TallaMD)
	c.buscar = ui.NuevaEntrada(tt("ob.search.placeholder"))
	c.buscar.AlCambiar(func(s string) { cambia(func() { b.buscado = s }) })
	c.buscarBtn = ui.NuevoBoton(tt("ob.search.check"), func() { b.hacer(b.buscarDominio) }).Con(ui.Primario, ui.TallaMD)
	c.usarBtn = ui.NuevoBoton(tt("ob.search.useOwn"), func() {
		cambia(func() {
			b.eleccion = "own"
			if b.disponible != nil {
				b.dominio = b.disponible.Domain
			} else {
				b.dominio = strings.TrimSpace(b.buscado)
			}
		})
		c.host.PonerTexto(b.dominio)
	}).Con(ui.Secundario, ui.TallaMD)

	c.correos = ui.NuevaAreaTexto(tt("ob.people.emailsPlaceholder"))
	c.correos.Filas = 3
	c.correos.AlCambiar(func(s string) { cambia(func() { b.correos = s }) })
	opciones := make([]string, len(valoresRol))
	for i, v := range valoresRol {
		opciones[i] = tt("ob.roleOption." + v)
	}
	c.rol = ui.NuevoSelector(opciones, func(i int) { cambia(func() { b.rolInvitar = valoresRol[i] }) })
	c.enviar = ui.NuevoBoton(tt("ob.people.send"), func() { b.hacer(b.invitar) }).Con(ui.Primario, ui.TallaMD)

	campo := func(marca string, pon func(string)) *ui.Entrada {
		e := ui.NuevaEntrada(marca)
		e.AlCambiar(func(s string) { cambia(func() { pon(s) }) })
		return e
	}
	c.legal = campo(tt("ob.billing.legalPlaceholder"), func(s string) { b.facturacion.LegalName = s })
	c.nif = campo(tt("ob.billing.taxPlaceholder"), func(s string) { b.facturacion.TaxID = s })
	c.email = campo(tt("ob.billing.emailPlaceholder"), func(s string) { b.facturacion.BillingEmail = s })
	c.asientos = campo("", func(s string) { b.asientos = cantidad(s, b.asientos) })
	c.direccion = ui.NuevaAreaTexto(tt("ob.billing.addressPlaceholder"))
	c.direccion.Filas = 2
	c.direccion.AlCambiar(func(s string) { cambia(func() { b.facturacion.Address = s }) })

	c.casillasRol, c.sustituir = map[string]*ui.Eleccion{}, map[string]*ui.Eleccion{}
	c.permisos, c.editar = map[string]*ui.Enlace{}, map[string]*ui.Enlace{}
	ir := func(d Destino) func() {
		return func() {
			if b.Ir != nil {
				b.Ir(d)
			}
		}
	}
	web := func(ruta string) func() {
		return func() {
			if b.AbrirURL != nil {
				// Sin sesión todavía (el alta va antes de entrar) no hay org que poner: el enlace
				// sale sin ella y el middleware de Admin la añade al entrar (ADR 0056).
				b.AbrirURL(api.AppURL(b.Apex, "console", "", ruta))
			}
		}
	}
	c.siguientes = []*enlaceSiguiente{
		nuevoEnlaceSiguiente(iconos.Cohete, tt("ob.next.devices.title"), tt("ob.next.devices.body"), ir(ADispositivos)),
		nuevoEnlaceSiguiente(iconos.Nube, tt("ob.next.migrations.title"), tt("ob.next.migrations.body"), web("migrations")),
		nuevoEnlaceSiguiente(iconos.Rejilla, tt("ob.next.apps.title"), tt("ob.next.apps.body"), web("apps")),
		nuevoEnlaceSiguiente(iconos.Usuarios, tt("ob.next.users.title"), tt("ob.next.users.body"), web("users")),
	}
}

// sincronizar lleva a los campos lo que el estado dice cuando no lo ha escrito la
// persona (la carga inicial, lo que devuelve el servidor normalizado). Solo toca un
// campo si su texto es otro: reescribirlo movería el cursor de quien escribe. Va SIN el
// cerrojo: escribir en un campo llama a su `AlCambiar`, que lo toma.
func (r *bienvenidaRenderer) sincronizar() {
	b, c := r.b, &r.c
	type par struct {
		e *ui.Entrada
		v string
	}
	b.mu.Lock()
	pares := []par{
		{c.nombre, b.nombre},
		{c.slug, b.slug},
		{c.host, b.dominio},
		{c.token, b.cfToken},
		{c.buscar, b.buscado},
		{c.legal, b.facturacion.LegalName},
		{c.nif, b.facturacion.TaxID},
		{c.email, b.facturacion.BillingEmail},
		{&c.correos.Entrada, b.correos},
		{&c.direccion.Entrada, b.facturacion.Address},
	}
	asientos := b.asientos
	b.mu.Unlock()
	b.sincronizando.Store(true)
	defer b.sincronizando.Store(false)
	for _, x := range pares {
		if x.e.Texto() != x.v {
			x.e.PonerTexto(x.v)
		}
	}
	if cantidad(c.asientos.Texto(), 0) != asientos {
		c.asientos.PonerTexto(strconv.Itoa(asientos))
	}
}

// tituloPaso es la etiqueta corta de un paso en el progreso.
func (r *bienvenidaRenderer) tituloPaso(id string) string {
	t := r.b.T
	switch id {
	case api.AltaPasoModo:
		return t.T("ob.mode.title", nil)
	case api.AltaPasoIdentidad:
		if r.b.esOrg() {
			return t.T("ob.identity.titleOrg", nil)
		}
		return t.T("ob.identity.titleMe", nil)
	case api.AltaPasoPersonas:
		return t.T("ob.people.title", nil)
	case api.AltaPasoFacturacion:
		return t.T("ob.billing.title", nil)
	case api.AltaPasoRoles:
		return t.T("ob.roles.title", nil)
	}
	return t.T("ob.done.title", nil)
}

// pasoAlta es lo que el armazón necesita de cada paso: `WizardStep`.
type pasoAlta struct {
	titulo, encabezado, descripcion, siguiente string
	puede                                      bool
	cuerpo                                     bloque
}

// paso construye uno. Se llama con el cerrojo tomado.
func (r *bienvenidaRenderer) paso(id string, v tema.Variante) pasoAlta {
	b, t := r.b, r.b.T
	tt := func(k string) string { return t.T(k, nil) }
	switch id {
	case api.AltaPasoModo:
		return pasoAlta{
			titulo: tt("ob.mode.title"), encabezado: tt("ob.mode.heading"), descripcion: tt("ob.mode.description"),
			puede: b.modo != "", cuerpo: r.cuerpoModo(),
		}
	case api.AltaPasoIdentidad:
		return r.pasoIdentidad(v)
	case api.AltaPasoPersonas:
		return pasoAlta{
			titulo: tt("ob.people.title"), encabezado: tt("ob.people.heading"), descripcion: tt("ob.people.description"),
			siguiente: tt("ob.people.next"), puede: true, cuerpo: r.cuerpoPersonas(v),
		}
	case api.AltaPasoFacturacion:
		return r.pasoFacturacion()
	case api.AltaPasoRoles:
		return pasoAlta{
			titulo: tt("ob.roles.title"), encabezado: tt("ob.roles.heading"), descripcion: tt("ob.roles.description"),
			siguiente: tt("ob.roles.next"), puede: true, cuerpo: r.cuerpoRoles(v),
		}
	}
	encabezado := tt("ob.done.heading")
	if b.estado != nil && b.estado.Completed {
		encabezado = tt("ob.done.headingCompleted")
	}
	return pasoAlta{
		titulo: tt("ob.done.title"), encabezado: encabezado, descripcion: tt("ob.done.description"),
		siguiente: tt("ob.done.next"), puede: true, cuerpo: r.cuerpoListo(v),
	}
}

// ---- Paso 1: ¿para ti o para tu organización? ----

func (r *bienvenidaRenderer) cuerpoModo() bloque {
	b, c := r.b, &r.c
	c.modoPersona.Elegida, c.modoOrg.Elegida = b.modo == api.AltaIndividual, b.modo == api.AltaOrganizacion
	c.modoPersona.Refresh()
	c.modoOrg.Refresh()
	return tope{AnchoIntro, opcionesB{HuecoFilaAlta, []*ui.TarjetaEleccion{c.modoPersona, c.modoOrg}}}
}

// ---- Paso 2: identidad y dirección ----

//nolint:gocyclo // La pantalla refleja los estados mutuamente excluyentes del alta.
func (r *bienvenidaRenderer) pasoIdentidad(v tema.Variante) pasoAlta {
	b, c, t := r.b, &r.c, r.b.T
	tt := func(k string) string { return t.T(k, nil) }
	org, esOrg := b.org(), b.esOrg()
	suite := b.estado.SuiteDomain
	slugValor := strings.ToLower(strings.TrimSpace(b.slug))
	listo := strings.TrimSpace(b.nombre) != "" && (org != nil || (b.comprobado != nil && b.comprobado.Available))

	p := pasoAlta{
		titulo: tt("ob.identity.titleMe"), encabezado: tt("ob.identity.headingMe"),
		descripcion: tt("ob.identity.description"), siguiente: tt("ob.identity.next"), puede: listo,
	}
	pistaNombre, marcaNombre := tt("ob.identity.nameHintMe"), tt("ob.identity.namePlaceholderMe")
	if esOrg {
		p.titulo, p.encabezado = tt("ob.identity.titleOrg"), tt("ob.identity.headingOrg")
		pistaNombre, marcaNombre = tt("ob.identity.nameHintOrg"), tt("ob.identity.namePlaceholderOrg")
	}
	switch {
	case org != nil:
		p.descripcion, p.siguiente = tt("ob.identity.descriptionCreated"), tt("ob.identity.nextCreated")
	case b.eleccion == "own":
		p.siguiente = tt("ob.identity.nextOwnDomain")
	}
	c.nombre.PonerMarcaDeAgua(marcaNombre)
	c.slug.Inerte(org != nil)

	pista, tono := tt("ob.identity.slugChecking"), ui.AyudaNeutra
	switch {
	case org != nil:
		pista, tono = t.T("ob.identity.slugCreated", i18n.Vars{"subdomain": org.Subdomain}), ui.AyudaExito
	case slugValor == "":
		pista = t.T("ob.identity.slugEmpty", i18n.Vars{"suite": suite})
	case b.comprobado == nil:
	case b.comprobado.Available:
		pista, tono = t.T("ob.identity.slugFree", i18n.Vars{"subdomain": b.comprobado.Subdomain}), ui.AyudaExito
	default:
		pista, tono = b.comprobado.Reason, ui.AyudaPeligro
		if pista == "" {
			pista = tt("ob.identity.slugTaken")
		}
	}
	c.slug.Invalida = slugValor != "" && b.comprobado != nil && !b.comprobado.Available
	partes := []bloque{
		&campoB{rotulo: tt("ob.identity.nameLabel"), obligatorio: true, control: c.nombre, pista: ui.NuevaPistaCampo(pistaNombre)},
		&campoB{
			rotulo: tt("ob.identity.slugLabel"), obligatorio: true, control: c.slug, sufijo: "." + suite,
			pista: ui.NuevaPistaCampo(pista).ConTono(tono),
		},
	}
	if org != nil && org.DNS != nil {
		partes = append(partes, r.estadoSubdominio(*org.DNS))
	}
	anfitrion := slugValor
	if anfitrion == "" {
		anfitrion = tt("ob.domain.keep.placeholder")
	}
	c.opcDominio[0].Titulo = t.T("ob.domain.keep.title", i18n.Vars{"host": anfitrion + "." + suite})
	for i, valor := range []string{"subdomain", "own", "search"} {
		c.opcDominio[i].Elegida = b.eleccion == valor
		c.opcDominio[i].Refresh()
	}
	partes = append(partes, col(MargenLeyenda,
		parrafo{ui.NuevoParrafo(tt("ob.domain.legend"), tema.TextoSM, ui.SemiNegrita, "text", v)},
		opcionesB{HuecoFilaAlta, c.opcDominio}))
	switch b.eleccion {
	case "own":
		if org == nil {
			partes = append(partes, nuevaAyuda(ui.AyudaInfo, tt("ob.identity.firstTitle"), tt("ob.identity.firstBody")))
		} else {
			partes = append(partes, r.dominioPropio(v))
		}
	case "search":
		partes = append(partes, r.panelBusqueda(v))
	}
	p.cuerpo = tope{AnchoFormAlta, col(HuecoFormAlta, partes...)}
	return p
}

// estadoSubdominio es `SubdomainStatus`: no se dice «funciona» hasta que el registro DNS
// está publicado de verdad.
func (r *bienvenidaRenderer) estadoSubdominio(dns api.AltaSubdominio) bloque {
	t := r.b.T
	switch dns.Status {
	case "live":
		return nuevaAyuda(ui.AyudaExito, t.T("ob.sub.liveTitle", nil), t.T("ob.sub.liveBody", i18n.Vars{"host": dns.Host}))
	case "failed":
		motivo := dns.Detail
		if motivo == "" {
			motivo = t.T("ob.sub.failedDefault", nil)
		}
		return nuevaAyuda(ui.AyudaAviso, t.T("ob.sub.failedTitle", nil), motivo+" "+t.T("ob.sub.failedTail", nil))
	}
	return nuevaAyuda(ui.AyudaNeutra, t.T("ob.sub.reservedTitle", nil), t.T("ob.sub.reservedBody", i18n.Vars{"host": dns.Host}))
}

// dominioPropio es el bloque del dominio que ya se tiene: el campo, quién crea los
// registros y el panel que toca, con el camino manual siempre a mano.
func (r *bienvenidaRenderer) dominioPropio(v tema.Variante) bloque {
	b, c, t := r.b, &r.c, r.b.T
	tt := func(k string) string { return t.T(k, nil) }
	dominio := strings.TrimSpace(b.dominio)
	if dominio == "" {
		dominio = tt("ob.dns.fallback")
	}
	for i, valor := range []string{"cloudflare", "manual"} {
		c.opcDNS[i].Elegida = b.dnsHost == valor
		c.opcDNS[i].Refresh()
	}
	partes := []bloque{
		&campoB{rotulo: tt("ob.domain.hostLabel"), control: c.host, pista: ui.NuevaPistaCampo(tt("ob.domain.hostHint"))},
		col(MargenLeyenda,
			parrafo{ui.NuevoParrafo(t.T("ob.dns.legend", i18n.Vars{"domain": dominio}), tema.TextoSM, ui.SemiNegrita, "text", v)},
			opcionesB{HuecoFilaAlta, c.opcDNS}),
	}
	if b.dnsHost != "cloudflare" {
		return col(HuecoPanelAlta, append(partes, r.panelDominio(v))...)
	}
	partes = append(partes, r.panelCloudflare(v, dominio))
	// El plan B, plegado: la tabla copiable está SIEMPRE disponible.
	if c.manual == nil {
		c.manual = nuevosDetalles(tt("ob.dns.manualSummary"), col(0))
		c.manual.AlCambiar = func() {
			b.mu.Lock()
			b.manualAbier = c.manual.Abierto
			b.mu.Unlock()
			b.Refresh()
		}
	}
	c.manual.Abierto = b.manualAbier
	c.manual.contenido = r.panelDominio(v)
	return col(HuecoPanelAlta, append(partes, c.manual)...)
}

// panelDominio es `DomainPanel`: el registro exacto para copiar y su estado real.
func (r *bienvenidaRenderer) panelDominio(v tema.Variante) bloque {
	b, c, t := r.b, &r.c, r.b.T
	tt := func(k string) string { return t.T(k, nil) }
	d := b.estado.Domain
	var partes []bloque
	if d == nil {
		partes = append(partes, parrafo{ui.NuevoParrafo(tt("ob.dnsRec.empty"), tema.TextoSM, ui.Normal, "text-subtle", v)})
		c.comprobar.Texto, c.comprobar.Variante = tt("ob.dnsRec.save"), ui.Primario
	} else {
		fila := func(rotulo, valor, clave string) bloque {
			f := nuevaFilaCopia(t, tt(rotulo), valor, func(s string) { r.copiar(s, clave) })
			f.Ventana, f.RotuloSM = b.Ancho(), RotuloCopiaAlta
			return f
		}
		partes = append(partes,
			col(0,
				parrafo{ui.NuevoParrafo(tt("ob.dnsRec.title"), tema.TextoSM, ui.SemiNegrita, "text", v)},
				espacio{MargenEntradaAlta},
				parrafo{ui.NuevoParrafo(tt("ob.dnsRec.body"), smRelajada, ui.Normal, "text-muted", v)},
				espacio{HuecoFilaAlta},
				col(HuecoCasillas,
					fila("ob.dnsRec.type", "TXT", "ob.copied.recordType"),
					fila("ob.dnsRec.name", d.RecordName, "ob.copied.recordName"),
					fila("ob.dnsRec.value", d.RecordValue, "ob.copied.recordValue"))),
			r.estadoDominio(*d))
		c.comprobar.Texto, c.comprobar.Variante = tt("ob.dnsRec.check"), ui.Primario
		if d.Verified {
			c.comprobar.Variante = ui.Secundario
		}
	}
	c.comprobar.Cargando = b.ocupado
	c.comprobar.Refresh()
	partes = append(partes, fijo{c.comprobar})
	return panelB(v, col(HuecoPanelAlta, partes...))
}

func (r *bienvenidaRenderer) estadoDominio(d api.AltaDominio) bloque {
	t := r.b.T
	tt := func(k string) string { return t.T(k, nil) }
	switch {
	case d.Verified:
		return nuevaAyuda(ui.AyudaExito, tt("ob.dnsRec.verifiedTitle"), t.T("ob.dnsRec.verifiedBody", i18n.Vars{"host": d.Host}))
	case d.Status == "mismatch":
		encontrado := ""
		if len(d.Found) > 0 {
			encontrado = d.Found[0]
		}
		return nuevaAyuda(ui.AyudaAviso, tt("ob.dnsRec.mismatchTitle"),
			t.T("ob.dnsRec.mismatchBody", i18n.Vars{"name": d.RecordName, "found": encontrado}))
	case d.Status == "unreachable":
		return nuevaAyuda(ui.AyudaPeligro, tt("ob.dnsRec.unreachableTitle"), tt("ob.dnsRec.unreachableBody"))
	case d.Status == "pending":
		return nuevaAyuda(ui.AyudaNeutra, tt("ob.dnsRec.pendingTitle"), tt("ob.dnsRec.pendingBody"))
	}
	// Sin `status` no se ha consultado el DNS aquí: no se afirma nada.
	return nuevaAyuda(ui.AyudaNeutra, tt("ob.dnsRec.uncheckedTitle"), tt("ob.dnsRec.uncheckedBody"))
}

// estadoCloudflare es cómo se lee cada estado del plan: texto además de color.
var estadoCloudflare = map[string]ui.Tono{
	"ok": ui.InsigniaExito, "created": ui.InsigniaExito, "replaced": ui.InsigniaExito,
	"missing": ui.InsigniaInfo, "conflict": ui.InsigniaAviso, "failed": ui.InsigniaPeligro, "pending": ui.InsigniaNeutra,
}

// panelCloudflare es `CloudflarePanel`: primero se MIRA y después se toca la zona, y un
// registro que choca con algo del cliente no se sustituye sin que lo marque.
//
//nolint:gocyclo // La respuesta de Cloudflare tiene ramas de estado que se muestran juntas.
func (r *bienvenidaRenderer) panelCloudflare(v tema.Variante, dominio string) bloque {
	b, c, t := r.b, &r.c, r.b.T
	tt := func(k string) string { return t.T(k, nil) }
	plan := b.cfPlan
	var registros []api.AltaRegistroCloudflare
	if plan != nil {
		registros = plan.Records
	}
	porHacer, conflictos, fallos := 0, 0, 0
	for _, x := range registros {
		switch {
		case x.State == "missing", x.State == "conflict" && slices.Contains(b.cfSustituir, x.Key):
			porHacer++
		}
		if x.State == "conflict" {
			conflictos++
		}
		if x.State == "failed" {
			fallos++
		}
	}
	puede := strings.TrimSpace(b.dominio) != "" && (strings.TrimSpace(b.cfToken) != "" || (plan != nil && plan.TokenStored))

	pasos := nuevaNumerada(v,
		ui.NuevoParrafo(tt("ob.cf.step1"), smRelajada, ui.Normal, "text-muted", v),
		ui.NuevoParrafo(tt("ob.cf.step2"), smRelajada, ui.Normal, "text-muted", v),
		ui.NuevoParrafo(t.T("ob.cf.step3", i18n.Vars{"domain": dominio}), smRelajada, ui.Normal, "text-muted", v),
	)
	// ponytail: en la web el enlace va en la misma línea que el primer paso, detrás de
	// su texto; aquí va en su propia línea debajo de la lista. Un `Parrafo` no sabe
	// meter un enlace en línea.
	enlace := ui.NuevoEnlace(tt("ob.cf.step1Link"), tema.TextoSM, ui.Medio, "primary-text", func() {
		if b.AbrirURL != nil {
			b.AbrirURL("https://dash.cloudflare.com/profile/api-tokens")
		}
	})
	c.correo.Etiqueta, c.correo.Marcada = t.T("ob.cf.mailLabel", i18n.Vars{"domain": dominio}), b.cfCorreo
	c.record.Marcada = b.cfRecordar
	c.correo.Refresh()
	c.record.Refresh()
	pistaCorreo := ui.NuevaPistaCampo(tt("ob.cf.mailHintOff"))
	if b.cfCorreo {
		pistaCorreo = ui.NuevaPistaCampo(tt("ob.cf.mailHintOn")).ConTono(ui.AyudaAviso)
	}
	c.cfMirar.Texto, c.cfMirar.Variante = tt("ob.cf.preview"), ui.Primario
	if plan != nil {
		c.cfMirar.Texto, c.cfMirar.Variante = tt("ob.cf.previewAgain"), ui.Secundario
	}
	c.cfMirar.Inerte, c.cfMirar.Cargando = !puede, b.ocupado
	c.cfMirar.Refresh()
	botones := []fyne.CanvasObject{c.cfMirar}
	if plan != nil {
		c.cfApl.Texto = tt("ob.cf.nothing")
		if porHacer > 0 {
			c.cfApl.Texto = t.T("ob.cf.apply", i18n.Vars{"count": porHacer})
		}
		c.cfApl.Inerte, c.cfApl.Cargando = porHacer == 0 || !puede, b.ocupado
		c.cfApl.Refresh()
		botones = append(botones, c.cfApl)
	}

	partes := []bloque{
		col(0,
			parrafo{ui.NuevoParrafo(tt("ob.cf.permTitle"), tema.TextoSM, ui.SemiNegrita, "text", v)},
			espacio{HuecoCasillas},
			pasos, fijo{enlace}),
		&campoB{rotulo: tt("ob.cf.tokenField"), control: c.token, pista: ui.NuevaPistaCampo(tt("ob.cf.tokenHint"))},
		col(HuecoCasillas, fijo{c.correo}, pistaB{pistaCorreo}, fijo{c.record}, pistaB{ui.NuevaPistaCampo(tt("ob.cf.rememberHint"))}),
		filaB{HuecoCasillas, botones},
	}
	if plan != nil {
		token, estado := tt("ob.cf.tokenDiscarded"), tt("ob.cf.pending")
		if plan.TokenStored {
			token = tt("ob.cf.tokenStored")
		}
		if plan.Domain.Verified {
			estado = tt("ob.cf.verified")
		}
		partes = append(partes, nuevoResumen(b.Ancho(), [][2]string{
			{tt("ob.cf.zone"), plan.Zone.Name}, {tt("ob.cf.token"), token}, {tt("ob.cf.domainState"), estado},
		}))
		lista := make([]bloque, 0, len(registros))
		for _, x := range registros {
			lista = append(lista, r.registroCloudflare(v, x))
		}
		partes = append(partes, col(HuecoCasillas, lista...))
		switch {
		case fallos > 0:
			partes = append(partes, nuevaAyuda(ui.AyudaPeligro, tt("ob.cf.failTitle"), tt("ob.cf.failBody")))
		case conflictos > 0:
			partes = append(partes, nuevaAyuda(ui.AyudaAviso, tt("ob.cf.conflictTitle"), tt("ob.cf.conflictBody")))
		case plan.Domain.Verified:
			partes = append(partes, nuevaAyuda(ui.AyudaExito, tt("ob.cf.okTitle"), t.T("ob.cf.okBody", i18n.Vars{"host": plan.Domain.Host})))
		default:
			partes = append(partes, nuevaAyuda(ui.AyudaNeutra, tt("ob.cf.todoTitle"), tt("ob.cf.todoBody")))
		}
	}
	return panelB(v, col(HuecoPanelAlta, partes...))
}

// registroCloudflare es una fila del plan: su estado, el registro, para qué es y, en un
// conflicto, lo que hay ahora y la casilla que autoriza a sustituirlo.
func (r *bienvenidaRenderer) registroCloudflare(v tema.Variante, x api.AltaRegistroCloudflare) bloque {
	b, c, t := r.b, &r.c, r.b.T
	cabeza := []fyne.CanvasObject{ui.NuevaInsignia(t.T("ob.cf.state."+x.State, nil), estadoCloudflare[x.State])}
	codigo := ui.Texto(x.Type+" "+x.Name, tema.TextoXS, ui.Normal, tema.Color("text", v))
	codigo.FontSource, codigo.TextStyle.Monospace = nil, true
	cabeza = append(cabeza, codigo)
	if x.Content != "" {
		contenido := x.Content
		cabeza = append(cabeza, ui.NuevoBoton(t.T("devices.copy", nil), func() { r.copiar(contenido, "ob.copied.recordValue") }).Con(ui.Fantasma, ui.TallaSM))
	}
	partes := []bloque{filaB{HuecoCasillas, cabeza}, parrafo{ui.NuevoParrafo(x.Purpose, smAjustada, ui.Normal, "text-muted", v)}}
	if x.Content != "" {
		valor := x.Content
		if x.Priority > 0 {
			valor = strconv.Itoa(x.Priority) + " " + valor
		}
		partes = append(partes, parrafo{ui.NuevoParrafo(valor, tema.TextoXS, ui.Normal, "text-subtle", v)})
	}
	if x.Detail != "" {
		partes = append(partes, parrafo{ui.NuevoParrafo(x.Detail, xsAjustada, ui.Normal, "text-subtle", v)})
	}
	if x.State == "conflict" {
		casilla, ok := c.sustituir[x.Key]
		if !ok {
			clave := x.Key
			casilla = ui.NuevaCasilla(t.T("ob.cf.replaceWithSytue", nil), func(bool) {
				b.mu.Lock()
				if i := slices.Index(b.cfSustituir, clave); i >= 0 {
					b.cfSustituir = slices.Delete(b.cfSustituir, i, i+1)
				} else {
					b.cfSustituir = append(b.cfSustituir, clave)
				}
				b.mu.Unlock()
				b.Refresh()
			})
			c.sustituir[x.Key] = casilla
		}
		casilla.Marcada = slices.Contains(b.cfSustituir, x.Key)
		casilla.Refresh()
		partes = append(partes, col(MargenEntradaAlta,
			parrafo{ui.NuevoParrafo(t.T("ob.cf.nowThere", i18n.Vars{"current": x.Current}), xsAjustada, ui.Normal, "text-muted", v)},
			fijo{casilla}))
	}
	return &cajaB{
		fondo: "surface", borde: "border", radio: tema.RadioMD, rx: HuecoFilaAlta, ry: 2.5 * 0.25 * tema.Rem,
		hijo: col(1.5*0.25*tema.Rem, partes...),
	}
}

// panelBusqueda es `DomainSearchPanel`: SOLO una consulta; aquí no se compra nada.
func (r *bienvenidaRenderer) panelBusqueda(v tema.Variante) bloque {
	b, c, t := r.b, &r.c, r.b.T
	tt := func(k string) string { return t.T(k, nil) }
	partes := []bloque{&campoB{rotulo: tt("ob.search.label"), control: c.buscar, pista: ui.NuevaPistaCampo(tt("ob.search.hint"))}}
	if d := b.disponible; d != nil {
		partes = append(partes, r.resultadoDisponibilidad(*d))
	}
	c.buscarBtn.Inerte, c.buscarBtn.Cargando = strings.TrimSpace(b.buscado) == "", b.ocupado
	c.buscarBtn.Refresh()
	botones := []fyne.CanvasObject{c.buscarBtn}
	if b.disponible != nil && b.disponible.Status == "available" {
		botones = append(botones, c.usarBtn)
	}
	nota := ui.NuevaAyudaEnLinea(tt("ob.search.note"))
	nota.SinIcono = true
	partes = append(partes, filaB{HuecoFilaAlta, botones}, ayuda{nota})
	return panelB(v, col(HuecoPanelAlta, partes...))
}

func (r *bienvenidaRenderer) resultadoDisponibilidad(d api.AltaDisponibilidad) bloque {
	t := r.b.T
	tt := func(k string) string { return t.T(k, nil) }
	switch d.Status {
	case "available":
		cuerpo := tt("ob.avail.freeNoPrice")
		if p := d.Price; p != nil {
			cuerpo = t.T("ob.avail.freePrice", i18n.Vars{
				"registration": dinero(t, p.Registration, p.Currency), "renewal": dinero(t, p.Renewal, p.Currency),
			})
		}
		return nuevaAyuda(ui.AyudaExito, t.T("ob.avail.freeTitle", i18n.Vars{"domain": d.Domain}), cuerpo)
	case "taken":
		return nuevaAyuda(ui.AyudaAviso, t.T("ob.avail.takenTitle", i18n.Vars{"domain": d.Domain}), tt("ob.avail.takenBody"))
	case "unsupported":
		return nuevaAyuda(ui.AyudaNeutra, tt("ob.avail.unsupportedTitle"), tt("ob.avail.unsupportedBody"))
	case "invalid":
		return nuevaAyuda(ui.AyudaAviso, tt("ob.avail.invalidTitle"), tt("ob.avail.invalidBody"))
	case "unconfigured":
		return nuevaAyuda(ui.AyudaNeutra, tt("ob.avail.unconfiguredTitle"), tt("ob.avail.unconfiguredBody"))
	}
	return nuevaAyuda(ui.AyudaPeligro, tt("ob.avail.unknownTitle"), tt("ob.avail.unknownBody"))
}

// dinero es `money` de la web: el importe del registrador con el formato del idioma; si
// no es un número, su respuesta literal antes que un importe inventado.
//
// ponytail: el formato de la suite solo sabe euros (`i18n.FormatCurrency`); otra
// moneda sale literal, que es lo que la web hace cuando `Intl` no la conoce.
func dinero(t *i18n.Translator, cantidad, moneda string) string {
	n, err := strconv.ParseFloat(cantidad, 64)
	if err != nil || moneda != "EUR" {
		return cantidad + " " + moneda
	}
	return euros(t, n)
}

// euros es un importe con el formato del idioma.
func euros(t *i18n.Translator, n float64) string {
	return i18n.FormatCurrency(t.Locale(), int64(math.Round(n*100)))
}

// ---- Paso 3: primeras personas ----

func (r *bienvenidaRenderer) cuerpoPersonas(v tema.Variante) bloque {
	b, c, t := r.b, &r.c, r.b.T
	tt := func(k string) string { return t.T(k, nil) }
	c.rol.Elegida = max(slices.Index(valoresRol, b.rolInvitar), 0)
	c.rol.Refresh()
	c.enviar.Inerte, c.enviar.Cargando = strings.TrimSpace(b.correos) == "", b.ocupado
	c.enviar.Refresh()
	partes := []bloque{
		&campoB{rotulo: tt("ob.people.emailsLabel"), control: c.correos, pista: ui.NuevaPistaCampo(tt("ob.people.emailsHint"))},
		&campoB{rotulo: tt("ob.people.roleLabel"), control: c.rol, pista: ui.NuevaPistaCampo(tt("ob.people.roleHint"))},
		filaB{HuecoFilaAlta, []fyne.CanvasObject{c.enviar, ui.Texto(tt("ob.people.sendNote"), tema.TextoXS, ui.Normal, tema.Color("text-subtle", v))}},
	}
	if len(b.invitadas) > 0 {
		res := make([]bloque, 0, len(b.invitadas))
		for _, x := range b.invitadas {
			res = append(res, r.resultadoInvitacion(x))
		}
		partes = append(partes, col(HuecoCasillas, res...))
	}
	if inv := b.estado.Invitations; len(inv) > 0 {
		filas := make([]bloque, 0, len(inv))
		for _, i := range inv {
			filas = append(filas, &filaPendiente{
				tic:    iconos.Trazo(iconos.Tic, 14, "success-text", v),
				correo: ui.Texto(i.Email, tema.TextoXS, ui.Normal, tema.Color("text-muted", v)),
				rol:    ui.Texto("· "+nombreRol(t, i.Role), tema.TextoXS, ui.Normal, tema.Color("text-subtle", v)),
			})
		}
		partes = append(partes, &cajaB{
			fondo: "surface-raised", borde: "border", radio: tema.RadioLG, rx: HuecoFilaAlta, ry: HuecoFilaAlta,
			hijo: col(HuecoCasillas, parrafo{ui.NuevoParrafo(tt("ob.people.pendingTitle"), tema.TextoSM, ui.SemiNegrita, "text", v)},
				col(MargenEntradaAlta, filas...)),
		})
	} else {
		vacio := ui.NuevoVacio(tt("ob.people.emptyTitle"), tt("ob.people.emptyBody"))
		vacio.Ilustracion = Glifo(iconos.Trazo(iconos.Usuarios, 28, "text-muted", v), 28)
		// `border border-dashed border-border-subtle`: el borde discontinuo de la web.
		partes = append(partes, &cajaB{fondo: "bg", borde: "border-subtle", discontinuo: true, radio: tema.RadioLG, hijo: vacioB{vacio}})
	}
	return tope{AnchoFormAlta, col(HuecoFormAlta, partes...)}
}

func (r *bienvenidaRenderer) resultadoInvitacion(x api.AltaInvitacion) bloque {
	t := r.b.T
	tono, clave := ui.AyudaPeligro, "ob.invite.failed"
	switch x.Status {
	case "sent":
		tono, clave = ui.AyudaExito, "ob.invite.sent"
	case "already_member":
		tono, clave = ui.AyudaAviso, "ob.invite.already"
	case "invalid":
		tono, clave = ui.AyudaAviso, "ob.invite.invalid"
	case "not_sent":
		clave = "ob.invite.notSent"
	}
	cuerpo := x.Detail
	if cuerpo == "" {
		cuerpo = t.T("ob.invite.default", nil)
	}
	return nuevaAyuda(tono, x.Email+" — "+t.T(clave, nil), cuerpo)
}

// nombreRol es `roleLabel`: el nombre del catálogo, o el identificador si no hay.
func nombreRol(t *i18n.Translator, rol string) string {
	switch rol {
	case "owner", "admin", "member", "billing":
		return t.T("ob.roleName."+rol, nil)
	}
	return rol
}

// ---- Paso 4: contacto y facturación ----

func (r *bienvenidaRenderer) pasoFacturacion() pasoAlta {
	b, c, t := r.b, &r.c, r.b.T
	tt := func(k string) string { return t.T(k, nil) }
	sub := b.suscripcion
	activos := 1
	if sub != nil {
		activos = max(sub.ActiveMembers, 1)
	}
	p := pasoAlta{
		titulo: tt("ob.billing.title"), encabezado: tt("ob.billing.heading"), descripcion: tt("ob.billing.description"),
		siguiente: tt("ob.billing.pay"), puede: b.org() != nil && b.asientos >= activos,
	}
	switch {
	case sub != nil && sub.Entitled:
		p.siguiente = tt("ob.billing.next")
	case b.pagoVuelto:
		p.siguiente = tt("ob.billing.verify")
	}
	var partes []bloque
	if b.esOrg() {
		partes = append(partes,
			&campoB{rotulo: tt("ob.billing.legalLabel"), control: c.legal, pista: ui.NuevaPistaCampo(tt("ob.billing.legalHint"))},
			&campoB{rotulo: tt("ob.billing.taxLabel"), control: c.nif, pista: ui.NuevaPistaCampo(tt("ob.billing.taxHint"))},
			&campoB{rotulo: tt("ob.billing.addressLabel"), control: c.direccion, pista: ui.NuevaPistaCampo(tt("ob.billing.addressHint"))})
	}
	if b.estado.Account != nil {
		c.email.PonerMarcaDeAgua(b.estado.Account.Email)
	}
	asientos := &campoB{rotulo: tt("ob.billing.seatsLabel"), obligatorio: true, control: c.asientos}
	if sub != nil {
		precio := float64(sub.UnitPriceCents) / 100
		asientos.pista = ui.NuevaPistaCampo(t.T("ob.billing.seatsHint", i18n.Vars{
			"seats": b.asientos, "price": euros(t, precio), "total": euros(t, float64(b.asientos)*precio),
		}))
	}
	partes = append(partes,
		&campoB{rotulo: tt("ob.billing.emailLabel"), control: c.email, pista: ui.NuevaPistaCampo(tt("ob.billing.emailHint"))},
		asientos)
	switch {
	case sub != nil && sub.Entitled:
		partes = append(partes, nuevaAyuda(ui.AyudaExito, tt("ob.billing.active"), tt("ob.billing.activeBody")))
	case b.pagoVuelto:
		partes = append(partes, nuevaAyuda(ui.AyudaAviso, tt("ob.billing.pending"), tt("ob.billing.pendingBody")))
	}
	nota := ui.NuevaAyudaEnLinea(tt("ob.billing.note"))
	nota.SinIcono = true
	p.cuerpo = tope{AnchoFormAlta, col(HuecoFormAlta, append(partes, ayuda{nota})...)}
	return p
}

// ---- Paso 5: roles recomendados ----

func (r *bienvenidaRenderer) cuerpoRoles(v tema.Variante) bloque {
	b, c, t := r.b, &r.c, r.b.T
	creados := map[string]bool{}
	for _, x := range b.estado.Roles {
		creados[x.Name] = true
	}
	partes := make([]bloque, 0, len(b.estado.RecommendedRoles)+1)
	for _, rol := range b.estado.RecommendedRoles {
		ya := creados[rol.Name]
		casilla, ok := c.casillasRol[rol.Key]
		if !ok {
			clave := rol.Key
			nombre := rol.Name
			casilla = ui.NuevaCasilla(rol.Name, func(si bool) {
				b.mu.Lock()
				// El `disabled` de la web: uno ya creado, o con una acción en vuelo,
				// no se desmarca.
				// ponytail: `ui.Eleccion` no tiene estado inerte; se ignora el cambio.
				if !b.ocupado && !b.rolCreado(nombre) {
					b.elegidos = slices.DeleteFunc(b.elegidos, func(k string) bool { return k == clave })
					if si {
						b.elegidos = append(b.elegidos, clave)
					}
				}
				b.mu.Unlock()
				b.Refresh()
			})
			c.casillasRol[rol.Key] = casilla
		}
		casilla.Marcada = ya || slices.Contains(b.elegidos, rol.Key)
		casilla.Refresh()
		cabeza := []fyne.CanvasObject{casilla}
		if ya {
			cabeza = append(cabeza, ui.NuevaInsignia(t.T("ob.roles.already", nil), ui.InsigniaExito))
		}
		ver, ok := c.permisos[rol.Key]
		if !ok {
			clave := rol.Key
			ver = ui.NuevoEnlace(t.T("ob.roles.seePermissions", nil), tema.TextoXS, ui.Normal, "text-subtle", func() {
				b.mu.Lock()
				b.permisos[clave] = !b.permisos[clave]
				b.mu.Unlock()
				b.Refresh()
			})
			ver.Boton = true
			c.permisos[rol.Key] = ver
		}
		dentro := []bloque{parrafo{ui.NuevoParrafo(rol.Summary, smAjustada, ui.Normal, "text-muted", v)}, espacio{1.5 * 0.25 * tema.Rem}, fijo{ver}}
		if b.permisos[rol.Key] {
			var fichas []fyne.CanvasObject
			for _, p := range rol.Permissions {
				fichas = append(fichas, nuevaFicha(p))
			}
			dentro = append(dentro, espacio{MargenEntradaAlta}, filaB{MargenEntradaAlta, fichas})
		}
		borde := "border"
		if casilla.Marcada {
			borde = "primary"
		}
		partes = append(partes, &cajaB{
			fondo: "surface", borde: borde, radio: tema.RadioLG, rx: HuecoFilaAlta, ry: HuecoFilaAlta,
			hijo: col(MargenEntradaAlta, filaB{HuecoCasillas, cabeza}, sangria{ui.LadoEleccion + ui.HuecoEleccion, col(0, dentro...)}),
		})
	}
	partes = append(partes, nuevaAyuda(ui.AyudaInfo, t.T("ob.roles.helpTitle", nil), t.T("ob.roles.helpBody", nil)))
	return tope{AnchoIntro, col(HuecoFilaAlta, partes...)}
}

// ---- Paso final: el resumen de lo que hay DE VERDAD ----

//nolint:gocognit,gocyclo // El resumen reúne cada condición visible del alta finalizada.
func (r *bienvenidaRenderer) cuerpoListo(v tema.Variante) bloque {
	b, c, t := r.b, &r.c, r.b.T
	tt := func(k string) string { return t.T(k, nil) }
	ids := pasosDe(b.modoEfectivo())
	org, dom, esOrg := b.org(), b.estado.Domain, b.esOrg()
	editar := func(paso string) *ui.Enlace {
		e, ok := c.editar[paso]
		if !ok {
			e = ui.NuevoEnlace(tt("ob.sum.edit"), tema.TextoSM, ui.SemiNegrita, "primary-text", func() {
				b.mu.Lock()
				b.paso = slices.Index(pasosDe(b.modoEfectivo()), paso)
				b.mu.Unlock()
				b.Refresh()
			})
			e.Boton = true
			c.editar[paso] = e
		}
		return e
	}
	raya := "—"
	cuenta, direccion := raya, raya
	if org != nil {
		cuenta, direccion = org.Name, org.Subdomain
		if org.DNS != nil && org.DNS.Status == "live" {
			direccion = t.T("ob.sum.addressLive", i18n.Vars{"host": org.Subdomain})
		} else if org.DNS != nil && org.DNS.Status == "failed" {
			direccion = t.T("ob.sum.addressPending", i18n.Vars{"host": org.Subdomain})
		}
	}
	propio := tt("ob.sum.noDomain")
	if dom != nil {
		propio = t.T("ob.sum.domainPending", i18n.Vars{"host": dom.Host})
		if dom.Verified {
			propio = t.T("ob.sum.domainVerified", i18n.Vars{"host": dom.Host})
		}
	}
	etiqueta := tt("ob.sum.account")
	if esOrg {
		etiqueta = tt("ob.sum.org")
	}
	filas := []filaResumenAlta{
		{etiqueta, cuenta, false, editar(api.AltaPasoIdentidad)},
		{tt("ob.sum.address"), direccion, true, nil},
		{tt("ob.sum.ownDomain"), propio, false, editar(api.AltaPasoIdentidad)},
	}
	if esOrg {
		inv := tt("ob.sum.noInvitations")
		if n := len(b.estado.Invitations); n > 0 {
			inv = t.T("ob.sum.invitationsPending", i18n.Vars{"count": n})
		}
		filas = append(filas, filaResumenAlta{tt("ob.sum.invitations"), inv, false, editar(api.AltaPasoPersonas)})
	}
	fact := tt("ob.sum.billingEmpty")
	if f := b.estado.Billing; f != nil && (f.LegalName != "" || f.BillingEmail != "") {
		fact = f.LegalName
		if fact == "" {
			fact = f.BillingEmail
		}
	}
	filas = append(filas, filaResumenAlta{tt("ob.sum.billing"), fact, false, editar(api.AltaPasoFacturacion)})
	if esOrg {
		roles := tt("ob.sum.noRoles")
		if len(b.estado.Roles) > 0 {
			nombres := make([]string, len(b.estado.Roles))
			for i, x := range b.estado.Roles {
				nombres[i] = x.Name
			}
			roles = strings.Join(nombres, ", ")
		}
		filas = append(filas, filaResumenAlta{tt("ob.sum.roles"), roles, false, editar(api.AltaPasoRoles)})
	}
	_ = ids
	partes := []bloque{nuevoResumenAlta(b.Ancho(), filas)}
	if dom != nil && !dom.Verified {
		sub := ""
		if org != nil {
			sub = org.Subdomain
		}
		paso := tt("ob.identity.titleMe")
		if esOrg {
			paso = tt("ob.identity.titleOrg")
		}
		partes = append(partes, nuevaAyuda(ui.AyudaAviso, t.T("ob.done.warnTitle", i18n.Vars{"host": dom.Host}),
			t.T("ob.done.warnBody", i18n.Vars{"subdomain": sub, "step": paso})))
	}
	siguientes := c.siguientes[:3]
	if esOrg {
		siguientes = c.siguientes
	}
	enlaces := make([]bloque, 0, len(siguientes))
	for _, s := range siguientes {
		enlaces = append(enlaces, s)
	}
	partes = append(partes, &cajaB{
		fondo: "surface-raised", borde: "border", radio: tema.RadioLG, rx: RellenoPanelAlta, ry: RellenoPanelAlta,
		hijo: col(HuecoFilaAlta, parrafo{ui.NuevoParrafo(tt("ob.done.nextTitle"), tema.TextoSM, ui.SemiNegrita, "text", v)},
			col(HuecoFilaAlta, enlaces...)),
	})
	return col(HuecoFormAlta, partes...)
}

// copiar lleva un valor al portapapeles con el aviso de la web.
func (r *bienvenidaRenderer) copiar(valor, clave string) {
	if r.b.Copiar != nil {
		r.b.Copiar(valor, clave)
	}
}

// ---- Piezas de los pasos ----

// campoB es `FieldGroup`: el rótulo con su asterisco, el control a todo el ancho —con
// el sufijo pegado a la derecha si lo lleva— y la pista debajo.
type campoB struct {
	rotulo      string
	obligatorio bool
	control     fyne.CanvasObject
	sufijo      string
	pista       *ui.PistaCampo

	cabeza, asterisco *canvas.Text
	caja              *ui.Caja
	suf               *canvas.Text
}

// RotuloCopiaAlta es el `sm:w-24` del rótulo de las filas de copiar del alta.
const RotuloCopiaAlta float32 = 24 * 0.25 * tema.Rem

// MargenSufijo es el `px-3` de la caja del sufijo.
const MargenSufijo float32 = 3 * 0.25 * tema.Rem

func (f *campoB) preparar() {
	if f.cabeza != nil {
		return
	}
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	f.cabeza = ui.Texto(f.rotulo, tema.TextoSM, ui.SemiNegrita, tema.Color("text", v))
	if f.obligatorio {
		f.asterisco = ui.Texto("*", tema.TextoSM, ui.SemiNegrita, tema.Color("danger-text", v))
	}
	if f.sufijo != "" {
		f.caja = ui.NuevaCaja(tema.Color("bg-subtle", v), tema.Color("border-strong", v), tema.RadioMD)
		f.suf = ui.Texto(f.sufijo, tema.TextoSM, ui.Normal, tema.Color("text-muted", v))
		f.suf.FontSource, f.suf.TextStyle.Monospace = nil, true
	}
}

func (f *campoB) altoPista(ancho float32) float32 {
	if f.pista == nil {
		return 0
	}
	f.pista.Ancho = ancho
	return ui.HuecoGrupoCampo + ui.MedidaPistaCampo(f.pista).Height
}

func (f *campoB) alto(ancho float32) float32 {
	return tema.TextoSM.Linea + ui.HuecoGrupoCampo + f.control.MinSize().Height + f.altoPista(ancho)
}

func (f *campoB) poner(x, y, ancho float32) {
	f.preparar()
	ui.ColocarTexto(f.cabeza, tema.TextoSM, x, y)
	if f.asterisco != nil {
		ui.ColocarTexto(f.asterisco, tema.TextoSM, x+ui.Medir(f.rotulo, tema.TextoSM, ui.SemiNegrita).Width+ui.MargenAsterisco, y)
	}
	y += tema.TextoSM.Linea + ui.HuecoGrupoCampo
	h := f.control.MinSize().Height
	campo := ancho
	if f.suf != nil {
		// ponytail: el campo de la web pierde el radio de la derecha (`rounded-r-none`)
		// y el sufijo se monta un píxel encima (`-ml-px`); aquí el sufijo tapa ese
		// píxel pero el campo conserva su esquina redonda por debajo.
		w := fyne.MeasureText(f.sufijo, tema.TextoSM.Tam, fyne.TextStyle{Monospace: true}).Width + 2*MargenSufijo + 2*tema.Borde
		campo = ancho - w + tema.Borde
		ui.MoverCaja(f.caja, fyne.NewPos(x+campo-tema.Borde, y), fyne.NewSize(w, h))
		ui.ColocarTexto(f.suf, tema.TextoSM, x+campo+MargenSufijo, y+(h-tema.TextoSM.Linea)/2)
	}
	ponerObjeto(f.control, x, y, fyne.NewSize(campo, h), true)
	if f.pista != nil {
		f.pista.Ancho = ancho
		ponerObjeto(f.pista, x, y+h+ui.HuecoGrupoCampo, ui.MedidaPistaCampo(f.pista), true)
	}
}

func (f *campoB) objetos() []fyne.CanvasObject {
	f.preparar()
	out := []fyne.CanvasObject{f.cabeza}
	if f.asterisco != nil {
		out = append(out, f.asterisco)
	}
	if f.caja != nil {
		out = append(out, f.control)
		out = append(out, f.caja.Objetos()...)
		out = append(out, f.suf)
	} else {
		out = append(out, f.control)
	}
	if f.pista != nil {
		out = append(out, f.pista)
	}
	return out
}

// opcionesB es una columna de tarjetas de elección a todo el ancho.
type opcionesB struct {
	hueco    float32
	opciones []*ui.TarjetaEleccion
}

func (o opcionesB) alto(ancho float32) float32 {
	var h float32
	for i, c := range o.opciones {
		if i > 0 {
			h += o.hueco
		}
		h += ui.AltoTarjetaEleccion(c, ancho)
	}
	return h
}

func (o opcionesB) poner(x, y, ancho float32) {
	for i, c := range o.opciones {
		if i > 0 {
			y += o.hueco
		}
		c.Ancho = ancho
		h := ui.AltoTarjetaEleccion(c, ancho)
		ponerObjeto(c, x, y, fyne.NewSize(ancho, h), true)
		y += h
	}
}

func (o opcionesB) objetos() []fyne.CanvasObject {
	out := make([]fyne.CanvasObject, len(o.opciones))
	for i, c := range o.opciones {
		out[i] = c
	}
	return out
}

// filaB es `flex items-center gap-*`: cada hijo a su medida, centrados en la fila.
//
// ponytail: sin `flex-wrap`; en estrecho una fila que no cabe se sale por la derecha.
// Las de este asistente son dos botones o un botón y una nota corta.
type filaB struct {
	hueco float32
	hijos []fyne.CanvasObject
}

func (f filaB) alto(float32) float32 {
	var h float32
	for _, o := range f.hijos {
		h = max(h, medidaDe(o).Height)
	}
	return h
}

func (f filaB) poner(x, y, _ float32) {
	h := f.alto(0)
	for _, o := range f.hijos {
		m := medidaDe(o)
		if t, ok := o.(*canvas.Text); ok {
			ui.ColocarTexto(t, tema.TextoXS, x, y+(h-m.Height)/2)
		} else {
			ponerObjeto(o, x, y+(h-m.Height)/2, m, true)
		}
		x += m.Width + f.hueco
	}
}

func (f filaB) objetos() []fyne.CanvasObject { return f.hijos }

// medidaDe es la caja de un hijo de fila. Los textos sueltos de estas filas son todos
// `text-xs`: su caja de línea es la de CSS (ver `ui.Medir`), y la monoespaciada se
// mide con su propia cara.
func medidaDe(o fyne.CanvasObject) fyne.Size {
	t, ok := o.(*canvas.Text)
	if !ok {
		return o.MinSize()
	}
	if t.TextStyle.Monospace {
		return fyne.NewSize(fyne.MeasureText(t.Text, t.TextSize, t.TextStyle).Width, tema.TextoXS.Linea)
	}
	return ui.Medir(t.Text, tema.TextoXS, ui.Normal)
}

// cajaB es una caja con borde y relleno: los paneles del alta.
type cajaB struct {
	fondo, borde string
	radio        float32
	rx, ry       float32
	hijo         bloque
	// discontinuo pinta el borde a trazos (`border-dashed`) con ui.BordeDiscontinuo.
	discontinuo bool

	caja  *ui.Caja
	trazo *ui.BordeDiscontinuo
}

func panelB(_ tema.Variante, hijo bloque) bloque {
	return &cajaB{fondo: "surface-raised", borde: "border", radio: tema.RadioLG, rx: RellenoPanelAlta, ry: RellenoPanelAlta, hijo: hijo}
}

func (c *cajaB) dentro(ancho float32) float32 { return max(ancho-2*(tema.Borde+c.rx), 0) }

func (c *cajaB) alto(ancho float32) float32 {
	return 2*(tema.Borde+c.ry) + c.hijo.alto(c.dentro(ancho))
}

func (c *cajaB) poner(x, y, ancho float32) {
	ui.MoverCaja(c.laCaja(), fyne.NewPos(x, y), fyne.NewSize(ancho, c.alto(ancho)))
	if c.trazo != nil {
		c.trazo.Move(fyne.NewPos(x, y))
		c.trazo.Resize(fyne.NewSize(ancho, c.alto(ancho)))
	}
	c.hijo.poner(x+tema.Borde+c.rx, y+tema.Borde+c.ry, c.dentro(ancho))
}

func (c *cajaB) objetos() []fyne.CanvasObject {
	out := c.laCaja().Objetos()
	if c.discontinuo {
		if c.trazo == nil {
			c.trazo = ui.NuevoBordeDiscontinuo(tema.Borde, c.radio, c.borde)
		}
		out = append(out, c.trazo)
	}
	return append(out, c.hijo.objetos()...)
}

// laCaja se crea al primer uso y es la misma entre `objetos` y `poner`.
func (c *cajaB) laCaja() *ui.Caja {
	if c.caja == nil {
		v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
		borde := tema.Color(c.borde, v)
		if c.discontinuo {
			borde = color.NRGBA{} // lo pinta el trazo
		}
		c.caja = ui.NuevaCaja(tema.Color(c.fondo, v), borde, c.radio)
	}
	return c.caja
}

// espacio es un hueco vertical fijo (`mt-*` entre dos bloques de una columna sin hueco).
type espacio struct{ h float32 }

func (e espacio) alto(float32) float32            { return e.h }
func (e espacio) poner(float32, float32, float32) {}
func (e espacio) objetos() []fyne.CanvasObject    { return nil }

// sangria desplaza lo de dentro (`pl-*`).
type sangria struct {
	x float32
	b bloque
}

func (s sangria) alto(ancho float32) float32   { return s.b.alto(ancho - s.x) }
func (s sangria) poner(x, y, ancho float32)    { s.b.poner(x+s.x, y, ancho-s.x) }
func (s sangria) objetos() []fyne.CanvasObject { return s.b.objetos() }

// pistaB es una pista suelta a todo el ancho.
type pistaB struct{ p *ui.PistaCampo }

func (p pistaB) alto(ancho float32) float32 {
	p.p.Ancho = ancho
	return ui.MedidaPistaCampo(p.p).Height
}

func (p pistaB) poner(x, y, ancho float32) {
	p.p.Ancho = ancho
	ponerObjeto(p.p, x, y, ui.MedidaPistaCampo(p.p), true)
}

func (p pistaB) objetos() []fyne.CanvasObject { return []fyne.CanvasObject{p.p} }

// numerada es el `<ol class="list-decimal pl-5 gap-1">` de los pasos de Cloudflare: el
// número del navegador va en la sangría, pegado al texto por la derecha.
type numerada struct {
	pasos  []*ui.Parrafo
	marcas []*canvas.Text
}

const (
	sangriaLista float32 = 5 * 0.25 * tema.Rem
	huecoLista   float32 = 1 * 0.25 * tema.Rem
	// huecoMarca es lo que el navegador deja entre el «1.» y el texto.
	huecoMarca float32 = 0.5 * 0.25 * tema.Rem
)

func nuevaNumerada(v tema.Variante, pasos ...*ui.Parrafo) *numerada {
	n := &numerada{pasos: pasos}
	for i := range pasos {
		n.marcas = append(n.marcas, ui.Texto(strconv.Itoa(i+1)+".", smRelajada, ui.Normal, tema.Color("text-muted", v)))
	}
	return n
}

func (n *numerada) alto(ancho float32) float32 {
	var h float32
	for i, p := range n.pasos {
		if i > 0 {
			h += huecoLista
		}
		h += ui.AltoParrafo(p, ancho-sangriaLista)
	}
	return h
}

func (n *numerada) poner(x, y, ancho float32) {
	for i, p := range n.pasos {
		if i > 0 {
			y += huecoLista
		}
		m := n.marcas[i]
		ui.ColocarTexto(m, smRelajada, x+sangriaLista-huecoMarca-ui.Medir(m.Text, smRelajada, ui.Normal).Width, y)
		y = ponerParrafo(p, x+sangriaLista, y, ancho-sangriaLista, true)
	}
}

func (n *numerada) objetos() []fyne.CanvasObject {
	out := make([]fyne.CanvasObject, 0, 2*len(n.pasos))
	for i, p := range n.pasos {
		out = append(out, n.marcas[i], p)
	}
	return out
}

// filaPendiente es una invitación pendiente: el tic, el correo y el rol.
type filaPendiente struct {
	tic         *iconos.Dibujo
	correo, rol *canvas.Text
}

func (f *filaPendiente) alto(float32) float32 { return tema.TextoSM.Linea }

func (f *filaPendiente) poner(x, y, _ float32) {
	f.tic.Colocar(x, y+(tema.TextoSM.Linea-14)/2, 14)
	x += 14 + HuecoCasillas
	ui.ColocarTexto(f.correo, tema.TextoXS, x, y+(tema.TextoSM.Linea-tema.TextoXS.Linea)/2)
	x += ui.Medir(f.correo.Text, tema.TextoXS, ui.Normal).Width + HuecoCasillas
	ui.ColocarTexto(f.rol, tema.TextoXS, x, y+(tema.TextoSM.Linea-tema.TextoXS.Linea)/2)
}

func (f *filaPendiente) objetos() []fyne.CanvasObject {
	return []fyne.CanvasObject{f.tic.Objeto(), f.correo, f.rol}
}

// nuevaFicha es un permiso: `bg-bg-subtle rounded px-1.5 py-0.5 font-mono text-xs`.
func nuevaFicha(texto string) fyne.CanvasObject {
	i := ui.NuevaInsignia(texto, ui.InsigniaNeutra)
	i.Cuadrada = true
	return i
}

// ---- El resumen del último paso, con «Editar» ----

type filaResumenAlta struct {
	clave, valor string
	mono         bool
	editar       *ui.Enlace
}

// resumenAlta es `ReviewSummary` con su «Editar» a la derecha de cada fila que lo lleva.
type resumenAlta struct {
	*resumen
	editar []*ui.Enlace
}

func nuevoResumenAlta(ventana float32, filas []filaResumenAlta) *resumenAlta {
	pares := make([][2]string, len(filas))
	r := &resumenAlta{}
	for i, f := range filas {
		pares[i] = [2]string{f.clave, f.valor}
		r.editar = append(r.editar, f.editar)
	}
	r.resumen = nuevoResumen(ventana, pares)
	for i, f := range filas {
		if f.mono {
			r.valores[i].Escalon = tema.TextoXS
		}
		// La tercera columna (`auto`) mide lo que el «Editar» más ancho.
		if f.editar != nil {
			r.derecha = max(r.derecha, ui.MedidaEnlace(f.editar).Width+HuecoResumen)
		}
	}
	return r
}

func (r *resumenAlta) poner(x, y, ancho float32) {
	r.resumen.poner(x, y, ancho)
	fy := y + tema.Borde
	for i, e := range r.editar {
		if i > 0 {
			fy += tema.Borde
		}
		if e != nil {
			m := ui.MedidaEnlace(e)
			ponerObjeto(e, x+ancho-tema.Borde-MargenResumenX-m.Width, fy+MargenResumenY, m, true)
		}
		fy += r.altoFila(i, ancho)
	}
}

func (r *resumenAlta) objetos() []fyne.CanvasObject {
	out := r.resumen.objetos()
	for _, e := range r.editar {
		if e != nil {
			out = append(out, e)
		}
	}
	return out
}

// enlaceSiguiente es `NextLink`: una fila pulsable con su icono, título y frase.
type enlaceSiguiente struct {
	ui.Pulsable
	icono         iconos.Icono
	titulo, texto string
}

var _ fyne.Widget = (*enlaceSiguiente)(nil)

func nuevoEnlaceSiguiente(i iconos.Icono, titulo, texto string, alPulsar func()) *enlaceSiguiente {
	e := &enlaceSiguiente{icono: i, titulo: titulo, texto: texto}
	e.Nombre, e.AlPulsar = titulo, alPulsar
	e.ExtendBaseWidget(e)
	return e
}

// Medidas del `NextLink`: `p-3 gap-3 rounded-md border`, icono de 18 con `mt-0.5`.
const (
	rellenoSiguiente float32 = 3 * 0.25 * tema.Rem
	iconoSiguiente   float32 = 18
)

func (e *enlaceSiguiente) dentro(ancho float32) float32 {
	return max(ancho-2*(tema.Borde+rellenoSiguiente)-iconoSiguiente-HuecoFilaAlta, 0)
}

func (e *enlaceSiguiente) alto(ancho float32) float32 {
	d := e.dentro(ancho)
	return 2*(tema.Borde+rellenoSiguiente) + tema.TextoSM.Linea + MargenDescEleccion +
		float32(len(ui.Repartir(e.texto, smAjustada, ui.Normal, d)))*smAjustada.Linea
}

func (e *enlaceSiguiente) poner(x, y, ancho float32) {
	ponerObjeto(e, x, y, fyne.NewSize(ancho, e.alto(ancho)), true)
}

func (e *enlaceSiguiente) objetos() []fyne.CanvasObject { return []fyne.CanvasObject{e} }

func (e *enlaceSiguiente) CreateRenderer() fyne.WidgetRenderer {
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	r := &enlaceSiguienteRenderer{
		e:      e,
		aro:    ui.NuevoAro(tema.RadioMD, ui.SinHueco, v),
		caja:   ui.NuevaCaja(tema.Color("bg", v), tema.Color("border-subtle", v), tema.RadioMD),
		icono:  iconos.Trazo(e.icono, iconoSiguiente, "text-muted", v),
		titulo: ui.Texto(e.titulo, tema.TextoSM, ui.Medio, tema.Color("text", v)),
		texto:  ui.NuevoParrafo(e.texto, smAjustada, ui.Normal, "text-muted", v),
	}
	r.objetos = append(r.aro.Objetos(), r.caja.Objetos()...)
	r.objetos = append(r.objetos, r.icono.Objeto(), r.titulo, r.texto)
	r.Refresh()
	return r
}

type enlaceSiguienteRenderer struct {
	e       *enlaceSiguiente
	aro     *ui.Aro
	caja    *ui.Caja
	icono   *iconos.Dibujo
	titulo  *canvas.Text
	texto   *ui.Parrafo
	objetos []fyne.CanvasObject
}

func (r *enlaceSiguienteRenderer) MinSize() fyne.Size {
	return fyne.NewSize(0, r.e.alto(r.e.Size().Width))
}
func (r *enlaceSiguienteRenderer) Objects() []fyne.CanvasObject { return r.objetos }
func (r *enlaceSiguienteRenderer) Destroy()                     {}

func (r *enlaceSiguienteRenderer) Layout(t fyne.Size) {
	r.caja.Redimensionar(t)
	r.aro.Colocar(t)
	x, y := tema.Borde+rellenoSiguiente, tema.Borde+rellenoSiguiente
	r.icono.Colocar(x, y+DesplazaIconoAlta, iconoSiguiente)
	x += iconoSiguiente + HuecoFilaAlta
	ui.ColocarTexto(r.titulo, tema.TextoSM, x, y)
	ponerParrafo(r.texto, x, y+tema.TextoSM.Linea+MargenDescEleccion, r.e.dentro(t.Width), true)
}

// DesplazaIconoAlta es el `mt-0.5` del icono.
const DesplazaIconoAlta float32 = 0.5 * 0.25 * tema.Rem

// MargenDescEleccion es el `mt-0.5` de la frase del `NextLink`.
const MargenDescEleccion float32 = 0.5 * 0.25 * tema.Rem

// Refresh pinta el `hover:border-border-strong` y el anillo del foco.
func (r *enlaceSiguienteRenderer) Refresh() {
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	borde := "border-subtle"
	if r.e.Encima() {
		borde = "border-strong"
	}
	r.caja.Pintar(tema.Color("bg", v), tema.Color(borde, v))
	r.aro.Pintar(v)
	r.aro.Ver(r.e.Enfocado())
	r.Layout(r.e.Size())
}
