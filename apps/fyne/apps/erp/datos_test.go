// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package erp

import (
	"io"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/test"

	"git.sytue.eu/sytue/one/apps/fyne/iconos"
	"git.sytue.eu/sytue/one/apps/fyne/tema"
	"git.sytue.eu/sytue/one/apps/fyne/ui"
	"git.sytue.eu/sytue/one/apps/nucleo/api"
)

// servicioFalso es el ERP de mentira: apunta lo que se le pide y contesta lo que diga
// `responder`. Las peticiones llegan en OTRA rutina (la del servidor), de ahí el cerrojo.
type servicioFalso struct {
	mu        sync.Mutex
	pedidas   []string
	responder func(w http.ResponseWriter, r *http.Request)
}

func (s *servicioFalso) vistas() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return slices.Clone(s.pedidas)
}

// pantallaConServicio monta la pantalla contra el servicio falso, con el trabajo de red
// en la MISMA rutina del test: sin relojes y sin carreras.
func pantallaConServicio(t *testing.T, responder func(w http.ResponseWriter, r *http.Request)) (*Pantalla, *servicioFalso) {
	t.Helper()
	sf := &servicioFalso{responder: responder}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		sf.mu.Lock()
		sf.pedidas = append(sf.pedidas, r.Method+" "+r.URL.Path+"?"+r.URL.RawQuery)
		sf.mu.Unlock()
		sf.responder(w, r)
	}))
	t.Cleanup(srv.Close)
	p := NuevaPantalla(traductor(t), api.NewERP(srv.URL, nil, nil, "es"), nil)
	p.fondo = func(f func()) { f() }
	p.Catalogo = catalogoEjemplo()
	p.paleta.Modelos = p.Modelos()
	p.Objeto()
	return p, sf
}

func facturas() Modelo { return catalogoEjemplo().Modulos[0].Modelos[0] }

// TestLaListaPideAlServicioLoQueSeFiltra: las píldoras, el orden y la papelera cambiaban
// el estado de la lista y NO LLEGABAN al servicio —la petición solo llevaba la
// búsqueda—, así que pulsar «Pagado» repintaba la píldora y dejaba las mismas filas.
//
// VISTO EN ROJO: con `filtro` devolviendo solo `Q` y `Limite` (lo que hacía antes
// `cargarLista`), las tres últimas consultas salen como la primera.
func TestLaListaPideAlServicioLoQueSeFiltra(t *testing.T) {
	p, sf := pantallaConServicio(t, func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/estadisticas") {
			_, _ = io.WriteString(w, `{"total":42,"por_estado":{"pagado":40,"borrador":2},"suma_dinero":1210000}`)
			return
		}
		_, _ = io.WriteString(w, `{"registros":[]}`)
	})
	p.Abrir(facturas())
	l := p.lista
	l.AlEstado("pagado")
	l.AlOrdenar(l.Orden.Siguiente("total"))
	l.AlPapelera(true)

	var listas []string
	for _, v := range sf.vistas() {
		if strings.HasPrefix(v, "GET /api/v1/factura_venta/?") {
			listas = append(listas, strings.TrimPrefix(v, "GET /api/v1/factura_venta/?"))
		}
	}
	quiere := []string{
		"limite=50",
		"estado=pagado&limite=50",
		"estado=pagado&limite=50&orden=total",
		"estado=pagado&limite=50&orden=total&papelera=1",
	}
	if !slices.Equal(listas, quiere) {
		t.Errorf("consultas = %q\nquiere     %q", listas, quiere)
	}
	// Y el recuento del servidor llega a la cabecera y a las píldoras.
	if l.Total != 42 || l.PorEstado["borrador"] != 2 || l.SumaDinero != 1210000 {
		t.Errorf("recuento: total %d, por estado %v, suma %d", l.Total, l.PorEstado, l.SumaDinero)
	}
	textos := textosDe(l.Objeto(tema.Claro))
	for _, s := range []string{"Todos (42)", "Borrador (2)", "42 registros cargados"} {
		if !contiene(textos, s) {
			t.Errorf("no se pinta «%s»; se pintó %v", s, textos)
		}
	}
}

