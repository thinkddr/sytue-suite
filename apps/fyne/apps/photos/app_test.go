// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package photos

import (
	"context"
	"encoding/json"
	"fmt"
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
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/driver/desktop"
	"fyne.io/fyne/v2/test"

	"git.sytue.eu/sytue/one/apps/fyne/ui"
	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/auth"
	"git.sytue.eu/sytue/one/apps/nucleo/i18n"
	"git.sytue.eu/sytue/one/apps/nucleo/webref"
)

// servicioFalso es un syphotos en memoria: contesta lo que la web espera y apunta lo
// que le piden, para comparar contra photosApi.ts.
type servicioFalso struct {
	mu        sync.Mutex
	pedido    []string
	fotos     []api.Photo
	papelera  []api.Photo
	porPagina int
	// sinIA contesta a `/assist` como un servicio sin modelo (503 ai_not_configured).
	sinIA            bool
	excluidaLoteIA   bool
	puedeGestionarIA bool
}

func (s *servicioFalso) vistas() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]string{}, s.pedido...)
}

func (s *servicioFalso) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	defer s.mu.Unlock()
	cuerpo, _ := io.ReadAll(r.Body)
	s.pedido = append(s.pedido, strings.TrimSpace(r.Method+" "+r.URL.Path+"?"+r.URL.RawQuery+" "+string(cuerpo)))
	escribir := func(v any) { _ = json.NewEncoder(w).Encode(v) }
	switch {
	case r.Method == http.MethodGet && r.URL.Path == "/api/v1/timeline":
		q := r.URL.Query()
		desde := 0
		if c := q.Get("cursor"); c != "" {
			_, _ = fmt.Sscanf(c, "c%d", &desde)
		}
		var lista []api.Photo
		for _, f := range s.fotos {
			if q.Get("favoritas") == "1" && !f.Favorite {
				continue
			}
			if t := q.Get("q"); t != "" && !strings.Contains(f.Name, t) {
				continue
			}
			lista = append(lista, f)
		}
		hasta := min(desde+s.porPagina, len(lista))
		sig := ""
		if hasta < len(lista) {
			sig = fmt.Sprintf("c%d", hasta)
		}
		escribir(map[string]any{"items": lista[desde:hasta], "nextCursor": sig})
	case r.Method == http.MethodPut && strings.HasSuffix(r.URL.Path, "/favorite"):
		w.WriteHeader(http.StatusNoContent)
	case r.Method == http.MethodDelete && strings.HasPrefix(r.URL.Path, "/api/v1/assets/"):
		w.WriteHeader(http.StatusNoContent)
	case r.Method == http.MethodGet && r.URL.Path == "/api/v1/trash":
		escribir(map[string]any{"items": s.papelera})
	case r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/restore"):
		w.WriteHeader(http.StatusNoContent)
	case r.Method == http.MethodDelete && strings.HasPrefix(r.URL.Path, "/api/v1/trash"):
		escribir(map[string]int{"deleted": len(s.papelera)})
	case r.Method == http.MethodPost && r.URL.Path == "/api/v1/assist":
		if s.sinIA {
			w.WriteHeader(http.StatusServiceUnavailable)
			escribir(map[string]string{"code": "ai_not_configured"})
			return
		}
		var c struct{ Action, Context string }
		_ = json.Unmarshal(cuerpo, &c)
		switch c.Action {
		case "describir-foto":
			escribir(map[string]string{"text": `{"descripcion":"Una cala al atardecer","etiquetas":["playa"]}`})
		case "preguntar-foto":
			escribir(map[string]string{"text": "Hay dos barcas."})
		case "terminos-busqueda":
			escribir(map[string]string{"text": `["orilla","mar"]`})
		case "editar-foto":
			escribir(map[string]string{"text": `{"explicacion":"Más cálida.","ajustes":{"temperatura":0.4,"exposicion":0.3}}`})
		}
	case r.Method == http.MethodGet && strings.HasSuffix(r.URL.Path, "/variants/miniatura"):
		_, _ = io.WriteString(w, "JPEG")
	case r.Method == http.MethodGet && strings.HasSuffix(r.URL.Path, "/variants/previa"):
		_, _ = w.Write(pngDe(bandas(40, 20)))
	case r.Method == http.MethodGet && strings.HasSuffix(r.URL.Path, "/export"):
		_, _ = w.Write(pngDe(bandas(80, 60)))
	case r.Method == http.MethodGet && strings.HasSuffix(r.URL.Path, "/recipe"):
		escribir(map[string]any{"recipe": map[string]any{"version": 1, "pasos": []any{}}})
	case r.Method == http.MethodPut && strings.HasSuffix(r.URL.Path, "/recipe"):
		escribir(map[string]string{"fingerprint": "abc123abc123"})
	case r.Method == http.MethodPut && strings.HasSuffix(r.URL.Path, "/description"):
		w.WriteHeader(http.StatusNoContent)
	// El álbum compartido: «tok» existe y tiene las tres primeras fotos; otro, no.
	case r.URL.Path == "/api/v1/public/albums/tok":
		escribir(map[string]any{"album": map[string]any{"title": "Verano", "count": 3}, "items": s.fotos[:3]})
	case strings.HasPrefix(r.URL.Path, "/api/v1/public/albums/tok/assets/"):
		_, _ = w.Write(pngDe(bandas(40, 20)))
	// «Compartido conmigo»: «a1» lo comparte «sub-ana» con las dos primeras fotos.
	case r.URL.Path == "/api/v1/shared/albums":
		escribir(map[string]any{"items": []any{
			map[string]any{"id": "a1", "title": "Viaje", "coverAssetId": s.fotos[0].ID, "count": 2, "sharedBy": "sub-ana", "permission": "read"},
			map[string]any{"id": "a2", "title": "Boda", "count": 2, "sharedBy": "sub-ana", "permission": "write"},
		}})
	case r.URL.Path == "/api/v1/shared/albums/a2":
		escribir(map[string]any{"album": map[string]any{"id": "a2", "title": "Boda", "count": 2, "sharedBy": "sub-ana", "permission": "write"}, "items": s.fotos[:2]})
	case r.Method == http.MethodPatch && r.URL.Path == "/api/v1/albums/a2":
		escribir(map[string]any{"id": "a2", "title": "Boda en Getaria"})
	case r.Method == http.MethodDelete && strings.HasPrefix(r.URL.Path, "/api/v1/albums/a2/assets/"):
		w.WriteHeader(http.StatusNoContent)
	case r.Method == http.MethodPost && r.URL.Path == "/api/v1/albums/a2/assets":
		// Lo añadido se copia al Drive de la dueña, y no cabe.
		w.WriteHeader(http.StatusInsufficientStorage)
		escribir(map[string]any{"error": map[string]string{"code": "quota_exceeded"}})
	case r.URL.Path == "/api/v1/shared/albums/a1":
		escribir(map[string]any{"album": map[string]any{"id": "a1", "title": "Viaje", "count": 2, "sharedBy": "sub-ana"}, "items": s.fotos[:2]})
	case strings.HasPrefix(r.URL.Path, "/api/v1/shared/albums/a1/assets/"):
		_, _ = w.Write(pngDe(bandas(40, 20)))
	case r.URL.Path == "/api/v1/directory:resolve":
		escribir(map[string]string{"user_id": "sub-ana", "email": "ana@acme.test", "name": "Ana"})
	case r.URL.Path == "/api/v1/memories":
		escribir(map[string]any{"memories": []any{}})
	case r.URL.Path == "/api/v1/albums":
		escribir(map[string]any{"items": []any{}})
	case r.Method == http.MethodGet && r.URL.Path == "/api/v1/settings":
		escribir(map[string]bool{"excludeAiBatch": s.excluidaLoteIA, "canManageAiBatch": s.puedeGestionarIA})
	case r.Method == http.MethodPut && r.URL.Path == "/api/v1/settings":
		var body map[string]bool
		_ = json.Unmarshal(cuerpo, &body)
		s.excluidaLoteIA = body["excludeAiBatch"]
		escribir(map[string]bool{"excludeAiBatch": s.excluidaLoteIA, "canManageAiBatch": s.puedeGestionarIA})
	default:
		w.WriteHeader(http.StatusNotFound)
	}
}

