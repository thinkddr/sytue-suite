// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package calendario

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"git.sytue.eu/sytue/one/apps/nucleo/api"

	"git.sytue.eu/sytue/one/apps/fyne/agenda"
	"git.sytue.eu/sytue/one/apps/fyne/tema"
)

// TestElRangoEsElDeLaWeb compara `Rango` con `rangeFor` de CalendarApp.tsx, calculado a
// mano para septiembre de 2026 (empieza en martes; el 22 es martes).
func TestElRangoEsElDeLaWeb(t *testing.T) {
	cursor := time.Date(2026, 9, 22, 15, 30, 0, 0, time.UTC)
	for _, c := range []struct {
		v           Vista
		desde, hata string
	}{
		{VistaMes, "2026-08-31", "2026-10-12"},    // las 42 celdas de monthGrid
		{VistaSemana, "2026-09-21", "2026-09-28"}, // de lunes a lunes
		{VistaDia, "2026-09-22", "2026-09-23"},
		{VistaAgenda, "2026-09-22", "2026-10-22"}, // los treinta días de AGENDA_DAYS
	} {
		d, h := Rango(c.v, cursor, time.UTC)
		if got := d.Format(time.DateOnly) + ".." + h.Format(time.DateOnly); got != c.desde+".."+c.hata {
			t.Errorf("vista %d: el rango es %s y la web pide %s..%s", c.v, got, c.desde, c.hata)
		}
	}
}

// TestNavegarEsElDeLaWeb: `navigate(delta)`. El mes desborda como `setMonth` de JS, y la
// agenda salta su ventana entera de treinta días.
func TestNavegarEsElDeLaWeb(t *testing.T) {
	enero31 := time.Date(2026, 1, 31, 9, 0, 0, 0, time.UTC)
	for _, c := range []struct {
		v     Vista
		delta int
		desde time.Time
		llega string
	}{
		{VistaMes, 1, enero31, "2026-03-03"},
		{VistaSemana, -1, enero31, "2026-01-24"},
		{VistaDia, 1, enero31, "2026-02-01"},
		{VistaAgenda, 1, enero31, "2026-03-02"},
	} {
		if got := Desplazar(c.v, c.desde, c.delta).Format(time.DateOnly); got != c.llega {
			t.Errorf("vista %d, %+d: llega al %s y la web al %s", c.v, c.delta, got, c.llega)
		}
	}
}

// TestElTituloEsElDeLaWeb: `titleFor`, en español, con los patrones de Intl.
func TestElTituloEsElDeLaWeb(t *testing.T) {
	p := &Pantalla{T: traductor(t), Cursor: time.Date(2026, 9, 22, 12, 0, 0, 0, time.UTC), Zona: time.UTC}
	for v, quiero := range map[Vista]string{
		VistaMes:    "Septiembre de 2026",
		VistaSemana: "21–27 de septiembre de 2026",
		VistaDia:    "Martes, 22 de septiembre de 2026",
		VistaAgenda: "22 de septiembre",
	} {
		p.Vista = v
		if got := p.Titulo(); got != quiero {
			t.Errorf("vista %d: el título es %q y la web pinta %q", v, got, quiero)
		}
	}
}

// servicioFalso es Sytue Calendar con un calendario propio, uno compartido para ver y
// un evento en cada uno. Apunta cada rango pedido para que el test vea qué se pide.
type servicioFalso struct {
	mu       sync.Mutex
	pedidos  []string
	romper   bool
	listados int
	retirado bool // «suyo» contesta 403: le han retirado la compartición
}

