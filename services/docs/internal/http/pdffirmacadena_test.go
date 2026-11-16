// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package http

import (
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"git.sytue.eu/sytue/one/services/docs/internal/store"
)

// jonAcme es una tercera persona de acme, para las cadenas.
const jonAcme = "55555555-5555-4555-8555-555555555555"

// firmarSobre firma `pdf` como `sub` en dos pasos, con un certificado nuevo: lo que
// hace CertiPDF, y sobre un PDF que ya puede llevar firmas.
func firmarSobre(t *testing.T, h *Handler, sub string, pdf []byte) []byte {
	t.Helper()
	cert, clave := certificadoDeFirma(t)
	rec := httptest.NewRecorder()
	h.pdfFirmaPreparar(rec, multipartFirma(t, sub, "/api/v1/pdf/sign/prepare",
		map[string]string{"firma": `{"Motivo":"Conforme"}`}, pdf, cert.Raw))
	if rec.Code != http.StatusOK {
		t.Fatalf("preparar: %d %s", rec.Code, rec.Body)
	}
	var p firmaPreparadaJSON
	if err := json.Unmarshal(rec.Body.Bytes(), &p); err != nil {
		t.Fatal(err)
	}
	sig, _ := rsa.SignPKCS1v15(rand.Reader, clave, crypto.SHA256, p.Resumen)
	firmado := completar(t, h, sub, p, p.Preparado, sig, map[string]string{"sin_sello": "1"})
	if firmado.Code != http.StatusOK {
		t.Fatalf("firmar: %d %s", firmado.Code, firmado.Body)
	}
	return firmado.Body.Bytes()
}

