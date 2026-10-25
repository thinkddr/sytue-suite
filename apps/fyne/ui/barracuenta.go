// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package ui

import (
	"bytes"
	"cmp"
	"context"
	"image"
	"image/color"
	_ "image/gif"  // los cuatro formatos que Sytue ID acepta de avatar
	_ "image/jpeg" // (validation.RasterImageDataURI)
	_ "image/png"  //
	"log/slog"
	"slices"
	"sync"
	"time"

	_ "golang.org/x/image/webp" //

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"

	"git.sytue.eu/sytue/one/apps/fyne/iconos"
	"git.sytue.eu/sytue/one/apps/fyne/sesion"
	"git.sytue.eu/sytue/one/apps/fyne/tema"
	"git.sytue.eu/sytue/one/apps/nucleo/ajustes"
	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/i18n"
	"git.sytue.eu/sytue/one/apps/nucleo/tokens"
)

// El menú de cuenta: `AccountMenu.tsx`. De arriba abajo, la cabecera con el avatar
// grande, la presencia, la apariencia, el idioma, «Mi cuenta» y «Cerrar sesión».
//
// # Nada de esto lo guarda la barra
//
// La sesión, el tema, el idioma y la presencia son TRANSVERSALES a las trece
// aplicaciones y viven en `apps/fyne/sesion`: el menú los lee de ahí y los cambia por
// ahí. Es lo que hace que elegir «Oscuro» en Sytue Mail lo cambie en Drive, y que
// cerrar sesión cierre en las trece.
//
// Y por eso el tic de la presencia sigue a `sesion.Estado` y no a lo último que se
// pulsó: el estado lo puede cambiar el teléfono o la web, y `AlCambiar` avisa venga de
// donde venga —siempre en el hilo de la interfaz—.
const (
	// AnchoCuenta es el `w-68` del menú: los 272 px de su CONTENIDO, dentro del marco y
	// del `p-1` del panel. Con 16 más que el `w-64` de antes, las cuatro apariencias
	// caben en una línea (AccountMenu.tsx). Fue el ancho de FUERA del panel, que lo
	// dejaba diez píxeles más estrecho que el de la web.
	AnchoCuenta = 17 * tema.Rem
	// HuecoCabeceraCuenta es el `gap-3` entre el avatar y los datos, y MargenCuentaX /
	// MargenCuentaY el `px-3 pt-3 pb-2.5` de la cabecera.
	HuecoCabeceraCuenta = 3 * 0.25 * tema.Rem
	MargenCuentaX       = 3 * 0.25 * tema.Rem
	MargenCuentaY       = 3 * 0.25 * tema.Rem
	// MargenPreferencias es el `px-2.5 py-2` de la banda de apariencia e idioma, y
	// HuecoPreferencias su `gap-2`.
	MargenPreferencias = 2.5 * 0.25 * tema.Rem
	HuecoPreferencias  = 2 * 0.25 * tema.Rem
	// HuecoFilaPreferencia y SaltoFilaPreferencia son el `gap-x-3 gap-y-2` de una fila:
	// entre el rótulo y su grupo, y entre el rótulo y el grupo cuando baja a su línea.
	HuecoFilaPreferencia = 3 * 0.25 * tema.Rem
	SaltoFilaPreferencia = 2 * 0.25 * tema.Rem
	// AnchoIdiomaCuenta es el `w-40` del desplegable de idioma, con su `max-w-[60%]`.
	AnchoIdiomaCuenta = 40 * 0.25 * tema.Rem
	TopeIdiomaCuenta  = 0.6
)

// presencias son las cuatro opciones, en el orden de `PRESENCE_OPTIONS`.
//
// La clave del catálogo es la de la WEB (`presence.dnd`) y el valor es el que el
// SERVICIO conoce (`busy`): en `api.NoMolestar` está escrito por qué no son lo mismo y
// por qué «No molestar» no funciona hoy en la web.
var presencias = []struct {
	estado api.EstadoPresencia
	clave  string
	token  string
	punto  Presencia
}{
	{api.EnLinea, "presence.online", "presence-online", EnLinea},
	{api.Ausente, "presence.away", "presence-away", Ausente},
	{api.NoMolestar, "presence.dnd", "presence-dnd", NoMolestar},
	{api.Desconectado, "presence.offline", "presence-offline", Desconectado},
}

