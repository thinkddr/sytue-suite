// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

// Package nube pone Sytue Drive en el Explorador de Windows como carpeta de archivos a
// petición (Cloud Files, la misma API que usa OneDrive): cada fichero aparece como un
// marcador sin contenido y se baja al abrirlo. «Mantener siempre en este dispositivo» lo
// baja entero y lo deja para usarlo sin conexión; «Liberar espacio» lo vuelve a vaciar.
// Sustituye en Windows al montaje WebDAV de «Conectar Drive en este equipo», que no
// guardaba nada sin conexión.
//
// Y de ida y vuelta: lo que se guarda encima sube como versión nueva del mismo fichero,
// lo que se crea dentro sube a su carpeta, y renombrar, mover y borrar (también a la
// papelera de reciclaje) se hacen en Drive. Si Drive no lo acepta, Windows tampoco lo
// hace. Si dos equipos guardan a la vez, gana el último: Drive guarda la versión
// anterior, que se recupera desde su historial.
//
// Aquí vive lo que se decide —qué marcadores salen de una carpeta de Drive y con qué
// identidad—, probado en cualquier sistema; en `nube_windows.go`, solo el puente a
// `cldapi.dll`. Verificado el 2026-09-27 en Windows Server 2022 contra Drive de
// producción: marcadores, bajar al abrir (SHA-256 igual al de Drive, también con un
// fichero de 2,5 MB en varios trozos), fijar y soltar un fichero y una carpeta, leer lo
// fijado con el proveedor parado y dar de baja la carpeta.
package nube

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode/utf16"

	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/nombres"
)

// Fuente es lo que hace falta de Drive; la da `*api.Drive`, y el guardián, un doble.
type Fuente interface {
	Root(ctx context.Context) (api.Folder, error)
	Children(ctx context.Context, carpeta, cursor string) (api.Children, error)
	Download(ctx context.Context, fichero string) (io.ReadCloser, error)
	File(ctx context.Context, id string) (api.File, error)
	Upload(ctx context.Context, carpeta, nombre, tipo string, cuerpo io.Reader, tamano int64) (api.File, error)
	CreateFolder(ctx context.Context, padre, nombre string) (api.Folder, error)
	Rename(ctx context.Context, k api.DriveKind, id, nombre string) error
	Move(ctx context.Context, k api.DriveKind, id, destino string) error
	Delete(ctx context.Context, k api.DriveKind, id string) error
}

var _ Fuente = (*api.Drive)(nil)

// ErrNoDisponible es que este sistema no tiene archivos a petición.
var ErrNoDisponible = errors.New("nube: este sistema no tiene archivos a petición")

// Conectar registra `dir` como carpeta de Drive si no lo estaba, y la atiende —bajar lo
// que se abre, enseñar lo que hay en cada carpeta— hasta que `ctx` se cancela. Bloquea.
func Conectar(ctx context.Context, dir string, f Fuente) error { return conectar(ctx, dir, f) }

// Desconectar la da de baja; los ficheros que ya estaban en el equipo se quedan.
func Desconectar(dir string) error { return desconectar(dir) }

// raiz es la identidad de «Mi unidad»; las demás son `c:<id>` y `f:<id>`, como en el
// proveedor de documentos de Android.
const raiz = "raiz"

// Entrada es un marcador: lo que el Explorador enseña antes de bajar nada.
type Entrada struct {
	Nombre     string
	Identidad  string
	Carpeta    bool
	Tamano     int64
	Modificado time.Time
}

// entradas son los marcadores de una carpeta de Drive. Los nombres que Windows no admite
// (`\ / : * ? " < > |` o terminados en punto o espacio) no se enseñan: un marcador con
// ese nombre haría fallar la carpeta entera.
func entradas(h api.Children) []Entrada {
	out := make([]Entrada, 0, len(h.Folders)+len(h.Files))
	for _, c := range h.Folders {
		if nombreValido(c.Name) {
			out = append(out, Entrada{Nombre: c.Name, Identidad: "c:" + c.ID, Carpeta: true, Modificado: c.UpdatedAt})
		}
	}
	for _, f := range h.Files {
		if nombreValido(f.Name) {
			out = append(out, Entrada{Nombre: f.Name, Identidad: IdentidadFichero(f), Tamano: f.SizeBytes, Modificado: f.UpdatedAt})
		}
	}
	return out
}

