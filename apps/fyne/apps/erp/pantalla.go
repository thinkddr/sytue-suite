// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package erp

import (
	"context"
	"slices"
	"sync"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"

	"git.sytue.eu/sytue/one/apps/fyne/iconos"
	"git.sytue.eu/sytue/one/apps/fyne/tema"
	"git.sytue.eu/sytue/one/apps/fyne/ui"
	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/i18n"
	"git.sytue.eu/sytue/one/apps/nucleo/offline"
)

// Pantalla es Sytue ERP entero: el menú y lo que ocupe el panel principal.
//
// Es ESTADO, no vista: la app cambia sus campos y llama a Repintar. Todo lo que hay
// aquí son datos y punteros a funciones; el armazón y las vistas ya saben pintarse, y
// por eso los guardianes de píxel no necesitan ni red ni servicio.
//
// La TOPBAR no está, y no es un olvido: es una de las dos exclusiones de paridad
// escritas en `docs/CONVENTIONS.md` §13.6.
type Pantalla struct {
	svc *api.ERP
	tr  *i18n.Translator
	red *offline.Monitor

	// Catalogo es lo que publica el servicio y Permisos lo que esta persona alcanza.
	Catalogo Catalogo
	Permisos Permisos
	// SabePermisos distingue «todavía no se sabe» de «no tiene ninguno». Sin él, el
	// menú saldría vacío mientras llega la respuesta, que es distinto de estar vacío.
	SabePermisos bool
	// Fase es la del CATÁLOGO: sin él no hay ni menú ni lista.
	Fase Fase
	// Recientes son las claves de los últimos modelos abiertos, el más reciente
	// primero y como mucho seis. En la web viven en `sessionStorage` —duran lo que la
	// pestaña—, y aquí lo que dura el proceso, que es lo mismo: no se guarda nada de
	// negocio ni cruza sesiones.
	Recientes []string

	menu    *Menu
	lista   *Lista
	inicio  *Bienvenida
	paleta  *Paleta
	abierta bool
	// ficha es el panel del registro abierto; nil, ninguno.
	ficha *Ficha
	altas []*altaRapida
	// avisos es la pila de avisos de la ventana (el `ToastProvider` de la web).
	avisos *ui.PilaAvisos
	// cajon es el menú de módulos abierto: es un cajón a cualquier ancho.
	cajon bool
	// arm es el armazón, UNO para toda la vida de la pantalla: el cajón abierto es suyo
	// —la entrada, el velo, el teclado—, y uno nuevo en cada repintado lo dejaba abierto
	// sin dueño (Escape cerraba el anterior, que ya no estaba en pantalla).
	arm *ui.Armazon
	// Organizacion es el perfil de la organización: decide Facturae y VERI*FACTU.
	// SabeOrg distingue «no hay perfil» (404) de «no se sabe» (un corte).
	Organizacion *api.ERPOrganizacion
	SabeOrg      bool
	// Requisitos es lo que tiene que llevar una factura en el país.
	Requisitos *api.ERPRequisitosFactura
	// revision sube con cada escritura: la lista vuelve a pedir su recuento.
	revision int
	// Vista es lo que ocupa el panel principal cuando no es la lista de un modelo ni la
	// portada: «tareas», «libro» o «balance» (el `vista` de la web).
	Vista string
	// peticiones son las franjas de «Pedir acceso de edición» (peticiones.go).
	peticiones modoPeticiones
	// TareasHoy son las tareas propias que vencen hoy o ya vencieron: la insignia del
	// menú.
	TareasHoy int
	tareas    *Tareas
	libro     *Libro
	// oblig es la pantalla de Obligaciones y asistente el de arranque (nil, cerrado).
	oblig     *Obligaciones
	asistente *Asistente
	banco     *Banco
	// AbrirApp abre otra app de la suite por su esquema (o la web): el correo que pide
	// una factura se escribe en Sytue Mail. nil, no se ofrece.
	AbrirApp func(producto, ruta string)
	// ElegirFicheros abre el selector de ficheros DEL SISTEMA y le pasa a `listo` las
	// rutas elegidas en el hilo de la interfaz. nil: no hay selector (el móvil, un Linux
	// sin zenity ni kdialog) y no se ofrece ni subir ni importar.
	ElegirFicheros func(listo func(rutas []string))
	// verifactu es el estado de VERI*FACTU de la tarjeta de las facturas de cliente;
	// nil, no se sabe (y no se pinta).
	verifactu   *api.ERPEstadoVerifactu
	vfActivando bool
	// Tableros son los guardados de Sytue AI, el más reciente primero; tablero, el que
	// se está viendo (nil, ninguno).
	Tableros []api.ERPTablero
	tablero  *Tablero
	// repintarIA repinta el diálogo de Sytue AI abierto (nil, ninguno): su cuerpo se
	// reparte al ancho de la ventana.
	repintarIA func()
	// panelIA es el panel de Sytue AI abierto por última vez (para los guardianes).
	panelIA *PanelIA
	// scrollMain es el desplazador de `main`: se lleva lo vertical de lo que se desplaza
	// de lado dentro de él.
	scrollMain *container.Scroll

	// cache es lo ya visto por (modelo, búsqueda, estado, orden, papelera). Volver a
	// una lista la pinta AL INSTANTE con lo que había y la refresca por detrás, que es
	// lo que hace la web con su `Map`: es la diferencia entre una app que se siente
	// instantánea y una que parpadea en cada clic. Vive lo que el proceso y se olvida
	// entera de un modelo en cuanto se escribe en él.
	cache map[string]Pagina
	// mu guarda `cache` y `espera`: el reloj de la búsqueda dispara en otra rutina.
	mu     sync.Mutex
	espera *time.Timer

	// Ventana es el tamaño del lienzo. Decide el reparto del contenido y dónde cae la
	// paleta.
	Ventana fyne.Size
	// Plazo es cuánto se espera a una petición. Se deja abierto para el guardián.
	Plazo time.Duration

	raiz *fyne.Container
	// fondo lanza el trabajo que habla con la red. Es `go f()`; los guardianes lo
	// cambian por `f()` para que la carga entera ocurra en su rutina y en orden, sin
	// relojes ni carreras.
	fondo func(func())
	// lienzo es el de la ventana, para mover el foco (la paleta al abrirse, «/»). Nil
	// hasta `Atajos`, y en los guardianes.
	lienzo fyne.Canvas
}

