// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package httpx

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// failBody ejecuta Fail y devuelve la respuesta cruda.
func failBody(t *testing.T, err error) (*httptest.ResponseRecorder, map[string]any) {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/x", http.NoBody)
	rec := httptest.NewRecorder()
	Fail(rec, req, err)
	var body map[string]any
	if e := json.Unmarshal(rec.Body.Bytes(), &body); e != nil {
		t.Fatalf("cuerpo no es JSON: %v (%s)", e, rec.Body.String())
	}
	return rec, body
}

// codeFromBody replica al pie de la letra el consumidor que ya existe en
// apps/frontend/apps/cloud-dashboard/src/lib/cloud.ts: si esto deja de
// devolver el código, el front está roto.
func codeFromBody(t *testing.T, raw []byte, fallback string) string {
	t.Helper()
	var envelope struct {
		Error json.RawMessage `json:"error"`
	}
	if err := json.Unmarshal(raw, &envelope); err != nil || len(envelope.Error) == 0 {
		return fallback
	}
	var asString string
	if err := json.Unmarshal(envelope.Error, &asString); err == nil {
		if asString != "" {
			return asString
		}
		return fallback
	}
	var asObject struct {
		Code string `json:"code"`
	}
	if err := json.Unmarshal(envelope.Error, &asObject); err == nil && asObject.Code != "" {
		return asObject.Code
	}
	return fallback
}

// El sobre no cambia de forma: mismas claves, mismos tipos, y "params" solo
// aparece cuando hay parámetros (nada nuevo para los consumidores actuales).
func TestEnvelopeCompatibilidadHaciaAtras(t *testing.T) {
	rec, body := failBody(t, ErrNotFound)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d", rec.Code)
	}
	errObj, ok := body["error"].(map[string]any)
	if !ok {
		t.Fatalf("error no es objeto: %#v", body["error"])
	}
	if errObj["code"] != "not_found" {
		t.Fatalf("code = %#v", errObj["code"])
	}
	if _, ok := errObj["message"].(string); !ok {
		t.Fatalf("message ausente o no string: %#v", errObj["message"])
	}
	if _, exists := errObj["params"]; exists {
		t.Fatal("params debe omitirse cuando no hay parámetros")
	}
	if _, exists := errObj["details"]; exists {
		t.Fatal("details debe omitirse cuando no hay detalles")
	}
	if _, exists := errObj["status"]; exists {
		t.Fatal("status no debe viajar dentro del error (json:\"-\")")
	}
	// Claves del error: exactamente las de antes cuando no hay params.
	if len(errObj) != 2 {
		t.Fatalf("claves inesperadas en el error: %#v", errObj)
	}
	if got := codeFromBody(t, rec.Body.Bytes(), "backend_error"); got != "not_found" {
		t.Fatalf("codeFromBody = %q", got)
	}
}

// Los consumidores que leen "error" como cadena suelta siguen funcionando: es
// otro formato, pero codeFromBody lo acepta y no lo hemos tocado.
func TestCodeFromBodyAceptaErrorCadena(t *testing.T) {
	if got := codeFromBody(t, []byte(`{"error":"rate_limited"}`), "x"); got != "rate_limited" {
		t.Fatalf("= %q", got)
	}
	if got := codeFromBody(t, []byte(`no es json`), "backend_error"); got != "backend_error" {
		t.Fatalf("= %q", got)
	}
}

func TestFailPreservaDetailsYRequestID(t *testing.T) {
	req := httptest.NewRequest(http.MethodPost, "/x", http.NoBody)
	rec := httptest.NewRecorder()
	RequestID(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		Fail(w, r, ErrUnprocessable.WithDetails(map[string]string{"name": "required"}))
	})).ServeHTTP(rec, req)

	var body struct {
		Error struct {
			Code    string            `json:"code"`
			Details map[string]string `json:"details"`
		} `json:"error"`
		RequestID string `json:"request_id"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body.Error.Details["name"] != "required" {
		t.Fatalf("details = %#v", body.Error.Details)
	}
	if body.RequestID == "" {
		t.Fatal("falta request_id")
	}
}

// Un error con parámetros llega COMPLETO: código, params crudos y respaldo
// inglés ya compuesto (lo que ve un curl).
func TestErrorConParametrosLlegaCompleto(t *testing.T) {
	// Este es el error de ejemplo migrado de punta a punta: sustituye a
	// fmt.Errorf("%w: tamaño de instancia desconocido %q", ErrValidation, size).
	errUnknownInstanceSize := ErrBadRequest.
		WithCode("unknown_instance_size").
		WithMessage(`Unknown instance size "{size}". Allowed: {allowed}.`)

	rec, _ := failBody(t, errUnknownInstanceSize.WithParams(map[string]any{
		"size":    "xl",
		"allowed": "s, m, l",
		"count":   3,
	}))

	var body struct {
		Error struct {
			Code    string         `json:"code"`
			Message string         `json:"message"`
			Params  map[string]any `json:"params"`
		} `json:"error"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body.Error.Code != "unknown_instance_size" {
		t.Fatalf("code = %q", body.Error.Code)
	}
	if body.Error.Message != `Unknown instance size "xl". Allowed: s, m, l.` {
		t.Fatalf("message = %q", body.Error.Message)
	}
	if body.Error.Params["size"] != "xl" || body.Error.Params["allowed"] != "s, m, l" {
		t.Fatalf("params = %#v", body.Error.Params)
	}
	// Los números viajan como números, sin preformatear: el cliente decide si
	// escribe 1,234 o 1.234 según el idioma.
	if n, ok := body.Error.Params["count"].(float64); !ok || n != 3 {
		t.Fatalf("count = %#v", body.Error.Params["count"])
	}
	if got := rec.Header().Get("Content-Language"); got != BaseLanguage {
		t.Fatalf("Content-Language = %q, quiero %q", got, BaseLanguage)
	}
}

