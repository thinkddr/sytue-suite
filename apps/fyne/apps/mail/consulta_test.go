// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package mail

import (
	"context"
	"encoding/json"
	"os"
	"testing"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/test"

	"git.sytue.eu/sytue/one/apps/fyne/tema"
	"git.sytue.eu/sytue/one/apps/fyne/ui"
)

// casoConsulta es un caso de testdata/consulta-busqueda.json.
type casoConsulta struct {
	Nombre   string
	Opciones OpcionesBusqueda
	Consulta string
}

// vectoresConsulta lee testdata/consulta-busqueda.json, que recorren también
// utils.test.ts y searchquery_panel_test.go del servicio.
func vectoresConsulta(t *testing.T) (componer, desmontar []casoConsulta) {
	t.Helper()
	crudo, err := os.ReadFile("testdata/consulta-busqueda.json")
	if err != nil {
		t.Fatal(err)
	}
	var v struct {
		Componer, Desmontar []casoConsulta
	}
	if err := json.Unmarshal(crudo, &v); err != nil {
		t.Fatal(err)
	}
	if len(v.Componer) < 10 || len(v.Desmontar) < 10 {
		t.Fatalf("solo hay %d casos de componer y %d de desmontar", len(v.Componer), len(v.Desmontar))
	}
	return v.Componer, v.Desmontar
}

// TestLaConsultaSeComponeComoEnLaWeb: el panel escribe la consulta que entiende el
// servicio, con las comillas, las fechas inclusivas y los campos vacíos fuera.
//
// VISTO EN ROJO (2026-11-13): escribiendo «Hasta» tal cual, el 28 de febrero se
// quedaba fuera (before excluye su día); sin quitar las comillas de dentro,
// `subject:"el "plan" B"` partía la consulta en tres. Deshecho.
func TestLaConsultaSeComponeComoEnLaWeb(t *testing.T) {
	componer, _ := vectoresConsulta(t)
	for _, c := range componer {
		if got := ComponerConsulta(c.Opciones); got != c.Consulta {
			t.Errorf("%s:\n   sale %q\n quiero %q", c.Nombre, got, c.Consulta)
		}
	}
}

// TestLaConsultaSeDesmontaComoEnLaWeb: al reabrir el panel con una consulta escrita,
// cada operador vuelve a su campo y lo que el panel no tiene se queda en «Contiene».
func TestLaConsultaSeDesmontaComoEnLaWeb(t *testing.T) {
	_, desmontar := vectoresConsulta(t)
	for _, c := range desmontar {
		if got := DesmontarConsulta(c.Consulta); got != c.Opciones {
			t.Errorf("%s:\n   sale %+v\n quiero %+v", c.Nombre, got, c.Opciones)
		}
	}
}

// TestLaConsultaDesmontadaSeVuelveAComponer: aplicar el panel sin tocar nada no cambia
// lo que se busca, aunque lleve operadores que el panel no tiene.
func TestLaConsultaDesmontadaSeVuelveAComponer(t *testing.T) {
	_, desmontar := vectoresConsulta(t)
	for _, c := range desmontar {
		o := DesmontarConsulta(c.Consulta)
		if otra := DesmontarConsulta(ComponerConsulta(o)); otra != o {
			t.Errorf("%s: ida y vuelta\n   sale %+v\n quiero %+v", c.Nombre, otra, o)
		}
	}
}

// bandejaConPanel monta la bandeja en una ventana del ancho del panel de la lista, con
// `consulta` escrita y apuntando lo que pide buscar.
func bandejaConPanel(t *testing.T, consulta string) (*Bandeja, *bandejaRenderer, fyne.Window, *[]string) {
	t.Helper()
	b := NuevaBandeja(traductor(t), "es")
	b.Ahora = reloj()
	b.Titulo = "Recibidos"
	b.Estado = Listo
	var pedidas []string
	b.AlBuscar = func(q string) {
		pedidas = append(pedidas, q)
		b.Consulta = q // lo que hace Buzon.Buscar
		b.Refresh()
	}
	w := test.NewWindow(b)
	t.Cleanup(w.Close)
	w.SetPadded(false)
	w.Resize(fyne.NewSize(ui.AnchoPanelBase, 744))
	r := test.WidgetRenderer(b).(*bandejaRenderer)
	if consulta != "" {
		r.busqueda.PonerTexto(consulta)
		pedidas = nil
	}
	return b, r, w, &pedidas
}

