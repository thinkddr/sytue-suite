// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package aihttp

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"git.sytue.eu/sytue/one/platform/pkg/authn"
	"git.sytue.eu/sytue/one/services/ai/internal/config"
	"git.sytue.eu/sytue/one/services/ai/internal/presupuesto"
	"git.sytue.eu/sytue/one/services/ai/internal/provider"
)

// fakeProvider implementa provider.Provider con una respuesta fija (o error).
type fakeProvider struct {
	text string
	err  error
}

func (f fakeProvider) Complete(_ context.Context, _, _ string, _ provider.Opciones) (provider.Resultado, error) {
	return provider.Resultado{Texto: f.text}, f.err
}
func (f fakeProvider) Name() string   { return "fake" }
func (f fakeProvider) Modelo() string { return "modelo-de-prueba" }

func testCfg() config.Config {
	return config.Config{
		Provider:            "scaleway",
		Model:               "mistral-small-3.2-24b-instruct-2506",
		Timeout:             5 * time.Second,
		MaxInstructionBytes: 8 * 1024,
		MaxContextBytes:     256 * 1024,
	}
}

func doAssist(h *handlers, body string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodPost, "/api/v1/assist", strings.NewReader(body))
	rec := httptest.NewRecorder()
	h.assist(rec, req)
	return rec
}

// errorCode extrae error.code del sobre canónico de httpx.
func errorCode(t *testing.T, body []byte) string {
	t.Helper()
	var env struct {
		Error struct {
			Code string `json:"code"`
		} `json:"error"`
	}
	if err := json.Unmarshal(body, &env); err != nil {
		t.Fatalf("respuesta no es JSON: %s", body)
	}
	return env.Error.Code
}

