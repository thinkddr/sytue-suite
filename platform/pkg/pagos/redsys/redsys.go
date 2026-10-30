// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package redsys

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"

	"git.sytue.eu/sytue/one/platform/pkg/pagos"
)

const (
	testURL     = "https://sis-t.redsys.es:25443/sis/realizarPago"
	liveURL     = "https://sis.redsys.es/sis/realizarPago"
	testRESTURL = "https://sis-t.redsys.es:25443/sis/rest/trataPeticionREST"
	liveRESTURL = "https://sis.redsys.es/sis/rest/trataPeticionREST"
)

var (
	merchantRe     = regexp.MustCompile(`^\d{9}$`)
	terminalRe     = regexp.MustCompile(`^\d{1,3}$`)
	errUnavailable = errors.New("redsys: proveedor no disponible")
)

// Redsys prepara el formulario firmado que el navegador envía al TPV. No abre
// una conexión de egress: Redsys recibe el formulario directamente y el webhook
// posterior se verifica antes de consultar o modificar el pedido.
type Redsys struct {
	merchant, terminal, key, mode string
	restURL                       string
	http                          *http.Client
}

var _ pagos.Pasarela = (*Redsys)(nil)

func New(merchant, terminal, key, mode string) (*Redsys, error) {
	merchant, terminal, key = strings.TrimSpace(merchant), strings.TrimSpace(terminal), strings.TrimSpace(key)
	if !merchantRe.MatchString(merchant) || !terminalRe.MatchString(terminal) ||
		(mode != pagos.ModoPrueba && mode != pagos.ModoReal) {
		return nil, errors.New("redsys: credenciales inválidas")
	}
	if decoded, err := base64.StdEncoding.DecodeString(key); err != nil || len(decoded) != 24 {
		return nil, errors.New("redsys: clave inválida")
	}
	restURL := testRESTURL
	if mode == pagos.ModoReal {
		restURL = liveRESTURL
	}
	return &Redsys{merchant: merchant, terminal: terminal, key: key, mode: mode, restURL: restURL, http: &http.Client{Timeout: 12 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}}, nil
}

// ConTransporte sustituye el transporte HTTP manteniendo el timeout y la prohibición de redirecciones.
func (r *Redsys) ConTransporte(rt http.RoundTripper) *Redsys {
	if r != nil && rt != nil {
		r.http.Transport = rt
	}
	return r
}

// CrearPago abre el formulario del TPV. El `Ds_Merchant_Order` sale del id de
// nuestro pago (pagos.MetadatoPago), no de `Referencia`: Redsys exige 12
// caracteres (4 dígitos y 8 alfanuméricos) y únicos por comercio, y el número de
// pedido (P-000123) ni cumple el formato ni es único entre tiendas ni entre
// intentos de un mismo pedido. Cada intento es un pago con su id, así que cada
// uno lleva su orden. Lo mismo que se hizo con Adyen en #776.
func (r *Redsys) CrearPago(_ context.Context, s pagos.Solicitud) (pagos.Creado, error) {
	if r == nil || s.Centimos <= 0 || s.Divisa != pagos.DivisaEUR || !urlHTTPS(s.URLRetorno) ||
		!urlHTTPS(s.URLWebhook) || s.Metadatos[pagos.MetadatoPago] == "" {
		return pagos.Creado{}, errors.New("redsys: solicitud inválida")
	}
	orden := Orden(s.Metadatos[pagos.MetadatoPago])
	p := map[string]string{
		"DS_MERCHANT_AMOUNT":             strconv.FormatInt(s.Centimos, 10),
		"DS_MERCHANT_CURRENCY":           "978",
		"DS_MERCHANT_MERCHANTCODE":       r.merchant,
		"DS_MERCHANT_MERCHANTDATA":       s.Metadatos[pagos.MetadatoPago],
		"DS_MERCHANT_MERCHANTURL":        s.URLWebhook,
		"DS_MERCHANT_ORDER":              orden,
		"DS_MERCHANT_TERMINAL":           r.terminal,
		"DS_MERCHANT_TRANSACTIONTYPE":    "0",
		"DS_MERCHANT_URLOK":              s.URLRetorno,
		"DS_MERCHANT_URLKO":              s.URLRetorno,
		"DS_MERCHANT_PRODUCTDESCRIPTION": limitar(s.Descripcion, 125),
	}
	if s.Metodo == "bizum" {
		p["DS_MERCHANT_PAYMETHODS"] = "z"
	}
	crudo, err := json.Marshal(p)
	if err != nil {
		return pagos.Creado{}, err
	}
	params := base64.StdEncoding.EncodeToString(crudo)
	firma, err := Firmar(params, orden, r.key)
	if err != nil {
		return pagos.Creado{}, errUnavailable
	}
	accion := testURL
	if r.mode == pagos.ModoReal {
		accion = liveURL
	}
	return pagos.Creado{Ref: orden, Formulario: &pagos.Formulario{Accion: accion, Campos: map[string]string{
		"Ds_SignatureVersion": FirmaVersion, "Ds_MerchantParameters": params, "Ds_Signature": firma,
	}}}, nil
}

// ConsultarPago no inventa una consulta remota inexistente en la integración
// REST de Redsys. La ruta de webhook verifica la respuesta firmada y la convierte
// con EstadoParametros antes de tocar el pedido.
func (*Redsys) ConsultarPago(context.Context, string) (pagos.Estado, error) {
	return pagos.Estado{}, errUnavailable
}
func (*Redsys) Caducar(context.Context, string) error { return nil }

