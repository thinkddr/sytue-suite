// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

// paridad-web: apps/frontend/apps/portal/src/pages/descargas.astro

package hub

import (
	"cmp"
	"regexp"
	"runtime"
	"slices"
	"strings"
	"unicode"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"

	"git.sytue.eu/sytue/one/apps/fyne/iconos"
	"git.sytue.eu/sytue/one/apps/fyne/tema"
	"git.sytue.eu/sytue/one/apps/fyne/ui"
	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/i18n"
)

// «Descarga las aplicaciones»: `descargas.astro`. La lista la pide al catálogo de
// instaladores del Hub (`GET /api/v1/instaladores`, el mismo del actualizador), que es
// quien lleva la credencial del almacenamiento, y
// cada instalador se baja con el token a la carpeta de descargas del sistema, que es lo
// que hace el navegador con su `<a download>`.
//
// La página no es un `SettingsShell`: es una `section` suelta con
// `mx-auto max-w-5xl flex-col gap-6 px-4 py-8`, así que el margen es `px-4` también en
// escritorio (a 1280 la caja empieza en x = 206 y mide 868).
//
// Los dos «botones» de la web llevan `sy-btn sy-btn--primary` y `sy-btn--ghost`, dos
// clases que NO define ninguna hoja del repositorio (desde el primer commit): el
// navegador los pinta como un `<a>` sin clases, texto raíz en la tinta del texto y sin
// subrayado. Se copia lo que la web HACE; el arreglo, si se decide, va en la web y aquí
// a la vez.

// Medidas propias de la página, de las clases del Astro.
const (
	// HuecoCabeceraDescargas es el `gap-1` de la cabecera.
	HuecoCabeceraDescargas float32 = 1 * 0.25 * tema.Rem
	// HuecoGrupoDescargas es el `gap-3` entre el `h2` y su lista, y entre tarjetas.
	HuecoGrupoDescargas float32 = 3 * 0.25 * tema.Rem
	// RellenoVacioDescargas es el `p-6` de la caja de «todavía no hay», y
	// MargenCuerpoVacio el `mt-1` de su segunda línea.
	RellenoVacioDescargas float32 = 6 * 0.25 * tema.Rem
	MargenCuerpoVacio     float32 = 1 * 0.25 * tema.Rem
	// RellenoTarjetaDescarga es el `p-4` de cada tarjeta destacada, y
	// MargenEnlaceTarjeta el `mt-3` de su enlace.
	RellenoTarjetaDescarga float32 = 4 * 0.25 * tema.Rem
	MargenEnlaceTarjeta    float32 = 3 * 0.25 * tema.Rem
	// La fila de la lista: `px-4 py-3` y `gap-4` entre el nombre y el enlace.
	RellenoFilaDescargaX float32 = 4 * 0.25 * tema.Rem
	RellenoFilaDescargaY float32 = 3 * 0.25 * tema.Rem
	HuecoFilaDescarga    float32 = 4 * 0.25 * tema.Rem
)

// NombrePlataforma es el `NOMBRE` de la página: no se traduce.
var NombrePlataforma = map[string]string{
	"macos": "macOS", "windows": "Windows", "linux": "Linux", "android": "Android",
}

// NombreArquitectura es el `ARQUITECTURA` de la página: nomenclatura técnica, igual en
// todos los idiomas. Un «Linux» a secas no dice que no arranca en un ARM.
var NombreArquitectura = map[string]string{
	"amd64": "x86_64", "arm64": "ARM64", "universal": "Universal",
}

// PlataformaPropia es la de esta máquina, para poner SU descarga primero. En la web es
// una pista del user agent; aquí se sabe. iOS no tiene instalador: sin destacados.
func PlataformaPropia(goos string) string {
	if goos == "darwin" {
		return "macos"
	}
	if _, ok := NombrePlataforma[goos]; ok {
		return goos
	}
	return ""
}

var deFyne = regexp.MustCompile(`^sytue-([a-z0-9]+)-`)

