// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package pruebasfyne

import (
	"strings"
	"testing"

	"git.sytue.eu/sytue/one/tools/guiontest"
)

// Traducción de apps/fyne/scripts/apple_llavero-test.py (y de su entrada de
// compatibilidad apple-llavero-test.py, que solo la ejecutaba): pruebas del
// aislamiento del contexto de firma de apple_llavero.py, el real. Todas las
// llamadas a Security son sintéticas: `SecuritySintetico` sustituye en proceso
// a /usr/bin/security, como en el original.

const appleLlaveroCarga = `
import atexit, contextlib, hashlib, importlib.util, io, ssl, tempfile
from pathlib import Path
from types import SimpleNamespace
from unittest import mock
SCRIPT = Path("apple_llavero.py").resolve()
spec = importlib.util.spec_from_file_location("apple_llavero_pruebas", SCRIPT)
module = importlib.util.module_from_spec(spec)
spec.loader.exec_module(module)
ORIGINAL_HERRAMIENTA = module.herramienta

class SecuritySintetico:
    def __init__(self):
        self.defaults = ["/fixture/login.keychain-db", "/fixture/login.keychain-db"]
        self.paths = ["/fixture/login.keychain-db", "/fixture/otra carpeta/own.keychain-db"]
        self.calls = []
        self.change_default_on_create = False
        self.replace_on_unlock = False
        self.replace_on_import = False
        self.certificates = []

    def __call__(self, argv, entrada=None, vacio_no_encontrado=False):
        assert argv[0] == "/usr/bin/security", "no se permiten programas alternativos"
        self.calls.append(list(argv))
        accion = argv[1]
        if accion in ("default-keychain", "list-keychains"):
            assert "-s" not in argv, "el contrato prohíbe escribir default/search-list"
            if accion == "default-keychain":
                return (json.dumps(self.defaults[1 if "-d" in argv else 0]) + "\n").encode()
            return ("\n".join(json.dumps(path) for path in self.paths) + "\n").encode()
        if accion == "find-certificate":
            return "".join(ssl.DER_cert_to_PEM_cert(c) for c in self.certificates).encode()
        path = Path(argv[argv.index("-k") + 1]) if accion in ("import", "add-certificates") else Path(argv[-1])
        if accion == "create-keychain":
            fd = os.open(path, os.O_WRONLY | os.O_CREAT | os.O_EXCL, 0o600)
            os.close(fd)
            self.paths.append(str(path))
            if self.change_default_on_create:
                self.defaults = [str(path), str(path)]
        elif accion == "delete-keychain":
            path.unlink()
            self.paths.remove(str(path))
        elif accion == "add-certificates":
            self.certificates.append(Path(argv[-1]).read_bytes())
        else:
            assert accion in ("set-keychain-settings", "unlock-keychain", "import", "set-key-partition-list"), "operación sintética no declarada"
            if (accion == "unlock-keychain" and self.replace_on_unlock) or (accion == "import" and self.replace_on_import):
                replacement = path.with_name("security-write.tmp")
                fd = os.open(replacement, os.O_WRONLY | os.O_CREAT | os.O_EXCL, 0o600)
                os.close(fd)
                os.replace(replacement, path)
        return b""

def setUp():
    global temp, root, env_file, security, stack
    temp = tempfile.TemporaryDirectory(prefix="sytue-llavero-test-", dir=os.environ["LLAVERO_TMP"])
    root = Path(temp.name)
    env_file = root / "github-env"
    env_file.write_text("")
    env_file.chmod(0o600)
    security = SecuritySintetico()
    stack = contextlib.ExitStack()
    stack.enter_context(mock.patch.dict(os.environ, {"GITHUB_ENV": str(env_file)}, clear=False))
    stack.enter_context(mock.patch.object(module, "entorno", return_value=(root, "12345", "1")))
    stack.enter_context(mock.patch.object(module, "herramienta", side_effect=security))
    stack.enter_context(contextlib.redirect_stdout(io.StringIO()))
    atexit.register(tearDown)

def tearDown():
    stack.close()
    temp.cleanup()

def exportados():
    return dict(line.split("=", 1) for line in env_file.read_text().splitlines())

def crear():
    module.crear()
    values = exportados()
    os.environ["SYTUE_APPLE_SIGNING_STATE"] = values["SYTUE_APPLE_SIGNING_STATE"]
    os.environ["SYTUE_APPLE_JOB_DIR"] = values["SYTUE_APPLE_JOB_DIR"]
    return Path(values["SYTUE_APPLE_SIGNING_STATE"])

def borrado_llavero():
    return any(call[1] == "delete-keychain" for call in security.calls)

def perfil_sintetico():
    state = crear()
    folder = state.parent / "perfiles-apple"
    folder.mkdir(mode=0o700)
    source = folder / "profile.mobileprovision"
    source.write_bytes(b"synthetic public profile; crypto boundary covered by real CMS tests")
    source.chmod(0o600)
    home = root / "synthetic-home"
    home.mkdir(mode=0o700)
    (home / "Library").mkdir(mode=0o700)
    stack.enter_context(mock.patch.object(module.Path, "home", return_value=home))
    stack.enter_context(mock.patch.object(module, "perfil_ios_verificado", return_value="11111111-2222-3333-4444-555555555555"))
    return state, source, module.cache_perfiles()

def cache_parcial():
    state, _, cache = perfil_sintetico()
    _, contenido, identidad, _ = module.cargar()
    for directory in (cache.parents[2], cache.parents[1], cache.parent, cache):
        directory.mkdir(mode=0o700)
        entry = {"path": str(directory), "identity": module.propietario(directory, directorio=True, privado=True)}
        contenido.setdefault("createdProfileDirectories", []).append(entry)
    module.guardar_estado(state, contenido, identidad)
    return state, cache

INSTALADO = "11111111-2222-3333-4444-555555555555.mobileprovision"
`

