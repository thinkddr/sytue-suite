// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package sesion

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/storage"

	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/ficheros"
	"git.sytue.eu/sytue/one/apps/nucleo/sistema/escritorio"
)

// # Un fichero de ESTA máquina entra por el mismo sitio que uno de Drive
//
// Es lo que el sistema da y la web no puede: pulsar un `.docx` del escritorio con «Abrir
// con → Sytue Docus», arrancar la app con la ruta, o soltar el fichero encima de la
// ventana. Los tres acaban en `recibirURL` como una URI —`file://…` en escritorio,
// `content://…` en Android— y de ahí en el MISMO `cargar` que ya abre lo de Drive: la app
// no escribe un segundo camino, y el servicio de Docs lo convierte igual que convierte lo
// que baja de Drive (`api.Docs.Importar`, que decide el formato por la FIRMA del
// contenido y no por la extensión).
//
// Dónde lo entrega cada sistema, porque no son el mismo:
//
//   - Linux: el `.desktop` declara los tipos MIME (`empaquetar.sh`) y el gestor de
//     ficheros ejecuta `Exec=… %u` con la ruta o con `file://…`.
//   - Windows: el instalador pone la app en «Abrir con» (`OpenWithProgids`) y el
//     sistema la ejecuta con `"%1"`, que es una ruta con letra de unidad.
//   - Android: un `intent-filter` por tipo MIME, y el fork de Fyne entrega el
//     `getDataString()` del Intent, que es un `content://`. Se lee con `storage.Reader`,
//     que en Android ya sabe abrirlo.
//   - macOS: el Info.plist declara los tipos (`CFBundleDocumentTypes`, `empaquetar.sh`)
//     y el Finder no pasa argumentos: manda el Apple Event `kAEOpenDocuments`, que el
//     fork de Fyne entrega como `file://…` (`app/incoming_urls_darwin.m`).
//
// Y la primera vez que arranca, la app se pone POR DEFECTO para esos tipos donde el
// sistema lo deja (`escritorio.Predeterminada`; decisión del dueño, 25/09/2026).
//
// ponytail: sin sesión, la conversión falla y se dice con el aviso de «no se pudo abrir».
// Lo mejor sería entrar y reintentar solo; se hace cuando alguien lo pida, porque quien
// tiene la suite instalada ya ha entrado una vez para las trece.

// AceptarFicheros hace que esta app abra ficheros de la máquina: soltados sobre `w`,
// pedidos con «Abrir con» o pasados al arrancar. Sin esta llamada se ignoran, y es a
// propósito: solo la llaman las apps cuyo `cargar` sabe convertir un fichero suelto
// (Docus, Calcus y Diapos) y que declaran sus `extensiones` en `FyneApp.toml`.
//
// Si un fichero que era de esta app no se puede abrir, se DICE con un aviso del sistema
// con `titulo` y `texto`, ya traducidos. Aquí no hay web a la que caer —el navegador no ve
// el disco de nadie—, y callarse es el fallo que §13.6 prohíbe.
//
// ponytail: es un aviso del SISTEMA y no uno de la ventana (`ui.PilaAvisos`) porque la
// ventana no tiene todavía una pila montada que la sesión pueda alcanzar. Cuando la barra
// de la suite monte la suya, el aviso va ahí.
func (s *Suite) AceptarFicheros(w fyne.Window, titulo, texto string) {
	s.aceptarFicheros(w, func() { AvisoDeRespaldo(s.app, titulo, texto) })
}

func (s *Suite) aceptarFicheros(w fyne.Window, alFallar func()) {
	s.mu.Lock()
	s.alFallarLocal = alFallar
	antes := s.localesAntes
	s.localesAntes = nil
	s.mu.Unlock()
	for _, f := range antes {
		f()
	}
	s.predeterminarUnaVez()
	// ponytail: una ventana, un documento. Si se sueltan varios, se abren todos por el
	// mismo camino y se queda el último que termina de convertirse.
	w.SetOnDropped(func(_ fyne.Position, uris []fyne.URI) {
		for _, uri := range uris {
			if u, err := url.Parse(uri.String()); err == nil {
				s.recibirURL(u)
			}
		}
	})
}

// clavePredeterminada es la preferencia que recuerda que esta app ya se puso por defecto
// en este equipo. Una vez y no en cada arranque: si el usuario elige después otra, no se
// le vuelve a quitar, y en Windows no se le abre Configuración cada vez. Con guion bajo y
// no con punto: con forma de clave de i18n, `catalogos` la buscaría en los catálogos.
const clavePredeterminada = "escritorio_predeterminada"

// predeterminarUnaVez pone esta app por defecto para las `extensiones` de su FyneApp.toml
// la primera vez que arranca. Sin identificador de paquete —las pruebas, `go run`— no
// hay app que registrar. Si falla, se reintenta en el próximo arranque.
func (s *Suite) predeterminarUnaVez() {
	m := s.app.Metadata()
	ext := strings.FieldsFunc(m.Custom["extensiones"], func(r rune) bool { return r == ',' || r == ' ' })
	prefs := s.app.Preferences()
	if m.ID == "" || len(ext) == 0 || s.app.Driver().Device().IsMobile() || prefs.Bool(clavePredeterminada) {
		return
	}
	go func() {
		if err := escritorio.Predeterminada(m.Name, ext); err != nil && !errors.Is(err, escritorio.ErrNoDisponible) {
			slog.Info("sesion: la app no se ha podido poner por defecto", "error", err)
			return
		}
		prefs.SetBool(clavePredeterminada, true)
	}()
}

