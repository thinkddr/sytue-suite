// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

//go:build linux && !android

package auth

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/godbus/dbus/v5"
	"github.com/zalando/go-keyring"

	"git.sytue.eu/sytue/one/apps/nucleo/internal/busprueba"
)

// carpetasDePrueba pone XDG_CONFIG_HOME y XDG_STATE_HOME en temporales y devuelve las
// rutas del sobre y de la clave.
func carpetasDePrueba(t *testing.T) (sesion, clave string) {
	t.Helper()
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	sesion, clave, err := rutasDelFichero()
	if err != nil {
		t.Fatal(err)
	}
	return sesion, clave
}

// TestLaSesionDelFicheroVaCifradaYSoloAbreAqui es el guardián del fichero sin llavero: va
// cifrado (el refresh token no se lee en él), 0600, con la clave en OTRO fichero de otra
// carpeta; se abre con esa clave, no con otra (el fichero llevado a otra máquina), y uno
// que no se abre, o que otros pueden leer, se borra. VISTO EN ROJO guardando el JSON en
// claro, sin comprobar los permisos al leer y con la clave en la carpeta del sobre.
func TestLaSesionDelFicheroVaCifradaYSoloAbreAqui(t *testing.T) {
	rutaSesion, rutaClave := carpetasDePrueba(t)
	if filepath.Dir(rutaSesion) == filepath.Dir(rutaClave) {
		t.Fatal("la clave vive en la misma carpeta que la sesión")
	}
	guardada := Tokens{Refresh: "refresco-secreto", Subject: "ada"}
	nuevo, err := guardarFichero(guardada)
	if err != nil || !nuevo {
		t.Fatalf("guardarFichero: nuevo %v, %v", nuevo, err)
	}
	for _, ruta := range []string{rutaSesion, rutaClave} {
		info, err := os.Stat(ruta)
		if err != nil {
			t.Fatal(err)
		}
		if info.Mode().Perm() != 0o600 {
			t.Errorf("%s con permisos %v", ruta, info.Mode().Perm())
		}
	}
	crudo, _ := os.ReadFile(rutaSesion)
	if bytes.Contains(crudo, []byte("refresco-secreto")) || bytes.Contains(crudo, []byte("ada")) {
		t.Fatal("la sesión va en claro en el fichero")
	}
	if got, _, err := cargarFichero(); err != nil || got != guardada {
		t.Fatalf("cargarFichero = %+v, %v", got, err)
	}
	if nuevo, err := guardarFichero(guardada); err != nil || nuevo {
		t.Fatalf("reescribir la sesión cuenta como nueva: %v, %v", nuevo, err)
	}

	// Otros pueden leerlo: no se usa, y se borra.
	if err := os.Chmod(rutaSesion, 0o644); err != nil {
		t.Fatal(err)
	}
	if got, _, err := cargarFichero(); err == nil || got != (Tokens{}) {
		t.Fatalf("un fichero legible por otros se ha usado: %+v", got)
	}
	if _, err := os.Stat(rutaSesion); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("el fichero legible por otros sigue ahí")
	}

	// Otra clave (el fichero en otra máquina, o la clave perdida): no se abre, y se borra.
	if _, err := guardarFichero(guardada); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(rutaClave, bytes.Repeat([]byte{7}, longitudSecret), 0o600); err != nil {
		t.Fatal(err)
	}
	if got, _, err := cargarFichero(); err == nil || got != (Tokens{}) {
		t.Fatalf("la sesión se ha abierto con otra clave: %+v", got)
	}
	if _, err := os.Stat(rutaSesion); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("el fichero que no se abre sigue ahí")
	}
}

// secretsFalso es un Secret Service vivo: solo contesta a ReadAlias, con la colección
// por defecto que tenga (`/` es ninguna, el KWallet de un Plasma recién instalado).
type secretsFalso struct{ porDefecto dbus.ObjectPath }

func (s secretsFalso) ReadAlias(string) (dbus.ObjectPath, *dbus.Error) { return s.porDefecto, nil }

