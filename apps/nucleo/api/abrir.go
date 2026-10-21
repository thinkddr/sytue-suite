// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package api

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/url"
	"slices"
	"strings"
)

// # Abrir un fichero en SU aplicación
//
// `docs/CONVENTIONS.md` §13.6: un documento se abre en la aplicación de su formato, no
// dentro de la que lo listaba. Un `.docx` de Drive abre Docus; una hoja, Calcus; una
// presentación, Diapos; un PDF, CertiPDF; una imagen, SyPhotos. Lo mismo para un adjunto
// de Sytue Mail o de Sytue Rooms.
//
// La consecuencia práctica es que **ninguna app necesita un visor de los formatos de las
// demás**: lo que se porta es la ACCIÓN de abrir. Y abrir es pasar una REFERENCIA —el
// identificador del fichero en Drive, o su ruta—, nunca una copia: quien guarda es la
// app dueña del formato, sobre el fichero que vive en Drive con su ACL y su historial.
//
// # Por qué vive aquí y no en el paquete del Hub
//
// El mecanismo nació en `apps/fyne/apps/hub` para las teselas del lanzador. Ahora lo
// necesitan Drive, Mail, Rooms y la barra transversal de las trece, así que sube a un
// sitio compartido (§13.2: lo que existe se importa o se amplía, no se copia). De los
// dos candidatos, `apps/nucleo` y `apps/fyne/ui`, es `apps/nucleo/api`:
//
//   - aquí ya vive la MITAD de esta tabla —`hostDe` en `suite.go`, que es el `appDomains`
//     de la web—, y lo que faltaba era su otra mitad, `appPaths`;
//   - no pinta nada: `apps/fyne/ui` son widgets, y esto no tiene ni una primitiva;
//   - no lleva toolkit, así que vale igual para escritorio y para móvil, y `Abridor` es
//     una interfaz de un método que `fyne.App` ya cumple sin saberlo.
//
// # La referencia es la web, leída
//
// La tabla de qué extensión abre qué producto es el espejo de
// `apps/frontend/apps/drive/src/lib/abrir-con.ts` (y del `THUMB_EXTS` de
// `FileBrowser.tsx`, que es lo que decide si Fotos sabe leer esa imagen). Las rutas son
// las de `urlDeApertura`, literales: el respaldo abre esa MISMA página en el navegador,
// así que una ruta inventada aquí es un 404 que se lee como «esa app está caída».
// `TestLaTablaDeAperturaEsLaDeLaWeb` las compara por igualdad en los dos sentidos.

// rutaDe es el `appPaths` de `packages/config/src/tokens.ts`: el prefijo bajo el que
// cada superficie se sirve DENTRO de su host.
//
// No es decoración. `astro.config` lo fija con `base` y el chart del frontend con
// `path`, así que la página de CertiPDF vive en `/certipdf/pdf` y no en `/pdf`. Traefik solo
// redirige la RAÍZ al prefijo, de modo que cualquier enlace PROFUNDO escrito sin él da
// 404 — y el fallo no se parece a un enlace roto: el host resuelve, el ingress enruta y
// lo que contesta es el 404 de Astro.
//
// Los que faltan —`id`, `hub`, `integrations`— faltan también en la web: no son apps
// Astro y cuelgan de la raíz. `notes` faltaba en los dos lados y no debía: Notepads se
// sirve bajo `/notes`, y sin él el `/p/{token}` de un cuaderno publicado era un 404.
var rutaDe = map[string]string{
	"one": "/one", "mail": "/mail", "drive": "/drive", "calendar": "/calendar",
	"contacts": "/contacts", "docs": "/docus", "sheets": "/calcus", "slides": "/diapos",
	"pdf": "/certipdf", "notes": "/notes", "photos": "/photos", "chat": "/rooms", "meet": "/rooms",
	"rooms": "/rooms", "cloud": "/cloud", "console": "/admin", "erp": "/erp", "cast": "/cast",
	"pages": "/web", "bases": "/bases",
}

