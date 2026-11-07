// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package calendario

import (
	"image"
	"image/color"
	"testing"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/test"

	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/i18n"

	"git.sytue.eu/sytue/one/apps/fyne/agenda"
	"git.sytue.eu/sytue/one/apps/fyne/catalogos"
	"git.sytue.eu/sytue/one/apps/fyne/tema"
	"git.sytue.eu/sytue/one/apps/fyne/ui"
)

// Los guardianes del mes del teléfono (vistamescompacta.go), con los datos de la
// captura del dueño: un iPhone de 390 el viernes 6 de noviembre de 2026 a las 19:36, con
// un punto en el 2, 3, 6 (Tenis, 14:00–15:00), 10, 13, 17, 19, 20, 21, 23, 24, 26, 27 y
// 30 y dos en el 9 y el 16. Son los mismos que pinta el banco de la web
// (apps/calendar/test/mes-movil.tsx).

var ahoraCaptura = time.Date(2026, 11, 6, 19, 36, 0, 0, time.UTC)

func eventosCaptura() []Ocurrencia {
	uno := []int{2, 3, 10, 13, 17, 19, 20, 21, 23, 24, 26, 27, 30}
	e := make([]Ocurrencia, 0, len(uno)+5)
	for _, d := range uno {
		ini := time.Date(2026, 11, d, 10, 0, 0, 0, time.UTC)
		e = append(e, conHora("Reunión", ini, ini.Add(time.Hour), color.NRGBA{}))
	}
	for _, d := range []int{9, 16} {
		for h := range 2 {
			ini := time.Date(2026, 11, d, 9+2*h, 0, 0, 0, time.UTC)
			e = append(e, conHora("Clase", ini, ini.Add(time.Hour), color.NRGBA{}))
		}
	}
	tenis := time.Date(2026, 11, 6, 14, 0, 0, 0, time.UTC)
	e = append(e, conHora("Tenis", tenis, tenis.Add(time.Hour), color.NRGBA{}))
	ordenar(e)
	return e
}

// pantallaCaptura es la pantalla del teléfono de la captura, sin red.
func pantallaCaptura(t *testing.T) *Pantalla {
	t.Helper()
	return &Pantalla{
		T: traductor(t), Vista: VistaMes, Estado: agenda.Exito, Ventana: 390,
		Ahora: func() time.Time { return ahoraCaptura }, Cursor: ahoraCaptura, Zona: time.UTC,
		Eventos: eventosCaptura(),
	}
}

// telefono monta la pantalla en una ventana de 390×844.
func telefono(t *testing.T, p *Pantalla) (fyne.Window, image.Image) {
	t.Helper()
	w := test.NewWindow(p.Construir())
	t.Cleanup(w.Close)
	w.SetPadded(false)
	w.Resize(fyne.NewSize(390, 844))
	return w, w.Canvas().Capture()
}

// appTelefono es la App sin red con los eventos de la captura ya en su caché, montada
// en una ventana de 390×844 con la ranura de atajos de cmd/calendar.
func appTelefono(t *testing.T) (*App, fyne.Window) {
	t.Helper()
	p := pantallaCaptura(t)
	p.Eventos = nil
	a := NuevaApp(p, nil)
	a.lanzar = func(f func()) { f() }
	a.enUI = func(f func()) { f() }
	a.cargandoCal = false
	p.Calendarios = []api.Calendar{{ID: "c", DisplayName: "Personal"}}
	ocs := eventosCaptura()
	inst := make([]api.Instance, 0, len(ocs))
	for _, o := range ocs {
		inst = append(inst, o.Instance)
	}
	a.porCal = map[string][]api.Instance{"c": inst}
	desde, hasta := Rango(p.Vista, p.Cursor, p.zona())
	a.clave = desde.Format(time.RFC3339) + ".." + hasta.Format(time.RFC3339)
	a.pintar()
	w := test.NewWindow(a.Objeto())
	t.Cleanup(w.Close)
	w.SetPadded(false)
	w.Resize(fyne.NewSize(390, 844))
	w.Canvas().SetOnTypedRune(func(r rune) {
		if len(w.Canvas().Overlays().List()) == 0 {
			a.Atajo(r)
		}
	})
	return a, w
}

func mesDe(t *testing.T, raiz fyne.CanvasObject) *mesCompacto {
	t.Helper()
	m, _ := buscarObjeto(raiz, func(o fyne.CanvasObject) bool { _, es := o.(*mesCompacto); return es }).(*mesCompacto)
	if m == nil {
		t.Fatal("no hay mes compacto en la pantalla")
	}
	return m
}

func barraDe(t *testing.T, raiz fyne.CanvasObject) *ui.BarraMovil {
	t.Helper()
	b, _ := buscarObjeto(raiz, func(o fyne.CanvasObject) bool { _, es := o.(*ui.BarraMovil); return es }).(*ui.BarraMovil)
	if b == nil {
		t.Fatal("no hay barra móvil")
	}
	return b
}

// textos son todos los textos pintados bajo `raiz`.
func textos(raiz fyne.CanvasObject) map[string]bool {
	out := map[string]bool{}
	buscarObjeto(raiz, func(o fyne.CanvasObject) bool {
		if tx, es := o.(*canvas.Text); es && tx.Visible() {
			out[tx.Text] = true
		}
		return false
	})
	return out
}

