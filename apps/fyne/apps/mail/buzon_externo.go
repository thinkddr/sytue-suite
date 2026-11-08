// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package mail

import (
	"context"
	"encoding/base64"
	"errors"
	"slices"
	"strings"
	"sync"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"

	"git.sytue.eu/sytue/one/apps/fyne/tema"
	"git.sytue.eu/sytue/one/apps/fyne/ui"
	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/eml"
	"git.sytue.eu/sytue/one/apps/nucleo/htmltext"
	"git.sytue.eu/sytue/one/apps/nucleo/i18n"
)

// # Una carpeta externa con la Bandeja y el Lector (fase B, puntos 4 y 5)
//
// Es la parte de `MailApp.tsx` que abre `selectedExternal`: la carpeta de una cuenta
// externa, o un buzón unificado («Todos los buzones de entrada», «Todos los
// enviados», «Todos los borradores»), toma la vista como un buzón compartido.
// La lista, el lector, el triaje y la paginación son los de siempre; lo que cambia es
// de dónde sale cada cosa —del proveedor, en vivo— y adónde va cada acción.

// RefrescoExterno es cada cuánto se vuelve a pedir la carpeta externa, o el buzón
// unificado, a la vista: el proveedor no avisa de lo que llega (`EXTERNAL_POLL_MS`).
// Solo la carpeta visible: con «Todos los enviados» abierto no se pide ninguna Entrada.
const RefrescoExterno = 30 * time.Second

// ElegirExterna abre la carpeta de una cuenta externa en el panel principal; sin
// carpeta, su Entrada, que se elige al cargar.
func (b *Buzon) ElegirExterna(cuenta, carpeta string) {
	b.abrirVistaExterna(VistaExterna{Cuenta: cuenta, Carpeta: carpeta})
}

// ElegirUnificado abre un buzón unificado («Todos los buzones de entrada», «Todos los
// enviados», «Todos los borradores»): la carpeta de ese rol de Sytue y la de cada
// cuenta, mezcladas por fecha. Un rol que no se mezcla (rolesMezclados) no abre nada.
func (b *Buzon) ElegirUnificado(rol string) {
	if esMezclado(rol) {
		b.abrirVistaExterna(VistaExterna{Unificado: rol})
	}
}

func (b *Buzon) abrirVistaExterna(v VistaExterna) {
	b.P.CerrarCajon()
	b.P.Bandeja.Filtro = FiltroTodas // otra vista empieza sin filtro
	b.dejarCompartido()
	b.externa = &v
	b.carpetaID, b.etiquetaID = "", ""
	b.P.Menu.CarpetaID, b.P.Menu.EtiquetaID, b.P.Menu.Externa = "", "", b.externa
	b.P.Bandeja.Titulo = b.tituloExterno()
	// Una cuenta POP3 no tiene acciones en el proveedor: se lee, no se tría.
	a, ok := b.cuentaExterna(v.Cuenta)
	b.soloLectura(!v.Mezclada() && ok && !a.Capabilities.Actions)
	b.limpiarBusqueda()
	b.cargarLista()
}

// dejarExterna vuelve a una vista de Sytue.
func (b *Buzon) dejarExterna() {
	if b.externa == nil {
		return
	}
	b.externa, b.P.Menu.Externa = nil, nil
	b.mezcla, b.mezclaCaida, b.errorExterno, b.masExternos = nil, false, "", false
	b.pararRefrescoExterno()
	b.P.Franja = nil
	b.soloLectura(false)
}

// cuentaExterna es la cuenta por su instancia: la del resumen, o la que el diálogo
// acaba de conectar y el resumen aún no trae.
func (b *Buzon) cuentaExterna(id string) (api.ExternalMailAccount, bool) {
	for _, c := range b.resumenExternas {
		if c.InstanceID == id {
			return c.ExternalMailAccount, true
		}
	}
	for _, a := range b.cuentasDialogo {
		if a.InstanceID == id {
			return a, true
		}
	}
	return api.ExternalMailAccount{}, false
}