// webDe es el host de la WEB de los productos cuya API vive en otro: la ofimática, una
// app por producto desde el 2026-10-12 (ADR 0076), mientras su servicio sigue siendo uno y
// publica la API en `docs.<dominio>`. Es la parte de `appDomains` que no es `hostDe`.
var webDe = map[string]string{"docs": "docus", "sheets": "calcus", "slides": "diapos", "pdf": "certipdf"}

// ProductoDeTipo es el `PRODUCTO_DE_TIPO` de la web: qué producto de la ofimática abre
// cada tipo de documento, ahora que cada uno tiene su app y su host (ADR 0076).
var ProductoDeTipo = map[DocKind]string{DocText: "docs", DocSheet: "sheets", DocSlides: "slides"}

// EnlaceDocumento es el `enlaceDocumento` de la web: el editor de un documento en SU
// producto (`calcus.<dominio>/calcus/<org>/edit?id=…`). Sin tipo conocido va a Docus, que
// manda el documento a la app que le toca.
func EnlaceDocumento(apex, org, id string, tipo DocKind) string {
	producto := ProductoDeTipo[tipo]
	if producto == "" {
		producto = "docs"
	}
	return AppURL(apex, producto, org, "/edit?id="+url.QueryEscape(id))
}

// webDeProducto es la raíz de la WEB de un producto: su host de `webDe` si lo tiene, y si
// no el de su servicio, que es el mismo.
func webDeProducto(apex, producto string) string {
	if host, ok := webDe[producto]; ok {
		return "https://" + host + "." + apex
	}
	return ServiceURL(apex, producto)
}

// AppURL es el `appUrl(key, path, org)` de la web: la dirección pública de una PÁGINA de
// otro producto, con su host Y su prefijo. La ruta se pasa tal y como se escribe dentro
// de esa app, sin el prefijo, que es justo lo que uno tiende a escribir.
//
// Con org (ADR 0056) la ORGANIZACIÓN va dentro del path, tras el prefijo:
// `https://calcus.sytue.eu/calcus/<org>/edit?id=…`. Es la forma canónica y aterriza sin el
// salto del middleware; la org es la de la SESIÓN de quien compone el enlace, que es
// quien sabe a dónde manda. Vacía, el enlace sale sin ella — forma transicional que el
// middleware de la app de destino completa con un 302—.
func AppURL(apex, producto, org, ruta string) string {
	base := webDeProducto(apex, producto) + rutaDe[producto]
	if org != "" {
		base += "/" + org
	}
	if limpia := strings.TrimLeft(ruta, "/"); limpia != "" {
		return base + "/" + limpia
	}
	return base
}

// Esquema es el esquema de URL con el que se invoca a la aplicación NATIVA de un
// producto: `sytue-<clave>`, la misma clave del catálogo y del token de color.
//
// Cada producto es su propia aplicación también en escritorio y en móvil (§13.6), así que
// saltar de una a otra no es cambiar de pantalla dentro de este binario: es pedírselo al
// SISTEMA —`.desktop` y xdg en Linux, `CFBundleURLTypes` en macOS e iOS, registro de
// protocolo en Windows, `intent-filter` en Android—. Cada app lo declara UNA vez en su
// `FyneApp.toml`, en `[Development] esquema`.
func Esquema(producto string) string { return "sytue-" + producto }

// Abridor es lo que hace falta para delegar en el sistema. Lo cumple `fyne.App`; en los
// tests es un doble que se limita a apuntar la URL.
type Abridor interface{ OpenURL(*url.URL) error }

