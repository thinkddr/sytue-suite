// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

//go:build linux && !android

package portapapeles

import (
	"bytes"
	"cmp"
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"math"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"syscall"
	"time"
)

// # El portapapeles de Wayland, leído directamente
//
// Sin X11 (una sesión Wayland pura, o la app con `-tags wayland`), se habla con el
// compositor por su socket con el protocolo de siempre. Es poco: el registro, un `wl_seat`
// y el protocolo de CONTROL DE DATOS —`ext_data_control_v1` (el estándar, wayland-
// protocols 1.39) o su antecesor `zwlr_data_control_v1` (wlroots, KDE)—, que es el que deja
// leer la selección sin tener el foco: el `wl_data_device` del núcleo solo se la da a la
// ventana que lo tiene, y esta conexión no tiene ninguna.
//
// El protocolo va escrito aquí en Go, sin libwayland-client: son una docena de mensajes, y
// así no hay cgo, ni una biblioteca más que exigir en el equipo, ni una dependencia nueva.
//
// ponytail: GNOME (mutter) no tiene control de datos. Ahí, y donde el compositor no lo
// ofrezca, se usa `wl-paste` si está instalado, que para leer sin foco abre una ventana un
// instante. Con XWayland —lo normal para una app de Fyne— no hace falta nada de esto.

const esperaWayland = 2 * time.Second

// Las interfaces de control de datos, en el orden en que se prefieren. Las dos tienen los
// mismos códigos de mensaje.
var interfacesControl = []string{"ext_data_control_manager_v1", "zwlr_data_control_manager_v1"}

// conexionWL es la conexión con el compositor (ver «El protocolo, lo justo», abajo).
type conexionWL struct {
	c        *net.UnixConn
	sig      uint32 // el último id; el 1 es wl_display
	registro uint32
	buf      []byte
	fds      []int // los recibidos, en orden
	err      error // el primer fallo al escribir
}

func leerWayland() ([]byte, error) {
	if os.Getenv("WAYLAND_DISPLAY") == "" {
		return nil, ErrNoDisponible
	}
	datos, err := leerControlDeDatos("text/html")
	if errors.Is(err, ErrNoDisponible) {
		return leerWlPaste()
	}
	return datos, err
}

// leerControlDeDatos lee la selección como `tipo` (o uno que empiece por él, como
// `text/html;charset=utf-8`) con el control de datos del compositor.
func leerControlDeDatos(tipo string) ([]byte, error) {
	w, err := conectarWayland()
	if err != nil {
		return nil, ErrNoDisponible
	}
	defer w.cerrar()
	globales, err := w.globales()
	if err != nil {
		return nil, ErrNoDisponible
	}
	var seat, gestor *globalWL
	for i := range globales {
		g := &globales[i]
		if g.interfaz == "wl_seat" && seat == nil {
			seat = g
		}
	}
	for _, nombre := range interfacesControl {
		if i := slices.IndexFunc(globales, func(g globalWL) bool { return g.interfaz == nombre }); i >= 0 && gestor == nil {
			gestor = &globales[i]
		}
	}
	if seat == nil || gestor == nil {
		return nil, ErrNoDisponible
	}
	idSeat, idGestor := w.enlazar(seat, 1), w.enlazar(gestor, 1)
	dispositivo := w.nuevo()
	w.pedir(idGestor, 1, u32(dispositivo), u32(idSeat)) // get_data_device: el dispositivo nuevo, del seat
	seleccion, tipos, err := w.seleccion(dispositivo)
	if err != nil {
		return nil, ErrNoDisponible
	}
	i := slices.IndexFunc(tipos, func(t string) bool { return strings.HasPrefix(t, tipo) })
	if seleccion == 0 || i < 0 {
		return nil, ErrSinHTML
	}
	r, e, err := os.Pipe()
	if err != nil {
		return nil, ErrNoDisponible
	}
	defer func() { _ = r.Close() }()
	// receive: el tipo, y la tubería por la que lo escribe quien copió; su extremo de
	// escribir ya es suyo.
	w.pedirConFD(seleccion, 0, int(e.Fd()), cadena(tipos[i]))
	_ = e.Close()
	if w.err != nil {
		return nil, ErrNoDisponible
	}
	_ = r.SetReadDeadline(time.Now().Add(esperaWayland))
	datos, err := io.ReadAll(io.LimitReader(r, MaxHTML*2+1))
	if err != nil || len(datos) == 0 || len(datos) > MaxHTML*2 {
		return nil, ErrSinHTML
	}
	return datos, nil
}

