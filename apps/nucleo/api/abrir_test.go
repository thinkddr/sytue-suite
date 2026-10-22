// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package api

import (
	"context"
	"errors"
	"io"
	"maps"
	"net/http"
	"net/http/httptest"
	"net/url"
	"regexp"
	"strings"
	"testing"

	"git.sytue.eu/sytue/one/apps/nucleo/webref"
)

// conjunto lee un `const <nombre> = new Set([...])` de un fichero de la web y devuelve
// sus elementos. Se lee POR NOMBRE y no con un barrido: en `FileBrowser.tsx` hay cuatro
// conjuntos de extensiones y solo uno decide quién abre.
func conjunto(t *testing.T, fichero, nombre string) []string {
	t.Helper()
	re := regexp.MustCompile(`const ` + nombre + `[^=]*= new Set\(\[([^\]]*)\]\)`)
	m := re.FindStringSubmatch(webref.Read(t, fichero))
	if m == nil {
		t.Fatalf("%s ya no declara %s: la tabla de apertura hay que rehacerla a mano", fichero, nombre)
	}
	var out []string
	for _, crudo := range strings.Split(m[1], ",") {
		if v := strings.Trim(strings.TrimSpace(crudo), "'\""); v != "" {
			out = append(out, v)
		}
	}
	return out
}

// TestLaTablaDeAperturaEsLaDeLaWeb compara `abre` con la de la web POR IGUALDAD y en los
// dos sentidos, que es lo que la hace útil: así falla también cuando la web añade un
// formato —un `.rtf` a Docus— y aquí no se entera nadie.
//
// Visto en rojo el 2026-09-22 quitando "odp" de `abre` (dijo «la web manda odp a slides y
// aquí no lo abre nadie») y luego añadiéndole "svg": "photos" (dijo «aquí svg abre photos
// y en la web no lo abre nadie»). Los dos sentidos, comprobados uno por uno.
func TestLaTablaDeAperturaEsLaDeLaWeb(t *testing.T) {
	const drive = "apps/frontend/apps/drive/src/lib/abrir-con.ts"
	const navegador = "apps/frontend/apps/drive/src/components/FileBrowser.tsx"

	web := map[string]string{}
	for conj, producto := range map[string]string{
		"DOCS_EXTS": "docs", "SHEETS_EXTS": "sheets", "SLIDES_EXTS": "slides",
	} {
		for _, ext := range conjunto(t, drive, conj) {
			web[ext] = producto
		}
	}
	// Las imágenes van a Fotos SOLO si el servicio sabe leerlas, y eso en la web es lo
	// mismo que decide si hay miniatura (`conMiniatura: thumbable(item)`).
	for _, ext := range conjunto(t, navegador, "THUMB_EXTS") {
		web[ext] = "photos"
	}
	// El PDF no está en ningún conjunto: es un `kind` propio, reconocido por la
	// extensión en `previewKind` y devuelto el primero en `appQueAbre`.
	for fichero, literal := range map[string]string{
		navegador: `ext === 'pdf'`,
		drive:     `if (kind === 'pdf') return 'pdf';`,
	} {
		if !strings.Contains(webref.Read(t, fichero), literal) {
			t.Errorf("%s ya no dice %s: ¿sigue abriendo CertiPDF los PDF?", fichero, literal)
		}
	}
	web["pdf"] = "pdf"

	if !maps.Equal(web, abre) {
		for ext, producto := range web {
			if abre[ext] != producto {
				t.Errorf("la web abre %q con %q y aquí con %q", ext, producto, abre[ext])
			}
		}
		for ext, producto := range abre {
			if _, ok := web[ext]; !ok {
				t.Errorf("aquí %q abre %q y en la web no lo abre nadie", ext, producto)
			}
		}
	}
}