// diaN es el widget del día `n` del mes en pantalla.
func diaN(t *testing.T, m *mesCompacto, n int) *diaCompacto {
	t.Helper()
	for _, d := range m.dias {
		if d != nil && d.d.Fecha.Day() == n && !d.d.OtroMes {
			return d
		}
	}
	t.Fatalf("no hay día %d", n)
	return nil
}

// TestElMesDelTelefonoSeParecaALaCaptura: la geometría, los colores y los textos del
// mes del teléfono, en oscuro y en claro. Con SYTUE_CAPTURAS deja las PNG para mirarlas
// junto a la captura del dueño y a la de la web.
//
// VISTOS EN ROJO: con `compacta()` siempre falso (sale la rejilla de escritorio), con la
// caja del elegido sin rellenar y con la pastilla en `surface` en vez de `bg-subtle`.
func TestElMesDelTelefonoSeParecaALaCaptura(t *testing.T) {
	t.Cleanup(func() { tema.Elegir("") })
	for _, modo := range []string{"oscuro", "claro"} {
		tema.Elegir(modo)
		v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
		a, w := appTelefono(t)
		p := a.P
		img := w.Canvas().Capture()
		guardar(t, img, "calendario-mes-movil-"+modo)
		raiz := w.Content()

		m := mesDe(t, raiz)
		if got := numeros(m.semanas); len(got) != 6 || got[0] != 44 || got[5] != 49 {
			t.Fatalf("%s: semanas %v", modo, got)
		}
		if h := m.Size().Height; h != AltoDiasCompacta+6*AltoSemanaCompacta+AltoAsaMes {
			t.Errorf("%s: la hoja mide %v", modo, h)
		}
		seis := p.diaElegido
		if seis == nil || seis.d.Fecha.Day() != 6 {
			t.Fatalf("%s: el elegido no es el 6", modo)
		}
		col := columnasDiasCompacta.AnchoColumna(390 - AnchoColSemana)
		if seis.Size().Width != col || seis.Size().Height != AltoSemanaCompacta {
			t.Errorf("%s: el día mide %v y es %v × 40", modo, seis.Size(), col)
		}
		pos := fyne.CurrentApp().Driver().AbsolutePositionForObject(seis)
		if x := pos.X - fyne.CurrentApp().Driver().AbsolutePositionForObject(m).X; x != AnchoColSemana+4*col {
			t.Errorf("%s: el viernes está en x=%v y va tras la columna de semana (%v)", modo, x, AnchoColSemana+4*col)
		}
		cx := int(pos.X + seis.Size().Width/2)
		arriba := int(pos.Y + arribaNumero)
		if c := px(img, cx, arriba+3); !mismoColor(c, tema.Color("primary", v)) {
			t.Errorf("%s: dentro de la caja del 6 hay %v y es el primario %v", modo, c, tema.Color("primary", v))
		}
		hayOnPrimary := false
		for y := arriba; y < arriba+int(LadoNumeroCompacto); y++ {
			for x := cx - 14; x < cx+14; x++ {
				if mismoColor(px(img, x, y), tema.Color("on-primary", v)) {
					hayOnPrimary = true
				}
			}
		}
		if !hayOnPrimary {
			t.Errorf("%s: el número del 6 no va en on-primary", modo)
		}
		pas := m.pastis[1]
		pp := fyne.CurrentApp().Driver().AbsolutePositionForObject(pas)
		if c := px(img, int(pp.X)+2, int(pp.Y)+2); !mismoColor(c, tema.Color("bg-subtle", v)) {
			t.Errorf("%s: la pastilla es %v y es bg-subtle", modo, c)
		}
		for d, quiero := range map[int]int{2: 1, 9: 2, 4: 0} {
			if got := len(diaN(t, m, d).d.Puntos); got != quiero {
				t.Errorf("%s: el %d con %d puntos y son %d", modo, d, got, quiero)
			}
		}

		b := barraDe(t, raiz)
		if b.Titulo != "noviembre" || !b.TituloCentrado || len(b.Acciones) != 2 {
			t.Errorf("%s: barra %q centrado %v con %d acciones", modo, b.Titulo, b.TituloCentrado, len(b.Acciones))
		}
		if tx := textos(b); !tx["noviembre"] || !tx["6"] {
			t.Errorf("%s: la barra no pinta el mes y el 6 de la hoja: %v", modo, tx)
		}
		if buscarObjeto(raiz, func(o fyne.CanvasObject) bool { _, es := o.(*CabeceraVista); return es }) != nil {
			t.Errorf("%s: en el teléfono no hay cabecera de vista", modo)
		}
		if n := len(a.capaFlotante().Objects); n != 0 {
			t.Errorf("%s: en el mes del teléfono no hay botón flotante (hay %d)", modo, n)
		}
		tx := textos(raiz)
		for _, quiero := range []string{"viernes, 6 de noviembre de 2026", "14:00 – 15:00", "Tenis", "lun", "dom", "44", "49"} {
			if !tx[quiero] {
				t.Errorf("%s: falta %q en la pantalla", modo, quiero)
			}
		}
		if modo == "oscuro" {
			p.mesPlegado = true
			a.pintar()
			guardar(t, w.Canvas().Capture(), "calendario-mes-movil-plegado-oscuro")
			p.mesPlegado = false
			p.Recordatorios = []api.Reminder{{ID: "r1", Summary: "Tenis", DTStart: time.Date(2026, 11, 6, 13, 0, 0, 0, time.UTC)}}
			p.Invitaciones = 1
			a.recCargando = false
			p.recEnCajon = true
			p.cajonAbierto = true
			a.pintar()
			guardar(t, w.Canvas().Capture(), "calendario-mes-movil-cajon-oscuro")
		}
	}
}

