// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package bases

import (
	"regexp"
	"slices"
	"strings"
	"testing"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/test"

	"git.sytue.eu/sytue/one/apps/fyne/ui"
	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/i18n"
	"git.sytue.eu/sytue/one/apps/nucleo/webref"
)

// TestLaHojaDeAtajosEsLaDeLaWeb compara las filas con `GRUPOS_BASES` de `lib/atajos.ts`,
// en el mismo orden: grupos y descripciones. Una fila nueva en un lado y no en el otro
// sale aquí en rojo.
func TestLaHojaDeAtajosEsLaDeLaWeb(t *testing.T) {
	fuente := webref.Read(t, "apps/frontend/apps/bases/src/lib/atajos.ts")
	inicio := strings.Index(fuente, "GRUPOS_BASES:")
	if inicio < 0 {
		t.Fatal("no está `GRUPOS_BASES` en atajos.ts: ¿ha cambiado su forma?")
	}
	filas := regexp.MustCompile(`(?:titulo|descripcion): '([a-zA-Z.]+)'`).FindAllStringSubmatch(fuente[inicio:], -1)
	web := make([]string, 0, len(filas))
	for _, m := range filas {
		web = append(web, m[1])
	}
	var nativa []string
	for _, g := range gruposDeBases {
		nativa = append(nativa, g.titulo)
		for _, f := range g.filas {
			nativa = append(nativa, f.descripcion)
		}
	}
	if len(web) == 0 || !slices.Equal(web, nativa) {
		t.Errorf("la hoja de Fyne es\n  %v\ny la de la web\n  %v", nativa, web)
	}
}

// TestLaHojaDeAtajosSeAbreConInterrogacion: «?» sin nada enfocado abre la hoja, y sobre
// una celda que no se escribe tecleando (una casilla) también; en una de texto la
// escribe.
func TestLaHojaDeAtajosSeAbreConInterrogacion(t *testing.T) {
	f := nuevoFalso(2)
	p, w := abrir(t, f)
	p.alTeclear('?')
	if !strings.Contains(textos(w), "Atajos de teclado") || !strings.Contains(textos(w), "Copiar la celda") {
		t.Fatalf("«?» no abre la hoja de atajos:\n%s", textos(w))
	}
	w.Canvas().Overlays().Top().Hide()
	w.Canvas().Overlays().Remove(w.Canvas().Overlays().Top())
	g := p.abierta.rejilla
	g.foco = celda{0, 3} // «Activa», una casilla
	g.TypedRune('?')
	if w.Canvas().Overlays().Top() == nil {
		t.Fatal("«?» sobre una casilla no abre la hoja")
	}
}

// TestDuplicarDesdeLaFicha: «Duplicar registro» pide la copia al servicio y abre su ficha;
// las flechas pasan al registro de al lado guardando lo cambiado.
func TestDuplicarYRecorrerDesdeLaFicha(t *testing.T) {
	f := nuevoFalso(3)
	p, w := abrir(t, f)
	p.abrirRegistro(p.abierta.rejilla.Registros[0], false)
	pulsar(t, w, "Duplicar registro")
	if !f.vio("POST /records/r001/duplicate") {
		t.Fatalf("no se ha pedido la copia: %v", f.peticiones("POST"))
	}
	if n := len(p.abierta.rejilla.Registros); n != 4 || p.ficha == nil || p.ficha.r.ID == "r001" {
		t.Fatalf("la copia no se añade ni se abre su ficha: %d registros", n)
	}
	p.ficha.cerrar()
	p.abrirRegistro(p.abierta.rejilla.Registros[0], false)
	campoDeDialogo(t, w, "Nombre").PonerTexto("Ana")
	p.ficha.ir(1)
	if !f.vio(`PUT /records/r001/cells/nom?v=1 {"expected":"Persona 1","value":"Ana"}`) || p.ficha == nil || p.ficha.r.ID != "r002" {
		t.Fatalf("la flecha no guarda lo cambiado o no pasa al siguiente: %v", f.peticiones("PUT"))
	}
	p.ficha.ir(-1)
	if p.ficha.r.ID != "r001" {
		t.Fatalf("la flecha de arriba no vuelve: %s", p.ficha.r.ID)
	}
	p.ficha.ir(-1) // no hay anterior: se queda
	if p.ficha.r.ID != "r001" {
		t.Fatal("la flecha de arriba sale del primer registro")
	}
}

