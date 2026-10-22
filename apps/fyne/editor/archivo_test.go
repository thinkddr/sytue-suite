// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package editor

import (
	"image/color"
	"math"
	"testing"
	"time"

	"fyne.io/fyne/v2"
	fcanvas "fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/test"

	"git.sytue.eu/sytue/one/apps/fyne/catalogos"
	"git.sytue.eu/sytue/one/apps/fyne/tema"
	"git.sytue.eu/sytue/one/apps/fyne/ui"
	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/i18n"
	"git.sytue.eu/sytue/one/apps/nucleo/office"
)

// textos del menú, por el orden en que se pintan.
func textos(m *ui.Menu) []string {
	var out []string
	for _, e := range m.Elementos {
		if it, ok := e.(*ui.ElementoMenu); ok {
			out = append(out, it.Texto)
		}
	}
	return out
}

// TestElMenuArchivoEsElDeLaWeb: las cinco descargas, las dos plantillas con el nombre
// del formato que toca a cada tipo, guardar versión, renombrar y volver; y sin gancho, la entrada no hace como
// que funciona.
func TestElMenuArchivoEsElDeLaWeb(t *testing.T) {
	var pedido string
	ctx := &Contexto{Tipo: Hoja, Exportar: func(f string) { pedido = f }}
	m := NuevoBotonArchivo(ctx).Menu()
	quiero := []string{
		"file.downloadPdf", "file.pdfAccessible", "file.downloadXlsx", "file.downloadOds", "file.downloadHtml",
		"file.downloadMarkdown", "file.downloadTemplateOoxml", "file.downloadTemplateOdf",
		"file.saveVersion", "file.rename", "ui.backToList",
	}
	got := textos(m)
	if len(got) != len(quiero) {
		t.Fatalf("el menú: %v", got)
	}
	for i := range quiero {
		if got[i] != quiero[i] {
			t.Errorf("entrada %d: %q, quiero %q", i, got[i], quiero[i])
		}
	}
	if op := m.Elementos[1].(*ui.ElementoMenu); !op.Marcada || !op.MantieneAbierto {
		t.Errorf("«PDF accesible» sale marcada y sin cerrar el menú: %+v", op)
	} else {
		op.AlPulsar()
		if !PDFSimple || NuevoBotonArchivo(ctx).Menu().Elementos[1].(*ui.ElementoMenu).Marcada {
			t.Error("pulsar «PDF accesible» no la apaga")
		}
		PDFSimple = false
	}
	m.Elementos[2].(*ui.ElementoMenu).AlPulsar()
	if pedido != "ooxml" {
		t.Errorf("«Descargar como Excel» pide %q", pedido)
	}
	if !m.Elementos[9].(*ui.ElementoMenu).Inerte {
		t.Error("«Guardar versión» sin gancho tiene que salir inerte")
	}
	ctx.GuardarEnDrive = func() {}
	if got := textos(NuevoBotonArchivo(ctx).Menu()); got[0] != "file.saveToDrive" {
		t.Errorf("con documento de Drive, la primera entrada es devolverlo a Drive: %v", got)
	}
}

func TestDescargasOfflineEnAmbosMenus(t *testing.T) {
	for _, tipo := range []Tipo{Documento, Hoja, Presentacion} {
		ctx := &Contexto{Tipo: tipo, SinRed: true, Exportar: func(string) {}}
		archivo := NuevoBotonArchivo(ctx).Menu()
		cabecera := NuevaCabecera(ctx, nil).MenuDescargas()
		j := 0 // en Archivo, tras una descarga con interruptor («PDF accesible») va su entrada
		for i, command := range office.Exports() {
			if rotuloFormato(tipo, command.ID) != command.Labels[tipo.officeKind()] {
				t.Errorf("label %d/%s", tipo, command.ID)
			}
			enArchivo := archivo.Elementos[j].(*ui.ElementoMenu)
			j++
			if command.Option != nil {
				j++
			}
			if !enArchivo.Inerte || !cabecera.Elementos[i].(*ui.ElementoMenu).Inerte {
				t.Errorf("offline export enabled: %d/%s", tipo, command.ID)
			}
		}
	}
}