func TestAssist_HappyPath(t *testing.T) {
	h := &handlers{cfg: testCfg(), provider: fakeProvider{text: "resultado"}}
	rec := doAssist(h, `{"action":"reescribir","instruction":"hazlo formal","context":"hola","kind":"docx"}`)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body=%s", rec.Code, rec.Body.String())
	}
	var out struct {
		Text string `json:"text"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatalf("json: %v", err)
	}
	if out.Text != "resultado" {
		t.Errorf("text = %q", out.Text)
	}
}

func TestAssist_NotConfigured(t *testing.T) {
	h := &handlers{cfg: testCfg(), provider: nil}
	rec := doAssist(h, `{"action":"x","instruction":"y","context":"","kind":"docx"}`)
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d", rec.Code)
	}
	if code := errorCode(t, rec.Body.Bytes()); code != "ai_not_configured" {
		t.Errorf("code = %q", code)
	}
}

func TestAssist_MissingInstruction(t *testing.T) {
	h := &handlers{cfg: testCfg(), provider: fakeProvider{text: "x"}}
	rec := doAssist(h, `{"action":"x","instruction":"  ","context":"","kind":"docx"}`)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d", rec.Code)
	}
}

func TestAssist_ContextTooLarge(t *testing.T) {
	cfg := testCfg()
	cfg.MaxContextBytes = 8
	h := &handlers{cfg: cfg, provider: fakeProvider{text: "x"}}
	rec := doAssist(h, `{"action":"x","instruction":"y","context":"contexto demasiado grande","kind":"docx"}`)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d", rec.Code)
	}
}

func TestAssist_ProviderErrorsMapping(t *testing.T) {
	cases := []struct {
		name     string
		err      error
		wantCode int
		wantSlug string
	}{
		{"timeout", context.DeadlineExceeded, http.StatusGatewayTimeout, "ai_timeout"},
		{"refused", provider.ErrRefused, http.StatusUnprocessableEntity, "ai_refused"},
		{"empty", provider.ErrEmpty, http.StatusBadGateway, "ai_failed"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			h := &handlers{cfg: testCfg(), provider: fakeProvider{err: tc.err}}
			rec := doAssist(h, `{"action":"x","instruction":"y","context":"z","kind":"docx"}`)
			if rec.Code != tc.wantCode {
				t.Fatalf("status = %d, want %d", rec.Code, tc.wantCode)
			}
			if code := errorCode(t, rec.Body.Bytes()); code != tc.wantSlug {
				t.Errorf("code = %q, want %q", code, tc.wantSlug)
			}
		})
	}
}

func TestRateLimitKey(t *testing.T) {
	// Con claims: clave por tenant+usuario.
	req := httptest.NewRequest(http.MethodPost, "/api/v1/assist", http.NoBody)
	req = req.WithContext(authn.Into(req.Context(), authn.Claims{Subject: "u1", Tenant: "t1"}))
	if got := rateLimitKey(req); got != "u:t1:u1" {
		t.Errorf("clave = %q, want u:t1:u1", got)
	}

	// Sin claims: cae a la IP.
	plain := httptest.NewRequest(http.MethodPost, "/api/v1/assist", http.NoBody)
	if got := rateLimitKey(plain); !strings.HasPrefix(got, "ip:") {
		t.Errorf("clave = %q, want prefijo ip:", got)
	}
}

// --- Modo de atención (interactivo / calidad) --------------------------------
//
// Lo que se comprueba aquí es el ENRUTADO, que es donde está el riesgo: mandar
// a Gemma lo que debía ir a Mistral cuesta catorce segundos de espera en un
// panel en línea, y al revés cuesta una fórmula mal.

// nombraProvider es un fake que se identifica, para saber CUÁL atendió, y que
// deja ver con qué opciones se le llamó.
type nombraProvider struct {
	nombre string
	visto  chan provider.Opciones
}

// nombra construye el fake con un buzón holgado: los tests no leen `visto` salvo
// cuando comprueban las opciones, y un canal sin hueco bloquearía la llamada.
func nombra(n string) nombraProvider {
	return nombraProvider{nombre: n, visto: make(chan provider.Opciones, 8)}
}

func (p nombraProvider) Complete(_ context.Context, _, _ string, o provider.Opciones) (provider.Resultado, error) {
	p.visto <- o
	return provider.Resultado{Texto: p.nombre, TokensEntrada: 100, TokensSalida: 10}, nil
}
func (p nombraProvider) Name() string   { return p.nombre }
func (p nombraProvider) Modelo() string { return p.nombre }

func cfgDosModos() config.Config {
	c := testCfg()
	c.ModelCalidad = "gemma-4-26b-a4b-it"
	c.TimeoutCalidad = 120 * time.Second
	c.MaxTokensCalidad = 4096
	return c
}

func textoDe(t *testing.T, rec *httptest.ResponseRecorder) string {
	t.Helper()
	var out struct {
		Text string `json:"text"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatalf("json: %v — body=%s", err, rec.Body.String())
	}
	return out.Text
}

func TestAssist_ModoEligeProveedor(t *testing.T) {
	h := &handlers{
		cfg:      cfgDosModos(),
		provider: nombra("rapido"), calidad: nombra("bueno"),
	}

	for _, c := range []struct{ cuerpo, quiero string }{
		{`{"instruction":"x"}`, "rapido"},                      // sin modo: el rápido
		{`{"instruction":"x","modo":"interactivo"}`, "rapido"}, // explícito
		{`{"instruction":"x","modo":"calidad"}`, "bueno"},      // el bueno
	} {
		rec := doAssist(h, c.cuerpo)
		if rec.Code != http.StatusOK {
			t.Fatalf("%s: status = %d, body=%s", c.cuerpo, rec.Code, rec.Body.String())
		}
		if got := textoDe(t, rec); got != c.quiero {
			t.Errorf("%s: atendió %q, quería %q", c.cuerpo, got, c.quiero)
		}
	}
}

