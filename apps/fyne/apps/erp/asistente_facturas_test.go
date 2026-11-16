// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package erp

import (
	"encoding/json"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/test"

	"git.sytue.eu/sytue/one/apps/fyne/tema"
	"git.sytue.eu/sytue/one/apps/fyne/ui"
	"git.sytue.eu/sytue/one/apps/nucleo/api"
)

// catalogoFacturas es el módulo de facturación con las dos facturas, y contactos.
func catalogoFacturas() Catalogo {
	estados := []api.ERPOpcion{{Valor: "borrador", Etiqueta: "Borrador"}, {Valor: "confirmado", Etiqueta: "Confirmada"}}
	return Catalogo{
		Activos: []string{"facturacion", "contactos"},
		Modulos: []Modulo{
			{Clave: "facturacion", Etiqueta: "Facturación", Modelos: []Modelo{
				{
					Clave: api.ERPFacturaCompra, Modulo: "facturacion", Etiqueta: "Facturas de proveedor",
					Contable: true, Recibe: true, Lista: []string{"ref", "proveedor", "total"}, Estados: estados,
					Campos: []Campo{
						{Clave: "proveedor", Tipo: api.ERPRelacion, Modelo: "contacto", Etiqueta: "Proveedor"},
						{Clave: "fecha", Tipo: api.ERPFecha, Etiqueta: "Fecha"},
						{Clave: "total", Tipo: api.ERPDinero, Etiqueta: "Total"},
						{Clave: "externo_id", Tipo: "texto", Etiqueta: "Identificador externo"},
					},
				},
				{
					Clave: api.ERPFacturaVenta, Modulo: "facturacion", Etiqueta: "Facturas de cliente",
					Contable: true, Cobro: true, Lista: []string{"ref", "cliente", "total"}, Estados: estados,
					Campos: []Campo{
						{Clave: "cliente", Tipo: api.ERPRelacion, Modelo: "contacto", Etiqueta: "Cliente"},
						{Clave: "total", Tipo: api.ERPDinero, Etiqueta: "Total"},
						{Clave: "externo_id", Tipo: "texto", Etiqueta: "Identificador externo"},
					},
				},
			}},
			{Clave: "contactos", Etiqueta: "Contactos", Modelos: []Modelo{{
				Clave: "contacto", Modulo: "contactos", Etiqueta: "Contactos", Lista: []string{"nombre"},
				Campos: []Campo{{Clave: "nombre", Tipo: "texto", Etiqueta: "Nombre"}, {Clave: "nif", Tipo: "texto", Etiqueta: "NIF"}},
			}}},
		},
	}
}

// pantallaFacturas monta la pantalla con permiso para crear y asentar facturas, en
// una ventana (el asistente es un diálogo) y con el trabajo de red en la misma rutina.
func pantallaFacturas(t *testing.T, responder func(w http.ResponseWriter, r *http.Request)) (*Pantalla, *servicioFalso) {
	t.Helper()
	p, sf := pantallaConServicio(t, responder)
	p.Catalogo = catalogoFacturas()
	p.SabePermisos = true
	p.Permisos = api.ERPPermisos{
		Modulos: map[string]string{"facturacion": api.ERPUsuario, "contactos": api.ERPUsuario},
		Verbos:  map[string][]string{"facturacion": {"crear", "asentar"}, "contactos": {"crear"}},
	}
	w := test.NewWindow(p.raiz)
	w.Resize(fyne.NewSize(1280, 800))
	t.Cleanup(w.Close)
	p.lienzo = w.Canvas()
	p.Ventana = fyne.NewSize(1280, 800)
	return p, sf
}

// original escribe un fichero de prueba y lo devuelve con su tamaño.
func original(t *testing.T, nombre, contenido string) (string, int64) {
	t.Helper()
	ruta := filepath.Join(t.TempDir(), nombre)
	if err := os.WriteFile(ruta, []byte(contenido), 0o600); err != nil {
		t.Fatal(err)
	}
	return ruta, int64(len(contenido))
}

