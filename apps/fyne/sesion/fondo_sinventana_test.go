// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package sesion

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"fyne.io/fyne/v2/test"

	"git.sytue.eu/sytue/one/apps/nucleo/ajustes"
	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/auth"
	"git.sytue.eu/sytue/one/apps/nucleo/widgets"
	"git.sytue.eu/sytue/one/apps/nucleo/widgets/fuentes"
)

// bancoSinVentana es un proceso de Android despertado por su widget: sin Suite, con la
// sesión guardada como la guarda la app (el almacén del cliente del móvil) y un servicio
// que apunta cada petición. Su Fábrica lee del servicio con la sesión que recibe.
type bancoSinVentana struct {
	mu           sync.Mutex
	rutas        []string
	tokens       []string
	fabricas     atomic.Int32
	renovaciones atomic.Int32
	valor        atomic.Int64
}

func nuevoBancoSinVentana(t *testing.T) *bancoSinVentana {
	t.Helper()
	aparte(t)
	t.Cleanup(widgets.SimularLector(true))
	anterior := suiteViva.Swap(nil)
	t.Cleanup(func() { suiteViva.Store(anterior) })
	b := &bancoSinVentana{}
	b.valor.Store(4)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b.mu.Lock()
		b.rutas = append(b.rutas, r.Method+" "+r.URL.Path)
		b.tokens = append(b.tokens, r.Header.Get("Authorization"))
		b.mu.Unlock()
		_, _ = w.Write([]byte(`{}`))
	}))
	t.Cleanup(srv.Close)
	fabrica, url, renovar := fabricaSinVentana, urlSinVentana, renovarSinVentana
	t.Cleanup(func() { fabricaSinVentana, urlSinVentana, renovarSinVentana = fabrica, url, renovar })
	// Sytue ID: lo guardado es solo el refresh, así que la primera lectura renueva.
	renovarSinVentana = func(*auth.OIDC) auth.Renewer {
		return func(_ context.Context, t auth.Tokens) (auth.Tokens, error) {
			b.renovaciones.Add(1)
			t.Access, t.Expiry = "acceso-"+t.Subject, time.Now().Add(time.Hour)
			return t, nil
		}
	}
	urlSinVentana = func(string) string { return srv.URL }
	fabricaSinVentana = func(producto string) (fuentes.Fabrica, bool) {
		if _, ok := fuentes.Registrada(producto); !ok {
			return nil, false
		}
		return func(_ fuentes.Comunes, url func(string) string, ses api.Session) widgets.Fuente {
			b.fabricas.Add(1)
			cli := api.NewDocs(url("docs"), ses, nil)
			return func(ctx context.Context) (widgets.Detail, error) {
				if _, _, err := cli.List(ctx, api.DocsFilter{Kind: api.DocText}); err != nil {
					return widgets.Detail{}, err
				}
				return conDocumentos(b.valor.Load()), nil
			}
		}, true
	}
	return b
}