// cuentaBarra es el avatar de la barra y su menú.
type cuentaBarra struct {
	t *i18n.Translator
	s *sesion.Suite

	boton *botonAvatar
	menu  *Menu

	mu       sync.Mutex
	cerrando bool // protege una sola retirada compartida y su resultado de UI
	estado   sesion.Estado
	ficha    api.Profile // el nombre y el correo, que el estado transversal no lleva
	// foto es el avatar elegido en Sytue ID, ya decodificado, y versionFoto su
	// `avatar_version`, que es el ETag de los bytes: con la misma no se vuelve a bajar.
	// nil, las iniciales.
	foto        image.Image
	versionFoto string

	// cuenta es el cliente de Sytue ID; nil, el de la sesión. Lo cambian las pruebas.
	cuenta *api.Account
}

func nuevaCuentaBarra(t *i18n.Translator, s *sesion.Suite) *cuentaBarra {
	c := &cuentaBarra{t: t, s: s}
	c.boton = nuevoBotonAvatar("", t.T("account.menu", nil), c.abrir)
	c.menu = NuevoMenu()
	c.menu.Rotulo = t.T("account.menu", nil)
	// El botón se tiñe mientras el menú está puesto (`aria-expanded`), y lo PREGUNTA
	// en vez de que se lo digan: el menú también se cierra con Escape y con el clic
	// de fuera, y esos dos caminos no pasan por aquí.
	c.boton.estaAbierto = c.menu.Abierto
	if s != nil {
		c.estado = s.Estado()
	}
	return c
}

// arrancar se apunta a lo transversal y pide la ficha de quien ha entrado.
func (c *cuentaBarra) arrancar() {
	if c.s == nil {
		return
	}
	c.s.AlCambiar(c.cambio) // llega SIEMPRE en el hilo de la interfaz
	e := c.s.Estado()
	c.cambio(e)
	// Con la sesión GUARDADA ya se estaba dentro al construir la barra —`sesion.Abrir`
	// la carga del llavero—, y `cambio` solo pide la ficha al ENTRAR: una app abierta
	// con sesión no traía nunca ni el nombre ni la foto, y el avatar salía con «?».
	if e.Dentro() {
		go c.pedirFicha()
	}
	// Al volver a la app —el foco a su ventana en escritorio, el regreso del segundo
	// plano en el móvil— se vuelve a preguntar la ficha: es donde se nota que la foto
	// se cambió en la web. Si no ha cambiado, cuesta un GET /me y nada más.
	//
	// Fyne admite UN receptor por app: los reparte la sesión (`sesion.AlVolver`).
	c.s.AlVolver(c.alVolver)
}

// alVolver vuelve a pedir la ficha si hay sesión.
func (c *cuentaBarra) alVolver() {
	c.mu.Lock()
	dentro := c.estado.Dentro()
	c.mu.Unlock()
	if dentro {
		go c.pedirFicha()
	}
}

// cambio es lo que hay que repintar cuando algo transversal se mueve: aquí o en otra
// de las doce aplicaciones.
func (c *cuentaBarra) cambio(e sesion.Estado) {
	c.mu.Lock()
	entraba := c.estado.Dentro()
	c.estado = e
	if !e.Dentro() {
		c.ficha, c.foto, c.versionFoto = api.Profile{}, nil, ""
	}
	c.mu.Unlock()

	if c.s != nil && e.Dentro() && !entraba {
		go c.pedirFicha()
	}
	c.pintar()
}

// pedirFicha trae el nombre, el correo y la foto de Sytue ID, que es lo que la cabecera
// del menú enseña y lo que el estado transversal no lleva: ahí solo va el `sub`.
func (c *cuentaBarra) pedirFicha() {
	ctx, cancelar := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancelar()
	cuenta := c.cuenta
	if cuenta == nil {
		cuenta = api.NewAccount(c.s.URL("id"), c.s.Sesion, nil)
	}
	p, err := cuenta.Me(ctx)
	if err != nil {
		return // sin ficha la cabecera enseña lo que sepa; no es motivo para no abrir el menú
	}
	foto, version := c.fotoDe(ctx, cuenta, p)
	c.mu.Lock()
	c.ficha, c.foto, c.versionFoto = p, foto, version
	c.mu.Unlock()
	c.pintar()
}