// ProductoDeFichero es `productoDe` de la web: «sytue-calcus-linux-amd64.tar.xz» →
// «Sytue Calcus». La marca sale del catálogo, por clave o por nombre, y no de una
// tabla paralela: un producto nuevo aparece solo.
func ProductoDeFichero(nombre string) string {
	if m := deFyne.FindStringSubmatch(nombre); m != nil {
		app := m[1]
		// El bundle de la suite en escritorio (`sytue-one-…`): en el catálogo `one` es la
		// clave del Hub, que solo es una de las apps que lleva dentro.
		if app == "one" {
			return "Sytue One"
		}
		for p := iconos.One; p <= iconos.ERP; p++ {
			if p.Clave() == app || strings.ToLower(strings.ReplaceAll(p.Nombre(), " ", "")) == "sytue"+app {
				return p.Nombre()
			}
		}
		return "Sytue " + capital(app)
	}
	base, _, _ := strings.Cut(nombre, "-")
	if strings.HasPrefix(strings.ToLower(base), "sytue") {
		resto := base[len("sytue"):]
		if i := strings.IndexAny(resto, "._"); i >= 0 {
			resto = resto[:i]
		}
		if resto == "" {
			return "Sytue"
		}
		return "Sytue " + resto
	}
	// Los APK viejos van en minúsculas con guiones: `sytue-one-android.apk`.
	partes := strings.Split(strings.TrimSuffix(nombre, "-android.apk"), "-")
	for i, p := range partes {
		partes[i] = capital(p)
	}
	return strings.Join(partes, " ")
}

func capital(s string) string {
	for i, r := range s {
		return string(unicode.ToUpper(r)) + s[i+len(string(r)):]
	}
	return s
}

// MetaInstalador es la `ficha` de la página: «macOS · Universal · 84 MB». El tamaño,
// con el `formatBytes` de la suite y en el idioma de quien lee; lo que no se sabe, fuera.
func MetaInstalador(l i18n.Locale, i api.Instalador) string {
	partes := []string{NombrePlataforma[i.Plataforma]}
	if a := NombreArquitectura[i.Arquitectura]; a != "" {
		partes = append(partes, a)
	}
	if i.Bytes > 0 {
		partes = append(partes, i18n.FormatBytes(l, float64(i.Bytes)))
	}
	return strings.Join(partes, " · ")
}

// SirveEn es `sirveEn` del portal: si el instalador arranca en esa plataforma y ese
// procesador. Un universal sirve a los dos, y Windows en ARM emula x64. La web no sabe
// el procesador de un Mac ni de un Windows y lo deja vacío (no descarta nada); aquí se
// sabe siempre.
func SirveEn(i api.Instalador, plataforma, arq string) bool {
	if plataforma == "" || i.Plataforma != plataforma {
		return false
	}
	if arq == "" || i.Arquitectura == "" || i.Arquitectura == "universal" || i.Arquitectura == arq {
		return true
	}
	return plataforma == "windows" && arq == "arm64" && i.Arquitectura == "amd64"
}

// Repartir separa lo de esta máquina de lo demás, cada mitad por producto y, dentro,
// por plataforma, como la web: quien entra busca SU app, no su sistema. Destacado es lo
// que arranca aquí (`SirveEn`): un Linux ARM no ve destacado el de x86_64.
func Repartir(todos []api.Instalador, suya, arq string) (destacados, resto []api.Instalador) {
	ordenados := slices.Clone(todos)
	slices.SortStableFunc(ordenados, func(a, b api.Instalador) int {
		return cmp.Or(cmp.Compare(ProductoDeFichero(a.Fichero), ProductoDeFichero(b.Fichero)),
			cmp.Compare(a.Plataforma, b.Plataforma))
	})
	for _, i := range ordenados {
		if SirveEn(i, suya, arq) {
			destacados = append(destacados, i)
		} else {
			resto = append(resto, i)
		}
	}
	return destacados, resto
}

// Descargas es la pantalla `/descargas`.
type Descargas struct {
	pagina

	T        *i18n.Translator
	Estado   Estado
	Catalogo api.HubDescargas
	// Suya es la plataforma de esta máquina; vacía, nada destacado. Arq, su procesador
	// (`runtime.GOARCH`).
	Suya, Arq string
	// Bajando son los ficheros en curso: su enlace se queda inerte hasta que acaban.
	Bajando map[string]bool

	AlDescargar  func(api.Instalador)
	AlReintentar func()
}

var _ fyne.Widget = (*Descargas)(nil)

