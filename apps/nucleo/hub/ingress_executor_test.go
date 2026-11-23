// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package hub

import (
	"bytes"
	"errors"
	"strings"
	"testing"
	"time"

	"git.sytue.eu/sytue/one/apps/nucleo/storage"
	"git.sytue.eu/sytue/one/apps/nucleo/storage/ingress"
	"git.sytue.eu/sytue/one/apps/nucleo/storage/records"
)

const ingressNoticeID = "4b1f0e2a-6c55-4e7e-9a59-2f0d7c3f9a10"

// ingressMetadata es lo que cifra el Hub (store.AvisoCifrado): las mismas claves.
func ingressMetadata(title string) []byte {
	return []byte(`{"v":1,"tenant":"acme","id":"` + ingressNoticeID + `","source":"drive","type":"security.storage.released","level":"warning","title":"` + title + `","url":"https://sytue.eu/one","params":{"who":"ana"},"created_at":"2026-11-23T10:00:00Z"}`)
}

func ingressClaim(scope storage.Scope, target storage.PublicationTarget) ingress.Claim {
	hex := strings.Repeat("ab", 32)
	task := ingress.Task{
		Version: 1, Scope: scope, Epoch: "0b5c7c2e-1f7a-4d43-8d0e-5a3c9b1e2f40", ID: ingressNoticeID, Kind: IngressKind, KeyID: hex, KeyGeneration: 1,
		Cipher: storage.StreamSourceRow{Kind: "ingress", ID: ingressNoticeID, Size: 512, SHA256: hex},
	}
	return ingress.Claim{Version: 1, Task: task, Target: target, Token: "token1", Generation: 1, ExpiresAt: time.Now().Add(time.Minute)}
}

// Un aviso que llega por la entrada privada se guarda una vez en el almacén privado,
// se reconoce al reintentar (también ya leído) y devuelve el commit de la sala del
// reclamo, que es lo que el Hub verifica para dar el recibo.
func TestIngressExecutorStoresNoticeOnceAndReconciles(t *testing.T) {
	r, port, _ := setupPrivate(t)
	exec := IngressExecutor{Repository: r}
	claim := ingressClaim(port.binding.Scope, port.binding.Target)
	metadata := ingressMetadata("Soltaste la fuente")

	if ref, err := exec.Reconcile(t.Context(), claim, metadata, bytes.NewReader(nil), 0); err != nil || ref != nil {
		t.Fatal("reconciled a notice that was never stored", ref, err)
	}
	ref, err := exec.Execute(t.Context(), claim, metadata, bytes.NewReader(nil), 0)
	if err != nil || ref.RoomID != port.binding.Target.RoomID || !ref.Valid() {
		t.Fatal(ref, err)
	}
	got, err := r.List(t.Context())
	if err != nil || len(got) != 1 || got[0].ID != ingressNoticeID || got[0].Title != "Soltaste la fuente" || got[0].Type != "security.storage.released" || got[0].Params["who"] != "ana" {
		t.Fatal("stored notice", got, err)
	}
	if err := r.MarkRead(t.Context(), ingressNoticeID); err != nil {
		t.Fatal(err)
	}
	port.mu.Lock()
	objects := len(port.objects)
	port.mu.Unlock()
	again, err := exec.Execute(t.Context(), claim, metadata, bytes.NewReader(nil), 0)
	if err != nil || !again.Valid() {
		t.Fatal("retry after read", again, err)
	}
	port.mu.Lock()
	if len(port.objects) != objects {
		port.mu.Unlock()
		t.Fatal("retry published another revision")
	}
	port.mu.Unlock()
	if got, _ := r.List(t.Context()); len(got) != 1 || !got[0].Read {
		t.Fatal("retry reset read state or duplicated notice", got)
	}
	if _, err := exec.Execute(t.Context(), claim, ingressMetadata("Otro texto"), bytes.NewReader(nil), 0); !errors.Is(err, records.ErrConflict) {
		t.Fatal("different notice with the same id was not a conflict", err)
	}
	other := claim
	other.Target.RoomID = "!other:test"
	if _, err := exec.Reconcile(t.Context(), other, metadata, bytes.NewReader(nil), 0); !errors.Is(err, ingress.ErrReceipt) {
		t.Fatal("accepted a commit outside the claimed room", err)
	}
}

func TestIngressExecutorRejectsForeignOrMalformedDeliveries(t *testing.T) {
	r, port, _ := setupPrivate(t)
	exec := IngressExecutor{Repository: r}
	claim := ingressClaim(port.binding.Scope, port.binding.Target)
	good := ingressMetadata("Aviso")
	for name, tc := range map[string]struct {
		claim    func(ingress.Claim) ingress.Claim
		metadata []byte
		size     int64
	}{
		"otra organización": {metadata: bytes.Replace(good, []byte(`"tenant":"acme"`), []byte(`"tenant":"globex"`), 1)},
		"otro id":           {metadata: bytes.Replace(good, []byte(ingressNoticeID), []byte("otro"), 1)},
		"campo desconocido": {metadata: bytes.Replace(good, []byte(`"v":1`), []byte(`"v":1,"subject":"bob"`), 1)},
		"versión":           {metadata: bytes.Replace(good, []byte(`"v":1`), []byte(`"v":2`), 1)},
		"enlace inseguro":   {metadata: bytes.Replace(good, []byte(`https://sytue.eu/one`), []byte(`javascript:alert(1)`), 1)},
		"con cuerpo":        {metadata: good, size: 3},
		"otro tipo": {metadata: good, claim: func(c ingress.Claim) ingress.Claim {
			c.Task.Kind = "mail.receive"
			return c
		}},
		"otra persona": {metadata: good, claim: func(c ingress.Claim) ingress.Claim {
			c.Task.Scope.Subject = "bob"
			return c
		}},
	} {
		t.Run(name, func(t *testing.T) {
			c := claim
			if tc.claim != nil {
				c = tc.claim(c)
			}
			if _, err := exec.Execute(t.Context(), c, tc.metadata, bytes.NewReader(nil), tc.size); err == nil {
				t.Fatal("accepted")
			}
		})
	}
	if got, err := r.List(t.Context()); err != nil || len(got) != 0 {
		t.Fatal("a rejected delivery was stored", got, err)
	}
}
