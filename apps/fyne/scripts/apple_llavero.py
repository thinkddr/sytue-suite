#!/usr/bin/env python3
# SPDX-License-Identifier: Apache-2.0
# Copyright (c) 2026 Daniel Delgado Ramos
"""Llavero de UN job: ninguna escritura al default o a la lista de búsqueda.

La creación/borrado nativos sólo pueden añadir/retirar nuestra propia entrada.
No lee contenidos de llaveros personales ni crea recursos en la cuenta Apple.
"""
import argparse
import errno
import hashlib
import json
import os
from pathlib import Path
import re
import secrets
import shlex
import shutil
import stat
import ssl
import subprocess
import sys
import tempfile


class ErrorLlavero(Exception):
    pass


HUELLAS_PUBLICAS = {
    "AppleRootCA.cer": "b0b1730ecbc7ff4505142c49f1295e6eda6bcaed7e2c68c5be91b5a11001f024",
    "DeveloperIDCA.cer": "7afc9d01a62f03a2de9637936d4afe68090d2de18d03f29c88cfb0b1ba63587f",
    "DeveloperIDG2CA.cer": "f16cd3c54c7f83cea4bf1a3e6a0819c8aaa8e4a1528fd144715f350643d2df3a",
    "AppleWWDRCAG3.cer": "dcf21878c77f4198e4b4614f03d696d89c66c66008d4244e1b99161aac91601f",
}


def exigir(valor, motivo):
    if not valor:
        raise ErrorLlavero(motivo)


def propietario(ruta, directorio=False, privado=False):
    dato = ruta.lstat()
    tipo = stat.S_ISDIR if directorio else stat.S_ISREG
    exigir(tipo(dato.st_mode) and dato.st_uid == os.getuid() and
           not stat.S_IMODE(dato.st_mode) & 0o022, "ruta_propia_invalida")
    if not directorio:
        exigir(dato.st_nlink == 1, "archivo_con_enlaces")
    if privado:
        exigir(stat.S_IMODE(dato.st_mode) == (0o700 if directorio else 0o600), "permisos_privados_invalidos")
    return [dato.st_dev, dato.st_ino]


def herramienta(argumentos, entrada=None, vacio_no_encontrado=False):
    # Sólo programas nativos con destino explícito. Nunca mostrar stderr/argv:
    # algunos comandos reciben la contraseña aleatoria de NUESTRO llavero.
    try:
        salida = subprocess.run(argumentos, input=entrada, capture_output=True, timeout=30, check=False)
    except (OSError, subprocess.TimeoutExpired):
        raise ErrorLlavero("herramienta_no_disponible") from None
    if vacio_no_encontrado and salida.returncode == 44 and salida.stdout == b"":
        return b""
    exigir(salida.returncode == 0 and len(salida.stdout) <= 1048576, "herramienta_del_llavero_fallo")
    return salida.stdout


def metadatos():
    busqueda = shlex.split(herramienta(["/usr/bin/security", "list-keychains", "-d", "user"]).decode())
    defaults = [shlex.split(herramienta(["/usr/bin/security", "default-keychain", *dominio]).decode())
                for dominio in ([], ["-d", "user"])]
    exigir(all(Path(p).is_absolute() for p in busqueda) and
           all(len(items) == 1 and Path(items[0]).is_absolute() for items in defaults), "metadatos_incompletos")
    return {"busqueda": busqueda, "defaults": [items[0] for items in defaults]}


def archivo_nuevo(ruta, datos):
    fd = os.open(ruta, os.O_WRONLY | os.O_CREAT | os.O_EXCL | os.O_NOFOLLOW, 0o600)
    with os.fdopen(fd, "wb") as fichero:
        fichero.write(datos)
        fichero.flush()
        os.fsync(fichero.fileno())
    return propietario(ruta, privado=True)


def json_bytes(valor):
    return json.dumps(valor, sort_keys=True).encode() + b"\n"