// NuevasDescargas crea la pantalla, cargando.
func NuevasDescargas(t *i18n.Translator) *Descargas {
	d := &Descargas{T: t, Estado: Cargando, Suya: PlataformaPropia(runtime.GOOS), Arq: runtime.GOARCH, Bajando: map[string]bool{}}
	d.ExtendBaseWidget(d)
	return d
}

// Resize recuerda el ancho: el alto depende de él.
func (d *Descargas) Resize(s fyne.Size) {
	if d.recordar(s) {
		d.BaseWidget.Resize(s)
		d.Refresh()
		return
	}
	d.BaseWidget.Resize(s)
}

// AccessibilityRole: la pantalla agrupa.
func (d *Descargas) AccessibilityRole() fyne.AccessibleRole { return fyne.AccessibleRoleContainer }

// CreateRenderer construye la pantalla.
func (d *Descargas) CreateRenderer() fyne.WidgetRenderer {
	r := &descargasRenderer{d: d, fondo: ui.NuevoFondoSuite(tema.De(fyne.CurrentApp().Settings().ThemeVariant()))}
	r.rehacer()
	return r
}

// tarjetaDescarga es una destacada: el producto, su línea gris y el enlace.
type tarjetaDescarga struct {
	caja         *ui.Caja
	nombre, meta *ui.Parrafo
	enlace       *ui.Enlace
}

// filaDescarga es una de la lista: el nombre recortado, la línea gris y el enlace.
type filaDescarga struct {
	nombre, meta *canvas.Text
	completo     string
	enlace       *ui.Enlace
}

type descargasRenderer struct {
	d *Descargas

	fondo                    *ui.FondoSuite
	titulo, entrada, version *ui.Parrafo
	aviso                    *ui.Vacio
	vacio                    *ui.Caja
	vacioTitulo, vacioTexto  *ui.Parrafo
	tituloDestacados         *ui.Parrafo
	tarjetas                 []tarjetaDescarga
	tituloResto              *ui.Parrafo
	lista                    *ui.Caja
	filas                    []filaDescarga
	divisores                []*canvas.Rectangle
	// enlaces sobreviven a `rehacer`, uno por fichero: rehacerlos al empezar una
	// descarga le quitaría el foco a quien la ha pedido con el teclado.
	enlaces map[string]*ui.Enlace

	objetos []fyne.CanvasObject
}

func (r *descargasRenderer) enlace(i api.Instalador) *ui.Enlace {
	d := r.d
	e, ok := r.enlaces[i.Fichero]
	if !ok {
		e = ui.NuevoEnlace(d.T.T("downloads.get", nil), tema.TextoRaiz, ui.Normal, "text", func() {
			if d.AlDescargar != nil {
				d.AlDescargar(i)
			}
		})
		e.Liso = true
		// El `<a download>` se anuncia con su rótulo; aquí, con el producto delante
		// para que la lista no sean diez «Descargar» iguales.
		e.Nombre = d.T.T("downloads.get", nil) + " — " + ProductoDeFichero(i.Fichero) + " (" + MetaInstalador(d.T.Locale(), i) + ")"
		r.enlaces[i.Fichero] = e
	}
	if e.Inerte != d.Bajando[i.Fichero] {
		e.Inerte = d.Bajando[i.Fichero]
		e.Refresh()
	}
	return e
}

