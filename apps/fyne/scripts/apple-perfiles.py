#!/usr/bin/env python3
# SPDX-License-Identifier: Apache-2.0
# Copyright (c) 2026 Daniel Delgado Ramos
"""Consulta o prepara perfiles Apple para los hosts y widgets de la suite.

Por defecto sólo consulta la API. --preparar reutiliza una identidad válida del
llavero efímero y crea perfiles, nunca certificados, App IDs ni capacidades.
Antes de prepararlos hay que asociar el App Group real en Developer Portal;
ningún perfil se guarda si no autoriza todos los derechos que se van a firmar.
"""
import argparse
import base64
import datetime
import hashlib
import json
import os
import pathlib
import plistlib
import re
import runpy
import subprocess
import sys
import tempfile
import urllib.parse


AQUI = pathlib.Path(__file__).resolve().parent
REGISTRO = runpy.run_path(str(AQUI / "apple-widgets.py"))
TIPOS = {"ios": "IOS_APP_STORE", "macappstore": "MAC_APP_STORE", "developerid": "MAC_APP_DIRECT"}
CERTIFICADOS = {"ios": ("DISTRIBUTION",), "macappstore": ("DISTRIBUTION",),
                "developerid": ("DEVELOPER_ID_APPLICATION", "DEVELOPER_ID_APPLICATION_G2")}


def capturar(argv):
    # Los perfiles se piden para la firma CON widgets, que es la que van a autorizar:
    # se preparan antes de encender SYTUE_WIDGETS_APPLE en Sytue.xcconfig.
    return subprocess.check_output([str(a) for a in argv], text=True, stderr=subprocess.PIPE,
                                   env=dict(os.environ, SYTUE_WIDGETS_APPLE="1"))


def requisitos(generador, app, plataforma, widget=False):
    """Los derechos del empaquetador/generador son también los del perfil."""
    firma = AQUI / "empaquetar.sh"
    host = AQUI.parent / "cmd" / app["clave"]
    comunes = plistlib.loads(capturar(["bash", firma, "derechos-sesion", host, generador.EQUIPO, "ios"]).encode())
    grupo = comunes["com.apple.security.application-groups"][0]
    llavero = comunes["keychain-access-groups"][1]
    if widget:
        if not hasattr(generador, "derechos_widgets") or not app.get("widget_acciones"):
            raise ValueError(app["clave"] + ": el catálogo no declara un target WidgetKit")
        derechos = plistlib.loads(generador.derechos_widgets(app, "ios" if plataforma == "ios" else "macos").encode())
        identificador = app["id"] + ".widgets"
        llavero = ""
    else:
        if plataforma == "macappstore":
            argv = ["bash", firma, "derechos-tienda", host, generador.EQUIPO]
        else:
            argv = ["bash", firma, "derechos-sesion", host, generador.EQUIPO, plataforma]
        derechos = plistlib.loads(capturar(argv).encode())
        identificador = app["id"]
    if not re.fullmatch(r"eu\.sytue\.[a-z]+(?:\.widgets)?", identificador):
        raise ValueError("el catálogo no declara un App ID de la suite")
    return {"app": app["clave"], "id": identificador, "widget": widget, "group": grupo,
            "keychain": llavero, "entitlements": derechos,
            "protection": derechos.get("com.apple.developer.default-data-protection", "")}


def identidad_importada(plataforma, equipo, elegida=""):
    # El selector compartido rechaza el llavero personal antes de inspeccionarlo.
    llavero = capturar(["bash", AQUI / "apple-firma.sh", "llavero-efimero"]).strip()
    salida = capturar(["security", "find-identity", "-v", "-p", "codesigning", llavero])
    prefijo = "Developer ID Application: " if plataforma == "developerid" else "Apple Distribution: "
    validas = {huella.lower() for huella, nombre in re.findall(r'\b([0-9A-Fa-f]{40})\s+"([^"\n]+)"', salida)
               if nombre.startswith(prefijo) and nombre.endswith("(" + equipo + ")")}
    if elegida:
        elegida = elegida.replace(":", "").lower()
        if not re.fullmatch(r"[0-9a-f]{40}", elegida) or elegida not in validas:
            raise ValueError("la huella no corresponde a una identidad válida importada en el llavero efímero")
    else:
        if len(validas) != 1:
            raise ValueError("se necesita una identidad válida única; selecciona su huella pública con --certificado-sha1")
        elegida = next(iter(validas))
    # SHA-1 sólo selecciona la identidad de codesign en este llavero. Conservamos
    # el DER local para autenticar por igualdad el certificado que devuelve Apple.
    # -Z incluye ambos identificadores junto a cada PEM; no consultamos el llavero
    # personal ni aceptamos dos certificados distintos con el mismo selector.
    salida = capturar(["security", "find-certificate", "-a", "-Z", "-p", llavero])
    certificados = []
    bloques = salida.split("SHA-256 hash: ")
    if bloques[0].strip():
        raise ValueError("no se puede verificar la exportación de certificados del llavero efímero")
    for bloque in bloques[1:]:
        registro = re.fullmatch(r"([0-9A-Fa-f]{64})\s+SHA-1 hash: ([0-9A-Fa-f]{40})\s+"
                                r"-----BEGIN CERTIFICATE-----\s+([A-Za-z0-9+/=\s]+?)"
                                r"-----END CERTIFICATE-----\s*", bloque)
        if registro is None:
            raise ValueError("no se puede verificar la exportación de certificados del llavero efímero")
        sha256, selector, contenido = registro.groups()
        try:
            der = base64.b64decode("".join(contenido.split()), validate=True)
        except ValueError as error:
            raise ValueError("el llavero efímero no devuelve un certificado válido") from error
        if not der or hashlib.sha256(der).hexdigest() != sha256.lower():
            raise ValueError("el certificado exportado no coincide con su huella SHA-256")
        if selector.lower() == elegida:
            certificados.append(der)
    if len(certificados) != 1:
        raise ValueError("la identidad importada no tiene un certificado único en el llavero efímero")
    return certificados[0]