// seleccion lee, hasta un sync, lo que el compositor cuenta al crear el dispositivo:
// cada oferta con sus tipos y cuál es la selección (0 si no hay ninguna).
func (w *conexionWL) seleccion(dispositivo uint32) (uint32, []string, error) {
	listo := w.nuevo()
	w.pedir(1, 0, u32(listo)) // wl_display.sync
	if w.err != nil {
		return 0, nil, w.err
	}
	ofertas := map[uint32][]string{}
	var sel uint32
	for {
		ev, err := w.evento()
		if err != nil {
			return 0, nil, err
		}
		switch {
		case ev.obj == listo:
			return sel, ofertas[sel], nil
		case ev.obj == 1 && ev.op == 0: // wl_display.error
			return 0, nil, errors.New("wayland: error del compositor")
		case ev.obj == dispositivo && ev.op == 0: // data_offer: una oferta nueva
			ofertas[ev.u32(0)] = nil
		case ev.obj == dispositivo && ev.op == 1: // selection: la oferta elegida, o 0 si no hay
			sel = ev.u32(0)
		case ev.obj == dispositivo && ev.op == 2: // finished: el compositor lo ha retirado
			return 0, nil, errors.New("wayland: dispositivo retirado")
		case ev.op == 0:
			if tipos, ok := ofertas[ev.obj]; ok { // offer(mime)
				ofertas[ev.obj] = append(tipos, ev.cadena(0))
			}
		}
	}
}

// leerWlPaste es el respaldo: `wl-paste` (wl-clipboard), si está.
func leerWlPaste() ([]byte, error) {
	ctx, cancelar := context.WithTimeout(context.Background(), esperaWayland)
	defer cancelar()
	tipos, err := exec.CommandContext(ctx, "wl-paste", "--list-types").Output()
	if err != nil {
		return nil, ErrNoDisponible
	}
	if !slices.Contains(strings.Fields(string(tipos)), "text/html") {
		return nil, ErrSinHTML
	}
	datos, err := exec.CommandContext(ctx, "wl-paste", "--no-newline", "--type", "text/html").Output()
	if err != nil || len(datos) == 0 {
		return nil, ErrSinHTML
	}
	return datos, nil
}

// ---- El protocolo, lo justo ----
//
// Cada mensaje es el objeto (u32), el tamaño y el código (u32: tamaño<<16 | código) y sus
// argumentos, en el orden de bytes de la máquina: enteros de 32 bits, y las cadenas con su
// largo (con el NUL) y rellenas hasta 4. Los descriptores de fichero no van en el mensaje:
// viajan aparte, como datos de control del socket (SCM_RIGHTS).

type globalWL struct {
	nombre, version uint32
	interfaz        string
}

type eventoWL struct {
	obj  uint32
	op   uint32 // los 16 bits bajos de su segunda palabra
	args []byte
}

func conectarWayland() (*conexionWL, error) {
	ruta := os.Getenv("WAYLAND_DISPLAY")
	if !filepath.IsAbs(ruta) {
		dir := os.Getenv("XDG_RUNTIME_DIR")
		if dir == "" {
			return nil, ErrNoDisponible
		}
		ruta = filepath.Join(dir, ruta)
	}
	c, err := net.DialUnix("unix", nil, &net.UnixAddr{Name: ruta, Net: "unix"})
	if err != nil {
		return nil, err
	}
	_ = c.SetDeadline(time.Now().Add(esperaWayland))
	return &conexionWL{c: c, sig: 1}, nil
}

func (w *conexionWL) nuevo() uint32 { w.sig++; return w.sig }

func u32(v uint32) []byte { return binary.NativeEndian.AppendUint32(nil, v) }

// largo16 es `n` como tamaño del protocolo, que va en 16 bits: un mensaje no pasa de
// 64 KiB.
func largo16(n int) (uint32, bool) {
	if n < 0 || n > math.MaxUint16 {
		return 0, false
	}
	return uint32(n), true
}

// cadena es una cadena del protocolo. Una que no quepa en 16 bits hace el mensaje
// demasiado largo, y `mensaje` no lo deja salir.
func cadena(s string) []byte {
	l, _ := largo16(len(s) + 1)
	b := u32(l)
	b = append(b, s...)
	b = append(b, 0)
	for len(b)%4 != 0 {
		b = append(b, 0)
	}
	return b
}