// NuevaPantalla monta la aplicación. `svc` puede ser nil: la pantalla se pinta igual y
// se queda en la fase que se le ponga, que es lo que hace el guardián.
func NuevaPantalla(tr *i18n.Translator, svc *api.ERP, red *offline.Monitor) *Pantalla {
	p := &Pantalla{
		svc: svc, tr: tr, red: red,
		Plazo:   30 * time.Second,
		Ventana: fyne.NewSize(1280, 800),
		lista:   NuevaLista(tr),
		inicio:  NuevaBienvenida(tr),
		menu:    &Menu{Plegadas: map[string]bool{}},
		cache:   map[string]Pagina{},
		fondo:   func(f func()) { go f() },
		avisos:  ui.NuevaPilaAvisos(),
		tareas:  NuevaTareas(tr),
		libro:   &Libro{tr: tr, loc: tr.Locale()},
		oblig:   NuevaObligaciones(tr),
		banco:   NuevoBanco(tr),
	}
	p.paleta = NuevaPaleta(tr, nil)
	p.enganchar()
	return p
}

func (p *Pantalla) t(clave string, vars ...i18n.Vars) string {
	var v i18n.Vars
	if len(vars) > 0 {
		v = vars[0]
	}
	return p.tr.T(clave, v)
}

// PuedeLeer es `puedeLeer` de ErpApp.tsx: mientras no se sepan los permisos se enseña
// todo y manda el servicio, que es quien responde 403. En cuanto se saben, un módulo
// sin nivel concedido NO se ofrece: enseñar un área que va a dar 403 es prometer algo
// que no se puede cumplir.
func (p *Pantalla) PuedeLeer(modulo string) bool {
	return !p.SabePermisos || p.Permisos.Alcanza(modulo, api.ERPLectura)
}

// AreasVisibles son los módulos ENCENDIDOS en los que esta persona tiene lectura, que
// es exactamente lo que la web pone en la barra.
func (p *Pantalla) AreasVisibles() []Modulo {
	var out []Modulo
	for _, m := range p.Catalogo.Modulos {
		if slices.Contains(p.Catalogo.Activos, m.Clave) && p.PuedeLeer(m.Clave) {
			out = append(out, m)
		}
	}
	return out
}

// Modelos son todos los del catálogo, encendidos o no: es lo que ve la paleta.
func (p *Pantalla) Modelos() []Modelo {
	var out []Modelo
	for _, m := range p.Catalogo.Modulos {
		out = append(out, m.Modelos...)
	}
	return out
}

// Objeto arma el armazón con el menú y la vista, y la paleta por encima. Vigila su
// propio tamaño: el reparto (móvil, columnas, ficha a pantalla completa) sale de él.
func (p *Pantalla) Objeto() fyne.CanvasObject {
	p.raiz = container.New(&vigia{p: p}, p.marco())
	return p.raiz
}

// esMovil es la ventana por debajo de `md` (768).
func (p *Pantalla) esMovil() bool { return p.Ventana.Width < ui.Estrecho }

func (p *Pantalla) marco() fyne.CanvasObject {
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	p.rellenarMenu()
	if p.arm == nil {
		p.arm = ui.NuevoArmazon(nil)
		// El cajón se abre con el botón del carril o con el de `main`, como en la web.
		p.arm.SinBarraMovil = true
		p.arm.AlCajon = func(abierto bool) { p.cajon = abierto }
		p.arm.AlPulsar = func() { p.cajon = false; p.Repintar() }
	}
	arm := p.arm
	// La carcasa de ErpApp.tsx: `sytue-frame` con el carril de iconos (`.erp-rail`) y el
	// `main` dentro de `sytue-sheet`, la hoja que pinta el armazón. El menú de módulos
	// (`#erp-cajon`) es un cajón a cualquier ancho; su `<nav>` desplaza lo suyo.
	arm.Contenido = p.principal(v)
	arm.Menu = desplazable(p.menu.Objeto(v))
	arm.Carril = p.carril().Objeto(v)
	if p.cajon {
		arm.AbrirCajon()
	} else {
		arm.CerrarCajon()
	}
	arm.Refresh()
	capas := []fyne.CanvasObject{arm}
	if p.abierta {
		capas = append(capas, p.paleta.Objeto(v, p.Ventana))
	}
	return container.NewStack(append(capas, p.avisos)...)
}

