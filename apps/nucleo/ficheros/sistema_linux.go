// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

//go:build linux && !android

package ficheros

import (
	"context"
	"errors"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/godbus/dbus/v5"
)

// Carpeta es la de descargas CONFIGURADA en el escritorio, no «~/Downloads» a pelo: en un
// sistema en español es «~/Descargas», y escribir ahí es lo que hace que la persona
// encuentre lo que se acaba de bajar. Antes se componía a mano y en este mismo equipo
// —Ubuntu en español— creaba un «~/Downloads» vacío al lado del bueno.
//
// El orden es el de la especificación de xdg-user-dirs: la variable de entorno manda, y
// si no está, se lee la configuración. Sin ninguna de las dos queda «~/Downloads», que es
// lo que el propio xdg-user-dirs usa por defecto.
func Carpeta() (string, error) { return carpetaXDG("XDG_DOWNLOAD_DIR", "Downloads") }

// CarpetaImagenes es la de imágenes CONFIGURADA en el escritorio (`XDG_PICTURES_DIR`),
// con la misma regla que [Carpeta]: en un sistema en español es «~/Imágenes». Es la que
// leen GNOME Fotos, Shotwell o digiKam, y por eso es «la galería» del sistema.
func CarpetaImagenes() (string, error) { return carpetaXDG("XDG_PICTURES_DIR", "Pictures") }

// carpetaXDG resuelve una carpeta de xdg-user-dirs: la variable de entorno, la
// configuración, y si no hay ninguna, `~/<porDefecto>`.
func carpetaXDG(clave, porDefecto string) (string, error) {
	casa, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	if d := resolverXDG(os.Getenv(clave), casa); d != "" {
		return d, nil
	}
	config := os.Getenv("XDG_CONFIG_HOME")
	if config == "" {
		config = filepath.Join(casa, ".config")
	}
	// #nosec G703 -- la ruta es la del propio XDG: la configuración de carpetas DE quien
	// corre el proceso, en el sitio que la especificación dice y que su propio entorno
	// nombra. No viene de la red ni de un documento; que apunte a otro sitio solo puede
	// hacerlo quien ya podría leer ese fichero de todas formas.
	if datos, err := os.ReadFile(filepath.Join(config, "user-dirs.dirs")); err == nil { //nolint:gosec // G703: ver arriba
		if d := xdgDe(string(datos), clave, casa); d != "" {
			return d, nil
		}
	}
	return filepath.Join(casa, porDefecto), nil
}

// descargasDe saca XDG_DOWNLOAD_DIR de un user-dirs.dirs. El fichero es un guion de shell
// que xdg-user-dirs escribe siempre igual —`XDG_DOWNLOAD_DIR="$HOME/Descargas"`—, así que
// se lee como lo que es y no se ejecuta nada: entrecomillado, con `$HOME` al principio o
// con una ruta absoluta, y comentarios con `#`.
func descargasDe(contenido, casa string) string { return xdgDe(contenido, "XDG_DOWNLOAD_DIR", casa) }

// xdgDe es [descargasDe] para cualquier clave de user-dirs.dirs.
func xdgDe(contenido, clave, casa string) string {
	for linea := range strings.Lines(contenido) {
		linea = strings.TrimSpace(linea)
		if strings.HasPrefix(linea, "#") {
			continue
		}
		valor, ok := strings.CutPrefix(linea, clave+"=")
		if !ok {
			continue
		}
		if d := resolverXDG(strings.Trim(strings.TrimSpace(valor), `"'`), casa); d != "" {
			return d
		}
	}
	return ""
}

// resolverXDG convierte un valor de xdg-user-dirs en una ruta absoluta, o en "" si no lo
// es. Devolver "" para `$HOME` a secas es deliberado: la especificación dice que la
// carpeta de descargas NO puede ser la casa —borrarla se llevaría todo por delante—, y un
// valor así significa «desactivada», no «descarga en ~».
func resolverXDG(valor, casa string) string {
	valor = strings.TrimSpace(valor)
	switch {
	case valor == "":
		return ""
	case valor == "$HOME" || valor == "$HOME/" || valor == casa:
		return ""
	case strings.HasPrefix(valor, "$HOME/"):
		return filepath.Join(casa, strings.TrimPrefix(valor, "$HOME/"))
	case filepath.IsAbs(valor):
		return filepath.Clean(valor)
	}
	return ""
}

