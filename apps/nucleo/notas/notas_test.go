// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package notas

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"
	"testing"

	"git.sytue.eu/sytue/one/apps/nucleo/auth/authtest"

	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/offline"
)

// TestSubirLlevaLosBorradoresYLaCola: al cerrar sesión con «Subir», cada borrador sale
// como la nota que habría hecho Intro en su cuaderno (o en su día, si aún no existe),
// la cola también, y no queda nada. Sin red, todo se queda y se cuenta.
func TestSubirLlevaLosBorradoresYLaCola(t *testing.T) {
	st, err := offline.OpenWithSecrets(t.TempDir(), new(authtest.Secrets))
	if err != nil {
		t.Fatal(err)
	}
	const dueño = "ada/acme"
	for id, texto := range map[string]string{"nb-1": "[ ] comprar pan", PrefijoDia + "2026-10-17": "idea suelta", "nb-2": "   "} {
		if err := st.Write(dueño, EspacioBorradores, id, texto); err != nil {
			t.Fatal(err)
		}
	}
	cola := Captura{ClientID: "c1", Cuaderno: "nb-3", In: api.NoteInput{Kind: "text", Content: "<p>en cola</p>"}.ConClientID("c1")}
	if err := st.Write(dueño, EspacioCapturas, "c1", cola); err != nil {
		t.Fatal(err)
	}

	var enviadas []string
	sinRed := true
	e := Envio{
		Hoy: "2026-10-16",
		Crear: func(_ context.Context, nb string, in api.NoteInput) (api.Note, error) {
			if sinRed {
				return api.Note{}, errors.New("sin red")
			}
			if in.ClientID() == "" {
				t.Errorf("la nota a %s sale sin client_id: reintentar la duplicaría", nb)
			}
			enviadas = append(enviadas, nb+":"+in.Kind)
			return api.Note{}, nil
		},
		Diario: func(_ context.Context, date, _ string, _ bool) (api.Notebook, error) {
			return api.Notebook{ID: "dia-" + date}, nil
		},
	}
	n := 0
	nuevo := func() string { n++; return string(rune('a' + n)) }

	if quedan := Subir(context.Background(), st, dueño, e, nuevo); quedan != 3 {
		t.Fatalf("sin red quedan %d; quiero 3 (dos borradores, ya en la cola, y la captura)", quedan)
	}
	sinRed = false
	if quedan := Subir(context.Background(), st, dueño, e, nuevo); quedan != 0 {
		t.Fatalf("con red quedan %d; quiero 0", quedan)
	}
	slices.Sort(enviadas)
	if want := []string{"dia-2026-10-17:text", "nb-1:todo", "nb-3:text"}; !slices.Equal(enviadas, want) {
		t.Fatalf("enviadas %v; quiero %v", enviadas, want)
	}
}

func TestVaciarConservaYMarcaCapturaCifradaConEnlacesEnClaro(t *testing.T) {
	st, err := offline.OpenWithSecrets(t.TempDir(), new(authtest.Secrets))
	if err != nil {
		t.Fatal(err)
	}
	const owner = "ada/acme"
	c := Captura{ClientID: "legacy", Fecha: "2026-10-17", In: api.NoteInput{ContentCipher: "encv1:synthetic", CipherVersion: 2, Links: []api.NoteLink{{TargetKind: "docs_document", TargetID: "secret", Label: "private"}}}}
	if err := st.Write(owner, EspacioCapturas, c.ClientID, c); err != nil {
		t.Fatal(err)
	}
	calls := 0
	e := Envio{Crear: func(context.Context, string, api.NoteInput) (api.Note, error) { calls++; return api.Note{}, nil }, Diario: func(context.Context, string, string, bool) (api.Notebook, error) { calls++; return api.Notebook{}, nil }}
	Vaciar(t.Context(), st, owner, e)
	got := Pendientes(st, owner)
	var stored Captura
	found, err := st.Read(owner, EspacioCapturas, c.ClientID, &stored)
	if calls != 0 || len(got) != 1 || !got[0].Rechazada || err != nil || !found || len(stored.In.Links) != 1 || stored.In.Links[0] != c.In.Links[0] || stored.In.ContentCipher != c.In.ContentCipher || stored.Rechazada {
		t.Fatal("unsafe capture was sent, erased or altered", calls, got)
	}
}

