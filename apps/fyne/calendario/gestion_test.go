// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package calendario

import (
	"context"
	"strings"
	"testing"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/test"

	"git.sytue.eu/sytue/one/apps/nucleo/api"

	"git.sytue.eu/sytue/one/apps/fyne/tema"
	"git.sytue.eu/sytue/one/apps/fyne/ui"
)

// recorrer baja por contenedores y widgets llamando a `f` con cada objeto.
func recorrer(o fyne.CanvasObject, f func(fyne.CanvasObject)) {
	f(o)
	if v, ok := o.(*fyne.Container); ok {
		for _, h := range v.Objects {
			recorrer(h, f)
		}
		return
	}
	if w, ok := o.(fyne.Widget); ok {
		for _, h := range test.WidgetRenderer(w).Objects() {
			recorrer(h, f)
		}
	}
}

// enCapas recorre todo lo que haya en las superposiciones del lienzo.
func enCapas(c fyne.Canvas, f func(fyne.CanvasObject)) {
	for _, capa := range c.Overlays().List() {
		recorrer(capa, f)
	}
}

// elementosMenu son los rótulos del menú desplegado, en orden, con «—» por separador.
func elementosMenu(c fyne.Canvas) []string {
	var out []string
	enCapas(c, func(o fyne.CanvasObject) {
		switch e := o.(type) {
		case *ui.ElementoMenu:
			out = append(out, e.Texto)
		case *ui.SeparadorMenu:
			out = append(out, "—")
		}
	})
	return out
}

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

// TestElMenuDeUnCalendarioEsElDeLaWeb: el propio por defecto no se borra; una
// suscripción se refresca y se «cancela», no se «elimina».
func TestElMenuDeUnCalendarioEsElDeLaWeb(t *testing.T) {
	a, w := appConVentana(t, &escrituras{})
	p := a.P
	propio := api.Calendar{ID: "mio", DisplayName: "Personal", IsDefault: true}
	a.menuCalendario(propio, a.raiz)
	quiero := []string{p.t("nav.rename", nil), p.t("nav.changeColour", nil), p.t("cal.share", nil), p.t("cal.downloadIcs", nil)}
	if got := elementosMenu(w.Canvas()); !iguales(got, quiero) {
		t.Errorf("menú del calendario por defecto: %q, y la web da %q", got, quiero)
	}
	ui.Descartar(w.Canvas())
	sus := api.Calendar{ID: "fest", DisplayName: "Festivos", ReadOnly: true}
	a.menuCalendario(sus, a.raiz)
	got := elementosMenu(w.Canvas())
	if len(got) < 3 || got[0] != p.t("cal.refresh", nil) || got[1] != "—" || got[len(got)-1] != p.t("cal.sub.delete", nil) {
		t.Errorf("menú de una suscripción: %q", got)
	}
}

// TestCrearUnCalendarioPideSuNombre: sin nombre avisa junto al campo; con él da de alta
// con el primer tono de la paleta.
func TestCrearUnCalendarioPideSuNombre(t *testing.T) {
	s := &escrituras{}
	a, w := appConVentana(t, s)
	a.formularioCalendario(nil)
	var nombre *ui.Entrada
	var guardar *ui.Boton
	enCapas(w.Canvas(), func(o fyne.CanvasObject) {
		switch v := o.(type) {
		case *ui.Entrada:
			nombre = v
		case *ui.Boton:
			if v.Texto == a.P.t("common.save", nil) {
				guardar = v
			}
		}
	})
	if nombre == nil || guardar == nil {
		t.Fatal("no está el formulario del calendario")
	}
	guardar.Tapped(nil)
	avisa := false
	enCapas(w.Canvas(), func(o fyne.CanvasObject) {
		if r, ok := o.(*ui.Rotulo); ok && r.Texto == a.P.t("calendar.nameRequired", nil) {
			avisa = true
		}
	})
	if !avisa || !nombre.Invalida {
		t.Error("sin nombre no avisa junto al campo")
	}
	nombre.PonerTexto("Equipo")
	guardar.Tapped(nil)
	hechas, cuerpos := s.apuntadas()
	if len(hechas) != 1 || hechas[0] != "POST /api/v1/calendars" || cuerpos[0]["display_name"] != "Equipo" || cuerpos[0]["color"] != PaletaCalendario[0].Hex {
		t.Errorf("alta: %v %v", hechas, cuerpos)
	}
}

