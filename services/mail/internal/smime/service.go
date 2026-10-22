// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package smime

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
)

// Vault sella y abre las claves privadas at-rest. Lo satisface
// platform/pkg/keycrypto.Cipher; se declara aquí como interfaz para no acoplar
// este paquete a una implementación concreta ni obligar a tenerla en los tests.
type Vault interface {
	Enabled() bool
	Encrypt(plaintext string) (string, error)
	Decrypt(ciphertext string) (string, error)
}

// Store es la persistencia del material S/MIME.
type Store interface {
	Authority(ctx context.Context, tenant string) (certPEM, keyEnc string, err error)
	SaveAuthority(ctx context.Context, tenant, certPEM, keyEnc string) error
	IdentityOf(ctx context.Context, accountID uuid.UUID) (address, certPEM, keyEnc string, notAfter time.Time, err error)
	SaveIdentity(ctx context.Context, accountID uuid.UUID, tenant, address, certPEM, keyEnc string, notAfter time.Time) error
}

// ErrNoMaterial lo devuelve el store cuando aún no hay autoridad o identidad.
// No es un fallo: es la primera vez, y este servicio la crea.
var ErrNoMaterial = fmt.Errorf("smime: todavía no hay material")

// Service emite, renueva y usa el material S/MIME de cada buzón.
//
// Todo ocurre SOLO: la primera vez que alguien envía un correo se le emite su
// certificado, y treinta días antes de caducar se renueva sin que nadie haga
// nada. Es la diferencia entre «tenemos S/MIME» y «la gente firma su correo».
type Service struct {
	store  Store
	vault  Vault
	logger *slog.Logger

	// La autoridad de cada organización se cachea: firmar un correo no puede
	// costar una consulta extra a la base de datos.
	mu    sync.RWMutex
	byOrg map[string]*CA
}

// NewService construye el servicio. Sin vault (o con la KEK sin configurar) NO
// se emite nada: guardar una clave privada en claro sería peor que no firmar.
func NewService(store Store, vault Vault, logger *slog.Logger) *Service {
	if logger == nil {
		logger = slog.Default()
	}
	return &Service{store: store, vault: vault, logger: logger, byOrg: map[string]*CA{}}
}

// Enabled dice si el servicio puede operar.
func (s *Service) Enabled() bool {
	return s != nil && s.store != nil && s.vault != nil && s.vault.Enabled()
}

// IdentityFor devuelve la identidad del buzón, emitiéndola o renovándola si
// hace falta. Devuelve nil (sin error) cuando el servicio no está disponible:
// firmar es una mejora, y un correo sin firmar sale igual.
func (s *Service) IdentityFor(ctx context.Context, tenant, org string, accountID uuid.UUID, address, displayName string) (*Identity, error) {
	if !s.Enabled() {
		return nil, nil //nolint:nilnil // «no hay identidad y no es un error» es el caso normal sin KEK.
	}
	addr, cert, keyEnc, notAfter, err := s.store.IdentityOf(ctx, accountID)
	switch {
	case err == nil && addr != "":
		keyPEM, decErr := s.vault.Decrypt(keyEnc)
		if decErr != nil {
			return nil, fmt.Errorf("smime: abrir la clave del buzón: %w", decErr)
		}
		id, loadErr := LoadIdentity(addr, cert, keyPEM)
		if loadErr != nil {
			return nil, loadErr
		}
		if !id.NeedsRenewal(time.Now()) {
			return id, nil
		}
		// Le queda poco. Si el certificado NO lo emitimos nosotros, no hay nada
		// que renovar aquí: reemitirlo con la autoridad de la organización
		// cambiaría un certificado que el mundo valida solo por otro que fuera no
		// valida nadie. Se sigue usando mientras esté vigente y se avisa.
		if ca, caErr := s.authority(ctx, tenant, org); caErr == nil && !ca.Emitio(id.Certificate) {
			if time.Now().Before(id.NotAfter) {
				s.logger.Warn("smime: el certificado importado de este buzón caduca pronto; nadie lo renueva solo",
					"tenant", tenant, "address", addr, "not_after", id.NotAfter)
				return id, nil
			}
		}
		// Caduca pronto: se renueva sola. Quien envía no se entera.
	case err != nil && !isNoMaterial(err):
		return nil, err
	}
	_ = notAfter
	return s.issue(ctx, tenant, org, accountID, address, displayName)
}

// issue emite (o renueva) el certificado de un buzón con la autoridad de su
// organización, creándola la primera vez.
func (s *Service) issue(ctx context.Context, tenant, org string, accountID uuid.UUID, address, displayName string) (*Identity, error) {
	ca, err := s.authority(ctx, tenant, org)
	if err != nil {
		return nil, err
	}
	id, err := ca.Issue(address, displayName)
	if err != nil {
		return nil, err
	}
	certPEM, keyPEM, err := id.PEM()
	if err != nil {
		return nil, err
	}
	keyEnc, err := s.vault.Encrypt(keyPEM)
	if err != nil {
		return nil, fmt.Errorf("smime: sellar la clave del buzón: %w", err)
	}
	if err := s.store.SaveIdentity(ctx, accountID, tenant, id.Address, certPEM, keyEnc, id.NotAfter); err != nil {
		return nil, err
	}
	s.logger.Info("smime: certificado emitido",
		"tenant", tenant, "address", id.Address, "not_after", id.NotAfter)
	return id, nil
}

