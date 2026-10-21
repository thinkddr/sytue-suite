// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

//go:build windows

package nube

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"sync"
	"syscall"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
)

// # El puente a cldapi.dll
//
// Sin cgo, con `syscall`: las estructuras de `cfapi.h` se reproducen campo a campo con
// su relleno de x64, y su tamaño lo comprueba el guardián en Windows
// (nube_windows_test.go). Windows llama a las dos retrollamadas desde su propio grupo de
// hilos; cada una copia lo que necesita y hace el trabajo en una rutina, como pide la
// documentación, que no quiere el hilo de la retrollamada bloqueado en la red.

var (
	cldapi          = syscall.NewLazyDLL("cldapi.dll")
	procRegister    = cldapi.NewProc("CfRegisterSyncRoot")
	procUnregister  = cldapi.NewProc("CfUnregisterSyncRoot")
	procConnect     = cldapi.NewProc("CfConnectSyncRoot")
	procDisconnect  = cldapi.NewProc("CfDisconnectSyncRoot")
	procExecute     = cldapi.NewProc("CfExecute")
	procInfoPorRuta = cldapi.NewProc("CfGetSyncRootInfoByPath")
	procHidratar    = cldapi.NewProc("CfHydratePlaceholder")
	procDeshidratar = cldapi.NewProc("CfDehydratePlaceholder")
)

// Constantes de cfapi.h.
const (
	hidratacionCompleta = 2 // CF_HYDRATION_POLICY_FULL
	poblacionCompleta   = 2 // CF_POPULATION_POLICY_FULL: las carpetas se piden al abrirlas
	// CF_HYDRATION_POLICY_MODIFIER_AUTO_DEHYDRATION_ALLOWED: «Liberar espacio» del
	// Explorador vacía el fichero sin pasar por aquí.
	deshidratarSolo = 0x4

	llamadaBajar        = 0          // CF_CALLBACK_TYPE_FETCH_DATA
	llamadaMarcadores   = 3          // CF_CALLBACK_TYPE_FETCH_PLACEHOLDERS
	llamadaFin          = 0xffffffff // CF_CALLBACK_TYPE_NONE
	operacionDatos      = 0          // CF_OPERATION_TYPE_TRANSFER_DATA
	operacionMarcadores = 4          // CF_OPERATION_TYPE_TRANSFER_PLACEHOLDERS
	sinPoblarDespues    = 0x1        // CF_PLACEHOLDER_CREATE_FLAG_DISABLE_ON_DEMAND_POPULATION
	// CF_OPERATION_TRANSFER_PLACEHOLDERS_FLAG_DISABLE_ON_DEMAND_POPULATION: la carpeta
	// queda poblada. Es 0x2 y NO 0x1 (STOP_ON_ERROR): con 0x1 Windows volvía a pedir la
	// carpeta en cada acceso y crear algo dentro fallaba con «The name of the file cannot
	// be resolved by the system» (medido en Windows Server 2022).
	carpetaPoblada       = 0x2
	marcadorSincronizado = 0x2 // CF_PLACEHOLDER_CREATE_FLAG_MARK_IN_SYNC

	atributoCarpeta = 0x10 // FILE_ATTRIBUTE_DIRECTORY

	fallo = -0x3fffffff // STATUS_UNSUCCESSFUL (0xC0000001)

	bloque = 4096    // lo que se transfiere va alineado a 4 KiB, salvo el final
	trozo  = 1 << 20 // de un mega en un mega
)

// proveedorID es el GUID de Sytue Drive como proveedor de sincronización: fijo, para que
// Windows reconozca la misma carpeta entre versiones.
var proveedorID = syscall.GUID{Data1: 0x5d1e0a17, Data2: 0x7c3b, Data3: 0x4e8a, Data4: [8]byte{0x9f, 0x21, 0x5a, 0x0d, 0xe4, 0x6b, 0x13, 0x77}}