// TestElPanelDeBusquedaComponeYLanzaLaConsulta: el embudo junto al buscador abre el
// panel con la consulta escrita desmontada en campos; «Buscar» escribe la consulta
// compuesta en el buscador, busca UNA vez y cierra el panel.
//
// VISTO EN ROJO (2026-11-13): sin el embudo en la franja no hay panel que abrir;
// abriendo los campos en blanco, «De» sale vacío; buscando además de escribir en el
// campo, la búsqueda se pedía dos veces. Deshecho.
func TestElPanelDeBusquedaComponeYLanzaLaConsulta(t *testing.T) {
	_, r, w, pedidas := bandejaConPanel(t, `From:ana subject:"informe anual" factura`)
	if !r.opciones.Visible() || r.opciones.Rotulo != "Opciones de búsqueda" {
		t.Fatalf("el embudo no está o no se anuncia: visible=%v rótulo=%q", r.opciones.Visible(), r.opciones.Rotulo)
	}
	// Con «Limpiar» a la vista, todo cabe en la franja: el campo encoge.
	fin := r.limpiar.Position().X + r.limpiar.Size().Width
	if dentro := ui.AnchoPanelBase - tema.Borde; fin > dentro-MargenBusqueda+0.5 {
		t.Errorf("la franja se sale: «Limpiar» acaba en %v y el panel en %v", fin, dentro-MargenBusqueda)
	}
	if r.opciones.Position().X <= r.busqueda.Position().X+r.busqueda.Size().Width {
		t.Error("el embudo pisa el campo de búsqueda")
	}

	p := r.b.abrirOpcionesBusqueda(r.opciones)
	if p.emergente == nil || len(w.Canvas().Overlays().List()) == 0 {
		t.Fatal("el panel no se ha abierto")
	}
	if p.de.Texto() != "ana" || p.asunto.Texto() != "informe anual" || p.contiene.Texto() != "factura" {
		t.Errorf("no desmonta la consulta: de=%q asunto=%q contiene=%q", p.de.Texto(), p.asunto.Texto(), p.contiene.Texto())
	}
	p.hasta.PonerTexto("28/02/2026")
	p.adjuntos.Marcada = true
	test.Tap(p.buscar)

	quiero := `from:ana subject:"informe anual" has:attachment before:2026-03-01 factura`
	if len(*pedidas) != 1 || (*pedidas)[0] != quiero {
		t.Errorf("búsquedas pedidas %q; quiero una sola, %q", *pedidas, quiero)
	}
	if r.busqueda.Texto() != quiero {
		t.Errorf("el buscador dice %q y quiero %q", r.busqueda.Texto(), quiero)
	}
	if len(w.Canvas().Overlays().List()) != 0 {
		t.Error("el panel sigue abierto tras buscar")
	}
}

// TestVaciarElPanelVuelveALaCarpeta: «Vaciar campos» deja el panel en blanco, y buscar
// así vacía el buscador, que es volver a la carpeta.
func TestVaciarElPanelVuelveALaCarpeta(t *testing.T) {
	_, r, _, pedidas := bandejaConPanel(t, "from:ana is:unread after:2026-01-31")
	p := r.b.abrirOpcionesBusqueda(r.opciones)
	if !p.sinLeer.Marcada || p.desde.Texto() != "31/01/2026" {
		t.Fatalf("no desmonta la consulta: sin leer=%v desde=%q", p.sinLeer.Marcada, p.desde.Texto())
	}
	test.Tap(p.vaciar)
	for _, e := range p.campos() {
		if e.Texto() != "" {
			t.Errorf("queda %q tras vaciar", e.Texto())
		}
	}
	if p.sinLeer.Marcada {
		t.Error("«Sin leer» sigue marcada tras vaciar")
	}
	if len(*pedidas) != 0 {
		t.Errorf("vaciar ya ha buscado: %q", *pedidas)
	}
	test.Tap(p.buscar)
	if len(*pedidas) != 1 || (*pedidas)[0] != "" {
		t.Errorf("búsquedas pedidas %q; quiero volver a la carpeta", *pedidas)
	}
}

// TestBuscarCorreosDeEstaPersona: «Más acciones» del lector ofrece buscar lo que ha
// mandado el remitente, y pulsarlo escribe `from:` en el buscador y busca.
//
// VISTO EN ROJO (2026-11-13): sin el elemento en `menuMas` no se ofrece. Deshecho.
func TestBuscarCorreosDeEstaPersona(t *testing.T) {
	s := servicioConTodas()
	b, pendientes := buzonDePrueba(t, s)
	b.Arrancar(context.Background())
	c := b.P.Bandeja.Conversaciones[0]
	b.Abrir(c)
	var buscar *ui.ElementoMenu
	for _, o := range b.P.Lector.menuMas().Elementos {
		if e, ok := o.(*ui.ElementoMenu); ok && e.Texto == "Buscar correos de esta persona" {
			buscar = e
		}
	}
	if buscar == nil {
		t.Fatal("«Más acciones» no ofrece «Buscar correos de esta persona»")
	}
	antes := s.pidio("Search")
	buscar.AlPulsar()
	(*pendientes)[len(*pendientes)-1]()
	quiero := "from:" + remitenteDe(c)
	if b.P.Bandeja.Consulta != quiero || !b.P.Bandeja.Buscando || s.pidio("Search") != antes+1 {
		t.Errorf("consulta %q (quiero %q), buscando=%v, búsquedas %d→%d",
			b.P.Bandeja.Consulta, quiero, b.P.Bandeja.Buscando, antes, s.pidio("Search"))
	}
}
