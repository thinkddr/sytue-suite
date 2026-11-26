// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package calendario

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/test"

	"git.sytue.eu/sytue/one/apps/nucleo/api"

	"git.sytue.eu/sytue/one/apps/fyne/iconos"
	"git.sytue.eu/sytue/one/apps/fyne/tema"
	"git.sytue.eu/sytue/one/apps/fyne/ui"
)

// escrituras es Sytue Calendar con un calendario y dos eventos —uno suelto y una serie
// semanal—, que apunta cada escritura con su cuerpo.
type escrituras struct {
	mu     sync.Mutex
	hechas []string
	cuerpo []map[string]any
	// huecos es la consulta de «buscar hueco», la última.
	huecos url.Values
}

func (s *escrituras) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	defer s.mu.Unlock()
	ruta := r.Method + " " + r.URL.Path
	switch ruta {
	case "GET /api/v1/calendars":
		_, _ = io.WriteString(w, `{"calendars":[{"id":"mio","display_name":"Personal","color":"#1e50a8","is_default":true}]}`)
		return
	case "GET /api/v1/calendars/shared-with-me":
		_, _ = io.WriteString(w, `{"calendars":[]}`)
		return
	case "GET /api/v1/calendars/mio/shares":
		_, _ = io.WriteString(w, `{"shares":[{"id":"s1","grantee_email":"ana@sytue.eu","permission":"writer"}]}`)
		return
	case "GET /api/v1/freebusy/suggest":
		s.huecos = r.URL.Query()
		_, _ = io.WriteString(w, `{"slots":[{"start":"2031-03-04T07:00:00Z","end":"2031-03-04T09:00:00Z"},{"start":"2031-03-05T13:00:00Z","end":"2031-03-05T14:00:00Z"}]}`)
		return
	case "GET /api/v1/invitations":
		_, _ = io.WriteString(w, `{"invitations":[{"id":"i1","uid":"u9","organizer":"mailto:ada@sytue.eu","status":"pending"}]}`)
		return
	case "GET /api/v1/events/e1":
		_, _ = io.WriteString(w, `{"id":"e1","uid":"u1","summary":"Comité","reminders":[10,45]}`)
		return
	case "GET /api/v1/events/s1":
		// El maestro de la serie, con SU inicio: en /instances cada ocurrencia trae el
		// suyo, como en el servicio de verdad (`domain.ExpandEvents`).
		_, _ = io.WriteString(w, `{"id":"s1","uid":"us","summary":"Diario","rrule":"FREQ=WEEKLY;COUNT=10","dtstart":"2026-09-01T07:00:00Z","dtend":"2026-09-01T07:30:00Z"}`)
		return
	case "GET /api/v1/resources":
		// r1 libre y r2 ocupada de 09:30 a 10:30 del 22.
		_, _ = io.WriteString(w, `{"resources":[
			{"id":"r1","kind":"room","name":"Sala Curie","capacity":8,"location":"Planta 2","auto_accept":true,"active":true,"busy":[]},
			{"id":"r2","kind":"equipment","name":"Proyector","capacity":0,"auto_accept":true,"active":true,
			 "busy":[{"start":"2026-09-22T09:30:00Z","end":"2026-09-22T10:30:00Z"}]}]}`)
		return
	case "GET /api/v1/calendars/mio/instances":
		_, _ = io.WriteString(w, `{"instances":[
			{"event":{"id":"e1","uid":"u1","summary":"Comité","location":"Sala 2","dtstart":"2026-09-10T08:00:00Z","dtend":"2026-09-10T09:00:00Z","attendees":[{"email":"yo@sytue.eu","partstat":"NEEDS-ACTION"}]},"start":"2026-09-10T08:00:00Z","end":"2026-09-10T09:00:00Z"},
			{"event":{"id":"s1","uid":"us","summary":"Diario","rrule":"FREQ=WEEKLY;COUNT=10","dtstart":"2026-09-15T07:00:00Z","dtend":"2026-09-15T07:30:00Z"},"start":"2026-09-15T07:00:00Z","end":"2026-09-15T07:30:00Z"}]}`)
		return
	}
	var c map[string]any
	if datos, _ := io.ReadAll(r.Body); len(datos) > 0 {
		_ = json.Unmarshal(datos, &c)
	}
	s.hechas, s.cuerpo = append(s.hechas, ruta), append(s.cuerpo, c)
	switch {
	case ruta == "POST /api/v1/resources/r2/book":
		_, _ = io.WriteString(w, `{"status":"declined"}`)
	case strings.HasSuffix(ruta, "/book"):
		_, _ = io.WriteString(w, `{"status":"accepted"}`)
	default:
		_, _ = io.WriteString(w, `{}`)
	}
}

func (s *escrituras) apuntadas() ([]string, []map[string]any) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]string(nil), s.hechas...), append([]map[string]any(nil), s.cuerpo...)
}

// appConVentana monta la App en una ventana de 1280 × 744, con las cargas en línea.
func appConVentana(t *testing.T, s http.Handler) (*App, fyne.Window) {
	t.Helper()
	srv := httptest.NewServer(s)
	t.Cleanup(srv.Close)
	p := &Pantalla{T: traductor(t), Cursor: time.Date(2026, 9, 21, 12, 0, 0, 0, time.UTC), Zona: time.UTC}
	a := NuevaApp(p, api.NewCalendars(srv.URL, nil, nil))
	a.lanzar = func(f func(context.Context)) bool { f(a.vida); return true }
	a.enUI = func(f func()) { f() }
	w := test.NewWindow(a.Objeto())
	t.Cleanup(w.Close)
	t.Cleanup(func() {
		for _, aviso := range a.avisos.Avisos() {
			a.avisos.Descartar(aviso)
		}
	})
	w.SetPadded(false)
	w.Resize(fyne.NewSize(ventana, altoUtil))
	a.Arrancar()
	return a, w
}

// buscarBoton recorre el árbol hasta dar con el botón de ese rótulo.
func buscarBoton(o fyne.CanvasObject, rotulo string) *ui.Boton {
	switch v := o.(type) {
	case *ui.Boton:
		if v.Texto == rotulo {
			return v
		}
	case *fyne.Container:
		for _, h := range v.Objects {
			if b := buscarBoton(h, rotulo); b != nil {
				return b
			}
		}
		return nil
	}
	if w, ok := o.(fyne.Widget); ok {
		for _, h := range test.WidgetRenderer(w).Objects() {
			if b := buscarBoton(h, rotulo); b != nil {
				return b
			}
		}
	}
	return nil
}

func ocurrenciaDe(t *testing.T, a *App, id string) Ocurrencia {
	t.Helper()
	for _, o := range a.P.Eventos {
		if o.Event.ID == id {
			return o
		}
	}
	t.Fatalf("no está la ocurrencia %s entre %d", id, len(a.P.Eventos))
	return Ocurrencia{}
}

// TestLaVistaRapidaSeColocaComoEventPopover: debajo del evento a 6 px y alineada a su
// izquierda; encima si no cabe debajo; y acotada a la ventana con 8 px.
func TestLaVistaRapidaSeColocaComoEventPopover(t *testing.T) {
	ventanaP := fyne.NewSize(1280, 744)
	panel := fyne.NewSize(252, 200)
	evento := fyne.NewSize(140, 18)
	if got := ColocarVistaRapida(fyne.NewPos(400, 100), evento, panel, ventanaP); got != fyne.NewPos(400, 124) {
		t.Errorf("debajo: %v", got)
	}
	if got := ColocarVistaRapida(fyne.NewPos(400, 600), evento, panel, ventanaP); got != fyne.NewPos(400, 394) {
		t.Errorf("encima: %v", got)
	}
	if got := ColocarVistaRapida(fyne.NewPos(1200, 100), evento, panel, ventanaP); got.X != 1280-8-252 {
		t.Errorf("acotada a la derecha: %v", got)
	}
}

