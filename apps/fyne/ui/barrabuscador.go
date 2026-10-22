// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package ui

import (
	"context"
	"image/color"
	"strings"
	"sync"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/driver/desktop"
	"fyne.io/fyne/v2/driver/mobile"
	"fyne.io/fyne/v2/widget"

	"git.sytue.eu/sytue/one/apps/fyne/iconos"
	"git.sytue.eu/sytue/one/apps/fyne/sesion"
	"git.sytue.eu/sytue/one/apps/fyne/tema"
	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/i18n"
)

// El buscador de la suite: `GlobalSearch.tsx`.
//
// # El panel NO va a una capa, y esa es la decisión que lo sostiene
//
// Los menús de la suite se despliegan en `Overlays()` —es lo que hace `ui.Menu`— y ahí
// Fyne apila un gestor de foco nuevo: mientras la capa está puesta, `Canvas.Focused()`
// contesta por ella y el campo de debajo DEJA DE RECIBIR TECLAS. Para un menú está
// bien, porque el foco se va al menú a propósito; para un combobox es justo lo que no
// puede pasar: los resultados aparecen mientras se escribe y hay que poder seguir
// escribiendo (patrón `combobox` de WAI-ARIA, que deja el foco en el campo y marca la
// fila con `aria-activedescendant`).
//
// Así que el panel es un hijo más del árbol, colgado por debajo de la barra. Lo que
// eso obliga a resolver a mano: que se pinte ENCIMA del contenido —de ahí el orden de
// `BarraSuite.Montar`— y que se cierre solo, que aquí es con Escape, al elegir, y
// cuando el campo pierde el foco.
const (
	// EsperaBuscador es el `setTimeout(…, 250)` de la web entre tecla y petición.
	EsperaBuscador = 250 * time.Millisecond
	// MinimoBuscador es el `query.trim().length < 2` que decide si se busca.
	MinimoBuscador = 2
	// MaxResultadosBarra es lo que se pide al Hub. La web no acota y el panel enseña
	// `max-h-96` con desplazamiento; aquí se acota a lo que cabe sin desplazar.
	MaxResultadosBarra = 12
	// SeparacionPanel es el `top-[calc(100%+6px)]` del panel bajo el campo.
	SeparacionPanel float32 = 6
	// MargenPanel es el `p-1` del panel.
	MargenPanel = 1 * 0.25 * tema.Rem
	// MargenAtajo y AltoAtajo son el `px-1.5 py-0.5` de la pista del atajo, con
	// `leading-none`; RaizAtajo es su `right-3`.
	MargenAtajo = 1.5 * 0.25 * tema.Rem
	RaizAtajo   = 3 * 0.25 * tema.Rem
	// CorteAtajo es el `hidden md:inline-flex` de la pista: `md:` son 768 px de VENTANA
	// (rem de 16). Por debajo, y siempre en un teléfono, no hay teclado que anunciar.
	CorteAtajo float32 = 768
)

// EscalonAtajo es el `text-[11px] leading-none` de la pista `⌘K`.
var EscalonAtajo = tema.Escalon{Tam: 11, Linea: 11}

// estadoBarra es en qué punto está una de las piezas que piden datos.
type estadoBarra uint8

const (
	enReposo estadoBarra = iota
	cargando
	listo
	fallo
)

// buscadorBarra es el campo de la barra y su panel de resultados.
type buscadorBarra struct {
	t   *i18n.Translator
	hub *api.Hub
	s   *sesion.Suite

	campo *Entrada
	panel *panelBarra
	atajo *pistaAtajo

	mu       sync.Mutex
	termino  string
	estado   estadoBarra
	hits     []api.HubHit
	fallidos []string
	activa   int
	// estrecho es la ventana por debajo de `md:` o un teléfono: sin pista del atajo.
	estrecho bool
	espera   *time.Timer
	atajoUna sync.Once
	// alAtajo, si una app se ha quedado ⌘K (`BarraSuite.CederAtajo`), se llama en vez de
	// enfocar este campo. alAtajoSi es lo mismo pero puede devolverlo: con false, ⌘K
	// enfoca el buscador como siempre (`BarraSuite.CederAtajoSi`).
	alAtajo   func()
	alAtajoSi func() bool
}

