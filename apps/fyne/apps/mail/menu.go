// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package mail

import (
	"math"
	"slices"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"

	"git.sytue.eu/sytue/one/apps/fyne/iconos"
	"git.sytue.eu/sytue/one/apps/fyne/tema"
	"git.sytue.eu/sytue/one/apps/fyne/ui"
	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/i18n"
)

// El menú lateral de Sytue Mail. NO se construye nada aquí: el carril, la sección, la
// fila, el botón de acción y el medidor son de `apps/fyne/ui` y están clavados contra
// el DOM. Esto solo dice QUÉ filas hay y en qué orden, que es lo único de Mail.
//
// Medido en producción (1280×800): carril 260, botón «Redactar» 231 × 31,5 en
// y = 66,5, encabezado de sección en y = 122,5, primera fila en y = 145,25 y las filas
// a 33,25 con 1,75 entre ellas. Todo eso lo pone `ui.Lateral`; aquí solo se comprueba.

// Menu son los datos que el lateral necesita. Es una estructura y no una función con
// doce argumentos porque la app la rellena entera en cada repintado.
type Menu struct {
	T      *i18n.Translator
	Locale i18n.Locale

	Carpetas  []Carpeta
	Etiquetas []Etiqueta
	// Compartidos son los buzones ajenos con acceso; sin ninguno, no hay sección.
	Compartidos  []Compartido
	CompartidoID string
	// Unificados y Cuentas son el árbol de buzones (`arbolDeBuzones`): «Buzones» arriba
	// —Todos los buzones de entrada, Enviados…— y cada cuenta externa con sus carpetas.
	Unificados []BuzonUnificado
	Cuentas    []CuentaDeBuzones
	// TituloSytue nombra el buzón de Sytue entre las fuentes de un unificado: la
	// dirección principal, o «Buzón de Sytue» mientras no se sabe.
	TituloSytue string
	// Externa es la carpeta externa, o el buzón unificado, en el panel
	// principal: se marca a la vista. Nil, una vista de Sytue.
	Externa *VistaExterna
	// Ramas son las ramas del árbol que la persona ha plegado o desplegado, por su
	// clave (claveRol, claveCuenta): cada una arranca como dice su sección.
	Ramas map[string]bool
	// EstadoExternas es el de su sección: Error pinta el aviso con reintento, como la
	// web; mientras carga no se pinta nada más que «Añadir cuenta de correo».
	EstadoExternas Estado
	// EstadoCarpetas es el de la sección «Carpetas»: la web pinta un cargador, un
	// aviso con reintento o «no hay carpetas», y no la lista vacía en los tres casos.
	EstadoCarpetas Estado
	// CarpetaID y EtiquetaID marcan la fila activa; la etiqueta gana, como en la web.
	CarpetaID  string
	EtiquetaID string
	// SinConexion esconde lo que no se puede hacer sin servidor, igual que la web:
	// gestionar carpetas y etiquetas, programados y todos los ajustes.
	SinConexion bool
	// AjustesAbiertos es el plegado de la sección de ajustes, cerrada de serie.
	AjustesAbiertos bool

	// Cuota alimenta el medidor del pie. Los textos llegan ya escritos porque el «%»
	// y el «de» son del idioma; los calcula Cuota. EstadoCuota es Cargando mientras
	// no ha contestado el servidor, Error si falló y Vacio si la cuenta no tiene
	// límite, que es cuando la web no pinta nada.
	EstadoCuota                          Estado
	CuotaFraccion                        float64
	CuotaPorcentaje, CuotaUso, CuotaNota string
	// Importar es «Trae tu correo» (`ImportData`), encima del medidor. Nil, o sin nada
	// que ofrecer, no ocupa sitio.
	Importar *ui.ImportarDatos

	AlElegirCarpeta    func(Carpeta)
	AlElegirEtiqueta   func(Etiqueta)
	AlElegirCompartido func(Compartido)
	// AlAbrirExterna abre el buzón de una cuenta externa; con carpeta, en ella. Sin
	// carpeta es la cuenta en el diálogo, para volver a iniciar sesión.
	AlAbrirExterna  func(cuenta, carpeta string)
	AlAnadirExterna func()
	// AlAbrirUnificado abre un buzón unificado («Todos los buzones de entrada», «Todos
	// los enviados», «Todos los borradores») en el panel principal, por su rol.
	AlAbrirUnificado func(rol string)
	// AlPlegarRama pliega o despliega una rama del árbol (Ramas).
	AlPlegarRama         func(clave string)
	AlReintentarExternas func()
	AlRedactar           func()
	AlGestionarCarpetas  func()
	AlGestionarEtiquetas func()
	AlPlegarAjustes      func()
	AlAbrir              func(string) // una entrada de ajustes, por su clave
	AlReintentarCarpetas func()
	// AlRefrescar es «Actualizar», que solo lleva la carpeta que se está viendo: es la
	// única lista que puede recargar, y repetirlo en las seis haría de la barra una
	// hilera de iconos que hay que leer para saber cuál sirve.
	AlRefrescar func()
	// CargandoLista es el `loadingList` de la web: el «Actualizar» gira y se apaga
	// mientras la carpeta carga. Lo pone la pantalla al refrescar.
	CargandoLista bool
}

