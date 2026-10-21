// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

// Checks del control de REDIRECCIONES, que es el bypass clásico de una barrera
// SSRF: el destino que se valida es público, responde 302 y manda al cliente a
// 169.254.169.254 — las credenciales de la instancia en la nube. Una barrera que
// solo mira la URL de entrada deja pasar eso, y no da ningún error: devuelve el
// contenido del servicio interno como si fuera la respuesta del proveedor.
//
// `NewClient` ya lo hace bien (revalida cada salto con `ValidateURL` desde
// `CheckRedirect`), pero no había ni un test que lo fijara.
//
// QUÉ PRUEBA CADA COSA, dicho con precisión porque el sabotaje me corrigió:
// los tests de cliente comprueban el RESULTADO —que no se conecta— con las dos
// barreras puestas, no la capa concreta que lo impide. Quitando la revalidación
// de `CheckRedirect` siguen pasando, porque la barrera 2 (el dialer, que
// re-resuelve y valida al conectar) los ataja igual. Eso NO es un fallo del
// diseño: es exactamente lo que promete una defensa en profundidad, y saberlo
// vale más que un test que aparente cubrir una capa que no cubre.
// `TestValidateURLEsLaQueCortaElSalto` es el que sí aísla la barrera 1.
package ssrfhttp

import (
	"context"
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// soloLoopback permite loopback (para que httptest funcione) y mantiene
// bloqueado TODO lo demás que bloquearía producción. Es lo que hace posible
// probar el salto a un destino privado sin desactivar la barrera entera: si
// relajáramos la política por completo, el test pasaría también con la defensa
// rota, que es exactamente el test que no sirve para nada.
func soloLoopback(t *testing.T) {
	t.Helper()
	restore := SetIPPolicyForTest(func(ip net.IP) bool {
		if ip != nil && ip.IsLoopback() {
			return false // permitida: es el servidor del test
		}
		return IsBlockedIP(ip) // el resto, como en producción
	})
	t.Cleanup(restore)
}

func TestRedirectABloqueadoSeCorta(t *testing.T) {
	soloLoopback(t)

	// Destino "público" desde el punto de vista del test, que redirige a la IP
	// de metadatos de la nube. Es el ataque, literalmente.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "http://169.254.169.254/latest/meta-data/", http.StatusFound)
	}))
	defer srv.Close()

	_, err := NewClient(5*time.Second, 5).Get(srv.URL)
	if err == nil {
		t.Fatal("el redirect a 169.254.169.254 debería cortarse; no dio error")
	}
	if !errors.Is(err, ErrBlocked) {
		t.Errorf("err = %v; se esperaba ErrBlocked", err)
	}
}

func TestRedirectAPrivadaSeCorta(t *testing.T) {
	soloLoopback(t)

	// Variante con RFC1918: el mismo ataque contra un servicio de la red interna.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "http://10.0.0.5:8080/admin", http.StatusMovedPermanently)
	}))
	defer srv.Close()

	if _, err := NewClient(5*time.Second, 5).Get(srv.URL); !errors.Is(err, ErrBlocked) {
		t.Errorf("err = %v; se esperaba ErrBlocked", err)
	}
}

func TestRedirectAEsquemaNoHTTPSeCorta(t *testing.T) {
	soloLoopback(t)

	// `file://` no lo bloquea ninguna comprobación de IP: lo para la validación
	// de esquema, que también se ejecuta en cada salto.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "file:///etc/passwd", http.StatusFound)
	}))
	defer srv.Close()

	_, err := NewClient(5*time.Second, 5).Get(srv.URL)
	if err == nil {
		t.Fatal("un redirect a file:// debería cortarse")
	}
	if !errors.Is(err, ErrInvalidURL) {
		t.Errorf("err = %v; se esperaba ErrInvalidURL", err)
	}
}

func TestRedirectAPublicaSigue(t *testing.T) {
	soloLoopback(t)

	// El caso simétrico, y no es de adorno: una barrera que corta TODOS los
	// redirects «pasaría» los tres tests de arriba y rompería media integración
	// —los proveedores redirigen constantemente: apex→www, http→https, CDN—.
	destino := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("llegué"))
	}))
	defer destino.Close()

	origen := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, destino.URL, http.StatusFound)
	}))
	defer origen.Close()

	res, err := NewClient(5*time.Second, 5).Get(origen.URL)
	if err != nil {
		t.Fatalf("un redirect a un destino permitido no debería cortarse: %v", err)
	}
	defer func() { _ = res.Body.Close() }()
	if res.StatusCode != http.StatusOK {
		t.Errorf("code = %d, quería 200", res.StatusCode)
	}
}

