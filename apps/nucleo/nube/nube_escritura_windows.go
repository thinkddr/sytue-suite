// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

//go:build windows

package nube

import (
	"context"
	"errors"
	"log/slog"
	"mime"
	"os"
	"path/filepath"
	"sync"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"

	"git.sytue.eu/sytue/one/apps/nucleo/api"
)

// # De vuelta a Drive
//
// Tres avisos de Windows y la vigilancia de la carpeta:
//
//   - NOTIFY_FILE_CLOSE_COMPLETION: alguien cierra un marcador. Si lo que tiene ya no es
//     lo que se bajó (Windows le quita el «sincronizado» al escribir), sube como versión
//     nueva y se vuelve a marcar sincronizado.
//   - NOTIFY_DELETE y NOTIFY_RENAME: borrar, renombrar y mover. Windows ESPERA la
//     respuesta (ACK_DELETE, ACK_RENAME): si Drive no lo acepta, en el disco tampoco pasa.
//     Mandar a la papelera de reciclaje es un «mover fuera»: en Drive, a su papelera.
//   - Lo NUEVO (un fichero o una carpeta que no eran marcadores) lo ve `vigilar`: sube
//     cuando quien lo escribe lo suelta, y se convierte en marcador sincronizado.

var (
	procInfoMarcador = cldapi.NewProc("CfGetPlaceholderInfo")
	procConvertir    = cldapi.NewProc("CfConvertToPlaceholder")
	procActualizar   = cldapi.NewProc("CfUpdatePlaceholder")
	procCrear        = cldapi.NewProc("CfCreatePlaceholders")
)

// CF_FS_METADATA: FILE_BASIC_INFO (un 0 es «no cambiar») y el tamaño.
type metadatos struct {
	creado, accedido, escrito, cambiado int64
	atributos                           uint32
	_                                   uint32
	tamano                              int64
}

const (
	llamadaCerrado   = 6  // CF_CALLBACK_TYPE_NOTIFY_FILE_CLOSE_COMPLETION
	llamadaBorrar    = 9  // CF_CALLBACK_TYPE_NOTIFY_DELETE
	llamadaRenombrar = 11 // CF_CALLBACK_TYPE_NOTIFY_RENAME

	operacionAckBorrar    = 6 // CF_OPERATION_TYPE_ACK_DELETE
	operacionAckRenombrar = 7 // CF_OPERATION_TYPE_ACK_RENAME

	cerradoBorrado = 0x1 // CF_CALLBACK_CLOSE_COMPLETION_FLAG_DELETED
	// CF_UPDATE_FLAG_MARK_IN_SYNC: recién subido, lo del disco es lo de Drive.
	actualizarEnSync = 0x2
	// CF_UPDATE_FLAG_VERIFY_IN_SYNC | …_MARK_IN_SYNC | …_DEHYDRATE: Drive tiene
	// contenido nuevo y el disco no tiene nada sin subir (si lo tuviera, Windows se niega
	// y gana lo local): se vacía y se baja al abrir.
	actualizarVaciar = 0x1 | 0x2 | 0x4
	// Sin VERIFY: lo del disco ya se ha subido aparte (conflicto) o no ha cambiado.
	actualizarTomar = 0x2 | 0x4
	destinoDentro   = 0x4 // CF_CALLBACK_RENAME_FLAG_TARGET_IN_SCOPE
	sincronizado    = 1   // CF_IN_SYNC_STATE_IN_SYNC
	convertirEnSync = 0x1 // CF_CONVERT_FLAG_MARK_IN_SYNC
)

// CF_CALLBACK_PARAMETERS de NOTIFY_FILE_CLOSE_COMPLETION y NOTIFY_DELETE: solo Flags.
type paramsBanderas struct {
	_        [8]byte // ParamSize y relleno
	banderas uint32
}

// CF_CALLBACK_PARAMETERS de NOTIFY_RENAME.
type paramsRenombrar struct {
	_        [8]byte
	banderas uint32
	_        uint32
	destino  *uint16 // TargetPath, desde la raíz del volumen
}

// CF_OPERATION_PARAMETERS de ACK_DELETE y ACK_RENAME.
type opAck struct {
	tamano uint32
	_      uint32
	_      uint32 // Flags
	estado int32
}

