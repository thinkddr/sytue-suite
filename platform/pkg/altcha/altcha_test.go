// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package altcha

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestAltcha_CreateAndVerify(t *testing.T) {
	v := New([]byte("clave-secreta-test-2026"))

	ch, err := v.CreateChallenge(500, time.Minute)
	if err != nil {
		t.Fatalf("CreateChallenge falló: %v", err)
	}

	if ch.Algorithm != "SHA-256" || ch.Challenge == "" || ch.Salt == "" || ch.Signature == "" {
		t.Fatalf("Challenge incompleto: %+v", ch)
	}

	sol, err := SolveChallenge(ch)
	if err != nil {
		t.Fatalf("SolveChallenge falló: %v", err)
	}

	ok, err := v.Verify(context.Background(), sol)
	if err != nil || !ok {
		t.Fatalf("Verify(sol) = (%v, %v); quería (true, nil)", ok, err)
	}

	// Replay: el mismo payload debe ser rechazado
	ok2, err2 := v.Verify(context.Background(), sol)
	if ok2 || !errors.Is(err2, ErrReplayDetected) {
		t.Fatalf("Replay no detectado: (%v, %v)", ok2, err2)
	}
}

func TestAltcha_FirmaAlterada(t *testing.T) {
	v := New([]byte("clave-secreta-test-2026"))
	ch, _ := v.CreateChallenge(100, time.Minute)
	sol, _ := SolveChallenge(ch)

	raw, _ := base64.StdEncoding.DecodeString(sol)
	var p Payload
	_ = json.Unmarshal(raw, &p)
	p.Signature = "alterada1234567890abcdef"
	b, _ := json.Marshal(p)
	tampered := base64.StdEncoding.EncodeToString(b)

	ok, err := v.Verify(context.Background(), tampered)
	if ok || !errors.Is(err, ErrInvalidSignature) {
		t.Fatalf("esperaba ErrInvalidSignature, obtuve (%v, %v)", ok, err)
	}
}

func TestAltcha_NumeroIncorrecto(t *testing.T) {
	v := New([]byte("clave-secreta-test-2026"))
	ch, _ := v.CreateChallenge(100, time.Minute)
	sol, _ := SolveChallenge(ch)

	raw, _ := base64.StdEncoding.DecodeString(sol)
	var p Payload
	_ = json.Unmarshal(raw, &p)
	p.Number++ // Número equivocado
	b, _ := json.Marshal(p)
	tampered := base64.StdEncoding.EncodeToString(b)

	ok, err := v.Verify(context.Background(), tampered)
	if ok || !errors.Is(err, ErrInvalidSolution) {
		t.Fatalf("esperaba ErrInvalidSolution, obtuve (%v, %v)", ok, err)
	}
}

func TestAltcha_Expirado(t *testing.T) {
	v := New([]byte("clave-secreta-test-2026"))
	// TTL negativo: ya nace expirado
	ch, _ := v.CreateChallenge(100, -time.Second)
	sol, _ := SolveChallenge(ch)

	ok, err := v.Verify(context.Background(), sol)
	if ok || !errors.Is(err, ErrExpired) {
		t.Fatalf("esperaba ErrExpired, obtuve (%v, %v)", ok, err)
	}
}

func TestTestBypassRoundTrip(t *testing.T) {
	clave := []byte("clave-de-pruebas-solo-del-despliegue")
	ahora := time.Now()
	msg := SignTestBypass(clave, "ops@sytue.eu", ahora)
	if !VerifyTestBypass(clave, msg, "ops@sytue.eu", ahora) {
		t.Fatal("un mensaje recién firmado se rechaza")
	}
	// Ventana de ±5 min: bordes válidos, fuera de ella no.
	if !VerifyTestBypass(clave, SignTestBypass(clave, "ops@sytue.eu", ahora.Add(-4*time.Minute)), "ops@sytue.eu", ahora) {
		t.Error("un mensaje de hace 4 minutos se rechaza: la ventana es ±5 min")
	}
	if !VerifyTestBypass(clave, SignTestBypass(clave, "ops@sytue.eu", ahora.Add(4*time.Minute)), "ops@sytue.eu", ahora) {
		t.Error("un mensaje de +4 min (reloj adelantado del runner) se rechaza")
	}
	if VerifyTestBypass(clave, SignTestBypass(clave, "ops@sytue.eu", ahora.Add(-10*time.Minute)), "ops@sytue.eu", ahora) {
		t.Error("un mensaje de hace 10 minutos (fuera de ventana) se acepta")
	}
	if VerifyTestBypass(clave, msg, "otra@sytue.eu", ahora) {
		t.Error("un mensaje firmado para un email se acepta para OTRO")
	}
	if VerifyTestBypass(clave, msg+"x", "ops@sytue.eu", ahora) {
		t.Error("una firma manipulada se acepta")
	}
	if VerifyTestBypass([]byte("otra-clave"), msg, "ops@sytue.eu", ahora) {
		t.Error("una clave distinta verifica el mensaje")
	}
	if VerifyTestBypass(nil, msg, "ops@sytue.eu", ahora) {
		t.Error("sin clave configurada el bypass existe")
	}
	if VerifyTestBypass(clave, "", "ops@sytue.eu", ahora) {
		t.Error("un mensaje vacío se acepta")
	}
	if VerifyTestBypass(clave, "notimestamp|firma", "ops@sytue.eu", ahora) {
		t.Error("un mensaje sin timestamp numérico se acepta")
	}
}

