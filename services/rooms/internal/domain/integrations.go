// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package domain

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"net/url"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"
)

// Ola 15 · Integraciones. Tres identidades sintéticas publican en canales:
//   - webhooks ENTRANTES (autor hook:<id>): token opaco en la URL /hooks/{token}.
//   - webhooks SALIENTES: POST SSRF-safe a un callback cuando casa un mensaje; su
//     respuesta {text} se publica de vuelta (autor hook:<id>).
//   - BOTS (autor bot:<id>): token Bearer bot_… con alcance mínimo (publicar).
//
// El prefijo del autor (hook:/bot:) es la señal de «mensaje de integración» para el
// badge BOT: un sub de usuario real de Sytue ID nunca empieza así.

// Prefijos del author_sub sintético de las integraciones.
const (
	authorPrefixHook = "hook:"
	authorPrefixBot  = "bot:"
	// BotTokenPrefix precede al token Bearer de un bot; el middleware de auth lo usa
	// para distinguir un token de bot de un JWT de Sytue ID sin intentar validarlo.
	BotTokenPrefix = "bot_"
)

// IsBotAuthor indica si un author_sub corresponde a una integración (webhook o
// bot), es decir, si el mensaje debe llevar el badge «BOT». Deriva del prefijo del
// autor sintético: no requiere columna ni migración en `messages`.
func IsBotAuthor(sub string) bool {
	return strings.HasPrefix(sub, authorPrefixHook) || strings.HasPrefix(sub, authorPrefixBot)
}

// HookAuthorSub es el author_sub sintético de un webhook (entrante o la respuesta
// de uno saliente).
func HookAuthorSub(id uuid.UUID) string { return authorPrefixHook + id.String() }

// BotAuthorSub es el author_sub sintético (identidad) de un bot.
func BotAuthorSub(id uuid.UUID) string { return authorPrefixBot + id.String() }

// IncomingWebhook es un webhook entrante ligado a un canal. El token NO se
// almacena ni se expone tras la creación (solo su hash vive en la BD).
type IncomingWebhook struct {
	ID        uuid.UUID `json:"id"`
	Tenant    string    `json:"-"`
	ChannelID uuid.UUID `json:"channel_id"`
	Name      string    `json:"name"`
	Icon      string    `json:"icon,omitempty"`
	CreatedBy string    `json:"created_by,omitempty"`
	CreatedAt time.Time `json:"created_at"`
}

// OutgoingWebhook es un webhook saliente: dispara un POST a CallbackURL cuando un
// mensaje casa (canal y/o palabra disparadora). Token es el de VERIFICACIÓN que
// viaja en la cabecera del POST (no autentica hacia nosotros; se puede mostrar).
type OutgoingWebhook struct {
	ID           uuid.UUID  `json:"id"`
	Tenant       string     `json:"-"`
	ChannelID    *uuid.UUID `json:"channel_id,omitempty"`
	TriggerWords []string   `json:"trigger_words,omitempty"`
	CallbackURL  string     `json:"callback_url"`
	Token        string     `json:"token,omitempty"`
	// Dialect es el formato del POST y de la respuesta: "sytue" (por defecto),
	// "slack" (form-urlencoded estilo outgoing webhook de Slack) o "teams" (la
	// actividad de Bot Framework). Lo valida el servicio con compat.NormalizeOutgoing;
	// el dominio solo lo transporta.
	Dialect string `json:"dialect,omitempty"`
	// SigningSecret firma cada entrega (X-Slack-Signature o Authorization: HMAC).
	// A diferencia de Token, NUNCA viaja: es lo que hace que la firma pruebe algo.
	// Se guarda en base64 porque Slack lo usa como cadena y Teams lo decodifica.
	SigningSecret string    `json:"signing_secret,omitempty"`
	CreatedBy     string    `json:"created_by,omitempty"`
	CreatedAt     time.Time `json:"created_at"`
}

// Bot es una identidad del tenant con token Bearer propio. El token NO se almacena
// ni se expone tras la creación (solo su hash).
type Bot struct {
	ID        uuid.UUID `json:"id"`
	Tenant    string    `json:"-"`
	Name      string    `json:"name"`
	OwnerSub  string    `json:"owner_sub,omitempty"`
	CreatedAt time.Time `json:"created_at"`
}

// Topes de las integraciones (evitan abuso a escala).
const (
	maxTriggerWords   = 20
	maxTriggerWordLen = 64
	maxCallbackURLLen = 2048
)