// Construir devuelve el `ui.Lateral` ya montado.
func (m *Menu) Construir() *ui.Lateral {
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	t := m.T

	// `leadingIcon={<Icon name="plus" />}`: el «+» de 20 (el `size` por defecto de
	// Icon.tsx) y el `gap-2`, centrados como bloque en la caja `fullWidth`.
	redactar := ui.NuevoBoton(t.T("nav.compose", nil), m.AlRedactar)
	redactar.Ancho = ui.AnchoUtilMenu
	redactar.Icono = iconos.Mas

	// `ui.Columna` sin hueco: cada sección ya trae su `mb-4` dentro, así que meterlas
	// en otra `SeccionLateral` sumaría un margen de más al final del cuerpo.
	// `overflow-y-auto`: el cuerpo desplaza y no empuja al pie. Lo decide quien monta
	// el menú, que es lo que dice `ui.Lateral`: meter un scroll dentro del widget
	// traería la barra del tema de Fyne a los seis menús de la suite a la vez.
	cuerpo := container.NewVScroll(container.New(ui.Columna{}, m.secciones(v)...))

	l := ui.NuevoLateral(t.T("ui.mainNav", nil), cuerpo).ConCabecera(redactar)
	l.Pie = m.pie()
	return l
}

// secciones son las del cuerpo, en el orden de MailApp.tsx. «Buzones» va arriba si hay
// alguna cuenta externa que juntar (`MailboxesSection`); después «Cuentas de correo
// externas», SIEMPRE, con «Añadir cuenta de correo» al final, como
// `ExternalAccountsSection` en la web. Antes iba después de «Etiquetas», solo con
// alguna cuenta y sin «Añadir»: el alta estaba escondida en los ajustes plegados.
func (m *Menu) secciones(v tema.Variante) []fyne.CanvasObject {
	var secciones []fyne.CanvasObject
	if b := m.seccionBuzones(v); b != nil {
		secciones = append(secciones, b)
	}
	secciones = append(secciones, m.seccionExternas(v), m.seccionCarpetas(v), m.seccionEtiquetas(v))
	if len(m.Compartidos) > 0 {
		secciones = append(secciones, m.seccionCompartidos(v))
	}
	if !m.SinConexion {
		secciones = append(secciones, m.seccionEnvios(v), m.seccionAjustes(v))
	}
	return secciones
}

