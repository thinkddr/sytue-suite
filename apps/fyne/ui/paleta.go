// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos
// paridad-web: apps/frontend/packages/ui/src/components/PaletaDeComandos.tsx

package ui

import (
	"context"
	"image/color"
	"strings"
	"sync"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/driver/desktop"

	"git.sytue.eu/sytue/one/apps/fyne/iconos"
	"git.sytue.eu/sytue/one/apps/fyne/tema"
	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/buscar"
	"git.sytue.eu/sytue/one/apps/nucleo/i18n"
)

// # La paleta de la suite (⌘K / Ctrl+K)
//
// Es `PaletaDeLaSuite` de @sytue/shell sobre `PaletaDeComandos` de @sytue/ui: buscar en
// Sytue One y, encima, lo que la app en la que se está ha registrado como CAPA (sus
// pantallas, sus acciones, sus registros). En Fyne había una sola paleta, la del ERP,
// con su propio ⌘K que la barra le cedía; ahora ⌘K abre SIEMPRE esta, la de la barra de
// la suite, y el ERP es una capa más, como en la web.
//
// Lo que se reparte igual que allí:
//
//   - La barra (`BarraSuite`) la crea, escucha ⌘K y la abre o la cierra; cualquier
//     botón la abre con `Abrir` (el `abrirPaleta()` de la web).
//   - Cada app registra su capa con `PonerCapa(id, capa)` y la quita con `QuitarCapa`;
//     quien busca en su servidor oye la consulta con `Oir` (el `usePaletaDeLaSuite`).
//   - Con dos letras o más pregunta también al Hub y pone sus resultados debajo,
//     agrupados por producto.
//
// La CAPA visual es la de un diálogo —el velo, la superficie, el radio de tarjeta y la
// sombra `xl`— en `Overlays()`, con su atrapa-foco; pero su cabecera es el CAMPO, no un
// título con aspa, y el foco no sale de él: las flechas mueven la opción activa. En el
// teléfono, la HOJA a toda la altura, con «Cancelar» junto al campo y sin el pie de
// teclas. Las medidas salen de Chromium (`medir-paleta.mjs`, 2026-10-16): a 1280×800 el
// panel va en (360, 96) con 560 de ancho; a 390×844, en (0, 40) a todo el ancho.

// Medidas de la paleta, en px de CSS.
const (
	// AnchoPaleta es `md:max-w-[560px]`: px literales, el rem no los toca.
	AnchoPaleta float32 = 560
	// FraccionArribaPaleta es el `md:pt-[12dvh]` y FraccionAltoPaleta el `md:max-h-[70dvh]`.
	FraccionArribaPaleta float32 = 0.12
	FraccionAltoPaleta   float32 = 0.70
	// IconoPaleta es la lupa de 16, y el icono de una opción.
	IconoPaleta float32 = 16
)

var (
	// PaletaPX es el `px-4` de la fila del campo y del pie; HuecoCampoPaleta su `gap-2.5`.
	PaletaPX         = tema.Espacio(4)
	HuecoCampoPaleta = tema.Espacio(2.5)
	// AltoCampoPaleta es el `h-12` del campo; en la hoja, `max-md:h-14` con `pt-1` encima.
	AltoCampoPaleta     = tema.Espacio(12)
	AltoCampoPaletaHoja = tema.Espacio(14)
	ArribaCampoHoja     = tema.Espacio(1)
	// ListaPaletaP es el `p-1.5` de la lista; FilaPaletaPX y FilaPaletaPY los `px-2.5 py-2`
	// de una opción, sobre `text-base`.
	ListaPaletaP = tema.Espacio(1.5)
	FilaPaletaPX = tema.Espacio(2.5)
	FilaPaletaPY = tema.Espacio(2)
	// AltoVacioPaleta es «Nada coincide»: `py-6` sobre `text-sm`.
	AltoVacioPaleta = tema.TextoSM.Linea + 2*tema.Espacio(6)
	// GrupoPaletaPX y GrupoPaletaPT/PB son el `px-2.5 pt-3 pb-1` del rótulo de un grupo
	// (`sytue-overline`); el primero de la lista lleva `first:pt-1`.
	GrupoPaletaPX       = tema.Espacio(2.5)
	GrupoPaletaPT       = tema.Espacio(3)
	GrupoPaletaPTPrimer = tema.Espacio(1)
	GrupoPaletaPB       = tema.Espacio(1)
	// PiePaletaPY es el `py-2` del pie y HuecoPiePaleta su `gap-3`.
	PiePaletaPY    = tema.Espacio(2)
	HuecoPiePaleta = tema.Espacio(3)
)

// AltoGrupoPaleta es lo que ocupa el rótulo de un grupo: su caja de `text-xs` con su
// aire. El primero lleva menos arriba, que ya tiene el `p-1.5` de la lista.
func AltoGrupoPaleta(primero bool) float32 {
	arriba := GrupoPaletaPT
	if primero {
		arriba = GrupoPaletaPTPrimer
	}
	return arriba + tema.TextoXS.Linea + GrupoPaletaPB
}

// AltoFilaPaleta es `py-2` sobre la caja de `text-base`; con el dedo, los 44 de
// `pointer-coarse:min-h-11`.
func AltoFilaPaleta() float32 {
	alto := tema.TextoBase.Linea + 2*FilaPaletaPY
	if EsTactil() {
		return max(alto, SueloTactilBarra)
	}
	return alto
}

// AltoPiePaleta es el pie de las teclas: `py-2` alrededor de la tecla de `h-5`, que es
// más alta que la línea de `text-xs`, con su filete de arriba.
func AltoPiePaleta() float32 {
	return tema.Borde + 2*PiePaletaPY + max(LadoTecla, tema.TextoXS.Linea)
}