// TestLasRutasDeAperturaSonLasDeLaWeb: el respaldo abre esa MISMA página en el
// navegador, así que una ruta que aquí se escriba de otra forma es un 404 con aspecto de
// «esa app está caída». Se comprueban los tres literales de `urlDeApertura`.
func TestLasRutasDeAperturaSonLasDeLaWeb(t *testing.T) {
	fuente := webref.Read(t, "apps/frontend/apps/drive/src/lib/abrir-con.ts")
	ref := Referencia{ID: "abc-123", Ruta: "/a b/informe.pdf"}
	for producto, quiero := range map[string]string{
		"pdf":    "/pdf?path=%2Fa+b%2Finforme.pdf&volver=%2Fa+b",
		"photos": "/abrir?file=abc-123&volver=%2Fa+b",
		"docs":   "/open-from-drive?file=abc-123&volver=%2Fa+b",
		"sheets": "/open-from-drive?file=abc-123&volver=%2Fa+b",
		"slides": "/open-from-drive?file=abc-123&volver=%2Fa+b",
	} {
		if got := RutaDeApertura(producto, ref); got != quiero {
			t.Errorf("RutaDeApertura(%q) = %q, quiero %q", producto, got, quiero)
		}
		// El nombre de la página, tal cual está escrito en la web.
		pagina := strings.SplitN(strings.TrimPrefix(quiero, "/"), "?", 2)[0]
		if !strings.Contains(fuente, "`"+pagina+"?") {
			t.Errorf("abrir-con.ts ya no abre por %q", pagina)
		}
	}
	// Sin id se manda la ruta, que es el respaldo que la web tiene escrito.
	if got := RutaDeApertura("docs", Referencia{Ruta: "/x/a.odt"}); !strings.Contains(got, "path=%2Fx%2Fa.odt") {
		t.Errorf("sin id la ruta tiene que viajar: %q", got)
	}
}

// TestElPrefijoDeCadaAppEsElDeLaWeb: `rutaDe` es el `appPaths` de tokens.ts. Sin él,
// `AppURL` manda a `certipdf.sytue.eu/pdf` cuando la página vive en `/certipdf/pdf` — el fallo
// que la web ya pagó una vez y que el respaldo de aquí repetía.
func TestElPrefijoDeCadaAppEsElDeLaWeb(t *testing.T) {
	fuente := webref.Read(t, "apps/frontend/packages/config/src/tokens.ts")
	bloque := regexp.MustCompile(`(?s)const appPaths[^{]*\{(.*?)\n\};`).FindStringSubmatch(fuente)
	if bloque == nil {
		t.Fatal("tokens.ts ya no declara appPaths: revisar rutaDe a mano")
	}
	web := map[string]string{}
	for _, m := range regexp.MustCompile(`(\w+): '([^']*)'`).FindAllStringSubmatch(bloque[1], -1) {
		web[m[1]] = m[2]
	}
	if !maps.Equal(web, rutaDe) {
		t.Errorf("los prefijos no son los de la web:\n web = %v\naquí = %v", web, rutaDe)
	}
}

// abridorDoble apunta las URL y falla las que se le digan, que es como se prueba el
// respaldo sin un sistema operativo debajo.
type abridorDoble struct {
	urls  []string
	falla func(string) bool
}

func (a *abridorDoble) OpenURL(u *url.URL) error {
	a.urls = append(a.urls, u.String())
	if a.falla != nil && a.falla(u.String()) {
		return errors.New("el sistema no tiene quién atienda eso")
	}
	return nil
}

