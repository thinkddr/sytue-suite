// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package photos

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"slices"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/test"

	"git.sytue.eu/sytue/one/apps/fyne/ui"
	"git.sytue.eu/sytue/one/apps/nucleo/api"
)

// «Organizar con IA» contra un syphotos de mentira con UN álbum («Verano en Cádiz», con
// f9) y seis fotos: f1, f2 y f5 ya descritas, f3 y f4 sin nada y f9 ya en el álbum. Son
// los mismos casos que `organizar-ia.test.tsx` y `organizarIA.test.ts` de la web.
type servicioOrganizar struct {
	mu     sync.Mutex
	pedido []string
	// contexto es lo que se le contó al modelo al agrupar.
	contexto string
	fotos    []api.Photo
	// sinIA contesta a `/assist` como un servicio sin modelo; albumRoto, 500 al leer el
	// álbum; agrupa es lo que contesta el modelo al agrupar.
	sinIA, albumRoto bool
	agrupa           string
}

func nuevoServicioOrganizar() *servicioOrganizar {
	f := func(id, desc string) api.Photo {
		return api.Photo{
			ID: id, Name: id + ".jpg", Width: 4, Height: 3, Description: desc,
			TakenAt: time.Date(2024, 7, 14, 10, 0, 0, 0, time.UTC), TakenFromEXIF: true,
		}
	}
	return &servicioOrganizar{
		fotos:  []api.Photo{f("f1", "Una cala"), f("f2", "La sierra"), f("f3", ""), f("f4", ""), f("f5", "Nieve"), f("f9", "Ya en un álbum")},
		agrupa: `{"albumes":[{"titulo":"VERANO EN CADIZ","ids":["1","3"]},{"titulo":"Montaña","ids":["2","4","5"]}]}`,
	}
}

func (s *servicioOrganizar) vistas() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return slices.Clone(s.pedido)
}

func (s *servicioOrganizar) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	defer s.mu.Unlock()
	cuerpo, _ := io.ReadAll(r.Body)
	escribir := func(v any) { _ = json.NewEncoder(w).Encode(v) }
	ruta := r.Method + " " + r.URL.Path
	switch {
	case ruta == "GET /api/v1/albums":
		escribir(map[string]any{"items": []any{map[string]any{"id": "v", "title": "Verano en Cádiz", "count": 1}}})
	case ruta == "GET /api/v1/albums/v/assets":
		if s.albumRoto {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		escribir(map[string]any{"items": []api.Photo{{ID: "f9"}}})
	case ruta == "GET /api/v1/timeline":
		escribir(map[string]any{"items": s.fotos})
	case strings.HasSuffix(ruta, "/variants/miniatura"):
		_, _ = io.WriteString(w, "JPEG")
	case ruta == "POST /api/v1/assist":
		var c struct{ Action, Context string }
		_ = json.Unmarshal(cuerpo, &c)
		ruta += " " + c.Action
		if s.sinIA {
			w.WriteHeader(http.StatusServiceUnavailable)
			escribir(map[string]string{"code": "ai_not_configured"})
			break
		}
		if c.Action == "agrupar-fotos" {
			s.contexto = c.Context
			escribir(map[string]string{"text": s.agrupa})
		} else {
			escribir(map[string]string{"text": `{"descripcion":"Descrita","etiquetas":["playa"]}`})
		}
	case r.Method == http.MethodPut && strings.HasSuffix(r.URL.Path, "/description"):
		w.WriteHeader(http.StatusNoContent)
	case ruta == "POST /api/v1/albums":
		var c struct{ Title string }
		_ = json.Unmarshal(cuerpo, &c)
		ruta += " " + c.Title
		escribir(map[string]any{"id": "nuevo", "title": c.Title})
	case r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/assets"):
		var c struct{ IDs []string }
		_ = json.Unmarshal(cuerpo, &c)
		ruta += " " + strings.Join(c.IDs, ",")
		escribir(map[string]int{"added": len(c.IDs)})
	case r.Method == http.MethodDelete:
		w.WriteHeader(http.StatusNoContent)
	default:
		w.WriteHeader(http.StatusNotFound)
	}
	s.pedido = append(s.pedido, ruta)
}

// appOrganizar es appDePrueba contra este servicio, en «Álbumes».
func appOrganizar(t *testing.T, s *servicioOrganizar) *App {
	t.Helper()
	srv := httptest.NewServer(s)
	t.Cleanup(srv.Close)
	w := test.NewWindow(nil)
	t.Cleanup(w.Close)
	w.Resize(fyne.NewSize(ventanaAncho, ventanaAlto))
	a := Nueva(api.NewPhotos(srv.URL, nil, nil), vistaDePrueba(t, NuevaFototeca()), w)
	a.IA = api.NewIA(srv.URL, nil, nil)
	a.lanzar = func(f func()) { f() }
	a.espera = 0
	w.SetContent(a.Objeto())
	a.Ir(RutaAlbumes)
	return a
}