// principal es `main` —desplazable, con el botón del cajón arriba en el móvil— y, a su
// derecha, la ficha abierta. En el móvil la ficha va ENCIMA y a pantalla completa.
func (p *Pantalla) principal(v tema.Variante) fyne.CanvasObject {
	movil := p.esMovil()
	ancho := p.Ventana.Width
	if !movil {
		ancho -= ocupaCarcasa()
	}
	var ficha fyne.CanvasObject
	anchoFicha := float32(0)
	if p.ficha != nil && p.lista.Modelo.Clave != "" {
		anchoFicha = min(AnchoFicha, max(ancho, 0))
		if movil {
			anchoFicha = ancho
		}
		p.ficha.Movil = movil
		ficha = p.ficha.Objeto(v, anchoFicha)
	}
	if !movil {
		p.lista.Ocupado = anchoFicha
	} else {
		p.lista.Ocupado = 0
	}
	contenido := p.vista(v)
	if fs := p.franjasPeticion(); len(fs) > 0 {
		contenido = container.New(ui.Columna{}, append(fs, contenido)...)
	}
	var cajon *BotonCajon
	if movil {
		cajon = NuevoBotonCajon(p.t("nav.menu"), p.abrirCajon)
	}
	m := &mainERP{cajon: cajon, contenido: contenido}
	hijos := []fyne.CanvasObject{}
	if cajon != nil {
		hijos = append(hijos, cajon)
	}
	hijos = append(hijos, contenido)
	// ERP usa el lienzo del armazón compartido: bg en claro/oscuro y el fondo
	// canónico traslúcido, como .erp-main de app.css.
	p.scrollMain = container.NewVScroll(container.New(m, hijos...))
	desplaza := container.NewThemeOverride(p.scrollMain, temaSinSombra{})
	return container.New(&conFicha{main: desplaza, ficha: ficha, ancho: anchoFicha, movil: movil},
		quitarNil(desplaza, ficha)...)
}

// desplazadorMain es el desplazador de `main`, o nil si todavía no hay.
func (p *Pantalla) desplazadorMain() Desplazador {
	if p.scrollMain == nil {
		return nil
	}
	return p.scrollMain
}

// vista es lo que ocupa el panel principal: la lista de un modelo, o la portada.
func (p *Pantalla) vista(v tema.Variante) fyne.CanvasObject {
	p.conFicheros()
	switch p.Vista {
	case VistaTareas:
		p.tareas.Ventana = p.Ventana.Width
		return p.tareas.Objeto(v)
	case VistaLibro:
		p.libro.Ventana = p.Ventana.Width
		return p.libro.ObjetoLibro(v)
	case VistaBalance:
		p.libro.Ventana = p.Ventana.Width
		return p.libro.ObjetoBalance(v)
	case VistaAsistente:
		return p.objetoAsistente(v)
	case VistaBanco:
		b := p.banco
		b.Ventana, b.SoloLectura = p.Ventana.Width, !p.PuedeEscribir("contabilidad")
		return b.Objeto(v)
	case VistaTablero:
		if p.tablero != nil {
			p.tablero.Ventana = p.Ventana.Width
			p.tablero.Afuera = p.desplazadorMain
			return p.tablero.Objeto(v)
		}
	case VistaObligaciones:
		// Sin perfil no hay calendario que enseñar: lo que toca es el asistente.
		if p.SabeOrg && p.Organizacion == nil {
			return p.objetoAsistente(v)
		}
		o := p.oblig
		o.Ventana, o.SoloLectura = p.Ventana.Width, !p.PuedeEscribir("obligaciones")
		if p.Organizacion != nil {
			o.Pais = p.Organizacion.Pais
		}
		return o.Objeto(v)
	}
	if p.lista.Modelo.Clave != "" {
		p.lista.Ventana = p.Ventana.Width
		lista := p.lista.Objeto(v)
		if p.lista.Modelo.Clave == api.ERPFacturaVenta && p.facturae() && !p.lista.Papelera && p.verifactu != nil {
			return p.conTarjetaVF(v, lista)
		}
		return lista
	}
	b := p.inicio
	b.Modulos, b.Activos, b.Ventana, b.Fase = p.Catalogo.Modulos, p.Catalogo.Activos, p.Ventana.Width, p.Fase
	b.Tareas, b.ConPerfil, b.SabePerfil = p.TareasHoy, p.Organizacion != nil, p.SabeOrg
	b.VerObligaciones = p.PuedeLeer("obligaciones")
	b.Recientes = b.Recientes[:0]
	for _, clave := range p.Recientes {
		if m, ok := p.modeloPorClave(clave); ok {
			b.Recientes = append(b.Recientes, m)
		}
	}
	return b.Objeto(v)
}