// TestAbrirFicheroPruebaElEsquemaYCaeALaWeb: las dos mitades de la acción. Con la app
// instalada se abre por esquema y no se toca el navegador; sin ella se cae a la
// dirección web CON su prefijo.
func TestAbrirFicheroPruebaElEsquemaYCaeALaWeb(t *testing.T) {
	ref := Referencia{ID: "abc-123", Ruta: "/Docs/acta.odt"}

	instalada := &abridorDoble{}
	if !AbrirFichero(instalada, ApexProd, "", "acta.odt", ref) {
		t.Fatal("un .odt lo abre Docus")
	}
	quiero := "sytue-docs:/open-from-drive?file=abc-123&volver=%2FDocs"
	if len(instalada.urls) != 1 || instalada.urls[0] != quiero {
		t.Errorf("con la app instalada = %v, quiero solo %q", instalada.urls, quiero)
	}

	sinApp := &abridorDoble{falla: func(u string) bool { return strings.HasPrefix(u, "sytue-") }}
	if !AbrirFichero(sinApp, ApexProd, "", "acta.odt", ref) {
		t.Fatal("sin la app nativa se abre el navegador, no se deja el fichero muerto")
	}
	web := "https://docus.sytue.eu/docus/open-from-drive?file=abc-123&volver=%2FDocs"
	if len(sinApp.urls) != 2 || sinApp.urls[1] != web {
		t.Errorf("el respaldo = %v, quiero %q el segundo", sinApp.urls, web)
	}

	// Con la organización de la sesión el respaldo sale CANÓNICO (ADR 0056): la org va
	// tras el prefijo, y el esquema ni se entera — es un enlace entre apps de la misma
	// sesión.
	conOrg := &abridorDoble{falla: func(u string) bool { return strings.HasPrefix(u, "sytue-") }}
	if !AbrirFichero(conOrg, ApexProd, "acme", "acta.odt", ref) {
		t.Fatal("la org no cambia quién abre el fichero")
	}
	webConOrg := "https://docus.sytue.eu/docus/acme/open-from-drive?file=abc-123&volver=%2FDocs"
	if len(conOrg.urls) != 2 || conOrg.urls[0] != quiero || conOrg.urls[1] != webConOrg {
		t.Errorf("con org = %v, quiero %q y %q", conOrg.urls, quiero, webConOrg)
	}

	// Un formato que no reclama nadie no se abre ni se intenta: quien llama sigue con
	// lo que hacía (previsualizar, descargar).
	nadie := &abridorDoble{}
	if AbrirFichero(nadie, ApexProd, "", "copia.zip", ref) || len(nadie.urls) != 0 {
		t.Errorf("un .zip no tiene app dueña: %v", nadie.urls)
	}
	// Y un fichero sin extensión tampoco.
	if _, ok := AppQueAbre("LEEME"); ok {
		t.Error("sin extensión no hay tabla que valga")
	}
	// La extensión no distingue mayúsculas, como el `toLowerCase()` de la web.
	if p, _ := AppQueAbre("Informe.PDF"); p != "pdf" {
		t.Errorf("Informe.PDF = %q, quiero pdf", p)
	}
}

// TestLoQueSeEmiteEsLoQueSeLee: el emisor y el receptor son dos mitades de lo mismo, y
// se prueban juntas o no se prueba nada. Se emite como emite Drive, se ENTREGA como
// entrega el sistema —texto que se vuelve a parsear, no un url.URL armado a mano, que
// sale con host y no coincide (PR #816)— y se lee al otro lado.
func TestLoQueSeEmiteEsLoQueSeLee(t *testing.T) {
	ref := Referencia{ID: "abc-123", Ruta: "/Docs/2026/acta.odt"}
	for _, producto := range []string{"docs", "sheets", "slides", "pdf", "photos"} {
		emisor := &abridorDoble{}
		if err := AbrirEn(emisor, ApexProd, producto, "", RutaDeApertura(producto, ref)); err != nil {
			t.Fatalf("%s: %v", producto, err)
		}
		u, err := url.Parse(emisor.urls[0])
		if err != nil {
			t.Fatalf("%s: el sistema entrega texto y no se deja parsear: %v", producto, err)
		}
		ap, ok := LeerApertura(producto, u)
		if !ok {
			t.Fatalf("%s: %q no se lee como apertura suya", producto, u)
		}
		if ap.Volver != "/Docs/2026" {
			t.Errorf("%s: volver = %q", producto, ap.Volver)
		}
		// CertiPDF abre por RUTA y el resto por identificador: es la tabla de la web,
		// no una elección de aquí.
		quiero := Referencia{ID: ref.ID}
		if producto == "pdf" {
			quiero = Referencia{Ruta: ref.Ruta}
		}
		if ap.Ref != quiero {
			t.Errorf("%s: referencia = %+v, quiero %+v", producto, ap.Ref, quiero)
		}
		// El enlace de OTRO producto no es de este: el esquema se compara entero.
		if _, ok := LeerApertura("photos", u); ok && producto != "photos" {
			t.Errorf("%s: Photos se ha quedado con un enlace ajeno", producto)
		}
	}
}

// Pages no tiene app nativa: el Hub y sus avisos abren la web directamente.
func TestPagesAbreLaWebSinIntentarUnEsquema(t *testing.T) {
	for _, ruta := range []string{"", "/edit/s1?version=v2"} {
		a := &abridorDoble{}
		if err := AbrirEn(a, ApexProd, "pages", "acme", ruta); err != nil {
			t.Fatal(err)
		}
		quiero := "https://web.sytue.eu/web/acme"
		if ruta != "" {
			quiero += ruta
		}
		if len(a.urls) != 1 || a.urls[0] != quiero {
			t.Fatalf("desde el lanzador = %v, quiero solo %q", a.urls, quiero)
		}
		a.urls = nil
		if err := AbrirURL(a, "pages", quiero); err != nil {
			t.Fatal(err)
		}
		if len(a.urls) != 1 || a.urls[0] != quiero {
			t.Fatalf("desde un aviso = %v, quiero solo %q", a.urls, quiero)
		}
	}
	a := &abridorDoble{falla: func(string) bool { return true }}
	if err := AbrirEn(a, ApexProd, "pages", "acme", ""); err == nil {
		t.Fatal("el fallo al abrir el navegador tiene que llegar a quien llama")
	}
}