func pedidosQueEmpiezan(s *servicioOrganizar, prefijo string) []string {
	var out []string
	for _, p := range s.vistas() {
		if strings.HasPrefix(p, prefijo) {
			out = append(out, p)
		}
	}
	return out
}

// TestOrganizarDescribeSoloLoQueFaltaYEnsenyaLaPropuesta: parte de las fotos sin álbum,
// describe SOLO las que no tienen nada, le cuenta al modelo lo visto con números cortos
// y enseña la propuesta SIN crear nada.
//
// VISTO EN ROJO: describiendo todas, se piden seis descripciones; sin quitar las del
// álbum, f9 llega al contexto.
func TestOrganizarDescribeSoloLoQueFaltaYEnsenyaLaPropuesta(t *testing.T) {
	s := nuevoServicioOrganizar()
	a := appOrganizar(t, s)
	a.organizar()

	if got := pedidosQueEmpiezan(s, "GET /api/v1/assets/"); !slices.Equal(got, []string{
		"GET /api/v1/assets/f3/variants/miniatura", "GET /api/v1/assets/f4/variants/miniatura",
	}) {
		t.Errorf("se describieron %v y solo tocaban f3 y f4", got)
	}
	if n := len(pedidosQueEmpiezan(s, "PUT /api/v1/assets/")); n != 2 {
		t.Errorf("se guardaron %d descripciones y tocaban 2", n)
	}
	if lineas := strings.Split(s.contexto, "\n"); len(lineas) != 5 || strings.Contains(s.contexto, "f9") ||
		!strings.Contains(lineas[2], `"descripcion":"Descrita"`) {
		t.Errorf("el contexto no es el de las cinco sueltas con lo descrito:\n%s", s.contexto)
	}
	if n := len(pedidosQueEmpiezan(s, "POST /api/v1/albums")); n != 0 {
		t.Errorf("la propuesta creó %d cosas antes de aceptarla", n)
	}
	o := a.alb.org
	if o.fase != OrgPropuesta || len(o.propuesta) != 2 {
		t.Fatalf("fase %v con %d álbumes propuestos", o.fase, len(o.propuesta))
	}
	if got := []string{o.propuesta[0].titulo.Texto(), o.propuesta[1].titulo.Texto()}; !slices.Equal(got, []string{"VERANO EN CADIZ", "Montaña"}) {
		t.Errorf("títulos %v", got)
	}
	if got := idsDe(o.propuesta[1].fotos); !slices.Equal(got, []string{"f2", "f4", "f5"}) {
		t.Errorf("el segundo lleva %v", got)
	}

	// Y se ve: el título que ya existe lo dice, con cuántas fotos y sus miniaturas.
	d := a.datosOrganizar()
	if !d.Propuesta[0].Existe || d.Propuesta[1].Existe {
		t.Errorf("«ya existe» es %v y %v", d.Propuesta[0].Existe, d.Propuesta[1].Existe)
	}
	p := SeccionOrganizar(a.v, d)
	p.Resize(fyne.NewSize(1200, 2000))
	textos := strings.Join(textosDePantalla(p), "|")
	for _, quiero := range []string{"Álbumes propuestos: 2", "2 fotos", "3 fotos", "Se añadirán al álbum que ya tiene este nombre"} {
		if !strings.Contains(textos, quiero) {
			t.Errorf("la vista previa no dice «%s»: %s", quiero, textos)
		}
	}
	if botonCon(p, "Crear álbumes") == nil || botonCon(p, "Descartar") == nil {
		t.Error("faltan «Crear álbumes» o «Descartar»")
	}
}

