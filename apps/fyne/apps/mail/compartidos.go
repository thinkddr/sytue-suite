// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package mail

import (
	"context"
	"errors"
	"image/color"
	"slices"
	"strings"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"

	"git.sytue.eu/sytue/one/apps/fyne/iconos"
	"git.sytue.eu/sytue/one/apps/fyne/peticion"
	"git.sytue.eu/sytue/one/apps/fyne/tema"
	"git.sytue.eu/sytue/one/apps/fyne/ui"
	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/i18n"
)

// # Buzones compartidos y delegación
//
// La sección «Compartidos» del lateral (los buzones ajenos a los que se tiene acceso:
// uno de equipo, o el de alguien que te lo delegó) abre su Recibidos EN SOLO LECTURA,
// como la web: sin triaje ni marcar leído, y responder o redactar salen desde su
// dirección si hay permiso de envío. Y el diálogo de `SharedMailboxesModal`: delegar
// el buzón propio, crear uno compartido y gestionar quién entra en los que gestionas.

// GestorCompartidos es lo que todo esto le pide al servidor. Lo cumple *api.Mail.
type GestorCompartidos interface {
	SharedMailboxes(ctx context.Context) ([]api.MailShared, error)
	CreateSharedMailbox(ctx context.Context, direccion, nombre string) (api.MailShared, error)
	SharedFolders(ctx context.Context, cuenta string) ([]api.Mailbox, error)
	SharedMessages(ctx context.Context, cuenta, carpeta, cursor string, limite int) ([]api.MailMessage, string, error)
	Grants(ctx context.Context, cuenta string) ([]api.MailGrant, error)
	Grant(ctx context.Context, cuenta, quien, permiso string) error
	Revoke(ctx context.Context, cuenta, quien string) error
	Delegations(ctx context.Context) ([]api.MailGrant, error)
	Delegate(ctx context.Context, quien, permiso string) error
	Undelegate(ctx context.Context, quien string) error
}

var _ GestorCompartidos = (*api.Mail)(nil)

// Compartido es una fila de la sección «Compartidos».
type Compartido struct {
	ID, Direccion string
	// Delegado es el buzón de una persona (icono de persona); si no, de equipo.
	Delegado bool
}

// ---- La vista de un buzón compartido ----

// cargarCompartidos es la lista del lateral. Best-effort, como en la web: sin ella,
// no hay sección.
func (b *Buzon) cargarCompartidos() {
	pedir(b, b.S.SharedMailboxes, func(xs []api.MailShared, err error) {
		if err != nil {
			return
		}
		b.compartidos = xs
		b.P.Menu.Compartidos = b.P.Menu.Compartidos[:0]
		for _, x := range xs {
			b.P.Menu.Compartidos = append(b.P.Menu.Compartidos, Compartido{ID: x.AccountID, Direccion: x.Address, Delegado: x.Kind != "shared"})
		}
		b.P.Refrescar()
		// Tras una concesión, o llegando desde el aviso, se vuelve a abrir ese buzón.
		if id := b.reabrir; id != "" {
			b.reabrir = ""
			b.ElegirCompartido(Compartido{ID: id})
		}
	})
}

// ElegirCompartido es `selectShared`: su Recibidos, en solo lectura.
func (b *Buzon) ElegirCompartido(c Compartido) {
	i := slices.IndexFunc(b.compartidos, func(x api.MailShared) bool { return x.AccountID == c.ID })
	if i < 0 {
		return
	}
	b.P.CerrarCajon()
	x := b.compartidos[i]
	b.compartido, b.carpetaComp = &x, ""
	b.carpetaID, b.etiquetaID = "", ""
	b.P.Menu.CarpetaID, b.P.Menu.EtiquetaID, b.P.Menu.CompartidoID = "", "", c.ID
	b.P.Bandeja.Titulo = x.Address
	b.soloLectura(true)
	b.engancharPeticion(x)
	b.limpiarBusqueda()
	b.cargarLista()
}

// dejarCompartido es `leaveShared`: se vuelve a la vista propia.
func (b *Buzon) dejarCompartido() {
	if b.compartido == nil {
		return
	}
	b.compartido, b.carpetaComp = nil, ""
	b.P.Menu.CompartidoID = ""
	b.soloLectura(false)
	b.peticion.Soltar()
	b.peticion = peticion.Estado{}
	b.P.Franja = nil
}

