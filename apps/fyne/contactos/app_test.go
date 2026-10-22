// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package contactos

import (
	"encoding/json"
	"image"
	"image/color"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/test"

	"git.sytue.eu/sytue/one/apps/nucleo/api"

	"git.sytue.eu/sytue/one/apps/fyne/agenda"
	"git.sytue.eu/sytue/one/apps/fyne/tema"
	"git.sytue.eu/sytue/one/apps/fyne/ui"
)

// servicioFalso es Sytue Contacts por HTTP, con las rutas que usa la app. Se prueba el
// SDK de verdad contra él: un doble del cliente probaría el doble.
type servicioFalso struct {
	mu        sync.Mutex
	contactos []api.Contact
	favoritos []string
	fallarFav bool
	// fallarGrupo hace que el PUT de una etiqueta conteste 500.
	fallarGrupo bool
	// escrituras son las peticiones que cambian algo, «MÉTODO ruta», con su cuerpo.
	escrituras []escritura
	// papelera es lo que devuelve GET /trash.
	papelera []api.TrashItem
	// fotos son los bytes de la foto de cada UID; sin entrada, 404. fotosVistas son
	// los UID por los que se ha preguntado, en orden.
	fotos       map[string][]byte
	fotosVistas []string
	// directorio es la agenda de Sytue ID (GET /api/v1/directory); con ella, la app
	// sincroniza «Organización». organizacion son las fichas de esa libreta (l2), y
	// sinOrganizacion la quita de las libretas.
	directorio      []api.Miembro
	organizacion    []api.Contact
	sinOrganizacion bool
	// ia es lo que contesta Sytue AI (POST /api/v1/assist); con ella, el formulario
	// lleva el asistente.
	ia string
	// cola es el trabajo que las filas lanzan al pintarse (enFondoFila), para correrlo
	// fuera del relleno de la lista.
	cola []func()
}

// correrCola corre lo que las filas han pedido, y lo que eso pida a su vez.
func (s *servicioFalso) correrCola() {
	for len(s.cola) > 0 {
		f := s.cola[0]
		s.cola = s.cola[1:]
		f()
	}
}

type escritura struct{ ruta, cuerpo string }