func nombreValido(n string) bool {
	return n != "" && n != "." && n != ".." && !strings.ContainsAny(n, `\/:*?"<>|`) &&
		!strings.HasSuffix(n, ".") && !strings.HasSuffix(n, " ") && !strings.ContainsFunc(n, func(r rune) bool { return r < 32 }) &&
		!reservadoEnWindows(n) && nombres.Limpio(n) == n
}

// reservadoEnWindows dice si el nombre es un dispositivo de Windows (`CON`, `NUL`,
// `COM1.pdf`…): con extensión o sin ella, y sin distinguir mayúsculas, Windows no lo
// trata como un fichero. Esos, y los que llevan un giro de escritura (U+202E), no se
// ponen en la carpeta (FYS-08).
func reservadoEnWindows(n string) bool {
	base, _, _ := strings.Cut(strings.ToUpper(strings.TrimRight(n, ". ")), ".")
	switch base {
	case "CON", "PRN", "AUX", "NUL", "CONIN$", "CONOUT$":
		return true
	}
	if len(base) == 4 && (strings.HasPrefix(base, "COM") || strings.HasPrefix(base, "LPT")) {
		c := base[3]
		return c >= '0' && c <= '9'
	}
	return false
}

// carpetaDe es el id de Drive de una identidad de carpeta (`raiz` o `c:<id>`).
func carpetaDe(ctx context.Context, f Fuente, identidad string) (string, error) {
	switch {
	case identidad == raiz:
		r, err := f.Root(ctx)
		return r.ID, err
	case strings.HasPrefix(identidad, "c:") && len(identidad) > 2:
		return identidad[2:], nil
	}
	return "", errors.New("nube: no es una carpeta: " + identidad)
}

// ficheroDe es el id de Drive de `f:<id>@<versión>`.
func ficheroDe(identidad string) (string, error) {
	if b := base(identidad); strings.HasPrefix(b, "f:") && len(b) > 2 {
		return b[2:], nil
	}
	return "", errors.New("nube: no es un fichero: " + identidad)
}

// IdentidadFichero es la de un fichero: su id y, detrás de «@», la fecha de Drive de la
// versión que tiene el disco (FILETIME). Con ella se sabe, al subir lo guardado, si
// Drive ha cambiado desde entonces —y entonces hay conflicto— y, al refrescar, si hay
// una versión nueva que bajar.
func IdentidadFichero(f api.File) string {
	return "f:" + f.ID + "@" + strconv.FormatInt(hora(f.UpdatedAt), 10)
}

// base es la identidad sin la versión: la que no cambia aunque cambie el contenido.
func base(identidad string) string {
	if i := strings.IndexByte(identidad, '@'); i > 0 {
		return identidad[:i]
	}
	return identidad
}

// version es la fecha de la versión que tiene el disco, o 0 si la identidad no la lleva.
func version(identidad string) int64 {
	i := strings.IndexByte(identidad, '@')
	if i < 0 {
		return 0
	}
	v, _ := strconv.ParseInt(identidad[i+1:], 10, 64)
	return v
}

// Subida es qué hacer con un fichero que Windows ya no da por sincronizado.
type Subida int

// Los cuatro casos, según si ha cambiado en el disco y si ha cambiado en Drive desde la
// versión que tiene el disco.
const (
	SoloMarcar Subida = iota // no ha cambiado nada (un renombre, un atributo)
	Subir                    // solo el disco: versión nueva
	TomarDrive               // solo Drive: se vacía y se baja la de Drive
	Conflicto                // los dos: lo del disco sube APARTE, con otro nombre, y se baja lo de Drive
)