// escrituras son las peticiones que no son de lectura: lo que deja rastro.
func escrituras(sf *servicioFalso) []string {
	var out []string
	for _, v := range sf.vistas() {
		if !strings.HasPrefix(v, "GET ") {
			out = append(out, strings.TrimSuffix(v, "?"))
		}
	}
	return out
}

// pintarAsistente pinta el cuerpo y el pie: lo que se ve en cada paso no debe
// romperse con ningún estado.
func pintarAsistente(t *testing.T, x *AsistenteFacturas) {
	t.Helper()
	for _, v := range []tema.Variante{tema.Claro, tema.Oscuro} {
		if c := x.Cuerpo(v, 600); c.MinSize().Height <= 0 {
			t.Errorf("paso %d: el cuerpo no mide nada", x.Paso)
		}
		_ = x.Pie(func() {})
	}
}

// TestAsistenteFacturasRecorreLosCincoPasosConUnPDF: elegir, leer el PDF con Sytue AI,
// revisar en el alta rápida (que es quien crea), adjuntar el MISMO original y llevarla
// al libro. Antes, en Fyne solo existía «Rellenar con IA» suelto dentro de una ficha.
func TestAsistenteFacturasRecorreLosCincoPasosConUnPDF(t *testing.T) {
	var creada map[string]any
	var adjunto string
	p, sf := pantallaFacturas(t, func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/api/v1/factura_compra/ia/rellenar":
			if r.Header.Get("Content-Type") != "application/pdf" {
				t.Errorf("el PDF va como application/pdf, no %q", r.Header.Get("Content-Type"))
			}
			_, _ = io.WriteString(w, `{"data":{"fecha":"2026-11-02","total":12100,"proveedor":"c1"},"etiquetas":{"c1":"Lobo SL"}}`)
		case r.Method == http.MethodPost && r.URL.Path == "/api/v1/factura_compra/":
			var body struct {
				Data map[string]any `json:"data"`
			}
			_ = json.NewDecoder(r.Body).Decode(&body)
			creada = body.Data
			_, _ = io.WriteString(w, `{"id":"fc1","modelo":"factura_compra","ref":"FC-1","etiqueta":"Lobo SL","estado":"borrador","data":{}}`)
		case r.Method == http.MethodPost && r.URL.Path == "/api/v1/adjuntos":
			cuerpo, _ := io.ReadAll(r.Body)
			adjunto = r.URL.Query().Get("registro") + " " + r.URL.Query().Get("nombre") + " " + string(cuerpo)
			_, _ = io.WriteString(w, `{"tareas_hechas":0}`)
		case r.Method == http.MethodPost && r.URL.Path == "/api/v1/factura_compra/fc1/asentar":
			_, _ = io.WriteString(w, `{"ref":"AS-7"}`)
		default:
			_, _ = io.WriteString(w, `{"registros":[]}`)
		}
	})
	if p.AbrirAsistenteFacturas() == nil {
		t.Fatal("no abrió el asistente con permiso para crear")
	}
	x := p.facturas
	if x.Clave != api.ERPFacturaCompra || x.Paso != PasoFacturaTipo {
		t.Fatalf("debe abrir en la recibida, paso 0: %q/%d", x.Clave, x.Paso)
	}
	pintarAsistente(t, x)
	x.Siguiente()
	if x.Paso != PasoFacturaOrigen {
		t.Fatalf("paso = %d", x.Paso)
	}
	pintarAsistente(t, x)
	ruta, tam := original(t, "lobo.pdf", "%PDF-1.7 factura")
	x.PonerFichero(ruta, tam)
	x.Siguiente()
	if x.Paso != PasoFacturaRevisar || x.Fallo != "" || x.Semilla["total"] != float64(12100) {
		t.Fatalf("la lectura no llegó a la revisión: paso %d, %q, %v", x.Paso, x.Fallo, x.Semilla)
	}
	pintarAsistente(t, x)
	if len(escrituras(sf)) != 1 {
		t.Fatalf("leer no debe crear nada: %v", escrituras(sf))
	}

	x.Siguiente()
	if len(p.altas) != 1 {
		t.Fatal("revisar no abrió el alta rápida")
	}
	alta := p.altas[0].ficha
	if alta.Valores["proveedor"] != "c1" || alta.Rotulos["c1"] != "Lobo SL" || alta.Valores["fecha"] == "" {
		t.Errorf("el alta no trae lo leído: %v / %v", alta.Valores, alta.Rotulos)
	}
	p.lista.Modelo = p.Catalogo.Modulos[0].Modelos[0]
	antes := len(sf.vistas())
	alta.AlGuardar()
	if !slices.ContainsFunc(sf.vistas()[antes:], func(v string) bool { return strings.HasPrefix(v, "GET /api/v1/factura_compra") }) {
		t.Errorf("crear la factura no refrescó la lista de facturas abierta: %v", sf.vistas()[antes:])
	}
	if x.Paso != PasoFacturaAdjuntar || x.Registro == nil || x.Registro.ID != "fc1" {
		t.Fatalf("tras crear toca adjuntar: paso %d, %+v", x.Paso, x.Registro)
	}
	if creada["proveedor"] != "c1" || creada["total"] != float64(12100) {
		t.Errorf("cuerpo del alta: %v", creada)
	}
	pintarAsistente(t, x)
	x.IrA(PasoFacturaOrigen)
	if x.Paso != PasoFacturaAdjuntar {
		t.Fatal("con la factura creada no se vuelve atrás")
	}

	x.AlAdjuntar()
	if x.Paso != PasoFacturaAsentar || adjunto != "fc1 lobo.pdf %PDF-1.7 factura" {
		t.Fatalf("adjuntar debe subir el mismo original: paso %d, %q", x.Paso, adjunto)
	}
	pintarAsistente(t, x)
	if !x.PuedeAsentarla() {
		t.Fatal("una recibida en borrador, contable y con permiso, se puede asentar")
	}
	x.AlAsentar()
	if x.Asiento != "AS-7" || x.Registro.Estado != "confirmado" || x.PuedeAsentarla() {
		t.Errorf("asiento %q, estado %q", x.Asiento, x.Registro.Estado)
	}
	pintarAsistente(t, x)
	quiero := []string{
		"POST /api/v1/factura_compra/ia/rellenar",
		"POST /api/v1/factura_compra/",
		"POST /api/v1/adjuntos?modelo=factura_compra&nombre=lobo.pdf&registro=fc1",
		"POST /api/v1/factura_compra/fc1/asentar",
	}
	if got := escrituras(sf); !slices.Equal(got, quiero) {
		t.Errorf("escrituras:\n%v\nquiero:\n%v", got, quiero)
	}
}

