// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package service

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"

	"git.sytue.eu/sytue/one/services/cloud/internal/domain"
	"git.sytue.eu/sytue/one/services/cloud/internal/store"
)

// TokenPrefix identifica los tokens de API de Sytue Cloud (para distinguirlos de
// un JWT de sesión en el middleware de auth y para que sean reconocibles/scaneables
// si se filtran). El secreto que sigue al prefijo es aleatorio de 256 bits.
const TokenPrefix = "scl_"

// tokenSecretBytes es la entropía del secreto (256 bits) tras el prefijo.
const tokenSecretBytes = 32

// GenerateAPIToken produce un token en claro (prefijo + secreto aleatorio) y su
// hash SHA-256 (hex) para persistir. El claro solo se muestra una vez; en reposo
// vive únicamente el hash. Es la contraparte de HashAPIToken.
func GenerateAPIToken() (plaintext, hashHex string, err error) {
	buf := make([]byte, tokenSecretBytes)
	if _, err := rand.Read(buf); err != nil {
		return "", "", fmt.Errorf("service: generar token: %w", err)
	}
	plaintext = TokenPrefix + base64.RawURLEncoding.EncodeToString(buf)
	return plaintext, HashAPIToken(plaintext), nil
}

// HashAPIToken devuelve el hash SHA-256 (hex) del token en claro. Determinista:
// el mismo claro produce el mismo hash, que es la clave de búsqueda del token.
func HashAPIToken(plaintext string) string {
	sum := sha256.Sum256([]byte(plaintext))
	return hex.EncodeToString(sum[:])
}

// CreateTokenInput son los datos para emitir un token de API.
type CreateTokenInput struct {
	Name      string
	Scope     domain.TokenScope
	CreatedBy string
	ExpiresAt *time.Time
	// ProjectID (opcional) confina el token a un proyecto. Si != nil se valida que
	// el proyecto exista EN EL TENANT activo (GetProject bajo RLS); nil = tenant-wide.
	ProjectID *uuid.UUID
}

// CreateAPIToken emite un token del tenant activo y devuelve el claro UNA vez
// (domain.APIToken.Plaintext). En la BD solo queda el hash.
func (s *Service) CreateAPIToken(ctx context.Context, in CreateTokenInput) (domain.APIToken, error) {
	in.Name = strings.TrimSpace(in.Name)
	if in.Name == "" {
		return domain.APIToken{}, fmt.Errorf("%w: nombre de token requerido", domain.ErrInvalidInput)
	}
	if in.Scope == "" {
		in.Scope = domain.ScopeRead
	}
	if !domain.ValidTokenScope(in.Scope) {
		return domain.APIToken{}, fmt.Errorf("%w: scope inválido (read|write|deploy|hook)", domain.ErrInvalidInput)
	}
	// Una URL de despliegue publica UN proyecto: sin confinar publicaría todos.
	if in.Scope == domain.ScopeHook && in.ProjectID == nil {
		return domain.APIToken{}, fmt.Errorf("%w: un token hook va confinado a un proyecto", domain.ErrInvalidInput)
	}
	if in.ExpiresAt != nil && !in.ExpiresAt.After(time.Now()) {
		return domain.APIToken{}, fmt.Errorf("%w: expires_at debe ser futuro", domain.ErrInvalidInput)
	}
	// Confinamiento por-proyecto: el proyecto debe existir EN EL TENANT activo.
	// GetProject corre bajo RLS, así que un project_id de otro tenant (o inexistente)
	// devuelve ErrNotFound — impide emitir un token que apunte fuera del tenant.
	if in.ProjectID != nil {
		if _, err := s.store.GetProject(ctx, *in.ProjectID); err != nil {
			return domain.APIToken{}, err
		}
	}
	plaintext, hashHex, err := GenerateAPIToken()
	if err != nil {
		return domain.APIToken{}, err
	}
	tok, err := s.store.CreateAPIToken(ctx, store.NewAPITokenRecord{
		Name:      in.Name,
		TokenHash: hashHex,
		Scope:     in.Scope,
		CreatedBy: in.CreatedBy,
		ExpiresAt: in.ExpiresAt,
		ProjectID: in.ProjectID,
	})
	if err != nil {
		return domain.APIToken{}, err
	}
	tok.Plaintext = plaintext // se muestra una sola vez
	// Emisión de credencial: se registra el QUÉ (nombre, alcance), jamás el secreto
	// ni su hash — el registro de auditoría no puede convertirse en un almacén de
	// credenciales.
	s.audit(ctx, "token.create", tok.ID.String(), map[string]any{
		"name": tok.Name, "scope": string(tok.Scope),
	})
	return tok, nil
}

// ListAPITokens lista los tokens del tenant (metadatos; nunca el secreto).
func (s *Service) ListAPITokens(ctx context.Context) ([]domain.APIToken, error) {
	return s.store.ListAPITokens(ctx)
}

