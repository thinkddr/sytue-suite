// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

// Command drive es Sytue Drive de escritorio y móvil.
//
// Es SU PROPIA aplicación, no una pantalla de un binario que las contenga a todas:
// `docs/CONVENTIONS.md` §13.6 excepción 2. Lo que en la web es ir a otro subdominio,
// aquí es invocar a la otra app por su esquema de URL, que está declarado en
// `FyneApp.toml` y del que la cadena de empaquetado hace las cinco copias que cada
// sistema necesita.
//
// Para VERLA hace falta una máquina con pantalla. Lo que corre en cualquier sitio, sin
// CGO y sin servidor gráfico, es `go test ./apps/drive/...`, que pinta lo mismo con el
// pintor de software y compara los píxeles.
package main

import (
	"context"
	"errors"
	"log/slog"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/app"
	"fyne.io/fyne/v2/storage"

	"git.sytue.eu/sytue/one/apps/fyne/apps/drive"
	"git.sytue.eu/sytue/one/apps/fyne/catalogos"
	"git.sytue.eu/sytue/one/apps/fyne/iconos"
	"git.sytue.eu/sytue/one/apps/fyne/sesion"
	"git.sytue.eu/sytue/one/apps/fyne/tema"
	"git.sytue.eu/sytue/one/apps/fyne/ui"
	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/documentos"
	"git.sytue.eu/sytue/one/apps/nucleo/ficheros"
	"git.sytue.eu/sytue/one/apps/nucleo/i18n"
	"git.sytue.eu/sytue/one/apps/nucleo/sistema"
)

// Medidas de la ventana al abrir: el ancho canónico de la suite y un alto que deja ver
// una docena de filas de 51.
const (
	anchoInicial = 1280
	altoInicial  = 800
)

