// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package contactos

import (
	"image"
	"image/color"
	"image/png"
	"os"
	"testing"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/test"

	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/i18n"

	"git.sytue.eu/sytue/one/apps/fyne/agenda"
	"git.sytue.eu/sytue/one/apps/fyne/catalogos"
	"git.sytue.eu/sytue/one/apps/fyne/iconos"
	"git.sytue.eu/sytue/one/apps/fyne/tema"
	"git.sytue.eu/sytue/one/apps/fyne/ui"
)

// Guardianes de Sytue Contacts: clases de ContactsApp.tsx y AppShell.tsx con la
// raíz de 16 px de theme.css, a 1280×800.
func TestMain(m *testing.M) {
	test.NewApp().Settings().SetTheme(tema.Sytue{})
	tema.FormaDeLosGuardianes()
	os.Exit(m.Run())
}

// Lo medido, con su origen. Cambiar uno es decir que la web ha cambiado.
const (
	ventana    = 1280
	altoUtil   = 744
	anchoMenu  = 260 // el carril
	anchoPanel = 320 // `md:w-80`; el `<section>` de dentro mide 319
	anchoFicha = 700 // lo que sobra: 1280 - 260 - 320
	altoFila   = 56  // text-base/text-sm (22 + 18) y py-2 (8 + 8)
)

func traductor(t *testing.T) *i18n.Translator {
	t.Helper()
	tr, err := i18n.New("es", i18n.Common(), catalogos.Contacts())
	if err != nil {
		t.Fatalf("no se pudo cargar el catálogo: %v", err)
	}
	return tr
}

// TestLaFilaDeContactoMide56: ContactsApp.tsx usa text-base/text-sm (22 + 18 px
// de caja de línea) y py-2 (8 + 8). Sin subtítulo manda el avatar sm de 32 px:
// 32 + 8 + 8 = 48.
func TestLaFilaDeContactoMide56(t *testing.T) {
	if got := MedidaFilaContacto(true); got != altoFila {
		t.Errorf("la fila mide %v y producción dice %v", got, altoFila)
	}
	if got := MedidaFilaContacto(false); got != 48 {
		t.Errorf("sin subtítulo la fila mide %v y manda el avatar: 32 + 2 × 8 = 48", got)
	}
}

// TestElBloqueDelBuscadorMideLoDeLaWeb: ContactsApp.tsx usa p-3 (12 px por lado),
// Input md h-9 (36 px) y border-b (1 px): 61 px. Con recuento suma mt-3 (12)
// y la casilla h-4/text-xs (16 px de caja de línea): 89 px.
func TestElBloqueDelBuscadorMideLoDeLaWeb(t *testing.T) {
	if got := AltoBloqueBuscador(true); got != 89 {
		t.Errorf("con recuento el bloque mide %v y la web 89", got)
	}
	if got := AltoBloqueBuscador(false); got != 61 {
		t.Errorf("sin recuento el bloque mide %v y la web 61", got)
	}
}

// TestElArmazonRepiteLosTresAnchosDeContacts: AppShell.tsx fija el rail de 260 px
// y md:w-80 (320 px con raíz de 16): 260 + 320 + 700 = 1280.
func TestElArmazonRepiteLosTresAnchosDeContacts(t *testing.T) {
	if got := ui.AnchoMenu(false); got != anchoMenu {
		t.Errorf("el menú mide %v y producción dice %v", got, anchoMenu)
	}
	if got := ui.AnchoPanel(ventana); got != anchoPanel {
		t.Errorf("el panel mide %v y producción dice %v", got, anchoPanel)
	}
	if got := float32(ventana) - ui.AnchoMenu(false) - ui.AnchoPanel(ventana); got != anchoFicha {
		t.Errorf("a la ficha le quedan %v y producción dice %v", got, anchoFicha)
	}
}