// Sin segundo modelo configurado, "calidad" NO es un error: se degrada al
// rápido. Es la diferencia entre responder peor y no responder.
func TestAssist_CalidadSinSegundoModeloCaeAlRapido(t *testing.T) {
	h := &handlers{cfg: testCfg(), provider: nombra("rapido"), calidad: nil}
	rec := doAssist(h, `{"instruction":"x","modo":"calidad"}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body=%s", rec.Code, rec.Body.String())
	}
	if got := textoDe(t, rec); got != "rapido" {
		t.Errorf("atendió %q, quería el rápido", got)
	}
}

func TestAssist_ModoDesconocidoSeRechaza(t *testing.T) {
	h := &handlers{cfg: cfgDosModos(), provider: nombra("rapido"), calidad: nombra("bueno")}
	rec := doAssist(h, `{"instruction":"x","modo":"turbo"}`)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d (se esperaba 400), body=%s", rec.Code, rec.Body.String())
	}
}

// El plazo del modo calidad debe ser el suyo, no el del interactivo: es la
// diferencia entre esperar los catorce segundos que tarda y cortar a los cinco.
func TestAssist_ModoCalidadUsaSuPlazo(t *testing.T) {
	h := &handlers{cfg: cfgDosModos(), provider: nombra("rapido"), calidad: nombra("bueno")}
	if _, plazo := h.elegir("calidad"); plazo != 120*time.Second {
		t.Errorf("plazo de calidad = %v, quería 120s", plazo)
	}
	if _, plazo := h.elegir(""); plazo != 5*time.Second {
		t.Errorf("plazo interactivo = %v, quería 5s", plazo)
	}
}

// Un truncado NO se puede confundir con una respuesta vacía: el primero se
// arregla subiendo AI_MAX_TOKENS y el segundo mirando al proveedor.
func TestAssist_TruncadoTieneSuPropioCodigo(t *testing.T) {
	h := &handlers{cfg: testCfg(), provider: fakeProvider{err: provider.ErrTruncated}}
	rec := doAssist(h, `{"instruction":"x"}`)
	if rec.Code != http.StatusBadGateway {
		t.Fatalf("status = %d", rec.Code)
	}
	if got := errorCode(t, rec.Body.Bytes()); got != "ai_truncated" {
		t.Errorf("código = %q, quería ai_truncated", got)
	}
}

// --- Presupuesto y degradación ------------------------------------------------
//
// El comportamiento que se protege aquí es el que pidió el usuario: agotar el
// tope NO deja sin asistente, cambia de modelo. Un 429 o un 402 en su lugar
// serían un fallo de producto, no una protección.

// contadorFalso implementa lo justo para forzar «con presupuesto» y «sin él».
// Se usa el Contador de verdad con caché nil (permite siempre) y, para el caso
// agotado, un tope negativo no vale: hay que simular la caché. Como el paquete
// degrada abierto sin caché, el caso «agotado» se prueba en su propio paquete;
// aquí se comprueba el CABLEADO con un contador que siempre permite.

func TestAssist_RespuestaDiceModeloYDegradacion(t *testing.T) {
	h := &handlers{cfg: cfgDosModos(), provider: nombra("rapido"), calidad: nombra("bueno")}
	rec := doAssist(h, `{"instruction":"x","modo":"calidad"}`)
	var out struct {
		Text      string `json:"text"`
		Modelo    string `json:"modelo"`
		Degradado bool   `json:"degradado"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatalf("json: %v — %s", err, rec.Body.String())
	}
	if out.Modelo != "bueno" || out.Degradado {
		t.Errorf("modelo=%q degradado=%v; se esperaba bueno/false", out.Modelo, out.Degradado)
	}
}

// El esfuerzo de razonamiento llega al proveedor tal cual: es el parámetro que
// decide un factor ocho de coste, así que perderlo por el camino no se ve en
// ningún test de contrato pero se ve en la factura.
func TestAssist_EsfuerzoLlegaAlProveedor(t *testing.T) {
	bueno := nombra("bueno")
	h := &handlers{cfg: cfgDosModos(), provider: nombra("rapido"), calidad: bueno}
	if rec := doAssist(h, `{"instruction":"x","modo":"calidad","esfuerzo":"none"}`); rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body=%s", rec.Code, rec.Body.String())
	}
	select {
	case o := <-bueno.visto:
		if o.Esfuerzo != "none" {
			t.Errorf("esfuerzo = %q, quería none", o.Esfuerzo)
		}
	default:
		t.Fatal("el proveedor no recibió ninguna llamada")
	}
}

