// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package cicontratos

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"testing"

	"git.sytue.eu/sytue/one/tools/guiontest"
)

// LA FIRMA DE LO QUE SE PUBLICA (auditoría 2026-10-06: CI-01, CI-05, INF-03, NAT-03).
//
// Prueba scripts/firma-publicacion.sh con una clave ECDSA P-256 hecha aquí mismo:
// que verifica lo bueno y rechaza lo tocado, que el cuerpo de la marca se
// reconstruye igual con o sin líneas vacías, que un SHA256SUMS de otra versión no
// vale, y los tres modos de `firmar` —sin clave pública avisa y sigue; con pública
// y sin privada es rojo; y lo firmado se comprueba antes de darlo por bueno—.
// cosign se sustituye por un doble que firma con openssl y escribe el mismo bundle:
// el formato real se comprobó contra cosign 3.1.3 (ver la cabecera del guion).

const firmaPublicacionGuion = "scripts/firma-publicacion.sh"

// firmaPublicacionBanco es el directorio de una prueba, con dos pares de claves:
// buena.pem/buena.pub y otra.pem/otra.pub.
type firmaPublicacionBanco struct{ tmp string }

func firmaPublicacionMontar(t *testing.T) firmaPublicacionBanco {
	t.Helper()
	if _, err := os.Stat(filepath.Join(guiontest.Raiz(t), firmaPublicacionGuion)); err != nil {
		t.Fatal("no existe scripts/firma-publicacion.sh")
	}
	// El original no se saltaba sin ellas: fallaba.
	for _, h := range []string{"openssl", "jq", "base64"} {
		if _, err := exec.LookPath(h); err != nil {
			t.Fatalf("falta %s para probar la firma", h)
		}
	}
	b := firmaPublicacionBanco{tmp: t.TempDir()}
	for _, nombre := range []string{"buena", "otra"} {
		r := guiontest.Bash(t, guiontest.Opciones{Dir: b.tmp}, `set -e
openssl ecparam -name prime256v1 -genkey -noout -out "$1.pem" 2>/dev/null
openssl ec -in "$1.pem" -pubout -out "$1.pub" 2>/dev/null`, nombre)
		if r.Codigo != 0 {
			t.Fatalf("no se pudo crear la clave %s: %s", nombre, r.Todo())
		}
	}
	return b
}

func (b firmaPublicacionBanco) ruta(nombre string) string { return filepath.Join(b.tmp, nombre) }

// escribir deja un fichero en el banco y devuelve su ruta.
func (b firmaPublicacionBanco) escribir(t *testing.T, nombre, contenido string) string {
	t.Helper()
	guiontest.Escribir(t, b.ruta(nombre), contenido)
	return b.ruta(nombre)
}

// firmaDe firma un fichero con la clave privada de ese nombre: base64 en una línea.
func (b firmaPublicacionBanco) firmaDe(t *testing.T, clave, fichero string) string {
	t.Helper()
	r := guiontest.Bash(t, guiontest.Opciones{Dir: b.tmp},
		`set -o pipefail; openssl dgst -sha256 -sign "$1.pem" "$2" | base64 | tr -d '\n'`, clave, fichero)
	if r.Codigo != 0 || r.Salida == "" {
		t.Fatalf("no se pudo firmar %s con %s: %s", fichero, clave, r.Todo())
	}
	return r.Salida
}

// firmaPublicacionEspera es el `espera <0|1>` del original: la orden tiene que
// salir bien (true) o mal (false).
func firmaPublicacionEspera(t *testing.T, bien bool, que string, r guiontest.Resultado) {
	t.Helper()
	t.Run(que, func(t *testing.T) {
		if (r.Codigo == 0) != bien {
			t.Fatalf("%s (salió con %d, se esperaba bien=%v)\n%s", que, r.Codigo, bien, r.Todo())
		}
	})
}

func firmaPublicacionLanzar(t *testing.T, entrada string, args ...string) guiontest.Resultado {
	t.Helper()
	return guiontest.Guion(t, guiontest.Opciones{Entrada: entrada}, firmaPublicacionGuion, args...)
}