// seccionCarpetas es la lista de carpetas en sus estados: cargando, error con
// reintento, vacía y la lista.
func (m *Menu) seccionCarpetas(v tema.Variante) *ui.SeccionLateral {
	t := m.T
	s := ui.NuevaSeccionLateral(t.T("nav.folders", nil))
	if !m.SinConexion && m.EstadoCarpetas != Error {
		s.Acciones = []fyne.CanvasObject{ui.NuevaAccionLateralCon(
			t.T("nav.manageFolders", nil),
			iconos.Trazo(iconos.Ajustes, iconos.TallaSM, "sidebar-fg-muted", v),
			m.AlGestionarCarpetas,
		)}
	}
	switch m.EstadoCarpetas {
	case Cargando:
		// `flex items-center justify-center gap-2 py-6`: la rueda de 16 y «Cargando…»,
		// centradas, en el tono apagado del cromo.
		rueda := ui.NuevaRueda(16, "sidebar-fg-muted", v)
		rueda.Rotulo = t.T("nav.loadingFolders", nil)
		s.Filas = []fyne.CanvasObject{container.New(ui.Margen{Arr: tema.Espacio(6), Abj: tema.Espacio(6)},
			container.NewCenter(container.New(ui.Fila{Hueco: tema.Espacio(2)}, rueda,
				ui.NuevoRotulo(t.T("common.loading", nil), tema.TextoSM, ui.Normal, "sidebar-fg-muted"))))}
	case Error:
		// Sobre la tinta del menú, el aro del cromo, como en MailApp.tsx.
		reintentar := ui.NuevoBoton(t.T("common.retry", nil), m.AlReintentarCarpetas).
			Con(ui.Secundario, ui.TallaSM)
		reintentar.TokenAro = "sidebar-focus"
		s.Filas = []fyne.CanvasObject{
			ui.NuevoRotulo(t.T("nav.foldersError", nil), tema.TextoSM, ui.Normal, "sidebar-fg"),
			reintentar,
		}
	case SinConexion, SinPermiso, Vacio:
		s.Filas = []fyne.CanvasObject{
			ui.NuevoRotulo(t.T("nav.foldersEmpty", nil), tema.TextoSM, ui.Normal, "sidebar-fg-muted"),
		}
	case Listo:
		for _, c := range m.Carpetas {
			s.Filas = append(s.Filas, m.filaCarpeta(c, v))
		}
	}
	return s
}

func (m *Menu) filaCarpeta(c Carpeta, v tema.Variante) *ui.FilaLateral {
	token := "sidebar-fg"
	if c.ID == m.CarpetaID && m.EtiquetaID == "" {
		token = "sidebar-active-fg"
	}
	f := ui.NuevaFilaLateral(
		RotuloCarpeta(m.T, c.Nombre, c.Rol),
		Glifo(GlifoCarpeta(c.Rol), ui.GlifoMenu, token, v).Objeto(),
		func() { m.AlElegirCarpeta(c) },
	)
	f.Activa = token == "sidebar-active-fg"
	if c.SinLeer > 0 {
		f.Contador = numero(c.SinLeer)
	}
	if f.Activa && m.AlRefrescar != nil {
		// La `action` de SidebarItem: en `absolute right-1` con `pr-9` en la fila, y
		// girando mientras la carpeta carga (`animate-spin`, `disabled={loadingList}`).
		a := ui.NuevaAccionLateralCon(m.T.T("nav.refresh", nil),
			iconos.Trazo(iconos.Rotar, iconos.TallaSM, "sidebar-fg-muted", v), m.AlRefrescar)
		a.Girando, a.Inerte = m.CargandoLista, m.CargandoLista
		f.Accion = a
	}
	return f
}

// seccionEtiquetas: la etiqueta lleva un PUNTO de color en vez de icono, y su color
// es un dato de la cuenta, no un token del tema.
func (m *Menu) seccionEtiquetas(v tema.Variante) *ui.SeccionLateral {
	t := m.T
	s := ui.NuevaSeccionLateral(t.T("nav.labels", nil))
	if !m.SinConexion {
		s.Acciones = []fyne.CanvasObject{ui.NuevaAccionLateralCon(
			t.T("nav.manageLabels", nil),
			iconos.Trazo(iconos.Ajustes, iconos.TallaSM, "sidebar-fg-muted", v),
			m.AlGestionarEtiquetas,
		)}
	}
	for _, e := range m.Etiquetas {
		f := ui.NuevaFilaLateral(e.Nombre, nil, func() { m.AlElegirEtiqueta(e) })
		f.Punto = ColorEtiqueta(e.Color, v)
		f.Activa = e.ID == m.EtiquetaID
		if e.SinLeer > 0 {
			f.Contador = numero(e.SinLeer)
		}
		s.Filas = append(s.Filas, f)
	}
	return s
}

