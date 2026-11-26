// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

// Package local es la cáscara de las apps de Sytue Office, la edición de la suite que
// funciona entera en el dispositivo: SIN cuenta de Sytue ID, SIN cifrado propio y SIN
// ningún servidor de Sytue (apps/fyne/PLAN-EDICION-LOCAL.md).
//
// Sustituye, para estas apps, a `sesion.Abrir` y a `cifrado.NewPrivateApplication`:
//
//   - no lee ningún llavero de sesión ni construye un cliente OIDC;
//   - los datos van a la carpeta de datos de la persona (`~/Sytue Office`), en claro;
//   - la IA y la nube son opcionales y las configura la persona en Ajustes; sin
//     configurar nada, la app no abre ni una conexión (R-RED).
//
// Las pantallas son las mismas que las de la edición conectada (D-04): lo que cambia es
// lo que se les enchufa, y eso se hace en `cmd-local/<app>/main.go` con lo de aquí.
package local

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/storage"

	"git.sytue.eu/sytue/one/apps/fyne/anfitrion"
	"git.sytue.eu/sytue/one/apps/fyne/tema"
	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/ficheros"
	"git.sytue.eu/sytue/one/apps/nucleo/i18n"
	"git.sytue.eu/sytue/one/apps/nucleo/local/agenda"
	"git.sytue.eu/sytue/one/apps/nucleo/local/almacen"
	"git.sytue.eu/sytue/one/apps/nucleo/local/asistente"
	"git.sytue.eu/sytue/one/apps/nucleo/local/avisos"
	"git.sytue.eu/sytue/one/apps/nucleo/local/biblioteca"
	"git.sytue.eu/sytue/one/apps/nucleo/local/copia"
	"git.sytue.eu/sytue/one/apps/nucleo/local/cuentas"
	"git.sytue.eu/sytue/one/apps/nucleo/local/espacio"
	"git.sytue.eu/sytue/one/apps/nucleo/local/nube"
	"git.sytue.eu/sytue/one/apps/nucleo/local/red"
	"git.sytue.eu/sytue/one/apps/nucleo/local/secretos"
	"git.sytue.eu/sytue/one/apps/nucleo/sistema"
	"git.sytue.eu/sytue/one/apps/nucleo/sistema/escritorio"
)

// App es una app de Sytue Office abierta.
type App struct {
	A        fyne.App
	Producto string // la clave de producto de la suite: "docs", "notes", "mail"…
	Espacio  *espacio.Espacio
	Secretos *secretos.Almacen
	IA       *asistente.Asistente
	Nube     *nube.Registro
	// Red es el portero: la única puerta a la red (§5.8). Cuentas, las cuentas externas
	// de toda la edición (§5.10); Copia, la copia de seguridad (§5.13); Avisos, los
	// recordatorios programados en el sistema (§5.12).
	Red     *red.Portero
	Cuentas *cuentas.Registro
	Copia   *copia.Copia
	Avisos  *avisos.Avisos

	mu            sync.Mutex
	abrirElemento func(clase, id string)
	elementos     [][2]string
	abrirEnlace   func(*url.URL)
	enlaces       []*url.URL
	// arrancada: el bucle de la interfaz ya corre y la ventana tiene lienzo. Hasta
	// entonces, los enlaces y elementos que llegan (los argumentos) esperan: un formulario
	// abierto antes no tendría dónde pintarse.
	arrancada   bool
	abrir       func(context.Context, api.Apertura) error
	pendientes  []api.Apertura
	alCambiarIA []func()
	alFallar    func(error)
	exportar    func() (string, []byte, error)
	cerrar      []func()
	// Lo que la app añade a Ajustes (ajustes_apps.go).
	secciones    []seccionApp
	gestores     map[string]GestorCuentas
	exportarTodo func(ctx context.Context, carpeta string) error
	agenda       *agenda.Registro
	// vuelta recoge en el móvil la vuelta del navegador de las altas OAuth por el
	// esquema de la app (`sytueoffice-<producto>:/oauth`, §5.10); nil en el escritorio,
	// donde vuelven a 127.0.0.1.
	vuelta *nube.Vuelta
}