// rehacer construye las piezas del estado actual. La página es corta —trece productos
// por cuatro sistemas como mucho—: se rehace entera, como las demás del Hub.
func (r *descargasRenderer) rehacer() {
	d, t := r.d, r.d.T
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	p := func(texto string, e tema.Escalon, peso ui.Peso, token string) *ui.Parrafo {
		return ui.NuevoParrafo(texto, e, peso, token, v)
	}
	enlaces := r.enlaces
	if enlaces == nil {
		enlaces = map[string]*ui.Enlace{}
	}
	*r = descargasRenderer{d: d, enlaces: enlaces, fondo: r.fondo}
	r.fondo.PonerVariante(v)
	// ponytail: `sytue-h1` lleva `letter-spacing: -0.01em`, como la cabecera del Hub
	// (ver `cabecera.colocar`): el mismo hueco, el mismo relevo.
	r.titulo = p(t.T("downloads.title", nil), ui.EscalonTitulo(1), ui.PesoTitulo(1), "text")
	r.entrada = p(t.T("downloads.lead", nil), tema.TextoRaiz, ui.Normal, "text-muted")
	if c := d.Catalogo; d.Estado == Listo && c.Version != "" {
		r.version = p(t.T("downloads.version", i18n.Vars{"version": c.Version}), tema.TextoXS, ui.Normal, "text-subtle")
	}
	r.objetos = []fyne.CanvasObject{r.fondo, r.titulo, r.entrada}
	if r.version != nil {
		r.objetos = append(r.objetos, r.version)
	}
	switch {
	case d.Estado == Cargando:
		return
	case d.Estado != Listo:
		r.aviso = Aviso(t, v, d.Estado, textosDescargas, d.AlReintentar)
		r.objetos = append(r.objetos, r.aviso)
		return
	case len(d.Catalogo.Instaladores) == 0:
		motivo := "downloads.emptyNoRelease"
		if d.Catalogo.Motivo == api.DescargasSinConfigurar {
			motivo = "downloads.emptyUnconfigured"
		}
		r.vacio = ui.NuevaCaja(tema.Color("bg-subtle", v), tema.Color("border", v), tema.RadioLG)
		r.vacioTitulo = p(t.T("downloads.empty", nil), tema.TextoRaiz, ui.Medio, "text")
		r.vacioTexto = p(t.T(motivo, nil), tema.TextoSM, ui.Normal, "text-muted")
		r.objetos = append(append(r.objetos, r.vacio.Objetos()...), r.vacioTitulo, r.vacioTexto)
		return
	}

	destacados, resto := Repartir(d.Catalogo.Instaladores, d.Suya, d.Arq)
	if len(destacados) > 0 {
		r.tituloDestacados = p(t.T("downloads.forYou", i18n.Vars{"platform": NombrePlataforma[d.Suya]}), ui.EscalonTitulo(3), ui.PesoTitulo(3), "text")
		r.objetos = append(r.objetos, r.tituloDestacados)
		for _, i := range destacados {
			c := tarjetaDescarga{
				caja:   ui.NuevaCaja(tema.Color("bg", v), tema.Color("border", v), tema.RadioLG),
				nombre: p(ProductoDeFichero(i.Fichero), tema.TextoRaiz, ui.Medio, "text"),
				meta:   p(MetaInstalador(d.T.Locale(), i), tema.TextoXS, ui.Normal, "text-subtle"),
				enlace: r.enlace(i),
			}
			r.tarjetas = append(r.tarjetas, c)
			r.objetos = append(append(r.objetos, c.caja.Objetos()...), c.nombre, c.meta, c.enlace)
		}
	}
	if len(resto) > 0 {
		titulo := "downloads.all"
		if len(destacados) > 0 {
			titulo = "downloads.others"
		}
		r.tituloResto = p(t.T(titulo, nil), ui.EscalonTitulo(3), ui.PesoTitulo(3), "text")
		// El `ul` no tiene fondo: el de la página, para que la caja no enseñe su borde
		// por dentro.
		r.lista = ui.NuevaCaja(tema.Color("bg", v), tema.Color("border", v), tema.RadioLG)
		r.divisores = Divisores(v, len(resto))
		r.objetos = append(append(r.objetos, r.tituloResto), r.lista.Objetos()...)
		for _, dv := range r.divisores {
			r.objetos = append(r.objetos, dv)
		}
		for _, i := range resto {
			nombre := ProductoDeFichero(i.Fichero)
			f := filaDescarga{
				nombre:   ui.Texto(nombre, tema.TextoRaiz, ui.Medio, tema.Color("text", v)),
				meta:     ui.Texto(MetaInstalador(d.T.Locale(), i), tema.TextoXS, ui.Normal, tema.Color("text-subtle", v)),
				completo: nombre,
				enlace:   r.enlace(i),
			}
			r.filas = append(r.filas, f)
			r.objetos = append(r.objetos, f.nombre, f.meta, f.enlace)
		}
	}
}

// textosDescargas son los dos estados propios de la página en nativo: la web no tiene
// estado de error porque la lista la pinta su propio servidor.
var textosDescargas = Textos{Fallo: "downloads.failedTitle", FalloCuerpo: "downloads.failedBody"}