// fotoDe es la foto que va con la ficha `p`: la que ya se tenía si su versión no ha
// cambiado, y si no la de `GET /api/v1/me/avatar` con el token de la sesión —la MISMA
// fuente que la barra de la web—. Sin foto (404) o si no se puede bajar o leer, nil:
// las iniciales, nunca un hueco.
func (c *cuentaBarra) fotoDe(ctx context.Context, cuenta *api.Account, p api.Profile) (image.Image, string) {
	if !p.HasAvatar {
		return nil, ""
	}
	c.mu.Lock()
	foto, version := c.foto, c.versionFoto
	c.mu.Unlock()
	if foto != nil && version == p.AvatarVersion {
		return foto, version
	}
	datos, _, err := cuenta.Avatar(ctx)
	if err != nil || datos == nil {
		return nil, ""
	}
	img, _, err := image.Decode(bytes.NewReader(datos))
	if err != nil {
		return nil, ""
	}
	return img, p.AvatarVersion
}

// pintar repinta el avatar y, si el menú está puesto, su contenido.
func (c *cuentaBarra) pintar() {
	fyne.Do(func() {
		c.mu.Lock()
		presencia, nombre, foto := c.estado.Presencia, c.nombreSinCerrojo(), c.foto
		c.mu.Unlock()
		c.boton.avatar.Nombre = nombre
		c.boton.avatar.Foto = foto
		c.boton.avatar.Presencia = puntoDe(presencia)
		c.boton.Nombre = c.t.T("account.menuFor", i18n.Vars{"name": nombre})
		c.boton.Refresh()
		if c.menu.Abierto() {
			c.menu.Elementos = c.contenido()
			c.menu.Refresh()
		}
	})
}

// nombreSinCerrojo es con qué se nombra a quien ha entrado: su nombre, y si no se sabe
// todavía, su correo. Sin sesión, vacío —el avatar sale sin iniciales—.
func (c *cuentaBarra) nombreSinCerrojo() string {
	switch {
	case c.ficha.Name != "":
		return c.ficha.Name
	case c.ficha.Email != "":
		return c.ficha.Email
	}
	return ""
}

// puntoDe traduce el estado del servicio al punto del avatar. Lo que el Hub llama
// `busy` es «No molestar» en la pantalla. Uno que no se conoce, desconectado.
func puntoDe(estado api.EstadoPresencia) Presencia {
	if p, ok := PresenciaDe(estado); ok {
		return p
	}
	return Desconectado
}

// PresenciaDe es el punto que le toca a un estado de presencia del Hub, y `false` si no
// es ninguno de los cuatro. Es la tabla del menú de cuenta, para quien pinte presencias
// en otro sitio (el carril de Sytue Rooms) sin copiarla.
func PresenciaDe(estado api.EstadoPresencia) (Presencia, bool) {
	for _, p := range presencias {
		if p.estado == estado {
			return p.punto, true
		}
	}
	return SinPresencia, false
}

// elegirPresencia la publica en el Hub y deja el menú puesto, como la web. El tic no
// se mueve hasta que el Hub lo confirma: quien manda es el servidor.
func (c *cuentaBarra) elegirPresencia(e api.EstadoPresencia) {
	if c.s == nil {
		return
	}
	go func() {
		ctx, cancelar := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancelar()
		_ = c.s.PonerPresencia(ctx, e, "")
	}()
}

func (c *cuentaBarra) abrir() {
	if c.menu.Abierto() {
		c.menu.Cerrar()
		c.boton.Refresh()
		return
	}
	c.menu.Elementos = c.contenido()
	c.menu.Mostrar(c.boton)
	c.boton.Refresh()
}

// contenido es el menú entero.
func (c *cuentaBarra) contenido() []fyne.CanvasObject {
	c.mu.Lock()
	estado, nombre, correo, org := c.estado, c.nombreSinCerrojo(), c.ficha.Email, c.ficha.ActiveOrg
	foto := c.foto
	c.mu.Unlock()

	out := make([]fyne.CanvasObject, 0, len(presencias)+8)
	out = append(out, c.cabecera(nombre, correo, org, foto, estado), NuevoSeparadorMenu(), c.versal("account.presence"))
	for _, p := range presencias {
		e := NuevoElementoMenu(c.t.T(p.clave, nil), iconos.Ninguno, func() { c.elegirPresencia(p.estado) })
		e.Punto = p.token
		e.TamIcono = 18 // `inline-flex w-[18px]`: el punto se alinea con los iconos del menú
		// Cambiar la presencia no cierra el menú, como en AccountMenu.tsx: el tic se
		// mueve a la vista.
		e.MantieneAbierto = true
		e.Marcada = p.estado == estado.Presencia
		e.Inerte = !estado.Dentro() // sin sesión no hay presencia que declarar
		e.menu = c.menu
		out = append(out, e)
	}
	out = append(out, NuevoSeparadorMenu(), c.preferencias(estado), c.miCuenta(),
		NuevoSeparadorMenu(), c.entrarOSalir(estado))
	return out
}