// mensaje es la petición entera; si no cabe, deja el fallo en w.err y no se manda.
func (w *conexionWL) mensaje(obj uint32, op uint16, args [][]byte) []byte {
	cuerpo := slices.Concat(args...)
	tam, ok := largo16(8 + len(cuerpo))
	if !ok {
		w.err = cmp.Or(w.err, errors.New("wayland: mensaje demasiado largo"))
		return nil
	}
	m := u32(obj)
	m = binary.NativeEndian.AppendUint32(m, tam<<16|uint32(op))
	return append(m, cuerpo...)
}

func (w *conexionWL) pedir(obj uint32, op uint16, args ...[]byte) {
	if m := w.mensaje(obj, op, args); w.err == nil {
		_, w.err = w.c.Write(m)
	}
}

func (w *conexionWL) pedirConFD(obj uint32, op uint16, fd int, args ...[]byte) {
	if m := w.mensaje(obj, op, args); w.err == nil {
		_, _, w.err = w.c.WriteMsgUnix(m, syscall.UnixRights(fd), nil)
	}
}

// enlazar es wl_registry.bind(nombre, interfaz, versión, id): da el id del objeto.
func (w *conexionWL) enlazar(g *globalWL, version uint32) uint32 {
	id := w.nuevo()
	w.pedir(w.registro, 0, u32(g.nombre), cadena(g.interfaz), u32(min(version, g.version)), u32(id))
	return id
}

// globales pide el registro y lo lee hasta el sync.
func (w *conexionWL) globales() ([]globalWL, error) {
	registro := w.nuevo()
	w.registro = registro
	w.pedir(1, 1, u32(registro)) // get_registry
	listo := w.nuevo()
	w.pedir(1, 0, u32(listo)) // sync
	if w.err != nil {
		return nil, w.err
	}
	var out []globalWL
	for {
		ev, err := w.evento()
		if err != nil {
			return nil, err
		}
		switch {
		case ev.obj == listo:
			return out, nil
		case ev.obj == 1 && ev.op == 0:
			return nil, fmt.Errorf("wayland: error %d del compositor", ev.u32(4))
		case ev.obj == registro && ev.op == 0: // global: su nombre, su interfaz y su versión
			nombre, interfaz := ev.u32(0), ev.cadena(4)
			out = append(out, globalWL{nombre: nombre, interfaz: interfaz, version: ev.u32(8 + relleno(len(interfaz)+1))})
		}
	}
}

func relleno(n int) int { return (n + 3) &^ 3 }

// evento lee el siguiente mensaje del compositor.
func (w *conexionWL) evento() (eventoWL, error) {
	for len(w.buf) < 8 || len(w.buf) < int(binary.NativeEndian.Uint32(w.buf[4:])>>16) {
		if err := w.recibir(); err != nil {
			return eventoWL{}, err
		}
	}
	tam := int(binary.NativeEndian.Uint32(w.buf[4:]) >> 16)
	if tam < 8 {
		return eventoWL{}, errors.New("wayland: mensaje mal formado")
	}
	ev := eventoWL{
		obj:  binary.NativeEndian.Uint32(w.buf),
		op:   binary.NativeEndian.Uint32(w.buf[4:]) & 0xffff,
		args: bytes.Clone(w.buf[8:tam]),
	}
	w.buf = w.buf[tam:]
	return ev, nil
}

func (w *conexionWL) recibir() error {
	b, oob := make([]byte, 4096), make([]byte, syscall.CmsgSpace(4*28))
	n, noob, _, _, err := w.c.ReadMsgUnix(b, oob)
	if err != nil {
		return err
	}
	if n == 0 {
		return io.EOF
	}
	w.buf = append(w.buf, b[:n]...)
	if noob > 0 {
		if msgs, err := syscall.ParseSocketControlMessage(oob[:noob]); err == nil {
			for _, m := range msgs {
				if fds, err := syscall.ParseUnixRights(&m); err == nil {
					w.fds = append(w.fds, fds...)
				}
			}
		}
	}
	return nil
}

// cerrar cierra la conexión y los descriptores que hayan llegado sin usarse.
func (w *conexionWL) cerrar() {
	for _, fd := range w.fds {
		_ = syscall.Close(fd)
	}
	w.fds = nil
	_ = w.c.Close()
}

func (e eventoWL) u32(desde int) uint32 {
	if desde+4 > len(e.args) {
		return 0
	}
	return binary.NativeEndian.Uint32(e.args[desde:])
}

func (e eventoWL) cadena(desde int) string {
	n := int(e.u32(desde))
	if n == 0 || desde+4+n > len(e.args) {
		return ""
	}
	return string(e.args[desde+4 : desde+4+n-1])
}