// Con ConRepeticiones, la memoria del proceso no cuenta: manda el almacén que se
// pasa, que recibe la firma y la caducidad del reto, y cuyo error cierra la puerta.
func TestConRepeticionesMandaElAlmacen(t *testing.T) {
	var (
		mu     sync.Mutex
		vistas = map[string]time.Time{}
	)
	almacen := func(_ context.Context, firma string, caduca time.Time) (bool, error) {
		mu.Lock()
		defer mu.Unlock()
		if _, ok := vistas[firma]; ok {
			return false, nil
		}
		vistas[firma] = caduca
		return true, nil
	}
	// Dos verificadores con la misma clave y el mismo almacén son dos réplicas:
	// la solución que acepta una no la acepta la otra.
	a := New([]byte("clave"), ConRepeticiones(almacen))
	b := New([]byte("clave"), ConRepeticiones(almacen))
	ch, err := a.CreateChallenge(100, time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	sol, _ := SolveChallenge(ch)
	if ok, err := a.Verify(context.Background(), sol); !ok || err != nil {
		t.Fatalf("primera vez: (%v, %v)", ok, err)
	}
	if ok, err := b.Verify(context.Background(), sol); ok || !errors.Is(err, ErrReplayDetected) {
		t.Fatalf("la otra réplica acepta la misma solución: (%v, %v)", ok, err)
	}
	for firma, caduca := range vistas {
		if firma != ch.Signature || time.Until(caduca) <= 0 || time.Until(caduca) > time.Minute+time.Second {
			t.Fatalf("el almacén recibe %q hasta %v", firma, caduca)
		}
	}

	roto := New([]byte("clave"), ConRepeticiones(func(context.Context, string, time.Time) (bool, error) {
		return false, errors.New("base caída")
	}))
	ch2, _ := roto.CreateChallenge(100, time.Minute)
	sol2, _ := SolveChallenge(ch2)
	if ok, err := roto.Verify(context.Background(), sol2); ok || err == nil || errors.Is(err, ErrReplayDetected) {
		t.Fatalf("con el almacén caído se acepta o se confunde con una repetición: (%v, %v)", ok, err)
	}
}

// Un reto de ámbito solo vale en su ámbito y con la dificultad que se exige al
// comprobar: el reto barato de una web no sirve en otra, ni en la misma cuando
// ahora pide más. Y ninguno de los dos mundos acepta el reto del otro.
func TestElAmbitoYLaDificultadVanFirmados(t *testing.T) {
	ctx := context.Background()
	v := New([]byte("clave"))
	resolver := func(ambito string, dificultad int64) string {
		t.Helper()
		ch, err := v.CrearReto(ambito, dificultad, time.Minute)
		if err != nil {
			t.Fatal(err)
		}
		sol, err := SolveChallenge(ch)
		if err != nil {
			t.Fatal(err)
		}
		return sol
	}

	if err := v.Comprobar(ctx, "sitio-a:checkout", 200, resolver("sitio-a:checkout", 300)); err != nil {
		t.Fatalf("un reto de su ámbito y dificultad se rechaza: %v", err)
	}
	if err := v.Comprobar(ctx, "sitio-b:checkout", 200, resolver("sitio-a:checkout", 300)); !errors.Is(err, ErrWrongScope) {
		t.Fatalf("el reto de otro sitio vale: %v", err)
	}
	if err := v.Comprobar(ctx, "sitio-a:checkout", 400, resolver("sitio-a:checkout", 300)); !errors.Is(err, ErrTooEasy) {
		t.Fatalf("un reto más fácil de lo exigido vale: %v", err)
	}
	if ok, err := v.Verify(context.Background(), resolver("sitio-a:checkout", 300)); ok || !errors.Is(err, ErrWrongScope) {
		t.Fatalf("un reto de ámbito vale sin ámbito: (%v, %v)", ok, err)
	}
	ch, _ := v.CreateChallenge(100, time.Minute)
	sinAmbito, _ := SolveChallenge(ch)
	if err := v.Comprobar(ctx, "sitio-a:checkout", 0, sinAmbito); !errors.Is(err, ErrWrongScope) {
		t.Fatalf("un reto sin ámbito vale en un ámbito: %v", err)
	}
	if err := v.Comprobar(ctx, "", 0, sinAmbito); !errors.Is(err, ErrWrongScope) {
		t.Fatalf("Comprobar con ámbito vacío acepta: %v", err)
	}
	if _, err := v.CrearReto("", 100, time.Minute); err == nil {
		t.Fatal("un reto de ámbito sin ámbito se crea")
	}

	// Cambiar el ámbito en la sal rompe la solución: va firmado con el reto.
	raw, _ := base64.StdEncoding.DecodeString(resolver("sitio-a:checkout", 300))
	var p Payload
	_ = json.Unmarshal(raw, &p)
	p.Salt = strings.Replace(p.Salt, "sitio-a", "sitio-b", 1)
	b, _ := json.Marshal(p)
	if err := v.Comprobar(ctx, "sitio-b:checkout", 0, base64.StdEncoding.EncodeToString(b)); !errors.Is(err, ErrInvalidSolution) {
		t.Fatalf("una sal con el ámbito cambiado vale: %v", err)
	}
}