// TestLaVistaRapidaAbreSeAnclaYElimina: pulsar el evento abre el panel de 252 junto a
// él, con «¿Vas a ir?» porque me han invitado, y «Eliminar» borra y recarga.
func TestLaVistaRapidaAbreSeAnclaYElimina(t *testing.T) {
	s := &escrituras{}
	a, w := appConVentana(t, s)
	a.Correo = "yo@sytue.eu"
	o := ocurrenciaDe(t, a, "e1")
	ancla := container.NewWithoutLayout()
	ancla.Move(fyne.NewPos(700, 200))
	ancla.Resize(fyne.NewSize(140, 18))
	a.raiz.Add(container.NewWithoutLayout(ancla)) // la envoltura se estira; el ancla, no
	a.abrirVistaRapida(o, ancla)
	capas := w.Canvas().Overlays().List()
	if len(capas) != 1 || a.vista == nil {
		t.Fatalf("hay %d capas", len(capas))
	}
	panel := a.vista
	if got := panel.Size().Width; got != AnchoVistaRapida {
		t.Errorf("el panel mide %v y `w-72` son 252", got)
	}
	if got := panel.Position(); got.X != 700 || got.Y != 200+18+SeparacionVistaRapida {
		t.Errorf("el panel está en %v y no bajo el evento", got)
	}
	img := w.Canvas().Capture()
	if got, quiero := px(img, 700+240, int(panel.Position().Y)+60), tema.Color("surface-raised", tema.Claro); !mismoColor(got, quiero) {
		t.Errorf("el fondo del panel es %v y `surface-raised` es %v", got, quiero)
	}
	guardar(t, img, "calendario-vista-rapida")
	if buscarBoton(panel, "Sí") == nil {
		t.Error("me han invitado y no sale «¿Vas a ir?»")
	}
	borrar := buscarBoton(panel, "Eliminar")
	if borrar == nil {
		t.Fatal("no está «Eliminar»")
	}
	borrar.Tapped(nil)
	if hechas, _ := s.apuntadas(); len(hechas) != 1 || hechas[0] != "DELETE /api/v1/events/e1" {
		t.Errorf("eliminar hizo %v", hechas)
	}
	if len(w.Canvas().Overlays().List()) != 0 {
		t.Error("la vista rápida sigue puesta tras eliminar")
	}
	if len(a.avisos.Avisos()) != 1 {
		t.Errorf("hay %d avisos y tocaba «Evento eliminado»", len(a.avisos.Avisos()))
	}
}

// TestLaHojaRapidaDelTelefonoNoAbreConEliminarRojo: en el teléfono la vista rápida es
// una hoja, y su pie pone sola a todo el ancho la primera de tres acciones. «Eliminar»
// iba ahí, en el rojo macizo de Peligro: lo primero que se veía al tocar un evento
// (auditoría del 2026-11-24). Ahora el pie es Duplicar | Editar y «Eliminar» es un
// fantasma de peligro a su ancho al final del cuerpo, que sigue borrando.
// VISTO EN ROJO con el pie de antes.
func TestLaHojaRapidaDelTelefonoNoAbreConEliminarRojo(t *testing.T) {
	s := &escrituras{}
	a, w := appConVentana(t, s)
	w.Resize(fyne.NewSize(390, 844))
	o := ocurrenciaDe(t, a, "e1")
	ancla := container.NewWithoutLayout()
	a.raiz.Add(container.NewWithoutLayout(ancla))
	a.abrirVistaRapida(o, ancla)
	capas := w.Canvas().Overlays().List()
	if len(capas) != 1 {
		t.Fatalf("hay %d capas y tocaba la hoja", len(capas))
	}
	guardar(t, w.Canvas().Capture(), "calendario-hoja-rapida-telefono")
	borrar := buscarBoton(capas[0], "Eliminar")
	if borrar == nil {
		t.Fatal("no está «Eliminar»")
	}
	if borrar.Variante != ui.Fantasma || !borrar.Destructivo {
		t.Errorf("«Eliminar» es de la variante %v (destructivo %v) y tocaba el fantasma de peligro",
			borrar.Variante, borrar.Destructivo)
	}
	if ancho := borrar.Size().Width; ancho > 390/2 {
		t.Errorf("«Eliminar» mide %v de ancho: sigue a todo el ancho de la hoja", ancho)
	}
	editar := buscarBoton(capas[0], "Editar")
	if editar == nil || editar.Variante != ui.Primario {
		t.Fatal("falta «Editar» como acción principal")
	}
	if editar.Position().Y < 0 || borrar.Size().Height <= 0 {
		t.Error("las acciones no se han colocado")
	}
	borrar.Tapped(nil)
	if hechas, _ := s.apuntadas(); len(hechas) != 1 || hechas[0] != "DELETE /api/v1/events/e1" {
		t.Errorf("eliminar hizo %v", hechas)
	}
}

// TestElFormularioMovilDejaVerLasAccionesSinTeclado abre el modal de edición completo
// en el tamaño de un iPhone SE. El cuerpo se desplaza; los campos y las tres acciones
// quedan dentro del ancho, y el pie sigue a la vista con el teclado cerrado.
func TestElFormularioMovilDejaVerLasAccionesSinTeclado(t *testing.T) {
	p := &Pantalla{T: traductor(t), Zona: time.UTC}
	inicio := time.Date(2026, 9, 22, 10, 0, 0, 0, time.UTC)
	for _, ancho := range []float32{320, 375, 390} {
		w := test.NewWindow(canvas.NewRectangle(tema.Color("surface", tema.Claro)))
		w.SetPadded(false)
		w.Resize(fyne.NewSize(ancho, 568))
		f := p.nuevoFormulario(Edicion{
			O: &Ocurrencia{}, Titulo: "Reunión", Inicio: inicio, Fin: inicio.Add(time.Hour),
			Invitados: []string{"ana@sytue.eu", "grace@sytue.eu", "persona.con.un.correo.largo@example.eu"},
		}, []api.Calendar{{ID: "mio", DisplayName: "Personal"}}, AccionesFormulario{})
		// El driver de software no abre teclado; LG reproduce el mínimo de44 de iOS.
		for _, b := range []*ui.Boton{f.eliminar, f.cancelar, f.guardar} {
			b.Talla = ui.TallaLG
		}
		f.d.Mostrar(w.Canvas())
		abs := fyne.CurrentApp().Driver().AbsolutePositionForObject
		for _, b := range []*ui.Boton{f.eliminar, f.cancelar, f.guardar} {
			pos, tam := abs(b), b.Size()
			if pos.X < 0 || pos.Y < 0 || pos.X+tam.Width > ancho+0.5 || pos.Y+tam.Height > 568.5 {
				t.Errorf("a %g sin teclado: %q queda en%v/%v fuera de la ventana", ancho, b.Texto, pos, tam)
			}
		}
		for _, campo := range []fyne.CanvasObject{f.titulo, f.inicio, f.fin, f.lugar, f.correo, f.zona, f.repetir} {
			pos := abs(campo)
			if pos.X < 0 || pos.X+campo.Size().Width > ancho+0.5 {
				t.Errorf("a %g sin teclado: %T queda enX%g con ancho%g", ancho, campo, pos.X, campo.Size().Width)
			}
		}
		recorrer(f.cuerpo, func(o fyne.CanvasObject) {
			if g, ok := o.(*grupoMuestras); ok {
				for _, m := range g.muestras {
					if m.Position().Y+m.Size().Height > g.Size().Height+0.5 {
						t.Errorf("a %g: las muestras de color se pintan fuera del alto reservado", ancho)
					}
				}
			}
			if e, ok := o.(*ui.Etiqueta); ok {
				if a := e.Aspa(); a != nil && a.Position().X+a.Size().Width > e.Size().Width+0.5 {
					t.Errorf("a %g: el correo largo deja «Quitar invitado» fuera de su píldora", ancho)
				}
			}
		})
		guardar(t, w.Canvas().Capture(), "calendario-formulario-movil-"+contar(int(ancho)))
		f.d.Cerrar()
		w.Close()
	}
}

