// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package sesion

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"fyne.io/fyne/v2"

	"git.sytue.eu/sytue/one/apps/nucleo/sistema/escritorio"
)

// # Un enlace entrante va a la app QUE YA ESTÁ ABIERTA
//
// En macOS, iOS y Android el sistema se lo entrega a la instancia viva —Apple Event,
// `application:openURL:` y `onNewIntent`—, y el fork de Fyne lo pasa a `recibirURL`. En
// Linux y en Windows no: ahí «abrir un esquema» es EJECUTAR el programa con la URL como
// argumento, así que sin nada más pulsar un documento abre una segunda ventana con una
// sesión, un vigilante y un latido más, y la primera se queda como estaba.
//
// Se resuelve como lo resuelve cualquier escritorio: un enchufe local por aplicación. La
// que arranca primero escucha; la segunda, si trae URL y hay alguien al otro lado, se la
// pasa y se va sin pintar nada.
//
// ponytail: el enchufe se abre al Arrancar, así que dos arranques a la vez pueden
// escuchar los dos —el perdedor no puede crear el socket y sigue como app suelta—. Un
// cerrojo de fichero lo cerraría; se deja hasta que a alguien le pase, porque el caso es
// pulsar dos veces en el mismo instante.

// enchufe es el socket de esta aplicación, uno por identificador de paquete. Va en la
// carpeta de caché del usuario y no en /tmp: ahí solo entra quien es él, y por este
// canal se entrega lo mismo que entregaría el sistema.
func enchufe(id string) (string, error) {
	base, err := os.UserCacheDir()
	if err != nil {
		return "", err
	}
	dir := filepath.Join(base, "sytue")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", err
	}
	ruta := filepath.Join(dir, id+".sock")
	// Un socket Unix en Darwin admite como máximo 104 bytes (108 en Linux). Si la ruta
	// de la caché supera los 96 bytes (en entornos de prueba con t.TempDir() o cuentas con
	// nombres de usuario largos), caemos a un directorio temporal privado (0700).
	if len(ruta) > 96 {
		dirTmp := filepath.Join(os.TempDir(), fmt.Sprintf("sytue-%d", os.Getuid()))
		if len(dirTmp) > 60 {
			dirTmp = fmt.Sprintf("/tmp/sytue-%d", os.Getuid())
		}
		if err := os.MkdirAll(dirTmp, 0o700); err != nil {
			return "", err
		}
		return filepath.Join(dirTmp, id+".sock"), nil
	}
	return ruta, nil
}

// EsEnlaceDeLaSuite dice si un argumento de la línea de órdenes es una URL de la suite
// —así es como Linux y Windows entregan una apertura por esquema— y no un fichero.
//
// Se mira el esquema y que empiece por `sytue`, no solo que lo haya: en Windows una ruta
// absoluta (`C:\Docs\acta.json`) TAMBIÉN se parsea con esquema, el `c:`, y confundirla
// con un enlace es dejar de abrir el fichero que se pidió.
func EsEnlaceDeLaSuite(arg string) bool {
	u, err := url.Parse(arg)
	return err == nil && strings.HasPrefix(u.Scheme, "sytue")
}

// urlsDeArgs son las URL de la suite con las que se ha arrancado el programa, y los
// ficheros de la máquina que se le han pedido abrir, estos como `file://`: la instancia
// que ya corre recibe URL por su enchufe, no rutas. Una opción o una ruta que no existe
// no es asunto de esto.
func urlsDeArgs(args []string) []string {
	var out []string
	for _, a := range args {
		if EsEnlaceDeLaSuite(a) {
			out = append(out, a)
		} else if f, ok := ficheroDeArg(a); ok {
			out = append(out, f)
		}
	}
	return out
}

// reenviar entrega las URL de arranque a la instancia que ya corre y dice si lo ha
// conseguido. Si no contesta nadie —no hay otra, o murió dejando el socket— devuelve
// false y este proceso sigue siendo la aplicación.
func reenviar(id string, args []string) bool {
	enlaces := urlsDeArgs(args)
	if len(enlaces) == 0 {
		return false
	}
	ruta, err := enchufe(id)
	if err != nil {
		return false
	}
	c, err := llamar(context.Background(), ruta)
	if err != nil {
		return false
	}
	defer c.Close()
	// La instancia viva se pone delante al recibirlo (aparecer); en Windows solo puede si
	// esta, que tiene el primer plano del clic que la lanzó, se lo cede.
	escritorio.CederPrimerPlano()
	_ = c.SetDeadline(time.Now().Add(2 * time.Second))
	for _, e := range enlaces {
		if _, err := io.WriteString(c, e+"\n"); err != nil {
			return false
		}
	}
	return true
}