// AnchoDatosCuenta es lo que le queda al nombre y al correo dentro del menú: el ancho
// del panel menos su marco, el relleno de la banda, el avatar y su hueco. Lo que no
// quepa se recorta con puntos, que es el `truncate` de la web —sin él, «Organización:
// corporate» se salía del panel, y se vio MIRANDO la captura—.
var AnchoDatosCuenta = AnchoCuenta - 2*MargenCuentaX - pxAvatar[AvatarMD] - HuecoCabeceraCuenta

// sangria es el relleno de una banda del menú que no es un `ElementoMenu` —la
// cabecera, el rótulo de sección, las preferencias—: esos llevan su `px-3 py-2`
// dentro, y un hijo suelto no.
//
// `Min` es además donde se declara el ANCHO del panel: `MedidaMenu` toma el mayor de
// sus hijos. Antes esto se hacía con un rectángulo invisible dentro de la fila, y el
// rectángulo se comía el sitio: la cabecera salía pegada a la derecha con el nombre
// cortado. Se vio mirando el PNG, no fallando un test.
type sangria struct{ X, Y, Min float32 }

var _ fyne.Layout = sangria{}

func (s sangria) MinSize(objs []fyne.CanvasObject) fyne.Size {
	var m fyne.Size
	for _, o := range objs {
		m = m.Max(o.MinSize())
	}
	return fyne.NewSize(max(m.Width+2*s.X, s.Min), m.Height+2*s.Y)
}

func (s sangria) Layout(objs []fyne.CanvasObject, t fyne.Size) {
	for _, o := range objs {
		o.Move(fyne.NewPos(s.X, s.Y))
		o.Resize(fyne.NewSize(max(t.Width-2*s.X, 0), max(t.Height-2*s.Y, 0)))
	}
}

// cabecera es el avatar grande con el nombre, el correo y la organización.
//
// `org` es el slug de la organización ACTIVA tal y como la da Sytue ID
// (`api.Profile.ActiveOrg`, el claim `tenant` del token). Hasta el 2026-09-22 se
// deducía del dominio del correo, que acierta solo cuando los dos coinciden: con un
// `ada@gmail.com` invitada a `acme` decía «Organización: gmail.com».
func (c *cuentaBarra) cabecera(nombre, correo, org string, foto image.Image, e sesion.Estado) fyne.CanvasObject {
	if nombre == "" {
		// Sin sesión el menú lo dice con todas las letras en vez de enseñar una ficha
		// vacía: hoy ninguna aplicación nativa arranca con sesión.
		nombre = c.t.T("account.menu", nil)
	}
	avatar := NuevoAvatar(nombre).Con(AvatarMD)
	avatar.Foto = foto
	avatar.Presencia = puntoDe(e.Presencia)

	corta := func(texto string, esc tema.Escalon, p Peso) string {
		return recorta(texto, esc, p, AnchoDatosCuenta)
	}
	datos := []fyne.CanvasObject{
		NuevoRotulo(corta(nombre, tema.TextoBase, SemiNegrita), tema.TextoBase, SemiNegrita, "text"),
	}
	if correo != "" {
		datos = append(datos, NuevoRotulo(corta(correo, tema.TextoSM, Normal), tema.TextoSM, Normal, "text-muted"))
	}
	if org != "" {
		linea := c.t.T("account.organization", i18n.Vars{"tenant": org})
		datos = append(datos, NuevoRotulo(corta(linea, tema.TextoXS, Normal), tema.TextoXS, Normal, "text-subtle"))
	}
	fila := container.New(Fila{Hueco: HuecoCabeceraCuenta}, avatar, container.New(Columna{}, datos...))
	return container.New(sangria{X: MargenCuentaX, Y: MargenCuentaY, Min: AnchoCuenta}, fila)
}