// Las plantillas son las dos entradas de Ribbon.tsx después de OFFICE_EXPORTS.
// Exportar el documento y exportar su plantilla son callbacks distintos.
func TestArchivoDescargaPlantillasDeLosTresTipos(t *testing.T) {
	tr, err := i18n.New("es", i18n.Common(), catalogos.Docs())
	if err != nil {
		t.Fatal(err)
	}
	for _, caso := range []struct {
		tipo    Tipo
		oo, odf string
	}{
		{Documento, "dotx", "ott"},
		{Hoja, "xltx", "ots"},
		{Presentacion, "potx", "otp"},
	} {
		var pedido string
		ctx := &Contexto{
			Tipo: caso.tipo, T: tr.T,
			Exportar:          func(f string) { pedido = "documento:" + f },
			ExportarPlantilla: func(f string) { pedido = "plantilla:" + f },
		}
		m := NuevoBotonArchivo(ctx).Menu()
		for _, entrada := range []struct {
			indice                 int
			formato, ext, etiqueta string
		}{
			{6, "ooxml", caso.oo, "Descargar como plantilla de Office"},
			{7, "odf", caso.odf, "Descargar como plantilla de OpenDocument"},
		} {
			it := m.Elementos[entrada.indice].(*ui.ElementoMenu)
			if it.Texto != entrada.etiqueta+" ("+entrada.ext+")" || it.Inerte {
				t.Errorf("tipo %d plantilla %s: %q, inerte %v", caso.tipo, entrada.formato, it.Texto, it.Inerte)
			}
			it.AlPulsar()
			if pedido != "plantilla:"+entrada.formato {
				t.Errorf("la plantilla pide %q", pedido)
			}
		}
		m.Elementos[2].(*ui.ElementoMenu).AlPulsar()
		if pedido != "documento:ooxml" {
			t.Errorf("el documento pide %q", pedido)
		}
		for _, estado := range []struct {
			sinRed    bool
			ocupado   string
			sinGancho bool
		}{
			{true, "", false}, {false, "pdf", false}, {false, "", true},
		} {
			ctx.SinRed, ctx.Exportando = estado.sinRed, estado.ocupado
			if estado.sinGancho {
				ctx.ExportarPlantilla = nil
			}
			m = NuevoBotonArchivo(ctx).Menu()
			for _, indice := range []int{6, 7} {
				it := m.Elementos[indice].(*ui.ElementoMenu)
				if !it.Inerte {
					t.Errorf("plantilla %d activa sin red, durante exportación o sin gancho", indice)
				}
				if estado.sinGancho && it.AlPulsar != nil {
					t.Error("sin gancho se inventó una acción de plantilla")
				}
			}
		}
	}
}

// TestElChipDiceCuantoHace: «Guardado», «Guardado hace 15 s», «Guardado hace 2 min», y
// «Guardado en este dispositivo» sin red, con la frase entera del catálogo.
func TestElChipDiceCuantoHace(t *testing.T) {
	ctx := &Contexto{Tipo: Documento, Guardado: Limpio, T: func(k string, v i18n.Vars) string {
		switch k {
		case "header.save.agoSeconds":
			return "hace " + itoa(v["seconds"]) + " s"
		case "header.save.agoMinutes":
			return "hace " + itoa(v["minutes"]) + " min"
		}
		return k
	}}
	c := NuevaCabecera(ctx, nil)
	ahora := time.Now()
	c.guardadoEn = ahora
	for _, caso := range []struct {
		pasado time.Duration
		quiero string
	}{
		{2 * time.Second, "header.save.saved"},
		{15 * time.Second, "hace 15 s"},
		{100 * time.Second, "hace 2 min"},
	} {
		if got, _ := c.textoChip(ahora.Add(caso.pasado)); got != caso.quiero {
			t.Errorf("a los %v el chip dice %q y la web %q", caso.pasado, got, caso.quiero)
		}
	}
	ctx.SinRed = true
	if got, token := c.textoChip(ahora); got != "header.save.offline" || token != "warning" {
		t.Errorf("sin red: %q en %q", got, token)
	}
}

func itoa(v any) string {
	n, _ := v.(int)
	s := ""
	for {
		s = string(rune('0'+n%10)) + s
		n /= 10
		if n == 0 {
			return s
		}
	}
}