// TestCargandoElMesDelTelefonoNoCambiaDeForma: mientras carga, la hoja es la del mes (5
// semanas en diciembre de 2026) sin puntos, y la lista su esqueleto: NO las 42 celdas
// del esqueleto de escritorio. VISTO EN ROJO dejando que la carga pinte `cuerpo()`
// (`if p.compacta() && p.Estado != agenda.Cargando` en `contenido`).
func TestCargandoElMesDelTelefonoNoCambiaDeForma(t *testing.T) {
	p := pantallaCaptura(t)
	p.Cursor, p.Estado, p.Eventos = time.Date(2026, 12, 6, 12, 0, 0, 0, time.UTC), agenda.Cargando, nil
	w, _ := telefono(t, p)
	m := mesDe(t, w.Content())
	if got := numeros(m.semanas); len(got) != 5 || got[0] != 49 || got[4] != 53 {
		t.Errorf("semanas %v y son [49…53]", got)
	}
	for _, d := range m.dias {
		if d != nil && len(d.d.Puntos) > 0 {
			t.Errorf("cargando, el %d lleva puntos", d.d.Fecha.Day())
		}
	}
	n := 0
	buscarObjeto(w.Content(), func(o fyne.CanvasObject) bool {
		if c, es := o.(*fyne.Container); es {
			if _, es := c.Layout.(*filaEsqueleto); es {
				n++
			}
		}
		return false
	})
	if n != 3 {
		t.Errorf("la lista tiene %d filas de esqueleto y son 3", n)
	}
}

// TestTocarUnDiaLoEligeYMasCrea: tocar un día lo ELIGE y no crea; el «+» crea en el día
// elegido, y sin red está inerte. VISTO EN ROJO con el `AlPulsar` del día en
// `AlCrearEvento` (lo de la rejilla de escritorio).
func TestTocarUnDiaLoEligeYMasCrea(t *testing.T) {
	p := pantallaCaptura(t)
	var elegido, creado time.Time
	p.AlElegirDia = func(d time.Time) { elegido = d }
	p.AlCrearEvento = func(d time.Time) { creado = d }
	w, _ := telefono(t, p)
	m := mesDe(t, w.Content())
	test.Tap(diaN(t, m, 13))
	if elegido.Day() != 13 || !creado.IsZero() {
		t.Errorf("tocar el 13 eligió %v y creó %v", elegido, creado)
	}
	b := barraDe(t, w.Content())
	mas := b.Acciones[1].(*ui.BotonIcono)
	test.Tap(mas)
	if !creado.Equal(p.Cursor) {
		t.Errorf("el + creó en %v y el elegido es %v", creado, p.Cursor)
	}
	p.SinConexion = true
	w2, _ := telefono(t, p)
	if !barraDe(t, w2.Content()).Acciones[1].(*ui.BotonIcono).Inerte {
		t.Error("sin red el + no está inerte")
	}
}

// TestLasTeclasMuevenLaSeleccionYElFocoLaSigue: ←→↑↓, Inicio/Fin y AvPág (31-ene +
// AvPág es el 28-feb), y el foco va al día nuevo. VISTO EN ROJO sin recolocar el foco en `pintar` (el
// foco se queda en un día que ya no existe).
func TestLasTeclasMuevenLaSeleccionYElFocoLaSigue(t *testing.T) {
	a, w := appTelefono(t)
	p := a.P
	tecla := func(k fyne.KeyName) {
		t.Helper()
		w.Canvas().Focus(p.diaElegido)
		p.diaElegido.TypedKey(&fyne.KeyEvent{Name: k})
	}
	for _, c := range []struct {
		k     fyne.KeyName
		llega string
	}{
		{fyne.KeyRight, "2026-11-07"},
		{fyne.KeyDown, "2026-11-14"},
		{fyne.KeyLeft, "2026-11-13"},
		{fyne.KeyUp, "2026-11-06"},
		{fyne.KeyHome, "2026-11-02"},
		{fyne.KeyEnd, "2026-11-08"},
		{fyne.KeyPageDown, "2026-12-08"},
		{fyne.KeyPageUp, "2026-11-08"},
	} {
		tecla(c.k)
		if got := p.Cursor.Format(time.DateOnly); got != c.llega {
			t.Errorf("%s llega al %s y es el %s", c.k, got, c.llega)
		}
		if w.Canvas().Focused() != p.diaElegido {
			t.Errorf("tras %s el foco no está en el día elegido", c.k)
		}
	}
	a.P.AlElegirDia(time.Date(2027, 1, 31, 9, 0, 0, 0, time.UTC))
	tecla(fyne.KeyPageDown)
	if got := p.Cursor.Format(time.DateOnly); got != "2027-02-28" {
		t.Errorf("31-ene + AvPág llega al %s", got)
	}
}