// TestPedirFirmaEnCadena: Ana pide que firmen Irati y después Jon. Jon no sabe nada
// hasta que Irati entrega; entonces le llega el aviso con el PDF que firmó Irati, y
// lo que entrega él lleva las dos firmas.
func TestPedirFirmaEnCadena(t *testing.T) {
	const irati = "22222222-2222-4222-8222-222222222222"
	h, pet, ficheros, avisos := entornoFuera(t)
	h.firmas = &firmasEnMemoria{}
	ficheros["f"] = struct {
		nombre string
		datos  []byte
	}{"Contrato.pdf", pdfCon("contrato de servicios")}
	pedir := func(cuerpo string) *httptest.ResponseRecorder {
		return llamarJSON(t, h.pedirFirma, http.MethodPost, "/", anaAcme, `{"file_id":"f","path":"/PDF/Contrato.pdf",`+cuerpo+`}`, nil)
	}
	muchos := make([]string, store.MaxFirmantesEnCadena+1)
	for i := range muchos {
		muchos[i] = `{"email":"p` + strconv.Itoa(i) + `@acme.eu"}`
	}
	for _, c := range []struct {
		nombre, cuerpo, codigo string
		estado                 int
	}{
		{"con alguien de fuera", `"signers":[{"email":"irati@acme.eu"},{"email":"@leire"}]`, "pdf_signature_chain_external", http.StatusBadRequest},
		{"con alguien dos veces", `"signers":[{"email":"irati@acme.eu"},{"to":"` + irati + `"}]`, "pdf_signature_chain_repeated", http.StatusBadRequest},
		{"con quien pide", `"signers":[{"email":"irati@acme.eu"},{"email":"ana@acme.eu"}]`, "pdf_signature_request_self", http.StatusBadRequest},
		{"con signers y email", `"signers":[{"email":"irati@acme.eu"},{"email":"jon@acme.eu"}],"email":"jon@acme.eu"`, "", http.StatusBadRequest},
		{"con alguien sin decir quién", `"signers":[{"email":"irati@acme.eu"},{}]`, "", http.StatusBadRequest},
		{"demasiado larga", `"signers":[` + strings.Join(muchos, ",") + `]`, "pdf_signature_chain_too_long", http.StatusBadRequest},
		{"caducando a los 91 días", `"email":"irati@acme.eu","expires_in_days":91`, "", http.StatusBadRequest},
		{"recordando en negativo", `"email":"irati@acme.eu","remind_every_days":-1`, "", http.StatusBadRequest},
	} {
		rec := pedir(c.cuerpo)
		if rec.Code != c.estado || (c.codigo != "" && codigoDe(rec) != c.codigo) || len(pet.filas) != 0 {
			t.Errorf("%s: %d %s (%d filas)", c.nombre, rec.Code, rec.Body, len(pet.filas))
		}
	}

	rec := pedir(`"signers":[{"email":"irati@acme.eu"},{"email":"jon@acme.eu"}],"message":"Por orden","expires_in_days":7,"remind_every_days":2`)
	if rec.Code != http.StatusCreated {
		t.Fatalf("pedir: %d %s", rec.Code, rec.Body)
	}
	var cadena struct {
		Chain    string
		Requests []store.PeticionFirma
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &cadena)
	if len(cadena.Requests) != 2 || cadena.Chain == "" ||
		cadena.Requests[0].Destinatario != irati || cadena.Requests[0].Estado != store.FirmaPendiente ||
		cadena.Requests[1].Destinatario != jonAcme || cadena.Requests[1].Estado != store.FirmaEnEspera ||
		cadena.Requests[1].Orden != 1 || cadena.Requests[0].RecordarDias != 2 || cadena.Requests[0].Caduca == nil ||
		cadena.Requests[0].Caduca.Before(time.Now().Add(6*24*time.Hour)) {
		t.Fatalf("la cadena: %s", rec.Body)
	}
	if n, ok := hubo(avisos); !ok || n.Subject != irati || n.Type != "docs.signature.requested" {
		t.Fatalf("el primer aviso es para Irati: %+v", n)
	}
	if n, ok := hubo(avisos); ok {
		t.Errorf("a Jon no le toca aún: %+v", n)
	}
	recibidas := func(sub string) []store.PeticionFirma {
		rec := llamarJSON(t, h.listarPeticionesFirma, http.MethodGet, "/", sub, "", nil)
		var l struct{ Received []store.PeticionFirma }
		_ = json.Unmarshal(rec.Body.Bytes(), &l)
		return l.Received
	}
	if r := recibidas(jonAcme); len(r) != 0 {
		t.Errorf("Jon ve una que espera turno: %+v", r)
	}
	firmar := func(sub, id, fichero, ruta string) *httptest.ResponseRecorder {
		return llamarJSON(t, h.firmarPeticion, http.MethodPost, "/", sub,
			`{"signed_file_id":"`+fichero+`","signed_path":"`+ruta+`"}`, map[string]string{"id": id})
	}
	if rec := firmar(jonAcme, cadena.Requests[1].ID, "f", "/PDF/Contrato.pdf"); rec.Code != http.StatusNotFound {
		t.Errorf("Jon firma antes de su turno: %d %s", rec.Code, rec.Body)
	}

	// Irati firma: el turno pasa a Jon, sobre el PDF que ella ha firmado.
	ficheros["de-irati"] = struct {
		nombre string
		datos  []byte
	}{"Contrato (firmado).pdf", firmarSobre(t, h, irati, ficheros["f"].datos)}
	if rec := firmar(irati, cadena.Requests[0].ID, "de-irati", "/PDF/Contrato (firmado).pdf"); rec.Code != http.StatusOK {
		t.Fatalf("firma Irati: %d %s", rec.Code, rec.Body)
	}
	vistos := map[string]string{}
	for range 2 {
		if n, ok := hubo(avisos); ok {
			vistos[n.Subject] = n.Type + " " + n.URL
		}
	}
	if !strings.HasPrefix(vistos[anaAcme], "docs.signature.completed") ||
		!strings.HasPrefix(vistos[jonAcme], "docs.signature.requested") || !strings.Contains(vistos[jonAcme], "firmado") {
		t.Fatalf("tras firmar Irati: %v", vistos)
	}
	r := recibidas(jonAcme)
	if len(r) != 1 || r[0].Estado != store.FirmaPendiente || r[0].Fichero != "de-irati" ||
		r[0].Ruta != "/PDF/Contrato (firmado).pdf" || r[0].Mensaje != "Por orden" {
		t.Fatalf("lo que ve Jon: %+v", r)
	}
	// Jon tiene que firmar lo de Irati: entregar una firma sobre el original no vale.
	ficheros["de-jon-mal"] = struct {
		nombre string
		datos  []byte
	}{"Contrato (Jon).pdf", firmarSobre(t, h, jonAcme, ficheros["f"].datos)}
	if rec := firmar(jonAcme, r[0].ID, "de-jon-mal", "/PDF/Contrato (Jon).pdf"); rec.Code != http.StatusUnprocessableEntity {
		t.Errorf("Jon firma sobre el original: %d %s", rec.Code, rec.Body)
	}
	ficheros["de-jon"] = struct {
		nombre string
		datos  []byte
	}{"Contrato (firmado 2).pdf", firmarSobre(t, h, jonAcme, ficheros["de-irati"].datos)}
	if rec := firmar(jonAcme, r[0].ID, "de-jon", "/PDF/Contrato (firmado 2).pdf"); rec.Code != http.StatusOK {
		t.Fatalf("firma Jon: %d %s", rec.Code, rec.Body)
	}
	if n, ok := hubo(avisos); !ok || n.Subject != anaAcme || n.Type != "docs.signature.completed" {
		t.Errorf("el último aviso es para Ana: %+v", n)
	}
	for _, f := range pet.filas {
		if f.Estado != store.FirmaFirmada {
			t.Errorf("al final: %+v", f)
		}
	}
}