// TestSinLlaveroLaSesionVaAlFicheroYVuelveAlLlaveroEnCuantoLoHay: sin Secret Service,
// Save la deja en el fichero y avisa UNA vez; con un Secret Service vivo que dice que no
// (bloqueado, la persona canceló), NO va a disco; y con el llavero de vuelta, la sesión
// se muda a él al LEERLA y el fichero se borra; salir borra también la clave. Un
// Secret Service sin colección por defecto (KWallet sin cartera) cuenta como ninguno. VISTO EN
// ROJO cayendo al fichero ante cualquier error del llavero, sin mudarla al leer, dando
// siempre por buena la del fichero y sin borrar la clave al salir.
func TestSinLlaveroLaSesionVaAlFicheroYVuelveAlLlaveroEnCuantoLoHay(t *testing.T) {
	llavero := busprueba.Nuevo(t)
	rutaSesion, _ := carpetasDePrueba(t)
	avisos := 0
	SinLlavero = func() { avisos++ }
	defer func() { SinLlavero = nil }()

	keyring.MockInitWithError(errors.New("org.freedesktop.DBus.Error.ServiceUnknown"))
	sesion := Tokens{Refresh: "r1", Subject: "ada"}
	for range 2 {
		if err := Save(sesion); err != nil {
			t.Fatalf("sin llavero, Save: %v", err)
		}
	}
	if avisos != 1 {
		t.Fatalf("el aviso de «sin llavero» ha salido %d veces", avisos)
	}
	if got, err := Load(); err != nil || got != sesion {
		t.Fatalf("sin llavero, Load = %+v, %v", got, err)
	}

	// Un Secret Service vivo que no deja guardar: el error llega y el fichero no cambia.
	// Un KWallet sin cartera todavía: contesta, pero no hay dónde guardar. Es como no
	// tener llavero.
	if err := llavero.Export(secretsFalso{"/"}, "/org/freedesktop/secrets", "org.freedesktop.Secret.Service"); err != nil {
		t.Fatal(err)
	}
	if _, err := llavero.RequestName("org.freedesktop.secrets", 0); err != nil {
		t.Fatal(err)
	}
	if !sinLlavero() {
		t.Fatal("un Secret Service sin colección por defecto cuenta como llavero: la sesión no se guardaría nunca")
	}
	if err := llavero.Export(secretsFalso{"/org/freedesktop/secrets/collection/login"}, "/org/freedesktop/secrets", "org.freedesktop.Secret.Service"); err != nil {
		t.Fatal(err)
	}
	if err := Save(Tokens{Refresh: "r2", Subject: "ada"}); err == nil {
		t.Fatal("con el llavero bloqueado, la sesión se ha ido a disco sin decir nada")
	}
	if got, _, _ := cargarFichero(); got != sesion {
		t.Fatalf("con el llavero bloqueado, el fichero ha cambiado: %+v", got)
	}

	// El llavero vuelve: Load devuelve lo del fichero (es lo último) y lo muda en el acto.
	keyring.MockInit()
	if got, err := Load(); err != nil || got != sesion {
		t.Fatalf("con llavero nuevo y vacío, Load = %+v, %v", got, err)
	}
	if _, err := os.Stat(rutaSesion); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("con la sesión ya en el llavero, el fichero cifrado sigue en disco")
	}
	if crudo, err := keyring.Get(llaveroServicio, llaveroCuenta); err != nil || !strings.Contains(crudo, `"r1"`) {
		t.Fatalf("la sesión del fichero no ha llegado al llavero: %q, %v", crudo, err)
	}
	if err := Save(Tokens{Refresh: "r3", Subject: "ada"}); err != nil {
		t.Fatal(err)
	}
	if got, err := Load(); err != nil || got.Refresh != "r3" {
		t.Fatalf("Load = %+v, %v", got, err)
	}

	// Con llavero, un fichero escrito después (una sesión por ssh, sin bus) es el bueno.
	if _, err := guardarFichero(Tokens{Refresh: "r4", Subject: "ada"}); err != nil {
		t.Fatal(err)
	}
	if got, err := Load(); err != nil || got.Refresh != "r4" {
		t.Fatalf("con un fichero más nuevo que el llavero, Load = %+v, %v", got, err)
	}

	// Y al revés: un fichero que no se pudo borrar es VIEJO frente al llavero, y no vuelve.
	if _, err := guardarFichero(Tokens{Refresh: "r5-viejo", Subject: "ada"}); err != nil {
		t.Fatal(err)
	}
	nuevo, _ := serializar(Tokens{Refresh: "r6", Subject: "ada"})
	if err := keyring.Set(llaveroServicio, llaveroCuenta, string(nuevo)); err != nil {
		t.Fatal(err)
	}
	if got, err := Load(); err != nil || got.Refresh != "r6" {
		t.Fatalf("un fichero viejo ha ganado al llavero: Load = %+v, %v", got, err)
	}
	if _, err := os.Stat(rutaSesion); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("el fichero viejo sigue en disco")
	}

	// Salir borra la sesión, el fichero y su clave.
	if _, err := guardarFichero(sesion); err != nil {
		t.Fatal(err)
	}
	if err := Save(Tokens{}); err != nil {
		t.Fatal(err)
	}
	_, rutaClave, _ := rutasDelFichero()
	for _, ruta := range []string{rutaSesion, rutaClave} {
		if _, err := os.Stat(ruta); !errors.Is(err, os.ErrNotExist) {
			t.Errorf("tras salir, %s sigue en disco", ruta)
		}
	}
}