// ComandoDePaleta es lo que una app ofrece en la paleta (`ComandoDeApp` de la web). Los
// textos llegan YA traducidos.
type ComandoDePaleta struct {
	ID string
	// Rotulo es lo que se lee, y lo primero en que se busca.
	Rotulo string
	// Grupo es el encabezado; también entra en la búsqueda («áreas» encuentra el grupo).
	Grupo string
	// Icono va a la izquierda; `iconos.Ninguno`, sin icono.
	Icono iconos.Icono
	// Pista es el contexto a la derecha (el área, la referencia).
	Pista string
	// Palabras son términos que se buscan pero no se ven (alias, sinónimos).
	Palabras string
	// YaFiltrado: lo buscó el servidor con la consulta de ahora (los registros del ERP)
	// y no pasa otra vez por el filtro, que lo descartaría por el grupo.
	YaFiltrado bool
	// Ejecutar es lo que pasa al elegirlo: navegar o hacer algo de verdad.
	Ejecutar func()
}

// CapaDePaleta es lo que registra una app (o una parte de ella).
type CapaDePaleta struct {
	Comandos []ComandoDePaleta
	// Limite es cuántas opciones de esta capa caben como mucho; cero, sin tope (el ERP
	// tiene cientos de modelos y la lista no es un índice).
	Limite int
	// Buscando: la app aún busca lo de esta capa en su servidor; mientras, la paleta no
	// afirma que no haya nada.
	Buscando bool
}

// ComandosQueCasan es `comandosQueCasan` de @sytue/shell: lo que casa con la consulta,
// capa a capa en el orden en que se registraron y dentro de cada una en el suyo. El
// filtro es `filterCommands` de @sytue/ui: cada palabra de la consulta tiene que estar
// en el rótulo, las palabras, la pista o el grupo, sin acentos ni mayúsculas (la regla
// de `nucleo/buscar`, la misma que `plegar` en la web).
func ComandosQueCasan(capas []CapaDePaleta, consulta string) []ComandoDePaleta {
	tokens := strings.Fields(buscar.Plegar(consulta))
	var out []ComandoDePaleta
	for _, capa := range capas {
		var casan []ComandoDePaleta
		for _, c := range capa.Comandos {
			if !c.YaFiltrado && casaComando(c, tokens) {
				casan = append(casan, c)
			}
		}
		for _, c := range capa.Comandos {
			if c.YaFiltrado {
				casan = append(casan, c)
			}
		}
		if capa.Limite > 0 && len(casan) > capa.Limite {
			casan = casan[:capa.Limite]
		}
		out = append(out, casan...)
	}
	return out
}

func casaComando(c ComandoDePaleta, tokens []string) bool {
	if len(tokens) == 0 {
		return true
	}
	hay := buscar.Plegar(c.Rotulo + " " + strings.TrimSpace(c.Palabras+" "+c.Pista) + " " + c.Grupo)
	for _, t := range tokens {
		if !strings.Contains(hay, t) {
			return false
		}
	}
	return true
}

// OpcionDePaleta es una fila de la lista: o un comando de una capa o un resultado del
// Hub. Lleva exactamente uno de los dos.
type OpcionDePaleta struct {
	Rotulo, Grupo, Pista string
	Icono                iconos.Icono
	Comando              *ComandoDePaleta
	Resultado            *api.HubHit
}

// EsperaPaleta es el `ESPERA_BUSQUEDA_MS` de la shell entre la última tecla y la
// pregunta al Hub: el mismo que el buscador de la barra.
var EsperaPaleta = EsperaBuscador

// PaletaDeLaSuite es la paleta: el registro de capas, lo escrito, la fila resaltada, lo
// que contestó el Hub y la capa que la pinta mientras está abierta.
//
// Todo se toca desde el hilo de la interfaz salvo lo del Hub, que llega de la rutina de
// la búsqueda y va bajo `mu`.
type PaletaDeLaSuite struct {
	t *i18n.Translator
	// BuscarEnHub pregunta al Hub. Nil: no hay Hub (una barra sin sesión, los
	// guardianes) y la paleta ofrece solo las capas.
	BuscarEnHub func(ctx context.Context, q string) ([]api.HubHit, error)
	// AlAbrirResultado abre un resultado del Hub. Nil es abrirlo en el sistema, como el
	// buscador de la barra (`api.AbrirURL`).
	AlAbrirResultado func(api.HubHit)

	ids     []string
	capas   map[string]CapaDePaleta
	oyentes map[int]func(abierta bool, consulta string)
	ultimo  int

	abierta  bool
	consulta string
	cursor   int

	mu          sync.Mutex
	hits        []api.HubHit
	buscandoHub bool
	falloHub    bool
	espera      *time.Timer
	cancelar    context.CancelFunc
	generacion  int

	lienzo fyne.Canvas
	campo  *Entrada
	capa   *capa
	previo fyne.Focusable
	panel  *fyne.Container
	disp   *panelPaleta
	// desplazado es dónde estaba la lista: cada cambio de opciones rehace el árbol, y
	// sin recordarlo la lista volvería arriba.
	desplazado float32
}

