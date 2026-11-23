// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package pruebasfyne

import (
	"strings"
	"testing"
)

// Traducción de apps/fyne/scripts/apple_firma_sonda-test.py: regresiones de la
// sonda de firma sin claves, sin acceso al llavero y sin firmar de verdad. El
// doble `Herramientas` sustituye en proceso a codesign/security/openssl, como
// en el original; la sonda (apple_firma_sonda.py) es la real.

const appleSondaCarga = `
import importlib.util, ssl, subprocess, tempfile
SPEC = importlib.util.spec_from_file_location("sonda", pathlib.Path("apple_firma_sonda.py").resolve())
s = importlib.util.module_from_spec(SPEC)
SPEC.loader.exec_module(s)
NOMBRE, LLAVERO = "Developer ID Application: Sytue", "/tmp/probe.keychain-db"
SELECTOR, OTRO_SELECTOR = "A" * 40, "B" * 40
DER, OTRO_DER = b"expected public certificate", b"another public certificate"

def exportado(der=DER, selector=SELECTOR):
    return (f"SHA-256 hash: {s.huella(der)}\nSHA-1 hash: {selector}\n"
            + ssl.DER_cert_to_PEM_cert(der))

class Herramientas:
    def __init__(self):
        self.comandos = []
        self.certificados = exportado()
        self.globales = False
        self.hojas = {}
        self.verificar = self.extraer = self.firmar = self.exportar = self.identidades = 0
        self.sin_hoja = False
        self.firmas = 0
        self.firmas_fallidas = set()

    def __call__(self, argv):
        self.comandos.append(argv)
        status, output = 0, ""
        if argv[:2] == ["codesign", "-f"]:
            self.firmas += 1
            status = self.firmar or int(self.firmas in self.firmas_fallidas)
        elif argv[:2] == ["codesign", "--verify"]:
            status = self.verificar
        elif argv[:2] == ["codesign", "-d"]:
            status = self.extraer
            prefijo = argv[2].split("=", 1)[1]
            indice = int(pathlib.Path(argv[-1]).name.split("-")[-1])
            if not self.sin_hoja:
                pathlib.Path(prefijo + "0").write_bytes(self.hojas.get(indice, DER))
            # Un certificado esperado en el índice 1 no puede sustituir a una hoja ausente.
            pathlib.Path(prefijo + "1").write_bytes(DER)
        elif argv[:2] == ["security", "find-identity"]:
            status = self.identidades
            output = f' 1) {SELECTOR} "{NOMBRE}"\n'
            if self.globales and argv[-1] != LLAVERO:
                output += f' 2) {OTRO_SELECTOR} "{NOMBRE}"\n'
        elif argv[:2] == ["security", "find-certificate"]:
            if argv[-1] == LLAVERO:
                output, status = self.certificados, self.exportar
            elif argv[-1] == NOMBRE:
                output = exportado(OTRO_DER, OTRO_SELECTOR)
            else:
                assert argv[-1] == "/System/Library/Keychains/SystemRootCertificates.keychain"
        elif argv[0] == "openssl" or argv[:2] in (
                ["security", "list-keychains"], ["security", "show-keychain-info"],
                ["security", "verify-cert"]):
            pass
        else:
            raise AssertionError("unexpected command: " + repr(argv))
        return subprocess.CompletedProcess(argv, status, output, "")

R["huella_der"] = s.huella(DER)
`

func TestAppleSondaLosCertificadosExportadosSeIndexanPorSHA256(t *testing.T) {
	R := applePy(t, appleSondaCarga+`
with tempfile.TemporaryDirectory() as carpeta:
    archivos, metadatos = s.certificados_publicos(exportado(), pathlib.Path(carpeta), "cert", Herramientas())
    R["claves"] = sorted(archivos)
    R["der_igual"] = ssl.PEM_cert_to_DER_cert(archivos[s.huella(DER)].read_text()) == DER
R["sha1"] = metadatos[0]["sha1"]
R["por_selector"] = s.certificados_por_selector(metadatos)
`)
	huella, _ := R["huella_der"].(string)
	appleIgual(t, "un solo fichero, por su SHA-256", R["claves"], `["`+huella+`"]`)
	appleIgual(t, "el PEM guardado es el DER exportado", R["der_igual"], `true`)
	appleIgual(t, "el selector SHA-1 queda en los metadatos", R["sha1"], `"`+strings.Repeat("A", 40)+`"`)
	appleIgual(t, "selector → huella", R["por_selector"], `{"`+strings.Repeat("A", 40)+`": "`+huella+`"}`)
}

func TestAppleSondaExportacionSinHuellaOConHuellaDistintaFallaCerrada(t *testing.T) {
	R := applePy(t, appleSondaCarga+`
R["casos"] = []
for texto in [ssl.DER_cert_to_PEM_cert(DER), exportado().replace(s.huella(DER), "0" * 64)]:
    with tempfile.TemporaryDirectory() as carpeta:
        R["casos"].append(atrapar(s.certificados_publicos, texto, pathlib.Path(carpeta), "cert", Herramientas()))
`)
	for i, nombre := range []string{"PEM sin huellas", "huella SHA-256 alterada"} {
		appleExc(t, nombre, appleCampo(R, "casos", i), "ValueError", "")
	}
}