// TestConUnDiaEnfocadoSiguenLosAtajos: las letras de la app (`t`, `w`) llegan a la
// ventana con un día enfocado. VISTO EN ROJO con `TypedRune` sin subir la runa.
func TestConUnDiaEnfocadoSiguenLosAtajos(t *testing.T) {
	a, w := appTelefono(t)
	p := a.P
	p.AlElegirDia(time.Date(2026, 11, 20, 0, 0, 0, 0, time.UTC))
	w.Canvas().Focus(p.diaElegido)
	p.diaElegido.TypedRune('t')
	if !mismoDia(p.Cursor, ahoraCaptura) {
		t.Errorf("«t» con un día enfocado no llevó a hoy: %v", p.Cursor)
	}
	w.Canvas().Focus(p.diaElegido)
	p.diaElegido.TypedRune('w')
	if p.Vista != VistaSemana {
		t.Errorf("«w» con un día enfocado no cambió a la semana: vista %d", p.Vista)
	}
}

// deslizar arrastra sobre el mes desde x con dx en cuatro pasos y suelta.
func deslizar(m *mesCompacto, x, dx float32) {
	pos := fyne.NewPos(x, 100)
	for range 4 {
		pos = pos.Add(fyne.NewDelta(dx/4, 0))
		m.Dragged(&fyne.DragEvent{PointEvent: fyne.PointEvent{Position: pos}, Dragged: fyne.NewDelta(dx/4, 0)})
	}
	m.DragEnd()
}

// TestDeslizarSobreLosDiasCambiaDeMes: con el dedo y empezando sobre los días, a la
// izquierda es el mes siguiente (plegado, la semana siguiente); en la columna de semana,
// o con el ratón, nada. VISTOS EN ROJO sin mirar dónde empieza y sin mirar el dedo.
func TestDeslizarSobreLosDiasCambiaDeMes(t *testing.T) {
	a, w := appTelefono(t)
	p := a.P
	antes := conDedo
	t.Cleanup(func() { conDedo = antes })

	conDedo = func() bool { return false }
	deslizar(mesDe(t, w.Content()), 120, -120)
	if p.Cursor.Month() != time.November {
		t.Errorf("con el ratón cambió de mes: %v", p.Cursor)
	}
	conDedo = func() bool { return true }
	deslizar(mesDe(t, w.Content()), 30, -120)
	if p.Cursor.Month() != time.November {
		t.Errorf("empezando en la columna de semana cambió de mes: %v", p.Cursor)
	}
	deslizar(mesDe(t, w.Content()), 120, -120)
	if got := p.Cursor.Format(time.DateOnly); got != "2026-12-06" {
		t.Errorf("a la izquierda llega al %s y es el 6 de diciembre", got)
	}
	deslizar(mesDe(t, w.Content()), 120, 120)
	p.mesPlegado = true
	a.pintar()
	deslizar(mesDe(t, w.Content()), 120, -120)
	if got := p.Cursor.Format(time.DateOnly); got != "2026-11-13" {
		t.Errorf("plegado, a la izquierda llega al %s y es la semana siguiente (13-nov)", got)
	}
}

// TestElAsaPliegaALaSemanaYVuelve: plegado queda una fila con los días del mes vecino
// (V14 pintado: desde el 1-nov se ven del 26 al 31 de octubre) y el asa cambia de nombre;
// otra vez, el mes entero. VISTO EN ROJO sin conmutar `mesPlegado`.
func TestElAsaPliegaALaSemanaYVuelve(t *testing.T) {
	a, w := appTelefono(t)
	p := a.P
	p.AlElegirDia(time.Date(2026, 11, 1, 0, 0, 0, 0, time.UTC))
	m := mesDe(t, w.Content())
	if m.asa.Nombre != "Mostrar solo esta semana" {
		t.Errorf("desplegado, el asa se llama %q", m.asa.Nombre)
	}
	test.Tap(m.asa)
	m = mesDe(t, w.Content())
	if len(m.semanas) != 1 || m.semanas[0].Numero != 44 || m.asa.Nombre != "Mostrar el mes entero" {
		t.Fatalf("plegado: %d semanas (%v), asa %q", len(m.semanas), numeros(m.semanas), m.asa.Nombre)
	}
	if d := m.dias[0]; d == nil || d.d.Fecha.Day() != 26 || !d.d.OtroMes {
		t.Errorf("plegado, el lunes es %+v y es el 26 de octubre", d)
	}
	if h := m.Size().Height; h != AltoDiasCompacta+AltoSemanaCompacta+AltoAsaMes {
		t.Errorf("plegado, la hoja mide %v", h)
	}
	test.Tap(m.asa)
	if m = mesDe(t, w.Content()); len(m.semanas) != 6 {
		t.Errorf("desplegado otra vez: %d semanas", len(m.semanas))
	}
}

// TestElMesDelTelefonoSoloPorDebajoDe640: a 640 de ventana, o en la semana, la pantalla
// es la de siempre. VISTO EN ROJO con `<=` en vez de `<` en `compacta`.
func TestElMesDelTelefonoSoloPorDebajoDe640(t *testing.T) {
	p := pantallaCaptura(t)
	if !p.compacta() {
		t.Fatal("a 390 en el mes tiene que ser compacto")
	}
	p.Ventana = ui.CorteSM
	if p.compacta() {
		t.Error("a 640 no es compacto")
	}
	p.Ventana, p.Vista = 390, VistaSemana
	if p.compacta() {
		t.Error("en la semana no es compacto")
	}
	p.Ventana, p.Vista = 0, VistaMes
	if p.compacta() {
		t.Error("sin medir la ventana no es compacto")
	}
}

