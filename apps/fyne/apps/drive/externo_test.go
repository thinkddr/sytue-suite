// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package drive

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"sync"
	"testing"

	"fyne.io/fyne/v2"

	"git.sytue.eu/sytue/one/apps/fyne/tema"
	"git.sytue.eu/sytue/one/apps/fyne/ui"
	"git.sytue.eu/sytue/one/apps/nucleo/api"
)

// servicioExterno hace de Drive con el almacén de terceros: 403 si no se administra.
type servicioExterno struct {
	mu         sync.Mutex
	admin      bool
	escrituras []string
	cuentas    *api.CuentasAlmacenExterno
	// falloAlGuardar es el motivo con el que el PUT contesta 422, como un almacén que
	// no responde a la prueba del servicio.
	falloAlGuardar string
	nasListo       bool
}

func (s *servicioExterno) servidor(t *testing.T) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		cuerpo, _ := io.ReadAll(r.Body)
		s.mu.Lock()
		defer s.mu.Unlock()
		switch r.Method + " " + r.URL.Path {
		case "GET /api/v1/storage/external":
			if !s.admin {
				w.WriteHeader(http.StatusForbidden)
				return
			}
			_, _ = io.WriteString(w, `{"connected":false,"available":true}`)
		case "GET /api/v1/storage/external/instances":
			if s.cuentas != nil {
				_ = json.NewEncoder(w).Encode(s.cuentas)
				return
			}
			_, _ = io.WriteString(w, `{"items":[{"id":"i1","template_key":"google-drive","name":"Ventas","connected":true,"oauth_available":true}],"providers":{"gdrive":true,"onedrive":true,"dropbox":true}}`)
		case "GET /api/v1/storage/external/nas/status":
			_ = json.NewEncoder(w).Encode(api.EstadoNAS{Command: "docker run newt", Online: s.nasListo, Reachable: s.nasListo})
		case "GET /api/v1/folders/root":
			_, _ = io.WriteString(w, `{"id":"raiz","is_root":true}`)
		case "GET /api/v1/folders/raiz/children":
			_, _ = io.WriteString(w, `{"folders":[],"files":[]}`)
		case "GET /api/v1/quota":
			_, _ = io.WriteString(w, `{"limit_bytes":100,"used_bytes":1}`)
		default:
			s.escrituras = append(s.escrituras, strings.TrimSpace(r.Method+" "+r.URL.Path+" "+string(cuerpo)))
			switch {
			case r.Method == http.MethodPut && s.falloAlGuardar != "":
				w.WriteHeader(http.StatusUnprocessableEntity)
				_ = json.NewEncoder(w).Encode(map[string]any{"error": map[string]string{"code": "unprocessable", "message": s.falloAlGuardar}})
			case r.URL.Path == "/api/v1/storage/external/nas":
				_, _ = io.WriteString(w, `{"endpoint":"https://nas-x.sytue.eu","command":"docker run newt"}`)
			default:
				_, _ = io.WriteString(w, `{"ok":true}`)
			}
		}
	}))
	t.Cleanup(srv.Close)
	return srv
}

func (s *servicioExterno) escritas() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return slices.Clone(s.escrituras)
}

func nubeDe(t *testing.T, kind string) nubeExterna {
	t.Helper()
	for _, n := range nubesExternas {
		if n.kind == kind {
			return n
		}
	}
	t.Fatalf("no hay nube %q", kind)
	return nubeExterna{}
}

// botonDelPie busca en el pie el botón con ese texto.
func botonDelPie(de *dialogoExterno, texto string) *ui.Boton {
	var b *ui.Boton
	recorrer(de.pie, func(o fyne.CanvasObject) {
		if x, ok := o.(*ui.Boton); ok && x.Texto == texto {
			b = x
		}
	})
	return b
}