// TestAsistenteFacturasExistenteSaltaAAdjuntar: un XML que ya entró no prepara otra
// igual; el asistente salta a adjuntar sobre la existente.
func TestAsistenteFacturasExistenteSaltaAAdjuntar(t *testing.T) {
	p, sf := pantallaFacturas(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v1/factura_compra/asistencia/leer" {
			if r.Header.Get("Content-Type") != "application/xml" {
				t.Errorf("el XML va como application/xml, no %q", r.Header.Get("Content-Type"))
			}
			_, _ = io.WriteString(w, `{"data":{},"etiquetas":{},"numero_origen":"F-9","existente":{"id":"fc9","modelo":"factura_compra","etiqueta":"Lobo SL","estado":"confirmado","data":{}}}`)
			return
		}
		_, _ = io.WriteString(w, `{"registros":[]}`)
	})
	p.AbrirAsistenteFacturas()
	x := p.facturas
	x.Siguiente()
	ruta, tam := original(t, "lobo.xml", "<Invoice/>")
	x.PonerFichero(ruta, tam)
	x.Siguiente()
	if x.Paso != PasoFacturaAdjuntar || x.Registro == nil || x.Registro.ID != "fc9" || x.NumeroOrigen != "F-9" {
		t.Fatalf("paso %d, %+v", x.Paso, x.Registro)
	}
	if len(p.altas) != 0 {
		t.Fatal("con la existente no se abre el alta")
	}
	if got := escrituras(sf); len(got) != 1 {
		t.Errorf("solo debía leer: %v", got)
	}
	pintarAsistente(t, x)
	x.Adjuntado()
	if x.PuedeAsentarla() {
		t.Error("una factura ya confirmada no se vuelve a asentar")
	}
	pintarAsistente(t, x)
}

