#!/usr/bin/env python3
# SPDX-License-Identifier: Apache-2.0
# Copyright (c) 2026 Daniel Delgado Ramos
"""Perfiles CMS Apple verificados sin consultar ni importar a ningún llavero."""
import datetime
import fnmatch
import hashlib
import os
import pathlib
import plistlib
import re
import shutil
import ssl
import stat
import subprocess
import sys
import tempfile
import time
import tomllib

AQUI = pathlib.Path(__file__).resolve().parent
RAIZ_APPLE = AQUI.parents[2] / ".github/actions/apple/AppleRootCA.cer"
HUELLA_RAIZ = "b0b1730ecbc7ff4505142c49f1295e6eda6bcaed7e2c68c5be91b5a11001f024"
LIMITE = 2 * 1024 * 1024
FIRMANTES = {
    "ios": "subject=C=US,O=Apple Inc.,CN=Apple iPhone OS Provisioning Profile Signing",
    "macappstore": "subject=C=US,O=Apple Inc.,CN=Mac OS X Provisioning Profile Signing",
    "developerid": "subject=C=US,O=Apple Inc.,CN=Mac OS X Provisioning Profile Signing",
}


class PerfilInvalido(ValueError):
    pass


def exigir(condicion, motivo):
    if not condicion:
        raise PerfilInvalido(motivo)


def leer_limitado(ruta, limite=LIMITE):
    """Sólo ficheros regulares: ni enlaces ni dispositivos ni lectura sin límite."""
    fd = os.open(ruta, os.O_RDONLY | os.O_NOFOLLOW | os.O_NONBLOCK)
    with os.fdopen(fd, "rb") as archivo:
        dato = os.fstat(archivo.fileno())
        exigir(stat.S_ISREG(dato.st_mode) and 0 < dato.st_size <= limite, "profile_input_invalid")
        contenido = archivo.read(limite + 1)
        exigir(len(contenido) == dato.st_size, "profile_input_changed_or_over_bound")
        return contenido


def nodo(datos, inicio=0):
    """Límites DER estrictos; OpenSSL realiza toda la verificación criptográfica."""
    exigir(inicio + 2 <= len(datos), "cms_der_invalid")
    etiqueta, longitud = datos[inicio:inicio + 2]
    cursor = inicio + 2
    if longitud & 0x80:
        cantidad = longitud & 0x7f
        exigir(1 <= cantidad <= 4 and cursor + cantidad <= len(datos), "cms_der_invalid")
        exigir(datos[cursor] != 0, "cms_der_nonminimal")
        longitud = int.from_bytes(datos[cursor:cursor + cantidad], "big")
        exigir(longitud >= 128, "cms_der_nonminimal")
        cursor += cantidad
    fin = cursor + longitud
    exigir(fin <= len(datos), "cms_der_invalid")
    return etiqueta, datos[cursor:fin], fin


def hijos(datos, maximo=16):
    resultado, cursor = [], 0
    while cursor < len(datos):
        exigir(len(resultado) < maximo, "cms_der_too_many_elements")
        etiqueta, contenido, cursor = nodo(datos, cursor)
        resultado.append((etiqueta, contenido))
    return resultado


def contenido_firmado(datos):
    """CMS SignedData con un firmante y contenido adjunto, sin datos posteriores."""
    etiqueta, contenido, fin = nodo(datos)
    exigir(etiqueta == 0x30 and fin == len(datos), "cms_content_info_invalid_or_trailing")
    exterior = hijos(contenido)
    exigir(len(exterior) == 2 and exterior[0] == (0x06, bytes.fromhex("2a864886f70d010702"))
            and exterior[1][0] == 0xa0, "cms_signed_data_required")
    envuelto = hijos(exterior[1][1])
    exigir(len(envuelto) == 1 and envuelto[0][0] == 0x30, "cms_signed_data_invalid")
    firmado = hijos(envuelto[0][1])
    exigir(4 <= len(firmado) <= 6 and [e[0] for e in firmado[:3]] == [0x02, 0x31, 0x30]
            and firmado[-1][0] == 0x31 and [e[0] for e in firmado[3:-1]] in ([], [0xa0], [0xa1], [0xa0, 0xa1]),
            "cms_signed_data_invalid")
    firmantes = hijos(firmado[-1][1])
    exigir(len(firmantes) == 1 and firmantes[0][0] == 0x30, "cms_exactly_one_signer_required")
    adjunto = hijos(firmado[2][1])
    exigir(len(adjunto) == 2 and adjunto[0] == (0x06, bytes.fromhex("2a864886f70d010701"))
            and adjunto[1][0] == 0xa0, "cms_attached_content_required")
    octetos = hijos(adjunto[1][1])
    exigir(len(octetos) == 1 and octetos[0][0] == 0x04 and 0 < len(octetos[0][1]) <= LIMITE,
            "cms_attached_content_invalid")
    return octetos[0][1]


