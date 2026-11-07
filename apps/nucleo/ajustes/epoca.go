// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package ajustes

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"os"
	"path/filepath"
	"strings"
)

// # La época de identidad
//
// `sesion.marca` cambia en CADA guardado de la sesión, y Sytue ID rota el refresh en
// cada renovación: cada ~15 min, aunque nadie cambie de cuenta. Lo que se publica para
// los widgets nativos no puede atarse a eso, porque una captura hecha antes de renovar
// dejaba de valer y la publicación se perdía en silencio (ADR 0078, enmienda del
// 2026-11-06). La ÉPOCA es la marca que solo cambia cuando cambia QUIÉN: otra persona,
// otra organización, otro emisor o cliente, o el cierre de la sesión.
//
// `sesion.epoca` son cuatro líneas, sin nada en claro:
//
//	época       128 bits al azar, en hexadecimal
//	huella      SHA-256(época ‖ emisor ‖ cliente ‖ sujeto)
//	huella org  SHA-256(época ‖ organización)
//	confirmada  la `sesion.marca` que había al escribirla
//
// La época hace de sal: la huella no deja adivinar el sujeto. La organización va en su
// propia huella para poder conservarla cuando llega vacía con el mismo sujeto (un
// vacío transitorio no es un cambio de organización). `confirmada` dice que la última
// sesión la escribió o adoptó una compilación que ya conoce la época; si no coincide
// con la marca, quien publica falla cerrado.

const ficheroEpoca = "sesion.epoca"

// Identidad es a quién pertenece la sesión guardada. La vacía es «no hay sesión».
type Identidad struct {
	Emisor, Cliente, Sujeto, Organizacion string
}

type epocaGuardada struct {
	epoca, huella, huellaOrg, confirmada string
}

// MarcarIdentidad apunta la época de la identidad de la sesión que se acaba de guardar
// y la confirma con la marca actual. La época solo rota si cambia la identidad; con la
// identidad vacía (cierre de sesión) rota siempre.
//
// Quien llama TIENE `CerrojoSesion`: la llaman `auth.Save` y `AlmacenSesion.Guardar`,
// que se ejecutan siempre bajo él (Compartida, AdoptarCon, Logout y Migrar lo toman
// antes de guardar), y la suite al adoptar una sesión. Tomarlo aquí otra vez
// bloquearía al propio proceso: el cerrojo del sistema no es reentrante entre
// descriptores. La escritura es atómica (EscribirEntero), así que un lector nunca ve
// el fichero a medias.
func MarcarIdentidad(id Identidad) error {
	dir, err := Carpeta()
	if err != nil {
		return err
	}
	marca := Leer().Sesion
	anterior, _ := leerEpoca(dir)
	nueva := anterior
	switch {
	case id.Sujeto == "", anterior.epoca == "",
		huellaIdentidad(anterior.epoca, id) != anterior.huella,
		id.Organizacion != "" && huellaOrganizacion(anterior.epoca, id.Organizacion) != anterior.huellaOrg:
		var azar [16]byte
		if _, err := rand.Read(azar[:]); err != nil {
			return err
		}
		nueva.epoca = hex.EncodeToString(azar[:])
		nueva.huella = huellaIdentidad(nueva.epoca, id)
		nueva.huellaOrg = huellaOrganizacion(nueva.epoca, id.Organizacion)
	}
	nueva.confirmada = marca
	if nueva == anterior {
		return nil
	}
	contenido := strings.Join([]string{nueva.epoca, nueva.huella, nueva.huellaOrg, nueva.confirmada}, "\n") + "\n"
	return EscribirEntero(filepath.Join(dir, ficheroEpoca), []byte(contenido))
}

func huellaIdentidad(epoca string, id Identidad) string {
	if id.Sujeto == "" {
		return ""
	}
	suma := sha256.Sum256([]byte(epoca + "\x00" + id.Emisor + "\x00" + id.Cliente + "\x00" + id.Sujeto))
	return hex.EncodeToString(suma[:])
}

func huellaOrganizacion(epoca, org string) string {
	suma := sha256.Sum256([]byte(epoca + "\x00org\x00" + org))
	return hex.EncodeToString(suma[:])
}

// EpocaLegible dice si la época que da Leer es la de verdad: nil si `sesion.epoca` se
// ha leído, si no existe (nadie ha entrado con una compilación que la escriba) o si está
// mal formada; el error si está ahí y no se deja leer. Leer da la época vacía en los
// tres casos, y quien BORRA por ella tiene que distinguirlos: en iOS, con el teléfono
// bloqueado, el fichero es NSFileProtectionComplete y da EPERM, y una época vacía «por
// no poder leer» no es «no hay sesión» (ADR 0078, enmienda del 2026-11-06).
func EpocaLegible() error {
	dir, err := Carpeta()
	if err != nil {
		return err
	}
	_, err = leerEpoca(dir)
	if err == nil || errors.Is(err, os.ErrNotExist) || errors.Is(err, os.ErrInvalid) {
		return nil
	}
	return err
}

// leerEpoca devuelve la época guardada, o la vacía si falta o está mal formada.
func leerEpoca(dir string) (epocaGuardada, error) {
	crudo, err := os.ReadFile(filepath.Join(dir, ficheroEpoca))
	if err != nil {
		return epocaGuardada{}, err
	}
	lineas := strings.Split(strings.TrimSuffix(string(crudo), "\n"), "\n")
	if len(lineas) != 4 || len(lineas[0]) != 32 {
		return epocaGuardada{}, os.ErrInvalid
	}
	return epocaGuardada{epoca: lineas[0], huella: lineas[1], huellaOrg: lineas[2], confirmada: lineas[3]}, nil
}
