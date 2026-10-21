// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

// Package crypto cifra/descifra en reposo los secretos de Sytue Cloud
// (variables marcadas como secreto y credenciales de servicios gestionados),
// usando AES-256-GCM con clave de SECRET_ENC_KEY. CONVENTIONS §10: cifrado de
// campos sensibles; secretos nunca en claro en logs ni respuestas.
//
// Implementación 100 % stdlib (crypto/aes, crypto/cipher) — sin dependencias.
//
// # ANILLO DE CLAVES: rotar la KEK sin parar el servicio
//
// SECRET_ENC_KEY admite varias claves versionadas, no solo una:
//
//	SECRET_ENC_KEY="1:<base64>,2:<base64>"
//
// La versión MÁS ALTA es la PRIMARIA: con ella se cifra todo lo nuevo. Las demás
// se conservan SOLO para descifrar. Una clave suelta sin prefijo (el formato
// histórico, y el de cualquier despliegue que nunca haya rotado) equivale a
// "1:<base64>", así que nada existente cambia de comportamiento.
//
// Ese anillo es lo que hace que rotar no exija ventana de parada: mientras la
// clave vieja siga en él, TODA lectura funciona sea cual sea la versión con la
// que se cifró la fila, y el re-sellado (store.RotateSecretKeys) puede ir
// pasando los datos a la nueva en lotes, en caliente. La secuencia completa es
// añadir la clave nueva → esperar a que el censo no muestre la vieja → quitar la
// vieja del anillo.
package crypto

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"slices"
	"strconv"
	"strings"
)

// ErrCiphertextInvalid indica un texto cifrado corrupto o con clave incorrecta.
var ErrCiphertextInvalid = errors.New("crypto: texto cifrado inválido")

// MaxKeyVersion acota la versión al rango de la columna `key_version` (smallint).
const MaxKeyVersion = 32767

// Sealer cifra y descifra valores con AES-256-GCM sobre un anillo de claves.
type Sealer struct {
	// version y aead son la KEK PRIMARIA: la única con la que se cifra.
	version int
	aead    cipher.AEAD
	// prev son las KEK anteriores del anillo, en orden descendente. Solo
	// descifran: existen para que una rotación a medias no rompa lecturas.
	prev []cipher.AEAD
}

// NewSealer construye un Sealer a partir de SECRET_ENC_KEY, que puede ser una
// clave suelta en base64 (std o url-safe, ⇒ versión 1) o un anillo versionado
// "1:<base64>,2:<base64>". Cada clave decodificada debe tener 32 bytes (AES-256)
// y la versión más alta pasa a ser la primaria.
func NewSealer(spec string) (*Sealer, error) {
	type entry struct {
		version int
		aead    cipher.AEAD
	}
	var ring []entry
	for _, part := range strings.Split(spec, ",") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		version, raw := 1, part
		// El base64 (std o url-safe) no usa ':' ni ',', así que el troceado es
		// inequívoco y no hace falta escapar nada.
		if i := strings.IndexByte(part, ':'); i >= 0 {
			v, err := strconv.Atoi(strings.TrimSpace(part[:i]))
			if err != nil || v < 1 || v > MaxKeyVersion {
				return nil, fmt.Errorf("crypto: versión de clave inválida %q (se espera 1..%d)", part[:i], MaxKeyVersion)
			}
			version, raw = v, strings.TrimSpace(part[i+1:])
		}
		if slices.ContainsFunc(ring, func(e entry) bool { return e.version == version }) {
			return nil, fmt.Errorf("crypto: versión de clave %d repetida en SECRET_ENC_KEY", version)
		}
		aead, err := newAEAD(raw)
		if err != nil {
			return nil, err
		}
		ring = append(ring, entry{version: version, aead: aead})
	}
	if len(ring) == 0 {
		return nil, errors.New("crypto: SECRET_ENC_KEY vacía")
	}
	// Descendente: la primaria delante, y Open prueba de la más nueva a la más
	// vieja (que es el orden en que están casi todas las filas tras rotar).
	slices.SortFunc(ring, func(a, b entry) int { return b.version - a.version })
	s := &Sealer{version: ring[0].version, aead: ring[0].aead}
	for _, e := range ring[1:] {
		s.prev = append(s.prev, e.aead)
	}
	return s, nil
}

