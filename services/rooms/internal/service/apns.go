// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package service

// APNs «SOLO DESPERTAR» — el aviso de la app nativa de iOS.
//
// POR QUÉ EXISTE ESTE FICHERO Y NO SU EQUIVALENTE EN ANDROID
// En Android la app puede mantener ella misma el flujo SSE del hub y enterarse sin
// pedir permiso a nadie: el aviso nace y muere dentro de nuestra infraestructura
// europea. En iOS no: el sistema mata la conexión al pasar a segundo plano y el
// ÚNICO camino para despertar una app es APNs, que es de Apple y no tiene región
// europea. No hay alternativa técnica; solo cabe decidir QUÉ se le cuenta.
//
// QUÉ SALE HACIA APPLE
// Exactamente el token del dispositivo, el identificador de la app y el momento.
// El cuerpo es siempre el mismo (`cuerpoDespertar`) y va VACÍO de contenido: sin
// título, sin texto, sin autor, sin canal, sin contador. La app despierta, pide el
// mensaje a nuestros servidores y pinta ella la notificación. Apple puede saber que
// ese aparato recibió algo y cuándo; no qué, ni de quién.
//
// Por eso el cuerpo es una CONSTANTE y no se compone: que nadie pueda añadirle un
// campo «solo para depurar» sin darse cuenta de lo que está publicando. El test
// `TestAPNSNoMandaNadaMasQueElDespertar` lo fija.

import (
	"bytes"
	"context"
	"crypto/ecdsa"
	"crypto/rand"
	"crypto/sha256"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"io"
	"net/http"
	"sync"
	"time"

	"git.sytue.eu/sytue/one/services/rooms/internal/config"
	"git.sytue.eu/sytue/one/services/rooms/internal/domain"
)

const (
	// apnsHostProd es el servidor de producción de Apple. El de pruebas
	// (api.sandbox.push.apple.com) se elige por config: un token de una compilación
	// de desarrollo NO vale en producción y viceversa, y el error que da Apple
	// (BadDeviceToken) no distingue «token roto» de «servidor equivocado».
	apnsHostProd = "https://api.push.apple.com"
	// apnsTokenTTL renueva el JWT antes de que Apple lo rechace. Apple exige entre 20
	// y 60 minutos de antigüedad y devuelve TooManyProviderTokenUpdates si se pide uno
	// nuevo demasiado seguido, así que se reutiliza hasta cerca del límite.
	apnsTokenTTL = 45 * time.Minute
	apnsTimeout  = 10 * time.Second
	// apnsMaxTargets acota los aparatos por mensaje, igual que pushMaxTargets.
	apnsMaxTargets = 50
)

// cuerpoDespertar es lo ÚNICO que se manda a Apple: «hay algo nuevo». Ni un campo más.
var cuerpoDespertar = []byte(`{"aps":{"content-available":1}}`)

// APNSSender firma los JWT de proveedor y entrega los avisos vacíos a Apple.
type APNSSender struct {
	keyID  string
	teamID string
	topic  string // bundle id de la app (apns-topic)
	host   string
	key    *ecdsa.PrivateKey
	client *http.Client

	// ahora se inyecta en los tests para comprobar la caducidad del JWT sin esperar
	// tres cuartos de hora.
	ahora func() time.Time

	mu       sync.Mutex
	bearer   string
	bearerAt time.Time
}

// NewAPNSSender construye el enviador desde la config. Los cuatro valores vacíos =>
// (nil, nil): iOS sin push, como el Web Push sin VAPID. A medias sí es un error de
// config y aborta el arranque, porque «creía que estaba puesto» es peor que «no está».
func NewAPNSSender(cfg config.Config) (*APNSSender, error) {
	puestos := 0
	for _, v := range []string{cfg.APNSKeyP8, cfg.APNSKeyID, cfg.APNSTeamID, cfg.APNSBundleID} {
		if v != "" {
			puestos++
		}
	}
	if puestos == 0 {
		return nil, nil //nolint:nilnil // iOS sin push: ausencia intencional, no error.
	}
	if puestos < 4 {
		return nil, errors.New("apns: define SYTUE_APNS_KEY_P8, _KEY_ID, _TEAM_ID y _BUNDLE_ID juntas (o ninguna)")
	}
	key, err := parseAPNSKey(cfg.APNSKeyP8)
	if err != nil {
		return nil, err
	}
	host := cfg.APNSHost
	if host == "" {
		host = apnsHostProd
	}
	return &APNSSender{
		keyID:  cfg.APNSKeyID,
		teamID: cfg.APNSTeamID,
		topic:  cfg.APNSBundleID,
		host:   host,
		key:    key,
		client: ssrfSafeClient(),
		ahora:  time.Now,
	}, nil
}