// Abrir prepara una app local. No habla con la red. Un binario compilado sin la
// etiqueta `edicionlocal` no arranca ([ErrSinEtiqueta], D-02).
func Abrir(a fyne.App, producto string) (*App, error) {
	if !escritorio.EdicionLocal {
		return nil, ErrSinEtiqueta
	}
	// En Android no hay $HOME hasta que se le da la carpeta privada de la app: sin ella
	// no se lee ni se guarda nada.
	anfitrion.PrepararCasa()
	// En Android, `ficheros.Elegir` («Abrir…», «Añadir fotos», «Importar…») abre el
	// selector de documentos del sistema, como en la edición conectada (anfitrion
	// elegir.go). En iOS ya lo pone `ficheros` al arrancar (movil_ios.go).
	if runtime.GOOS == "android" {
		anfitrion.PonerSelector(a)
	}
	raiz, grupo := "", ""
	if Tactil() {
		raiz = a.Storage().RootURI().Path()
		// En iOS las apps de la edición comparten los ajustes por su App Group (si el
		// perfil de aprovisionamiento lo lleva; si no, cada app tiene los suyos).
		grupo, _ = ficheros.CarpetaDeGrupo(espacio.GrupoApple)
	}
	e, err := espacio.AbrirMovil(raiz, grupo)
	if err != nil {
		return nil, fmt.Errorf("local: no se pudo abrir la carpeta de Sytue Office: %w", err)
	}
	s := secretos.Nuevo(espacio.ServicioLlavero, filepath.Join(e.Rutas.Ajustes, "secretos"))
	l := &App{A: a, Producto: producto, Espacio: e, Secretos: s, IA: asistente.Abrir(e, s), Nube: nube.Abrir(e, s)}
	// `.docus`, `.calcus` y `.diapos` son de la suite desde aquí (api.AppQueAbre).
	api.FormatosOffice = true
	l.instalarPortero()
	l.prepararVuelta(raiz != "")
	l.prepararCopia()
	l.prepararAvisos()
	l.aplicarApariencia()
	// Las apps de la edición no usan SetOnStarted: es de la cáscara (alArrancar).
	a.Lifecycle().SetOnStarted(l.alArrancar)
	if r, ok := a.(fyne.URLHandler); ok {
		r.SetOnOpenURL(func(u *url.URL) { l.recibir(u) })
	}
	// Una ruta pelada en los argumentos («Abrir con» en Linux y Windows, o a mano), o un
	// enlace de los esquemas que la app registra (el suyo, `mailto:`…).
	extra := esquemasExtra(a)
	var enlaces []*url.URL
	for _, arg := range os.Args[1:] {
		if u, ok := uriDeArgumento(arg, extra); ok {
			enlaces = append(enlaces, u)
		}
	}
	// Si la app ya está abierta, lo que traía va a esa copia y esta se cierra.
	l.copiaUnica(enlaces)
	for _, u := range enlaces {
		l.recibir(u)
	}
	return l, nil
}

// prepararVuelta pone, en el móvil, la vuelta de las altas OAuth por el esquema de la
// app (el de su FyneApp.toml, que `empaquetar.sh` declara en iOS y Android): todas las
// altas (nube, Calendar, Contacts, Mail, Rooms) pasan por `nube.OAuth.Conectar`.
func (l *App) prepararVuelta(movil bool) {
	if !movil {
		return
	}
	l.vuelta = nube.NuevaVuelta(cmp.Or(l.A.Metadata().Custom["esquema"], espacio.Esquema(l.Producto)))
	nube.UsarVuelta(l.vuelta)
}

// Idioma es el del perfil, o el del sistema; `SYTUE_IDIOMA` manda para mirar otro.
func (l *App) Idioma() i18n.Locale {
	return i18n.Negotiate(sistema.Idioma(), cmp.Or(os.Getenv("SYTUE_IDIOMA"), l.Espacio.Perfil.Idioma))
}

// Traductor carga los catálogos del producto en el idioma de la app.
func (l *App) Traductor(catalogos ...fs.FS) (*i18n.Translator, error) {
	return i18n.New(l.Idioma(), append([]fs.FS{i18n.Common()}, catalogos...)...)
}

func (l *App) aplicarApariencia() {
	l.A.Settings().SetTheme(tema.Sytue{})
	tema.Elegir(l.Espacio.Perfil.Tema)
}

// Biblioteca abre la biblioteca de documentos de un producto de la ofimática.
func (l *App) Biblioteca(producto string) (*biblioteca.Biblioteca, error) {
	b, err := biblioteca.Abrir(l.Espacio.Rutas.Biblioteca(), producto, l.Espacio.Persona())
	if err == nil {
		b.Idioma = string(l.Idioma())
	}
	return b, err
}