// abrirCajon y cerrarCajon mueven el cajón de módulos por el armazón, que es quien le
// pone la entrada, el velo y el teclado.
func (p *Pantalla) abrirCajon() {
	if p.arm != nil {
		p.arm.AbrirCajon()
	}
}

func (p *Pantalla) cerrarCajon() {
	if p.arm != nil {
		p.arm.CerrarCajon()
	}
}

// ocupaCarcasa es lo que `main` no tiene de la ventana en ancho: el carril de iconos a
// su izquierda y, en Marco, lo que la hoja se separa del borde derecho.
func ocupaCarcasa() float32 { return AnchoCarril + tema.Forma("sheet-gap") }

// radioTarjeta es el `rounded-card` de las cajas del contenido: un token de forma.
func radioTarjeta() float32 { return tema.Forma("radius-card") }

// carril arma el carril de iconos con los destinos de arriba del menú que llevan icono
// —los mismos, con los mismos permisos— y, abajo, el buscador y Sytue AI.
//
// ponytail: falta el botón del TPV (`pos.title`), que en la web va entre Tareas y Banco:
// el punto de venta no está portado y aquí no hay a dónde llevarlo.
func (p *Pantalla) carril() *CarrilIconos {
	c := &CarrilIconos{Arriba: []*AccionCarril{NuevaAccionCarril(iconos.Menu, p.t("nav.menu"), false, p.abrirCajon)}}
	for _, d := range p.menu.Fijas {
		if d.Icono == iconos.Ninguno {
			continue
		}
		a := NuevaAccionCarril(d.Icono, d.Rotulo, d.Activa, d.AlPulsar)
		if a.Contador = d.Contador; d.Contador > 0 && d.Nombre != "" {
			a.Nombre = d.Nombre
		}
		c.Arriba = append(c.Arriba, a)
	}
	c.Abajo = []*AccionCarril{
		NuevaAccionCarril(iconos.Buscar, p.t("palette.placeholder"), false, p.AbrirPaleta),
		NuevaAccionCarril(iconos.IA, p.menu.RotuloIA, false, p.AbrirIA),
	}
	return c
}

func (p *Pantalla) rellenarMenu() {
	m := p.menu
	m.Areas = p.AreasVisibles()
	m.ModeloActivo = p.lista.Modelo.Clave
	m.RotuloPaleta = p.t("palette.placeholder")
	// El atajo, con la tecla del sistema: «⌘K» en un Mac y «Ctrl K» en lo demás. El
	// nombre accesible lo lleva también (VoiceOver lee «⌘» como «comando»).
	m.AtajoPaleta = ui.AtajoDelSistema("Ctrl K")
	m.NombrePaleta = m.RotuloPaleta + " — " + p.t("palette.hint", i18n.Vars{"keys": ui.AtajoDelSistema("Ctrl+K")})
	m.RotuloIA = "Sytue AI"
	m.RotuloCerrar = p.t("action.close")
	m.TituloTableros = p.t("nav.dashboards")
	m.Tableros = p.destinosTableros()
	// Arriba y fuera de los módulos: son el trabajo de cada semana de quien lleva un
	// negocio, y el libro y el balance no pertenecen a ninguno. Los que llevan Icono
	// van además en el carril de iconos (`.erp-rail`), con ese.
	m.Fijas = []Destino{
		{
			Rotulo: p.t("nav.home"), Activa: p.Vista == "" && p.lista.Modelo.Clave == "",
			Icono:    iconos.Rejilla,
			AlPulsar: func() { p.Abrir(Modelo{}) },
		},
		{
			Rotulo: p.t("nav.tasks"), Activa: p.Vista == VistaTareas, Contador: p.TareasHoy,
			Icono:    iconos.Tic,
			Nombre:   p.t("nav.tasks") + " — " + p.t("nav.tasksDue", i18n.Vars{"count": p.TareasHoy}),
			AlPulsar: func() { p.IrA(VistaTareas) },
		},
	}
	if p.PuedeLeer("contabilidad") {
		m.Fijas = append(m.Fijas, Destino{
			Rotulo: p.t("nav.bank"), Activa: p.Vista == VistaBanco, AlPulsar: func() { p.IrA(VistaBanco) },
			Icono: iconos.Edificio,
		})
	}
	if p.PuedeLeer("obligaciones") {
		m.Fijas = append(m.Fijas, Destino{
			Rotulo: p.t("nav.obligations"), Activa: p.Vista == VistaObligaciones || p.Vista == VistaAsistente,
			Icono:    iconos.Auditoria,
			AlPulsar: func() { p.IrA(VistaObligaciones) },
		})
	}
	if p.PuedeLeer("contabilidad") {
		m.Fijas = append(m.Fijas,
			Destino{Rotulo: p.t("nav.ledger"), Activa: p.Vista == VistaLibro, AlPulsar: func() { p.IrA(VistaLibro) }},
			Destino{Rotulo: p.t("nav.balance"), Activa: p.Vista == VistaBalance, AlPulsar: func() { p.IrA(VistaBalance) }},
		)
	}
}

