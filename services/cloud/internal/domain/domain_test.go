// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package domain

import (
	"errors"
	"strings"
	"testing"
)

func TestValidateRepoURL(t *testing.T) {
	ok := []string{
		"https://github.com/acme/app",
		"https://gitlab.com/acme/app.git",
		"git@github.com:acme/app.git",
		"  https://example.com/repo  ", // se recorta
	}
	bad := []string{
		"",
		"http://github.com/acme/app",          // http en claro
		"file:///etc/passwd",                  // esquema peligroso
		"gopher://evil/x",                     // esquema peligroso
		"https://169.254.169.254/latest/meta", // metadata cloud (link-local)
		"https://127.0.0.1/repo",              // loopback
		"https://localhost/repo",              // loopback por nombre
		"https://10.0.0.5/repo",               // privada RFC1918
		"https://192.168.1.1/repo",            // privada
		"https://172.16.3.4/repo",             // privada
		"https://[::1]/repo",                  // loopback IPv6
		"https://100.64.5.7/repo",             // CGNAT RFC6598 (CIDR de pods del clúster)
		"https://postgres/repo",               // nombre "desnudo" (servicio interno)
		"git@10.0.0.5:acme/app.git",           // SSH a IP privada
		"git@localhost:acme/app.git",          // SSH a loopback
	}
	for _, s := range ok {
		if err := ValidateRepoURL(s); err != nil {
			t.Errorf("ValidateRepoURL(%q) = %v, want nil", s, err)
		}
	}
	for _, s := range bad {
		if err := ValidateRepoURL(s); err == nil {
			t.Errorf("ValidateRepoURL(%q) = nil, want error", s)
		} else if !errors.Is(err, ErrInvalidInput) {
			t.Errorf("ValidateRepoURL(%q) error = %v, want ErrInvalidInput", s, err)
		}
	}
}

func TestValidateWebhookURL(t *testing.T) {
	ok := []string{
		"https://hooks.slack.com/services/T000/B000/xxxx",
		"https://discord.com/api/webhooks/123/abc",
		"http://example.com/hook", // http admitido para genéricos
		"  https://ci.example.com/notify  ",
	}
	bad := []string{
		"",
		"ftp://example.com/x",                  // esquema no http(s)
		"file:///etc/passwd",                   // esquema peligroso
		"https://169.254.169.254/latest/meta",  // metadata cloud (link-local)
		"http://127.0.0.1/hook",                // loopback
		"https://localhost/hook",               // loopback por nombre
		"http://10.0.0.5/hook",                 // privada RFC1918
		"https://192.168.1.1/hook",             // privada
		"https://[::1]/hook",                   // loopback IPv6
		"https://100.64.5.7/hook",              // CGNAT RFC6598 (CIDR de pods del clúster)
		"http://internal.cluster.local/notify", // nombre local del clúster
		"https://postgres/hook",                // nombre "desnudo" (servicio interno)
	}
	for _, s := range ok {
		if err := ValidateWebhookURL(s); err != nil {
			t.Errorf("ValidateWebhookURL(%q) = %v, want nil", s, err)
		}
	}
	for _, s := range bad {
		if err := ValidateWebhookURL(s); err == nil {
			t.Errorf("ValidateWebhookURL(%q) = nil, want error", s)
		} else if !errors.Is(err, ErrInvalidInput) {
			t.Errorf("ValidateWebhookURL(%q) error = %v, want ErrInvalidInput", s, err)
		}
	}
}

func TestValidCommitSHAAndGitRef(t *testing.T) {
	if !ValidCommitSHA("abc1234") || !ValidCommitSHA("0123456789abcdef0123456789abcdef01234567") {
		t.Error("ValidCommitSHA rechazó un SHA válido")
	}
	for _, s := range []string{"", "main", "abc", "zzzzzzz", "abc123; rm -rf"} {
		if ValidCommitSHA(s) {
			t.Errorf("ValidCommitSHA(%q) = true, want false", s)
		}
	}
	if !ValidGitRef("main") || !ValidGitRef("feature/x-1.2") || !ValidGitRef("release/v1.0.0") {
		t.Error("ValidGitRef rechazó un ref válido")
	}
	for _, s := range []string{"", "-x", "a b", "a..b", "a\nb", "../etc"} {
		if ValidGitRef(s) {
			t.Errorf("ValidGitRef(%q) = true, want false", s)
		}
	}
}