def entorno():
    exigir(sys.platform == "darwin" and os.getuid() != 0 and os.getuid() == os.geteuid(), "mac_sin_privilegios_requerido")
    run, intento = os.environ.get("GITHUB_RUN_ID", ""), os.environ.get("GITHUB_RUN_ATTEMPT", "")
    exigir(re.fullmatch(r"[0-9]+", run) and re.fullmatch(r"[0-9]+", intento), "identidad_job_invalida")
    temporal = Path(os.environ["RUNNER_TEMP"])
    exigir(temporal.is_absolute() and not temporal.is_symlink(), "temporal_invalido")
    temporal = temporal.resolve(strict=True)
    propietario(temporal, directorio=True)
    return temporal, run, intento


def guardar_estado(ruta, estado, identidad):
    exigir(propietario(ruta, privado=True) == identidad, "estado_sustituido")
    temporal = ruta.with_name(".estado-" + secrets.token_hex(12))
    archivo_nuevo(temporal, json_bytes(estado))
    exigir(propietario(ruta, privado=True) == identidad, "estado_sustituido")
    os.replace(temporal, ruta)
    return propietario(ruta, privado=True)


def cargar():
    temporal, run, intento = entorno()
    ruta = Path(os.environ["SYTUE_APPLE_SIGNING_STATE"])
    exigir(ruta.name == "estado.json" and ruta.parent.parent == temporal and
           re.fullmatch("apple-signing-" + run + "-" + intento + r"-[a-z0-9_]+", ruta.parent.name), "estado_fuera_del_job")
    exigir(os.environ.get("SYTUE_APPLE_JOB_DIR") == str(ruta.parent), "directorio_job_no_corresponde")
    propietario(ruta.parent, directorio=True, privado=True)
    identidad = propietario(ruta, privado=True)
    exigir(ruta.stat().st_size <= 32768, "estado_demasiado_grande")
    estado = json.loads(ruta.read_bytes())
    exigir(isinstance(estado, dict) and estado.get("schemaVersion") == 1 and estado.get("runID") == run and
           estado.get("attempt") == intento and estado.get("ownerUID") == os.getuid() and
           estado.get("rootIdentity") == propietario(ruta.parent, directorio=True, privado=True) and
           re.fullmatch(r"[a-f0-9]{64}", estado.get("password", "")), "procedencia_estado_invalida")
    return ruta, estado, identidad, ruta.parent / "firma.keychain-db"


def validar_llavero(estado, ruta):
    exigir(estado.get("keychainIdentity") == propietario(ruta, privado=True), "llavero_sustituido")
    actual = metadatos()
    exigir(str(ruta) not in actual["defaults"], "llavero_propio_es_default_detener")
    return actual


def escritura_nativa(ruta, estado, identidad, llavero, argumentos):
    validar_llavero(estado, llavero)
    try:
        herramienta(argumentos)
    finally:
        # También una importación parcialmente fallida puede reemplazar el DB.
        estado["keychainIdentity"] = propietario(llavero, privado=True)
        identidad = guardar_estado(ruta, estado, identidad)
        validar_llavero(estado, llavero)
    return identidad


def verificar_publicos(archivos):
    exigir(archivos and len(archivos) <= len(HUELLAS_PUBLICAS), "certificados_publicos_invalidos")
    for archivo in archivos:
        exigir(archivo.name in HUELLAS_PUBLICAS and archivo.is_file() and not archivo.is_symlink() and
               hashlib.sha256(archivo.read_bytes()).hexdigest() == HUELLAS_PUBLICAS[archivo.name], "certificado_publico_no_fijado")


def publicos(archivos):
    verificar_publicos(archivos)
    ruta, estado, identidad, llavero = cargar()
    validar_llavero(estado, llavero)
    pem = herramienta(["/usr/bin/security", "find-certificate", "-a", "-p", str(llavero)], vacio_no_encontrado=True).decode("ascii")
    existentes = {hashlib.sha256(ssl.PEM_cert_to_DER_cert(p)).hexdigest()
                  for p in re.findall(r"-----BEGIN CERTIFICATE-----.*?-----END CERTIFICATE-----", pem, re.S)}
    for archivo in archivos:
        if HUELLAS_PUBLICAS[archivo.name] in existentes:
            continue
        identidad = escritura_nativa(ruta, estado, identidad, llavero,
                                     ["/usr/bin/security", "add-certificates", "-k", str(llavero), str(archivo)])