// AbrirEn invoca a la aplicación de otro producto por su esquema y, si el sistema no
// tiene quién lo atienda —la app no está instalada—, cae a su dirección web.
//
// El respaldo no es un adorno: hasta que las trece estén empaquetadas e instaladas, la
// mayoría de los destinos no tienen aplicación nativa detrás, y un fichero que no hace
// nada al pulsarlo es peor que uno que abre el navegador.
//
// La org solo sale en el RESPALDO (la página web, canónica con ella): el esquema es un
// enlace ENTRE apps de la misma máquina y lo lee la propia sesión, que no necesita que
// nadie le diga su organización.
func AbrirEn(a Abridor, apex, producto, org, ruta string) error {
	// Pages solo tiene web: un protocolo sin aplicación puede devolver éxito sin abrir
	// nada, así que no sirve intentar el esquema y esperar al respaldo.
	if producto == "pages" {
		return abrirWeb(a, AppURL(apex, producto, org, ruta))
	}
	u, err := url.Parse(Esquema(producto) + ":" + rutaEsquema(ruta))
	if err != nil {
		return err
	}
	if err = a.OpenURL(u); err == nil {
		return nil
	}
	return abrirWeb(a, AppURL(apex, producto, org, ruta))
}

func abrirWeb(a Abridor, direccion string) error { return AbrirFuera(a, direccion) }

// # Abrir con el sistema: UN solo sitio
//
// `OpenURL` le da la dirección al sistema (`rundll32 url.dll`, `open`, `xdg-open`, un
// Intent), y el sistema decide qué hacer con ella por su ESQUEMA: con `file:` abre —o
// ejecuta— un fichero del equipo, y cada esquema registrado lanza su programa. Así que a
// `OpenURL` solo se llega desde este fichero, por una de estas puertas, y
// `TestSoloSeAbreConElSistemaPorSuPuerta` lo comprueba:
//
//   - [AbrirFuera]: una dirección que no ha escrito la suite. Solo http y https.
//   - [AbrirCorreo] y [AbrirTelefono]: `mailto:` y `tel:` que compone la app con sus piezas.
//   - [AbrirDAVx5]: el alta de DAVx⁵ (`caldavs:`/`carddavs:`), sin contraseña.
//   - [AbrirEn] y [AbrirURL]: el esquema `sytue-<producto>` de otra app de la suite.
//
// Un fichero del equipo no se abre por aquí: para eso está `ficheros.Abrir`.

// ErrNoSeAbre es una dirección que no se le da al sistema.
var ErrNoSeAbre = errors.New("api: esa dirección no se abre con el sistema")

// AbrirFuera abre en el navegador una dirección que viene de FUERA de la app: el enlace
// de un PDF, de una nota o de un mensaje, la videollamada de un evento, un resultado de
// búsqueda, lo que mande un servicio. Solo http y https, con su host. Que el servicio ya
// la haya filtrado no cuenta: esta es la última puerta antes del sistema.
func AbrirFuera(a Abridor, direccion string) error {
	u, err := url.Parse(strings.TrimSpace(direccion))
	if err != nil || (u.Scheme != "https" && u.Scheme != "http") || u.Host == "" {
		return ErrNoSeAbre
	}
	return a.OpenURL(u)
}

// AbrirReunionTeams entrega únicamente un enlace de reunión Teams validado al
// cliente oficial. No habilita esquemas arbitrarios ni una versión web sin E2EE.
func AbrirReunionTeams(a Abridor, direccion string) error {
	if !validTeamsMeetingLink(direccion) {
		return ErrNoSeAbre
	}
	u, err := url.Parse(direccion)
	if err != nil {
		return ErrNoSeAbre
	}
	u.Scheme = "msteams"
	return a.OpenURL(u)
}

// AbrirCorreo abre el programa de correo del sistema con un mensaje nuevo. El `mailto:`
// lo compone la app: `para` es UNA dirección (sin `?`, `&` ni espacios, que es por donde
// un valor ajeno añadiría campos) y el asunto y el cuerpo van escapados.
func AbrirCorreo(a Abridor, para, asunto, cuerpo string) error {
	if strings.ContainsAny(para, "?&#%\\") || strings.ContainsFunc(para, func(r rune) bool { return r <= ' ' || r == 0x7f }) {
		return ErrNoSeAbre
	}
	destino, sep := "mailto:"+para, "?"
	for _, campo := range [][2]string{{"subject", asunto}, {"body", cuerpo}} {
		if campo[1] != "" {
			// QueryEscape y no PathEscape: este deja `&` y `=` sin escapar, y por ahí un asunto
			// añadiría campos (`&bcc=`). Los espacios, `%20` (RFC 6068), no `+`.
			destino += sep + campo[0] + "=" + strings.ReplaceAll(url.QueryEscape(campo[1]), "+", "%20")
			sep = "&"
		}
	}
	u, err := url.Parse(destino)
	if err != nil {
		return ErrNoSeAbre
	}
	return a.OpenURL(u)
}