func TestSanitizeCommitMessage(t *testing.T) {
	cases := []struct {
		name, in, want string
	}{
		{"asunto simple", "fix: carrito con cantidad negativa", "fix: carrito con cantidad negativa"},
		{"solo la primera línea", "feat: buscador\n\nCierra #42\nCo-authored-by: X", "feat: buscador"},
		{"CRLF", "chore: subir deps\r\ndetalle", "chore: subir deps"},
		{"espacios de sobra", "  fix: typo  ", "fix: typo"},
		{"tabulador a espacio", "fix:\tsangría", "fix: sangría"},
		{"caracteres de control fuera", "fix: \x00\x07color \x1b[31mrojo\x1b[0m", "fix: color [31mrojo[0m"},
		{"vacío", "", ""},
		{"solo saltos", "\n\n", ""},
		{"acentos y emoji intactos", "feat: añade búsqueda 🇪🇺", "feat: añade búsqueda 🇪🇺"},
	}
	for _, c := range cases {
		if got := SanitizeCommitMessage(c.in); got != c.want {
			t.Errorf("%s: SanitizeCommitMessage(%q) = %q, want %q", c.name, c.in, got, c.want)
		}
	}

	// Tope duro: el CHECK de la tabla mide en CARACTERES, así que el recorte tiene
	// que contarlos igual —por runas, no por bytes— o un mensaje de acentos con
	// 200 caracteres pasaría el dominio y lo rechazaría Postgres.
	long := SanitizeCommitMessage(strings.Repeat("á", 500))
	if n := len([]rune(long)); n != MaxCommitMessage {
		t.Errorf("mensaje largo recortado a %d runas, want %d", n, MaxCommitMessage)
	}
	if !strings.HasSuffix(long, "…") {
		t.Error("el recorte no marca que hay más texto")
	}
}

func TestValidSlug(t *testing.T) {
	ok := []string{"app", "mi-app", "a1", "x-y-z", "app123"}
	bad := []string{
		"", "-app", "app-", "App", "mi_app", "a..b", "espacio x",
		"toooooooooooooooooooooooooooooooooooooooolong",
	} // >40
	for _, s := range ok {
		if !ValidSlug(s) {
			t.Errorf("ValidSlug(%q) = false, want true", s)
		}
	}
	for _, s := range bad {
		if ValidSlug(s) {
			t.Errorf("ValidSlug(%q) = true, want false", s)
		}
	}
}

func TestValidHostname(t *testing.T) {
	ok := []string{"sytue.eu", "app.sytue.eu", "a.b.c.example.com", "MI-APP.sytue.eu"}
	bad := []string{"", "localhost", "sin-punto", ".sytue.eu", "sytue.eu.", "-x.sytue.eu", "espacio .eu"}
	for _, h := range ok {
		if !ValidHostname(h) {
			t.Errorf("ValidHostname(%q) = false, want true", h)
		}
	}
	for _, h := range bad {
		if ValidHostname(h) {
			t.Errorf("ValidHostname(%q) = true, want false", h)
		}
	}
}

func TestValidEnvKey(t *testing.T) {
	ok := []string{"PORT", "DATABASE_URL", "_X", "A1_B2"}
	bad := []string{"", "port", "1VAR", "MY-VAR", "MY VAR", "MY.VAR"}
	for _, k := range ok {
		if !ValidEnvKey(k) {
			t.Errorf("ValidEnvKey(%q) = false, want true", k)
		}
	}
	for _, k := range bad {
		if ValidEnvKey(k) {
			t.Errorf("ValidEnvKey(%q) = true, want false", k)
		}
	}
}

func TestEnumValidators(t *testing.T) {
	if !ValidPlan(PlanPro) || ValidPlan(Plan("enterprise")) {
		t.Error("ValidPlan")
	}
	if !ValidFramework(FrameworkAstro) || ValidFramework(Framework("rails")) {
		t.Error("ValidFramework")
	}
	if !ValidTarget(TargetPreview) || ValidTarget(DeployTarget("staging")) {
		t.Error("ValidTarget")
	}
	if !ValidScope(ScopeProduction) || ValidScope(EnvScope("dev")) {
		t.Error("ValidScope")
	}
	if !ValidServiceKind(ServicePostgres) || ValidServiceKind(ServiceKind("kafka")) {
		t.Error("ValidServiceKind")
	}
}

func TestBuildStatusIsTerminal(t *testing.T) {
	terminal := []BuildStatus{BuildSucceeded, BuildFailed, BuildCanceled}
	nonTerminal := []BuildStatus{BuildQueued, BuildRunning}
	for _, s := range terminal {
		if !s.IsTerminal() {
			t.Errorf("%s debería ser terminal", s)
		}
	}
	for _, s := range nonTerminal {
		if s.IsTerminal() {
			t.Errorf("%s no debería ser terminal", s)
		}
	}
}

func TestDeployStatusIsActive(t *testing.T) {
	if !DeployReady.IsActive() {
		t.Error("DeployReady debe ser activo")
	}
	for _, s := range []DeployStatus{DeployQueued, DeployBuilding, DeployError, DeployRetired} {
		if s.IsActive() {
			t.Errorf("%s no debe ser activo", s)
		}
	}
}

func TestGitEventIsPullRequest(t *testing.T) {
	if (GitEvent{PRNumber: 1}).IsPullRequest() == false {
		t.Error("PR con número debe ser PR")
	}
	if (GitEvent{}).IsPullRequest() {
		t.Error("evento sin PR no debe ser PR")
	}
}

