// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

//go:build linux && !android

package auth

import (
	"bytes"
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/hkdf"
	"crypto/rand"
	"crypto/sha256"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"syscall"
	"time"

	"github.com/godbus/dbus/v5"

	"git.sytue.eu/sytue/one/apps/nucleo/ajustes"
)

// # La sesión sin llavero, en Linux
//
// Un escritorio sin Secret Service (KWallet apagado, un gestor de ventanas suelto, una
// sesión por ssh) no tiene dónde guardar la sesión. Antes eso era entrar otra vez en cada
// arranque de cada una de las trece apps; ahora la sesión va a un fichero CIFRADO:
//
//   - `$XDG_CONFIG_HOME/sytue/sesion.cifrada` (0600, carpeta 0700): el sobre de AES-256-GCM
//     ([empaquetar]) con el mismo JSON que iría al llavero.
//   - La clave NO está en ese fichero: sale con HKDF-SHA256 de 32 bytes al azar guardados
//     en `$XDG_STATE_HOME/sytue/sesion.clave` (0600, carpeta 0700), con el machine-id de
//     la máquina como sal.
//
// # Modelo de amenaza
//
// Protege de que el fichero de la sesión salga solo o con una parte de la casa: una copia
// de `~/.config` (dotfiles en un repositorio, una sincronización, un informe de fallo que
// la adjunta) no lleva la clave, que está en `~/.local/state`. Y de que salga con la casa
// ENTERA (Déjà Dup, restic o Timeshift sí copian `~/.local/state`): el machine-id, 128
// bits al azar de `/etc`, no va en esa copia, y sin él la clave no se deriva.
//
// NO protege de:
//
//   - Lo que corre como la misma persona en la misma máquina: cualquier proceso suyo lee
//     los dos ficheros y el machine-id (que es legible por todos), igual que podría leer
//     la memoria de la app. Eso incluye las apps de Flatpak o Snap con acceso a la casa,
//     que al llavero no llegarían.
//   - root, ni una copia de la casa junto con `/etc`.
//   - Un portátil robado sin cifrado de disco: un llavero con la contraseña de inicio de
//     sesión sí protege ahí, y este fichero no.
//
// Es lo que da un llavero sin contraseña propia, no lo que da uno bloqueado. Por eso la
// app avisa ([SinLlavero]) de que conviene activar KWallet o GNOME Keyring, y en cuanto
// hay llavero la sesión se muda a él y se borran el fichero y la clave.

const (
	ficheroCifrado = "sesion.cifrada"
	ficheroClave   = "sesion.clave"
	etiquetaSobre  = "sytue-one sesion v1" // va de dato asociado: un sobre de otra cosa no pasa
	longitudSecret = 32
)

// errSobreInvalido es un sobre o una clave que no valen (manipulados, de otra máquina,
// legibles por otros): lo que se borra. Un error de E/S no lo es, y no borra nada.
var errSobreInvalido = errors.New("auth: la sesión cifrada no vale en esta máquina")

// sinLlavero dice si no hay llavero donde guardar: ningún Secret Service que conteste, o
// uno sin colección por defecto, que es donde go-keyring guarda. Lo segundo es el KWallet
// de un Plasma recién instalado (medido en KDE neon con Plasma 6.7): contesta, pero hasta
// que algo crea la cartera cada guardado falla con «Alias default does not exist», y la
// sesión no se recordaba nunca. Con llavero y cartera, un fallo es del llavero
// (bloqueado, la persona dijo que no) y no se esquiva escribiendo a disco. Preguntar por
// el alias no pide desbloquear nada.
//
// Con plazo ([plazoLlavero]): esto corre dentro del cerrojo de la sesión compartida.
// Un llavero que no contesta a tiempo cuenta como un llavero que falla,
// no como ninguno: esa vez no se guarda, y la siguiente irá bien.
func sinLlavero() bool {
	conn, err := dbus.SessionBus()
	if err != nil {
		return true // sin bus de sesión no hay llavero que valga
	}
	ctx, parar := context.WithTimeout(context.Background(), plazoLlavero)
	defer parar()
	var coleccion dbus.ObjectPath
	err = conn.Object("org.freedesktop.secrets", "/org/freedesktop/secrets").CallWithContext(ctx,
		"org.freedesktop.Secret.Service.ReadAlias", 0, "default").Store(&coleccion)
	if ctx.Err() != nil {
		return false
	}
	return err != nil || coleccion == "/"
}

// plazoLlavero es cuánto se espera al Secret Service: lo que tarda en despertar
// gnome-keyring la primera vez (un par de segundos), sin bloquear indefinidamente
// a las demás apps de la sesión compartida.
var plazoLlavero = 5 * time.Second