// sinAcciones dice si un mensaje es de una cuenta sin acciones en su proveedor (POP3):
// se lee, no se tría. En un buzón unificado la vista no es de solo lectura, pero su
// Entrada va mezclada con las demás (`sinAccionesExternas` en la web).
func (b *Buzon) sinAcciones(id string) bool {
	o, ok := origenExterno(id)
	if !ok {
		return false
	}
	a, ok := b.cuentaExterna(o.Cuenta)
	return ok && !a.Capabilities.Actions
}

// carpetaExterna es una carpeta de una cuenta, del árbol del lateral.
func (b *Buzon) carpetaExterna(cuenta string, cumple func(api.ExternalMailFolder) bool) (api.ExternalMailFolder, bool) {
	for _, c := range b.P.Menu.Cuentas {
		if c.ID != cuenta {
			continue
		}
		if i := slices.IndexFunc(c.Carpetas, cumple); i >= 0 {
			return c.Carpetas[i], true
		}
	}
	return api.ExternalMailFolder{}, false
}

func (b *Buzon) carpetaExternaDeRol(cuenta, rol string) (api.ExternalMailFolder, bool) {
	return b.carpetaExterna(cuenta, func(f api.ExternalMailFolder) bool { return f.Role == rol })
}

// tituloExterno es el de la lista: «Recibidos · ana@gmail.com» —la carpeta sola no
// dice de qué cuenta es— o el del buzón unificado («Todos los enviados»).
func (b *Buzon) tituloExterno() string {
	v := b.externa
	if v == nil {
		return ""
	}
	if v.Mezclada() {
		return b.P.T.T(rotulosUnificados[v.Unificado], nil)
	}
	var partes []string
	if f, ok := b.carpetaExterna(v.Cuenta, func(f api.ExternalMailFolder) bool { return f.ID == v.Carpeta }); ok {
		partes = append(partes, RotuloCarpeta(b.P.T, f.Name, f.Role))
	}
	for _, c := range b.P.Menu.Cuentas {
		if c.ID == v.Cuenta && c.Titulo != "" {
			partes = append(partes, c.Titulo)
		}
	}
	return strings.Join(partes, " · ")
}

// fuentesDe son las carpetas del buzón unificado de la vista, por cuenta: solo las de
// su rol. Nil para la carpeta de una cuenta.
func (b *Buzon) fuentesDe(v VistaExterna) []FuenteDeBuzon {
	if !v.Mezclada() {
		return nil
	}
	return fuentesDeUnificado(b.P.Menu.Unificados, v.Unificado)
}

// direccionesDeCuentas son la dirección de cada cuenta, leídas en el hilo de la
// interfaz para las llamadas que corren fuera de él.
func (b *Buzon) direccionesDeCuentas() map[string]string {
	out := map[string]string{}
	for _, a := range b.cuentasDialogo {
		out[a.InstanceID] = a.Address
	}
	for _, c := range b.resumenExternas {
		out[c.InstanceID] = c.Address
	}
	return out
}

// paginaDeFuente es una página de una carpeta, de Sytue (cuentaSytue) o de una cuenta.
func (b *Buzon) paginaDeFuente(ctx context.Context, f FuenteDeBuzon, cursor, direccion string) (pagina, error) {
	if f.Cuenta == cuentaSytue {
		return b.pagina(ctx, f.Carpeta, "", cursor)
	}
	pg, err := b.CorreoExterno.MailMessages(ctx, f.Cuenta, f.Carpeta, cursor, direccion)
	if err != nil {
		return pagina{}, err
	}
	p := pagina{siguiente: pg.NextCursor}
	for _, m := range pg.Items {
		p.mensajes = append(p.mensajes, sobreExterno(f.Cuenta, f.Carpeta, m))
	}
	return p, nil
}

// primeraExterna es lo que trae la primera carga de una vista externa.
type primeraExterna struct {
	mensajes []api.MailMessage
	cursor   string
	mezcla   []fuenteMezcla
	// caidas dice que alguna carpeta del buzón unificado no contestó.
	caidas bool
	// entrada es la Entrada que elegir, para una cuenta abierta sin carpeta.
	entrada string
}