// applePaso (y appleTienda) viven aquí y no en apple_perfiles_workflow_test.go
// porque esta prueba es pública y aquella no se publica en la suite
// (tools/publicar-suite/excluir.txt).
const (
	appleWorkflow = ".github/workflows/release-desktop.yml"
	appleTienda   = "Perfiles de tienda de apps y widgets (solo con los widgets de Apple)"
)

// applePaso es el `run: |` del paso con ese nombre, sin la sangría común.
func applePaso(t *testing.T, nombre string) string {
	t.Helper()
	lineas := strings.Split(guiontest.Leer(t, appleWorkflow), "\n")
	inicio := -1
	for i, l := range lineas {
		if strings.TrimPrefix(strings.TrimSpace(l), "- ") == "name: "+nombre {
			inicio = i
			break
		}
	}
	if inicio < 0 {
		t.Fatalf("no hay paso %q en %s", nombre, appleWorkflow)
	}
	for inicio < len(lineas) && lineas[inicio] != "        run: |" {
		inicio++
	}
	var cuerpo []string
	for _, l := range lineas[inicio+1:] {
		if l != "" && !strings.HasPrefix(l, "          ") {
			break
		}
		cuerpo = append(cuerpo, l)
	}
	return appleDedent(cuerpo) + "\n"
}

// appleDedent hace lo que textwrap.dedent: quita la sangría común.
func appleDedent(lineas []string) string {
	sangria := -1
	for _, l := range lineas {
		if strings.TrimSpace(l) == "" {
			continue
		}
		n := len(l) - len(strings.TrimLeft(l, " \t"))
		if sangria < 0 || n < sangria {
			sangria = n
		}
	}
	out := make([]string, len(lineas))
	for i, l := range lineas {
		if strings.TrimSpace(l) == "" {
			out[i] = ""
		} else {
			out[i] = l[sangria:]
		}
	}
	return strings.Join(out, "\n")
}

func appleLlavero(t *testing.T, codigo string, env ...string) map[string]any {
	t.Helper()
	return applePy(t, appleLlaveroCarga+codigo, append([]string{"LLAVERO_TMP=" + t.TempDir()}, env...)...)
}

func TestAppleLlaveroCrearYLimpiarConservaDefaultsYCadaEntradaAjena(t *testing.T) {
	R := appleLlavero(t, `
setUp()
before = (list(security.defaults), list(security.paths))
state = crear()
R["estado"] = state.is_file()
R["defaults_igual"] = security.defaults == before[0]
R["paths_igual"] = security.paths[:-1] == before[1]
module.limpiar()
R["restaurado"] = (security.defaults, security.paths) == before
R["borrado"] = not state.parent.exists()
`)
	for _, k := range []string{"estado", "defaults_igual", "paths_igual", "restaurado", "borrado"} {
		appleIgual(t, k, R[k], `true`)
	}
}

func TestAppleLlaveroUnReemplazoAtomicoHechoPorUnlockNoPierdeLaProcedencia(t *testing.T) {
	R := appleLlavero(t, `
setUp()
security.replace_on_unlock = True
state = crear()
R["estado"] = state.is_file()
module.limpiar()
R["borrado"] = not state.parent.exists()
`)
	appleIgual(t, "estado", R["estado"], `true`)
	appleIgual(t, "borrado", R["borrado"], `true`)
}

