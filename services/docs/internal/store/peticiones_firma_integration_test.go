// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

//go:build integration

package store_test

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strconv"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"git.sytue.eu/sytue/one/services/docs/internal/store"
)

// TestPeticionesDeFirmaEnLaBase: una pendiente por fichero y persona, cada paso solo
// lo da quien toca, nada sale de su organización, y el CHECK de la 0011 no deja una
// firmada sin fichero firmado.
func TestPeticionesDeFirmaEnLaBase(t *testing.T) {
	st := newStore(t)
	acme, globex := tenantCtx("acme"), tenantCtx("globex")
	nueva := store.PeticionFirma{Solicitante: "ana", SolicitanteNombre: "Ana", Destinatario: "irati", DestinatarioCorreo: "irati@acme.eu", Fichero: "f1", Ruta: "/PDF/a.pdf"}
	p, err := st.CrearPeticionFirma(acme, nueva)
	if err != nil || p.Estado != store.FirmaPendiente || p.ID == "" || p.DestinatarioCorreo != "irati@acme.eu" {
		t.Fatalf("crear: %+v %v", p, err)
	}
	if _, err := st.CrearPeticionFirma(acme, nueva); !errors.Is(err, store.ErrPeticionFirmaRepetida) {
		t.Errorf("repetida: %v", err)
	}
	if r, _, err := st.PeticionesFirma(globex, "irati"); err != nil || len(r) != 0 {
		t.Errorf("otra organización ve %d (%v)", len(r), err)
	}
	if r, s, err := st.PeticionesFirma(acme, "irati"); err != nil || len(r) != 1 || len(s) != 0 {
		t.Errorf("irati: %d recibidas, %d pedidas (%v)", len(r), len(s), err)
	}
	if _, err := st.PeticionFirmaPendiente(acme, p.ID, "ana"); !errors.Is(err, store.ErrPeticionFirmaNoEsta) {
		t.Errorf("pendiente para quien pidió: %v", err)
	}
	if _, err := st.ResolverPeticionFirma(acme, p.ID, "ana", store.FirmaFirmada, "f2", "/PDF/b.pdf", nil, ""); !errors.Is(err, store.ErrPeticionFirmaNoEsta) {
		t.Errorf("firma quien pidió: %v", err)
	}
	if _, err := st.ResolverPeticionFirma(globex, p.ID, "irati", store.FirmaFirmada, "f2", "/PDF/b.pdf", nil, ""); !errors.Is(err, store.ErrPeticionFirmaNoEsta) {
		t.Errorf("firma desde otra organización: %v", err)
	}
	if _, err := st.ResolverPeticionFirma(acme, p.ID, "irati", store.FirmaFirmada, "", "", nil, ""); err == nil {
		t.Error("firmada sin fichero firmado")
	}
	f, err := st.ResolverPeticionFirma(acme, p.ID, "irati", store.FirmaFirmada, "f2", "/PDF/b.pdf", nil, "")
	if err != nil || f.Estado != store.FirmaFirmada || f.Firmado != "f2" || f.Resuelta == nil {
		t.Fatalf("firmar: %+v %v", f, err)
	}
	if _, err := st.ResolverPeticionFirma(acme, p.ID, "ana", store.FirmaCancelada, "", "", nil, ""); !errors.Is(err, store.ErrPeticionFirmaNoEsta) {
		t.Errorf("cancelar una firmada: %v", err)
	}
	if _, err := st.CrearPeticionFirma(acme, nueva); err != nil {
		t.Errorf("tras firmarla se puede volver a pedir: %v", err)
	}
	// El tope por hora cuenta todas las de quien pide, pendientes o no.
	for i := 3; i <= store.MaxPeticionesFirmaPorHora; i++ {
		otra := nueva
		otra.Fichero = "f" + strconv.Itoa(i)
		if _, err := st.CrearPeticionFirma(acme, otra); err != nil {
			t.Fatalf("la %d: %v", i, err)
		}
	}
	otra := nueva
	otra.Fichero = "de más"
	if _, err := st.CrearPeticionFirma(acme, otra); !errors.Is(err, store.ErrDemasiadasPeticionesFirma) {
		t.Errorf("una de más: %v", err)
	}
}