// seccionCompartidos es «Compartidos»: el icono de equipo o de persona y la dirección.
func (m *Menu) seccionCompartidos(v tema.Variante) *ui.SeccionLateral {
	s := ui.NuevaSeccionLateral(m.T.T("nav.shared", nil))
	for _, c := range m.Compartidos {
		token := "sidebar-fg"
		if c.ID == m.CompartidoID {
			token = "sidebar-active-fg"
		}
		icono := iconos.Usuarios
		if c.Delegado {
			icono = iconos.Usuario
		}
		f := ui.NuevaFilaLateral(c.Direccion, iconos.Trazo(icono, ui.GlifoMenu, token, v).Objeto(), func() {
			if m.AlElegirCompartido != nil {
				m.AlElegirCompartido(c)
			}
		})
		f.Activa = c.ID == m.CompartidoID
		s.Filas = append(s.Filas, f)
	}
	return s
}

// iconoExterna es el glifo de una cuenta externa en el lateral: la nube para iCloud,
// la persona para las demás (`ExternalAccountsSection` en la web).
func iconoExterna(c CuentaDeBuzones) iconos.Icono {
	if c.ICloud {
		return iconos.Nube
	}
	return iconos.Usuario
}

// Las claves de las ramas del árbol en Menu.Ramas: un buzón unificado por su rol y una
// cuenta por su instancia, que no se pueden confundir.
func claveRol(rol string) string                        { return "rol:" + rol }
func claveCuenta(id string) string                      { return "cuenta:" + id }
func (m *Menu) abierta(clave string, deSerie bool) bool { return m.Ramas[clave] != deSerie }

// rotulosUnificados son los nombres de los buzones de «Buzones» (`UNIFIED_KEYS`).
var rotulosUnificados = map[string]string{
	"inbox":   "nav.allInboxes",
	"drafts":  "nav.allDrafts",
	"sent":    "nav.allSent",
	"archive": "nav.allArchive",
	"junk":    "nav.allJunk",
	"trash":   "nav.allTrash",
}

// rama es una fila que se despliega, como la de Apple Mail con su triángulo
// (`SidebarItem` con `expanded`): el chevron al final, del mismo tamaño que el del
// rótulo plegable de la sección, y pulsarla solo pliega, así que no recoge el cajón.
// Abierta, devuelve debajo lo que cuelga de ella, sangrado hasta su rótulo (`ml-7`).
// Es `ui.RamaDesplegable`, que subió de aquí cuando Calendar la necesitó.
func (m *Menu) rama(f *ui.FilaLateral, clave string, abierta bool, hijos []fyne.CanvasObject, v tema.Variante) []fyne.CanvasObject {
	return ui.RamaDesplegable(f, abierta, func() {
		if m.AlPlegarRama != nil {
			m.AlPlegarRama(clave)
		}
	}, v, hijos...)
}

// ramaConDestino es la fila que es un destino y además se despliega, como «Todos los
// buzones de entrada» en Apple Mail (`SidebarItem` con `onToggle`): pulsarla abre su
// vista y recoge el cajón; su triángulo, aparte —la acción de la fila, en la columna de
// «Actualizar»—, pliega y despliega sin abrir nada.
func (m *Menu) ramaConDestino(f *ui.FilaLateral, clave, rotuloTriangulo string, abierta bool, hijos []fyne.CanvasObject, v tema.Variante) []fyne.CanvasObject {
	galon := iconos.ChevronDerecha
	if abierta {
		galon = iconos.ChevronAbajo
	}
	f.Accion = ui.NuevaAccionLateralCon(rotuloTriangulo, iconos.Trazo(galon, ui.LadoGalon, "sidebar-fg-muted", v), func() {
		if m.AlPlegarRama != nil {
			m.AlPlegarRama(clave)
		}
	})
	if !abierta || len(hijos) == 0 {
		return []fyne.CanvasObject{f}
	}
	return []fyne.CanvasObject{f, container.New(ui.Margen{Izq: tema.Espacio(7)},
		container.New(ui.Columna{Hueco: tema.Espacio(0.5)}, hijos...))}
}