// appDePrueba monta la app contra el servicio falso, con las llamadas EN LÍNEA: así el
// guardián lee el estado justo después de cada acción, sin esperas ni carreras.
func appDePrueba(t *testing.T, s *servicioFalso) (*App, fyne.Window) {
	t.Helper()
	srv := httptest.NewServer(s)
	t.Cleanup(srv.Close)
	w := test.NewWindow(nil)
	w.SetPadded(false)
	t.Cleanup(w.Close)
	w.Resize(fyne.NewSize(ventanaAncho, ventanaAlto))
	teca := NuevaFototeca()
	if escenasPlay {
		teca.Pedir = pedirEscena
	}
	a := Nueva(api.NewPhotos(srv.URL, nil, nil), vistaDePrueba(t, teca), w)
	a.IA = api.NewIA(srv.URL, nil, nil)
	a.lanzar = func(f func()) { f() }
	a.espera = 0
	w.SetContent(a.Objeto())
	return a, w
}

func fotosDelServicio(n int) []api.Photo {
	fotos := fotosDePrueba(n)
	for i := range fotos {
		fotos[i].Favorite = i%2 == 0
	}
	return fotos
}

// TestLaGaleriaPideLaLineaDeTiempoYPagina: la primera página al entrar y la siguiente
// cuando el final queda a menos de una pantalla (el centinela con `rootMargin: 800px`).
func TestLaGaleriaPideLaLineaDeTiempoYPagina(t *testing.T) {
	s := &servicioFalso{fotos: fotosDelServicio(30), porPagina: 10}
	a, _ := appDePrueba(t, s)
	a.Arrancar()
	if got := len(a.g.fotos); got < 10 {
		t.Fatalf("tras arrancar hay %d fotos y la primera página trae 10", got)
	}
	primera := s.vistas()[0]
	if primera != "GET /api/v1/timeline?limit=100" {
		t.Errorf("la primera petición es %q y la web pide `timeline?limit=100`", primera)
	}
	// Diez fotos no llenan 800 de alto con teselas de 201: el centinela se ve y pide
	// la siguiente página (20 fotos). Al desplazarse hacia el final, pide las 30 y el fin.
	if len(a.g.fotos) < 20 {
		t.Fatalf("con la ventana sin llenar se esperaban al menos 20 fotos; hay %d", len(a.g.fotos))
	}
	a.scroll.Offset.Y = 100
	a.quizaMas()
	if !a.g.fin || len(a.g.fotos) != 30 {
		t.Errorf("tras desplazarse se esperaban las 30 fotos y el fin; hay %d (fin=%v)", len(a.g.fotos), a.g.fin)
	}
	for _, p := range s.vistas() {
		if strings.Contains(p, "cursor=c10") {
			return
		}
	}
	t.Errorf("no se pidió la segunda página con su cursor: %v", s.vistas())
}

// TestBuscarEmpiezaOtraLinea: escribir vacía la lista y la vuelve a pedir con `q`.
func TestBuscarEmpiezaOtraLinea(t *testing.T) {
	s := &servicioFalso{fotos: fotosDelServicio(12), porPagina: 100}
	a, _ := appDePrueba(t, s)
	a.Arrancar()
	a.buscador.PonerTexto("IMG_1003")
	if len(a.g.fotos) != 1 || a.g.fotos[0].Name != "IMG_1003.jpg" {
		t.Fatalf("buscar «IMG_1003» deja %d fotos: %+v", len(a.g.fotos), a.g.fotos)
	}
	ultima := s.vistas()[len(s.vistas())-1]
	if !strings.Contains(ultima, "q=IMG_1003") {
		t.Errorf("la búsqueda no viajó en `q`: %q", ultima)
	}
}

// TestLaFavoritaCambiaAlVolverLaLlamada: en «Favoritas», desmarcar saca la foto de la
// lista —al volver la llamada, no antes—.
func TestLaFavoritaCambiaAlVolverLaLlamada(t *testing.T) {
	s := &servicioFalso{fotos: fotosDelServicio(6), porPagina: 100}
	a, _ := appDePrueba(t, s)
	a.Arrancar()
	a.Ir(RutaFavoritas)
	antes := len(a.g.fotos)
	if antes != 3 {
		t.Fatalf("«Favoritas» trae %d fotos y el servicio tiene 3", antes)
	}
	f := a.g.fotos[0]
	a.cambiarFavorita(f, false)
	if len(a.g.fotos) != antes-1 || posicion(a.g.fotos, f.ID) >= 0 {
		t.Errorf("desmarcar en «Favoritas» deja %d fotos y la desmarcada sigue: %v", len(a.g.fotos), posicion(a.g.fotos, f.ID))
	}
	ultima := s.vistas()[len(s.vistas())-1]
	if ultima != "PUT /api/v1/assets/"+f.ID+"/favorite? {\"favorite\":false}" {
		t.Errorf("la favorita se guardó con %q", ultima)
	}
}