// AbrirTelefono abre el marcador del sistema con ese número. Solo lo que lleva un número
// escrito por una persona: cifras, espacios y `+ - ( ) . * #`.
func AbrirTelefono(a Abridor, numero string) error {
	numero = strings.TrimSpace(numero)
	if numero == "" || strings.ContainsFunc(numero, func(r rune) bool {
		return (r < '0' || r > '9') && !strings.ContainsRune(" +-().*#", r)
	}) {
		return ErrNoSeAbre
	}
	return a.OpenURL(&url.URL{Scheme: "tel", Opaque: numero})
}

// EnlaceDAVx5 es la dirección con la que DAVx⁵ abre su alta ya rellena: la `base`
// https de CalDAV (o de CardDAV, con `contactos`) con el esquema `caldavs`/`carddavs`,
// que su `LoginActivity` traduce de vuelta a https, y el `usuario` en la parte de
// usuario. La contraseña NO va nunca: una URL acaba en historiales y registros del
// sistema, y la contraseña de aplicación viaja por el portapapeles.
func EnlaceDAVx5(base, usuario string, contactos bool) (*url.URL, error) {
	u, err := url.Parse(strings.TrimSpace(base))
	if err != nil || u.Scheme != "https" || u.Host == "" || u.User != nil || usuario == "" {
		return nil, ErrNoSeAbre
	}
	v := *u
	v.Scheme = "caldavs"
	if contactos {
		v.Scheme = "carddavs"
	}
	v.User = url.User(usuario)
	return &v, nil
}

// AbrirDAVx5 abre el alta de DAVx⁵ con [EnlaceDAVx5]. Sin DAVx⁵ instalado el sistema no
// tiene quién lo atienda: la pantalla ofrece al lado su ficha para instalarlo.
func AbrirDAVx5(a Abridor, base, usuario string, contactos bool) error {
	u, err := EnlaceDAVx5(base, usuario, contactos)
	if err != nil {
		return err
	}
	return a.OpenURL(u)
}

// AbrirURL es AbrirEn cuando ya se tiene la DIRECCIÓN del destino: un resultado de la
// búsqueda federada o un aviso del Hub traen la URL entera, y esa es el respaldo.
//
// No se puede usar `AbrirEn` con la ruta de esa URL: `AppURL` le antepondría otra vez
// el prefijo del producto —`/certipdf/pdf` y compañía—, y el respaldo sería un 404. Aquí
// el esquema se arma con la ruta que trae la URL y el respaldo es la URL tal cual.
func AbrirURL(a Abridor, producto, direccion string) error {
	destino, err := url.Parse(direccion)
	if err != nil {
		return err
	}
	if producto == "pages" {
		return AbrirFuera(a, direccion)
	}
	u, err := enlaceNativo(Esquema(producto), destino)
	if err != nil {
		return err
	}
	if err = a.OpenURL(u); err == nil {
		return nil
	}
	return AbrirFuera(a, direccion)
}

// enlaceNativo es la página `destino` de la web vista por el esquema de la app nativa:
// `https://calcus.sytue.eu/calcus/edit?id=x` → `sytue-sheets:/calcus/edit?id=x`. La ruta va
// CON el prefijo del producto, tal cual la trae la URL; los receptores lo quitan.
func enlaceNativo(esquema string, destino *url.URL) (*url.URL, error) {
	ruta := destino.EscapedPath()
	if destino.RawQuery != "" {
		ruta += "?" + destino.RawQuery
	}
	return url.Parse(esquema + ":" + rutaEsquema(ruta))
}