// rutasDelFichero son la del sobre, en la carpeta de ajustes de la suite
// (`$XDG_CONFIG_HOME/sytue`), y la del secreto, en `$XDG_STATE_HOME/sytue`. No crea nada:
// eso lo hace [carpetaPrivada] al guardar.
func rutasDelFichero(cuenta ...string) (sesion, clave string, err error) {
	config, err := os.UserConfigDir()
	if err != nil {
		return "", "", err
	}
	estado := os.Getenv("XDG_STATE_HOME")
	if !filepath.IsAbs(estado) { // la especificación: una relativa se ignora
		casa, err := os.UserHomeDir()
		if err != nil {
			return "", "", err
		}
		estado = filepath.Join(casa, ".local", "state")
	}
	sufijo := ""
	if len(cuenta) > 0 && cuenta[0] != llaveroCuenta {
		sufijo = "." + cuenta[0]
	}
	return filepath.Join(config, "sytue", ficheroCifrado+sufijo), filepath.Join(estado, "sytue", ficheroClave+sufijo), nil
}

// carpetaPrivada crea la carpeta de un fichero, o comprueba la que ya había: una carpeta
// de quien ejecuta la app, y 0700 aunque viniera más abierta.
func carpetaPrivada(fichero string) error {
	dir := filepath.Dir(fichero)
	if err := os.MkdirAll(dir, 0o700); err != nil { //nolint:gosec // G703: XDG_CONFIG_HOME/XDG_STATE_HOME de quien ejecuta la app
		return err
	}
	// Stat y no Lstat: una carpeta enlazada (GNU stow, un gestor de dotfiles) vale si lo
	// que hay al otro lado es una carpeta tuya.
	info, err := os.Stat(dir)
	if err != nil {
		return err
	}
	st, ok := info.Sys().(*syscall.Stat_t)
	if !info.IsDir() || !ok || int(st.Uid) != os.Getuid() {
		return fmt.Errorf("auth: %s no es una carpeta tuya: no se guarda ahí la sesión", dir)
	}
	if info.Mode().Perm()&0o077 != 0 {
		return os.Chmod(dir, 0o700)
	}
	return nil
}

// leerPrivado lee un fichero que tiene que ser solo de quien ejecuta la app, sin seguir
// enlaces, sin quedarse colgado en una tubería y mirando el mismo fichero que se lee. Si otros pueden leerlo, no vale
// ([errSobreInvalido]): quien lo haya leído ya tiene lo que protege.
func leerPrivado(ruta string) ([]byte, error) {
	f, err := os.OpenFile(ruta, os.O_RDONLY|syscall.O_NOFOLLOW|syscall.O_NONBLOCK, 0) //nolint:gosec // G304: ruta de las carpetas XDG de quien ejecuta la app
	if errors.Is(err, syscall.ELOOP) {
		return nil, fmt.Errorf("%w: %s es un enlace", errSobreInvalido, ruta)
	}
	if err != nil {
		return nil, err
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil {
		return nil, err
	}
	st, ok := info.Sys().(*syscall.Stat_t)
	if !info.Mode().IsRegular() || info.Mode().Perm()&0o077 != 0 || !ok || int(st.Uid) != os.Getuid() {
		return nil, fmt.Errorf("%w: %s no es un fichero solo tuyo (%v)", errSobreInvalido, ruta, info.Mode())
	}
	return io.ReadAll(f)
}

// crearClave pone un secreto nuevo en `ruta`, entero o nada: se escribe al lado y se pone
// en su sitio de una vez. Si no había ninguno, con `link`, que falla si otra app se
// adelantó (y entonces vale la suya); si había uno que no vale (`sustituir`), con
// `rename` encima.
//
// ponytail: dos apps que sustituyen A LA VEZ una clave ya rota pueden quedarse cada una
// con la suya, y la sesión de una no se abre: se vuelve a entrar. Cerrarlo pediría el
// cerrojo de la sesión, que ya tiene puesto quien guarda desde Compartida.
func crearClave(ruta string, sustituir bool) error {
	secreto := make([]byte, longitudSecret)
	if _, err := rand.Read(secreto); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(ruta), "."+ficheroClave+".*")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name()) //nolint:errcheck // queda con su nombre, o sobra
	if _, err := tmp.Write(secreto); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Sync(); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if sustituir {
		return os.Rename(tmp.Name(), ruta)
	}
	if err := os.Link(tmp.Name(), ruta); err != nil && !errors.Is(err, fs.ErrExist) {
		return err
	}
	return nil
}

// machineID es el de systemd, o el de D-Bus donde no lo hay.
func machineID() ([]byte, error) {
	id, err := os.ReadFile("/etc/machine-id")
	if err != nil || len(bytes.TrimSpace(id)) == 0 {
		id, err = os.ReadFile("/var/lib/dbus/machine-id")
	}
	if id = bytes.TrimSpace(id); err == nil && len(id) == 0 {
		err = errors.New("vacío")
	}
	if err != nil {
		return nil, fmt.Errorf("auth: sin machine-id no se ata la sesión a esta máquina: %w", err)
	}
	return id, nil
}