// primeraPaginaExterna corre FUERA del hilo de la interfaz: todo lo que lee del buzón
// llega en sus argumentos. Un buzón unificado pide la primera página de la carpeta de
// su rol de cada cuenta a la vez y las mezcla; una que no contesta no tumba las demás.
func (b *Buzon) primeraPaginaExterna(ctx context.Context, v VistaExterna, fuentes []FuenteDeBuzon, dirs map[string]string) (primeraExterna, error) {
	if !v.Mezclada() {
		if v.Carpeta == "" {
			cs, err := b.CorreoExterno.MailFolders(ctx, v.Cuenta, dirs[v.Cuenta])
			if err != nil {
				return primeraExterna{}, err
			}
			i := slices.IndexFunc(cs, func(f api.ExternalMailFolder) bool { return f.Role == "inbox" })
			if i < 0 && len(cs) > 0 {
				i = 0
			}
			if i < 0 {
				return primeraExterna{}, nil
			}
			return primeraExterna{entrada: cs[i].ID}, nil
		}
		p, err := b.paginaDeFuente(ctx, FuenteDeBuzon{Cuenta: v.Cuenta, Carpeta: v.Carpeta}, "", dirs[v.Cuenta])
		return primeraExterna{mensajes: p.mensajes, cursor: p.siguiente}, err
	}
	paginas := make([]pagina, len(fuentes))
	fallos := make([]error, len(fuentes))
	var wg sync.WaitGroup
	for i, f := range fuentes {
		wg.Go(func() { paginas[i], fallos[i] = b.paginaDeFuente(ctx, f, "", dirs[f.Cuenta]) })
	}
	wg.Wait()
	var leidas []fuenteMezcla
	for i, f := range fuentes {
		if fallos[i] == nil {
			leidas = append(leidas, fuenteMezcla{clave: f.Cuenta, cursor: paginas[i].siguiente, pendientes: paginas[i].mensajes})
		}
	}
	if len(fuentes) > 0 && len(leidas) == 0 {
		return primeraExterna{}, fallos[0]
	}
	salida, quedan := mezclarPorFecha(leidas)
	return primeraExterna{mensajes: salida, mezcla: quedan, caidas: len(leidas) < len(fuentes)}, nil
}

// cargarListaExterna es la rama externa de cargarLista (el efecto de `selectedExternal`).
func (b *Buzon) cargarListaExterna(vista int, v VistaExterna) {
	b.mezcla, b.mezclaCaida, b.errorExterno, b.masExternos = nil, false, "", false
	b.pararRefrescoExterno()
	b.ponerAvisoExterno()
	fuentes, dirs := b.fuentesDe(v), b.direccionesDeCuentas()
	pedir(b, func(ctx context.Context) (primeraExterna, error) {
		return b.primeraPaginaExterna(ctx, v, fuentes, dirs)
	}, func(p primeraExterna, err error) {
		if vista != b.vista {
			return
		}
		if err != nil {
			a, _ := b.cuentaExterna(v.Cuenta)
			b.errorExterno = errorCorreo(err, a.IsICloud())
			b.P.Bandeja.Estado = estadoDe(err)
			if b.P.Bandeja.Estado == SinPermiso {
				b.P.Bandeja.Estado = Error // un 403 del proveedor no es una ACL de Sytue
			}
			b.ponerAvisoExterno()
			b.P.Refrescar()
			return
		}
		if p.entrada != "" {
			// Su Entrada: la vista cambia y se carga otra vez.
			b.externa = &VistaExterna{Cuenta: v.Cuenta, Carpeta: p.entrada}
			b.P.Menu.Externa = b.externa
			b.P.Bandeja.Titulo = b.tituloExterno()
			b.cargarLista()
			return
		}
		b.mensajes, b.cursor, b.mezcla, b.mezclaCaida = p.mensajes, p.cursor, p.mezcla, p.caidas
		b.ponerAvisoExterno()
		b.pintarLista()
		b.programarExterna()
	})
}

