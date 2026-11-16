// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package erp

import (
	"io"
	"net/http"
	"slices"
	"strings"
	"testing"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/test"

	"git.sytue.eu/sytue/one/apps/fyne/iconos"
	"git.sytue.eu/sytue/one/apps/fyne/ui"
)

// opcionDe es la opción de la paleta con el comando `id`; falla si no se ofrece.
func opcionDe(t *testing.T, p *Pantalla, id string) ui.OpcionDePaleta {
	t.Helper()
	for _, o := range p.paleta.Opciones() {
		if o.Comando != nil && o.Comando.ID == id {
			return o
		}
	}
	t.Fatalf("la paleta no ofrece %q", id)
	return ui.OpcionDePaleta{}
}

// registrosOfrecidos son las opciones del grupo de los registros.
func registrosOfrecidos(p *Pantalla) []ui.OpcionDePaleta {
	var out []ui.OpcionDePaleta
	for _, o := range p.paleta.Opciones() {
		if o.Comando != nil && strings.HasPrefix(o.Comando.ID, "registro:") {
			out = append(out, o)
		}
	}
	return out
}

// TestElERPPoneSusTresCapasEnLaPaletaDeLaSuite: el ERP ya no tiene paleta propia;
// registra en la de la barra las mismas capas que `ComandosDelErp` de la web, con los
// mismos ids, y al cambiar de paleta (la propia de la pantalla suelta por la de la
// barra) se las lleva.
//
// VISTO EN ROJO: con la paleta propia de antes, la de la barra no tenía ni una capa.
func TestElERPPoneSusTresCapasEnLaPaletaDeLaSuite(t *testing.T) {
	p := pantallaDePrueba(t)
	p.Objeto()
	propia := p.paleta
	barra := ui.NuevaBarraSuite(p.tr, nil, iconos.ERP)
	t.Cleanup(barra.Parar)
	w := test.NewWindow(container.NewStack())
	defer w.Close()
	p.Atajos(w.Canvas(), barra)
	if p.paleta != barra.Paleta() {
		t.Fatal("el ERP no usa la paleta de la barra")
	}
	if n := len(barra.Paleta().Capas()); n != 3 {
		t.Errorf("la paleta de la suite tiene %d capas del ERP; son tres (vistas, áreas, registros)", n)
	}
	if n := len(propia.Capas()); n != 0 {
		t.Errorf("la paleta propia se queda con %d capas", n)
	}
	// Las áreas llevan su tope de doce y la pista con el NOMBRE del área.
	if o := opcionDe(t, p, "modelo:factura_venta"); o.Pista != "Ventas" || o.Grupo != "Áreas" {
		t.Errorf("Facturas de cliente se ofrece con %+v", o)
	}
}

// TestLaPaletaLlevaALasPantallas: ⌘K prometía «ir a cualquier sitio» y solo filtraba
// modelos; al Libro diario, al Banco o a Tareas no se llegaba escribiendo. Ahora la
// paleta ofrece las mismas pantallas que el menú, bajo «Ir a», y antes que las áreas.
func TestLaPaletaLlevaALasPantallas(t *testing.T) {
	p := pantallaDePrueba(t)
	w := test.NewWindow(p.Objeto()) // rellena el menú, y con él las pantallas de la paleta
	defer w.Close()
	w.Resize(fyne.NewSize(1280, 800))
	p.Atajos(w.Canvas(), nil)
	p.AbrirPaleta()

	p.paleta.Escribir("libro")
	o := p.paleta.Opciones()
	if len(o) == 0 || o[0].Rotulo != "Libro diario" || o[0].Grupo != "Ir a" {
		t.Fatalf("«libro» ofrece %+v; se esperaba el Libro diario bajo «Ir a»", o)
	}
	p.paleta.Teclear(&fyne.KeyEvent{Name: fyne.KeyReturn})
	if p.Vista != VistaLibro || p.paleta.Abierta() {
		t.Errorf("Intro sobre el Libro diario deja la vista %q y la paleta abierta=%v", p.Vista, p.paleta.Abierta())
	}

	// Sin escribir nada: primero las pantallas, luego las áreas, cada una con su rótulo.
	p.AbrirPaleta()
	var grupos []string
	for _, o := range p.paleta.Opciones() {
		if len(grupos) == 0 || grupos[len(grupos)-1] != o.Grupo {
			grupos = append(grupos, o.Grupo)
		}
	}
	if !slices.Equal(grupos, []string{"Ir a", "Áreas"}) {
		t.Errorf("grupos = %q", grupos)
	}
	// Y se pintan: el rótulo del grupo en versalitas (`sytue-overline`), letra a letra.
	letras := strings.Join(textosDe(w.Canvas().Overlays().Top()), "")
	if !strings.Contains(letras, "IR A") || !strings.Contains(letras, "ÁREAS") {
		t.Errorf("no se leen los rótulos de grupo: %q", letras)
	}
}

