// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package store_test

import (
	"context"
	"errors"
	"io"
	"testing"

	"git.sytue.eu/sytue/one/apps/nucleo/storage"
	"git.sytue.eu/sytue/one/platform/pkg/authn"
	"git.sytue.eu/sytue/one/platform/pkg/callaccess"
	"git.sytue.eu/sytue/one/platform/pkg/objectstore"
	"git.sytue.eu/sytue/one/platform/pkg/pg"
	"git.sytue.eu/sytue/one/platform/pkg/tenancy"
	"git.sytue.eu/sytue/one/services/mail/internal/jmap"
	"git.sytue.eu/sytue/one/services/mail/internal/store"
)

type objetosFalsos struct{}

func (objetosFalsos) Put(context.Context, tenancy.Tenant, string, io.Reader, int64, objectstore.PutOptions) error {
	return nil
}

func (objetosFalsos) Get(context.Context, tenancy.Tenant, string) (io.ReadCloser, error) {
	return nil, errors.New("sin objetos")
}

// La entrada privada es la de producción (jmap.PrivateIngress). Sin ella, el
// estado del correo privado decía `private_ingress_ready: false` a TODAS las
// cuentas y Mail no abría (`prepare/error.Error`), ni vaciando los datos.
var _ jmap.PrivateIngress = store.EntradaPrivada{}

func TestLaEntradaPrivadaEstaListaParaQuienEsDueñoDelCorreo(t *testing.T) {
	ana := storage.Scope{Tenant: "acme", Subject: "ana", Product: "mail"}
	ctx := tenancy.Into(t.Context(), tenancy.Tenant{Slug: "acme"})
	ctx = authn.Into(ctx, authn.Claims{Subject: "ana", Tenant: "acme"})
	rooms := callaccess.New("http://rooms.test", "secreto")
	if rooms == nil {
		t.Fatal("cliente de Rooms nil")
	}
	var sinRooms *callaccess.Client // lo que da callaccess.New sin URL o sin secreto

	casos := []struct {
		nombre string
		e      store.EntradaPrivada
		ctx    context.Context
		scope  storage.Scope
		lista  bool
	}{
		{"montada y del dueño", store.NuevaEntradaPrivada(rooms, objetosFalsos{}), ctx, ana, true},
		{"sin Rooms", store.NuevaEntradaPrivada(nil, objetosFalsos{}), ctx, ana, false},
		{"con Rooms nil dentro de la interfaz", store.NuevaEntradaPrivada(sinRooms, objetosFalsos{}), ctx, ana, false},
		{"sin almacén de objetos", store.NuevaEntradaPrivada(rooms, nil), ctx, ana, false},
		{"de otra persona", store.NuevaEntradaPrivada(rooms, objetosFalsos{}), ctx, storage.Scope{Tenant: "acme", Subject: "bea", Product: "mail"}, false},
		{"de otro producto", store.NuevaEntradaPrivada(rooms, objetosFalsos{}), ctx, storage.Scope{Tenant: "acme", Subject: "ana", Product: "drive"}, false},
		{"en nombre de otro", store.NuevaEntradaPrivada(rooms, objetosFalsos{}), authn.Into(ctx, authn.Claims{Subject: "ana", Tenant: "acme", Delegado: true}), ana, false},
		{"sin sesión", store.NuevaEntradaPrivada(rooms, objetosFalsos{}), tenancy.Into(t.Context(), tenancy.Tenant{Slug: "acme"}), ana, false},
	}
	for _, c := range casos {
		lista, err := c.e.PrivateIngressReady(c.ctx, c.scope)
		if err != nil || lista != c.lista {
			t.Errorf("%s: PrivateIngressReady = %v, %v; quería %v", c.nombre, lista, err, c.lista)
		}
		err = c.e.PreparePrivateIngress(c.ctx, c.scope)
		if c.lista && err != nil {
			t.Errorf("%s: PreparePrivateIngress = %v", c.nombre, err)
		}
		if !c.lista && !errors.Is(err, pg.ErrPrivateIngressMissing) {
			t.Errorf("%s: PreparePrivateIngress = %v; quería ErrPrivateIngressMissing", c.nombre, err)
		}
	}
}
