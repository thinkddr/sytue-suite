// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos
package notes

import (
	"bytes"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"fyne.io/fyne/v2"

	"git.sytue.eu/sytue/one/apps/nucleo/api"
)

func TestDatosDeNotaCifradaSeRestauranYSeVuelvenACifrar(t *testing.T) {
	key := bytes.Repeat([]byte{3}, 32)
	in := api.NoteInput{Title: "private title", Content: "body", Data: json.RawMessage(`{"attachments":[{"id":"a","name":"secret.txt","dataUrl":"data:text/plain;base64,c2VjcmV0"}],"url":"https://private.invalid","color":"#fff","icon":"key"}`)}
	if err := api.CifrarContenidoNota(key, &in); err != nil {
		t.Fatal(err)
	}
	n := api.Note{ID: "n1", NotebookID: "c1", Kind: "text", ContentCipher: in.ContentCipher, Data: in.Data}
	p, _ := pantallaConServicio(t, nil, n)
	p.activo.Cifrado = true
	p.clave = key
	v := p.deNota(n)
	if v.Cifrada || v.Titulo != "private title" || len(v.Adjuntos) != 1 || v.Adjuntos[0].Name != "secret.txt" {
		t.Fatalf("vista no restaura adjunto: %+v", v)
	}
	title := "changed"
	edited, err := p.entradaEditada(v, Parche{Titulo: &title})
	if err != nil {
		t.Fatal(err)
	}
	wire, _ := json.Marshal(edited)
	for _, secret := range []string{"secret.txt", "data:text/plain", "private.invalid", "#fff", "changed"} {
		if bytes.Contains(wire, []byte(secret)) {
			t.Fatalf("filtrado: %s", secret)
		}
	}
	claro, err := api.DesempaquetarNota(key, edited.ContentCipher)
	if err != nil || !bytes.Contains(claro.Data, []byte("secret.txt")) || claro.Title != "changed" {
		t.Fatalf("editar borró Data: %+v %v", claro, err)
	}
	p.clave = nil
	locked := p.deNota(api.Note{ID: "legacy", Title: "historic", Content: "text", Data: json.RawMessage(`{"attachments":[{"dataUrl":"data:legacy"}]}`)})
	if !locked.Cifrada || locked.Titulo != "" || locked.Contenido != "" || len(locked.Adjuntos) > 0 {
		t.Fatal("un toggle legacy se muestra antes de desbloquear")
	}
	if locked.Crudo.Title != "historic" || len(locked.Crudo.Data) == 0 {
		t.Fatal("ocultar vista destruyó original")
	}
}

func TestMigracionNotasEsCASYNoRepiteResultadoIncierto(t *testing.T) {
	for _, fallar := range []bool{false, true} {
		t.Run(map[bool]string{false: "confirmed", true: "uncertain"}[fallar], func(t *testing.T) {
			key := bytes.Repeat([]byte{4}, 32)
			crypto := json.RawMessage(`{"v":1,"salt":"public","iterations":600000,"check":"encv1:public","extra":"keep"}`)
			old, _ := api.EmpaquetarNota(key, api.NotaEnClaro{Title: "secret", Content: "body"})
			n := api.Note{ID: "n1", NotebookID: "c1", Kind: "text", ContentCipher: old, Data: json.RawMessage(`{"attachments":[{"dataUrl":"data:legacy"}],"client_id":"opaque"}`), UpdatedAt: time.Date(2026, 10, 20, 12, 0, 0, 123456000, time.UTC)}
			var calls atomic.Int32
			finished := make(chan struct{})
			var once sync.Once
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method == http.MethodGet {
					if r.Header.Get("Cache-Control") != "no-store" {
						t.Error("CAS usa caché histórica")
					}
					_ = json.NewEncoder(w).Encode(map[string]any{"notes": []api.Note{n}})
					return
				}
				calls.Add(1)
				defer once.Do(func() { close(finished) })
				var in api.NoteCipherMigration
				if json.NewDecoder(r.Body).Decode(&in) != nil || in.CipherVersion != 2 || !in.ExpectedUpdatedAt.Equal(n.UpdatedAt) {
					t.Error("CAS incompleto")
				}
				var params map[string]any
				_ = json.Unmarshal(in.ExpectedCrypto, &params)
				if params["extra"] != "keep" {
					t.Error("CAS crypto parcial")
				}
				claro, err := api.DesempaquetarNota(key, in.ContentCipher)
				if err != nil || !bytes.Contains(claro.Data, []byte("data:legacy")) || string(in.Data) != `{"client_id":"opaque"}` {
					t.Error("migración perdió o publicó Data")
				}
				if fallar {
					w.WriteHeader(http.StatusBadGateway)
					return
				}
				result := n
				result.Data = in.Data
				result.ContentCipher = in.ContentCipher
				result.UpdatedAt = result.UpdatedAt.Add(time.Second)
				_ = json.NewEncoder(w).Encode(result)
			}))
			defer srv.Close()
			p, _ := pantallaConServicio(t, nil, n)
			p.svc = api.NewNotes(srv.URL, nil, nil)
			p.activo.Cifrado = true
			p.activo.Crudo.Crypto = crypto
			fyne.DoAndWait(func() { p.abrirCon("c1", key) })
			esperar(t, func() bool {
				select {
				case <-finished:
					return true
				default:
					return false
				}
			})
			esperar(t, func() bool { done := false; fyne.DoAndWait(func() { done = p.migracionCancel == nil }); return done })
			fyne.DoAndWait(func() {
				if calls.Load() != 1 {
					t.Error("POST incierto repetido")
				}
				if fallar {
					if !p.migrando || !bytes.Contains(p.notas[0].Crudo.Data, []byte("data:legacy")) {
						t.Error("fallo perdió original o permite sobrescribirlo")
					}
				} else {
					if p.migrando || bytes.Contains(p.notas[0].Crudo.Data, []byte("data:legacy")) {
						t.Error("confirmación conserva copia en claro")
					}
				}
				p.cancelarMigracion()
			})
		})
	}
}

