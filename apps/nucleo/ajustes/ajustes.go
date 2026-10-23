// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

// Package ajustes guarda lo que es TRANSVERSAL a todas las apps nativas de la suite y
// NO es un secreto: la apariencia y el idioma.
//
// # Por qué existe, y por qué no vale `fyne.Preferences`
//
// La suite instalada son TRECE procesos distintos y la persona espera que elegir tema
// oscuro en Sytue Mail lo cambie también en Drive. `fyne.Preferences` no sirve para eso,
// y no es una opinión: en `fyne.io/fyne/v2@v2.8.1/app/preferences_other.go:18` el
// almacén es `filepath.Join(app.RootConfigDir(), a.UniqueID())`, o sea una carpeta POR
// `ID` de `FyneApp.toml`. Trece apps, trece `preferences.json`, trece temas. En Android e
// iOS sí es una sola carpeta (`preferences_android.go:19`, «we are in a sandbox»), pero
// ahí también es una sola app instalada, así que tampoco comparte nada entre productos.
//
// Aquí hay UNA carpeta para toda la suite, con un fichero que las trece leen y escriben.
//
// # Lo que NO va aquí
//
//   - La SESIÓN. Es un secreto y va al llavero del sistema (apps/nucleo/auth). De ella
//     aquí solo queda una MARCA —un número, sin ningún valor— que dice «ha cambiado, id
//     a preguntarle al llavero».
//   - La PRESENCIA. Vive en el servidor porque la ven los demás; una copia local sería
//     una segunda verdad que se queda vieja en cuanto alguien la cambia desde la web o
//     desde el teléfono. Ver el ADR 0050.
//
// # La regla que ordena todo esto
//
// Un fichero de esta carpeta dice QUÉ ha cambiado, nunca guarda el valor de algo cuya
// verdad está en otro sitio. Por eso la sesión deja una marca y no un token, y por eso
// la presencia no deja ni marca.
package ajustes

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"time"
)

const (
	ficheroAjustes = "ajustes.json"
	ficheroSesion  = "sesion.marca"
	ficheroCerrojo = "cerrojo-v2"

	// permisoCarpeta y permisoFichero: son preferencias, no secretos, pero tampoco hay
	// motivo para que las lea otro usuario de la máquina.
	permisoCarpeta = 0o700
	permisoFichero = 0o600
)

// ErrSesionNoDisponible distingue la pérdida del broker de un fallo ordinario de
// preferencias. Quien tenga tokens en memoria debe olvidarlos y volver a identificar.
var ErrSesionNoDisponible = errors.New("ajustes: el almacén de sesión compartida no está disponible")

// Ajustes son las preferencias transversales de la persona.
type Ajustes struct {
	// Tema es "claro", "oscuro", "traslucido" o "" (el del sistema).
	Tema string `json:"tema,omitempty"`
	// Color es el color de la suite que eligió la persona: "petroleo" o uno de los que
	// declara la hoja de tokens ("cobalto", "app"), o "" si no ha elegido (y entonces
	// manda el de su organización y, si no, el de cada app). Es un eje aparte del tema
	// (ADR 0071).
	Color string `json:"color,omitempty"`
	// Forma es la forma de la suite que eligió la persona: "marco", "carril" o "" si no ha
	// elegido (y entonces manda la de su organización y, si no, Marco, la de la casa). Es
	// el tercer eje, aparte del tema y del color.
	Forma string `json:"forma,omitempty"`
	// Idioma es una etiqueta BCP-47 ("es-ES") o "" (el del sistema).
	Idioma string `json:"idioma,omitempty"`
	// Bloqueo pide al dueño que diga quién es (Touch ID, Windows Hello, la contraseña del
	// equipo) antes de que una app abra la sesión guardada. No es un secreto: lo que
	// guarda es la sesión, en el llavero; esto solo dice si se pregunta.
	Bloqueo bool `json:"bloqueo,omitempty"`
	// Invisible dice que la persona ELIGIÓ aparecer desconectada desde una de las apps de
	// este equipo. Hace falta porque el Hub no lo distingue: «desconectada» es también lo
	// que contesta de quien no ha latido en un rato, y sin esto la primera app que se
	// abría lo adoptaba y lo volvía a publicar, con la persona desconectada para todos
	// mientras trabajaba (visto en una ventana real el 2026-09-27).
	Invisible bool `json:"invisible,omitempty"`
	// SubirAlAbrir dice que la persona autorizó en este dispositivo subir al servicio los
	// ficheros del equipo que abre con Docus, Calcus, Diapos o CertiPDF desde el sistema
	// (FYP-02).
	SubirAlAbrir bool `json:"subir_al_abrir,omitempty"`
}