func nuevoBuscadorBarra(t *i18n.Translator, s *sesion.Suite) *buscadorBarra {
	b := &buscadorBarra{t: t, s: s, activa: -1}
	if s != nil {
		b.hub = s.Hub
	}
	b.campo = NuevaEntrada(t.T("search.placeholder", nil))
	b.campo.Icono = iconos.Buscar
	b.campo.SobreCromo = true
	b.campo.Radio = tema.RadioPill // `rounded-pill!`
	b.campo.AlCambiar(b.alEscribir)
	b.campo.AlTeclear = b.alTeclear
	b.campo.AlEnfocar = b.alEnfocar
	b.panel = nuevoPanelBarra()
	b.panel.Hide()
	b.atajo = nuevaPistaAtajo(atajoDelSistema())
	return b
}

// atajoDelSistema es «⌘K» en un Mac y «Ctrl K» en todo lo demás, igual que la web lo
// decide al montar y no en el servidor.
func atajoDelSistema() string { return AtajoDelSistema("Ctrl K") }

// colocar sitúa el campo, la pista del atajo y el panel. `ventana` es el tamaño de la
// barra, que es lo que acota el ancho del panel.
func (b *buscadorBarra) colocar(x, y, ancho float32, ventana fyne.Size) {
	b.campo.Move(fyne.NewPos(x, y))
	b.campo.Resize(fyne.NewSize(max(ancho, 0), AltoCampo(CampoMD)))
	tam := b.atajo.MinSize()
	b.atajo.Move(fyne.NewPos(x+ancho-RaizAtajo-tam.Width, y+(AltoCampo(CampoMD)-tam.Height)/2))
	b.atajo.Resize(tam)
	b.estrecho = EsTactil() || ventana.Width < CorteAtajo
	b.verAtajo()
	b.registrarAtajo()

	alto := b.panel.MinSize().Height
	if ventana.Width > 0 && ventana.Width < Estrecho {
		// En el teléfono, a todo el ancho bajo la barra (como la web, `max-md:inset-x-0
		// top-14`): colgado del campo medía lo que él y los títulos no se leían.
		b.panel.aSangre = true
		b.panel.Move(fyne.NewPos(0, AltoBarraSuite()))
		b.panel.Resize(fyne.NewSize(ventana.Width, alto))
		return
	}
	b.panel.aSangre = false
	b.panel.Move(fyne.NewPos(x, AltoBarraSuite()+SeparacionPanel))
	b.panel.Resize(fyne.NewSize(max(ancho, 0), alto))
}

// verAtajo enseña la pista solo en reposo, vacío y con sitio: con texto, `type=search`
// pinta ahí su aspa, y enfocado ya no hace falta decir cómo se llega.
func (b *buscadorBarra) verAtajo() {
	b.atajo.Visible2(!b.estrecho && !b.campo.enfocado && b.campo.Texto() == "")
}

// parar cancela la búsqueda que estuviera esperando su turno.
//
// No es cosmética: entre la última tecla y la petición hay 250 ms, y ese
// temporizador dispara en SU PROPIA gorutina. Si la ventana se cierra por medio, lo
// que despierta después repinta un panel que ya no existe. En los guardianes se nota
// más todavía —el `fyne.Do` del driver de pruebas NO marshalla nada, así que la
// gorutina del reloj conforma texto a la vez que el test siguiente y el conformador
// revienta—, y es como se encontró.
func (b *buscadorBarra) parar() {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.espera != nil {
		b.espera.Stop()
		b.espera = nil
	}
}

// registrarAtajo engancha ⌘K / Ctrl+K al lienzo, UNA vez, en cuanto hay lienzo.
//
// En la web lo escucha `document`; aquí el equivalente es el lienzo de la ventana, al
// que solo se llega cuando el widget ya está montado — de ahí que se intente en cada
// colocación y se quede con la primera que encuentra algo.
func (b *buscadorBarra) registrarAtajo() {
	c := lienzoDe(b.campo)
	if c == nil {
		return
	}
	b.atajoUna.Do(func() {
		c.AddShortcut(&desktop.CustomShortcut{
			KeyName: fyne.KeyK, Modifier: fyne.KeyModifierShortcutDefault,
		}, func(fyne.Shortcut) { b.pulsarAtajo(c) })
	})
}

