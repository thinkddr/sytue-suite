// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

// Package oidc implementa el núcleo OIDC/OAuth2 de Sytue ID: gestión de claves
// de firma (JWKS), emisión y validación de tokens (ID Token, access token JWT),
// y los flujos Authorization Code + PKCE, refresh_token y client_credentials.
//
// Clean-room: construido sobre OIDC Core 1.0, OAuth2 (RFC 6749/6750/7636),
// OIDC Discovery (RFC 8414) y JOSE (RFC 7515/7517/7519) con la librería
// permisiva lestrrat-go/jwx (MIT). No deriva de Keycloak ni de ningún motor AGPL
// (docs/CONVENTIONS.md §3, §12).
//
// AVISO DE SEGURIDAD: este núcleo de autenticación DEBE someterse a una auditoría
// de seguridad independiente ANTES de producción (docs/CONVENTIONS.md §0.7, §10).
package oidc

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/lestrrat-go/jwx/v2/jwa"
	"github.com/lestrrat-go/jwx/v2/jwk"

	"git.sytue.eu/sytue/one/platform/pkg/pg"
	"git.sytue.eu/sytue/one/services/id/internal/store"
)

// keyEntry es una clave de firma cargada en memoria.
type keyEntry struct {
	kid     string
	alg     jwa.SignatureAlgorithm
	private jwk.Key // clave privada (firma)
	public  jwk.Key // clave pública (JWKS)
	active  bool
}

// KeyManager mantiene en memoria el conjunto de claves de firma, con la clave
// 'active' usada para firmar y el resto publicadas en el JWKS para validar
// tokens emitidos antes de una rotación.
type KeyManager struct {
	mu     sync.RWMutex
	keys   []keyEntry
	active *keyEntry
}

// NewKeyManager construye un KeyManager vacío.
func NewKeyManager() *KeyManager { return &KeyManager{} }

// Bootstrap asegura que exista al menos una clave de firma activa. Si no hay
// ninguna en la base de datos, genera una RSA-2048 y la persiste. Después carga
// todas las claves no retiradas en memoria.
func (km *KeyManager) Bootstrap(ctx context.Context, st *store.Store) error {
	n, err := st.CountSigningKeys(ctx)
	if err != nil {
		return err
	}
	if n == 0 {
		if err := generateAndStore(ctx, st); err != nil {
			return err
		}
	}
	return km.Reload(ctx, st)
}

// generateAndStore crea una clave RSA-2048 activa y la persiste.
func generateAndStore(ctx context.Context, st *store.Store) error {
	priv, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		return fmt.Errorf("oidc: generar RSA: %w", err)
	}
	kid, err := randomKID()
	if err != nil {
		return err
	}
	pubJWK, err := jwk.FromRaw(priv.Public())
	if err != nil {
		return fmt.Errorf("oidc: JWK pública: %w", err)
	}
	_ = pubJWK.Set(jwk.KeyIDKey, kid)
	_ = pubJWK.Set(jwk.AlgorithmKey, jwa.RS256)
	_ = pubJWK.Set(jwk.KeyUsageKey, "sig")
	pubBytes, err := json.Marshal(pubJWK)
	if err != nil {
		return fmt.Errorf("oidc: serializar JWK: %w", err)
	}

	privDER := x509.MarshalPKCS1PrivateKey(priv)
	privPEM := pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: privDER})

	return st.InsertSigningKey(ctx, nil, store.SigningKeyRow{
		KID:        kid,
		Algorithm:  "RS256",
		PublicJWK:  pubBytes,
		PrivatePEM: string(privPEM),
		Status:     "active",
	})
}

// Reload recarga las claves desde la base de datos a memoria.
func (km *KeyManager) Reload(ctx context.Context, st *store.Store) error {
	rows, err := st.ListSigningKeys(ctx)
	if err != nil {
		return err
	}
	entries := make([]keyEntry, 0, len(rows))
	var active *keyEntry
	for i := range rows {
		r := rows[i]
		block, _ := pem.Decode([]byte(r.PrivatePEM))
		if block == nil {
			return fmt.Errorf("oidc: PEM inválido en kid %s", r.KID)
		}
		priv, err := x509.ParsePKCS1PrivateKey(block.Bytes)
		if err != nil {
			return fmt.Errorf("oidc: parsear clave privada kid %s: %w", r.KID, err)
		}
		privKey, err := jwk.FromRaw(priv)
		if err != nil {
			return fmt.Errorf("oidc: JWK privada kid %s: %w", r.KID, err)
		}
		_ = privKey.Set(jwk.KeyIDKey, r.KID)
		_ = privKey.Set(jwk.AlgorithmKey, jwa.RS256)

		pubKey, err := jwk.ParseKey(r.PublicJWK)
		if err != nil {
			return fmt.Errorf("oidc: parsear JWK pública kid %s: %w", r.KID, err)
		}
		e := keyEntry{kid: r.KID, alg: jwa.RS256, private: privKey, public: pubKey, active: r.Status == "active"}
		entries = append(entries, e)
	}
	km.mu.Lock()
	km.keys = entries
	for i := range km.keys {
		if km.keys[i].active {
			active = &km.keys[i]
			break
		}
	}
	km.active = active
	km.mu.Unlock()
	if active == nil {
		return fmt.Errorf("oidc: no hay clave de firma activa")
	}
	return nil
}