def crear():
    temporal, run, intento = entorno()
    raiz = Path(tempfile.mkdtemp(prefix="apple-signing-" + run + "-" + intento + "-", dir=temporal))
    raiz_id = propietario(raiz, directorio=True, privado=True)
    antes = metadatos()
    estado = {"schemaVersion": 1, "runID": run, "attempt": intento, "ownerUID": os.getuid(),
              "rootIdentity": raiz_id, "password": secrets.token_hex(32), "before": antes, "keychainIdentity": None}
    ruta = raiz / "estado.json"
    identidad = archivo_nuevo(ruta, json_bytes(estado))
    # Publicar sólo la RUTA del estado antes de crear: always() puede limpiar
    # incluso si un paso posterior falla. Nunca exportar la contraseña.
    destino_env = Path(os.environ["GITHUB_ENV"])
    propietario(destino_env)
    with os.fdopen(os.open(destino_env, os.O_WRONLY | os.O_APPEND | os.O_NOFOLLOW), "a") as fichero:
        fichero.write("SYTUE_APPLE_SIGNING_STATE=" + str(ruta) + "\n")
        fichero.write("SYTUE_APPLE_JOB_DIR=" + str(raiz) + "\n")
    llavero = raiz / "firma.keychain-db"
    try:
        herramienta(["/usr/bin/security", "create-keychain", "-p", estado["password"], str(llavero)])
    finally:
        if os.path.lexists(llavero):
            estado["keychainIdentity"] = propietario(llavero, privado=True)
            identidad = guardar_estado(ruta, estado, identidad)
    actual = validar_llavero(estado, llavero)
    exigir(actual["defaults"] == antes["defaults"] and actual["busqueda"].count(str(llavero)) <= 1 and
           [p for p in actual["busqueda"] if p != str(llavero)] == antes["busqueda"], "metadatos_ajenos_cambiaron")
    herramienta(["/usr/bin/security", "set-keychain-settings", "-lut", "14400", str(llavero)])
    # Security puede reemplazar atómicamente el DB en una escritura nuestra.
    # Actualizar la identidad sólo inmediatamente después de ESA operación;
    # una sustitución posterior sin una operación controlada sigue rechazándose.
    estado["keychainIdentity"] = propietario(llavero, privado=True)
    identidad = guardar_estado(ruta, estado, identidad)
    validar_llavero(estado, llavero)
    herramienta(["/usr/bin/security", "unlock-keychain", "-p", estado["password"], str(llavero)])
    estado["keychainIdentity"] = propietario(llavero, privado=True)
    identidad = guardar_estado(ruta, estado, identidad)
    final = validar_llavero(estado, llavero)
    exigir(final["defaults"] == antes["defaults"] and
           [p for p in final["busqueda"] if p != str(llavero)] == antes["busqueda"], "metadatos_ajenos_cambiaron")
    print(json.dumps({"status": "own_keychain_created", "jobDir": str(raiz), "defaultPreserved": True, "foreignSearchListPreserved": True}))


