// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package hub

import (
	"fmt"
	"image"
	"testing"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/test"

	"git.sytue.eu/sytue/one/apps/fyne/tema"
	"git.sytue.eu/sytue/one/apps/fyne/ui"
	"git.sytue.eu/sytue/one/apps/nucleo/api"
)

// Contratos del marcado actividad.astro con rem16: 16+6+42+8+22;
// píldora18+12+2; fila18+16+4+20; grupo25+12+3×58+2×4.
const (
	altoCabActividad = 94  // sobretítulo + h1 + `mt-2` + intro
	altoPildora      = 32  // filtro `rounded-control px-3 py-1.5`
	altoFilaActiv    = 58  // fila con título y resumen
	altoGrupoTres    = 219 // un día con tres filas
)

func hits(n int, origen string, cuando time.Time) []api.HubHit {
	out := make([]api.HubHit, 0, n)
	for i := range n {
		c := cuando
		out = append(out, api.HubHit{
			Source: origen, ID: fmt.Sprint(i), Title: "Auditoría · no borrar (32).odt",
			Snippet: "1.0 KiB", URL: "https://drive.sytue.eu/", Timestamp: &c,
		})
	}
	return out
}

func pintaWidget(t *testing.T, w fyne.Widget, alto func(float32) float32) image.Image {
	t.Helper()
	ven := test.NewWindow(w)
	t.Cleanup(ven.Close)
	ven.SetPadded(false)
	w.Resize(fyne.NewSize(ventana, alto(ventana)))
	ven.Resize(fyne.NewSize(ventana, alto(ventana)))
	return ven.Canvas().Capture()
}

// TestLaActividadMideLoQueMideLaWeb ata la geometría de `/actividad` a los números
// del DOM de producción.
//
// Visto en ROJO a mano el 2026-09-22: quitando el `tema.Borde` de
// `AltoEncabezadoDia` —el `border-b` que en `border-box` va DENTRO del alto—, el
// grupo pasa a medir 190,75 y el test lo dice con los dos números.
func TestLaActividadMideLoQueMideLaWeb(t *testing.T) {
	a := NuevaActividad(traductor(t), time.Date(2026, 9, 21, 12, 0, 0, 0, time.UTC))
	a.Estado = Listo
	a.Items = append(hits(3, "drive", time.Date(2026, 9, 20, 10, 0, 0, 0, time.UTC)),
		hits(2, "mail", time.Date(2026, 9, 19, 10, 0, 0, 0, time.UTC))...)
	r := test.WidgetRenderer(a).(*actividadRenderer)
	pintaWidget(t, a, r.alto)

	_, ancho := CajaContenido(ventana)
	if got := r.cabecera.alto(ancho); !cerca(got, altoCabActividad) {
		t.Errorf("la cabecera mide %v y en producción %v", got, altoCabActividad)
	}
	if got := MedidaPildora("Todo").Height; !cerca(got, altoPildora) {
		t.Errorf("la píldora mide %v de alto y en producción %v", got, altoPildora)
	}
	if got := MedidaActividad(true); !cerca(got, altoFilaActiv) {
		t.Errorf("la fila mide %v y en producción %v", got, altoFilaActiv)
	}
	// Un día con tres filas: encabezado 22 + `mb-3` 10,5 + 3 × 50,75 + 2 × 3,5.
	grupo := AltoEncabezadoDia() + HuecoEncabezado + 3*MedidaActividad(true) + 2*HuecoFilas
	if !cerca(grupo, altoGrupoTres) {
		t.Errorf("un grupo de tres mide %v y en producción %v", grupo, altoGrupoTres)
	}
	if len(r.grupos) != 2 {
		t.Fatalf("hay %d grupos y tiene que haber 2 (drive ayer, mail anteayer)", len(r.grupos))
	}
	// Los filtros: «Todo» más un producto por origen presente.
	if len(r.filtros) != 3 {
		t.Errorf("hay %d filtros y tienen que ser 3 (Todo, Drive, Mail)", len(r.filtros))
	}
}