// parseAPNSKey lee la clave .p8 que descarga Apple (PKCS#8, PEM, ECDSA P-256).
func parseAPNSKey(p8 string) (*ecdsa.PrivateKey, error) {
	blk, _ := pem.Decode([]byte(p8))
	if blk == nil {
		return nil, errors.New("apns: SYTUE_APNS_KEY_P8 no es un PEM (el fichero .p8 entero, con sus cabeceras)")
	}
	k, err := x509.ParsePKCS8PrivateKey(blk.Bytes)
	if err != nil {
		return nil, fmt.Errorf("apns: clave ilegible: %w", err)
	}
	ec, ok := k.(*ecdsa.PrivateKey)
	if !ok {
		return nil, errors.New("apns: la clave no es ECDSA (Apple emite P-256)")
	}
	return ec, nil
}

// token devuelve el JWT de proveedor, reutilizándolo mientras siga vigente.
func (a *APNSSender) token() (string, error) {
	now := a.ahora()
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.bearer != "" && now.Sub(a.bearerAt) < apnsTokenTTL {
		return a.bearer, nil
	}
	cab, err := json.Marshal(map[string]string{"alg": "ES256", "kid": a.keyID, "typ": "JWT"})
	if err != nil {
		return "", err
	}
	cla, err := json.Marshal(map[string]any{"iss": a.teamID, "iat": now.Unix()})
	if err != nil {
		return "", err
	}
	firmado := b64urlEncode(cab) + "." + b64urlEncode(cla)
	sum := sha256.Sum256([]byte(firmado))
	r, s, err := ecdsa.Sign(rand.Reader, a.key, sum[:])
	if err != nil {
		return "", err
	}
	// ES256 quiere R||S en 32 bytes cada uno; el DER que devuelve ecdsa.Sign no vale.
	sig := make([]byte, 64)
	r.FillBytes(sig[:32])
	s.FillBytes(sig[32:])
	a.bearer = firmado + "." + b64urlEncode(sig)
	a.bearerAt = now
	return a.bearer, nil
}

// SetAPNSSender inyecta el enviador de iOS (lo construye main.go). nil = deshabilitado.
func (s *Service) SetAPNSSender(a *APNSSender) { s.apns = a }

// APNSEnabled indica si el aviso a iOS está configurado.
func (s *Service) APNSEnabled() bool { return s.apns != nil }

// RegisterAPNSDevice guarda el token de dispositivo de la app nativa de iOS.
func (s *Service) RegisterAPNSDevice(ctx context.Context, subject, token string) error {
	if !domain.ValidAPNSToken(token) {
		return domain.ErrInvalidInput
	}
	if ps := s.pushStore(); ps != nil {
		return ps.SaveAPNSDevice(ctx, subject, token)
	}
	return nil
}

// ForgetAPNSDevice borra el token: lo llama la app al cerrar sesión o al desactivar los
// avisos. Que se pueda dejar de ser despertable es parte de la promesa.
func (s *Service) ForgetAPNSDevice(ctx context.Context, subject, token string) error {
	if token == "" {
		return domain.ErrInvalidInput
	}
	if ps := s.pushStore(); ps != nil {
		return ps.DeleteAPNSDevice(ctx, subject, token)
	}
	return nil
}

// sendOneAPNS despierta un aparato. Devuelve errPushGone si Apple dice que el token ya
// no vale, para borrarlo igual que una suscripción Web Push caduca.
func (s *Service) sendOneAPNS(ctx context.Context, t domain.APNSTarget) error {
	jwt, err := s.apns.token()
	if err != nil {
		return err
	}
	reqCtx, cancel := context.WithTimeout(ctx, apnsTimeout)
	defer cancel()
	req, err := http.NewRequestWithContext(reqCtx, http.MethodPost,
		s.apns.host+"/3/device/"+t.Token, bytes.NewReader(cuerpoDespertar))
	if err != nil {
		return err
	}
	req.Header.Set("authorization", "bearer "+jwt)
	req.Header.Set("apns-topic", s.apns.topic)
	// `background` + prioridad 5: es un «mira si tienes algo», no una alerta. Apple
	// exige esa combinación para un push sin contenido y castiga lo contrario.
	req.Header.Set("apns-push-type", "background")
	req.Header.Set("apns-priority", "5")
	req.Header.Set("content-type", "application/json")
	resp, err := s.apns.client.Do(req)
	if err != nil {
		return err
	}
	defer func() {
		_, _ = io.Copy(io.Discard, resp.Body)
		_ = resp.Body.Close()
	}()
	if resp.StatusCode == http.StatusOK {
		return nil
	}
	if resp.StatusCode == http.StatusGone {
		return errPushGone
	}
	var e struct {
		Reason string `json:"reason"`
	}
	cuerpo, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
	_ = json.Unmarshal(cuerpo, &e)
	switch e.Reason {
	case "BadDeviceToken", "Unregistered", "DeviceTokenNotForTopic":
		return errPushGone
	}
	return fmt.Errorf("apns: %d %s", resp.StatusCode, e.Reason)
}