func TestCuentaCambiadaDuranteLecturaMigracionNoEscribeNiRestauraClave(t *testing.T) {
	key := bytes.Repeat([]byte{5}, 32)
	var calls atomic.Int32
	entered, release := make(chan struct{}), make(chan struct{})
	var once sync.Once
	n := api.Note{ID: "n1", NotebookID: "c1", Kind: "text", Title: "legacy", Data: json.RawMessage(`{"secret":"old"}`)}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet {
			close(entered)
			<-release
			_ = json.NewEncoder(w).Encode(map[string]any{"notes": []api.Note{n}})
			return
		}
		calls.Add(1)
	}))
	defer func() { once.Do(func() { close(release) }); srv.Close() }()
	p, _ := pantallaConServicio(t, nil, n)
	p.svc = api.NewNotes(srv.URL, nil, nil)
	p.activo.Cifrado = true
	p.activo.Crudo.Crypto = json.RawMessage(`{"v":1}`)
	generation := "one"
	p.GeneracionSesion = func() string { return generation }
	fyne.DoAndWait(func() { p.abrirCon("c1", key) })
	esperar(t, func() bool {
		select {
		case <-entered:
			return true
		default:
			return false
		}
	})
	fyne.DoAndWait(func() {
		generation = "two"
		p.ReiniciarSesion()
		p.activo = DeCuaderno(api.Notebook{ID: "c1", Encrypted: true})
	})
	once.Do(func() { close(release) })
	time.Sleep(40 * time.Millisecond)
	fyne.DoAndWait(func() {
		if calls.Load() != 0 || p.clave != nil || len(p.notas) != 0 {
			t.Error("respuesta de otra sesión restauró contenido o escribió")
		}
	})
}