// TestElFormularioValidaYCreaConLoQueManda: sin título dice «El título es obligatorio»
// junto al campo y no guarda; con título crea en el calendario por defecto, con la hora
// de pared en su zona, la regla y los invitados, y pide que se entreguen.
func TestElFormularioValidaYCreaConLoQueManda(t *testing.T) {
	s := &escrituras{}
	a, w := appConVentana(t, s)
	madrid, err := time.LoadLocation("Europe/Madrid")
	if err != nil {
		t.Skip("sin tzdata")
	}
	inicio := time.Date(2026, 9, 22, 10, 0, 0, 0, time.UTC)
	f := a.P.nuevoFormulario(Edicion{Inicio: inicio, Fin: inicio.Add(time.Hour), Zona: "Europe/Madrid"}, a.destinos(), AccionesFormulario{
		Guardar: a.guardar, Aviso: func(string) {},
	})
	f.d.Mostrar(w.Canvas())
	guardar(t, w.Canvas().Capture(), "calendario-formulario")

	f.enviar()
	if f.errTitulo == "" {
		t.Fatal("sin título no avisa junto al campo")
	}
	if hechas, _ := s.apuntadas(); len(hechas) != 0 {
		t.Fatalf("sin título guardó: %v", hechas)
	}
	f.titulo.PonerTexto("Gestoría")
	f.e.Regla = &Recurrencia{Freq: "WEEKLY", Cuenta: 4}
	f.correo.PonerTexto("ana@sytue.eu")
	f.enviar()
	hechas, cuerpos := s.apuntadas()
	if len(hechas) != 1 || hechas[0] != "POST /api/v1/calendars/mio/events" {
		t.Fatalf("guardar hizo %v", hechas)
	}
	c := cuerpos[0]
	quiero := time.Date(2026, 9, 22, 10, 0, 0, 0, madrid).Format(time.RFC3339)
	if c["summary"] != "Gestoría" || c["tzid"] != "Europe/Madrid" || c["rrule"] != "FREQ=WEEKLY;COUNT=4" || c["send"] != true {
		t.Errorf("cuerpo del alta: %v", c)
	}
	if got, _ := time.Parse(time.RFC3339, c["start"].(string)); got.Format(time.RFC3339) != quiero {
		t.Errorf("el inicio es %v y la hora de pared en Madrid es %s", c["start"], quiero)
	}
	if at, _ := c["attendees"].([]any); len(at) != 1 || at[0].(map[string]any)["email"] != "ana@sytue.eu" {
		t.Errorf("invitados: %v", c["attendees"])
	}
	if len(w.Canvas().Overlays().List()) != 0 {
		t.Error("el formulario sigue abierto tras guardar")
	}
}

// TestUnaSerieSeParteSegunElAlcance es applyRecurringEdit/Delete por REST: «solo este»
// crea el suelto y excluye la ocurrencia; «este y los siguientes» crea la serie nueva
// con lo que le queda de COUNT y corta el maestro; «todos» sustituye el maestro con el
// inicio DESPLAZADO lo mismo que se movió la ocurrencia.
func TestUnaSerieSeParteSegunElAlcance(t *testing.T) {
	for _, c := range []struct {
		alcance string
		quiero  []string
	}{
		{AlcanceEste, []string{"POST /api/v1/calendars/mio/events", "POST /api/v1/calendars/mio/amend-recurrence"}},
		{AlcanceSiguientes, []string{"POST /api/v1/calendars/mio/events", "POST /api/v1/calendars/mio/amend-recurrence"}},
		{AlcanceTodos, []string{"PUT /api/v1/events/s1"}},
	} {
		s := &escrituras{}
		a, _ := appConVentana(t, s)
		o := ocurrenciaDe(t, a, "s1")
		in := o.Event.Input()
		in.Start, in.End = o.Start.Add(time.Hour), o.End.Add(time.Hour)
		regla, _ := LeerRRule(o.Event.RRule)
		if err := EditarSerie(context.Background(), a.Cal, o, c.alcance, in, &regla); err != nil {
			t.Fatalf("%s: %v", c.alcance, err)
		}
		hechas, cuerpos := s.apuntadas()
		if strings.Join(hechas, " | ") != strings.Join(c.quiero, " | ") {
			t.Errorf("%s hizo %v", c.alcance, hechas)
			continue
		}
		switch c.alcance {
		case AlcanceEste:
			if cuerpos[0]["rrule"] != nil || cuerpos[1]["mode"] != "exclude" || cuerpos[1]["recurrence_id"] != "2026-09-15T07:00:00Z" {
				t.Errorf("solo este: %v", cuerpos)
			}
		case AlcanceSiguientes:
			// Del 1 al 15 de septiembre caben dos semanas enteras: quedan 8 de 10.
			if cuerpos[0]["rrule"] != "FREQ=WEEKLY;COUNT=8" || cuerpos[1]["mode"] != "until" {
				t.Errorf("siguientes: %v", cuerpos)
			}
		case AlcanceTodos:
			if cuerpos[0]["start"] != "2026-09-01T08:00:00Z" || cuerpos[0]["rrule"] != "FREQ=WEEKLY;COUNT=10" {
				t.Errorf("todos: %v", cuerpos)
			}
		}
	}
	s := &escrituras{}
	a, _ := appConVentana(t, s)
	if err := BorrarSerie(context.Background(), a.Cal, ocurrenciaDe(t, a, "s1"), AlcanceTodos); err != nil {
		t.Fatal(err)
	}
	if hechas, _ := s.apuntadas(); len(hechas) != 1 || hechas[0] != "DELETE /api/v1/events/s1" {
		t.Errorf("borrar todos hizo %v", hechas)
	}
}

// TestLaReglaEsLaDeIcalTs: lo que la interfaz sabe leer y escribir, y lo que no (los
// BY* que no son el BYDAY de una semanal). Los vectores de BYDAY y YEARLY son los de
// «Repetir: cada año, cada día laborable y ciertos días» en ical.test.ts.
func TestLaReglaEsLaDeIcalTs(t *testing.T) {
	for _, s := range []string{
		"FREQ=DAILY", "FREQ=WEEKLY;INTERVAL=2;COUNT=5", "FREQ=MONTHLY;UNTIL=20261231T235959Z",
		"FREQ=WEEKLY;BYDAY=MO,TU,WE,TH,FR", "FREQ=WEEKLY;INTERVAL=2;BYDAY=MO,WE;COUNT=6",
		"FREQ=YEARLY", "FREQ=YEARLY;UNTIL=20301231T235959Z",
	} {
		r, ok := LeerRRule(s)
		if !ok || r.RRule() != s {
			t.Errorf("%q vuelve como %q (ok=%v)", s, r.RRule(), ok)
		}
	}
	if r, _ := LeerRRule("FREQ=WEEKLY;BYDAY=MO,TU,WE,TH,FR"); !r.EsLaborable() {
		t.Error("lunes a viernes no se reconoce como «cada día laborable»")
	}
	if r, ok := LeerRRule("FREQ=WEEKLY;BYDAY=we,MO;WKST=MO"); !ok || !slices.Equal(r.Dias, []string{"MO", "WE"}) {
		t.Errorf("los días no salen ordenados de lunes a domingo: %v", r.Dias)
	}
	if got := (Recurrencia{Freq: "WEEKLY", Dias: []string{"FR", "MO", "FR", "XX"}}).RRule(); got != "FREQ=WEEKLY;BYDAY=MO,FR" {
		t.Errorf("BYDAY sale %q", got)
	}
	if got := (Recurrencia{Freq: "DAILY", Dias: []string{"MO"}}).RRule(); got != "FREQ=DAILY" {
		t.Errorf("BYDAY solo acompaña a una semanal y sale %q", got)
	}
	for _, s := range []string{
		"FREQ=MONTHLY;BYDAY=2MO", "FREQ=WEEKLY;BYDAY=1MO", "FREQ=WEEKLY;BYDAY=MO;BYMONTH=3",
		"FREQ=DAILY;BYDAY=MO,TU", "FREQ=YEARLY;BYMONTH=3;BYMONTHDAY=8",
		"FREQ=WEEKLY;INTERVAL=2;BYDAY=MO;WKST=SU", "FREQ=HOURLY", "",
	} {
		if _, ok := LeerRRule(s); ok {
			t.Errorf("%q no debería leerse: la web la trata como personalizada", s)
		}
	}
}