// pulsarAtajo es lo que hace ⌘K: lo de la app que se lo quedó, o enfocar este campo.
func (b *buscadorBarra) pulsarAtajo(c fyne.Canvas) {
	if b.alAtajoSi != nil && b.alAtajoSi() {
		return
	}
	if b.alAtajo != nil {
		b.alAtajo()
		return
	}
	enfocar(c, b.campo)
}

func (b *buscadorBarra) alEnfocar(enfocado bool) {
	b.verAtajo()
	if !enfocado {
		b.cerrar()
	}
}

// alEscribir es el efecto de la web: por debajo de dos letras no se busca y el panel
// se cierra; por encima, se espera 250 ms desde la última tecla.
func (b *buscadorBarra) alEscribir(texto string) {
	b.verAtajo()
	b.mu.Lock()
	if b.espera != nil {
		b.espera.Stop()
	}
	b.termino = texto
	corto := len([]rune(strings.TrimSpace(texto))) < MinimoBuscador
	if corto {
		b.estado, b.hits, b.fallidos, b.activa = enReposo, nil, nil, -1
		b.mu.Unlock()
		b.pintar()
		return
	}
	b.estado = cargando
	q := strings.TrimSpace(texto)
	b.espera = time.AfterFunc(EsperaBuscador, func() { b.buscar(q) })
	b.mu.Unlock()
	b.pintar()
}

// buscar pregunta al Hub. Sin Hub no hay búsqueda posible y se dice: el panel enseña
// el fallo con su reintento, que es lo mismo que enseña la web cuando el Hub no
// contesta.
func (b *buscadorBarra) buscar(q string) {
	var cuenta sesion.Estado
	if b.s != nil {
		cuenta = b.s.Estado()
		if !cuenta.Dentro() {
			return
		}
	}
	if b.hub == nil {
		b.mu.Lock()
		if strings.TrimSpace(b.termino) != q {
			b.mu.Unlock()
			return
		}
		b.estado = fallo
		b.mu.Unlock()
		b.pintar()
		return
	}
	ctx, cancelar := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancelar()
	res, err := b.hub.Search(ctx, q, MaxResultadosBarra)
	if b.s != nil {
		actual := b.s.Estado()
		if actual.Sujeto != cuenta.Sujeto || actual.GeneracionSesion != cuenta.GeneracionSesion {
			return
		}
	}

	b.mu.Lock()
	if strings.TrimSpace(b.termino) != q { // ha seguido escribiendo: esta respuesta ya no vale
		b.mu.Unlock()
		return
	}
	// Agrupados por origen, que es el orden en que se VEN y por el que se mueven las
	// flechas (`ordenados` de GlobalSearch.tsx).
	b.hits, b.fallidos, b.activa = agruparPorOrigen(res.Hits), res.Fallidos(), 0
	b.estado = listo
	if err != nil {
		b.hits, b.fallidos, b.activa, b.estado = nil, nil, -1, fallo
	}
	b.mu.Unlock()
	b.pintar()
}

// alTeclear es la navegación del combobox. Devuelve true cuando se queda la tecla: el
// campo no la ve y el cursor no se mueve.
func (b *buscadorBarra) alTeclear(k *fyne.KeyEvent) bool {
	b.mu.Lock()
	n := len(b.hits)
	abierto := b.panel.Visible()
	b.mu.Unlock()

	switch k.Name {
	case fyne.KeyEscape, mobile.KeyBack:
		if !abierto {
			return false
		}
		b.cerrar()
		return true
	case fyne.KeyReturn, fyne.KeyEnter:
		b.mu.Lock()
		i := b.activa
		var hit api.HubHit
		if i >= 0 && i < len(b.hits) {
			hit = b.hits[i]
		}
		b.mu.Unlock()
		if hit.URL == "" {
			return false
		}
		b.abrir(hit)
		return true
	case fyne.KeyDown, fyne.KeyUp:
		if n == 0 {
			return false
		}
		paso := 1
		if k.Name == fyne.KeyUp {
			paso = -1
		}
		b.mu.Lock()
		b.activa = (b.activa + paso + n) % n
		b.mu.Unlock()
		b.pintar()
		return true
	default: // las demás son del editor: escribir, borrar, mover el cursor
		return false
	}
}