// DesdeLaWeb traduce el enlace de «Abrir en la aplicación» de la web
// (`packages/shell/src/openInApp.ts`). Ese enlace, `sytue-<p>://open?url=<https>`, lleva
// la dirección de la PÁGINA que se estaba mirando; aquí se convierte en
// `sytue-<p>:<ruta>?<consulta>` de esa página, que es lo que ya emite [AbrirURL] desde la
// búsqueda y los avisos, y lo que los receptores de cada app ya leen. Así ninguna app
// escribe un segundo lector.
//
// Devuelve false si no es ese enlace o si la página no es de la suite de `apex`. El
// enlace lo puede escribir cualquier web o cualquier correo y el sistema no mira quién:
// solo https y solo un host de la suite, como `sameSuite` en la web. De la página solo
// se queda la ruta y la consulta —el host se tira—, así que lo peor que puede traer es
// un destino que la app no conoce, y eso ya es la página «no encontrada».
func DesdeLaWeb(apex string, u *url.URL) (*url.URL, bool) {
	if u == nil || u.Host != "open" || !strings.HasPrefix(u.Scheme, "sytue-") {
		return nil, false
	}
	pagina, err := url.Parse(u.Query().Get("url"))
	if err != nil || pagina.Scheme != "https" {
		return nil, false
	}
	if h := strings.ToLower(pagina.Hostname()); h != apex && !strings.HasSuffix(h, "."+apex) {
		return nil, false
	}
	nativo, err := enlaceNativo(u.Scheme, pagina)
	return nativo, err == nil
}

// rutaEsquema convierte la ruta de la web en la parte de camino del esquema. Un esquema
// sin autoridad lleva UNA barra (`sytue-mail:/compose`), y sin ruta se queda en la barra
// sola, que es lo que abre la pantalla de inicio de la otra aplicación.
func rutaEsquema(ruta string) string {
	if ruta == "" || ruta[0] != '/' {
		return "/" + ruta
	}
	return ruta
}

// abre es QUÉ PRODUCTO ABRE CADA EXTENSIÓN, el espejo de `abrir-con.ts`.
//
// Las imágenes son las CUATRO que el servicio de fotos sabe leer (`THUMB_EXTS`, las
// mismas que tienen miniatura): un SVG o un AVIF darían un error tras salir de Drive, que
// es peor que quedarse donde se estaba. Lo que no está aquí —un `.csv`, un `.zip`, un
// `.go`— no tiene app dueña y se queda con quien lo listó.
//
// Los de iWork (`.pages`, `.numbers`, `.key`) se abren CONVERTIDOS por el servicio, como en
// la web: nunca se guardan en formato de Apple.
//
// El editor ofimático externo (WOPI) NO está: lo despliega quien opera la suite, es una
// página web y no una aplicación de la suite, así que en nativo no hay esquema al que
// llamar.
var abre = map[string]string{
	"pdf":     "pdf",
	"jpg":     "photos",
	"jpeg":    "photos",
	"png":     "photos",
	"gif":     "photos",
	"odt":     "docs",
	"docx":    "docs",
	"ott":     "docs",
	"dotx":    "docs",
	"ods":     "sheets",
	"xlsx":    "sheets",
	"ots":     "sheets",
	"xltx":    "sheets",
	"odp":     "slides",
	"pptx":    "slides",
	"otp":     "slides",
	"potx":    "slides",
	"pages":   "docs",
	"numbers": "sheets",
	"key":     "slides",
}

// AppQueAbre es la clave del producto que abre ese fichero, por su nombre. El segundo
// valor dice si alguno lo reclama.
func AppQueAbre(nombre string) (string, bool) {
	i := strings.LastIndexByte(nombre, '.')
	if i < 0 || i == len(nombre)-1 {
		return "", false
	}
	p, ok := abre[strings.ToLower(nombre[i+1:])]
	return p, ok
}