// TestLosTotalesSonDelServicio: encendidos desde el menú «⋯», se piden al servicio con la
// lectura de la rejilla y se pintan bajo ella; apagados, desaparecen.
func TestLosTotalesSonDelServicio(t *testing.T) {
	f := nuevoFalso(3)
	_, w := abrir(t, f)
	if len(f.peticiones("GET /tables/t1/summary")) != 0 {
		t.Fatal("los totales se piden sin encenderlos")
	}
	pulsarEnMenu(t, w, "Personas", "Ver totales")
	if len(f.peticiones("GET /tables/t1/summary")) != 1 {
		t.Fatalf("no se piden los totales: %v", f.peticiones("GET"))
	}
	got := textos(w)
	if !strings.Contains(got, "Registros: 3") || !strings.Contains(got, "Edad — Suma: 66") {
		t.Fatalf("no se pintan los totales:\n%s", got)
	}
	pulsarEnMenu(t, w, "Personas", "Ocultar totales")
	if strings.Contains(textos(w), "Registros: 3") {
		t.Fatal("los totales siguen tras ocultarlos")
	}
}

// TestDuplicarUnaVista: la copia lleva la configuración y «(copia)» en el nombre.
func TestDuplicarUnaVista(t *testing.T) {
	f := nuevoFalso(2)
	p, w := abrir(t, f)
	f.vistas["t1"] = []m{{"id": "v1", "table_id": "t1", "name": "Grandes", "kind": "grid", "config": m{"sort": []m{{"field": "eda", "dir": "desc"}}}}}
	p.elegirTabla("t1")
	p.elegirVista("v1")
	pulsarEnMenu(t, w, "Personas", "Duplicar vista")
	if !f.vio(`POST /tables/t1/views {"config":{"sort":[{"field":"eda","dir":"desc"}]},"kind":"grid","name":"Grandes (copia)"}`) {
		t.Fatalf("la copia no lleva la configuración: %v", f.peticiones("POST"))
	}
	if p.abierta.VistaID == "v1" {
		t.Fatal("no se pasa a la vista copiada")
	}
}

// TestElValorPorDefecto: el diálogo abre con el que tiene el campo y envía lo de su tipo;
// en una fecha, «el día del alta».
func TestElValorPorDefecto(t *testing.T) {
	fecha := api.BasesField{ID: "f", Name: "Alta", Kind: api.BasesKindDate, Default: api.BasesDefaultToday}
	b := borradorDeDefecto(fecha)
	if v, ok := valorDeDefecto(fecha, b); !b.hoy || !ok || v != "today" {
		t.Fatalf("una fecha «hoy»: %+v → %v %v", b, v, ok)
	}
	b.hoy, b.texto = false, "31/12/2026"
	if v, ok := valorDeDefecto(fecha, b); !ok || v != "2026-12-31" {
		t.Fatalf("un día fijo se escribe como se lee y viaja en ISO: %v %v", v, ok)
	}
	sel := api.BasesField{ID: "s", Kind: api.BasesKindSelect, Choices: []string{"Nuevo"}, Default: "Nuevo"}
	if b := borradorDeDefecto(sel); b.texto != "Nuevo" {
		t.Fatalf("la opción guardada no se abre: %+v", b)
	}
	if _, ok := valorDeDefecto(sel, borradorDefecto{texto: "Otra"}); ok {
		t.Fatal("una opción que no existe se deja guardar")
	}
	if v, ok := valorDeDefecto(api.BasesField{Kind: api.BasesKindCheckbox}, borradorDefecto{}); !ok || v != nil {
		t.Fatalf("una casilla sin marcar quita el defecto: %v", v)
	}
	if admiteDefecto(api.BasesField{Kind: api.BasesKindLink}) || !admiteDefecto(api.BasesField{Kind: api.BasesKindText}) {
		t.Fatal("solo los campos que se escriben admiten defecto")
	}

	f := nuevoFalso(1)
	p, w := abrir(t, f)
	p.alCampo(accionDefecto, api.BasesField{ID: "niv", TableID: "t1", Name: "Nivel", Kind: api.BasesKindSelect, Choices: []string{"Oro", "Plata"}})
	elegir(t, w, "Nivel", "Plata")
	pulsar(t, w, "Guardar")
	if !f.vio(`PATCH /fields/niv {"default":"Plata"}`) {
		t.Fatalf("no se envía el defecto: %v", f.peticiones("PATCH"))
	}
}