// TestLaSesionDelFicheroAguantaLoQueDejaElDisco: una clave vacía (un corte de luz) o de
// otro tamaño se cambia por una nueva al guardar en vez de dejar la sesión sin recordar
// para siempre; una carpeta que ya existía abierta se cierra a 0700; un enlace en lugar
// del sobre no se sigue, ni se queda colgado en una tubería; sin la clave, el sobre no se
// borra; una carpeta enlazada vale; dos apps que guardan a la vez por primera vez no se pisan la
// clave; y con llavero, leer no crea carpetas. VISTO EN ROJO sin regenerar la clave, sin
// cerrar la carpeta, siguiendo enlaces, creando la clave con rename, creando carpetas al
// leer, borrando el sobre sin clave, sin O_NONBLOCK y con Lstat en la carpeta.
func TestLaSesionDelFicheroAguantaLoQueDejaElDisco(t *testing.T) {
	rutaSesion, rutaClave := carpetasDePrueba(t)
	sesion := Tokens{Refresh: "r", Subject: "ada"}

	// Con llavero y sin nada guardado, Load no crea ninguna carpeta.
	keyring.MockInit()
	if _, err := Load(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Dir(rutaClave)); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("leer la sesión con llavero ha creado la carpeta de la clave")
	}

	if err := os.MkdirAll(filepath.Dir(rutaSesion), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(rutaClave), 0o700); err != nil {
		t.Fatal(err)
	}
	for _, clave := range [][]byte{{}, []byte("corta")} {
		if err := os.WriteFile(rutaClave, clave, 0o600); err != nil {
			t.Fatal(err)
		}
		if _, err := guardarFichero(sesion); err != nil {
			t.Fatalf("con una clave de %d bytes no se guarda: %v", len(clave), err)
		}
		if got, _, err := cargarFichero(); err != nil || got != sesion {
			t.Fatalf("con la clave regenerada, cargarFichero = %+v, %v", got, err)
		}
	}
	if info, _ := os.Stat(filepath.Dir(rutaSesion)); info.Mode().Perm() != 0o700 {
		t.Errorf("la carpeta de la sesión sigue con %v", info.Mode().Perm())
	}

	// Un enlace donde iba el sobre: no se lee lo que apunta, y el enlace se quita.
	// Apunta a un sobre BUENO: seguirlo daría una sesión que no es la de este fichero.
	fuera := filepath.Join(t.TempDir(), "otro")
	bueno, err := os.ReadFile(rutaSesion)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(fuera, bueno, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(rutaSesion); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(fuera, rutaSesion); err != nil {
		t.Fatal(err)
	}
	if _, _, err := cargarFichero(); err == nil {
		t.Fatal("se ha seguido un enlace en lugar del sobre")
	}
	if _, err := os.Lstat(rutaSesion); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("el enlace sigue donde iba el sobre")
	}
	if _, err := os.Stat(fuera); err != nil {
		t.Fatal("se ha borrado el fichero al que apuntaba el enlace")
	}

	// Sin la clave (otra app la está poniendo), el sobre no se abre pero NO se borra.
	if _, err := guardarFichero(sesion); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(rutaClave, rutaClave+".aparte"); err != nil {
		t.Fatal(err)
	}
	if _, _, err := cargarFichero(); err == nil {
		t.Fatal("el sobre se ha abierto sin clave")
	}
	if _, err := os.Stat(rutaSesion); err != nil {
		t.Fatal("sin la clave, se ha borrado el sobre que la otra app acababa de escribir")
	}
	if err := os.Rename(rutaClave+".aparte", rutaClave); err != nil {
		t.Fatal(err)
	}

	// Una tubería donde iba el sobre: no se queda colgado leyéndola.
	if err := os.Remove(rutaSesion); err != nil {
		t.Fatal(err)
	}
	if err := syscall.Mkfifo(rutaSesion, 0o600); err != nil {
		t.Fatal(err)
	}
	hecho := make(chan error, 1)
	go func() { _, _, err := cargarFichero(); hecho <- err }()
	select {
	case err := <-hecho:
		if err == nil {
			t.Fatal("una tubería se ha tomado por la sesión")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("leer la sesión se ha quedado colgado en una tubería")
	}

	// Una carpeta de configuración enlazada (stow): vale, si al otro lado es tuya.
	destino := t.TempDir()
	config := filepath.Dir(filepath.Dir(rutaSesion))
	if err := os.RemoveAll(filepath.Dir(rutaSesion)); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(destino, filepath.Join(config, "sytue")); err != nil {
		t.Fatal(err)
	}
	if _, err := guardarFichero(sesion); err != nil {
		t.Fatalf("con la carpeta enlazada no se guarda: %v", err)
	}

	// Otra app ya ha puesto la clave: la segunda en llegar se queda con la suya y no la
	// cambia (sería dejar sin abrir lo que la primera cifró).
	if err := os.Remove(rutaClave); err != nil {
		t.Fatal(err)
	}
	var claves [2][]byte
	for i := range claves {
		if err := crearClave(rutaClave, false); err != nil {
			t.Fatal(err)
		}
		claves[i], _ = os.ReadFile(rutaClave)
	}
	if len(claves[0]) != longitudSecret || !bytes.Equal(claves[0], claves[1]) {
		t.Fatal("una segunda app ha cambiado la clave que acababa de poner la primera")
	}
}