// abrir lanza el resultado EN EL SISTEMA: la aplicación de su producto por su esquema
// y, si no está instalada, su dirección web.
//
// Es la regla del encargo y de §13.6: un resultado de otra aplicación no se pinta
// dentro de esta. Y también vale para uno de la propia: en nativo cada producto es su
// binario, así que «abrir en Sytue Mail» es lo mismo se mire desde donde se mire.
//
// ponytail: un resultado de la aplicación en la que ya estás sale por el sistema y
// vuelve a entrar por el esquema, en vez de navegar aquí dentro. Se releva cuando las
// pantallas tengan rutas: hoy ninguna de las trece sabe «ve a este correo».
func (b *buscadorBarra) abrir(h api.HubHit) {
	b.cerrar()
	_ = api.AbrirURL(fyne.CurrentApp(), h.Source, h.URL)
}

func (b *buscadorBarra) cerrar() {
	b.mu.Lock()
	b.estado, b.activa = enReposo, -1
	b.mu.Unlock()
	b.pintar()
}

// pintar rehace el panel con el estado de ahora. Va por `fyne.Do` porque lo llama
// también el temporizador de la búsqueda, que corre en otra gorutina.
func (b *buscadorBarra) pintar() {
	fyne.Do(func() {
		b.mu.Lock()
		estado, hits, fallidos, activa, termino := b.estado, b.hits, b.fallidos, b.activa, strings.TrimSpace(b.termino)
		b.mu.Unlock()

		switch estado {
		case enReposo:
			b.panel.Poner(nil)
			b.panel.Hide()
			return
		case cargando:
			b.panel.Poner(esqueletosDeLista(3))
		case fallo:
			b.panel.Poner([]fyne.CanvasObject{b.vacio(
				b.t.T("search.failed", nil), b.t.T("error.connection", nil), true)})
		case listo:
			b.panel.Poner(b.filas(hits, fallidos, activa, termino))
		}
		b.panel.Show()
		b.panel.Refresh()
	})
}

// filas son los resultados, o el estado vacío si no hay ninguno.
func (b *buscadorBarra) filas(hits []api.HubHit, fallidos []string, activa int, termino string) []fyne.CanvasObject {
	if len(hits) == 0 {
		desc := b.t.T("search.emptyDescription", nil)
		if len(fallidos) > 0 {
			desc = b.t.T("search.partial", i18n.Vars{"sources": nombresDeOrigenes(fallidos)})
		}
		return []fyne.CanvasObject{b.vacio(
			b.t.T("search.emptyTitle", i18n.Vars{"query": termino}), desc, false)}
	}
	out := make([]fyne.CanvasObject, 0, len(hits)+1)
	if len(fallidos) > 0 {
		// El aviso de los orígenes caídos, `px-3 py-2 text-xs` sobre el panel: sin
		// él, «no hay resultados» afirma algo falso cuando el único origen que los
		// tenía no contestó.
		out = append(out, NuevoParrafo(
			b.t.T("search.partial", i18n.Vars{"sources": nombresDeOrigenes(fallidos)}),
			tema.TextoXS, Normal, "text-muted",
			tema.De(fyne.CurrentApp().Settings().ThemeVariant())))
	}
	for i, h := range hits {
		hit := h
		// Un grupo por producto, con su cabecera: la marca de 15 y el nombre en
		// versalitas. El color del producto va SOLO en la tesela.
		if i == 0 || hits[i-1].Source != hit.Source {
			out = append(out, nuevaCabeceraGrupo(hit.Source, i == 0))
		}
		f := nuevaFilaResultado(hit.Title, DetalleHit(b.t.Locale(), hit), func() { b.abrir(hit) })
		f.marcada = i == activa
		out = append(out, f)
	}
	return out
}