func (s *servicioFalso) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	defer s.mu.Unlock()
	responder := func(v any) { _ = json.NewEncoder(w).Encode(v) }
	ruta := r.Method + " " + r.URL.Path
	if r.Method != http.MethodGet {
		cuerpo, _ := io.ReadAll(r.Body)
		s.escrituras = append(s.escrituras, escritura{ruta, string(cuerpo)})
	}
	switch {
	case ruta == "GET /api/v1/contacts/photo":
		s.fotosVistas = append(s.fotosVistas, r.URL.Query().Get("uid"))
		if b, hay := s.fotos[r.URL.Query().Get("uid")]; hay {
			_, _ = w.Write(b)
			return
		}
		w.WriteHeader(http.StatusNotFound)
		return
	case r.Method == http.MethodGet && strings.HasSuffix(r.URL.Path, "/vcard"):
		id := strings.TrimSuffix(strings.TrimPrefix(r.URL.Path, "/api/v1/contacts/"), "/vcard")
		_, _ = io.WriteString(w, "BEGIN:VCARD\r\nVERSION:4.0\r\nFN:"+id+"\r\nEND:VCARD\r\n")
		return
	case r.Method == http.MethodGet && strings.HasPrefix(r.URL.Path, "/api/v1/contacts/"):
		id := strings.TrimPrefix(r.URL.Path, "/api/v1/contacts/")
		for _, c := range s.contactos {
			if c.ID == id {
				c.AddressBookID = "l1"
				responder(c)
				return
			}
		}
		w.WriteHeader(http.StatusNotFound)
		return
	case ruta == "GET /api/v1/export-csv":
		w.Header().Set("Content-Disposition", `attachment; filename="contactos.csv"`)
		_, _ = io.WriteString(w, "Name\r\nAna\r\n")
		return
	case ruta == "POST /api/v1/address-books/l1/import-csv":
		responder(map[string]int{"imported": 2, "skipped": 1})
		return
	case ruta == "POST /api/v1/address-books/l1/contacts":
		responder(api.Contact{ID: "nuevo", UID: "u-nuevo", FormattedName: "Nuevo"})
		return
	case strings.HasPrefix(ruta, "PUT /api/v1/contacts/"):
		responder(api.Contact{ID: strings.TrimPrefix(r.URL.Path, "/api/v1/contacts/")})
		return
	case strings.HasPrefix(ruta, "DELETE /api/v1/contacts/"):
		w.WriteHeader(http.StatusNoContent)
		return
	}
	switch ruta {
	case "GET /api/v1/address-books":
		libretas := []api.AddressBook{
			{ID: "l2", Name: "organizacion", DisplayName: "Organización"},
			{ID: "l1", Name: "contacts", DisplayName: "Contactos", IsDefault: true},
		}
		if s.sinOrganizacion {
			libretas = libretas[1:]
		}
		responder(map[string]any{"address_books": libretas})
	case "POST /api/v1/address-books":
		responder(api.AddressBook{ID: "l2", Name: "organizacion"})
	case "GET /api/v1/address-books/l2/contacts":
		responder(map[string]any{"contacts": s.organizacion})
	case "GET /api/v1/directory":
		responder(map[string]any{"items": s.directorio})
	case "POST /api/v1/assist":
		responder(map[string]string{"text": s.ia})
	case "GET /api/v1/address-books/l1/contacts":
		responder(map[string]any{"contacts": s.contactos})
	case "GET /api/v1/address-books/l1/groups":
		responder(map[string]any{"groups": []api.ContactGroup{{ID: "g1", Name: "Amigos", Members: []string{"u-bruno"}}}})
	case "GET /api/v1/trash":
		responder(map[string]any{"items": s.papelera})
	case "GET /api/v1/favorites":
		responder(map[string]any{"uids": s.favoritos})
	case "POST /api/v1/favorites":
		if s.fallarFav {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		responder(map[string]any{"ok": true})
	case "PUT /api/v1/groups/g1":
		if s.fallarGrupo {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		responder(api.ContactGroup{ID: "g1", Name: "Amigos"})
	default:
		switch r.Method {
		case http.MethodGet:
			w.WriteHeader(http.StatusNotFound)
		case http.MethodDelete:
			w.WriteHeader(http.StatusNoContent)
		default:
			responder(map[string]any{})
		}
	}
}

func persona(id, nombre string) api.Contact {
	return api.Contact{ID: id, UID: "u-" + id, FormattedName: nombre}
}

// appDePrueba monta la app contra el servicio falso, con el trabajo de fondo en línea:
// así cada paso termina antes de que el test mire.
func appDePrueba(t *testing.T, s *servicioFalso) *App {
	t.Helper()
	srv := httptest.NewServer(s)
	t.Cleanup(srv.Close)
	a := NuevaApp(traductor(t), api.NewContacts(srv.URL, nil, nil))
	a.enFondo = func(f func()) { f() }
	a.enFondoFila = func(f func()) { s.cola = append(s.cola, f) }
	if s.ia != "" {
		a.IA = api.NewIA(srv.URL, nil, nil)
	}
	if s.directorio != nil {
		a.Directorio, a.Yo = api.NewDirectorio(srv.URL, nil, nil), func() string { return "yo" }
	}
	w := test.NewWindow(a.Objeto())
	t.Cleanup(w.Close)
	w.SetPadded(false)
	w.Resize(fyne.NewSize(ventana, altoUtil))
	return a
}

func nombres(cs []api.Contact) []string {
	v := make([]string, 0, len(cs))
	for _, c := range cs {
		v = append(v, c.DisplayName())
	}
	return v
}

// TestLaAppCargaYOrdenaComoElIdioma: entra, abre la libreta PREDETERMINADA aunque no
// venga la primera, y ordena como `localeCompare` en español. Por bytes, «Ángela»,
// «Ñandú» y «Óscar» se irían al final; la «ñ» va detrás de la «n».
func TestLaAppCargaYOrdenaComoElIdioma(t *testing.T) {
	s := &servicioFalso{
		contactos: []api.Contact{
			persona("oscar", "Óscar"), persona("nandu", "Ñandú"), persona("nuria", "Nuria"),
			persona("angela", "Ángela"), persona("bruno", "Bruno"), persona("ana", "Ana"),
		},
		favoritos: []string{"urn:uuid:u-nuria"},
	}
	a := appDePrueba(t, s)
	a.Cargar()
	p := a.Pantalla

	if p.Estado != agenda.Exito || p.CargandoLibretas || p.ErrorLibretas {
		t.Fatalf("tras cargar: estado %v, cargando %v, error %v", p.Estado, p.CargandoLibretas, p.ErrorLibretas)
	}
	if p.Activa == nil || p.Activa.ID != "l1" {
		t.Fatalf("se abrió %+v y la predeterminada es l1", p.Activa)
	}
	quiero := []string{"Ana", "Ángela", "Bruno", "Nuria", "Ñandú", "Óscar"}
	if got := nombres(p.Contactos); !iguales(got, quiero) {
		t.Errorf("orden %v, y en español es %v", got, quiero)
	}
	if p.Total != 6 || len(p.Etiquetas) != 1 {
		t.Errorf("total %d y %d etiquetas", p.Total, len(p.Etiquetas))
	}
	// El favorito llega con prefijo `urn:uuid:` y se cruza igual.
	if !p.Destacados["u-nuria"] {
		t.Errorf("Nuria está destacada y la app no lo sabe: %v", p.Destacados)
	}
}

// TestLaBusquedaMiraElTelefonoPorDigitos: «600 11» encuentra «+34 600-11-22-33»,
// pero dos dígitos no bastan, que si no «1» devolvería media libreta.
func TestLaBusquedaMiraElTelefonoPorDigitos(t *testing.T) {
	marta := persona("marta", "Marta")
	marta.Phones = []api.ContactPhone{{Phone: "+34 600-11-22-33"}}
	todos := []api.Contact{marta, persona("luis", "Luis")}

	if got := filtrar(todos, "600 11", nil, false, false, nil); len(got) != 1 || got[0].ID != "marta" {
		t.Errorf("«600 11» encuentra %v", nombres(got))
	}
	if got := filtrar(todos, "60", nil, false, false, nil); len(got) != 0 {
		t.Errorf("con dos dígitos no se busca por teléfono, y salen %v", nombres(got))
	}
	// Etiqueta y destacados se combinan con la búsqueda, no la sustituyen.
	if got := filtrar(todos, "", []string{"u-luis"}, true, false, nil); len(got) != 1 || got[0].ID != "luis" {
		t.Errorf("la etiqueta filtra %v", nombres(got))
	}
	if got := filtrar(todos, "", nil, false, true, map[string]bool{"u-marta": true}); len(got) != 1 || got[0].ID != "marta" {
		t.Errorf("destacados filtra %v", nombres(got))
	}
}

// TestElBuscadorNoSeRehaceAlTeclear: la pantalla se reconstruye con cada tecla, y un
// campo nuevo perdería el foco a mitad de palabra.
func TestElBuscadorNoSeRehaceAlTeclear(t *testing.T) {
	a := appDePrueba(t, &servicioFalso{contactos: []api.Contact{persona("ana", "Ana"), persona("luis", "Luis")}})
	a.Cargar()
	campo := a.Pantalla.buscador
	if campo == nil {
		t.Fatal("no hay buscador en la lista")
	}
	test.Type(campo, "lu")
	if a.Pantalla.buscador != campo {
		t.Fatal("al teclear se ha rehecho el buscador: el foco se pierde")
	}
	if got := nombres(a.Pantalla.Contactos); !iguales(got, []string{"Luis"}) {
		t.Errorf("tras teclear «lu» la lista enseña %v", got)
	}
	// «Limpiar búsqueda» lo vacía desde fuera, y el campo lo refleja.
	a.Pantalla.AlLimpiarFiltro()
	if campo.Texto() != "" || len(a.Pantalla.Contactos) != 2 {
		t.Errorf("tras limpiar: campo %q, %d contactos", campo.Texto(), len(a.Pantalla.Contactos))
	}
}

// TestDestacarEsOptimistaYVuelveAtras: la estrella cambia al pulsar y, si el servicio
// dice que no, vuelve como estaba y sale un aviso de peligro.
func TestDestacarEsOptimistaYVuelveAtras(t *testing.T) {
	s := &servicioFalso{contactos: []api.Contact{persona("ana", "Ana")}, fallarFav: true}
	a := appDePrueba(t, s)
	a.Cargar()
	var visto bool
	a.enFondo = func(f func()) {
		visto = a.favoritos["u-ana"] // lo que se ve MIENTRAS la petición va
		f()
	}
	a.destacar(a.Pantalla.Contactos[0])
	if !visto {
		t.Error("la estrella no cambia hasta que contesta el servicio: no es optimista")
	}
	if a.favoritos["u-ana"] {
		t.Error("el servicio dijo que no y la estrella se ha quedado puesta")
	}
	if av := a.avisos.Avisos(); len(av) != 1 {
		t.Fatalf("hay %d avisos y debería haber uno", len(av))
	}
}

type abridorFalso struct{ urls []string }

func (f *abridorFalso) OpenURL(u *url.URL) error {
	f.urls = append(f.urls, u.String())
	return nil
}

// TestEnviarCorreoInvocaAMail: la ficha no redacta, invoca a Sytue Mail por su esquema
// con el `?to=` de la web.
func TestEnviarCorreoInvocaAMail(t *testing.T) {
	a := appDePrueba(t, &servicioFalso{})
	ab := &abridorFalso{}
	a.Abridor = ab
	a.Pantalla.AlEnviarCorreo("ana+casa@ejemplo.test")
	if len(ab.urls) != 1 || ab.urls[0] != "sytue-mail:/?to=ana%2Bcasa%40ejemplo.test" {
		t.Errorf("se abrió %v", ab.urls)
	}
}

// TestSinServicioLasLibretasDicenQueFallaron: con el servicio caído el menú dice que
// no ha podido y ofrece reintentar, y la lista enseña el error, no el vacío.
func TestSinServicioLasLibretasDicenQueFallaron(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusBadGateway)
	}))
	t.Cleanup(srv.Close)
	a := NuevaApp(traductor(t), api.NewContacts(srv.URL, nil, nil))
	a.enFondo = func(f func()) { f() }
	a.Cargar()
	if !a.Pantalla.ErrorLibretas || a.Pantalla.Estado != agenda.Error {
		t.Errorf("error de libretas %v, estado %v", a.Pantalla.ErrorLibretas, a.Pantalla.Estado)
	}
}