// NuevaPaletaDeLaSuite crea la paleta cerrada y sin capas.
func NuevaPaletaDeLaSuite(t *i18n.Translator) *PaletaDeLaSuite {
	p := &PaletaDeLaSuite{t: t, capas: map[string]CapaDePaleta{}, oyentes: map[int]func(bool, string){}}
	// `bg-transparent text-base focus-visible:outline-none`: el campo NO es una caja,
	// es la cabecera del panel, con la lupa a la izquierda y el filete debajo.
	p.campo = NuevaEntrada(t.T("suitePalette.placeholder", nil)).ConPropia(CampoPropio{
		MargenY: (AltoCampoPaleta - 2*tema.Borde - tema.TextoBase.Linea) / 2,
		Texto:   tema.TextoBase, Plano: true,
	})
	p.campo.Rotulo = t.T("suitePalette.input", nil)
	p.campo.AlCambiar(func(s string) {
		if s != p.consulta {
			p.Escribir(s)
		}
	})
	p.campo.AlTeclear = p.Teclear
	return p
}

// EnLienzo dice sobre qué ventana se abre. La pone la barra al montarse; sin lienzo la
// paleta guarda su estado pero no pinta nada (los guardianes de una app suelta).
func (p *PaletaDeLaSuite) EnLienzo(c fyne.Canvas) { p.lienzo = c }

// Campo es el buscador de la paleta: tiene el foco mientras está abierta.
func (p *PaletaDeLaSuite) Campo() fyne.Focusable { return p.campo }

// Abierta dice si la paleta está a la vista.
func (p *PaletaDeLaSuite) Abierta() bool { return p.abierta }

// Consulta es lo escrito, tal cual.
func (p *PaletaDeLaSuite) Consulta() string { return p.consulta }

// Cursor es la fila resaltada, acotada a las opciones de ahora; -1 si no hay ninguna.
func (p *PaletaDeLaSuite) Cursor() int {
	n := len(p.Opciones())
	if n == 0 {
		return -1
	}
	return min(max(p.cursor, 0), n-1)
}

// PonerCapa registra (o cambia) la capa `id`. Una capa nueva va al final; una que ya
// estaba conserva su sitio, como el `Map` de la web.
func (p *PaletaDeLaSuite) PonerCapa(id string, c CapaDePaleta) {
	if _, ya := p.capas[id]; !ya {
		p.ids = append(p.ids, id)
	}
	p.capas[id] = c
	p.pintar()
}

// QuitarCapa retira la capa `id`, si estaba.
func (p *PaletaDeLaSuite) QuitarCapa(id string) {
	if _, ya := p.capas[id]; !ya {
		return
	}
	delete(p.capas, id)
	for i, x := range p.ids {
		if x == id {
			p.ids = append(p.ids[:i:i], p.ids[i+1:]...)
			break
		}
	}
	p.pintar()
}

// Capas son las registradas, en orden.
func (p *PaletaDeLaSuite) Capas() []CapaDePaleta {
	out := make([]CapaDePaleta, 0, len(p.ids))
	for _, id := range p.ids {
		out = append(out, p.capas[id])
	}
	return out
}

// Oir avisa a `f` cada vez que la paleta se abre, se cierra o cambia lo escrito: es lo
// que lee de `usePaletaDeLaSuite` quien busca en su servidor (los registros del ERP).
// Devuelve con qué dejar de oír.
func (p *PaletaDeLaSuite) Oir(f func(abierta bool, consulta string)) (dejar func()) {
	p.ultimo++
	n := p.ultimo
	p.oyentes[n] = f
	return func() { delete(p.oyentes, n) }
}

func (p *PaletaDeLaSuite) avisar() {
	for i := 1; i <= p.ultimo; i++ {
		if f := p.oyentes[i]; f != nil {
			f(p.abierta, p.consulta)
		}
	}
}

// Abrir la pone a la vista con el campo vacío y el foco en él: cada apertura empieza en
// blanco, como Spotlight. Abierta, no hace nada.
func (p *PaletaDeLaSuite) Abrir() {
	if p.abierta {
		return
	}
	p.abierta = true
	p.ponerConsulta("")
	p.buscarEnHub()
	p.avisar()
	p.mostrar()
}

// Cerrar la retira sin elegir nada y devuelve el foco a donde estaba.
func (p *PaletaDeLaSuite) Cerrar() {
	if !p.abierta {
		return
	}
	p.abierta = false
	p.buscarEnHub() // lo que estuviera buscando ya no lo va a ver nadie
	p.avisar()
	if p.capa != nil {
		c := lienzoDe(p.capa)
		if c == nil {
			c = p.lienzo
		}
		ocultar(c, p.capa, p.previo)
		p.capa, p.previo, p.panel, p.disp = nil, nil, nil, nil
	}
}

// Alternar es ⌘K: abre la cerrada y cierra la abierta.
func (p *PaletaDeLaSuite) Alternar() {
	if p.abierta {
		p.Cerrar()
		return
	}
	p.Abrir()
}

// Parar suelta la búsqueda que estuviera esperando: la llama la barra al cerrarse la
// ventana, para que su reloj no despierte sobre una paleta que ya no existe.
func (p *PaletaDeLaSuite) Parar() {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.soltarBusqueda()
}

func (p *PaletaDeLaSuite) ponerConsulta(s string) {
	p.consulta, p.cursor, p.desplazado = s, 0, 0
	if p.campo.Texto() != s {
		p.campo.PonerTexto(s)
	}
}

// Escribir cambia lo escrito y vuelve a la primera opción, como la web: con la lista
// recalculada, dejar el cursor donde estaba resalta otra cosa.
func (p *PaletaDeLaSuite) Escribir(s string) {
	p.ponerConsulta(s)
	p.buscarEnHub()
	p.avisar()
	p.pintar()
}