// --- 1. verificar ---------------------------------------------------------------.
func TestFirmaPublicacionVerificar(t *testing.T) {
	b := firmaPublicacionMontar(t)
	f := b.escribir(t, "f", "lo que se publica\n")
	firma := b.firmaDe(t, "buena", f)
	sig := b.escribir(t, "f.sig", firma)
	buena, otra := b.ruta("buena.pub"), b.ruta("otra.pub")

	firmaPublicacionEspera(t, true, "una firma buena verifica, dada en fichero",
		firmaPublicacionLanzar(t, "", "verificar", buena, f, sig))
	firmaPublicacionEspera(t, true, "y dada como texto",
		firmaPublicacionLanzar(t, "", "verificar", buena, f, firma))
	firmaPublicacionEspera(t, false, "con otra clave pública no verifica",
		firmaPublicacionLanzar(t, "", "verificar", otra, f, sig))
	tocado := b.escribir(t, "tocado", "lo que se publica.\n")
	firmaPublicacionEspera(t, false, "un byte cambiado no verifica",
		firmaPublicacionLanzar(t, "", "verificar", buena, tocado, sig))
	firmaPublicacionEspera(t, false, "una firma que no es base64 no verifica",
		firmaPublicacionLanzar(t, "", "verificar", buena, f, "esto no es una firma"))
	firmaPublicacionEspera(t, false, "sin clave pública no se da nada por bueno",
		firmaPublicacionLanzar(t, "", "verificar", b.ruta("no-existe.pub"), f, sig))
}

const firmaPublicacionCuerpo = `sytue-desplegable 1
version: v9.9.9
perfil: completo
commit: 0123456789abcdef0123456789abcdef01234567
registro: ghcr.io/thinkddr
sha256:1111111111111111111111111111111111111111111111111111111111111111  sytue-id:v9.9.9
sha256:2222222222222222222222222222222222222222222222222222222222222222  sytue-portal:v9.9.9
`

// --- 2. la marca ----------------------------------------------------------------.
func TestFirmaPublicacionLaMarca(t *testing.T) {
	b := firmaPublicacionMontar(t)
	buena, otra := b.ruta("buena.pub"), b.ruta("otra.pub")
	cuerpo := b.escribir(t, "cuerpo", firmaPublicacionCuerpo)
	marca := firmaPublicacionCuerpo + "firma: " + b.firmaDe(t, "buena", cuerpo) + "\n"
	sinSalto := strings.TrimRight(firmaPublicacionCuerpo, "\n")

	t.Run("una marca firmada verifica y devuelve su cuerpo", func(t *testing.T) {
		r := firmaPublicacionLanzar(t, marca, "verificar-marca", buena)
		if r.Codigo != 0 || r.Texto() != sinSalto {
			t.Fatalf("la marca firmada no verificó o no devolvió su cuerpo (código %d): %q\n%s", r.Codigo, r.Salida, r.Error)
		}
	})
	// Como la deja git: con una línea vacía tras el asunto, otra al final y CRLF.
	lineas := strings.Split(strings.TrimSuffix(marca, "\n"), "\n")
	deGit := slices.Concat([]string{lineas[0], ""}, lineas[1:], []string{""})
	marcaGit := strings.Join(deGit, "\r\n") + "\r\n"
	firmaPublicacionEspera(t, true, "con líneas vacías y CRLF de por medio verifica igual",
		firmaPublicacionLanzar(t, marcaGit, "verificar-marca", buena))
	if !strings.Contains(marca, "sha256:2222") {
		t.Fatal("la marca de la prueba ya no lleva el digest que se cambia")
	}
	firmaPublicacionEspera(t, false, "con un digest cambiado no verifica",
		firmaPublicacionLanzar(t, strings.ReplaceAll(marca, "sha256:2222", "sha256:3333"), "verificar-marca", buena))
	firmaPublicacionEspera(t, false, "sin línea de firma no verifica",
		firmaPublicacionLanzar(t, firmaPublicacionCuerpo, "verificar-marca", buena))
	firmaPublicacionEspera(t, false, "firmada con otra clave no verifica",
		firmaPublicacionLanzar(t, marca, "verificar-marca", otra))
	x := b.escribir(t, "x", "otra cosa\n")
	noEsMarca := "otra cosa\nfirma: " + b.firmaDe(t, "buena", x) + "\n"
	firmaPublicacionEspera(t, false, "algo bien firmado que no es un manifiesto de marca no vale",
		firmaPublicacionLanzar(t, noEsMarca, "verificar-marca", buena))
	t.Run("cuerpo-marca reconstruye el mismo cuerpo que se firmó", func(t *testing.T) {
		if r := firmaPublicacionLanzar(t, marcaGit, "cuerpo-marca"); r.Texto() != sinSalto {
			t.Fatalf("cuerpo-marca no reconstruye el cuerpo firmado: %q", r.Salida)
		}
	})
}