// Revelar enseña el fichero en el gestor de ficheros, ya marcado. Lo pide por la interfaz
// org.freedesktop.FileManager1, que es lo que implementan Nautilus, Dolphin, Nemo y Thunar
// y lo que usa cualquier otra aplicación para esto.
//
// Fallback en dos escalones, porque esa interfaz no la hay en todas partes: si no
// contesta, se abre la CARPETA con xdg-open —se ve dónde está el fichero, aunque no salga
// marcado—, y si tampoco hay xdg-open, ErrNoDisponible y quien llama no ofrece el botón.
func Revelar(ruta string) error {
	abs, err := local(ruta)
	if err != nil {
		return err
	}
	if err := porFileManager1(abs); err == nil {
		return nil
	}
	if _, err := exec.LookPath("xdg-open"); err != nil {
		return ErrNoDisponible
	}
	return lanzar("xdg-open", filepath.Dir(abs))
}

func porFileManager1(abs string) error {
	conn, err := dbus.SessionBus()
	if err != nil {
		return err
	}
	// No se cierra: dbus.SessionBus() es la conexión COMPARTIDA del proceso (la del
	// llavero de la sesión y la de la insignia, la bandeja y los avisos de
	// `sistema/escritorio`, entre otras). Cerrarla aquí las dejaría a todas sin bus.
	obj := conn.Object("org.freedesktop.FileManager1", "/org/freedesktop/FileManager1")
	return obj.Call("org.freedesktop.FileManager1.ShowItems", 0, []string{"file://" + abs}, "").Err
}

// abrirConContexto entrega el fichero a la aplicación que el sistema tenga asociada a su tipo: es el
// «Abrir con…» del escritorio, y quien elige es la configuración de la persona, no Sytue.
func abrirConContexto(ctx context.Context, ruta string) error {
	abs, err := local(ruta)
	if err != nil {
		return err
	}
	if _, err := exec.LookPath("xdg-open"); err != nil {
		return ErrNoDisponible
	}
	return lanzarConContexto(ctx, "xdg-open", abs)
}

// Imprimir enseña el diálogo de impresión DEL SISTEMA con el fichero ya dentro, por el
// portal de escritorio (org.freedesktop.portal.Print): el mismo diálogo de GTK o de KDE
// que usa cualquier aplicación, con las impresoras de CUPS que la persona ya tiene y
// dentro del Flatpak si lo hay. El portal imprime PDF, que es lo que la suite genera.
//
// Si la persona cancela no es un error: lo ha decidido ella, y no se abre nada más.
//
// Si el portal no está —un escritorio sin xdg-desktop-portal, o sin bus de sesión—, el
// camino de repuesto es abrirlo en el visor asociado, donde Ctrl+P es el mismo diálogo
// con una pulsación más. Mandarlo con `lp` a la impresora por defecto sin preguntar
// sería peor: imprimir es irreversible y gasta papel de alguien.
//
// BLOQUEA mientras el diálogo está abierto: quien llama lo hace fuera del hilo de la
// interfaz.
func Imprimir(ruta string) error {
	abs, err := local(ruta)
	if err != nil {
		return err
	}
	err = porPortal(abs)
	if errors.Is(err, errSinPortal) {
		return Abrir(abs)
	}
	return err
}

// errSinPortal es que en este escritorio no hay a quién pedirle el diálogo.
var errSinPortal = errors.New("ficheros: no hay portal de impresión")

// plazoDialogo es cuánto se espera a que la persona conteste al diálogo.
//
// ponytail: un portal que muere con el diálogo abierto no avisa por la señal Response, y
// sin plazo la rutina que llama se quedaría esperando para siempre. Quince minutos es de
// sobra para elegir impresora; si alguien necesita más, se escucha NameOwnerChanged.
const plazoDialogo = 15 * time.Minute