// Las vistas que no son la lista de un modelo ni la portada.
const (
	VistaTareas       = "tareas"
	VistaLibro        = "libro"
	VistaBalance      = "balance"
	VistaObligaciones = "obligaciones"
	VistaAsistente    = "onboarding"
	VistaBanco        = "banco"
)

// IrA abre una de las vistas de arriba del menú y pide lo suyo.
func (p *Pantalla) IrA(vista string) {
	p.pararEspera()
	p.Vista, p.ficha, p.cajon, p.abierta = vista, nil, false, false
	p.lista.Modelo = Modelo{}
	switch vista {
	case VistaTareas:
		p.tareas.Lista = nil
		p.cargarTareas()
	case VistaLibro:
		p.libro.Asientos, p.libro.Fallo = nil, ""
		p.cargarLibro()
	case VistaBalance:
		p.libro.Balance, p.libro.Fallo = nil, ""
		p.cargarBalance()
	case VistaObligaciones:
		if p.SabeOrg && p.Organizacion == nil {
			p.abrirAsistente()
		} else {
			p.cargarObligaciones()
		}
	case VistaAsistente:
		p.abrirAsistente()
	case VistaBanco:
		p.banco.Datos, p.banco.Fallo, p.banco.pistas = nil, "", false
		p.cargarBanco()
	}
	p.Repintar()
}

// MaxRecientes es el `.slice(0, 6)` de `recordarModelo`.
const MaxRecientes = 6

// recordar pone un modelo el primero de los recientes, sin repetirlo.
func (p *Pantalla) recordar(clave string) {
	p.Recientes = append([]string{clave}, slices.DeleteFunc(p.Recientes, func(c string) bool {
		return c == clave
	})...)
	if len(p.Recientes) > MaxRecientes {
		p.Recientes = p.Recientes[:MaxRecientes]
	}
}

// Abrir cambia el modelo que se está mirando. Con el modelo cero se vuelve a la
// portada, que es lo que hace «Inicio».
//
// Los filtros NO pasan de un modelo a otro: los estados los declara CADA modelo, y
// arrastrar «pagado» a una lista de vehículos pediría un estado que ese modelo no tiene
// —el servicio contestaría 400 con un error que nadie sabría de dónde sale—.
func (p *Pantalla) Abrir(m Modelo) {
	l := p.lista
	p.pararEspera()
	l.Modelo = m
	l.Busqueda, l.Estado, l.Papelera = "", "", false
	l.limpiarBuscador()
	l.Orden = Orden{}
	l.Total, l.SumaDinero, l.PorEstado = -1, 0, nil
	l.CargandoMas = false
	l.Marcadas = map[string]bool{}
	p.abierta, p.cajon, p.ficha, p.Vista = false, false, nil, ""
	// Lo de ESTE modelo, o nada: sin esto se quedaban en pantalla las filas del modelo
	// anterior, pintadas con las columnas del nuevo, mientras llegaba la lista.
	l.Pagina, l.HayMas, l.Aviso = Pagina{}, false, ""
	if m.Clave == "" {
		p.cargarPortada()
		p.Repintar()
		return
	}
	p.recordar(m.Clave)
	if m.Clave == api.ERPFacturaVenta && p.facturae() {
		p.cargarVerifactu()
	}
	p.pedirLista()
	if p.svc != nil {
		p.fondo(func() { p.cargarEstadisticas(m.Clave) })
	}
}