def escribir_privado(ruta, contenido):
    fd = os.open(ruta, os.O_WRONLY | os.O_CREAT | os.O_EXCL | os.O_NOFOLLOW, 0o600)
    with os.fdopen(fd, "wb") as archivo:
        archivo.write(contenido)


def _decodificar_con_raiz(datos, plataforma, raiz_der):
    """Primitiva interna para pruebas con CA sintética; producción fija su raíz."""
    exigir(plataforma in FIRMANTES and isinstance(datos, bytes) and 0 < len(datos) <= LIMITE,
            "profile_input_or_platform_invalid")
    contenido = contenido_firmado(datos)
    candidato = os.environ.get("SYTUE_OPENSSL") or shutil.which("openssl")
    exigir(candidato and pathlib.Path(candidato).is_absolute(), "openssl3_required")
    ejecutable = pathlib.Path(candidato).resolve(strict=True)
    identidad = ejecutable.stat()
    exigir(stat.S_ISREG(identidad.st_mode) and not stat.S_IMODE(identidad.st_mode) & 0o022
            and os.access(ejecutable, os.X_OK), "openssl3_required")
    firma_binario = (identidad.st_dev, identidad.st_ino, identidad.st_size, identidad.st_mtime_ns)
    plazo = time.monotonic() + 25
    with tempfile.TemporaryDirectory(prefix="sytue-profile-cms-") as temporal:
        base = pathlib.Path(temporal)
        exigir(stat.S_IMODE(base.stat().st_mode) == 0o700, "private_cms_directory_required")
        configuracion, raiz, firmado, claro = (base / nombre for nombre in
                                              ("openssl.cnf", "root.pem", "signer.pem", "profile.plist"))
        escribir_privado(configuracion, b"")
        escribir_privado(raiz, ssl.DER_cert_to_PEM_cert(raiz_der).encode("ascii"))
        escribir_privado(firmado, b"")
        escribir_privado(claro, b"")
        inodos = {p: (p.stat().st_dev, p.stat().st_ino) for p in (firmado, claro)}
        env = dict(os.environ, OPENSSL_CONF=str(configuracion))

        def ejecutar(argumentos, entrada=None):
            actual = ejecutable.stat()
            exigir((actual.st_dev, actual.st_ino, actual.st_size, actual.st_mtime_ns) == firma_binario,
                    "openssl_executable_changed")
            restante = plazo - time.monotonic()
            exigir(restante > 0, "cms_verification_timeout")
            try:
                resultado = subprocess.run([str(ejecutable), *argumentos], input=entrada, env=env,
                                           capture_output=True, timeout=min(10, restante), check=False)
            except subprocess.TimeoutExpired:
                raise PerfilInvalido("cms_verification_timeout") from None
            exigir(resultado.returncode == 0 and len(resultado.stdout) <= 16384,
                    "cms_signature_chain_or_tool_verification_failed")
            return resultado.stdout

        version = ejecutar(["version"])
        exigir(re.fullmatch(rb"OpenSSL 3\.\d+\.\d+[^\r\n]*\r?\n?", version) is not None, "openssl3_required")
        ejecutar(["cms", "-verify", "-inform", "DER", "-CAfile", str(raiz),
                  "-no-CApath", "-no-CAstore", "-purpose", "any", "-signer", str(firmado),
                  "-out", str(claro)], datos)
        for ruta in (firmado, claro):
            estado = ruta.lstat()
            exigir(stat.S_ISREG(estado.st_mode) and estado.st_uid == os.getuid() and estado.st_nlink == 1
                    and stat.S_IMODE(estado.st_mode) == 0o600
                    and (estado.st_dev, estado.st_ino) == inodos[ruta], "private_cms_output_required")
        firmante = leer_limitado(firmado, 65536)
        exigir(re.fullmatch(rb"-----BEGIN CERTIFICATE-----\r?\n[A-Za-z0-9+/=\r\n]+-----END CERTIFICATE-----\r?\n?", firmante)
                is not None, "cms_exactly_one_signer_certificate_required")
        sujeto = ejecutar(["x509", "-in", str(firmado), "-noout", "-subject", "-nameopt", "RFC2253"])
        exigir(sujeto.decode("utf8").strip() == FIRMANTES[plataforma], "cms_official_profile_signer_required")
        verificado = leer_limitado(claro)
        exigir(verificado == contenido, "cms_verified_content_differs")
        try:
            perfil = plistlib.loads(verificado)
        except (ValueError, TypeError, plistlib.InvalidFileException):
            raise PerfilInvalido("cms_profile_plist_invalid") from None
        exigir(isinstance(perfil, dict) and isinstance(perfil.get("Entitlements"), dict)
                and isinstance(perfil.get("Platform"), list)
                and ("iOS" if plataforma == "ios" else "OSX") in perfil["Platform"], "cms_profile_platform_invalid")
        directo = perfil.get("ProvisionsAllDevices", False)
        exigir(type(directo) is bool and directo == (plataforma == "developerid"), "cms_profile_distribution_invalid")
        return perfil


