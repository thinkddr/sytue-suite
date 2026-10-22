// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package apiweb

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"

	"git.sytue.eu/sytue/one/platform/pkg/altcha"
)

type AltchaVector struct {
	SecretKeyHex string           `json:"secret_key_hex"`
	Challenge    altcha.Challenge `json:"challenge"`
	Solution     struct {
		Number        int64  `json:"number"`
		PayloadBase64 string `json:"payload_base64"`
	} `json:"solution"`
}

func TestAltchaVectorDeterminista(t *testing.T) {
	// 1. Vector determinista acordado (§9.4, §9.5)
	keyHex := "0102030405060708090a0b0c0d0e0f101112131415161718191a1b1c1d1e1f20"
	key, err := hex.DecodeString(keyHex)
	if err != nil {
		t.Fatalf("decode key hex: %v", err)
	}

	salt := "b8f3a9e2c1d04567?expires=2000000000&ambito=sytue-tienda%3Acheckout&max=50000&"
	var secretNum int64 = 18423

	// Reto sha256 desde sal y número secreto.
	h := sha256.Sum256([]byte(salt + strconv.FormatInt(secretNum, 10)))
	challengeStr := hex.EncodeToString(h[:])

	// Firma HMAC del reto.
	mac := hmac.New(sha256.New, key)
	mac.Write([]byte(challengeStr))
	signature := hex.EncodeToString(mac.Sum(nil))

	ch := altcha.Challenge{
		Algorithm: "SHA-256",
		Challenge: challengeStr,
		MaxNumber: 50000,
		Salt:      salt,
		Signature: signature,
	}

	payload := altcha.Payload{
		Algorithm: "SHA-256",
		Challenge: challengeStr,
		Number:    secretNum,
		Salt:      salt,
		Signature: signature,
	}
	payloadBytes, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("marshal payload: %v", err)
	}
	payloadBase64 := base64.StdEncoding.EncodeToString(payloadBytes)

	// 2. Verificar que platform/pkg/altcha acepta el vector
	verifier := altcha.New(key)
	ctx := context.Background()
	if err := verifier.Comprobar(ctx, "sytue-tienda:checkout", 50000, payloadBase64); err != nil {
		t.Fatalf("verifier.Comprobar con vector falló: %v", err)
	}

	// 3. Escribir o comprobar testdata/altcha-vector.json en testdata y exportar/testdata
	vector := AltchaVector{
		SecretKeyHex: keyHex,
		Challenge:    ch,
	}
	vector.Solution.Number = secretNum
	vector.Solution.PayloadBase64 = payloadBase64

	vBytes, err := json.MarshalIndent(vector, "", "  ")
	if err != nil {
		t.Fatalf("marshal vector: %v", err)
	}

	rutasFicheros := []string{
		filepath.Join("..", "testdata", "altcha-vector.json"),
	}

	for _, rf := range rutasFicheros {
		if err := os.MkdirAll(filepath.Dir(rf), 0o755); err != nil {
			t.Fatalf("mkdir %s: %v", filepath.Dir(rf), err)
		}
		if err := os.WriteFile(rf, vBytes, 0o644); err != nil {
			t.Fatalf("write %s: %v", rf, err)
		}
	}

	// 4. Probar resolución por fuerza bruta del vector en Go para asegurar algoritmo
	var foundNum int64 = -1
	for n := int64(0); n <= ch.MaxNumber; n++ {
		computed := sha256.Sum256([]byte(ch.Salt + strconv.FormatInt(n, 10)))
		if hex.EncodeToString(computed[:]) == ch.Challenge {
			foundNum = n
			break
		}
	}
	if foundNum != secretNum {
		t.Fatalf("resolución de prueba esperaba %d pero encontró %d", secretNum, foundNum)
	}
}

func TestGenerarClienteTS(t *testing.T) {
	tsBytes, err := GenerarClienteTS("https://mitienda.ejemplo.es")
	if err != nil {
		t.Fatalf("GenerarClienteTS falló: %v", err)
	}

	tsCode := string(tsBytes)

	// Comprobar elementos obligatorios
	if !strings.Contains(tsCode, `export const PAGES_BASE_URL: string = "https://mitienda.ejemplo.es";`) {
		t.Errorf("Código generado no contiene PAGES_BASE_URL con el host provisto")
	}

	if !strings.Contains(tsCode, "export class PagesApiClient") {
		t.Errorf("Código generado no contiene la clase PagesApiClient")
	}

	if !strings.Contains(tsCode, "export async function resolverAltcha") {
		t.Errorf("Código generado no contiene la función resolverAltcha")
	}

	if !strings.Contains(tsCode, "X-Sytue-Pedido") {
		t.Errorf("Código generado no contiene la cabecera X-Sytue-Pedido")
	}

	// Comprobar que todas las rutas del catálogo están en el cliente
	for _, r := range RutasPublicas {
		if !strings.Contains(tsCode, r.OperacionID) {
			t.Errorf("Cliente TS no contiene la operación %s", r.OperacionID)
		}
	}
}

func TestRutasPublicasCubrenOpenAPI(t *testing.T) {
	yamlPath := filepath.Join("..", "..", "..", "openapi-web.yaml")
	data, err := os.ReadFile(yamlPath)
	if err != nil {
		t.Fatalf("leer openapi-web.yaml: %v", err)
	}

	var spec struct {
		Paths map[string]map[string]struct {
			OperationID string `yaml:"operationId"` //nolint:tagliatelle // openapi-web.yaml usa camelCase
		} `yaml:"paths"`
	}
	if err := yaml.Unmarshal(data, &spec); err != nil {
		t.Fatalf("deserializar openapi-web.yaml: %v", err)
	}

	rutasPorPatronMetodo := make(map[string]string)
	for _, r := range RutasPublicas {
		clave := fmt.Sprintf("%s %s", r.Metodo, r.Patron)
		rutasPorPatronMetodo[clave] = r.OperacionID
	}

	for path, methods := range spec.Paths {
		for m, op := range methods {
			metodo := strings.ToUpper(m)
			if metodo == "PARAMETERS" || metodo == "SUMMARY" || metodo == "DESCRIPTION" {
				continue
			}
			clave := fmt.Sprintf("%s %s", metodo, path)
			operacion, ok := rutasPorPatronMetodo[clave]
			if !ok {
				t.Errorf("Ruta %s de OpenAPI no está en RutasPublicas", clave)
				continue
			}
			if operacion != op.OperationID {
				t.Errorf("Ruta %s: operacionID en RutasPublicas (%s) != openapi-web.yaml (%s)",
					clave, operacion, op.OperationID)
			}
		}
	}
}