// TestElVisorPasaDeFotoYSeCierra: las flechas pasan, Escape cierra, y la capa se va de
// la ventana (una capa que se queda deja la app entera sin poder pulsar).
func TestElVisorPasaDeFotoYSeCierra(t *testing.T) {
	s := &servicioFalso{fotos: fotosDelServicio(5), porPagina: 100}
	a, w := appDePrueba(t, s)
	a.Arrancar()
	a.abrirEnGaleria(a.g.fotos[1].ID)
	if a.vis == nil || w.Canvas().Overlays().Top() == nil {
		t.Fatal("abrir una foto no puso el visor encima de la ventana")
	}
	a.teclaSuelta(&fyne.KeyEvent{Name: fyne.KeyRight})
	if f, _ := a.vis.foto(); f.ID != a.g.fotos[2].ID {
		t.Errorf("la flecha derecha dejó %s y tocaba %s", f.ID, a.g.fotos[2].ID)
	}
	a.teclaSuelta(&fyne.KeyEvent{Name: fyne.KeyLeft})
	a.teclaSuelta(&fyne.KeyEvent{Name: fyne.KeyLeft})
	if a.vis.i != 0 {
		t.Errorf("dos flechas a la izquierda desde la 2 dejan la %d", a.vis.i)
	}
	a.runaSuelta('i')
	if !a.vis.conFicha {
		t.Error("«i» no abrió la ficha")
	}
	a.teclaSuelta(&fyne.KeyEvent{Name: fyne.KeyEscape})
	if a.vis != nil || w.Canvas().Overlays().Top() != nil {
		t.Error("Escape no cerró el visor, o dejó su capa puesta")
	}
}

// TestBorrarDesdeElVisorPideConfirmacion: Suprimir enseña la banda y solo el botón de
// la banda manda la foto a la papelera.
func TestBorrarDesdeElVisorPideConfirmacion(t *testing.T) {
	s := &servicioFalso{fotos: fotosDelServicio(3), porPagina: 100}
	a, _ := appDePrueba(t, s)
	a.Arrancar()
	f := a.g.fotos[0]
	a.abrirEnGaleria(f.ID)
	a.teclaSuelta(&fyne.KeyEvent{Name: fyne.KeyDelete})
	if !a.vis.borrando {
		t.Fatal("Suprimir no enseñó la confirmación")
	}
	if n := len(s.vistas()); strings.HasPrefix(s.vistas()[n-1], "DELETE") {
		t.Fatal("Suprimir borró sin confirmar")
	}
	a.tirarDelVisor(f)
	if a.vis != nil || posicion(a.g.fotos, f.ID) >= 0 {
		t.Error("confirmar no cerró el visor ni sacó la foto de la galería")
	}
	if n := len(s.vistas()); s.vistas()[n-1] != "DELETE /api/v1/assets/"+f.ID+"?" {
		t.Errorf("la papelera se pidió con %q", s.vistas()[n-1])
	}
}

// TestLaPapeleraRestauraYVacia: restaurar saca la foto; vaciar pregunta primero.
func TestLaPapeleraRestauraYVacia(t *testing.T) {
	s := &servicioFalso{papelera: fotosDePrueba(3), porPagina: 100}
	a, w := appDePrueba(t, s)
	a.Ir(RutaPapelera)
	if len(a.pap.fotos) != 3 {
		t.Fatalf("la papelera trae %d fotos y el servicio tiene 3", len(a.pap.fotos))
	}
	a.restaurar(a.pap.fotos[0])
	if len(a.pap.fotos) != 2 {
		t.Errorf("restaurar dejó %d fotos en la papelera", len(a.pap.fotos))
	}
	a.datosPapelera().AlVaciar()
	d, ok := w.Canvas().Overlays().Top().(interface{ Descartar() })
	if !ok || d == nil {
		t.Fatal("vaciar la papelera no preguntó antes")
	}
	pulsarConfirmar(t, w)
	if len(a.pap.fotos) != 0 || !strings.HasPrefix(s.vistas()[len(s.vistas())-1], "DELETE /api/v1/trash?") {
		t.Errorf("tras confirmar quedan %d fotos y la última petición fue %q", len(a.pap.fotos), s.vistas()[len(s.vistas())-1])
	}
}

// pulsarConfirmar pulsa el botón del verbo del diálogo de arriba.
func pulsarConfirmar(t *testing.T, w fyne.Window) {
	t.Helper()
	var pulsado bool
	var buscar func(o fyne.CanvasObject)
	buscar = func(o fyne.CanvasObject) {
		if pulsado {
			return
		}
		switch x := o.(type) {
		case *ui.Dialogo:
			if b := botonConfirmar(x); b != nil {
				b.Tapped(nil)
				pulsado = true
			}
		case *fyne.Container:
			for _, h := range x.Objects {
				buscar(h)
			}
		case fyne.Widget:
			for _, h := range test.WidgetRenderer(x).Objects() {
				buscar(h)
			}
		}
	}
	buscar(w.Canvas().Overlays().Top())
	if !pulsado {
		t.Fatal("no hay diálogo con botón de confirmar")
	}
}

// sesionSinEntrar es una sesión en la que nadie ha entrado.
type sesionSinEntrar struct{}

func (sesionSinEntrar) AccessToken(context.Context) (string, error) { return "", auth.ErrLoginRequired }
func (sesionSinEntrar) Invalidate(string)                           {}

// TestSinSesionSeOfreceEntrar: la galería no dice «sin conexión» cuando lo que pasa es
// que nadie ha entrado.
func TestSinSesionSeOfreceEntrar(t *testing.T) {
	srv := httptest.NewServer(&servicioFalso{})
	defer srv.Close()
	w := test.NewWindow(nil)
	defer w.Close()
	a := Nueva(api.NewPhotos(srv.URL, sesionSinEntrar{}, nil), vistaDePrueba(t, nil), w)
	a.lanzar = func(f func()) { f() }
	a.Arrancar()
	if e := estadoDeFallo(a.g.fallo); e != SinSesion {
		t.Errorf("sin sesión la galería queda en el estado %d y tocaba SinSesion (%d)", e, SinSesion)
	}
}

