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
	"fyne.io/fyne/v2/test"

	"git.sytue.eu/sytue/one/apps/fyne/tema"
)

// TestLaPaletaLlevaALasPantallas: ⌘K prometía «ir a cualquier sitio» y solo filtraba
// modelos; al Libro diario, al Banco o a Tareas no se llegaba escribiendo. Ahora la
// paleta ofrece las mismas pantallas que el menú, bajo «Ir a», y antes que las áreas.
func TestLaPaletaLlevaALasPantallas(t *testing.T) {
	p := pantallaDePrueba(t)
	p.Objeto() // rellena el menú, y con él las pantallas de la paleta
	p.AbrirPaleta()

	p.paleta.Escribir("libro")
	o := p.paleta.Opciones()
	if len(o) == 0 || o[0].Pantalla == nil || o[0].Rotulo != "Libro diario" || o[0].Grupo != "Ir a" {
		t.Fatalf("«libro» ofrece %+v; se esperaba el Libro diario bajo «Ir a»", o)
	}
	p.paleta.Teclear(&fyne.KeyEvent{Name: fyne.KeyReturn})
	if p.Vista != VistaLibro || p.abierta {
		t.Errorf("Intro sobre el Libro diario deja la vista %q y la paleta abierta=%v", p.Vista, p.abierta)
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
	obj := p.paleta.Objeto(tema.Claro, fyne.NewSize(1280, 800))
	w := test.NewTempWindow(t, obj)
	w.Resize(fyne.NewSize(1280, 800))
	if letras := strings.Join(textosDe(obj), ""); !strings.Contains(letras, "IR A") || !strings.Contains(letras, "ÁREAS") {
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
	if !p.paleta.Buscando {
		t.Error("mientras el servicio contesta la paleta no dice que está buscando")
	}
	if p.paleta.Objeto(tema.Claro, p.Ventana) != nil && !contiene(textosDe(p.paleta.Objeto(tema.Claro, p.Ventana)), "Buscando…") {
		t.Error("sin opciones y buscando, se afirmaba que nada coincide")
	}
	esperar(t, func() bool { return len(p.paleta.Registros) > 0 })
	if !slices.Contains(sf.vistas(), "GET /api/v1/search?limit=8&q=FV-2026-0012") {
		t.Errorf("peticiones = %q", sf.vistas())
	}
	var reg *OpcionDestino
	for _, o := range p.paleta.Opciones() {
		if o.Registro != nil {
			if reg != nil {
				t.Fatalf("se ofrece también %q, de un modelo que no está en el catálogo", o.Rotulo)
			}
			reg = &o
		}
	}
	if reg == nil || reg.Grupo != "Registros" || reg.Rotulo != "Acme S.L." || reg.Pista != "FV-2026-0012 · Facturas de cliente" {
		t.Fatalf("registro ofrecido = %+v", reg)
	}
	p.paleta.elegir(*reg)
	esperar(t, func() bool { return p.ficha != nil })
	if p.abierta || p.lista.Modelo.Clave != "factura_venta" {
		t.Errorf("elegir el registro deja la paleta abierta=%v y la lista en %q", p.abierta, p.lista.Modelo.Clave)
	}
}

// TestLaPaletaNoPreguntaConUnaLetraNiAceptaRespuestasViejas: con una letra no se
// pregunta al servicio, y la respuesta a una consulta que ya no es la de ahora no pisa
// la lista.
func TestLaPaletaNoPreguntaConUnaLetraNiAceptaRespuestasViejas(t *testing.T) {
	p := NuevaPaleta(traductor(t), catalogoEjemplo().Modulos[0].Modelos)
	var pedidas []string
	p.AlBuscar = func(q string) { pedidas = append(pedidas, q) }
	p.Escribir("f")
	if p.Buscando || len(p.Registros) != 0 {
		t.Error("con una letra no se busca en el servicio")
	}
	p.Escribir("acme")
	if !p.Buscando || !slices.Equal(pedidas, []string{"f", "acme"}) {
		t.Errorf("buscando=%v, pedidas=%q", p.Buscando, pedidas)
	}
	viejo := []Registro{{ID: "r1", Modelo: "factura_venta", Etiqueta: "Acme"}}
	if p.Encontrados("ac", viejo) || len(p.Registros) != 0 {
		t.Error("la respuesta de «ac» pisó la consulta «acme»")
	}
	if !p.Encontrados("acme", viejo) || p.Buscando || len(p.Opciones()) != 1 {
		t.Errorf("la respuesta de «acme» no llegó: %+v", p.Opciones())
	}
	// Sin coincidencias en ningún grupo, el texto de siempre.
	p.Escribir("zz")
	p.Encontrados("zz", nil)
	if !contiene(textosDe(p.Objeto(tema.Claro, fyne.NewSize(1280, 800))), "Nada coincide") {
		t.Error("sin resultados no se dice «Nada coincide»")
	}
}

// TestAlCambiarLaBusquedaNoQuedanLosRegistrosDeLaAnterior: con otra consulta, los
// registros de la anterior se sueltan y la paleta queda «buscando» hasta la respuesta
// nueva; añadir solo espacios no es otra consulta.
func TestAlCambiarLaBusquedaNoQuedanLosRegistrosDeLaAnterior(t *testing.T) {
	p := NuevaPaleta(traductor(t), catalogoEjemplo().Modulos[0].Modelos)
	p.AlBuscar = func(string) {}
	p.Escribir("acme")
	p.Encontrados("acme", []Registro{{ID: "r1", Modelo: "factura_venta", Etiqueta: "Acme"}})
	p.Escribir("acme ")
	if len(p.Registros) != 1 {
		t.Error("un espacio de más no es otra búsqueda: los registros se quedan")
	}
	p.Escribir("globex")
	if len(p.Registros) != 0 || !p.Buscando {
		t.Errorf("con «globex» siguen los registros de «acme»: %+v (buscando=%v)", p.Registros, p.Buscando)
	}
	for _, o := range p.Opciones() {
		if o.Registro != nil {
			t.Errorf("se ofrece un registro de la búsqueda anterior: %+v", o)
		}
	}
}

// TestLaPaletaLargaDesplazaHastaElCursor: con las pantallas, las doce áreas y los
// registros la lista no cabe en el 70 % del alto; se desplaza dentro del panel y la fila
// del cursor queda siempre a la vista, como el `scrollIntoView` de la web.
func TestLaPaletaLargaDesplazaHastaElCursor(t *testing.T) {
	modelos := make([]Modelo, 0, 12)
	for _, n := range []string{"a", "b", "c", "d", "e", "f", "g", "h", "i", "j", "k", "l"} {
		modelos = append(modelos, Modelo{Clave: n, Modulo: "m", Etiqueta: "Modelo " + n})
	}
	p := NuevaPaleta(traductor(t), modelos)
	for _, r := range []string{"Inicio", "Tareas", "Banco", "Obligaciones", "Libro diario", "Sumas y saldos"} {
		p.Pantallas = append(p.Pantallas, Destino{Rotulo: r})
	}
	lienzo := fyne.NewSize(1280, 500)
	p.Mover(-1) // da la vuelta: la última área
	obj := p.Objeto(tema.Claro, lienzo)
	w := test.NewTempWindow(t, obj)
	w.SetPadded(false)
	w.Resize(lienzo)
	obj.Resize(lienzo)
	var panel *panelPaleta
	if c, ok := obj.(*fyne.Container); ok {
		panel, _ = c.Layout.(*panelPaleta)
	}
	if panel == nil || panel.desplaza == nil {
		t.Fatal("la lista de la paleta no va en un desplazable")
	}
	if alto := panel.AltoPanel(lienzo); alto > FraccionAltoPaleta*lienzo.Height+0.5 {
		t.Errorf("el panel mide %v y no pasa del 70 %% (%v)", alto, FraccionAltoPaleta*lienzo.Height)
	}
	ultima := panel.lista.filas[len(panel.lista.filas)-1]
	visto := panel.desplaza.Offset.Y + panel.desplaza.Size().Height
	if !ultima.Resaltada || ultima.Position().Y+ultima.Size().Height > visto+0.5 {
		t.Errorf("la última fila (y=%v) queda fuera de lo visible (hasta %v)", ultima.Position().Y, visto)
	}
}

// TestLaPaletaAcotaElCursorSiLaRespuestaAcortaLaLista: con el cursor en el último
// registro, una respuesta con menos registros no deja la lista sin resaltado; la fila
// resaltada es la que abre Intro, como en la web.
func TestLaPaletaAcotaElCursorSiLaRespuestaAcortaLaLista(t *testing.T) {
	p := NuevaPaleta(traductor(t), catalogoEjemplo().Modulos[0].Modelos)
	p.AlBuscar = func(string) {}
	p.Escribir("zz")
	p.Encontrados("zz", []Registro{
		{ID: "r1", Modelo: "factura_venta", Etiqueta: "Uno"},
		{ID: "r2", Modelo: "factura_venta", Etiqueta: "Dos"},
	})
	p.Mover(-1) // da la vuelta: el último registro
	p.Encontrados("zz", []Registro{{ID: "r1", Modelo: "factura_venta", Etiqueta: "Uno"}})
	obj := p.Objeto(tema.Claro, fyne.NewSize(1280, 800))
	var panel *panelPaleta
	if c, ok := obj.(*fyne.Container); ok {
		panel, _ = c.Layout.(*panelPaleta)
	}
	if panel == nil || panel.lista == nil || len(panel.lista.filas) != 1 {
		t.Fatal("no se pinta la fila del registro")
	}
	if !panel.lista.filas[0].Resaltada {
		t.Error("la respuesta acortó la lista y ninguna fila queda resaltada")
	}
	if o, hay := p.Elegido(); !hay || o.Registro == nil || o.Registro.ID != "r1" {
		t.Errorf("Intro abriría %+v", o)
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
	if !p.paleta.Buscando {
		t.Fatal("la paleta no empezó a buscar")
	}
	esperar(t, func() bool { return !p.paleta.Buscando })
	if !contiene(textosDe(p.paleta.Objeto(tema.Claro, p.Ventana)), "Nada coincide") {
		t.Error("con el plazo agotado no se dice «Nada coincide»")
	}
}
