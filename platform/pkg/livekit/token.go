// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

// Package livekit integra el SFU de medios LiveKit (Apache-2.0) por sus
// interfaces PÚBLICAS, sin portar su código (clean-room, CONVENTIONS §12):
//
//   - Minteo del JWT de acceso a sala (RFC 7519, HS256 firmado con el API
//     secret), con los grants de vídeo de LiveKit en el claim "video".
//   - Cliente HTTP mínimo de la API de servidor (RoomService) para crear/listar/
//     cerrar rooms, autenticado con el mismo JWT (grant roomCreate/roomAdmin).
//   - Verificación de la firma de los webhooks (cabecera Authorization: un JWT
//     HS256 cuyo claim "sha256" debe coincidir con el hash del cuerpo).
//
// Solo usa la librería estándar de Go (crypto/hmac, crypto/sha256, encoding/...)
// y dependencias permisivas ya presentes en el módulo. No depende del SDK de
// LiveKit ni de ningún código AGPL.
package livekit

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

// VideoGrant son los permisos de medios de LiveKit codificados en el JWT
// (subconjunto de su esquema público). Los nombres JSON son los que LiveKit
// espera; no inventamos formato.
type VideoGrant struct {
	// Room es el nombre de la sala a la que se concede acceso.
	Room string `json:"room,omitempty"`
	// RoomJoin autoriza a unirse a la sala.
	RoomJoin bool `json:"roomJoin,omitempty"`
	// RoomCreate autoriza a crear salas (uso del control plane).
	RoomCreate bool `json:"roomCreate,omitempty"`
	// RoomList autoriza a listar salas.
	RoomList bool `json:"roomList,omitempty"`
	// RoomAdmin autoriza a administrar la sala (mutear, expulsar, metadata).
	RoomAdmin bool `json:"roomAdmin,omitempty"`
	// RoomRecord autoriza a la API de Egress a grabar. Va SOLO en los tokens de
	// servidor que mintea el control plane para arrancar o parar una grabación:
	// jamás en el de un participante, o cualquiera con su token podría grabar la
	// reunión de los demás desde fuera.
	RoomRecord bool `json:"roomRecord,omitempty"`
	// IngressAdmin autoriza a la API de Ingress (crear, cambiar y borrar las
	// puertas RTMP). Solo en tokens de servidor: quien lo tuviera en un token de
	// participante podría redirigir la emisión de cualquiera a su propia room.
	IngressAdmin bool `json:"ingressAdmin,omitempty"`
	// CanPublish / CanSubscribe / CanPublishData gobiernan las pistas de medios.
	CanPublish     *bool `json:"canPublish,omitempty"`
	CanSubscribe   *bool `json:"canSubscribe,omitempty"`
	CanPublishData *bool `json:"canPublishData,omitempty"`
}

// claims es el cuerpo del JWT de LiveKit (claims registrados RFC 7519 + "video").
type claims struct {
	Issuer    string     `json:"iss"`
	Subject   string     `json:"sub,omitempty"`
	Name      string     `json:"name,omitempty"`
	NotBefore int64      `json:"nbf,omitempty"`
	ExpiresAt int64      `json:"exp"`
	Video     VideoGrant `json:"video"`
	Metadata  string     `json:"metadata,omitempty"`
	// SHA256 es el hash base64 del cuerpo del webhook (solo en webhooks, no en
	// los tokens de acceso); se incluye aquí para reusar el verificador.
	SHA256 string `json:"sha256,omitempty"`
}

// AccessTokenOptions configura un JWT de acceso a sala para un participante.
type AccessTokenOptions struct {
	// Identity es la identidad del participante (sub de Sytue ID). Va en "sub".
	Identity string
	// Name es el nombre legible mostrado en la sala.
	Name string
	// Room es el nombre de la sala (room de LiveKit).
	Room string
	// Grant son los permisos de medios concedidos.
	Grant VideoGrant
	// Metadata es metadata opaca del participante (JSON serializado).
	Metadata string
	// TTL es la validez del token.
	TTL time.Duration
}

// Signer mintea y verifica JWT HS256 con el par api_key/api_secret de LiveKit.
type Signer struct {
	apiKey string
	secret []byte
}

// NewSigner construye un Signer. El api_secret no abandona nunca el servidor.
func NewSigner(apiKey, apiSecret string) *Signer {
	return &Signer{apiKey: apiKey, secret: []byte(apiSecret)}
}

// b64url codifica en base64url sin padding (formato JWT).
func b64url(b []byte) string { return base64.RawURLEncoding.EncodeToString(b) }