// TestAbrirEnLaAplicacionLlegaALaPagina: «Abrir en la aplicación» de la web emite
// `sytue-<p>://open?url=<página>` y aquí se lee como la misma página vista por el
// esquema, que es lo que ya entienden los receptores. Antes nadie leía el `url=` y la app
// abría en su portada. VISTO EN ROJO devolviendo `false` siempre y aceptando `http`.
func TestAbrirEnLaAplicacionLlegaALaPagina(t *testing.T) {
	// El emisor es la web: si cambia la forma del enlace, esto tiene que enterarse.
	if fuente := webref.Read(t, "apps/frontend/packages/shell/src/openInApp.ts"); !strings.Contains(fuente,
		"`${scheme}://open?url=${encodeURIComponent(url)}`") {
		t.Fatal("openInApp.ts ya no emite `<esquema>://open?url=<página>`: DesdeLaWeb hay que rehacerlo")
	}
	// Y el editor web dice qué app es: por el host solo, una hoja iría a Docus.
	if !strings.Contains(webref.Read(t, "apps/frontend/packages/office/src/editor/layout/EditorHeader.tsx"),
		"openInApp(window.location.href, TESELA_POR_KIND[ctx.kind])") {
		t.Error("EditorHeader.tsx ya no pasa su producto a openInApp: las hojas y las presentaciones abrirían Docus")
	}
	for crudo, quiero := range map[string]string{
		// El vector del propio test de la web (openInApp.test.ts).
		"sytue-docs://open?url=https%3A%2F%2Fdocus.sytue.eu%2Fdocus%2Fedit%3Fid%3Da%26b%3Dc":   "sytue-docs:/docus/edit?id=a&b=c",
		"sytue-sheets://open?url=https%3A%2F%2Fcalcus.sytue.eu%2Fcalcus%2Fedit%3Fid%3Dh1":      "sytue-sheets:/calcus/edit?id=h1",
		"sytue-drive://open?url=https%3A%2F%2Fdrive.sytue.eu%2F%3Fpath%3D%252FEquipo":          "sytue-drive:/?path=%2FEquipo",
		"sytue-one://open?url=https%3A%2F%2Fcorporate.sytue.eu%2Fnotificaciones":               "sytue-one:/notificaciones",
		"sytue-mail://open?url=https%3A%2F%2Fmail.sytue.eu":                                    "sytue-mail:/",
		"sytue-docs://open?url=https%3A%2F%2FDOCUS.SYTUE.EU%3A443%2Fdocus%2Fedit%3Fid%3Dmayus": "sytue-docs:/docus/edit?id=mayus",
	} {
		nativo, ok := DesdeLaWeb(ApexProd, enlaceDe(t, crudo))
		if !ok || nativo.String() != quiero {
			t.Errorf("%s → %v, %v; quiero %s", crudo, nativo, ok, quiero)
		}
	}
	// Lo que puede escribir cualquiera y no es una página de la suite.
	for _, crudo := range []string{
		"sytue-docs://open?url=http%3A%2F%2Fdocs.sytue.eu%2Fedit%3Fid%3Da",      // sin TLS
		"sytue-docs://open?url=https%3A%2F%2Fsytue.eu.evil.com%2Fedit%3Fid%3Da", // otro dominio
		"sytue-docs://open?url=https%3A%2F%2Fevilsytue.eu%2Fedit%3Fid%3Da",      // sufijo sin punto
		"sytue-docs://open?url=javascript%3Aalert(1)",
		"sytue-docs://open",                            // sin página
		"sytue-docs:/edit?id=a",                        // ya es nativo: no hay que traducir
		"sytue-docs://auth/callback?code=x",            // la vuelta del login
		"https://open?url=https%3A%2F%2Fdocs.sytue.eu", // no es un esquema de la suite
	} {
		if nativo, ok := DesdeLaWeb(ApexProd, enlaceDe(t, crudo)); ok {
			t.Errorf("%s se ha traducido a %s", crudo, nativo)
		}
	}
	// Y la cadena entera, del editor web al documento: la página acaba en su id, y solo
	// para la app de su esquema.
	nativo, _ := DesdeLaWeb(ApexProd, enlaceDe(t, "sytue-docs://open?url=https%3A%2F%2Fdocus.sytue.eu%2Fdocus%2Fedit%3Fid%3Dd1"))
	if id := IDDeEdicion("docs", nativo); id != "d1" {
		t.Errorf("IDDeEdicion(docs, %s) = %q, quiero d1", nativo, id)
	}
	if id := IDDeEdicion("sheets", nativo); id != "" {
		t.Errorf("Calcus se ha quedado con un documento de Docus: %q", id)
	}
}