func main() {
	a := app.New()
	a.Settings().SetTheme(tema.Sytue{})

	// La sesión, la apariencia y el idioma son TRANSVERSALES a las trece apps: esto lo
	// lee de donde lo dejó la última que lo tocó. `Abrir` no habla con la red.
	s := sesion.Abrir(a, "")

	tr, err := i18n.New(s.Idioma(), i18n.Common(), catalogos.Drive())
	if err != nil {
		slog.Error("no se pudo cargar el catálogo de idiomas", "error", err)
		os.Exit(1)
	}

	// El nombre del producto es una MARCA y no se traduce: es el mismo en los 26
	// idiomas, igual que en la web.
	w := a.NewWindow("Sytue Drive")
	w.SetPadded(false)

	// El cliente lleva la sesión COMPARTIDA: el mismo puntero que renueva el token para
	// las trece apps. Sin sesión la pantalla no pide nada y enseña cómo entrar, que es
	// lo que hace la web al redirigir al inicio de sesión.
	// Sin conexión, solo lectura: la caché «red primero» y su monitor son de la sesión.
	red, cliente := s.Conexion("drive", drive.SinGuardarSinConexion...)
	p := drive.NuevaPantalla(tr, api.NewDrive(s.URL("drive"), s.Sesion, cliente), red, w)
	p.Widget = s.CapturarWidget
	p.Docs = api.NewDocs(s.URL("docs"), s.Sesion, cliente)
	p.Directorio = api.NewDirectorio(s.URL("id"), s.Sesion, cliente)
	p.Grupos = s.Sesion.Grupos
	p.IA = api.NewIA(s.URL("ai"), s.Sesion, nil)
	integraciones := api.NewIntegraciones(s.URL("integrations"), s.Sesion, nil)
	p.ConIntegraciones(integraciones)
	p.AutorizarApp = func(ctx context.Context, plantilla, instancia string) (string, error) {
		return s.AutorizarApp(ctx, integraciones, plantilla, instancia, "native-storage")
	}
	p.AutorizarAppNueva = func(ctx context.Context, plantilla string) (string, error) {
		return s.AutorizarAppNueva(ctx, integraciones, plantilla, "native-storage")
	}
	p.AutorizarNube = func(ctx context.Context, proveedor string) (string, error) {
		return s.AutorizarNube(ctx, integraciones, proveedor)
	}
	p.Apex = s.Apex()
	p.Org = s.Sesion.Organizacion
	p.Entrar = func() error { return s.Entrar(context.Background()) }
	abrirSistema := s.AbrirDriveEnSistema
	if archivos := conectarArchivos(s); archivos != nil {
		abrirSistema = func() error { return archivos.abrir(s.AbrirDriveEnSistema) }
	}
	p.ConfigurarSistema = func() { configurarSistema(s, tr, w, abrirSistema) }
	montarSelector(w, p)
	identidadImportador := ""
	var pararImportador context.CancelFunc
	actualizarImportador := func(e sesion.Estado) {
		actual := ""
		if e.Dentro() {
			actual = s.URL("id") + "\x00" + e.Sujeto + "\x00" + s.Sesion.Organizacion()
		}
		if actual == identidadImportador {
			return
		}
		identidadImportador = actual
		if pararImportador != nil {
			pararImportador()
		}
		p.PararImportador()
		if actual != "" {
			vida, parar := context.WithCancel(context.Background())
			pararImportador = parar
			p.IniciarImportador(vida)
		}
	}
	s.AlCambiar(func(e sesion.Estado) {
		actualizarImportador(e)
		p.Sesion(e.Dentro())
	})

	// Arrastrar ficheros del sistema a la ventana los sube a la carpeta abierta, como
	// soltarlos sobre el explorador de la web.
	w.SetOnDropped(func(_ fyne.Position, uris []fyne.URI) { p.Subir(rutasLocales(uris)) })
	// «u» y «n» sueltas, como en la web. El lienzo solo las recibe si no hay nada
	// enfocado —o sea, fuera de un campo—; y con un diálogo o un menú encima, no.
	w.Canvas().SetOnTypedRune(func(r rune) {
		if w.Canvas().Overlays().Top() == nil {
			p.Atajo(r)
		}
	})

	w.SetContent(ui.NuevaBarraSuite(tr, s, iconos.Drive).Montar(p.Objeto()))
	w.Resize(fyne.NewSize(anchoInicial, altoInicial))
	actualizarImportador(s.Estado())
	w.SetOnClosed(func() {
		if pararImportador != nil {
			pararImportador()
		}
		p.PararImportador()
	})
	p.Arrancar(s.Estado().Dentro())
	s.RutaPropia(func(u *url.URL) bool {
		if u == nil {
			return false
		}
		if u.Scheme == "file" || u.Scheme == "content" || (u.Scheme == "" && filepath.IsAbs(u.Path)) {
			return true
		}
		if u.Query().Get("open-system") == "1" && strings.Trim(u.Path, "/") == "" && u.Host == "" {
			return true
		}
		if u.Query().Get("connect-storage") == "1" && strings.Trim(u.Path, "/") == "" && u.Host == "" {
			return true
		}
		if _, _, ok := drive.DestinoDeEnlace(u); ok {
			return true
		}
		if _, ok := api.LeerPeticionDrive(u); ok {
			return true
		}
		if _, ok := api.LeerCompartido(u); ok {
			return true
		}
		if _, ok := drive.SeccionDeEnlace(u); ok {
			return true
		}
		return false
	})
	// «Abrir en la aplicación» del explorador web: la carpeta y el fichero, por ruta.
	s.AlAbrirURL(func(u *url.URL) {
		if u == nil {
			return
		}
		if u.Query().Get("open-system") == "1" && strings.Trim(u.Path, "/") == "" && u.Host == "" {
			go func() {
				if err := abrirSistema(); err != nil {
					slog.Warn("drive: no se pudo abrir en el sistema", "error", err)
				}
			}()
			return
		}
		if u.Query().Get("connect-storage") == "1" && strings.Trim(u.Path, "/") == "" && u.Host == "" {
			fyne.Do(p.AbrirAlmacenamiento)
		} else if carpeta, fichero, ok := drive.DestinoDeEnlace(u); ok {
			fyne.Do(func() { p.IrAEnlace(carpeta, fichero, func() { s.NoEncontrado() }) })
		} else if id, ok := api.LeerPeticionDrive(u); ok {
			fyne.Do(func() { p.AtenderPeticion(id) }) // el aviso de una petición de edición
		} else if id, ok := api.LeerCompartido(u); ok {
			fyne.Do(func() { p.AbrirCompartido(id) }) // el aviso de «compartido contigo»
		} else if sec, ok := drive.SeccionDeEnlace(u); ok {
			fyne.Do(func() { p.IrASeccion(sec) })
		} else if u.Scheme == "file" || u.Scheme == "content" || (u.Scheme == "" && filepath.IsAbs(u.Path)) {
			go func() {
				uriStr := u.String()
				if u.Scheme == "" {
					uriStr = "file://" + u.Path
				}
				uri, err := storage.ParseURI(uriStr)
				if err != nil {
					return
				}
				r, err := storage.Reader(uri)
				if err != nil {
					return
				}
				nombre := uri.Name()
				var tam int64 = -1
				if u.Scheme == "file" || (u.Scheme == "" && u.Path != "") {
					cleanPath := filepath.Clean(u.Path)
					//nolint:gosec // G703: Intencionalmente inspecciona el tamaño del fichero elegido por el usuario.
					if fi, err := os.Stat(cleanPath); err == nil {
						tam = fi.Size()
						if nombre == "" {
							nombre = fi.Name()
						}
					}
				}
				if nombre == "" {
					nombre = "archivo"
				}
				entrada := drive.Fluido(nombre, tam, r)
				fyne.Do(func() {
					p.SubirEntradas([]drive.Entrada{entrada})
				})
			}()
		} else if strings.Trim(u.Path, "/") != "" || strings.Trim(u.Opaque, "/") != "" {
			s.NoEncontrado() // con un receptor apuntado, la sesión ya no sabe qué no es de nadie
		}
	})
	s.Arrancar(context.Background())
	// Los textos del proveedor de documentos de Android y la carpeta sincronizada de iOS.
	// Con el cliente EN LÍNEA: lo que otra app guarda aquí tiene que llegar a Drive, no a
	// la caché sin conexión.
	// En iOS, la copia en «En mi iPhone» sobra donde está la extensión: serían dos Drive.
	go func() {
		if runtime.GOOS == "ios" && sistema.ArchivosDisponible() {
			return
		}
		err := documentos.Servir(context.Background(), api.NewDrive(s.URL("drive"), s.Sesion, nil), documentos.Textos{
			Titulo: a.Metadata().Name, SinApp: tr.T("shell.documentsProvider.openApp", nil),
		})
		if err != nil && !errors.Is(err, documentos.ErrNoDisponible) {
			slog.Warn("drive: el proveedor de documentos de Android no arranca", "error", err)
		}
	}()
	ejecutarDrive(a, w, s, tr)
}

