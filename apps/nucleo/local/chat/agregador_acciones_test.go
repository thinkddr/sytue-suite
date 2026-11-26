// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package chat

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func agregadorConMatrix(t *testing.T) (*Agregador, *Notas, *accionesFalso) {
	t.Helper()
	dir := t.TempDir()
	n, err := AbrirNotas(filepath.Join(dir, "notas"), "Notas", "notas", "")
	if err != nil {
		t.Fatal(err)
	}
	f, m := nuevaMatrixAcciones(t)
	a := NuevoAgregador(filepath.Join(dir, "preferencias.json"))
	a.CarpetaSubidas = filepath.Join(dir, "subidas")
	a.GuardarEn = filepath.Join(dir, "Drive")
	a.Poner(CuentaNotas, n)
	a.Poner("mx1", m)
	return a, n, f
}

func TestAgregadorAdjuntosEnNotas(t *testing.T) {
	ctx := context.Background()
	a, n, _ := agregadorConMatrix(t)
	cs, _, _ := n.Canales(ctx)
	canal := IDCanal(CuentaNotas, cs[0].ID)
	f, err := a.UploadToChannel(ctx, canal, "lista.txt", "text/plain", strings.NewReader("pan, leche"), 10)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(a.CarpetaSubidas, f.ID)); err != nil {
		t.Fatal("el adjunto espera en su carpeta")
	}
	// Se puede ver antes de mandarlo.
	rc, err := a.ChannelContent(ctx, canal, f.ID, false)
	if err != nil {
		t.Fatal(err)
	}
	rc.Close()
	m, err := a.PostWith(ctx, canal, "la compra", apiSend(f.ID))
	if err != nil {
		t.Fatal(err)
	}
	if len(m.Attachments) != 1 || m.Attachments[0].Name != "lista.txt" || m.Body != "la compra" {
		t.Fatalf("nota con adjunto: %+v", m)
	}
	if _, err := os.Stat(filepath.Join(a.CarpetaSubidas, f.ID)); !os.IsNotExist(err) {
		t.Fatal("mandado, el de espera se borra")
	}
	rc, err = a.ChannelContent(ctx, canal, m.Attachments[0].FileID, false)
	if err != nil {
		t.Fatal(err)
	}
	raw, _ := io.ReadAll(rc)
	rc.Close()
	if string(raw) != "pan, leche" {
		t.Fatalf("leído %q", raw)
	}
	if _, err := a.Messages(ctx, canal, time.Time{}, 50); err != nil {
		t.Fatal(err)
	}
	g, err := a.SaveChannelFileToMyDrive(ctx, canal, m.Attachments[0].FileID)
	if err != nil || g.Name != "lista.txt" {
		t.Fatalf("guardar en Drive: %+v %v", g, err)
	}
	// Quitar una ficha borra lo que esperaba.
	f2, _ := a.UploadToChannel(ctx, canal, "otro.txt", "", strings.NewReader("x"), 1)
	a.ForgetUploads(canal)
	if _, err := os.Stat(filepath.Join(a.CarpetaSubidas, f2.ID)); !os.IsNotExist(err) {
		t.Fatal("olvidado, se borra")
	}
	if _, err := a.PostWith(ctx, canal, "", apiSend(f2.ID)); err == nil {
		t.Fatal("un adjunto olvidado no se manda")
	}
}