// Extensiones son las extensiones que abre ese producto, en orden alfabético. Es la
// misma tabla vista desde el otro lado: la que el empaquetado declara al sistema para que
// «Abrir con» ofrezca esta app (`FyneApp.toml`, `[Development] extensiones`), y la que
// `TestCadaAppDeclaraLasExtensionesDeSuProducto` compara con lo declarado.
func Extensiones(producto string) []string {
	var out []string
	for ext, p := range abre {
		if p == producto {
			out = append(out, ext)
		}
	}
	slices.Sort(out)
	return out
}

// Referencia es lo que se le pasa a la otra aplicación: DÓNDE está el fichero, no el
// fichero. `ID` es el identificador en Drive y `Ruta` su camino completo.
//
// Por qué el id y no la ruta donde se puede: el id se resuelve con el token de quien
// pulsa, así que si esa persona no puede ver el fichero el servicio contesta 404 y no hay
// nada que abrir; una ruta es un dato elegido por quien llama y habría que volver a
// comprobar de quién es.
type Referencia struct {
	ID   string
	Ruta string
}

// RutaDeApertura es la página de ese producto que abre la referencia, con la vuelta a la
// CARPETA DEL FICHERO puesta. Es el `urlDeApertura` de la web, ruta a ruta.
func RutaDeApertura(producto string, ref Referencia) string {
	volver := "volver=" + url.QueryEscape(carpetaDe(ref.Ruta))
	switch producto {
	case "pdf":
		// CertiPDF abre por RUTA: es lo único que su página acepta (`pdf.astro`).
		return "/pdf?path=" + url.QueryEscape(ref.Ruta) + "&" + volver
	case "photos":
		return "/abrir?file=" + url.QueryEscape(ref.ID) + "&" + volver
	case "docs", "sheets", "slides":
		fichero := "path=" + url.QueryEscape(ref.Ruta)
		if ref.ID != "" {
			fichero = "file=" + url.QueryEscape(ref.ID)
		}
		return "/open-from-drive?" + fichero + "&" + volver
	}
	return ""
}

// carpetaDe es la carpeta que contiene a esa ruta, el `carpetaDe` de `arrastrar.ts`. Se
// vuelve a la carpeta DEL FICHERO y no a la lista que se estuviera mirando: desde una
// búsqueda, lo que se mira no es una carpeta.
func carpetaDe(ruta string) string {
	i := strings.LastIndexByte(ruta, '/')
	if i <= 0 {
		return ""
	}
	return ruta[:i]
}

// AbrirFichero abre el fichero en la aplicación de su formato. Devuelve false si ningún
// producto de la suite reclama ese formato, o si el sistema no pudo abrir ni el esquema
// ni la web: en los dos casos quien llama sigue haciendo lo que hacía —previsualizar,
// descargar— en vez de quedarse callado. La org es la de la sesión de quien abre (para
// el respaldo web, ver [AbrirEn]).
func AbrirFichero(a Abridor, apex, org, nombre string, ref Referencia) bool {
	producto, ok := AppQueAbre(nombre)
	if !ok {
		return false
	}
	return AbrirEn(a, apex, producto, org, RutaDeApertura(producto, ref)) == nil
}

// WebDeApertura es la página web de una ruta de esquema ENTRANTE — el respaldo de
// `sesion.AlAbrirFichero` cuando la carga falla—. La ruta llega en las formas que
// circulan: la pelada que emite [AbrirFichero] (`/open-from-drive?…`), la de la web con
// su prefijo (`/docus/open-from-drive?…`) y las dos con la organización detrás del
// prefijo (`/docus/<org>/open-from-drive?…`, la de los avisos y de «Abrir en la
// aplicación»).
//
//   - Sin prefijo se compone como la web: prefijo, org de la SESIÓN —de quien abre, que
//     es quien falló a cargar— y ruta.
//   - Con prefijo la ruta YA es una página de la web —con org o sin ella, canónica o
//     transicional— y va TAL CUAL: recomponerla añadiría el prefijo por segunda vez y
//     el 404 no se parece a un enlace roto.
func WebDeApertura(apex, producto, org, rutaEsquema string) string {
	if p := rutaDe[producto]; p == "" || (!strings.HasPrefix(rutaEsquema, p+"/") && rutaEsquema != p) {
		return AppURL(apex, producto, org, rutaEsquema)
	}
	return webDeProducto(apex, producto) + rutaEsquema
}

