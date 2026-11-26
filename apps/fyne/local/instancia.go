// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package local

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
)

// # Una sola copia de cada app
//
// En el escritorio, abrir un fichero con una app que ya está abierta («Abrir con», el
// Hub, otra app de la edición) lanzaría un segundo proceso con su propia ventana… y su
// propio almacén abierto sobre los mismos ficheros, que no admite dos escritores. Así
// que la primera copia escucha en un enchufe local y la segunda le pasa lo que traía en
// los argumentos, le pide que se enseñe y se va. Es la idea de `sesion/instancia.go` de
// la conectada, que la edición no enlaza. No es red: el enchufe es un socket Unix (en
// Windows 10 y posteriores también) en una carpeta 0700 de la persona. En el móvil cada
// app es una sola copia por el sistema y esto no se usa.

const (
	ordenTraer  = "traer"
	ordenEnlace = "enlace "
	// maxOrden es lo más largo que se acepta en una línea (un enlace con una ruta).
	maxOrden = 64 << 10
)

// EnchufeDe es el socket de la copia viva de una app, por su identificador de paquete.
func EnchufeDe(id string) (string, error) {
	base, err := os.UserCacheDir()
	if err != nil {
		return "", err
	}
	dir := filepath.Join(base, "sytue-office")
	ruta := filepath.Join(dir, id+".sock")
	// Un socket Unix admite unos 104 bytes de ruta (108 en Linux).
	if len(ruta) > 96 {
		dir = filepath.Join(os.TempDir(), fmt.Sprintf("sytue-office-%d", os.Getuid()))
		ruta = filepath.Join(dir, id+".sock")
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", err
	}
	return ruta, nil
}

// llamarCopia llama al enchufe de la copia viva, con un plazo corto: una que no contesta
// enseguida es el enchufe de una que murió.
func llamarCopia(ctx context.Context, ruta string) (net.Conn, error) {
	ctx, parar := context.WithTimeout(ctx, 300*time.Millisecond)
	defer parar()
	var d net.Dialer
	return d.DialContext(ctx, "unix", ruta)
}

// CopiaViva dice si la app con ese identificador está abierta en este equipo.
func CopiaViva(id string) bool {
	ruta, err := EnchufeDe(id)
	if err != nil {
		return false
	}
	c, err := llamarCopia(context.Background(), ruta)
	if err != nil {
		return false
	}
	c.Close()
	return true
}

// PasarAOtraCopia le da a la copia viva los enlaces y le pide que se enseñe; dice si la
// había (y entonces esta copia sobra).
func PasarAOtraCopia(id string, enlaces []*url.URL) bool {
	ruta, err := EnchufeDe(id)
	if err != nil {
		return false
	}
	c, err := llamarCopia(context.Background(), ruta)
	if err != nil {
		return false
	}
	defer c.Close()
	_ = c.SetDeadline(time.Now().Add(2 * time.Second))
	var b strings.Builder
	for _, u := range enlaces {
		if s := u.String(); len(s) < maxOrden-len(ordenEnlace) && !strings.ContainsAny(s, "\r\n") {
			b.WriteString(ordenEnlace + s + "\n")
		}
	}
	b.WriteString(ordenTraer + "\n")
	_, err = io.WriteString(c, b.String())
	return err == nil
}

// EscucharCopias atiende a las copias que se abran después: cada enlace que pasan llega
// a enlace y cada «ábrete», a traer (los dos desde otra goroutine).
func EscucharCopias(ctx context.Context, id string, enlace func(*url.URL), traer func()) {
	ruta, err := EnchufeDe(id)
	if err != nil {
		slog.Info("local: sin carpeta de caché no hay copia única", "error", err)
		return
	}
	var cfg net.ListenConfig
	l, err := cfg.Listen(ctx, "unix", ruta)
	if err != nil {
		// Otra viva (y esta no debería estar aquí) o el socket de una que murió.
		if c, e := llamarCopia(ctx, ruta); e == nil {
			c.Close()
			return
		}
		_ = os.Remove(ruta)
		if l, err = cfg.Listen(ctx, "unix", ruta); err != nil {
			slog.Info("local: no se ha podido abrir el enchufe de copia única", "error", err)
			return
		}
	}
	go func() {
		<-ctx.Done()
		l.Close()
		_ = os.Remove(ruta)
	}()
	go func() {
		for {
			c, err := l.Accept()
			if err != nil {
				return
			}
			go func() {
				defer c.Close()
				_ = c.SetDeadline(time.Now().Add(5 * time.Second))
				s := bufio.NewScanner(io.LimitReader(c, 16*maxOrden))
				s.Buffer(make([]byte, 0, 4096), maxOrden)
				for s.Scan() {
					switch linea := s.Text(); {
					case linea == ordenTraer:
						traer()
					case strings.HasPrefix(linea, ordenEnlace):
						if u, err := url.Parse(strings.TrimPrefix(linea, ordenEnlace)); err == nil {
							enlace(u)
						}
					}
				}
			}()
		}
	}()
}

// idDeCopia es el identificador con que la app busca su copia viva; "" en las pruebas
// (la app de pruebas de Fyne se llama igual en todas) y sin identificador.
func idDeCopia(a fyne.App) string {
	id := a.Metadata().ID
	if id == "" {
		id = a.UniqueID()
	}
	if id == "" || strings.HasPrefix(id, "testApp") {
		return ""
	}
	return id
}

// copiaUnica: si ya hay una copia viva de esta app, le pasa los enlaces y sale; si no,
// escucha a las que vengan.
func (l *App) copiaUnica(enlaces []*url.URL) {
	id := idDeCopia(l.A)
	if id == "" || Tactil() {
		return
	}
	if PasarAOtraCopia(id, enlaces) {
		// La copia viva ya tiene lo que traía esta, y la app aún no ha abierto ventana ni
		// almacén (lo hace al volver de Abrir).
		os.Exit(0) //nolint:revive // deep-exit: esta copia sobra y no tiene nada que cerrar
	}
	ctx, parar := context.WithCancel(context.Background())
	l.AlCerrar(parar)
	EscucharCopias(ctx, id,
		func(u *url.URL) { fyne.Do(func() { l.recibir(u) }) },
		func() { fyne.Do(l.traer) })
}

// traer enseña la ventana de la app y le da el foco.
func (l *App) traer() {
	if ws := l.A.Driver().AllWindows(); len(ws) > 0 {
		ws[0].Show()
		ws[0].RequestFocus()
	}
}
