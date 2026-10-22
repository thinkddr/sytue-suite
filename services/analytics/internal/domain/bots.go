// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package domain

import (
	_ "embed"
	"strings"
)

// botSignatures son las firmas de bots empotradas (ver bots.txt para el formato
// y la transformación aplicada).
//
// ATRIBUCIÓN (obligatoria, va también al NOTICE del repo): derivado de
// crawler-user-agents — Copyright (c) Martin Monperrus y contribuyentes,
// licencia MIT — https://github.com/monperrus/crawler-user-agents
//
//go:embed bots.txt
var botSignatures string

// IsBot indica si el user-agent es de un crawler, monitor o previsualizador.
// Se comprueba ANTES DE CUALQUIER ESCRITURA y el evento se DESCARTA, no se
// marca: marcarlo sería guardar más.
//
// Techo honesto y escrito: el filtrado por user-agent lo derrota cualquiera que
// quiera derrotarlo. Quita la basura sincera —que es la que arruina los números
// de un sitio pequeño—, no a un atacante. El filtrado por rangos de datacenter
// (ASN) queda fuera hasta que un cliente reporte números inflados desde nubes.
func IsBot(userAgent string) bool {
	// Sin user-agent no hay navegador: ningún navegador omite la cabecera.
	if strings.TrimSpace(userAgent) == "" {
		return true
	}
	return bots.match(strings.ToLower(userAgent))
}

// bots se construye una vez al cargar el paquete (~1 ms sobre 1500 firmas).
var bots = newBotSet(botSignatures)

// botSet indexa las firmas por su bigrama inicial. Con eso, comprobar un
// user-agent cuesta una pasada sobre sus ~120 bytes en vez de 1500 búsquedas de
// subcadena: 2,5 µs frente a los 3,4 ms de una alternancia RE2 equivalente, que
// es la diferencia entre filtrar bots y no poder permitírselo por petición.
type botSet struct {
	byGram   map[uint16][]string
	prefixes []string
	suffixes []string
}

func newBotSet(data string) *botSet {
	b := &botSet{byGram: make(map[uint16][]string, 2048)}
	for _, line := range strings.Split(data, "\n") {
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		switch {
		case strings.HasPrefix(line, "^"):
			b.prefixes = append(b.prefixes, line[1:])
		case strings.HasSuffix(line, "$"):
			b.suffixes = append(b.suffixes, line[:len(line)-1])
		case len(line) >= 2:
			g := uint16(line[0])<<8 | uint16(line[1])
			b.byGram[g] = append(b.byGram[g], line)
		}
	}
	return b
}

// match espera el user-agent YA en minúsculas (las firmas lo están).
func (b *botSet) match(ua string) bool {
	for _, p := range b.prefixes {
		if strings.HasPrefix(ua, p) {
			return true
		}
	}
	for _, s := range b.suffixes {
		if strings.HasSuffix(ua, s) {
			return true
		}
	}
	for i := 0; i+1 < len(ua); i++ {
		for _, n := range b.byGram[uint16(ua[i])<<8|uint16(ua[i+1])] {
			if len(ua)-i >= len(n) && ua[i:i+len(n)] == n {
				return true
			}
		}
	}
	return false
}