func TestAppleSondaElMismoSelectorAntiguoParaCertificadosDistintosSeRechaza(t *testing.T) {
	// Modela una colisión SHA-1 sin usar un hash débil ni credenciales reales.
	R := applePy(t, appleSondaCarga+`
h = Herramientas()
h.certificados += exportado(OTRO_DER, SELECTOR)
R["r"] = atrapar(s.sondear, LLAVERO, NOMBRE, h)
`)
	appleExc(t, "selector ambiguo", R["r"], "ValueError", "ambiguo")
}

func TestAppleSondaExportarDosVecesElMismoCertificadoNoEsAmbiguo(t *testing.T) {
	R := applePy(t, appleSondaCarga+`
h = Herramientas()
h.certificados += exportado()
R["r"] = s.sondear(LLAVERO, NOMBRE, h)["production_ok"]
`)
	appleIgual(t, "production_ok", R["r"], `true`)
}

func TestAppleSondaHojaValidaConservaLosSelectoresNativosYLaSondaEnFrio(t *testing.T) {
	R := applePy(t, appleSondaCarga+`
h = Herramientas()
r = s.sondear(LLAVERO, NOMBRE, h)
R["resultado"] = r
R["comandos"] = h.comandos
`)
	r := R["resultado"]
	huella, _ := R["huella_der"].(string)
	appleIgual(t, "production_ok", appleCampo(r, "production_ok"), `true`)
	appleIgual(t, "expected_signatures", appleCampo(r, "expected_signatures"), `5`)
	firmasR, _ := appleCampo(r, "signatures").([]any)
	for i, f := range firmasR {
		appleIgual(t, "firma ok", appleCampo(f, "ok"), `true`)
		if appleCampo(f, "chain", 0, "sha256") != huella {
			t.Errorf("la cadena de la firma %d no empieza por la hoja esperada: %s", i, appleTexto(f))
		}
	}
	comandos, _ := R["comandos"].([]any)
	var firmas, politicas []int
	var selectores []any
	for i, c := range comandos {
		switch {
		case appleCampo(c, 0) == "codesign" && appleCampo(c, 1) == "-f":
			firmas = append(firmas, i)
			selectores = append(selectores, appleCampo(c, 3))
		case appleCampo(c, 0) == "security" && appleCampo(c, 1) == "verify-cert":
			politicas = append(politicas, i)
			if !appleContiene(c, "-L") {
				t.Errorf("verify-cert sin -L: %s", appleTexto(c))
			}
		case appleCampo(c, 0) == "codesign" && appleCampo(c, 1) == "-d":
			l, _ := c.([]any)
			arg, _ := appleCampo(c, 2).(string)
			if len(l) != 4 || len(arg) < 23 || arg[:23] != "--extract-certificates=" {
				t.Errorf("extracción con forma inesperada: %s", appleTexto(c))
			}
		}
	}
	if len(firmas) == 0 || firmas[0] != 0 {
		t.Errorf("la primera orden tiene que ser la firma en frío: %s", appleTexto(appleCampo(comandos, 0)))
	}
	nombre, sel := "Developer ID Application: Sytue", strings.Repeat("A", 40)
	appleIgual(t, "selectores de codesign", selectores, appleTexto([]string{nombre, nombre, nombre, sel, sel}))
	politicasR, _ := appleCampo(r, "policies").([]any)
	if len(politicasR) != 4 {
		t.Errorf("se esperaban 4 políticas: %s", appleTexto(politicasR))
	}
	tipos := map[any]bool{}
	for _, p := range politicasR {
		if appleCampo(p, "sha256") != huella {
			t.Errorf("política sobre otro certificado: %s", appleTexto(p))
		}
		tipos[appleCampo(p, "policy")] = true
	}
	if len(tipos) != 2 || !tipos["basic"] || !tipos["codeSign"] {
		t.Errorf("las políticas tienen que ser basic y codeSign: %s", appleTexto(politicasR))
	}
	if len(firmas) > 0 && len(politicas) > 0 && firmas[len(firmas)-1] >= politicas[0] {
		t.Errorf("las políticas se comprueban después de todas las firmas")
	}
}

func TestAppleSondaOtraHojaSeRechazaAunqueCodesignVerifique(t *testing.T) {
	R := applePy(t, appleSondaCarga+`
h = Herramientas()
h.hojas = {i: OTRO_DER for i in range(5)}
r = s.sondear(LLAVERO, NOMBRE, h)
R["ok"] = r["production_ok"]
R["firmas"] = [f["ok"] for f in r["signatures"]]
`)
	appleIgual(t, "production_ok", R["ok"], `false`)
	appleIgual(t, "ninguna firma vale", R["firmas"], `[false, false, false, false, false]`)
}