def ejecutar_cli(argumentos):
    ruta, estado, _, llavero = cargar()
    validar_llavero(estado, llavero)
    exigir(argumentos and (argumentos[0] == "fetch-signing-files" or argumentos[:2] == ["certificates", "list"]), "solo_lecturas_de_firma_existente")
    exigir(not any(value in ("--create", "--delete-stale-profiles", "--p12-password") or
                   value.startswith(("--create=", "--delete-stale-profiles=", "--p12-password=")) for value in argumentos),
           "creacion_o_password_externo_prohibido")
    opciones = {"--type", "--platform", "--certificates-dir", "--certificate-key", "--strict-match-identifier", "--save"}
    exigir(all(not value.startswith("-") or value in opciones for value in argumentos), "opcion_cli_fuera_del_contrato")
    exigir(argumentos.count("--certificates-dir") == 1 and argumentos.count("--certificate-key") == 1, "destino_o_clave_no_unicos")
    indice = argumentos.index("--certificates-dir") + 1
    exigir(indice < len(argumentos), "falta_directorio_certificados")
    carpeta = Path(argumentos[indice])
    exigir(carpeta.parent == ruta.parent and re.fullmatch(r"certificates\.[a-zA-Z0-9_]+", carpeta.name), "certificados_fuera_del_job")
    propietario(carpeta, directorio=True, privado=True)
    indice = argumentos.index("--certificate-key") + 1
    exigir(indice < len(argumentos) and argumentos[indice] == "@env:CERTIFICATE_PRIVATE_KEY", "clave_fuera_del_contrato")
    perfiles = ruta.parent / "perfiles-apple"
    perfiles.mkdir(mode=0o700, exist_ok=True)
    propietario(perfiles, directorio=True, privado=True)
    from apple_clave import normalizar_clave_firma
    entorno_cli = dict(os.environ, CERTIFICATE_PRIVATE_KEY=normalizar_clave_firma(os.environ.get("CERTIFICATE_PRIVATE_KEY", "")))
    # La CLI existente exporta el material sólo como P12 cifrado; la clave
    # privada permanece en el contrato @env de la herramienta ya fijada.
    programa = ruta.parent / "cm/bin/app-store-connect"
    propietario(programa)
    try:
        salida = subprocess.run([str(programa), *argumentos, "--profiles-dir", str(perfiles), "--p12-password", estado["password"]],
                                env=entorno_cli, capture_output=True, timeout=240, check=False)
    except (OSError, subprocess.TimeoutExpired):
        raise ErrorLlavero("consulta_de_material_no_disponible") from None
    exigir(salida.returncode == 0, "consulta_de_material_existente_fallo")


def importar(directorio, herramientas):
    ruta, estado, identidad, llavero = cargar()
    validar_llavero(estado, llavero)
    exigir(herramientas and set(herramientas) <= {"codesign", "productsign", "productbuild"}, "herramientas_de_firma_invalidas")
    carpeta = Path(directorio)
    exigir(carpeta.parent == ruta.parent and re.fullmatch(r"certificates\.[a-zA-Z0-9_]+", carpeta.name), "certificados_fuera_del_job")
    propietario(carpeta, directorio=True, privado=True)
    certificados = sorted(carpeta.glob("*.p12"))
    exigir(1 <= len(certificados) <= 8, "cantidad_de_identidades_invalida")
    for certificado in certificados:
        propietario(certificado, privado=True)
        argv = ["/usr/bin/security", "import", str(certificado), "-f", "pkcs12", "-k", str(llavero), "-P", estado["password"]]
        for nombre in herramientas:
            argv += ["-T", "/usr/bin/" + nombre]
        identidad = escritura_nativa(ruta, estado, identidad, llavero, argv)
    escritura_nativa(ruta, estado, identidad, llavero,
                     ["/usr/bin/security", "set-key-partition-list", "-S", "apple-tool:,apple:,codesign:", "-s", "-k", estado["password"], str(llavero)])


def cache_perfiles():
    # Xcode's documented per-user profile cache, not a browser profile. Never
    # override HOME, change its defaults, or overwrite a preexisting file.
    biblioteca = Path.home() / "Library"
    propietario(biblioteca, directorio=True)
    return biblioteca / "Developer/Xcode/UserData/Provisioning Profiles"