// HashToken devuelve el SHA-256 hex de un token: lo que se persiste. La
// verificación rehashea el token entrante y compara con la columna (comparación de
// digests de longitud fija; el índice único cubre el lookup por igualdad).
func HashToken(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}

// newSecret genera 32 bytes aleatorios (256 bits) en base64url sin relleno
// (URL-safe: cabe en /hooks/{token} sin escapar).
func newSecret() (string, error) {
	var b [32]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b[:]), nil
}

// NewOpaqueToken genera un token opaco (webhook entrante / verificación de
// saliente) y su hash. El claro solo se devuelve una vez.
func NewOpaqueToken() (plain, hash string, err error) {
	plain, err = newSecret()
	if err != nil {
		return "", "", err
	}
	return plain, HashToken(plain), nil
}

// NewSigningSecret genera el secreto con el que se firma cada entrega saliente.
//
// En base64 ESTÁNDAR, no en base64url como el resto: Teams decodifica su clave de
// seguridad con el alfabeto estándar, y así el MISMO valor sirve para pegarlo tal cual
// en un bot de Slack y para que uno de Teams lo decodifique.
func NewSigningSecret() (string, error) {
	var b [32]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", err
	}
	return base64.StdEncoding.EncodeToString(b[:]), nil
}

// NewBotToken genera un token de bot (con prefijo BotTokenPrefix) y su hash.
func NewBotToken() (plain, hash string, err error) {
	s, err := newSecret()
	if err != nil {
		return "", "", err
	}
	plain = BotTokenPrefix + s
	return plain, HashToken(plain), nil
}

// NormalizeTriggerWords sanea las palabras disparadoras de un webhook saliente:
// recorta, pasa a minúsculas, descarta vacías/duplicadas y acota número y longitud.
func NormalizeTriggerWords(words []string) []string {
	seen := make(map[string]struct{}, len(words))
	out := make([]string, 0, len(words))
	for _, w := range words {
		w = strings.ToLower(strings.TrimSpace(w))
		if w == "" || utf8.RuneCountInString(w) > maxTriggerWordLen {
			continue
		}
		if _, dup := seen[w]; dup {
			continue
		}
		seen[w] = struct{}{}
		out = append(out, w)
		if len(out) >= maxTriggerWords {
			break
		}
	}
	return out
}

// ValidCallbackURL valida la FORMA del callback de un webhook saliente: http/https
// y con host. La barrera SSRF real (resolución DNS + rechazo de IPs privadas) se
// aplica en el momento del POST, no aquí: el DNS puede cambiar entre la creación y
// el envío (rebinding), así que validar la IP al crear no protegería.
func ValidCallbackURL(raw string) bool {
	if raw == "" || len(raw) > maxCallbackURLLen {
		return false
	}
	u, err := url.Parse(raw)
	if err != nil {
		return false
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return false
	}
	return u.Hostname() != ""
}

// MatchesOutgoing indica si un mensaje del canal channelID con cuerpo body dispara
// el webhook saliente w. Reglas (estilo Mattermost):
//   - Si w está ligado a un canal, debe coincidir; si no (channel NULL), aplica a
//     cualquiera pero EXIGE palabras disparadoras (lo garantiza la validación al
//     crear: un saliente sin canal ni palabras no puede existir).
//   - Sin palabras disparadoras (solo canal): dispara con cualquier mensaje del canal.
//   - Con palabras: dispara si el cuerpo EMPIEZA por una de ellas (primera palabra),
//     como el trigger por defecto de Mattermost.
func MatchesOutgoing(channelID uuid.UUID, body string, w OutgoingWebhook) bool {
	if w.ChannelID != nil && *w.ChannelID != channelID {
		return false
	}
	if len(w.TriggerWords) == 0 {
		// Solo válido si está ligado a un canal (garantía de la validación al crear).
		return w.ChannelID != nil
	}
	return MatchedTrigger(body, w.TriggerWords) != ""
}

// MatchedTrigger devuelve la palabra disparadora que casa con el cuerpo (o ""). La
// usa MatchesOutgoing y el dialecto Slack del saliente, que envía `trigger_word`.
func MatchedTrigger(body string, words []string) string {
	lower := strings.ToLower(strings.TrimSpace(body))
	for _, tw := range words {
		if lower == tw || strings.HasPrefix(lower, tw+" ") || strings.HasPrefix(lower, tw+"\n") {
			return tw
		}
	}
	return ""
}

// ValidBotName valida el nombre de un bot (reusa ValidName: no vacío, <=100, sin
// control). Envuelto por claridad en el call site.
func ValidBotName(name string) bool { return ValidName(name) }