// masExterna es la rama externa de MasPagina: la página siguiente de la carpeta, o, en
// un buzón unificado, la de las carpetas que se quedaron sin pendientes, mezclada otra
// vez.
func (b *Buzon) masExterna(v VistaExterna) {
	if b.pidiendo {
		return
	}
	if !v.Mezclada() && b.cursor == "" || v.Mezclada() && !quedanEnMezcla(b.mezcla) {
		return
	}
	b.pidiendo = true
	b.P.Bandeja.CargandoMas = true
	b.P.Bandeja.Refresh()
	vista, cursor, dirs := b.vista, b.cursor, b.direccionesDeCuentas()
	fuentes, mezcla := b.fuentesDe(v), slices.Clone(b.mezcla)
	pedir(b, func(ctx context.Context) (masPaginas, error) {
		if !v.Mezclada() {
			p, err := b.paginaDeFuente(ctx, FuenteDeBuzon{Cuenta: v.Cuenta, Carpeta: v.Carpeta}, cursor, dirs[v.Cuenta])
			return masPaginas{mensajes: p.mensajes, cursor: p.siguiente}, err
		}
		return b.masMezclados(ctx, mezcla, fuentes, dirs), nil
	}, func(r masPaginas, err error) {
		if vista != b.vista {
			return
		}
		b.pidiendo = false
		b.P.Bandeja.CargandoMas = false
		if err != nil {
			b.cursor = ""
			b.P.Bandeja.Refresh()
			return
		}
		b.mensajes = sinRepetir(b.mensajes, r.mensajes)
		b.cursor, b.masExternos = r.cursor, true
		if v.Mezclada() {
			b.mezcla = r.mezcla
			b.mezclaCaida = b.mezclaCaida || r.caida
			b.ponerAvisoExterno()
		}
		b.pintarLista()
	})
}

// masPaginas es lo que trae «Cargar más» de una vista externa.
type masPaginas struct {
	mensajes []api.MailMessage
	cursor   string
	mezcla   []fuenteMezcla
	caida    bool
}

// masMezclados pide la página siguiente de las carpetas que se quedaron sin pendientes
// y vuelve a mezclar, hasta que salga algo o no quede nada, con un tope de vueltas: una
// carpeta que da páginas vacías con cursor no puede dejar la lista pidiendo para
// siempre. Una que falla deja de pedirse y se dice. Corre fuera del hilo de la interfaz.
func (b *Buzon) masMezclados(ctx context.Context, mezcla []fuenteMezcla, fuentes []FuenteDeBuzon, dirs map[string]string) masPaginas {
	r := masPaginas{mezcla: mezcla}
	for range 5 {
		for _, clave := range porPedir(r.mezcla) {
			i := slices.IndexFunc(r.mezcla, func(f fuenteMezcla) bool { return f.clave == clave })
			j := slices.IndexFunc(fuentes, func(f FuenteDeBuzon) bool { return f.Cuenta == clave })
			if j < 0 {
				r.mezcla[i].cursor = ""
				continue
			}
			p, err := b.paginaDeFuente(ctx, fuentes[j], r.mezcla[i].cursor, dirs[clave])
			if err != nil {
				r.caida, r.mezcla[i].cursor = true, ""
				continue
			}
			r.mezcla[i].pendientes = append(slices.Clone(r.mezcla[i].pendientes), p.mensajes...)
			r.mezcla[i].cursor = p.siguiente
		}
		salida, quedan := mezclarPorFecha(r.mezcla)
		r.mensajes, r.mezcla = append(r.mensajes, salida...), quedan
		if len(r.mensajes) > 0 || !quedanEnMezcla(r.mezcla) {
			break
		}
	}
	return r
}