// Almacen abre el almacén en claro de un producto que usa el motor de registros
// (notas, contactos, calendario, fotos, bases…). Se cierra al cerrar la app.
func (l *App) Almacen(ctx context.Context, producto string) (*almacen.Recursos, error) {
	r, err := almacen.Abrir(ctx, l.Espacio.Rutas.Producto(producto), producto, l.Espacio.ID)
	if err == nil {
		// Las versiones viejas y lo borrado del todo se van del disco al cerrar (§5.6);
		// con otra copia abierta o trabajo a medias no se compacta y no pasa nada.
		r.CompactarAlCerrar = true
		l.AlCerrar(r.Cerrar)
	}
	return r, err
}

// AlCerrar apunta algo que hacer cuando se cierre la ventana principal.
func (l *App) AlCerrar(f func()) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.cerrar = append(l.cerrar, f)
}

// Cerrar ejecuta lo apuntado con AlCerrar, del último al primero.
func (l *App) Cerrar() {
	l.mu.Lock()
	pendientes := l.cerrar
	l.cerrar = nil
	l.mu.Unlock()
	for i := len(pendientes) - 1; i >= 0; i-- {
		pendientes[i]()
	}
}

// Asistir es la función de IA para las pantallas; nil con la IA apagada (y entonces
// las pantallas ocultan sus botones de IA).
func (l *App) Asistir() func(context.Context, api.PeticionIA) (string, error) {
	return l.IA.Funcion()
}

// AlCambiarIA avisa cuando la persona cambia la IA en Ajustes, para volver a enchufarla.
func (l *App) AlCambiarIA(f func()) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.alCambiarIA = append(l.alCambiarIA, f)
}

func (l *App) iaCambiada() {
	l.mu.Lock()
	avisar := append([]func(){}, l.alCambiarIA...)
	l.mu.Unlock()
	for _, f := range avisar {
		f()
	}
}

// AlExportar da a «Guardar en la nube» el documento actual: nombre y bytes.
func (l *App) AlExportar(f func() (string, []byte, error)) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.exportar = f
}

// ----- Ficheros que llegan -----

// AlAbrirFichero registra cómo abre esta app un fichero de la máquina (`Apertura.Local`):
// el que llega por «Abrir con», por los argumentos, soltado en la ventana o descargado de
// la nube. Lo que llegó antes de registrarlo se entrega ahora.
func (l *App) AlAbrirFichero(f func(context.Context, api.Apertura) error, alFallar func(error)) {
	l.mu.Lock()
	l.abrir, l.alFallar = f, alFallar
	antes := l.pendientes
	l.pendientes = nil
	l.mu.Unlock()
	for _, ap := range antes {
		l.entregar(ap)
	}
}

// AceptarFicheros abre lo que se suelte en la ventana.
func (l *App) AceptarFicheros(w fyne.Window) {
	w.SetOnDropped(func(_ fyne.Position, uris []fyne.URI) {
		for _, u := range uris {
			if p, err := url.Parse(u.String()); err == nil {
				l.recibir(p)
			}
		}
	})
}

// AbrirLocal abre un fichero de la máquina en esta app.
func (l *App) AbrirLocal(ruta string) {
	l.entregar(api.Apertura{Local: storage.NewFileURI(ruta).String()})
}

func (l *App) recibir(u *url.URL) {
	if u == nil {
		return
	}
	// La vuelta de un alta OAuth en el móvil no es un enlace para la app: es de la
	// cuenta que se está conectando (y, si no hay ninguna en marcha, se descarta).
	if l.vuelta != nil && l.vuelta.Recibir(u) {
		return
	}
	switch {
	case u.Scheme == "file" || u.Scheme == "content":
		l.entregar(api.Apertura{Local: u.String()})
	case len(u.Scheme) == 1 && u.Opaque != "": // C:\… en Windows
		l.entregar(api.Apertura{Local: storage.NewFileURI(u.Scheme + ":" + u.Opaque).String()})
	case strings.HasPrefix(u.Scheme, espacio.PrefijoEsquema):
		// `sytueoffice-docs:/abrir?local=file:///…` (abrir entre apps, §5.11).
		// `sytueoffice-notes:/abrir?clase=nota&id=…` (el Hub, un aviso): AlAbrirElemento.
		// Cualquier otro enlace del esquema (`sytueoffice-mail:/redactar?…`,
		// `sytueoffice-calendar:/events/new?action=create…`): AlAbrirEnlace.
		q := u.Query()
		if loc := q.Get("local"); loc != "" {
			l.entregar(api.Apertura{Local: loc})
		} else if id := q.Get("id"); id != "" {
			l.entregarElemento(q.Get("clase"), id)
		} else {
			l.entregarEnlace(u)
		}
	case u.Scheme != "" && u.Scheme != "http" && u.Scheme != "https":
		// Un esquema más de la app (`mailto:` en Mail), por `SetOnOpenURL` (macOS, iOS,
		// Android) o por los argumentos. Las direcciones web no se atienden aquí: soltar
		// un enlace en la ventana no abre nada.
		l.entregarEnlace(u)
	}
}