// TestLaPantallaSePintaConSusTresColumnas pinta Contacts entero y comprueba en los
// PÍXELES que cada columna empieza donde dice la medida.
func TestLaPantallaSePintaConSusTresColumnas(t *testing.T) {
	img := pintar(t, pantallaDeMuestra(t))

	if got, quiero := px(img, 100, 400), tema.Color("sidebar-bg", tema.Claro); !mismoColor(got, quiero) {
		t.Errorf("dentro del menú (100,400) se pinta %v y `sidebar-bg` es %v", got, quiero)
	}
	// El panel de lista, por debajo de sus seis filas de 56 px y del buscador,
	// donde solo queda la superficie.
	if got, quiero := px(img, anchoMenu+40, 650), tema.Color("surface", tema.Claro); !mismoColor(got, quiero) {
		t.Errorf("en el panel de lista (%d,650) se pinta %v y debería ser %v", anchoMenu+40, got, quiero)
	}
	// Y la ficha, que en esta escena está vacía («Selecciona un contacto») sobre `bg`.
	if got, quiero := px(img, anchoMenu+anchoPanel+300, 700), tema.Color("bg", tema.Claro); !mismoColor(got, quiero) {
		t.Errorf("en la ficha se pinta %v y el fondo debería ser %v", got, quiero)
	}
	guardar(t, img, "contactos-lista")
}

// TestLosSeisEstadosSePintan: los seis del encargo, y ninguno en blanco.
func TestLosSeisEstadosSePintan(t *testing.T) {
	for _, c := range []struct {
		nombre string
		e      Estado
	}{
		{"carga", agenda.Cargando},
		{"vacio", agenda.Vacia},
		{"error", agenda.Error},
		{"sin-conexion", agenda.SinConexion},
		{"sin-permiso", agenda.SinPermiso},
		{"exito", agenda.Exito},
	} {
		t.Run(c.nombre, func(t *testing.T) {
			p := pantallaDeMuestra(t)
			p.Estado = c.e
			p.SinConexion = c.e == agenda.SinConexion
			if c.e == agenda.Vacia {
				p.Contactos = nil
			}
			img := pintar(t, p)
			if !hayTinta(img, anchoMenu+10, 120, anchoMenu+anchoPanel-10, altoUtil-20) {
				t.Errorf("el estado %q deja el panel de lista en blanco", c.nombre)
			}
		})
	}
}

// TestElVacioExplicaElFiltroQueLoDejoVacio es el guardián de un fallo real de la web:
// en «Destacados» sin destacados se leía «tu libreta está vacía» —con la libreta
// llena— y el único botón creaba un contacto que tampoco iba a salir ahí. Cada filtro
// explica LO SUYO y ofrece SU salida.
func TestElVacioExplicaElFiltroQueLoDejoVacio(t *testing.T) {
	base := pantallaDeMuestra(t)
	base.Contactos = nil
	tr := traductor(t)

	casos := []struct {
		nombre   string
		preparar func(*Pantalla)
		clave    string
	}{
		{"búsqueda", func(p *Pantalla) { p.Busqueda = "zzz" }, "list.emptySearch"},
		{"etiqueta", func(p *Pantalla) { p.EtiquetaActiva = "g1" }, "list.emptyLabel"},
		{"destacados", func(p *Pantalla) { p.SoloDestacados = true }, "list.emptyStarred"},
		{"libreta", func(*Pantalla) {}, "list.emptyBook"},
	}
	vistos := map[string]bool{}
	for _, c := range casos {
		t.Run(c.nombre, func(t *testing.T) {
			p := pantallaDeMuestra(t)
			p.Contactos = nil
			c.preparar(p)
			got := p.textosDeVacio()
			quiero := tr.T(c.clave, i18n.Vars{"label": "Amigos"})
			if got.Descripcion != quiero {
				t.Errorf("con el filtro %q se lee %q y la web dice %q", c.nombre, got.Descripcion, quiero)
			}
			if vistos[got.Descripcion] {
				t.Errorf("el filtro %q repite la copia de otro: el vacío no explica cuál es", c.nombre)
			}
			vistos[got.Descripcion] = true
			if got.RotuloAccion == "" {
				t.Errorf("el vacío de %q no ofrece salida", c.nombre)
			}
		})
	}
}

