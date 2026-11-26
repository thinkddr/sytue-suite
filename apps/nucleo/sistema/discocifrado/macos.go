// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package discocifrado

import (
	"context"
	"regexp"
	"strings"
)

// # macOS
//
// El volumen de la carpeta sale de `statfs` (su punto de montaje y su dispositivo).
//
//   - El disco de arranque (`/` o `/System/Volumes/Data`, donde están las carpetas de la
//     persona): `fdesetup isactive`, que escribe «true» o «false» sin ser administrador.
//   - Otro volumen (un disco externo): `diskutil info -plist <dispositivo>`, con las
//     claves `FileVault` (APFS cifrado) y `CoreStorageEncrypted` (los de antes de APFS).
//     Un exFAT, FAT o NTFS no se cifran nunca así.
//
// En un Mac con Apple silicon el disco interno va cifrado por hardware siempre, pero sin
// FileVault la clave no depende de la contraseña: se dice «sin cifrar», que es lo que
// importa para X-10.

// entornoMac es lo que se pregunta al sistema; las pruebas lo dan grabado.
type entornoMac struct {
	// volumen es el punto de montaje y el dispositivo (`/dev/disk3s5`) de ruta.
	volumen func(ruta string) (punto, dispositivo string, err error)
	orden   orden
}

// arranque son los puntos de montaje del disco de arranque.
var arranque = map[string]bool{"/": true, "/System/Volumes/Data": true}

func deMac(ctx context.Context, ruta string, e entornoMac) Resultado {
	punto, disp, err := e.volumen(ruta)
	if err != nil {
		return noSeSabe
	}
	if arranque[punto] {
		salida, _ := e.orden(ctx, "fdesetup", "isactive")
		switch strings.TrimSpace(salida) {
		case "true":
			return cifradoCon("FileVault")
		case "false":
			return sinCifrar
		}
		return noSeSabe
	}
	if !strings.HasPrefix(disp, "/dev/") {
		// Un volumen de red o una imagen sin dispositivo de bloques.
		return noSeSabe
	}
	salida, err := e.orden(ctx, "diskutil", "info", "-plist", disp)
	if err != nil {
		return noSeSabe
	}
	return leerDiskutil(salida)
}

var (
	plistBool   = regexp.MustCompile(`<key>(FileVault|CoreStorageEncrypted|Encryption)</key>\s*<(true|false)/>`)
	plistTipoFS = regexp.MustCompile(`<key>FilesystemType</key>\s*<string>([^<]*)</string>`)
)

// leerDiskutil lee el plist de `diskutil info -plist`.
func leerDiskutil(plist string) Resultado {
	visto := false
	for _, m := range plistBool.FindAllStringSubmatch(plist, -1) {
		visto = true
		if m[2] == "true" {
			return cifradoCon("FileVault")
		}
	}
	if visto {
		return sinCifrar
	}
	if m := plistTipoFS.FindStringSubmatch(plist); m != nil {
		switch strings.ToLower(m[1]) {
		case "exfat", "msdos", "ntfs":
			return sinCifrar
		}
	}
	return noSeSabe
}