func (r *descargasRenderer) MinSize() fyne.Size           { return fyne.NewSize(0, r.alto(r.d.Ancho())) }
func (r *descargasRenderer) Objects() []fyne.CanvasObject { return r.objetos }
func (r *descargasRenderer) Destroy()                     {}

// Refresh rehace la pantalla (ver el comentario del lanzador).
func (r *descargasRenderer) Refresh() {
	r.rehacer()
	r.Layout(r.d.Size())
	canvas.Refresh(r.d)
}

func (r *descargasRenderer) alto(ventana float32) float32 { return r.colocar(ventana, false) }

func (r *descargasRenderer) Layout(t fyne.Size) {
	if t.Width <= 0 {
		return
	}
	r.fondo.Resize(fyne.NewSize(t.Width, max(t.Height, r.alto(t.Width))))
	r.colocar(t.Width, true)
}

// CajaDescargas es la `section`: `max-w-5xl` con `px-4` a cualquier ancho.
func CajaDescargas(ventana float32) (x, ancho float32) {
	caja := min(ventana, AnchoAjustes)
	return (ventana-caja)/2 + MargenAjustesX, max(caja-2*MargenAjustesX, 0)
}

// ColumnasDescargas es el `sm:grid-cols-2 lg:grid-cols-3` de las destacadas, que mide
// la VENTANA.
func ColumnasDescargas(ventana float32) int {
	switch {
	case ventana >= ui.CorteLG:
		return 3
	case ventana >= ui.CorteSM:
		return 2
	}
	return 1
}

// colocar recorre la pantalla de arriba abajo; con `mover` a false solo mide.
func (r *descargasRenderer) colocar(ventana float32, mover bool) float32 {
	x, ancho := CajaDescargas(ventana)
	y := ponerParrafo(r.titulo, x, MargenY, ancho, mover) + HuecoCabeceraDescargas
	y = ponerParrafo(r.entrada, x, y, ancho, mover)
	if r.version != nil {
		y = ponerParrafo(r.version, x, y+HuecoCabeceraDescargas, ancho, mover)
	}
	switch {
	case r.aviso != nil:
		y += MargenAjustesY
		y = ponerObjeto(r.aviso, x, y, fyne.NewSize(ancho, r.aviso.MedidaVacio(ancho)), mover)
	case r.vacio != nil:
		y = r.colocarVacio(x, y+MargenAjustesY, ancho, mover)
	}
	if r.tituloDestacados != nil {
		y = ponerParrafo(r.tituloDestacados, x, y+MargenAjustesY, ancho, mover) + HuecoGrupoDescargas
		y = r.colocarTarjetas(x, y, ancho, ColumnasDescargas(ventana), mover)
	}
	if r.tituloResto != nil {
		y = ponerParrafo(r.tituloResto, x, y+MargenAjustesY, ancho, mover) + HuecoGrupoDescargas
		y = r.colocarLista(x, y, ancho, mover)
	}
	return y + MargenY
}

func (r *descargasRenderer) colocarVacio(x, y, ancho float32, mover bool) float32 {
	dentro := max(ancho-2*(tema.Borde+RellenoVacioDescargas), 0)
	alto := 2*(tema.Borde+RellenoVacioDescargas) + ui.AltoParrafo(r.vacioTitulo, dentro) +
		MargenCuerpoVacio + ui.AltoParrafo(r.vacioTexto, dentro)
	if mover {
		ui.MoverCaja(r.vacio, fyne.NewPos(x, y), fyne.NewSize(ancho, alto))
		cx, cy := x+tema.Borde+RellenoVacioDescargas, y+tema.Borde+RellenoVacioDescargas
		cy = ponerParrafo(r.vacioTitulo, cx, cy, dentro, true) + MargenCuerpoVacio
		ponerParrafo(r.vacioTexto, cx, cy, dentro, true)
	}
	return y + alto
}

// altoTarjeta es su contenido con el borde y el `p-4`.
func altoTarjeta(c tarjetaDescarga, dentro float32) float32 {
	return 2*(tema.Borde+RellenoTarjetaDescarga) + ui.AltoParrafo(c.nombre, dentro) +
		ui.AltoParrafo(c.meta, dentro) + MargenEnlaceTarjeta + ui.MedidaEnlace(c.enlace).Height
}