func TestVaciarRetieneBytesLegadosAmbiguosSinRed(t *testing.T) {
	st, err := offline.OpenWithSecrets(t.TempDir(), new(authtest.Secrets))
	if err != nil {
		t.Fatal(err)
	}
	const owner = "ada/acme"
	inputs := []string{
		`{"client_id":"x","notebook_id":"book","input":{"cipher_version":2,"content_cipher":"encv1:x","Links":[{"target_kind":"note","target_id":"secret"}]}}`,
		`{"client_id":"x","notebook_id":"book","input":{"cipher_version":2,"content_cipher":"encv1:x","links":[],"links":[{"target_kind":"note","target_id":"secret"}]}}`,
		`{"client_id":"x","notebook_id":"book","input":{"cipher_version":2,"content_cipher":"encv1:x","links":{"label":"secret"}}}`,
		`{"client_id":"x","notebook_id":"book","input":{"cipher_version":2,"content_cipher":"encv1:x","data":{"attachment":"secret"}}}`,
		`{"client_id":"x","notebook_id":"book","input":{"cipher_version":2,"content_cipher":"encv1:x","expected_crypto":{"secret":"private"}}}`,
		`{"client_id":"x","notebook_id":"book","input":{"cipher_version":2,"content_cipher":"encv1:x","title":"secret"}}`,
		`{"client_id":"x","notebook_id":"book","input":{"cipher_version":2,"content_cipher":"encv1:x","unknown":"secret"}}`,
		`{"client_id":"x","notebook_id":"book","input":{"cipher_version":2,"content_cipher":"encv1:x","title":"\ud800"}}`,
	}
	for _, raw := range inputs {
		if err := st.Write(owner, EspacioCapturas, "x", json.RawMessage(raw)); err != nil {
			t.Fatal(err)
		}
		var before json.RawMessage
		if found, err := st.Read(owner, EspacioCapturas, "x", &before); err != nil || !found {
			t.Fatal(found, err)
		}
		calls := 0
		e := Envio{Crear: func(context.Context, string, api.NoteInput) (api.Note, error) { calls++; return api.Note{}, nil }, Diario: func(context.Context, string, string, bool) (api.Notebook, error) { calls++; return api.Notebook{}, nil }}
		Vaciar(t.Context(), st, owner, e)
		var after json.RawMessage
		found, err := st.Read(owner, EspacioCapturas, "x", &after)
		pending := Pendientes(st, owner)
		if err != nil || !found || calls != 0 || !bytes.Equal(before, after) || len(pending) != 1 || !pending[0].Rechazada {
			t.Fatal("ambiguous source was sent, altered or hidden", found, calls, err)
		}
	}
}

func TestCapturaExigeIdentidadIgualEnRegistroYEntrada(t *testing.T) {
	st, err := offline.OpenWithSecrets(t.TempDir(), new(authtest.Secrets))
	if err != nil {
		t.Fatal(err)
	}
	const owner = "ada/acme"
	for _, raw := range []string{
		`{"client_id":"other","notebook_id":"book","input":{"data":{"client_id":"x"}}}`,
		`{"client_id":"x","notebook_id":"book","input":{"data":{"client_id":"other"}}}`,
		`{"client_id":"x","notebook_id":"book","input":{"data":{}}}`,
		`{"client_id":"x","notebook_id":"book","input":{"data":{"client_id":"x","Client_ID":"other"}}}`,
	} {
		if err := st.Write(owner, EspacioCapturas, "x", json.RawMessage(raw)); err != nil {
			t.Fatal(err)
		}
		var before json.RawMessage
		if found, err := st.Read(owner, EspacioCapturas, "x", &before); err != nil || !found {
			t.Fatal(found, err)
		}
		calls := 0
		e := Envio{Crear: func(context.Context, string, api.NoteInput) (api.Note, error) { calls++; return api.Note{}, nil }, Diario: func(context.Context, string, string, bool) (api.Notebook, error) { calls++; return api.Notebook{}, nil }}
		Vaciar(t.Context(), st, owner, e)
		var after json.RawMessage
		found, err := st.Read(owner, EspacioCapturas, "x", &after)
		pending := Pendientes(st, owner)
		if err != nil || !found || calls != 0 || !bytes.Equal(before, after) || len(pending) != 1 || !pending[0].Insegura {
			t.Fatal("mismatched identity was sent or altered", found, calls, err)
		}
	}
}