// TestCruzarElCortePideRehacer: el armazón mide la ventana y, si cruza 640, la pantalla
// pide reconstruirse; dentro del mismo lado, no. VISTO EN ROJO pidiéndolo en cada medida.
func TestCruzarElCortePideRehacer(t *testing.T) {
	p := pantallaCaptura(t)
	n := 0
	p.AlRehacer = func() { n++ }
	p.medirVentana(380)
	p.medirVentana(1280)
	p.medirVentana(1000)
	if n != 1 || p.Ventana != 1000 {
		t.Errorf("AlRehacer %d veces (quiero 1) y ventana %v", n, p.Ventana)
	}
}

// TestElCajonDelTelefonoLlevaLoDeLaCabecera: la vista, buscar, recordatorios (una rama
// con su contador que se despliega sin cerrar el cajón) e invitaciones con el suyo; el
// enlace de un recordatorio es un destino y cierra el cajón. VISTO EN ROJO sin cerrar
// el cajón en `saltarA`. (Que la rama no cierre el cajón lo vigila ui.TestRamaDesplegable.)
func TestElCajonDelTelefonoLlevaLoDeLaCabecera(t *testing.T) {
	a, w := appTelefono(t)
	p := a.P
	p.Recordatorios = []api.Reminder{
		{ID: "r1", Summary: "Tenis", DTStart: time.Date(2026, 11, 6, 13, 0, 0, 0, time.UTC)},
		{ID: "r2", Summary: "Dentista", DTStart: time.Date(2026, 11, 9, 8, 0, 0, 0, time.UTC)},
	}
	p.Invitaciones = 1
	a.recCargando = false
	p.cajonAbierto = true
	a.pintar()
	raiz := w.Content()
	if buscarObjeto(raiz, func(o fyne.CanvasObject) bool { _, es := o.(*GrupoVista); return es }) == nil {
		t.Error("el cajón no lleva el selector de vista")
	}
	fila := func(rotulo string) *ui.FilaLateral {
		f, _ := buscarObjeto(w.Content(), func(o fyne.CanvasObject) bool {
			f, es := o.(*ui.FilaLateral)
			return es && f.Rotulo == rotulo
		}).(*ui.FilaLateral)
		if f == nil {
			t.Fatalf("no hay fila %q en el cajón", rotulo)
		}
		return f
	}
	fila("Buscar eventos")
	if f := fila("Recordatorios"); f.Contador != "2" || !f.MantieneCajon {
		t.Errorf("recordatorios: contador %q, mantiene el cajón %v", f.Contador, f.MantieneCajon)
	}
	if f := fila("Invitaciones"); f.Contador != "1" {
		t.Errorf("invitaciones: contador %q", f.Contador)
	}
	test.Tap(fila("Recordatorios"))
	if !p.recEnCajon || !p.cajonAbierto {
		t.Fatalf("pulsar recordatorios: desplegado %v, cajón abierto %v", p.recEnCajon, p.cajonAbierto)
	}
	if !textos(w.Content())["Dentista"] {
		t.Error("desplegado, el cajón no enseña los recordatorios")
	}
	a.saltarA(p.Recordatorios[1])
	if p.Vista != VistaDia || p.cajonAbierto {
		t.Errorf("el enlace del recordatorio deja la vista %d y el cajón abierto %v", p.Vista, p.cajonAbierto)
	}
}

// TestElMenuAvisaDeLoPendiente: con 2 recordatorios y 1 invitación el ☰ se llama «Abrir
// menú, 3 pendientes»; sin nada pendiente, el nombre de siempre. VISTO EN ROJO con
// `ui.openNav` en vez de `nav.menuPending`.
func TestElMenuAvisaDeLoPendiente(t *testing.T) {
	p := pantallaCaptura(t)
	p.Recordatorios = []api.Reminder{{ID: "a"}, {ID: "b"}}
	p.Invitaciones = 1
	if got := p.nuevaBarraMovil().Aviso; got != "Abrir menú, 3 pendientes" {
		t.Errorf("aviso %q", got)
	}
	p.Recordatorios, p.Invitaciones = nil, 0
	if got := p.nuevaBarraMovil().Aviso; got != "" {
		t.Errorf("sin pendientes el aviso es %q", got)
	}
}

// TestLosNombresDelDia: hoy y elegido van en el nombre, porque Fyne no tiene
// `aria-current` ni `aria-pressed`; un día con eventos dice cuántos. Y el refresco al
// tirar vale en el mes del teléfono. VISTOS EN ROJO sin «Hoy, …» en el nombre y con
// `Refresco.Puede` solo en la agenda.
func TestLosNombresDelDia(t *testing.T) {
	a, w := appTelefono(t)
	m := mesDe(t, w.Content())
	if got := diaN(t, m, 6).AccessibilityLabel(); got != "Hoy, viernes, 6 de noviembre, 1 evento, seleccionado" {
		t.Errorf("el 6: %q", got)
	}
	if got := diaN(t, m, 9).AccessibilityLabel(); got != "lunes, 9 de noviembre, 2 eventos" {
		t.Errorf("el 9: %q", got)
	}
	if got := diaN(t, m, 4).AccessibilityLabel(); got != "miércoles, 4 de noviembre" {
		t.Errorf("el 4: %q", got)
	}
	// La fila de la lista con el nombre del catálogo (`grid.blockAria`), no con una coma
	// escrita a mano. VISTO EN ROJO con `cuando + ", " + titulo`.
	fila, _ := buscarObjeto(w.Content(), func(o fyne.CanvasObject) bool { _, es := o.(*filaEventoDia); return es }).(*filaEventoDia)
	if fila == nil {
		t.Fatal("no hay filas en la lista del día")
	}
	if fila.Nombre != "Tenis, 14:00 – 15:00" {
		t.Errorf("la fila de la lista se llama %q", fila.Nombre)
	}
	a.Cal = api.NewCalendars("http://127.0.0.1:9", nil, nil)
	if !a.P.Refresco.Puede() {
		t.Error("en el mes del teléfono se puede tirar para refrescar")
	}
}