// TestAsistenteFacturasTopesYTipos: PDF de 10 MB, XML de 8 MB y XML solo para las
// recibidas. Lo que no vale no sale hacia el servicio.
func TestAsistenteFacturasTopesYTipos(t *testing.T) {
	p, sf := pantallaFacturas(t, func(w http.ResponseWriter, r *http.Request) {
		t.Errorf("no debía pedir nada: %s %s", r.Method, r.URL.Path)
	})
	p.AbrirAsistenteFacturas()
	x := p.facturas
	invalido, grande := p.t("invoiceAssist.invalidFile"), "10"
	casos := []struct {
		clave, nombre string
		tamano        int64
		quiere        string
	}{
		{api.ERPFacturaCompra, "a.pdf", MaxPDFFactura + 1, grande},
		{api.ERPFacturaCompra, "a.xml", MaxXMLFactura + 1, "8"},
		{api.ERPFacturaCompra, "a.txt", 10, invalido},
		{api.ERPFacturaVenta, "a.xml", 10, invalido},
		{api.ERPFacturaVenta, "a.PDF", MaxPDFFactura + 1, grande},
	}
	for _, c := range casos {
		x.IrA(PasoFacturaTipo)
		x.Elegir(c.clave)
		x.Siguiente()
		x.PonerFichero(filepath.Join(t.TempDir(), c.nombre), c.tamano)
		x.Siguiente()
		if x.Paso != PasoFacturaOrigen || !strings.Contains(x.Fallo, c.quiere) {
			t.Errorf("%s %s (%d): paso %d, fallo %q", c.clave, c.nombre, c.tamano, x.Paso, x.Fallo)
		}
		if c.quiere != invalido && !strings.Contains(x.Fallo, "MB") {
			t.Errorf("%s: el tope debe decir su límite: %q", c.nombre, x.Fallo)
		}
	}
	// Justo en el tope, vale (y aquí ya se pediría: se corta antes con un PDF vacío).
	x.PonerFichero("a.pdf", MaxPDFFactura)
	if x.Comprobar() != "" {
		t.Errorf("un PDF de 10 MB justos vale: %q", x.Comprobar())
	}
	if len(sf.vistas()) != 0 {
		t.Errorf("peticiones: %v", sf.vistas())
	}
	// Cambiar de factura olvida el original elegido.
	x.Elegir(api.ERPFacturaCompra)
	if x.Fichero != "" {
		t.Error("el original de una emitida no sirve para una recibida")
	}
}

// TestAsistenteFacturasCancelarNoDejaRestos: cerrar en los tres primeros pasos no
// escribe nada; mientras lee no se cierra, y la respuesta tardía de un asistente ya
// cerrado no se aplica a nada.
func TestAsistenteFacturasCancelarNoDejaRestos(t *testing.T) {
	p, sf := pantallaFacturas(t, func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, `{"data":{"total":100},"etiquetas":{}}`)
	})
	for paso := PasoFacturaTipo; paso <= PasoFacturaRevisar; paso++ {
		d := p.AbrirAsistenteFacturas()
		if d == nil {
			t.Fatalf("paso %d: no reabre", paso)
		}
		x := p.facturas
		if paso >= PasoFacturaOrigen {
			x.Siguiente()
		}
		if paso == PasoFacturaRevisar {
			x.Siguiente() // sin original: a mano
		}
		if x.Paso != paso {
			t.Fatalf("quería el paso %d y está en %d", paso, x.Paso)
		}
		d.Descartar()
		if p.facturas != nil || len(p.altas) != 0 {
			t.Errorf("paso %d: cerrar dejó el asistente vivo", paso)
		}
	}
	if got := escrituras(sf); len(got) != 0 {
		t.Errorf("cancelar no debe escribir: %v", got)
	}

	d := p.AbrirAsistenteFacturas()
	x := p.facturas
	x.Siguiente()
	ruta, tam := original(t, "a.pdf", "%PDF")
	x.PonerFichero(ruta, tam)
	var leer func()
	p.fondo = func(f func()) { leer = f }
	x.Siguiente()
	if !x.Ocupado {
		t.Fatal("preparar no marca el asistente ocupado")
	}
	d.Descartar()
	if p.facturas != x {
		t.Fatal("se cerró mientras leía")
	}
	x.Ocupado = false // como si el usuario hubiera podido cerrar
	d.Descartar()
	leer()
	if x.Paso != PasoFacturaOrigen || p.facturas != nil {
		t.Errorf("la respuesta tardía se aplicó a un asistente cerrado: paso %d", x.Paso)
	}
}

