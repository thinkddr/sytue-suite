// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package cicontratos

import (
	"os"
	"os/exec"
	"testing"
)

// Ejecuta el altool REAL de codemagic-cli-tools contra la salida de xcrun, sin
// SDK ni Apple. Corre en ci-desktop, también con empaquetado_solo. No simula el
// parser ni publica paquetes: solo sustituye subprocess y silencia el log del
// resultado.
//
// Necesita el Python del venv donde instalar-cli.sh dejó la CLI con el
// backport (SYTUE_CODEMAGIC_PYTHON); sin él se prueba con python3, y si ahí no
// está la CLI la prueba se salta: el unittest original tampoco podía correr.

const codemagicAltoolCargar = `
import copy, hashlib, importlib.metadata, subprocess
from unittest.mock import patch
from codemagic.models.altool.altool import Altool, AltoolCommandError
from codemagic.models.altool.altool_result import AltoolResult
backport = cargar("backport_altool", APPLE / "codemagic-altool.py")

EXITO = {
    "os-version": "Version 27.0.1 (Build 26A434)",
    "tool-version": "27.0.5 (2.1)",
    "tool-path": "/Applications/Xcode.app/Contents/SharedFrameworks/ContentDelivery.framework/Resources",
    "success-message": "No errors uploading archive at 'apps/fyne/dist/sytue-notes-ios.ipa'.",
    "details": {
        "delivery-uuid": "603d6112-b893-4545-9035-6d47288060ee",
        "transferred": "19800788 bytes in 19.105 seconds",
    },
}
TRANSITORIO = "Running altool...\nERROR: GET UPLOAD STATE (ASSET): received status code 500; internal server error. (500)\n"
CONFIRMACION = "INFO: ContentDelivery.Uploader\nUPLOAD SUCCEEDED with no errors\n"
distribucion = importlib.metadata.distribution("codemagic-cli-tools")
FUENTE = pathlib.Path(distribucion.locate_file("codemagic/models/altool/altool.py")).read_bytes()


def salida(texto, codigo=0, clase=Altool):
    cli = clase(username="prueba", password="abcd-abcd-abcd-abcd")
    with patch.object(cli, "_log_process_output"), patch("subprocess.check_output") as ejecutar:
        if codigo:
            ejecutar.side_effect = subprocess.CalledProcessError(codigo, ("xcrun", "altool"), output=texto.encode())
        else:
            ejecutar.return_value = texto.encode()
        return cli._run_command(("xcrun", "altool"), "carga fallida", None)


def resultado(texto, codigo=0, clase=Altool):
    try:
        r = salida(texto, codigo, clase)
    except Exception as e:
        return {"error": excepcion(e), "codigo": getattr(e, "return_code", None)}
    return {"tipo": type(r).__name__, "es_resultado": isinstance(r, AltoolResult),
            "delivery_uuid": r.details.delivery_uuid, "success_message": r.success_message,
            "product_errors": bool(r.product_errors)}
`

type codemagicAltoolResultado struct {
	Error          *pyRestoExcepcion `json:"error"`
	Codigo         *int              `json:"codigo"`
	EsResultado    bool              `json:"es_resultado"`
	DeliveryUUID   string            `json:"delivery_uuid"`
	SuccessMessage string            `json:"success_message"`
	ProductErrors  bool              `json:"product_errors"`
}

const (
	codemagicAltoolUUID    = "603d6112-b893-4545-9035-6d47288060ee"
	codemagicAltoolMensaje = "No errors uploading archive at 'apps/fyne/dist/sytue-notes-ios.ipa'."
)

// codemagicAltoolPython es el intérprete con la CLI revisada, comprobado como
// hacía setUpClass: la versión de la CLI es la del backport y su altool.py
// tiene la huella corregida.
func codemagicAltoolPython(t *testing.T) string {
	t.Helper()
	python := os.Getenv("SYTUE_CODEMAGIC_PYTHON")
	if python == "" {
		python = "python3"
		if err := exec.Command(python, "-c", "import codemagic.models.altool.altool").Run(); err != nil { //nolint:gosec // G204: intérprete de la prueba.
			t.Skip("falta codemagic-cli-tools (SYTUE_CODEMAGIC_PYTHON apunta al Python del venv de instalar-cli.sh): la prueba no puede ejercitar el altool real")
		}
	}
	var r struct {
		Version   string `json:"version"`
		Revisada  string `json:"revisada"`
		Huella    string `json:"huella"`
		Corregida string `json:"corregida"`
	}
	pyRestoCorrerCon(t, python, codemagicAltoolCargar+`
devolver({"version": distribucion.version, "revisada": backport.VERSION,
          "huella": hashlib.sha256(FUENTE).hexdigest(), "corregida": backport.CORREGIDO_SHA256})
`, &r)
	if r.Version != r.Revisada {
		t.Fatalf("la regresión tiene que usar la versión revisada de la CLI real: instalada %s, revisada %s", r.Version, r.Revisada)
	}
	if r.Huella != r.Corregida {
		t.Fatalf("la CLI instalada no tiene el backport verificado: %s, no %s", r.Huella, r.Corregida)
	}
	return python
}

func codemagicAltoolCorrer(t *testing.T, python, llamada string) codemagicAltoolResultado {
	t.Helper()
	var r codemagicAltoolResultado
	pyRestoCorrerCon(t, python, codemagicAltoolCargar+"\ndevolver("+llamada+")\n", &r)
	return r
}