// TestElSelectorDelCajonCabeEnTodosLosIdiomas: en el cajón del teléfono el selector de
// vista es una rejilla de 2×2 que llena el ancho del cajón, y en ninguno de los 26
// idiomas se trunca un rótulo; tampoco en los cinco en los que en una fila desbordaba
// (de, fi, el, bg, lt). Es la pareja del banco e2e de la web.
// VISTO EN ROJO con `grupo.Rejilla = false` en menu.go: en una fila a su ancho natural
// el grupo de finés mide más que el cajón y el último botón queda fuera.
func TestElSelectorDelCajonCabeEnTodosLosIdiomas(t *testing.T) {
	for _, idioma := range i18n.Locales {
		t.Run(string(idioma), func(t *testing.T) {
			tr, err := i18n.New(idioma, i18n.Common(), catalogos.Calendar())
			if err != nil {
				t.Fatal(err)
			}
			if idioma == "el" || idioma == "lt" || idioma == "fi" {
				tema.Elegir("oscuro")
				t.Cleanup(func() { tema.Elegir("") })
			}
			a, w := appTelefono(t)
			a.P.T = tr
			a.P.cajonAbierto = true
			a.pintar()
			if idioma == "el" || idioma == "lt" || idioma == "fi" {
				guardar(t, w.Canvas().Capture(), "calendario-mes-movil-cajon-"+string(idioma)+"-oscuro")
			}
			g, _ := buscarObjeto(w.Content(), func(o fyne.CanvasObject) bool { _, es := o.(*GrupoVista); return es }).(*GrupoVista)
			if g == nil {
				t.Fatal("el cajón no lleva el selector de vista")
			}
			fila, _ := buscarObjeto(w.Content(), func(o fyne.CanvasObject) bool {
				f, es := o.(*ui.FilaLateral)
				return es && f.Rotulo == tr.T("search.events", nil)
			}).(*ui.FilaLateral)
			if fila == nil {
				t.Fatal("no hay fila de buscar en el cajón")
			}
			// La sección no crece por el grupo: mide lo útil del cajón, y el grupo la llena
			// menos su `px-1`. (Antes el grupo ensanchaba la sección más allá del cajón,
			// que recorta.)
			gx := g.Position().X + g.Size().Width
			fx := fila.Size().Width
			if fx > ui.AnchoUtilMenu {
				t.Errorf("la sección mide %v y el cajón deja %v", fx, ui.AnchoUtilMenu)
			}
			if gx > fx || g.Size().Width < fx-2*tema.Espacio(1)-1 {
				t.Errorf("el grupo mide %v y acaba en %v; la fila mide %v", g.Size().Width, gx, fx)
			}
			n := 0
			buscarObjeto(g, func(o fyne.CanvasObject) bool {
				b, es := o.(*botonVista)
				if !es {
					return false
				}
				n++
				if b.Position().X+b.Size().Width > g.Size().Width {
					t.Errorf("%q se sale del grupo", b.AccessibilityLabel())
				}
				if !b.CabeRotulo() {
					t.Errorf("%q no cabe en su botón de %v", b.AccessibilityLabel(), b.Size().Width)
				}
				return false
			})
			if n != 4 {
				t.Errorf("%d botones de vista", n)
			}
		})
	}
}

// TestSiguienteYAnteriorDanElPasoDeLoQueSeVe: en el mes del teléfono `n`/`p` dan el
// mismo paso que deslizar: un mes recortando el día desplegado y una semana plegado.
// VISTO EN ROJO con `p.AlNavegar` en el atajo (plegado, `n` saltaba un mes entero).
func TestSiguienteYAnteriorDanElPasoDeLoQueSeVe(t *testing.T) {
	a, _ := appTelefono(t)
	p := a.P
	p.AlElegirDia(time.Date(2027, 1, 31, 0, 0, 0, 0, time.UTC))
	a.Atajo('n')
	if !mismoDia(p.Cursor, time.Date(2027, 2, 28, 0, 0, 0, 0, time.UTC)) {
		t.Errorf("desplegado, «n» desde el 31-ene lleva a %v", p.Cursor)
	}
	p.conmutarPlegado()
	a.Atajo('n')
	if !mismoDia(p.Cursor, time.Date(2027, 3, 7, 0, 0, 0, 0, time.UTC)) {
		t.Errorf("plegado, «n» desde el 28-feb lleva a %v", p.Cursor)
	}
	a.Atajo('p')
	a.Atajo('p')
	if !mismoDia(p.Cursor, time.Date(2027, 2, 21, 0, 0, 0, 0, time.UTC)) {
		t.Errorf("plegado, «p» dos veces desde el 7-mar lleva a %v", p.Cursor)
	}
}