// TestFormulariosConObligatoriosYFiltrosRelativos: la configuración que sale del diálogo.
func TestFormulariosConObligatoriosYFiltrosRelativos(t *testing.T) {
	campos := []api.BasesField{
		{ID: "n", Kind: api.BasesKindText},
		{ID: "s", Kind: api.BasesKindSelect, Choices: []string{"a"}},
		{ID: "f", Kind: api.BasesKindDate},
	}
	cfg, ok := configDeBorrador(borradorVista{tipo: api.BasesForm, campos: []string{"s", "n"}, obligatorios: []string{"n", "x"}}, campos)
	if !ok || !slices.Equal(cfg.Required, []string{"n"}) {
		t.Fatalf("obligatorios: %+v", cfg)
	}
	con := func(op, valor string) (api.BasesViewConfig, bool) {
		return configDeBorrador(borradorVista{tipo: api.BasesGrid, filtroCampo: "f", filtroOp: op, filtroValor: valor}, campos)
	}
	if cfg, ok := con("past_days", "7"); !ok || cfg.Filters[0].Value != 7 {
		t.Fatalf("últimos 7 días: %+v", cfg)
	}
	if cfg, ok := con("today", "basura"); !ok || cfg.Filters[0].Value != nil {
		t.Fatalf("hoy no lleva valor: %+v", cfg)
	}
	for _, malo := range []string{"", "0", "1,5", "3651"} {
		if _, ok := con("next_days", malo); ok {
			t.Errorf("próximos %q días se deja guardar", malo)
		}
	}
}

// TestElPieDeColumna es `pieDeColumna` de la web: la suma de un número (con su detalle), la
// media de una valoración, las marcadas de una casilla y nada de una columna vacía.
func TestElPieDeColumna(t *testing.T) {
	tr, err := Traductor("es")
	if err != nil {
		t.Fatal(err)
	}
	f := func(x float64) *float64 { return &x }
	tt := func(k string, v ...i18n.Vars) string {
		var x i18n.Vars
		if len(v) > 0 {
			x = v[0]
		}
		return tr.T(k, x)
	}
	texto, detalle, ok := pieDeColumna(api.BasesField{Kind: api.BasesKindNumber}, &api.BasesFieldSummary{Filled: 2, Sum: f(1234.5), Avg: f(617.25), Min: f(4.5), Max: f(1230)}, tt, "es")
	if !ok || texto != "Suma: 1234,5" || detalle != "Media 617,25 · Mínimo 4,5 · Máximo 1230" {
		t.Errorf("número: %q %q", texto, detalle)
	}
	if texto, _, _ := pieDeColumna(api.BasesField{Kind: api.BasesKindRating}, &api.BasesFieldSummary{Filled: 2, Sum: f(9), Avg: f(4.5)}, tt, "es"); texto != "Media: 4,5" {
		t.Errorf("valoración: %q", texto)
	}
	if texto, _, _ := pieDeColumna(api.BasesField{Kind: api.BasesKindCheckbox}, &api.BasesFieldSummary{Filled: 3}, tt, "es"); texto != "Marcadas: 3" {
		t.Errorf("casilla: %q", texto)
	}
	if _, _, ok := pieDeColumna(api.BasesField{Kind: api.BasesKindText}, &api.BasesFieldSummary{}, tt, "es"); ok {
		t.Error("una columna sin valores dice algo")
	}
}

// TestElDiaDelDispositivoViajaEnLecturasYAltas: los filtros relativos y el defecto «hoy»
// son del día de quien mira, no del servidor.
func TestElDiaDelDispositivoViajaEnLecturasYAltas(t *testing.T) {
	f := nuevoFalso(1)
	_, w := abrir(t, f)
	pulsar(t, w, "Añadir registro")
	hoy := time.Now().Format(time.DateOnly)
	for _, quiero := range []string{"GET /tables/t1/records " + hoy, "POST /tables/t1/records " + hoy} {
		if !slices.Contains(f.dias, quiero) {
			t.Errorf("no viaja el día: falta %q en %v", quiero, f.dias)
		}
	}
}

var (
	_ fyne.Canvas = test.NewCanvas()
	_             = ui.EsTactil
)