def vigente(fecha):
    try:
        valor = datetime.datetime.fromisoformat(fecha.replace("Z", "+00:00"))
        return valor.tzinfo is not None and valor > datetime.datetime.now(datetime.timezone.utc)
    except (AttributeError, TypeError, ValueError):
        return False


def certificado_de_identidad(asc, plataforma, identidad):
    if not isinstance(identidad, bytes) or not identidad:
        raise ValueError("se necesita el certificado DER de la identidad importada")
    consulta = urllib.parse.urlencode({"filter[certificateType]": ",".join(CERTIFICADOS[plataforma]), "limit": 200})
    candidatos = []
    for certificado in asc.lista("/v1/certificates?" + consulta):
        attrs = certificado.get("attributes", {})
        if attrs.get("certificateType") not in CERTIFICADOS[plataforma] or not vigente(attrs.get("expirationDate")) or attrs.get("activated") is False:
            continue
        try:
            der = base64.b64decode(attrs["certificateContent"], validate=True)
        except (KeyError, TypeError, ValueError):
            continue
        # El selector SHA-1 de codesign no autentica datos remotos: exigimos
        # exactamente el certificado DER exportado del llavero efímero.
        if der == identidad:
            candidatos.append(certificado["id"])
    if len(candidatos) != 1:
        raise ValueError("Apple no devuelve un certificado vigente único para la identidad importada")
    return candidatos[0]


def validar_perfil(perfil, requisito, plataforma):
    try:
        datos = base64.b64decode(perfil["attributes"]["profileContent"], validate=True)
        if not datos or len(datos) > 2 * 1024 * 1024:
            return None
    except (KeyError, TypeError, ValueError):
        return None
    with tempfile.TemporaryDirectory(prefix="sytue-profile-check-") as tmp:
        ruta, derechos = pathlib.Path(tmp, "profile"), pathlib.Path(tmp, "rights.plist")
        ruta.write_bytes(datos)
        derechos.write_bytes(plistlib.dumps(requisito["entitlements"]))
        llamada = ["bash", AQUI / "apple-firma.sh", "verificar-perfil-grupo", ruta,
                   requisito["id"], plataforma, requisito["group"], requisito["protection"],
                   requisito["keychain"], derechos]
        r = subprocess.run([str(a) for a in llamada], capture_output=True, check=False)
        return datos if r.returncode == 0 else None


def perfiles_validos(asc, bundle, requisito, plataforma, certificado):
    perfiles = asc.lista("/v1/bundleIds/" + bundle["id"] + "/profiles?limit=200")
    for perfil in sorted(perfiles, key=lambda p: p.get("attributes", {}).get("createdDate", ""), reverse=True):
        attrs = perfil.get("attributes", {})
        if attrs.get("profileType") != TIPOS[plataforma] or attrs.get("profileState") != "ACTIVE" or not vigente(attrs.get("expirationDate")):
            continue
        if certificado:
            certificados = asc.lista("/v1/profiles/" + perfil["id"] + "/certificates?limit=200")
            if certificado not in {c["id"] for c in certificados}:
                continue
        datos = validar_perfil(perfil, requisito, plataforma)
        if datos is not None:
            return perfil, datos
    return None, None


def guardar_perfil(salida, identificador, plataforma, datos):
    salida.mkdir(parents=True, exist_ok=True, mode=0o700)
    extension = ".mobileprovision" if plataforma == "ios" else ".provisionprofile"
    destino = salida / (identificador + extension)
    fd, temporal = tempfile.mkstemp(prefix=".profile-", dir=salida)
    try:
        with os.fdopen(fd, "wb") as archivo:
            os.fchmod(archivo.fileno(), 0o600)
            archivo.write(datos)
            archivo.flush()
            os.fsync(archivo.fileno())
        os.replace(temporal, destino)
    finally:
        if os.path.exists(temporal):
            os.unlink(temporal)
    return str(destino)