// TestTituloSugeridoEsElDeLaWeb repite los casos de lib/tituloAlbum.test.ts.
func TestTituloSugeridoEsElDeLaWeb(t *testing.T) {
	f := func(tags ...string) api.Photo { return api.Photo{Tags: tags} }
	for _, c := range []struct {
		fotos  []api.Photo
		quiero string
	}{
		{[]api.Photo{f("playa", "sombrilla"), f("playa", "gaviota")}, "Playa"},
		{[]api.Photo{f("playa", "mar", "arena", "toalla"), f("playa", "mar", "arena", "barco"), f("playa", "mar", "arena", "nube")}, "Arena · mar · playa"},
		{[]api.Photo{f("playa", "mar")}, ""},
		{[]api.Photo{f(), f()}, ""},
		{[]api.Photo{f("playa"), f("montaña"), f("ciudad"), f("bosque")}, ""},
		{[]api.Photo{f("mar", "cielo"), f("mar", "cielo")}, "Cielo · mar"},
		{[]api.Photo{f("playa", "Playa", " playa "), f("mar")}, ""},
	} {
		if got := TituloSugerido(c.fotos); got != c.quiero {
			t.Errorf("TituloSugerido(%v) = %q y la web da %q", c.fotos, got, c.quiero)
		}
	}
}

// TestLosNumerosSonLosDePhotosAppTsx ata los tiempos, los topes y las claves de las
// preferencias a los literales de la web. Cambiar uno allí pone esto en rojo.
func TestLosNumerosSonLosDePhotosAppTsx(t *testing.T) {
	app := webref.Read(t, "apps/frontend/apps/photos/src/components/PhotosApp.tsx")
	fondo := webref.Read(t, "apps/frontend/apps/photos/src/lib/fondoLienzo.ts")
	recuerdos := webref.Read(t, "apps/frontend/apps/photos/src/components/Recuerdos.tsx")
	subida := webref.Read(t, "apps/frontend/apps/photos/src/client/subida.ts")
	for _, c := range []struct{ fuente, literal, que string }{
		{app, fmt.Sprintf("setInterval(onNext, %d)", DuracionPase/time.Millisecond), "el pase"},
		{app, fmt.Sprintf("lineaDeTiempo(cursor, %d,", PaginaLinea), "la página"},
		{app, fmt.Sprintf("rootMargin: '%dpx'", AntesDelFinal), "el centinela"},
		{app, fmt.Sprintf("setTimeout(() => void cargarMas(), %d)", EsperaBusqueda/time.Millisecond), "la búsqueda"},
		{app, fmt.Sprintf(".slice(0, %d)", TopeParecidas), "«Más como estas»"},
		{app, fmt.Sprintf("min: %d, max: %d, defecto: %d", AnchoFichaMin, AnchoFichaMax, AnchoFicha), "la ficha"},
		{app, fmt.Sprintf("TOPE_DESCRIBIR = %d", TopeDescribir), "el lote de Sytue AI"},
		{fondo, "'" + prefFondo + "'", "el fondo del lienzo"},
		{recuerdos, "sytue-fotos-recuerdos-ocultos:", "los recuerdos ocultos"},
		{subida, "'/" + CarpetaFotos + "'", "la carpeta de Drive"},
	} {
		if !strings.Contains(c.fuente, c.literal) {
			t.Errorf("%s: la web ya no dice %q", c.que, c.literal)
		}
	}
}

// contar dice cuántas peticiones al servicio empiezan por ese prefijo.
func contar(s *servicioFalso, prefijo string) int {
	n := 0
	for _, p := range s.vistas() {
		if strings.HasPrefix(p, prefijo) {
			n++
		}
	}
	return n
}

// TestDescribirDesdeElVisorGuardaYAbreLaFicha: lo que ve Sytue AI se guarda sin pedir
// confirmación, llega a la foto de la galería y abre la ficha, donde se lee.
func TestDescribirDesdeElVisorGuardaYAbreLaFicha(t *testing.T) {
	s := &servicioFalso{fotos: fotosDelServicio(3), porPagina: 100}
	a, _ := appDePrueba(t, s)
	a.Arrancar()
	f := a.g.fotos[1]
	a.abrirEnGaleria(f.ID)
	a.describirFoto()
	if got := a.g.fotos[1]; got.Description != "Una cala al atardecer" || len(got.Tags) != 1 {
		t.Fatalf("la foto de la galería se quedó con %q %v", got.Description, got.Tags)
	}
	if !a.vis.conFicha || a.vis.aviso != a.v.Traduce("viewer.describedInInfo") {
		t.Errorf("tras describir la ficha está %v y el aviso dice %q", a.vis.conFicha, a.vis.aviso)
	}
	if contar(s, "PUT /api/v1/assets/"+f.ID+"/description") != 1 {
		t.Errorf("no se guardó la descripción: %v", s.vistas())
	}
	a.preguntarFoto("¿Cuántas barcas?")
	if a.vis.respuesta != "Hay dos barcas." {
		t.Errorf("la respuesta es %q", a.vis.respuesta)
	}
}

// TestSinModeloSeDiceQueNoEstaConfigurada: un 503 `ai_not_configured` no es «ha
// fallado», y la web lo dice con su propio texto.
func TestSinModeloSeDiceQueNoEstaConfigurada(t *testing.T) {
	s := &servicioFalso{fotos: fotosDelServicio(2), porPagina: 100, sinIA: true}
	a, _ := appDePrueba(t, s)
	a.Arrancar()
	a.abrirEnGaleria(a.g.fotos[0].ID)
	a.describirFoto()
	if quiero := a.v.Traduce("viewer.describeUnavailable"); a.vis.aviso != quiero {
		t.Errorf("sin modelo el aviso dice %q y la web dice %q", a.vis.aviso, quiero)
	}
}

// TestElLoteSeSaltaLasYaDescritas: describir en lote no paga dos veces por la misma
// foto, y al acabar dice cuántas describió.
func TestElLoteSeSaltaLasYaDescritas(t *testing.T) {
	fotos := fotosDelServicio(3)
	fotos[0].Description = "Ya descrita"
	s := &servicioFalso{fotos: fotos, porPagina: 100}
	a, _ := appDePrueba(t, s)
	a.Arrancar()
	a.entrarEnSeleccion()
	for _, f := range a.g.fotos {
		a.alternar(f)
	}
	a.describirSeleccion()
	if n := contar(s, "POST /api/v1/assist"); n != 2 {
		t.Errorf("el lote hizo %d llamadas a Sytue AI y tocaban 2", n)
	}
	if quiero := a.v.T.T("gallery.described", i18n.Vars{"count": 2}); a.g.aviso != quiero || a.g.lote != nil {
		t.Errorf("al acabar el aviso dice %q (quiero %q) y el lote sigue en %v", a.g.aviso, quiero, a.g.lote)
	}
}