// signJWT serializa y firma un JWT HS256 con los claims dados. El "iss" del JWT
// es el api_key de LiveKit (así el servidor localiza el secreto de verificación).
func (s *Signer) signJWT(c claims) (string, error) {
	c.Issuer = s.apiKey
	header := map[string]string{"alg": "HS256", "typ": "JWT"}
	hb, err := json.Marshal(header)
	if err != nil {
		return "", fmt.Errorf("livekit: cabecera JWT: %w", err)
	}
	pb, err := json.Marshal(c)
	if err != nil {
		return "", fmt.Errorf("livekit: claims JWT: %w", err)
	}
	signingInput := b64url(hb) + "." + b64url(pb)
	mac := hmac.New(sha256.New, s.secret)
	mac.Write([]byte(signingInput))
	sig := b64url(mac.Sum(nil))
	return signingInput + "." + sig, nil
}

// AccessToken mintea el JWT que el cliente web entrega a LiveKit para unirse a
// la sala. Se llama SOLO tras validar el JWT de Sytue ID y autorizar al usuario.
func (s *Signer) AccessToken(opt AccessTokenOptions) (string, error) {
	if opt.Room == "" || opt.Identity == "" {
		return "", fmt.Errorf("livekit: room e identity son obligatorios")
	}
	ttl := opt.TTL
	if ttl <= 0 {
		ttl = 6 * time.Hour
	}
	now := time.Now()
	grant := opt.Grant
	grant.Room = opt.Room
	grant.RoomJoin = true
	return s.signJWT(claims{
		Subject:   opt.Identity,
		Name:      opt.Name,
		NotBefore: now.Add(-30 * time.Second).Unix(),
		ExpiresAt: now.Add(ttl).Unix(),
		Video:     grant,
		Metadata:  opt.Metadata,
	})
}

// serverToken mintea un JWT efímero con grants de administración para autenticar
// las llamadas a la API de servidor (RoomService) de LiveKit.
func (s *Signer) serverToken(grant VideoGrant) (string, error) {
	now := time.Now()
	return s.signJWT(claims{
		NotBefore: now.Add(-30 * time.Second).Unix(),
		ExpiresAt: now.Add(10 * time.Minute).Unix(),
		Video:     grant,
	})
}

// SignWebhook firma un cuerpo de webhook como lo haría el servidor de LiveKit:
// un JWT HS256 con el claim "sha256" = hash del cuerpo. Es la contraparte de
// ParseWebhook y existe para que quien consume webhooks pueda probarlos de
// extremo a extremo sin copiar la firma en cada paquete.
func (s *Signer) SignWebhook(body []byte, ttl time.Duration) (string, error) {
	if ttl <= 0 {
		ttl = time.Minute
	}
	sum := sha256.Sum256(body)
	return s.signJWT(claims{
		ExpiresAt: time.Now().Add(ttl).Unix(),
		SHA256:    base64.StdEncoding.EncodeToString(sum[:]),
	})
}

// verifyWebhook valida la firma del JWT de webhook y comprueba que su claim
// "sha256" coincide con el hash del cuerpo recibido. Devuelve nil si es válido.
func (s *Signer) verifyWebhook(authToken string, body []byte) error {
	parts := strings.Split(authToken, ".")
	if len(parts) != 3 {
		return fmt.Errorf("livekit: JWT de webhook malformado")
	}
	signingInput := parts[0] + "." + parts[1]
	mac := hmac.New(sha256.New, s.secret)
	mac.Write([]byte(signingInput))
	expected := b64url(mac.Sum(nil))
	// Comparación en tiempo constante para no filtrar la firma.
	if !hmac.Equal([]byte(expected), []byte(parts[2])) {
		return fmt.Errorf("livekit: firma de webhook inválida")
	}

	payload, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return fmt.Errorf("livekit: payload de webhook ilegible: %w", err)
	}
	var c claims
	if err := json.Unmarshal(payload, &c); err != nil {
		return fmt.Errorf("livekit: claims de webhook ilegibles: %w", err)
	}
	if c.ExpiresAt != 0 && time.Now().Unix() > c.ExpiresAt {
		return fmt.Errorf("livekit: JWT de webhook expirado")
	}
	// El cuerpo debe corresponder al hash firmado (anti-replay/integridad).
	sum := sha256.Sum256(body)
	if c.SHA256 != base64.StdEncoding.EncodeToString(sum[:]) {
		return fmt.Errorf("livekit: hash del cuerpo del webhook no coincide")
	}
	return nil
}