func TestAppleLlaveroUnaListaDeBusquedaVaciaSeConservaSinEscribirla(t *testing.T) {
	R := appleLlavero(t, `
setUp()
security.paths = []
defaults = list(security.defaults)
state = crear()
R["paths"] = security.paths == [str(state.parent / "firma.keychain-db")]
module.limpiar()
R["defaults"] = security.defaults == defaults
R["vacia"] = security.paths
`)
	appleIgual(t, "solo el llavero propio en la lista", R["paths"], `true`)
	appleIgual(t, "defaults", R["defaults"], `true`)
	appleIgual(t, "lista vacía otra vez", R["vacia"], `[]`)
}

func TestAppleLlaveroLaLimpiezaConservaCambiosPersonalesPosteriores(t *testing.T) {
	R := appleLlavero(t, `
setUp()
crear()
security.paths.append("/fixture/anadido-por-la-persona.keychain-db")
security.defaults = ["/fixture/otro-default.keychain-db"] * 2
current = (list(security.defaults), list(security.paths))
own = current[1][-2]
module.limpiar()
R["defaults"] = security.defaults == current[0]
R["paths"] = security.paths == [path for path in current[1] if path != own]
`)
	appleIgual(t, "defaults de la persona", R["defaults"], `true`)
	appleIgual(t, "solo se retira el propio", R["paths"], `true`)
}

func TestAppleLlaveroLaCreacionDetectaUnDefaultModificadoYNoLoRestaura(t *testing.T) {
	R := appleLlavero(t, `
setUp()
security.change_default_on_create = True
R["crear"] = atrapar(crear)
values = exportados()
os.environ["SYTUE_APPLE_SIGNING_STATE"] = values["SYTUE_APPLE_SIGNING_STATE"]
os.environ["SYTUE_APPLE_JOB_DIR"] = values["SYTUE_APPLE_JOB_DIR"]
current = list(security.defaults)
R["limpiar"] = atrapar(module.limpiar)
R["defaults"] = security.defaults == current
R["borrado"] = borrado_llavero()
`)
	appleExc(t, "crear", R["crear"], "ErrorLlavero", "")
	appleExc(t, "limpiar", R["limpiar"], "ErrorLlavero", "")
	appleIgual(t, "no restaura el default", R["defaults"], `true`)
	appleIgual(t, "no borra llaveros", R["borrado"], `false`)
}

func TestAppleLlaveroNoBorraUnLlaveroQueSustituyeAlCreado(t *testing.T) {
	R := appleLlavero(t, `
setUp()
crear()
_, _, _, own = module.cargar()
moved = own.with_name("original.keychain-db")
own.rename(moved)
fd = os.open(own, os.O_WRONLY | os.O_CREAT | os.O_EXCL, 0o600)
os.close(fd)
R["limpiar"] = atrapar(module.limpiar)
R["sigue"] = own.is_file()
R["borrado"] = borrado_llavero()
`)
	appleExc(t, "limpiar", R["limpiar"], "ErrorLlavero", "")
	appleIgual(t, "el sustituto sigue", R["sigue"], `true`)
	appleIgual(t, "no borra llaveros", R["borrado"], `false`)
}

func TestAppleLlaveroNoAceptaElEstadoDeOtroJob(t *testing.T) {
	R := appleLlavero(t, `
setUp()
crear()
with mock.patch.object(module, "entorno", return_value=(root, "99999", "1")):
    R["r"] = atrapar(module.cargar)
`)
	appleExc(t, "cargar en otro job", R["r"], "ErrorLlavero", "")
}

func TestAppleLlaveroNoImportaUnP12DeOtroDirectorio(t *testing.T) {
	R := appleLlavero(t, `
setUp()
crear()
outside = root / "certificates.ajeno"
outside.mkdir(mode=0o700)
R["r"] = atrapar(module.importar, str(outside), ["codesign"])
`)
	appleExc(t, "importar de fuera", R["r"], "ErrorLlavero", "")
}

func TestAppleLlaveroLaImportacionControladaActualizaLaIdentidadYLimpia(t *testing.T) {
	R := appleLlavero(t, `
setUp()
state = crear()
folder = state.parent / "certificates.synthetic"
folder.mkdir(mode=0o700)
p12 = folder / "synthetic.p12"
p12.write_bytes(b"synthetic fixture without a private key")
p12.chmod(0o600)
security.replace_on_import = True
module.importar(str(folder), ["codesign", "productbuild"])
module.limpiar()
R["borrado"] = not state.parent.exists()
`)
	appleIgual(t, "borrado", R["borrado"], `true`)
}