// rutaDe es la ruta completa de la retrollamada: la letra de la unidad y lo demás.
func rutaDe(info *infoLlamada, resto *uint16) string {
	return windows.UTF16PtrToString(info.volumen) + windows.UTF16PtrToString(resto)
}

// alCerrar sube lo que se ha guardado encima de un marcador.
func alCerrar(info *infoLlamada, p *paramsBanderas) uintptr {
	if p.banderas&cerradoBorrado == 0 {
		go versionSiHaceFalta(ctxConexion(), rutaDe(info, info.ruta))
	}
	return 0
}

// alBorrar lo borra en Drive (a su papelera) y se lo confirma a Windows.
func alBorrar(info *infoLlamada, _ *paramsBanderas) uintptr {
	c := claves{info.conexion, info.transferencia, info.peticion}
	identidad := identidadDe(info.identidad, info.largoIdentidad)
	if _, ok := propios.Load(identidad); ok {
		go confirmar(c, operacionAckBorrar, nil, "borrar", identidad) // lo hace el refresco
		return 0
	}
	go func() {
		err := enDrive(ctxConexion(), func(ctx context.Context, f Fuente) error {
			return aplicarMovimiento(ctx, f, identidad, Movimiento{Borrar: true})
		})
		confirmar(c, operacionAckBorrar, err, "borrar", identidad)
	}()
	return 0
}

// alRenombrar lo renombra o lo mueve en Drive, o lo borra si sale de la carpeta.
func alRenombrar(info *infoLlamada, p *paramsRenombrar) uintptr {
	c := claves{info.conexion, info.transferencia, info.peticion}
	identidad := identidadDe(info.identidad, info.largoIdentidad)
	antes, despues := rutaDe(info, info.ruta), rutaDe(info, p.destino)
	dentro := p.banderas&destinoDentro != 0
	if _, ok := propios.Load(identidad); ok {
		go confirmar(c, operacionAckRenombrar, nil, "renombrar", identidad) // lo hace el refresco
		return 0
	}
	go func() {
		err := enDrive(ctxConexion(), func(ctx context.Context, f Fuente) error {
			padreAntes, err := identidadDeRuta(filepath.Dir(antes))
			if err != nil {
				return err
			}
			padreDespues := ""
			if dentro {
				if padreDespues, err = identidadDeRuta(filepath.Dir(despues)); err != nil {
					return err
				}
			}
			m := planRenombre(dentro, padreAntes, padreDespues, filepath.Base(antes), filepath.Base(despues))
			return aplicarMovimiento(ctx, f, identidad, m)
		})
		confirmar(c, operacionAckRenombrar, err, "renombrar", identidad)
	}()
	return 0
}

// enDrive hace `hacer` con la fuente de la conexión viva; sin ella, falla.
func enDrive(ctx context.Context, hacer func(context.Context, Fuente) error) error {
	f, _ := actual()
	if f == nil || ctx == nil {
		return errors.New("nube: sin conexión con Drive")
	}
	return hacer(ctx, f)
}

// ctxConexion es el contexto de la conexión viva, para lo que arranca Windows.
func ctxConexion() context.Context {
	_, ctx := actual()
	return ctx
}

// confirmar responde a Windows: sí, o no y entonces no se hace.
func confirmar(c claves, tipo uint32, err error, que, identidad string) {
	op := c.operacion(tipo)
	p := opAck{}
	if err != nil {
		p.estado = fallo // con un NTSTATUS de error, Windows no lo hace en el disco
		slog.Warn("nube: Drive no lo ha aceptado y no se hace en el disco", "que", que, "elemento", identidad, "error", err)
	}
	p.tamano = uint32(unsafe.Sizeof(p))
	r, _, _ := procExecute.Call(uintptr(unsafe.Pointer(&op)), uintptr(unsafe.Pointer(&p)))
	if e := hr(r, "CfExecute(ACK)"); e != nil {
		slog.Warn("nube: Windows no ha recibido la respuesta", "que", que, "error", e)
	}
}

// abrir da un manejador de `ruta` con `acceso`, también para una carpeta.
func abrir(ruta string, acceso uint32) (windows.Handle, error) {
	p, err := windows.UTF16PtrFromString(ruta)
	if err != nil {
		return 0, err
	}
	return windows.CreateFile(p, acceso, windows.FILE_SHARE_READ|windows.FILE_SHARE_WRITE|windows.FILE_SHARE_DELETE,
		nil, windows.OPEN_EXISTING, windows.FILE_FLAG_BACKUP_SEMANTICS, 0)
}