// enVista dice si esa carpeta de esa cuenta es la del panel principal.
func (m *Menu) enVista(cuenta, carpeta string) bool {
	return m.Externa != nil && !m.Externa.Mezclada() && m.Externa.Cuenta == cuenta && m.Externa.Carpeta == carpeta
}

// tinta es el token del glifo de una fila: el del activo si es la que se ve.
func tinta(activa bool) string {
	if activa {
		return "sidebar-active-fg"
	}
	return "sidebar-fg"
}

func (m *Menu) abrirExterna(cuenta, carpeta string) func() {
	return func() {
		if m.AlAbrirExterna != nil {
			m.AlAbrirExterna(cuenta, carpeta)
		}
	}
}

// seccionBuzones es «Buzones», arriba del todo como en Apple Mail: Todos los buzones de
// entrada, Enviados… de todas las cuentas, Sytue incluida, con la suma de sus no leídos.
// Solo si alguna cuenta externa aporta una carpeta: con Sytue sola (sin cuentas, o
// todas pidiendo volver a iniciar sesión) sería «Carpetas» dos veces. Cada uno se
// despliega en la carpeta de cada cuenta. «Todos los buzones de entrada», «Todos los
// enviados» y «Todos los borradores» son además buzones en sí (rolesMezclados): la
// fila abre esa carpeta de todas las cuentas mezclada, y su triángulo, aparte,
// pliega; Archivo, Spam y Papelera solo se despliegan. Es `MailboxesSection` de la web.
func (m *Menu) seccionBuzones(v tema.Variante) *ui.SeccionLateral {
	if !slices.ContainsFunc(m.Unificados, func(u BuzonUnificado) bool {
		return slices.ContainsFunc(u.Fuentes, func(f FuenteDeBuzon) bool { return f.Cuenta != cuentaSytue })
	}) {
		return nil
	}
	s := ui.NuevaSeccionLateral(m.T.T("nav.mailboxes", nil))
	for _, u := range m.Unificados {
		var hijos []fyne.CanvasObject
		for _, fuente := range u.Fuentes {
			if fuente.Cuenta == cuentaSytue {
				hijos = append(hijos, m.fuenteSytue(fuente.Carpeta, v))
				continue
			}
			i := slices.IndexFunc(m.Cuentas, func(c CuentaDeBuzones) bool { return c.ID == fuente.Cuenta })
			if i < 0 {
				continue
			}
			c := m.Cuentas[i]
			// Marcada a la vista; la «página actual» es la de su cuenta, más abajo.
			activa := m.enVista(c.ID, fuente.Carpeta)
			f := ui.NuevaFilaLateral(c.Titulo, iconos.Trazo(iconoExterna(c), ui.GlifoMenu, tinta(activa), v).Objeto(), m.abrirExterna(c.ID, fuente.Carpeta))
			f.Activa = activa
			if j := slices.IndexFunc(c.Carpetas, func(f api.ExternalMailFolder) bool { return f.ID == fuente.Carpeta }); j >= 0 && c.Carpetas[j].Unread > 0 {
				f.Contador = numero(int(c.Carpetas[j].Unread))
			}
			hijos = append(hijos, f)
		}
		clave := claveRol(u.Rol)
		if esMezclado(u.Rol) {
			// «Todos los buzones de entrada», «Todos los enviados» y «Todos los
			// borradores» son buzones en sí: esa carpeta de todas las cuentas, mezclada
			// por fecha (fase B, punto 5). Archivo, Spam y Papelera solo se despliegan.
			rol := u.Rol
			activa := m.Externa != nil && m.Externa.Unificado == rol
			f := ui.NuevaFilaLateral(m.T.T(rotulosUnificados[rol], nil), Glifo(GlifoCarpeta(rol), ui.GlifoMenu, tinta(activa), v).Objeto(), func() {
				if m.AlAbrirUnificado != nil {
					m.AlAbrirUnificado(rol)
				}
			})
			f.Activa = activa
			if u.NoLeidos > 0 {
				f.Contador = numero(int(u.NoLeidos))
			}
			s.Filas = append(s.Filas, m.ramaConDestino(f, clave, m.T.T("nav.accountsIn", i18n.Vars{"mailbox": f.Rotulo}), m.abierta(clave, false), hijos, v)...)
			continue
		}
		f := ui.NuevaFilaLateral(m.T.T(rotulosUnificados[u.Rol], nil), Glifo(GlifoCarpeta(u.Rol), ui.GlifoMenu, "sidebar-fg", v).Objeto(), nil)
		if u.NoLeidos > 0 {
			f.Contador = numero(int(u.NoLeidos))
		}
		s.Filas = append(s.Filas, m.rama(f, clave, m.abierta(clave, false), hijos, v)...)
	}
	return s
}