// decidirSubida compara la versión del disco (la de su identidad) con la de Drive y con
// la fecha y el tamaño del fichero. Al sincronizar, el marcador lleva la fecha de Drive:
// si la sigue teniendo y el tamaño no ha cambiado, el contenido es el de entonces.
func decidirSubida(identidad string, drive api.File, fechaLocal, tamanoLocal int64) Subida {
	v := version(identidad)
	if v == 0 {
		return Subir // una identidad sin versión: no hay con qué comparar
	}
	cambioLocal := fechaLocal != v || tamanoLocal != drive.SizeBytes && hora(drive.UpdatedAt) == v
	cambioDrive := hora(drive.UpdatedAt) != v
	switch {
	case cambioLocal && cambioDrive:
		return Conflicto
	case cambioLocal:
		return Subir
	case cambioDrive:
		return TomarDrive
	}
	return SoloMarcar
}

// nombreConflicto es el nombre con el que sube lo del disco cuando Drive también cambió:
// «informe (conflicto de Windows).docx», junto al original.
func nombreConflicto(nombre string) string {
	ext := filepath.Ext(nombre)
	return strings.TrimSuffix(nombre, ext) + " (conflicto de Windows)" + ext
}

// maxEntradas es el tope de una carpeta, como en el proveedor de Android.
// ponytail: una carpeta de más de 5000 sale cortada; se sube cuando alguien la tenga.
const maxEntradas = 5000

// listar es la carpeta entera, página tras página. `completa` es falso si se cortó en
// `maxEntradas`: lo que falta no se ha visto, no es que Drive no lo tenga.
func listar(ctx context.Context, f Fuente, identidad string) (_ []Entrada, completa bool, _ error) {
	id, err := carpetaDe(ctx, f, identidad)
	if err != nil {
		return nil, false, err
	}
	var todo api.Children
	for cursor := ""; ; {
		h, err := f.Children(ctx, id, cursor)
		if err != nil {
			return nil, false, err
		}
		todo.Folders, todo.Files = append(todo.Folders, h.Folders...), append(todo.Files, h.Files...)
		if cursor = h.NextCursor; cursor == "" || len(todo.Folders)+len(todo.Files) >= maxEntradas {
			return entradas(todo), cursor == "", nil
		}
	}
}

// clase dice qué es una identidad para la API: fichero o carpeta.
func clase(identidad string) (api.DriveKind, string, error) {
	identidad = base(identidad)
	switch {
	case strings.HasPrefix(identidad, "f:") && len(identidad) > 2:
		return api.DriveFile, identidad[2:], nil
	case strings.HasPrefix(identidad, "c:") && len(identidad) > 2:
		return api.DriveFolder, identidad[2:], nil
	}
	return "", "", errors.New("nube: identidad desconocida: " + identidad)
}

// Movimiento es lo que un cambio de nombre o de sitio en el Explorador pide a Drive.
type Movimiento struct {
	Borrar  bool   // ha salido de la carpeta (la papelera de reciclaje, otra unidad)
	Destino string // carpeta nueva (identidad), o "" si no cambia de carpeta
	Nombre  string // nombre nuevo, o "" si no cambia
}

// planRenombre decide qué hacer con un «de aquí a allí» del Explorador.
func planRenombre(dentro bool, padreAntes, padreDespues, nombreAntes, nombreDespues string) Movimiento {
	if !dentro {
		return Movimiento{Borrar: true}
	}
	var m Movimiento
	if padreDespues != padreAntes {
		m.Destino = padreDespues
	}
	if nombreDespues != nombreAntes {
		m.Nombre = nombreDespues
	}
	return m
}