func TestAppleLlaveroImportarDeduplicaCopiasP12DelCatalogoYToleraUnItemYaExistente(t *testing.T) {
	R := appleLlavero(t, `
setUp()
state = crear()
folder = state.parent / "certificates.catalogo"
folder.mkdir(mode=0o700)
for idx in range(15):
    copia = folder / f"DISTRIBUTION_9A8B7C6D5E_{idx:08d}.p12"
    copia.write_bytes(b"synthetic distribution fixture")
    copia.chmod(0o600)
instalador = folder / "MAC_INSTALLER_DISTRIBUTION_1122334455_1234_567.p12"
instalador.write_bytes(b"synthetic installer fixture")
instalador.chmod(0o600)
module.importar(str(folder), ["codesign", "productsign", "productbuild"])
R["importadas"] = len([call[2] for call in security.calls if call[1] == "import"])

# Segunda importación en el mismo job (como preparar macappstore tras preparar ios):
# /usr/bin/security import devuelve código 1 cuando el ítem ya existe en el llavero.
real_security = security
def run_con_duplicado(argv, input=None, capture_output=True, timeout=30, check=False):
    if argv[1] == "import":
        return SimpleNamespace(returncode=1, stdout=b"",
            stderr=b"security: SecKeychainItemImport: The specified item already exists in the keychain.\n")
    return SimpleNamespace(returncode=0, stdout=real_security(argv, input), stderr=b"")
with mock.patch.object(module, "herramienta", new=ORIGINAL_HERRAMIENTA), \
        mock.patch.object(module.subprocess, "run", side_effect=run_con_duplicado):
    R["duplicado"] = atrapar(module.importar, str(folder), ["codesign", "productsign", "productbuild"])

def run_con_otro_error(argv, input=None, capture_output=True, timeout=30, check=False):
    if argv[1] == "import":
        return SimpleNamespace(returncode=1, stdout=b"", stderr=b"security: SecKeychainItemImport: MAC verification failed\n")
    return SimpleNamespace(returncode=0, stdout=real_security(argv, input), stderr=b"")
with mock.patch.object(module, "herramienta", new=ORIGINAL_HERRAMIENTA), \
        mock.patch.object(module.subprocess, "run", side_effect=run_con_otro_error):
    R["otro_error"] = atrapar(module.importar, str(folder), ["codesign"])

exceso = state.parent / "certificates.exceso"
exceso.mkdir(mode=0o700)
for idx in range(9):
    distinto = exceso / f"DISTRIBUTION_CERT{idx:06d}_abcdefgh.p12"
    distinto.write_bytes(b"synthetic excess fixture")
    distinto.chmod(0o600)
R["exceso"] = atrapar(module.importar, str(exceso), ["codesign"])
module.limpiar()
`)
	appleIgual(t, "dos identidades distintas importadas", R["importadas"], `2`)
	appleOK(t, "un ítem ya existente se tolera", R["duplicado"])
	appleExc(t, "otro error de security", R["otro_error"], "ErrorLlavero", "^herramienta_del_llavero_fallo$")
	appleExc(t, "demasiadas identidades", R["exceso"], "ErrorLlavero", "^cantidad_de_identidades_invalida$")
}

func TestAppleLlaveroElDERPublicoFijadoNoSeDuplicaYUnDERAlteradoNoSeImporta(t *testing.T) {
	R := appleLlavero(t, `
setUp()
crear()
pinned = SCRIPT.parents[3] / ".github/actions/apple/AppleRootCA.cer"
module.publicos([pinned])
module.publicos([pinned])
R["tras_dos"] = len(security.certificates)
changed = root / pinned.name
changed.write_bytes(pinned.read_bytes() + b"changed")
R["alterado"] = atrapar(module.publicos, [changed])
R["tras_alterado"] = len(security.certificates)
module.limpiar()
`)
	appleIgual(t, "una sola copia del DER fijado", R["tras_dos"], `1`)
	appleExc(t, "DER alterado", R["alterado"], "ErrorLlavero", "")
	appleIgual(t, "el alterado no entra", R["tras_alterado"], `1`)
}

