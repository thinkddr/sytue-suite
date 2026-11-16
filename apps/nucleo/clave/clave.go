// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

// Package clave guarda y comprueba contraseñas de recurso (enlaces públicos de Drive y de
// Docs) con Argon2id en formato PHC.
//
// POR QUÉ EXISTE. Drive tenía su propia copia («`internal` impide compartir el código entre
// servicios, así que se replica el perfil») y Docs iba a necesitar otra. Una contraseña de
// enlace es una frontera de seguridad: que dos servicios la comprueben con parámetros
// distintos, o que uno se endurezca y el otro no, es justo lo que no debe pasar. Los
// parámetros son los de Sytue ID (OWASP, perfil de servidor); cambiar uno aquí cambia los
// dos servicios, y los hashes ya guardados siguen verificando porque llevan los suyos.
package clave

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"fmt"
	"strings"

	"golang.org/x/crypto/argon2"
)

const (
	tiempo   = 3
	memoria  = 64 * 1024 // 64 MiB
	hilos    = 2
	largoKey = 32
	largoSal = 16
)

// ErrVacia es la contraseña vacía: no se guarda ni se acepta.
var ErrVacia = errors.New("clave: contraseña vacía")

// Hash devuelve el hash Argon2id en formato PHC de una contraseña. Nunca se guarda en claro.
func Hash(password string) (string, error) {
	if password == "" {
		return "", ErrVacia
	}
	sal := make([]byte, largoSal)
	if _, err := rand.Read(sal); err != nil {
		return "", fmt.Errorf("clave: leer aleatoriedad: %w", err)
	}
	key := argon2.IDKey([]byte(password), sal, tiempo, memoria, hilos, largoKey)
	b64 := base64.RawStdEncoding.EncodeToString
	return fmt.Sprintf("$argon2id$v=%d$m=%d,t=%d,p=%d$%s$%s",
		argon2.Version, memoria, tiempo, hilos, b64(sal), b64(key)), nil
}

// Verificar compara una contraseña con un hash PHC en tiempo constante.
func Verificar(password, codificado string) (bool, error) {
	if password == "" || codificado == "" {
		return false, nil
	}
	partes := strings.Split(codificado, "$")
	// ["", "argon2id", "v=19", "m=...,t=...,p=...", sal, hash]
	if len(partes) != 6 || partes[1] != "argon2id" {
		return false, errors.New("clave: hash de contraseña con formato desconocido")
	}
	var version int
	if _, err := fmt.Sscanf(partes[2], "v=%d", &version); err != nil {
		return false, err
	}
	var mem, t uint32
	var p uint8
	if _, err := fmt.Sscanf(partes[3], "m=%d,t=%d,p=%d", &mem, &t, &p); err != nil {
		return false, err
	}
	sal, err := base64.RawStdEncoding.DecodeString(partes[4])
	if err != nil {
		return false, err
	}
	want, err := base64.RawStdEncoding.DecodeString(partes[5])
	if err != nil {
		return false, err
	}
	// Un hash legítimo mide 32 bytes; se acota para descartar PHC corruptos y el
	// desbordamiento int→uint32 (gosec G115).
	if len(want) == 0 || len(want) > 512 {
		return false, errors.New("clave: hash de contraseña con longitud inválida")
	}
	got := argon2.IDKey([]byte(password), sal, t, mem, p, uint32(len(want))) //nolint:gosec // G115: len(want) acotado a (0,512].
	return subtle.ConstantTimeCompare(got, want) == 1, nil
}