// soltarBusqueda para el reloj y la petición en vuelo. Con `mu`.
func (p *PaletaDeLaSuite) soltarBusqueda() {
	p.generacion++
	if p.espera != nil {
		p.espera.Stop()
		p.espera = nil
	}
	if p.cancelar != nil {
		p.cancelar()
		p.cancelar = nil
	}
}

// buscarEnHub es el efecto de la web: por debajo de dos letras (o cerrada) no se busca y
// se suelta lo que hubiera; por encima, se pregunta 250 ms después de la última tecla.
// Mientras, se dejan los resultados de antes: vaciar la lista en cada letra la hace
// parpadear. Una respuesta de una consulta ya cambiada no pisa la nueva.
func (p *PaletaDeLaSuite) buscarEnHub() {
	q := strings.TrimSpace(p.consulta)
	p.mu.Lock()
	defer p.mu.Unlock()
	p.soltarBusqueda()
	if !p.abierta || len([]rune(q)) < MinimoBuscador || p.BuscarEnHub == nil {
		p.hits, p.buscandoHub, p.falloHub = nil, false, false
		return
	}
	p.buscandoHub = true
	gen, buscarEnHub := p.generacion, p.BuscarEnHub
	ctx, cancelar := context.WithCancel(context.Background())
	p.cancelar = cancelar
	p.espera = time.AfterFunc(EsperaPaleta, func() {
		pidiendo, fin := context.WithTimeout(ctx, 20*time.Second)
		hits, err := buscarEnHub(pidiendo, q)
		fin()
		p.mu.Lock()
		if gen != p.generacion {
			p.mu.Unlock()
			return
		}
		p.hits, p.falloHub, p.buscandoHub = agruparPorOrigen(hits), err != nil, false
		if err != nil {
			p.hits = nil
		}
		p.mu.Unlock()
		fyne.Do(p.pintar)
	})
}

// Buscando dice si falta alguna respuesta: la del Hub o la de una capa.
func (p *PaletaDeLaSuite) Buscando() bool {
	p.mu.Lock()
	hub := p.buscandoHub
	p.mu.Unlock()
	if hub {
		return true
	}
	for _, c := range p.capas {
		if c.Buscando {
			return true
		}
	}
	return false
}

// Opciones son todas las filas, en el orden en que se leen: lo de las capas y, debajo,
// lo del Hub agrupado por producto (el Hub ya pone delante lo mejor de cada uno).
func (p *PaletaDeLaSuite) Opciones() []OpcionDePaleta {
	comandos := ComandosQueCasan(p.Capas(), p.consulta)
	p.mu.Lock()
	hits := p.hits
	p.mu.Unlock()
	out := make([]OpcionDePaleta, 0, len(comandos)+len(hits))
	for _, c := range comandos {
		out = append(out, OpcionDePaleta{Rotulo: c.Rotulo, Grupo: c.Grupo, Pista: c.Pista, Icono: c.Icono, Comando: &c})
	}
	for _, h := range hits {
		out = append(out, OpcionDePaleta{
			Rotulo: h.Title, Grupo: nombreDeOrigen(h.Source),
			Pista: DetalleHit(p.t.Locale(), h), Resultado: &h,
		})
	}
	return out
}

// Vacio es lo que se lee cuando no casa nada: invita a escribir, dice que se está
// buscando o que ha fallado, y solo entonces que no hay nada.
func (p *PaletaDeLaSuite) Vacio() string {
	q := strings.TrimSpace(p.consulta)
	p.mu.Lock()
	fallo := p.falloHub
	p.mu.Unlock()
	switch {
	case q == "":
		return p.t.T("suitePalette.typeToSearch", nil)
	case p.Buscando():
		return p.t.T("search.busy", nil)
	case fallo:
		return p.t.T("search.failedShort", nil)
	}
	return p.t.T("search.emptyTitle", i18n.Vars{"query": q})
}

// Elegir lleva a donde dice la opción. La paleta se va ANTES: si lo elegido pregunta
// algo (dejar una ficha a medias), que la pregunta no quede debajo de otra capa.
func (p *PaletaDeLaSuite) Elegir(o OpcionDePaleta) {
	p.Cerrar()
	switch {
	case o.Comando != nil && o.Comando.Ejecutar != nil:
		o.Comando.Ejecutar()
	case o.Resultado != nil && p.AlAbrirResultado != nil:
		p.AlAbrirResultado(*o.Resultado)
	case o.Resultado != nil:
		_ = api.AbrirURL(fyne.CurrentApp(), o.Resultado.Source, o.Resultado.URL)
	}
}

// Mover baja o sube la fila resaltada, dando la vuelta por los extremos.
func (p *PaletaDeLaSuite) Mover(d int) {
	n := len(p.Opciones())
	if n == 0 {
		p.cursor = 0
		return
	}
	p.cursor = ((p.Cursor()+d)%n + n) % n
	p.marcar()
}

// Teclear son las teclas del campo: las flechas recorren, Inicio y Fin van a los
// extremos, Intro abre lo resaltado, Escape (o «atrás») cierra y Tab no tiene adónde
// ir (las filas no son paradas: `opcionPaleta.Disabled`). Devuelve si se quedó la
// tecla; el resto es del campo.
func (p *PaletaDeLaSuite) Teclear(k *fyne.KeyEvent) bool {
	switch {
	case k == nil:
		return false
	case EsCierre(k):
		p.Cerrar()
	case k.Name == fyne.KeyDown:
		p.Mover(1)
	case k.Name == fyne.KeyUp:
		p.Mover(-1)
	case k.Name == fyne.KeyHome:
		p.cursor = 0
		p.marcar()
	case k.Name == fyne.KeyEnd:
		p.cursor = len(p.Opciones()) - 1
		p.marcar()
	case k.Name == fyne.KeyReturn || k.Name == fyne.KeyEnter:
		if o := p.Opciones(); len(o) > 0 {
			p.Elegir(o[p.Cursor()])
		}
	case k.Name == fyne.KeyTab:
	default:
		return false
	}
	return true
}