func (s *servicioFalso) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	defer s.mu.Unlock()
	switch r.URL.Path {
	case "/api/v1/calendars":
		s.listados++
		if s.romper {
			http.Error(w, `{"error":"boom"}`, http.StatusInternalServerError)
			return
		}
		_, _ = io.WriteString(w, `{"calendars":[{"id":"mio","display_name":"Personal","color":"#16734d","is_default":true}]}`)
	case "/api/v1/calendars/shared-with-me":
		_, _ = io.WriteString(w, `{"calendars":[{"id":"suyo","display_name":"Equipo","color":"#b04a1f","permission":"reader","shared_by":"ada@sytue.eu"}]}`)
	case "/api/v1/calendars/mio/instances":
		s.pedidos = append(s.pedidos, "mio "+r.URL.Query().Get("start"))
		_, _ = io.WriteString(w, `{"instances":[{"event":{"id":"e1","summary":"Comité","color":"#123456"},"start":"2026-09-10T08:00:00Z","end":"2026-09-10T09:00:00Z"}]}`)
	case "/api/v1/calendars/suyo/instances":
		s.pedidos = append(s.pedidos, "suyo "+r.URL.Query().Get("start"))
		if s.retirado {
			http.Error(w, `{"error":"forbidden"}`, http.StatusForbidden)
			return
		}
		_, _ = io.WriteString(w, `{"instances":[{"event":{"id":"e2","summary":"Guardia"},"start":"2026-09-11T08:00:00Z","end":"2026-09-11T09:00:00Z"}]}`)
	case "/api/v1/invitations":
		_, _ = io.WriteString(w, `{"invitations":[{"id":"i1","uid":"u1","status":"pending"},{"id":"i2","uid":"u2","status":"pending"}]}`)
	default:
		http.NotFound(w, r)
	}
}

func (s *servicioFalso) cuantos() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.pedidos)
}

// appDePrueba monta la App contra el servicio falso, con las cargas en línea: sin
// gorutinas, lo que el test lee es lo que ya ha pasado.
func appDePrueba(t *testing.T, s http.Handler) *App {
	t.Helper()
	srv := httptest.NewServer(s)
	t.Cleanup(srv.Close)
	p := &Pantalla{T: traductor(t), Cursor: time.Date(2026, 9, 21, 12, 0, 0, 0, time.UTC), Zona: time.UTC}
	a := NuevaApp(p, api.NewCalendars(srv.URL, nil, nil))
	a.lanzar = func(f func()) { f() }
	a.enUI = func(f func()) { f() }
	return a
}

// TestLaAppCargaFundeYCachea es CalendarApp de punta a punta contra el servicio: pide
// los calendarios, enciende todos, pide los eventos de cada uno y los funde con el color
// que toca; conmutar un calendario ya cargado NO vuelve a la red, y cambiar de mes sí.
func TestLaAppCargaFundeYCachea(t *testing.T) {
	s := &servicioFalso{}
	a := appDePrueba(t, s)
	a.Arrancar()
	p := a.P

	if p.Estado != agenda.Exito || len(p.Calendarios) != 1 || len(p.Compartidos) != 1 {
		t.Fatalf("tras arrancar: estado %d, %d propios, %d compartidos", p.Estado, len(p.Calendarios), len(p.Compartidos))
	}
	if len(p.Eventos) != 2 || s.cuantos() != 2 {
		t.Fatalf("se esperaban 2 eventos de 2 peticiones; hay %d eventos y %d peticiones", len(p.Eventos), s.cuantos())
	}
	// El color del EVENTO manda sobre el de su calendario; sin color, el del calendario.
	if got := p.Eventos[0].Color; got != colorDe("#123456") {
		t.Errorf("el evento con color propio sale %v", got)
	}
	if got := p.Eventos[1].Color; got != colorDe("#b04a1f") {
		t.Errorf("el evento sin color no toma el de su calendario: %v", got)
	}
	// Un compartido para VER es de solo lectura: no se arrastra ni se edita.
	if !p.Eventos[1].SoloLectura || p.Eventos[0].SoloLectura {
		t.Errorf("solo lectura mal repartida: %+v / %+v", p.Eventos[0].SoloLectura, p.Eventos[1].SoloLectura)
	}
	if p.Invitaciones != 2 {
		t.Errorf("la insignia de invitaciones dice %d y hay 2 pendientes", p.Invitaciones)
	}

	// Apagar y encender un calendario ya cargado no pide nada: es la caché del rango.
	p.AlConmutarCalendario("suyo")
	if len(p.Eventos) != 1 {
		t.Errorf("con «Equipo» apagado se ven %d eventos", len(p.Eventos))
	}
	p.AlConmutarCalendario("suyo")
	if len(p.Eventos) != 2 || s.cuantos() != 2 {
		t.Errorf("volver a encender fue a la red: %d peticiones", s.cuantos())
	}

	// Otro mes es otro rango: se piden los dos calendarios otra vez, desde su lunes.
	p.AlNavegar(1)
	if s.cuantos() != 4 {
		t.Fatalf("cambiar de mes hizo %d peticiones en total, y tocaban 4", s.cuantos())
	}
	if got := s.pedidos[2]; !strings.HasSuffix(got, "2026-09-28T00:00:00Z") {
		t.Errorf("octubre de 2026 empieza su rejilla el lunes 28 de septiembre; se pidió %q", got)
	}
	// Y las flechas del mini-mes cambian de MES aunque la vista sea la semana.
	p.AlCambiarVista(VistaSemana)
	antes := p.Cursor
	p.AlDesplazarMes(1)
	if p.Cursor.Month() != antes.Month()+1 {
		t.Errorf("la flecha del mini-mes movió el cursor de %s a %s", antes, p.Cursor)
	}
}