// TestRecurrenciaCuentaLosDiasAlPartir es `countBefore`/`carryRule` con BYDAY y YEARLY:
// partir «este y los siguientes» descuenta cada día elegido, no una por semana.
func TestRecurrenciaCuentaLosDiasAlPartir(t *testing.T) {
	inicio := time.Date(2026, 7, 1, 9, 0, 0, 0, time.UTC) // miércoles
	corte := time.Date(2026, 7, 15, 9, 0, 0, 0, time.UTC)
	for _, c := range []struct {
		r     Recurrencia
		corte time.Time
		n     int
	}{
		{Recurrencia{Freq: "WEEKLY"}, corte, 2},
		{Recurrencia{Freq: "WEEKLY", Dias: []string{"MO", "WE"}}, corte, 4},
		{Recurrencia{Freq: "WEEKLY", Dias: Laborables}, corte, 10},
		{Recurrencia{Freq: "WEEKLY", Intervalo: 2, Dias: []string{"MO", "WE"}}, corte, 2},
		{Recurrencia{Freq: "YEARLY"}, time.Date(2028, 8, 1, 9, 0, 0, 0, time.UTC), 3},
	} {
		if got := contarAntes(inicio, c.r, c.corte); got != c.n {
			t.Errorf("%s: %d antes del corte, y son %d", c.r.RRule(), got, c.n)
		}
	}
	nueva := arrastrarRegla(Recurrencia{Freq: "WEEKLY", Dias: []string{"MO", "WE"}, Cuenta: 10}, inicio, corte)
	if nueva.RRule() != "FREQ=WEEKLY;BYDAY=MO,WE;COUNT=6" {
		t.Errorf("la serie nueva es %q", nueva.RRule())
	}
	// Lunes a las 00:30 en Madrid, que en UTC es domingo: los días se cuentan en la zona
	// del evento, como los expande el servicio. En UTC el primer lunes se perdía.
	madrid, err := time.LoadLocation("Europe/Madrid")
	if err != nil {
		t.Skip(err)
	}
	lunes := time.Date(2026, 7, 6, 0, 30, 0, 0, madrid)
	maestro := api.Event{DTStart: lunes.UTC(), TZID: "Europe/Madrid"}
	viernes := time.Date(2026, 7, 10, 0, 30, 0, 0, madrid)
	r := Recurrencia{Freq: "WEEKLY", Dias: Laborables, Cuenta: 10}
	if got := arrastrarRegla(r, inicioEnSuZona(maestro), viernes).Cuenta; got != 6 {
		t.Errorf("partida el viernes quedan %d, y son 6 (de lunes a jueves van 4)", got)
	}
}

// TestElCuandoEsElDeFormatWhen, en español.
func TestElCuandoEsElDeFormatWhen(t *testing.T) {
	p := &Pantalla{T: traductor(t), Zona: time.UTC}
	d := func(dia, h, m int) time.Time { return time.Date(2026, 9, dia, h, m, 0, 0, time.UTC) }
	for _, c := range []struct {
		ini, fin  time.Time
		todoElDia bool
		quiero    string
	}{
		{d(22, 10, 0), d(22, 11, 30), false, "22 de septiembre, 10:00 – 11:30"},
		{d(22, 0, 0), d(23, 0, 0), true, "Todo el día · 22 de septiembre"},
	} {
		if got := p.Cuando(c.ini, c.fin, c.todoElDia); got != c.quiero {
			t.Errorf("Cuando = %q y la web da %q", got, c.quiero)
		}
	}
}

// TestElAlcanceArrancaEnSoloEste: la opción menos destructiva es la de partida, y la
// que se elige es la que llega.
func TestElAlcanceArrancaEnSoloEste(t *testing.T) {
	p := &Pantalla{T: traductor(t)}
	w := test.NewWindow(container.NewWithoutLayout())
	defer w.Close()
	w.Resize(fyne.NewSize(1280, 744))
	var elegido string
	d := p.DialogoAlcance(true, func(a string) { elegido = a }, func() {})
	d.Mostrar(w.Canvas())
	if b := buscarBoton(d, "Eliminar"); b != nil {
		b.Tapped(nil)
	}
	if elegido != AlcanceEste {
		t.Errorf("sin tocar nada llega %q", elegido)
	}
}

// TestLosRecordatoriosSeLeenSeEditanYViajan es el bloque «Recordatorios» de EventModal:
// al editar se leen los del evento (GET /events/{id}) y cada uno sale en su fila —el
// preset en el selector; el personalizado con su cantidad y su unidad—; cambiar, añadir
// y quitar viaja ENTERO en el PUT. Un evento nuevo empieza sin ninguno, y uno cuyos
// recordatorios no se pudieron leer no enseña el bloque ni los manda (se conservan).
func TestLosRecordatoriosSeLeenSeEditanYViajan(t *testing.T) {
	s := &escrituras{}
	a, w := appConVentana(t, s)
	e := a.P.EdicionDe(ocurrenciaDe(t, a, "e1"))
	f := a.P.nuevoFormulario(e, a.destinos(), a.accionesFormulario(e))
	if f.e.Recordatorios == nil || !slices.Equal(*f.e.Recordatorios, []int{10, 45}) {
		t.Fatalf("no se han leído los recordatorios del evento: %v", f.e.Recordatorios)
	}
	if len(f.filasRec) != 2 || f.filasRec[0].eleccion.Valor() != "10 minutos antes" ||
		f.filasRec[1].eleccion.Valor() != "Personalizado…" || f.filasRec[1].cantidad.Texto() != "45" ||
		f.filasRec[1].unidad.Valor() != "minutos antes" {
		t.Fatalf("las filas no son las de la web: %+v", f.filasRec)
	}
	f.d.Mostrar(w.Canvas())
	guardar(t, w.Canvas().Capture(), "calendario-formulario-recordatorios")
	// Lo medido en la web (1280, diálogo md): el selector de un preset se queda con la
	// fila menos el aspa (470); el de un personalizado, con lo que dejan la cantidad (80)
	// y la unidad (139); filas cada 36 + 8 y añadir a 94 del primer selector.
	// Medido en EventModal real con raíz de 16 el 2026-10-04.
	casi := func(a, b float32) bool { return a-b < 1 && b-a < 1 }
	abs := func(o fyne.CanvasObject) fyne.Position {
		return fyne.CurrentApp().Driver().AbsolutePositionForObject(o)
	}
	f0, f1 := f.filasRec[0], f.filasRec[1]
	if !casi(f0.eleccion.Size().Width, 470) || !casi(f0.quitar.Position().X, 478) ||
		!casi(f1.eleccion.Size().Width, 235) || !casi(f1.cantidad.Size().Width, 80) ||
		!casi(f1.unidad.Size().Width, 139) || !casi(f1.quitar.Position().X, 478) || !casi(f.anadirRec.Size().Width, 510) {
		t.Errorf("anchos: preset %v, aspa en %v; personalizado %v + %v + %v, aspa en %v; añadir %v",
			f0.eleccion.Size().Width, f0.quitar.Position().X, f1.eleccion.Size().Width, f1.cantidad.Size().Width,
			f1.unidad.Size().Width, f1.quitar.Position().X, f.anadirRec.Size().Width)
	}
	if d := abs(f1.eleccion).Y - abs(f0.eleccion).Y; !casi(d, 44) {
		t.Errorf("las filas van cada %v y en la web cada 44", d)
	}
	if d := abs(f.anadirRec).Y - abs(f0.eleccion).Y; !casi(d, 94) {
		t.Errorf("«Añadir» está a %v del principio de la lista y en la web a 94", d)
	}

	// 45 minutos pasan a horas: 45 horas; teclear 3 son 180 minutos, que siguen siendo
	// «personalizado» (3 horas).
	f.filasRec[1].unidad.AlElegir(1)
	f.filasRec[1].cantidad.PonerTexto("3")
	if got := *f.e.Recordatorios; !slices.Equal(got, []int{10, 180}) {
		t.Fatalf("tras cambiar la unidad y la cantidad: %v", got)
	}
	f.anadirRec.Tapped(nil)
	f.filasRec[0].quitar.Tapped(nil)
	f.filasRec[1].eleccion.AlElegir(7) // «1 día antes»
	if got := *f.e.Recordatorios; !slices.Equal(got, []int{180, 1440}) {
		t.Fatalf("tras añadir, quitar y elegir: %v", got)
	}
	f.enviar()
	hechas, cuerpos := s.apuntadas()
	if len(hechas) != 1 || hechas[0] != "PUT /api/v1/events/e1" {
		t.Fatalf("guardar hizo %v", hechas)
	}
	if got, _ := cuerpos[0]["reminders"].([]any); len(got) != 2 || got[0] != 180.0 || got[1] != 1440.0 {
		t.Fatalf("los recordatorios del PUT: %#v", cuerpos[0]["reminders"])
	}

	// Nuevo: «Sin recordatorios», y el alta los manda vacíos.
	nuevo := Edicion{Inicio: time.Date(2026, 9, 22, 10, 0, 0, 0, time.UTC), Fin: time.Date(2026, 9, 22, 11, 0, 0, 0, time.UTC)}
	fn := a.P.nuevoFormulario(nuevo, a.destinos(), a.accionesFormulario(nuevo))
	if fn.bloqueRecordatorios() == nil || fn.e.Recordatorios == nil || len(*fn.e.Recordatorios) != 0 {
		t.Fatal("un evento nuevo no enseña el bloque vacío")
	}
	if in := fn.e.EntradaDe(a.P); in.Reminders == nil || len(*in.Reminders) != 0 {
		t.Fatalf("el alta tiene que mandar la lista vacía: %v", in.Reminders)
	}

	// La serie no trae recordatorios en su respuesta (el falso no los manda): no se
	// sabe cuáles tiene, así que ni se pinta el bloque ni viajan.
	es := a.P.EdicionDe(ocurrenciaDe(t, a, "s1"))
	fs := a.P.nuevoFormulario(es, a.destinos(), a.accionesFormulario(es))
	if fs.e.Recordatorios != nil || fs.bloqueRecordatorios() != nil {
		t.Fatalf("sin saberlos se ofrecen igual: %v", fs.e.Recordatorios)
	}
	if in := fs.e.EntradaDe(a.P); in.Reminders != nil {
		t.Fatalf("sin saberlos se mandan y borrarían los del evento: %v", *in.Reminders)
	}
}