// TestColaboracionLlamaAlProducto: el historial pide las versiones al abrirse; sin
// ganchos, los botones salen inertes.
func TestColaboracionLlamaAlProducto(t *testing.T) {
	pedidas := 0
	ctx := &Contexto{Tipo: Documento, Versiones: func(listo func([]api.DocVersion)) {
		pedidas++
		listo([]api.DocVersion{{ID: "v1", Label: "Antes"}})
	}}
	cuerpo := cuerpoColaboracion(ctx)
	var chip *ChipCinta
	var botones []*BotonCinta
	var buscar func(o fyne.CanvasObject)
	buscar = func(o fyne.CanvasObject) {
		switch x := o.(type) {
		case *ChipCinta:
			chip = x
		case *BotonCinta:
			botones = append(botones, x)
		case *Grupo:
			for _, h := range x.Controles {
				buscar(h)
			}
		case *fyne.Container:
			for _, h := range x.Objects {
				buscar(h)
			}
		}
	}
	buscar(cuerpo)
	// El grupo «Revisión» es solo del documento, como en la web: activar, el panel, y
	// aceptar y rechazar todo, entre los comentarios y las versiones.
	if chip == nil || len(botones) != 8 {
		t.Fatalf("la pestaña: chip %v, %d botones", chip, len(botones))
	}
	inertes := []bool{true, false, true, false, true, true, true, true}
	for i, b := range botones {
		if b.Inerte != inertes[i] {
			t.Errorf("sin ganchos, el botón %d (%s) inerte = %v", i, b.Rotulo, b.Inerte)
		}
	}
	if botones[2].Rotulo != "review.track" || botones[5].Rotulo != "review.rejectAll" {
		t.Errorf("el grupo «Revisión» va tras los comentarios: %s … %s", botones[2].Rotulo, botones[5].Rotulo)
	}
	chip.Tapped(nil)
	if pedidas != 1 {
		t.Errorf("abrir el historial pide las versiones %d veces", pedidas)
	}
	botones = nil
	buscar(cuerpoColaboracion(&Contexto{Tipo: Hoja}))
	if len(botones) != 4 {
		t.Errorf("en una hoja no hay control de cambios: %d botones", len(botones))
	}
	// Con los ganchos, «Control de cambios» se enciende y el panel se abre.
	encendido := false
	ctx.EnRastreo = func() bool { return encendido }
	ctx.AlternarRastreo = func() { encendido = !encendido }
	botones = nil
	buscar(cuerpoColaboracion(ctx))
	botones[2].Tapped(nil)
	botones[3].Tapped(nil)
	botones = nil
	buscar(cuerpoColaboracion(ctx))
	if !encendido || !botones[2].Activo || ctx.Panel != PanelRevision || !botones[3].Activo || botones[3].Rotulo != "review.panelHide" {
		t.Errorf("tras pulsarlos: encendido %v (%v), panel %v (%v, %s)", encendido, botones[2].Activo, ctx.Panel, botones[3].Activo, botones[3].Rotulo)
	}
}