func TestAgregadorDirectosPersonasYCrearEnCuenta(t *testing.T) {
	ctx := context.Background()
	a, _, f := agregadorConMatrix(t)
	a.RotuloCuenta = "Cuenta"
	rot, cuentas := a.CuentasParaCrear()
	if rot != "Cuenta" || len(cuentas) != 2 || cuentas[1].ID != "mx1" || cuentas[1].Name != yoMx {
		t.Fatalf("cuentas para crear: %q %+v", rot, cuentas)
	}
	c, err := a.CreateChannel(ctx, "mx1", "Obra", "", false)
	if err != nil || !strings.HasPrefix(c.ID, "mx1|!nueva") || c.CategoryID != PrefijoCategoria+"mx1" || len(f.creadas) != 1 {
		t.Fatalf("crear en Matrix: %+v %v", c, err)
	}
	if c, err := a.CreateChannel(ctx, Equipo, "Ideas", "", false); err != nil || !strings.HasPrefix(c.ID, CuentaNotas+"|") {
		t.Fatalf("sin cuenta elegida, en Notas: %+v %v", c, err)
	}
	ms, _ := a.TeamMembers(ctx, Equipo)
	if len(ms) != 2 || ms[1].Subject != "mx1|"+otroMx {
		t.Fatalf("gente: %+v", ms)
	}
	p, err := a.ResolvePerson(ctx, otroMx)
	if err != nil || p.UserID != "mx1|"+otroMx {
		t.Fatalf("resolver: %+v %v", p, err)
	}
	d, err := a.OpenDirect(ctx, Equipo, []string{p.UserID})
	if err != nil || d.Kind != "direct" || !strings.HasPrefix(d.ID, "mx1|") {
		t.Fatalf("directo: %+v %v", d, err)
	}
	if _, err := a.OpenDirect(ctx, Equipo, []string{"notas|yo", p.UserID}); err == nil {
		t.Fatal("no se mezclan cuentas")
	}
	if _, err := a.AddChannelMember(ctx, c.ID, p.UserID, ""); err != nil || f.invites[0] != otroMx {
		t.Fatalf("añadir: %v %v", err, f.invites)
	}
	if _, err := a.AddChannelMember(ctx, IDCanal(CuentaNotas, "x"), p.UserID, ""); err == nil {
		t.Fatal("una persona de Matrix no entra en un canal de Notas")
	}
	cs, err := a.Browse(ctx, Equipo, "")
	if err != nil || len(cs) != 1 || cs[0].ID != "mx1|!plaza:ejemplo.org" || !strings.HasPrefix(cs[0].Purpose, yoMx+" · ") {
		t.Fatalf("explorar: %+v %v", cs, err)
	}
	if u, err := a.Join(ctx, cs[0].ID); err != nil || u.ID != cs[0].ID {
		t.Fatalf("unirse: %+v %v", u, err)
	}
}

func TestAgregadorHilos(t *testing.T) {
	ctx := context.Background()
	a, n, _ := agregadorConMatrix(t)
	cs, _, _ := n.Canales(ctx)
	canal := IDCanal(CuentaNotas, cs[0].ID)
	raiz, _ := a.Post(ctx, canal, "raíz")
	if _, err := a.ReplyWith(ctx, raiz.ID, "respuesta", apiSend()); err != nil {
		t.Fatal(err)
	}
	hs, sig, err := a.MyThreads(ctx, "")
	if err != nil || sig != "" || len(hs) != 1 || hs[0].Root.ID != raiz.ID {
		t.Fatalf("hilos: %+v %q %v", hs, sig, err)
	}
	if err := a.Follow(ctx, raiz.ID, false); err != nil {
		t.Fatal(err)
	}
	if hs, _, _ := a.MyThreads(ctx, ""); len(hs) != 0 {
		t.Fatalf("dejado de seguir: %+v", hs)
	}
	if h, _ := a.Thread(ctx, raiz.ID); h.Following {
		t.Fatal("el hilo dice que no se sigue")
	}
}

// Con dos cuentas que conocen gente, el nombre de cada persona lleva su cuenta detrás
// para distinguirlas, y PlainName es el de la persona: de él saca la pantalla las
// iniciales del avatar («Luis · @ana:servidor» daba «L@»). Con una sola, el nombre va
// tal cual y PlainName vacío, como hasta ahora.
func TestConVariasCuentasElNombreSinCuentaVaAparte(t *testing.T) {
	ctx := context.Background()
	a, _, _ := agregadorConMatrix(t)
	solo, _ := a.TeamMembers(ctx, Equipo)
	if len(solo) != 2 || solo[1].PlainName != "" || strings.Contains(solo[1].Name, " · ") {
		t.Fatalf("una cuenta: el nombre va tal cual y sin PlainName: %+v", solo)
	}
	if p, err := a.ResolvePerson(ctx, otroMx); err != nil || p.PlainName != "" {
		t.Fatalf("una cuenta: la persona encontrada va sin PlainName: %+v %v", p, err)
	}
	persona := solo[1].Name
	_, otra := nuevaMatrixAcciones(t)
	a.Poner("mx2", otra)

	ms, _ := a.TeamMembers(ctx, Equipo)
	if len(ms) != 3 {
		t.Fatalf("la persona sale una vez por cuenta: %+v", ms)
	}
	for _, m := range ms[1:] {
		if m.PlainName != persona || m.Name != persona+" · "+yoMx {
			t.Errorf("gente de dos cuentas: %+v (persona %q)", m, persona)
		}
	}
	p, err := a.ResolvePerson(ctx, otroMx)
	if err != nil || p.PlainName == "" || p.Name != p.PlainName+" · "+yoMx || strings.Contains(p.PlainName, " · ") {
		t.Fatalf("resolver con dos cuentas: %+v %v", p, err)
	}
}