// aplicarMovimiento lo hace en Drive: primero el sitio y luego el nombre, que es lo que
// el servicio comprueba contra los demás de la carpeta de destino.
func aplicarMovimiento(ctx context.Context, f Fuente, identidad string, m Movimiento) error {
	k, id, err := clase(identidad)
	if err != nil {
		return err
	}
	if m.Borrar {
		// Si Drive ya no lo tiene (lo borró otro equipo, o Windows avisa dos veces de lo
		// mismo), borrarlo aquí es lo correcto: negarse dejaría en el disco algo que ya
		// no existe. Medido: el segundo aviso de la papelera de reciclaje daba 404.
		var e *api.Error
		if err := f.Delete(ctx, k, id); err != nil && (!errors.As(err, &e) || e.Status != http.StatusNotFound) {
			return err
		}
		return nil
	}
	if m.Destino != "" {
		destino, err := carpetaDe(ctx, f, m.Destino)
		if err != nil {
			return err
		}
		if err := f.Move(ctx, k, id, destino); err != nil {
			return err
		}
	}
	if m.Nombre != "" {
		return f.Rename(ctx, k, id, m.Nombre)
	}
	return nil
}

// temporal dice si un fichero NUEVO es de los que los editores crean y borran al
// guardar (`~$informe.docx`, `ABC123.tmp`, `.~lock.informe.odt#`): esos no se suben.
func temporal(nombre string) bool {
	n := strings.ToLower(nombre)
	return strings.HasPrefix(n, "~$") || strings.HasSuffix(n, ".tmp") || strings.HasPrefix(n, ".~lock.")
}

// subirNuevo sube un fichero que no estaba en Drive a la carpeta de su identidad.
func subirNuevo(ctx context.Context, f Fuente, carpeta, nombre, tipo string, cuerpo io.Reader, tamano int64) (api.File, error) {
	id, err := carpetaDe(ctx, f, carpeta)
	if err != nil {
		return api.File{}, err
	}
	return f.Upload(ctx, id, nombre, tipo, cuerpo, tamano)
}

// crearCarpeta crea en Drive una carpeta hecha en el Explorador.
func crearCarpeta(ctx context.Context, f Fuente, padre, nombre string) (string, error) {
	id, err := carpetaDe(ctx, f, padre)
	if err != nil {
		return "", err
	}
	c, err := f.CreateFolder(ctx, id, nombre)
	var e *api.Error
	if errors.As(err, &e) && e.Status == http.StatusConflict {
		// Ya hay una con ese nombre: una carpeta que se creó en el disco antes de que
		// Windows la marcase, o la misma creada desde la web. Se adopta, como hace
		// OneDrive, en vez de dejarla sin sincronizar para siempre (visto en Windows
		// Server 2022: «Fotos» avisaba del 409 en cada repaso).
		hay, _, lerr := listar(ctx, f, padre)
		if lerr != nil {
			return "", err
		}
		for _, x := range hay {
			if x.Carpeta && strings.EqualFold(x.Nombre, nombre) {
				return x.Identidad, nil
			}
		}
	}
	if err != nil {
		return "", err
	}
	return "c:" + c.ID, nil
}

// subirVersion sube el contenido nuevo de un fichero que ya estaba en Drive: a su
// carpeta y con su nombre, que el servicio guarda como versión nueva del mismo fichero.
func subirVersion(ctx context.Context, f Fuente, identidad, tipo string, cuerpo io.Reader, tamano int64) (api.File, error) {
	id, err := ficheroDe(identidad)
	if err != nil {
		return api.File{}, err
	}
	x, err := f.File(ctx, id)
	if err != nil {
		return api.File{}, err
	}
	if tipo == "" {
		tipo = x.ContentType
	}
	return f.Upload(ctx, x.FolderID, x.Name, tipo, cuerpo, tamano)
}

// Local es un marcador que ya está en el disco, con lo que Windows sabe de él.
type Local struct {
	Nombre     string
	Identidad  string
	Carpeta    bool
	Tamano     int64
	Modificado int64 // FILETIME: el de Drive cuando se creó o se sincronizó
	EnSync     bool
	SinBajar   bool // solo el marcador: un fichero sin contenido, una carpeta sin poblar
}

// Renombre es un marcador que en Drive se llama de otra forma.
type Renombre struct {
	Local  Local
	Nombre string
}