// versal es el rótulo de sección en versalitas (`sytue-overline text-text-subtle`),
// alineado con el texto de los ítems y no con el borde del panel.
func (c *cuentaBarra) versal(clave string) fyne.CanvasObject {
	rotulo := NuevoRotulo(Versalitas(c.t.T(clave, nil)), tema.TextoXS, SemiNegrita, "text-subtle")
	return container.New(sangria{X: MargenCuentaX, Y: tema.Espacio(1)}, rotulo)
}

// preferencias es la banda de apariencia, color, forma e idioma: los ajustes de la
// PERSONA, que son transversales a todas las aplicaciones de la suite. Cada uno es una
// fila con su rótulo a la izquierda y su control a la derecha (AccountMenu.tsx).
func (c *cuentaBarra) preferencias(e sesion.Estado) fyne.CanvasObject {
	columna := container.New(Columna{Hueco: HuecoPreferencias},
		c.fila("account.appearance", c.apariencia(e)),
		// El color es un eje aparte del modo (ADR 0071), y la forma, el tercero.
		c.fila("account.color", c.color(e)),
		c.fila("account.forma", c.forma(e)),
		c.idioma(e))
	return container.New(sangria{X: MargenPreferencias, Y: tema.Espacio(2)}, columna)
}

// fila es `flex flex-wrap items-center justify-between gap-x-3 gap-y-2`: el rótulo a la
// izquierda y el grupo a la derecha, en la misma línea si caben; si no, el grupo baja a
// la suya. Y el grupo, que también envuelve, reparte ahí sus opciones.
func (c *cuentaBarra) fila(clave string, grupo *Segmentado) fyne.CanvasObject {
	grupo.Ancho = anchoPreferenciasCuenta
	grupo.EnMenu = true
	rotulo := NuevoRotulo(c.t.T(clave, nil), tema.TextoSM, Normal, "text")
	return container.New(filaPreferencia{}, rotulo, grupo)
}

// filaPreferencia coloca [rótulo, control] a lo ancho de la banda.
type filaPreferencia struct{}

var _ fyne.Layout = filaPreferencia{}

// enLinea dice si el rótulo y el control caben uno al lado del otro.
func (filaPreferencia) enLinea(objs []fyne.CanvasObject) bool {
	return objs[0].MinSize().Width+HuecoFilaPreferencia+objs[1].MinSize().Width <= anchoPreferenciasCuenta
}

func (f filaPreferencia) MinSize(objs []fyne.CanvasObject) fyne.Size {
	r, g := objs[0].MinSize(), objs[1].MinSize()
	if f.enLinea(objs) {
		return fyne.NewSize(anchoPreferenciasCuenta, max(r.Height, g.Height))
	}
	return fyne.NewSize(anchoPreferenciasCuenta, r.Height+SaltoFilaPreferencia+g.Height)
}

func (f filaPreferencia) Layout(objs []fyne.CanvasObject, t fyne.Size) {
	r, g := objs[0].MinSize(), objs[1].MinSize()
	objs[0].Resize(r)
	objs[1].Resize(g)
	if f.enLinea(objs) {
		objs[0].Move(fyne.NewPos(0, (t.Height-r.Height)/2))
		objs[1].Move(fyne.NewPos(t.Width-g.Width, (t.Height-g.Height)/2))
		return
	}
	objs[0].Move(fyne.NewPos(0, 0))
	objs[1].Move(fyne.NewPos(0, r.Height+SaltoFilaPreferencia))
}

// formasAjuste y coloresAjuste son los `THEME_FORMAS` y los `THEME_COLORS` de la web, en
// su orden: lo que se guarda y su clave del catálogo (`account.forma.<clave>`,
// `account.color.<clave>`) son la misma palabra.
var (
	formasAjuste  = []string{"marco", "carril"}
	coloresAjuste = []string{"petroleo", "cobalto", "app"}
)

// forma es el grupo de la forma de la suite y color, el de su color. Cambian AL
// MOMENTO, aquí y en las otras apps abiertas, por los ajustes compartidos.
func (c *cuentaBarra) forma(e sesion.Estado) *Segmentado {
	return c.grupoDeEje("forma", formasAjuste, e.FormaPropia, e.Forma, e.ConOrganizacion,
		func(a *ajustes.Ajustes, valor string) { a.Forma = valor })
}