// TestAmpliarLaBusquedaVuelveAPedirConLosTerminos: sin aciertos se ofrece ampliar, y
// ampliar pide la línea de tiempo con `tambien`.
func TestAmpliarLaBusquedaVuelveAPedirConLosTerminos(t *testing.T) {
	s := &servicioFalso{fotos: fotosDelServicio(3), porPagina: 100}
	a, _ := appDePrueba(t, s)
	a.Arrancar()
	a.buscador.PonerTexto("playa")
	if len(a.g.fotos) != 0 {
		t.Fatalf("«playa» no debería encontrar nada y encuentra %d", len(a.g.fotos))
	}
	a.ampliarBusqueda()
	if ultima := s.vistas()[len(s.vistas())-1]; !strings.Contains(ultima, "tambien=orilla%2Cmar") {
		t.Errorf("la búsqueda ampliada no llevó los términos: %q", ultima)
	}
	if !strings.Contains(a.g.aviso, "orilla, mar") {
		t.Errorf("el aviso no dice con qué se amplió: %q", a.g.aviso)
	}
}

// TestElEditorEditaYGuarda: pide la previa del ORIGINAL (la huella de la receta vacía),
// un deslizador es un paso, deshacer lo quita, y guardar manda la receta entera.
func TestElEditorEditaYGuarda(t *testing.T) {
	s := &servicioFalso{fotos: fotosDelServicio(2), porPagina: 100}
	a, w := appDePrueba(t, s)
	a.Arrancar()
	f := a.g.fotos[0]
	a.AbrirEditor(f, "")
	if a.ed == nil || a.ed.cargando || a.ed.fuente == nil || w.Canvas().Overlays().Top() == nil {
		t.Fatal("el editor no cargó la foto o no está encima de la ventana")
	}
	if contar(s, "GET /api/v1/assets/"+f.ID+"/variants/previa?f="+api.RecetaVacia().Huella()) != 1 {
		t.Errorf("no se pidió la previa del original: %v", s.vistas())
	}
	a.ed.mandos[api.PasoContraste].Poner(0.25)
	if valorDe(a.ed.receta, api.PasoContraste) != 0.25 || !a.ed.sucia() {
		t.Fatalf("mover el contraste deja la receta en %v", a.ed.receta.Pasos)
	}
	a.anyadirPaso(api.PasoReceta{Tipo: api.PasoGiro, Params: map[string]float64{"grados": 90}})
	if g := a.ed.fuente.Rect; g.Dx() != 20 || g.Dy() != 40 {
		t.Errorf("girar no rehízo la fuente: %v", g)
	}
	a.deshacerEditor()
	if len(a.ed.receta.Pasos) != 1 {
		t.Errorf("deshacer deja %d pasos", len(a.ed.receta.Pasos))
	}
	a.guardarEdicion()
	if contar(s, `PUT /api/v1/assets/`+f.ID+`/recipe? {"recipe":{"version":1,"pasos":[{"tipo":"contraste","params":{"valor":0.25}}]}}`) != 1 {
		t.Errorf("la receta se guardó con %q", s.vistas()[len(s.vistas())-1])
	}
	if a.ed.sucia() || a.ed.aviso != a.v.Traduce("editor.saved") {
		t.Errorf("tras guardar sigue sucia (%v) o el aviso dice %q", a.ed.sucia(), a.ed.aviso)
	}
	a.salirDelEditor()
	if a.ed != nil || w.Canvas().Overlays().Top() != nil {
		t.Error("salir no cerró el editor")
	}
}

// TestSytueAIEditaEnElOrdenDeLosDeslizadores: lo que propone el modelo entra en la receta
// en el orden de DESLIZADORES (exposición antes que temperatura), que es el de la web.
func TestSytueAIEditaEnElOrdenDeLosDeslizadores(t *testing.T) {
	s := &servicioFalso{fotos: fotosDelServicio(1), porPagina: 100}
	a, _ := appDePrueba(t, s)
	a.Arrancar()
	a.AbrirEditor(a.g.fotos[0], "")
	a.editarConIA("mejora-auto")
	p := a.ed.receta.Pasos
	if len(p) != 2 || p[0].Tipo != api.PasoExposicion || p[1].Tipo != api.PasoTemperatura || a.ed.iaResultado == nil {
		t.Fatalf("Sytue AI dejó %v", p)
	}
}

// TestElRecorteEntraComoUnPaso: aplicar un marco añade el recorte en fracciones y sale
// del modo recorte; uno de menos del 1 % no se aplica.
func TestElRecorteEntraComoUnPaso(t *testing.T) {
	s := &servicioFalso{fotos: fotosDelServicio(1), porPagina: 100}
	a, _ := appDePrueba(t, s)
	a.Arrancar()
	a.AbrirEditor(a.g.fotos[0], "")
	a.alternarRecorte()
	a.ed.lienzo.Marco = &Marco{X: 0.1, Y: 0.2, W: 0.005, H: 0.5}
	a.aplicarRecorte()
	if len(a.ed.receta.Pasos) != 0 || a.ed.recortando {
		t.Fatalf("un marco de medio por ciento se aplicó: %v", a.ed.receta.Pasos)
	}
	a.alternarRecorte()
	a.ed.lienzo.Marco = &Marco{X: 0.1, Y: 0.2, W: 0.5, H: 0.5}
	a.aplicarRecorte()
	if p := a.ed.receta.Pasos; len(p) != 1 || p[0].Tipo != api.PasoRecorte || p[0].Params["ancho"] != 0.5 {
		t.Errorf("el recorte entró como %v", p)
	}
}

// TestElAsaDeLaFichaSeMueveConLasFlechas: 16 por flecha y 48 con Mayúsculas, y la
// izquierda la hace CRECER (la ficha está a la derecha).
func TestElAsaDeLaFichaSeMueveConLasFlechas(t *testing.T) {
	v := vistaDePrueba(t, nil)
	var medida float32
	a := NuevaAsaFicha(v, Visor{AnchoFicha: 340, AlAnchoFicha: func(m float32, _ bool) { medida = m }})
	a.TypedKey(&fyne.KeyEvent{Name: fyne.KeyLeft})
	if medida != 356 {
		t.Errorf("la flecha izquierda deja %v y toca 340 + 16", medida)
	}
	a.KeyDown(&fyne.KeyEvent{Name: desktop.KeyShiftLeft})
	a.TypedKey(&fyne.KeyEvent{Name: fyne.KeyRight})
	a.KeyUp(&fyne.KeyEvent{Name: desktop.KeyShiftLeft})
	if medida != 292 {
		t.Errorf("Mayúsculas y la flecha derecha dejan %v y toca 340 − 48", medida)
	}
}