// TestGuardarVersionPreguntaLaEtiquetaOpcional: «Guardar versión», desde Colaboración y
// desde el menú Archivo, pregunta antes la etiqueta con `versions.labelPrompt` y los
// rótulos comunes, como `guardarVersion` de la web; vacía también se acepta, lo escrito
// llega tal cual al producto, y cancelar no guarda nada.
func TestGuardarVersionPreguntaLaEtiquetaOpcional(t *testing.T) {
	var guardadas []string
	ctx := &Contexto{Tipo: Documento, GuardarVersion: func(e string) { guardadas = append(guardadas, e) }}
	archivo := NuevoBotonArchivo(ctx)
	var cinta *BotonCinta
	var buscar func(o fyne.CanvasObject)
	buscar = func(o fyne.CanvasObject) {
		switch x := o.(type) {
		case *BotonCinta:
			if x.Rotulo == "versions.save" {
				cinta = x
			}
		case *Grupo:
			for _, h := range x.Controles {
				buscar(h)
			}
		case *fyne.Container:
			for _, h := range x.Objects {
				buscar(h)
			}
		}
	}
	colaboracion := cuerpoColaboracion(ctx)
	buscar(colaboracion)
	if cinta == nil {
		t.Fatal("la pestaña Colaboración no tiene «Guardar versión»")
	}
	w := test.NewTempWindow(t, container.NewVBox(archivo, colaboracion))
	w.Resize(fyne.NewSize(1280, 800))

	desdeMenu := func() {
		for _, e := range archivo.Menu().Elementos {
			if it, ok := e.(*ui.ElementoMenu); ok && it.Texto == "file.saveVersion" {
				it.AlPulsar()
			}
		}
	}
	for _, c := range []struct {
		nombre string
		pulsar func()
	}{{"Colaboración", func() { cinta.AlPulsar() }}, {"el menú Archivo", desdeMenu}} {
		for _, escrito := range []string{"", "Antes de enviar"} {
			guardadas = nil
			c.pulsar()
			d := dialogoArriba(w.Canvas())
			if d == nil || d.Titulo != "versions.labelPrompt" {
				t.Fatalf("desde %s, «Guardar versión» no pregunta la etiqueta: %+v", c.nombre, d)
			}
			campo := w.Canvas().Focused().(*ui.Entrada)
			campo.PonerTexto(escrito)
			campo.TypedKey(&fyne.KeyEvent{Name: fyne.KeyReturn})
			if len(guardadas) != 1 || guardadas[0] != escrito {
				t.Errorf("desde %s, con %q escrito se ha guardado %q", c.nombre, escrito, guardadas)
			}
		}
		guardadas = nil
		c.pulsar()
		pie := dialogoArriba(w.Canvas()).Pie.(*fyne.Container).Objects
		if pie[0].(*ui.Boton).Texto != "ui.cancel" || pie[1].(*ui.Boton).Texto != "ui.save" {
			t.Errorf("desde %s, el pie de la pregunta: %q y %q", c.nombre, pie[0].(*ui.Boton).Texto, pie[1].(*ui.Boton).Texto)
		}
		pie[0].(*ui.Boton).Tapped(nil)
		if len(guardadas) != 0 {
			t.Errorf("desde %s, cancelar ha guardado %q", c.nombre, guardadas)
		}
	}
}

// dialogoArriba es el diálogo de la capa de más arriba del lienzo, o nil.
func dialogoArriba(c fyne.Canvas) *ui.Dialogo {
	top, ok := c.Overlays().Top().(fyne.Widget)
	if !ok {
		return nil
	}
	for _, o := range test.WidgetRenderer(top).Objects() {
		if d, ok := o.(*ui.Dialogo); ok {
			return d
		}
	}
	return nil
}

// TestVistaEsLaDeLaWeb: el selector de zoom enseña el peldaño más cercano, la regla solo
// sale en el folio y «Ajustar al ancho» y «Pantalla completa» sin gancho se apagan.
func TestVistaEsLaDeLaWeb(t *testing.T) {
	for _, c := range []struct {
		zoom, quiero int
	}{{100, 100}, {110, 100}, {140, 150}, {25, 50}, {190, 200}} {
		if got := presetCercano(c.zoom); got != c.quiero {
			t.Errorf("con zoom %d el selector dice %d y la web %d", c.zoom, got, c.quiero)
		}
	}
	contar := func(tipo Tipo) (botones int, ajustar *BotonCinta) {
		var ir func(fyne.CanvasObject)
		ir = func(o fyne.CanvasObject) {
			switch x := o.(type) {
			case *BotonCinta:
				botones++
				if x.Rotulo == "view.fitWidth" {
					ajustar = x
				}
			case *Grupo:
				for _, h := range x.Controles {
					ir(h)
				}
			case *fyne.Container:
				for _, h := range x.Objects {
					ir(h)
				}
			}
		}
		ir(cuerpoVista(&Contexto{Tipo: tipo, Zoom: 100}))
		return botones, ajustar
	}
	doc, ajustar := contar(Documento)
	hoja, _ := contar(Hoja)
	if doc != hoja+1 {
		t.Errorf("el folio tiene la regla y la hoja no: %d botones contra %d", doc, hoja)
	}
	if ajustar == nil || !ajustar.Inerte {
		t.Error("«Ajustar al ancho» sin gancho tiene que salir inerte")
	}
}