func (r *Redsys) Reembolsar(ctx context.Context, ref string, centimos int64, clave string) error {
	if r == nil || !ordenValido(ref) || centimos <= 0 || clave == "" {
		return errors.New("redsys: reembolso inválido")
	}
	parametros, err := json.Marshal(map[string]string{
		"DS_MERCHANT_AMOUNT":          strconv.FormatInt(centimos, 10),
		"DS_MERCHANT_CURRENCY":        "978",
		"DS_MERCHANT_MERCHANTCODE":    r.merchant,
		"DS_MERCHANT_ORDER":           ref,
		"DS_MERCHANT_TERMINAL":        r.terminal,
		"DS_MERCHANT_TRANSACTIONTYPE": "3",
	})
	if err != nil {
		return errUnavailable
	}
	encoded := base64.StdEncoding.EncodeToString(parametros)
	firma, err := Firmar(encoded, ref, r.key)
	if err != nil {
		return errUnavailable
	}
	cuerpo, err := json.Marshal(map[string]string{"Ds_SignatureVersion": FirmaVersion, "Ds_MerchantParameters": encoded, "Ds_Signature": firma})
	if err != nil {
		return errUnavailable
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, r.restURL, strings.NewReader(string(cuerpo)))
	if err != nil {
		return errUnavailable
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	resp, err := r.http.Do(req)
	if err != nil {
		return errUnavailable
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 64<<10))
		return errUnavailable
	}
	var resultado struct {
		Parametros string `json:"Ds_MerchantParameters"` //nolint:tagliatelle // Contrato REST de Redsys.
		Firma      string `json:"Ds_Signature"`          //nolint:tagliatelle // Contrato REST de Redsys.
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&resultado); err != nil || !Verificar(resultado.Parametros, ref, r.key, resultado.Firma) {
		return errUnavailable
	}
	var datos map[string]any
	if err := decodificarParametros(resultado.Parametros, &datos); err != nil || texto(datos, "Ds_Response") != "0900" || texto(datos, "Ds_Order") != ref || texto(datos, "Ds_Amount") != strconv.FormatInt(centimos, 10) {
		return errUnavailable
	}
	return nil
}

// EstadoParametros convierte el cuerpo de respuesta de Redsys DESPUÉS de
// comprobar Ds_Signature. En Redsys los metadatos viajan en MerchantData.
func (r *Redsys) EstadoParametros(parametros, firma string) (pagos.Estado, error) {
	var datos map[string]any
	if err := decodificarParametros(parametros, &datos); err != nil {
		return pagos.Estado{}, errUnavailable
	}
	orden := texto(datos, "Ds_Order")
	if !ordenValido(orden) || !Verificar(parametros, orden, r.key, firma) || texto(datos, "Ds_MerchantCode") != r.merchant || texto(datos, "Ds_Terminal") != r.terminal || texto(datos, "Ds_Currency") != "978" || texto(datos, "Ds_TransactionType") != "0" {
		return pagos.Estado{}, errUnavailable
	}
	importe, err := strconv.ParseInt(texto(datos, "Ds_Amount"), 10, 64)
	if err != nil || importe <= 0 {
		return pagos.Estado{}, errUnavailable
	}
	respuesta, err := strconv.Atoi(texto(datos, "Ds_Response"))
	if err != nil || respuesta < 0 || respuesta > 9999 {
		return pagos.Estado{}, errUnavailable
	}
	estado := pagos.EstadoFallido
	if respuesta <= 99 {
		estado = pagos.EstadoPagado
	}
	return pagos.Estado{Estado: estado, Importe: importe, Divisa: pagos.DivisaEUR, Modo: r.mode, Comercio: r.merchant + ":" + r.terminal, Metadatos: map[string]string{pagos.MetadatoPago: texto(datos, "Ds_MerchantData")}}, nil
}

func decodificarParametros(parametros string, destino any) error {
	b, err := base64.StdEncoding.DecodeString(parametros)
	if err != nil {
		b, err = base64.RawURLEncoding.DecodeString(parametros)
		if err != nil {
			return err
		}
	}
	return json.Unmarshal(b, destino)
}

func texto(datos map[string]any, clave string) string {
	v, _ := datos[clave].(string)
	return v
}

// Orden deriva del id del pago un Ds_Merchant_Order válido y estable: 4 dígitos
// y 8 caracteres en base 36 de su SHA-256 (unos 2,8·10^16 valores).
func Orden(pagoID string) string {
	h := sha256.Sum256([]byte(pagoID))
	n := binary.BigEndian.Uint64(h[:8])
	const alfabeto = "0123456789abcdefghijklmnopqrstuvwxyz"
	b := []byte(fmt.Sprintf("%04d", n%10000))
	n /= 10000
	for range 8 {
		b = append(b, alfabeto[n%36])
		n /= 36
	}
	return string(b)
}

func ordenValido(v string) bool {
	if len(v) != 12 {
		return false
	}
	for i, r := range v {
		if (i < 4 && (r < '0' || r > '9')) ||
			(i >= 4 && (r < '0' || r > '9') && (r < 'a' || r > 'z')) {
			return false
		}
	}
	return true
}

func urlHTTPS(raw string) bool {
	u, err := url.Parse(raw)
	return err == nil && u.Scheme == "https" && u.Host != "" && u.User == nil
}

func limitar(v string, n int) string {
	v = strings.TrimSpace(v)
	if len(v) > n {
		return v[:n]
	}
	return v
}

func (r *Redsys) String() string { return fmt.Sprintf("Redsys(%s)", r.mode) }