// TestElMesSePegaArribaAlDesplazarse: con el primer mes ya pasado por arriba, su
// cabecera va pegada en lo alto de la vista (`sticky top-0`); con la selección puesta
// la barra se pega en 0 y el mes debajo, en `top-[3.25rem]`.
func TestElMesSePegaArribaAlDesplazarse(t *testing.T) {
	s := &servicioFalso{fotos: fotosDelServicio(40), porPagina: 100}
	a, w := appDePrueba(t, s)
	a.Arrancar()
	w.Canvas().Refresh(a.Objeto())
	a.scroll.Resize(a.scroll.Size()) // que la pila coloque sus bloques
	if len(a.peg.meses) < 2 {
		t.Fatalf("la galería tiene %d meses y la prueba necesita dos", len(a.peg.meses))
	}
	primero := a.peg.meses[0]
	a.scroll.Offset.Y = a.peg.pila.Position().Y + primero.cabecera.Position().Y + 100
	a.pegar()
	if len(a.peg.capa.Objects) != 1 || a.peg.cual != 0 || a.peg.capa.Objects[0].Position().Y != 0 {
		t.Fatalf("pasado el primer mes la capa tiene %d objetos, el mes %d, en y=%v",
			len(a.peg.capa.Objects), a.peg.cual, func() float32 {
				if len(a.peg.capa.Objects) > 0 {
					return a.peg.capa.Objects[0].Position().Y
				}
				return -1
			}())
	}
	a.scroll.Offset.Y = 0
	a.pegar()
	if len(a.peg.capa.Objects) != 0 {
		t.Error("arriba del todo no se pega nada")
	}

	a.entrarEnSeleccion()
	a.scroll.Resize(a.scroll.Size())
	a.scroll.Offset.Y = a.peg.pila.Position().Y + a.peg.meses[0].cabecera.Position().Y + 100
	a.pegar()
	if n := len(a.peg.capa.Objects); n != 2 {
		t.Fatalf("con la selección se pegan %d cosas y tocan la barra y el mes", n)
	}
	if mes, barra := a.peg.capa.Objects[0], a.peg.capa.Objects[1]; barra.Position().Y != 0 || mes.Position().Y != TopeBajoBarra {
		t.Errorf("la barra va en %v (toca 0) y el mes en %v (toca %v)", barra.Position().Y, mes.Position().Y, TopeBajoBarra)
	}
}

// TestElEnlaceAbreElAlbumCompartido: `sytue-photos:/p/{token}` enseña el álbum sin nav,
// pedido por `/api/v1/public/…` (lo que sale por el ingress); una tesela abre la foto
// sola en una capa sobre la ventana, pedida por el mismo enlace, y Escape la cierra. Un
// token que no existe es la página «no encontrada», como el 404 de la web.
func TestElEnlaceAbreElAlbumCompartido(t *testing.T) {
	s := &servicioFalso{fotos: fotosDelServicio(5), porPagina: 100}
	a, w := appDePrueba(t, s)
	noEncontrada := 0
	a.NoEncontrado = func() bool { noEncontrada++; return true }
	enlace, _ := url.Parse("sytue-photos:/p/tok")
	a.AbrirPublico(TokenDeEnlace(enlace))
	a.Arrancar() // el enlace llegó antes de arrancar: no se pisa con la galería

	if a.ruta != RutaPublica || a.pub.album.Title != "Verano" || len(a.pub.album.Items) != 3 {
		t.Fatalf("el enlace deja la ruta %d con %+v", a.ruta, a.pub.album)
	}
	if contar(s, "GET /api/v1/public/albums/tok?") != 1 || contar(s, "GET /api/v1/timeline") != 0 {
		t.Errorf("pidió %v", s.vistas())
	}

	var tesela *Miniatura
	var buscar func(o fyne.CanvasObject)
	buscar = func(o fyne.CanvasObject) {
		switch x := o.(type) {
		case *Miniatura:
			if tesela == nil {
				tesela = x
			}
		case *fyne.Container:
			for _, h := range x.Objects {
				buscar(h)
			}
		case *container.Scroll:
			buscar(x.Content)
		}
	}
	buscar(a.scroll.Content)
	if tesela == nil {
		t.Fatal("el álbum compartido no pinta teselas")
	}
	tesela.Tapped(nil)
	if a.pub.capa == nil || w.Canvas().Overlays().Top() != a.pub.capa || a.pub.abierta.ID != a.pub.album.Items[0].ID {
		t.Fatal("pulsar la tesela no abre la foto sola")
	}
	_ = w.Canvas().Capture() // la foto grande se pide al pintarla
	a.pub.grande.Esperar()
	if contar(s, "GET /api/v1/public/albums/tok/assets/"+a.pub.album.Items[0].ID+"/variants/grande") != 1 {
		t.Errorf("la foto abierta no se pidió por el enlace: %v", s.vistas())
	}
	a.teclaSuelta(&fyne.KeyEvent{Name: fyne.KeyEscape})
	if a.pub.capa != nil || w.Canvas().Overlays().Top() != nil {
		t.Error("Escape no cierra la foto")
	}

	a.AbrirPublico("otro")
	if noEncontrada != 1 {
		t.Errorf("un enlace que no existe no lleva a «no encontrada» (%d)", noEncontrada)
	}
}

// TestTokenDeEnlace: el esquema de la app y la dirección web, con o sin el prefijo de
// la app; nada más.
func TestTokenDeEnlace(t *testing.T) {
	for enlace, want := range map[string]string{
		"sytue-photos:/p/tok":                         "tok",
		"https://photos.sytue.eu/photos/p/tok":        "tok",
		"https://photos.sytue.eu/p/tok":               "tok",
		"sytue-photos:/p/":                            "",
		"sytue-photos:/photos/abrir?file=f1":          "",
		"https://photos.sytue.eu/p/tok/img/f1/grande": "",
		"sytue-photos:/open-from-drive?file=p%2Ftok":  "",
	} {
		u, err := url.Parse(enlace)
		if err != nil {
			t.Fatal(err)
		}
		if got := TokenDeEnlace(u); got != want {
			t.Errorf("TokenDeEnlace(%q) = %q, quería %q", enlace, got, want)
		}
	}
}