// claveDelFichero deriva la clave de AES. Con `crear`, si no hay secreto o el que hay no
// vale (otro tamaño, legible por otros, un fichero vacío tras un corte de luz), pone uno
// nuevo: lo que estuviera cifrado con el viejo ya no se abre, y se vuelve a entrar.
func claveDelFichero(rutaClave string, crear bool) ([]byte, error) {
	secreto, err := leerPrivado(rutaClave)
	if err == nil && len(secreto) != longitudSecret {
		err = fmt.Errorf("%w: la clave no tiene el tamaño que toca", errSobreInvalido)
	}
	if crear && (errors.Is(err, fs.ErrNotExist) || errors.Is(err, errSobreInvalido)) {
		if err := crearClave(rutaClave, errors.Is(err, errSobreInvalido)); err != nil {
			return nil, err
		}
		return claveDelFichero(rutaClave, false)
	}
	if err != nil {
		return nil, err
	}
	maquina, err := machineID()
	if err != nil {
		return nil, err
	}
	return hkdf.Key(sha256.New, secreto, maquina, etiquetaSobre, 32)
}

func aead(clave []byte) (cipher.AEAD, error) {
	bloque, err := aes.NewCipher(clave)
	if err != nil {
		return nil, err
	}
	return cipher.NewGCM(bloque)
}

// cargarFichero devuelve la sesión del fichero y cuándo se guardó, o vacía si no hay. Un
// sobre que no vale (otra máquina, clave perdida, manipulado, legible por otros) se borra:
// guarda un token que ya no se puede usar, o que ya no es secreto, y la persona vuelve a
// entrar. Un error de E/S, o que la clave no esté (otra app puede estar poniéndola), no
// borra nada: guardarFichero lo sobrescribirá.
func cargarFichero(cuenta ...string) (Tokens, int64, error) {
	rutaSesion, rutaClave, err := rutasDelFichero(cuenta...)
	if err != nil {
		return Tokens{}, 0, err
	}
	sobre, err := leerPrivado(rutaSesion)
	if errors.Is(err, fs.ErrNotExist) {
		return Tokens{}, 0, nil
	}
	var t Tokens
	var cuando int64
	if err == nil {
		t, cuando, err = abrirSobre(sobre, rutaClave)
	}
	if errors.Is(err, errSobreInvalido) {
		_ = os.Remove(rutaSesion)
	}
	return t, cuando, err
}

func abrirSobre(sobre []byte, rutaClave string) (Tokens, int64, error) {
	clave, err := claveDelFichero(rutaClave, false)
	if err != nil {
		return Tokens{}, 0, err
	}
	g, err := aead(clave)
	if err != nil {
		return Tokens{}, 0, err
	}
	iv, cifrado, err := desempaquetar(sobre)
	if err != nil {
		return Tokens{}, 0, fmt.Errorf("%w: %w", errSobreInvalido, err)
	}
	claro, err := g.Open(nil, iv, cifrado, []byte(etiquetaSobre))
	if err != nil {
		return Tokens{}, 0, fmt.Errorf("%w: no se abre con la clave de esta máquina", errSobreInvalido)
	}
	t, cuando, err := recuperarConInstante(claro)
	if err != nil {
		return Tokens{}, 0, fmt.Errorf("%w: %w", errSobreInvalido, err)
	}
	return t, cuando, nil
}

// guardarFichero cifra la sesión en el fichero y dice si lo ha creado (no existía).
func guardarFichero(t Tokens, cuenta ...string) (bool, error) {
	rutaSesion, rutaClave, err := rutasDelFichero(cuenta...)
	if err != nil {
		return false, err
	}
	for _, ruta := range []string{rutaSesion, rutaClave} {
		if err := carpetaPrivada(ruta); err != nil {
			return false, err
		}
	}
	claro, err := serializar(t)
	if err != nil {
		return false, err
	}
	clave, err := claveDelFichero(rutaClave, true)
	if err != nil {
		return false, err
	}
	g, err := aead(clave)
	if err != nil {
		return false, err
	}
	iv := make([]byte, g.NonceSize())
	if _, err := rand.Read(iv); err != nil {
		return false, err
	}
	sobre, err := empaquetar(iv, g.Seal(nil, iv, claro, []byte(etiquetaSobre))) //nolint:gosec // G407: falso positivo, el nonce sale de crypto/rand dos líneas más arriba
	if err != nil {
		return false, err
	}
	_, errAntes := os.Lstat(rutaSesion)
	return errors.Is(errAntes, fs.ErrNotExist), ajustes.EscribirEntero(rutaSesion, sobre)
}

// borrarFichero quita la sesión del fichero y su clave (salir, o mudarse al llavero): un
// sobre viejo que quedara en una copia de `~/.config` ya no se abre con nada.
func borrarFichero(cuenta ...string) error {
	rutaSesion, rutaClave, err := rutasDelFichero(cuenta...)
	if err != nil {
		return err
	}
	var errs []error
	for _, ruta := range []string{rutaSesion, rutaClave} {
		if err := os.Remove(ruta); err != nil && !errors.Is(err, fs.ErrNotExist) {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}