// ---- La fila ----

// TestMarcarNoAbreYAbrirNoMarca: la zona de la casilla se estira 8,75 px a cada lado
// (el `<label>` de la web) y lo que cae ahí MARCA; el resto de la fila ABRE.
func TestMarcarNoAbreYAbrirNoMarca(t *testing.T) {
	f := NuevaFilaContacto()
	var abiertas, marcadas int
	f.AlAbrir, f.AlMarcar = func() { abiertas++ }, func() { marcadas++ }
	f.Resize(fyne.NewSize(anchoPanel-1, altoFila))
	// La casilla dibuja de 12,5 a 26,5; su zona de toque va de 3,75 a 35,25.
	f.Tapped(&fyne.PointEvent{Position: fyne.NewPos(5, 24)})
	f.Tapped(&fyne.PointEvent{Position: fyne.NewPos(34, 24)})
	if marcadas != 2 || abiertas != 0 {
		t.Errorf("en el margen de la casilla: %d marcadas y %d abiertas", marcadas, abiertas)
	}
	f.Tapped(&fyne.PointEvent{Position: fyne.NewPos(150, 24)})
	if abiertas != 1 || marcadas != 2 {
		t.Errorf("en el nombre: %d abiertas y %d marcadas", abiertas, marcadas)
	}
}

