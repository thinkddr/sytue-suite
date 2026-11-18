// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package compose

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// hacer ejecuta un objetivo del Makefile de la raíz con COMPOSE_DIR apuntando a un
// directorio de prueba y `docker compose` sustituido por `echo`: lo que se habría
// ejecutado contra Docker sale por la salida estándar. El entorno es el mínimo, para
// que una SYTUE_ID_DEV_SEED_PASSWORD de quien corre los tests no cuele.
func hacer(t *testing.T, dir string, entorno []string, objetivo string) string {
	t.Helper()
	if _, err := exec.LookPath("make"); err != nil {
		t.Skip("sin make en el PATH")
	}
	cmd := exec.Command("make", "-s", "-C", raiz, objetivo, "COMPOSE=echo compose", "COMPOSE_DIR="+dir)
	cmd.Env = append([]string{"PATH=" + os.Getenv("PATH"), "HOME=" + t.TempDir()}, entorno...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("make %s: %v\n%s", objetivo, err, out)
	}
	return string(out)
}

// El seed de desarrollo recibe la contraseña que se le pide. El compose no pasa
// SYTUE_ID_DEV_SEED_PASSWORD al contenedor id, así que antes daba igual fijarla en el
// entorno o en deploy/compose/.env: el admin se creaba siempre con la de desarrollo.
func TestDevSeedPasaLaContrasenaAlContenedor(t *testing.T) {
	const orden = " id /app/id seed"
	casos := []struct {
		nombre  string
		dotenv  string
		entorno []string
		quiere  string
	}{
		{"sin nada, la de desarrollo", "", nil, "sytue-dev-admin"},
		{"la del .env", "OTRA=1\nSYTUE_ID_DEV_SEED_PASSWORD=del-dotenv\n", nil, "del-dotenv"},
		{
			"la del entorno gana al .env", "SYTUE_ID_DEV_SEED_PASSWORD=del-dotenv\n",
			[]string{"SYTUE_ID_DEV_SEED_PASSWORD=del-entorno"},
			"del-entorno",
		},
	}
	for _, c := range casos {
		t.Run(c.nombre, func(t *testing.T) {
			dir := t.TempDir()
			if c.dotenv != "" {
				if err := os.WriteFile(filepath.Join(dir, ".env"), []byte(c.dotenv), 0o600); err != nil {
					t.Fatal(err)
				}
			}
			out := hacer(t, dir, c.entorno, "dev-seed")
			quiere := "compose exec -T -e SYTUE_ID_DEV_SEED_PASSWORD=" + c.quiere + orden
			if !strings.Contains(out, quiere) {
				t.Fatalf("dev-seed no pasa la contraseña %q al contenedor:\n%s", c.quiere, out)
			}
		})
	}
}

// dev-up y dev-app llaman a dev-certs en cada arranque: con el certificado ya hecho,
// no se vuelve a llamar a mkcert; sin él (o sin su clave), sí.
func TestDevCertsSoloGeneraSiFalta(t *testing.T) {
	bin := t.TempDir()
	registro := filepath.Join(t.TempDir(), "mkcert.log")
	falso := "#!/bin/sh\necho \"$@\" >> " + registro + "\n"
	if err := os.WriteFile(filepath.Join(bin, "mkcert"), []byte(falso), 0o755); err != nil { //nolint:gosec // G306: el mkcert falso tiene que ser ejecutable.
		t.Fatal(err)
	}
	ruta := []string{}
	if p := os.Getenv("PATH"); p != "" {
		ruta = append(ruta, p)
	}
	path := "PATH=" + strings.Join(append([]string{bin}, ruta...), string(os.PathListSeparator))

	dir := t.TempDir()
	certs := filepath.Join(dir, "traefik", "certs")
	if err := os.MkdirAll(certs, 0o750); err != nil {
		t.Fatal(err)
	}
	llamadas := func() string {
		b, err := os.ReadFile(registro) //nolint:gosec // G304: ruta del propio test.
		if os.IsNotExist(err) {
			return ""
		}
		if err != nil {
			t.Fatal(err)
		}
		return string(b)
	}
	correr := func() {
		t.Helper()
		if _, err := exec.LookPath("make"); err != nil {
			t.Skip("sin make en el PATH")
		}
		cmd := exec.Command("make", "-s", "-C", raiz, "dev-certs", "COMPOSE_DIR="+dir)
		cmd.Env = []string{path, "HOME=" + t.TempDir()}
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("make dev-certs: %v\n%s", err, out)
		}
	}

	// Solo el certificado, sin la clave: falta algo y se genera.
	if err := os.WriteFile(filepath.Join(certs, "sytue-localhost.pem"), []byte("cert"), 0o600); err != nil {
		t.Fatal(err)
	}
	correr()
	if l := llamadas(); !strings.Contains(l, "-install") || !strings.Contains(l, "*.sytue.localhost") {
		t.Fatalf("sin la clave, dev-certs no llamó a mkcert como debía:\n%s", l)
	}

	// Con los dos: no se toca nada.
	if err := os.WriteFile(filepath.Join(certs, "sytue-localhost-key.pem"), []byte("clave"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(registro); err != nil {
		t.Fatal(err)
	}
	correr()
	if l := llamadas(); l != "" {
		t.Fatalf("con el certificado y la clave ya hechos, dev-certs volvió a llamar a mkcert:\n%s", l)
	}
}