// conPeticiones es lo que el servicio ofrece para «pedir acceso». Lo cumple *api.Mail;
// un servicio de prueba que no lo tenga deja la franja sin botón.
type conPeticiones interface {
	PeticionesBuzon(cuenta string) api.Peticiones
}

// engancharPeticion pone la franja del buzón compartido (`FranjaPeticion` de la web):
// en "read", «Pedir acceso» a quien lo gestiona; con "manage" (o llegando desde el
// aviso), quién lo pide, con «Dar acceso». Al concederse, el aviso del Hub vuelve a
// leer los compartidos y el buzón se abre con su permiso nuevo.
func (b *Buzon) engancharPeticion(x api.MailShared) {
	g, ok := b.S.(conPeticiones)
	if !ok {
		b.P.Franja = nil
		return
	}
	pedida := x.EditRequested
	t := func(k string, v i18n.Vars) string { return b.P.T.T(k, v) }
	peticion.Enganchar(&b.peticion, g.PeticionesBuzon(x.AccountID), peticion.Opciones{
		SinPermiso:    x.Permission == "read",
		PuedeConceder: x.Permission == "manage" || x.AccountID == b.buzonDelAviso,
		YaPedida:      func(context.Context) (bool, error) { return pedida, nil },
		Recomponer:    func() { b.ponerFranja(); b.P.Refrescar() },
		T:             t,
		Concedido:     "mail.access.granted", Recurso: x.AccountID,
		AlConceder: func() { b.reabrir = x.AccountID; b.cargarCompartidos() },
	})
	b.ponerFranja()
}

// ponerFranja deja en la pantalla la franja del buzón compartido abierto, o ninguna.
func (b *Buzon) ponerFranja() {
	if b.compartido == nil {
		b.P.Franja = nil
		return
	}
	t := func(k string, v i18n.Vars) string { return b.P.T.T(k, v) }
	b.P.Franja = nil // un *FranjaSoloLectura nil en la interfaz no sería nil
	if f := peticion.Franja(&b.peticion, b.compartido.Permission == "read",
		t("readOnly.sharedWithYou", i18n.Vars{"name": b.compartido.Address}), t); f != nil {
		b.P.Franja = f
	}
}

// AtenderBuzon abre, desde el aviso de una petición, el buzón que se gestiona: su
// franja enseña quién pide acceso. Si la lista aún no ha llegado, se abre al llegar.
func (b *Buzon) AtenderBuzon(cuenta string) {
	b.buzonDelAviso, b.reabrir = cuenta, cuenta
	b.cargarCompartidos()
}

// soloLectura es el `readOnly` de la web para el buzón compartido: la lista sin
// casillas ni triaje y el lector sin las acciones que cambian nada.
func (b *Buzon) soloLectura(si bool) {
	b.P.Bandeja.SoloLectura, b.P.Lector.SoloLectura = si, si
	b.ponerAcciones()
}

// deCompartido es la dirección «De» en un buzón compartido con permiso de envío.
func (b *Buzon) deCompartido() string {
	if b.compartido == nil || b.compartido.Permission == "read" {
		return ""
	}
	return b.compartido.Address
}

// paginaCompartida es una página del Recibidos del buzón compartido. La primera
// busca antes su Recibidos (o la primera carpeta, si no tiene).
func (b *Buzon) paginaCompartida(ctx context.Context, cuenta, carpeta, cursor string) (pagina, error) {
	var p pagina
	if carpeta == "" {
		cs, err := b.S.SharedFolders(ctx, cuenta)
		if err != nil {
			return p, err
		}
		i := slices.IndexFunc(cs, func(m api.Mailbox) bool { return m.Role == "inbox" })
		if i < 0 && len(cs) > 0 {
			i = 0
		}
		if i < 0 {
			return p, errSinCarpetas
		}
		carpeta = cs[i].ID
	}
	p.carpeta = carpeta
	var err error
	p.mensajes, p.siguiente, err = b.S.SharedMessages(ctx, cuenta, carpeta, cursor, PorPagina)
	return p, err
}