// El color lleva su muestra (`themeColorSwatch`): la de «Organización» es la del color
// de la organización, que es lo que se va a ver al elegirla.
func (c *cuentaBarra) color(e sesion.Estado) *Segmentado {
	s := c.grupoDeEje("color", coloresAjuste, e.ColorPropio, e.Color, e.ConOrganizacion,
		func(a *ajustes.Ajustes, valor string) { a.Color = valor })
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	claves := coloresAjuste
	if e.ConOrganizacion {
		claves = append([]string{cmp.Or(e.ColorOrg, coloresAjuste[0])}, coloresAjuste...)
	}
	for _, clave := range claves {
		s.Muestras = append(s.Muestras, muestraDeColor(clave, v))
	}
	return s
}

// muestraDeColor es `themeColorSwatch`: petróleo y cobalto tienen un token fijo en la
// hoja; «el de cada app» no es UN color, y su muestra son cuatro colores de producto en
// cuartos, sin fundido.
func muestraDeColor(clave string, v tema.Variante) []color.NRGBA {
	if clave == tokens.ColorDeCadaApp {
		return []color.NRGBA{
			tema.Color("product-mail", v), tema.Color("product-drive", v),
			tema.Color("product-docs", v), tema.Color("product-calendar", v),
		}
	}
	return []color.NRGBA{tema.Color("swatch-"+clave, v)}
}

// grupoDeEje es el grupo de un eje que la organización puede fijar por defecto
// (`SegmentadoDeEje` de AccountMenu.tsx): sus valores y, delante, «Organización», que es
// NO elegir —borra la elección de la persona y vuelve a seguir a la suya—. Sin ella,
// quien eligió una vez ya no podía volver. Una cuenta personal no tiene quien fije nada
// y no la ve: enseña lo que se está pintando, que sin elección es lo de la casa.
func (c *cuentaBarra) grupoDeEje(eje string, valores []string, propio, enVigor string, conOrg bool, poner func(*ajustes.Ajustes, string)) *Segmentado {
	claves, elegido := valores, cmp.Or(propio, enVigor)
	if conOrg {
		claves, elegido = append([]string{""}, valores...), propio // "" es seguir a la organización
	}
	rotulos := make([]string, 0, len(claves))
	for _, clave := range claves {
		if clave == "" {
			rotulos = append(rotulos, c.t.T("account.followOrg", nil))
			continue
		}
		rotulos = append(rotulos, c.t.T("account."+eje+"."+clave, nil))
	}
	s := NuevoSegmentado(c.t.T("account."+eje, nil), rotulos, func(i int) {
		if c.s == nil || i < 0 || i >= len(claves) {
			return
		}
		_ = c.s.Poner(func(a *ajustes.Ajustes) { poner(a, claves[i]) })
	})
	s.Elegida = max(slices.Index(claves, elegido), 0)
	return s
}

// temaAjuste es un modo de apariencia: lo que se guarda y cómo lo llama el catálogo.
type temaAjuste struct{ valor, clave string }

// temasAjuste son las `APARIENCIAS` de AccountMenu.tsx, en su orden. «Sistema» conserva
// su valor vacío; las otras tres usan las claves de la web.
var temasAjuste = []temaAjuste{
	{"claro", "light"}, {"traslucido", "translucent"}, {"oscuro", "dark"}, {"", "system"},
}

// anchoPreferenciasCuenta es lo que le queda a una fila: el `w-68` menos su `px-2.5`.
var anchoPreferenciasCuenta float32 = AnchoCuenta - 2*MargenPreferencias

// apariencia es el grupo de las cuatro apariencias. Cambia el tema AL MOMENTO —aquí y en
// las otras aplicaciones abiertas, que se enteran en menos de dos segundos—.
func (c *cuentaBarra) apariencia(e sesion.Estado) *Segmentado {
	rotulos := make([]string, 0, len(temasAjuste))
	for _, m := range temasAjuste {
		rotulos = append(rotulos, c.t.T("account.appearance."+m.clave, nil))
	}
	s := NuevoSegmentado(c.t.T("account.appearance", nil), rotulos, func(i int) {
		if c.s == nil || i < 0 || i >= len(temasAjuste) {
			return
		}
		valor := temasAjuste[i].valor
		_ = c.s.Poner(func(a *ajustes.Ajustes) { a.Tema = valor })
	})
	// Una elección desconocida deja mandar al sistema, como tema.Elegir.
	s.Elegida = slices.IndexFunc(temasAjuste, func(m temaAjuste) bool { return m.valor == e.Tema })
	if s.Elegida < 0 {
		s.Elegida = slices.IndexFunc(temasAjuste, func(m temaAjuste) bool { return m.valor == "" })
	}
	return s
}