func TestAppleLlaveroUnPerfilExistenteSeInstalaTemporalmenteYSoloSeRetiraLaCopiaPropia(t *testing.T) {
	R := appleLlavero(t, `
setUp()
state, source, cache = perfil_sintetico()
module.instalar_perfil(str(source), "eu.sytue.contacts")
installed = cache / INSTALADO
R["copia"] = installed.read_bytes() == source.read_bytes()
module.limpiar()
R["instalado"] = installed.exists()
R["estado"] = state.parent.exists()
R["cache"] = cache.exists()
`)
	appleIgual(t, "copia idéntica", R["copia"], `true`)
	appleIgual(t, "perfil retirado", R["instalado"], `false`)
	appleIgual(t, "estado retirado", R["estado"], `false`)
	appleIgual(t, "caché retirada", R["cache"], `false`)
}

func TestAppleLlaveroUnPadreDePerfilesSustituidoPorSymlinkNoSeLee(t *testing.T) {
	R := appleLlavero(t, `
setUp()
state, source, cache = perfil_sintetico()
contenido = source.read_bytes()
outside = root / "outside-profiles"
source.parent.rename(outside)
source.parent.symlink_to(outside, target_is_directory=True)
with mock.patch.object(module, "perfil_ios_verificado", side_effect=AssertionError("no leer fuera del job")):
    R["r"] = atrapar(module.instalar_perfil, str(source), "eu.sytue.contacts")
R["cache"] = cache.exists()
module.limpiar()
R["estado"] = state.parent.exists()
R["fuera"] = (outside / source.name).read_bytes() == contenido
`)
	appleExc(t, "instalar a través del symlink", R["r"], "ErrorLlavero", "")
	appleIgual(t, "sin caché", R["cache"], `false`)
	appleIgual(t, "estado retirado", R["estado"], `false`)
	appleIgual(t, "lo de fuera intacto", R["fuera"], `true`)
}

func TestAppleLlaveroLosDirectoriosRegistradosAntesDelFalloSeLimpianPorIdentidad(t *testing.T) {
	R := appleLlavero(t, `
setUp()
state, cache = cache_parcial()
module.limpiar()
R["creados"] = cache.parents[2].exists()
R["padre"] = cache.parents[3].is_dir()
R["estado"] = state.parent.exists()
`)
	appleIgual(t, "directorios creados retirados", R["creados"], `false`)
	appleIgual(t, "el padre preexistente sigue", R["padre"], `true`)
	appleIgual(t, "estado retirado", R["estado"], `false`)
}

func TestAppleLlaveroUnaCacheParcialSustituidaSeConserva(t *testing.T) {
	R := appleLlavero(t, `
setUp()
state, cache = cache_parcial()
original = cache.with_name("original-cache")
cache.rename(original)
cache.mkdir(mode=0o700)
other = cache / "user-file.mobileprovision"
other.write_bytes(b"synthetic user replacement; preserve")
R["limpiar"] = atrapar(module.limpiar)
R["original"] = original.is_dir()
R["estado"] = state.parent.exists()
R["otro"] = other.read_bytes()
`)
	appleExc(t, "limpiar", R["limpiar"], "ErrorLlavero", "")
	appleIgual(t, "la caché original sigue", R["original"], `true`)
	appleIgual(t, "el estado sigue", R["estado"], `true`)
	appleIgual(t, "el fichero de la persona", R["otro"], `"synthetic user replacement; preserve"`)
}

func TestAppleLlaveroUnPerfilPreexistenteIdenticoYOtrosArchivosSeConservan(t *testing.T) {
	R := appleLlavero(t, `
setUp()
state, source, cache = perfil_sintetico()
# Datos de Xcode preexistentes y propios sin depender del umask del runner; los
# directorios y perfiles que crea el ayudante siguen siendo 0700/0600.
for directory in (cache.parents[2], cache.parents[1], cache.parent, cache):
    directory.mkdir(mode=0o755)
    directory.chmod(0o755)
installed = cache / INSTALADO
installed.write_bytes(source.read_bytes())
installed.chmod(0o644)
original_inode = installed.stat().st_ino
other = cache / "unrelated-existing.mobileprovision"
other.write_text("unrelated user data, do not modify")
other.chmod(0o644)
module.instalar_perfil(str(source), "eu.sytue.contacts")
module.limpiar()
R["inodo"] = installed.stat().st_ino == original_inode
R["modo_perfil"] = installed.stat().st_mode & 0o777
R["modo_cache"] = cache.stat().st_mode & 0o777
R["otro"] = other.read_text()
R["estado"] = state.parent.exists()
`)
	appleIgual(t, "mismo inodo", R["inodo"], `true`)
	appleIgual(t, "modo del perfil 0644", R["modo_perfil"], `420`)
	appleIgual(t, "modo de la caché 0755", R["modo_cache"], `493`)
	appleIgual(t, "otro fichero intacto", R["otro"], `"unrelated user data, do not modify"`)
	appleIgual(t, "estado retirado", R["estado"], `false`)
}