// TestElFocoVuelveTrasRepintar: repintar rehace el árbol y el foco no se queda en un
// widget que ya no está. Plegar con el asa enfocada deja el foco en el asa NUEVA; elegir
// con Intro, o `t` con un día enfocado, lo deja en el día elegido, y la flecha siguiente
// mueve desde ahí. VISTO EN ROJO sin recolocar el foco en `pintar` (se queda en el asa
// vieja, y tras `t` desde el 20 la → lleva al 21 y no al 7).
func TestElFocoVuelveTrasRepintar(t *testing.T) {
	a, w := appTelefono(t)
	p := a.P
	c := w.Canvas()
	asa := mesDe(t, w.Content()).asa
	c.Focus(asa)
	asa.TypedRune(' ')
	if !p.mesPlegado || c.Focused() != mesDe(t, w.Content()).asa || c.Focused() == asa {
		t.Errorf("tras plegar con Espacio el foco está en %T (asa nueva %v)", c.Focused(), c.Focused() == mesDe(t, w.Content()).asa)
	}
	p.conmutarPlegado()

	p.AlElegirDia(time.Date(2026, 11, 20, 0, 0, 0, 0, time.UTC))
	c.Focus(p.diaElegido)
	p.diaElegido.TypedRune('t')
	if c.Focused() != p.diaElegido {
		t.Fatalf("tras «t» el foco está en %T y no en el día elegido", c.Focused())
	}
	c.Focused().TypedKey(&fyne.KeyEvent{Name: fyne.KeyRight})
	if got := p.Cursor.Format(time.DateOnly); got != "2026-11-07" {
		t.Errorf("→ tras «t» llega al %s y es el 7 (desde hoy)", got)
	}

	m := mesDe(t, w.Content())
	trece := diaN(t, m, 13)
	c.Focus(trece)
	trece.TypedKey(&fyne.KeyEvent{Name: fyne.KeyReturn})
	if !mismoDia(p.Cursor, time.Date(2026, 11, 13, 0, 0, 0, 0, time.UTC)) || c.Focused() != p.diaElegido || c.Focused() == trece {
		t.Errorf("Intro sobre el 13: cursor %v, foco en el día nuevo %v", p.Cursor, c.Focused() == p.diaElegido)
	}
}

// TestElPrimerPintadoDelTelefonoYaEsCompacto: con la ventana sembrada (`VentanaInicial`)
// el primer Construir de un teléfono en vertical es el mes compacto, sin cabecera de
// vista; la medida de verdad solo corrige, y pide rehacer solo si cruza 640 (una
// tableta). Fuera del móvil, o en horizontal, no se siembra nada. VISTO EN ROJO con
// `VentanaInicial` devolviendo siempre 0 (el primer árbol traía la cabecera).
func TestElPrimerPintadoDelTelefonoYaEsCompacto(t *testing.T) {
	if VentanaInicial(false, true) != 0 || VentanaInicial(true, false) != 0 {
		t.Error("fuera de un móvil en vertical no se siembra la ventana")
	}
	p := pantallaCaptura(t)
	p.Ventana = VentanaInicial(true, true)
	raiz := p.Construir()
	mesDe(t, raiz)
	if buscarObjeto(raiz, func(o fyne.CanvasObject) bool { _, es := o.(*CabeceraVista); return es }) != nil {
		t.Error("el primer pintado del teléfono trae la cabecera de escritorio")
	}
	n := 0
	p.AlRehacer = func() { n++ }
	p.medirVentana(375)
	if n != 0 {
		t.Error("medir un teléfono tras sembrarlo pidió rehacer")
	}
	p.medirVentana(768)
	if n != 1 {
		t.Errorf("una tableta en vertical pide rehacer %d veces (quiero 1)", n)
	}
}

// TestElTituloDeSeptiembreA320: a 320 (iPhone SE), con el cursor en otro año, el título
// del mes en griego sale entero, con su año: no cabe centrado ni en `text-md`, así que
// usa el hueco entre ☰ y las acciones en `text-sm` (ui.TestElTituloLargoUsaElHuecoAntesDeTruncar
// vigila la regla; esto, que Calendar la recibe). Y en nominativo, «Σεπτέμβριος».
func TestElTituloDeSeptiembreA320(t *testing.T) {
	tema.Elegir("oscuro")
	t.Cleanup(func() { tema.Elegir("") })
	tr, err := i18n.New("el", i18n.Common(), catalogos.Calendar())
	if err != nil {
		t.Fatal(err)
	}
	a, w := appTelefono(t)
	a.P.T = tr
	a.P.AlElegirDia(time.Date(2027, 9, 6, 0, 0, 0, 0, time.UTC))
	w.Resize(fyne.NewSize(320, 640))
	a.pintar()
	guardar(t, w.Canvas().Capture(), "calendario-mes-movil-sep2027-el-320")
	b := barraDe(t, w.Content())
	r := test.WidgetRenderer(b)
	var titulo *canvas.Text
	for _, o := range r.Objects() {
		if tx, es := o.(*canvas.Text); es && tx.Text != "" {
			titulo = tx
		}
	}
	if titulo == nil || titulo.Text != "Σεπτέμβριος 2027" {
		t.Errorf("a 320 el título no es «Σεπτέμβριος 2027» entero")
	}
}