// CF_SYNC_REGISTRATION.
type registro struct {
	tamano          uint32
	_               uint32
	nombre          *uint16
	version         *uint16
	identidadRaiz   *byte
	largoRaiz       uint32
	_               uint32
	identidadCarpet *byte
	largoCarpeta    uint32
	proveedor       syscall.GUID
}

// CF_SYNC_POLICIES.
type politicas struct {
	tamano                      uint32
	hidratacion, hidratacionMod uint16
	poblacion, poblacionMod     uint16
	_, _, _                     uint32 // InSync, HardLink y PlaceholderManagement: por omisión
}

// CF_CALLBACK_REGISTRATION.
type llamada struct {
	tipo    uint32
	_       uint32
	funcion uintptr
}

// CF_CALLBACK_INFO (lo que se lee de él).
type infoLlamada struct {
	_              [8]byte  // StructSize
	conexion       int64    // ConnectionKey
	_              [16]byte // CallbackContext y VolumeGuidName
	volumen        *uint16  // VolumeDosName: «C:»
	_              [16]byte // VolumeSerialNumber y SyncRootFileId
	_              [16]byte // SyncRootIdentity y su largo
	_              int64    // FileId
	tamanoFichero  int64    // FileSize
	identidad      *byte    // FileIdentity
	largoIdentidad uint32
	_              uint32
	ruta           *uint16  // NormalizedPath: desde la raíz del volumen, sin la letra
	transferencia  int64    // TransferKey
	_              [24]byte // PriorityHint, CorrelationVector, ProcessInfo
	peticion       int64    // RequestKey
}

// CF_CALLBACK_PARAMETERS de FETCH_DATA.
type paramsBajar struct {
	_             [16]byte // ParamSize, relleno, Flags y relleno
	desde, cuanto int64
	_, _          int64 // OptionalFileOffset y OptionalLength
}

// CF_OPERATION_INFO.
type operacion struct {
	tamano        uint32
	tipo          uint32
	conexion      int64
	transferencia int64
	_, _          uintptr // CorrelationVector y SyncStatus
	peticion      int64
}

// CF_OPERATION_PARAMETERS de TRANSFER_DATA.
type opDatos struct {
	tamano uint32
	_      uint32
	_      uint32 // Flags
	estado int32
	datos  *byte
	desde  int64
	largo  int64
}

// CF_OPERATION_PARAMETERS de TRANSFER_PLACEHOLDERS.
type opMarcadores struct {
	tamano     uint32
	_          uint32
	banderas   uint32
	estado     int32
	total      int64
	marcadores *marcador
	cuantos    uint32
	_          uint32 // EntriesProcessed
}

// CF_PLACEHOLDER_CREATE_INFO, con su CF_FS_METADATA (FILE_BASIC_INFO + tamaño).
type marcador struct {
	nombre                              *uint16
	creado, accedido, escrito, cambiado int64
	atributos                           uint32
	_                                   uint32
	tamano                              int64
	identidad                           *byte
	largoIdentidad                      uint32
	banderas                            uint32
	_                                   [16]byte // Result, relleno y CreateUsn
}

// hr pasa un HRESULT a error.
func hr(r uintptr, que string) error {
	if int32(r) >= 0 { //nolint:gosec // G115: un HRESULT es un entero de 32 bits con signo
		return nil
	}
	return fmt.Errorf("nube: %s: HRESULT 0x%08x", que, uint32(r)) //nolint:gosec // G115: ídem
}

// La conexión que atienden las dos retrollamadas: hay una por proceso (la del Hub).
var (
	muConexion sync.Mutex
	fuente     Fuente
	contexto   context.Context
	raizDir    string
	llamadas   = []llamada{
		{tipo: llamadaBajar, funcion: syscall.NewCallback(alBajar)},
		{tipo: llamadaMarcadores, funcion: syscall.NewCallback(alPoblar)},
		{tipo: llamadaCerrado, funcion: syscall.NewCallback(alCerrar)},
		{tipo: llamadaBorrar, funcion: syscall.NewCallback(alBorrar)},
		{tipo: llamadaRenombrar, funcion: syscall.NewCallback(alRenombrar)},
		{tipo: llamadaFin},
	}
)