// agruparPorOrigen es `groupBySource(...).flatMap(g => g.items)`: los resultados juntos
// por producto, en el orden en que aparece cada producto por primera vez.
func agruparPorOrigen(hits []api.HubHit) []api.HubHit {
	var orden []string
	grupos := map[string][]api.HubHit{}
	for _, h := range hits {
		if _, ya := grupos[h.Source]; !ya {
			orden = append(orden, h.Source)
		}
		grupos[h.Source] = append(grupos[h.Source], h)
	}
	out := make([]api.HubHit, 0, len(hits))
	for _, o := range orden {
		out = append(out, grupos[o]...)
	}
	return out
}

// Medidas de un grupo de resultados de GlobalSearch.tsx.
var (
	margenXResultado = tema.Espacio(2.5) // px-2.5 de la cabecera y de la fila
	margenYResultado = tema.Espacio(1.5) // py-1.5 de la fila
	arribaGrupo      = tema.Espacio(2.5) // pt-2.5 de la cabecera…
	arribaPrimero    = tema.Espacio(1.5) // …y first:pt-1.5
	abajoGrupo       = tema.Espacio(1)   // pb-1
	huecoGrupo       = tema.Espacio(1.5) // gap-1.5 entre la marca y el nombre
)

// LadoMarcaGrupo es el `<ProductIcon size={15}>` de la cabecera de grupo.
const LadoMarcaGrupo float32 = 15

// cabeceraGrupo es el `<li role="presentation">` de un grupo: la marca del producto y su
// nombre corto en `sytue-overline`. Un origen sin marca (hosts legados) enseña su clave.
type cabeceraGrupo struct {
	widget.BaseWidget
	origen  string
	primero bool
}

func nuevaCabeceraGrupo(origen string, primero bool) *cabeceraGrupo {
	c := &cabeceraGrupo{origen: origen, primero: primero}
	c.ExtendBaseWidget(c)
	return c
}

func (c *cabeceraGrupo) arriba() float32 {
	if c.primero {
		return arribaPrimero
	}
	return arribaGrupo
}

func (c *cabeceraGrupo) MinSize() fyne.Size {
	c.ExtendBaseWidget(c)
	return fyne.NewSize(0, c.arriba()+max(LadoMarcaGrupo, tema.TextoXS.Linea)+abajoGrupo)
}

func (c *cabeceraGrupo) CreateRenderer() fyne.WidgetRenderer {
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	r := &cabeceraGrupoRenderer{
		c:      c,
		nombre: NuevoVersal(nombreDeOrigen(c.origen), tema.TextoXS, SemiNegrita, tema.Color("text-muted", v)),
	}
	if p, ok := iconos.ProductoDe(c.origen); ok {
		r.marca = iconos.Marca(p, LadoMarcaGrupo, v)
		r.objetos = append(r.objetos, r.marca.Objeto())
	}
	r.objetos = append(r.objetos, r.nombre.Objetos()...)
	return r
}

type cabeceraGrupoRenderer struct {
	c       *cabeceraGrupo
	marca   *iconos.Dibujo
	nombre  *Versal
	objetos []fyne.CanvasObject
}

func (r *cabeceraGrupoRenderer) MinSize() fyne.Size           { return r.c.MinSize() }
func (r *cabeceraGrupoRenderer) Objects() []fyne.CanvasObject { return r.objetos }
func (r *cabeceraGrupoRenderer) Destroy()                     {}

func (r *cabeceraGrupoRenderer) Layout(fyne.Size) {
	fila := max(LadoMarcaGrupo, tema.TextoXS.Linea)
	y, x := r.c.arriba(), margenXResultado
	if r.marca != nil {
		r.marca.Colocar(x, y+(fila-LadoMarcaGrupo)/2, LadoMarcaGrupo)
		x += LadoMarcaGrupo + huecoGrupo
	}
	r.nombre.Colocar(x, y+(fila-tema.TextoXS.Linea)/2)
}

func (r *cabeceraGrupoRenderer) Refresh() {
	r.nombre.Pintar(tema.Color("text-muted", tema.De(fyne.CurrentApp().Settings().ThemeVariant())))
}

// filaResultado es el `<a>` de un resultado: `rounded-md px-2.5 py-1.5`, el título en
// `text-base font-medium` y el fragmento en `text-sm` apagado, los dos en una línea.
// El marcado va en `bg-menu-selected` —el foco se queda en el campo, así que es un estado
// propio y no un foco— y el puntero encima, en `bg-menu-hover`.
type filaResultado struct {
	pulsable
	titulo, fragmento string
	marcada           bool
}