// TestLaActividadEnSusSeisEstados: los seis se pintan, y ninguno se pinta como otro.
func TestLaActividadEnSusSeisEstados(t *testing.T) {
	visto := map[string]string{}
	for _, caso := range []struct {
		nombre string
		e      Estado
	}{
		{"carga", Cargando},
		{"exito", Listo},
		{"vacio", SinNada},
		{"error", Fallo},
		{"sin-conexion", SinConexion},
		{"prohibido", Prohibido},
	} {
		t.Run(caso.nombre, func(t *testing.T) {
			a := NuevaActividad(traductor(t), time.Date(2026, 9, 21, 12, 0, 0, 0, time.UTC))
			a.Estado = caso.e
			if caso.e == Listo {
				a.Items = hits(3, "drive", time.Date(2026, 9, 20, 10, 0, 0, 0, time.UTC))
			}
			r := test.WidgetRenderer(a).(*actividadRenderer)
			guardar(t, pintaWidget(t, a, r.alto), "actividad-"+caso.nombre)
			firma := fmt.Sprintf("esq=%d grupos=%d aviso=%v alto=%.2f",
				len(r.esqueleto), len(r.grupos), r.aviso != nil, r.alto(ventana))
			if otro, repe := visto[firma]; repe && caso.e != Fallo {
				t.Errorf("el estado %q se pinta igual que %q: %s", caso.nombre, otro, firma)
			}
			visto[firma] = caso.nombre
		})
	}
}

// TestPorDiaConservaElOrdenDelHub: el corte por día no reordena, y los que no traen
// fecha van al final —no se les inventa una, porque los mezclaría con actividad real—.
func TestPorDiaConservaElOrdenDelHub(t *testing.T) {
	ayer := time.Date(2026, 9, 20, 10, 0, 0, 0, time.UTC)
	anteayer := time.Date(2026, 9, 19, 10, 0, 0, 0, time.UTC)
	lista := []api.HubHit{
		{Source: "drive", ID: "a", Timestamp: &ayer},
		{Source: "mail", ID: "b"},
		{Source: "drive", ID: "c", Timestamp: &anteayer},
		{Source: "drive", ID: "d", Timestamp: &ayer},
	}
	g := PorDia(lista, time.UTC)
	if len(g) != 3 {
		t.Fatalf("salen %d grupos y tienen que ser 3", len(g))
	}
	if g[0].Dia != "2026-09-20" || len(g[0].Items) != 2 {
		t.Errorf("el primer grupo es %q con %d", g[0].Dia, len(g[0].Items))
	}
	if g[1].Dia != "2026-09-19" {
		t.Errorf("el segundo grupo es %q", g[1].Dia)
	}
	if g[2].Dia != "" || g[2].Items[0].ID != "b" {
		t.Errorf("los sin fecha no van al final: %+v", g[2])
	}
}

// TestLaBusquedaEnSusSeisEstados. El estado «sin consulta» es el SÉPTIMO y es el que
// se midió en producción: la captura de `buscar.ancho.png` es esa.
func TestLaBusquedaEnSusSeisEstados(t *testing.T) {
	ayer := time.Date(2026, 9, 20, 10, 0, 0, 0, time.UTC)
	visto := map[string]string{}
	casos := []struct {
		nombre  string
		termino string
		e       Estado
		items   []api.HubHit
	}{
		{"sin-consulta", "", Listo, nil},
		{"carga", "factura", Cargando, nil},
		{"exito", "factura", Listo, append(hits(2, "drive", ayer), hits(1, "mail", ayer)...)},
		{"vacio", "factura", Listo, nil},
		{"error", "factura", Fallo, nil},
		{"sin-conexion", "factura", SinConexion, nil},
		{"prohibido", "factura", Prohibido, nil},
	}
	for _, caso := range casos {
		t.Run(caso.nombre, func(t *testing.T) {
			b := NuevoBuscar(traductor(t))
			b.Termino, b.Estado, b.Items = caso.termino, caso.e, caso.items
			b.Total = len(caso.items)
			r := test.WidgetRenderer(b).(*buscarRenderer)
			guardar(t, pintaWidget(t, b, r.alto), "buscar-"+caso.nombre)
			// La firma lleva el TEXTO y no solo el alto: en `/buscar` el estado de
			// carga y el vacío son los dos una línea suelta, y lo que los distingue
			// es lo que dicen.
			firma := fmt.Sprintf("msg=%q grupos=%d aviso=%v alto=%.2f",
				textoPrevio(r), len(r.grupos), r.aviso != nil, r.alto(ventana))
			if otro, repe := visto[firma]; repe && caso.e != Fallo {
				t.Errorf("el estado %q se pinta igual que %q: %s", caso.nombre, otro, firma)
			}
			visto[firma] = caso.nombre
		})
	}
}

