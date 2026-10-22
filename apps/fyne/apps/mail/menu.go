// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package mail

import (
	"math"

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

	AlElegirCarpeta      func(Carpeta)
	AlElegirEtiqueta     func(Etiqueta)
	AlElegirCompartido   func(Compartido)
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

	secciones := []fyne.CanvasObject{m.seccionCarpetas(v), m.seccionEtiquetas(v)}
	if len(m.Compartidos) > 0 {
		secciones = append(secciones, m.seccionCompartidos(v))
	}
	if !m.SinConexion {
		secciones = append(secciones, m.seccionEnvios(v), m.seccionAjustes(v))
	}
	// `ui.Columna` sin hueco: cada sección ya trae su `mb-4` dentro, así que meterlas
	// en otra `SeccionLateral` sumaría un margen de más al final del cuerpo.
	// `overflow-y-auto`: el cuerpo desplaza y no empuja al pie. Lo decide quien monta
	// el menú, que es lo que dice `ui.Lateral`: meter un scroll dentro del widget
	// traería la barra del tema de Fyne a los seis menús de la suite a la vez.
	cuerpo := container.NewVScroll(container.New(ui.Columna{}, secciones...))

	l := ui.NuevoLateral(t.T("ui.mainNav", nil), cuerpo).ConCabecera(redactar)
	l.Pie = m.pie()
	return l
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

// ajustes es el orden EXACTO de MailApp.tsx. Se pliega de serie porque seis entradas
// abiertas hacían el lateral más largo que la lista de carpetas y se leían como
// carpetas.
var ajustes = []entradaAjuste{
	{clave: "external-mail", texto: "externalMail.title", icono: iconos.Usuario},
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
