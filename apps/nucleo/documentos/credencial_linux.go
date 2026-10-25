// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

//go:build linux && !android

package documentos

import (
	"context"
	"errors"
	"net/url"
	"os"
	"os/exec"
	"strings"
	"time"
)

// En Linux, Drive se monta con GVfs (`gio mount davs://…`): sale en la barra lateral de
// Archivos, Nemo, Caja y Thunar, y en el selector de ficheros de GTK. La contraseña va
// por la entrada estándar, que es como `gio mount` la pide; nunca en la línea de órdenes,
// que ve cualquiera con `ps`.
//
// Sin `gio` (un KDE sin GVfs), la integración informa de que el proveedor no está
// disponible. No instala componentes del sistema ni muestra contraseñas de aplicación.
var hayProveedor = func() bool { _, err := exec.LookPath("gio"); return err == nil }()

// plazoGio acota `gio mount`: con una contraseña que no vale vuelve a preguntar, y la
// entrada ya está cerrada.
const plazoGio = time.Minute

// errNoDavs es una URL de WebDAV que no es https.
var errNoDavs = errors.New("documentos: la URL de WebDAV no es https")

// ubicacion es `davs://<usuario>@host/ruta`: con el usuario dentro, GVfs solo pregunta
// la contraseña.
func ubicacion(c Credencial) (string, error) {
	u, err := url.Parse(c.URL)
	if err != nil || u.Scheme != "https" || u.Host == "" {
		return "", errNoDavs
	}
	u.Scheme, u.User = "davs", url.User(c.Usuario)
	return u.String(), nil
}

func gio(ctx context.Context, entrada string, args ...string) error {
	ctx, parar := context.WithTimeout(ctx, plazoGio)
	defer parar()
	cmd := exec.CommandContext(ctx, "gio", args...)
	cmd.Stdin = strings.NewReader(entrada)
	cmd.Env = append(os.Environ(), "LC_ALL=C") // los mensajes, en inglés: se leen abajo
	salida, err := cmd.CombinedOutput()
	if err != nil {
		return errors.Join(ctx.Err(), err, errors.New(strings.TrimSpace(string(salida))))
	}
	return nil
}

var conectar, desconectar = montar, desmontar

func montar(ctx context.Context, c Credencial) error {
	u, err := ubicacion(c)
	if err != nil {
		return err
	}
	// Montar algo ya montado falla («already mounted»): cuenta como hecho.
	if err := gio(ctx, c.Clave+"\n", "mount", u); err != nil && !strings.Contains(err.Error(), "already mounted") {
		return err
	}
	return nil
}

func desmontar(c Credencial) {
	if u, err := ubicacion(c); err == nil {
		avisar("Drive no se ha podido desmontar", gio(context.Background(), "", "mount", "-u", u))
	}
}