// TestLaMarcaAproximadaSeVeConElFoco: la «~» de una foto sin fecha de captura sale al
// pasar por encima Y con el foco dentro (`group-focus-within`), que es como la ve quien
// recorre la galería con Tab.
func TestLaMarcaAproximadaSeVeConElFoco(t *testing.T) {
	m := NuevaMiniatura(api.Photo{ID: "f", Name: "f.jpg"}, nil)
	m.Resize(MedidaMiniatura())
	r, ok := test.WidgetRenderer(m).(*miniaturaRenderer)
	if !ok {
		t.Fatal("la tesela no usa su renderer")
	}
	if r.aprox.Visible() {
		t.Fatal("sin foco ni puntero la marca ya se ve")
	}
	m.FocusGained()
	if !r.aprox.Visible() {
		t.Error("con el foco dentro no se ve la marca «~»")
	}
}

// TestElTamanyoRealPideElOriginalUnaVez: la «z» pasa a tamaño real y pide la foto
// entera por `/export` (la de «Descargar», con su receta); volver y repetir no la pide
// otra vez, y pasar a otra foto la suelta.
func TestElTamanyoRealPideElOriginalUnaVez(t *testing.T) {
	s := &servicioFalso{fotos: fotosDelServicio(3), porPagina: 100}
	a, _ := appDePrueba(t, s)
	a.Arrancar()
	a.abrirEnGaleria(a.g.fotos[0].ID)
	a.runaVisor('z')
	f, _ := a.vis.foto()
	if !a.vis.tamanyoReal || a.vis.originalDe(f.ID, a.Huella(f.ID)) == nil {
		t.Fatal("la «z» no dejó el original puesto")
	}
	a.runaVisor('z')
	a.runaVisor('z')
	if n := contar(s, "GET /api/v1/assets/"+f.ID+"/export?formato=jpeg"); n != 1 {
		t.Errorf("el original se pidió %d veces: %v", n, s.vistas())
	}
	a.siguiente()
	if a.vis.original != nil {
		t.Error("al pasar de foto el original de la anterior sigue en memoria")
	}
}

// TestLasFotosDelTelefonoSonImagenes: HEIC, HEIF y AVIF son imágenes aunque el sistema
// no traiga la tabla de `mime` (Android, iOS), y suben con su tipo. Lo que no es imagen,
// no.
func TestLasFotosDelTelefonoSonImagenes(t *testing.T) {
	for ruta, want := range map[string]string{
		"/tmp/IMG_0001.HEIC": "image/heic", "a.heif": "image/heif", "b.avif": "image/avif",
		"c.jpg": "image/jpeg", "notas.txt": "",
	} {
		if got := tipoDe(ruta); !strings.HasPrefix(got, want) || (want == "" && esImagen(ruta)) {
			t.Errorf("tipoDe(%q) = %q, quería %q", ruta, got, want)
		}
	}
}

// TestArrastrarEnciendeSueltaParaAnyadir: con ficheros por encima de la ventana, la
// galería cambia su aviso por «Suelta para añadir» —también si había otro aviso—, y lo
// quita al dejar de estar encima. Fuera de «Fotos» y «Favoritas» no se enseña.
func TestArrastrarEnciendeSueltaParaAnyadir(t *testing.T) {
	s := &servicioFalso{fotos: fotosDelServicio(3), porPagina: 100}
	a, _ := appDePrueba(t, s)
	a.Arrastrando(true) // antes de arrancar: se recuerda
	a.Arrancar()
	a.g.aviso = "otro aviso"
	a.pintar()
	hay := func() (borde, aviso bool) {
		var buscar func(fyne.CanvasObject)
		buscar = func(o fyne.CanvasObject) {
			switch x := o.(type) {
			case *ui.BordeDiscontinuo:
				borde = true
			case *ui.Rotulo:
				aviso = aviso || x.Texto == "otro aviso"
			case *fyne.Container:
				for _, h := range x.Objects {
					buscar(h)
				}
			}
		}
		buscar(a.scroll.Content)
		return borde, aviso
	}
	if borde, aviso := hay(); !borde || aviso {
		t.Fatalf("con ficheros encima: borde=%v y el otro aviso=%v", borde, aviso)
	}
	a.Arrastrando(false)
	if borde, aviso := hay(); borde || !aviso {
		t.Errorf("al dejar de estar encima: borde=%v y el otro aviso=%v", borde, aviso)
	}
	a.Ir(RutaAlbumes)
	a.Arrastrando(true)
	if borde, _ := hay(); borde {
		t.Error("en «Álbumes» se enseña «Suelta para añadir», y ahí no se puede soltar")
	}
}

// TestCompartidoConmigo: la pestaña pide los álbumes que me han compartido y dice quién
// (con el directorio); abrir uno pide sus fotos por su propia puerta —la del álbum
// compartido, no la de mi galería— y enseña «Solo lectura»; el aviso del Hub
// (`/photos/<org>/shared?album=`) abre ese mismo álbum.
func TestCompartidoConmigo(t *testing.T) {
	s := &servicioFalso{fotos: fotosDelServicio(4), porPagina: 100}
	a, _ := appDePrueba(t, s)
	srvDir := httptest.NewServer(s)
	t.Cleanup(srvDir.Close)
	a.Directorio = api.NewDirectorio(srvDir.URL, nil, nil)
	a.Arrancar()
	a.irSeccion(SecCompartidos)
	if a.Ruta() != RutaCompartidos || len(a.cmp.lista) != 2 || a.cmp.nombres["sub-ana"] != "Ana" {
		t.Fatalf("«Compartido conmigo»: ruta %v, lista %+v, nombres %+v", a.Ruta(), a.cmp.lista, a.cmp.nombres)
	}
	if got := a.cmp.portadas[s.fotos[0].ID]; got != "a1" {
		t.Errorf("la portada no sabe de qué álbum es: %q", got)
	}
	u, _ := url.Parse("sytue-photos:/photos/acme/shared?album=a1")
	a.AbrirCompartido(AlbumDeEnlace(u))
	if a.Ruta() != RutaCompartido || len(a.pub.album.Items) != 2 || a.cmp.por != "Ana" {
		t.Fatalf("el álbum compartido: ruta %v, fotos %d, por %q", a.Ruta(), len(a.pub.album.Items), a.cmp.por)
	}
	if _, err := a.pub.teca.Pedir(context.Background(), s.fotos[0].ID, "miniatura"); err != nil {
		t.Fatal(err)
	}
	ultima := s.vistas()[len(s.vistas())-1]
	if !strings.HasPrefix(ultima, "GET /api/v1/shared/albums/a1/assets/"+s.fotos[0].ID+"/variants/miniatura") {
		t.Errorf("la miniatura de un álbum compartido se pidió por %q", ultima)
	}
	if !contieneTextoApp(a.scroll.Content, a.v.Traduce("sharedWithMe.readOnly")) {
		t.Error("el álbum compartido no dice «Solo lectura»")
	}
}