// TestLaAppRecuerdaLoQueSeApago: al recargar la lista, lo apagado sigue apagado y solo lo
// NUEVO se enciende. Es la regla de `knownUrls` de la web.
func TestLaAppRecuerdaLoQueSeApago(t *testing.T) {
	a := appDePrueba(t, &servicioFalso{})
	a.Arrancar()
	a.P.AlConmutarCalendario("mio")
	a.RecargarCalendarios()
	if a.P.Visibles["mio"] || !a.P.Visibles["suyo"] {
		t.Errorf("tras recargar, visibles = %v: se perdió lo que se apagó", a.P.Visibles)
	}
}

// TestUnCalendarioSinAccesoSeQuitaYNoTumbaLaVista es lo visto en producción: retirada la
// compartición, el servicio da 403 a ESE calendario. La pantalla no entra en «sin
// permiso»: el calendario se quita, se avisa con su nombre, los demás siguen, y cambiar
// de mes no lo vuelve a pedir.
func TestUnCalendarioSinAccesoSeQuitaYNoTumbaLaVista(t *testing.T) {
	s := &servicioFalso{retirado: true}
	a := appDePrueba(t, s)
	a.Arrancar()
	p := a.P
	if p.Estado != agenda.Exito {
		t.Fatalf("un 403 de un calendario dejó la pantalla en el estado %d (fallo %q)", p.Estado, p.Fallo)
	}
	if len(p.Compartidos) != 0 || p.Visibles["suyo"] || len(p.Eventos) != 1 {
		t.Errorf("«Equipo» sigue: %d compartidos, visible %v, %d eventos", len(p.Compartidos), p.Visibles["suyo"], len(p.Eventos))
	}
	if av := a.avisos.Avisos(); len(av) != 1 || !strings.Contains(av[0].Titulo, "Equipo") {
		t.Errorf("se esperaba un aviso de «ya no tienes acceso a Equipo»: %+v", av)
	}
	antes := s.cuantos()
	p.AlNavegar(1)
	if got := s.cuantos() - antes; got != 1 {
		t.Errorf("cambiar de mes hizo %d peticiones: el calendario retirado se volvió a pedir", got)
	}
}

// TestLaAppSinCalendariosEnseñaElError: si falla la lista, error con su mensaje y
// «Reintentar» vuelve a pedir la LISTA.
func TestLaAppSinCalendariosEnsenaElError(t *testing.T) {
	s := &servicioFalso{romper: true}
	a := appDePrueba(t, s)
	a.Arrancar()
	if a.P.Estado != agenda.Error || a.P.Fallo != "cal.error.loadCalendars" {
		t.Fatalf("estado %d con fallo %q", a.P.Estado, a.P.Fallo)
	}
	s.mu.Lock()
	s.romper = false
	s.mu.Unlock()
	a.P.AlReintentar()
	if a.P.Estado != agenda.Exito || s.listados != 2 {
		t.Errorf("reintentar: estado %d, %d listados", a.P.Estado, s.listados)
	}
}