// TestAsistenteFacturasProponeElProveedorLeido: si el XML trae un proveedor que no
// está en contactos, crear la relación desde el formulario lo propone con sus datos,
// y la emitida no hereda la identidad del documento de apoyo.
func TestAsistenteFacturasProponeElProveedorLeido(t *testing.T) {
	p, _ := pantallaFacturas(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v1/factura_compra/asistencia/leer":
			_, _ = io.WriteString(w, `{"data":{"total":100,"externo_id":"x1"},"etiquetas":{},"numero_origen":"F-1","contacto":{"nombre":"Lobo SL","nif":"B12345678","raro":{"a":1}}}`)
		case "/api/v1/factura_venta/ia/rellenar":
			_, _ = io.WriteString(w, `{"data":{"total":100,"externo_id":"x1","medio_pago":"tpv"},"etiquetas":{},"contacto":{"nombre":"No"}}`)
		default:
			_, _ = io.WriteString(w, `{"registros":[]}`)
		}
	})
	d := p.AbrirAsistenteFacturas()
	x := p.facturas
	x.Siguiente()
	ruta, tam := original(t, "lobo.xml", "<Invoice/>")
	x.PonerFichero(ruta, tam)
	x.Siguiente()
	if x.Contacto["nif"] != "B12345678" || x.Contacto["raro"] != nil || x.Semilla["externo_id"] != "x1" {
		t.Fatalf("contacto %v, semilla %v", x.Contacto, x.Semilla)
	}
	pintarAsistente(t, x)
	x.Siguiente()
	alta := p.altas[0].ficha
	alta.CrearRelacion("contacto", func(Registro) {})
	if len(p.altas) != 2 {
		t.Fatal("crear el proveedor no abrió su alta")
	}
	if c := p.altas[1].ficha; c.Valores["nombre"] != "Lobo SL" || c.Valores["nif"] != "B12345678" {
		t.Errorf("el alta del proveedor no trae lo leído: %v", c.Valores)
	}
	p.altas[1].dialogo.Descartar()
	p.altas[0].dialogo.Descartar()
	if x.Paso != PasoFacturaRevisar || x.Registro != nil {
		t.Fatal("cancelar el alta vuelve a la revisión sin crear nada")
	}
	if p.facturas != x {
		t.Fatal("cancelar el alta cerró el asistente")
	}
	d.Descartar()

	// La emitida: el apoyo no le presta su identidad ni sus marcas de emisión.
	p.AbrirAsistenteFacturas()
	y := p.facturas
	y.Elegir(api.ERPFacturaVenta)
	y.Siguiente()
	pdf, tp := original(t, "pedido.pdf", "%PDF")
	y.PonerFichero(pdf, tp)
	y.Siguiente()
	if _, hay := y.Semilla["externo_id"]; hay || y.Semilla["medio_pago"] != nil || y.Contacto != nil {
		t.Errorf("la emitida heredó del apoyo: %v / %v", y.Semilla, y.Contacto)
	}
}