// contieneTextoApp busca un rótulo en el árbol de la pantalla: un texto o una insignia.
func contieneTextoApp(o fyne.CanvasObject, quiero string) bool {
	switch x := o.(type) {
	case *canvas.Text:
		return x.Text == quiero
	case *ui.Insignia:
		return x.Texto == quiero
	case *ui.Boton:
		return x.Texto == quiero
	case *fyne.Container:
		for _, h := range x.Objects {
			if contieneTextoApp(h, quiero) {
				return true
			}
		}
	case *container.Scroll:
		return contieneTextoApp(x.Content, quiero)
	}
	return false
}

// TestUn401EsLaSesionYUn403ElPermiso: con el cliente ya renovado y reintentado, un 401
// dice que hay que volver a entrar, no que falte un permiso. VISTO EN ROJO con el 401
// en SinPermiso, que es lo que tenía.
func TestUn401EsLaSesionYUn403ElPermiso(t *testing.T) {
	if got := estadoDeFallo(&api.Error{Status: 401}); got != SinSesion {
		t.Errorf("un 401 da %v, quiero SinSesion", got)
	}
	if got := estadoDeFallo(&api.Error{Status: 403}); got != SinPermiso {
		t.Errorf("un 403 da %v, quiero SinPermiso", got)
	}
}

// Un álbum compartido conmigo PARA EDITAR: dice «Puede editar», se renombra y se le
// quitan fotos por las rutas de su dueña, sale en «Añadir a un álbum…» con de quién es, y
// si lo añadido no cabe en su Drive se dice con la frase de la cuota.
func TestAlbumCompartidoParaEditar(t *testing.T) {
	s := &servicioFalso{fotos: fotosDelServicio(4), porPagina: 100}
	a, _ := appDePrueba(t, s)
	srvDir := httptest.NewServer(s)
	t.Cleanup(srvDir.Close)
	a.Directorio = api.NewDirectorio(srvDir.URL, nil, nil)
	a.Arrancar()
	a.AbrirCompartido("a2")
	if !a.cmp.editable || !contieneTextoApp(a.scroll.Content, a.v.Traduce("sharePeople.write")) ||
		!contieneTextoApp(a.scroll.Content, a.v.Traduce("sharedAlbum.rename")) {
		t.Fatal("un álbum compartido para editar no se ofrece para editar")
	}
	a.renombrarCompartidoA("a2", "Boda en Getaria")
	if a.pub.album.Title != "Boda en Getaria" {
		t.Errorf("renombrar deja %q", a.pub.album.Title)
	}
	a.verPublica(a.pub.album.Items[0])
	a.quitarCompartida(a.pub.album.Items[0])
	if len(a.pub.album.Items) != 1 || a.pub.capa != nil {
		t.Errorf("quitar deja %d fotos y el visor abierto=%v", len(a.pub.album.Items), a.pub.capa != nil)
	}
	var destino api.Album
	a.albumes = nil
	a.conAlbumes(func(l []api.Album) {
		for _, al := range l {
			if al.ID == "a2" {
				destino = al
			}
		}
		for _, al := range l {
			if al.ID == "a1" {
				t.Error("uno compartido solo para ver sale en «Añadir a un álbum…»")
			}
		}
	})
	if destino.Rotulo() != "Boda · Ana" {
		t.Fatalf("en «Añadir a un álbum…» sale %q", destino.Rotulo())
	}
	a.Ir(RutaFotos)
	a.g.seleccion = map[string]bool{s.fotos[0].ID: true}
	a.anyadirSeleccionA(destino)
	if a.g.error != a.v.Traduce("sharedAlbum.quotaFull") {
		t.Errorf("sin espacio dice %q", a.g.error)
	}
}

// TestElEditorComparaConElOriginal: sin ajustes de color el botón está inerte; con un ajuste
// de color la tecla `\` y alternarComparar activan la comparación sin pasos de color.
func TestElEditorComparaConElOriginal(t *testing.T) {
	s := &servicioFalso{fotos: fotosDelServicio(1), porPagina: 100}
	a, _ := appDePrueba(t, s)
	a.Arrancar()
	a.AbrirEditor(a.g.fotos[0], "")

	// Sin ajustes de color: no compara
	a.alternarComparar()
	if a.ed.comparar {
		t.Fatal("sin ajustes de color no debe activar la comparación")
	}

	// Ajuste de color
	a.ed.mandos[api.PasoContraste].Poner(0.5)
	if !conAjusteDeColor(a.ed.receta) {
		t.Fatal("el contraste no cuenta como ajuste de color")
	}

	// Tecla '\' activa comparar
	a.runaEditor('\\')
	if !a.ed.comparar {
		t.Fatal("la tecla \\ no activó la comparación")
	}

	// Segunda pulsación desactiva comparar
	a.runaEditor('\\')
	if a.ed.comparar {
		t.Fatal("la tecla \\ no desactivó la comparación")
	}

	// Activa y luego deshace el ajuste: debe apagar comparar
	a.alternarComparar()
	if !a.ed.comparar {
		t.Fatal("alternarComparar no activó comparar")
	}
	a.deshacerEditor()
	if a.ed.comparar {
		t.Fatal("deshacer el último ajuste de color debe apagar comparar")
	}
}

func TestExcluirOrganizacionDelLoteIA(t *testing.T) {
	s := &servicioFalso{fotos: fotosDelServicio(2), porPagina: 100, puedeGestionarIA: true}
	a, _ := appDePrueba(t, s)
	a.Arrancar()
	a.Ir(RutaAlbumes)
	if !a.aj.CanManageAIBatch || a.aj.ExcludeAIBatch {
		t.Fatalf("ajustes iniciales inesperados: %+v", a.aj)
	}
	a.cambiarExclusionLote(true)
	if !a.aj.ExcludeAIBatch || !contieneTextoApp(a.scroll.Content, a.v.Traduce("aiBatch.excludedHint")) {
		t.Fatalf("tras excluir debe quedar activo y mostrar el aviso: %+v", a.aj)
	}
}