// TestLaAgendaAnclaYFiltra: lo que empezó antes de la ventana se ancla al primer día, y
// el buscador encuentra sin tildes ni mayúsculas, en título, lugar y descripción.
func TestLaAgendaAnclaYFiltra(t *testing.T) {
	dia := func(d, h int) time.Time { return time.Date(2026, 9, d, h, 0, 0, 0, time.UTC) }
	ev := func(inicio time.Time, titulo, lugar string) Ocurrencia {
		return Ocurrencia{Instance: api.Instance{Event: api.Event{Summary: titulo, Location: lugar}, Start: inicio, End: inicio.Add(time.Hour)}}
	}
	p := &Pantalla{Cursor: dia(22, 12), Zona: time.UTC, Eventos: []Ocurrencia{
		ev(dia(20, 9), "Congreso", ""), ev(dia(22, 10), "Reunión de producto", ""), ev(dia(24, 10), "Comida", "Sala Ágora"),
	}}
	g := p.porDia("")
	if len(g) != 2 || !mismoDia(g[0].dia, dia(22, 0)) || len(g[0].dentro) != 2 {
		t.Fatalf("el congreso del 20 no se ancló al 22: %+v", g)
	}
	if got := p.porDia("REUNION"); len(got) != 1 || got[0].dentro[0].Event.Summary != "Reunión de producto" {
		t.Errorf("«REUNION» no encuentra «Reunión»: %+v", got)
	}
	if got := p.porDia("agora"); len(got) != 1 {
		t.Errorf("el buscador no mira el lugar: %+v", got)
	}
}

// TestLaInsigniaDeVerMideDosPixelesMas: el borde de un `<span>` en línea va por fuera
// del relleno, así que «Puede ver» es dos píxeles más ancha y más alta que «Puede
// editar» con el mismo texto.
func TestLaInsigniaDeVerMideDosPixelesMas(t *testing.T) {
	ver, editar := nuevaInsigniaPermiso("X", false).medida(), nuevaInsigniaPermiso("X", true).medida()
	if ver.Width-editar.Width != 2 || ver.Height-editar.Height != 2 {
		t.Errorf("ver %v, editar %v: tienen que diferir en el borde, 1 px por lado", ver, editar)
	}
	// `text-2xs` (caja de 16) con `py-0.5` (2 por lado).
	if editar.Height != 20 {
		t.Errorf("la de editar mide %v de alto y la web 20", editar.Height)
	}
}

// TestLaAgendaSePintaDondeDicenSusClases pinta la agenda con dos días y mira el píxel
// del punto de la primera fila, que es donde se suman TODAS las medidas de la vista: la
// cabecera (46,5), la banda del buscador (`p-2` + `h-9` + filete), el `py-2.5` del día,
// el `px-3` + `w-14` + `gap-3` de la columna de fecha y el `border` + `px-2` de la fila.
// Los números son de la HOJA: la agenda no está en el volcado de producción.
func TestLaAgendaSePintaDondeDicenSusClases(t *testing.T) {
	p := pantallaDeMuestra(t)
	p.Vista = VistaAgenda
	p.Cursor = time.Date(2026, 9, 11, 12, 0, 0, 0, time.UTC)
	img := pintar(t, p)
	guardar(t, img, "calendario-agenda")

	y := altoCab + 2*espacio2 + 36 + 1 + espacio2p5 + AltoFilaAgenda()/2
	x := float32(tema.AnchoLateral) + espacio3 + AnchoColumnaDia + espacio3 + 1 + espacio2 + LadoPunto/2
	if got := px(img, int(x), int(y)); !mismoColor(got, AcentoPorDefecto) {
		t.Errorf("el punto de la primera fila (%v,%v) pinta %v y el evento es %v", x, y, got, AcentoPorDefecto)
	}
	// Y el filete del `divide-y` entre el primer día (dos eventos) y el segundo.
	yFilete := altoCab + 2*espacio2 + 36 + 1 + MedidaGrupoAgenda(2, false)
	// El filete ocupa [170,5 · 171,5): Fyne cuantiza cada borde al píxel más cercano y
	// pinta el 171.
	if got, quiero := px(img, int(tema.AnchoLateral)+300, int(yFilete+0.5)), tema.Color("border", tema.Claro); !mismoColor(got, quiero) {
		t.Errorf("el filete entre días en y=%v pinta %v y el token `border` es %v", yFilete, got, quiero)
	}
}