def decodificar(datos, plataforma):
    raiz = leer_limitado(RAIZ_APPLE, 65536)
    exigir(hashlib.sha256(raiz).hexdigest() == HUELLA_RAIZ, "pinned_apple_root_invalid")
    return _decodificar_con_raiz(datos, plataforma, raiz)


def desde_archivo(ruta, plataforma):
    return decodificar(leer_limitado(ruta), plataforma)


def validar_derechos(perfil, appid, modo, grupo, proteccion="", llavero="", solicitados="", fuente=None):
    """Las guardas semánticas originales, después de autenticar el CMS."""
    fuente = fuente or AQUI / "../../nucleo/ajustes/grupo_apple.go"
    def mal(motivo):
        raise PerfilInvalido(motivo)
    if modo not in ('ios', 'macappstore', 'developerid') or not re.fullmatch(r'eu\.sytue\.[a-z0-9.]+', appid):
        mal('AppID o plataforma no admitidos')
    if not re.fullmatch(r'group\.[a-z0-9.]+|[A-Z0-9]{10}\.[a-z0-9.]+', grupo):
        mal('grupo no admitido')
    if proteccion and (modo != 'ios' or proteccion != 'NSFileProtectionComplete'):
        mal('protección no admitida')
    try:
        grupos_llavero = re.findall(r'^const GrupoSuite = "([A-Z0-9]{10}\.[a-z0-9.]+)"$', pathlib.Path(fuente).read_text(), re.M)
        if len(grupos_llavero) != 1:
            mal('no hay un equipo canónico en ajustes/grupo_apple.go')
        equipo = grupos_llavero[0].split('.', 1)[0]
        llavero_proveedor = ""
        if appid.endswith('.archivos'):
            host = appid.removesuffix('.archivos')
            catalogo = pathlib.Path(fuente).resolve().parents[2] / 'fyne/cmd'
            declarados = [tomllib.loads(p.read_text()) for p in catalogo.glob('*/FyneApp.toml')]
            if not any(a.get('Details', {}).get('ID') == host and a.get('Development', {}).get('archivos') == 'si' for a in declarados):
                mal('el catálogo no declara este proveedor de archivos')
            llavero_proveedor = equipo + '.' + host + '.shared'
            if grupo != 'group.' + host or llavero != llavero_proveedor:
                mal('el proveedor exige su propio AppGroup y Keychain')
        elif llavero and llavero != grupos_llavero[0]:
            mal('el grupo de Keychain no es el canónico de la suite')
        derechos = perfil['Entitlements']
        equipo = grupos_llavero[0].split('.', 1)[0]
        if perfil.get('TeamIdentifier') != [equipo]:
            mal('el equipo no es el de la suite')
        clave = 'application-identifier' if modo == 'ios' else 'com.apple.application-identifier'
        if derechos.get(clave) != equipo + '.' + appid:
            mal('el AppID no coincide exactamente')
        plataformas = perfil.get('Platform', [])
        if not isinstance(plataformas, list) or ('iOS' if modo == 'ios' else 'OSX') not in plataformas:
            mal('la plataforma del perfil no coincide')
        caduca = perfil['ExpirationDate'].replace(tzinfo=datetime.timezone.utc)
        if caduca <= datetime.datetime.now(datetime.timezone.utc):
            mal('el perfil ha caducado')
        if 'ProvisionedDevices' in perfil or derechos.get('get-task-allow') or derechos.get('com.apple.security.get-task-allow'):
            mal('se requiere distribución, sin dispositivos de desarrollo')
        directo = perfil.get('ProvisionsAllDevices', False)
        if not isinstance(directo, bool) or directo != (modo == 'developerid'):
            mal('el tipo de distribución no coincide')
        grupos = derechos.get('com.apple.security.application-groups', [])
        if not isinstance(grupos, list) or grupo not in grupos:
            mal('el perfil no autoriza el AppGroup: registrar/asociar el grupo y regenerar el perfil')
        if proteccion and derechos.get('com.apple.developer.default-data-protection') != proteccion:
            mal('el perfil no autoriza Data Protection Complete')
        if llavero:
            permitidos = derechos.get('keychain-access-groups', [])
            if not isinstance(permitidos, list) or not any(isinstance(p, str) and fnmatch.fnmatchcase(llavero, p) for p in permitidos):
                mal('el perfil no autoriza el grupo de Keychain de la sesión')
        if solicitados:
            peticion = plistlib.loads(pathlib.Path(solicitados).read_bytes())
            grupos_pedidos = peticion.get('com.apple.security.application-groups')
            claves_pedidas = peticion.get('keychain-access-groups', [])
            if not isinstance(grupos_pedidos, list) or grupo not in grupos_pedidos or not all(isinstance(g, str) for g in grupos_pedidos):
                mal('los derechos no declaran los AppGroups esperados')
            if not isinstance(claves_pedidas, list) or not all(isinstance(g, str) for g in claves_pedidas):
                mal('los derechos de Keychain no son una lista válida')
            if llavero and llavero not in claves_pedidas:
                mal('los derechos del host omiten el Keychain de la suite')
            if appid.endswith('.widgets') and claves_pedidas:
                mal('los widgets no pueden solicitar acceso al Keychain')
            if llavero_proveedor and claves_pedidas != [llavero_proveedor]:
                mal('File Provider no puede leer el Keychain de la sesión ni de otra app')
            for pedido in grupos_pedidos:
                # macOS autoriza también grupos <TeamID>.* mediante la firma del
                # mismo equipo. El grupo legado se conserva sólo para migrar MAS.
                legado_mac = modo != 'ios' and pedido == grupos_llavero[0]
                if not legado_mac and pedido not in grupos:
                    mal('el perfil no autoriza todos los AppGroups solicitados')
            permitidos = derechos.get('keychain-access-groups', [])
            if not isinstance(permitidos, list) or any(not any(isinstance(p, str) and fnmatch.fnmatchcase(g, p) for p in permitidos) for g in claves_pedidas):
                mal('el perfil no autoriza todos los grupos de Keychain solicitados')
    except PerfilInvalido:
        raise
    except (OSError, ValueError, KeyError, TypeError, AttributeError):
        mal('el perfil o la configuración no son válidos')


