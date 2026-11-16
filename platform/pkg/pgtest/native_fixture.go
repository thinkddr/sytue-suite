// SPDX-License-Identifier: Apache-2.0
package pgtest

import (
	"bytes"
	"context"
	"errors"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"
)

// NativeFixture keeps native SDK/keyring dependencies out of the service module.
// Env contains synthetic routing only; database and provider credentials are not
// inherited. Child output is bounded and never printed, including on failure.
type NativeFixture struct {
	Module, Package, Test, Marker string
	Timeout                       time.Duration
	Env                           map[string]string
}

var nativeFixtureTest = regexp.MustCompile(`^Test[A-Za-z0-9_]+$`)

func (f NativeFixture) validate() error {
	if f.Module == "" || !f.validPackage() || !nativeFixtureTest.MatchString(f.Test) || f.Marker == "" || len(f.Marker) > 128 || f.Timeout <= 0 || f.Timeout > 15*time.Minute {
		return errors.New("invalid native fixture")
	}
	for key, value := range f.Env {
		if err := validateNativeFixtureEnv(key, value); err != nil {
			return err
		}
	}
	if f.Timeout > 10*time.Minute && !f.mailWebComposite() {
		return errors.New("extended timeout requires the exact Mail web composite fixture")
	}
	return nil
}

// mailWebComposite permits the measured native-plus-browser proof only. Other
// native fixtures retain their ten-minute ceiling, even when a flag is inherited.
func (f NativeFixture) mailWebComposite() bool {
	exactCase := (f.Test == "TestNativeMailApplicationAgainstAuthorityQueueAndSynapse" && f.Marker == "NATIVE_MAIL_AUTHORITY_QUEUE_SDK_MIME_PASS" && f.Env["SYTUE_MAIL_WEB_PROOF_PARENT"] == "TestPrivateIngressLiveNativeMailSharedOutbound") ||
		(f.Test == "TestNativeMailBrowserFullSourceAgainstAuthorityAndSynapse" && f.Marker == "NATIVE_MAIL_BROWSER_FULL_SOURCE_MIME_RELOAD_RETAINED_ACK_PASS" && f.Env["SYTUE_MAIL_WEB_PROOF_PARENT"] == "TestPrivateIngressLiveNativeMailBrowserFullSource")
	return exactCase && filepath.Clean(f.Module) == "../../../../apps/fyne" &&
		f.Package == "./cifrado" &&
		f.Env["SYTUE_MAIL_WEB_PROOF"] == "1" &&
		f.Env["SYTUE_INGRESS_DEVICE_PROOF"] == "1" &&
		f.Env["SYTUE_INGRESS_HANDOFF_CASE"] == "native-mail-sql" &&
		f.Env["SYTUE_MAIL_SHARED_OUTBOUND_PROOF"] == "1"
}

func (f NativeFixture) validPackage() bool {
	switch f.Package {
	case "./api", "./matrix", "./ingress":
		return true
	case "./cifrado", "./cmd/notes":
		if filepath.Clean(f.Module) != "../../../../apps/fyne" {
			return false
		}
		return f.Package == "./cifrado" || f.Test == "TestPrivateNotepadsApplicationAgainstPostgresAndFreshDevice"
	default:
		return false
	}
}

func validateNativeFixtureEnv(key, value string) error {
	if !strings.HasPrefix(key, "SYTUE_") || strings.ContainsAny(key, "=\x00\r\n") || strings.ContainsRune(value, '\x00') || len(value) > 128<<10 {
		return errors.New("invalid native fixture environment")
	}
	for _, secret := range []string{"PASSWORD", "SECRET", "TOKEN", "DSN"} {
		if strings.Contains(key, secret) {
			return errors.New("credential in native fixture environment")
		}
	}
	if !strings.HasSuffix(key, "_URL") {
		return nil
	}
	u, err := url.Parse(value)
	if err != nil || u.Scheme != "http" || u.User != nil || u.Hostname() != "127.0.0.1" && u.Hostname() != "localhost" || u.Port() == "" || u.RawQuery != "" || u.Fragment != "" {
		return errors.New("native fixture requires a disposable loopback endpoint")
	}
	return nil
}