// fuenteSytue es la carpeta de Sytue dentro de un buzón unificado: elegirla es elegirla
// en «Carpetas», y se marca igual a la vista (en la web, sin `aria-current`: esa es la
// de «Carpetas»). Lleva sus no leídos, para que la suma de la rama cuadre debajo.
func (m *Menu) fuenteSytue(id string, v tema.Variante) *ui.FilaLateral {
	activa := id == m.CarpetaID && m.EtiquetaID == "" && m.CompartidoID == "" && m.Externa == nil
	token := "sidebar-fg"
	if activa {
		token = "sidebar-active-fg"
	}
	titulo := m.TituloSytue
	if titulo == "" {
		titulo = m.T.T("nav.sytueMailbox", nil)
	}
	f := ui.NuevaFilaLateral(titulo, iconos.Trazo(iconos.Correo, ui.GlifoMenu, token, v).Objeto(), func() {
		if i := slices.IndexFunc(m.Carpetas, func(c Carpeta) bool { return c.ID == id }); i >= 0 && m.AlElegirCarpeta != nil {
			m.AlElegirCarpeta(m.Carpetas[i])
		}
	})
	f.Activa = activa
	if i := slices.IndexFunc(m.Carpetas, func(c Carpeta) bool { return c.ID == id }); i >= 0 && m.Carpetas[i].SinLeer > 0 {
		f.Contador = numero(m.Carpetas[i].SinLeer)
	}
	return f
}

// seccionExternas lista las cuentas externas como en Apple Mail, con el aviso si no
// cargaron y «Añadir cuenta de correo» como última fila. Cada cuenta es una rama que se
// pliega: su icono (la nube de iCloud), su dirección y, plegada, los no leídos de su
// Entrada; abierta, sus carpetas traducidas con su glifo y su contador. La que perdió el
// inicio de sesión lo dice en lugar de sus carpetas, y la que el proveedor no contestó
// deja reintentar. Es `ExternalAccountsSection` de la web.
func (m *Menu) seccionExternas(v tema.Variante) *ui.SeccionLateral {
	t := m.T
	s := ui.NuevaSeccionLateral(t.T("externalMail.title", nil))
	if m.EstadoExternas == Error {
		// El mismo aviso que «Carpetas»: sobre la tinta del menú, el aro del cromo.
		reintentar := ui.NuevoBoton(t.T("common.retry", nil), m.AlReintentarExternas).
			Con(ui.Secundario, ui.TallaSM)
		reintentar.TokenAro = "sidebar-focus"
		s.Filas = append(s.Filas,
			ui.NuevoRotulo(t.T("externalMail.accountsError", nil), tema.TextoSM, ui.Normal, "sidebar-fg"),
			reintentar,
		)
	}
	for _, c := range m.Cuentas {
		clave := claveCuenta(c.ID)
		abierta := m.abierta(clave, true)
		var hijos []fyne.CanvasObject
		switch c.Estado {
		case api.ExternalMailReconnect:
			hijos = append(hijos, ui.NuevaFilaLateral(t.T("externalMail.reconnectNeeded", nil),
				iconos.Trazo(iconos.Aviso, ui.GlifoMenu, "sidebar-fg", v).Objeto(), m.abrirExterna(c.ID, "")))
		case api.ExternalMailUnavailable:
			hijos = append(hijos, ui.NuevaFilaLateral(t.T("externalMail.accountUnavailable", nil),
				iconos.Trazo(iconos.Rotar, ui.GlifoMenu, "sidebar-fg", v).Objeto(), func() {
					if m.AlReintentarExternas != nil {
						m.AlReintentarExternas()
					}
				}))
		default:
			for _, carpeta := range c.Carpetas {
				activa := m.enVista(c.ID, carpeta.ID)
				f := ui.NuevaFilaLateral(RotuloCarpeta(t, carpeta.Name, carpeta.Role),
					Glifo(GlifoCarpeta(carpeta.Role), ui.GlifoMenu, tinta(activa), v).Objeto(), m.abrirExterna(c.ID, carpeta.ID))
				f.Activa = activa
				if carpeta.Unread > 0 {
					f.Contador = numero(int(carpeta.Unread))
				}
				hijos = append(hijos, f)
			}
		}
		// El icono, en el `size` por defecto de Icon.tsx como en la web.
		f := ui.NuevaFilaLateral(c.Titulo, iconos.Trazo(iconoExterna(c), ui.GlifoMenu, "sidebar-fg", v).Objeto(), nil)
		if !abierta && c.NoLeidos > 0 {
			// Abierta, ya lo dice su Entrada.
			f.Contador = numero(int(c.NoLeidos))
		}
		s.Filas = append(s.Filas, m.rama(f, clave, abierta, hijos, v)...)
	}
	s.Filas = append(s.Filas, ui.NuevaFilaLateral(
		t.T("externalMail.add", nil),
		iconos.Trazo(iconos.Mas, ui.GlifoMenu, "sidebar-fg", v).Objeto(),
		func() {
			if m.AlAnadirExterna != nil {
				m.AlAnadirExterna()
			}
		},
	))
	return s
}