def main(argv=None):
    argumentos = sys.argv[1:] if argv is None else argv
    try:
        exigir(bool(argumentos), "profile_command_required")
        if argumentos[0] == "decodificar":
            exigir(len(argumentos) == 3, "profile_decode_arguments_invalid")
            perfil = desde_archivo(argumentos[1], argumentos[2])
            sys.stdout.buffer.write(plistlib.dumps(perfil))
        elif argumentos[0] == "verificar":
            exigir(5 <= len(argumentos) <= 8, "profile_verify_arguments_invalid")
            ruta, appid, plataforma, grupo = argumentos[1:5]
            opcionales = argumentos[5:] + [""] * (8 - len(argumentos))
            perfil = desde_archivo(ruta, plataforma)
            validar_derechos(perfil, appid, plataforma, grupo, *opcionales)
            print("perfil válido: " + appid + " (" + plataforma + ")")
        else:
            raise PerfilInvalido("profile_command_invalid")
        return 0
    except PerfilInvalido as error:
        mensaje = "se requiere OpenSSL 3 existente" if str(error) == "openssl3_required" else "perfil CMS o derechos no válidos"
        print("::error::" + mensaje, file=sys.stderr)
        return 1
    except Exception:
        # Un error de OpenSSL, del fichero o de la configuración nunca publica
        # certificados, payload, rutas ni detalles no controlados de terceros.
        print("::error::perfil CMS o derechos no válidos", file=sys.stderr)
        return 1


if __name__ == "__main__":
    sys.exit(main())