// TestOrganizarCreaLoMarcadoYLoDeshace: lo desmarcado no se crea, lo que se llama como un
// álbum que ya existe va a ese, el título editado se respeta y el aviso trae «Deshacer»,
// que borra el nuevo y saca las fotos del que ya existía.
//
// VISTO EN ROJO: comparando títulos sin plegar, «VERANO EN CADIZ» crea un álbum nuevo; y
// sin `ids` en lo hecho, «Deshacer» no saca nada del que ya existía.
func TestOrganizarCreaLoMarcadoYLoDeshace(t *testing.T) {
	s := nuevoServicioOrganizar()
	a := appOrganizar(t, s)
	a.organizar()
	o := &a.alb.org
	o.propuesta[1].titulo.PonerTexto("Sierra Nevada")
	a.datosOrganizar().Propuesta[0].AlIncluir(false)
	if o.propuesta[0].incluido {
		t.Fatal("la casilla no desmarca el álbum")
	}
	a.datosOrganizar().Propuesta[0].AlIncluir(true)

	antes := len(s.vistas())
	a.crearOrganizados()
	creado := s.vistas()[antes:]
	quiero := []string{
		"POST /api/v1/albums/v/assets f1,f3",
		"POST /api/v1/albums Sierra Nevada",
		"POST /api/v1/albums/nuevo/assets f2,f4,f5",
		"GET /api/v1/albums", // la lista se vuelve a pedir
	}
	if !slices.Equal(creado, quiero) {
		t.Fatalf("al crear se pidió\n%v\ny tocaba\n%v", creado, quiero)
	}
	av := ultimoAviso(a)
	if av.Titulo != "Álbumes organizados: 2" || av.Descripcion != "Fotos añadidas: 5" ||
		av.Duracion != ui.DuracionDeshacer || av.RotuloAccion != "Deshacer" || av.AlPulsarAccion == nil {
		t.Fatalf("el aviso no trae su «Deshacer» de 10 s: %+v", av)
	}
	if o.fase != OrgInactiva {
		t.Errorf("tras crear la sección sigue en %v", o.fase)
	}

	antes = len(s.vistas())
	av.AlPulsarAccion()
	deshecho := slices.Clone(s.vistas()[antes:])
	sort.Strings(deshecho)
	if !slices.Equal(deshecho, []string{
		"DELETE /api/v1/albums/nuevo", "DELETE /api/v1/albums/v/assets/f1", "DELETE /api/v1/albums/v/assets/f3", "GET /api/v1/albums",
	}) {
		t.Errorf("«Deshacer» pidió %v", deshecho)
	}
	if ultimoAviso(a).Titulo != "Se ha deshecho la organización" {
		t.Errorf("tras deshacer se avisa «%s»", ultimoAviso(a).Titulo)
	}
}

// TestOrganizarSinSytueAI: sin cliente de IA lo dice sin pedir nada, y con un servicio
// sin modelo se para en la primera descripción, con el mismo texto que describir.
func TestOrganizarSinSytueAI(t *testing.T) {
	s := nuevoServicioOrganizar()
	a := appOrganizar(t, s)
	a.IA = nil
	antes := len(s.vistas())
	a.organizar()
	if a.alb.org.error != "Sytue AI no está configurada en esta instalación" || len(s.vistas()) != antes {
		t.Errorf("sin IA: %q y %v", a.alb.org.error, s.vistas()[antes:])
	}

	s = nuevoServicioOrganizar()
	s.sinIA = true
	a = appOrganizar(t, s)
	a.organizar()
	if a.alb.org.error != "Sytue AI no está configurada en esta instalación" {
		t.Errorf("con el servicio sin modelo dice %q", a.alb.org.error)
	}
	if n := len(pedidosQueEmpiezan(s, "POST /api/v1/assist")); n != 1 {
		t.Errorf("sin modelo se hicieron %d llamadas y con la primera bastaba", n)
	}
}

// TestOrganizarSinNadaQueOrganizar: si todas están en un álbum, o el modelo no ve grupos,
// es un estado vacío y no un fallo.
func TestOrganizarSinNadaQueOrganizar(t *testing.T) {
	s := nuevoServicioOrganizar()
	s.fotos = s.fotos[5:]
	a := appOrganizar(t, s)
	a.organizar()
	if a.alb.org.fase != OrgVacia || a.alb.org.vacio != "albums.organize.nothing" || len(pedidosQueEmpiezan(s, "POST")) != 0 {
		t.Errorf("sin sueltas: %+v %v", a.alb.org, s.vistas())
	}
	if v := vacioDe(SeccionOrganizar(a.v, a.datosOrganizar())); v == nil || v.Titulo != "Todas tus fotos ya están en algún álbum" {
		t.Errorf("el vacío es %+v", v)
	}

	s = nuevoServicioOrganizar()
	s.agrupa = `{"albumes":[]}`
	a = appOrganizar(t, s)
	a.organizar()
	if a.alb.org.fase != OrgVacia || a.alb.org.vacio != "albums.organize.noGroups" {
		t.Errorf("sin grupos: %+v", a.alb.org)
	}
}

// TestOrganizarNoSigueSiUnAlbumNoSeLee: sin saber qué tiene un álbum se propondría
// ordenar fotos ya ordenadas.
func TestOrganizarNoSigueSiUnAlbumNoSeLee(t *testing.T) {
	s := nuevoServicioOrganizar()
	s.albumRoto = true
	a := appOrganizar(t, s)
	a.organizar()
	if a.alb.org.error == "" || len(pedidosQueEmpiezan(s, "GET /api/v1/timeline")) != 0 {
		t.Errorf("con un álbum ilegible: %q y %v", a.alb.org.error, s.vistas())
	}
}