// TestElRecuentoCuentaElFiltroDeDestacados es el otro fallo medido de la web: el
// filtro de destacados TAMBIÉN es un filtro, y sin contarlo la vista decía «1.234
// contactos» mientras enseñaba cinco.
func TestElRecuentoCuentaElFiltroDeDestacados(t *testing.T) {
	p := pantallaDeMuestra(t)
	p.Total = 1234
	sinFiltro := p.recuento()
	p.SoloDestacados = true
	conFiltro := p.recuento()
	if sinFiltro == conFiltro {
		t.Errorf("con el filtro de destacados el recuento no cambia: %q", conFiltro)
	}
}

// TestLosGlifosPropiosNoEstanEnElCatalogoCompartido: la estrella y la tarta viven en
// esta app porque en la web tampoco están en `Icon.tsx`. Si alguien las mete en el
// catálogo compartido, su guardián se pondrá rojo; este dice por qué están aquí.
func TestLosGlifosPropiosNoEstanEnElCatalogoCompartido(t *testing.T) {
	for _, d := range []string{EstrellaD, TartaD, EtiquetaD, PapeleraD} {
		if d == "" {
			t.Fatal("un glifo de la app está vacío")
		}
		// Se construyen de verdad: una ruta que `oksvg` no entienda entra en pánico
		// al ARMAR el dibujo, no la primera vez que se abre la pantalla.
		if got := iconos.TrazoDeRuta(d, 18, color.NRGBA{A: 255}); got == nil {
			t.Fatalf("no se pudo armar el glifo %q", d[:12])
		}
	}
}

// TestLaEstrellaRellenaYLaVaciaNoSonElMismoDibujo: el estado de «destacado» se
// distingue por el RELLENO, y es lo único que lo distingue cuando el color del glifo
// no llega (ver el `ponytail:` de la ficha).
func TestLaEstrellaRellenaYLaVaciaNoSonElMismoDibujo(t *testing.T) {
	tinta := color.NRGBA{R: 0x7f, G: 0x5a, B: 0x0f, A: 255}
	llena := tintaDe(t, iconos.TrazoDeRutaRelleno(EstrellaD, 24, tinta), tinta)
	vacia := tintaDe(t, iconos.TrazoDeRuta(EstrellaD, 24, tinta), tinta)
	if llena <= vacia {
		t.Errorf("la estrella rellena cubre %d píxeles y la vacía %d: tiene que cubrir más", llena, vacia)
	}
}

// TestLosTextosSalenDelCatalogo: una clave que no exista sale CRUDA en pantalla,
// porque `Translator.T` devuelve la propia clave.
func TestLosTextosSalenDelCatalogo(t *testing.T) {
	tr := traductor(t)
	for _, clave := range []string{
		"nav.newContact", "nav.views", "nav.starred", "nav.birthdays", "nav.trash",
		"nav.addressBooks", "nav.newAddressBook", "nav.booksEmpty", "nav.booksError",
		"nav.labels", "nav.labelsEmpty", "nav.newLabel",
		"list.title", "list.search", "list.count", "list.countFiltered",
		"list.emptyTitle", "list.emptySearch", "list.emptyLabel", "list.emptyStarred",
		"list.emptyBook", "list.clearSearch", "list.clearFilter",
		"list.errorTitle", "list.errorDescription",
		"detail.emptyTitle", "detail.emptyDescription", "detail.infoTitle",
		"detail.sendEmail", "detail.moreActions", "detail.star", "detail.unstar",
		"field.email", "field.phone", "field.organization", "field.address",
		"field.birthday", "field.note", "common.edit", "common.retry", "io.button",
		"list.countWithLabel", "list.select", "list.selectAll", "nav.labelsError",
		"detail.starFailed", "common.tryAgainSoon",
	} {
		if got := tr.T(clave, i18n.Vars{"count": 1, "shown": 1, "label": "x", "name": "x", "summary": "x"}); got == clave {
			t.Errorf("la clave %q no está en el catálogo de la app: saldría cruda en pantalla", clave)
		}
	}
}

// ---- utilidades ----