// elegir abre el selector de ficheros DEL SISTEMA —el mismo que abre el navegador con
// `<input type="file" multiple>`, que es lo que hay que igualar— y le pasa a `listo`
// lo elegido en la rutina de la interfaz. El selector bloquea, así que va aparte.
//
// ponytail: sin selector del sistema (un Linux sin zenity ni kdialog) no se abre nada y
// queda soltar los ficheros en la ventana. El móvil tiene el suyo (`selector_movil.go`).
func elegir(listo func([]string)) {
	go func() {
		rutas, err := ficheros.Elegir()
		if err != nil {
			if !errors.Is(err, ficheros.ErrNoDisponible) {
				slog.Warn("drive: el selector de ficheros ha fallado", "error", err)
			}
			return
		}
		if len(rutas) > 0 {
			fyne.Do(func() { listo(rutas) })
		}
	}()
}

// rutasLocales se queda con lo que es del disco —un fichero o una CARPETA, que se sube
// con su árbol—. Lo demás —un `content://` de Android, un enlace soltado desde el
// navegador— no se puede abrir con `os.Open`: el `content://` del selector de Android
// entra por `selector_movil.go`, como flujo.
func rutasLocales(uris []fyne.URI) []string {
	var rutas []string
	for _, u := range uris {
		if u == nil || u.Scheme() != "file" {
			continue
		}
		if _, err := os.Stat(u.Path()); err == nil {
			rutas = append(rutas, u.Path())
		}
	}
	return rutas
}