// programarExterna arma el refresco de 30 s de la vista externa: solo con la lista en
// reposo (nada abierto, marcado ni cargado de más) y la app en primer plano. Un fallo
// aquí no pinta nada: la lista que hay sigue valiendo.
func (b *Buzon) programarExterna() {
	b.pararRefrescoExterno()
	if b.externa == nil || b.segundoPlano || b.ctx == nil || b.ctx.Err() != nil {
		return
	}
	vista := b.vista
	b.pararExterno = b.Esperar(RefrescoExterno, func() {
		b.pararExterno = nil
		if vista != b.vista || b.externa == nil || b.segundoPlano {
			return
		}
		if !b.enReposoExterno() {
			b.programarExterna()
			return
		}
		v, dirs := *b.externa, b.direccionesDeCuentas()
		fuentes := b.fuentesDe(v)
		pedir(b, func(ctx context.Context) (primeraExterna, error) {
			return b.primeraPaginaExterna(ctx, v, fuentes, dirs)
		}, func(p primeraExterna, err error) {
			if vista != b.vista {
				return
			}
			if err == nil && p.entrada == "" && b.enReposoExterno() {
				b.mensajes, b.cursor, b.mezcla, b.mezclaCaida = p.mensajes, p.cursor, p.mezcla, p.caidas
				b.ponerAvisoExterno()
				b.pintarLista()
			}
			b.programarExterna()
		})
	})
}

func (b *Buzon) enReposoExterno() bool {
	return b.hiloID == "" && b.P.Bandeja.CuantasMarcadas() == 0 && !b.masExternos && !b.pidiendo && b.consulta == ""
}

func (b *Buzon) pararRefrescoExterno() {
	if b.pararExterno != nil {
		b.pararExterno()
		b.pararExterno = nil
	}
}

// ponerAvisoExterno es la franja de la vista externa (`AvisoExterno` en la web): por
// qué el proveedor no dejó leer la carpeta —con «Conectar cuenta» si es el inicio de
// sesión—, que alguna carpeta del buzón unificado no contestó, o que una cuenta POP3
// solo deja leer.
func (b *Buzon) ponerAvisoExterno() {
	b.P.Franja = nil
	v := b.externa
	if v == nil {
		return
	}
	t := b.P.T
	a, _ := b.cuentaExterna(v.Cuenta)
	var texto string
	tono := ui.AyudaAviso
	switch {
	case b.errorExterno != "":
		texto = t.T(b.errorExterno, nil)
	case v.Mezclada() && b.mezclaCaida:
		texto = t.T("externalMail.someUnavailable", nil)
	case !v.Mezclada() && a.Provider == "pop3":
		texto, tono = t.T("externalMail.pop3Note", nil), ui.AyudaInfo
	default:
		return
	}
	filas := []fyne.CanvasObject{ui.NuevaAyudaEnLinea(texto).ConTono(tono)}
	if (b.errorExterno == "externalMail.authError" || b.errorExterno == "externalMail.icloudAuthError") && !v.Mezclada() {
		cuenta := v.Cuenta
		conectar := ui.NuevoBoton(t.T("externalMail.connect", nil), func() { b.abrirCorreoExternoEn(cuenta) }).Con(ui.Secundario, ui.TallaSM)
		filas = append(filas, conectar)
	}
	// `border-b px-4 py-3`, con el aviso y su botón uno debajo del otro.
	b.P.Franja = container.New(ui.Margen{Izq: tema.Espacio(4), Der: tema.Espacio(4), Arr: tema.Espacio(3), Abj: tema.Espacio(3)},
		container.New(ui.Columna{Hueco: tema.Espacio(2)}, filas...))
}

// ---- Lo que se hace con un mensaje externo ----

// marcarEnServidor pone o quita «leído» o «destacado»: los de Sytue en UNA llamada en
// lote; los de una cuenta, uno a uno en su proveedor (no hay lote). Corre fuera del
// hilo de la interfaz con las direcciones ya leídas.
func (b *Buzon) marcarEnServidor(ctx context.Context, ids []string, marca string, poner bool, dirs map[string]string) error {
	var propios []string
	var errs []error
	for _, id := range ids {
		o, ok := origenExterno(id)
		if !ok {
			propios = append(propios, id)
			continue
		}
		errs = append(errs, b.CorreoExterno.MailAction(ctx, o.Cuenta, o.Carpeta, o.ID, accionDeMarca(marca, poner), "", dirs[o.Cuenta]))
	}
	if len(propios) > 0 {
		errs = append(errs, b.S.SetFlagMany(ctx, propios, marca, poner))
	}
	return errors.Join(errs...)
}