// TestLasAperturasEntrenConYSinOrganizacion: los avisos del Hub y «Abrir en la
// aplicación» de la web traen la organización detrás del prefijo (ADR 0056); lo que
// emite [AbrirFichero] va pelado, y las formas con prefijo sin org siguen circulando en
// enlaces guardados. Las cuatro abren lo mismo; algo que no sea la página de apertura no
// se abre aunque encabece con un slug.
func TestLasAperturasEntrenConYSinOrganizacion(t *testing.T) {
	entradas := []string{
		"sytue-docs:/open-from-drive?file=d1&volver=%2FDocs",            // pelada, de AbrirFichero
		"sytue-docs:/docus/open-from-drive?file=d1&volver=%2FDocs",      // con el prefijo de la web
		"sytue-docs:/docus/acme/open-from-drive?file=d1&volver=%2FDocs", // con prefijo y org
		"sytue-docs:/acme/open-from-drive?file=d1&volver=%2FDocs",       // con org y sin prefijo
	}
	for _, crudo := range entradas {
		u, err := url.Parse(crudo)
		if err != nil {
			t.Fatal(err)
		}
		ap, ok := LeerApertura("docs", u)
		if !ok {
			t.Errorf("%q no se lee como apertura", crudo)
			continue
		}
		if ap.Ref.ID != "d1" || ap.Volver != "/Docs" {
			t.Errorf("%q: se lee %+v", crudo, ap)
		}
	}
	// «acme/x» no es una organización, y una página que no es la de apertura tampoco da
	// apertura aunque el primer segmento parezca una org. Tampoco una ruta que empieza
	// por el prefijo pero a MITAD DE PALABRA: recortar «/docus» de
	// «/docusments/open-from-drive» haría pasar «ments» por organización.
	for _, crudo := range []string{
		"sytue-docs:/docus/acme%2Fx/open-from-drive?file=d1",
		"sytue-docs:/docus/acme/otra?file=d1",
		"sytue-docs:/acme?file=d1",
		"sytue-docs:/docusments/open-from-drive?file=d1",
	} {
		u, err := url.Parse(crudo)
		if err != nil {
			t.Fatal(err)
		}
		if ap, ok := LeerApertura("docs", u); ok {
			t.Errorf("%q se ha leído como apertura: %+v", crudo, ap)
		}
	}
	// El id de edición también llega con la org detrás del prefijo.
	for _, crudo := range []string{
		"sytue-sheets:/edit?id=h1",
		"sytue-sheets:/calcus/edit?id=h1",
		"sytue-sheets:/calcus/acme/edit?id=h1",
	} {
		u, err := url.Parse(crudo)
		if err != nil {
			t.Fatal(err)
		}
		if id := IDDeEdicion("sheets", u); id != "h1" {
			t.Errorf("IDDeEdicion(%q) = %q, quiero h1", crudo, id)
		}
	}
	// Pero una ruta que lleva el prefijo de la web a mitad de palabra no es de las
	// nuestras, aunque recortarla deje una org con forma de slug («ments»).
	if id := IDDeEdicion("docs", enlaceDe(t, "sytue-docs:/docusments/edit?id=h1")); id != "" {
		t.Errorf("IDDeEdicion(/docusments/edit) = %q, quiero vacío", id)
	}
}