// porPortal es PreparePrint (el diálogo) y Print (el trabajo), cada uno con su respuesta
// por la señal org.freedesktop.portal.Request.Response. Va por una conexión PROPIA y no
// por la compartida del proceso: se suscribe a señales y así no le llegan a nadie más.
func porPortal(abs string) error {
	conn, err := dbus.ConnectSessionBus()
	if err != nil {
		return errSinPortal
	}
	defer func() { _ = conn.Close() }()
	if !conn.SupportsUnixFDs() {
		return errSinPortal
	}
	if err := conn.AddMatchSignal(
		dbus.WithMatchInterface("org.freedesktop.portal.Request"),
		dbus.WithMatchMember("Response"),
	); err != nil {
		return errSinPortal
	}
	senales := make(chan *dbus.Signal, 8)
	conn.Signal(senales)

	portal := conn.Object("org.freedesktop.portal.Desktop", "/org/freedesktop/portal/desktop")
	titulo := filepath.Base(abs)

	var pedido dbus.ObjectPath
	err = portal.Call("org.freedesktop.portal.Print.PreparePrint", 0, "", titulo,
		map[string]dbus.Variant{}, map[string]dbus.Variant{},
		map[string]dbus.Variant{"modal": dbus.MakeVariant(true)}).Store(&pedido)
	if err != nil {
		return sinPortalSi(err)
	}
	codigo, resultado, err := respuesta(senales, pedido, plazoDialogo)
	if err != nil || codigo != 0 {
		return err // 1 es «cancelado» y no es un error; 2 lo trae ya como error
	}
	ficha, _ := resultado["token"].Value().(uint32)

	f, err := os.Open(abs) //nolint:gosec // G304: la ruta la ha validado local()
	if err != nil {
		return err
	}
	defer func() { _ = f.Close() }()
	fd := dbus.UnixFD(f.Fd()) //nolint:gosec // G115: un descriptor de fichero cabe en int32
	err = portal.Call("org.freedesktop.portal.Print.Print", 0, "", titulo,
		fd, map[string]dbus.Variant{"token": dbus.MakeVariant(ficha)}).Store(&pedido)
	if err != nil {
		return sinPortalSi(err)
	}
	_, _, err = respuesta(senales, pedido, plazoDialogo)
	return err
}

// respuesta espera la señal Response del pedido `ruta`. El código es el del portal: 0
// hecho, 1 cancelado por la persona, 2 cualquier otra cosa (y entonces vuelve con error).
//
// `plazo` 0 es esperar sin límite. Quien lo pide así se suscribe también a
// NameOwnerChanged del portal: si el portal muere con el diálogo abierto no llega
// Response, pero sí que el nombre se ha quedado sin dueño, y eso corta la espera.
func respuesta(senales <-chan *dbus.Signal, ruta dbus.ObjectPath, plazo time.Duration) (uint32, map[string]dbus.Variant, error) {
	var vence <-chan time.Time
	if plazo > 0 {
		vence = time.After(plazo)
	}
	for {
		select {
		case s := <-senales:
			if portalCaido(s) {
				return 0, nil, errors.New("ficheros: el portal se cerró con el diálogo abierto")
			}
			if s == nil || s.Path != ruta || len(s.Body) < 2 {
				continue
			}
			codigo, _ := s.Body[0].(uint32)
			resultado, _ := s.Body[1].(map[string]dbus.Variant)
			if codigo > 1 {
				return codigo, nil, errors.New("ficheros: el diálogo del portal no pudo terminar")
			}
			return codigo, resultado, nil
		case <-vence:
			return 0, nil, errors.New("ficheros: el diálogo del portal no contestó")
		}
	}
}

// nombrePortal es el nombre del bus de xdg-desktop-portal.
const nombrePortal = "org.freedesktop.portal.Desktop"

// portalCaido dice si la señal es el NameOwnerChanged de un portal que se ha ido.
func portalCaido(s *dbus.Signal) bool {
	if s == nil || s.Name != "org.freedesktop.DBus.NameOwnerChanged" || len(s.Body) < 3 {
		return false
	}
	nombre, _ := s.Body[0].(string)
	nuevo, _ := s.Body[2].(string)
	return nombre == nombrePortal && nuevo == ""
}