// TestPeticionesDeFirmaAFuera (ADR 0057): la de fuera vive en la organización de quien
// pide; su destinatario la encuentra desde la suya —sin la ruta ni el id del original—
// y solo él, y la organización anfitriona solo se le dice a él.
func TestPeticionesDeFirmaAFuera(t *testing.T) {
	st := newStore(t)
	acme, globex, initech := tenantCtx("acme"), tenantCtx("globex"), tenantCtx("initech")
	fuera := store.PeticionFirma{
		Solicitante: "ana", Destinatario: "leire", DestinatarioCorreo: "@leire", Fichero: "f1", Ruta: "/Legal/Contrato.pdf",
		Externo: true, Enlace: []byte("cifrado"), EnlaceShare: "s1", Huella: "h1",
	}
	p, err := st.CrearPeticionFirma(acme, fuera)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := st.CrearPeticionFirma(acme, store.PeticionFirma{
		Solicitante: "ana", Destinatario: "irati", Fichero: "f1", Ruta: "/a.pdf",
		Enlace: []byte("x"),
	}); err == nil {
		t.Error("un enlace en una de dentro")
	}
	sinHuella := fuera
	sinHuella.Fichero, sinHuella.Huella = "f0", ""
	if _, err := st.CrearPeticionFirma(acme, sinHuella); err == nil {
		t.Error("una de fuera sin huella")
	}
	for _, c := range []struct {
		id, sub, quiere string
	}{{p.ID, "leire", "acme"}, {p.ID, "ana", ""}, {p.ID, "irati", ""}, {"00000000-0000-4000-8000-000000000009", "leire", ""}} {
		if got, err := st.AnfitrionPeticionFirma(globex, c.id, c.sub); err != nil || got != c.quiere {
			t.Errorf("anfitriona de %s para %s = %q (%v)", c.id, c.sub, got, err)
		}
	}
	r, _, err := st.PeticionesFirma(globex, "leire")
	if err != nil || len(r) != 1 || r[0].Ruta != "Contrato.pdf" || r[0].Fichero != "" || r[0].Enlace != nil || r[0].EnlaceShare != "" {
		t.Fatalf("lo que llega a globex: %+v %v", r, err)
	}
	if v := r[0].ParaQuienFirma(); !v.Externo || v.Nombre != "Contrato.pdf" || v.Ruta != "" || v.Fichero != "" {
		t.Errorf("lo que ve leire: %+v", v)
	}
	if r, _, _ := st.PeticionesFirma(initech, "irati"); len(r) != 0 {
		t.Errorf("otra persona ve %d", len(r))
	}

	// Una vuelta de «abrir» a la vez.
	if _, err := st.OcuparPeticionFirma(acme, p.ID, "ana", true); !errors.Is(err, store.ErrPeticionFirmaNoEsta) {
		t.Errorf("la abre quien no la recibió: %v", err)
	}
	if _, err := st.OcuparPeticionFirma(acme, p.ID, "leire", true); err != nil {
		t.Fatal(err)
	}
	if _, err := st.OcuparPeticionFirma(acme, p.ID, "leire", true); !errors.Is(err, store.ErrPeticionFirmaOcupada) {
		t.Errorf("dos vueltas a la vez: %v", err)
	}
	c, err := st.GuardarCopiaFirma(acme, p.ID, "leire", "c1", "/Sytue Docs/Contrato.pdf")
	if err != nil || c.Copia != "c1" || c.Enlace == nil {
		t.Fatalf("copia: %+v %v", c, err)
	}
	if _, err := st.OcuparPeticionFirma(acme, p.ID, "leire", true); err != nil {
		t.Errorf("la copia no soltó la reserva: %v", err)
	}
	if err := st.SoltarPeticionFirma(acme, p.ID, "leire"); err != nil {
		t.Fatal(err)
	}
	if _, err := st.GuardarCopiaFirma(acme, p.ID, "ana", "c3", "/x"); !errors.Is(err, store.ErrPeticionFirmaNoEsta) {
		t.Errorf("copia por otra persona: %v", err)
	}
	if r, _, _ := st.PeticionesFirma(globex, "leire"); len(r) != 1 || r[0].ParaQuienFirma().Ruta != "/Sytue Docs/Contrato.pdf" || r[0].ParaQuienFirma().Fichero != "c1" {
		t.Errorf("tras la copia: %+v", r)
	}
	if _, err := st.OcuparPeticionFirma(acme, p.ID, "ana", false); !errors.Is(err, store.ErrPeticionFirmaNoEsta) {
		t.Errorf("se trae una pendiente: %v", err)
	}
	if _, err := st.ResolverPeticionFirma(acme, p.ID, "leire", store.FirmaFirmada, "g1", "", []byte("cifrado2"), "hf"); err != nil {
		t.Fatal(err)
	}
	if _, err := st.OcuparPeticionFirma(acme, p.ID, "leire", false); !errors.Is(err, store.ErrPeticionFirmaNoEsta) {
		t.Errorf("la trae quien no pidió: %v", err)
	}
	if f, err := st.OcuparPeticionFirma(acme, p.ID, "ana", false); err != nil || string(f.Enlace) != "cifrado2" || f.EnlaceShare != "s1" || f.FirmadoHuella != "hf" {
		t.Fatalf("firmada: %+v %v", f, err)
	}
	if f, err := st.GuardarFirmadoTraido(acme, p.ID, "ana", "a2", "/Sytue Docs/Contrato (firmado).pdf"); err != nil || f.Enlace != nil || f.FirmadoRuta == "" {
		t.Fatalf("traída: %+v %v", f, err)
	}
	if _, err := st.GuardarFirmadoTraido(acme, p.ID, "ana", "a3", "/x"); !errors.Is(err, store.ErrPeticionFirmaNoEsta) {
		t.Errorf("dos veces: %v", err)
	}
	if err := st.OlvidarEnlaceFirma(acme, p.ID, "ana"); err != nil {
		t.Fatal(err)
	}
	if _, s, _ := st.PeticionesFirma(acme, "ana"); len(s) != 1 || s[0].EnlaceShare != "" {
		t.Errorf("el enlace revocado sigue apuntado: %+v", s)
	}
}