func TestAppleSondaSinHojaNoSeSustituyePorElIntermedioEsperado(t *testing.T) {
	R := applePy(t, appleSondaCarga+`
h = Herramientas()
h.sin_hoja = True
r = s.sondear(LLAVERO, NOMBRE, h)
R["ok"] = r["production_ok"]
R["firmas"] = [f["ok"] for f in r["signatures"]]
`)
	appleIgual(t, "production_ok", R["ok"], `false`)
	appleIgual(t, "ninguna firma vale", R["firmas"], `[false, false, false, false, false]`)
}

func TestAppleSondaFallosDeCodesignYDeExportacionNoPasan(t *testing.T) {
	opciones := []string{"verificar", "extraer", "firmar", "exportar", "identidades"}
	R := applePy(t, appleSondaCarga+`
R["r"] = {}
for opcion in ["verificar", "extraer", "firmar", "exportar", "identidades"]:
    h = Herramientas()
    setattr(h, opcion, 1)
    R["r"][opcion] = s.sondear(LLAVERO, NOMBRE, h)["production_ok"]
`)
	for _, o := range opciones {
		t.Run(o, func(t *testing.T) { appleIgual(t, o, appleCampo(R, "r", o), `false`) })
	}
}

func TestAppleSondaUnFalloEnFrioNoLoTapanFirmasValidasPosteriores(t *testing.T) {
	R := applePy(t, appleSondaCarga+`
h = Herramientas()
h.firmas_fallidas = {1}
r = s.sondear(LLAVERO, NOMBRE, h)
R["ok"] = r["production_ok"]
R["firmas"] = [f["ok"] for f in r["signatures"]]
`)
	appleIgual(t, "production_ok", R["ok"], `false`)
	appleIgual(t, "firmas", R["firmas"], `[false, true, true, true, true]`)
}

func TestAppleSondaUnFalloPorNombreNoLoTapanLosSelectoresPorHash(t *testing.T) {
	R := applePy(t, appleSondaCarga+`
h = Herramientas()
h.firmas_fallidas = {1, 2, 3}
r = s.sondear(LLAVERO, NOMBRE, h)
R["ok"] = r["production_ok"]
R["firmas"] = [f["ok"] for f in r["signatures"]]
`)
	appleIgual(t, "production_ok", R["ok"], `false`)
	appleIgual(t, "firmas", R["firmas"], `[false, false, false, true, true]`)
}

func TestAppleSondaLaBusquedaGlobalPorNombreUsaElCertificadoExportadoYElLlaveroSigueLocal(t *testing.T) {
	R := applePy(t, appleSondaCarga+`
h = Herramientas()
h.globales = True
h.hojas = {i: OTRO_DER for i in range(5)}
r = s.sondear(LLAVERO, NOMBRE, h)
R["ok"] = r["production_ok"]
R["fuera"] = r["identities_outside_ephemeral"]
R["firmas"] = [f["ok"] for f in r["signatures"]]
R["busco_por_nombre"] = ["security", "find-certificate", "-a", "-Z", "-p", "-c", NOMBRE] in h.comandos
`)
	appleIgual(t, "production_ok", R["ok"], `true`)
	appleIgual(t, "identidades fuera del llavero efímero", R["fuera"], `["`+strings.Repeat("B", 40)+`"]`)
	appleIgual(t, "firmas", R["firmas"], `[true, true, false, false, false]`)
	appleIgual(t, "exporta el certificado buscado por nombre", R["busco_por_nombre"], `true`)
}

func TestAppleSondaUnaColisionDeSelectorEntreLlaverosSeRechaza(t *testing.T) {
	R := applePy(t, appleSondaCarga+`
class Colision(Herramientas):
    def __call__(self, argv):
        r = super().__call__(argv)
        if argv[:2] == ["security", "find-certificate"] and argv[-1] == NOMBRE:
            r.stdout += exportado(OTRO_DER, SELECTOR)
        return r
h = Colision()
h.globales = True
R["r"] = atrapar(s.sondear, LLAVERO, NOMBRE, h)
`)
	appleExc(t, "colisión global", R["r"], "ValueError", "ambiguo")
}

func TestAppleSondaUnaIdentidadGlobalSinCertificadoExportadoNoSeFiaPorSelector(t *testing.T) {
	R := applePy(t, appleSondaCarga+`
class SinCertificado(Herramientas):
    def __call__(self, argv):
        r = super().__call__(argv)
        if argv[:2] == ["security", "find-certificate"] and argv[-1] == NOMBRE:
            r.stdout = ""
        return r
h = SinCertificado()
h.globales = True
R["ok"] = s.sondear(LLAVERO, NOMBRE, h)["production_ok"]
`)
	appleIgual(t, "production_ok", R["ok"], `false`)
}

func appleContiene(lista, v any) bool {
	l, _ := lista.([]any)
	for _, x := range l {
		if x == v {
			return true
		}
	}
	return false
}