// subirDeUnBorrador sube un único borrador y devuelve lo que ha llegado al servicio, en
// orden, y cuántas quedan; con `sinRed`, no llega nada.
func subirDeUnBorrador(t *testing.T, texto string, sinRed bool) (*offline.Store, []string, int) {
	t.Helper()
	st, err := offline.OpenWithSecrets(t.TempDir(), new(authtest.Secrets))
	if err != nil {
		t.Fatal(err)
	}
	if err := st.Write("ada/acme", EspacioBorradores, "nb-1", texto); err != nil {
		t.Fatal(err)
	}
	var enviadas []string
	e := Envio{
		Hoy: "2026-11-06",
		Crear: func(_ context.Context, _ string, in api.NoteInput) (api.Note, error) {
			if sinRed {
				return api.Note{}, errors.New("sin red")
			}
			enviadas = append(enviadas, in.Kind+":"+in.Title+in.Content)
			return api.Note{}, nil
		},
		Diario: func(context.Context, string, string, bool) (api.Notebook, error) { return api.Notebook{}, nil },
	}
	n := 0
	nuevo := func() string { n++; return fmt.Sprintf("id-%d", n) }
	return st, enviadas, Subir(context.Background(), st, "ada/acme", e, nuevo)
}

// TestUnBorradorDeVariasLineasSaleUnaNotaPorLinea: lo que quedó en la captura tras pegar
// varias líneas se sube como lo habría hecho Intro —una nota por línea, en orden y con
// la sintaxis de siempre—, no como una nota con `<br>`. Visto en rojo con el
// `CapturaRapida(texto, "")` de antes sobre el borrador entero: salía una sola nota.
func TestUnBorradorDeVariasLineasSaleUnaNotaPorLinea(t *testing.T) {
	_, enviadas, quedan := subirDeUnBorrador(t, "uno\n- dos\n\n---\r\ntres", false)
	want := []string{"text:<p>uno</p>", "todo:dos", "text:<hr>", "text:<p>tres</p>"}
	if quedan != 0 || !slices.Equal(enviadas, want) {
		t.Fatalf("quedan %d y salen %q; quiero 0 y %q", quedan, enviadas, want)
	}
}

// TestUnBorradorDeMasDeCincuentaDejaElResto: como en la captura, nacen cincuenta de una
// vez; lo demás se queda en el borrador, con sus saltos, y cuenta como sin subir. Visto
// en rojo sin el tope en encolarBorrador (se encolaban las cincuenta y tres).
func TestUnBorradorDeMasDeCincuentaDejaElResto(t *testing.T) {
	lineas := make([]string, 53)
	for i := range lineas {
		lineas[i] = fmt.Sprintf("l%d", i+1)
	}
	st, _, quedan := subirDeUnBorrador(t, strings.Join(lineas, "\n"), true)
	var resto string
	if bs, err := st.List("ada/acme", EspacioBorradores); err != nil || len(bs) != 1 || json.Unmarshal(bs[0].Value, &resto) != nil {
		t.Fatalf("el borrador no se ha quedado con el resto: %v", err)
	}
	if n := len(Pendientes(st, "ada/acme")); n != MaxLineasPegadas || resto != "l51\nl52\nl53" || quedan != MaxLineasPegadas+1 {
		t.Fatalf("en la cola %d, en el borrador %q y quedan %d; quiero %d, «l51…l53» y %d", n, resto, quedan, MaxLineasPegadas, MaxLineasPegadas+1)
	}
	if p := Pendientes(st, "ada/acme"); p[0].In.Content != "<p>l1</p>" || p[49].In.Content != "<p>l50</p>" {
		t.Errorf("la cola no va en el orden del borrador: %q … %q", p[0].In.Content, p[49].In.Content)
	}
}