// accionDeMarca es la acción del proveedor para una marca de IMAP.
func accionDeMarca(marca string, poner bool) string {
	switch {
	case marca == api.MailFlagSeen && poner:
		return "read"
	case marca == api.MailFlagSeen:
		return "unread"
	case poner:
		return "flag"
	}
	return "unflag"
}

// hayExternos y hayPropios dicen de quién son unos mensajes, para refrescar los
// contadores que tocan: el resumen de las cuentas o las carpetas de Sytue.
func hayExternos(ids []string) bool { return slices.ContainsFunc(ids, esExterno) }

func hayPropios(ids []string) bool {
	return slices.ContainsFunc(ids, func(id string) bool { return !esExterno(id) })
}

// refrescarContadoresDe pide los contadores de quien cambió.
func (b *Buzon) refrescarContadoresDe(ids []string) {
	if hayPropios(ids) {
		b.refrescarContadores()
	}
	if hayExternos(ids) {
		b.cargarExternas()
	}
}

// moverExternos mueve mensajes de cuentas externas, cada uno en su proveedor: a la
// carpeta que `destino` dé para su cuenta, o a la papelera, que el servidor resuelve
// aunque el proveedor no tenga la acción. Sin «Deshacer»: un IMAP o Outlook le dan
// otro id al mensaje movido, y devolverlo sería adivinar. Si algo falla, se vuelve a
// pedir la lista —lo que sí se movió ya no está ahí— y se avisa con la carpeta.
func (b *Buzon) moverExternos(cs []Conversacion, destino func(cuenta string) (api.ExternalMailFolder, bool), rotulo string, papelera bool) {
	type llamada struct {
		o       OrigenExterno
		destino string
	}
	var llamadas []llamada
	var mover []Conversacion
	sinDestino := false
	for _, c := range cs {
		ok := true
		var suyas []llamada
		for _, m := range c.Mensajes {
			o, externo := origenExterno(m.ID)
			if !externo {
				continue
			}
			l := llamada{o: o}
			if !papelera {
				f, hay := destino(o.Cuenta)
				if !hay {
					ok = false
					break
				}
				l.destino = f.ID
			}
			suyas = append(suyas, l)
		}
		if !ok {
			sinDestino = true
			continue
		}
		llamadas = append(llamadas, suyas...)
		mover = append(mover, c)
	}
	if sinDestino {
		b.avisar(ui.AvisoPeligro, "error.move", i18n.Vars{"folder": rotulo})
	}
	if len(llamadas) == 0 {
		return
	}
	b.quitarDeLaVista(mover)
	b.pintarLista()
	dirs := b.direccionesDeCuentas()
	pedir(b, func(ctx context.Context) (struct{}, error) {
		errs := make([]error, 0, len(llamadas))
		for _, l := range llamadas {
			accion := "move"
			if papelera {
				accion = "trash"
			}
			errs = append(errs, b.CorreoExterno.MailAction(ctx, l.o.Cuenta, l.o.Carpeta, l.o.ID, accion, l.destino, dirs[l.o.Cuenta]))
		}
		return struct{}{}, errors.Join(errs...)
	}, func(_ struct{}, err error) {
		b.cargarExternas()
		if err != nil {
			b.avisar(ui.AvisoPeligro, "error.move", i18n.Vars{"folder": rotulo})
			b.cargarLista()
			return
		}
		b.avisar(ui.AvisoExito, "triage.moved", i18n.Vars{"folder": rotulo})
	})
}