// Estado es todo lo transversal que otra app de la suite puede haber cambiado.
type Estado struct {
	Ajustes
	// Sesion es una marca OPACA: lo único que significa es que, si ha cambiado desde
	// la última vez, la sesión guardada en el llavero ya no es la que se leyó.
	Sesion string
}

// Carpeta es dónde vive todo esto, ya creada: `<config del usuario>/sytue`. En la
// variante de la Mac App Store, el contenedor de grupo de la suite (base_tienda_darwin.go).
func Carpeta() (string, error) {
	base, err := base()
	if err != nil {
		return "", err
	}
	dir := filepath.Join(base, "sytue")
	if err := os.MkdirAll(dir, permisoCarpeta); err != nil {
		return "", err
	}
	return dir, nil
}

// Leer devuelve lo transversal que haya guardado. Sin ajustes legibles, la suite
// abre con el tema claro y el idioma del sistema. Un fichero existente sin tema
// conserva la elección del sistema, que se serializa omitiendo ese campo.
func Leer() Estado {
	dir, err := Carpeta()
	if err != nil {
		return estadoPlataforma(Estado{Ajustes: Ajustes{Tema: "claro"}})
	}
	e := Estado{Ajustes: leerAjustes(dir)}
	if crudo, err := os.ReadFile(filepath.Join(dir, ficheroSesion)); err == nil {
		e.Sesion = string(crudo)
	}
	return estadoPlataforma(e)
}

func leerAjustes(dir string) Ajustes {
	var a Ajustes
	crudo, err := os.ReadFile(filepath.Join(dir, ficheroAjustes))
	if err != nil || json.Unmarshal(crudo, &a) != nil {
		return Ajustes{Tema: "claro"}
	}
	return a
}

// Cambiar aplica `cambio` a lo guardado y lo escribe, TODO bajo el cerrojo entre
// procesos: si Mail está poniendo el tema mientras Drive pone el idioma, no se pierde
// ninguno de los dos.
func Cambiar(cambio func(*Ajustes)) error { return CambiarCon(context.Background(), cambio) }

// CambiarCon es Cambiar con el plazo de quien llama para esperar el cerrojo.
func CambiarCon(ctx context.Context, cambio func(*Ajustes)) error {
	dir, err := Carpeta()
	if err != nil {
		return err
	}
	soltar, err := Cerrojo(ctx)
	if err != nil {
		return err
	}
	defer soltar()

	a := leerAjustes(dir)
	cambio(&a)
	crudo, err := json.Marshal(a)
	if err != nil {
		return err
	}
	return EscribirEntero(filepath.Join(dir, ficheroAjustes), crudo)
}

// MarcarSesion deja dicho que la sesión guardada ha cambiado. Lo llama auth.Save; no
// escribe NADA de la sesión, solo el instante en que cambió.
func MarcarSesion() {
	dir, err := Carpeta()
	if err != nil {
		return
	}
	// Sin leer-modificar-escribir a propósito: el valor no significa nada, solo que sea
	// distinto del anterior, así que un instante en nanosegundos basta y no hace falta
	// cerrojo para escribirlo.
	_ = EscribirEntero(filepath.Join(dir, ficheroSesion), []byte(strconv.FormatInt(time.Now().UnixNano(), 10)))
}