func nuevaFilaResultado(titulo, fragmento string, alPulsar func()) *filaResultado {
	f := &filaResultado{titulo: titulo, fragmento: fragmento}
	f.Nombre, f.AlPulsar = titulo, alPulsar
	f.ExtendBaseWidget(f)
	return f
}

// MedidaFilaResultado es el alto de la fila, con fragmento o sin él.
func MedidaFilaResultado(conFragmento bool) float32 {
	alto := 2*margenYResultado + tema.TextoBase.Linea
	if conFragmento {
		alto += tema.TextoSM.Linea
	}
	return alto
}

func (f *filaResultado) MinSize() fyne.Size {
	f.ExtendBaseWidget(f)
	return fyne.NewSize(0, MedidaFilaResultado(f.fragmento != ""))
}

func (f *filaResultado) fondo(v tema.Variante) color.NRGBA {
	switch {
	case f.pulsado:
		return tema.Color("menu-selected", v)
	case f.marcada:
		return tema.Color("menu-selected", v)
	case f.encima:
		return tema.Color("menu-hover", v)
	}
	return color.NRGBA{}
}

func (f *filaResultado) CreateRenderer() fyne.WidgetRenderer {
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	r := &filaResultadoRenderer{
		f:         f,
		caja:      NuevaCaja(f.fondo(v), color.NRGBA{}, tema.RadioMD),
		titulo:    Texto(f.titulo, tema.TextoBase, Medio, tema.Color("text", v)),
		fragmento: Texto(f.fragmento, tema.TextoSM, Normal, tema.Color("text-muted", v)),
	}
	r.objetos = append(r.caja.Objetos(), r.titulo, r.fragmento)
	return r
}

type filaResultadoRenderer struct {
	f                 *filaResultado
	caja              *Caja
	titulo, fragmento *canvas.Text
	objetos           []fyne.CanvasObject
}

func (r *filaResultadoRenderer) MinSize() fyne.Size           { return r.f.MinSize() }
func (r *filaResultadoRenderer) Objects() []fyne.CanvasObject { return r.objetos }
func (r *filaResultadoRenderer) Destroy()                     {}

func (r *filaResultadoRenderer) Layout(t fyne.Size) {
	r.caja.Redimensionar(t)
	ancho := max(t.Width-2*margenXResultado, 0)
	r.titulo.Text = recorta(r.f.titulo, tema.TextoBase, Medio, ancho)
	ColocarTexto(r.titulo, tema.TextoBase, margenXResultado, margenYResultado)
	r.fragmento.Hidden = r.f.fragmento == ""
	r.fragmento.Text = recorta(r.f.fragmento, tema.TextoSM, Normal, ancho)
	ColocarTexto(r.fragmento, tema.TextoSM, margenXResultado, margenYResultado+tema.TextoBase.Linea)
}

func (r *filaResultadoRenderer) Refresh() {
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	r.caja.Pintar(r.f.fondo(v), color.NRGBA{})
	r.titulo.Color, r.fragmento.Color = tema.Color("text", v), tema.Color("text-muted", v)
	r.Layout(r.f.Size())
	r.titulo.Refresh()
	r.fragmento.Refresh()
}

// vacio es el `EmptyState` del panel, con su reintento cuando ha fallado.
func (b *buscadorBarra) vacio(titulo, descripcion string, conReintento bool) fyne.CanvasObject {
	v := NuevoVacio(titulo, descripcion)
	if conReintento {
		v.Accion = NuevoBoton(b.t.T("action.retry", nil), func() {
			b.mu.Lock()
			q := strings.TrimSpace(b.termino)
			b.mu.Unlock()
			go b.buscar(q)
		}).Con(Secundario, TallaSM)
	}
	return v
}

// nombreDeOrigen es la marca del producto que manda ese resultado, con el nombre
// corto: «Mail», no `mail`. Un origen que no conocemos se enseña con su clave, que es
// lo que hace la web (`appForKey(f.source)?.name ?? f.source`).
func nombreDeOrigen(clave string) string {
	if p, ok := iconos.ProductoDe(clave); ok {
		return p.NombreCorto()
	}
	return clave
}