// ---- la capa ----

func enHojaPaleta(lienzo fyne.Size) bool { return lienzo.Width > 0 && lienzo.Width < Estrecho }

// mostrar cuelga la capa del lienzo, si lo hay, y le da el foco al campo (el
// `autofocus` de la web).
func (p *PaletaDeLaSuite) mostrar() {
	c := p.lienzo
	if c == nil {
		return
	}
	p.disp = &panelPaleta{p: p, hoja: enHojaPaleta(c.Size())}
	p.panel = container.New(p.disp)
	p.rehacer()
	p.capa = nuevaCapa(NuevoVelo(true, p.Cerrar), p.panel, p.colocar)
	if !enHojaPaleta(c.Size()) {
		p.capa.sombra = NuevaSombra(tema.SombraXL, tema.Forma("radius-card")).SinInterior()
	}
	p.capa.alDescartar = p.Cerrar
	p.previo = mostrar(c, p.capa)
	enfocar(c, p.campo)
	if enHojaPaleta(c.Size()) {
		p.capa.entrar(c, DuracionHoja, p.panel.Size().Height, 1, 1)
		return
	}
	p.capa.entrar(c, DuracionDialogo, DesplazamientoDialogo, EscalaEntrada, 0.5)
}

// colocar es la geometría de la capa: la tarjeta de 560 a 12 dvh, o la hoja desde
// `HuecoHoja` hasta abajo.
func (p *PaletaDeLaSuite) colocar(lienzo fyne.Size) (fyne.Position, fyne.Size) {
	if hoja := enHojaPaleta(lienzo); p.disp != nil && p.disp.hoja != hoja {
		// La ventana ha cruzado los 768 con la paleta abierta: la hoja lleva «Cancelar»
		// y no el pie, así que se rehace entera.
		p.disp.hoja = hoja
		p.rehacer()
	}
	if enHojaPaleta(lienzo) {
		return fyne.NewPos(0, HuecoHoja), fyne.NewSize(lienzo.Width, max(lienzo.Height-HuecoHoja, 0))
	}
	ancho := min(AnchoPaleta, lienzo.Width-2*tema.Espacio(4))
	alto := min(p.disp.altoNatural(), FraccionAltoPaleta*lienzo.Height)
	return fyne.NewPos((lienzo.Width-ancho)/2, lienzo.Height*FraccionArribaPaleta), fyne.NewSize(ancho, alto)
}

// pintar rehace la lista con las opciones de ahora, si la paleta se ve.
func (p *PaletaDeLaSuite) pintar() {
	if p.capa == nil {
		return
	}
	p.rehacer()
	p.capa.Refresh()
}

// marcar mueve el resaltado sin rehacer las filas, y lleva la activa a la vista.
func (p *PaletaDeLaSuite) marcar() {
	if p.disp == nil || p.disp.lista == nil {
		return
	}
	cur := p.Cursor()
	p.disp.lista.cursor = cur
	for i, f := range p.disp.lista.filas {
		if f.resaltada != (i == cur) {
			f.resaltada = i == cur
			f.Refresh()
		}
	}
	p.panel.Refresh()
}

// rehacer construye los hijos del panel con el estado de ahora. El campo es el MISMO de
// una vez a otra: uno nuevo perdería el foco, y con él las flechas, a la primera tecla.
func (p *PaletaDeLaSuite) rehacer() {
	l, v := p.disp, tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	l.caja = NuevaCaja(tema.Color("menu-surface", v), tema.Color("border", v), tema.Forma("radius-card"))
	// `menu-surface` y no `panel-surface`: en el modo traslúcido el panel de los
	// diálogos deja ver lo de detrás, y la web lo compensa con el desenfoque de
	// `sytue-glass-surface`, que aquí no hay; la lista tiene que leerse sobre cualquier
	// pantalla (`TestLaPaletaTraslucidaOcultaLaPantalla`).
	l.filete = canvas.NewRectangle(tema.Color("border-subtle", v))
	l.lupa = iconos.Trazo(iconos.Buscar, IconoPaleta, "text-subtle", v).Objeto()
	hijos := append(l.caja.Objetos(), l.lupa, p.campo, l.filete)
	l.cancelar = nil
	if l.hoja {
		// Con el dedo no hay Escape: la salida va donde está el pulgar al escribir.
		l.cancelar = NuevoBoton(p.t.T("ui.cancel", nil), p.Cerrar).Con(Fantasma, TallaMD)
		hijos = append(hijos, l.cancelar)
	}
	opciones := p.Opciones()
	l.vacio, l.lista, l.desplaza = nil, nil, nil
	if len(opciones) == 0 {
		l.vacio = NuevoParrafo(p.Vacio(), tema.TextoSM, Normal, "text-muted", v)
		hijos = append(hijos, l.vacio)
	} else {
		cursor := min(max(p.cursor, 0), len(opciones)-1)
		l.lista = &listaPaleta{cursor: cursor}
		var objs []fyne.CanvasObject
		for i, o := range opciones {
			if o.Grupo != "" && (i == 0 || opciones[i-1].Grupo != o.Grupo) {
				g := grupoPaleta{
					antesDe: i, primero: i == 0,
					rotulo: NuevoVersal(o.Grupo, tema.TextoXS, SemiNegrita, tema.Color("text-subtle", v)),
				}
				l.lista.grupos = append(l.lista.grupos, g)
				objs = append(objs, g.rotulo.Objetos()...)
			}
			indice, opcion := i, o
			f := nuevaOpcionPaleta(o.Rotulo, o.Pista, o.Icono, i == cursor, func() { p.Elegir(opcion) })
			// `onMouseEnter={() => setActiva(n)}`: el ratón mueve el resaltado, y así
			// Intro abre lo que se está señalando y no otra cosa.
			f.alEntrar = func() {
				if p.Cursor() != indice {
					p.cursor = indice
					p.marcar()
				}
			}
			l.lista.filas = append(l.lista.filas, f)
			objs = append(objs, f)
		}
		l.desplaza = container.NewVScroll(container.New(l.lista, objs...))
		l.desplaza.Offset.Y = p.desplazado
		l.desplaza.OnScrolled = func(o fyne.Position) { p.desplazado = o.Y }
		hijos = append(hijos, l.desplaza)
	}
	l.pie = nil
	if !l.hoja {
		l.pie = nuevoPiePaleta(p.t, v)
		hijos = append(hijos, l.pie.objetos()...)
	}
	p.panel.Objects = hijos
}