// TestAnadirAlmacenamientoEsUnaSolaEntrada: «Añadir almacenamiento» es la única fila de
// alta. Sale para quien administra y para quien puede conectar una nube personal; a
// quien no puede ninguna de las dos no se le ofrece. Ya no hay «Conectar <proveedor>»
// por cada nube ni enlace aparte en el pie.
func TestAnadirAlmacenamientoEsUnaSolaEntrada(t *testing.T) {
	for _, c := range []struct {
		admin, personal, sale bool
	}{{false, false, false}, {false, true, true}, {true, false, true}} {
		s := &servicioExterno{admin: c.admin}
		p := pantallaDePrueba(t, s.servidor(t))
		p.cargarExterno()
		if (p.lateral.Externo != nil) != c.admin {
			t.Errorf("administra=%v y el almacén se cargó=%v", c.admin, p.lateral.Externo != nil)
		}
		if c.personal {
			p.lateral.Nubes = &api.NubesPersonales{Providers: []api.ProveedorNube{{Provider: "dropbox", Available: true}}}
		}
		lat := p.lateral.Objeto(tema.Claro)
		if contieneTexto(lat, p.tr.T("storage.add", nil)) != c.sale {
			t.Errorf("%+v: «Añadir almacenamiento» sale=%v", c, !c.sale)
		}
		if contieneTexto(lat, "Dropbox") {
			t.Errorf("%+v: sigue habiendo una fila por proveedor", c)
		}
	}
}

// TestGuardarUnServidorPideElAvisoYEnseñaElFallo: un solo formulario; sin aceptar el
// aviso no se guarda, y guardar es UNA petición (no hay «comprobar» aparte). Si el
// servicio no llega al almacén, su motivo sale en el diálogo, que sigue abierto.
//
// VISTO EN ROJO (2026-10-21): sin el `!de.acepta` de `Completo`, se guarda sin aviso.
func TestGuardarUnServidorPideElAvisoYEnseñaElFallo(t *testing.T) {
	s := &servicioExterno{admin: true, falloAlGuardar: "bucket not found"}
	p := pantallaDePrueba(t, s.servidor(t))
	p.cargarExterno()
	de := p.pedirExterno()
	if de == nil {
		t.Fatal("el diálogo no se abre")
	}
	de.elegirServidor("s3")
	de.endpoint.PonerTexto("s3.fr-par.scw.cloud")
	de.bucket.PonerTexto("copias")
	de.acceso.PonerTexto("SCW123")
	de.secreta.PonerTexto("secreto")
	if de.Completo() {
		t.Fatal("sin aceptar el aviso se puede guardar")
	}
	de.acepta = true
	if !de.Completo() {
		t.Fatal("con todo escrito y el aviso aceptado no se puede guardar")
	}
	de.guardar()
	escrito := s.escritas()
	if len(escrito) != 1 || !strings.HasPrefix(escrito[0], "PUT /api/v1/storage/external ") ||
		!strings.Contains(escrito[0], `"accept_terms":true`) || !strings.Contains(escrito[0], `"bucket":"copias"`) {
		t.Errorf("guardar manda %q", escrito)
	}
	if !de.abierto || de.vista != "formulario" || !strings.Contains(de.aviso, "bucket not found") {
		t.Fatalf("el fallo del servicio no se enseña en el diálogo: abierto=%v vista=%s aviso=%q", de.abierto, de.vista, de.aviso)
	}
	s.mu.Lock()
	s.falloAlGuardar = ""
	s.mu.Unlock()
	de.guardar()
	if de.abierto {
		t.Fatal("guardado, el diálogo no se cierra")
	}
}

// TestElAlmacenPropioSePintaEnLosDosTemas: cada vista, en claro y en oscuro; un token
// mal escrito entra en pánico AL PINTAR.
func TestElAlmacenPropioSePintaEnLosDosTemas(t *testing.T) {
	t.Cleanup(func() { tema.Elegir("") })
	for _, nombre := range []string{"claro", "oscuro"} {
		tema.Elegir(nombre)
		p := pantallaDePrueba(t, (&servicioExterno{admin: true}).servidor(t))
		p.cargarExterno()
		de := p.pedirExterno()
		de.aviso = "x"
		de.paraOrg = true
		de.pintar()
		p.win.Canvas().Capture()
		for _, tipo := range servidoresExternos {
			de.elegirServidor(tipo)
			p.win.Canvas().Capture()
		}
		de.tipo = "gdrive"
		de.confirmar(&api.InstanciaExterna{ID: "i1", Name: "Ventas"})
		p.win.Canvas().Capture()
		de.tipo, de.vista, de.nas = "nas", "nas", &api.EstadoNAS{Command: "docker run fosrl/newt", Online: true}
		de.pintar()
		p.win.Canvas().Capture()
		de.vista = "gestionar"
		de.estado = &api.AlmacenExterno{Connected: true, Kind: "s3", Bucket: "b"}
		de.pintar()
		p.win.Canvas().Capture()
	}
}