// TestLasSalasSeEligenYSeReservanAlGuardar es ResourcePicker dentro del formulario: los
// recursos con LIBRE/OCUPADO en la franja del evento, añadir desde el selector, quitar
// con el aspa y, al guardar, una reserva por recurso con el UID del evento; el aviso
// resume cuántas quedaron y cuántas estaban cogidas.
func TestLasSalasSeEligenYSeReservanAlGuardar(t *testing.T) {
	s := &escrituras{}
	a, w := appConVentana(t, s)
	inicio := time.Date(2026, 9, 22, 10, 0, 0, 0, time.UTC)
	e := Edicion{Inicio: inicio, Fin: inicio.Add(time.Hour), Zona: "UTC"}
	f := a.P.nuevoFormulario(e, a.destinos(), a.accionesFormulario(e))
	if f.rec == nil || len(f.rec.lista) != 2 || f.bloqueRecursos() == nil {
		t.Fatalf("no se han cargado los recursos: %+v", f.rec)
	}
	f.bloqueRecursos()
	if got := f.rec.anadir.Opciones; len(got) != 3 || got[0] != "Añadir sala o recurso…" ||
		got[1] != "Sala Curie · 8 personas — libre" || got[2] != "Proyector — ocupado" {
		t.Fatalf("opciones: %q", got)
	}
	f.rec.anadir.AlElegir(1)
	f.rec.anadir.AlElegir(1) // ahora la primera ofrecida es el proyector
	if !slices.Equal(f.e.Recursos, []string{"r1", "r2"}) {
		t.Fatalf("elegidos: %v", f.e.Recursos)
	}
	// Una ventana alta para ver el formulario ENTERO, con las salas y los recordatorios.
	w.Resize(fyne.NewSize(ventana, 1700))
	f.d.Mostrar(w.Canvas())
	guardar(t, w.Canvas().Capture(), "calendario-formulario-salas")
	w.Resize(fyne.NewSize(ventana, altoUtil))

	f.titulo.PonerTexto("Comité")
	f.enviar()
	hechas, cuerpos := s.apuntadas()
	if len(hechas) != 3 || hechas[0] != "POST /api/v1/calendars/mio/events" ||
		hechas[1] != "POST /api/v1/resources/r1/book" || hechas[2] != "POST /api/v1/resources/r2/book" {
		t.Fatalf("guardar hizo %v", hechas)
	}
	uid, _ := cuerpos[0]["uid"].(string)
	if uid == "" || cuerpos[1]["uid"] != uid || cuerpos[1]["summary"] != "Comité" || cuerpos[1]["tzid"] != "UTC" ||
		cuerpos[1]["start"] != "2026-09-22T10:00:00Z" {
		t.Fatalf("la reserva no va ligada al evento: evento %v, reserva %v", cuerpos[0]["uid"], cuerpos[1])
	}
	avisos := a.avisos.Avisos()
	if len(avisos) != 1 || avisos[0].Titulo != "Evento guardado · 1 recurso reservado, 1 ocupado" || avisos[0].Tono != ui.AvisoAdvertencia {
		t.Fatalf("aviso: %+v", avisos)
	}
}

// TestLaGestionDeRecursosDaDeAltaEditaYDaDeBaja es ResourcesModal: el catálogo, el alta
// con su validación, la edición (PATCH, sin la clase) y la baja.
func TestLaGestionDeRecursosDaDeAltaEditaYDaDeBaja(t *testing.T) {
	s := &escrituras{}
	a, w := appConVentana(t, s)
	w.Resize(fyne.NewSize(1280, 800))
	g := a.nuevoGestorRecursos()
	g.d.Mostrar(w.Canvas())
	g.cargar()
	if len(g.lista) != 2 || g.cargando {
		t.Fatalf("catálogo: %+v", g.lista)
	}
	guardar(t, w.Canvas().Capture(), "calendario-recursos")
	// ResourcesModal real, 1280 × 800 y raíz de 16, medido el 2026-10-04:
	// panel en (280,54.40625), nombre a 497.375 del borde; aforo y ubicación
	// en columnas de 329 desde x=25 y x=366; correo a 702.875 del borde. Con el
	// diálogo del 2026-10-15 el panel sigue en el tope y lo de dentro sube 20: la
	// cabecera mide 61 (no 73) y el cuerpo empieza con `pt-3` (no `pt-5`).
	abs := fyne.CurrentApp().Driver().AbsolutePositionForObject
	panel := abs(g.d)
	casi := func(a, b float32) bool { return a-b < 1 && b-a < 1 }
	if d := abs(g.nombre).Y - panel.Y; !casi(d, 477.375) {
		t.Errorf("el nombre está a %v del panel y en la web a 477,375", d)
	}
	if x, x2, an := abs(g.aforo).X-panel.X, abs(g.lugar).X-panel.X, g.aforo.Size().Width; !casi(x, 25) || !casi(x2, 366) || !casi(an, 329) {
		t.Errorf("aforo en %v (%v de ancho) y ubicación en %v; en la web 25, 329 y 366", x, an, x2)
	}
	if d := abs(g.correo).Y - panel.Y; !casi(d, 682.875) {
		t.Errorf("el correo está a %v del panel y en la web a 682,875", d)
	}

	g.enviar()
	if g.errNombre != "El nombre es obligatorio." {
		t.Fatalf("sin nombre no avisa: %q", g.errNombre)
	}
	g.clase = api.RecursoEquipo
	g.nombre.PonerTexto("  Portátil de préstamo ")
	g.aforo.PonerTexto("x")
	g.enviar()
	g.editar(g.lista[0])
	g.aforo.PonerTexto("12")
	g.enviar()
	g.quitar(g.lista[1])
	hechas, cuerpos := s.apuntadas()
	if !slices.Equal(hechas, []string{"POST /api/v1/resources", "PATCH /api/v1/resources/r1", "DELETE /api/v1/resources/r2"}) {
		t.Fatalf("la gestión hizo %v", hechas)
	}
	if c := cuerpos[0]; c["kind"] != "equipment" || c["name"] != "Portátil de préstamo" || c["capacity"] != 0.0 || c["auto_accept"] != true {
		t.Errorf("alta: %v", c)
	}
	if c := cuerpos[1]; c["kind"] != nil || c["capacity"] != 12.0 || c["location"] != "Planta 2" {
		t.Errorf("edición: %v", c)
	}
	if len(g.lista) != 1 || g.id != "" {
		t.Errorf("tras la baja quedan %d y el formulario edita %q", len(g.lista), g.id)
	}
}

// buscarEnlace recorre el árbol hasta dar con el enlace de ese texto.
func buscarEnlace(o fyne.CanvasObject, texto string) *ui.Enlace {
	switch v := o.(type) {
	case *ui.Enlace:
		if v.Texto == texto {
			return v
		}
		return nil
	case *fyne.Container:
		for _, h := range v.Objects {
			if e := buscarEnlace(h, texto); e != nil {
				return e
			}
		}
		return nil
	}
	if w, ok := o.(fyne.Widget); ok {
		for _, h := range test.WidgetRenderer(w).Objects() {
			if e := buscarEnlace(h, texto); e != nil {
				return e
			}
		}
	}
	return nil
}