// TestLaBandejaContestaYBajaElContador: aceptar una invitación la quita de la lista y
// del contador de la cabecera.
func TestLaBandejaContestaYBajaElContador(t *testing.T) {
	s := &escrituras{}
	a, w := appConVentana(t, s)
	if a.P.Invitaciones != 1 {
		t.Fatalf("el contador arranca en %d", a.P.Invitaciones)
	}
	a.abrirInvitaciones()
	var aceptar *ui.Boton
	enCapas(w.Canvas(), func(o fyne.CanvasObject) {
		if b, ok := o.(*ui.Boton); ok && b.Texto == a.P.t("invitations.accept", nil) {
			aceptar = b
		}
	})
	if aceptar == nil {
		t.Fatal("no está «Aceptar»")
	}
	guardar(t, w.Canvas().Capture(), "calendario-invitaciones")
	aceptar.Tapped(nil)
	if hechas, _ := s.apuntadas(); len(hechas) != 1 || hechas[0] != "POST /api/v1/invitations/i1/respond" {
		t.Errorf("responder hizo %v", hechas)
	}
	if a.P.Invitaciones != 0 {
		t.Errorf("tras aceptar el contador sigue en %d", a.P.Invitaciones)
	}
}

// TestLosRecordatoriosSeAnuncianUnaVezYSeDescartan: cada recordatorio nuevo saca UN
// aviso; la ronda siguiente no lo repite; descartarlo lo quita ya, se lo dice al
// servicio, y la ronda siguiente no lo resucita.
func TestLosRecordatoriosSeAnuncianUnaVezYSeDescartan(t *testing.T) {
	s := &escrituras{}
	a, w := appConVentana(t, s)
	fyne.CurrentApp().Preferences().RemoveValue(preferenciaAnunciados)
	r := api.Reminder{ID: "r1", Summary: "Comité", DTStart: time.Date(2026, 9, 22, 9, 30, 0, 0, time.UTC)}
	antes := len(a.avisos.Avisos())
	a.llegaronRecordatorios([]api.Reminder{r}, nil)
	a.llegaronRecordatorios([]api.Reminder{r}, nil)
	if n := len(a.avisos.Avisos()) - antes; n != 1 {
		t.Errorf("dos rondas con el mismo recordatorio sacan %d avisos", n)
	}
	if len(a.P.Recordatorios) != 1 {
		t.Fatalf("la campana cuenta %d", len(a.P.Recordatorios))
	}
	var campana *botonCampana
	recorrer(a.raiz, func(o fyne.CanvasObject) {
		if c, ok := o.(*botonCampana); ok {
			campana = c
		}
	})
	if campana == nil || campana.n != 1 {
		t.Fatalf("la campana de la cabecera no cuenta el recordatorio: %+v", campana)
	}
	a.abrirRecordatorios(campana)
	guardar(t, w.Canvas().Capture(), "calendario-recordatorios")
	a.descartar(r)
	if len(a.P.Recordatorios) != 0 {
		t.Error("descartado y sigue en la campana")
	}
	if hechas, _ := s.apuntadas(); len(hechas) != 1 || hechas[0] != "POST /api/v1/reminders/r1/dismiss" {
		t.Errorf("descartar hizo %v", hechas)
	}
	a.llegaronRecordatorios([]api.Reminder{r}, nil)
	if len(a.P.Recordatorios) != 0 {
		t.Error("el descartado vuelve con la ronda siguiente")
	}
}

// TestLaBusquedaDiceQueFaltaUnaLetra: con menos de dos letras no pregunta al servicio y
// lo dice con la clave del catálogo; sin resultados, con el término dentro.
func TestLaBusquedaDiceQueFaltaUnaLetra(t *testing.T) {
	p := &Pantalla{T: traductor(t), Zona: time.UTC}
	w := test.NewWindow(p.cuerpoBusqueda("a", nil, "", false, false, nil, nil, nil))
	defer w.Close()
	hay := func(o fyne.CanvasObject, texto string) bool {
		encontrado := false
		recorrer(o, func(x fyne.CanvasObject) {
			if par, ok := x.(*ui.Parrafo); ok && par.Texto == texto {
				encontrado = true
			}
		})
		return encontrado
	}
	if o := p.cuerpoBusqueda("a", nil, "", false, false, nil, nil, nil); !hay(o, p.t("search.minChars", map[string]any{"count": MinBusqueda})) {
		t.Error("con una letra no dice que faltan")
	}
	if o := p.cuerpoBusqueda("zz", nil, "", false, false, nil, nil, nil); !hay(o, p.t("search.noResults", map[string]any{"term": "zz"})) {
		t.Error("sin resultados no lo dice con el término")
	}
}