// --- 3. las sumas ---------------------------------------------------------------.
func TestFirmaPublicacionLasSumas(t *testing.T) {
	b := firmaPublicacionMontar(t)
	buena := b.ruta("buena.pub")
	const contenido = `# sytue-instaladores 1
# version: v9.9.9
# commit: 0123456789abcdef0123456789abcdef01234567
1111111111111111111111111111111111111111111111111111111111111111  sytue-one-linux-amd64.tar.xz
`
	sumas := b.escribir(t, "SHA256SUMS", contenido)
	sig := b.escribir(t, "SHA256SUMS.sig", b.firmaDe(t, "buena", sumas))
	firmaPublicacionEspera(t, true, "un SHA256SUMS firmado y de esa versión verifica",
		firmaPublicacionLanzar(t, "", "verificar-sumas", buena, sumas, sig, "v9.9.9"))
	firmaPublicacionEspera(t, false, "bien firmado pero de OTRA versión no vale",
		firmaPublicacionLanzar(t, "", "verificar-sumas", buena, sumas, sig, "v9.9.10"))
	tocadas := b.escribir(t, "sumas-tocadas", strings.Replace(contenido, "\n1111", "\n2222", 1))
	if guiontest.Leer(t, tocadas) == contenido {
		t.Fatal("la suma de la prueba no se cambió")
	}
	firmaPublicacionEspera(t, false, "con una suma cambiada no verifica",
		firmaPublicacionLanzar(t, "", "verificar-sumas", buena, tocadas, sig, "v9.9.9"))
}

// Doble de `cosign sign-blob`: firma con openssl y escribe el bundle con el campo
// que lee el guion. La clave «privada» que recibe es la ruta de un .pem de la prueba.
const firmaPublicacionCosign = `bundle=""; fichero=""
while [ $# -gt 0 ]; do
  case "$1" in --bundle) bundle="$2"; shift ;; --key | --tlog-upload=* | --use-signing-config=* | --yes | sign-blob) ;; *) fichero="$1" ;; esac
  shift
done
firma="$(openssl dgst -sha256 -sign "$COSIGN_PRIVATE_KEY" "$fichero" | base64 | tr -d '\n')" || exit 1
printf '{"mediaType":"application/vnd.dev.sigstore.bundle.v0.3+json","messageSignature":{"signature":"%s"}}' "$firma" >"$bundle"`

// --- 4. firmar: los tres modos --------------------------------------------------.
func TestFirmaPublicacionFirmar(t *testing.T) {
	b := firmaPublicacionMontar(t)
	f := b.escribir(t, "f", "lo que se publica\n")
	sig := b.ruta("f.sig")
	bin := b.ruta("bin")
	guiontest.Doble(t, bin, "cosign", firmaPublicacionCosign)
	// firmar <pública o ruta inexistente> <privada o vacío>.
	firmar := func(t *testing.T, publica, privada string) guiontest.Resultado {
		t.Helper()
		if err := os.RemoveAll(sig); err != nil {
			t.Fatal(err)
		}
		return guiontest.Guion(t, guiontest.Opciones{Env: []string{
			guiontest.ConPATH(bin), "SYTUE_FIRMA_PUBLICA=" + publica, "SYTUE_FIRMA_CLAVE=" + privada, "SYTUE_FIRMA_PASSWORD=",
		}}, firmaPublicacionGuion, "firmar", f)
	}
	hayFirma := func() bool {
		_, err := os.Lstat(sig)
		return err == nil
	}
	buena := b.ruta("buena.pub")

	t.Run("sin clave pública en el repositorio: avisa, no firma y NO rompe la publicación", func(t *testing.T) {
		r := firmar(t, b.ruta("no-existe.pub"), "")
		if r.Codigo != 0 || hayFirma() || !strings.Contains(r.Todo(), "::warning") {
			t.Fatalf("sin clave pública tenía que avisar y salir bien sin firmar: %s", r.Todo())
		}
	})
	t.Run("con clave pública y sin la privada: rojo, porque lo publicado no verificaría", func(t *testing.T) {
		if r := firmar(t, buena, ""); r.Codigo == 0 || hayFirma() {
			t.Fatalf("con clave pública y sin privada tenía que fallar: %s", r.Todo())
		}
	})
	t.Run("con las dos: firma, y lo firmado verifica con la pública", func(t *testing.T) {
		r := firmar(t, buena, b.ruta("buena.pem"))
		if r.Codigo != 0 {
			t.Fatalf("con las dos claves no firmó: %s", r.Todo())
		}
		if v := firmaPublicacionLanzar(t, "", "verificar", buena, f, sig); v.Codigo != 0 {
			t.Fatalf("con las dos claves no firmó algo que verifique: %s\n%s", r.Todo(), v.Todo())
		}
	})
	t.Run("si la clave del entorno no es la pareja de la del repositorio: rojo, y no deja firma", func(t *testing.T) {
		if r := firmar(t, buena, b.ruta("otra.pem")); r.Codigo == 0 || hayFirma() {
			t.Fatalf("firmó con una clave que no es la del repositorio y lo dio por bueno: %s", r.Todo())
		}
	})
}

