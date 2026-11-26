// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package correo

import (
	"context"
	"errors"
	"net"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"git.sytue.eu/sytue/one/apps/nucleo/eml"
)

// Otra app de la edición (el ERP) envía por la cuenta de Mail sin abrir su buzón: lee
// las cuentas del mismo `cuentas.json` y la contraseña del mismo llavero, y el mensaje
// sale por el SMTP de la cuenta con la dirección de la cuenta en el sobre.
func TestRemitenteEnviaPorLaCuentaDeMail(t *testing.T) {
	ctx := context.Background()
	ss := nuevoSMTP(t)
	raiz := t.TempDir()
	secretos := &secretosEnMemoria{m: map[string]string{}}

	// Sin Mail configurado: ninguna cuenta, sin error y sin conexión.
	vacio := NuevoRemitente(filepath.Join(raiz, "no-existe"), secretos, Red{Marcar: marcarDirecto})
	if cs, err := vacio.Cuentas(); err != nil || len(cs) != 0 {
		t.Fatalf("sin cuentas: %v %v", cs, err)
	}

	b, err := Abrir(raiz, secretos, Red{Marcar: marcarDirecto})
	if err != nil {
		t.Fatal(err)
	}
	c, err := b.Anadir(Cuenta{
		Proveedor: ProveedorIMAP, Direccion: usuario, Nombre: "Ana",
		IMAP: Servidor{Host: "127.0.0.1", Puerto: 1, Seguridad: SeguridadNinguna},
		SMTP: Servidor{Host: "127.0.0.1", Puerto: puerto(ss.ln), Seguridad: SeguridadNinguna},
	}, clave)
	if err != nil {
		t.Fatal(err)
	}
	b.Cerrar()

	var marcados []string
	r := NuevoRemitente(raiz, secretos, Red{Marcar: func(ctx context.Context, red, dir string) (net.Conn, error) {
		marcados = append(marcados, dir)
		var d net.Dialer
		return d.DialContext(ctx, red, dir)
	}})
	cs, err := r.Cuentas()
	if err != nil || len(cs) != 1 || cs[0].ID != c.ID || cs[0].Direccion != usuario {
		t.Fatalf("cuentas: %+v %v", cs, err)
	}
	crudo, err := eml.Componer(eml.Salida{
		De: usuario, DeNombre: "Acme", Para: []string{"pagos@cliente.es"}, Asunto: "Factura FV-2026-00001",
		Texto: "Hola", Adjuntos: []eml.Adjunto{{Nombre: "FV-2026-00001.pdf", Tipo: "application/pdf", Datos: []byte("%PDF-1.7")}},
	}, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if err := r.Enviar(ctx, c.ID, []string{"pagos@cliente.es"}, crudo); err != nil {
		t.Fatal(err)
	}
	got := ss.lista()
	if len(got) != 1 || got[0].de != usuario || len(got[0].para) != 1 || got[0].para[0] != "pagos@cliente.es" ||
		!strings.Contains(string(got[0].datos), "FV-2026-00001.pdf") {
		t.Fatalf("recibido: %+v", got)
	}
	if len(marcados) != 1 {
		t.Errorf("la conexión no salió por la Red inyectada (el portero): %v", marcados)
	}

	if err := r.Enviar(ctx, "otra", []string{"pagos@cliente.es"}, crudo); !errors.Is(err, ErrSinCuenta) {
		t.Errorf("cuenta que no existe: %v", err)
	}
	if err := r.Enviar(ctx, c.ID, nil, crudo); err == nil {
		t.Error("sin destinatarios no se envía")
	}
	// La contraseña es la del llavero: sin ella, el servidor no deja entrar.
	_ = secretos.Borrar(claveContrasena(c.ID))
	if err := r.Enviar(ctx, c.ID, []string{"pagos@cliente.es"}, crudo); !errors.Is(err, ErrAcceso) {
		t.Errorf("sin contraseña: %v", err)
	}
	if len(ss.lista()) != 1 {
		t.Error("lo que falla no llega al servidor")
	}
}