var errSinCarpetas = errors.New("mail: el buzón compartido no tiene carpetas")

// ---- «Buzones compartidos y delegación» (`SharedMailboxesModal.tsx`) ----

// Permiso es una opción de «Qué podrá hacer»: su valor, su tarjeta y su icono.
type permiso struct {
	valor, titulo, descripcion string
	icono                      iconos.Icono
}

var (
	permisosDelegar = []permiso{
		{"read", "shared.delegateReadTitle", "shared.delegateReadDescription", iconos.Usuario},
		{"send", "shared.delegateSendTitle", "shared.delegateSendDescription", iconos.Usuarios},
	}
	permisosBuzon = []permiso{
		{"read", "shared.mailboxReadTitle", "shared.mailboxReadDescription", iconos.Usuario},
		{"send", "shared.mailboxSendTitle", "shared.mailboxSendDescription", iconos.Usuarios},
		{"manage", "shared.mailboxManageTitle", "shared.mailboxManageDescription", iconos.Candado},
	}
)

// RotuloPermiso es `permLabel`: enviar es «en mi nombre» en una delegación y «como»
// en un buzón compartido.
func RotuloPermiso(t *i18n.Translator, p string, delegado bool) string {
	switch {
	case p == "read":
		return t.T("shared.permRead", nil)
	case p == "manage":
		return t.T("shared.permManage", nil)
	case delegado:
		return t.T("shared.permSendOnBehalf", nil)
	}
	return t.T("shared.permSendAs", nil)
}

type dialogoCompartidos struct {
	b       *Buzon
	d       *ui.Dialogo
	caja    *fyne.Container
	ancho   float32
	ventana float32
	estado  Estado

	compartidos []api.MailShared
	delegados   []api.MailGrant

	// La delegación del buzón propio.
	persona   *ui.Entrada
	permDel   string
	delegando bool
	// Crear un buzón compartido.
	direccion, nombre *ui.Entrada
	creando           bool
	// El acceso a uno que gestiono: nil es la vista principal.
	editando   *api.MailShared
	accesos    []api.MailGrant
	estadoACL  Estado
	personaACL *ui.Entrada
	permACL    string
	dando      bool
}

// verCompartidos abre «Buzones compartidos y delegación».
func (b *Buzon) verCompartidos() *dialogoCompartidos {
	lienzo := b.P.Lienzo()
	if lienzo == nil {
		return nil
	}
	t := b.P.T
	g := &dialogoCompartidos{
		b: b, ancho: AnchoCuerpoDialogo(ui.DialogoLG, lienzo.Size().Width), ventana: lienzo.Size().Width,
		permDel: "read", permACL: "read",
	}
	entrada := func(marca string, alIntro func()) *ui.Entrada {
		e := ui.NuevaEntrada(marca)
		e.AlCambiar(func(string) { g.pintar() })
		if alIntro != nil {
			e.AlTeclear = func(k *fyne.KeyEvent) bool {
				if k.Name == fyne.KeyReturn || k.Name == fyne.KeyEnter {
					alIntro()
					return true
				}
				return false
			}
		}
		return e
	}
	g.persona = entrada(t.T("shared.personPlaceholder", nil), g.delegar)
	g.personaACL = entrada(t.T("shared.aclPlaceholder", nil), g.darAcceso)
	g.direccion = entrada(t.T("shared.addressPlaceholder", nil), nil)
	g.nombre = entrada(t.T("shared.namePlaceholder", nil), nil)
	g.caja = container.New(ui.Columna{})
	cerrar := ui.NuevoBoton(t.T("common.close", nil), nil).Con(ui.Secundario, ui.TallaMD)
	g.d = ui.NuevoDialogo(t.T("shared.title", nil), g.caja).Con(ui.DialogoLG, cerrar)
	cerrar.AlPulsar = g.d.Cerrar
	g.d.RotuloCerrar = t.T("ui.close", nil)
	g.d.Mostrar(lienzo)
	g.cargar()
	return g
}