// --- 4b. El SHA256SUMS de los instaladores -----------------------------------------.
func TestFirmaPublicacionElSHA256SUMSDeLosInstaladores(t *testing.T) {
	b := firmaPublicacionMontar(t)
	buena := b.ruta("buena.pub")
	inst := b.ruta("inst")
	for nombre, contenido := range map[string]string{
		"sytue-one-linux-amd64.tar.xz":          "one",
		"sytue-drive-macos-universal.dmg":       "drive",
		"sytue-one-windows-amd64-installer.exe": "win",
		"sytue-one-android.aab":                 "no",
		"SHA256SUMS.sig":                        "no",
	} {
		guiontest.Escribir(t, filepath.Join(inst, nombre), contenido)
	}
	// Un directorio sin instaladores: el del doble de cosign en el original.
	sinInstaladores := b.ruta("bin")
	guiontest.Doble(t, sinInstaladores, "cosign", firmaPublicacionCosign)
	const commit = "0123456789abcdef0123456789abcdef01234567"

	r := firmaPublicacionLanzar(t, "", "sumas", "v9.9.9", commit, inst)
	sums := b.escribir(t, "SUMS", r.Salida)
	t.Run("sumas: cabecera, solo instaladores, por nombre, y sha256sum -c lo acepta tal cual", func(t *testing.T) {
		lineas := strings.Split(strings.TrimRight(r.Salida, "\n"), "\n")
		if r.Codigo != 0 || len(lineas) < 3 {
			t.Fatalf("sumas no escribe el SHA256SUMS del formato (código %d): %s", r.Codigo, r.Todo())
		}
		if cabecera := strings.Join(lineas[:3], "\n"); cabecera != "# sytue-instaladores 1\n# version: v9.9.9\n# commit: "+commit {
			t.Fatalf("sumas no escribe la cabecera del formato: %s", r.Salida)
		}
		var nombres []string
		for _, linea := range lineas[3:] {
			if campos := strings.Fields(linea); len(campos) > 1 {
				nombres = append(nombres, campos[1])
			} else {
				nombres = append(nombres, "")
			}
		}
		quiero := []string{"sytue-drive-macos-universal.dmg", "sytue-one-linux-amd64.tar.xz", "sytue-one-windows-amd64-installer.exe"}
		if !slices.Equal(nombres, quiero) {
			t.Fatalf("sumas no lista solo los instaladores, por nombre: %q", nombres)
		}
		if c := guiontest.Ejecutar(t, guiontest.Opciones{Dir: inst}, "sha256sum", "-c", "--quiet", sums); c.Codigo != 0 {
			t.Fatalf("sha256sum -c no acepta lo que escribe sumas: %s\n%s", c.Todo(), r.Salida)
		}
	})
	sig := b.escribir(t, "SUMS.sig", b.firmaDe(t, "buena", sums))
	firmaPublicacionEspera(t, true, "y lo que escribe, firmado, pasa verificar-sumas",
		firmaPublicacionLanzar(t, "", "verificar-sumas", buena, sums, sig, "v9.9.9"))
	firmaPublicacionEspera(t, false, "sumas no acepta una versión que no sea vX.Y.Z",
		firmaPublicacionLanzar(t, "", "sumas", "main", commit, inst))
	firmaPublicacionEspera(t, false, "ni un directorio sin instaladores",
		firmaPublicacionLanzar(t, "", "sumas", "v9.9.9", commit, sinInstaladores))
}

// --- 5. Quién ve la clave y quién escribe en el repositorio ----------------------
//
// El entorno protegido solo protege si el job que lo usa es mínimo. Se fija que en
// cada flujo que publica hay UN job con el entorno, que es el único con
// `contents: write`, que no monta la caché de Go ni el socket de Docker, y que la
// clave solo entra en el paso que firma.

var firmaPublicacionEntorno = regexp.MustCompile(`needs\.[a-z-]+\.outputs\.firma == .si. && .publicacion. \|\| ..`)

func firmaPublicacionEscribe(job any) bool {
	return mapa(mapa(job)["permissions"])["contents"] == "write"
}