// Vigilar avisa cuando otra app cambia algo, y vuelve cuando ctx muere.
//
// `desde` es lo que quien llama YA leyó y aplicó. Se pide en vez de leerlo aquí porque
// entre el `Leer` del arranque y el primer latido de esta goroutine caben cambios de otra
// app, y leyéndolo aquí esos cambios se perdían sin que nadie se enterara: la app se
// quedaba con el tema viejo hasta el siguiente. Se vio como un `Vigilar` que no avisaba.
//
// ponytail: es un sondeo, no un `inotify`/`FSEvents`/`ReadDirectoryChangesW`. Son dos
// lecturas de menos de 100 bytes cada `cada`; con 2 s eso es una app entera por debajo
// del ruido de fondo, y a cambio no entra ni una dependencia ni una implementación por
// plataforma. El día que se mida que molesta, fsnotify.
func Vigilar(ctx context.Context, cada time.Duration, desde Estado, cambio func(Estado)) {
	anterior := desde
	reloj := time.NewTicker(cada)
	defer reloj.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-reloj.C:
			if ahora := Leer(); ahora != anterior {
				anterior = ahora
				cambio(ahora)
			}
		}
	}
}

// EscribirEntero deja el fichero completo o no lo toca: se escribe al lado, 0600, y se
// renombra. Lo usa también auth para la sesión cifrada sin llavero.
// Sin esto, quien esté vigilando puede leer un JSON a medio escribir y creer que se han
// borrado las preferencias.
func EscribirEntero(destino string, datos []byte) error {
	return escribirEntero(destino, datos, false)
}

func escribirEntero(destino string, datos []byte, soloNuevo bool) error {
	return escribirProtegido(destino, datos, soloNuevo, protegerFichero)
}

func escribirProtegido(destino string, datos []byte, soloNuevo bool, proteger func(string) error) error {
	tmp, err := os.CreateTemp(filepath.Dir(destino), filepath.Base(destino)+".*")
	if err != nil {
		return err
	}
	defer func() { _ = os.Remove(tmp.Name()) }() // si el rename salió bien, ya no existe
	// El temporal no contiene datos hasta que el sistema autoriza su protección.
	// En iOS también alcanza los snapshots y marcas del App Group compartido.
	if err := proteger(tmp.Name()); err != nil {
		_ = tmp.Close()
		return err
	}
	if _, err := tmp.Write(datos); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Chmod(permisoFichero); err != nil {
		_ = tmp.Close()
		return err
	}
	// A disco antes del rename: tras un corte de luz, el nombre no puede apuntar a un
	// fichero vacío.
	if err := tmp.Sync(); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if soloNuevo {
		if err := os.Link(tmp.Name(), destino); err != nil && !errors.Is(err, os.ErrExist) {
			return err
		}
		return nil
	}
	return os.Rename(tmp.Name(), destino)
}

// Cerrojo excluye procesos mediante un bloqueo del sistema operativo. El fichero es
// estable: no se borra ni caduca mientras otra app renueva. El sistema libera el
// bloqueo incluso si la app muere, sin confundir una red lenta con un proceso muerto.
func Cerrojo(ctx context.Context) (soltar func(), err error) {
	dir, err := Carpeta()
	if err != nil {
		return nil, err
	}
	f, err := os.OpenFile(filepath.Join(dir, ficheroCerrojo), os.O_CREATE|os.O_RDWR, permisoFichero)
	if err != nil {
		return nil, err
	}
	return CerrojoFichero(ctx, f)
}

// CerrojoSesion usa el mismo bloqueo del sistema que Cerrojo. En Android obtiene
// el inode del UID propietario del broker; las preferencias ordinarias siguen locales.
func CerrojoSesion(ctx context.Context) (soltar func(), err error) {
	f, compartido, err := abrirCerrojoSesion()
	if err != nil {
		return nil, errors.Join(ErrSesionNoDisponible, err)
	}
	if !compartido {
		return Cerrojo(ctx)
	}
	return CerrojoFichero(ctx, f)
}

// CerrojoFichero excludes processes on a caller-selected file using the same OS
// mechanism as the suite locks. It takes ownership of f: release or any error closes
// it. A separate file avoids coupling cache writes to OIDC/preferences locks.
func CerrojoFichero(ctx context.Context, f *os.File) (soltar func(), err error) {
	for {
		if err := ctx.Err(); err != nil {
			_ = f.Close()
			return nil, err
		}
		libre, err := intentarCerrojo(f)
		if err != nil {
			_ = f.Close()
			return nil, err
		}
		if libre {
			return func() { _ = f.Close() }, nil
		}
		select {
		case <-ctx.Done():
			_ = f.Close()
			return nil, ctx.Err()
		case <-time.After(20 * time.Millisecond):
		}
	}
}