func (g *dialogoCompartidos) cargar() {
	g.estado, g.editando = Cargando, nil
	g.pintar()
	pedir(g.b, func(ctx context.Context) (listaCompartidos, error) {
		xs, err := g.b.S.SharedMailboxes(ctx)
		if err != nil {
			return listaCompartidos{}, err
		}
		ds, err := g.b.S.Delegations(ctx)
		return listaCompartidos{xs, ds}, err
	}, func(l listaCompartidos, err error) {
		if err != nil {
			g.estado = Error
		} else {
			g.compartidos, g.delegados, g.estado = l.compartidos, l.delegados, Listo
		}
		g.pintar()
	})
}

type listaCompartidos struct {
	compartidos []api.MailShared
	delegados   []api.MailGrant
}

// cambiado es `onChanged`: el lateral y el «De» del redactor se enteran.
func (g *dialogoCompartidos) cambiado() { g.b.cargarCompartidos() }

func (g *dialogoCompartidos) pintar() {
	t := g.b.P.T
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	var dentro fyne.CanvasObject
	switch {
	case g.estado == Cargando:
		rueda := ui.NuevaRueda(16, "text-subtle", v)
		rueda.Rotulo = t.T("common.loading", nil)
		dentro = container.New(ui.Margen{Arr: tema.Espacio(8), Abj: tema.Espacio(8)},
			container.New(ui.Fila{Hueco: tema.Espacio(2)}, rueda,
				ui.NuevoRotulo(t.T("common.loading", nil), tema.TextoSM, ui.Normal, "text-subtle")))
	case g.estado == Error:
		dentro = errorConReintento(t.T("shared.loadError", nil), t.T("common.retry", nil), g.cargar, tema.Espacio(4), v)
	case g.editando != nil:
		dentro = g.vistaACL(v)
	default:
		dentro = container.New(ui.Columna{Hueco: tema.Espacio(8)}, g.seccionDelegar(v), g.seccionGestiono(v))
	}
	g.caja.Objects = []fyne.CanvasObject{dentro}
	g.caja.Refresh()
	if g.d != nil {
		g.d.Recolocar()
	}
}

// errorConReintento es el `role="alert"` de la web: el icono de aviso, la frase y
// «Reintentar» pequeño debajo, con `py-*` alrededor.
func errorConReintento(texto, reintentar string, alReintentar func(), py float32, v tema.Variante) fyne.CanvasObject {
	icono := iconos.Trazo(iconos.Aviso, 16, "text", v)
	return container.New(ui.Margen{Arr: py, Abj: py},
		container.New(ui.Columna{Hueco: tema.Espacio(2)},
			container.New(ui.Fila{Hueco: tema.Espacio(2)}, container.New(ui.Margen{Arr: tema.Espacio(0.5)}, icono.Objeto()),
				ui.NuevoRotulo(texto, tema.TextoSM, ui.Normal, "text")),
			container.New(ui.Fila{}, ui.NuevoBoton(reintentar, alReintentar).Con(ui.Secundario, ui.TallaSM))))
}

// pareceDireccion es el `EMAIL_RE` del diálogo.
func pareceDireccion(s string) bool { return pareceCorreo.MatchString(strings.TrimSpace(s)) }

// eleccionPermiso es `PermissionChoice`: «Qué podrá hacer» y una tarjeta densa por
// permiso, con su consecuencia a la vista.
func (g *dialogoCompartidos) eleccionPermiso(opciones []permiso, elegido string, elegir func(string)) fyne.CanvasObject {
	t := g.b.P.T
	objs := make([]fyne.CanvasObject, 0, 2)
	objs = append(objs, ui.NuevoRotulo(t.T("shared.permGroup", nil), tema.TextoSM, ui.SemiNegrita, "text"))
	tarjetas := make([]fyne.CanvasObject, 0, len(opciones))
	for _, o := range opciones {
		c := ui.NuevaTarjetaEleccion(t.T(o.titulo, nil), t.T(o.descripcion, nil), func() { elegir(o.valor); g.pintar() })
		c.Icono, c.Elegida, c.Densa, c.Ancho = o.icono, o.valor == elegido, true, g.ancho
		tarjetas = append(tarjetas, c)
	}
	// El `legend` con su `mb-1` y las tarjetas a `gap-2`.
	return container.New(ui.Columna{Hueco: tema.Espacio(1)}, append(objs, container.New(ui.Columna{Hueco: tema.Espacio(2)}, tarjetas...))...)
}