// sinPortalSi traduce «no hay nadie que atienda eso» en errSinPortal, que es lo que hace
// caer al visor. Cualquier otro fallo del portal se devuelve tal cual.
func sinPortalSi(err error) error {
	var e dbus.Error
	if errors.As(err, &e) {
		switch e.Name {
		case "org.freedesktop.DBus.Error.ServiceUnknown",
			"org.freedesktop.DBus.Error.UnknownMethod",
			"org.freedesktop.DBus.Error.UnknownInterface",
			"org.freedesktop.DBus.Error.UnknownObject":
			return errSinPortal
		}
	}
	return err
}

// Elegir abre el selector de ficheros del escritorio, con selección múltiple, y
// devuelve las rutas elegidas: zenity en GNOME y los que lo traen, kdialog en KDE, y si
// no hay ninguno de los dos, el portal de escritorio ([elegirPorPortal]). Cancelar
// devuelve nada, sin error. BLOQUEA: desde una rutina propia.
func Elegir() ([]string, error) {
	if _, err := exec.LookPath("zenity"); err == nil {
		return elegirCon("zenity", "--file-selection", "--multiple", "--separator=\n")
	}
	if _, err := exec.LookPath("kdialog"); err == nil {
		return elegirCon("kdialog", "--getopenfilename", "--multiple", "--separate-output")
	}
	return selectorDelPortal("", nil)
}

func init() {
	porPortalElegir = selectorDelPortal
	porPortalCarpeta = carpetaPorPortal
}

// selectorDelPortal es [elegirPorPortal] con «no hay portal» traducido a ErrNoDisponible,
// que es lo que las apps entienden como «no se ofrece».
func selectorDelPortal(titulo string, extensiones []string) ([]string, error) {
	rutas, err := elegirPorPortal(titulo, extensiones)
	if errors.Is(err, errSinPortal) {
		return nil, ErrNoDisponible
	}
	return rutas, err
}

// conectarPortal abre el bus de sesión y se suscribe a lo que contesta el portal: la
// señal Response del pedido y la caída del propio portal. Cerrar la conexión es de quien
// llama; un error es «no hay portal».
func conectarPortal() (*dbus.Conn, chan *dbus.Signal, error) {
	conn, err := dbus.ConnectSessionBus()
	if err != nil {
		return nil, nil, errSinPortal
	}
	if err := conn.AddMatchSignal(
		dbus.WithMatchInterface("org.freedesktop.portal.Request"),
		dbus.WithMatchMember("Response"),
	); err != nil {
		_ = conn.Close()
		return nil, nil, errSinPortal
	}
	// Sin plazo, como zenity y kdialog: la persona puede tardar lo que quiera en elegir.
	// Lo que corta la espera si el portal muere es su NameOwnerChanged.
	if err := conn.AddMatchSignal(
		dbus.WithMatchSender("org.freedesktop.DBus"),
		dbus.WithMatchInterface("org.freedesktop.DBus"),
		dbus.WithMatchMember("NameOwnerChanged"),
		dbus.WithMatchArg(0, nombrePortal),
	); err != nil {
		_ = conn.Close()
		return nil, nil, errSinPortal
	}
	senales := make(chan *dbus.Signal, 8)
	conn.Signal(senales)
	return conn, senales, nil
}

// elegirPorPortal pide el selector a org.freedesktop.portal.FileChooser: el diálogo de
// GTK o de KDE que el escritorio tenga, sin depender de que haya zenity o kdialog
// instalados, y el único que funciona dentro de un Flatpak. Es el mismo baile que
// [porPortal]: la llamada devuelve un pedido y la respuesta llega por su señal Response,
// con las URI elegidas.
//
// Va DETRÁS de zenity y kdialog y no delante: esos ya funcionan donde están, y el portal
// se estrena aquí para el escritorio que no los trae.
func elegirPorPortal(titulo string, extensiones []string) ([]string, error) {
	conn, senales, err := conectarPortal()
	if err != nil {
		return nil, errSinPortal
	}
	defer func() { _ = conn.Close() }()

	opciones := map[string]dbus.Variant{
		"multiple": dbus.MakeVariant(true),
		"modal":    dbus.MakeVariant(true),
	}
	if f := filtrosPortal(titulo, extensiones); f != nil {
		opciones["filters"] = dbus.MakeVariant(f)
		opciones["current_filter"] = dbus.MakeVariant(f[0])
	}
	return pedirAlPortal(conn, senales, titulo, opciones)
}