// TestEnElTelefonoLaCabeceraSeDesplazaYSaleElBotonFlotante: a 390 de ancho —el teléfono
// medido el 2026-10-04— la cabecera conserva una fila de 53 y desplaza de lado,
// el menú se va a un cajón que saca la hamburguesa, y abajo a la derecha está el botón
// flotante de «Crear evento» en `primary`.
func TestEnElTelefonoLaCabeceraSeDesplazaYSaleElBotonFlotante(t *testing.T) {
	a, w := appConVentana(t, &escrituras{})
	// Como en la captura de producción: sin invitaciones pendientes, que meten su
	// insignia dentro del botón y lo ensanchan.
	a.P.Invitaciones = 0
	a.pintar()
	w.Resize(fyne.NewSize(390, 740))
	var cab *CabeceraVista
	recorrer(a.raiz, func(o fyne.CanvasObject) {
		if c, ok := o.(*CabeceraVista); ok {
			cab = c
		}
	})
	if cab == nil {
		t.Fatal("no está la cabecera")
	}
	if got := cab.AltoPara(390); got != 53 {
		t.Errorf("a 390 la cabecera mide %v y CalendarApp real, 53 (una fila desplazable)", got)
	}
	if got := cab.AltoPara(1020); got != AltoCabeceraVista() {
		t.Errorf("a 1020 la cabecera mide %v y debería seguir en una fila de %v", got, AltoCabeceraVista())
	}
	r := test.WidgetRenderer(cab).(*cabeceraVistaRenderer)
	if r.tira == nil || r.tira.AnchoContenido() <= r.tira.Size().Width {
		t.Fatal("la cabecera no conserva los controles en una tira desplazable")
	}
	grupo := cab.Derecha[len(cab.Derecha)-1].(*GrupoVista)
	gr := test.WidgetRenderer(grupo).(*grupoVistaRenderer)
	gr.botones[3].FocusGained()
	if r.tira.Desplazamiento() <= 0 {
		t.Error("no se puede alcanzar el conmutador de vista")
	}
	img := w.Canvas().Capture()
	guardar(t, img, "calendario-movil")
	k := float32(img.Bounds().Dx()) / 390
	// En diagonal desde el centro, fuera de los trazos del «+».
	cx := int((390 - ui.MargenBotonFlotante - ui.LadoBotonFlotante/2 + 8) * k)
	cy := int((740 - ui.MargenBotonFlotante - ui.LadoBotonFlotante/2 + 8) * k)
	if got, quiero := px(img, cx, cy), tema.Color("primary", tema.Claro); !mismoColor(got, quiero) {
		t.Errorf("en el botón flotante (%d,%d) se pinta %v y `primary` es %v", cx, cy, got, quiero)
	}
	a.P.AlConmutarCajon()
	img = w.Canvas().Capture()
	guardar(t, img, "calendario-movil-cajon")
	if got, quiero := px(img, int(100*k), int(400*k)), tema.Color("sidebar-bg", tema.Claro); !mismoColor(got, quiero) {
		t.Errorf("con el cajón abierto (100,400) es %v y el menú es %v", got, quiero)
	}
	// Elegir un día en el mini-mes cierra el cajón, como en la web.
	a.P.AlElegirDia(time.Date(2026, 9, 24, 0, 0, 0, 0, time.UTC))
	if a.P.cajonAbierto {
		t.Error("elegir un día no cerró el cajón")
	}
}

