// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package domain

import (
	"regexp"
	"strings"
	"time"

	"github.com/google/uuid"
)

// SLASH COMMANDS de terceros: «/desplegar prod» ejecuta código de otro y contesta.
//
// Sin esto, a un bot solo se le puede hablar mencionándolo: no hay forma de pedirle algo
// con un verbo. El payload y la firma son los de Slack, así que un comando ya escrito
// para Slack funciona pegando su URL.

// SlashCommand es un comando registrado por la organización.
type SlashCommand struct {
	ID     uuid.UUID `json:"id"`
	Tenant string    `json:"-"`
	// Name va SIN la barra y en minúsculas.
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	UsageHint   string `json:"usage_hint,omitempty"`
	RequestURL  string `json:"request_url"`
	// Token viaja en cada ejecución y por eso NO prueba el origen; para eso está
	// SigningSecret, que no sale nunca.
	Token         string    `json:"token,omitempty"`
	SigningSecret string    `json:"signing_secret,omitempty"`
	CreatedBy     string    `json:"created_by,omitempty"`
	CreatedAt     time.Time `json:"created_at"`
}

// slashNameRe es la MISMA forma que impone la restricción de la tabla: empieza por
// letra, sigue con minúsculas, dígitos, guion o guion bajo, y no pasa de 32.
var slashNameRe = regexp.MustCompile(`^[a-z][a-z0-9_-]{0,31}$`)

// builtinSlashCommands son los comandos INTEGRADOS del cliente
// (apps/frontend/apps/rooms/src/lib/commands.ts). Registrar uno con el mismo nombre lo
// dejaría inalcanzable —el cliente lo resuelve antes de preguntar al servidor— y quien
// lo registró vería un comando que no se ejecuta nunca. Mejor rechazarlo al crearlo.
var builtinSlashCommands = map[string]struct{}{
	"shrug": {}, "me": {}, "away": {}, "online": {}, "dnd": {}, "status": {},
	"leave": {}, "topic": {}, "header": {}, "purpose": {}, "invite": {}, "msg": {},
	"dm": {}, "search": {}, "help": {},
}

// NormalizeSlashName recorta, quita la barra inicial y pasa a minúsculas. Devuelve ""
// si el nombre no vale o pisa un comando integrado.
func NormalizeSlashName(raw string) string {
	n := strings.ToLower(strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(raw), "/")))
	if !slashNameRe.MatchString(n) {
		return ""
	}
	if _, choca := builtinSlashCommands[n]; choca {
		return ""
	}
	return n
}

// SplitSlashInput separa «/nombre resto» en sus dos partes. El nombre sale normalizado
// para comparar; el resto se entrega TAL CUAL, porque es el argumento del comando y
// tocarlo cambiaría lo que su autor escribió.
func SplitSlashInput(input string) (name, args string) {
	s := strings.TrimSpace(input)
	if !strings.HasPrefix(s, "/") {
		return "", ""
	}
	s = s[1:]
	if i := strings.IndexAny(s, " \t\n"); i >= 0 {
		name, args = s[:i], strings.TrimSpace(s[i+1:])
	} else {
		name = s
	}
	name = strings.ToLower(strings.TrimSpace(name))
	if !slashNameRe.MatchString(name) {
		return "", ""
	}
	return name, args
}

// Respuestas que puede dar un comando. `ephemeral` la ve SOLO quien lo ejecutó; es el
// valor por defecto a propósito: un comando que consulta algo no tiene por qué llenar el
// canal para todos.
const (
	SlashResponseEphemeral = "ephemeral"
	SlashResponseInChannel = "in_channel"
)

// NormalizeSlashResponseType acepta lo que declare el comando y cae a efímero ante
// cualquier otra cosa. Publicar en el canal por error es lo irreversible de los dos.
func NormalizeSlashResponseType(raw string) string {
	if strings.EqualFold(strings.TrimSpace(raw), SlashResponseInChannel) {
		return SlashResponseInChannel
	}
	return SlashResponseEphemeral
}