func TestAssist_EsfuerzoDesconocidoSeRechaza(t *testing.T) {
	h := &handlers{cfg: cfgDosModos(), provider: nombra("rapido"), calidad: nombra("bueno")}
	if rec := doAssist(h, `{"instruction":"x","esfuerzo":"maximo"}`); rec.Code != http.StatusBadRequest {
		t.Errorf("status = %d, se esperaba 400", rec.Code)
	}
}

// La imagen llega decodificada al proveedor: es lo que habilita OCR y la receta
// de revelado de Fotos.
func TestAssist_ImagenLlegaDecodificada(t *testing.T) {
	rapido := nombra("rapido")
	h := &handlers{cfg: cfgDosModos(), provider: rapido, calidad: nombra("bueno")}
	// "PNG" en base64.
	if rec := doAssist(h, `{"instruction":"x","imagen":"UE5H","imagen_tipo":"image/jpeg"}`); rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body=%s", rec.Code, rec.Body.String())
	}
	select {
	case o := <-rapido.visto:
		if o.Imagen == nil {
			t.Fatal("el proveedor no recibió imagen")
		}
		if string(o.Imagen.Datos) != "PNG" || o.Imagen.Tipo != "image/jpeg" {
			t.Errorf("imagen = %q / %q", o.Imagen.Datos, o.Imagen.Tipo)
		}
	default:
		t.Fatal("el proveedor no recibió ninguna llamada")
	}
}

func TestAssist_ImagenNoBase64SeRechaza(t *testing.T) {
	h := &handlers{cfg: cfgDosModos(), provider: nombra("rapido"), calidad: nombra("bueno")}
	if rec := doAssist(h, `{"instruction":"x","imagen":"esto no es base64 %%%"}`); rec.Code != http.StatusBadRequest {
		t.Errorf("status = %d, se esperaba 400", rec.Code)
	}
}

// presupuestoFalso permite decidir el caso a probar y apunta lo cobrado.
type presupuestoFalso struct {
	permite bool
	gastado float64
	cobrado float64
}

func (p *presupuestoFalso) Permite(context.Context, string, string) (bool, float64) {
	return p.permite, p.gastado
}

func (p *presupuestoFalso) Cobrar(_ context.Context, _, _ string, euros float64) float64 {
	p.cobrado += euros
	return p.cobrado
}

// EL COMPORTAMIENTO CENTRAL: agotado el tope, la petición de calidad se sirve
// con el modelo rápido. No es un 429 ni un 402: el usuario sigue teniendo
// asistente, y la respuesta lo dice para que la interfaz pueda avisar.
func TestAssist_PresupuestoAgotadoDegradaAlRapido(t *testing.T) {
	pf := &presupuestoFalso{permite: false, gastado: 1.51}
	h := &handlers{
		cfg: cfgDosModos(), provider: nombra("rapido"), calidad: nombra("bueno"),
		presupuesto: pf, tarifas: map[string]presupuesto.Precio{
			"rapido": {Entrada: 0.15, Salida: 0.35},
			"bueno":  {Entrada: 0.25, Salida: 0.50},
		},
	}
	rec := doAssist(h, `{"instruction":"x","modo":"calidad"}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body=%s — agotar el tope NO debe fallar", rec.Code, rec.Body.String())
	}
	var out struct {
		Text      string `json:"text"`
		Modelo    string `json:"modelo"`
		Degradado bool   `json:"degradado"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatalf("json: %v", err)
	}
	if out.Text != "rapido" || out.Modelo != "rapido" {
		t.Errorf("atendió %q/%q, quería el rápido", out.Text, out.Modelo)
	}
	if !out.Degradado {
		t.Error("la respuesta no marca `degradado`: la interfaz no podría avisar")
	}
	// Y lo servido se cobra igual, con la tarifa del modelo que SÍ atendió.
	// 100 tokens de entrada y 10 de salida a 0,15/0,35 = 1,85e-5 €.
	if pf.cobrado <= 0 {
		t.Error("la petición degradada no se cobró")
	}
	if quiero := 100.0/1e6*0.15 + 10.0/1e6*0.35; pf.cobrado != quiero {
		t.Errorf("cobrado = %v, quería %v (tarifa del rápido, no la del caro)", pf.cobrado, quiero)
	}
}