// colocarTarjetas pone la rejilla de destacadas. Las de una misma fila se estiran al
// alto de la mayor, que es lo que hace `grid` con su `align-items: stretch`.
func (r *descargasRenderer) colocarTarjetas(x, y, ancho float32, columnas int, mover bool) float32 {
	n := float32(columnas)
	w := (ancho - (n-1)*HuecoGrupoDescargas) / n
	dentro := max(w-2*(tema.Borde+RellenoTarjetaDescarga), 0)
	for inicio := 0; inicio < len(r.tarjetas); inicio += columnas {
		fila := r.tarjetas[inicio:min(inicio+columnas, len(r.tarjetas))]
		var alto float32
		for _, c := range fila {
			alto = max(alto, altoTarjeta(c, dentro))
		}
		if inicio > 0 {
			y += HuecoGrupoDescargas
		}
		for i, c := range fila {
			if !mover {
				continue
			}
			cx := x + float32(i)*(w+HuecoGrupoDescargas)
			ui.MoverCaja(c.caja, fyne.NewPos(cx, y), fyne.NewSize(w, alto))
			ix, iy := cx+tema.Borde+RellenoTarjetaDescarga, y+tema.Borde+RellenoTarjetaDescarga
			iy = ponerParrafo(c.nombre, ix, iy, dentro, true)
			iy = ponerParrafo(c.meta, ix, iy, dentro, true) + MargenEnlaceTarjeta
			ponerObjeto(c.enlace, ix, iy, ui.MedidaEnlace(c.enlace), true)
		}
		y += alto
	}
	return y
}

// altoFilaDescarga es el `py-3` y dos cajas de línea de texto raíz: la del nombre, que
// es `block`, y la de la línea gris, que es un `span` suelto y por eso ocupa la caja de
// línea de su PADRE (19,25) y no la suya (14).
var altoFilaDescarga = 2*RellenoFilaDescargaY + 2*tema.TextoRaiz.Linea

// bajadaDeBase es cuánto baja un texto pequeño en línea para compartir la línea base
// con la caja de línea de su padre. Con Figtree (ascendente 0,95 em, descendente
// 0,25 em) la base cae a `(L − 1,2·S)/2 + 0,95·S` de lo alto de cada caja; la resta
// entre las dos es esto. A 14/19,25 con 10,5/14 salen 3,85.
func bajadaDeBase(padre, hijo tema.Escalon) float32 {
	return (padre.Linea-hijo.Linea)/2 + 0.35*(padre.Tam-hijo.Tam)
}

func (r *descargasRenderer) colocarLista(x, y, ancho float32, mover bool) float32 {
	alto := 2*tema.Borde + float32(len(r.filas))*altoFilaDescarga + float32(len(r.divisores))*tema.Borde
	if !mover {
		return y + alto
	}
	ui.MoverCaja(r.lista, fyne.NewPos(x, y), fyne.NewSize(ancho, alto))
	fx, fy := x+tema.Borde, y+tema.Borde
	dentro := ancho - 2*tema.Borde - 2*RellenoFilaDescargaX
	for i, f := range r.filas {
		if i > 0 {
			ColocarDivisor(r.divisores[i-1], fx, fy, ancho-2*tema.Borde)
			fy += tema.Borde
		}
		m := ui.MedidaEnlace(f.enlace)
		// `min-w-0` a la izquierda y `shrink-0` a la derecha: el nombre se recorta.
		cabe := max(dentro-m.Width-HuecoFilaDescarga, 0)
		cx, cy := fx+RellenoFilaDescargaX, fy+RellenoFilaDescargaY
		ui.Truncar(f.nombre, f.completo, tema.TextoRaiz, ui.Medio, cabe)
		ui.ColocarTexto(f.nombre, tema.TextoRaiz, cx, cy)
		ui.ColocarTexto(f.meta, tema.TextoXS, cx, cy+tema.TextoRaiz.Linea+bajadaDeBase(tema.TextoRaiz, tema.TextoXS))
		// `items-center`: el enlace, centrado en la fila.
		ponerObjeto(f.enlace, fx+RellenoFilaDescargaX+dentro-m.Width, fy+(altoFilaDescarga-m.Height)/2, m, true)
		fy += altoFilaDescarga
	}
	return y + alto
}