// newAEAD valida una clave en base64 y devuelve su AES-256-GCM.
func newAEAD(keyB64 string) (cipher.AEAD, error) {
	key, err := decodeKey(keyB64)
	if err != nil {
		return nil, err
	}
	if len(key) != 32 {
		return nil, fmt.Errorf("crypto: la clave debe ser de 32 bytes (AES-256), tiene %d", len(key))
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, fmt.Errorf("crypto: cipher AES: %w", err)
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("crypto: GCM: %w", err)
	}
	return aead, nil
}

// Version es la versión de la KEK primaria: la que se persiste en `key_version`
// al re-sellar, y con la que se compara para saber qué filas quedan por rotar.
func (s *Sealer) Version() int { return s.version }

// Seal cifra plaintext y devuelve nonce||ciphertext||tag.
func (s *Sealer) Seal(plaintext []byte) ([]byte, error) {
	nonce := make([]byte, s.aead.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return nil, fmt.Errorf("crypto: generar nonce: %w", err)
	}
	// Prefija el nonce; Seal añade el tag de autenticación al final.
	return s.aead.Seal(nonce, nonce, plaintext, nil), nil
}

// SealString es un atajo para cifrar una cadena.
func (s *Sealer) SealString(plaintext string) ([]byte, error) {
	return s.Seal([]byte(plaintext))
}

// Open descifra un valor producido por Seal con CUALQUIER clave del anillo.
//
// El formato sellado no lleva marca de versión a propósito: el tag de
// autenticación de GCM ya distingue la clave correcta (una equivocada falla
// siempre, no «casi siempre»), así que probar por orden es tan seguro como leer
// una etiqueta y no obliga a migrar el formato de los blobs ya escritos. El
// anillo tiene 1-2 claves en la práctica, de modo que el coste es despreciable.
func (s *Sealer) Open(blob []byte) ([]byte, error) {
	ns := s.aead.NonceSize()
	if len(blob) < ns {
		return nil, ErrCiphertextInvalid
	}
	nonce, ct := blob[:ns], blob[ns:]
	if plaintext, err := s.aead.Open(nil, nonce, ct, nil); err == nil {
		return plaintext, nil
	}
	for _, prev := range s.prev {
		if plaintext, err := prev.Open(nil, nonce, ct, nil); err == nil {
			return plaintext, nil
		}
	}
	return nil, ErrCiphertextInvalid
}

// Reseal descifra con el anillo y vuelve a cifrar con la KEK primaria. Es la
// operación que ejecuta la rotación fila a fila; devolver error aborta el lote
// (mejor eso que persistir un secreto que ya nadie sabrá abrir).
func (s *Sealer) Reseal(blob []byte) ([]byte, error) {
	plaintext, err := s.Open(blob)
	if err != nil {
		return nil, err
	}
	return s.Seal(plaintext)
}

// OpenString descifra un valor a cadena.
func (s *Sealer) OpenString(blob []byte) (string, error) {
	p, err := s.Open(blob)
	if err != nil {
		return "", err
	}
	return string(p), nil
}

// decodeKey acepta base64 estándar o url-safe, con o sin padding.
func decodeKey(s string) ([]byte, error) {
	for _, enc := range []*base64.Encoding{
		base64.StdEncoding, base64.RawStdEncoding,
		base64.URLEncoding, base64.RawURLEncoding,
	} {
		if b, err := enc.DecodeString(s); err == nil {
			return b, nil
		}
	}
	return nil, fmt.Errorf("crypto: SECRET_ENC_KEY no es base64 válido")
}