// idioma ofrece seguir al sistema o elegir uno de los veintiséis, EN UNA LÍNEA con su rótulo a la izquierda,
// como las filas de encima (`LanguageSelect.tsx`: `flex items-center justify-between
// gap-3`, y el control en `w-40 max-w-[60%]`). Con el rótulo arriba eran 24 px más en un
// menú que no cabía a 1280×800.
func (c *cuentaBarra) idioma(e sesion.Estado) fyne.CanvasObject {
	rotulos := make([]string, 0, 1+len(i18n.Locales))
	rotulos = append(rotulos, c.t.T("account.appearance.system", nil))
	for _, l := range i18n.Locales {
		rotulos = append(rotulos, i18n.Names[l])
	}
	s := NuevoSelector(rotulos, nil)
	s.Rotulo = c.t.T("account.language", nil)
	// Vacío (o desconocido) ya significa negociar con el SO al abrir la app.
	s.Elegida = 1 + slices.Index(i18n.Locales, i18n.Locale(e.Idioma))
	s.Ancho = min(AnchoIdiomaCuenta, TopeIdiomaCuenta*anchoPreferenciasCuenta)
	s.AlElegir = func(i int) {
		if c.s == nil || i < 0 || i > len(i18n.Locales) {
			return
		}
		etiqueta := ""
		if i > 0 {
			etiqueta = string(i18n.Locales[i-1])
		}
		_ = c.s.Poner(func(a *ajustes.Ajustes) { a.Idioma = etiqueta })
	}
	fila := container.New(filaPreferencia{},
		NuevoRotulo(c.t.T("account.language", nil), tema.TextoSM, Normal, "text"), s)
	// El aviso es de aquí; la web recarga al cambiar de idioma y una app
	// nativa lo aplica al volver a abrirse. Pegado a su fila: es su ayuda, no otro ajuste.
	aviso := NuevoParrafo(c.t.T("account.languageRestart", nil), tema.TextoXS, Normal, "text-subtle",
		tema.De(fyne.CurrentApp().Settings().ThemeVariant())).A(anchoPreferenciasCuenta)
	return container.New(Columna{Hueco: tema.Espacio(1)}, fila, aviso)
}

// miCuenta abre la WEB de Sytue ID, y no una aplicación.
//
// Es la excepción que el dueño escribió el 2026-09-22: «Mi cuenta solo se accede vía
// web». No hay binario nativo de «Mi cuenta», así que esto no prueba ningún esquema:
// va al navegador y punto.
func (c *cuentaBarra) miCuenta() fyne.CanvasObject {
	e := NuevoElementoMenu(c.t.T("account.myAccount", nil), iconos.Usuario, func() {
		base := api.ServiceURL(api.ApexProd, "id")
		if c.s != nil {
			base = c.s.URL("id")
		}
		_ = api.AbrirFuera(fyne.CurrentApp(), base+"/account")
	})
	e.TamIcono = 18 // `<Icon name="user" size={18} />`
	e.menu = c.menu
	return e
}

// entrarOSalir es «Cerrar sesión» —en rojo y separado— o, sin sesión, la entrada para
// iniciarla.
//
// Ninguna de las dos las hace esta barra: la sesión es transversal, así que entrar y
// salir es de las trece y lo resuelve `apps/fyne/sesion`. Salir cierra en todas en
// menos de dos segundos.
func (c *cuentaBarra) entrarOSalir(e sesion.Estado) fyne.CanvasObject {
	if !e.Dentro() {
		// El glifo es el de «Mi cuenta» porque el catálogo de iconos es CERRADO y no
		// tiene uno de entrar; el rótulo sí es el suyo (`account.signIn`, escrita en la
		// web el 2026-09-22: allí no hacía falta porque a la barra no se llega sin
		// haber entrado).
		entrar := NuevoElementoMenu(c.t.T("account.signIn", nil), iconos.Usuario, func() {
			if c.s == nil {
				return
			}
			// Entrar BLOQUEA: abre el navegador del sistema y espera la vuelta.
			go func() { _ = c.s.Entrar(context.Background()) }()
		})
		entrar.Inerte = c.s == nil
		entrar.TamIcono = 18
		entrar.menu = c.menu
		return entrar
	}
	salir := NuevoElementoMenu(c.t.T("account.signOut", nil), iconos.Salir, func() {
		if c.s != nil {
			c.salir(sesion.PendientesNotepads())
		}
	})
	salir.Peligro = true
	salir.TamIcono = 18
	salir.menu = c.menu
	return salir
}