// authority devuelve la autoridad de una organización, creándola si es la
// primera vez que alguien de esa organización envía correo.
func (s *Service) authority(ctx context.Context, tenant, org string) (*CA, error) {
	s.mu.RLock()
	cached := s.byOrg[tenant]
	s.mu.RUnlock()
	if cached != nil {
		return cached, nil
	}

	certPEM, keyEnc, err := s.store.Authority(ctx, tenant)
	if err == nil && certPEM != "" {
		keyPEM, decErr := s.vault.Decrypt(keyEnc)
		if decErr != nil {
			return nil, fmt.Errorf("smime: abrir la clave de la autoridad: %w", decErr)
		}
		ca, loadErr := LoadCA(certPEM, keyPEM)
		if loadErr != nil {
			return nil, loadErr
		}
		s.cache(tenant, ca)
		return ca, nil
	}
	if err != nil && !isNoMaterial(err) {
		return nil, err
	}

	name := strings.TrimSpace(org)
	if name == "" {
		name = tenant
	}
	ca, err := NewCA(name)
	if err != nil {
		return nil, err
	}
	newCertPEM, newKeyPEM, err := ca.PEM()
	if err != nil {
		return nil, err
	}
	sealed, err := s.vault.Encrypt(newKeyPEM)
	if err != nil {
		return nil, fmt.Errorf("smime: sellar la clave de la autoridad: %w", err)
	}
	if err := s.store.SaveAuthority(ctx, tenant, newCertPEM, sealed); err != nil {
		return nil, err
	}
	s.logger.Info("smime: autoridad de la organización creada", "tenant", tenant)
	s.cache(tenant, ca)
	return ca, nil
}

func (s *Service) cache(tenant string, ca *CA) {
	s.mu.Lock()
	s.byOrg[tenant] = ca
	s.mu.Unlock()
}

// Import guarda para un buzón un certificado emitido FUERA —el de una CA
// pública— con su cadena y su clave privada, y lo deja como el material con el
// que se firma a partir de ese momento.
//
// Es el único camino por el que un correo de Sytue se valida en el dispositivo
// de quien lo recibe SIN que instale nada: la autoridad de la organización no
// está en el almacén de confianza de nadie de fuera, y una CA pública sí.
//
// Lo que se guarda es lo mismo que emite la autoridad de aquí —mismo sitio,
// misma KEK, mismo camino de firma—, así que renovar, fusionar organizaciones y
// borrar la cuenta siguen funcionando igual.
func (s *Service) Import(
	ctx context.Context, tenant string, accountID uuid.UUID, address, certPEM, keyPEM string,
) (*Identity, error) {
	if !s.Enabled() {
		return nil, fmt.Errorf("smime: este despliegue no tiene S/MIME configurado")
	}
	id, err := ImportIdentity(address, certPEM, keyPEM)
	if err != nil {
		return nil, err
	}
	guardarCert, guardarKey, err := id.PEM()
	if err != nil {
		return nil, err
	}
	keyEnc, err := s.vault.Encrypt(guardarKey)
	if err != nil {
		return nil, fmt.Errorf("smime: sellar la clave del buzón: %w", err)
	}
	if err := s.store.SaveIdentity(ctx, accountID, tenant, id.Address, guardarCert, keyEnc, id.NotAfter); err != nil {
		return nil, err
	}
	s.logger.Info("smime: certificado importado",
		"tenant", tenant, "address", id.Address,
		"issuer", id.Certificate.Issuer.CommonName, "not_after", id.NotAfter)
	return id, nil
}

// AnchorPEM devuelve el certificado de la autoridad de una organización: es lo
// que se distribuye a los dispositivos (una vez) para que su correo firmado se
// vea como de confianza, y lo que puede publicar la consola.
func (s *Service) AnchorPEM(ctx context.Context, tenant string) (string, error) {
	if !s.Enabled() {
		return "", fmt.Errorf("smime: este despliegue no tiene S/MIME configurado")
	}
	certPEM, _, err := s.store.Authority(ctx, tenant)
	if err != nil {
		return "", err
	}
	if certPEM == "" {
		return "", ErrNoMaterial
	}
	return certPEM, nil
}

// SignMessage firma un mensaje saliente. Si no hay identidad utilizable, lo
// devuelve TAL CUAL: el correo sale igual, sin firma, y se registra el motivo.
// Un fallo al firmar no puede dejar a nadie sin poder enviar.
func (s *Service) SignMessage(ctx context.Context, tenant, org string, accountID uuid.UUID, address, displayName string, raw []byte) []byte {
	if !s.Enabled() {
		return raw
	}
	id, err := s.IdentityFor(ctx, tenant, org, accountID, address, displayName)
	if err != nil || id == nil {
		if err != nil {
			s.logger.Warn("smime: no se pudo preparar la firma; el correo sale sin firmar",
				"tenant", tenant, "address", address, "error", err)
		}
		return raw
	}
	signed, err := Sign(id, raw)
	if err != nil {
		s.logger.Warn("smime: no se pudo firmar; el correo sale sin firmar",
			"tenant", tenant, "address", address, "error", err)
		return raw
	}
	return signed
}

// isNoMaterial reconoce «todavía no hay nada guardado».
func isNoMaterial(err error) bool {
	return err != nil && strings.Contains(err.Error(), "no rows")
}