// moverPorRol es archivar, tirar, marcar spam o «No es spam» en una vista externa:
// cada hilo a la carpeta de ese rol de SU cuenta. En un buzón unificado
// los de Sytue van a las de Sytue, con su «Deshacer», y los de cada cuenta a las suyas.
func (b *Buzon) moverPorRol(cs []Conversacion, rol string) {
	var propias, ajenas []Conversacion
	for _, c := range cs {
		switch {
		case b.sinAcciones(c.Ultimo.ID):
			// POP3: se lee, no se tría.
		case esExterno(c.Ultimo.ID):
			ajenas = append(ajenas, c)
		default:
			propias = append(propias, c)
		}
	}
	if c, hay := b.carpetaDeRol(rol); hay && len(propias) > 0 {
		b.moverConvs(propias, c)
	}
	if len(ajenas) > 0 {
		b.moverExternos(ajenas, func(cuenta string) (api.ExternalMailFolder, bool) { return b.carpetaExternaDeRol(cuenta, rol) },
			RotuloCarpeta(b.P.T, "", rol), rol == "trash")
	}
}

// rolExterno es el rol de la vista externa a la vista: el de su carpeta, o el del
// buzón unificado («inbox», «junk»…).
func (b *Buzon) rolExterno() string {
	v := b.externa
	rol := v.Unificado
	if !v.Mezclada() {
		if f, ok := b.carpetaExterna(v.Cuenta, func(f api.ExternalMailFolder) bool { return f.ID == v.Carpeta }); ok {
			rol = f.Role
		}
	}
	return rol
}

// ponerAccionesExternas es el triaje de una vista externa, en ponerAcciones. Una
// carpeta de una cuenta: sus destinos son las carpetas de ESA cuenta, y una acción
// sale si la cuenta tiene la carpeta de su rol y no se está ya en ella. En un buzón
// unificado cada hilo va a la suya; «Mover a…» no sale, porque las carpetas de una
// cuenta no son las de otra. Etiquetas y reglas son de Sytue: en la carpeta de una
// cuenta no salen, y en un buzón unificado solo para lo de Sytue.
func (b *Buzon) ponerAccionesExternas(a *Acciones) {
	v := b.externa
	a.Destacar, a.Leido = b.alternarDestacado, b.alternarLeido
	rolActual := b.rolExterno()
	destino := func(rol string) func(Conversacion) {
		if rolActual == rol {
			return nil
		}
		if _, hay := b.carpetaExternaDeRol(v.Cuenta, rol); !v.Mezclada() && rol != "trash" && !hay {
			return nil
		}
		return func(x Conversacion) { b.moverPorRol([]Conversacion{x}, rol) }
	}
	a.Archivar, a.Papelera, a.Spam = destino("archive"), destino("trash"), destino("junk")
	if rolActual == "junk" {
		a.NoSpam = destino("inbox")
	}
	if v.Mezclada() {
		// Los de Sytue mezclados se etiquetan y dan reglas como siempre; el lector no
		// lo ofrece para uno externo.
		a.Etiquetar = b.etiquetar
		a.CrearRegla = func(c Conversacion) { b.crearReglaDesde(c) }
		return
	}
	for _, c := range b.P.Menu.Cuentas {
		if c.ID != v.Cuenta {
			continue
		}
		for _, f := range c.Carpetas {
			if f.ID != v.Carpeta && f.Role != "drafts" {
				a.Destinos = append(a.Destinos, Carpeta{ID: f.ID, Nombre: f.Name, Rol: f.Role, SinLeer: int(f.Unread), Total: int(f.Total)})
			}
		}
	}
	cuenta := v.Cuenta
	a.Mover = func(x Conversacion, c Carpeta) {
		b.moverExternos([]Conversacion{x}, func(string) (api.ExternalMailFolder, bool) {
			return b.carpetaExterna(cuenta, func(f api.ExternalMailFolder) bool { return f.ID == c.ID })
		}, RotuloCarpeta(b.P.T, c.Nombre, c.Rol), false)
	}
}