// marcadorDe lee CF_PLACEHOLDER_BASIC_INFO: la identidad y si está sincronizado. Un
// fichero que no es marcador da error.
func marcadorDe(h windows.Handle) (identidad string, enSync bool, err error) {
	var buf [32 + 4096]byte
	var largo uint32
	r, _, _ := procInfoMarcador.Call(uintptr(h), 0, uintptr(unsafe.Pointer(&buf[0])), uintptr(len(buf)), uintptr(unsafe.Pointer(&largo)))
	if err := hr(r, "CfGetPlaceholderInfo"); err != nil {
		return "", false, err
	}
	// PinState (4), InSyncState (4), FileId (8), SyncRootFileId (8), FileIdentityLength (4), FileIdentity.
	enSync = *(*uint32)(unsafe.Pointer(&buf[4])) == sincronizado
	n := *(*uint32)(unsafe.Pointer(&buf[24]))
	if n > 4096 {
		n = 0
	}
	return string(buf[28 : 28+n]), enSync, nil
}

// identidadDeRuta es la identidad de Drive de una carpeta o un fichero del disco; la
// carpeta de Drive misma es `raiz`.
func identidadDeRuta(ruta string) (string, error) {
	muConexion.Lock()
	raiz0 := raizDir
	muConexion.Unlock()
	if filepath.Clean(ruta) == filepath.Clean(raiz0) {
		return raiz, nil
	}
	h, err := abrir(ruta, windows.FILE_READ_ATTRIBUTES)
	if err != nil {
		return "", err
	}
	defer func() { _ = windows.CloseHandle(h) }()
	identidad, _, err := marcadorDe(h)
	if err == nil && identidad == "" {
		err = errors.New("nube: un marcador sin identidad: " + ruta)
	}
	return identidad, err
}

// versionSiHaceFalta atiende un marcador que Windows ya no da por sincronizado: lo sube
// si cambió en el disco, o no hace nada (un renombre), o baja lo de Drive, o —si
// cambiaron los dos— sube lo del disco APARTE y baja lo de Drive (`decidirSubida`).
func versionSiHaceFalta(ctx context.Context, ruta string) {
	h, err := abrir(ruta, windows.FILE_READ_ATTRIBUTES|windows.WRITE_DAC)
	if err != nil {
		return
	}
	defer func() { _ = windows.CloseHandle(h) }()
	identidad, enSync, err := marcadorDe(h)
	if err != nil || enSync || identidad == "" {
		return
	}
	id, err := ficheroDe(identidad)
	if err != nil {
		return // una carpeta «no sincronizada» es solo un atributo cambiado
	}
	var info windows.ByHandleFileInformation
	if err := windows.GetFileInformationByHandle(h, &info); err != nil {
		return
	}
	fecha := int64(info.LastWriteTime.HighDateTime)<<32 | int64(info.LastWriteTime.LowDateTime)
	largo := int64(info.FileSizeHigh)<<32 | int64(info.FileSizeLow)
	err = enDrive(ctx, func(ctx context.Context, f Fuente) error {
		x, err := f.File(ctx, id)
		if err != nil {
			return err
		}
		switch decidirSubida(identidad, x, fecha, largo) {
		case SoloMarcar:
			actualizar(h, ruta, identidad, nil, actualizarEnSync)
		case Subir:
			if x, err = subirDesde(ctx, f, ruta, func(c *os.File, n int64) (api.File, error) {
				return subirVersion(ctx, f, identidad, mime.TypeByExtension(filepath.Ext(ruta)), c, n)
			}); err != nil {
				return err
			}
			sincronizar(h, ruta, x)
		case Conflicto:
			if _, err := subirDesde(ctx, f, ruta, func(c *os.File, n int64) (api.File, error) {
				return f.Upload(ctx, x.FolderID, nombreConflicto(x.Name), mime.TypeByExtension(filepath.Ext(ruta)), c, n)
			}); err != nil {
				return err
			}
			slog.Warn("nube: cambió en el disco y en Drive; lo del disco sube aparte", "ruta", ruta, "como", nombreConflicto(x.Name))
			tomarDrive(h, ruta, x)
		case TomarDrive:
			tomarDrive(h, ruta, x)
		}
		return nil
	})
	if err != nil {
		slog.Warn("nube: no se ha podido subir lo guardado en un fichero de Drive", "ruta", ruta, "error", err)
	}
}