// TestLaPaletaTraeLosRegistros: una referencia o un nombre llega al servicio
// (`GET /search`), sus registros se ofrecen bajo «Registros» y elegir uno abre su ficha.
// Lo de un modelo que el catálogo no trae no se ofrece: no habría ficha que abrir.
func TestLaPaletaTraeLosRegistros(t *testing.T) {
	p, sf := pantallaConServicio(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v1/search":
			_, _ = io.WriteString(w, `{"items":[
				{"id":"f12","modelo":"factura_venta","ref":"FV-2026-0012","etiqueta":"Acme S.L.","data":{}},
				{"id":"x1","modelo":"fantasma","ref":"FV-2026-0012-X","etiqueta":"Oculto","data":{}}]}`)
		case "/api/v1/factura_venta/f12":
			_, _ = io.WriteString(w, `{"id":"f12","modelo":"factura_venta","ref":"FV-2026-0012","etiqueta":"Acme S.L.","data":{}}`)
		default:
			_, _ = io.WriteString(w, `{"registros":[]}`)
		}
	})
	p.AbrirPaleta()
	p.paleta.Escribir("FV-2026-0012")
	if !p.paleta.Buscando() {
		t.Error("mientras el servicio contesta la paleta no dice que está buscando")
	}
	if got := p.paleta.Vacio(); got != p.t("search.busy") {
		t.Errorf("sin opciones y buscando se lee %q", got)
	}
	esperar(t, func() bool {
		var hay bool
		fyne.DoAndWait(func() { hay = len(registrosOfrecidos(p)) > 0 })
		return hay
	})
	if !slices.Contains(sf.vistas(), "GET /api/v1/search?limit=8&q=FV-2026-0012") {
		t.Errorf("peticiones = %q", sf.vistas())
	}
	regs := registrosOfrecidos(p)
	if len(regs) != 1 {
		t.Fatalf("se ofrecen %d registros; el de un modelo fuera del catálogo no cuenta", len(regs))
	}
	if r := regs[0]; r.Grupo != "Registros" || r.Rotulo != "Acme S.L." || r.Pista != "FV-2026-0012 · Facturas de cliente" {
		t.Fatalf("registro ofrecido = %+v", r)
	}
	if p.paleta.Buscando() {
		t.Error("con la respuesta ya dentro la paleta sigue «buscando»")
	}
	p.paleta.Elegir(regs[0])
	esperar(t, func() bool { return p.ficha != nil })
	if p.paleta.Abierta() || p.lista.Modelo.Clave != "factura_venta" {
		t.Errorf("elegir el registro deja la paleta abierta=%v y la lista en %q", p.paleta.Abierta(), p.lista.Modelo.Clave)
	}
}

// TestLaPaletaNoAceptaRegistrosViejos: con una letra no se busca; la respuesta a una
// consulta que ya no es la de ahora no pisa la lista; con otra consulta, los registros
// de la anterior se sueltan y la paleta queda «buscando» hasta la respuesta nueva.
func TestLaPaletaNoAceptaRegistrosViejos(t *testing.T) {
	// El servicio no contesta a las búsquedas: las respuestas las pone el test a mano.
	soltar := make(chan struct{})
	p, _ := pantallaConServicio(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v1/search" {
			select {
			case <-soltar:
			case <-r.Context().Done():
			}
			return
		}
		_, _ = io.WriteString(w, `{"registros":[]}`)
	})
	t.Cleanup(func() { close(soltar) })
	p.AbrirPaleta()
	p.paleta.Escribir("f")
	if p.buscandoRegistros || p.paleta.Buscando() {
		t.Error("con una letra no se busca en el servicio")
	}
	p.paleta.Escribir("acme")
	viejo := []Registro{{ID: "r1", Modelo: "factura_venta", Etiqueta: "Acme"}}
	p.encontrados("ac", viejo)
	if len(registrosOfrecidos(p)) != 0 {
		t.Error("la respuesta de «ac» pisó la consulta «acme»")
	}
	p.encontrados("acme", viejo)
	if len(registrosOfrecidos(p)) != 1 || p.paleta.Buscando() {
		t.Errorf("la respuesta de «acme» no llegó: %+v", p.paleta.Opciones())
	}
	p.paleta.Escribir("acme ")
	if len(registrosOfrecidos(p)) != 1 {
		t.Error("un espacio de más no es otra búsqueda: los registros se quedan")
	}
	p.paleta.Escribir("globex")
	if len(registrosOfrecidos(p)) != 0 || !p.paleta.Buscando() {
		t.Errorf("con «globex» siguen los registros de «acme» (buscando=%v)", p.paleta.Buscando())
	}
	p.CerrarPaleta()
	p.encontrados("globex", viejo)
	if len(p.registrosPaleta) != 0 {
		t.Error("una respuesta que llega con la paleta cerrada se queda")
	}
}

// TestLaPaletaNoSeQuedaBuscandoSiElServicioNoContesta: una búsqueda que agota el plazo
// tiene que llegar a la paleta como «sin registros». Antes se descartaba igual que una
// cancelada y la paleta decía «Buscando…» para siempre.
func TestLaPaletaNoSeQuedaBuscandoSiElServicioNoContesta(t *testing.T) {
	soltar := make(chan struct{})
	p, _ := pantallaConServicio(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v1/search" {
			select {
			case <-soltar:
			case <-r.Context().Done():
			}
			return
		}
		_, _ = io.WriteString(w, `{"registros":[]}`)
	})
	t.Cleanup(func() { close(soltar) })
	p.Plazo = 50 * time.Millisecond
	p.AbrirPaleta()
	p.paleta.Escribir("zzqq")
	if !p.paleta.Buscando() {
		t.Fatal("la paleta no empezó a buscar")
	}
	esperar(t, func() bool {
		var listo bool
		fyne.DoAndWait(func() { listo = !p.paleta.Buscando() })
		return listo
	})
	if got := p.paleta.Vacio(); !strings.Contains(got, "zzqq") {
		t.Errorf("con el plazo agotado se lee %q y no que no hay nada", got)
	}
}