// secretsLento es un Secret Service colgado.
type secretsLento struct{}

func (secretsLento) ReadAlias(string) (dbus.ObjectPath, *dbus.Error) {
	time.Sleep(time.Second)
	return "/", nil
}

// TestUnLlaveroColgadoNoEsUnLlaveroQueNoEsta: si el Secret Service no contesta en su
// plazo, no se cae al fichero (sería escribir a disco con un llavero delante), y la
// pregunta no pasa de ese plazo, que es menos que lo que dura el cerrojo de la sesión.
// VISTO EN ROJO tomando el plazo vencido por «no hay llavero».
func TestUnLlaveroColgadoNoEsUnLlaveroQueNoEsta(t *testing.T) {
	llavero := busprueba.Nuevo(t)
	if err := llavero.Export(secretsLento{}, "/org/freedesktop/secrets", "org.freedesktop.Secret.Service"); err != nil {
		t.Fatal(err)
	}
	if _, err := llavero.RequestName("org.freedesktop.secrets", 0); err != nil {
		t.Fatal(err)
	}
	defer func(p time.Duration) { plazoLlavero = p }(plazoLlavero)
	plazoLlavero = 300 * time.Millisecond
	antes := time.Now()
	if sinLlavero() {
		t.Error("un llavero colgado cuenta como ninguno: la sesión iría a disco")
	}
	if d := time.Since(antes); d > 800*time.Millisecond {
		t.Errorf("preguntar al llavero ha tardado %v", d)
	}
}