// Cambios es lo que hay que hacer en el disco para que una carpeta ya poblada vuelva a
// ser la de Drive: lo nuevo en Drive, lo que Drive ya no tiene, lo renombrado y lo que
// tiene contenido nuevo.
type Cambios struct {
	Nuevas     []Entrada
	Quitar     []Local
	Renombrar  []Renombre
	Actualizar []Entrada
}

// comparar decide esos cambios por IDENTIDAD, no por nombre: una carpeta renombrada en
// la web es la misma. Lo que en el disco tiene cambios sin subir (no sincronizado) no se
// toca: gana lo local, que subirá como versión nueva. Con una lista que no está
// `completa` no se quita nada: lo que no se ha visto no es que Drive no lo tenga.
//
// Los nombres se comparan como Windows, sin distinguir mayúsculas. Si lo renombrado en
// Drive se llama como otra cosa del disco, lo nuestro toma un nombre libre en vez de
// pisarla (FYS-03, decidido por el dueño).
func comparar(remotas []Entrada, completa bool, locales []Local) Cambios {
	porIdentidad := map[string]Local{}
	enDisco, enDrive := map[string]bool{}, map[string]bool{}
	for _, l := range locales {
		enDisco[strings.ToLower(l.Nombre)] = true
		if l.Identidad != "" {
			porIdentidad[base(l.Identidad)] = l
		}
	}
	for _, r := range remotas {
		enDrive[strings.ToLower(r.Nombre)] = true
	}
	ocupado := func(n string) bool { return enDisco[strings.ToLower(n)] || enDrive[strings.ToLower(n)] }
	var c Cambios
	vistas := map[string]bool{}
	for _, r := range remotas {
		vistas[base(r.Identidad)] = true
		l, ok := porIdentidad[base(r.Identidad)]
		switch {
		case !ok && !enDisco[strings.ToLower(r.Nombre)]:
			c.Nuevas = append(c.Nuevas, r)
		case !ok || !l.EnSync:
			// Un nombre ocupado por algo que no es de Drive, o cambios sin subir.
		default:
			nombre := r.Nombre
			if enDisco[strings.ToLower(nombre)] && !strings.EqualFold(nombre, l.Nombre) {
				nombre = libre(nombre, r.Carpeta, l.Nombre, ocupado)
			}
			switch {
			case nombre != l.Nombre:
				c.Renombrar = append(c.Renombrar, Renombre{l, nombre})
			case !r.Carpeta && l.Identidad != r.Identidad:
				// Otra versión en Drive: la identidad lleva la fecha de la del disco. Se
				// vacía lo del disco, que puede llamarse de otra forma (un nombre ocupado).
				r.Nombre = l.Nombre
				c.Actualizar = append(c.Actualizar, r)
			}
		}
	}
	if !completa {
		return c
	}
	for _, l := range locales {
		if l.Identidad != "" && l.EnSync && !vistas[base(l.Identidad)] {
			c.Quitar = append(c.Quitar, l)
		}
	}
	return c
}

// libre es el primer «nombre (2).ext», «(3)»… que no usa nadie, como «Conservar ambos»
// del Explorador; una carpeta lo lleva al final. `propio`, el nombre que ya tiene en el
// disco, no cuenta como ocupado: así el refresco siguiente no lo vuelve a mover.
func libre(nombre string, carpeta bool, propio string, ocupado func(string) bool) string {
	ext := filepath.Ext(nombre)
	if carpeta {
		ext = ""
	}
	tronco := strings.TrimSuffix(nombre, ext)
	for i := 2; ; i++ {
		n := tronco + " (" + strconv.Itoa(i) + ")" + ext
		if strings.EqualFold(n, propio) || !ocupado(n) {
			return n
		}
	}
}

// propios son las identidades que el refresco está cambiando en el disco porque ya
// cambiaron en Drive: sus avisos de borrar y renombrar se confirman sin volver a Drive.
var propios sync.Map

// errPendiente es que dentro hay algo que Drive no tiene tal cual.
var errPendiente = errors.New("nube: dentro hay algo sin subir")