// enganchar conecta las vistas con el servicio. Es la ÚNICA costura: ni el menú ni la
// lista saben que existe una API.
func (p *Pantalla) enganchar() {
	p.menu.AlElegir = p.Abrir
	p.menu.AlPlegar = func(clave string) {
		p.menu.Plegadas[clave] = !p.menu.Plegadas[clave]
		p.Repintar()
	}
	// Desde el cajón, las dos lo cierran antes, como en la web (`setCajon(false)`).
	p.menu.AlPaleta = func() { p.cerrarCajon(); p.AbrirPaleta() }
	p.menu.AlIA = func() { p.cerrarCajon(); p.AbrirIA() }
	p.menu.AlCerrar = p.cerrarCajon

	p.paleta.AlCerrar = p.CerrarPaleta
	p.paleta.AlElegir = p.Abrir
	p.paleta.AlMover = p.Repintar

	l := p.lista
	l.AlBuscar = p.buscar
	l.AlEstado = func(e string) { l.Estado = e; p.recargar() }
	l.AlPapelera = func(b bool) { l.Papelera = b; p.recargar() }
	l.AlOrdenar = func(o Orden) { l.Orden = o; p.recargar() }
	l.AlLimpiar = func() {
		l.Estado, l.Papelera, l.Busqueda = "", false, ""
		l.limpiarBuscador()
		p.recargar()
	}
	l.AlReintentar = p.recargar
	l.AlMas = p.pedirMas
	l.AlAbrir = p.AbrirFicha
	l.AlNueva = func() { p.NuevaFicha(nil) }
	l.AlDuplicar = p.duplicarDeLista
	l.AlMarcar = func(id string) {
		if l.Marcadas[id] {
			delete(l.Marcadas, id)
		} else {
			l.Marcadas[id] = true
		}
		p.Repintar()
	}
	l.AlMarcarTodas = func() {
		if len(l.Marcadas) == len(l.registros()) {
			l.Marcadas = map[string]bool{}
		} else {
			for _, r := range l.registros() {
				l.Marcadas[r.ID] = true
			}
		}
		p.Repintar()
	}
	l.AlDesmarcar = func() { l.Marcadas = map[string]bool{}; p.Repintar() }
	l.AlLote = p.lote
	l.AlVista = func(tarjetas bool) { l.Tarjetas = tarjetas; p.Repintar() }
	l.AlExportar = p.exportar
	p.tareas.AlMias = func(mias bool) { p.tareas.Mias, p.tareas.Lista = mias, nil; p.Repintar(); p.cargarTareas() }
	p.tareas.AlCerrar = p.cerrarTareaDeLista
	p.tareas.AlAbrir = p.abrirRegistro
	o := p.oblig
	o.AlCambio = p.Repintar
	o.AlAnio = func(a int) { o.Anio = a; p.cargarObligaciones() }
	o.AlICS = p.icsObligaciones
	o.AlPortal = abrirURL
	o.AlPreparar = p.prepararObligacion
	o.AlProceso = p.AbrirProcesoObligacion
	o.AlDocumentos = p.documentosObligacion
	o.AlAbrirAdjunto = p.abrirAdjunto
	o.AlBorrarAdj = p.borrarAdjuntoObligacion
	o.AlEditarPerfil = func() { p.IrA(VistaAsistente) }
	o.AlBanco = func() { p.IrA(VistaBanco) }
	b := p.inicio
	b.AlAbrirModelo = p.Abrir
	b.AlPaleta = p.AbrirPaleta
	b.AlVista = p.IrA
	b.AlNuevaFicha = func(m Modelo) { p.Abrir(m); p.NuevaFicha(nil) }
	b.AlCambiar = p.cambiarModulo
	b.AlConfigurar = func() { p.IrA(VistaAsistente) }
	b.AlBanco = func() { p.IrA(VistaBanco) }
}

// AbrirPaleta y CerrarPaleta son ⌘K y Escape.
func (p *Pantalla) AbrirPaleta() {
	p.paleta.Escribir("")
	p.abierta = true
	p.Repintar()
	// El `autofocus` de la web: se abre para escribir.
	if p.lienzo != nil {
		p.lienzo.Focus(p.paleta.Campo())
	}
}

// CerrarPaleta la esconde sin elegir nada.
func (p *Pantalla) CerrarPaleta() {
	if !p.abierta {
		return
	}
	p.abierta = false
	p.Repintar()
}

// EsperaBusqueda es lo que la web espera a que se deje de teclear antes de pedir la
// lista: una lista por pulsación es una consulta por pulsación contra la base de datos
// de la organización. Con el campo vacío no se espera.
const EsperaBusqueda = 220 * time.Millisecond