// TestNadieLlenaLaListaDeAlguienDeFuera: topes por quien pide y por destinatario, de
// todas las organizaciones, y una cerrada sin firmar no guarda token.
func TestNadieLlenaLaListaDeAlguienDeFuera(t *testing.T) {
	st := newStore(t)
	pide := func(org, sol string, i int) error {
		_, err := st.CrearPeticionFirma(tenantCtx(org), store.PeticionFirma{
			Solicitante: sol, Destinatario: "leire", Fichero: "f" + strconv.Itoa(i), Ruta: "/a.pdf",
			Externo: true, Huella: "h",
		})
		return err
	}
	for i := range store.MaxPendientesPorPersona {
		if err := pide("acme", "ana", i); err != nil {
			t.Fatal(err)
		}
	}
	if err := pide("acme", "ana", 99); !errors.Is(err, store.ErrDemasiadasParaEsaPersona) {
		t.Errorf("de la misma persona: %v", err)
	}
	if err := st.PuedePedirFirma(tenantCtx("acme"), store.PeticionFirma{Solicitante: "ana", Destinatario: "leire", Fichero: "g", Externo: true}); !errors.Is(err, store.ErrDemasiadasParaEsaPersona) {
		t.Errorf("sin crear: %v", err)
	}
	n := store.MaxPendientesPorPersona
	for org := 0; n < store.MaxPendientesDeFuera; org++ {
		for i := 0; i < store.MaxPendientesPorPersona && n < store.MaxPendientesDeFuera; i++ {
			if err := pide("org"+strconv.Itoa(org), "p"+strconv.Itoa(org), i); err != nil {
				t.Fatal(err)
			}
			n++
		}
	}
	// Llena de todas las organizaciones: no se rechaza —un 429 diría a quien pide
	// cuáles le han llegado—, se crea silenciada.
	p, err := st.CrearPeticionFirma(tenantCtx("otra"), store.PeticionFirma{
		Solicitante: "x", Destinatario: "leire", Fichero: "fx", Ruta: "/a.pdf", Externo: true, Huella: "h",
	})
	if err != nil || !p.Silenciada {
		t.Errorf("entre todas las organizaciones: %+v %v", p, err)
	}
	if err := st.PuedePedirFirma(tenantCtx("otra"), store.PeticionFirma{Solicitante: "y", Destinatario: "leire", Fichero: "fy", Externo: true}); err != nil {
		t.Errorf("con el tope lleno, pedir se rechaza: %v", err)
	}
	// Cerrar una sin firmar con un token dentro no se deja.
	otra := tenantCtx("zeta")
	p, err = st.CrearPeticionFirma(otra, store.PeticionFirma{Solicitante: "z", Destinatario: "mikel", Fichero: "f", Ruta: "/a.pdf", Externo: true, Huella: "h"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := st.ResolverPeticionFirma(otra, p.ID, "mikel", store.FirmaRechazada, "", "", []byte("t"), ""); err == nil {
		t.Error("rechazada con token")
	}
}

// TestBloqueosYPoliticaDeFirma (ADR 0059) contra Postgres de verdad: el bloqueo es de
// la persona y silencia lo pendiente en todas las organizaciones; lo silenciado no le
// llega por ningún lado; la aplicación no lee los bloqueos de nadie; y la política
// admite según su modo, sin fila es abierta, y en una fusión la del origen se va.
func TestBloqueosYPoliticaDeFirma(t *testing.T) {
	h := harnessComoEnProduccion(t)
	st := store.New(h.Pool)
	acme, globex := tenantCtx("acme"), tenantCtx("globex")
	nueva := func(ctx context.Context, sol, fichero string, silenciada bool) store.PeticionFirma {
		p, err := st.CrearPeticionFirma(ctx, store.PeticionFirma{
			Solicitante: sol, SolicitanteNombre: sol, Destinatario: "leire",
			Fichero: fichero, Ruta: "/a.pdf", Externo: true, Huella: "h", Silenciada: silenciada,
		})
		if err != nil {
			t.Fatal(err)
		}
		return p
	}
	p1 := nueva(acme, "ana", "f1", false)
	nueva(tenantCtx("initech"), "ana", "f2", false)
	otra := nueva(acme, "bea", "f3", false)

	if err := st.BloquearFirma(globex, "leire", "ana", "Ana", "globex"); err != nil {
		t.Fatal(err)
	}
	if err := st.BloquearFirma(globex, "leire", "leire", "yo", "globex"); err == nil {
		t.Error("bloquearse a sí misma")
	}
	if si, _ := st.FirmaBloqueada(acme, "leire", "ana"); !si {
		t.Error("no consta el bloqueo")
	}
	if si, _ := st.FirmaBloqueada(acme, "ana", "leire"); si {
		t.Error("el bloqueo va en los dos sentidos")
	}
	r, _, err := st.PeticionesFirma(globex, "leire")
	if err != nil || len(r) != 1 || r[0].ID != otra.ID {
		t.Fatalf("lo que le llega tras bloquear: %+v %v", r, err)
	}
	if got, _ := st.AnfitrionPeticionFirma(globex, p1.ID, "leire"); got != "" {
		t.Error("una silenciada sigue teniendo anfitriona")
	}
	if _, err := st.PeticionFirmaPendiente(acme, p1.ID, "leire"); !errors.Is(err, store.ErrPeticionFirmaNoEsta) {
		t.Errorf("una silenciada, pendiente para quien firma: %v", err)
	}
	if _, err := st.OcuparPeticionFirma(acme, p1.ID, "leire", true); !errors.Is(err, store.ErrPeticionFirmaNoEsta) {
		t.Errorf("una silenciada se abre: %v", err)
	}
	if _, err := st.ResolverPeticionFirma(acme, p1.ID, "leire", store.FirmaRechazada, "", "", nil, ""); !errors.Is(err, store.ErrPeticionFirmaNoEsta) {
		t.Errorf("una silenciada se rechaza: %v", err)
	}
	if _, s, _ := st.PeticionesFirma(acme, "ana"); len(s) != 1 || s[0].Estado != store.FirmaPendiente {
		t.Errorf("quien pidió no la ve como siempre: %+v", s)
	}
	if _, err := st.ResolverPeticionFirma(acme, p1.ID, "ana", store.FirmaCancelada, "", "", nil, ""); err != nil {
		t.Errorf("quien pidió no la cancela: %v", err)
	}
	// Bloquear se deja a sí mismo fuera; y un bloqueo que llega justo antes de crear
	// la petición también la silencia (el INSERT lo mira).
	if err := st.BloquearFirma(globex, "leire", "carmen", "Carmen", "globex"); err != nil {
		t.Fatal(err)
	}
	if p, err := st.CrearPeticionFirma(acme, store.PeticionFirma{
		Solicitante: "carmen", Destinatario: "leire",
		Fichero: "f8", Ruta: "/a.pdf", Externo: true, Huella: "h",
	}); err != nil || !p.Silenciada {
		t.Errorf("creada tras el bloqueo sin silenciar: %+v %v", p, err)
	}
	if err := st.DesbloquearFirma(globex, "leire", "carmen"); err != nil {
		t.Fatal(err)
	}
	// Lo silenciado no cuenta para el tope de quien recibe: con su tope lleno de
	// silenciadas, la siguiente le sigue llegando.
	for i := range store.MaxPendientesDeFuera {
		nueva(tenantCtx("org"+strconv.Itoa(i)), "x"+strconv.Itoa(i), "s", true)
	}
	if p := nueva(tenantCtx("zeta"), "z", "q", false); p.Silenciada {
		t.Error("lo silenciado llena el tope de quien recibe")
	}
	// Una silenciada en SU organización (tras una fusión) tampoco le sale en la lista.
	nueva(globex, "y", "fy", true)
	if r, _, _ := st.PeticionesFirma(globex, "leire"); slices.ContainsFunc(r, func(x store.PeticionFirma) bool { return x.Silenciada }) {
		t.Error("una silenciada de su organización le sale en la lista")
	}
	// La aplicación no lee los bloqueos de nadie: solo por las funciones.
	var vistos int
	if err := h.Pool.InTenantTx(globex, func(tx pgx.Tx) error {
		return tx.QueryRow(context.Background(), `SELECT count(*) FROM pdf_firma_bloqueos`).Scan(&vistos)
	}); err != nil || vistos != 0 {
		t.Errorf("la aplicación lee la tabla de bloqueos: %d %v", vistos, err)
	}
	// Ni ve peticiones de otra organización fuera de las funciones.
	if err := h.Pool.InTenantTx(globex, func(tx pgx.Tx) error {
		return tx.QueryRow(context.Background(), `SELECT count(*) FROM pdf_peticiones_firma WHERE tenant <> 'globex'`).Scan(&vistos)
	}); err != nil || vistos != 0 {
		t.Errorf("globex ve peticiones de otras: %d %v", vistos, err)
	}
	if bs, err := st.BloqueosFirma(globex, "leire"); err != nil || len(bs) != 1 || bs[0].Nombre != "Ana" {
		t.Errorf("bloqueos: %+v %v", bs, err)
	}
	if bs, _ := st.BloqueosFirma(globex, "otra"); len(bs) != 0 {
		t.Error("los bloqueos de otra persona")
	}
	if err := st.DesbloquearFirma(globex, "leire", "ana"); err != nil {
		t.Fatal(err)
	}
	if si, _ := st.FirmaBloqueada(acme, "leire", "ana"); si {
		t.Error("sigue bloqueada")
	}

	// La política: sin fila, abierta; cerrada; de confianza.
	admite := func(orgs []string, origen string) bool {
		si, err := st.FirmaAdmitida(acme, orgs, origen)
		if err != nil {
			t.Fatal(err)
		}
		return si
	}
	if p, _ := st.PoliticaDeFirma(globex); p.Modo != store.PoliticaAbierta || !admite([]string{"globex"}, "acme") {
		t.Errorf("sin fila: %+v", p)
	}
	if _, err := st.GuardarPoliticaDeFirma(globex, store.PoliticaFirma{Modo: store.PoliticaCerrada, Confianza: []string{}}, "admin"); err != nil {
		t.Fatal(err)
	}
	if admite([]string{"globex"}, "acme") || !admite([]string{"globex", "personal"}, "acme") {
		t.Error("cerrada")
	}
	if admite([]string{"acme"}, "acme") {
		t.Error("la propia organización cuenta como de fuera")
	}
	if _, err := st.GuardarPoliticaDeFirma(globex, store.PoliticaFirma{Modo: store.PoliticaConfianza, Confianza: []string{"acme"}}, "admin"); err != nil {
		t.Fatal(err)
	}
	if !admite([]string{"globex"}, "acme") || admite([]string{"globex"}, "initech") {
		t.Error("de confianza")
	}
	if p, _ := st.PoliticaDeFirma(acme); p.Modo != store.PoliticaAbierta {
		t.Error("una organización ve la política de otra")
	}
	// Desde globex, de confianza solo acme: la lista de Leire deja fuera lo demás.
	nueva(tenantCtx("initech"), "carl", "f9", false)
	if r, _, _ := st.PeticionesFirma(globex, "leire"); len(r) != 1 || r[0].ID != otra.ID {
		t.Errorf("la lista no aplica la política: %+v", r)
	}

	// Fusión de acme en globex: la política de acme (si la hubiera) se va y quien
	// confiaba en acme confía en globex; initech confía en acme.
	if _, err := st.GuardarPoliticaDeFirma(tenantCtx("initech"), store.PoliticaFirma{Modo: store.PoliticaConfianza, Confianza: []string{"acme"}}, "a"); err != nil {
		t.Fatal(err)
	}
	if _, err := st.GuardarPoliticaDeFirma(acme, store.PoliticaFirma{Modo: store.PoliticaCerrada, Confianza: []string{}}, "a"); err != nil {
		t.Fatal(err)
	}
	var nota *string
	if err := h.Pool.InMergeTx(context.Background(), func(tx pgx.Tx) error {
		return tx.QueryRow(context.Background(), `SELECT platform_merge_prepare('acme', 'globex')`).Scan(&nota)
	}); err != nil || nota == nil {
		t.Fatalf("prepare: %v %v", nota, err)
	}
	if p, _ := st.PoliticaDeFirma(acme); p.Actualizada != nil {
		t.Error("la política del origen sigue")
	}
	if p, _ := st.PoliticaDeFirma(tenantCtx("initech")); fmt.Sprint(p.Confianza) != "[globex]" {
		t.Errorf("la confianza no pasó al destino: %+v", p)
	}
	// Y fuera de una fusión no se llama.
	if err := h.Pool.InTenantTx(acme, func(tx pgx.Tx) error {
		_, err := tx.Exec(context.Background(), `SELECT platform_merge_prepare('acme', 'globex')`)
		return err
	}); err == nil {
		t.Error("platform_merge_prepare fuera de una fusión")
	}
}

// TestCadenaDeFirmasEnLaBase (mig. 0034): una cadena de tres; solo la del turno está
// pendiente y solo ella la ve su destinatario; al firmar, el turno pasa a la siguiente
// con el firmado; rechazar cierra la cadena. Y el barrido encuentra lo vencido y lo
// que toca recordar en todas las organizaciones, pero lo cambia en la suya.
func TestCadenaDeFirmasEnLaBase(t *testing.T) {
	h := harnessComoEnProduccion(t)
	st := store.New(h.Pool)
	acme, globex := tenantCtx("acme"), tenantCtx("globex")
	fila := func(dest string) store.PeticionFirma {
		return store.PeticionFirma{Solicitante: "ana", SolicitanteNombre: "Ana", Destinatario: dest,
			Fichero: "f1", Ruta: "/PDF/a.pdf", RecordarDias: 2}
	}
	if _, err := st.CrearCadenaFirma(acme, []store.PeticionFirma{fila("irati")}); err == nil {
		t.Error("una cadena de una")
	}
	c, err := st.CrearCadenaFirma(acme, []store.PeticionFirma{fila("irati"), fila("jon"), fila("leire")})
	if err != nil || len(c) != 3 || c[0].Circuito == "" || c[0].Circuito != c[2].Circuito ||
		c[0].Estado != store.FirmaPendiente || c[1].Estado != store.FirmaEnEspera || c[2].Orden != 2 {
		t.Fatalf("crear: %+v %v", c, err)
	}
	if r, _, _ := st.PeticionesFirma(acme, "jon"); len(r) != 0 {
		t.Errorf("jon ve la suya antes de su turno: %+v", r)
	}
	if _, s, _ := st.PeticionesFirma(acme, "ana"); len(s) != 3 {
		t.Errorf("ana ve %d de su cadena", len(s))
	}
	if _, err := st.ResolverPeticionFirma(acme, c[1].ID, "jon", store.FirmaFirmada, "f2", "/PDF/b.pdf", nil, ""); !errors.Is(err, store.ErrPeticionFirmaNoEsta) {
		t.Errorf("jon firma antes de su turno: %v", err)
	}
	f, err := st.ResolverPeticionFirma(acme, c[0].ID, "irati", store.FirmaFirmada, "f2", "/PDF/a (firmado).pdf", nil, "")
	if err != nil || f.Siguiente == nil || f.Siguiente.ID != c[1].ID || f.Siguiente.Estado != store.FirmaPendiente ||
		f.Siguiente.Fichero != "f2" || f.Siguiente.Ruta != "/PDF/a (firmado).pdf" {
		t.Fatalf("firmar irati: %+v %v", f.Siguiente, err)
	}
	if r, _, _ := st.PeticionesFirma(acme, "jon"); len(r) != 1 || r[0].Fichero != "f2" {
		t.Errorf("jon, en su turno: %+v", r)
	}
	if _, err := st.ResolverPeticionFirma(acme, c[1].ID, "jon", store.FirmaRechazada, "", "", nil, ""); err != nil {
		t.Fatal(err)
	}
	if r, _, _ := st.PeticionesFirma(acme, "leire"); len(r) != 1 || r[0].Estado != store.FirmaCancelada {
		t.Errorf("leire tras rechazar jon: %+v", r)
	}

	// Una cadena de globex que caduca y una suelta de acme a la que le toca recordar.
	ayer := time.Now().Add(-24 * time.Hour)
	vencida := []store.PeticionFirma{fila("irati"), fila("jon")}
	for i := range vencida {
		vencida[i].Caduca = &ayer
	}
	cg, err := st.CrearCadenaFirma(globex, vencida)
	if err != nil {
		t.Fatal(err)
	}
	if cg[0].Estado != store.FirmaCaducada {
		t.Errorf("pasada su fecha se lee caducada aunque no haya pasado el barrido: %+v", cg[0])
	}
	if _, err := st.ResolverPeticionFirma(globex, cg[0].ID, "irati", store.FirmaFirmada, "f2", "/PDF/b.pdf", nil, ""); !errors.Is(err, store.ErrPeticionFirmaNoEsta) {
		t.Errorf("firmar una caducada: %v", err)
	}
	suelta, err := st.CrearPeticionFirma(acme, store.PeticionFirma{Solicitante: "ana", Destinatario: "jon", Fichero: "f9", Ruta: "/x.pdf", RecordarDias: 1})
	if err != nil {
		t.Fatal(err)
	}
	if err := h.Pool.InTx(acme, func(tx pgx.Tx) error {
		_, err := tx.Exec(acme, `SELECT set_config('app.merge', 'on', true)`)
		if err == nil {
			_, err = tx.Exec(acme, `UPDATE pdf_peticiones_firma SET avisada = now() - interval '25 hours' WHERE id = $1`, suelta.ID)
		}
		return err
	}); err != nil {
		t.Fatal(err)
	}
	v, r, err := st.PeticionesFirmaPorBarrer(acme)
	if err != nil || len(v) != 2 || v[0].Tenant != "globex" || len(r) != 1 || r[0].Peticion.ID != suelta.ID || r[0].Tenant != "acme" {
		t.Fatalf("por barrer: %+v %+v %v", v, r, err)
	}
	if _, err := st.CaducarPeticionFirma(acme, v[0].Peticion.ID); !errors.Is(err, store.ErrPeticionFirmaNoEsta) {
		t.Errorf("caducar desde otra organización: %v", err)
	}
	for _, x := range v {
		if p, err := st.CaducarPeticionFirma(globex, x.Peticion.ID); err != nil || p.Estado != store.FirmaCaducada || p.Resuelta == nil {
			t.Errorf("caducar: %+v %v", p, err)
		}
	}
	if p, err := st.RecordarPeticionFirma(acme, suelta.ID); err != nil || time.Since(p.Avisada) > time.Minute {
		t.Errorf("recordar: %+v %v", p, err)
	}
	if _, err := st.RecordarPeticionFirma(acme, suelta.ID); !errors.Is(err, store.ErrPeticionFirmaNoEsta) {
		t.Errorf("recordar dos veces seguidas: %v", err)
	}
	if v, r, err := st.PeticionesFirmaPorBarrer(acme); err != nil || len(v) != 0 || len(r) != 0 {
		t.Errorf("la segunda vuelta: %+v %+v %v", v, r, err)
	}
}