// TestVerMasAnadeYNoSustituye: la página siguiente va DETRÁS de lo que había, y las
// etiquetas de las relaciones se acumulan (las filas de la primera siguen necesitándolas).
func TestVerMasAnadeYNoSustituye(t *testing.T) {
	p, sf := pantallaConServicio(t, func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasSuffix(r.URL.Path, "/estadisticas"):
			w.WriteHeader(http.StatusForbidden)
		case r.URL.Query().Get("cursor") == "c2":
			_, _ = io.WriteString(w, `{"registros":[{"id":"3","data":{"cliente":"k3"}}],"etiquetas":{"k3":"Tercero"}}`)
		default:
			_, _ = io.WriteString(w, `{"registros":[{"id":"1","data":{"cliente":"k1"}},{"id":"2","data":{}}],`+
				`"etiquetas":{"k1":"Primero"},"siguiente":"c2"}`)
		}
	})
	p.Abrir(facturas())
	l := p.lista
	if !l.HayMas || len(l.registros()) != 2 {
		t.Fatalf("primera página: %d filas, hay más %v", len(l.registros()), l.HayMas)
	}
	l.AlMas()
	if n := len(l.registros()); n != 3 || l.HayMas {
		t.Fatalf("tras «Ver más» hay %d filas y hay más = %v; se esperaban 3 y false", n, l.HayMas)
	}
	if l.Pagina.Etiquetas["k1"] != "Primero" || l.Pagina.Etiquetas["k3"] != "Tercero" {
		t.Errorf("las etiquetas no se acumulan: %v", l.Pagina.Etiquetas)
	}
	if !slices.ContainsFunc(sf.vistas(), func(v string) bool { return strings.Contains(v, "cursor=c2") }) {
		t.Error("«Ver más» no mandó el cursor")
	}
	// Sin recuento del servidor (403) se cuenta lo que hay a la vista.
	if l.cuenta() != 3 {
		t.Errorf("sin estadísticas la cuenta es %d; se esperaban las 3 a la vista", l.cuenta())
	}
}