// # La otra punta: LEER el enlace que llega

// Apertura es lo que trae un enlace entrante de apertura, ya desarmado: la referencia
// del fichero y a dónde vuelve quien cierre.
//
// Es la vuelta de [RutaDeApertura]. Existe porque la app que RECIBE el enlace no puede
// volver a escribir la tabla de parámetros: son los mismos nombres —`file`, `path`,
// `volver`— y una segunda copia se desincroniza en cuanto uno cambie (CONVENTIONS.md
// §13.2). `TestLoQueSeEmiteEsLoQueSeLee` los ata por ida y vuelta.
type Apertura struct {
	// Ref es DÓNDE está el fichero. CertiPDF recibe Ruta; los demás, ID.
	Ref Referencia
	// Volver es la carpeta de Drive a la que vuelve quien cierre el documento. Puede
	// venir vacía: se llega a un fichero también desde una búsqueda.
	Volver string
	// Local es la URI de un fichero de ESTA máquina —`file://…` en escritorio,
	// `content://…` en Android— cuando se llega por «Abrir con», por la línea de órdenes
	// o soltándolo en la ventana. Entonces Ref va vacía: no está en Drive. La lee quien
	// tiene el toolkit (`apps/fyne/sesion`), porque `content://` solo lo sabe abrir él.
	Local string
}

// LeerApertura desarma el enlace que el sistema le ha entregado a la app de `producto`.
// Devuelve false —y la app sigue como estaba— si no es un enlace de apertura suyo:
//
//   - el esquema no es el de este producto: otra app registró el esquema, o llegó un
//     argumento cualquiera de la línea de órdenes (en Linux y Windows la URL entra por
//     ahí, así que aquí llega de todo);
//   - la ruta no es la que este producto abre, con o sin el prefijo de la web y con o
//     sin la organización detrás de él (ADR 0056), que es como llegan los avisos y
//     «Abrir en la aplicación»;
//   - no trae ni identificador ni ruta, así que no hay nada que abrir.
//
// Lo que NO hace es comprobar permisos: eso lo hace el servicio al resolver el id con el
// token de quien abre, que es justo por lo que se pasa el id y no el contenido.
func LeerApertura(producto string, u *url.URL) (Apertura, bool) {
	plantilla := RutaDeApertura(producto, Referencia{})
	corte := strings.IndexByte(plantilla, '?')
	if u == nil || corte < 0 || u.Scheme != Esquema(producto) ||
		!esPaginaDe(u.Path, producto, plantilla[:corte]) {
		return Apertura{}, false
	}
	q := u.Query()
	ap := Apertura{
		Ref:    Referencia{ID: q.Get("file"), Ruta: q.Get("path")},
		Volver: q.Get("volver"),
	}
	if ap.Ref.ID == "" && ap.Ref.Ruta == "" {
		return Apertura{}, false
	}
	return ap, true
}

// esPaginaDe dice si la ruta de un esquema llega a la página `pagina` del producto, en
// las cuatro formas que circulan: la pelada que emite [AbrirFichero]
// (`sytue-docs:/open-from-drive`), la de la web con su prefijo
// (`sytue-docs:/docs/open-from-drive`) y las dos con la organización detrás del prefijo
// (`sytue-docs:/docs/<org>/open-from-drive`, la de los avisos del Hub y de «Abrir en la
// aplicación» de la web).
//
// La página va AL FINAL, anclada: así no hay confusión posible entre una org y una ruta
// cuyo nombre sea también un slug válido («open-from-drive» lo es), que es la misma
// ambigüedad que la web resuelve con sus `sinOrg`.
func esPaginaDe(ruta, producto, pagina string) bool {
	if ruta == pagina || ruta == rutaDe[producto]+pagina {
		return true
	}
	prefijo := rutaDe[producto]
	// El prefijo solo se recorta como segmento ENTERO —`/docs` o `/docs/…`—, no
	// a mitad de palabra: de `/docsuments/edit` un TrimPrefix a saco haría
	// `uments/edit` y `uments` pasaría por organización. Misma condición de
	// borde que [WebDeApertura]; lo que no lleva el prefijo se lee pelado, que
	// es la cuarta forma (org sin prefijo).
	if prefijo != "" && strings.HasPrefix(ruta, prefijo) &&
		ruta != prefijo && !strings.HasPrefix(ruta, prefijo+"/") {
		return false
	}
	resto, conOrg := cortarOrg(strings.TrimPrefix(ruta, prefijo))
	return conOrg && resto == pagina
}