func TestAppleLlaveroNoSobrescribeUnPerfilPreexistenteDistinto(t *testing.T) {
	R := appleLlavero(t, `
setUp()
_, source, cache = perfil_sintetico()
for directory in (cache.parents[2], cache.parents[1], cache.parent, cache):
    directory.mkdir(mode=0o755)
    directory.chmod(0o755)
installed = cache / INSTALADO
installed.write_bytes(b"preexisting user file")
installed.chmod(0o644)
R["r"] = atrapar(module.instalar_perfil, str(source), "eu.sytue.contacts")
module.limpiar()
R["contenido"] = installed.read_bytes()
`)
	appleExc(t, "instalar", R["r"], "ErrorLlavero", "^perfil_preexistente_distinto_conservar$")
	appleIgual(t, "contenido de la persona", R["contenido"], `"preexisting user file"`)
}

func TestAppleLlaveroNoBorraElPerfilQueLaPersonaHaSustituido(t *testing.T) {
	R := appleLlavero(t, `
setUp()
_, source, cache = perfil_sintetico()
module.instalar_perfil(str(source), "eu.sytue.contacts")
installed = cache / INSTALADO
installed.rename(cache / "saved-copy.mobileprovision")
installed.write_bytes(b"user replacement, do not delete")
installed.chmod(0o600)
R["limpiar"] = atrapar(module.limpiar)
R["contenido"] = installed.read_bytes()
`)
	appleExc(t, "limpiar", R["limpiar"], "ErrorLlavero", "")
	appleIgual(t, "contenido de la persona", R["contenido"], `"user replacement, do not delete"`)
}