// ---- el panel ----

// panelPaleta coloca, dentro del panel, la caja, la fila del campo, la lista y el pie.
type panelPaleta struct {
	p        *PaletaDeLaSuite
	hoja     bool
	caja     *Caja
	lupa     fyne.CanvasObject
	cancelar *Boton
	filete   *canvas.Rectangle
	vacio    *Parrafo
	lista    *listaPaleta
	desplaza *container.Scroll
	pie      *piePaleta
}

var _ fyne.Layout = (*panelPaleta)(nil)

// altoCabecera es la fila del campo con su filete de abajo.
func (l *panelPaleta) altoCabecera() float32 {
	if l.hoja {
		return ArribaCampoHoja + AltoCampoPaletaHoja + tema.Borde
	}
	return AltoCampoPaleta + tema.Borde
}

func (l *panelPaleta) altoPie() float32 {
	if l.pie == nil {
		return 0
	}
	return AltoPiePaleta()
}

// altoNatural es lo que ocupa el panel en escritorio sin tope: la cabecera, la lista
// con su `p-1.5` y el pie. Quien lo coloca lo acota al `max-h-[70dvh]`.
func (l *panelPaleta) altoNatural() float32 {
	lista := AltoVacioPaleta
	if l.lista != nil {
		lista, _, _ = l.lista.colocar(0, false)
	}
	return 2*tema.Borde + l.altoCabecera() + lista + 2*ListaPaletaP + l.altoPie()
}

func (l *panelPaleta) MinSize([]fyne.CanvasObject) fyne.Size { return fyne.Size{} }

func (l *panelPaleta) Layout(_ []fyne.CanvasObject, t fyne.Size) {
	b := tema.Borde
	lado := b // el filete de los lados; la hoja no lo lleva
	if l.hoja {
		lado = 0
		// Redonda solo arriba, sin filetes a los lados ni abajo: la caja 1 px más ancha
		// por cada lado y más alta, como la hoja de `Dialogo`.
		MoverCaja(l.caja, fyne.NewPos(-b, 0), fyne.NewSize(t.Width+2*b, t.Height+b))
		for _, o := range l.caja.Objetos() {
			if r, es := o.(*canvas.Rectangle); es {
				r.BottomLeftCornerRadius, r.BottomRightCornerRadius = esquinaRecta, esquinaRecta
			}
		}
	} else {
		MoverCaja(l.caja, fyne.NewPos(0, 0), t)
	}

	// La fila del campo: la lupa, el campo que crece y, en la hoja, «Cancelar».
	arriba, altoCampo := b, AltoCampoPaleta
	if l.hoja {
		arriba += ArribaCampoHoja
		altoCampo = AltoCampoPaletaHoja
	}
	cx := lado + PaletaPX
	l.lupa.Move(fyne.NewPos(cx, arriba+(altoCampo-IconoPaleta)/2))
	l.lupa.Resize(fyne.NewSize(IconoPaleta, IconoPaleta))
	cx += IconoPaleta + HuecoCampoPaleta
	fin := t.Width - lado - PaletaPX
	if l.cancelar != nil {
		m := l.cancelar.MinSize()
		// `-mr-2`: el botón se mete en el `px-4` para que su rótulo quede alineado.
		fin += tema.Espacio(2)
		l.cancelar.Resize(m)
		l.cancelar.Move(fyne.NewPos(fin-m.Width, arriba+(altoCampo-m.Height)/2))
		fin -= m.Width + HuecoCampoPaleta
	}
	campo := l.p.campo
	campo.Move(fyne.NewPos(cx, arriba))
	campo.Resize(fyne.NewSize(max(fin-cx, 0), altoCampo))
	l.filete.Move(fyne.NewPos(lado, arriba+altoCampo))
	l.filete.Resize(fyne.NewSize(t.Width-2*lado, b))

	fy := arriba + altoCampo + b + ListaPaletaP
	abajo := t.Height - b - l.altoPie()
	if l.pie != nil {
		l.pie.colocar(lado, abajo, t.Width-2*lado)
	}
	ancho := t.Width - 2*lado - 2*ListaPaletaP
	if l.vacio != nil {
		l.vacio.Move(fyne.NewPos(lado+ListaPaletaP+(ancho-Medir(l.vacio.Texto, tema.TextoSM, Normal).Width)/2, fy+tema.Espacio(6)))
		l.vacio.Resize(fyne.NewSize(ancho, tema.TextoSM.Linea))
	}
	if l.desplaza != nil {
		// La lista llena lo que queda del panel menos su `p-1.5` de abajo, y la fila del
		// cursor siempre a la vista (el `scrollIntoView({ block: 'nearest' })`).
		visible := max(abajo-ListaPaletaP-fy, 0)
		l.desplaza.Move(fyne.NewPos(lado+ListaPaletaP, fy))
		l.desplaza.Resize(fyne.NewSize(ancho, visible))
		total, arribaCur, abajoCur := l.lista.colocar(0, false)
		off := l.desplaza.Offset.Y
		if arribaCur < off {
			off = arribaCur
		} else if abajoCur > off+visible {
			off = abajoCur - visible
		}
		off = min(max(off, 0), max(total-visible, 0))
		if off != l.desplaza.Offset.Y {
			l.desplaza.ScrollToOffset(fyne.NewPos(0, off))
		}
	}
}