// TestElPieDelAlmacenParteLaLineaEnElMovil: al confirmar una cuenta son tres botones; a
// 360 no caben en una línea y bajan, pegados a la derecha (`flex-wrap justify-end`), y a
// 1280 van en una.
//
// VISTO EN ROJO (2026-09-26, en el frontal): con todos en una sola línea, a 360 el
// primero se sale por la izquierda del pie.
func TestElPieDelAlmacenParteLaLineaEnElMovil(t *testing.T) {
	for _, c := range []struct {
		ancho  float32
		lineas int
	}{{360, 2}, {1280, 1}} {
		p := pantallaDePrueba(t, (&servicioExterno{admin: true}).servidor(t))
		p.explor.Ventana = c.ancho
		p.lateral.Externo = &api.AlmacenExterno{Available: true}
		p.AutorizarAppNueva = func(context.Context, string) (string, error) { return "", nil }
		de := p.pedirExterno()
		de.tipo = "gdrive"
		de.confirmar(&api.InstanciaExterna{ID: "i1", Name: "Ventas"})
		// Medido DENTRO del diálogo abierto, que es quien decide el reparto: en la hoja
		// del teléfono el pie es el de `PIE_TELEFONO` y a todo su ancho (312 a 360, sin
		// el `p-4` de la tarjeta, que es lo que este test descontaba antes).
		p.win.Resize(fyne.NewSize(c.ancho, 800))
		de.d.Refresh()
		ancho := AnchoCuerpoDialogo(ui.DialogoMD, c.ancho)
		if c.ancho < ui.Estrecho && de.pie.Size().Width != ancho {
			t.Errorf("a %v el pie mide %v y la hoja le deja %v", c.ancho, de.pie.Size().Width, ancho)
		}
		ys := map[float32]bool{}
		for _, o := range de.pie.Objects {
			ys[o.Position().Y] = true
			if o.Position().X < 0 || o.Position().X+o.Size().Width > ancho+0.01 {
				t.Errorf("a %v un botón se sale del pie: x=%v", c.ancho, o.Position().X)
			}
		}
		if len(de.pie.Objects) != 3 || len(ys) != c.lineas {
			t.Errorf("a %v: %d botones en %d líneas, y se esperaban 3 en %d", c.ancho, len(de.pie.Objects), len(ys), c.lineas)
		}
	}
}