// Con presupuesto disponible se usa el caro y se cobra a SU tarifa.
func TestAssist_ConPresupuestoUsaElCaroYCobraSuTarifa(t *testing.T) {
	pf := &presupuestoFalso{permite: true}
	h := &handlers{
		cfg: cfgDosModos(), provider: nombra("rapido"), calidad: nombra("bueno"),
		presupuesto: pf, tarifas: map[string]presupuesto.Precio{
			"bueno": {Entrada: 0.25, Salida: 0.50},
		},
	}
	if rec := doAssist(h, `{"instruction":"x","modo":"calidad"}`); rec.Code != http.StatusOK {
		t.Fatalf("status = %d", rec.Code)
	}
	if quiero := 100.0/1e6*0.25 + 10.0/1e6*0.50; pf.cobrado != quiero {
		t.Errorf("cobrado = %v, quería %v", pf.cobrado, quiero)
	}
}

// El modo interactivo NO consulta el presupuesto: el modelo rápido no tiene
// tope. Consultarlo sería gastar un viaje a la caché en cada tecla.
func TestAssist_InteractivoNoMiraElPresupuesto(t *testing.T) {
	pf := &presupuestoFalso{permite: false}
	h := &handlers{cfg: cfgDosModos(), provider: nombra("rapido"), calidad: nombra("bueno"), presupuesto: pf}
	rec := doAssist(h, `{"instruction":"x"}`)
	if rec.Code != http.StatusOK || textoDe(t, rec) != "rapido" {
		t.Errorf("status=%d texto=%q", rec.Code, rec.Body.String())
	}
}

// Un modelo sin tarifa se sirve igual y no se cobra: mejor no cobrar que
// inventarse un precio.
func TestAssist_ModeloSinTarifaNoSeCobra(t *testing.T) {
	pf := &presupuestoFalso{permite: true}
	h := &handlers{
		cfg: cfgDosModos(), provider: nombra("rapido"), calidad: nombra("bueno"),
		presupuesto: pf, tarifas: map[string]presupuesto.Precio{},
	}
	if rec := doAssist(h, `{"instruction":"x","modo":"calidad"}`); rec.Code != http.StatusOK {
		t.Fatalf("status = %d", rec.Code)
	}
	if pf.cobrado != 0 {
		t.Errorf("cobrado = %v, quería 0", pf.cobrado)
	}
}

// --- Transcripción por trozos -------------------------------------------------

// transcriptorDePrueba devuelve el nombre del fichero que le llega, que es lo
// que permite comprobar el ORDEN sin depender de ningún modelo.
type transcriptorDePrueba struct {
	vistos  []string
	fallaEn int // 1-based; 0 = no falla
}

func (t *transcriptorDePrueba) Modelo() string { return "prueba" }

func (t *transcriptorDePrueba) Transcribir(_ context.Context, audio []byte, nombre, _ string) (string, error) {
	t.vistos = append(t.vistos, nombre)
	if t.fallaEn == len(t.vistos) {
		return "", errors.New("el modelo dijo que no")
	}
	return string(audio), nil
}