// grupoPaleta es el rótulo de un grupo (`sytue-overline`), que va antes de la fila
// `antesDe`.
type grupoPaleta struct {
	antesDe int
	primero bool
	rotulo  *Versal
}

// listaPaleta coloca los rótulos de grupo y las filas una debajo de otra, y sabe dónde
// cae la fila del cursor para que la lista la tenga a la vista.
type listaPaleta struct {
	grupos []grupoPaleta
	filas  []*opcionPaleta
	cursor int
}

var _ fyne.Layout = (*listaPaleta)(nil)

// colocar recorre la lista; con `poner` coloca cada pieza. Devuelve el alto y dónde
// empieza y acaba la fila del cursor.
func (l *listaPaleta) colocar(ancho float32, poner bool) (alto, arribaCursor, abajoCursor float32) {
	y, g := float32(0), 0
	for i, f := range l.filas {
		for g < len(l.grupos) && l.grupos[g].antesDe == i {
			gr := l.grupos[g]
			arriba := GrupoPaletaPT
			if gr.primero {
				arriba = GrupoPaletaPTPrimer
			}
			if poner {
				gr.rotulo.Colocar(GrupoPaletaPX, y+arriba)
				gr.rotulo.Recortar(max(ancho-2*GrupoPaletaPX, 0))
			}
			y += AltoGrupoPaleta(gr.primero)
			g++
		}
		if poner {
			f.Move(fyne.NewPos(0, y))
			f.Resize(fyne.NewSize(ancho, AltoFilaPaleta()))
		}
		if i == l.cursor {
			arribaCursor, abajoCursor = y, y+AltoFilaPaleta()
		}
		y += AltoFilaPaleta()
	}
	return y, arribaCursor, abajoCursor
}

func (l *listaPaleta) MinSize([]fyne.CanvasObject) fyne.Size {
	alto, _, _ := l.colocar(0, false)
	return fyne.NewSize(0, alto)
}

func (l *listaPaleta) Layout(_ []fyne.CanvasObject, t fyne.Size) { l.colocar(t.Width, true) }

// opcionPaleta es una opción: el icono si lo hay, lo que se lee y, a la derecha, su
// pista. La activa lleva `bg-selected` con `rounded-md` y el icono en `primary-text`.
type opcionPaleta struct {
	pulsable
	rotulo, pista string
	icono         iconos.Icono
	resaltada     bool
	// alEntrar avisa de que el ratón ha entrado, para mover el cursor.
	alEntrar func()
}

func nuevaOpcionPaleta(rotulo, pista string, icono iconos.Icono, resaltada bool, alPulsar func()) *opcionPaleta {
	f := &opcionPaleta{rotulo: rotulo, pista: pista, icono: icono, resaltada: resaltada}
	f.Nombre, f.AlPulsar = rotulo, alPulsar
	f.ExtendBaseWidget(f)
	return f
}

var _ fyne.Disableable = (*opcionPaleta)(nil)

// Disabled aparta la fila del recorrido del Tab, y es lo ÚNICO que aparta: se sigue
// pulsando con el ratón o el dedo y se sigue resaltando al pasar. Es el `<li
// role="option">` de la web, que no es enfocable: en el patrón del combobox el foco se
// queda en el campo y las flechas mueven la opción activa (`Teclear`). Fyne no tiene
// `tabIndex={-1}`, y su gestor de foco salta los `Disableable` inhabilitados (ver
// `Boton.Disabled`). Con las filas como paradas, el Tab sacaba el foco del campo y las
// flechas dejaban de mover el resaltado.
func (f *opcionPaleta) Disabled() bool { return true }

// Enable y Disable no hacen nada: la fila nunca es parada del Tab.
func (f *opcionPaleta) Enable() {}

// Disable no hace nada (ver Enable).
func (f *opcionPaleta) Disable() {}

// MouseIn mueve el cursor a esta fila además del hover de `pulsable`.
func (f *opcionPaleta) MouseIn(e *desktop.MouseEvent) {
	f.pulsable.MouseIn(e)
	if f.alEntrar != nil {
		f.alEntrar()
	}
}

func (f *opcionPaleta) MinSize() fyne.Size {
	f.ExtendBaseWidget(f)
	return fyne.NewSize(0, AltoFilaPaleta())
}