// salir avisa, antes de cerrar sesión, de lo que Notepads tiene sin subir en este equipo
// —capturas en cola y borradores—, porque Salir lo borra del disco (NAT-08, «avisar y
// luego borrar», 2026-10-16). Se elige subirlo, descartarlo o no salir. Si al subir
// queda algo (sin red, rechazado, de otra organización), se vuelve a preguntar con lo
// que quede.
func (c *cuentaBarra) salir(pendientes int) {
	if pendientes == 0 {
		c.cerrarSesion()
		return
	}
	lienzo := fyne.CurrentApp().Driver().CanvasForObject(c.boton)
	if lienzo == nil {
		c.cerrarSesion() // sin ventana no hay a quién avisar; la web tampoco pregunta
		return
	}
	var d *Dialogo
	descartar := NuevoBoton(c.t.T("account.signOutPending.discard", nil), func() {
		d.Cerrar()
		c.cerrarSesion()
	}).Con(Peligro, TallaMD)
	subir := NuevoBoton(c.t.T("account.signOutPending.upload", nil), func() {
		d.Ocupado(true)
		go func() {
			ctx, parar := context.WithTimeout(context.Background(), 30*time.Second)
			quedan := c.s.SubirNotepads(ctx)
			parar()
			fyne.Do(func() {
				d.Cerrar()
				c.salir(quedan)
			})
		}()
	}).Con(Primario, TallaMD)
	cancelar := NuevoBoton(c.t.T("ui.cancel", nil), func() { d.Cerrar() }).Con(Fantasma, TallaMD)
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	escalon := tema.Escalon{Tam: tema.TextoSM.Tam, Linea: 1.625 * tema.TextoSM.Tam} // el párrafo de useConfirm
	cuerpo := NuevoParrafo(c.t.T("account.signOutPending.body", map[string]any{"count": pendientes}), escalon, Normal, "text", v)
	d = NuevoDialogo(c.t.T("account.signOutPending.title", nil), cuerpo).
		Con(DialogoSM, container.New(Fila{Hueco: HuecoPieDialogo}, cancelar, descartar, subir))
	d.cancelar, d.confirmar = cancelar, subir // para Ocupado
	d.RotuloCerrar = c.t.T("ui.close", nil)
	cuerpo.A(AnchoCuerpoDialogo(d, lienzo.Size()))
	d.Mostrar(lienzo)
}

// cerrarSesion conserva el aviso hasta que la persona lo cierre o reintente. No
// muestra rutas ni errores del llavero: el motivo operativo queda en el registro.
func (c *cuentaBarra) cerrarSesion() {
	c.mu.Lock()
	if c.cerrando {
		c.mu.Unlock()
		return
	}
	c.cerrando = true
	c.mu.Unlock()
	go func() {
		err := c.s.SalirConError()
		if err != nil {
			slog.Warn("ui: cierre de sesión compartida incompleto", "error", err)
		}
		fyne.Do(func() {
			defer func() { c.mu.Lock(); c.cerrando = false; c.mu.Unlock() }()
			if err != nil {
				c.mostrarFalloCierre()
			}
		})
	}()
}

func (c *cuentaBarra) mostrarFalloCierre() {
	lienzo := fyne.CurrentApp().Driver().CanvasForObject(c.boton)
	if lienzo == nil {
		return
	}
	var d *Dialogo
	cerrar := NuevoBoton(c.t.T("ui.close", nil), func() { d.Cerrar() }).Con(Fantasma, TallaMD)
	reintentar := NuevoBoton(c.t.T("action.retry", nil), func() { d.Cerrar(); c.cerrarSesion() }).Con(Primario, TallaMD)
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	escalon := tema.Escalon{Tam: tema.TextoSM.Tam, Linea: 1.625 * tema.TextoSM.Tam}
	cuerpo := NuevoParrafo(c.t.T("account.signOutFailed", nil), escalon, Normal, "text", v)
	d = NuevoDialogo(c.t.T("account.signOut", nil), cuerpo).
		Con(DialogoSM, container.New(Fila{Hueco: HuecoPieDialogo}, cerrar, reintentar))
	d.RotuloCerrar = c.t.T("ui.close", nil)
	cuerpo.A(AnchoCuerpoDialogo(d, lienzo.Size()))
	d.Mostrar(lienzo)
}