func TestConfirmacionMigracionTardiaNoReemplazaNotaDeOtraSesion(t *testing.T) {
	key := bytes.Repeat([]byte{6}, 32)
	old := api.Note{ID: "n1", NotebookID: "c1", Kind: "text", Title: "legacy", Data: json.RawMessage(`{"secret":"old"}`), UpdatedAt: time.Date(2026, 10, 20, 12, 0, 0, 0, time.UTC)}
	entered, release, done := make(chan struct{}), make(chan struct{}), make(chan struct{})
	var once sync.Once
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet {
			_ = json.NewEncoder(w).Encode(map[string]any{"notes": []api.Note{old}})
			return
		}
		var in api.NoteCipherMigration
		_ = json.NewDecoder(r.Body).Decode(&in)
		close(entered)
		<-release
		result := old
		result.Title = ""
		result.ContentCipher = in.ContentCipher
		result.Data = in.Data
		result.UpdatedAt = old.UpdatedAt.Add(time.Second)
		_ = json.NewEncoder(w).Encode(result)
		close(done)
	}))
	defer func() { once.Do(func() { close(release) }); srv.Close() }()
	p, _ := pantallaConServicio(t, nil, old)
	p.svc = api.NewNotes(srv.URL, nil, nil)
	p.activo.Cifrado = true
	p.activo.Crudo.Crypto = json.RawMessage(`{"v":1}`)
	generation := "one"
	p.GeneracionSesion = func() string { return generation }
	fyne.DoAndWait(func() { p.abrirCon("c1", key) })
	esperar(t, func() bool {
		select {
		case <-entered:
			return true
		default:
			return false
		}
	})
	fresh := old
	fresh.Title = "new session"
	fresh.Data = json.RawMessage(`{}`)
	fyne.DoAndWait(func() {
		generation = "two"
		p.ReiniciarSesion()
		p.activo = DeCuaderno(api.Notebook{ID: "c1", Encrypted: true})
		p.notas = []Nota{DeNota(fresh)}
	})
	once.Do(func() { close(release) })
	esperar(t, func() bool {
		select {
		case <-done:
			return true
		default:
			return false
		}
	})
	time.Sleep(30 * time.Millisecond)
	fyne.DoAndWait(func() {
		if p.clave != nil || len(p.notas) != 1 || p.notas[0].Crudo.Title != "new session" || !p.notas[0].Crudo.UpdatedAt.Equal(old.UpdatedAt) {
			t.Fatal("confirmación de otra sesión reemplaza la nota actual")
		}
	})
}

func TestPantallaPoseeSuClaveYLimpiaSoloSuCopia(t *testing.T) {
	for _, accion := range []string{"lock", "logout", "notebook", "replace"} {
		t.Run(accion, func(t *testing.T) {
			original := bytes.Repeat([]byte{9}, 32)
			p, _ := pantallaConServicio(t, nil)
			p.svc = nil
			p.activo.Cifrado = true
			fyne.DoAndWait(func() {
				p.abrirCon("c1", original)
				propia := p.clave
				if !bytes.Equal(propia, original) || &propia[0] == &original[0] {
					t.Fatal("la pantalla no posee una copia independiente")
				}
				switch accion {
				case "lock":
					p.bloquear()
				case "logout":
					p.ReiniciarSesion()
				case "notebook":
					p.abrirCuaderno(DeCuaderno(api.Notebook{ID: "c2", Encrypted: true}))
				case "replace":
					p.abrirCon("c1", bytes.Repeat([]byte{8}, 32))
				}
				if !bytes.Equal(propia, make([]byte, 32)) {
					t.Fatal("la pantalla conserva el slice activo tras descartarlo")
				}
				if !bytes.Equal(original, bytes.Repeat([]byte{9}, 32)) {
					t.Fatal("se ha limpiado la clave ajena del recordador")
				}
				if accion != "replace" && p.clave != nil {
					t.Fatal("la pantalla retiene una clave")
				}
			})
		})
	}
}

func TestEdicionCifradaPendienteNoRestauraClaroTrasBloquear(t *testing.T) {
	key := bytes.Repeat([]byte{7}, 32)
	input := api.NoteInput{Title: "private", Content: "body", Data: json.RawMessage(`{"secret":"data"}`)}
	if err := api.CifrarContenidoNota(key, &input); err != nil {
		t.Fatal(err)
	}
	old := api.Note{ID: "n1", NotebookID: "c1", Kind: "text", ContentCipher: input.ContentCipher, Data: input.Data}
	entered, release, done := make(chan struct{}), make(chan struct{}), make(chan struct{})
	var once sync.Once
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var in api.NoteInput
		_ = json.NewDecoder(r.Body).Decode(&in)
		close(entered)
		<-release
		row := old
		row.ContentCipher = in.ContentCipher
		_ = json.NewEncoder(w).Encode(row)
	}))
	defer func() { once.Do(func() { close(release) }); srv.Close() }()
	p, _ := pantallaConServicio(t, nil, old)
	p.svc = api.NewNotes(srv.URL, nil, nil)
	fyne.DoAndWait(func() {
		p.activo.Cifrado = true
		p.activo.Crudo.Crypto = json.RawMessage(`{"v":1}`)
		p.abrirCon("c1", key)
	})
	title := "changed"
	go func() { defer close(done); _ = p.editar("n1", Parche{Titulo: &title}, true) }()
	esperar(t, func() bool {
		select {
		case <-entered:
			return true
		default:
			return false
		}
	})
	fyne.DoAndWait(p.bloquear)
	once.Do(func() { close(release) })
	esperar(t, func() bool {
		select {
		case <-done:
			return true
		default:
			return false
		}
	})
	fyne.DoAndWait(func() {
		if p.clave != nil || len(p.notas) != 1 || !p.notas[0].Cifrada || p.notas[0].Titulo != "" || len(p.notas[0].Datos) > 0 {
			t.Fatal("una escritura pendiente restauró el claro tras bloquear")
		}
		if p.notas[0].Crudo.ContentCipher != old.ContentCipher {
			t.Fatal("la confirmación antigua cambió el estado del nuevo epoch")
		}
	})
}