// TestLaCabeceraNoRecortaHoyConControlesDe44 reproduce los mínimos de iOS sin
// falsear el dispositivo: el pintor de pruebas corre en Linux y ui.EsTactil consulta
// GOOS. La talla LG da a las flechas el mismo mínimo real de 44 que SM tiene en iOS.
// La tira fija de 32 colocaba el grupo en Y=-6 y perdía la parte superior de «Hoy».
func TestLaCabeceraNoRecortaHoyConControlesDe44(t *testing.T) {
	t.Cleanup(func() { tema.Elegir("") })
	for _, modo := range []string{"claro", "traslucido", "oscuro"} {
		tema.Elegir(modo)
		for _, ancho := range []float32{320, 375, 390} {
			pulsado := false
			p := &Pantalla{T: traductor(t), Cursor: time.Date(2026, 9, 22, 12, 0, 0, 0, time.UTC), Zona: time.UTC}
			p.AlIrHoy = func() { pulsado = true }
			cab := p.cabecera()
			for _, o := range append(append([]fyne.CanvasObject{}, cab.Izquierda...), cab.Derecha...) {
				if b, ok := o.(*ui.Boton); ok {
					b.Talla = ui.TallaLG
				}
			}
			if got := cab.AltoPara(ancho); got != 65 {
				t.Errorf("%s a %g: la cabecera mide %g; un control de44 con py10 y borde1 necesita65", modo, ancho, got)
			}
			w := test.NewWindow(cab)
			w.SetPadded(false)
			w.Resize(fyne.NewSize(ancho, cab.AltoPara(ancho)))
			r := test.WidgetRenderer(cab).(*cabeceraVistaRenderer)
			if r.tira == nil {
				w.Close()
				t.Fatal("falta la tira móvil")
			}
			for _, o := range r.tira.Objetos {
				if o.Position().Y < 0 || o.Position().Y+o.Size().Height > r.tira.Size().Height {
					t.Errorf("%s a %g: %T ocupa Y%g..%g fuera de la tira de%g", modo, ancho, o,
						o.Position().Y, o.Position().Y+o.Size().Height, r.tira.Size().Height)
				}
			}
			hoy := cab.Izquierda[0].(*botonInvitaciones)
			grupoIzq := r.tira.Objetos[0].(*fyne.Container)
			for _, o := range grupoIzq.Objects {
				if o.Position().Y < 0 || o.Position().Y+o.Size().Height > grupoIzq.Size().Height {
					t.Errorf("%s a %g: el grupo izquierdo recorta %T", modo, ancho, o)
				}
			}
			grupoVista := cab.Derecha[len(cab.Derecha)-1].(*GrupoVista)
			test.WidgetRenderer(grupoVista).(*grupoVistaRenderer).botones[3].FocusGained()
			if r.tira.Desplazamiento() <= 0 {
				t.Error("no se alcanza la vista Agenda")
			}
			hoy.FocusGained()
			if r.tira.Desplazamiento() != 0 {
				t.Error("volver a Hoy no enseña entero el primer grupo")
			}
			test.Tap(hoy)
			if !pulsado {
				t.Error("Hoy ha perdido su acción")
			}
			guardar(t, w.Canvas().Capture(), "calendario-cabecera-44-"+modo+"-"+contar(int(ancho)))
			// Al volver a una ventana ancha, los callbacks no deben mover los
			// controles con las coordenadas de la tira que ya no está en el árbol.
			cab.Resize(fyne.NewSize(1020, cab.AltoPara(1020)))
			posHoy := hoy.Position()
			buscar := cab.Derecha[0].(*ui.Boton)
			posBuscar := buscar.Position()
			grupoVista.AlEnfocar()
			hoy.FocusGained()
			buscar.FocusGained()
			if hoy.Position() != posHoy || buscar.Position() != posBuscar {
				t.Error("el foco mueve los controles de escritorio con el reparto de la tira móvil")
			}
			w.Close()
		}
	}
}