// TestLaCasillaEsLaSeleccionNoLaFicha: con `ui.FilaLista` abrir un contacto lo marcaba
// para el lote. Aquí la casilla sigue a `Marcada` y solo a eso.
func TestLaCasillaEsLaSeleccionNoLaFicha(t *testing.T) {
	primario := tema.Color("primary", tema.Claro)
	dentroCasilla := func(f *FilaContacto) color.NRGBA {
		img := pintarObjeto(t, f, fyne.NewSize(anchoPanel-1, altoFila))
		// Una esquina de dentro de la caja, lejos del tic: 14 + 1 y el centro − 5.
		return px(img, 15, altoFila/2-5)
	}
	abierta := NuevaFilaContacto()
	abierta.Nombre, abierta.Subtitulo, abierta.Abierta = "Ana", "ana@ejemplo.test", true
	if got := dentroCasilla(abierta); mismoColor(got, primario) {
		t.Error("una fila ABIERTA pinta la casilla marcada: abrir no es seleccionar")
	}
	marcada := NuevaFilaContacto()
	marcada.Nombre, marcada.Subtitulo, marcada.Marcada = "Ana", "ana@ejemplo.test", true
	if got := dentroCasilla(marcada); !mismoColor(got, primario) {
		t.Errorf("una fila MARCADA pinta %v dentro de la casilla y debería ser %v", got, primario)
	}
}