// subirDesde abre el fichero del disco y lo pasa a `subir` con su tamaño.
func subirDesde(_ context.Context, _ Fuente, ruta string, subir func(*os.File, int64) (api.File, error)) (api.File, error) {
	c, err := os.Open(ruta) //nolint:gosec // G304: la ruta es de la carpeta de Drive
	if err != nil {
		return api.File{}, err
	}
	defer func() { _ = c.Close() }()
	st, err := c.Stat()
	if err != nil {
		return api.File{}, err
	}
	return subir(c, st.Size())
}

// sincronizar marca el marcador como sincronizado con la versión que Drive acaba de
// guardar: su fecha y su tamaño, y la identidad con esa versión.
func sincronizar(h windows.Handle, ruta string, x api.File) {
	t := hora(x.UpdatedAt)
	actualizar(h, ruta, IdentidadFichero(x), &metadatos{escrito: t, cambiado: t, tamano: x.SizeBytes}, actualizarEnSync)
}

// tomarDrive vacía el marcador con la versión de Drive; se baja al abrirlo, o en el acto
// si está fijado.
func tomarDrive(h windows.Handle, ruta string, x api.File) {
	t := hora(x.UpdatedAt)
	actualizar(h, ruta, IdentidadFichero(x), &metadatos{escrito: t, cambiado: t, tamano: x.SizeBytes}, actualizarTomar)
	go aplicar(ruta)
}

// actualizar es CfUpdatePlaceholder: metadatos (nil, no cambiar) e identidad nuevos.
func actualizar(h windows.Handle, ruta, identidad string, m *metadatos, banderas uintptr) bool {
	// La raíz, registrada por el Explorador, no tiene identidad: sin ella se pasa NULL y
	// 0, que es «no cambiarla». Con &id[0] el Hub se caía al marcar la raíz como
	// sincronizada (visto en Windows Server 2022).
	var p *byte
	if id := []byte(identidad); len(id) > 0 {
		p = &id[0]
	}
	r, _, _ := procActualizar.Call(uintptr(h), uintptr(unsafe.Pointer(m)), uintptr(unsafe.Pointer(p)), uintptr(len(identidad)),
		0, 0, banderas, 0, 0)
	if e := hr(r, "CfUpdatePlaceholder"); e != nil {
		slog.Info("nube: Windows no deja actualizar un marcador", "ruta", ruta, "error", e)
		return false
	}
	return true
}

// errYaEsDeDrive es que, mientras se esperaba a poder abrirlo, ya se convirtió.
var errYaEsDeDrive = errors.New("nube: ya es de Drive")

// enCurso son las rutas que `nuevo` está subiendo ahora.
var enCurso sync.Map

// esperaSoltar es lo que se espera, como mucho, a que quien escribe un fichero nuevo lo
// cierre: una copia grande desde otra unidad tarda.
const esperaSoltar = 30 * time.Minute