func (m *Menu) seccionEnvios(v tema.Variante) *ui.SeccionLateral {
	t := m.T
	f := ui.NuevaFilaLateral(
		t.T("nav.scheduled", nil),
		Glifo(RelojD, ui.GlifoMenu, "sidebar-fg", v).Objeto(),
		func() { m.AlAbrir("scheduled") },
	)
	return ui.NuevaSeccionLateral(t.T("nav.sending", nil), f)
}

// entradaAjuste es una fila de la sección plegable de ajustes.
//
// El icono puede venir del catálogo compartido (`icono`) o ser uno de los glifos
// propios de esta app (`propio`, una ruta de `glifos.go`). No es un `any` ni una
// interfaz: son dos campos, y el que esté puesto manda.
type entradaAjuste struct {
	clave  string
	texto  string
	icono  iconos.Icono
	propio string
}

// ajustes son las entradas de «Ajustes» de MailApp.tsx, en su orden. Se pliega de
// serie porque seis entradas abiertas hacían el lateral más largo que la lista de
// carpetas y se leían como carpetas. Las cuentas externas no están aquí: tienen su
// sección arriba, con su «Añadir», como en la web.
var ajustes = []entradaAjuste{
	{clave: "signature", texto: "nav.signature", icono: iconos.Usuario},
	{clave: "rules", texto: "nav.rules", icono: iconos.Ajustes},
	{clave: "aliases", texto: "nav.aliases", icono: iconos.Usuarios},
	{clave: "shared", texto: "nav.sharedAndDelegation", icono: iconos.Rejilla},
	{clave: "smime-anchor", texto: "nav.smimeAnchor", icono: iconos.Descargar},
	{clave: "smime-identity", texto: "nav.smimeIdentity", icono: iconos.Subir},
	{clave: "vacation", texto: "nav.vacation", propio: RelojD},
	{clave: "mail-clients", texto: "nav.mailClients", icono: iconos.Ajustes},
}

// dibujo es el glifo de la entrada, venga del catálogo o de `glifos.go`.
func (a entradaAjuste) dibujo(v tema.Variante) *iconos.Dibujo {
	if a.propio != "" {
		return Glifo(a.propio, ui.GlifoMenu, "sidebar-fg", v)
	}
	return iconos.Trazo(a.icono, ui.GlifoMenu, "sidebar-fg", v)
}