// guardarMovil guarda la sesión de `sujeto` como la guarda la app en el teléfono: en el
// almacén del emisor de producción con el cliente del móvil, bajo su cerrojo.
func guardarMovil(t *testing.T, sujeto string) {
	t.Helper()
	soltar, err := ajustes.CerrojoSesion(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	defer soltar()
	tok := auth.Tokens{Refresh: "r-" + sujeto, Subject: sujeto, Expiry: time.Now().Add(time.Hour)}
	if err := oidcDe(apexSinVentana, true).Almacen().Guardar(tok); err != nil {
		t.Fatal(err)
	}
}

var docusSinVentana = SinVentana{Bundle: "eu.sytue.docus", Producto: "docs", Zona: time.UTC}

// TestSinVentanaPublicaConLaSesionGuardadaYNadaMas: despertada por el JobService, sin
// `main` ni Suite, la app lee su sesión guardada, pide lo de su Fuente con el token de
// esa sesión y publica, y no habla con el Hub (ni presencia ni SSE). Una segunda vez,
// sin cambios en el servicio, no reescribe nada.
//
// VISTO EN ROJO: capturando la vista con la marca de la sesión (marcas.generacion) en
// vez de su época, la ronda sale ErrPrivate y no se publica.
func TestSinVentanaPublicaConLaSesionGuardadaYNadaMas(t *testing.T) {
	b := nuevoBancoSinVentana(t)
	guardarMovil(t, "ana")
	if r := RefrescarSinVentana(t.Context(), docusSinVentana); r != Nuevos {
		t.Fatalf("resultado %v", r)
	}
	if n := publicado(t, "eu.sytue.docus"); n != 4 {
		t.Fatalf("en el widget hay %d documentos", n)
	}
	b.mu.Lock()
	rutas, tokens := slices.Clone(b.rutas), slices.Clone(b.tokens)
	b.mu.Unlock()
	if len(rutas) != 1 || !strings.HasPrefix(rutas[0], "GET ") {
		t.Fatalf("peticiones: %v", rutas)
	}
	for _, r := range rutas {
		if strings.Contains(r, "presence") || strings.Contains(r, "stream") || strings.Contains(r, "/me/") {
			t.Fatalf("sin ventana se ha pedido %s", r)
		}
	}
	if tokens[0] != "Bearer acceso-ana" {
		t.Fatalf("con el token %q", tokens[0])
	}
	if r := RefrescarSinVentana(t.Context(), docusSinVentana); r != SinCambios {
		t.Fatalf("sin cambios: %v", r)
	}
	b.valor.Store(9)
	if r := RefrescarSinVentana(t.Context(), docusSinVentana); r != Nuevos || publicado(t, "eu.sytue.docus") != 9 {
		t.Fatalf("con un cambio: %v, %d", r, publicado(t, "eu.sytue.docus"))
	}
}

// TestSinVentanaNoLeeSinSesionNiConElBloqueo: sin nadie dentro, con el bloqueo de la
// suite puesto (no hay interfaz con que pedir quién es), con un producto sin widget o
// con un paquete que no es de la suite, no se construye la Fuente ni se pide nada.
//
// VISTO EN ROJO: sin la comprobación de bloqueoPuesto, con el bloqueo puesto lee y
// publica.
func TestSinVentanaNoLeeSinSesionNiConElBloqueo(t *testing.T) {
	b := nuevoBancoSinVentana(t)
	if r := RefrescarSinVentana(t.Context(), docusSinVentana); r != SinCambios {
		t.Fatalf("sin sesión: %v", r)
	}
	if b.fabricas.Load() != 0 {
		t.Fatal("sin sesión se ha construido la Fuente")
	}
	guardarMovil(t, "ana")
	for _, o := range []SinVentana{
		{Bundle: "eu.sytue.docus", Producto: "fyne-demo"},
		{Bundle: "com.otro.app", Producto: "docs"},
		{Bundle: "eu.sytue.docus/../x", Producto: "docs"},
	} {
		if r := RefrescarSinVentana(t.Context(), o); r != SinCambios {
			t.Fatalf("%+v: %v", o, r)
		}
	}
	if err := ajustes.Cambiar(func(a *ajustes.Ajustes) { a.Bloqueo = true }); err != nil {
		t.Fatal(err)
	}
	if r := RefrescarSinVentana(t.Context(), docusSinVentana); r != SinCambios {
		t.Fatalf("con el bloqueo: %v", r)
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.fabricas.Load() != 0 || len(b.rutas) != 0 {
		t.Fatalf("se ha leído: %d fuentes, %v", b.fabricas.Load(), b.rutas)
	}
}

// TestSinVentanaConLaAppAbiertaSeLoDejaASuSuite: si la app está abierta en el mismo
// proceso, el refresco no lee por su cuenta (serían dos lecturas y dos sesiones): avisa a
// su publicador, que lee él.
//
// VISTO EN ROJO: sin mirar suiteViva, el refresco construye su propia Fuente.
func TestSinVentanaConLaAppAbiertaSeLoDejaASuSuite(t *testing.T) {
	b := nuevoBancoSinVentana(t)
	guardarMovil(t, "ana")
	app := test.NewApp()
	t.Cleanup(app.Quit)
	s := suiteConSesion(t, widgetApp{app}, "ana")
	reloj := widgets.NuevoRelojDePrueba(time.Now())
	s.ProbarWidgetsCon(reloj.Opciones(widgets.OpcionesPublicador{}))
	var lecturas atomic.Int32
	s.PublicadorWidget(func(context.Context) (widgets.Detail, error) {
		lecturas.Add(1)
		return conDocumentos(2), nil
	})
	suiteViva.Store(s)
	if r := RefrescarSinVentana(t.Context(), docusSinVentana); r != SinCambios {
		t.Fatalf("resultado %v", r)
	}
	if b.fabricas.Load() != 0 {
		t.Fatal("con la app abierta, el refresco ha construido su propia Fuente")
	}
	reloj.Avanzar(widgets.EsperaMaxWidget)
	limite := time.Now().Add(5 * time.Second)
	for lecturas.Load() == 0 {
		if time.Now().After(limite) {
			t.Fatal("el publicador de la app no se ha enterado")
		}
		time.Sleep(5 * time.Millisecond)
	}
}

// TestConRespaldoUsaLaActividadSiLaHay: la puerta a Java del refresco sin ventana usa la
// de la actividad en cuanto existe, y la propia mientras no.
//
// VISTO EN ROJO: llamando siempre a la propia, con la actividad ya creada.
func TestConRespaldoUsaLaActividadSiLaHay(t *testing.T) {
	var hayActividad bool
	var porActividad, porPropia int
	actividad := func(f func(vm, env, ctx uintptr) error) error {
		if !hayActividad {
			return errors.New("no current JVM")
		}
		porActividad++
		return f(1, 1, 1)
	}
	propia := func(f func(vm, env, ctx uintptr) error) error {
		porPropia++
		return f(2, 2, 2)
	}
	puerta := conRespaldo(actividad, propia)
	var ctx uintptr
	_ = puerta(func(_, _, c uintptr) error { ctx = c; return nil })
	if ctx != 2 || porPropia != 1 {
		t.Fatalf("sin actividad: contexto %d, %d por la propia", ctx, porPropia)
	}
	hayActividad = true
	_ = puerta(func(_, _, c uintptr) error { ctx = c; return nil })
	if ctx != 1 || porPropia != 1 {
		t.Fatalf("con actividad: contexto %d, %d por la propia", ctx, porPropia)
	}
	if err := conRespaldo(nil, propia)(func(_, _, _ uintptr) error { return nil }); err != nil || porPropia != 2 {
		t.Fatalf("sin toolkit: %v, %d", err, porPropia)
	}
}

func TestZonaDeAndroid(t *testing.T) {
	if z := zonaDe("Europe/Madrid", 3600); z.String() != "Europe/Madrid" {
		t.Fatalf("zona %v", z)
	}
	z := zonaDe("Marte/Olympus", 7200)
	if _, d := time.Now().In(z).Zone(); d != 7200 {
		t.Fatalf("sin tzdata de esa zona, el desfase es %d", d)
	}
}

// TestSoloOneReleva: One, al oír la señal de un producto, se la pasa a la app de ese
// producto; ninguna otra app releva (cada una oye la suya con la app abierta), ni a un
// producto que no tiene app, ni a sí misma.
//
// VISTO EN ROJO: sin comprobar que esta app es One, Docus relevaba a Mail.
func TestSoloOneReleva(t *testing.T) {
	anterior := relevarWidget
	t.Cleanup(func() { relevarWidget = anterior })
	relevados := make(chan string, 8)
	relevarWidget = func(p string) { relevados <- p }
	app := test.NewApp()
	t.Cleanup(app.Quit)
	one := &Suite{app: appDeWidget{App: app, id: "eu.sytue.one", esquema: "sytue-one"}}
	for _, p := range []string{"mail", "sheets", "one", "id", ""} {
		one.AvisoDelHub(p)
	}
	docus := &Suite{app: widgetApp{app}}
	docus.AvisoDelHub("mail")
	var vistos []string
	for {
		select {
		case p := <-relevados:
			vistos = append(vistos, p)
			continue
		case <-time.After(200 * time.Millisecond):
		}
		break
	}
	slices.Sort(vistos)
	if !slices.Equal(vistos, []string{"eu.sytue.calcus", "eu.sytue.mail"}) {
		t.Fatalf("relevados %v", vistos)
	}
}

// TestFuenteWidgetEsLaRegistrada: el `cmd` y el refresco sin ventana construyen la
// misma Fuente, la de su producto; una app sin widget, ninguna.
func TestFuenteWidgetEsLaRegistrada(t *testing.T) {
	app := test.NewApp()
	t.Cleanup(app.Quit)
	if (&Suite{app: widgetApp{app}, apex: api.ApexProd}).FuenteWidget(nil) == nil {
		t.Fatal("Docus sin Fuente")
	}
	demo := &Suite{app: appDeWidget{App: app, id: "eu.sytue.fyne.demo", esquema: "sytue-fyne-demo"}}
	if demo.FuenteWidget(nil) != nil {
		t.Fatal("la demo con Fuente")
	}
}

// TestLaEntradaJNIEsLaDeJava ata los dos lados del refresco sin ventana, que solo se
// encuentran en un teléfono: los códigos del Resultado que lee RefrescoWidgets, el
// nombre y los argumentos de su método nativo con la exportación de Go, y la firma de
// RelevoWidgets.avisar con la que One lo llama.
//
// VISTO EN ROJO cambiando FALLO = 2 por 4 en RefrescoReglas.java, y quitando el
// `int desfase` del método nativo.
func TestLaEntradaJNIEsLaDeJava(t *testing.T) {
	leer := func(ruta string) string {
		t.Helper()
		b, err := os.ReadFile(ruta)
		if err != nil {
			t.Fatal(err)
		}
		return string(b)
	}
	java := filepath.Join("..", "android", "java", "eu", "sytue", "nativo")
	reglas := leer(filepath.Join(java, "RefrescoReglas.java"))
	for nombre, r := range map[string]Resultado{"SIN_CAMBIOS": SinCambios, "NUEVOS": Nuevos, "FALLO": Fallo, "APLAZADO": Aplazado} {
		if !strings.Contains(reglas, fmt.Sprintf("static final int %s = %d;", nombre, int(r))) {
			t.Errorf("RefrescoReglas.%s no es %d (%v)", nombre, int(r), r)
		}
	}
	refresco := leer(filepath.Join(java, "RefrescoWidgets.java"))
	if !strings.Contains(refresco, "static native int refrescar(Context ctx, String paquete, String producto, String zona, int desfase);") {
		t.Error("RefrescoWidgets.refrescar ya no es (Context, String, String, String, int) → int")
	}
	entrada := leer("fondo_android.go")
	if !regexp.MustCompile(`func Java_eu_sytue_nativo_RefrescoWidgets_refrescar\(env \*C\.JNIEnv, _ C\.jclass, contexto C\.jobject,\s+paquete, producto, zona C\.jstring, desfase C\.jint\) \(r C\.jint\)`).MatchString(entrada) ||
		!strings.Contains(entrada, "//export Java_eu_sytue_nativo_RefrescoWidgets_refrescar") {
		t.Error("la exportación de Go no casa con RefrescoWidgets.refrescar")
	}
	relevo := leer(filepath.Join(java, "RelevoWidgets.java"))
	if !strings.Contains(relevo, "public static boolean avisar(Context ctx, String paquete)") ||
		!strings.Contains(entrada, `"avisar", "(Landroid/content/Context;Ljava/lang/String;)Z"`) {
		t.Error("RelevoWidgets.avisar no casa con la llamada de One")
	}
}