func (l *App) entregar(ap api.Apertura) {
	l.mu.Lock()
	f, alFallar := l.abrir, l.alFallar
	if f == nil {
		l.pendientes = append(l.pendientes, ap)
		l.mu.Unlock()
		return
	}
	l.mu.Unlock()
	go func() {
		if err := f(context.Background(), ap); err != nil {
			slog.Warn("local: no se pudo abrir el fichero", "error", err)
			if alFallar != nil {
				fyne.Do(func() { alFallar(err) })
			}
		}
	}()
}

// esquemasExtra son los esquemas de más que la app registra al empaquetarse
// (`[Development] esquemas_extra` del FyneApp.toml, que llega como metadato); `mailto`
// se entiende siempre, porque es el que el sistema da al programa de correo.
func esquemasExtra(a fyne.App) map[string]bool {
	extra := map[string]bool{"mailto": true}
	for _, e := range strings.Split(a.Metadata().Custom["esquemas_extra"], ",") {
		if e = strings.ToLower(strings.TrimSpace(e)); e != "" {
			extra[e] = true
		}
	}
	return extra
}

func uriDeArgumento(a string, extra map[string]bool) (*url.URL, bool) {
	if u, err := url.Parse(a); err == nil && u.Scheme != "" && len(u.Scheme) > 1 {
		esq := strings.ToLower(u.Scheme)
		return u, esq == "file" || strings.HasPrefix(esq, espacio.PrefijoEsquema) || extra[esq]
	}
	abs, err := filepath.Abs(a)
	if err != nil {
		return nil, false
	}
	if fi, err := os.Stat(abs); err != nil || !fi.Mode().IsRegular() { //nolint:gosec // G703: un argumento de la línea de órdenes, el fichero que la persona pidió abrir
		return nil, false
	}
	u, err := url.Parse(storage.NewFileURI(abs).String())
	return u, err == nil
}

// LeerLocal lee un fichero de la máquina (file:// o content://) con el tope del conversor.
func LeerLocal(uri string) (string, []byte, error) { return anfitrion.LeerLocal(uri) }

// LeerLocalHasta es LeerLocal con el tope que elige quien llama, en bytes (CertiPDF lee
// un `content://` de Android hasta los 100 MiB de su motor).
func LeerLocalHasta(uri string, tope int) (string, []byte, error) {
	return anfitrion.LeerLocalHasta(uri, tope)
}

// Importar lee el fichero local de ap y lo convierte al modelo de la suite EN EL
// DISPOSITIVO (office/model). Es el sustituto de `sesion.Importar`, que lo mandaba al
// servicio: aquí no sale nada del equipo.
func (l *App) Importar(_ context.Context, ap api.Apertura) (api.Importado, error) {
	return l.importar(ap, false)
}

// ImportarGuardando es Importar con los comentarios del fichero.
func (l *App) ImportarGuardando(_ context.Context, ap api.Apertura) (api.Importado, error) {
	return l.importar(ap, true)
}

// ErrSinFichero: la apertura no trae un fichero de la máquina (en local no hay Drive).
var ErrSinFichero = errors.New("local: no es un fichero de este equipo")

func (l *App) importar(ap api.Apertura, comentarios bool) (api.Importado, error) {
	if ap.Local == "" {
		return api.Importado{}, ErrSinFichero
	}
	nombre, datos, err := LeerLocal(ap.Local)
	if err != nil {
		return api.Importado{}, err
	}
	return biblioteca.Importar(nombre, datos, comentarios)
}

// ----- Macros y preferencias por persona -----

// Personal cumple `editor.AlmacenPersonal` en un fichero de los ajustes: las macros
// grabadas de quien usa el equipo.
type Personal struct {
	ruta string
	mu   sync.Mutex
	vals map[string]string
}

// Personal devuelve el almacén por persona de esta instalación.
func (l *App) Personal() *Personal {
	p := &Personal{ruta: l.Espacio.Ajuste("personal.json"), vals: map[string]string{}}
	_ = espacio.LeerJSON(p.ruta, &p.vals)
	return p
}

// Leer devuelve el valor de clave.
func (p *Personal) Leer(clave string) string {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.vals[clave]
}

// Poner guarda valor en clave.
func (p *Personal) Poner(clave, valor string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.vals[clave] = valor
	_ = espacio.EscribirJSON(p.ruta, p.vals)
}

// Persona es siempre la de esta instalación.
func (p *Personal) Persona() string { return "local" }