// ponerLector deja en el lector lo que existe para el mensaje abierto. Uno externo no
// está en Sytue: ni evento, ni Rooms, ni baja, ni Drive, ni el .eml original.
func (b *Buzon) ponerLector(externo bool) {
	l := b.P.Lector
	l.AlCrearEvento, l.AlCompartirEnRooms, l.AlGuardarEnDrive = nil, nil, nil
	l.AlOriginal, l.AlDarseDeBaja = nil, nil
	if externo {
		return
	}
	if b.Hub != nil && b.Abridor != nil {
		l.AlCrearEvento = b.crearEvento
	}
	if b.Abridor != nil {
		l.AlCompartirEnRooms = b.compartirEnRooms
	}
	if b.Drive != nil {
		l.AlGuardarEnDrive = b.guardarEnDrive
	}
	l.AlOriginal, l.AlDarseDeBaja = b.descargarOriginal, b.darseDeBaja
}

// pedirCuerpoExterno lee un mensaje externo de su proveedor: su texto y sus adjuntos,
// que llegan con sus bytes.
func (b *Buzon) pedirCuerpoExterno(id string, o OrigenExterno) {
	dir := b.direccionesDeCuentas()[o.Cuenta]
	pedir(b, func(ctx context.Context) (api.ExternalMailMessage, error) {
		return b.CorreoExterno.MailMessage(ctx, o.Cuenta, o.Carpeta, o.ID, dir)
	}, func(m api.ExternalMailMessage, err error) {
		b.cuerpos[id] = CuerpoExterno(m, err)
		b.P.Lector.Refresh()
	})
}

// CuerpoExterno es `CuerpoDe` para un mensaje del proveedor: el texto primero y el
// HTML pasado a texto después; los adjuntos, decodificados (el que no se decodifica no
// se ofrece: sería un fichero roto).
func CuerpoExterno(m api.ExternalMailMessage, err error) Cuerpo {
	var adjuntos []eml.Adjunto
	for _, a := range m.Attachments {
		datos, errB := base64.StdEncoding.DecodeString(a.ContentBase64)
		if errB != nil || a.ContentBase64 == "" {
			continue
		}
		adjuntos = append(adjuntos, eml.Adjunto{Nombre: a.Name, Tipo: a.Type, Datos: datos})
	}
	conFormato := strings.TrimSpace(m.HTML) != ""
	if texto := strings.TrimSpace(m.Text); texto != "" {
		return Cuerpo{Texto: strings.ReplaceAll(texto, "\r\n", "\n"), ConFormato: conFormato, Adjuntos: adjuntos}
	}
	if conFormato {
		return Cuerpo{Texto: htmltext.Text(m.HTML), ConFormato: true, Adjuntos: adjuntos}
	}
	return Cuerpo{Fallo: err != nil, Adjuntos: adjuntos}
}

// redactarExterno es «Redactar» con la carpeta de una cuenta a la vista: se escribe
// desde esa cuenta, que es su «De». `false` si no toca (otra vista, o una cuenta que no
// envía).
func (b *Buzon) redactarExterno() bool {
	v := b.externa
	if v == nil || v.Mezclada() || b.CorreoExterno == nil {
		return false
	}
	a, ok := b.cuentaExterna(v.Cuenta)
	if !ok || !a.Capabilities.Send {
		return false
	}
	b.abrirRedactor(Borrador{De: a.Address, Cuenta: a.InstanceID, Cuerpo: ConFirma("", b.firma, false)})
	return true
}

// responderExterno es responder o reenviar un mensaje externo desde su cuenta, que es
// a quien escribieron. `false` si el mensaje es de Sytue o la cuenta no envía.
func (b *Buzon) responderExterno(m Mensaje, modo Respuesta) bool {
	o, ok := origenExterno(m.ID)
	if !ok || b.CorreoExterno == nil {
		return false
	}
	a, ok := b.cuentaExterna(o.Cuenta)
	if !ok || !a.Capabilities.Send {
		return false
	}
	completo := ""
	if c, ya := b.cuerpos[m.ID]; ya {
		completo = c.Texto
	}
	br := BorradorDeRespuesta(b.P.T, b.P.Locale, m, modo, a.Address, completo)
	br.Cuerpo = ConFirma(br.Cuerpo, b.firma, false)
	br.De, br.Cuenta = a.Address, a.InstanceID
	b.abrirRedactor(br)
	return true
}