// TestLaListaOfreceElAsistenteDeFacturas: el botón está en facturación, con permiso
// para crear facturas y fuera de la papelera, como en ErpApp.tsx.
func TestLaListaOfreceElAsistenteDeFacturas(t *testing.T) {
	p, _ := pantallaFacturas(t, func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, `{"registros":[]}`)
	})
	p.lista.Modelo = p.Catalogo.Modulos[0].Modelos[0]
	p.Repintar()
	if p.lista.AlAsistenteFacturas == nil {
		t.Fatal("la lista de facturas no ofrece el asistente")
	}
	p.lista.Papelera = true
	p.Repintar()
	if p.lista.AlAsistenteFacturas != nil {
		t.Error("en la papelera no se ofrece")
	}
	p.lista.Papelera = false
	p.lista.Modelo = p.Catalogo.Modulos[1].Modelos[0]
	p.Repintar()
	if p.lista.AlAsistenteFacturas != nil {
		t.Error("fuera de facturación no se ofrece")
	}
	p.lista.Modelo = p.Catalogo.Modulos[0].Modelos[0]
	p.Permisos.Verbos["facturacion"] = []string{"asentar"}
	p.Repintar()
	if p.lista.AlAsistenteFacturas != nil || p.AbrirAsistenteFacturas() != nil {
		t.Error("sin permiso para crear facturas no se ofrece")
	}
}

// TestElBotonDelAsistenteVaFueraDeLaColumna: en la web el botón va en el `px-6 pt-4` de
// `main`, fuera del `max-w-5xl` centrado de la lista; en una ventana ancha no se mete
// en la columna.
func TestElBotonDelAsistenteVaFueraDeLaColumna(t *testing.T) {
	l := NuevaLista(traductor(t))
	l.Modelo, l.Ventana, l.AlAsistenteFacturas = catalogoFacturas().Modulos[0].Modelos[0], 1920, func() {}
	o := l.Objeto(tema.Claro)
	o.Resize(fyne.NewSize(1600, o.MinSize().Height))
	c, ok := o.(*fyne.Container)
	if !ok {
		t.Fatalf("la lista es %T", o)
	}
	if _, ok := c.Layout.(*conAsistente); !ok || len(c.Objects) != 2 {
		t.Fatalf("sin el botón encima de la lista: %T", c.Layout)
	}
	if pos := c.Objects[0].Position(); pos.X != MargenLista || pos.Y != SeparaBloques {
		t.Errorf("el botón está en %v; en la web, en (%v, %v)", pos, MargenLista, SeparaBloques)
	}
	if y := c.Objects[1].Position().Y; y != SeparaBloques+c.Objects[0].MinSize().Height {
		t.Errorf("la lista empieza en %v", y)
	}
}

// TestElPieDelAsistenteEnElTelefono: el pie es un `ui.PieAcciones`, así que en la hoja
// del teléfono sus botones miden los 44 del dedo. Envuelto en otro contenedor el diálogo
// no lo reconocía y quedaban con el alto de escritorio.
func TestElPieDelAsistenteEnElTelefono(t *testing.T) {
	p, _ := pantallaFacturas(t, func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, `{"registros":[]}`)
	})
	p.lienzo.(interface{ Resize(fyne.Size) }).Resize(fyne.NewSize(390, 800))
	p.Ventana = fyne.NewSize(390, 800)
	d := p.AbrirAsistenteFacturas()
	p.facturas.Siguiente()
	pie, ok := d.Pie.(*fyne.Container)
	if !ok {
		t.Fatalf("el pie es %T", d.Pie)
	}
	if _, ok := pie.Layout.(ui.PieAcciones); !ok {
		t.Fatalf("el pie no es un ui.PieAcciones: %T", pie.Layout)
	}
	if len(pie.Objects) != 3 {
		t.Fatalf("en el paso del original van Cancelar, Atrás y Continuar: %d", len(pie.Objects))
	}
	for i, o := range pie.Objects {
		if h := o.Size().Height; h < 44 {
			t.Errorf("acción %d del pie: %v de alto en el teléfono", i, h)
		}
	}
}
