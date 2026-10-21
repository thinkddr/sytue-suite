// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package http

import (
	"encoding/base64"
	"encoding/hex"
	"errors"
	"strings"
	"testing"
	"time"

	"git.sytue.eu/sytue/one/platform/pkg/keycrypto"
)

func TestLoadDefaults(t *testing.T) {
	t.Setenv("SYTUE_ID_DATABASE_DSN", "postgres://localhost/id")
	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.Issuer != "https://id.sytue.localhost" {
		t.Fatalf("Issuer=%q", cfg.Issuer)
	}
	// ResourceAudience por defecto = Issuer.
	if cfg.ResourceAudience != cfg.Issuer {
		t.Fatalf("ResourceAudience=%q", cfg.ResourceAudience)
	}
	// RPOrigins por defecto = [Issuer].
	if len(cfg.RPOrigins) != 1 || cfg.RPOrigins[0] != cfg.Issuer {
		t.Fatalf("RPOrigins=%v", cfg.RPOrigins)
	}
	// Puerto canónico de dev.
	if cfg.HTTPAddr != ":8081" {
		t.Fatalf("HTTPAddr=%q", cfg.HTTPAddr)
	}
	// TTLs por defecto.
	if cfg.AccessTokenTTL != 15*time.Minute {
		t.Fatalf("AccessTokenTTL=%v", cfg.AccessTokenTTL)
	}
	// KeyRetireAfter por defecto: máx(2·AccessTTL, 30m) = 30m.
	if cfg.KeyRetireAfter != 30*time.Minute {
		t.Fatalf("KeyRetireAfter=%v", cfg.KeyRetireAfter)
	}
	if cfg.CookieSecure != true {
		t.Fatalf("CookieSecure debe ser true por defecto")
	}
}

func TestLoadMissingDSN(t *testing.T) {
	// Sin SYTUE_ID_DATABASE_DSN → error.
	if _, err := Load(); err == nil {
		t.Fatal("Load debía fallar sin DATABASE_DSN")
	}
}

func TestLoadOverrides(t *testing.T) {
	t.Setenv("SYTUE_ID_DATABASE_DSN", "postgres://x/y")
	t.Setenv("SYTUE_ID_ISSUER", "https://id.example")
	t.Setenv("SYTUE_ID_RESOURCE_AUDIENCE", "aud-custom")
	t.Setenv("SYTUE_ID_WEBAUTHN_RP_ORIGINS", "https://a.example,https://b.example")
	t.Setenv("SYTUE_ID_ACCESS_TOKEN_TTL", "40m")
	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.Issuer != "https://id.example" || cfg.ResourceAudience != "aud-custom" {
		t.Fatalf("overrides: %+v", cfg)
	}
	if len(cfg.RPOrigins) != 2 {
		t.Fatalf("RPOrigins=%v", cfg.RPOrigins)
	}
	// KeyRetireAfter = 2·40m = 80m (supera el mínimo de 30m).
	if cfg.KeyRetireAfter != 80*time.Minute {
		t.Fatalf("KeyRetireAfter=%v", cfg.KeyRetireAfter)
	}
}

func TestLoadRejectsDomainCookies(t *testing.T) {
	t.Setenv("SYTUE_ID_DATABASE_DSN", "postgres://x/y")
	t.Setenv("SYTUE_ID_COOKIE_DOMAIN", ".sytue.eu")
	if _, err := Load(); err == nil || !strings.Contains(err.Error(), "host-only") {
		t.Fatalf("COOKIE_DOMAIN debe rechazarse: %v", err)
	}
}

func TestLoadProdRequiresLivePaidSubscription(t *testing.T) {
	t.Setenv("SYTUE_ID_DATABASE_DSN", "postgres://x/y")
	t.Setenv("SYTUE_ID_ENV", "prod")
	t.Setenv("SYTUE_ID_ISSUER", "https://id.sytue.eu")
	t.Setenv("SYTUE_ID_SMTP_ADDR", "sytue-one-mail-mx:587")

	if _, err := Load(); err == nil || !strings.Contains(err.Error(), "MOLLIE_API_KEY") {
		t.Fatalf("producción sin Mollie live debía fallar: %v", err)
	}
	t.Setenv("SYTUE_ID_MOLLIE_API_KEY", "test_123")
	if _, err := Load(); err == nil || !strings.Contains(err.Error(), "debe ser live_") {
		t.Fatalf("producción con test key debía fallar: %v", err)
	}
	t.Setenv("SYTUE_ID_MOLLIE_API_KEY", "live_123")
	cfg, err := Load()
	if err != nil || !cfg.RequirePaidSubscription {
		t.Fatalf("producción con live key válida: cfg=%+v err=%v", cfg, err)
	}

	t.Setenv("SYTUE_ID_REQUIRE_PAID_SUBSCRIPTION", "false")
	if _, err := Load(); err == nil || !strings.Contains(err.Error(), "no puede desactivarse") {
		t.Fatalf("la verja de pago no debe poder desactivarse en prod: %v", err)
	}
}