// EL ORDEN ES LA FUNCIONALIDAD: una reunión transcrita en desorden no sirve para
// nada, y es justo lo que pasaría al lanzar los trozos en paralelo por ir más
// rápido.
func TestTranscribirTodoRespetaElOrden(t *testing.T) {
	tp := &transcriptorDePrueba{}
	texto, err := transcribirTodo(context.Background(), tp,
		[][]byte{[]byte("uno"), []byte("dos"), []byte("tres")}, "reunion.ogg", "es")
	if err != nil {
		t.Fatalf("transcribirTodo: %v", err)
	}
	if texto != "uno\n\ndos\n\ntres" {
		t.Errorf("texto = %q", texto)
	}
	// Y cada trozo conserva la extensión: de ahí deduce el proveedor el contenedor.
	quiero := []string{"reunion-1.ogg", "reunion-2.ogg", "reunion-3.ogg"}
	if len(tp.vistos) != 3 || tp.vistos[0] != quiero[0] || tp.vistos[2] != quiero[2] {
		t.Errorf("nombres = %v, quería %v", tp.vistos, quiero)
	}
}

// Un hueco silencioso en mitad de una hora de reunión, sin avisar, es peor que
// no devolver nada: nadie sabría que le falta un trozo.
func TestUnTrozoQueFallaTumbaLaTranscripcion(t *testing.T) {
	tp := &transcriptorDePrueba{fallaEn: 2}
	_, err := transcribirTodo(context.Background(), tp,
		[][]byte{[]byte("uno"), []byte("dos"), []byte("tres")}, "reunion.ogg", "")
	if err == nil {
		t.Fatal("un trozo perdido tiene que fallar")
	}
	if !strings.Contains(err.Error(), "trozo 2 de 3") {
		t.Errorf("el error no dice cuál falló: %v", err)
	}
}

// Con un solo trozo el nombre no se toca: es el caso de la nota de voz de siempre.
func TestUnSoloAudioConservaSuNombre(t *testing.T) {
	tp := &transcriptorDePrueba{}
	if _, err := transcribirTodo(context.Background(), tp, [][]byte{[]byte("hola")}, "nota.webm", ""); err != nil {
		t.Fatalf("transcribirTodo: %v", err)
	}
	if len(tp.vistos) != 1 || tp.vistos[0] != "nota.webm" {
		t.Errorf("nombres = %v", tp.vistos)
	}
}

// Los trozos vacíos no dejan líneas en blanco sueltas: un silencio no es un párrafo.
func TestLosTrozosVaciosNoDejanHuecos(t *testing.T) {
	tp := &transcriptorDePrueba{}
	texto, err := transcribirTodo(context.Background(), tp,
		[][]byte{[]byte("uno"), []byte("   "), []byte("dos")}, "r.ogg", "")
	if err != nil {
		t.Fatalf("transcribirTodo: %v", err)
	}
	if texto != "uno\n\ndos" {
		t.Errorf("texto = %q", texto)
	}
}