// TestLaVistaRapidaEnsenaLasNotasDelEvento es EventNotes.tsx: las notas de Notepads que
// cuelgan del evento —candado si va cifrada—, con su enlace a la nota y «Escribir una
// nota para este evento», que lleva el UID y el título. Si Notepads no contesta, la
// sección no sale.
func TestLaVistaRapidaEnsenaLasNotasDelEvento(t *testing.T) {
	s := &escrituras{}
	a, w := appConVentana(t, s)
	var pedidas, abiertas []string
	fallar := false
	a.Notas = func(_ context.Context, uid string) ([]api.Note, error) {
		pedidas = append(pedidas, uid)
		if fallar {
			return nil, errSinServicio
		}
		return []api.Note{
			{ID: "n1", NotebookID: "c1", Title: "Acta del comité"},
			{ID: "n2", NotebookID: "c1", ContentCipher: "x"},
		}, nil
	}
	a.AbrirNotas = func(ruta string) { abiertas = append(abiertas, ruta) }
	o := ocurrenciaDe(t, a, "e1")
	ancla := container.NewWithoutLayout()
	ancla.Move(fyne.NewPos(700, 200))
	ancla.Resize(fyne.NewSize(140, 18))
	a.raiz.Add(container.NewWithoutLayout(ancla))
	a.abrirVistaRapida(o, ancla)
	guardar(t, w.Canvas().Capture(), "calendario-vista-rapida-notas")
	if !slices.Equal(pedidas, []string{"u1"}) {
		t.Fatalf("las notas se piden por el UID del evento: %v", pedidas)
	}
	if buscarEnlace(a.vista, "Nota sin título") == nil {
		t.Error("la nota sin título no dice «Nota sin título»")
	}
	acta := buscarEnlace(a.vista, "Acta del comité")
	if acta == nil {
		t.Fatal("no está la nota del evento")
	}
	acta.Tapped(nil)
	a.abrirVistaRapida(o, ancla)
	crear := buscarEnlace(a.vista, "Escribir una nota para este evento")
	if crear == nil {
		t.Fatal("no está «Escribir una nota para este evento»")
	}
	crear.Tapped(nil)
	if !slices.Equal(abiertas, []string{"/notes/?notebook=c1&note=n1", "/notes/?link=calendar_event:u1&label=Comit%C3%A9"}) {
		t.Fatalf("se abrió %v", abiertas)
	}
	fallar = true
	a.abrirVistaRapida(o, ancla)
	if buscarEnlace(a.vista, "Escribir una nota para este evento") != nil {
		t.Error("sin Notepads se ofrece escribir una nota")
	}
}

// buscarObjeto recorre el árbol hasta dar con el primer objeto que cumpla `vale`.
func buscarObjeto(o fyne.CanvasObject, vale func(fyne.CanvasObject) bool) fyne.CanvasObject {
	if vale(o) {
		return o
	}
	var hijos []fyne.CanvasObject
	switch v := o.(type) {
	case *fyne.Container:
		hijos = v.Objects
	case fyne.Widget:
		hijos = test.WidgetRenderer(v).Objects()
	}
	for _, h := range hijos {
		if x := buscarObjeto(h, vale); x != nil {
			return x
		}
	}
	return nil
}

// TestElSelectorDeFechaEligeDiaYHora es el desplegable del icono del campo: abre en lo
// TECLEADO, elegir un día cambia la fecha y conserva la hora (y el fin se arrastra con
// el inicio, como al teclear), y elegir una hora la fija y cierra.
func TestElSelectorDeFechaEligeDiaYHora(t *testing.T) {
	s := &escrituras{}
	a, w := appConVentana(t, s)
	inicio := time.Date(2026, 9, 22, 10, 0, 0, 0, time.UTC)
	e := Edicion{Inicio: inicio, Fin: inicio.Add(time.Hour), Zona: "UTC"}
	f := a.P.nuevoFormulario(e, a.destinos(), AccionesFormulario{})
	f.d.Mostrar(w.Canvas())
	f.inicio.PonerTexto("24/10/2026, 10:00")
	f.iconosFecha[0].Tapped(nil)
	capas := w.Canvas().Overlays().List()
	if len(capas) != 2 {
		t.Fatalf("hay %d capas: el selector no se ha abierto sobre el formulario", len(capas))
	}
	guardar(t, w.Canvas().Capture(), "calendario-selector-fecha")
	dia := buscarObjeto(capas[1], func(o fyne.CanvasObject) bool {
		d, ok := o.(*diaMini)
		return ok && d.dia.Month() == time.October && d.dia.Day() == 27
	})
	if dia == nil {
		t.Fatal("el selector no abre en el mes tecleado (octubre)")
	}
	dia.(*diaMini).Tapped(nil)
	if f.inicio.Texto() != "27/10/2026, 10:00" || f.fin.Texto() != "27/10/2026, 11:00" {
		t.Fatalf("tras elegir el 27: inicio %q, fin %q", f.inicio.Texto(), f.fin.Texto())
	}
	capas = w.Canvas().Overlays().List()
	hora := buscarObjeto(capas[len(capas)-1], func(o fyne.CanvasObject) bool {
		b, ok := o.(*ui.Boton)
		return ok && b.Texto == "14:30"
	})
	if hora == nil {
		t.Fatal("no está la hora 14:30")
	}
	hora.(*ui.Boton).Tapped(nil)
	if f.inicio.Texto() != "27/10/2026, 14:30" || f.fin.Texto() != "27/10/2026, 15:30" {
		t.Fatalf("tras elegir las 14:30: inicio %q, fin %q", f.inicio.Texto(), f.fin.Texto())
	}
	if n := len(w.Canvas().Overlays().List()); n != 1 {
		t.Errorf("elegir la hora deja %d capas y tenía que cerrar el selector", n)
	}
}

// TestSuscribirseDiceQueLosDatosSoloEntran es el `<DataFlow compact incoming={['events']}>`
// de SubscribeModal: bajo la frase, «Solo entra en Sytue One» y la política de
// privacidad del sitio público en el idioma de la app.
func TestSuscribirseDiceQueLosDatosSoloEntran(t *testing.T) {
	s := &escrituras{}
	a, w := appConVentana(t, s)
	var abiertos []string
	a.AbrirEnlace = func(u string) { abiertos = append(abiertos, u) }
	a.formularioSuscripcion()
	capas := w.Canvas().Overlays().List()
	if len(capas) != 1 {
		t.Fatalf("hay %d capas", len(capas))
	}
	// El título de la compacta se reparte por palabras (`ui.PalabrasFlujo`): basta la
	// primera de «Solo entra en Sytue One», que solo sale en el modo «in».
	if buscarObjeto(capas[0], func(o fyne.CanvasObject) bool { x, ok := o.(*canvas.Text); return ok && x.Text == "Solo" }) == nil {
		t.Fatal("el diálogo no dice que los datos solo entran")
	}
	guardar(t, w.Canvas().Capture(), "calendario-suscribirse")
	politica := buscarEnlace(capas[0], "Política de privacidad")
	if politica == nil {
		t.Fatal("no está el enlace a la política")
	}
	politica.Tapped(nil)
	if !slices.Equal(abiertos, []string{"https://sytue.eu/es/privacy"}) {
		t.Fatalf("se abrió %v", abiertos)
	}
}

// TestElAsistenteSeAnunciaComoDescribeElEvento es el `aria-label={t('event.ai')}` del
// campo del asistente, que no lleva `<label>` a la vista.
func TestElAsistenteSeAnunciaComoDescribeElEvento(t *testing.T) {
	a, _ := appConVentana(t, &escrituras{})
	inicio := time.Date(2026, 9, 22, 10, 0, 0, 0, time.UTC)
	f := a.P.nuevoFormulario(Edicion{Inicio: inicio, Fin: inicio.Add(time.Hour)}, a.destinos(), AccionesFormulario{
		Asistente: func(api.PeticionIA, func(string, error)) {},
	})
	if f.ia == nil || f.ia.AccessibilityLabel() != "Describe el evento" {
		t.Fatalf("el campo del asistente no se anuncia como «Describe el evento»")
	}
}

