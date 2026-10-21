// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

// Package redsys implementa la firma de operaciones del TPV virtual de Redsys.
// La firma no comparte código con otros proveedores: Redsys deriva una clave por
// pedido con 3DES y firma los parámetros Base64 con HMAC-SHA512.
package redsys

import (
	"crypto/des" //nolint:gosec // El protocolo HMAC de Redsys prescribe 3DES para derivar la clave por pedido.
	"crypto/hmac"
	"crypto/sha512"
	"encoding/base64"
	"errors"
)

// FirmaVersion identifica el algoritmo vigente para las peticiones REST de
// Redsys. HMAC-SHA256_V1 quedó obsoleto para integraciones nuevas.
const FirmaVersion = "HMAC_SHA512_V1"

var errFirma = errors.New("redsys: parámetros de firma inválidos")

// Firmar construye Ds_Signature sobre Ds_MerchantParameters. claveComercio es
// la clave Base64 entregada por Redsys; pedido es Ds_Merchant_Order.
func Firmar(parametros, pedido, claveComercio string) (string, error) {
	if parametros == "" || pedido == "" || claveComercio == "" {
		return "", errFirma
	}
	clave, err := base64.StdEncoding.DecodeString(claveComercio)
	if err != nil || len(clave) != 24 {
		return "", errFirma
	}
	derivada, err := claveOperacion([]byte(pedido), clave)
	if err != nil {
		return "", errFirma
	}
	mac := hmac.New(sha512.New, derivada)
	_, _ = mac.Write([]byte(parametros))
	return base64.RawURLEncoding.EncodeToString(mac.Sum(nil)), nil
}

// Verificar compara en tiempo constante una firma recibida. Redsys ha usado
// tanto Base64 URL sin relleno como Base64 estándar en respuestas históricas.
func Verificar(parametros, pedido, claveComercio, firma string) bool {
	esperada, err := Firmar(parametros, pedido, claveComercio)
	if err != nil {
		return false
	}
	recibida, err := base64.RawURLEncoding.DecodeString(firma)
	if err != nil {
		recibida, err = base64.StdEncoding.DecodeString(firma)
		if err != nil {
			return false
		}
	}
	propia, err := base64.RawURLEncoding.DecodeString(esperada)
	return err == nil && hmac.Equal(recibida, propia)
}

func claveOperacion(pedido, clave []byte) ([]byte, error) {
	if len(pedido) == 0 || len(clave) != 24 {
		return nil, errFirma
	}
	// CBC sin padding PKCS: Redsys completa Ds_Order con bytes cero hasta el
	// bloque de 8, como especifica su algoritmo de derivación.
	pad := 8 - len(pedido)%8
	if pad == 8 {
		pad = 0
	}
	bloques := append(append([]byte(nil), pedido...), make([]byte, pad)...)
	if len(bloques) == 0 {
		return nil, errFirma
	}
	c, err := des.NewTripleDESCipher(clave) //nolint:gosec // El protocolo HMAC de Redsys prescribe 3DES para derivar la clave por pedido.
	if err != nil {
		return nil, err
	}
	iv := make([]byte, des.BlockSize)
	out := make([]byte, len(bloques))
	for i := 0; i < len(bloques); i += des.BlockSize {
		for j := 0; j < des.BlockSize; j++ {
			bloques[i+j] ^= iv[j]
		}
		c.Encrypt(out[i:i+des.BlockSize], bloques[i:i+des.BlockSize])
		copy(iv, out[i:i+des.BlockSize])
	}
	return out[len(out)-des.BlockSize:], nil
}