func registrada(dir string) bool {
	ruta, err := syscall.UTF16PtrFromString(dir)
	if err != nil {
		return false
	}
	var info [8]byte // CF_SYNC_ROOT_BASIC_INFO
	var largo uint32
	r, _, _ := procInfoPorRuta.Call(uintptr(unsafe.Pointer(ruta)), 0, uintptr(unsafe.Pointer(&info[0])),
		uintptr(len(info)), uintptr(unsafe.Pointer(&largo)))
	return hr(r, "") == nil
}

func registrar(dir string) error {
	nombre, _ := syscall.UTF16PtrFromString(filepath.Base(dir))
	version, _ := syscall.UTF16PtrFromString("1")
	id := []byte(raiz)
	const largo = uint32(len(raiz))
	reg := registro{
		nombre: nombre, version: version,
		identidadRaiz: &id[0], largoRaiz: largo,
		identidadCarpet: &id[0], largoCarpeta: largo,
		proveedor: proveedorID,
	}
	reg.tamano = uint32(unsafe.Sizeof(reg))
	pol := politicas{hidratacion: hidratacionCompleta, hidratacionMod: deshidratarSolo, poblacion: poblacionCompleta}
	pol.tamano = uint32(unsafe.Sizeof(pol))
	ruta, err := syscall.UTF16PtrFromString(dir)
	if err != nil {
		return err
	}
	r, _, _ := procRegister.Call(uintptr(unsafe.Pointer(ruta)), uintptr(unsafe.Pointer(&reg)), uintptr(unsafe.Pointer(&pol)), 0)
	runtime.KeepAlive(id)
	return hr(r, "CfRegisterSyncRoot")
}

func desconectar(dir string) error {
	if enShell(context.Background(), dir, true) == nil && !registrada(dir) {
		return nil
	}
	ruta, err := syscall.UTF16PtrFromString(dir)
	if err != nil {
		return err
	}
	r, _, _ := procUnregister.Call(uintptr(unsafe.Pointer(ruta)))
	return hr(r, "CfUnregisterSyncRoot")
}