func nativeFixtureEnvironment(current []string, fixture map[string]string) []string {
	toolchain := []string{"PATH", "HOME", "TMPDIR", "GOTOOLCHAIN", "GOMAXPROCS", "GOMEMLIMIT", "GOMODCACHE", "GOCACHE", "GOPROXY", "GOSUMDB", "GONOSUMDB", "GONOPROXY", "GOPRIVATE", "GOFLAGS", "CGO_ENABLED", "CC", "CXX", "SSL_CERT_FILE", "SSL_CERT_DIR", "HTTP_PROXY", "HTTPS_PROXY", "NO_PROXY"}
	env := make([]string, 0, len(toolchain)+len(fixture))
	for _, entry := range current {
		key, _, ok := strings.Cut(entry, "=")
		if ok && slices.Contains(toolchain, key) {
			env = append(env, entry)
		}
	}
	for key, value := range fixture {
		env = append(env, key+"="+value)
	}
	return env
}

func RunNativeFixture(ctx context.Context, t *testing.T, fixture NativeFixture) {
	t.Helper()
	if err := fixture.validate(); err != nil {
		t.Fatal(err)
	}
	module, err := filepath.Abs(fixture.Module)
	if err != nil {
		t.Fatal(err)
	}
	life, cancel := context.WithTimeout(ctx, fixture.Timeout+30*time.Second)
	defer cancel()
	tags := []string{"goolm"}
	if fixture.Package == "./cifrado" || fixture.Package == "./cmd/notes" {
		tags = append(tags, "ci")
	}
	for _, flag := range strings.Fields(os.Getenv("GOFLAGS")) {
		if value, ok := strings.CutPrefix(flag, "-tags="); ok {
			tags = append(tags, value)
		}
	}
	cmd := exec.CommandContext(life, "go", "test", "-tags="+strings.Join(tags, ","), "-race", "-count=1", "-timeout="+fixture.Timeout.String(), "-v", "-run", "^"+fixture.Test+"$", fixture.Package) //nolint:gosec // G204: no shell; fixed executable, package allowlist and anchored test identifier validated above. The CI toolchain supplies build tags.
	cmd.Dir = module
	cmd.Env = nativeFixtureEnvironment(os.Environ(), fixture.Env)
	var output nativeFixtureLog
	defer func() { clear(output.tail) }()
	cmd.Stdout, cmd.Stderr = &output, &output
	if err := cmd.Run(); err != nil {
		t.Fatalf("native fixture failed: %v; child output suppressed (%d bytes); bounded diagnostic: %s", err, output.total, nativeFixtureDiagnostic(output.tail))
	}
	if !bytes.Contains(output.tail, []byte(fixture.Marker)) {
		t.Fatal("native fixture did not reach its final assertions")
	}
}

func nativeFixtureDiagnostic(tail []byte) string {
	f, err := os.CreateTemp("", "sytue-native-fixture-failed-*.log")
	if err != nil {
		return "unavailable"
	}
	_, writeErr := f.Write(tail)
	closeErr := f.Close()
	if writeErr != nil || closeErr != nil {
		_ = os.Remove(f.Name())
		return "unavailable"
	}
	return f.Name()
}

type nativeFixtureLog struct {
	mu    sync.Mutex
	tail  []byte
	total int
}

func (b *nativeFixtureLog) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.total += len(p)
	const limit = 8192
	if len(p) >= limit {
		b.tail = append(b.tail[:0], p[len(p)-limit:]...)
	} else {
		if len(b.tail)+len(p) > limit {
			b.tail = append(b.tail[:0], b.tail[len(b.tail)+len(p)-limit:]...)
		}
		b.tail = append(b.tail, p...)
	}
	return len(p), nil
}