// Archivo-1280 y Archivo-390, Chromium remoto 2026-10-04: botón h24
// en y51,5; el menú se ancla al contenedor h31 y comienza en y83.
// Escape conserva el disparador y recupera el foco, aunque su ancla sea otra.
func TestArchivoEsUnChipCentradoYSuMenuCuelgaDeLaTira(t *testing.T) {
	tr, err := i18n.New("es", i18n.Common(), catalogos.Docs())
	if err != nil {
		t.Fatal(err)
	}
	for _, ancho := range []float32{1280, 390} {
		// EditorDemo tiene exportAs(formato, plantilla), sin sourceRef ni acciones Nuevo/Abrir.
		ctx := &Contexto{Tipo: Documento, T: tr.T, Exportar: func(string) {}, ExportarPlantilla: func(string) {}, Volver: func() {}}
		c := NuevaCinta(ctx, Genericas())
		c.Move(fyne.NewPos(0, 48))
		c.Resize(fyne.NewSize(ancho, AltoCinta()))
		w := test.NewTempWindow(t, container.NewWithoutLayout(c))
		w.SetPadded(false)
		w.Resize(fyne.NewSize(ancho, 800))
		pos := fyne.CurrentApp().Driver().AbsolutePositionForObject(c.archivo)
		if math.Abs(float64(c.archivo.Size().Width-69.20313)) > 0.2*7 || c.archivo.Size().Height != 24 || pos != fyne.NewPos(8, 51.5) {
			t.Errorf("a %g Archivo está en %v y mide %v; DOM (8,51.5),69.20313×24", ancho, pos, c.archivo.Size())
		}
		r := test.WidgetRenderer(c.archivo).(*archivoRenderer)
		if r.rotulo.Color != (color.NRGBA{R: 15, G: 29, B: 32, A: 255}) || r.rotulo.TextSize != 13 || r.rotulo.FontSource != tema.Media {
			t.Error("Archivo difiere del texto DOM: RGB15/29/32, Figtree13/500")
		}
		if radio := r.caja.Objetos()[0].(*fcanvas.Rectangle).CornerRadius; radio != 4 {
			t.Errorf("Archivo radio %g; rounded-sm mide4", radio)
		}
		fondo, borde, tinta := c.archivo.colores(tema.Claro)
		if fondo == (color.NRGBA{}) || borde == fondo || tinta != tema.Color("header-fg", tema.Claro) {
			t.Error("el chip no tiene bg-header (--sytue-header-bg) y ring-chrome-border inset")
		}
		w.Canvas().Focus(c.archivo)
		test.Tap(c.archivo)
		m := c.archivo.menu
		pos = fyne.CurrentApp().Driver().AbsolutePositionForObject(m)
		// 358 = los 328 medidos en Chromium más la entrada «PDF accesible» (un MenuItem `size="sm"`,
		// 30 px), añadida el 2026-10-04 sin volver a medir en el navegador.
		if ancho >= 768 {
			if pos != fyne.NewPos(8, 83) || math.Abs(float64(m.Size().Width-317.17188)) > 2 || m.Size().Height != 358 {
				t.Errorf("a %g menú %v/%v; DOM (8,83),317.17188×358", ancho, pos, m.Size())
			}
		} else {
			// Por debajo de 768 px (enHoja), el menú es la hoja inferior a todo el ancho.
			tam := ui.MedidaMenu(m, fyne.NewSize(ancho, 800))
			if pos != fyne.NewPos(0, 800-tam.Height) || m.Size().Width != ancho || m.Size().Height != tam.Height {
				t.Errorf("a %g hoja inferior %v/%v; esperado (0,%v) %v×%v", ancho, pos, m.Size(), 800-tam.Height, ancho, tam.Height)
			}
		}
		w.Canvas().Focused().TypedKey(&fyne.KeyEvent{Name: fyne.KeyEscape})
		if m.Abierto() || w.Canvas().Focused() != fyne.Focusable(c.archivo) {
			t.Error("Escape deja el menú abierto o el foco fuera de Archivo")
		}
	}
}