// TestRechazarCierraLaCadena: si una persona rechaza, las que esperaban no reciben
// nada nunca; cancelar desde la que tiene el turno, igual.
func TestRechazarCierraLaCadena(t *testing.T) {
	const irati = "22222222-2222-4222-8222-222222222222"
	h, pet, _, _ := entornoFuera(t)
	pedir := func(fichero, ruta string) []store.PeticionFirma {
		rec := llamarJSON(t, h.pedirFirma, http.MethodPost, "/", anaAcme,
			`{"file_id":"`+fichero+`","path":"`+ruta+`","signers":[{"email":"irati@acme.eu"},{"email":"jon@acme.eu"}]}`, nil)
		var c struct{ Requests []store.PeticionFirma }
		if err := json.Unmarshal(rec.Body.Bytes(), &c); err != nil || len(c.Requests) != 2 {
			t.Fatalf("pedir: %d %s", rec.Code, rec.Body)
		}
		return c.Requests
	}
	c := pedir("f", "/Contrato.pdf")
	if rec := llamarJSON(t, h.rechazarPeticionFirma, http.MethodPost, "/", irati, "", map[string]string{"id": c[0].ID}); rec.Code != http.StatusOK {
		t.Fatalf("rechazar: %d %s", rec.Code, rec.Body)
	}
	if pet.filas[1].Estado != store.FirmaCancelada {
		t.Errorf("la de Jon tras rechazar Irati: %+v", pet.filas[1])
	}
	c = pedir("g", "/Otro.pdf")
	if rec := llamarJSON(t, h.cancelarPeticionFirma, http.MethodDelete, "/", anaAcme, "", map[string]string{"id": c[0].ID}); rec.Code != http.StatusNoContent {
		t.Fatalf("cancelar: %d %s", rec.Code, rec.Body)
	}
	if pet.filas[3].Estado != store.FirmaCancelada {
		t.Errorf("la de Jon tras cancelar Ana: %+v", pet.filas[3])
	}
}

// TestBarridoDePeticionesDeFirma: lo que pasó su fecha caduca —y ya no se puede firmar
// ni aunque el barrido no haya pasado—, a quien pidió le llega un aviso por cadena, y
// a quien tiene el turno se le recuerda cuando toca, una sola vez por vuelta.
func TestBarridoDePeticionesDeFirma(t *testing.T) {
	const irati = "22222222-2222-4222-8222-222222222222"
	h, pet, _, avisos := entornoFuera(t)
	rec := llamarJSON(t, h.pedirFirma, http.MethodPost, "/", anaAcme,
		`{"file_id":"f","path":"/Contrato.pdf","signers":[{"email":"irati@acme.eu"},{"email":"jon@acme.eu"}],"expires_in_days":3}`, nil)
	if rec.Code != http.StatusCreated {
		t.Fatalf("pedir: %d %s", rec.Code, rec.Body)
	}
	rec = llamarJSON(t, h.pedirFirma, http.MethodPost, "/", anaAcme,
		`{"file_id":"g","path":"/Otro.pdf","email":"irati@acme.eu","remind_every_days":2}`, nil)
	if rec.Code != http.StatusCreated {
		t.Fatalf("pedir: %d %s", rec.Code, rec.Body)
	}
	for range 2 {
		hubo(avisos) // los de «te piden firmar»
	}
	if b, err := h.BarrerPeticionesFirma(t.Context()); err != nil || b != (Barrido{}) {
		t.Fatalf("recién pedidas no hay nada que barrer: %+v %v", b, err)
	}

	// Pasan los días: la cadena caduca y a la suelta le toca recordatorio.
	ayer := time.Now().Add(-24 * time.Hour)
	pet.filas[0].Caduca, pet.filas[1].Caduca = &ayer, &ayer
	pet.filas[2].Avisada = time.Now().Add(-49 * time.Hour)
	if rec := llamarJSON(t, h.firmarPeticion, http.MethodPost, "/", irati,
		`{"signed_file_id":"f","signed_path":"/Contrato.pdf"}`, map[string]string{"id": pet.filas[0].ID}); rec.Code != http.StatusNotFound {
		t.Errorf("firmar una caducada: %d %s", rec.Code, rec.Body)
	}
	b, err := h.BarrerPeticionesFirma(t.Context())
	if err != nil || b != (Barrido{Caducadas: 2, Recordadas: 1}) {
		t.Fatalf("barrido: %+v %v", b, err)
	}
	vistos := map[string]string{}
	for range 3 {
		if n, ok := hubo(avisos); ok {
			vistos[n.Subject+" "+n.Type] = n.Params["name"]
		}
	}
	if len(vistos) != 2 || vistos[anaAcme+" docs.signature.expired"] != "Contrato.pdf" ||
		vistos[irati+" docs.signature.reminder"] != "Otro.pdf" {
		t.Errorf("avisos del barrido: %v", vistos)
	}
	if pet.filas[0].Estado != store.FirmaCaducada || pet.filas[1].Estado != store.FirmaCaducada ||
		pet.filas[2].Estado != store.FirmaPendiente {
		t.Errorf("tras el barrido: %+v", pet.filas)
	}
	if b, _ := h.BarrerPeticionesFirma(t.Context()); b != (Barrido{}) {
		t.Errorf("la segunda vuelta repite: %+v", b)
	}
}