func TestCreacionRechazadaPorCifradoDevuelveErrorYConservaBorrador(t *testing.T) {
	p, falso := pantallaConServicio(t, nil)
	p.activo.Cifrado = true
	p.clave = bytes.Repeat([]byte{8}, 32)
	borrador := `{"broken"`
	p.campoCaptura = p.nuevoCampoCaptura()
	p.campoCaptura.PonerTexto("private draft")
	despues := false
	err := p.crearCon(api.NoteInput{Title: "private draft", Data: json.RawMessage(borrador)}, func() { despues = true; p.campoCaptura.PonerTexto("") })
	if !errors.Is(err, api.ErrDatosCifradosNotas) || despues || len(falso.rutas()) != 0 || p.creando {
		t.Fatalf("creación fallida interpretada como éxito: %v, despues=%v, rutas=%v", err, despues, falso.rutas())
	}
	if p.campoCaptura.Texto() != "private draft" {
		t.Fatal("se borró el borrador rechazado")
	}
	if p.bloqueado || !p.puedeCapturar() {
		t.Fatal("el fallo impide corregir el borrador")
	}
}

func TestServidorAntiguoRechazaCreacionCifradaYConservaBorrador(t *testing.T) {
	var calls, writes atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		if r.Method == http.MethodPost && r.URL.Path == "/api/v1/notebooks/c1/notes" {
			writes.Add(1)
			_, _ = w.Write([]byte(`{"id":"unsafe-write","notebook_id":"c1"}`))
			return
		}
		if r.Method != http.MethodPost || r.URL.Path != "/api/v1/notebooks/c1/encrypted-notes" {
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
		http.NotFound(w, r)
	}))
	defer srv.Close()
	p, _ := pantallaConServicio(t, nil)
	p.svc = api.NewNotes(srv.URL, nil, nil)
	p.activo.Cifrado = true
	p.activo.Crudo.Crypto = json.RawMessage(`{"v":1}`)
	p.clave = bytes.Repeat([]byte{9}, 32)
	p.campoCaptura = p.nuevoCampoCaptura()
	p.campoCaptura.PonerTexto("private draft")
	despues := false
	err := p.crearCon(api.NoteInput{Title: "private draft"}, func() { despues = true; p.campoCaptura.PonerTexto("") })
	var response *api.Error
	if !errors.As(err, &response) || response.Status != http.StatusNotFound || despues || p.creando || calls.Load() != 1 || writes.Load() != 0 {
		t.Fatalf("creation against old server: err=%v after=%v creating=%v calls=%d writes=%d", err, despues, p.creando, calls.Load(), writes.Load())
	}
	if p.campoCaptura.Texto() != "private draft" || len(p.notas) != 0 {
		t.Fatal("failed creation erased its draft or appeared as saved")
	}
}

func TestEdicionAuxiliarConservaNumerosYNoExpandeHTMLAjeno(t *testing.T) {
	data := json.RawMessage(`{"future":9007199254740993123456789,"drawing":{"ratio":1.234567890123456789},"html":"` + strings.Repeat("<>&", 1024) + `"}`)
	for _, cambio := range []func(json.RawMessage) (json.RawMessage, error){
		func(d json.RawMessage) (json.RawMessage, error) { return conIcono(d, "key") },
		func(d json.RawMessage) (json.RawMessage, error) { return conAdjuntos(d, []Adjunto{}) },
	} {
		changed, err := cambio(data)
		if err != nil || len(changed) > len(data)+128 || !bytes.Contains(changed, []byte("9007199254740993123456789")) || !bytes.Contains(changed, []byte("1.234567890123456789")) || !bytes.Contains(changed, []byte("<>&")) {
			t.Fatalf("edición auxiliar altera datos ajenos o multiplica su tamaño: %v, %d→%d", err, len(data), len(changed))
		}
	}
}