// TestLaNubeDeLaOrganizacionVuelveDelConsentimientoAUnaSolaPantalla: con la casilla de
// organización, un toque abre el consentimiento y a la vuelta queda UNA pantalla con la
// cuenta que el servicio confirma, el aviso y Guardar; sin campo de secreto.
func TestLaNubeDeLaOrganizacionVuelveDelConsentimientoAUnaSolaPantalla(t *testing.T) {
	for _, kind := range []string{"gdrive", "onedrive", "dropbox"} {
		t.Run(kind, func(t *testing.T) {
			s := &servicioExterno{admin: true, cuentas: &api.CuentasAlmacenExterno{Items: []api.InstanciaExterna{}, Providers: map[string]bool{kind: true}}}
			p := pantallaDePrueba(t, s.servidor(t))
			p.cargarExterno()
			de := p.pedirExterno()
			n := nubeDe(t, kind)
			inicios := 0
			p.AutorizarAppNueva = func(_ context.Context, plantilla string) (string, error) {
				inicios++
				if plantilla != n.plantilla {
					t.Errorf("se abrió la plantilla %q", plantilla)
				}
				s.mu.Lock()
				s.cuentas.Items = []api.InstanciaExterna{{ID: "primera", TemplateKey: plantilla, Name: "Mi cuenta", Connected: true, OAuthAvailable: true}}
				s.mu.Unlock()
				return "primera", nil
			}
			de.paraOrg = true
			de.elegirNube(n)
			if inicios != 1 || de.vista != "confirmar" || de.cuenta == nil || de.cuenta.ID != "primera" {
				t.Fatalf("la vuelta no lleva a confirmar la cuenta: inicios=%d vista=%s cuenta=%+v", inicios, de.vista, de.cuenta)
			}
			recorrer(de.cuerpo, func(o fyne.CanvasObject) {
				if o == de.secreta || o == de.acceso {
					t.Error("una nube por consentimiento enseña un campo de secreto")
				}
			})
			if g := botonDelPie(de, de.t("storage.external.save")); g == nil || !g.Inerte || de.Completo() {
				t.Fatal("se puede guardar sin aceptar el aviso")
			}
			de.acepta = true
			de.guardar()
			escritas := s.escritas()
			if len(escritas) != 1 || !strings.HasPrefix(escritas[0], "PUT /api/v1/storage/external ") || !strings.Contains(escritas[0], `"instance_id":"primera"`) || !strings.Contains(escritas[0], `"kind":"`+kind+`"`) {
				t.Fatalf("no se guardó la cuenta confirmada: %v", escritas)
			}
			if de.abierto {
				t.Fatal("guardado, el diálogo no se cierra")
			}
		})
	}
}

// TestLaCuentaYaAutorizadaNoRepiteElConsentimiento: si la organización ya tiene esa
// cuenta autorizada, el toque va directo a confirmar.
func TestLaCuentaYaAutorizadaNoRepiteElConsentimiento(t *testing.T) {
	p := pantallaDePrueba(t, (&servicioExterno{admin: true}).servidor(t))
	p.cargarExterno()
	de := p.pedirExterno()
	p.AutorizarAppNueva = func(context.Context, string) (string, error) {
		t.Error("se repite el consentimiento de una cuenta ya autorizada")
		return "", nil
	}
	de.paraOrg = true
	de.elegirNube(nubeDe(t, "gdrive"))
	if de.vista != "confirmar" || de.cuenta == nil || de.cuenta.ID != "i1" {
		t.Fatalf("no se reutiliza la cuenta: vista=%s cuenta=%+v", de.vista, de.cuenta)
	}
}

// TestSinProveedorNoSeOfreceLaNube: sin el proveedor registrado la tarjeta va inerte y
// el toque no abre nada.
func TestSinProveedorNoSeOfreceLaNube(t *testing.T) {
	s := &servicioExterno{admin: true, cuentas: &api.CuentasAlmacenExterno{Providers: map[string]bool{}}}
	p := pantallaDePrueba(t, s.servidor(t))
	p.cargarExterno()
	de := p.pedirExterno()
	abiertas := 0
	p.AutorizarAppNueva = func(context.Context, string) (string, error) { abiertas++; return "", nil }
	p.AutorizarNube = func(context.Context, string) (string, error) { abiertas++; return "", nil }
	for _, org := range []bool{false, true} {
		de.paraOrg = org
		de.pintar()
		for _, n := range nubesExternas {
			de.elegirNube(n)
		}
		recorrer(de.cuerpo, func(o fyne.CanvasObject) {
			if x, ok := o.(*ui.TarjetaEleccion); ok && x.Titulo == "Google Drive" && !x.Inerte {
				t.Errorf("paraOrg=%v: la tarjeta de un proveedor sin registrar responde", org)
			}
		})
	}
	if abiertas != 0 || !de.abierto {
		t.Fatal("se abre un consentimiento imposible")
	}
}