// carpetaPorPortal es [elegirPorPortal] para UNA carpeta: la opción `directory` del
// FileChooser (versión 3 del portal). Sin portal, ErrNoDisponible, como el de ficheros.
func carpetaPorPortal(titulo string) (string, error) {
	conn, senales, err := conectarPortal()
	if err != nil {
		return "", ErrNoDisponible
	}
	defer func() { _ = conn.Close() }()
	rutas, err := pedirAlPortal(conn, senales, titulo, map[string]dbus.Variant{
		"directory": dbus.MakeVariant(true),
		"modal":     dbus.MakeVariant(true),
	})
	if errors.Is(err, errSinPortal) {
		return "", ErrNoDisponible
	}
	if err != nil || len(rutas) == 0 {
		return "", err
	}
	return rutas[0], nil
}

// pedirAlPortal abre el FileChooser con esas opciones y espera su respuesta.
func pedirAlPortal(conn *dbus.Conn, senales chan *dbus.Signal, titulo string, opciones map[string]dbus.Variant) ([]string, error) {
	portal := conn.Object("org.freedesktop.portal.Desktop", "/org/freedesktop/portal/desktop")
	var pedido dbus.ObjectPath
	err := portal.Call("org.freedesktop.portal.FileChooser.OpenFile", 0, "", titulo, opciones).Store(&pedido)
	if err != nil {
		return nil, sinPortalSi(err)
	}
	codigo, resultado, err := respuesta(senales, pedido, 0)
	if err != nil || codigo != 0 {
		return nil, err // 1 es «cancelado»: nada elegido y sin error
	}
	uris, _ := resultado["uris"].Value().([]string)
	return rutasDeURIs(uris), nil
}

// filtroPortal es un `(sa(us))` del portal: nombre y patrones; el 0 dice «patrón glob».
type filtroPortal struct {
	Nombre   string
	Patrones []patronPortal
}

type patronPortal struct {
	Tipo   uint32
	Patron string
}

// filtrosPortal traduce las extensiones al filtro del portal. Sin extensiones no hay
// filtro (cualquiera). El glob distingue mayúsculas, así que se piden las dos formas:
// una cámara escribe «IMG_0001.JPG».
func filtrosPortal(titulo string, extensiones []string) []filtroPortal {
	if len(extensiones) == 0 {
		return nil
	}
	f := filtroPortal{Nombre: titulo}
	var nombres []string
	for _, e := range extensiones {
		nombres = append(nombres, "*."+e)
		minus, mayus := strings.ToLower(e), strings.ToUpper(e)
		f.Patrones = append(f.Patrones, patronPortal{0, "*." + minus})
		if mayus != minus {
			f.Patrones = append(f.Patrones, patronPortal{0, "*." + mayus})
		}
	}
	// El nombre es lo que el desplegable de filtros enseña: sin título, los patrones, como
	// hace GTK, y no una entrada en blanco.
	if strings.TrimSpace(f.Nombre) == "" {
		f.Nombre = strings.Join(nombres, " ")
	}
	return []filtroPortal{f}
}

// rutasDeURIs se queda con las rutas locales de las `file://` que da el portal (dentro de
// un Flatpak, las de /run/user/…/doc, que son igual de locales). Lo que no sea un fichero
// local se descarta: quien llama espera rutas que abrir.
func rutasDeURIs(uris []string) []string {
	var rutas []string
	for _, u := range uris {
		p, err := url.Parse(u)
		if err != nil || p.Scheme != "file" || (p.Host != "" && p.Host != "localhost") {
			continue
		}
		if rutaElegida(p.Path) {
			rutas = append(rutas, p.Path)
		}
	}
	return rutas
}