// campoConPista es un FieldGroup con su pista (neutra, o de error si `mal`).
func (g *dialogoCompartidos) campoConPista(rotulo string, control *ui.Entrada, pista, fallo string, mal bool, ancho float32) *fyne.Container {
	c := ui.GrupoCampo(rotulo, false, control, "", "")
	p := ui.NuevaPistaCampo(pista)
	if mal {
		p = ui.NuevaPistaCampo(fallo).ConTono(ui.AyudaPeligro)
	}
	p.Ancho = ancho
	control.Invalida = mal
	control.Refresh()
	c.Add(p)
	return c
}

// botonDerecha es `flex justify-end` con el primario de «+».
func botonDerecha(texto string, alPulsar func(), cargando, inerte bool) fyne.CanvasObject {
	b := ui.NuevoBoton(texto, alPulsar).Con(ui.Primario, ui.TallaMD)
	b.Icono, b.Cargando, b.Inerte = iconos.Mas, cargando, inerte
	return container.New(extremos{}, canvas.NewRectangle(color.NRGBA{}), b)
}

// filaAcceso es una persona con acceso: su dirección, el permiso en `sytue-meta` y el
// aspa de retirar.
func filaAcceso(g api.MailGrant, rotulo, quitar string, alQuitar func(), v tema.Variante) fyne.CanvasObject {
	x := ui.NuevoBoton(quitar, alQuitar).Con(ui.Fantasma, ui.TallaSM)
	x.SoloIcono, x.Icono, x.TamIcono = true, iconos.Cerrar, 16
	// `flex shrink-0 items-center gap-2`: el permiso centrado con el aspa.
	derecha := container.New(extremosIzq{hueco: tema.Espacio(2)}, ui.NuevoRotulo(rotulo, tema.TextoXS, ui.Normal, "text-subtle"), x)
	return filaAlias(textoRecortado(g.Grantee, tema.TextoSM, ui.Normal, "text", v), derecha)
}

// seccionDelegar es `DelegationSection`.
func (g *dialogoCompartidos) seccionDelegar(v tema.Variante) fyne.CanvasObject {
	t := g.b.P.T
	var lista fyne.CanvasObject
	if len(g.delegados) > 0 {
		filas := make([]fyne.CanvasObject, 0, len(g.delegados))
		for _, d := range g.delegados {
			filas = append(filas, filaAcceso(d, RotuloPermiso(t, d.Permission, true),
				t.T("shared.revokeDelegate", i18n.Vars{"person": d.Grantee}), func() { g.retirarDelegacion(d) }, v))
		}
		lista = enCajaConFiletes(filas)
	} else {
		vacio := ui.NuevoVacio(t.T("shared.delegateEmptyTitle", nil), t.T("shared.delegateEmptyDescription", nil))
		vacio.Ilustracion = iconos.Trazo(iconos.Candado, 32, "text-subtle", v).Objeto()
		lista = vacio
	}
	escrito := strings.TrimSpace(g.persona.Texto())
	mal := escrito != "" && !pareceDireccion(escrito)
	form := container.New(ui.Columna{Hueco: tema.Espacio(3)},
		g.campoConPista(t.T("shared.person", nil), g.persona, t.T("shared.personHint", nil), t.T("shared.personHintError", nil), mal, g.ancho),
		g.eleccionPermiso(permisosDelegar, g.permDel, func(p string) { g.permDel = p }),
		botonDerecha(t.T("shared.grantAccess", nil), g.delegar, g.delegando, !pareceDireccion(escrito)))
	return seccion(t.T("shared.delegateTitle", nil), t.T("shared.delegateDescription", nil), g.ancho, lista, form)
}

// delegar es `handleAdd` de la delegación: la lista se pone al día sin recargar.
func (g *dialogoCompartidos) delegar() {
	quien, perm := strings.TrimSpace(g.persona.Texto()), g.permDel
	if !pareceDireccion(quien) || g.delegando {
		return
	}
	t := g.b.P.T
	g.delegando = true
	g.pintar()
	pedir(g.b, func(ctx context.Context) (struct{}, error) {
		return struct{}{}, g.b.S.Delegate(ctx, quien, perm)
	}, func(_ struct{}, err error) {
		g.delegando = false
		if err != nil {
			g.b.P.Avisar(ui.AvisoPeligro, t.T("shared.delegateFailed", nil), t.T("shared.delegateFailedDescription", nil))
			g.pintar()
			return
		}
		g.delegados = append(slices.DeleteFunc(g.delegados, func(x api.MailGrant) bool { return x.Grantee == quien }),
			api.MailGrant{Grantee: quien, Permission: perm})
		g.persona.PonerTexto("")
		g.b.avisar(ui.AvisoExito, "shared.delegated", nil)
		g.cambiado()
		g.pintar()
	})
}