// TestElRespaldoWebDeUnaAperturaEntrante: `sesion.AlAbrirFichero` recomputa la página
// web a partir de la ruta del esquema. Con org, la org del ENLACE manda —dice a qué
// organización apunta—; sin prefijo se compone con la de la sesión; y con prefijo la
// ruta va tal cual, porque ya es una página de la web (recomponerla duplicaría el
// prefijo, que es el 404 que no se parece a un enlace roto).
func TestElRespaldoWebDeUnaAperturaEntrante(t *testing.T) {
	for _, c := range []struct{ producto, crudo, quiero string }{
		{"docs", "/open-from-drive?file=d1", "https://docus.sytue.eu/docus/acme/open-from-drive?file=d1"},
		{"docs", "/docus/open-from-drive?file=d1", "https://docus.sytue.eu/docus/open-from-drive?file=d1"},
		{"docs", "/docus/acme/open-from-drive?file=d1", "https://docus.sytue.eu/docus/acme/open-from-drive?file=d1"},
		{"sheets", "/open-from-drive?file=d1", "https://calcus.sytue.eu/calcus/acme/open-from-drive?file=d1"},
		{"pdf", "/certipdf/acme/pdf?path=%2Fx", "https://certipdf.sytue.eu/certipdf/acme/pdf?path=%2Fx"},
		{"pdf", "/pdf?path=%2Fx", "https://certipdf.sytue.eu/certipdf/acme/pdf?path=%2Fx"},
		{"pdf", "/certipdf/pdf?path=%2Fx", "https://certipdf.sytue.eu/certipdf/pdf?path=%2Fx"},
		{"pdf", "/certipdf/otra-org/pdf?path=%2Fx", "https://certipdf.sytue.eu/certipdf/otra-org/pdf?path=%2Fx"},
	} {
		if got := WebDeApertura(ApexProd, c.producto, "acme", c.crudo); got != c.quiero {
			t.Errorf("WebDeApertura(%s, %q) = %q, quiero %q", c.producto, c.crudo, got, c.quiero)
		}
	}
}

func enlaceDe(t *testing.T, crudo string) *url.URL {
	t.Helper()
	u, err := url.Parse(crudo)
	if err != nil {
		t.Fatal(err)
	}
	return u
}

// TestLoQueNoEsUnaAperturaNoSeAbre: en Linux y en Windows la URL entra por la línea de
// órdenes, así que por aquí pasa de todo. Nada de esto debe mover la pantalla.
func TestLoQueNoEsUnaAperturaNoSeAbre(t *testing.T) {
	for _, crudo := range []string{
		"sytue-docs:/open-from-drive?volver=%2FDocs", // sin fichero que abrir
		"sytue-docs:/otra-cosa?file=abc",             // no es la ruta de apertura
		"sytue-docs://auth/callback?code=x",          // la vuelta del login, que es de la sesión
		"/Docs/acta.odt",                             // un fichero local
		"--modo=depuracion",                          // una opción
	} {
		u, err := url.Parse(crudo)
		if err != nil {
			continue
		}
		if ap, ok := LeerApertura("docs", u); ok {
			t.Errorf("%q se ha leído como apertura: %+v", crudo, ap)
		}
	}
	if _, ok := LeerApertura("docs", nil); ok {
		t.Error("un enlace nulo no abre nada")
	}
	// Un producto que no abre ficheros no acepta ninguno, aunque venga bien formado.
	u, _ := url.Parse("sytue-mail:/open-from-drive?file=abc")
	if _, ok := LeerApertura("mail", u); ok {
		t.Error("Mail no tiene ruta de apertura y no debe aceptar ninguna")
	}
}

