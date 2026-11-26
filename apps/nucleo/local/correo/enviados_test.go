// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package correo

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"git.sytue.eu/sytue/one/apps/nucleo/eml"
)

// colaDe son los mensajes que esperan en la cola de enviados de una cuenta.
func colaDe(t *testing.T, raiz, cuenta string) []string {
	t.Helper()
	es, err := os.ReadDir(rutaColaEnviados(raiz, cuenta))
	if err != nil && !os.IsNotExist(err) {
		t.Fatal(err)
	}
	out := make([]string, 0, len(es))
	for _, e := range es {
		out = append(out, e.Name())
	}
	return out
}

func facturaDePrueba(t *testing.T, asunto string) []byte {
	t.Helper()
	crudo, err := eml.Componer(eml.Salida{
		De: usuario, DeNombre: "Acme", Para: []string{"pagos@cliente.es"}, Asunto: asunto,
		Texto: "Adjunta la factura.", Adjuntos: []eml.Adjunto{{Nombre: "FV.pdf", Tipo: "application/pdf", Datos: []byte("%PDF-1.7")}},
	}, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	return crudo
}

// Lo que otra app (el ERP) envía por el Remitente en una cuenta cuyo servidor no guarda
// la copia (IMAP genérico) acaba en «Enviados» del servidor y del índice de Mail, sin
// que el Remitente abra el buzón: lo deja en la cola compartida y Mail, con su bucle en
// marcha y mientras la otra app envía desde otra rutina, lo recoge y lo sube con APPEND
// una sola vez. Recoger otra vez lo mismo (un corte entre guardarlo y quitarlo de la
// cola) no lo duplica.
func TestLoQueEnviaOtraAppAcabaEnEnviados(t *testing.T) {
	ctx := t.Context()
	si, ss := nuevoIMAP(t), nuevoSMTP(t)
	raiz := t.TempDir()
	secretos := &secretosEnMemoria{m: map[string]string{}}
	b, err := Abrir(raiz, secretos, Red{Marcar: marcarDirecto})
	if err != nil {
		t.Fatal(err)
	}
	defer b.Cerrar()
	c, err := b.Anadir(Cuenta{
		Proveedor: ProveedorIMAP, Direccion: usuario, Nombre: "Ana",
		IMAP: Servidor{Host: "127.0.0.1", Puerto: puerto(si.ln), Seguridad: SeguridadNinguna},
		SMTP: Servidor{Host: "127.0.0.1", Puerto: puerto(ss.ln), Seguridad: SeguridadNinguna},
	}, clave)
	if err != nil {
		t.Fatal(err)
	}
	if err := b.Sincronizar(ctx); err != nil {
		t.Fatal(err)
	}
	b.Intervalo = time.Hour // solo las vueltas que se piden
	b.Arrancar()

	r := NuevoRemitente(raiz, secretos, Red{Marcar: marcarDirecto})
	crudo := facturaDePrueba(t, "Factura FV-2026-00001")
	enviado := make(chan error, 1)
	go func() { enviado <- r.Enviar(ctx, c.ID, []string{"pagos@cliente.es"}, crudo) }()
	if err := <-enviado; err != nil {
		t.Fatal(err)
	}
	if len(ss.lista()) != 1 {
		t.Fatal("no salió por SMTP")
	}
	limite := time.Now().Add(20 * time.Second)
	for {
		n, _ := si.cuantos(t, "Sent")
		if n == 1 && len(colaDe(t, raiz, c.ID)) == 0 {
			break
		}
		if time.Now().After(limite) {
			t.Fatalf("Enviados del servidor: %d; en la cola: %v", n, colaDe(t, raiz, c.ID))
		}
		time.Sleep(50 * time.Millisecond)
	}
	if env, _, _ := b.Messages(ctx, "rol:sent", "", 50); len(env) != 1 || env[0].Subject != "Factura FV-2026-00001" {
		t.Fatalf("Enviados de Mail: %+v", env)
	}

	// Lo mismo otra vez en la cola: ya está en «Enviados», no se duplica.
	if err := dejarParaEnviados(raiz, c.ID, crudo); err != nil {
		t.Fatal(err)
	}
	limite = time.Now().Add(10 * time.Second)
	for len(colaDe(t, raiz, c.ID)) != 0 {
		if time.Now().After(limite) {
			t.Fatal("la cola no se vacía")
		}
		time.Sleep(50 * time.Millisecond)
	}
	if err := b.Sincronizar(ctx); err != nil {
		t.Fatal(err)
	}
	if n, _ := si.cuantos(t, "Sent"); n != 1 {
		t.Fatalf("duplicado en el servidor: %d", n)
	}
	if env, _, _ := b.Messages(ctx, "rol:sent", "", 50); len(env) != 1 {
		t.Fatalf("duplicado en Mail: %d", len(env))
	}
	// Un temporal a medio escribir no se toca.
	tmp := filepath.Join(rutaColaEnviados(raiz, c.ID), ".x.eml.123.tmp")
	if err := os.WriteFile(tmp, []byte("From: a"), 0o600); err != nil {
		t.Fatal(err)
	}
	if n := b.recogerEnviados(ctx); n != 0 {
		t.Fatalf("recogió un temporal: %d", n)
	}
	if _, err := os.Stat(tmp); err != nil {
		t.Fatal("el temporal desapareció:", err)
	}
}

// Una cuenta cuyo servidor guarda solo la copia (Gmail, Outlook) no deja nada en la cola;
// una cuenta que Mail aún no ha sincronizado nunca espera (su «Enviados» sería solo de
// este equipo); y la cola de una cuenta que se quita se va con ella.
func TestLaColaDeEnviadosEsperaYSeVaConLaCuenta(t *testing.T) {
	ctx := t.Context()
	si, ss := nuevoIMAP(t), nuevoSMTP(t)
	raiz := t.TempDir()
	secretos := &secretosEnMemoria{m: map[string]string{}}
	b, err := Abrir(raiz, secretos, Red{Marcar: marcarDirecto})
	if err != nil {
		t.Fatal(err)
	}
	defer b.Cerrar()
	// Las dos entran con el mismo usuario: los servidores de prueba solo conocen uno.
	servidores := func(c Cuenta) Cuenta {
		c.IMAP = Servidor{Host: "127.0.0.1", Puerto: puerto(si.ln), Seguridad: SeguridadNinguna, Usuario: usuario}
		c.SMTP = Servidor{Host: "127.0.0.1", Puerto: puerto(ss.ln), Seguridad: SeguridadNinguna, Usuario: usuario}
		return c
	}
	guarda, err := b.Anadir(servidores(Cuenta{Proveedor: ProveedorIMAP, Direccion: "otra@ejemplo.eu", GuardaEnviados: true}), clave)
	if err != nil {
		t.Fatal(err)
	}
	c, err := b.Anadir(servidores(Cuenta{Proveedor: ProveedorIMAP, Direccion: usuario}), clave)
	if err != nil {
		t.Fatal(err)
	}
	r := NuevoRemitente(raiz, secretos, Red{Marcar: marcarDirecto})
	if err := r.Enviar(ctx, guarda.ID, []string{"pagos@cliente.es"}, facturaDePrueba(t, "Con copia del servidor")); err != nil {
		t.Fatal(err)
	}
	if cola := colaDe(t, raiz, guarda.ID); len(cola) != 0 {
		t.Fatalf("la cuenta que guarda sola dejó algo en la cola: %v", cola)
	}
	if err := r.Enviar(ctx, c.ID, []string{"pagos@cliente.es"}, facturaDePrueba(t, "Sin sincronizar")); err != nil {
		t.Fatal(err)
	}
	if n := b.recogerEnviados(ctx); n != 0 || len(colaDe(t, raiz, c.ID)) != 1 {
		t.Fatalf("una cuenta sin sincronizar no debería recoger: %d", n)
	}
	if err := b.Sincronizar(ctx); err != nil {
		t.Fatal(err)
	}
	if n := b.recogerEnviados(ctx); n != 1 || len(colaDe(t, raiz, c.ID)) != 0 {
		t.Fatalf("tras sincronizar, recoger: %d", n)
	}
	if err := dejarParaEnviados(raiz, c.ID, facturaDePrueba(t, "Se va con la cuenta")); err != nil {
		t.Fatal(err)
	}
	if err := b.Quitar(c.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(rutaColaEnviados(raiz, c.ID)); !os.IsNotExist(err) {
		t.Fatal("la cola de la cuenta quitada sigue")
	}
	// Una cola de una cuenta que no existe (quitada con Mail cerrado) se tira al recoger.
	if err := dejarParaEnviados(raiz, "no-existe", facturaDePrueba(t, "Huérfano")); err != nil {
		t.Fatal(err)
	}
	b.recogerEnviados(ctx)
	if _, err := os.Stat(rutaColaEnviados(raiz, "no-existe")); !os.IsNotExist(err) {
		t.Fatal("la cola huérfana sigue")
	}
	if !strings.HasSuffix(rutaColaEnviados(raiz, "../x"), filepath.Join(carpetaColaEnviados, "x")) {
		t.Fatal("la cuenta no puede salirse de la cola")
	}
}