func TestCodemagicAltoolExitoNormal(t *testing.T) {
	python := codemagicAltoolPython(t)
	r := codemagicAltoolCorrer(t, python, "resultado(CONFIRMACION + json.dumps(EXITO))")
	if r.Error != nil || !r.EsResultado || r.DeliveryUUID != codemagicAltoolUUID {
		t.Fatalf("resultado %+v", r)
	}
}

func TestCodemagicAltool500RecuperadoConEntregaConfirmada(t *testing.T) {
	python := codemagicAltoolPython(t)
	r := codemagicAltoolCorrer(t, python, "resultado(TRANSITORIO + CONFIRMACION + json.dumps(EXITO, indent=2))")
	if r.Error != nil || r.SuccessMessage != codemagicAltoolMensaje || r.ProductErrors || r.DeliveryUUID != codemagicAltoolUUID {
		t.Fatalf("resultado %+v", r)
	}
}

func TestCodemagicAltoolErrorSinConfirmacionSigueRojo(t *testing.T) {
	python := codemagicAltoolPython(t)
	pyRestoLanza(t, codemagicAltoolCorrer(t, python, "resultado(TRANSITORIO)").Error, "AltoolCommandError", "")
}

func TestCodemagicAltoolProductErrorsSigueRojoAunqueDigaExito(t *testing.T) {
	python := codemagicAltoolPython(t)
	var r codemagicAltoolResultado
	pyRestoCorrerCon(t, python, codemagicAltoolCargar+`
respuesta = copy.deepcopy(EXITO)
respuesta["product-errors"] = [{"code": 90426, "message": "Invalid Swift Support"}]
devolver(resultado(TRANSITORIO + CONFIRMACION + json.dumps(respuesta)))
`, &r)
	pyRestoLanza(t, r.Error, "AltoolCommandError", "")
}

func TestCodemagicAltoolExitNoCeroSigueRojoAunqueDigaExito(t *testing.T) {
	python := codemagicAltoolPython(t)
	r := codemagicAltoolCorrer(t, python, "resultado(TRANSITORIO + CONFIRMACION + json.dumps(EXITO), codigo=1)")
	pyRestoLanza(t, r.Error, "AltoolCommandError", "")
	if r.Codigo == nil || *r.Codigo != 1 {
		t.Fatalf("return_code %v, no 1", r.Codigo)
	}
}

func TestCodemagicAltoolLaRegresionDetectaElOriginal(t *testing.T) {
	// El mismo módulo upstream con la única línea restaurada: la regresión
	// tiene que verlo en rojo. No hay una segunda implementación del parser.
	python := codemagicAltoolPython(t)
	var r struct {
		Huella   string            `json:"huella"`
		Original string            `json:"original"`
		Error    *pyRestoExcepcion `json:"error"`
		Esperado string            `json:"esperado"`
	}
	pyRestoCorrerCon(t, python, codemagicAltoolCargar+`
original = FUENTE.replace(backport.CORREGIDO, backport.ORIGINAL)
modulo = {"__name__": "codemagic.models.altool.regresion_original", "__package__": "codemagic.models.altool"}
exec(compile(original, "altool-original.py", "exec"), modulo)
error = fallo(salida, TRANSITORIO + CONFIRMACION + json.dumps(EXITO, indent=2), clase=modulo["Altool"])
devolver({"huella": hashlib.sha256(original).hexdigest(), "original": backport.ORIGINAL_SHA256,
          "error": error, "esperado": modulo["AltoolCommandError"].__name__})
`, &r)
	if r.Huella != r.Original {
		t.Fatalf("restaurar la línea no da la fuente original: %s, no %s", r.Huella, r.Original)
	}
	pyRestoLanza(t, r.Error, r.Esperado, "")
}

func TestCodemagicAltoolElBackportEsIdempotenteYRechazaDerivas(t *testing.T) {
	python := codemagicAltoolPython(t)
	var r struct {
		Corregido bool `json:"corregido"`
		Intacto   bool `json:"intacto"`
		Derivas   []struct {
			Version  string            `json:"version"`
			Error    *pyRestoExcepcion `json:"error"`
			Conserva bool              `json:"conserva"`
		} `json:"derivas"`
	}
	pyRestoCorrerCon(t, python, codemagicAltoolCargar+`
ruta = TMP / "altool.py"
original = FUENTE.replace(backport.CORREGIDO, backport.ORIGINAL)
ruta.write_bytes(original)
backport.corregir(ruta, backport.VERSION)
corregido = ruta.read_bytes() == FUENTE
instante = ruta.stat().st_mtime_ns
backport.corregir(ruta, backport.VERSION)
intacto = ruta.stat().st_mtime_ns == instante
derivas = []
for fuente, version in ((original, "0.71.0"), (FUENTE, "0.71.0"), (original + b"\n# otra fuente\n", backport.VERSION)):
    ruta.write_bytes(fuente)
    derivas.append({"version": version, "error": fallo(backport.corregir, ruta, version), "conserva": ruta.read_bytes() == fuente})
devolver({"corregido": corregido, "intacto": intacto, "derivas": derivas})
`, &r)
	if !r.Corregido {
		t.Fatal("corregir() no deja la fuente con la huella corregida")
	}
	if !r.Intacto {
		t.Fatal("corregir() reescribió una fuente ya corregida")
	}
	if len(r.Derivas) != 3 {
		t.Fatalf("%d derivas, no 3", len(r.Derivas))
	}
	for _, d := range r.Derivas {
		pyRestoLanza(t, d.Error, "ValueError", "")
		if !d.Conserva {
			t.Fatalf("versión %s: corregir() tocó una fuente que rechazó", d.Version)
		}
	}
}