// pantallaDeMuestra es la escena de la captura de producción: seis contactos en la
// libreta «Contactos», sin ninguno elegido.
func pantallaDeMuestra(t *testing.T) *Pantalla {
	t.Helper()
	libreta := api.AddressBook{ID: "l1", DisplayName: "Contactos", IsDefault: true}
	persona := func(nombre, correo string) api.Contact {
		return api.Contact{
			ID: correo, UID: correo, FormattedName: nombre,
			Emails: []api.ContactEmail{{Email: correo}},
		}
	}
	return &Pantalla{
		T:         traductor(t),
		Libretas:  []api.AddressBook{libreta, {ID: "l2", DisplayName: "Organización"}},
		Activa:    &libreta,
		Etiquetas: []api.ContactGroup{{ID: "g1", Name: "Amigos"}},
		Contactos: []api.Contact{
			persona("Carla Benet Ros", "carla.benet@ejemplo.test"),
			persona("Héctor Lamas Feijóo", "hector.lamas@ejemplo.test"),
			persona("Iñaki Aranburu Lete", "inaki.aranburu@ejemplo.test"),
			persona("Marta Vidal Soto", "marta.vidal@ejemplo.test"),
			persona("Ousmane Diallo Bâ", "ousmane.diallo@ejemplo.test"),
			persona("Sofía Nogueira Paz", "sofia.nogueira@ejemplo.test"),
		},
		Total:      6,
		Destacados: map[string]bool{},
	}
}

func pintar(t *testing.T, p *Pantalla) image.Image {
	t.Helper()
	w := test.NewWindow(p.Construir())
	t.Cleanup(w.Close)
	w.SetPadded(false)
	w.Resize(fyne.NewSize(ventana, altoUtil))
	return w.Canvas().Capture()
}

func px(img image.Image, x, y int) color.NRGBA {
	r, g, b, a := img.At(x, y).RGBA()
	return color.NRGBA{R: uint8(r >> 8), G: uint8(g >> 8), B: uint8(b >> 8), A: uint8(a >> 8)}
}

func mismoColor(a, b color.NRGBA) bool {
	d := func(x, y uint8) bool { return int(x)-int(y) > 1 || int(y)-int(x) > 1 }
	return !d(a.R, b.R) && !d(a.G, b.G) && !d(a.B, b.B)
}

func hayTinta(img image.Image, x0, y0, x1, y1 int) bool {
	fondo := px(img, x0, y0)
	for y := y0; y < y1; y += 3 {
		for x := x0; x < x1; x += 3 {
			if !mismoColor(px(img, x, y), fondo) {
				return true
			}
		}
	}
	return false
}

// tintaDe cuenta los píxeles de un dibujo que están TEÑIDOS con su color, que es como
// se distingue un glifo relleno de uno que solo lleva trazo.
//
// Se normaliza por la distancia entre el fondo y la TINTA, no por 255: con un color
// oscuro sobre blanco un píxel lleno no recorre el canal entero, y dividir por 255
// haría parecer que falta trazo. Es la trampa que ya está escrita en el roadmap.
func tintaDe(t *testing.T, d *iconos.Dibujo, tinta color.NRGBA) int {
	t.Helper()
	w := test.NewWindow(d.Objeto())
	t.Cleanup(w.Close)
	w.SetPadded(false)
	w.Resize(fyne.NewSize(24, 24))
	img := w.Canvas().Capture()
	fondo := px(img, 0, 0)
	rango := float64(int(fondo.R)-int(tinta.R)) + float64(int(fondo.G)-int(tinta.G)) +
		float64(int(fondo.B)-int(tinta.B))
	if rango == 0 {
		t.Fatal("el fondo y la tinta son el mismo color: no se puede medir cobertura")
	}
	n := 0
	for y := range 24 {
		for x := range 24 {
			c := px(img, x, y)
			recorrido := float64(int(fondo.R)-int(c.R)) + float64(int(fondo.G)-int(c.G)) +
				float64(int(fondo.B)-int(c.B))
			if recorrido/rango > 0.5 { // medio píxel de tinta cuenta como pintado
				n++
			}
		}
	}
	return n
}

func guardar(t *testing.T, img image.Image, nombre string) {
	t.Helper()
	dir := os.Getenv("SYTUE_CAPTURAS")
	if dir == "" {
		return
	}
	f, err := os.Create(dir + "/" + nombre + ".png")
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	if err := png.Encode(f, img); err != nil {
		t.Fatal(err)
	}
	t.Logf("captura en %s/%s.png", dir, nombre)
}