def preparar(asc, requisito, plataforma, certificado="", salida=None):
    identificador = requisito["id"]
    resultado = {"app": requisito["app"], "app_id": identificador, "platform": plataforma,
                 "profile_type": TIPOS[plataforma], "profile_ready": False,
                 "certificate_verified": bool(certificado), "created": False}
    bundle = REGISTRO["buscar"](asc, identificador)
    if bundle:
        resultado["registered_platform"] = bundle.get("attributes", {}).get("platform")
    esperado = "IOS" if plataforma == "ios" else "MAC_OS"
    if not bundle or bundle.get("attributes", {}).get("platform") not in (esperado, "UNIVERSAL"):
        return {**resultado, "reason": "bundle_missing_or_wrong_platform"}
    capacidades = asc.lista("/v1/bundleIds/" + bundle["id"] + "/bundleIdCapabilities")
    por_tipo = {c.get("attributes", {}).get("capabilityType"): c for c in capacidades}
    if "APP_GROUPS" not in por_tipo:
        return {**resultado, "reason": "app_group_capability_missing"}
    if requisito["protection"] and not REGISTRO["completa"](por_tipo.get("DATA_PROTECTION", {})):
        return {**resultado, "reason": "data_protection_complete_missing"}
    perfil, datos = perfiles_validos(asc, bundle, requisito, plataforma, certificado)
    if perfil is None and salida is not None:
        if not certificado:
            raise ValueError("preparar un perfil exige verificar antes la identidad importada")
        sello = hashlib.sha256(plistlib.dumps(requisito["entitlements"]) + certificado.encode()).hexdigest()[:12]
        fecha = datetime.datetime.now(datetime.timezone.utc).strftime("%Y%m%d")
        nombre = "Sytue suite " + identificador + " " + TIPOS[plataforma] + " " + sello + " " + fecha
        creado = asc.escribir("profiles", {"name": nombre, "profileType": TIPOS[plataforma]},
                              rels={"bundleId": {"type": "bundleIds", "id": bundle["id"]},
                                    "certificates": [{"type": "certificates", "id": certificado}]})
        perfil = creado["data"]
        if "profileContent" not in perfil.get("attributes", {}):
            perfil = asc.pedir("GET", "/v1/profiles/" + perfil["id"])["data"]
        datos = validar_perfil(perfil, requisito, plataforma)
        resultado["created"] = True
    if datos is None:
        return {**resultado, "reason": "profile_does_not_authorize_requested_rights"}
    resultado["profile_ready"] = True
    if salida is not None:
        resultado["path"] = guardar_perfil(salida, identificador, plataforma, datos)
    return resultado


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--plataforma", choices=TIPOS, required=True)
    parser.add_argument("--destinos", choices=("hosts", "widgets", "todos"), default="hosts")
    parser.add_argument("--preparar", action="store_true")
    parser.add_argument("--salida", type=pathlib.Path)
    parser.add_argument("--certificado-sha1", default="")
    parser.add_argument("--seco", action="store_true", help="mostrar el plan sin API ni llavero")
    parser.add_argument("apps", nargs="*")
    args = parser.parse_args()
    if args.preparar != (args.salida is not None) or (args.seco and args.preparar) or (args.certificado_sha1 and not args.preparar):
        parser.error("--preparar requiere --salida; --seco no prepara; la huella sólo selecciona una identidad al preparar")
    gen = REGISTRO["importar"]("sytue_xcode_perfiles", AQUI.parent / "xcode/generar.py")
    catalogo = {a["clave"]: a for a in gen.apps()}
    seleccion = args.apps or list(catalogo)
    if len(set(seleccion)) != len(seleccion) or any(a not in catalogo for a in seleccion):
        parser.error("selecciona apps del catálogo sin repetirlas")
    pendientes = [requisitos(gen, catalogo[a], args.plataforma, widget)
                  for a in seleccion for widget in (False, True)
                  if (args.destinos != "widgets" or widget) and (args.destinos != "hosts" or not widget)]
    if args.seco:
        for requisito in pendientes:
            print(json.dumps({"platform": args.plataforma, "profile_type": TIPOS[args.plataforma],
                              **{k: v for k, v in requisito.items() if k != "entitlements"}}, ensure_ascii=False))
        return 0
    api = REGISTRO["importar"]("sytue_asc_perfiles", AQUI / "appstore-publicar.py")
    asc = api.Apple()
    certificado = ""
    if args.preparar:
        identidad = identidad_importada(args.plataforma, gen.EQUIPO, args.certificado_sha1)
        certificado = certificado_de_identidad(asc, args.plataforma, identidad)
    fallos = 0
    for requisito in pendientes:
        try:
            resultado = preparar(asc, requisito, args.plataforma, certificado, args.salida)
            fallos += not resultado["profile_ready"]
            print(json.dumps(resultado, ensure_ascii=False))
        except (RuntimeError, ValueError, OSError, subprocess.CalledProcessError) as err:
            print(json.dumps({"app_id": requisito["id"], "profile_ready": False, "error": str(err)}, ensure_ascii=False))
            fallos += 1
    return bool(fallos)


if __name__ == "__main__":
    try:
        sys.exit(main())
    except (RuntimeError, ValueError, OSError, subprocess.CalledProcessError) as error:
        sys.exit("::error::perfiles Apple: " + str(error))