func TestValidCronSchedule(t *testing.T) {
	ok := []string{"*/5 * * * *", "0 3 * * *", "@daily", "@hourly", "@every 1h30m"}
	bad := []string{"", "  ", "* * * *", "* * * * * *", "@nope", "@every "}
	for _, s := range ok {
		if !ValidCronSchedule(s) {
			t.Errorf("%q debe ser válido", s)
		}
	}
	for _, s := range bad {
		if ValidCronSchedule(s) {
			t.Errorf("%q debe rechazarse", s)
		}
	}
}

func TestValidateImageRef(t *testing.T) {
	const reg, slug = "rg.fr-par.scw.cloud/sytue", "web"
	ok := []string{
		"rg.fr-par.scw.cloud/sytue/web@sha256:abc",
		"rg.fr-par.scw.cloud/sytue/web:v1",
		"rg.fr-par.scw.cloud/sytue/web:v1@sha256:abc", // tag + digest
	}
	for _, r := range ok {
		if err := ValidateImageRef(r, reg, slug); err != nil {
			t.Errorf("%q debe aceptarse: %v", r, err)
		}
	}
	bad := []string{
		"",                                    // vacío
		"rg.fr-par.scw.cloud/sytue/web",       // sin tag ni digest
		"docker.io/library/nginx:latest",      // registry externo/no-UE
		"nginx:latest",                        // docker.io implícito
		"gcr.io/proj/app@sha256:abc",          // no-UE
		"rg.fr-par.scw.cloud/sytue/other:v1",  // AISLAMIENTO: repo de OTRO tenant, mismo host
		"rg.fr-par.scw.cloud/sytue/websfx:v1", // no basta con prefijo, repo exacto
		"rg.fr-par.scw.cloud/other/web:v1",    // otro namespace bajo el mismo host
	}
	for _, r := range bad {
		if err := ValidateImageRef(r, reg, slug); !errors.Is(err, ErrInvalidInput) {
			t.Errorf("%q debe rechazarse con ErrInvalidInput, got %v", r, err)
		}
	}
	// Registry de plataforma o slug sin configurar ⇒ fail-closed.
	if err := ValidateImageRef("rg.fr-par.scw.cloud/sytue/web:v1", "", slug); !errors.Is(err, ErrInvalidInput) {
		t.Errorf("registry vacío debe fallar cerrado, got %v", err)
	}
	if err := ValidateImageRef("rg.fr-par.scw.cloud/sytue/web:v1", reg, ""); !errors.Is(err, ErrInvalidInput) {
		t.Errorf("slug vacío debe fallar cerrado, got %v", err)
	}
}

// ---------------------------------------------------------------------------
// Puerto de escucha y ruta de health check por proyecto
// ---------------------------------------------------------------------------

func TestEffectiveAppPortYHealthPath(t *testing.T) {
	// Sin configurar: exactamente el comportamiento anterior a la migración 0019.
	var p Project
	if got := p.EffectiveAppPort(); got != DefaultAppPort {
		t.Fatalf("puerto por defecto = %d, quiero %d", got, DefaultAppPort)
	}
	if got := p.EffectiveHealthPath(); got != DefaultHealthPath {
		t.Fatalf("ruta por defecto = %q, quiero %q", got, DefaultHealthPath)
	}
	// Configurado: manda el del proyecto.
	p.AppPort, p.HealthPath = 3000, "/healthz"
	if got := p.EffectiveAppPort(); got != 3000 {
		t.Fatalf("puerto = %d, quiero 3000", got)
	}
	if got := p.EffectiveHealthPath(); got != "/healthz" {
		t.Fatalf("ruta = %q, quiero /healthz", got)
	}
	// Valores imposibles guardados por una ruta que no validó: se cae al default en
	// vez de generar un Deployment que nunca pasaría a Ready.
	for _, bad := range []int{-1, 0, 70000} {
		p.AppPort = bad
		if got := p.EffectiveAppPort(); got != DefaultAppPort {
			t.Fatalf("puerto inválido %d → %d, quiero el default", bad, got)
		}
	}
	p.HealthPath = "healthz" // sin barra inicial
	if got := p.EffectiveHealthPath(); got != DefaultHealthPath {
		t.Fatalf("ruta relativa → %q, quiero el default", got)
	}
}

func TestValidateAppPort(t *testing.T) {
	for _, ok := range []int{0, 1, 80, 3000, 8080, 65535} {
		if err := ValidateAppPort(ok); err != nil {
			t.Fatalf("puerto %d debería valer: %v", ok, err)
		}
	}
	for _, bad := range []int{-1, 65536, 100000} {
		if err := ValidateAppPort(bad); err == nil {
			t.Fatalf("puerto %d debería fallar", bad)
		}
	}
}

func TestValidateHealthPath(t *testing.T) {
	for _, ok := range []string{"", "/", "/healthz", "/api/health?ready=1"} {
		if err := ValidateHealthPath(ok); err != nil {
			t.Fatalf("ruta %q debería valer: %v", ok, err)
		}
	}
	for _, bad := range []string{"healthz", "http://x/health", "/con espacio", "/" + strings.Repeat("a", 600)} {
		if err := ValidateHealthPath(bad); err == nil {
			t.Fatalf("ruta %q debería fallar", bad)
		}
	}
}