// TestCompartirEnseñaConQuienYComparte: la lista de quién lo ve, con su permiso, y
// «Compartir» con un correo que no vale avisa sin llamar al servicio; con uno que vale,
// comparte con el nivel elegido.
func TestCompartirEnsenaConQuienYComparte(t *testing.T) {
	s := &escrituras{}
	a, w := appConVentana(t, s)
	a.compartir(api.Calendar{ID: "mio", DisplayName: "Personal"})
	var correo *ui.Entrada
	var compartir *ui.Boton
	var editar *ui.TarjetaEleccion
	hayAna := false
	enCapas(w.Canvas(), func(o fyne.CanvasObject) {
		switch v := o.(type) {
		case *ui.Entrada:
			correo = v
		case *ui.Boton:
			if v.Texto == a.P.t("share.submit", nil) {
				compartir = v
			}
		case *ui.TarjetaEleccion:
			if v.Titulo == a.P.t("share.writerLabel", nil) {
				editar = v
			}
		}
	})
	enCapas(w.Canvas(), func(o fyne.CanvasObject) {
		if l, ok := o.(*fyne.Container); ok {
			if f, ok := l.Layout.(*filaCompartidoLayout); ok && f.correo == "ana@sytue.eu" {
				hayAna = true
			}
		}
	})
	if correo == nil || compartir == nil || editar == nil {
		t.Fatal("no está el formulario de compartir")
	}
	if !hayAna {
		t.Error("no sale con quién está compartido")
	}
	guardar(t, w.Canvas().Capture(), "calendario-compartir")
	// El `md` mide 560 y va centrado: si algo del cuerpo no envuelve, «Compartir» se
	// va fuera por la derecha y el diálogo lo recorta.
	pos := fyne.CurrentApp().Driver().AbsolutePositionForObject(compartir)
	if derecha := (w.Canvas().Size().Width + 560) / 2; pos.X+compartir.Size().Width > derecha {
		t.Errorf("«Compartir» acaba en x=%v, fuera del diálogo (%v)", pos.X+compartir.Size().Width, derecha)
	}
	correo.PonerTexto("no-es-un-correo")
	compartir.Tapped(nil)
	if hechas, _ := s.apuntadas(); len(hechas) != 0 || !correo.Invalida {
		t.Errorf("con un correo que no vale: %v, inválido=%v", hechas, correo.Invalida)
	}
	editar.Tapped(nil)
	correo.PonerTexto("bea@sytue.eu")
	compartir.Tapped(nil)
	hechas, cuerpos := s.apuntadas()
	if len(hechas) != 1 || hechas[0] != "POST /api/v1/calendars/mio/shares" || cuerpos[0]["permission"] != "writer" {
		t.Errorf("compartir hizo %v %v", hechas, cuerpos)
	}
}

func TestIniciosCandidatosComoLaWeb(t *testing.T) {
	h := func(desde, hasta string) api.Hueco {
		a, _ := time.Parse(time.RFC3339, desde)
		b, _ := time.Parse(time.RFC3339, hasta)
		return api.Hueco{Start: a, End: b}
	}
	// Tres horas libres: caben seis inicios de media en media hora, pero van cuatro.
	largo := h("2031-03-04T07:00:00Z", "2031-03-04T10:00:00Z")
	if got := IniciosCandidatos([]api.Hueco{largo}, time.Hour); len(got) != 4 || got[3].Hour() != 8 || got[3].Minute() != 30 {
		t.Errorf("un hueco largo da %v", got)
	}
	// Uno más corto que el evento no da ninguno; y en total no pasan de dieciséis.
	if got := IniciosCandidatos([]api.Hueco{h("2031-03-04T07:00:00Z", "2031-03-04T07:45:00Z")}, time.Hour); len(got) != 0 {
		t.Errorf("un hueco corto da %v", got)
	}
	if got := IniciosCandidatos([]api.Hueco{largo, largo, largo, largo, largo}, time.Hour); len(got) != 16 {
		t.Errorf("cinco huecos dan %d inicios", len(got))
	}
	if MinutosEntre(largo.Start, largo.Start.Add(10*time.Minute)) != 60 || MinutosEntre(largo.Start, largo.End) != 180 {
		t.Error("la duración no es la de durationMinutesOf")
	}
	madrid, _ := time.LoadLocation("Europe/Madrid")
	ini := time.Date(2031, 3, 4, 10, 0, 0, 0, madrid)
	desde, hasta := VentanaHuecos(ini, ini.Add(-48*time.Hour), 7, madrid)
	if !desde.Equal(time.Date(2031, 3, 4, 0, 0, 0, 0, madrid)) || !hasta.Equal(time.Date(2031, 3, 11, 0, 0, 0, 0, madrid)) {
		t.Errorf("ventana %v – %v", desde, hasta)
	}
	if desde, _ := VentanaHuecos(ini, ini, 7, madrid); !desde.Equal(ini) {
		t.Errorf("con el día ya empezado la ventana arranca en %v y no ahora", desde)
	}
	// Solo la primera letra: el mes en español va en minúscula.
	if got := capitalizar("mar, 4 mar"); got != "Mar, 4 mar" {
		t.Errorf("la cabecera del día da %q", got)
	}
}

