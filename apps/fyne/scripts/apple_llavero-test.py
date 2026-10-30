#!/usr/bin/env python3
# SPDX-License-Identifier: Apache-2.0
# Copyright (c) 2026 Daniel Delgado Ramos
"""Pruebas remotas del aislamiento; todas las llamadas a Security son sintéticas."""
import contextlib
import importlib.util
import io
import json
import os
import hashlib
from pathlib import Path
import ssl
import tempfile
from types import SimpleNamespace
import unittest
from unittest import mock

SCRIPT = Path(__file__).with_name("apple_llavero.py")
spec = importlib.util.spec_from_file_location("apple_llavero_pruebas", SCRIPT)
module = importlib.util.module_from_spec(spec)
spec.loader.exec_module(module)


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


class Aislamiento(unittest.TestCase):
    def setUp(self):
        self.temp = tempfile.TemporaryDirectory(prefix="sytue-llavero-test-")
        self.root = Path(self.temp.name)
        self.env_file = self.root / "github-env"
        self.env_file.write_text("")
        self.env_file.chmod(0o600)
        self.security = SecuritySintetico()
        self.stack = contextlib.ExitStack()
        self.stack.enter_context(mock.patch.dict(os.environ, {"GITHUB_ENV": str(self.env_file)}, clear=False))
        self.stack.enter_context(mock.patch.object(module, "entorno", return_value=(self.root, "12345", "1")))
        self.stack.enter_context(mock.patch.object(module, "herramienta", side_effect=self.security))
        self.stack.enter_context(contextlib.redirect_stdout(io.StringIO()))

    def tearDown(self):
        self.stack.close()
        self.temp.cleanup()

    def crear(self):
        module.crear()
        values = dict(line.split("=", 1) for line in self.env_file.read_text().splitlines())
        os.environ["SYTUE_APPLE_SIGNING_STATE"] = values["SYTUE_APPLE_SIGNING_STATE"]
        os.environ["SYTUE_APPLE_JOB_DIR"] = values["SYTUE_APPLE_JOB_DIR"]
        return Path(values["SYTUE_APPLE_SIGNING_STATE"])

    def test_crear_y_limpiar_conserva_defaults_y_cada_entrada_ajena(self):
        before = (list(self.security.defaults), list(self.security.paths))
        state = self.crear()
        self.assertTrue(state.is_file())
        self.assertEqual(self.security.defaults, before[0])
        self.assertEqual(self.security.paths[:-1], before[1])
        module.limpiar()
        self.assertEqual((self.security.defaults, self.security.paths), before)
        self.assertFalse(state.parent.exists())

    def test_reemplazo_atomico_hecho_por_unlock_no_pierde_procedencia(self):
        self.security.replace_on_unlock = True
        state = self.crear()
        self.assertTrue(state.is_file())
        module.limpiar()
        self.assertFalse(state.parent.exists())

    def test_lista_de_busqueda_vacia_se_conserva_sin_escribirla(self):
        self.security.paths = []
        defaults = list(self.security.defaults)
        state = self.crear()
        self.assertEqual(self.security.paths, [str(state.parent / "firma.keychain-db")])
        module.limpiar()
        self.assertEqual(self.security.defaults, defaults)
        self.assertEqual(self.security.paths, [])

    def test_limpieza_conserva_cambios_personales_posteriores(self):
        self.crear()
        self.security.paths.append("/fixture/anadido-por-la-persona.keychain-db")
        self.security.defaults = ["/fixture/otro-default.keychain-db"] * 2
        current = (list(self.security.defaults), list(self.security.paths))
        own = current[1][-2]
        module.limpiar()
        self.assertEqual(self.security.defaults, current[0])
        self.assertEqual(self.security.paths, [path for path in current[1] if path != own])

    def test_creacion_detecta_default_modificado_y_no_lo_restaura(self):
        self.security.change_default_on_create = True
        with self.assertRaises(module.ErrorLlavero):
            self.crear()
        values = dict(line.split("=", 1) for line in self.env_file.read_text().splitlines())
        os.environ["SYTUE_APPLE_SIGNING_STATE"] = values["SYTUE_APPLE_SIGNING_STATE"]
        os.environ["SYTUE_APPLE_JOB_DIR"] = values["SYTUE_APPLE_JOB_DIR"]
        current = list(self.security.defaults)
        with self.assertRaises(module.ErrorLlavero):
            module.limpiar()
        self.assertEqual(self.security.defaults, current)
        self.assertFalse(any(call[1] == "delete-keychain" for call in self.security.calls))

    def test_no_borra_un_llavero_que_sustituye_al_creado(self):
        self.crear()
        _, _, _, own = module.cargar()
        moved = own.with_name("original.keychain-db")
        own.rename(moved)
        fd = os.open(own, os.O_WRONLY | os.O_CREAT | os.O_EXCL, 0o600)
        os.close(fd)
        with self.assertRaises(module.ErrorLlavero):
            module.limpiar()
        self.assertTrue(own.is_file())
        self.assertFalse(any(call[1] == "delete-keychain" for call in self.security.calls))

    def test_no_acepta_estado_de_otro_job(self):
        self.crear()
        with mock.patch.object(module, "entorno", return_value=(self.root, "99999", "1")):
            with self.assertRaises(module.ErrorLlavero):
                module.cargar()

    def test_no_importa_p12_de_otro_directorio(self):
        self.crear()
        outside = self.root / "certificates.ajeno"
        outside.mkdir(mode=0o700)
        with self.assertRaises(module.ErrorLlavero):
            module.importar(str(outside), ["codesign"])

    def test_importacion_controlada_actualiza_identidad_y_limpia(self):
        state = self.crear()
        folder = state.parent / "certificates.synthetic"
        folder.mkdir(mode=0o700)
        p12 = folder / "synthetic.p12"
        p12.write_bytes(b"synthetic fixture without a private key")
        p12.chmod(0o600)
        self.security.replace_on_import = True
        module.importar(str(folder), ["codesign", "productbuild"])
        module.limpiar()
        self.assertFalse(state.parent.exists())

    def test_der_publico_fijado_no_se_duplica_y_un_der_alterado_no_se_importa(self):
        self.crear()
        pinned = SCRIPT.parents[3] / ".github/actions/apple/AppleRootCA.cer"
        module.publicos([pinned])
        module.publicos([pinned])
        self.assertEqual(len(self.security.certificates), 1)
        changed = self.root / pinned.name
        changed.write_bytes(pinned.read_bytes() + b"changed")
        with self.assertRaises(module.ErrorLlavero):
            module.publicos([changed])
        self.assertEqual(len(self.security.certificates), 1)
        module.limpiar()

    def perfil_sintetico(self):
        state = self.crear()
        folder = state.parent / "perfiles-apple"
        folder.mkdir(mode=0o700)
        source = folder / "profile.mobileprovision"
        source.write_bytes(b"synthetic public profile; crypto boundary covered by real CMS tests")
        source.chmod(0o600)
        home = self.root / "synthetic-home"
        home.mkdir(mode=0o700)
        (home / "Library").mkdir(mode=0o700)
        self.stack.enter_context(mock.patch.object(module.Path, "home", return_value=home))
        self.stack.enter_context(mock.patch.object(module, "perfil_ios_verificado", return_value="11111111-2222-3333-4444-555555555555"))
        return state, source, module.cache_perfiles()

    def test_perfil_existente_se_instala_temporalmente_y_solo_la_copia_propia_se_retira(self):
        state, source, cache = self.perfil_sintetico()
        module.instalar_perfil(str(source), "eu.sytue.contacts")
        installed = cache / "11111111-2222-3333-4444-555555555555.mobileprovision"
        self.assertEqual(installed.read_bytes(), source.read_bytes())
        module.limpiar()
        self.assertFalse(installed.exists())
        self.assertFalse(state.parent.exists())
        self.assertFalse(cache.exists())

    def test_parent_de_perfiles_sustituido_por_symlink_no_se_lee(self):
        state, source, cache = self.perfil_sintetico()
        contenido = source.read_bytes()
        outside = self.root / "outside-profiles"
        source.parent.rename(outside)
        source.parent.symlink_to(outside, target_is_directory=True)
        with mock.patch.object(module, "perfil_ios_verificado", side_effect=AssertionError("no leer fuera del job")):
            with self.assertRaises(module.ErrorLlavero):
                module.instalar_perfil(str(source), "eu.sytue.contacts")
        self.assertFalse(cache.exists())
        module.limpiar()
        self.assertFalse(state.parent.exists())
        self.assertEqual((outside / source.name).read_bytes(), contenido)

    def cache_parcial(self):
        state, _, cache = self.perfil_sintetico()
        _, contenido, identidad, _ = module.cargar()
        for directory in (cache.parents[2], cache.parents[1], cache.parent, cache):
            directory.mkdir(mode=0o700)
            entry = {"path": str(directory), "identity": module.propietario(directory, directorio=True, privado=True)}
            contenido.setdefault("createdProfileDirectories", []).append(entry)
        module.guardar_estado(state, contenido, identidad)
        return state, cache

    def test_directorios_registrados_antes_del_fallo_se_limpian_por_identidad(self):
        state, cache = self.cache_parcial()
        module.limpiar()
        self.assertFalse(cache.parents[2].exists())
        self.assertTrue(cache.parents[3].is_dir())
        self.assertFalse(state.parent.exists())

    def test_cache_parcial_sustituido_se_conserva(self):
        state, cache = self.cache_parcial()
        original = cache.with_name("original-cache")
        cache.rename(original)
        cache.mkdir(mode=0o700)
        other = cache / "user-file.mobileprovision"
        other.write_bytes(b"synthetic user replacement; preserve")
        with self.assertRaises(module.ErrorLlavero):
            module.limpiar()
        self.assertTrue(original.is_dir())
        self.assertTrue(state.parent.exists())
        self.assertEqual(other.read_bytes(), b"synthetic user replacement; preserve")

    def test_perfil_preexistente_identico_y_otros_archivos_se_conservan(self):
        state, source, cache = self.perfil_sintetico()
        # Model owned preexisting Xcode data without depending on the runner's
        # umask; helper-created directories and profiles remain 0700/0600.
        for directory in (cache.parents[2], cache.parents[1], cache.parent, cache):
            directory.mkdir(mode=0o755)
            directory.chmod(0o755)
        installed = cache / "11111111-2222-3333-4444-555555555555.mobileprovision"
        installed.write_bytes(source.read_bytes())
        installed.chmod(0o644)
        original_inode = installed.stat().st_ino
        other = cache / "unrelated-existing.mobileprovision"
        other.write_text("unrelated user data, do not modify")
        other.chmod(0o644)
        module.instalar_perfil(str(source), "eu.sytue.contacts")
        module.limpiar()
        self.assertEqual(installed.stat().st_ino, original_inode)
        self.assertEqual(installed.stat().st_mode & 0o777, 0o644)
        self.assertEqual(cache.stat().st_mode & 0o777, 0o755)
        self.assertEqual(other.read_text(), "unrelated user data, do not modify")
        self.assertFalse(state.parent.exists())

    def test_no_sobrescribe_un_perfil_preexistente_distinto(self):
        _, source, cache = self.perfil_sintetico()
        for directory in (cache.parents[2], cache.parents[1], cache.parent, cache):
            directory.mkdir(mode=0o755)
            directory.chmod(0o755)
        installed = cache / "11111111-2222-3333-4444-555555555555.mobileprovision"
        installed.write_bytes(b"preexisting user file")
        installed.chmod(0o644)
        with self.assertRaisesRegex(module.ErrorLlavero, "^perfil_preexistente_distinto_conservar$"):
            module.instalar_perfil(str(source), "eu.sytue.contacts")
        module.limpiar()
        self.assertEqual(installed.read_bytes(), b"preexisting user file")

    def test_no_borra_el_perfil_que_la_persona_ha_sustituido(self):
        _, source, cache = self.perfil_sintetico()
        module.instalar_perfil(str(source), "eu.sytue.contacts")
        installed = cache / "11111111-2222-3333-4444-555555555555.mobileprovision"
        installed.rename(cache / "saved-copy.mobileprovision")
        installed.write_bytes(b"user replacement, do not delete")
        installed.chmod(0o600)
        with self.assertRaises(module.ErrorLlavero):
            module.limpiar()
        self.assertEqual(installed.read_bytes(), b"user replacement, do not delete")

    def test_ruta_real_workflow_y_selector_encaja_con_instalador_privado(self):
        import datetime
        import plistlib
        import runpy
        import subprocess
        import sys
        import textwrap

        state, _, cache = self.perfil_sintetico()
        payload = self.root / "profile-fixture.plist"
        payload.write_bytes(plistlib.dumps({
            "CreationDate": datetime.datetime(2020, 1, 1),
            "ExpirationDate": datetime.datetime.now() + datetime.timedelta(days=1),
            "Name": "Existing synthetic host profile", "Platform": ["iOS"],
            "Entitlements": {"application-identifier": "ABCDE12345.eu.sytue.contacts"},
        }))
        workspace = self.root / "workflow-workspace"
        scripts = workspace / "apps/fyne/scripts"
        scripts.mkdir(parents=True)
        (scripts / "empaquetar.sh").write_text("#!/bin/sh\necho no\n")
        proxy = self.root / "profile-boundary.py"
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
                # Only the cryptographic decode boundary is synthetic here.
                sys.stdout.buffer.write(Path(args[2]).read_bytes())
            elif args[0].endswith("apple_llavero.py") and args[1] == "instalar-perfil":
                # Execute the real installer below with this exact CLI request
                # while the in-process Security and Path.home mocks are active.
                Path(os.environ["PROFILE_INSTALL_REQUEST"]).write_text(json.dumps(args[2:]))
            elif args[0] == "-c":
                os.execv(sys.executable, [sys.executable, *args])
            else:
                raise SystemExit("unexpected entrypoint at synthetic profile boundary")
            '''))
        binary = self.root / "profile-bin"
        binary.mkdir()
        python = binary / "python3"
        python.write_text('#!/bin/bash\nexec "$PYTHON_REAL" "$PROFILE_BOUNDARY" "$@"\n')
        python.chmod(0o700)
        request = self.root / "profile-install-request.json"
        env = dict(os.environ, PATH=str(binary) + os.pathsep + os.environ["PATH"],
                   PYTHON_REAL=sys.executable, PROFILE_BOUNDARY=str(proxy), PROFILE_PAYLOAD=str(payload),
                   PROFILE_INSTALL_REQUEST=str(request),
                   SYTUE_APPLE_JOB_DIR=str(state.parent), SYTUE_SOLO_MAS="false", SYTUE_SOLO_IOS="true",
                   SYTUE_PREPARAR_PERFILES_WIDGETS_SOLO="false", SYTUE_APPS_IOS_SELECCIONADAS="contacts",
                   GITHUB_ENV=str(self.env_file))
        workflow = runpy.run_path(str(SCRIPT.with_name("apple-perfiles-workflow-test.py")))
        result = subprocess.run(["bash", "-c", workflow["paso"]("Perfiles de tienda de apps y widgets (solo con los widgets de Apple)")],
                                cwd=workspace, env=env, capture_output=True, text=True)
        self.assertEqual(result.returncode, 0, result.stderr)
        exported = dict(line.split("=", 1) for line in self.env_file.read_text().splitlines())
        env["SYTUE_PERFILES_DIR"] = exported["SYTUE_PERFILES_DIR"]
        selected = subprocess.run(["bash", str(SCRIPT.with_name("apple-firma.sh")), "instalar-perfil-ios", "eu.sytue.contacts"],
                                  env=env, capture_output=True, text=True)
        self.assertEqual(selected.returncode, 0, selected.stderr)
        self.assertEqual(selected.stdout, "")
        source_path, appid = json.loads(request.read_text())
        source = Path(source_path)
        self.assertEqual(source.parent, state.parent / "perfiles-apple")
        self.assertEqual(appid, "eu.sytue.contacts")
        module.instalar_perfil(source_path, appid)
        installed = cache / "11111111-2222-3333-4444-555555555555.mobileprovision"
        self.assertEqual(installed.read_bytes(), payload.read_bytes())
        module.limpiar()
        self.assertFalse(cache.exists())

    def test_no_delega_creacion_de_recursos_a_la_cli(self):
        self.crear()
        for arguments in (["certificates", "create"], ["fetch-signing-files", "eu.sytue.contacts", "--create"],
                          ["certificates", "list", "--p12-password", "caller-controlled"]):
            with self.subTest(action=arguments[:2]):
                with self.assertRaises(module.ErrorLlavero):
                    module.ejecutar_cli(arguments)

    def test_cli_lectora_confina_destinos_y_no_vuelca_material(self):
        state = self.crear()
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
        for command in (["certificates", "list", "--save"], ["fetch-signing-files", "eu.sytue.contacts"]):
            arguments = [*command, "--certificates-dir", str(folder), "--certificate-key", "@env:CERTIFICATE_PRIVATE_KEY"]
            with self.subTest(action=command[:2]):
                out, err = io.StringIO(), io.StringIO()
                with mock.patch.dict(os.environ, {"CERTIFICATE_PRIVATE_KEY": key}), \
                        contextlib.redirect_stdout(out), contextlib.redirect_stderr(err), \
                        mock.patch.object(module.subprocess, "run", return_value=SimpleNamespace(returncode=0, stdout=b"private fixture", stderr=b"private fixture")) as run:
                    module.ejecutar_cli(arguments)
                argv = run.call_args.args[0]
                self.assertEqual(argv[0], str(program))
                self.assertEqual(argv[argv.index("--profiles-dir") + 1], str(state.parent / "perfiles-apple"))
                self.assertEqual(argv[argv.index("--p12-password") + 1], password)
                self.assertEqual(run.call_args.kwargs["env"]["CERTIFICATE_PRIVATE_KEY"], key)
                self.assertNotIn(key, " ".join(argv))
                self.assertEqual((out.getvalue(), err.getvalue()), ("", ""))
        module.limpiar()

    def test_la_frontera_real_rechaza_plataforma_no_macos(self):
        with mock.patch.object(module.sys, "platform", "linux"):
            # Recuperar la función real; ningún flag de producto permite omitirla.
            real_spec = importlib.util.spec_from_file_location("apple_llavero_frontera_real", SCRIPT)
            real = importlib.util.module_from_spec(real_spec)
            real_spec.loader.exec_module(real)
            with self.assertRaises(real.ErrorLlavero):
                real.entorno()


class ClaveFirma(unittest.TestCase):
    def test_referencias_directas_y_codificadas_no_se_resuelven(self):
        import base64
        from apple_clave import normalizar_clave_firma
        for value in ("@file:/fixture/no-leer", "@env:NO_LEER", base64.b64encode(b"@file:/fixture/no-leer").decode()):
            with self.subTest(referenceType=value.split(":", 1)[0]):
                with mock.patch("builtins.open", side_effect=AssertionError("no abrir referencias")):
                    with self.assertRaises(ValueError):
                        normalizar_clave_firma(value)

    def test_normaliza_clave_rsa_pem_y_base64_sin_delimitador_literal_en_fuente(self):
        import base64
        from apple_clave import normalizar_clave_firma
        fuente = SCRIPT.with_name("apple_clave.py").read_text(encoding="utf-8")
        for tipo in ("RSA PRIVATE KEY", "EC PRIVATE KEY"):
            self.assertNotIn(f"-----BEGIN {tipo}-----", fuente)
            self.assertNotIn(f"-----END {tipo}-----", fuente)
        tipo_rsa = "RSA PRIVATE KEY"
        cuerpo = base64.b64encode(b"material-sintetico-de-prueba-para-normalizar-columnas-64-exactas-rsa").decode("ascii")
        cruda = f"  -----BEGIN {tipo_rsa}-----\\r\\n {cuerpo[:40]}  {cuerpo[40:]} \\n-----END {tipo_rsa}-----  "
        esperada = (
            f"-----BEGIN {tipo_rsa}-----\n"
            + "\n".join(cuerpo[i : i + 64] for i in range(0, len(cuerpo), 64))
            + f"\n-----END {tipo_rsa}-----\n"
        )
        self.assertEqual(normalizar_clave_firma(cruda), esperada)
        self.assertEqual(normalizar_clave_firma(base64.b64encode(esperada.encode("utf-8")).decode("ascii")), esperada)


if __name__ == "__main__":
    unittest.main(verbosity=2)