// retirarDelegacion es `handleRemove`: optimista, y de vuelta si falla.
func (g *dialogoCompartidos) retirarDelegacion(d api.MailGrant) {
	antes := slices.Clone(g.delegados)
	g.delegados = slices.DeleteFunc(g.delegados, func(x api.MailGrant) bool { return x.Grantee == d.Grantee })
	g.pintar()
	pedir(g.b, func(ctx context.Context) (struct{}, error) {
		return struct{}{}, g.b.S.Undelegate(ctx, d.Grantee)
	}, func(_ struct{}, err error) {
		if err != nil {
			g.delegados = antes
			g.b.avisar(ui.AvisoPeligro, "shared.revokeDelegateFailed", nil)
			g.pintar()
			return
		}
		g.cambiado()
	})
}

// gestionados son los compartidos (de equipo) que gestiono.
func (g *dialogoCompartidos) gestionados() []api.MailShared {
	var out []api.MailShared
	for _, x := range g.compartidos {
		if x.Kind == "shared" && x.Permission == "manage" {
			out = append(out, x)
		}
	}
	return out
}

// seccionGestiono es `SharedSection`, con su `border-t pt-6`.
func (g *dialogoCompartidos) seccionGestiono(v tema.Variante) fyne.CanvasObject {
	t := g.b.P.T
	var lista fyne.CanvasObject
	if gs := g.gestionados(); len(gs) > 0 {
		filas := make([]fyne.CanvasObject, 0, len(gs))
		for _, x := range gs {
			gestionar := ui.NuevoBoton(t.T("shared.manageAccess", nil), func() { g.abrirACL(x) }).Con(ui.Secundario, ui.TallaSM)
			filas = append(filas, filaAlias(textoRecortado(x.Address, tema.TextoSM, ui.Medio, "text", v), gestionar))
		}
		lista = enCajaConFiletes(filas)
	} else {
		vacio := ui.NuevoVacio(t.T("shared.managedEmptyTitle", nil), t.T("shared.managedEmptyDescription", nil))
		vacio.Ilustracion = iconos.Trazo(iconos.Usuarios, 32, "text-subtle", v).Objeto()
		lista = vacio
	}
	dir := strings.TrimSpace(g.direccion.Texto())
	mal := dir != "" && !pareceDireccion(dir)
	// `grid gap-3 sm:grid-cols-2`: dirección y nombre lado a lado desde `sm:`.
	rejilla := ui.Rejilla{Fijas: 2, Hueco: tema.Espacio(3)}
	anchoCampo := g.ancho
	if g.ventana >= ui.CorteSM {
		anchoCampo = rejilla.AnchoColumna(g.ancho)
	}
	campos := []fyne.CanvasObject{
		g.campoConPista(t.T("common.address", nil), g.direccion, t.T("shared.addressHint", nil), t.T("shared.addressHintError", nil), mal, anchoCampo),
		g.campoConPista(t.T("common.name", nil), g.nombre, t.T("shared.nameHint", nil), "", false, anchoCampo),
	}
	var fila fyne.CanvasObject = container.New(ui.Columna{Hueco: tema.Espacio(3)}, campos...)
	if g.ventana >= ui.CorteSM {
		fila = container.New(rejilla, campos...)
	}
	form := container.New(ui.Columna{Hueco: tema.Espacio(3)}, fila,
		botonDerecha(t.T("shared.createMailbox", nil), g.crear, g.creando, !pareceDireccion(dir)))
	return container.New(ui.Columna{Hueco: tema.Espacio(6)}, canvasFilete(v),
		seccion(t.T("shared.managedTitle", nil), t.T("shared.managedDescription", nil), g.ancho, lista, form))
}