// TestImportarDeDriveTraeYConvierte: el viaje entero de una apertura, contra un servidor
// que hace de Drive y de Docs a la vez. Lo que se comprueba es lo que haría daño si
// cambiara: que se pide el fichero POR ID con el token de quien abre, que la conversión
// recibe los bytes tal cual en el campo «file» con el nombre del fichero —el servicio
// saca de ahí el título—, y que NO se crea ningún documento.
func TestImportarDeDriveTraeYConvierte(t *testing.T) {
	var recibido struct{ nombre, datos, token string }
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method + " " + r.URL.Path {
		case "GET /api/v1/files/f-9":
			_, _ = io.WriteString(w, `{"id":"f-9","name":"Acta de marzo.docx","size_bytes":8}`)
		case "GET /api/v1/files/f-9/content":
			_, _ = io.WriteString(w, "PK\x03\x04docx")
		case "POST /api/v1/import":
			recibido.token = r.Header.Get("Authorization")
			f, h, err := r.FormFile("file")
			if err != nil {
				t.Errorf("la conversión no trae el campo file: %v", err)
				return
			}
			crudo, _ := io.ReadAll(f)
			recibido.nombre, recibido.datos = h.Filename, string(crudo)
			_, _ = io.WriteString(w, `{"kind":"document","title":"Acta de marzo","content":"<p>Hola</p>"}`)
		default:
			t.Errorf("petición inesperada: %s %s", r.Method, r.URL)
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer srv.Close()
	ses := &sesionFalsa{tokens: []string{"tok"}}
	imp, err := ImportarDeDrive(context.Background(), NewDrive(srv.URL, ses, nil), NewDocs(srv.URL, ses, nil), "f-9")
	if err != nil {
		t.Fatal(err)
	}
	if imp.Kind != DocText || imp.Title != "Acta de marzo" || imp.Content != "<p>Hola</p>" {
		t.Errorf("importado = %+v", imp)
	}
	if recibido.nombre != "Acta de marzo.docx" || recibido.datos != "PK\x03\x04docx" || recibido.token != "Bearer tok" {
		t.Errorf("la conversión recibió %+v", recibido)
	}

	// Sin id no se pregunta a nadie: la ruta sola no se resuelve desde aquí.
	if _, err := ImportarDeDrive(context.Background(), nil, nil, ""); err == nil {
		t.Error("sin id tendría que fallar antes de tocar la red")
	}
}

// TestImportarDeDriveNoBajaLoQueNoSeVaAConvertir: si Drive dice que pasa del tope del
// servicio, no se descarga. Bajar 300 MB para que el servicio conteste 413 es tiempo y
// datos móviles de quien pulsó.
func TestImportarDeDriveNoBajaLoQueNoSeVaAConvertir(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/files/f-9" {
			t.Errorf("ha pedido %s después de saber que no cabe", r.URL.Path)
		}
		_, _ = io.WriteString(w, `{"id":"f-9","name":"enorme.pptx","size_bytes":314572800}`)
	}))
	defer srv.Close()
	ses := &sesionFalsa{tokens: []string{"tok"}}
	if _, err := ImportarDeDrive(context.Background(), NewDrive(srv.URL, ses, nil), NewDocs(srv.URL, ses, nil), "f-9"); err == nil {
		t.Error("un fichero de 300 MB no se puede convertir y tendría que fallar")
	}
}

// TestLasExtensionesDeUnProductoSonLasDeLaTabla: son la misma tabla vista desde el otro
// lado, y es lo que el empaquetado ofrece en «Abrir con».
func TestLasExtensionesDeUnProductoSonLasDeLaTabla(t *testing.T) {
	if got := strings.Join(Extensiones("docs"), ","); got != "docx,dotx,odt,ott,pages" {
		t.Errorf("docs = %q", got)
	}
	if got := Extensiones("mail"); len(got) != 0 {
		t.Errorf("mail no abre ficheros y dice %q", got)
	}
}

// TestImportarConComentariosLosPideYElContenidoSoloLlevaLosGuardados: ImportarConComentarios
// pide `?comments=1` (Importar a secas no), y ContenidoConMarks deja solo los marks de los ids
// que el servicio dijo guardados.
func TestImportarConComentariosLosPideYElContenidoSoloLlevaLosGuardados(t *testing.T) {
	var queries []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		queries = append(queries, r.URL.RawQuery)
		_, _ = io.WriteString(w, `{"kind":"document","title":"A","content":"<p><mark data-comment-id=\"aa\">x</mark> <mark data-comment-id=\"bb\">y</mark></p>","comments":[{"id":"aa","text":"t","anchored":true}]}`)
	}))
	defer srv.Close()
	d := NewDocs(srv.URL, &sesionFalsa{tokens: []string{"tok"}}, nil)
	if _, err := d.Importar(context.Background(), "a.docx", []byte("PK")); err != nil {
		t.Fatal(err)
	}
	imp, err := d.ImportarConComentarios(context.Background(), "a.docx", []byte("PK"))
	if err != nil {
		t.Fatal(err)
	}
	if queries[0] != "" || queries[1] != "comments=1" || len(imp.Comments) != 1 {
		t.Errorf("queries %v, comentarios %+v", queries, imp.Comments)
	}
	if got := imp.ContenidoConMarks([]string{"aa"}); got != `<p><mark data-comment-id="aa">x</mark> y</p>` {
		t.Errorf("con aa guardado: %s", got)
	}
	if got := imp.ContenidoConMarks(nil); got != "<p>x y</p>" {
		t.Errorf("sin guardados: %s", got)
	}
}

