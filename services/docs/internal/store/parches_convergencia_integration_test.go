// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

//go:build integration

package store_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math/rand/v2"
	"sync"
	"testing"

	"git.sytue.eu/sytue/one/services/docs/internal/domain"
	"git.sytue.eu/sytue/one/services/docs/internal/store"
)

const libroConvergencia = `{"active":0,"sheets":[{"id":"h1","name":"H1","cells":{}},{"id":"h2","name":"H2","cells":{}}]}`

// registroDe es el valor del registro de contenido de un estado, como cadena.
func registroDe(t *testing.T, st *domain.State) string {
	t.Helper()
	var s string
	if err := json.Unmarshal(st.Materialize().Registers[domain.ClaveHoja], &s); err != nil {
		t.Fatalf("registro de contenido: %v", err)
	}
	return s
}

// Convergencia con N clientes que escriben a la vez: parches con reintentos y
// duplicados, orden barajado, reconexiones con replay en desorden y un cliente
// «antiguo» que escribe el registro completo con base_seq. Lo que se exige:
//  1. nada se pierde: cada celda privada de cada cliente está en el resultado;
//  2. nada se repite: cada op_id consta una sola vez y las seq son 1..n sin huecos;
//  3. el resultado es el de aplicar el log en orden de seq (el orden lo pone el servidor);
//  4. todos los clientes, tras replay desordenado y con duplicados, ven lo mismo que el servidor.
func TestConvergenciaDeParchesConNClientes(t *testing.T) {
	st := newStore(t)
	svc := domain.NewService(st)
	ctx := tenantCtx("acme")
	doc, _, err := st.CreateDocument(ctx, domain.CreateDocumentParams{
		Kind: domain.KindSpreadsheet, Title: "Convergencia", OwnerSub: "user-0",
	})
	if err != nil {
		t.Fatal(err)
	}
	const clientes, porCliente = 6, 15
	for i := 1; i < clientes; i++ {
		if err := st.UpsertPermission(ctx, doc.ID, domain.Permission{
			PrincipalKind: domain.PrincipalUser, PrincipalID: fmt.Sprintf("user-%d", i),
			Role: domain.RoleEditor, GrantedBy: "user-0",
		}); err != nil {
			t.Fatal(err)
		}
	}
	semilla, _ := json.Marshal(libroConvergencia)
	if _, _, err := svc.ApplyOps(ctx, domain.Subject{Sub: "user-0"}, doc.ID, []domain.Op{{
		Kind: domain.OpSet, Key: domain.ClaveHoja, Value: semilla, Lamport: 1,
		Dot: domain.Dot{Actor: "semilla", Counter: 1},
	}}, nil); err != nil {
		t.Fatal(err)
	}

	// Guion de cada cliente: celdas privadas (únicas, tienen que sobrevivir) y
	// celdas disputadas (gana la última en el orden de seq).
	type guion struct {
		sub    string
		ops    []domain.Op
		privad map[string]string // ref → valor, en la hoja h1
	}
	guiones := make([]guion, clientes)
	for c := range guiones {
		g := guion{sub: fmt.Sprintf("user-%d", c), privad: map[string]string{}}
		rng := rand.New(rand.NewPCG(uint64(c), 7))
		for k := range porCliente {
			opID := fmt.Sprintf("c%d-op%d", c, k)
			if rng.IntN(4) == 0 { // disputada: 5 celdas de la hoja h2
				ref := fmt.Sprintf("A%d", 1+rng.IntN(5))
				g.ops = append(g.ops, celdaPatch(t, opID, "h2", ref, fmt.Sprintf("c%d-%d", c, k), 0))
				continue
			}
			ref := fmt.Sprintf("P%dK%d", c, k)
			valor := fmt.Sprintf("v%d-%d", c, k)
			g.privad[ref] = valor
			g.ops = append(g.ops, celdaPatch(t, opID, "h1", ref, valor, 0))
		}
		rng.Shuffle(len(g.ops), func(i, j int) { g.ops[i], g.ops[j] = g.ops[j], g.ops[i] })
		guiones[c] = g
	}

	// Estado local de cada cliente: lo que ha replayado del servidor.
	estados := make([]*domain.State, clientes)
	var wg sync.WaitGroup
	errs := make(chan error, clientes)
	for c := range guiones {
		estados[c] = domain.NewState()
		wg.Go(func() {
			errs <- ejecutarCliente(ctx, svc, st, doc.ID, c, guiones[c].sub, guiones[c].ops, estados[c])
		})
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatal(err)
		}
	}

	// El log, completo y en orden.
	log, err := st.OpsSince(ctx, doc.ID, 0, 5000)
	if err != nil {
		t.Fatal(err)
	}
	vistos := map[string]bool{}
	contenido := libroConvergencia
	for i, o := range log {
		if o.Seq != int64(i+1) {
			t.Fatalf("hueco en seq: posición %d tiene seq %d", i, o.Seq)
		}
		if o.Op.OpID == "" {
			continue
		}
		if vistos[o.Op.OpID] {
			t.Fatalf("op_id %s repetido en el log", o.Op.OpID)
		}
		vistos[o.Op.OpID] = true
		contenido, err = domain.AplicarCambios(contenido, o.Op.Cambios)
		if err != nil {
			t.Fatal(err)
		}
	}
	final, err := st.LoadState(ctx, doc.ID)
	if err != nil {
		t.Fatal(err)
	}
	servidor := registroDe(t, final.State)
	// (3) el servidor aplicó los parches, en orden de seq, sobre la semilla.
	if esperado := normalizar(t, contenido); normalizar(t, servidor) != esperado {
		t.Fatalf("el registro no es el log aplicado en orden de seq:\n got  %s\n want %s", servidor, esperado)
	}
	// (1) nada se pierde.
	var libro struct {
		Sheets []struct {
			ID    string            `json:"id"`
			Cells map[string]string `json:"cells"`
		} `json:"sheets"`
	}
	if err := json.Unmarshal([]byte(servidor), &libro); err != nil {
		t.Fatal(err)
	}
	for _, g := range guiones {
		for ref, v := range g.privad {
			if got := libro.Sheets[0].Cells[ref]; got != v {
				t.Fatalf("%s: la celda %s vale %q, quiero %q (se perdió)", g.sub, ref, got, v)
			}
		}
	}
	// (2) cada op enviada consta exactamente una vez (los clientes completos escribieron
	// además registros enteros, sin op_id).
	for _, g := range guiones {
		for _, op := range g.ops {
			if !vistos[op.OpID] {
				t.Fatalf("op %s no consta en el log", op.OpID)
			}
		}
	}
	// (4) replay final de cada cliente: el log entero, en desorden y duplicado.
	for c, es := range estados {
		rng := rand.New(rand.NewPCG(uint64(c), 99))
		reproducir(t, es, log, rng)
		if got := registroDe(t, es); got != servidor {
			t.Fatalf("cliente %d diverge del servidor:\n got  %s\n want %s", c, got, servidor)
		}
	}
}