// crear es `handleCreate`: con el buzón creado se recarga todo (y el lateral).
func (g *dialogoCompartidos) crear() {
	dir, nombre := strings.TrimSpace(g.direccion.Texto()), strings.TrimSpace(g.nombre.Texto())
	if !pareceDireccion(dir) || g.creando {
		return
	}
	t := g.b.P.T
	g.creando = true
	g.pintar()
	pedir(g.b, func(ctx context.Context) (api.MailShared, error) {
		return g.b.S.CreateSharedMailbox(ctx, dir, nombre)
	}, func(_ api.MailShared, err error) {
		g.creando = false
		if err != nil {
			g.b.P.Avisar(ui.AvisoPeligro, t.T("shared.createFailed", nil), t.T("shared.createFailedDescription", nil))
			g.pintar()
			return
		}
		g.direccion.PonerTexto("")
		g.nombre.PonerTexto("")
		g.b.avisar(ui.AvisoExito, "shared.created", nil)
		g.cambiado()
		g.cargar()
	})
}

// ---- El acceso a un buzón que gestiono (`AclEditor`) ----

func (g *dialogoCompartidos) abrirACL(x api.MailShared) {
	g.editando, g.accesos, g.permACL = &x, nil, "read"
	g.personaACL.PonerTexto("")
	g.cargarACL()
}

func (g *dialogoCompartidos) cargarACL() {
	if g.editando == nil {
		return
	}
	cuenta := g.editando.AccountID
	g.estadoACL = Cargando
	g.pintar()
	pedir(g.b, func(ctx context.Context) ([]api.MailGrant, error) {
		return g.b.S.Grants(ctx, cuenta)
	}, func(gs []api.MailGrant, err error) {
		if g.editando == nil || g.editando.AccountID != cuenta {
			return
		}
		if err != nil {
			g.estadoACL = Error
		} else {
			g.accesos, g.estadoACL = gs, Listo
		}
		g.pintar()
	})
}

func (g *dialogoCompartidos) vistaACL(v tema.Variante) fyne.CanvasObject {
	t := g.b.P.T
	// La cabecera: el «‹» fantasma pequeño y, al lado, «Acceso al buzón» en versalita
	// con la dirección debajo.
	atras := ui.NuevoBoton(t.T("common.back", nil), func() { g.editando = nil; g.pintar() }).Con(ui.Fantasma, ui.TallaSM)
	atras.SoloIcono, atras.Icono, atras.TamIcono = true, iconos.ChevronIzquierda, 18
	cabeza := container.New(extremosIzq{hueco: tema.Espacio(2)}, atras,
		container.New(ui.Columna{},
			ui.NuevoVersal(t.T("shared.aclHeading", nil), tema.TextoXS, ui.SemiNegrita, tema.Color("text-subtle", v)).Objeto(),
			textoRecortado(g.editando.Address, tema.TextoSM, ui.Medio, "text", v)))

	var lista fyne.CanvasObject
	switch {
	case g.estadoACL == Cargando:
		rueda := ui.NuevaRueda(16, "text-subtle", v)
		rueda.Rotulo = t.T("shared.aclLoading", nil)
		lista = container.New(ui.Margen{Arr: tema.Espacio(4), Abj: tema.Espacio(4)},
			container.New(ui.Fila{Hueco: tema.Espacio(2)}, rueda,
				ui.NuevoRotulo(t.T("common.loading", nil), tema.TextoSM, ui.Normal, "text-subtle")))
	case g.estadoACL == Error:
		lista = errorConReintento(t.T("shared.aclLoadError", nil), t.T("common.retry", nil), g.cargarACL, tema.Espacio(2), v)
	case len(g.accesos) == 0:
		vacio := ui.NuevoVacio(t.T("shared.aclEmptyTitle", nil), t.T("shared.aclEmptyDescription", nil))
		vacio.Ilustracion = iconos.Trazo(iconos.Usuarios, 32, "text-subtle", v).Objeto()
		lista = vacio
	default:
		filas := make([]fyne.CanvasObject, 0, len(g.accesos))
		for _, a := range g.accesos {
			filas = append(filas, filaAcceso(a, RotuloPermiso(t, a.Permission, false),
				t.T("shared.revokeAccess", i18n.Vars{"person": a.Grantee}), func() { g.quitarAcceso(a) }, v))
		}
		lista = enCajaConFiletes(filas)
	}

	escrito := strings.TrimSpace(g.personaACL.Texto())
	mal := escrito != "" && !pareceDireccion(escrito)
	form := []fyne.CanvasObject{
		g.campoConPista(t.T("shared.addPerson", nil), g.personaACL, t.T("shared.personHint", nil), t.T("shared.aclHintError", nil), mal, g.ancho),
		g.eleccionPermiso(permisosBuzon, g.permACL, func(p string) { g.permACL = p }),
	}
	if g.permACL == "manage" {
		form = append(form, ui.NuevaAyudaEnLinea(t.T("shared.manageWarningBody", nil)).
			ConTono(ui.AyudaAviso).ConTitulo(t.T("shared.manageWarningTitle", nil)))
	}
	form = append(form, botonDerecha(t.T("shared.grantAccess", nil), g.darAcceso, g.dando, !pareceDireccion(escrito)))
	// `border-t pt-4`.
	pie := container.New(ui.Columna{Hueco: tema.Espacio(4)}, canvasFilete(v), container.New(ui.Columna{Hueco: tema.Espacio(3)}, form...))
	return container.New(ui.Columna{Hueco: tema.Espacio(4)}, cabeza, lista, pie)
}