// buscar recoge una pulsación en el buscador de la lista.
//
// NO repinta: el campo es el mismo widget de una pintada a otra (`Lista.buscador`), y
// rehacer el árbol en cada letra es lo que se llevaba el foco a la segunda pulsación.
// Lo que se repinta es la respuesta, y solo si sigue siendo la de lo escrito.
func (p *Pantalla) buscar(q string) {
	l := p.lista
	if q == l.Busqueda {
		return
	}
	l.Busqueda = q
	p.pararEspera()
	if p.svc == nil {
		return // sin servicio no hay nada que pedir, ni reloj que dejar vivo
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	espera := EsperaBusqueda
	if q == "" {
		espera = 0
	}
	p.espera = time.AfterFunc(espera, func() {
		enHilo(func() {
			if l.Busqueda == q {
				p.pedirLista()
			}
		})
	})
}

// pararEspera suelta la búsqueda que estuviera esperando a que se dejara de teclear.
func (p *Pantalla) pararEspera() {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.espera != nil {
		p.espera.Stop()
		p.espera = nil
	}
}

// recargar vuelve a pedir la lista tras cambiar un filtro o el orden.
func (p *Pantalla) recargar() { p.pedirLista() }

// Repintar rehace el árbol con el estado de ahora. Se llama SIEMPRE desde la rutina
// principal: quien venga de otra pasa por `enHilo`.
func (p *Pantalla) Repintar() {
	for _, alta := range p.altas {
		alta.repintar()
	}
	if p.raiz == nil {
		return
	}
	p.raiz.Objects[0] = p.marco()
	p.raiz.Refresh()
	if p.repintarIA != nil {
		p.repintarIA()
	}
}

// enHilo ejecuta algo en la rutina de la interfaz. `fyne.Do` es obligatorio desde Fyne
// 2.6 para tocar la interfaz desde otra rutina.
func enHilo(f func()) { fyne.Do(f) }

func (p *Pantalla) ctx() (context.Context, context.CancelFunc) {
	return context.WithTimeout(context.Background(), p.Plazo)
}

// hayRed dice si el monitor ve red. Sin monitor se asume que sí: no tenerlo no es estar
// desconectado.
func (p *Pantalla) hayRed() bool {
	return p.red == nil || p.red.State() != offline.Offline
}

// Arrancar hace la primera carga: el catálogo y los permisos. Se puede volver a llamar
// —al entrar, tras haber arrancado sin sesión— y vuelve a pedirlo todo.
func (p *Pantalla) Arrancar() {
	p.fondo(p.cargarCatalogo)
	p.contarTareas()
}

func (p *Pantalla) cargarCatalogo() {
	if p.svc == nil {
		return
	}
	if !p.hayRed() {
		enHilo(func() { p.Fase = SinConexion; p.Repintar() })
		return
	}
	ctx, cancelar := p.ctx()
	defer cancelar()
	cat, err := p.svc.Catalogo(ctx)
	// Los permisos son una petición aparte y su fallo NO tumba la pantalla: sin
	// saberlos se enseña todo y manda el servicio, igual que en la web.
	permisos, errPerm := p.svc.MisPermisos(ctx)
	// El perfil y los requisitos de factura tampoco tumban nada: sin ellos no hay
	// Facturae ni VERI*FACTU, que es lo que hace la web.
	org, errOrg := p.svc.Organizacion(ctx)
	req, errReq := p.svc.RequisitosFactura(ctx)
	p.cargarTableros(ctx)
	enHilo(func() {
		p.Catalogo = cat
		if errPerm == nil {
			p.Permisos, p.SabePermisos = permisos, true
		}
		switch {
		case errOrg == nil:
			p.Organizacion, p.SabeOrg = &org, true
		case api.EsNoEncontrado(errOrg):
			p.Organizacion, p.SabeOrg = nil, true
		}
		if errReq == nil {
			p.Requisitos = &req
		}
		p.paleta.Modelos = p.Modelos()
		p.paleta.Areas = map[string]string{}
		for _, m := range cat.Modulos {
			p.paleta.Areas[m.Clave] = m.Etiqueta
		}
		p.Fase = FaseDe(err, p.hayRed(), len(cat.Modulos))
		if p.Vista == "" && p.lista.Modelo.Clave == "" {
			// Primera vez: sin perfil y sin ningún área encendida, lo que toca es el
			// asistente, no una cuadrícula de treinta y tres tarjetas apagadas.
			if p.SabeOrg && p.Organizacion == nil && len(cat.Activos) == 0 && err == nil {
				p.IrA(VistaAsistente)
				return
			}
			p.cargarPortada()
		}
		p.Repintar()
	})
}

// LimitePagina es el `limite` que la web pide por página. Cincuenta es lo que sirve el
// servicio por defecto y lo que la web enseña antes de «Ver más».
const LimitePagina = 50

// filtro es lo que se le pide al servicio con los filtros de ahora.
func (l *Lista) filtro(cursor string) api.ERPFiltro {
	return api.ERPFiltro{
		Q: l.Busqueda, Estado: l.Estado, Orden: l.Orden.Campo, Asc: l.Orden.Asc,
		Papelera: l.Papelera, Cursor: cursor, Limite: LimitePagina,
	}
}

// pedirLista pinta la lista del modelo abierto con los filtros de ahora y la pide al
// servicio. Va en la rutina de la interfaz; la petición, en otra.
//
// Si esa lista ya se vio, se pinta en el acto y sin rueda —la petición sigue, por
// detrás—; si no, la rueda. Con algo ya pintado, un fallo NO borra la pantalla: se
// avisa en la banda y se deja lo que había, que seguía siendo cierto hace un momento.
func (p *Pantalla) pedirLista() {
	l := p.lista
	if l.Modelo.Clave == "" {
		return
	}
	clave := l.clave()
	p.mu.Lock()
	pag, visto := p.cache[clave]
	p.mu.Unlock()
	if visto {
		l.Pagina, l.HayMas, l.Fase = pag, pag.Siguiente != "", FaseDe(nil, true, len(pag.Registros))
	} else {
		l.Fase = Cargando
	}
	p.Repintar()
	if p.svc == nil {
		return
	}
	if !p.hayRed() {
		if !visto {
			l.Fase = SinConexion
			p.Repintar()
		}
		return
	}
	modelo, f := l.Modelo.Clave, l.filtro("")
	p.fondo(func() { p.traerLista(modelo, clave, f, visto) })
}

func (p *Pantalla) traerLista(modelo, clave string, f api.ERPFiltro, visto bool) {
	ctx, cancelar := p.ctx()
	defer cancelar()
	pag, err := p.svc.ListarCon(ctx, modelo, f)
	enHilo(func() { p.ponerLista(clave, pag, err, visto) })
}

// ponerLista pinta lo que contestó el servicio a la lista `clave`.
func (p *Pantalla) ponerLista(clave string, pag Pagina, err error, visto bool) {
	l := p.lista
	// Entre que se pidió y llegó pudo cambiarse de modelo o de filtro: lo de OTRA lista
	// no se pinta aquí. Sin esto se quedan las filas del anterior bajo las columnas del
	// nuevo, que es una tabla de guiones con pinta de error de datos.
	if l.clave() != clave {
		return
	}
	fase, aviso := FaseDeLista(err, p.hayRed(), len(pag.Registros), p.tr)
	if err != nil && visto {
		if aviso == "" {
			aviso = textoDeFase(fase, p.tr)
		}
		l.Aviso = aviso
		p.Repintar()
		return
	}
	if err == nil {
		p.mu.Lock()
		p.cache[clave] = pag
		p.mu.Unlock()
	}
	l.Pagina, l.HayMas, l.Aviso = pag, pag.Siguiente != "", aviso
	l.Fase = fase
	p.Repintar()
}

// pedirMas pide la página siguiente por el cursor keyset y la AÑADE a lo que hay:
// quien pide «ver más» quiere más, no otra pantalla. Hasta ahora una lista se acababa
// en cincuenta filas —en un ERP, las facturas del mes pasado—.
func (p *Pantalla) pedirMas() {
	l := p.lista
	if l.CargandoMas || l.Pagina.Siguiente == "" || p.svc == nil {
		return
	}
	l.CargandoMas = true
	p.Repintar()
	modelo, clave, f := l.Modelo.Clave, l.clave(), l.filtro(l.Pagina.Siguiente)
	p.fondo(func() {
		ctx, cancelar := p.ctx()
		defer cancelar()
		mas, err := p.svc.ListarCon(ctx, modelo, f)
		enHilo(func() { p.ponerMas(clave, mas, err) })
	})
}

func (p *Pantalla) ponerMas(clave string, mas Pagina, err error) {
	l := p.lista
	if l.clave() != clave {
		return
	}
	l.CargandoMas = false
	if err != nil {
		l.Aviso = textoDeFase(FaseDe(err, p.hayRed(), 0), p.tr)
		p.Repintar()
		return
	}
	l.Pagina = unirPaginas(l.Pagina, mas)
	l.HayMas = l.Pagina.Siguiente != ""
	p.mu.Lock()
	p.cache[clave] = l.Pagina
	p.mu.Unlock()
	p.Repintar()
}

// unirPaginas añade `mas` detrás de `pag`. Las etiquetas de las relaciones se
// ACUMULAN: las de la página anterior siguen haciendo falta para pintar sus filas.
func unirPaginas(pag, mas Pagina) Pagina {
	out := Pagina{
		Registros: append(slices.Clone(pag.Registros), mas.Registros...),
		Etiquetas: make(map[string]string, len(pag.Etiquetas)+len(mas.Etiquetas)),
		Siguiente: mas.Siguiente,
	}
	for k, v := range pag.Etiquetas {
		out.Etiquetas[k] = v
	}
	for k, v := range mas.Etiquetas {
		out.Etiquetas[k] = v
	}
	return out
}

// cargarEstadisticas pide el recuento del servidor: el número de la cabecera, el de
// cada píldora y el importe acumulado. Su fallo es silencioso, como en la web: sin él
// se cuenta lo que hay a la vista.
func (p *Pantalla) cargarEstadisticas(modelo string) {
	if p.svc == nil {
		return
	}
	ctx, cancelar := p.ctx()
	defer cancelar()
	st, err := p.svc.Estadisticas(ctx, modelo)
	if err != nil {
		return
	}
	enHilo(func() {
		l := p.lista
		if l.Modelo.Clave != modelo {
			return
		}
		l.Total, l.PorEstado, l.SumaDinero = st.Total, st.PorEstado, st.SumaDinero
		p.Repintar()
	})
}

// Atajos engancha el teclado de la web al lienzo de la ventana: ⌘K abre la paleta,
// «/» lleva al buscador de la lista, «n» abre una ficha nueva y Escape cierra la
// paleta.
//
// ⌘K es de la PALETA y no del buscador de la suite, igual que en la web: allí los dos
// escuchan y el del ERP abre la paleta, que se queda el foco. Por eso se le quita a la
// barra (`BarraSuite.CederAtajo`) en vez de registrarlo encima, que dependería de cuál
// de los dos se enganchó el último.
//
// Las teclas sueltas van por `SetOnTypedRune`, que Fyne solo dispara cuando NO hay
// nada enfocado: es exactamente `esAtajoDeApp` de la web —teclear «/» en el nombre de
// un cliente no puede saltar al buscador—.
func (p *Pantalla) Atajos(c fyne.Canvas, b *ui.BarraSuite) {
	p.lienzo = c
	if b != nil {
		b.CederAtajo(p.AbrirPaleta)
	}
	c.SetOnTypedRune(func(r rune) {
		if p.lista.Modelo.Clave == "" || p.abierta {
			return
		}
		switch r {
		case '/':
			p.lista.enfocarBuscador(c)
		case 'n':
			// «n» abre una ficha nueva, como en la web (y como allí, no con el foco en
			// un campo: `SetOnTypedRune` solo llega sin nada enfocado).
			p.NuevaFicha(nil)
		}
	})
	c.SetOnTypedKey(func(k *fyne.KeyEvent) {
		if k.Name == fyne.KeyEscape {
			p.CerrarPaleta()
		}
	})
	ui.AlAtras(c, p.atras)
}

// olvidar tira lo cacheado de un modelo —todas sus búsquedas—: servir una lista vieja
// justo después de escribir es la forma más rápida de que alguien crea que no se ha
// guardado.
func (p *Pantalla) olvidar(modelo string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	for k := range p.cache {
		if claveDeModelo(k) == modelo {
			delete(p.cache, k)
		}
	}
}
