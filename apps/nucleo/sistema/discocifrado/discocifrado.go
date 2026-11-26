// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

// Package discocifrado dice si una carpeta está en un disco cifrado, donde el sistema lo
// deja saber SIN privilegios y SIN red. Es el X-10 de `apps/fyne/PLAN-EDICION-LOCAL.md`:
// Sytue Office guarda los datos en claro (R-CLARO) y su protección en reposo es la del
// cifrado de disco del sistema, así que Ajustes → Datos dice si lo hay.
//
// Tres respuestas: cifrado, sin cifrar o no se sabe. Lo que no se puede mirar sin ser
// administrador, o no se entiende, es «no se sabe»: nunca se dice «cifrado» a ciegas.
//
//   - Linux (linux.go): el montaje de la carpeta en `/proc/self/mountinfo` y su
//     dispositivo en `/sys`: un `dm-crypt` (LUKS o no) en la cadena —también debajo de
//     un LVM— es cifrado; fscrypt (el atributo «cifrado» de `statx`), eCryptfs, ZFS con
//     cifrado nativo y los FUSE que cifran (gocryptfs, CryFS, EncFS, securefs) también.
//     Sin `/sys`, `lsblk -s`. Red, memoria y capas (NFS, tmpfs, overlay…): no se sabe.
//   - macOS (macos.go): el disco de arranque por `fdesetup isactive` (FileVault); otro
//     volumen, por `diskutil info -plist`.
//   - Windows (windows.go): BitLocker por la propiedad del Explorador
//     `System.Volume.BitLockerProtection`, que se lee sin elevar (`manage-bde`, WMI y
//     `Get-BitLockerVolume` piden administrador). Sin ella, no se sabe.
//   - Android e iOS (movil.go): los dos cifran siempre el almacenamiento de la app.
//
// Cada sistema tiene su fichero con la lógica, sin etiqueta de compilación, para que sus
// pruebas corran en cualquiera con entradas grabadas; lo que toca el sistema de verdad
// (leer `/proc`, `statfs`, lanzar una orden) está en los `sistema_*.go`.
package discocifrado

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"time"
)

// Estado es lo que se sabe del cifrado del disco.
type Estado int

const (
	// NoSeSabe: el sistema no deja saberlo sin privilegios, o no se entiende lo que dice.
	NoSeSabe Estado = iota
	// Cifrado: el disco (o la carpeta) está cifrado.
	Cifrado
	// SinCifrar: no lo está.
	SinCifrar
)

// Resultado es el estado y, si está cifrado, con qué.
type Resultado struct {
	Estado Estado
	// Como es el cifrado («LUKS», «dm-crypt», «fscrypt», «eCryptfs», «ZFS», «gocryptfs»,
	// «FileVault», «BitLocker», «Android», «iOS»); vacío si no lo está o no se sabe.
	Como string
}

func cifradoCon(como string) Resultado { return Resultado{Estado: Cifrado, Como: como} }

var (
	sinCifrar = Resultado{Estado: SinCifrar}
	noSeSabe  = Resultado{Estado: NoSeSabe}
)

// De mira el disco de la carpeta ruta. Puede lanzar órdenes del sistema y tardar un
// poco (hasta `espera`): se llama fuera del hilo de la interfaz.
func De(ctx context.Context, ruta string) Resultado {
	abs, err := filepath.Abs(ruta)
	if err != nil {
		return noSeSabe
	}
	return deSistema(ctx, existente(abs))
}

// existente es ruta o, si aún no existe (una carpeta de datos que se va a crear), la
// carpeta más cercana por encima que sí: es la que dice en qué disco caerá.
func existente(ruta string) string {
	for {
		if _, err := os.Stat(ruta); err == nil {
			if r, err := filepath.EvalSymlinks(ruta); err == nil {
				return r
			}
			return ruta
		}
		arriba := filepath.Dir(ruta)
		if arriba == ruta {
			return ruta
		}
		ruta = arriba
	}
}

// orden lanza una orden del sistema y devuelve lo que escribe; en las pruebas, una
// salida grabada. Devuelve la salida AUNQUE la orden acabe con error (`fdesetup
// isactive` dice «false» y sale con 1).
type orden func(ctx context.Context, nombre string, args ...string) (string, error)

// espera es lo más que se espera a una orden del sistema.
const espera = 15 * time.Second

// ordenDelSistema lanza la orden de verdad, con su ventana oculta donde la tendría.
func ordenDelSistema(ctx context.Context, nombre string, args ...string) (string, error) {
	ctx, parar := context.WithTimeout(ctx, espera)
	defer parar()
	cmd := exec.CommandContext(ctx, nombre, args...) //nolint:gosec // G204: órdenes fijas de este paquete; los argumentos son rutas y unidades del propio equipo
	ocultarVentana(cmd)
	salida, err := cmd.Output()
	return string(salida), err
}