// TestElFocoEsUnAroAbierto: los tres controles del mes del teléfono —el día, el asa y una
// fila de la lista— no tienen caja opaca en reposo, así que su aro es ABIERTO (ui/foco.go,
// punto 3): enfocados, dentro sigue la superficie y la franja cae FUERA, 2 px alrededor,
// como el `sytue-focus-ring` de la web. VISTO EN ROJO sin `.Abierto()` en los tres: el día
// era un bloque de focus-ring de 54×40 con la caja primaria dentro, el asa una franja de
// 390×32 y la fila quedaba tapada entera.
func TestElFocoEsUnAroAbierto(t *testing.T) {
	t.Cleanup(func() { tema.Elegir("") })
	for _, modo := range []string{"oscuro", "claro"} {
		tema.Elegir(modo)
		v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
		superficie, anillo := tema.Color("surface", v), tema.Color("focus-ring", v)
		a, w := appTelefono(t)
		c := w.Canvas()
		fila, _ := buscarObjeto(w.Content(), func(o fyne.CanvasObject) bool { _, es := o.(*filaEventoDia); return es }).(*filaEventoDia)
		if fila == nil {
			t.Fatal("no hay fila de evento en la lista del 6")
		}
		for _, caso := range []struct {
			nombre string
			obj    fyne.Focusable
			dentro fyne.Position // respecto a la esquina del control
		}{
			// Dentro de la celda, fuera de la caja de 28 y de la franja del aro.
			{"dia", a.P.diaElegido, fyne.NewPos(2, 20)},
			// Lejos de la barrita del centro.
			{"asa", mesDe(t, w.Content()).asa, fyne.NewPos(8, 4)},
			// A la izquierda del punto de color.
			{"fila", fila, fyne.NewPos(4, 4)},
		} {
			c.Focus(caso.obj)
			obj := caso.obj.(fyne.CanvasObject)
			img := c.Capture()
			guardar(t, img, "calendario-mes-movil-foco-"+caso.nombre+"-"+modo)
			p := fyne.CurrentApp().Driver().AbsolutePositionForObject(obj)
			if got := px(img, int(p.X+caso.dentro.X), int(p.Y+caso.dentro.Y)); !mismoColor(got, superficie) {
				t.Errorf("%s, %s enfocado: dentro hay %v y es la superficie %v (¿aro relleno?)", modo, caso.nombre, got, superficie)
			}
			// Y el aro se ve: la franja de arriba, 1 px por encima de la caja.
			if got := px(img, int(p.X+obj.Size().Width/2), int(p.Y)-1); !mismoColor(got, anillo) {
				t.Errorf("%s, %s enfocado: encima hay %v y es el anillo %v", modo, caso.nombre, got, anillo)
			}
		}
	}
}

// TestLaCabeceraYLaListaVanEnLaZonaDeLaPantalla: con una zona que no es la del cursor, el
// título de la barra, la fecha de la lista, el nombre del «+» y la rejilla dicen el MISMO
// día. 30-nov-2026 23:30Z es el 1 de diciembre en Madrid. VISTO EN ROJO sin el `.In` en
// `listaDia` (la lista decía el 30 de noviembre bajo la rejilla de diciembre) y en
// `TituloCompacto` («noviembre»).
func TestLaCabeceraYLaListaVanEnLaZonaDeLaPantalla(t *testing.T) {
	p := pantallaCaptura(t)
	p.Zona = madrid(t)
	p.Cursor = time.Date(2026, 11, 30, 23, 30, 0, 0, time.UTC)
	w, _ := telefono(t, p)
	raiz := w.Content()
	if d := p.diaElegido; d == nil || d.d.Fecha.Month() != time.December || d.d.Fecha.Day() != 1 {
		t.Fatalf("la rejilla no elige el 1 de diciembre")
	}
	b := barraDe(t, raiz)
	if b.Titulo != "diciembre" {
		t.Errorf("el título de la barra es %q y es «diciembre»", b.Titulo)
	}
	if tx := textos(raiz); !tx["martes, 1 de diciembre de 2026"] {
		t.Errorf("la lista no dice «martes, 1 de diciembre de 2026»")
	}
	mas := b.Acciones[1].(*ui.BotonIcono)
	if want := p.t("nav.createEventOn", i18n.Vars{"date": i18n.FormatDayMonthLong(p.locale(), time.Date(2026, 12, 1, 0, 0, 0, 0, time.UTC))}); mas.Rotulo != want {
		t.Errorf("el «+» se llama %q y es %q", mas.Rotulo, want)
	}
}

// TestUnDiaNoEsShortcutable vigila la premisa de `diaCompacto.TypedKey`: como el día no
// implementa `fyne.Shortcutable`, el driver de escritorio se queda Ctrl/Alt/Super + tecla
// (un `desktop.CustomShortcut` para el lienzo) y TypedKey no la ve, igual que la web
// ignora las teclas con modificador. Si un día lo implementa, TypedShortcut tiene que
// tragarse las flechas, Inicio, Fin y RePág/AvPág con modificador.
func TestUnDiaNoEsShortcutable(t *testing.T) {
	if _, es := any(&diaCompacto{}).(fyne.Shortcutable); es {
		t.Error("diaCompacto implementa Shortcutable: revisa qué hace con las teclas con modificador")
	}
}