// DetalleHit es la segunda línea de un resultado del Hub —búsqueda, «Reciente», actividad—:
// el tamaño de un fichero en el idioma de la app, o el fragmento. El Hub escribe el tamaño
// siempre igual en el fragmento («11.7 KiB»), que salía en inglés y con otras unidades que
// el resto de la suite. Nadie lee `Snippet` de un HubHit sin pasar por aquí: lo vigila
// `TestLosTamanosSeEscribenConFormatBytes`.
func DetalleHit(l i18n.Locale, h api.HubHit) string {
	if h.SizeBytes != nil {
		return i18n.FormatBytes(l, float64(*h.SizeBytes))
	}
	return h.Snippet
}

// nombresDeOrigenes es la lista de los que no han contestado, para decirlo.
func nombresDeOrigenes(claves []string) string {
	out := ""
	for i, c := range claves {
		if i > 0 {
			out += ", "
		}
		out += nombreDeOrigen(c)
	}
	return out
}

// esqueletosDeLista son las tres filas grises de la carga, que es lo que enseña la web
// mientras pide (`Skeleton width="65%"` y `"90%"`).
func esqueletosDeLista(n int) []fyne.CanvasObject {
	out := make([]fyne.CanvasObject, 0, n)
	for range n {
		out = append(out, NuevoEsqueleto(0, AltoEsqueleto))
	}
	return out
}

// ---- el panel ----

// panelBarra es la caja flotante de la barra: el `rounded-lg border bg-menu-surface
// shadow-[var(--shadow-md)]` con su `p-1`, apilando lo que le den.
type panelBarra struct {
	widget.BaseWidget
	hijos []fyne.CanvasObject
	r     *panelBarraRenderer
	// aSangre es el teléfono: a todo el ancho bajo la barra, sin cantos.
	aSangre bool
}

var _ fyne.Widget = (*panelBarra)(nil)

func nuevoPanelBarra() *panelBarra {
	p := &panelBarra{}
	p.ExtendBaseWidget(p)
	return p
}

// Poner cambia el contenido del panel.
func (p *panelBarra) Poner(hijos []fyne.CanvasObject) {
	p.hijos = hijos
	if r := p.r; r != nil {
		r.rehacer()
	}
}

// MinSize es el alto de lo que lleva dentro más el marco. El ancho lo pone quien lo
// coloca (es `left-0 right-0` del campo).
func (p *panelBarra) MinSize() fyne.Size {
	p.ExtendBaseWidget(p)
	var alto float32
	for _, h := range p.hijos {
		alto += h.MinSize().Height
	}
	return fyne.NewSize(0, alto+2*(MargenPanel+tema.Borde))
}

func (p *panelBarra) CreateRenderer() fyne.WidgetRenderer {
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	r := &panelBarraRenderer{
		p:      p,
		sombra: NuevaSombra(tema.SombraMD, tema.RadioLG),
		caja:   NuevaCaja(tema.Color("menu-surface", v), tema.Color("border", v), tema.RadioLG),
	}
	p.r = r
	r.rehacer()
	r.Refresh()
	return r
}

type panelBarraRenderer struct {
	p       *panelBarra
	sombra  *Sombra
	caja    *Caja
	objetos []fyne.CanvasObject
}

// IsClip recorta lo que se salga: el panel es una caja con esquinas y lo de dentro no
// puede pintarse fuera de ellas.
func (r *panelBarraRenderer) IsClip() {}

func (r *panelBarraRenderer) rehacer() {
	r.objetos = append([]fyne.CanvasObject{r.sombra.Objeto()}, r.caja.Objetos()...)
	r.objetos = append(r.objetos, r.p.hijos...)
}

func (r *panelBarraRenderer) MinSize() fyne.Size           { return r.p.MinSize() }
func (r *panelBarraRenderer) Objects() []fyne.CanvasObject { return r.objetos }
func (r *panelBarraRenderer) Destroy()                     {}