// TestLaNubePersonalEsUnToque: sin la casilla, el toque es la nube PERSONAL de siempre:
// se cierra el selector y se abre su consentimiento.
func TestLaNubePersonalEsUnToque(t *testing.T) {
	p := pantallaDePrueba(t, (&servicioExterno{}).servidor(t))
	p.lateral.Nubes = &api.NubesPersonales{Providers: []api.ProveedorNube{{Provider: "onedrive", Available: true}}}
	de := p.pedirExterno()
	var pedida string
	p.AutorizarNube = func(_ context.Context, proveedor string) (string, error) {
		pedida = proveedor
		return "", context.Canceled
	}
	p.ConIntegraciones(api.NewIntegraciones("http://127.0.0.1:1", sesionDoble{}, nil))
	de.elegirNube(nubeDe(t, "onedrive"))
	if pedida != "onedrive" || de.abierto {
		t.Fatalf("la nube personal no es un toque: pedida=%q abierto=%v", pedida, de.abierto)
	}
}

// TestReintentarOAuthConservaElBorrador: un consentimiento denegado deja un borrador; el
// reintento lo reutiliza en vez de crear otra cuenta.
func TestReintentarOAuthConservaElBorrador(t *testing.T) {
	s := &servicioExterno{admin: true, cuentas: &api.CuentasAlmacenExterno{Items: []api.InstanciaExterna{}, Providers: map[string]bool{"gdrive": true}}}
	p := pantallaDePrueba(t, s.servidor(t))
	p.cargarExterno()
	de := p.pedirExterno()
	nuevas, reintentos := 0, 0
	p.AutorizarAppNueva = func(context.Context, string) (string, error) {
		nuevas++
		return "borrador", api.ErrOAuthDenied
	}
	p.AutorizarApp = func(_ context.Context, plantilla, instancia string) (string, error) {
		reintentos++
		if plantilla != "google-drive" || instancia != "borrador" {
			t.Error("el reintento reemplaza otra cuenta")
		}
		s.mu.Lock()
		s.cuentas.Items = []api.InstanciaExterna{{ID: "borrador", TemplateKey: plantilla, Connected: true, OAuthAvailable: true}}
		s.mu.Unlock()
		return instancia, nil
	}
	de.paraOrg = true
	de.elegirNube(nubeDe(t, "gdrive"))
	if de.pendiente != "borrador" || de.vista != "elegir" || de.aviso == "" {
		t.Fatal("denegar permisos se considera una cuenta conectada")
	}
	de.elegirNube(nubeDe(t, "gdrive"))
	if nuevas != 1 || reintentos != 1 || de.vista != "confirmar" || de.cuenta == nil || de.cuenta.ID != "borrador" {
		t.Fatal("reintentar crea otra cuenta o no adopta la verificada")
	}
}

// TestElNasEnseñaElComandoYCierraAlResponder: crear el túnel enseña el comando y el
// estado; cuando el WebDAV responde por el túnel, se avisa y se cierra.
func TestElNasEnseñaElComandoYCierraAlResponder(t *testing.T) {
	s := &servicioExterno{admin: true}
	p := pantallaDePrueba(t, s.servidor(t))
	p.cargarExterno()
	de := p.pedirExterno()
	de.elegirServidor("nas")
	de.endpoint.PonerTexto("http://192.168.1.50:5005")
	de.acceso.PonerTexto("yo")
	de.secreta.PonerTexto("x")
	de.acepta = true
	de.guardar()
	if e := s.escritas(); len(e) != 1 || !strings.HasPrefix(e[0], "POST /api/v1/storage/external/nas ") || !strings.Contains(e[0], `"accept_terms":true`) {
		t.Fatalf("crear el túnel manda %q", e)
	}
	if de.vista != "nas" || de.nas == nil || !contieneTexto(de.cuerpo, p.tr.T("storage.external.nas.command", nil)) {
		t.Fatalf("no se enseña el comando: vista=%s nas=%+v", de.vista, de.nas)
	}
	s.mu.Lock()
	s.nasListo = true
	s.mu.Unlock()
	de.sondeando = false
	de.sondearNas()
	if de.abierto {
		t.Fatal("con el NAS respondiendo, el diálogo sigue esperando")
	}
}