// extremosIzq es `flex items-center gap-2`: el primero a su medida, el segundo con el
// resto, centrados en vertical.
type extremosIzq struct{ hueco float32 }

func (e extremosIzq) MinSize(objs []fyne.CanvasObject) fyne.Size {
	a, b := objs[0].MinSize(), objs[1].MinSize()
	return fyne.NewSize(a.Width+e.hueco+b.Width, max(a.Height, b.Height))
}

func (e extremosIzq) Layout(objs []fyne.CanvasObject, t fyne.Size) {
	a := objs[0].MinSize()
	objs[0].Move(fyne.NewPos(0, (t.Height-a.Height)/2))
	objs[0].Resize(a)
	b := objs[1].MinSize()
	x := a.Width + e.hueco
	objs[1].Move(fyne.NewPos(x, (t.Height-b.Height)/2))
	objs[1].Resize(fyne.NewSize(max(t.Width-x, 0), b.Height))
}

// darAcceso es `handleAdd` del editor: con el acceso dado se recarga la lista.
func (g *dialogoCompartidos) darAcceso() {
	if g.editando == nil {
		return
	}
	quien, perm, cuenta := strings.TrimSpace(g.personaACL.Texto()), g.permACL, g.editando.AccountID
	if !pareceDireccion(quien) || g.dando {
		return
	}
	t := g.b.P.T
	g.dando = true
	g.pintar()
	pedir(g.b, func(ctx context.Context) (struct{}, error) {
		return struct{}{}, g.b.S.Grant(ctx, cuenta, quien, perm)
	}, func(_ struct{}, err error) {
		g.dando = false
		if err != nil {
			g.b.P.Avisar(ui.AvisoPeligro, t.T("shared.grantFailed", nil), t.T("shared.grantFailedDescription", nil))
			g.pintar()
			return
		}
		g.personaACL.PonerTexto("")
		g.b.avisar(ui.AvisoExito, "shared.accessGranted", nil)
		g.cambiado()
		g.cargarACL()
	})
}

// quitarAcceso es `handleRemove` del editor: optimista, y de vuelta si falla.
func (g *dialogoCompartidos) quitarAcceso(a api.MailGrant) {
	if g.editando == nil {
		return
	}
	cuenta := g.editando.AccountID
	antes := slices.Clone(g.accesos)
	g.accesos = slices.DeleteFunc(g.accesos, func(x api.MailGrant) bool { return x.Grantee == a.Grantee })
	g.pintar()
	pedir(g.b, func(ctx context.Context) (struct{}, error) {
		return struct{}{}, g.b.S.Revoke(ctx, cuenta, a.Grantee)
	}, func(_ struct{}, err error) {
		if err != nil {
			g.accesos = antes
			g.b.avisar(ui.AvisoPeligro, "shared.revokeFailed", nil)
			g.pintar()
			return
		}
		g.cambiado()
	})
}