// cortarOrg quita el segmento de organización que encabeza una ruta de esquema
// (`/<org>/edit` → `/edit`) y dice si lo había, que ha de ser un slug válido de
// organización.
func cortarOrg(ruta string) (resto string, ok bool) {
	primero, demas, hay := strings.Cut(strings.TrimPrefix(ruta, "/"), "/")
	return "/" + demas, hay && esSlugDeOrg(primero)
}

// IDDeEdicion es el identificador del documento de Sytue Docs de un enlace de edición a
// la app de `producto` (docs, sheets o slides), o "" si no lo es. Acepta la ruta del
// esquema (`sytue-sheets:/edit?id=…`), la de la web con su prefijo
// (`sytue-sheets:/calcus/edit?id=…`) y las dos con la organización detrás del prefijo
// (`sytue-sheets:/docs/<org>/edit?id=…`), que es lo que arman [AbrirURL] y [DesdeLaWeb]
// con la dirección de la página.
//
// Es un id del SERVICIO de Docs, no de Drive: el documento se lee y se guarda allí,
// editable, y no es la conversión en solo lectura de [LeerApertura].
func IDDeEdicion(producto string, u *url.URL) string {
	if u == nil || u.Scheme != Esquema(producto) || !esPaginaDe(u.Path, producto, "/edit") {
		return ""
	}
	return u.Query().Get("id")
}

// ImportarDeDrive es lo que hace `/open-from-drive` en la web para enseñar un fichero:
// lo trae de Drive por su id y lo convierte con el servicio de Docs. NO crea ningún
// documento: lo crea quien abre (`editor.AbrirArchivo.AbrirApertura`), anclado con
// `source_ref` como la web, para que abrir cien veces el mismo fichero no deje cien copias.
//
// Solo por id: la ruta la elige quien pulsa y habría que volver a comprobar de quién es,
// mientras que el id se resuelve con el token de quien abre (ver Referencia).
func ImportarDeDrive(ctx context.Context, drive *Drive, docs *Docs, id string) (Importado, error) {
	return importarDeDrive(ctx, drive, docs, id, false)
}

// ImportarDeDriveConComentarios es ImportarDeDrive para quien va a crear el documento (ver
// [Docs.ImportarConComentarios]).
func ImportarDeDriveConComentarios(ctx context.Context, drive *Drive, docs *Docs, id string) (Importado, error) {
	return importarDeDrive(ctx, drive, docs, id, true)
}

func importarDeDrive(ctx context.Context, drive *Drive, docs *Docs, id string, comentarios bool) (Importado, error) {
	if id == "" {
		return Importado{}, errors.New("api: la apertura no trae el id del fichero")
	}
	meta, err := drive.File(ctx, id)
	if err != nil {
		return Importado{}, err
	}
	demasiado := fmt.Errorf("api: %q pasa de los %d MiB que convierte el servicio", meta.Name, MaxImportar>>20)
	if meta.SizeBytes > MaxImportar {
		return Importado{}, demasiado
	}
	cuerpo, err := drive.Download(ctx, id)
	if err != nil {
		return Importado{}, err
	}
	defer func() { _ = cuerpo.Close() }()
	datos, err := io.ReadAll(io.LimitReader(cuerpo, MaxImportar+1))
	if err != nil {
		return Importado{}, err
	}
	if len(datos) > MaxImportar {
		return Importado{}, demasiado
	}
	return docs.importar(ctx, meta.Name, datos, comentarios)
}