func TestErrorInmutableAlDerivar(t *testing.T) {
	base := ErrConflict
	derived := base.WithCode("project_slug_taken").WithParams(map[string]any{"slug": "acme"})
	if base.Code != "conflict" || base.Params != nil {
		t.Fatalf("se mutó el error canónico: %#v", base)
	}
	if derived.Status != http.StatusConflict {
		t.Fatalf("status perdido: %d", derived.Status)
	}
	// El mapa se copia: mutar el original del llamante no toca al error.
	params := map[string]any{"slug": "acme"}
	e := ErrConflict.WithParams(params)
	params["slug"] = "otro"
	if e.Params["slug"] != "acme" {
		t.Fatalf("params comparte estado con el llamante: %#v", e.Params)
	}
	// errors.Is sigue funcionando con la causa envuelta.
	cause := errors.New("boom")
	if !errors.Is(derived.Wrap(cause), cause) {
		t.Fatal("Wrap rompió errors.Is")
	}
}

func TestRenderMessageCasosRaros(t *testing.T) {
	cases := []struct {
		name    string
		message string
		params  map[string]any
		want    string
	}{
		{"sin params", "Plain message.", nil, "Plain message."},
		{"sin marcadores", "Plain message.", map[string]any{"a": 1}, "Plain message."},
		{"marcador desconocido se deja visible", "Missing {nope}.", map[string]any{"a": 1}, "Missing {nope}."},
		{"llave sin cerrar", "Broken {size", map[string]any{"size": "xl"}, "Broken {size"},
		{"repetido", "{a} y {a}", map[string]any{"a": "x"}, "x y x"},
		{"varios", "{a}-{b}", map[string]any{"a": 1, "b": true}, "1-true"},
		{"llave suelta al final", "100% {", map[string]any{"a": 1}, "100% {"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			e := &Error{Message: tc.message, Params: tc.params}
			if got := e.RenderMessage(); got != tc.want {
				t.Fatalf("= %q, quiero %q", got, tc.want)
			}
		})
	}
}

// Los mensajes de respaldo canónicos están en inglés base, no en español.
func TestMensajesCanonicosEnIngles(t *testing.T) {
	for _, e := range []*Error{
		ErrBadRequest, ErrUnauthorized, ErrForbidden, ErrNotFound, ErrConflict,
		ErrUnprocessable, ErrTooManyRequests, ErrInternal, ErrUnavailable,
		errEmptyBody, errBodyTooLarge, errInvalidJSON, errMultipleJSONObjects,
	} {
		if strings.ContainsAny(e.Message, "áéíóúñ¿¡") {
			t.Fatalf("%s: mensaje de respaldo no está en inglés: %q", e.Code, e.Message)
		}
	}
}

// Cada motivo de fallo al decodificar tiene su propio código: sin códigos
// distintos el cliente no puede traducir ni distinguir la causa.
func TestDecodeJSONCodigosPorMotivo(t *testing.T) {
	cases := []struct {
		name string
		body string
		code string
	}{
		{"vacío", "", "empty_body"},
		{"json inválido", "{", "invalid_json"},
		{"campo desconocido", `{"nope":1}`, "invalid_json"},
		{"dos objetos", `{"name":"a"}{"name":"b"}`, "multiple_json_objects"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodPost, "/x", strings.NewReader(tc.body))
			var dst struct {
				Name string `json:"name"`
			}
			err := DecodeJSON(httptest.NewRecorder(), req, &dst)
			apiErr := AsError(err)
			if apiErr == nil || apiErr.Code != tc.code {
				t.Fatalf("code = %#v, quiero %q", apiErr, tc.code)
			}
			if apiErr.Status != http.StatusBadRequest {
				t.Fatalf("status = %d (el contrato HTTP no cambia)", apiErr.Status)
			}
		})
	}
}

func TestDecodeJSONCuerpoDemasiadoGrandeLlevaElLimite(t *testing.T) {
	big := `{"name":"` + strings.Repeat("a", maxBodyBytes+1) + `"}`
	req := httptest.NewRequest(http.MethodPost, "/x", strings.NewReader(big))
	var dst struct {
		Name string `json:"name"`
	}
	apiErr := AsError(DecodeJSON(httptest.NewRecorder(), req, &dst))
	if apiErr.Code != "body_too_large" {
		t.Fatalf("code = %q", apiErr.Code)
	}
	if apiErr.Params["limit_bytes"] != maxBodyBytes {
		t.Fatalf("params = %#v", apiErr.Params)
	}
	if !strings.Contains(apiErr.RenderMessage(), "1048576") {
		t.Fatalf("el respaldo inglés perdió el dato: %q", apiErr.RenderMessage())
	}
}