// TestAlSistemaSoloLlegaLoQueSuPuertaAdmite (NAT-11): lo que se le da a `OpenURL` decide
// qué programa se lanza, así que la puerta mira el esquema aunque el servicio ya lo
// haya filtrado. Un `file:` abre (o ejecuta) un fichero del equipo.
func TestAlSistemaSoloLlegaLoQueSuPuertaAdmite(t *testing.T) {
	a := &abridorDoble{}
	for _, d := range []string{
		"file:///C:/Windows/System32/calc.exe", "javascript:alert(1)", "ms-settings:", "smb://equipo/compartido",
		"https:sin-host", "//sytue.eu/a", "", " ",
	} {
		if err := AbrirFuera(a, d); !errors.Is(err, ErrNoSeAbre) {
			t.Errorf("AbrirFuera(%q) = %v, quiero ErrNoSeAbre", d, err)
		}
	}
	for _, para := range []string{"ana@acme.eu?bcc=todos@acme.eu", "ana@acme.eu&body=x", "ana @acme.eu", "a%40b"} {
		if err := AbrirCorreo(a, para, "", ""); !errors.Is(err, ErrNoSeAbre) {
			t.Errorf("AbrirCorreo(%q) = %v, quiero ErrNoSeAbre", para, err)
		}
	}
	for _, n := range []string{"", "123;rm", "javascript:1", "+34\n600"} {
		if err := AbrirTelefono(a, n); !errors.Is(err, ErrNoSeAbre) {
			t.Errorf("AbrirTelefono(%q) = %v, quiero ErrNoSeAbre", n, err)
		}
	}
	if len(a.urls) != 0 {
		t.Fatalf("llegaron al sistema: %q", a.urls)
	}

	_ = AbrirFuera(a, " https://sytue.eu/precios ")
	_ = AbrirCorreo(a, "ana@acme.eu", "Hola & adiós=bcc", "")
	_ = AbrirTelefono(a, "+34 (600) 11-22-33")
	quiero := []string{"https://sytue.eu/precios", "mailto:ana@acme.eu?subject=Hola%20%26%20adi%C3%B3s%3Dbcc", "tel:+34 (600) 11-22-33"}
	if strings.Join(a.urls, "\n") != strings.Join(quiero, "\n") {
		t.Fatalf("abrió %q, quiero %q", a.urls, quiero)
	}
}

// TestElAltaDeDAVx5NoLlevaContrasena: el enlace de DAVx⁵ lleva la base y el usuario y
// nada más. VISTO EN ROJO poniendo `url.UserPassword(usuario, "x")`: la contraseña
// aparecía en la URL que se le da al sistema.
func TestElAltaDeDAVx5NoLlevaContrasena(t *testing.T) {
	u, err := EnlaceDAVx5("https://calendar.sytue.eu/dav/", "ada@sytue.eu", false)
	if err != nil || u.String() != "caldavs://ada%40sytue.eu@calendar.sytue.eu/dav/" {
		t.Fatalf("calendario = %v (%v)", u, err)
	}
	if _, conClave := u.User.Password(); conClave {
		t.Error("el enlace de DAVx⁵ lleva contraseña")
	}
	u, err = EnlaceDAVx5("https://contacts.sytue.eu:8443/dav/", "ada@sytue.eu", true)
	if err != nil || u.String() != "carddavs://ada%40sytue.eu@contacts.sytue.eu:8443/dav/" {
		t.Errorf("contactos = %v (%v)", u, err)
	}
	for _, mal := range []string{"http://calendar.sytue.eu/dav/", "https://ada:clave@calendar.sytue.eu/dav/", "file:///etc", ""} {
		if _, err := EnlaceDAVx5(mal, "ada@sytue.eu", false); err == nil {
			t.Errorf("%q se ha dado por base de DAVx⁵", mal)
		}
	}
	if _, err := EnlaceDAVx5("https://calendar.sytue.eu/dav/", "", false); err == nil {
		t.Error("sin usuario no hay alta que rellenar")
	}
}