// TestLasHorasDeBuscarHuecoTienenCifrasTabulares es el `tabular-nums` de los botones de
// hora: «11:11» y «08:00» miden lo mismo, aunque en proporcional el 1 es más estrecho.
func TestLasHorasDeBuscarHuecoTienenCifrasTabulares(t *testing.T) {
	if ui.Medir("11:11", tema.TextoSM, ui.Normal).Width == ui.Medir("08:00", tema.TextoSM, ui.Normal).Width {
		t.Skip("con esta fuente las cifras ya son tabulares: el guardián no distingue")
	}
	a, b := nuevoBotonHueco("11:11", nil).MinSize().Width, nuevoBotonHueco("08:00", nil).MinSize().Width
	if a != b {
		t.Fatalf("«11:11» mide %v y «08:00» %v: sin cifras tabulares", a, b)
	}
}

// conReglaCruda es el servicio falso con una serie BYDAY —de las que crean Apple o
// Google y la interfaz no sabe editar— en vez de las de siempre. `regla` es su RRULE.
type conReglaCruda struct {
	*escrituras
	regla string
}

func (s conReglaCruda) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	switch r.Method + " " + r.URL.Path {
	case "GET /api/v1/calendars/mio/instances":
		// Cada ocurrencia con SU inicio, como en el servicio de verdad.
		_, _ = io.WriteString(w, `{"instances":[
			{"event":{"id":"b1","uid":"ub","summary":"Pilates","rrule":"`+s.regla+`","dtstart":"2026-09-16T17:00:00Z","dtend":"2026-09-16T18:00:00Z"},"start":"2026-09-16T17:00:00Z","end":"2026-09-16T18:00:00Z"}]}`)
		return
	case "GET /api/v1/events/b1":
		// El maestro, con el inicio de la serie y sin recordatorios.
		_, _ = io.WriteString(w, `{"id":"b1","uid":"ub","summary":"Pilates","rrule":"`+s.regla+`","dtstart":"2026-09-14T17:00:00Z","dtend":"2026-09-14T18:00:00Z"}`)
		return
	}
	s.escrituras.ServeHTTP(w, r)
}

// confirmarAlcance pulsa la opción `rotulo` del diálogo de alcance que está encima y
// «Guardar». Devuelve los rótulos de las opciones que ofrecía.
func confirmarAlcance(t *testing.T, w fyne.Window, rotulo string) []string {
	t.Helper()
	capas := w.Canvas().Overlays().List()
	if len(capas) == 0 {
		t.Fatal("no se ha preguntado el alcance")
	}
	arriba := capas[len(capas)-1]
	var ofrecidas []string
	var recorrer func(o fyne.CanvasObject)
	recorrer = func(o fyne.CanvasObject) {
		switch v := o.(type) {
		case *filaAlcance:
			ofrecidas = append(ofrecidas, v.etiqueta)
			if v.etiqueta == rotulo {
				v.Tapped(nil)
			}
			return
		case *fyne.Container:
			for _, h := range v.Objects {
				recorrer(h)
			}
			return
		}
		if wd, ok := o.(fyne.Widget); ok {
			for _, h := range test.WidgetRenderer(wd).Objects() {
				recorrer(h)
			}
		}
	}
	recorrer(arriba)
	b := buscarBoton(arriba, "Guardar")
	if b == nil {
		t.Fatal("el diálogo de alcance no tiene «Guardar»")
	}
	b.Tapped(nil)
	return ofrecidas
}

// TestLaReglaPersonalizadaSeConservaAlEditar: una serie BYDAY se abre con «Repetir» en
// «Personalizada»; cambiar el título y guardar «todos» manda la MISMA RRULE byte a
// byte, y elegir «No se repite» la quita. Antes el formulario la dejaba en «No se
// repite» sin preguntar y la serie se quedaba en un evento suelto.
func TestLaReglaPersonalizadaSeConservaAlEditar(t *testing.T) {
	const regla = "FREQ=MONTHLY;BYDAY=2MO"
	for _, quitar := range []bool{false, true} {
		s := &escrituras{}
		a, w := appConVentana(t, conReglaCruda{s, regla})
		o := ocurrenciaDe(t, a, "b1")
		if !esSerie(o) {
			t.Fatal("una serie «el segundo lunes» no se trata como serie")
		}
		e := a.P.EdicionDe(o)
		if e.Regla != nil || e.ReglaCruda != regla {
			t.Fatalf("la edición no guarda la regla: %+v %q", e.Regla, e.ReglaCruda)
		}
		f := a.P.nuevoFormulario(e, a.destinos(), a.accionesFormulario(e))
		f.d.Mostrar(w.Canvas())
		if got := f.repetir.Valor(); got != "Personalizada" {
			t.Fatalf("«Repetir» dice %q", got)
		}
		if !quitar {
			guardar(t, w.Canvas().Capture(), "calendario-formulario-regla-personalizada")
		}
		f.titulo.PonerTexto("Pilates avanzado")
		if quitar {
			f.repetir.AlElegir(0)
		}
		f.enviar()
		ofrecidas := confirmarAlcance(t, w, "Todos los eventos")
		if len(ofrecidas) != 3 {
			t.Errorf("sin COUNT se ofrecen los tres alcances y salen %v", ofrecidas)
		}
		// Los GET del maestro (los recordatorios y el inicio de la serie) los contesta el
		// falso y no se apuntan: la única escritura es el PUT.
		hechas, cuerpos := s.apuntadas()
		if len(hechas) != 1 || hechas[0] != "PUT /api/v1/events/b1" {
			t.Fatalf("guardar hizo %v", hechas)
		}
		c := cuerpos[0]
		if c["summary"] != "Pilates avanzado" {
			t.Errorf("el título del PUT es %v", c["summary"])
		}
		switch got, _ := c["rrule"].(string); {
		case !quitar && got != regla:
			t.Errorf("la regla del PUT es %q y era %q", got, regla)
		case quitar && got != "":
			t.Errorf("«No se repite» mandó la regla %q", got)
		}
	}
}

// TestLaReglaPersonalizadaVuelveSiSeReelige: probar «Semanalmente» y volver a
// «Personalizada» devuelve la regla de origen, no la semanal simple.
func TestLaReglaPersonalizadaVuelveSiSeReelige(t *testing.T) {
	const regla = "FREQ=YEARLY;BYMONTH=3;BYMONTHDAY=8"
	a, _ := appConVentana(t, conReglaCruda{&escrituras{}, regla})
	e := a.P.EdicionDe(ocurrenciaDe(t, a, "b1"))
	f := a.P.nuevoFormulario(e, a.destinos(), a.accionesFormulario(e))
	f.repetir.AlElegir(2)
	if in := f.e.EntradaDe(a.P); in.RRule != "FREQ=WEEKLY" {
		t.Fatalf("elegir «Semanalmente» manda %q", in.RRule)
	}
	f.repetir.AlElegir(len(frecuencias))
	if in := f.e.EntradaDe(a.P); in.RRule != regla {
		t.Fatalf("volver a «Personalizada» manda %q", in.RRule)
	}
}

// TestUnaReglaPersonalizadaSeParteSinPerderla: «solo este» saca el suelto y excluye la
// ocurrencia; «este y los siguientes» arrastra la regla tal cual a la serie nueva; con
// COUNT ese alcance no se ofrece al editar (no se sabe cuántas quedan), pero sí al
// borrar; y arrastrar el bloque conserva la regla en «todos».
func TestUnaReglaPersonalizadaSeParteSinPerderla(t *testing.T) {
	const regla = "FREQ=MONTHLY;BYDAY=2MO;UNTIL=20261231T235959Z"
	s := &escrituras{}
	a, _ := appConVentana(t, conReglaCruda{s, regla})
	o := ocurrenciaDe(t, a, "b1")
	in := o.Event.Input()
	if err := EditarSerie(context.Background(), a.Cal, o, AlcanceSiguientes, in, nil); err != nil {
		t.Fatal(err)
	}
	if err := EditarSerie(context.Background(), a.Cal, o, AlcanceTodos, in, nil); err != nil {
		t.Fatal(err)
	}
	_, cuerpos := s.apuntadas()
	if len(cuerpos) != 3 || cuerpos[0]["rrule"] != regla || cuerpos[1]["mode"] != "until" || cuerpos[2]["rrule"] != regla {
		t.Errorf("la regla no se arrastra: %v", cuerpos)
	}
	o.Event.RRule = "FREQ=MONTHLY;BYDAY=2MO;COUNT=6"
	if got := alcancesDe(o, false); !slices.Equal(got, []string{AlcanceEste, AlcanceTodos}) {
		t.Errorf("editar una personalizada con COUNT ofrece %v", got)
	}
	if got := alcancesDe(o, true); len(got) != 3 {
		t.Errorf("borrarla ofrece %v", got)
	}
	o.Event.RRule = "FREQ=WEEKLY;COUNT=6"
	if got := alcancesDe(o, false); len(got) != 3 {
		t.Errorf("una regla que se sabe editar ofrece %v", got)
	}
}