func (f *opcionPaleta) CreateRenderer() fyne.WidgetRenderer {
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	r := &opcionPaletaRenderer{
		f:      f,
		fondo:  NuevaCaja(color.NRGBA{}, color.NRGBA{}, tema.RadioMD),
		nombre: Texto(f.rotulo, tema.TextoBase, Normal, tema.Color("text", v)),
		pista:  Texto(f.pista, tema.TextoXS, Normal, tema.Color("text-subtle", v)),
	}
	r.objetos = append(r.fondo.Objetos(), r.nombre, r.pista)
	if f.icono != iconos.Ninguno {
		r.icono = iconos.Trazo(f.icono, IconoPaleta, r.tokenIcono(), v)
		r.objetos = append(r.objetos, r.icono.Objeto())
	}
	r.Refresh()
	return r
}

type opcionPaletaRenderer struct {
	f             *opcionPaleta
	fondo         *Caja
	icono         *iconos.Dibujo
	nombre, pista *canvas.Text
	objetos       []fyne.CanvasObject
}

func (r *opcionPaletaRenderer) tokenIcono() string {
	if r.f.resaltada {
		return "primary-text"
	}
	return "text-subtle"
}

func (r *opcionPaletaRenderer) MinSize() fyne.Size           { return r.f.MinSize() }
func (r *opcionPaletaRenderer) Objects() []fyne.CanvasObject { return r.objetos }
func (r *opcionPaletaRenderer) Destroy()                     {}

func (r *opcionPaletaRenderer) Layout(t fyne.Size) {
	r.fondo.Redimensionar(t)
	x := FilaPaletaPX
	if r.icono != nil {
		r.icono.Colocar(x, (t.Height-IconoPaleta)/2, IconoPaleta)
		x += IconoPaleta + HuecoCampoPaleta
	}
	// La pista: `max-w-[40%] truncate`, pegada a la derecha.
	pista := min(Medir(r.f.pista, tema.TextoXS, Normal).Width, 0.4*max(t.Width-2*FilaPaletaPX, 0))
	Truncar(r.pista, r.f.pista, tema.TextoXS, Normal, pista)
	ColocarTexto(r.pista, tema.TextoXS, t.Width-FilaPaletaPX-pista, (t.Height-tema.TextoXS.Linea)/2)
	hueco := float32(0)
	if pista > 0 {
		hueco = HuecoCampoPaleta
	}
	// El rótulo SIN recortar, siempre: truncar sobre lo truncado deja «…» para siempre
	// en cuanto el primer layout llega con ancho cero.
	Truncar(r.nombre, r.f.rotulo, tema.TextoBase, Normal, max(t.Width-x-FilaPaletaPX-pista-hueco, 0))
	ColocarTexto(r.nombre, tema.TextoBase, x, (t.Height-tema.TextoBase.Linea)/2)
}

func (r *opcionPaletaRenderer) Refresh() {
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	fondo := color.NRGBA{}
	if r.f.resaltada {
		fondo = tema.Color("selected", v)
	}
	r.fondo.Pintar(fondo, color.NRGBA{})
	r.nombre.Color = tema.Color("text", v)
	r.nombre.Refresh()
	r.pista.Color = tema.Color("text-subtle", v)
	r.pista.Refresh()
	if r.icono != nil {
		r.icono.Tenir(r.tokenIcono(), v)
	}
}

// ---- el pie ----

// piePaleta es el pie de escritorio: `↑↓ mover`, `↵ abrir`, `esc cerrar` con su `Kbd`, y
// el atajo de la paleta a la derecha, en `sytue-meta`.
type piePaleta struct {
	filete *canvas.Rectangle
	teclas []*Tecla
	textos []*canvas.Text
	atajo  *canvas.Text
}

func nuevoPiePaleta(t *i18n.Translator, v tema.Variante) *piePaleta {
	p := &piePaleta{filete: canvas.NewRectangle(tema.Color("border-subtle", v))}
	tinta := tema.Color("text-subtle", v)
	for _, par := range [][2]string{
		{"↑↓", "suitePalette.hint.move"}, {"↵", "suitePalette.hint.open"}, {"esc", "suitePalette.hint.close"},
	} {
		p.teclas = append(p.teclas, TeclaSuperficie(par[0]))
		p.textos = append(p.textos, Texto(t.T(par[1], nil), tema.TextoXS, Normal, tinta))
	}
	p.atajo = Texto(AtajoDelSistema("Ctrl K"), tema.TextoXS, Normal, tinta)
	return p
}

func (p *piePaleta) objetos() []fyne.CanvasObject {
	out := []fyne.CanvasObject{p.filete}
	for i := range p.teclas {
		out = append(out, p.teclas[i], p.textos[i])
	}
	return append(out, p.atajo)
}

// colocar pone el pie con su filete arriba en `y`, a partir de `x` y en `ancho`.
func (p *piePaleta) colocar(x, y, ancho float32) {
	p.filete.Move(fyne.NewPos(x, y))
	p.filete.Resize(fyne.NewSize(ancho, tema.Borde))
	fila := max(LadoTecla, tema.TextoXS.Linea)
	arriba := y + tema.Borde + PiePaletaPY
	espacio := Medir(" ", tema.TextoXS, Normal).Width
	cx := x + PaletaPX
	for i, k := range p.teclas {
		m := k.MinSize()
		k.Resize(m)
		k.Move(fyne.NewPos(cx, arriba+(fila-m.Height)/2))
		cx += m.Width + espacio
		ColocarTexto(p.textos[i], tema.TextoXS, cx, arriba+(fila-tema.TextoXS.Linea)/2)
		cx += Medir(p.textos[i].Text, tema.TextoXS, Normal).Width + HuecoPiePaleta
	}
	w := Medir(p.atajo.Text, tema.TextoXS, Normal).Width
	ColocarTexto(p.atajo, tema.TextoXS, x+ancho-PaletaPX-w, arriba+(fila-tema.TextoXS.Linea)/2)
}