// TestElFormularioSeRepartePorLoQueMideLaHoja: el cuerpo de un diálogo `md` se reparte
// al ancho donde está. Era un 510 fijo, y en la hoja de 390 las tarjetas de «Compartir»
// y las horas de «Buscar un hueco» se medían para 510 y se pintaban en 342.
func TestElFormularioSeRepartePorLoQueMideLaHoja(t *testing.T) {
	escritorio := 560 - 2*tema.Borde - 2*tema.Espacio(6)
	if got := anchoUtilFormulario(fyne.Size{}); got != escritorio {
		t.Errorf("sin lienzo, el del escritorio: %v", got)
	}
	if got := anchoUtilFormulario(fyne.NewSize(1280, 800)); got != escritorio {
		t.Errorf("en el escritorio: %v", got)
	}
	if got, want := anchoUtilFormulario(fyne.NewSize(390, 844)), 390-2*tema.Espacio(6); got != want {
		t.Errorf("en la hoja de 390: %v, y no %v", got, want)
	}
}

func TestBuscarUnHuecoFijaElEventoEnElHuecoElegido(t *testing.T) {
	s := &escrituras{}
	a, w := appConVentana(t, s)
	ini := time.Date(2031, 3, 4, 10, 0, 0, 0, time.UTC)
	a.abrirFormulario(Edicion{CalendarioID: "mio", Titulo: "Revisión", Inicio: ini, Fin: ini.Add(time.Hour), Zona: "UTC"})
	buscar := buscarEnCapas(w.Canvas(), a.P.t("event.findTimeShow", nil))
	if buscar == nil {
		t.Fatal("no está «Buscar un hueco»")
	}
	buscar.Tapped(nil)
	s.mu.Lock()
	q := s.huecos
	s.mu.Unlock()
	if q.Get("start") != "2031-03-04T00:00:00Z" || q.Get("end") != "2031-03-11T00:00:00Z" || q.Get("duration") != "60" ||
		q.Get("weekdays") != "true" || q.Get("tz") != "UTC" || q.Get("work_start") != "09:00" {
		t.Errorf("buscar hueco pidió %v", q)
	}
	var horas []*botonHueco
	enCapas(w.Canvas(), func(o fyne.CanvasObject) {
		if b, ok := o.(*botonHueco); ok {
			horas = append(horas, b)
		}
	})
	if len(horas) != 4 || horas[1].hora != "07:30" || horas[3].hora != "13:00" {
		t.Fatalf("las horas ofrecidas no son las del servicio: %d", len(horas))
	}
	guardar(t, w.Canvas().Capture(), "calendario-huecos")
	horas[1].Tapped(nil)
	cerrado := true
	enCapas(w.Canvas(), func(o fyne.CanvasObject) {
		if _, ok := o.(*botonHueco); ok {
			cerrado = false
		}
	})
	if !cerrado {
		t.Error("al elegir una hora el panel sigue abierto")
	}
	buscarEnCapas(w.Canvas(), a.P.t("common.save", nil)).Tapped(nil)
	_, cuerpos := s.apuntadas()
	if len(cuerpos) != 1 || cuerpos[0]["start"] != "2031-03-04T07:30:00Z" || cuerpos[0]["end"] != "2031-03-04T08:30:00Z" {
		t.Errorf("se guardó %v", cuerpos)
	}
}

// buscarEnCapas es buscarBoton sobre las capas del lienzo.
func buscarEnCapas(c fyne.Canvas, rotulo string) *ui.Boton {
	for _, capa := range c.Overlays().List() {
		if b := buscarBoton(capa, rotulo); b != nil {
			return b
		}
	}
	return nil
}

func TestEventoDeValidaLoQueDiceElModelo(t *testing.T) {
	madrid, _ := time.LoadLocation("Europe/Madrid")
	p, ok := EventoDe("Claro:\n```json\n{\"titulo\":\" Comida con Ana \",\"inicio\":\"2031-03-06T14:00\",\"fin\":\"2031-03-06 13:00\",\"lugar\":\"Casa Lucio\"}\n```", madrid)
	if !ok || p.Titulo != "Comida con Ana" || p.Lugar != "Casa Lucio" {
		t.Fatalf("propuesta %+v, %v", p, ok)
	}
	// Sin zona, la hora es de la zona de visualización; y un fin anterior se descarta solo.
	if !p.Inicio.Equal(time.Date(2031, 3, 6, 14, 0, 0, 0, madrid)) || !p.Fin.IsZero() {
		t.Errorf("inicio %v, fin %v", p.Inicio, p.Fin)
	}
	p, _ = EventoDe(`{"titulo":"Revisión","inicio":"2031-03-06T09:00:00+0100","fin":"el jueves a las 2"}`, madrid)
	if !p.Inicio.Equal(time.Date(2031, 3, 6, 8, 0, 0, 0, time.UTC)) || !p.Fin.IsZero() {
		t.Errorf("con desfase sin dos puntos: %v; una frase como fin: %v", p.Inicio, p.Fin)
	}
	if _, ok := EventoDe(`{"inicio":"2031-03-06T09:00:00Z"}`, madrid); ok {
		t.Error("sin título no hay propuesta")
	}
	if _, ok := EventoDe("no he entendido nada", madrid); ok {
		t.Error("sin JSON no hay propuesta")
	}
}