// Una instancia autoalojada es el ÚNICO caso en que producción arranca sin verja
// de pago, y tiene que pedirse por su nombre. Lo que este test protege de verdad
// es la alternativa: al chocar con «no puede desactivarse», lo natural es bajar
// el entorno a staging, y eso además deja a Docs, Rooms y Analytics sin exigir
// clave de cifrado y pone AllowInsecureAuth en el SMTP de Mail. Si alguien
// borra este interruptor, esa salida vuelve a ser la única.
func TestLoadProdAutoalojadoPermiteSinCobros(t *testing.T) {
	t.Setenv("SYTUE_ID_DATABASE_DSN", "postgres://x/y")
	t.Setenv("SYTUE_ID_ENV", "prod")
	t.Setenv("SYTUE_ID_ISSUER", "https://id.sytue.eu")
	t.Setenv("SYTUE_ID_SMTP_ADDR", "sytue-one-mail-mx:587")
	t.Setenv("SYTUE_ID_REQUIRE_PAID_SUBSCRIPTION", "false")

	// Sin el interruptor, el error de siempre.
	if _, err := Load(); err == nil || !strings.Contains(err.Error(), "no puede desactivarse") {
		t.Fatalf("sin SELF_HOSTED debía seguir fallando: %v", err)
	}

	// Con él, arranca y NO pide Mollie: no hay nada que cobrar.
	t.Setenv("SYTUE_ID_SELF_HOSTED", "true")
	cfg, err := Load()
	if err != nil {
		t.Fatalf("autoalojado sin cobros debía cargar: %v", err)
	}
	if cfg.RequirePaidSubscription {
		t.Fatalf("autoalojado no debe exigir suscripción: %+v", cfg)
	}
	if !cfg.SelfHosted {
		t.Fatalf("SelfHosted debía quedar activo: %+v", cfg)
	}

	// Y no relaja nada más: sin SMTP, producción sigue negándose a arrancar.
	t.Setenv("SYTUE_ID_SMTP_ADDR", "")
	if _, err := Load(); err == nil || !strings.Contains(err.Error(), "SMTP") {
		t.Fatalf("autoalojado no debe saltarse la exigencia de SMTP: %v", err)
	}
}

// El correo transaccional se perdía sin que fallara nada: sin SMTP_ADDR el mailer
// de desarrollo escribe el enlace en el log y devuelve nil, así que la invitación
// respondía 201 y la consola daba el aviso verde mientras el invitado esperaba un
// correo que nadie había mandado. Que el chart lo inyecte no basta —un values
// heredado o un `helm upgrade` a medias lo deshacen en silencio—, así que en
// producción esto tiene que impedir el arranque.
func TestLoadProdExigeSMTP(t *testing.T) {
	t.Setenv("SYTUE_ID_DATABASE_DSN", "postgres://x/y")
	t.Setenv("SYTUE_ID_ENV", "prod")
	t.Setenv("SYTUE_ID_ISSUER", "https://id.sytue.eu")
	t.Setenv("SYTUE_ID_MOLLIE_API_KEY", "live_123")

	if _, err := Load(); err == nil || !strings.Contains(err.Error(), "SMTP_ADDR") {
		t.Fatalf("producción sin SMTP debía fallar: %v", err)
	}

	t.Setenv("SYTUE_ID_SMTP_ADDR", "sytue-one-mail-mx:587")
	if _, err := Load(); err != nil {
		t.Fatalf("producción con SMTP: %v", err)
	}
}

func TestLoadKEKBase64(t *testing.T) {
	t.Setenv("SYTUE_ID_DATABASE_DSN", "postgres://x/y")
	key := make([]byte, keycrypto.KeyLen)
	for i := range key {
		key[i] = byte(i)
	}
	t.Setenv("SYTUE_ID_KEY_ENCRYPTION_KEY", base64.StdEncoding.EncodeToString(key))
	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if len(cfg.KeyEncryptionKey) != keycrypto.KeyLen {
		t.Fatalf("KEK longitud=%d", len(cfg.KeyEncryptionKey))
	}
}

func TestLoadKEKInvalid(t *testing.T) {
	t.Setenv("SYTUE_ID_DATABASE_DSN", "postgres://x/y")
	t.Setenv("SYTUE_ID_KEY_ENCRYPTION_KEY", "no-es-una-clave")
	if _, err := Load(); err == nil {
		t.Fatal("KEK inválida debía dar error")
	}
}

// --- decodeKEK ---

func TestDecodeKEK(t *testing.T) {
	key := make([]byte, keycrypto.KeyLen)
	for i := range key {
		key[i] = byte(i * 7)
	}

	if got, err := decodeKEK(base64.StdEncoding.EncodeToString(key)); err != nil || len(got) != keycrypto.KeyLen {
		t.Fatalf("base64: %v len=%d", err, len(got))
	}
	if got, err := decodeKEK(hex.EncodeToString(key)); err != nil || len(got) != keycrypto.KeyLen {
		t.Fatalf("hex: %v len=%d", err, len(got))
	}
	if _, err := decodeKEK("demasiado-corta"); err == nil {
		t.Fatal("cadena corta debía fallar")
	}
	// 16 bytes (AES-128) no es válido para AES-256.
	if _, err := decodeKEK(base64.StdEncoding.EncodeToString(make([]byte, 16))); err == nil {
		t.Fatal("16 bytes debía fallar")
	}
}

// --- mergeErr / errJoin ---

func TestMergeErr(t *testing.T) {
	e1 := errors.New("uno")
	e2 := errors.New("dos")
	if mergeErr(nil, nil) != nil {
		t.Fatal("nil+nil debe ser nil")
	}
	if got := mergeErr(e1, nil); !errors.Is(got, e1) {
		t.Fatalf("a+nil=%v", got)
	}
	if got := mergeErr(nil, e2); !errors.Is(got, e2) {
		t.Fatalf("nil+b=%v", got)
	}
	got := mergeErr(e1, e2)
	if !strings.Contains(got.Error(), "uno") || !strings.Contains(got.Error(), "dos") {
		t.Fatalf("mensaje combinado=%q", got.Error())
	}
}