// TestElNombreEmpiezaDondeLaWeb: ContactsApp.tsx fija 2 (border-l-2) + 12 (px-3) +
// 16 (h-4 de casilla) + 12 (gap-3) + 32 (avatar sm) + 10 (gap-2.5) = 84 px.
func TestElNombreEmpiezaDondeLaWeb(t *testing.T) {
	f := NuevaFilaContacto()
	f.Nombre, f.Subtitulo = "Ana Beltrán", "ana@ejemplo.test"
	f.Resize(fyne.NewSize(anchoPanel-1, altoFila))
	for _, o := range test.TempWidgetRenderer(t, f).Objects() {
		if tx, ok := o.(*canvas.Text); ok && tx.Text == f.Nombre {
			if x := tx.Position().X; x != 84 {
				t.Errorf("el nombre empieza en x = %v y en la web en 84", x)
			}
			return
		}
	}
	t.Fatal("la fila no pinta el nombre")
}

// TestLaEstrellaSoloSaleSiEstaDestacado: la estrella del final ocupa sitio SOLO con
// el contacto destacado; sin ella el nombre tiene 24 px más para no truncarse.
func TestLaEstrellaSoloSaleSiEstaDestacado(t *testing.T) {
	ambar := tema.Color("warning-text", tema.Claro)
	f := NuevaFilaContacto()
	f.Nombre = "Ana"
	tamano := fyne.NewSize(anchoPanel-1, altoFila)
	centroEstrella := float32(anchoPanel-1) - ui.MargenFila - LadoEstrellaFila/2
	x, y := int(centroEstrella), altoFila/2
	if hayColorCerca(pintarObjeto(t, f, tamano), x, y, ambar) {
		t.Error("sin destacar ya se pinta la estrella")
	}
	f.Destacado = true
	f.Refresh()
	if !hayColorCerca(pintarObjeto(t, f, tamano), x, y, ambar) {
		t.Error("destacado y la estrella no sale")
	}
}

// ---- Cumpleaños ----

// TestLosCumpleanosVanDelMasCercano: hoy, mañana y el de ayer, que ya es el año que
// viene; y la edad solo si el servicio sabe el año.
func TestLosCumpleanosVanDelMasCercano(t *testing.T) {
	hoy := time.Date(2026, 9, 22, 15, 0, 0, 0, time.Local)
	con := func(id string, y int, m time.Month, d int) api.Contact {
		c := persona(id, id)
		b := time.Date(y, m, d, 0, 0, 0, 0, time.UTC)
		c.Birthday = &b
		return c
	}
	cs := ProximosCumpleanos([]api.Contact{
		con("ayer", 1970, 9, 21), con("manana", 1985, 9, 23), con("hoy", 1990, 9, 22), persona("sin", "sin"),
	}, hoy, 365)
	if len(cs) != 3 || cs[0].Contacto.ID != "hoy" || cs[1].Contacto.ID != "manana" || cs[2].Contacto.ID != "ayer" {
		t.Fatalf("orden: %+v", cs)
	}
	if cs[0].Dias != 0 || cs[1].Dias != 1 || cs[2].Dias != 364 {
		t.Errorf("días: %d, %d, %d", cs[0].Dias, cs[1].Dias, cs[2].Dias)
	}
	if cs[0].Edad != 36 || cs[2].Edad != 57 {
		t.Errorf("edades: %d y %d", cs[0].Edad, cs[2].Edad)
	}
	if n := cumpleanosEn([]api.Contact{con("manana", 1985, 9, 23), con("ayer", 1970, 9, 21)}, hoy, 30); n != 1 {
		t.Errorf("en 30 días caen %d y es uno", n)
	}
}

// ---- utilidades ----

func iguales(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func pintarObjeto(t *testing.T, o fyne.CanvasObject, tam fyne.Size) image.Image {
	t.Helper()
	w := test.NewWindow(o)
	t.Cleanup(w.Close)
	w.SetPadded(false)
	w.Resize(tam)
	return w.Canvas().Capture()
}

// hayColorCerca mira un cuadrado de 7 × 7 alrededor de (x, y): un glifo antialiasado
// no tiene por qué tener su color exacto en el píxel del centro.
func hayColorCerca(img image.Image, x, y int, c color.NRGBA) bool {
	for dy := -3; dy <= 3; dy++ {
		for dx := -3; dx <= 3; dx++ {
			if mismoColor(px(img, x+dx, y+dy), c) {
				return true
			}
		}
	}
	return false
}