// La ruta real: el paso del workflow que prepara los perfiles de tienda (sacado
// de release-desktop.yml por applePaso, como hacía el original con el `paso` de
// apple-perfiles-workflow-test.py) y el selector de apple-firma.sh encajan con
// el instalador privado.
func TestAppleLlaveroLaRutaRealDelWorkflowYElSelectorEncajanConElInstaladorPrivado(t *testing.T) {
	R := appleLlavero(t, `
import plistlib, subprocess, textwrap
setUp()
state, _, cache = perfil_sintetico()
payload = root / "profile-fixture.plist"
payload.write_bytes(plistlib.dumps({
    "CreationDate": datetime.datetime(2020, 1, 1),
    "ExpirationDate": datetime.datetime.now() + datetime.timedelta(days=1),
    "Name": "Existing synthetic host profile", "Platform": ["iOS"],
    "Entitlements": {"application-identifier": "ABCDE12345.eu.sytue.contacts"},
}))
workspace = root / "workflow-workspace"
scripts = workspace / "apps/fyne/scripts"
scripts.mkdir(parents=True)
(scripts / "empaquetar.sh").write_text("#!/bin/sh\necho no\n")
proxy = root / "profile-boundary.py"
proxy.write_text(textwrap.dedent('''\
    import json, os
    from pathlib import Path
    import sys
    args = sys.argv[1:]
    if args[0].endswith("apple-perfiles.py"):
        destination = Path(args[args.index("--salida") + 1])
        destination.mkdir(mode=0o700, parents=True, exist_ok=True)
        profile = destination / "contacts.mobileprovision"
        profile.write_bytes(Path(os.environ["PROFILE_PAYLOAD"]).read_bytes())
        profile.chmod(0o600)
        print(json.dumps({"profile_ready": True, "created": False}))
    elif args[0].endswith("apple_perfil.py") and args[1] == "decodificar":
        # Solo la frontera criptográfica de decodificación es sintética aquí.
        sys.stdout.buffer.write(Path(args[2]).read_bytes())
    elif args[0].endswith("apple_llavero.py") and args[1] == "instalar-perfil":
        # Ejecuta abajo el instalador real con esta misma petición de CLI
        # mientras siguen activos los mocks de Security y Path.home.
        Path(os.environ["PROFILE_INSTALL_REQUEST"]).write_text(json.dumps(args[2:]))
    elif args[0] == "-c":
        os.execv(sys.executable, [sys.executable, *args])
    else:
        raise SystemExit("unexpected entrypoint at synthetic profile boundary")
    '''))
binary = root / "profile-bin"
binary.mkdir()
python = binary / "python3"
python.write_text('#!/bin/bash\nexec "$PYTHON_REAL" "$PROFILE_BOUNDARY" "$@"\n')
python.chmod(0o700)
request = root / "profile-install-request.json"
env = dict(os.environ, PATH=str(binary) + os.pathsep + os.environ["PATH"],
           PYTHON_REAL=sys.executable, PROFILE_BOUNDARY=str(proxy), PROFILE_PAYLOAD=str(payload),
           PROFILE_INSTALL_REQUEST=str(request),
           SYTUE_APPLE_JOB_DIR=str(state.parent), SYTUE_SOLO_MAS="false", SYTUE_SOLO_IOS="true",
           SYTUE_PREPARAR_PERFILES_WIDGETS_SOLO="false", SYTUE_APPS_IOS_SELECCIONADAS="contacts",
           GITHUB_ENV=str(env_file))
result = subprocess.run(["bash", "-c", os.environ["PASO_TIENDA"]], cwd=workspace, env=env, capture_output=True, text=True)
R["paso"] = [result.returncode, result.stderr]
exported = exportados()
env["SYTUE_PERFILES_DIR"] = exported["SYTUE_PERFILES_DIR"]
selected = subprocess.run(["bash", str(SCRIPT.with_name("apple-firma.sh")), "instalar-perfil-ios", "eu.sytue.contacts"],
                          env=env, capture_output=True, text=True)
R["selector"] = [selected.returncode, selected.stdout, selected.stderr]
source_path, appid = json.loads(request.read_text())
R["fuente_en_el_job"] = Path(source_path).parent == state.parent / "perfiles-apple"
R["appid"] = appid
module.instalar_perfil(source_path, appid)
installed = cache / INSTALADO
R["instalado"] = installed.read_bytes() == payload.read_bytes()
module.limpiar()
R["cache"] = cache.exists()
`, "PASO_TIENDA="+applePaso(t, appleTienda))
	if appleCampo(R, "paso", 0) != float64(0) {
		t.Fatalf("el paso del workflow falló: %s", appleTexto(R["paso"]))
	}
	if appleCampo(R, "selector", 0) != float64(0) {
		t.Fatalf("apple-firma.sh instalar-perfil-ios falló: %s", appleTexto(R["selector"]))
	}
	appleIgual(t, "el selector no imprime nada", appleCampo(R, "selector", 1), `""`)
	appleIgual(t, "la fuente está en el job", R["fuente_en_el_job"], `true`)
	appleIgual(t, "appid", R["appid"], `"eu.sytue.contacts"`)
	appleIgual(t, "instala el payload", R["instalado"], `true`)
	appleIgual(t, "caché retirada", R["cache"], `false`)
}

func TestAppleLlaveroNoDelegaLaCreacionDeRecursosEnLaCLI(t *testing.T) {
	R := appleLlavero(t, `
setUp()
crear()
R["casos"] = [atrapar(module.ejecutar_cli, a) for a in (["certificates", "create"], ["fetch-signing-files", "eu.sytue.contacts", "--create"],
              ["certificates", "list", "--p12-password", "caller-controlled"])]
`)
	for i, n := range []string{"certificates create", "fetch-signing-files --create", "certificates list --p12-password"} {
		appleExc(t, n, appleCampo(R, "casos", i), "ErrorLlavero", "")
	}
}