// normalizar reescribe un modelo con claves ordenadas, para comparar sin depender del
// orden en que cada lado serializó.
func normalizar(t *testing.T, s string) string {
	t.Helper()
	var v any
	if err := json.Unmarshal([]byte(s), &v); err != nil {
		t.Fatal(err)
	}
	b, _ := json.Marshal(v)
	return string(b)
}

// reproducir aplica el log al estado en orden barajado y con una copia extra de cada
// op elegida al azar: el CRDT tiene que dar lo mismo.
func reproducir(t *testing.T, es *domain.State, log []domain.SequencedOp, rng *rand.Rand) {
	t.Helper()
	ops := append([]domain.SequencedOp(nil), log...)
	for _, o := range log {
		if rng.IntN(3) == 0 {
			ops = append(ops, o)
		}
	}
	rng.Shuffle(len(ops), func(i, j int) { ops[i], ops[j] = ops[j], ops[i] })
	for _, o := range ops {
		if _, err := es.Apply(o.Op); err != nil {
			t.Fatal(err)
		}
	}
}

// ejecutarCliente manda las ops del guion con reintentos (duplicados), lotes de 1 a 3 y
// reconexiones con replay. El cliente 0 es «antiguo»: lee el registro, le cambia su celda y
// lo escribe entero con base_seq, releyendo si hay 409.
func ejecutarCliente(ctx context.Context, svc *domain.Service, st *store.Store, docID string, c int, sub string, ops []domain.Op, es *domain.State) error {
	rng := rand.New(rand.NewPCG(uint64(c), 1))
	yo := domain.Subject{Sub: sub}
	var ultima int64
	reconectar := func() error {
		nuevas, err := st.OpsSince(ctx, docID, ultima, 5000)
		if err != nil {
			return err
		}
		for _, o := range nuevas {
			ultima = max(ultima, o.Seq)
		}
		// el replay llega desordenado y con duplicados
		rng.Shuffle(len(nuevas), func(i, j int) { nuevas[i], nuevas[j] = nuevas[j], nuevas[i] })
		for _, o := range append(nuevas, nuevas...) {
			if _, err := es.Apply(o.Op); err != nil {
				return err
			}
		}
		return nil
	}
	for len(ops) > 0 {
		n := min(1+rng.IntN(3), len(ops))
		lote := append([]domain.Op(nil), ops[:n]...)
		ops = ops[n:]
		if c == 0 {
			if err := escribirCompleto(ctx, svc, st, docID, yo, lote[0], es); err != nil {
				return err
			}
			lote = lote[1:]
		}
		for intento := 0; intento < 1+rng.IntN(3) && len(lote) > 0; intento++ { // reenvíos: el mismo lote otra vez
			if _, _, err := svc.ApplyOps(ctx, yo, docID, lote, nil); err != nil {
				return fmt.Errorf("%s: %w", sub, err)
			}
		}
		if rng.IntN(3) == 0 {
			if err := reconectar(); err != nil {
				return err
			}
		}
	}
	return reconectar()
}

// escribirCompleto es el cliente que no sabe de parches: lee el registro, aplica el cambio
// él mismo y lo escribe entero con base_seq; ante un 409, relee y repite.
func escribirCompleto(ctx context.Context, svc *domain.Service, st *store.Store, docID string, yo domain.Subject, parche domain.Op, es *domain.State) error {
	for {
		cargado, err := st.LoadState(ctx, docID)
		if err != nil {
			return err
		}
		var actual string
		if err := json.Unmarshal(cargado.State.Materialize().Registers[domain.ClaveHoja], &actual); err != nil {
			return err
		}
		nuevo, err := domain.AplicarCambios(actual, parche.Cambios)
		if err != nil {
			return err
		}
		valor, _ := json.Marshal(nuevo)
		base := cargado.Seq
		lamport := cargado.State.Lamport + 1
		_, _, err = svc.ApplyOps(ctx, yo, docID, []domain.Op{{
			Kind: domain.OpSet, Key: domain.ClaveHoja, Value: valor, Lamport: lamport,
			Dot:     domain.Dot{Actor: "antiguo", Counter: lamport},
			OpID:    parche.OpID, // para comprobar que consta; un cliente antiguo no lo manda
			Cambios: parche.Cambios,
		}}, &base)
		if errors.Is(err, domain.ErrStaleSeq) {
			continue
		}
		return err
	}
}
