// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

// Package stripe contiene la frontera de Stripe compartida por las aplicaciones
// que cobran en cuentas de las organizaciones.
package stripe

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"strconv"
	"strings"
	"time"
)

const (
	MaxCuerpoWebhook = 1 << 20
	ToleranciaFirma  = 5 * time.Minute
)

var (
	ErrFirmaInvalida = errors.New("stripe: invalid webhook signature")
	ErrFirmaCaducada = errors.New("stripe: expired webhook signature")
)

// VerificarFirma comprueba el cuerpo crudo recibido de Stripe. Acepta varias
// firmas v1 durante la rotación de secretos y rechaza repeticiones fuera de la
// ventana de cinco minutos.
func VerificarFirma(cuerpo []byte, cabecera, secreto string, ahora time.Time) error {
	if cabecera == "" || secreto == "" {
		return ErrFirmaInvalida
	}
	var marca string
	var firmas []string
	for _, parte := range strings.Split(cabecera, ",") {
		clave, valor, ok := strings.Cut(strings.TrimSpace(parte), "=")
		if !ok {
			continue
		}
		switch clave {
		case "t":
			marca = valor
		case "v1":
			firmas = append(firmas, valor)
		}
	}
	segundos, err := strconv.ParseInt(marca, 10, 64)
	if err != nil || len(firmas) == 0 {
		return ErrFirmaInvalida
	}
	if desfase := ahora.Sub(time.Unix(segundos, 0)); desfase > ToleranciaFirma || desfase < -ToleranciaFirma {
		return ErrFirmaCaducada
	}
	mac := hmac.New(sha256.New, []byte(secreto))
	_, _ = mac.Write([]byte(marca))
	_, _ = mac.Write([]byte("."))
	_, _ = mac.Write(cuerpo)
	esperada := mac.Sum(nil)
	for _, firma := range firmas {
		recibida, err := hex.DecodeString(firma)
		if err == nil && hmac.Equal(recibida, esperada) {
			return nil
		}
	}
	return ErrFirmaInvalida
}