func (m *Menu) seccionAjustes(v tema.Variante) *ui.SeccionLateral {
	s := ui.NuevaSeccionLateral(m.T.T("nav.settings", nil))
	s.AlPlegar = m.AlPlegarAjustes
	s.Abierta = m.AjustesAbiertos
	if !s.Abierta {
		// La sección plegada no MONTA sus filas. `ui.SeccionLateral` las redimensiona
		// a cero al plegarse, y un `canvas.Text` a cero sigue pintando su texto: la
		// lista de ajustes se veía entera debajo del rótulo plegado. Está en el
		// informe como fallo de `ui`; aquí se evita no dándoselas.
		return s
	}
	for _, a := range ajustes {
		s.Filas = append(s.Filas, ui.NuevaFilaLateral(
			m.T.T(a.texto, nil),
			a.dibujo(v).Objeto(),
			func() { m.AlAbrir(a.clave) },
		))
	}
	return s
}

// pie es el `space-y-2` del pie de la web: «Trae tu correo» (`ImportData`), si la
// organización tiene de dónde, y debajo el medidor de cuota.
func (m *Menu) pie() fyne.CanvasObject {
	med := m.medidor()
	if m.Importar == nil {
		return med
	}
	traer := m.Importar.Objeto(ui.AnchoUtilMenu, tema.De(fyne.CurrentApp().Settings().ThemeVariant()))
	switch {
	case traer == nil:
		return med
	case med == nil:
		return traer
	}
	return container.New(ui.Columna{Hueco: tema.Espacio(2)}, traer, med)
}

// medidor es el medidor de cuota sobre el cromo oscuro, en los cuatro estados de
// `StorageMeter.tsx`.
func (m *Menu) medidor() fyne.CanvasObject {
	switch m.EstadoCuota {
	case Cargando, Error:
		// Un aviso discreto y no una rueda: es un indicador de apoyo, no bloquea el
		// correo. `px-1 text-xs` en el tono apagado del cromo.
		clave := "storage.calculating"
		if m.EstadoCuota == Error {
			clave = "storage.unavailable"
		}
		return container.New(ui.Margen{Izq: tema.Espacio(1), Der: tema.Espacio(1)},
			ui.NuevoRotulo(m.T.T(clave, nil), tema.TextoXS, ui.Normal, "sidebar-fg-muted"))
	case Vacio, SinConexion, SinPermiso:
		return nil
	case Listo:
	}
	med := ui.NuevoMedidor(m.T.T("storage.title", nil), m.CuotaFraccion, m.CuotaPorcentaje)
	med.Cuanto = m.CuotaUso
	med.Nota = m.CuotaNota
	med.SobreCromo = true
	return med
}

// Umbrales de `StorageMeter.tsx`: ámbar de aviso y rojo de lleno.
const (
	cuotaAviso = 0.8
	cuotaLlena = 0.95
)

// Cuota escribe el medidor: la fracción, el porcentaje ENTERO del idioma («42 %»), el
// uso con su «de {total}» y la nota que dice en palabras que se acaba el sitio (el
// color solo no basta: WCAG 1.4.1). Un límite a cero es «sin cuota» y devuelve Vacio.
func Cuota(t *i18n.Translator, l i18n.Locale, q api.MailQuota) (e Estado, frac float64, pct, uso, nota string) {
	if q.QuotaBytes <= 0 {
		return Vacio, 0, "", "", ""
	}
	frac = min(float64(q.UsedBytes)/float64(q.QuotaBytes), 1)
	switch {
	case frac >= cuotaLlena:
		nota = t.T("storage.full", nil)
	case frac >= cuotaAviso:
		nota = t.T("storage.almostFull", nil)
	}
	// `maximumFractionDigits: 0`: se redondea ANTES, y el formateador del idioma pone
	// el signo y su espacio duro.
	pct = i18n.FormatPercent(l, math.Round(frac*100)/100)
	uso = i18n.FormatBytes(l, float64(q.UsedBytes)) + " " + t.T("storage.of", i18n.Vars{"total": i18n.FormatBytes(l, float64(q.QuotaBytes))})
	return Listo, frac, pct, uso, nota
}