// TestLaBusquedaAvisaDeLosOrigenesCaidos: «no hay resultados» es una afirmación FALSA
// cuando el único producto que los tenía no contestó, así que el aviso tiene que
// salir también —y sobre todo— con la lista vacía.
func TestLaBusquedaAvisaDeLosOrigenesCaidos(t *testing.T) {
	b := NuevoBuscar(traductor(t))
	b.Termino, b.Estado, b.Fallidos = "factura", Listo, []string{"mail", "drive"}
	r := test.WidgetRenderer(b).(*buscarRenderer)
	pintaWidget(t, b, r.alto)
	texto := textoPrevio(r)
	if !contiene(texto, " y ") && !contiene(texto, " and ") {
		t.Errorf("los dos orígenes no van unidos con `search.partialJoin`: %q", texto)
	}
	for _, quiero := range []string{"Sytue Mail", "Sytue Drive"} {
		if !contiene(texto, quiero) {
			t.Errorf("el aviso no nombra a %s: %q", quiero, texto)
		}
	}
}

// textoPrevio junta lo que dice lo que va antes de los grupos de la búsqueda.
func textoPrevio(r *buscarRenderer) string {
	if r.previo == nil {
		return ""
	}
	var out string
	for _, o := range r.previo.objetos() {
		if p, ok := o.(*ui.Parrafo); ok {
			out += p.Texto + " "
		}
	}
	return out
}

func contiene(s, sub string) bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}

// TestPorOrigenConservaElRanking: el primer producto que aparece encabeza la lista.
func TestPorOrigenConservaElRanking(t *testing.T) {
	ayer := time.Date(2026, 9, 20, 10, 0, 0, 0, time.UTC)
	lista := []api.HubHit{
		{Source: "mail", ID: "1", Timestamp: &ayer},
		{Source: "drive", ID: "2", Timestamp: &ayer},
		{Source: "mail", ID: "3", Timestamp: &ayer},
	}
	g := PorOrigen(lista)
	if len(g) != 2 || g[0].Origen != "mail" || len(g[0].Items) != 2 || g[1].Origen != "drive" {
		t.Errorf("el agrupado por origen no conserva el ranking: %+v", g)
	}
}

// TestElTerminoSeRecortaEnElBorde: la web corta a 200 antes de llamar al Hub.
func TestElTerminoSeRecortaEnElBorde(t *testing.T) {
	largo := ""
	for range 300 {
		largo += "á"
	}
	if n := len([]rune(recortarTermino("  " + largo + "  "))); n != MaxTermino {
		t.Errorf("el término se recorta a %d runas y la web corta en %d", n, MaxTermino)
	}
}

// TestElTemaOscuroCambiaElPapel: las dos variantes existen y no pintan lo mismo. Es
// la comprobación mínima de que el tema oscuro no se ha quedado sin atender.
func TestElTemaOscuroCambiaElPapel(t *testing.T) {
	if tema.Color("bg", tema.Claro) == tema.Color("bg", tema.Oscuro) {
		t.Fatal("el papel es el mismo en claro y en oscuro")
	}
}

// TestLaAgendaNoSeQuedaCargandoParaSiempre: un esqueleto eterno es una pantalla que
// miente. Sin fuente conectada, «Hoy» tiene que caer al mismo mensaje que enseña la
// web cuando el servicio no contesta.
//
// Visto en ROJO a mano el 2026-09-22: quitando el `if v.agenda == nil` de
// `cargarAgenda`, la tarjeta se queda en Cargando y el test lo dice.
func TestLaAgendaNoSeQuedaCargandoParaSiempre(t *testing.T) {
	v := NuevaVista(traductor(t), nil, time.Date(2026, 9, 21, 12, 0, 0, 0, time.UTC))
	if v.lanzador.Agenda.Estado != Cargando {
		t.Fatalf("la agenda no arranca cargando, sino en %v", v.lanzador.Agenda.Estado)
	}
	v.cargarAgenda()
	if got := v.lanzador.Agenda.Estado; got == Cargando {
		t.Error("sin fuente conectada la agenda se queda cargando para siempre")
	}
}

// TestLosLimitesDelDiaSonLocales: de la medianoche local a la siguiente, que es lo
// que hace `limitesDelDia` en la web. En UTC daría «ayer» a las 00:30 de Madrid.
func TestLosLimitesDelDiaSonLocales(t *testing.T) {
	madrid := time.FixedZone("CEST", 2*60*60)
	desde, hasta := LimitesDelDia(time.Date(2026, 9, 21, 0, 30, 0, 0, madrid))
	if desde.Day() != 21 || desde.Hour() != 0 || desde.Location() != madrid {
		t.Errorf("el día empieza en %v", desde)
	}
	if hasta.Sub(desde) != 24*time.Hour {
		t.Errorf("el día dura %v", hasta.Sub(desde))
	}
}