func firmaPublicacionLlamaAFirmar(paso map[string]any) bool {
	run, _ := paso["run"].(string)
	return strings.Contains(run, "firma-publicacion.sh firmar")
}

// firmaPublicacionAjenos mira los demás jobs: ninguno usa un entorno ni escribe
// (salvo los que adjuntan), y la clave no llega a un paso que no firma.
func firmaPublicacionAjenos(jobs map[string]any, nombre string, adjuntan []string) []string {
	var malas []string
	for _, n := range claves(jobs) {
		j := mapa(jobs[n])
		if entorno := j["environment"]; n != nombre && entorno != nil && entorno != "" {
			malas = append(malas, fmt.Sprintf("%s usa un entorno: solo puede %s", n, nombre))
		}
		if n != nombre && !slices.Contains(adjuntan, n) && firmaPublicacionEscribe(j) {
			malas = append(malas, fmt.Sprintf("%s lleva contents: write: solo puede %s", n, nombre))
		}
		for _, p := range pasos(j) {
			env, err := json.Marshal(mapa(p["env"]))
			if err != nil {
				malas = append(malas, fmt.Sprintf("%s: el entorno de «%v» no se puede leer: %v", n, p["name"], err))
				continue
			}
			if strings.Contains(string(env), "SYTUE_FIRMA_") && (n != nombre || !firmaPublicacionLlamaAFirmar(p)) {
				malas = append(malas, fmt.Sprintf("%s: la clave llega a un paso que no firma («%v»)", n, p["name"]))
			}
		}
	}
	return malas
}

// firmaPublicacionHallazgos devuelve lo que incumple el flujo, o nada.
// adjuntan: los jobs que además adjuntan a la release y por eso escriben.
func firmaPublicacionHallazgos(f flujo, nombre string, adjuntan ...string) []string {
	jobs := mapa(f["jobs"])
	job := mapa(jobs[nombre])
	if job == nil {
		return []string{"no existe el job " + nombre}
	}
	malas := firmaPublicacionAjenos(jobs, nombre, adjuntan)
	if firmaPublicacionEscribe(f) {
		malas = append(malas, "el flujo entero lleva contents: write")
	}
	if !firmaPublicacionEscribe(job) {
		malas = append(malas, nombre+" no puede escribir en el repositorio")
	}
	if entorno, _ := job["environment"].(string); !firmaPublicacionEntorno.MatchString(strings.TrimSpace(entorno)) {
		malas = append(malas, fmt.Sprintf("%s no usa el entorno «publicacion» solo cuando hay clave pública: %v", nombre, job["environment"]))
	}
	if volumenes, _ := mapa(job["container"])["volumes"].([]any); len(volumenes) > 0 {
		malas = append(malas, fmt.Sprintf("%s monta volúmenes del host: %v", nombre, volumenes))
	}
	if !slices.ContainsFunc(pasos(job), firmaPublicacionLlamaAFirmar) {
		malas = append(malas, nombre+" no llama a firma-publicacion.sh firmar")
	}
	return malas
}

func TestFirmaPublicacionQuienVeLaClaveYQuienEscribe(t *testing.T) {
	flujos := leerFlujos(t)
	// release-desktop: los turnos de escritorio y el de macOS adjuntan sus instaladores (la
	// release es donde viven, y el almacén de artefactos no da para llevarlos a otro job).
	// Desde el 2026-11-05 también `tiendas-codemagic`, el mismo paso por ancla.
	// Desde el 2026-11-03 `tiendas` también escribe: adjunta la marca TESTFLIGHT.txt de la
	// subida a TestFlight (el despacho normal quedó reducido a las tiendas, decisión del
	// dueño; sin la marca, la cadena re-sometería a Apple lo ya publicado).
	for _, c := range []struct {
		fichero, job string
		adjuntan     []string
	}{
		{"imagenes-produccion.yml", "marcar", nil},
		{"release-desktop.yml", "sumas", []string{"escritorio", "macos", "tiendas", "tiendas-codemagic"}},
	} {
		t.Run(c.fichero+": solo «"+c.job+"» ve la clave y escribe en el repositorio, y no monta nada del host", func(t *testing.T) {
			f, ok := flujos[c.fichero]
			if !ok {
				t.Fatalf("no existe .github/workflows/%s", c.fichero)
			}
			if hallazgos := firmaPublicacionHallazgos(f, c.job, c.adjuntan...); len(hallazgos) > 0 {
				t.Fatalf("%s: el job que firma no es el único ni es mínimo:\n      %s", c.fichero, strings.Join(hallazgos, "\n      "))
			}
		})
	}
}