def perfil_ios_verificado(fichero, appid):
    from apple_perfil import desde_archivo, validar_derechos
    import tomllib
    fyne = Path(__file__).resolve().parents[1]
    ficha = json.loads((fyne / "appstore/ficha.json").read_bytes())
    ids = {tomllib.loads((fyne / "cmd" / app / "FyneApp.toml").read_text())["Details"]["ID"] for app in ficha["apps"]}
    exigir(appid in ids, "perfil_fuera_del_catalogo")
    perfil = desde_archivo(str(fichero), "ios")
    validar_derechos(perfil, appid, "ios", "group.eu.sytue.suite", "", ficha["equipo"] + ".eu.sytue.suite")
    identificador = perfil.get("UUID", "")
    exigir(isinstance(identificador, str) and re.fullmatch(r"[a-fA-F0-9]{8}(?:-[a-fA-F0-9]{4}){3}-[a-fA-F0-9]{12}", identificador),
           "uuid_perfil_invalido")
    return identificador


def instalar_perfil(fuente, appid):
    ruta, estado, identidad, llavero = cargar()
    validar_llavero(estado, llavero)
    fichero = Path(fuente)
    exigir(fichero.parent == ruta.parent / "perfiles-apple", "perfil_fuera_del_job")
    propietario(fichero.parent, directorio=True, privado=True)
    propietario(fichero, privado=True)
    exigir(fichero.stat().st_size <= 2097152, "perfil_demasiado_grande")
    uuid = perfil_ios_verificado(fichero, appid)
    contenido = fichero.read_bytes()
    digest = hashlib.sha256(contenido).hexdigest()
    destino_dir = cache_perfiles()
    directorios = [destino_dir.parents[2], destino_dir.parents[1], destino_dir.parent, destino_dir]
    for directorio in directorios:
        if not os.path.lexists(directorio):
            directorio.mkdir(mode=0o700)
            identificador = propietario(directorio, directorio=True, privado=True)
            estado.setdefault("createdProfileDirectories", []).append({"path": str(directorio), "identity": identificador})
            identidad = guardar_estado(ruta, estado, identidad)
        else:
            propietario(directorio, directorio=True)
    cache_id = propietario(destino_dir, directorio=True)
    exigir(estado.get("profileCacheIdentity", cache_id) == cache_id, "cache_perfiles_sustituido_conservar")
    estado["profileCacheIdentity"] = cache_id
    identidad = guardar_estado(ruta, estado, identidad)
    destino = destino_dir / (uuid + ".mobileprovision")
    if os.path.lexists(destino):
        propietario(destino)
        exigir(destino.stat().st_size == len(contenido) and destino.read_bytes() == contenido, "perfil_preexistente_distinto_conservar")
        print(json.dumps({"status": "existing_Xcode_profile_reused_without_changes", "createdAppleResources": 0}))
        return
    nuevo_id = archivo_nuevo(destino, contenido)
    estado.setdefault("installedProfiles", []).append({"path": str(destino), "identity": nuevo_id, "sha256": digest})
    try:
        guardar_estado(ruta, estado, identidad)
    except Exception:
        if propietario(destino, privado=True) == nuevo_id and hashlib.sha256(destino.read_bytes()).hexdigest() == digest:
            destino.unlink()
        raise
    print(json.dumps({"status": "existing_profile_temporarily_available_to_Xcode", "createdAppleResources": 0}))


def limpiar_perfiles(estado):
    if not estado.get("installedProfiles") and not estado.get("createdProfileDirectories"):
        return
    destino_dir = cache_perfiles()
    if os.path.lexists(destino_dir):
        for directorio in [destino_dir.parents[2], destino_dir.parents[1], destino_dir.parent, destino_dir]:
            propietario(directorio, directorio=True)
        cache_id = estado.get("profileCacheIdentity")
        if cache_id is None and not estado.get("installedProfiles"):
            # Puede fallar tras registrar el directorio nuevo y antes de guardar
            # profileCacheIdentity. Sólo su identidad registrada permite limpiarlo.
            propios = [entrada["identity"] for entrada in estado.get("createdProfileDirectories", [])
                       if entrada["path"] == str(destino_dir)]
            cache_id = propios[0] if len(propios) == 1 else None
        exigir(propietario(destino_dir, directorio=True) == cache_id, "cache_perfiles_sustituido_conservar")
    for perfil in estado.get("installedProfiles", []):
        destino = Path(perfil["path"])
        exigir(destino.parent == destino_dir and re.fullmatch(r"[a-fA-F0-9-]{36}\.mobileprovision", destino.name), "limpieza_perfil_fuera_del_cache")
        if not os.path.lexists(destino):
            continue
        exigir(propietario(destino, privado=True) == perfil["identity"] and destino.stat().st_size <= 2097152 and
               hashlib.sha256(destino.read_bytes()).hexdigest() == perfil["sha256"], "perfil_cache_sustituido_conservar")
        destino.unlink()
    permitidos = {destino_dir, destino_dir.parent, destino_dir.parents[1], destino_dir.parents[2]}
    for entrada in reversed(estado.get("createdProfileDirectories", [])):
        directorio = Path(entrada["path"])
        if not os.path.lexists(directorio):
            continue
        exigir(directorio in permitidos and propietario(directorio, directorio=True, privado=True) == entrada["identity"],
               "directorio_cache_sustituido_conservar")
        try:
            directorio.rmdir()
        except OSError as error:
            if error.errno != errno.ENOTEMPTY:
                raise