// TestUnFalloConAlgoPintadoNoBorraLaLista: con la lista ya vista, un fallo al
// refrescarla avisa en la banda y deja lo que había —seguía siendo cierto hace un
// momento—, igual que la web.
func TestUnFalloConAlgoPintadoNoBorraLaLista(t *testing.T) {
	var fallar atomic.Bool // lo lee la rutina del servidor
	p, _ := pantallaConServicio(t, func(w http.ResponseWriter, r *http.Request) {
		if fallar.Load() || strings.HasSuffix(r.URL.Path, "/estadisticas") {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		_, _ = io.WriteString(w, `{"registros":[{"id":"1","data":{}}]}`)
	})
	p.Abrir(facturas())
	fallar.Store(true)
	p.recargar()
	l := p.lista
	if len(l.registros()) != 1 || l.Fase != Exito {
		t.Fatalf("el fallo borró la lista: %d filas, fase %v", len(l.registros()), l.Fase)
	}
	if l.Aviso == "" {
		t.Error("el fallo no se avisa en la banda")
	}
}

// TestUnaRespuestaTardiaNoPisaOtraLista: si llega la respuesta de un filtro que ya no
// es el de ahora, no se pinta.
func TestUnaRespuestaTardiaNoPisaOtraLista(t *testing.T) {
	p := pantallaDePrueba(t)
	p.Objeto()
	p.Abrir(facturas())
	vieja := p.lista.clave()
	p.lista.Estado = "pagado"
	p.ponerLista(vieja, paginaEjemplo(), nil, false)
	if len(p.lista.registros()) != 0 {
		t.Error("se pintaron las filas de un filtro que ya no es el de ahora")
	}
}

// TestLosFiltrosNoPasanDeUnModeloAOtro: los estados los declara CADA modelo.
func TestLosFiltrosNoPasanDeUnModeloAOtro(t *testing.T) {
	p := pantallaDePrueba(t)
	p.Objeto()
	p.Abrir(facturas())
	l := p.lista
	l.Estado, l.Papelera, l.Orden = "pagado", true, Orden{Campo: "total"}
	l.buscador.PonerTexto("lobo")
	p.Abrir(catalogoEjemplo().Modulos[1].Modelos[0])
	if l.Estado != "" || l.Papelera || l.Orden.Campo != "" || l.Busqueda != "" || l.buscador.Texto() != "" {
		t.Errorf("filtros arrastrados: estado %q, papelera %v, orden %q, búsqueda %q/%q",
			l.Estado, l.Papelera, l.Orden.Campo, l.Busqueda, l.buscador.Texto())
	}
	if got := p.Recientes; !slices.Equal(got, []string{"cliente", "factura_venta"}) {
		t.Errorf("recientes = %v", got)
	}
}

// TestRestablecerVistaSoloConAlgoQueQuitar: el botón de la derecha de los filtros solo
// existe con estado, búsqueda o papelera puestos.
func TestRestablecerVistaSoloConAlgoQueQuitar(t *testing.T) {
	p := pantallaDePrueba(t)
	p.Objeto()
	p.Abrir(facturas())
	l := p.lista
	l.Fase, l.Pagina = Exito, paginaEjemplo()
	rotulo := p.t("action.resetView")
	if contiene(textosDe(l.Objeto(tema.Claro)), rotulo) {
		t.Errorf("sin filtros se ofrece «%s»", rotulo)
	}
	l.Papelera = true
	if !contiene(textosDe(l.Objeto(tema.Claro)), rotulo) {
		t.Errorf("con la papelera puesta no se ofrece «%s»", rotulo)
	}
	l.AlLimpiar()
	if l.Papelera {
		t.Error("«Restablecer vista» no quita la papelera")
	}
}

// TestElImporteCaeALaSumaDeLaPagina: sin acumulado del servidor, la insignia azul suma
// lo que hay a la vista en la columna de dinero, como la web.
func TestElImporteCaeALaSumaDeLaPagina(t *testing.T) {
	l := NuevaLista(traductor(t))
	l.Modelo, l.Pagina = facturas(), paginaEjemplo()
	if suma, hay := l.importe(); !hay || suma != 123456+9900 {
		t.Errorf("importe = %d (%v); se esperaba la suma de la página, %d", suma, hay, 123456+9900)
	}
	l.SumaDinero = 5
	if suma, _ := l.importe(); suma != 5 {
		t.Errorf("con acumulado del servidor manda el servidor: %d", suma)
	}
	l.SumaDinero, l.Pagina = 0, Pagina{}
	if _, hay := l.importe(); hay {
		t.Error("sin filas no hay insignia de importe")
	}
}

// TestLaPaletaSeAbreConElFocoEnSuCampo: es el `autofocus` de la web; se abre para
// escribir. (Que ⌘K llegue aquí y no al buscador de la suite lo guarda
// `ui.TestUnaAppSeQuedaElAtajoDelBuscador`: el lienzo de pruebas no dispara atajos.)
func TestLaPaletaSeAbreConElFocoEnSuCampo(t *testing.T) {
	p := pantallaDePrueba(t)
	barra := ui.NuevaBarraSuite(p.tr, nil, iconos.ERP)
	w := test.NewWindow(barra.Montar(p.Objeto()))
	defer w.Close()
	w.SetPadded(false)
	w.Resize(fyne.NewSize(1280, 800))
	p.Atajos(w.Canvas(), barra)

	p.AbrirPaleta()
	if !p.abierta {
		t.Fatal("la paleta no se abre")
	}
	if w.Canvas().Focused() != p.paleta.Campo() {
		t.Errorf("la paleta se abre sin el foco en su campo (enfocado: %T)", w.Canvas().Focused())
	}
}

// TestLaPaletaSeRecorreConElTeclado: flechas, Intro y Escape, desde el campo.
func TestLaPaletaSeRecorreConElTeclado(t *testing.T) {
	p := pantallaDePrueba(t)
	p.Objeto()
	p.AbrirPaleta()
	pa := p.paleta
	pa.Teclear(&fyne.KeyEvent{Name: fyne.KeyDown})
	if pa.Cursor != 1 {
		t.Fatalf("la flecha abajo deja el cursor en %d", pa.Cursor)
	}
	pa.Teclear(&fyne.KeyEvent{Name: fyne.KeyReturn})
	if p.abierta || p.lista.Modelo.Clave != "cliente" {
		t.Fatalf("Intro no abre lo resaltado: abierta %v, modelo %q", p.abierta, p.lista.Modelo.Clave)
	}
	p.AbrirPaleta()
	if pa.Cursor != 0 || pa.Consulta != "" {
		t.Errorf("la paleta reabre con cursor %d y consulta %q", pa.Cursor, pa.Consulta)
	}
	pa.Teclear(&fyne.KeyEvent{Name: fyne.KeyEscape})
	if p.abierta {
		t.Error("Escape no cierra la paleta")
	}
	// Una letra NO es de la paleta: va al campo.
	if pa.Teclear(&fyne.KeyEvent{Name: fyne.KeyA}) {
		t.Error("la paleta se come una tecla que es del campo")
	}
}