// Traer le pide a la instancia viva de esta app que enseñe su ventana, y dice si la
// había. Es para la app que vive en la bandeja con la ventana escondida —el Hub, que
// arranca así al iniciar sesión—: abrirla otra vez desde el lanzador la trae, en vez de
// poner una segunda copia con su propio icono en la bandeja. Las demás no lo llaman: una
// segunda copia a mano sigue siendo una segunda copia, que es lo que fija
// TestElEnlaceVaALaInstanciaViva.
//
// Va por el mismo enchufe con la raíz del esquema, que el receptor toma por «ábrete».
func Traer(a fyne.App) bool {
	m := a.Metadata()
	if m.ID == "" || m.Custom["esquema"] == "" || a.Driver().Device().IsMobile() {
		return false
	}
	return reenviar(m.ID, []string{m.Custom["esquema"] + ":/"})
}

// llamar toca el enchufe de esta app. Es una sola línea con nombre porque se usa dos
// veces —al reenviar y al distinguir un socket huérfano de una instancia viva— y porque
// el plazo es el mismo: si el de al lado no contesta en 300 ms, no está.
func llamar(padre context.Context, ruta string) (net.Conn, error) {
	ctx, parar := context.WithTimeout(padre, 300*time.Millisecond)
	defer parar()
	var d net.Dialer
	return d.DialContext(ctx, "unix", ruta)
}

// Abierta dice si la aplicación `id` (`eu.sytue.mail`) está abierta en esta máquina y
// en esta sesión de usuario: si su enchufe contesta. Solo en escritorio; en el móvil no
// hay enchufes y quien pregunta tiene que saberlo.
func Abierta(id string) bool {
	ruta, err := enchufe(id)
	if err != nil {
		return false
	}
	c, err := llamar(context.Background(), ruta)
	if err != nil {
		return false
	}
	_ = c.Close()
	return true
}

// maxEnlace es lo más largo que se acepta por el enchufe. Una URL de apertura ronda los
// cien bytes; ocho kilobytes dan margen de sobra y evitan que un cliente charlatán haga
// crecer la memoria.
const maxEnlace = 8 << 10

// escuchar abre el enchufe y entrega lo que llegue. Vuelve cuando ctx muere.
func escuchar(ctx context.Context, id string, entregar func(*url.URL)) {
	ruta, err := enchufe(id)
	if err != nil {
		slog.Warn("sesion: sin carpeta de caché no hay instancia única", "error", err)
		return
	}
	var cfg net.ListenConfig
	l, err := cfg.Listen(ctx, "unix", ruta)
	if err != nil {
		// O hay otra instancia viva —y entonces esta no debería estar aquí— o quedó el
		// socket de una que murió. Se distingue llamando: si nadie contesta, se borra.
		if c, e := llamar(ctx, ruta); e == nil {
			c.Close()
			return
		}
		_ = os.Remove(ruta)
		if l, err = cfg.Listen(ctx, "unix", ruta); err != nil {
			slog.Warn("sesion: no se ha podido abrir el enchufe de enlaces", "error", err)
			return
		}
	}
	defer os.Remove(ruta)
	go func() {
		<-ctx.Done()
		l.Close()
	}()
	for {
		c, err := l.Accept()
		if err != nil {
			return
		}
		go recibirDelEnchufe(c, entregar)
	}
}

func recibirDelEnchufe(c net.Conn, entregar func(*url.URL)) {
	defer c.Close()
	_ = c.SetDeadline(time.Now().Add(5 * time.Second))
	lector := bufio.NewScanner(io.LimitReader(c, maxEnlace))
	for lector.Scan() {
		u, err := url.Parse(strings.TrimSpace(lector.Text()))
		if err != nil || u.Scheme == "" {
			continue
		}
		entregar(u)
	}
}

// aparecer trae la ventana al frente: quien pulsa un documento espera verlo, no que la
// app lo cargue detrás de la ventana en la que estaba. `Show` es por la app que vive en
// la bandeja ([Residente]) con la ventana escondida; en una visible no hace nada.
func aparecer(a fyne.App) {
	for _, w := range a.Driver().AllWindows() {
		w.Show()
		w.RequestFocus()
		return
	}
}