// signingKey devuelve la clave privada activa y su kid/alg.
func (km *KeyManager) signingKey() (jwk.Key, error) {
	km.mu.RLock()
	defer km.mu.RUnlock()
	if km.active == nil {
		return nil, fmt.Errorf("oidc: sin clave de firma activa")
	}
	return km.active.private, nil
}

// PublicJWKS construye el conjunto JWKS público (todas las claves no retiradas).
func (km *KeyManager) PublicJWKS() (jwk.Set, error) {
	km.mu.RLock()
	defer km.mu.RUnlock()
	set := jwk.NewSet()
	for i := range km.keys {
		if err := set.AddKey(km.keys[i].public); err != nil {
			return nil, err
		}
	}
	return set, nil
}

// Rotate genera una nueva clave activa, marca la anterior como 'next' y recarga.
// Pensado para uso operativo (cron/console) con cobertura de validación previa.
// private_pem se cifra en reposo (F9) a través del store.
func (km *KeyManager) Rotate(ctx context.Context, st *store.Store) error {
	err := st.InTx(ctx, func(q pg.Querier) error {
		// La clave nueva se inserta como 'active'; la vieja pasa a 'next' aquí mismo
		// (marcando demoted_at) para mantener un solo 'active' y solaparse durante la
		// ventana de retiro (RetireStaleKeys la retira pasado demoted_at+retireAfter).
		if _, err := q.Exec(ctx, `UPDATE signing_keys SET status='next', demoted_at=now() WHERE status='active'`); err != nil {
			return err
		}
		return generateAndStoreTx(ctx, st, q)
	})
	if err != nil {
		return err
	}
	return km.Reload(ctx, st)
}

// generateAndStoreTx es generateAndStore pero dentro de una transacción dada.
// Cifra private_pem en reposo usando el cifrador del store.
func generateAndStoreTx(ctx context.Context, st *store.Store, q pg.Querier) error {
	priv, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		return fmt.Errorf("oidc: generar RSA: %w", err)
	}
	kid, err := randomKID()
	if err != nil {
		return err
	}
	pubJWK, err := jwk.FromRaw(priv.Public())
	if err != nil {
		return err
	}
	_ = pubJWK.Set(jwk.KeyIDKey, kid)
	_ = pubJWK.Set(jwk.AlgorithmKey, jwa.RS256)
	_ = pubJWK.Set(jwk.KeyUsageKey, "sig")
	pubBytes, err := json.Marshal(pubJWK)
	if err != nil {
		return err
	}
	privDER := x509.MarshalPKCS1PrivateKey(priv)
	privPEM := pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: privDER})
	enc, err := st.EncryptPrivatePEM(string(privPEM))
	if err != nil {
		return fmt.Errorf("oidc: cifrar clave privada: %w", err)
	}
	const sql = `INSERT INTO signing_keys (kid, algorithm, public_jwk, private_pem, status)
		VALUES ($1,'RS256',$2,$3,'active')`
	_, err = q.Exec(ctx, sql, kid, pubBytes, enc)
	return err
}

// RetireExpired retira las claves 'next' cuya ventana de gracia (retireAfter,
// que DEBE ser >= vida del access token) ya venció, y recarga si hubo cambios.
// Un token firmado por una clave retirada dejará de validar; por eso la ventana
// debe cubrir la vida máxima del access token en circulación (F10).
func (km *KeyManager) RetireExpired(ctx context.Context, st *store.Store, retireAfter time.Duration) (int64, error) {
	n, err := st.RetireStaleKeys(ctx, time.Now(), retireAfter)
	if err != nil {
		return 0, err
	}
	if n > 0 {
		if err := km.Reload(ctx, st); err != nil {
			return n, err
		}
	}
	return n, nil
}

// RotateLoop ejecuta la rotación programada de la clave de firma (F10): cada
// `interval` genera una clave nueva, degrada la anterior a 'next' y, en cada
// pasada, retira las 'next' cuya ventana de gracia venció. Bloquea hasta que el
// contexto se cancele; pensado para ejecutarse en una goroutine.
func (km *KeyManager) RotateLoop(ctx context.Context, st *store.Store, interval, retireAfter time.Duration, logger *slog.Logger) {
	if interval <= 0 {
		return
	}
	// La ventana de retiro nunca debe ser inferior al intervalo de rotación, o se
	// retiraría una clave antes de la siguiente rotación (pérdida de solapamiento).
	if retireAfter < interval {
		retireAfter = interval
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if err := km.Rotate(ctx, st); err != nil {
				if logger != nil {
					logger.Error("rotación de clave de firma falló", "error", err)
				}
				continue
			}
			if n, err := km.RetireExpired(ctx, st, retireAfter); err != nil {
				if logger != nil {
					logger.Warn("retiro de claves expiradas falló", "error", err)
				}
			} else if logger != nil {
				logger.Info("clave de firma rotada", "retiradas", n, "retire_after", retireAfter.String())
			}
		}
	}
}

func randomKID() (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("oidc: aleatoriedad kid: %w", err)
	}
	return fmt.Sprintf("%x", b), nil
}