// abrirLocal es la rama de AlAbrirFichero para un fichero de la máquina.
func (s *Suite) abrirLocal(producto string, uri fyne.URI, cargar func(context.Context, api.Apertura) error) {
	s.mu.Lock()
	alFallar := s.alFallarLocal
	if alFallar == nil && !s.arrancada {
		// Arrancar con el fichero (el caso de «Abrir con» con la app cerrada): llega antes
		// de que el `main` llame a AceptarFicheros, y se guarda para entonces.
		s.localesAntes = append(s.localesAntes, func() { s.abrirLocal(producto, uri, cargar) })
		s.mu.Unlock()
		return
	}
	s.mu.Unlock()
	if alFallar == nil || cargar == nil {
		return // esta app no abre ficheros sueltos
	}
	// Solo lo que es de ESTE producto: un `.png` soltado en Docus no es un documento que
	// convertir. En Android el nombre de un `content://` no siempre trae extensión, y ahí
	// ya filtró el sistema por tipo MIME con el `intent-filter`.
	if uri.Scheme() != "content" {
		if p, _ := api.AppQueAbre(uri.Name()); p != producto {
			return
		}
	}
	go func() {
		ctx, parar := context.WithTimeout(context.Background(), plazoApertura)
		defer parar()
		if err := cargar(ctx, api.Apertura{Local: uri.String()}); err != nil {
			slog.Info("sesion: el fichero de la máquina no se ha podido abrir", "fichero", uri.Name(), "motivo", err)
			fyne.Do(alFallar)
		}
	}()
}

// ficheroLocal dice si un enlace entrante es un fichero de esta máquina, y cuál.
//
// Una letra sola como esquema es una unidad de Windows: el fork de Fyne entrega cada
// argumento que se parsea con esquema, y `C:\Docs\acta.docx` se parsea como `c:` con el
// resto opaco. Es lo mismo que `EsEnlaceDeLaSuite` tiene que esquivar, visto al revés.
func ficheroLocal(u *url.URL) (fyne.URI, bool) {
	switch {
	case u == nil:
		return nil, false
	case u.Scheme == "file" || u.Scheme == "content":
		uri, err := storage.ParseURI(u.String())
		return uri, err == nil
	case len(u.Scheme) == 1 && u.Opaque != "":
		return storage.NewFileURI(u.Scheme + ":" + u.Opaque), true
	}
	return nil, false
}

// ficheroDeArg es un argumento de la línea de órdenes que nombra un fichero que existe,
// como `file://…`, o false.
//
// Hace falta por dos lados: el fork de Fyne solo entrega los argumentos CON esquema, así
// que una ruta pelada —`/home/ada/acta.docx`, que es lo que pasan Nautilus y Dolphin con
// `%u`— no llegaría; y la instancia que ya corre recibe por su enchufe URL, no rutas.
func ficheroDeArg(a string) (string, bool) {
	if u, err := url.Parse(a); err == nil && u.Scheme != "" {
		if u.Scheme == "file" {
			return a, true
		}
		if len(u.Scheme) != 1 {
			return "", false // otro esquema: no es un fichero
		}
		// una letra de unidad de Windows: se mira como ruta, igual que las demás
	}
	abs, err := filepath.Abs(a)
	if err != nil {
		return "", false
	}
	if fi, err := os.Stat(abs); err != nil || !fi.Mode().IsRegular() { //nolint:gosec // G703: es la ruta que el sistema pasa para abrir con la app; abrirla es lo que se pide
		return "", false
	}
	return storage.NewFileURI(abs).String(), true
}

// leerLocal trae el contenido de un fichero de la máquina con el mismo tope que tiene el
// conversor, para no subir cuarenta megas y recibir un 413.
func leerLocal(crudo string) (string, []byte, error) {
	uri, err := storage.ParseURI(crudo)
	if err != nil {
		return "", nil, err
	}
	nombre := uri.Name()
	if nombre == "" {
		nombre = filepath.Base(crudo)
	}

	// Si es un fichero del sistema (file:// o ruta absoluta), usamos la vía segura
	// del núcleo que maneja security scopes en iOS y lectura limpia sin depender
	// de punteros nativos efímeros.
	if uri.Scheme() == "file" || strings.HasPrefix(crudo, "/") {
		datos, errSeguro := ficheros.LeerLocalSeguro(crudo)
		if errSeguro == nil && len(datos) > 0 {
			if len(datos) > api.MaxImportar {
				return "", nil, fmt.Errorf("sesion: %q pasa de los %d MiB que convierte el servicio", nombre, api.MaxImportar>>20)
			}
			return nombre, datos, nil
		}
	}

	r, err := storage.Reader(uri)
	if err != nil {
		return "", nil, err
	}
	defer func() { _ = r.Close() }()
	datos, err := io.ReadAll(io.LimitReader(r, api.MaxImportar+1))
	if err != nil {
		return "", nil, err
	}
	if len(datos) > api.MaxImportar {
		return "", nil, fmt.Errorf("sesion: %q pasa de los %d MiB que convierte el servicio", nombre, api.MaxImportar>>20)
	}
	return nombre, datos, nil
}

// LeerLocal es leerLocal para quien no convierte el fichero sino que lo sube tal cual:
// CertiPDF manda el PDF de la máquina a la carpeta «PDF» de Drive, como hace la web.
func LeerLocal(uri string) (nombre string, datos []byte, err error) { return leerLocal(uri) }