func TestCadenaDeRedirectsAcotada(t *testing.T) {
	soloLoopback(t)

	// Un bucle de redirecciones no debe girar hasta agotar el timeout: cada
	// salto es una petición de red y una resolución DNS, y con varias instancias
	// apuntando al mismo sitio eso es un amplificador.
	var srv *httptest.Server
	srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, srv.URL, http.StatusFound) // a sí mismo, para siempre
	}))
	defer srv.Close()

	_, err := NewClient(5*time.Second, 3).Get(srv.URL)
	if err == nil {
		t.Fatal("un bucle de redirects debería cortarse")
	}
	if !strings.Contains(err.Error(), "demasiados redirects") {
		t.Errorf("err = %v; se esperaba el corte por número de saltos", err)
	}
}

func TestMaxRedirectsNegativoDegradaANumeroSano(t *testing.T) {
	soloLoopback(t)

	// Un 0 o un negativo por descuido no puede desactivar el tope. Con -1 se
	// degrada a 5, así que un bucle infinito sigue cortándose.
	var srv *httptest.Server
	srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, srv.URL, http.StatusFound)
	}))
	defer srv.Close()

	if _, err := NewClient(5*time.Second, -1).Get(srv.URL); err == nil {
		t.Error("con maxRedirects negativo el tope debería degradar a 5, no desaparecer")
	}
}

// TestValidateURLEsLaQueCortaElSalto aísla la barrera 1, que es la que los tests
// de cliente NO distinguen: `CheckRedirect` llama a `ValidateURL` con la URL del
// salto siguiente, así que probar esa función directamente es probar la decisión
// que toma el cliente en cada redirección — sin que el dialer pueda taparlo.
//
// Importa que exista esta capa aunque el dialer también corte: aquí se decide
// ANTES de tocar la red, y el error dice que el destino está bloqueado en vez de
// parecer un fallo de conexión cualquiera.
func TestValidateURLEsLaQueCortaElSalto(t *testing.T) {
	ctx := context.Background()

	bloqueadas := map[string]string{
		"metadatos de la nube": "http://169.254.169.254/latest/meta-data/",
		"RFC1918":              "http://10.0.0.5:8080/admin",
		"loopback":             "http://127.0.0.1/",
		"loopback IPv6":        "http://[::1]/",
		"CGNAT":                "http://100.64.0.1/",
		"unspecified":          "http://0.0.0.0/",
	}
	for nombre, url := range bloqueadas {
		if err := ValidateURL(ctx, url); !errors.Is(err, ErrBlocked) {
			t.Errorf("%s (%s): err = %v; se esperaba ErrBlocked", nombre, url, err)
		}
	}

	// Esquemas que ninguna comprobación de IP atraparía.
	for _, url := range []string{"file:///etc/passwd", "gopher://x/", "ftp://x/"} {
		if err := ValidateURL(ctx, url); !errors.Is(err, ErrInvalidURL) {
			t.Errorf("%s: err = %v; se esperaba ErrInvalidURL", url, err)
		}
	}
}

// --- Notaciones alternativas de IP ------------------------------------------

// Un atacante no escribe «127.0.0.1» si sabe que está en una lista: escribe la
// misma dirección de otra forma. `net.ParseIP` de Go NO acepta octal ni decimal
// —a diferencia de inet_aton de C—, así que estas cadenas no son literales IP
// para la barrera 1 y caen del lado del DNS. Lo que las para entonces es la
// barrera 2, que re-resuelve y valida en el momento de conectar.
//
// El test comprueba lo que importa de verdad: que NO se conecta. Si el resolver
// del sistema las expande a loopback, la barrera 2 las bloquea; si no las
// resuelve, falla la resolución. Las dos son un fallo cerrado, y ese es el
// criterio — no qué error concreto sale.
func TestNotacionesRarasDeLoopbackNoConectan(t *testing.T) {
	// Sin relajar NADA: aquí la política es la de producción.
	casos := map[string]string{
		"octal":           "http://0177.0.0.1/",
		"decimal":         "http://2130706433/",
		"hexadecimal":     "http://0x7f000001/",
		"ipv6 comprimida": "http://[::1]/",
		"ipv4 mapeada v6": "http://[::ffff:127.0.0.1]/",
		"cero abreviado":  "http://127.1/",
		"metadatos ipv6":  "http://[fd00::1]/",
	}
	cli := NewClient(3*time.Second, 3)
	for nombre, url := range casos {
		res, err := cli.Get(url)
		if err == nil {
			_ = res.Body.Close()
			t.Errorf("%s (%s): la petición NO falló; debería ser imposible conectar", nombre, url)
		}
	}
}