func conectar(ctx context.Context, dir string, f Fuente) error {
	if err := procConnect.Find(); err != nil {
		return ErrNoDisponible // Windows anterior a 10 1709
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	// Con el Explorador si se puede (`enShell`); si no, como siempre.
	if err := enShell(ctx, dir, false); err != nil {
		slog.Info("nube: la carpeta no se ha podido registrar en el Explorador", "error", err)
		if !registrada(dir) {
			if err := registrar(dir); err != nil {
				return err
			}
		}
	}
	muConexion.Lock()
	if fuente != nil {
		muConexion.Unlock()
		return errors.New("nube: Drive ya está conectado en este proceso")
	}
	fuente, contexto, raizDir = f, ctx, dir
	muConexion.Unlock()
	defer func() {
		muConexion.Lock()
		fuente, contexto, raizDir = nil, nil, ""
		muConexion.Unlock()
	}()
	ruta, err := syscall.UTF16PtrFromString(dir)
	if err != nil {
		return err
	}
	var clave int64
	r, _, _ := procConnect.Call(uintptr(unsafe.Pointer(ruta)), uintptr(unsafe.Pointer(&llamadas[0])), 0, 0,
		uintptr(unsafe.Pointer(&clave)))
	if err := hr(r, "CfConnectSyncRoot"); err != nil {
		return err
	}
	go vigilar(ctx, dir)
	go repasar(ctx, dir)
	go refrescar(ctx, dir)
	<-ctx.Done()
	r, _, _ = procDisconnect.Call(uintptr(clave)) //nolint:gosec // G115: CF_CONNECTION_KEY es un LARGE_INTEGER que Windows da y recibe tal cual
	return errors.Join(ctx.Err(), hr(r, "CfDisconnectSyncRoot"))
}

// actual es la fuente de la conexión viva, o nil.
func actual() (Fuente, context.Context) {
	muConexion.Lock()
	defer muConexion.Unlock()
	return fuente, contexto
}

// identidadDe copia la identidad que Windows guarda con cada marcador.
func identidadDe(p *byte, n uint32) string {
	if p == nil || n == 0 || n > 4096 {
		return ""
	}
	return string(unsafe.Slice(p, n))
}

// claves es lo que una operación necesita de su retrollamada.
type claves struct{ conexion, transferencia, peticion int64 }

func (c claves) operacion(tipo uint32) operacion {
	o := operacion{tipo: tipo, conexion: c.conexion, transferencia: c.transferencia, peticion: c.peticion}
	o.tamano = uint32(unsafe.Sizeof(o))
	return o
}

// alPoblar es FETCH_PLACEHOLDERS: alguien abre una carpeta que todavía no se ha pedido.
func alPoblar(info *infoLlamada, _ uintptr) uintptr {
	c := claves{info.conexion, info.transferencia, info.peticion}
	identidad := identidadDe(info.identidad, info.largoIdentidad)
	if identidad == "" {
		// La raíz registrada para el Explorador no lleva identidad de fichero (`enShell`).
		identidad = raiz
	}
	carpeta := windows.UTF16PtrToString(info.volumen) + windows.UTF16PtrToString(info.ruta)
	go poblar(c, identidad, carpeta)
	return 0
}

func poblar(c claves, identidad, carpeta string) {
	f, ctx := actual()
	var lista []Entrada
	err := errors.New("nube: sin conexión con Drive")
	if f != nil {
		lista, _, err = listar(ctx, f, identidad)
	}
	// Lo que ya está en el disco no se vuelve a crear: Windows corta el lote en el primer
	// «ya existe» y la carpeta se queda sin abrir (medido en Windows Server 2022). Pasa al
	// volver a conectar una carpeta que ya se había abierto.
	lista = slices.DeleteFunc(lista, func(e Entrada) bool {
		_, err := os.Lstat(filepath.Join(carpeta, e.Nombre))
		return err == nil
	})
	marcas, vivos := marcadores(lista)
	op := c.operacion(operacionMarcadores)
	p := opMarcadores{banderas: carpetaPoblada, total: int64(len(marcas)), cuantos: uint32(len(marcas))} //nolint:gosec // G115: acotado por maxEntradas
	if len(marcas) > 0 {
		p.marcadores = &marcas[0]
	}
	if err != nil {
		p = opMarcadores{estado: fallo}
		slog.Warn("nube: no se ha podido listar una carpeta de Drive", "carpeta", identidad, "error", err)
	}
	p.tamano = uint32(unsafe.Sizeof(p))
	r, _, _ := procExecute.Call(uintptr(unsafe.Pointer(&op)), uintptr(unsafe.Pointer(&p)))
	keepAlive(vivos, marcas)
	if e := hr(r, "CfExecute(TRANSFER_PLACEHOLDERS)"); e != nil {
		slog.Warn("nube: Windows no ha aceptado los marcadores", "carpeta", identidad, "error", e)
		return
	}
	// Los marcadores nuevos de una carpeta FIJADA heredan el atributo pero nadie los baja:
	// crearlos no avisa a `vigilar` (medido fijando una carpeta sin abrir).
	if err == nil && len(marcas) > 0 {
		go aplicar(carpeta)
	}
	// Y la carpeta, sincronizada: poblarla (o fijar algo dentro) le quita la marca, y el
	// Explorador la enseñaba con las flechas de «sincronizando» para siempre (medido).
	if err == nil {
		go marcarSincronizado(carpeta)
	}
}

// marcadores arma el CF_PLACEHOLDER_CREATE_INFO de cada entrada. `vivos` guarda los
// nombres y las identidades a los que apuntan, hasta que Windows los haya leído.
func marcadores(lista []Entrada) ([]marcador, []any) {
	marcas := make([]marcador, 0, len(lista))
	var vivos []any
	for _, e := range lista {
		nombre, err := syscall.UTF16FromString(e.Nombre)
		if err != nil {
			continue
		}
		id := []byte(e.Identidad)
		vivos = append(vivos, id, nombre)
		t := horaMarcador(e.Modificado)
		m := marcador{
			nombre: &nombre[0], creado: t, accedido: t, escrito: t, cambiado: t,
			identidad: &id[0], largoIdentidad: uint32(len(id)), banderas: marcadorSincronizado, //nolint:gosec // G115: una identidad son unas decenas de bytes
		}
		if e.Carpeta {
			m.atributos = atributoCarpeta
		} else {
			m.tamano = e.Tamano
			m.banderas |= sinPoblarDespues // un fichero no tiene nada que poblar
		}
		marcas = append(marcas, m)
	}
	return marcas, vivos
}

// horaMarcador es la fecha de un marcador nuevo: la de Drive, o ahora si no la trae.
func horaMarcador(t time.Time) int64 {
	if t.IsZero() {
		t = time.Now()
	}
	return hora(t)
}

// alBajar es FETCH_DATA: alguien abre un fichero que todavía es solo un marcador.
func alBajar(info *infoLlamada, p *paramsBajar) uintptr {
	c := claves{info.conexion, info.transferencia, info.peticion}
	go bajar(c, identidadDe(info.identidad, info.largoIdentidad), info.tamanoFichero, p.desde, p.cuanto)
	return 0
}

func bajar(c claves, identidad string, tamano, desde, cuanto int64) {
	inicio := desde &^ (bloque - 1)
	err := errors.New("nube: sin conexión con Drive")
	if f, ctx := actual(); f != nil {
		err = transferir(ctx, f, c, identidad, tamano, inicio)
	}
	if err != nil {
		slog.Warn("nube: no se ha podido bajar un fichero de Drive", "fichero", identidad, "error", err)
		// El fallo, sobre el tramo pedido, para que quien abría el fichero se entere.
		op := c.operacion(operacionDatos)
		p := opDatos{estado: fallo, desde: inicio, largo: desde + cuanto - inicio}
		p.tamano = uint32(unsafe.Sizeof(p))
		_, _, _ = procExecute.Call(uintptr(unsafe.Pointer(&op)), uintptr(unsafe.Pointer(&p)))
	}
}

// transferir baja el fichero y se lo da a Windows desde `inicio` hasta el final, de un
// mega en un mega: la hidratación es completa (CF_HYDRATION_POLICY_FULL).
func transferir(ctx context.Context, f Fuente, c claves, identidad string, tamano, inicio int64) error {
	id, err := ficheroDe(identidad)
	if err != nil {
		return err
	}
	cuerpo, err := f.Download(ctx, id)
	if err != nil {
		return err
	}
	defer func() { _ = cuerpo.Close() }()
	if _, err := io.CopyN(io.Discard, cuerpo, inicio); err != nil {
		return err
	}
	buf := make([]byte, trozo)
	for desde := inicio; desde < tamano; {
		n, err := io.ReadFull(cuerpo, buf[:min(trozo, tamano-desde)])
		if err != nil {
			return fmt.Errorf("el contenido acaba antes que el marcador (%d de %d): %w", desde+int64(n), tamano, err)
		}
		op := c.operacion(operacionDatos)
		p := opDatos{datos: &buf[0], desde: desde, largo: int64(n)}
		p.tamano = uint32(unsafe.Sizeof(p))
		r, _, _ := procExecute.Call(uintptr(unsafe.Pointer(&op)), uintptr(unsafe.Pointer(&p)))
		if err := hr(r, "CfExecute(TRANSFER_DATA)"); err != nil {
			return err
		}
		desde += int64(n)
	}
	return nil
}

// Los atributos de un marcador que dicen dónde está su contenido.
const (
	atributoFijado    = 0x80000   // FILE_ATTRIBUTE_PINNED: «Mantener siempre en este dispositivo»
	atributoSoltado   = 0x100000  // FILE_ATTRIBUTE_UNPINNED: «Liberar espacio»
	atributoSinBajar  = 0x400000  // FILE_ATTRIBUTE_RECALL_ON_DATA_ACCESS: solo el marcador
	cambioDeAtributos = 0x4       // FILE_NOTIFY_CHANGE_ATTRIBUTES
	cambioDeNombres   = 0x1 | 0x2 // FILE_NOTIFY_CHANGE_FILE_NAME | …_DIR_NAME

	accionCreado     = 1 // FILE_ACTION_ADDED
	accionAtributos  = 3 // FILE_ACTION_MODIFIED
	accionRenombrado = 5 // FILE_ACTION_RENAMED_NEW_NAME
)

// vigilar cumple lo que la persona pide desde el Explorador: lo FIJADO se baja entero
// —es la caché sin conexión— y lo SOLTADO se vacía. Windows no lo hace solo: solo cambia
// los atributos y espera a que el proveedor actúe (medido: «attrib +P» deja el fichero
// fijado y sin bajar). Y lo que aparece nuevo en la carpeta sube a Drive (`nuevo`).
func vigilar(ctx context.Context, dir string) {
	ruta, err := windows.UTF16PtrFromString(dir)
	if err != nil {
		return
	}
	h, err := windows.CreateFile(ruta, windows.FILE_LIST_DIRECTORY,
		windows.FILE_SHARE_READ|windows.FILE_SHARE_WRITE|windows.FILE_SHARE_DELETE, nil,
		windows.OPEN_EXISTING, windows.FILE_FLAG_BACKUP_SEMANTICS, 0)
	if err != nil {
		slog.Warn("nube: no se puede vigilar la carpeta de Drive", "error", err)
		return
	}
	go func() {
		<-ctx.Done()
		_ = windows.CancelIoEx(h, nil)
	}()
	defer func() { _ = windows.CloseHandle(h) }()
	const tamanoBuf = 64 << 10
	buf := make([]byte, tamanoBuf)
	for ctx.Err() == nil {
		var n uint32
		if err := windows.ReadDirectoryChanges(h, &buf[0], tamanoBuf, true, cambioDeAtributos|cambioDeNombres, &n, nil, 0); err != nil {
			if ctx.Err() == nil {
				slog.Warn("nube: la vigilancia de la carpeta de Drive se ha cortado", "error", err)
			}
			return
		}
		for _, c := range cambiados(buf[:n]) {
			ruta := filepath.Join(dir, c.ruta)
			switch c.accion {
			case accionAtributos:
				go aplicar(ruta)
			case accionCreado, accionRenombrado:
				go nuevo(ctx, ruta)
			}
		}
	}
}

// cambio es un FILE_NOTIFY_INFORMATION: qué ha pasado y dónde, relativo a la carpeta.
type cambio struct {
	accion uint32
	ruta   string
}

// cambiados lee los FILE_NOTIFY_INFORMATION de un lote.
func cambiados(b []byte) []cambio {
	var out []cambio
	for off := 0; off+12 <= len(b); {
		siguiente := int(binary.LittleEndian.Uint32(b[off:]))
		largo := int(binary.LittleEndian.Uint32(b[off+8:]))
		if off+12+largo > len(b) {
			break
		}
		nombre := make([]uint16, largo/2)
		for i := range nombre {
			nombre[i] = binary.LittleEndian.Uint16(b[off+12+2*i:])
		}
		out = append(out, cambio{binary.LittleEndian.Uint32(b[off+4:]), windows.UTF16ToString(nombre)})
		if siguiente == 0 {
			break
		}
		off += siguiente
	}
	return out
}

// aplicar baja o vacía un fichero según lo que diga su atributo. Una carpeta fijada o
// soltada lo aplica a lo que ya tiene dentro.
func aplicar(ruta string) {
	p, err := windows.UTF16PtrFromString(ruta)
	if err != nil {
		return
	}
	a, err := windows.GetFileAttributes(p)
	if err != nil {
		return
	}
	if a&windows.FILE_ATTRIBUTE_DIRECTORY != 0 {
		if a&(atributoFijado|atributoSoltado) == 0 {
			return
		}
		hijos, _ := os.ReadDir(ruta)
		for _, h := range hijos {
			aplicar(filepath.Join(ruta, h.Name()))
		}
		marcarSincronizado(ruta)
		return
	}
	var proc *syscall.LazyProc
	var acceso uint32
	switch {
	case a&atributoFijado != 0 && a&atributoSinBajar != 0:
		proc, acceso = procHidratar, windows.FILE_READ_DATA
	case a&atributoSoltado != 0 && a&atributoSinBajar == 0:
		// WRITE_DAC y no WRITE_DATA: abrir para escribir contaría como cambio.
		proc, acceso = procDeshidratar, windows.WRITE_DAC
	default:
		return
	}
	h, err := windows.CreateFile(p, acceso, windows.FILE_SHARE_READ|windows.FILE_SHARE_WRITE|windows.FILE_SHARE_DELETE,
		nil, windows.OPEN_EXISTING, 0, 0)
	if err != nil {
		slog.Warn("nube: no se puede abrir un fichero de Drive", "ruta", ruta, "error", err)
		return
	}
	defer func() { _ = windows.CloseHandle(h) }()
	// Length = -1 es «hasta el final»: el LARGE_INTEGER con todos los bits a uno.
	r, _, _ := proc.Call(uintptr(h), 0, ^uintptr(0), 0, 0)
	if err := hr(r, proc.Name); err != nil {
		slog.Warn("nube: Windows no ha cambiado el contenido de un fichero de Drive", "ruta", ruta, "error", err)
	}
}

// repasar aplica al conectar lo que se fijó o se soltó con el Hub cerrado. Solo baja por
// las carpetas ya pobladas: entrar en las demás las pediría todas a Drive.
func repasar(ctx context.Context, dir string) {
	// Una entrada que no se deja leer no para el repaso de las demás: por eso cada error
	// se salta, y el paseo solo se corta en las carpetas sin poblar.
	_ = filepath.WalkDir(dir, func(ruta string, d fs.DirEntry, err error) error {
		if err == nil && ruta != dir {
			err = repasarUna(ctx, ruta, d)
		}
		if errors.Is(err, filepath.SkipDir) {
			return err
		}
		return nil
	})
}

// repasarUna aplica un fichero, o dice SkipDir en una carpeta todavía sin poblar.
func repasarUna(ctx context.Context, ruta string, d fs.DirEntry) error {
	info, err := d.Info()
	if err != nil {
		return err
	}
	a, _ := info.Sys().(*syscall.Win32FileAttributeData)
	if d.IsDir() {
		if a != nil && a.FileAttributes&atributoSinBajar != 0 {
			return filepath.SkipDir
		}
		nuevo(ctx, ruta) // una carpeta hecha sin conexión
		return nil
	}
	aplicar(ruta)
	// Lo escrito con el Hub cerrado: lo nuevo sube, y lo cambiado, como versión.
	if a != nil && a.FileAttributes&atributoSinBajar == 0 {
		nuevo(ctx, ruta)
		versionSiHaceFalta(ctx, ruta)
	}
	return nil
}

// keepAlive mantiene vivo lo que Windows lee por puntero hasta después de la llamada.
func keepAlive(vivos []any, marcas []marcador) {
	runtime.KeepAlive(vivos)
	runtime.KeepAlive(marcas)
}