// nuevo sube lo que ha aparecido en la carpeta y no era de Drive: un fichero cuando
// quien lo escribe lo suelta, una carpeta en el acto. Después lo convierte en marcador
// sincronizado. Lo que ya es un marcador (algo renombrado) no se toca.
func nuevo(ctx context.Context, ruta string) {
	if temporal(filepath.Base(ruta)) {
		return
	}
	// Una carpeta copiada entera avisa de cada hijo Y los recorre al convertirse: sin
	// esto cada fichero subía dos veces (medido: la segunda conversión fallaba con
	// 0x8007017c).
	if _, ya := enCurso.LoadOrStore(ruta, true); ya {
		return
	}
	defer enCurso.Delete(ruta)
	if _, err := identidadDeRuta(ruta); err == nil {
		return // ya es de Drive
	}
	info, err := os.Lstat(ruta)
	if err != nil || info.Mode()&os.ModeSymlink != 0 {
		return
	}
	// La carpeta de arriba puede ser nueva también, y convertirse un poco después.
	var padre string
	for i := 0; i < 30 && ctx.Err() == nil; i++ {
		if padre, err = identidadDeRuta(filepath.Dir(ruta)); err == nil {
			break
		}
		time.Sleep(time.Second)
	}
	if err != nil {
		slog.Warn("nube: lo nuevo está en una carpeta que no es de Drive", "ruta", ruta, "error", err)
		return
	}
	var (
		identidad string
		x         api.File
	)
	if info.IsDir() {
		err = enDrive(ctx, func(c context.Context, f Fuente) error {
			identidad, err = crearCarpeta(c, f, padre, filepath.Base(ruta))
			return err
		})
	} else {
		x, err = subirCuandoSuelte(ctx, ruta, padre)
		identidad = IdentidadFichero(x)
	}
	if errors.Is(err, errYaEsDeDrive) {
		return
	}
	if err != nil {
		slog.Warn("nube: no se ha podido subir algo nuevo a Drive", "ruta", ruta, "error", err)
		return
	}
	convertir(ruta, identidad, x)
	if info.IsDir() {
		// Lo que ya venía dentro (una carpeta copiada entera) no avisa otra vez.
		hijos, _ := os.ReadDir(ruta)
		for _, h := range hijos {
			nuevo(ctx, filepath.Join(ruta, h.Name()))
		}
	}
}

// subirCuandoSuelte espera a poder abrir el fichero SIN compartir la escritura —es
// decir, a que nadie lo esté escribiendo— y lo sube con ese manejador abierto, para que
// nadie lo cambie a medias.
func subirCuandoSuelte(ctx context.Context, ruta, padre string) (api.File, error) {
	p, err := windows.UTF16PtrFromString(ruta)
	if err != nil {
		return api.File{}, err
	}
	fin := time.Now().Add(esperaSoltar)
	for {
		h, err := windows.CreateFile(p, windows.GENERIC_READ, windows.FILE_SHARE_READ, nil, windows.OPEN_EXISTING, 0, 0)
		if err == nil {
			c := os.NewFile(uintptr(h), ruta)
			if _, err := identidadDeRuta(ruta); err == nil {
				_ = c.Close()
				return api.File{}, errYaEsDeDrive
			}
			return subirAbierto(ctx, c, padre)
		}
		if !errors.Is(err, windows.ERROR_SHARING_VIOLATION) || time.Now().After(fin) || ctx.Err() != nil {
			return api.File{}, err
		}
		time.Sleep(2 * time.Second)
	}
}

// subirAbierto sube el fichero ya abierto sin compartir la escritura, y lo cierra.
func subirAbierto(ctx context.Context, c *os.File, padre string) (api.File, error) {
	defer func() { _ = c.Close() }()
	info, err := c.Stat()
	if err != nil {
		return api.File{}, err
	}
	var x api.File
	err = enDrive(ctx, func(cx context.Context, f Fuente) error {
		x, err = subirNuevo(cx, f, padre, filepath.Base(c.Name()), mime.TypeByExtension(filepath.Ext(c.Name())), c, info.Size())
		return err
	})
	return x, err
}

// convertir hace de un fichero o una carpeta del disco un marcador sincronizado con esa
// identidad: a partir de ahí es como lo que vino de Drive.
//
// ponytail: si alguien lo reescribe entre la subida y la conversión, queda marcado como
// sincronizado con lo de antes; el siguiente guardado lo arregla.
func convertir(ruta, identidad string, x api.File) {
	h, err := abrir(ruta, windows.FILE_READ_ATTRIBUTES|windows.WRITE_DAC)
	if err != nil {
		slog.Warn("nube: subido, pero no se puede abrir para marcarlo", "ruta", ruta, "error", err)
		return
	}
	defer func() { _ = windows.CloseHandle(h) }()
	id := []byte(identidad)
	r, _, _ := procConvertir.Call(uintptr(h), uintptr(unsafe.Pointer(&id[0])), uintptr(len(id)), convertirEnSync, 0, 0)
	if e := hr(r, "CfConvertToPlaceholder"); e != nil {
		slog.Warn("nube: subido, pero Windows no lo convierte en marcador", "ruta", ruta, "error", e)
		return
	}
	if x.ID != "" {
		sincronizar(h, ruta, x)
	}
}