// TestResponderDesdeLaVistaRapidaMarcaLaRespuesta: «¿Vas a ir?» manda la respuesta por
// el UID del evento (para que llegue al .ics que lee la web), y la respuesta ya dada
// sale primaria y con ✓ y las otras dos, secundarias. Sin responder, ninguna marcada.
func TestResponderDesdeLaVistaRapidaMarcaLaRespuesta(t *testing.T) {
	s := &escrituras{}
	a, w := appConVentana(t, s)
	a.Correo = "yo@sytue.eu"
	ancla := container.NewWithoutLayout()
	ancla.Move(fyne.NewPos(700, 200))
	ancla.Resize(fyne.NewSize(140, 18))
	a.raiz.Add(container.NewWithoutLayout(ancla))
	o := ocurrenciaDe(t, a, "e1")
	marcados := func() []string {
		var out []string
		for _, r := range []string{"Sí", "Quizá", "No"} {
			b := buscarBoton(a.vista, r)
			if b == nil {
				t.Fatalf("no está «%s»", r)
			}
			if b.Variante == ui.Primario || b.Icono == iconos.Tic {
				out = append(out, r)
			}
		}
		return out
	}

	a.abrirVistaRapida(o, ancla)
	if got := marcados(); len(got) != 0 {
		t.Errorf("sin responder salen marcados %v", got)
	}

	o.Event.Attendees = []api.Attendee{{Email: "yo@sytue.eu", PartStat: "TENTATIVE"}}
	a.abrirVistaRapida(o, ancla)
	if got := marcados(); !slices.Equal(got, []string{"Quizá"}) {
		t.Errorf("con «Quizá» dado salen marcados %v", got)
	}
	if b := buscarBoton(a.vista, "Quizá"); b.Variante != ui.Primario || b.Icono != iconos.Tic {
		t.Error("la respuesta dada no es primaria con ✓")
	}
	guardar(t, w.Canvas().Capture(), "calendario-vista-rapida-respondida")

	buscarBoton(a.vista, "No").Tapped(nil)
	hechas, cuerpos := s.apuntadas()
	if len(hechas) != 1 || hechas[0] != "PUT /api/v1/events/u1/attendees/partstat" {
		t.Fatalf("responder hizo %v", hechas)
	}
	if cuerpos[0]["uid"] != "u1" || cuerpos[0]["partstat"] != "DECLINED" {
		t.Errorf("responder mandó %v", cuerpos[0])
	}
}

// botonesDias son los siete botones de «Se repite el» del formulario, en orden, o nil
// si no está el selector.
func botonesDias(f *formulario) []*ui.Boton {
	var out []*ui.Boton
	var recorrer func(o fyne.CanvasObject)
	recorrer = func(o fyne.CanvasObject) {
		switch v := o.(type) {
		case *ui.Boton:
			if v.Ancho == tema.Espacio(8) && v.Rotulo != "" {
				out = append(out, v)
			}
		case *fyne.Container:
			for _, h := range v.Objects {
				recorrer(h)
			}
		}
	}
	recorrer(f.cuerpo)
	return out
}

func encendidos(bs []*ui.Boton) []string {
	var out []string
	for _, b := range bs {
		if b.Variante == ui.Primario {
			out = append(out, b.Rotulo)
		}
	}
	return out
}

// TestRepetirCadaAnoLaborablesYDias es CAL-11: «Repetir» ofrece cada año y cada día
// laborable, una semanal deja elegir los días, y reabrir la serie enseña lo elegido.
// Las reglas son las mismas que guarda la web (EventModal.repetir.test.tsx).
func TestRepetirCadaAnoLaborablesYDias(t *testing.T) {
	a, w := appConVentana(t, &escrituras{})
	inicio := time.Date(2026, 11, 11, 9, 0, 0, 0, time.UTC) // miércoles
	e := Edicion{Inicio: inicio, Fin: inicio.Add(time.Hour)}
	f := a.P.nuevoFormulario(e, a.destinos(), AccionesFormulario{})
	elegir := func(freq string) {
		t.Helper()
		i := slices.Index(frecuencias, freq)
		if i < 0 {
			t.Fatalf("«Repetir» no tiene %s", freq)
		}
		f.repetir.AlElegir(i)
		f.repetir.Elegida = i
		f.rehacer()
	}
	regla := func() string { return f.e.EntradaDe(a.P).RRule }

	elegir("WEEKDAYS")
	if got := f.repetir.Valor(); got != "Cada día laborable (de lunes a viernes)" {
		t.Errorf("la opción dice %q", got)
	}
	if got := regla(); got != "FREQ=WEEKLY;BYDAY=MO,TU,WE,TH,FR" {
		t.Errorf("«cada día laborable» guarda %q", got)
	}
	if len(botonesDias(f)) != 0 {
		t.Error("«cada día laborable» no enseña los días")
	}
	elegir("YEARLY")
	if got := regla(); got != "FREQ=YEARLY" {
		t.Errorf("«Anualmente» guarda %q", got)
	}

	elegir("WEEKLY")
	bs := botonesDias(f)
	if len(bs) != 7 || !slices.Equal(encendidos(bs), []string{"miércoles"}) {
		t.Fatalf("una semanal enciende %v de %d", encendidos(bs), len(bs))
	}
	if got := regla(); got != "FREQ=WEEKLY" {
		t.Errorf("sin tocar los días guarda %q", got)
	}
	bs[0].Tapped(nil) // + lunes
	if got := regla(); got != "FREQ=WEEKLY;BYDAY=MO,WE" {
		t.Errorf("lunes y miércoles guarda %q", got)
	}
	f.d.Mostrar(w.Canvas())
	guardar(t, w.Canvas().Capture(), "calendario-formulario-repetir-dias")
	bs = botonesDias(f)
	bs[0].Tapped(nil) // − lunes
	bs = botonesDias(f)
	bs[2].Tapped(nil) // el miércoles es el único: no se apaga
	if got := encendidos(botonesDias(f)); !slices.Equal(got, []string{"miércoles"}) {
		t.Errorf("tras apagar quedan %v", got)
	}

	// Reabrir: la serie enseña lo que se eligió.
	for _, c := range []struct {
		rrule, opcion string
		dias          []string
	}{
		{"FREQ=WEEKLY;BYDAY=MO,TU,WE,TH,FR", "Cada día laborable (de lunes a viernes)", nil},
		{"FREQ=WEEKLY;BYDAY=MO,WE", "Semanalmente", []string{"lunes", "miércoles"}},
		{"FREQ=YEARLY", "Anualmente", nil},
		// Una serie sin fin se guarda sin fin: el «4» de partida del campo de veces no
		// se cuela como COUNT.
		{"FREQ=DAILY", "Diariamente", nil},
		{"FREQ=WEEKLY;BYDAY=MO,WE;COUNT=2", "Semanalmente", []string{"lunes", "miércoles"}},
	} {
		a, _ := appConVentana(t, conReglaCruda{&escrituras{}, c.rrule})
		e := a.P.EdicionDe(ocurrenciaDe(t, a, "b1"))
		if e.Regla == nil || e.ReglaCruda != "" {
			t.Fatalf("%s se abre como personalizada", c.rrule)
		}
		f := a.P.nuevoFormulario(e, a.destinos(), a.accionesFormulario(e))
		if got := f.repetir.Valor(); got != c.opcion {
			t.Errorf("%s se abre con «%s»", c.rrule, got)
		}
		if got := encendidos(botonesDias(f)); !slices.Equal(got, c.dias) {
			t.Errorf("%s enciende %v", c.rrule, got)
		}
		if got := f.e.EntradaDe(a.P).RRule; got != c.rrule {
			t.Errorf("%s se guarda sin tocar como %q", c.rrule, got)
		}
	}
}