// quitar manda a la papelera de reciclaje (`papelera`) algo que Drive ya no tiene en esa
// carpeta; nunca lo borra para siempre. Si dentro hay algo que Drive no tiene tal cual
// —sin subir todavía, con cambios, rechazado o de los que no suben (`temporal`)—, se
// queda entero y da `errPendiente` (FYS-03, decidido por el dueño: «Papelera y no tocar
// lo pendiente»). `leer` es lo que hay en una carpeta del disco; si falla, se queda.
//
// Mientras, TODO lo que cuelga de ello está en `propios`: si Windows avisa de cada hijo
// que sale, ninguno se borra en Drive. Con solo la identidad de arriba, una carpeta que
// otra persona MOVIÓ en la web mandaba su contenido a la papelera de Drive.
func quitar(ruta string, l Local, leer func(string) ([]Local, error), papelera func(string) error) error {
	ids, err := subarbol(ruta, l, leer)
	if err != nil {
		return err
	}
	for _, id := range ids {
		propios.Store(id, true)
	}
	defer func() {
		for _, id := range ids {
			propios.Delete(id)
		}
	}()
	return papelera(ruta)
}

// subarbol son las identidades de `l` y de lo que cuelga de ella, o `errPendiente`. Una
// carpeta sin poblar no tiene nada en el disco: no se entra (entrar la poblaría).
func subarbol(ruta string, l Local, leer func(string) ([]Local, error)) ([]string, error) {
	ids := []string{l.Identidad}
	if !l.Carpeta || l.SinBajar {
		return ids, nil
	}
	hijos, err := leer(ruta)
	if err != nil {
		return nil, err
	}
	for _, h := range hijos {
		if h.Identidad == "" || !h.Carpeta && !h.EnSync {
			return nil, errPendiente
		}
		sub, err := subarbol(filepath.Join(ruta, h.Nombre), h, leer)
		if err != nil {
			return nil, err
		}
		ids = append(ids, sub...)
	}
	return ids, nil
}

// hora es un FILETIME: centésimas de microsegundo desde 1601. Es la fecha que llevan los
// marcadores, y con la que se ve si Drive tiene algo más nuevo.
func hora(t time.Time) int64 {
	if t.IsZero() {
		return 0
	}
	return t.UnixNano()/100 + 116444736000000000
}

// SHFILEOPSTRUCTW de x64 y arm64: sin empaquetar (shellapi.h solo empaqueta en 32 bits).
type operacionShell struct {
	_        uintptr // hwnd
	funcion  uint32
	desde    *uint16 // lista de rutas completas acabada en doble nulo
	_        *uint16 // pTo
	banderas uint16
	abortada int32   // fAnyOperationsAborted
	_        uintptr // hNameMappings
	_        *uint16 // lpszProgressTitle
}

const (
	foBorrar = 0x3 // FO_DELETE
	// FOF_SILENT | FOF_NOCONFIRMATION | FOF_ALLOWUNDO | FOF_NOCONFIRMMKDIR | FOF_NOERRORUI:
	// a la papelera, sin preguntar ni enseñar nada.
	papeleraCallada = 0x4 | 0x10 | 0x40 | 0x200 | 0x400
	// FOF_WANTNUKEWARNING: si algo no cabe en la papelera, Windows pregunta antes de
	// borrarlo para siempre. Sin él, lo borraría sin decir nada.
	avisarSiNoCabe = 0x4000
)

func enviarAPapelera(ruta string, llamar func(*operacionShell) uintptr) error {
	if ruta == "" || strings.IndexByte(ruta, 0) != -1 {
		return errors.New("nube: ruta inválida para la papelera")
	}
	desde := append(utf16.Encode([]rune(ruta)), 0, 0)
	op := operacionShell{funcion: foBorrar, desde: &desde[0], banderas: papeleraCallada | avisarSiNoCabe}
	r := llamar(&op)
	runtime.KeepAlive(desde)
	switch {
	case r != 0:
		return fmt.Errorf("nube: SHFileOperation: 0x%x", r)
	case op.abortada != 0:
		return errors.New("nube: no se ha mandado a la papelera")
	}
	return nil
}