func (r *panelBarraRenderer) Layout(t fyne.Size) {
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	r.sombra.Ajustar(fyne.NewPos(0, 0), t, v)
	// A sangre (teléfono): sin cantos, `max-md:rounded-none`.
	if r.p.aSangre {
		r.caja.CambiarRadio(0)
	} else {
		r.caja.CambiarRadio(tema.RadioLG)
	}
	r.caja.Redimensionar(t)
	dentro := MargenPanel + tema.Borde
	y := dentro
	for _, h := range r.p.hijos {
		alto := h.MinSize().Height
		h.Move(fyne.NewPos(dentro, y))
		h.Resize(fyne.NewSize(max(t.Width-2*dentro, 0), alto))
		y += alto
	}
}

func (r *panelBarraRenderer) Refresh() {
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	r.caja.Pintar(tema.Color("menu-surface", v), tema.Color("border", v))
	r.Layout(r.p.Size())
	for _, h := range r.p.hijos {
		h.Refresh()
	}
}

// dentroDelCampo es el relleno del buscador, que es lo que se ve detrás de la pista del
// atajo (`bg-transparent`): la superficie, como pinta la web (ver colorCampoCromo).
//
// Tiene que ser OPACO porque `ui.Caja` pinta el borde como un rectángulo RELLENO con la
// caja encima: con un relleno transparente el borde se vería ENTERO y la píldora
// saldría maciza.
func dentroDelCampo(v tema.Variante) color.NRGBA { return tema.Color("surface", v) }

// ---- la pista del atajo ----

// pistaAtajo es el `<kbd>⌘K</kbd>` del borde derecho del campo: una píldora perfilada
// que se ve mientras el campo está en reposo y vacío.
//
// Es decorativa —`aria-hidden`— porque el atajo de verdad lo escucha el lienzo; está
// por lo mismo que en Spotlight: quien lo ve una vez ya no lo busca en ningún manual.
type pistaAtajo struct {
	widget.BaseWidget
	Texto string
}

var _ fyne.Widget = (*pistaAtajo)(nil)

func nuevaPistaAtajo(texto string) *pistaAtajo {
	p := &pistaAtajo{Texto: texto}
	p.ExtendBaseWidget(p)
	return p
}

// Visible2 enseña u oculta la pista. (`Visible` ya es de `BaseWidget`.)
func (p *pistaAtajo) Visible2(si bool) {
	if si {
		p.Show()
		return
	}
	p.Hide()
}

func (p *pistaAtajo) MinSize() fyne.Size {
	p.ExtendBaseWidget(p)
	return fyne.NewSize(
		Medir(p.Texto, EscalonAtajo, Medio).Width+2*(MargenAtajo+tema.Borde),
		EscalonAtajo.Linea+2*(0.5*0.25*tema.Rem+tema.Borde))
}

func (p *pistaAtajo) CreateRenderer() fyne.WidgetRenderer {
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	r := &pistaAtajoRenderer{
		p:      p,
		caja:   NuevaCaja(dentroDelCampo(v), tema.Color("chrome-border", v), tema.RadioPill),
		rotulo: Texto(p.Texto, EscalonAtajo, Medio, tema.Color("on-chrome-muted", v)),
	}
	r.objetos = append(r.caja.Objetos(), r.rotulo)
	return r
}

type pistaAtajoRenderer struct {
	p       *pistaAtajo
	caja    *Caja
	rotulo  *canvas.Text
	objetos []fyne.CanvasObject
}

func (r *pistaAtajoRenderer) MinSize() fyne.Size           { return r.p.MinSize() }
func (r *pistaAtajoRenderer) Objects() []fyne.CanvasObject { return r.objetos }
func (r *pistaAtajoRenderer) Destroy()                     {}

func (r *pistaAtajoRenderer) Layout(t fyne.Size) {
	r.caja.Redimensionar(t)
	ColocarTexto(r.rotulo, EscalonAtajo, MargenAtajo+tema.Borde, (t.Height-EscalonAtajo.Linea)/2)
}

func (r *pistaAtajoRenderer) Refresh() {
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	r.caja.Pintar(dentroDelCampo(v), tema.Color("chrome-border", v))
	r.rotulo.Color = tema.Color("on-chrome-muted", v)
	r.rotulo.Refresh()
}
