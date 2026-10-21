// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package collab

import (
	"context"
	"testing"

	"git.sytue.eu/sytue/one/services/docs/internal/domain"
)

func TestVersionDelProtocoloNoPermiteEscribirTrasRechazo(t *testing.T) {
	c := NewClient(NewHub(nil), nil, "d1", domain.Subject{Sub: "ana"}, domain.RoleEditor, "Ana")
	h := &sinOps{}
	c.handle(context.Background(), h, ClientMessage{Type: MsgHello, Version: ProtocolVersion + 1})
	m := <-c.send
	if m.Code != "protocol_version" || m.Version != ProtocolVersion {
		t.Fatalf("no se negoció el rechazo: %+v", m)
	}
	// Un segundo mensaje sin versión no puede esquivar el rechazo.
	c.handle(context.Background(), h, ClientMessage{Type: MsgCData, Ops: []domain.Op{{Kind: domain.OpSet, Key: "k"}}})
	if m = <-c.send; m.Code != "protocol_version" || h.aplicadas != 0 {
		t.Fatalf("se escribieron operaciones de otro contrato: %+v, %d", m, h.aplicadas)
	}
}

func TestVersionActualYClientesPublicadosSiguenEditando(t *testing.T) {
	for _, version := range []int{0, ProtocolVersion} {
		c := NewClient(NewHub(nil), nil, "d1", domain.Subject{Sub: "ana"}, domain.RoleEditor, "Ana")
		h := &sinOps{}
		c.handle(context.Background(), h, ClientMessage{Type: MsgHello, Version: version})
		c.handle(context.Background(), h, ClientMessage{Type: MsgCData, Version: version, Ops: []domain.Op{{Kind: domain.OpSet, Key: "k"}}})
		m := <-c.send
		if m.Type != MsgAck || m.Version != ProtocolVersion || h.aplicadas != 1 {
			t.Fatalf("cliente v%d no compatible: %+v, %d", version, m, h.aplicadas)
		}
	}
}