func TestAppleLlaveroLaCLILectoraConfinaDestinosYNoVuelcaMaterial(t *testing.T) {
	R := appleLlavero(t, `
setUp()
state = crear()
folder = state.parent / "certificates.synthetic"
folder.mkdir(mode=0o700)
program = state.parent / "cm/bin/app-store-connect"
program.parent.mkdir(parents=True, mode=0o700)
program.write_text("synthetic CLI fixture; never executed")
program.chmod(0o700)
# Prueba del transporte, con un PEM sintético sin material criptográfico.
tipo_rsa = "RSA PRIVATE KEY"
key = f"-----BEGIN {tipo_rsa}-----\nc3ludGhldGljIGZpeHR1cmU=\n-----END {tipo_rsa}-----\n"
password = json.loads(state.read_text())["password"]
R["casos"] = []
for command in (["certificates", "list", "--save"], ["fetch-signing-files", "eu.sytue.contacts"]):
    arguments = [*command, "--certificates-dir", str(folder), "--certificate-key", "@env:CERTIFICATE_PRIVATE_KEY"]
    out, err = io.StringIO(), io.StringIO()
    with mock.patch.dict(os.environ, {"CERTIFICATE_PRIVATE_KEY": key}), \
            contextlib.redirect_stdout(out), contextlib.redirect_stderr(err), \
            mock.patch.object(module.subprocess, "run", return_value=SimpleNamespace(returncode=0, stdout=b"private fixture", stderr=b"private fixture")) as run:
        module.ejecutar_cli(arguments)
    argv = run.call_args.args[0]
    R["casos"].append({
        "programa": argv[0] == str(program),
        "perfiles": argv[argv.index("--profiles-dir") + 1] == str(state.parent / "perfiles-apple"),
        "password": argv[argv.index("--p12-password") + 1] == password,
        "clave_en_entorno": run.call_args.kwargs["env"]["CERTIFICATE_PRIVATE_KEY"] == key,
        "clave_en_argv": key in " ".join(argv),
        "salida": [out.getvalue(), err.getvalue()]})
module.limpiar()
`)
	for i, n := range []string{"certificates list --save", "fetch-signing-files"} {
		t.Run(n, func(t *testing.T) {
			c := appleCampo(R, "casos", i)
			for _, k := range []string{"programa", "perfiles", "password", "clave_en_entorno"} {
				appleIgual(t, k, appleCampo(c, k), `true`)
			}
			appleIgual(t, "la clave no va en argv", appleCampo(c, "clave_en_argv"), `false`)
			appleIgual(t, "no vuelca nada", appleCampo(c, "salida"), `["", ""]`)
		})
	}
}

func TestAppleLlaveroLaFronteraRealRechazaUnaPlataformaQueNoEsMacOS(t *testing.T) {
	R := appleLlavero(t, `
setUp()
with mock.patch.object(module.sys, "platform", "linux"):
    # Recuperar la función real; ningún flag de producto permite omitirla.
    real_spec = importlib.util.spec_from_file_location("apple_llavero_frontera_real", SCRIPT)
    real = importlib.util.module_from_spec(real_spec)
    real_spec.loader.exec_module(real)
    R["r"] = atrapar(real.entorno)
`)
	appleExc(t, "entorno() fuera de macOS", R["r"], "ErrorLlavero", "")
}

func TestAppleLlaveroLasReferenciasDirectasYCodificadasNoSeResuelven(t *testing.T) {
	R := applePy(t, `
from unittest import mock
from apple_clave import normalizar_clave_firma
R["casos"] = []
for value in ("@file:/fixture/no-leer", "@env:NO_LEER", base64.b64encode(b"@file:/fixture/no-leer").decode()):
    with mock.patch("builtins.open", side_effect=AssertionError("no abrir referencias")):
        R["casos"].append(atrapar(normalizar_clave_firma, value))
`)
	for i, n := range []string{"@file", "@env", "@file en base64"} {
		appleExc(t, n, appleCampo(R, "casos", i), "ValueError", "")
	}
}

func TestAppleLlaveroNormalizaLaClaveRSAPEMYBase64SinDelimitadorLiteralEnLaFuente(t *testing.T) {
	R := applePy(t, `
from apple_clave import normalizar_clave_firma
R["fuente"] = pathlib.Path("apple_clave.py").read_text(encoding="utf-8")
tipo_rsa = "RSA PRIVATE KEY"
cuerpo = base64.b64encode(b"material-sintetico-de-prueba-para-normalizar-columnas-64-exactas-rsa").decode("ascii")
cruda = f"  -----BEGIN {tipo_rsa}-----\\r\\n {cuerpo[:40]}  {cuerpo[40:]} \\n-----END {tipo_rsa}-----  "
esperada = (f"-----BEGIN {tipo_rsa}-----\n" + "\n".join(cuerpo[i : i + 64] for i in range(0, len(cuerpo), 64))
            + f"\n-----END {tipo_rsa}-----\n")
R["cruda"] = normalizar_clave_firma(cruda) == esperada
R["base64"] = normalizar_clave_firma(base64.b64encode(esperada.encode("utf-8")).decode("ascii")) == esperada
`)
	fuente, _ := R["fuente"].(string)
	for _, tipo := range []string{"RSA PRIVATE KEY", "EC PRIVATE KEY"} {
		for _, borde := range []string{"BEGIN", "END"} {
			if d := "-----" + borde + " " + tipo + "-----"; strings.Contains(fuente, d) {
				t.Errorf("apple_clave.py contiene el delimitador literal %q", d)
			}
		}
	}
	appleIgual(t, "PEM con espacios y \\r\\n escapados", R["cruda"], `true`)
	appleIgual(t, "PEM en base64", R["base64"], `true`)
}