// RevokeAPIToken revoca (borra) un token del tenant.
func (s *Service) RevokeAPIToken(ctx context.Context, id uuid.UUID) error {
	if err := s.store.DeleteAPIToken(ctx, id); err != nil {
		return err
	}
	// Fuera de la cache AHORA, no cuando venza el TTL. Revocar un token es la
	// operación que se hace cuando se sospecha que está en malas manos: un minuto
	// de gracia es justo lo que no se puede conceder ahí.
	s.invalidarToken(ctx, id)
	s.audit(ctx, "token.revoke", id.String(), nil)
	return nil
}

// invalidarToken echa de la cache la resolución de un token por su UUID.
func (s *Service) invalidarToken(ctx context.Context, id uuid.UUID) {
	if !s.cache.Enabled() {
		return
	}
	hash, found, err := s.cache.GetSigned(ctx, tokenIDKey(id))
	if err != nil || !found {
		return
	}
	_ = s.cache.Del(ctx, tokenCacheKey(hash), tokenIDKey(id))
}

// ResolveAPIToken valida un token en claro: comprueba el prefijo, calcula el hash
// y lo resuelve a tenant+scope (no expirado) contra la BD. Devuelve ErrNotFound
// si el token es desconocido/expirado (el llamante responde 401 sin distinguir
// para no filtrar cuáles existen).
func (s *Service) ResolveAPIToken(ctx context.Context, plaintext string) (store.ResolvedToken, error) {
	// Comparación en tiempo constante del prefijo (defensa en profundidad; el
	// prefijo no es secreto, pero mantiene el patrón uniforme).
	if subtle.ConstantTimeCompare([]byte(plaintext[:min(len(plaintext), len(TokenPrefix))]), []byte(TokenPrefix)) != 1 {
		return store.ResolvedToken{}, domain.ErrNotFound
	}
	hash := HashAPIToken(plaintext)
	if v, ok := s.tokenCacheado(ctx, hash); ok {
		return v, nil
	}
	rt, err := s.store.ResolveAPIToken(ctx, hash)
	if err != nil {
		// Solo se cachea el acierto. Cachear el «no existe» convertiría un token
		// recién creado en un 401 durante todo el TTL.
		return rt, err
	}
	s.cachearToken(ctx, hash, rt)
	return rt, nil
}

// tokenTTL es de un minuto: cachear la resolución de un token es cachear el «sí»
// a una petición autenticada, y ese es el retraso máximo con el que surte efecto
// una revocación por caducidad. Las revocaciones explícitas no esperan al TTL,
// invalidan (ver RevokeAPIToken).
const tokenTTL = time.Minute

// tokenCacheKey lleva el HASH del token, nunca el token en claro: la clave viaja
// en la cache y en sus registros. Y es el hash COMPLETO, porque es lo que separa
// un token de otro: con un prefijo, dos tokens podrían colisionar y un cliente
// hablaría en nombre de otra organización.
func tokenCacheKey(hash string) string { return "cloud:token:" + hash }

// tokenCacheado devuelve la resolución guardada, si la hay.
func (s *Service) tokenCacheado(ctx context.Context, hash string) (store.ResolvedToken, bool) {
	if !s.cache.Enabled() {
		return store.ResolvedToken{}, false
	}
	// GetSigned y no Get: el valor se devuelve como IDENTIDAD AUTENTICADA sin
	// volver a la base, así que solo vale si lo sellamos nosotros. Un valor sin
	// sello, con sello ajeno o copiado desde otra clave se trata como si no
	// estuviera y se resuelve contra PostgreSQL.
	raw, found, err := s.cache.GetSigned(ctx, tokenCacheKey(hash))
	if err != nil || !found {
		return store.ResolvedToken{}, false
	}
	var rt store.ResolvedToken
	if json.Unmarshal([]byte(raw), &rt) != nil {
		// Un valor ilegible se ignora y se resuelve contra la base: es lo que
		// pasaría con la cache apagada, que es el comportamiento correcto.
		return store.ResolvedToken{}, false
	}
	return rt, true
}

// cachearToken guarda la resolución. Un fallo aquí no cambia la respuesta: la
// siguiente petición vuelve a preguntar a la base.
func (s *Service) cachearToken(ctx context.Context, hash string, rt store.ResolvedToken) {
	if !s.cache.Enabled() {
		return
	}
	b, err := json.Marshal(rt)
	if err != nil {
		return
	}
	_ = s.cache.SetSignedTTL(ctx, tokenCacheKey(hash), string(b), tokenTTL)
	// Rastro inverso id→hash. Sin él, revocar sería imposible de propagar: la
	// revocación conoce el UUID del token y la cache está indexada por su hash,
	// que no se puede derivar del UUID (ni al revés, que es justamente el punto).
	// El rastro inverso también va sellado: envenenarlo haría que una revocación
	// borrase la entrada equivocada y dejara viva la del token revocado.
	_ = s.cache.SetSignedTTL(ctx, tokenIDKey(rt.ID), hash, tokenTTL)
}

// tokenIDKey indexa el hash por el UUID del token, que es lo que conoce quien
// revoca. Caduca a la vez que la entrada principal: si se evicta antes, la
// revocación no la encuentra y el token sigue admitido hasta que venza su TTL de
// un minuto. Ese es el peor caso, y es el motivo de que el TTL sea corto.
func tokenIDKey(id uuid.UUID) string { return "cloud:token-id:" + id.String() }