func TestElAsistenteRellenaElFormulario(t *testing.T) {
	s := &escrituras{}
	a, w := appConVentana(t, s)
	var pedida api.PeticionIA
	respuesta := `{"titulo":"Comida con Ana","inicio":"2031-03-06T14:00:00+01:00","lugar":"Casa Lucio"}`
	a.Asistente = func(_ context.Context, p api.PeticionIA) (string, error) {
		pedida = p
		return respuesta, nil
	}
	ini := time.Date(2031, 3, 4, 10, 0, 0, 0, time.UTC)
	a.abrirFormulario(Edicion{CalendarioID: "mio", Inicio: ini, Fin: ini.Add(90 * time.Minute), Zona: "UTC"})
	var frase *ui.Entrada
	enCapas(w.Canvas(), func(o fyne.CanvasObject) {
		if e, ok := o.(*ui.Entrada); ok && frase == nil {
			frase = e
		}
	})
	rellenar := buscarEnCapas(w.Canvas(), a.P.t("event.aiFill", nil))
	if frase == nil || rellenar == nil || !rellenar.Inerte {
		t.Fatal("no está el asistente, o «Rellenar» no nace apagado")
	}
	frase.PonerTexto("Comida con Ana el jueves a las 14:00 en Casa Lucio")
	guardar(t, w.Canvas().Capture(), "calendario-asistente")
	rellenar.Tapped(nil)
	if pedida.Accion != "evento" || pedida.Tipo != "calendar" || pedida.Modo != "calidad" ||
		pedida.Contexto != "Comida con Ana el jueves a las 14:00 en Casa Lucio" || !strings.Contains(pedida.Instruccion, "UTC") {
		t.Errorf("se pidió %+v", pedida)
	}
	if frase.Texto() != "" {
		t.Error("la frase no se vacía tras rellenar")
	}
	buscarEnCapas(w.Canvas(), a.P.t("common.save", nil)).Tapped(nil)
	_, cuerpos := s.apuntadas()
	// Sin fin en la propuesta, el fin arrastra al inicio con la hora y media que había.
	if len(cuerpos) != 1 || cuerpos[0]["summary"] != "Comida con Ana" || cuerpos[0]["location"] != "Casa Lucio" ||
		cuerpos[0]["start"] != "2031-03-06T13:00:00Z" || cuerpos[0]["end"] != "2031-03-06T14:30:00Z" {
		t.Errorf("se guardó %v", cuerpos)
	}
}

func TestElAsistenteDiceCuandoNoEntiende(t *testing.T) {
	a, w := appConVentana(t, &escrituras{})
	a.Asistente = func(context.Context, api.PeticionIA) (string, error) { return "No sé.", nil }
	a.abrirFormulario(Edicion{CalendarioID: "mio", Inicio: time.Date(2031, 3, 4, 10, 0, 0, 0, time.UTC), Fin: time.Date(2031, 3, 4, 11, 0, 0, 0, time.UTC), Zona: "UTC"})
	var frase *ui.Entrada
	enCapas(w.Canvas(), func(o fyne.CanvasObject) {
		if e, ok := o.(*ui.Entrada); ok && frase == nil {
			frase = e
		}
	})
	frase.PonerTexto("algo")
	buscarEnCapas(w.Canvas(), a.P.t("event.aiFill", nil)).Tapped(nil)
	visto := false
	enCapas(w.Canvas(), func(o fyne.CanvasObject) {
		if p, ok := o.(*ui.Parrafo); ok && p.Texto == a.P.t("event.aiError", nil) {
			visto = true
		}
	})
	if !visto || frase.Texto() != "algo" {
		t.Errorf("sin propuesta: error visto=%v, frase=%q", visto, frase.Texto())
	}
}