// TestPararMientrasDescribe: «Parar» entre una descripción y otra no llega a agrupar, y
// lo que ya volvió se tira.
//
// VISTO EN ROJO: sin comprobar la pasada (`vez`) al volver, la segunda descripción y la
// llamada de agrupar salen igual.
func TestPararMientrasDescribe(t *testing.T) {
	s := nuevoServicioOrganizar()
	a := appOrganizar(t, s)
	var cola []func()
	a.lanzar = func(f func()) { cola = append(cola, f) }
	a.organizar()
	siguiente := func() {
		f := cola[0]
		cola = cola[1:]
		f()
	}
	siguiente() // las fotos sin álbum
	if a.alb.org.fase != OrgDescribiendo || a.alb.org.total != 2 {
		t.Fatalf("tras buscar: fase %v de %d", a.alb.org.fase, a.alb.org.total)
	}
	if p := SeccionOrganizar(a.v, a.datosOrganizar()); botonCon(p, "Parar") == nil ||
		!slices.Contains(textosDePantalla(p), "Describiendo 0 de 2…") {
		t.Errorf("mientras describe no se ve el progreso con «Parar»: %v", textosDePantalla(p))
	}
	a.pararOrganizar()
	for len(cola) > 0 {
		siguiente()
	}
	if n := len(pedidosQueEmpiezan(s, "POST /api/v1/assist")); n != 1 {
		t.Errorf("tras parar se hicieron %d llamadas a Sytue AI y solo iba una en vuelo", n)
	}
	if a.alb.org.fase != OrgInactiva || len(a.alb.org.propuesta) != 0 {
		t.Errorf("tras parar: %+v", a.alb.org)
	}

	// Y volver a empezar con la anterior aún en vuelo no deja dos pasadas a la vez.
	a.organizar()
	siguiente() // las fotos sin álbum de la segunda
	a.pararOrganizar()
	a.organizar()
	for len(cola) > 0 {
		siguiente()
	}
	if n := len(pedidosQueEmpiezan(s, "POST /api/v1/assist agrupar-fotos")); n != 1 {
		t.Errorf("con dos pasadas se agrupó %d veces y tocaba una", n)
	}
	if a.alb.org.fase != OrgPropuesta {
		t.Errorf("la última pasada acaba en %v", a.alb.org.fase)
	}
}

// TestPlanDeAlbumes: el mismo caso que `planDeAlbumes` de la web.
func TestPlanDeAlbumes(t *testing.T) {
	existentes := []api.Album{{ID: "v", Title: "Verano en Cádiz", Count: 3}, {ID: "n", Title: "Navidad", Count: 2}}
	plan := PlanDeAlbumes([]AlbumElegido{
		{Titulo: "  VERANO EN CADIZ ", IDs: []string{"1", "2"}, Incluido: true},
		{Titulo: "Montaña", IDs: []string{"3", "4"}, Incluido: true},
		{Titulo: "montana", IDs: []string{"4", "5"}, Incluido: true},
		{Titulo: "Navidad", IDs: []string{"6", "7"}, Incluido: false},
		{Titulo: "   ", IDs: []string{"8", "9"}, Incluido: true},
	}, existentes)
	got := make([]string, len(plan))
	for i, p := range plan {
		destino := "nuevo"
		if p.Existente != nil {
			destino = p.Existente.ID
		}
		got[i] = fmt.Sprintf("%s→%s %v", p.Titulo, destino, p.IDs)
	}
	if quiero := []string{"VERANO EN CADIZ→v [1 2]", "Montaña→nuevo [3 4 5]"}; !slices.Equal(got, quiero) {
		t.Errorf("plan %v; quiero %v", got, quiero)
	}
}

// TestLasCifradasNoSeOrganizan: la misma regla que describir (ADR 0086). Si una cifrada
// llegara a la lista, no entra; y con el Drive en la bóveda se dice con el aviso de la
// galería.
func TestLasCifradasNoSeOrganizan(t *testing.T) {
	s := nuevoServicioOrganizar()
	s.fotos = append(s.fotos, api.Photo{ID: idDeLinea("drive-1"), Description: "Cifrada"})
	a := appOrganizar(t, s)
	a.cif.enBoveda = true
	a.organizar()
	if strings.Contains(s.contexto, "Cifrada") || len(strings.Split(s.contexto, "\n")) != 5 {
		t.Errorf("una cifrada llegó a Sytue AI:\n%s", s.contexto)
	}
	if !slices.Contains(textosDePantalla(SeccionOrganizar(a.v, a.datosOrganizar())), a.v.Traduce("gallery.encrypted.note")) {
		t.Error("con el Drive en la bóveda no se dice que las cifradas no entran")
	}
}