func TestInternalPages_SoloDesdeDentroAndAuth(t *testing.T) {
	cfg := testCfg()
	cfg.PagesSecret = "secret-pages-123"
	cfg.TimeoutPages = 5 * time.Second
	cfg.MaxTokensTope = 16384

	router := NewRouter(Deps{
		Cfg:      cfg,
		Provider: fakeProvider{text: "respuesta interna"},
	})

	// 1. Sin secreto -> 401
	req := httptest.NewRequest(http.MethodPost, "/internal/v1/assist", strings.NewReader(`{"action":"a","instruction":"i"}`))
	req.Header.Set("X-Sytue-Tenant", "tenant-1")
	req.Header.Set("X-Sytue-Sub", "sub-1")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Errorf("sin secreto esperada 401, got %d", rec.Code)
	}

	// 2. Con X-Forwarded-For -> 404 (SoloDesdeDentro)
	req = httptest.NewRequest(http.MethodPost, "/internal/v1/assist", strings.NewReader(`{"action":"a","instruction":"i"}`))
	req.Header.Set("Authorization", "Bearer secret-pages-123")
	req.Header.Set("X-Forwarded-For", "203.0.113.195")
	req.Header.Set("X-Sytue-Tenant", "tenant-1")
	req.Header.Set("X-Sytue-Sub", "sub-1")
	rec = httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Errorf("con X-Forwarded-For esperada 404, got %d", rec.Code)
	}

	// 3. Sin X-Sytue-Tenant o X-Sytue-Sub -> 400
	req = httptest.NewRequest(http.MethodPost, "/internal/v1/assist", strings.NewReader(`{"action":"a","instruction":"i"}`))
	req.Header.Set("Authorization", "Bearer secret-pages-123")
	rec = httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("sin tenant/sub esperada 400, got %d", rec.Code)
	}

	// 4. Formato de identidad inválido -> 400
	req = httptest.NewRequest(http.MethodPost, "/internal/v1/assist", strings.NewReader(`{"action":"a","instruction":"i"}`))
	req.Header.Set("Authorization", "Bearer secret-pages-123")
	req.Header.Set("X-Sytue-Tenant", "tenant\ncon salto")
	req.Header.Set("X-Sytue-Sub", "sub-1")
	rec = httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("con caracteres inválidos en tenant esperada 400, got %d", rec.Code)
	}
}

func TestInternalPages_AssistHappyPath(t *testing.T) {
	cfg := testCfg()
	cfg.PagesSecret = "secret-pages-123"
	cfg.TimeoutPages = 5 * time.Second
	cfg.MaxTokensTope = 16384

	router := NewRouter(Deps{
		Cfg:      cfg,
		Provider: fakeProvider{text: "{\"seccion\":\"hero\"}"},
	})

	body := `{
		"action": "generar",
		"instruction": "Crea una sección hero",
		"context": "Empresa de fontanería",
		"kind": "astro",
		"sistema": "pages",
		"esquema": {"type": "object"},
		"max_tokens": 4096
	}`

	req := httptest.NewRequest(http.MethodPost, "/internal/v1/assist", strings.NewReader(body))
	req.Header.Set("Authorization", "Bearer secret-pages-123")
	req.Header.Set("X-Sytue-Tenant", "org-acme")
	req.Header.Set("X-Sytue-Sub", "usr-123")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}

	var res struct {
		Text          string `json:"text"`
		Modelo        string `json:"modelo"`
		Degradado     bool   `json:"degradado"`
		TokensEntrada int    `json:"tokens_entrada"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &res); err != nil {
		t.Fatalf("json: %v", err)
	}
	if res.Text != `{"seccion":"hero"}` {
		t.Errorf("Text = %q", res.Text)
	}
	if res.Degradado {
		t.Errorf("Degradado no debería ser true para pages")
	}
}

func TestInternalPages_ValidationLimits(t *testing.T) {
	cfg := testCfg()
	cfg.PagesSecret = "secret-pages-123"
	cfg.TimeoutPages = 5 * time.Second
	cfg.MaxTokensTope = 16384

	router := NewRouter(Deps{
		Cfg:      cfg,
		Provider: fakeProvider{text: "ok"},
	})

	doPost := func(body string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodPost, "/internal/v1/assist", strings.NewReader(body))
		req.Header.Set("Authorization", "Bearer secret-pages-123")
		req.Header.Set("X-Sytue-Tenant", "org-1")
		req.Header.Set("X-Sytue-Sub", "usr-1")
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)
		return rec
	}

	// max_tokens > tope
	rec := doPost(`{"action":"a","instruction":"i","max_tokens":999999}`)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("max_tokens > tope esperaba 400, got %d", rec.Code)
	}

	// esquema no es JSON válido
	rec = doPost(`{"action":"a","instruction":"i","esquema":"no-es-json-valido"}`)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("esquema inválido esperaba 400, got %d", rec.Code)
	}

	// sistema desconocido
	rec = doPost(`{"action":"a","instruction":"i","sistema":"desconocido"}`)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("sistema desconocido esperaba 400, got %d", rec.Code)
	}
}