def limpiar():
    ruta, estado, _, llavero = cargar()
    antes = metadatos()
    exigir(str(llavero) not in antes["defaults"], "llavero_propio_es_default_conservar")
    if os.path.lexists(llavero):
        validar_llavero(estado, llavero)
        herramienta(["/usr/bin/security", "delete-keychain", str(llavero)])
    despues = metadatos()
    exigir(despues["defaults"] == antes["defaults"] and
           despues["busqueda"] == [p for p in antes["busqueda"] if p != str(llavero)], "limpieza_cambio_metadatos_ajenos")
    limpiar_perfiles(estado)
    raiz = ruta.parent
    exigir(propietario(raiz, directorio=True, privado=True) == estado["rootIdentity"], "raiz_propia_sustituida")
    # Sólo NUESTRO directorio exclusivo. No seguir enlaces de venv/cachés y
    # nunca borrar ~/Library/codemagic-cli-tools ni perfiles de la persona.
    for carpeta, dirs, nombres in os.walk(raiz, followlinks=False):
        for nombre in dirs + nombres:
            valor = Path(carpeta) / nombre
            dato = valor.lstat()
            exigir(dato.st_uid == os.getuid(), "archivo_ajeno_en_raiz_propia_conservar")
            if stat.S_ISDIR(dato.st_mode):
                valor.chmod(stat.S_IMODE(dato.st_mode) | 0o700)
    shutil.rmtree(raiz)
    print(json.dumps({"status": "own_keychain_and_job_directory_removed", "defaultPreserved": True, "foreignSearchListPreserved": True}))


def main():
    os.umask(0o077)
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("accion", choices=("crear", "ruta", "cli", "importar", "publicos", "instalar-perfil", "limpiar"))
    parser.add_argument("argumentos", nargs=argparse.REMAINDER)
    args = parser.parse_args()
    if args.accion == "crear":
        crear()
    elif args.accion == "limpiar":
        limpiar()
    elif args.accion == "ruta":
        _, estado, _, llavero = cargar()
        validar_llavero(estado, llavero)
        print(str(llavero))
    elif args.accion == "cli":
        ejecutar_cli(args.argumentos)
    elif args.accion == "publicos":
        publicos([Path(p) for p in args.argumentos])
    elif args.accion == "instalar-perfil":
        exigir(len(args.argumentos) == 2, "faltan_perfil_o_appid")
        instalar_perfil(*args.argumentos)
    else:
        exigir(len(args.argumentos) >= 2, "faltan_certificados_o_herramientas")
        importar(args.argumentos[0], args.argumentos[1:])


if __name__ == "__main__":
    try:
        main()
    except Exception as error:
        # Ninguna excepción de una CLI/JSON/FS debe volcar material sensible.
        reason = str(error) if isinstance(error, ErrorLlavero) else type(error).__name__
        print(json.dumps({"status": "owned_keychain_operation_stopped", "reason": reason}), file=sys.stderr)
        raise SystemExit(1)
