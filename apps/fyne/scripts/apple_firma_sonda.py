#!/usr/bin/env python3
# SPDX-License-Identifier: Apache-2.0
# Copyright (c) 2026 Daniel Delgado Ramos
"""Firma una copia temporal; solo imprime certificados públicos y resultados."""

import hashlib
import json
import os
import pathlib
import re
import shutil
import ssl
import subprocess
import sys
import tempfile


def ejecutar(argv):
    try:
        return subprocess.run(argv, capture_output=True, text=True, timeout=45)
    except subprocess.TimeoutExpired:
        # Conservar las mediciones anteriores si una consulta o el sellado se atascan.
        return subprocess.CompletedProcess(argv, 124, "", "timeout (45 s)")


def salida(resultado):
    return {"status": resultado.returncode, "output": (resultado.stdout + resultado.stderr)[-5000:]}


def huella(der):
    return hashlib.sha256(der).hexdigest().upper()


def certificados_publicos(pem, carpeta, prefijo, correr):
    archivos, metadatos = {}, []
    # security -Z -p entrega ambos identificadores junto al PEM. SHA-1 solo se
    # conserva como selector opaco para find-identity/codesign; no decide confianza.
    bloques = list(re.finditer(
        r'^SHA-256 hash: ([0-9a-fA-F]{64})\s*\nSHA-1 hash: ([0-9a-fA-F]{40})\s*\n'
        r'(-----BEGIN CERTIFICATE-----.*?-----END CERTIFICATE-----)', pem, re.M | re.S))
    if len(bloques) != pem.count("-----BEGIN CERTIFICATE-----"):
        raise ValueError("certificado sin los identificadores de security -Z")
    for indice, bloque in enumerate(bloques):
        sha256, sha1, certificado = bloque.groups()
        sha256, sha1 = sha256.upper(), sha1.upper()
        if huella(ssl.PEM_cert_to_DER_cert(certificado)) != sha256:
            raise ValueError("SHA-256 no corresponde al certificado exportado")
        archivo = carpeta / f"{prefijo}-{indice}.pem"
        archivo.write_text(certificado + "\n")
        archivos[sha256] = archivo
        metadatos.append({
            "sha1": sha1,
            "sha256": sha256,
            "metadata": salida(correr(["openssl", "x509", "-in", str(archivo), "-noout", "-subject", "-issuer", "-dates", "-serial", "-fingerprint", "-sha256"])),
        })
    return archivos, metadatos


def certificados_por_selector(metadatos):
    selectores = {}
    for certificado in metadatos:
        selector, sha256 = certificado["sha1"], certificado["sha256"]
        if selector in selectores and selectores[selector] != sha256:
            raise ValueError("selector de codesign ambiguo entre certificados distintos")
        selectores[selector] = sha256
    return selectores


def firmar(temporal, indice, variante, identidad, argumentos, correr):
    binario = temporal / f"sonda-{indice}"
    shutil.copyfile("/usr/bin/true", binario)
    binario.chmod(0o700)
    r = correr(["codesign", "-f", "-s", identidad, *argumentos, "--options", "runtime", "--timestamp", "--identifier", "eu.sytue.firma.sonda", str(binario)])
    return {"variant": variante, "selector": identidad, "sign": salida(r), "ok": False}, binario


def comprobar_firma(firma, binario, temporal, indice, esperado, correr):
    if firma["sign"]["status"] != 0:
        return
    verificacion = correr(["codesign", "--verify", "--strict", str(binario)])
    firma["verify"] = salida(verificacion)
    prefijo = str(temporal / f"cadena-{indice}-")
    # El argumento es opcional: separado se interpreta como otro binario, no prefijo.
    extraer = correr(["codesign", "-d", f"--extract-certificates={prefijo}", str(binario)])
    firma["extract"] = salida(extraer)
    cadena = []
    hoja = None
    for certificado in sorted(temporal.glob(f"cadena-{indice}-*")):
        der = certificado.read_bytes()
        sha256 = huella(der)
        if certificado.name == f"cadena-{indice}-0":
            hoja = sha256
        cadena.append({"sha256": sha256, "metadata": salida(correr(["openssl", "x509", "-inform", "DER", "-in", str(certificado), "-noout", "-subject", "-issuer", "-fingerprint", "-sha256"]))})
    firma["chain"] = cadena
    firma["ok"] = verificacion.returncode == 0 and extraer.returncode == 0 and hoja in esperado


def sondear(llavero, nombre, correr=ejecutar):
    resultado = {"identity_name": nombre, "keychain": llavero, "signatures": [], "production_ok": False}
    with tempfile.TemporaryDirectory(prefix="sytue-firma-sonda-") as carpeta:
        temporal = pathlib.Path(carpeta)
        # Medir el selector de producción antes de consultar identidades o evaluar
        # confianza otra vez: esas lecturas podrían cambiar el estado de trustd.
        fria, binario_frio = firmar(temporal, 0, "name_search_cold", nombre, [], correr)
        resultado["signatures"].append(fria)
        identidades = {}
        for alcance, argumentos in [("search_list", []), ("keychain", [llavero])]:
            r = correr(["security", "find-identity", "-v", "-p", "codesigning", *argumentos])
            identidades[alcance] = {
                "status": r.returncode,
                "matching": [
                    {"sha1": m[0].upper(), "name": m[1]}
                    for m in re.findall(r'^\s*\d+\)\s+([0-9a-fA-F]{40})\s+"([^"]+)"', r.stdout, re.M)
                    if m[1] == nombre
                ],
            }
        resultado["identities"] = identidades
        if any(i["status"] != 0 for i in identidades.values()):
            resultado["error"] = "no se pudieron consultar las identidades de firma"
            return resultado
        resultado["search_list"] = salida(correr(["security", "list-keychains", "-d", "user"]))
        resultado["keychain_settings"] = salida(correr(["security", "show-keychain-info", llavero]))
        r = correr(["security", "find-certificate", "-a", "-Z", "-p", llavero])
        certificados, resultado["certificates"] = certificados_publicos(r.stdout, temporal, "efimero", correr)
        resultado["certificates_status"] = r.returncode
        if r.returncode != 0:
            resultado["error"] = "no se pudieron exportar los certificados del llavero efímero"
            return resultado
        selectores = certificados_por_selector(resultado["certificates"])
        r = correr(["security", "find-certificate", "-a", "-Z", "-p", "-c", "Apple Root CA", "/System/Library/Keychains/SystemRootCertificates.keychain"])
        _, metadatos = certificados_publicos(r.stdout, temporal, "raiz", correr)
        resultado["system_apple_roots"] = {"status": r.returncode, "certificates": metadatos}
        globales = {i["sha1"] for i in identidades["search_list"]["matching"]}
        hashes = list(dict.fromkeys(i["sha1"] for i in identidades["keychain"]["matching"]))
        resultado["identities_outside_ephemeral"] = sorted(globales - set(hashes))
        if not hashes or len(hashes) > 8 or any(h not in selectores for h in hashes):
            resultado["error"] = "no hay identidades medibles del llavero efímero o hay más de ocho"
            return resultado

        # También las identidades fuera del llavero efímero necesitan el PEM real:
        # el selector de 40 caracteres, por sí solo, no autentica la hoja firmante.
        selectores_globales = dict(selectores)
        if globales - set(hashes):
            r = correr(["security", "find-certificate", "-a", "-Z", "-p", "-c", nombre])
            _, metadatos = certificados_publicos(r.stdout, temporal, "global", correr)
            resultado["search_certificates"] = {"status": r.returncode, "certificates": metadatos}
            selectores_globales = certificados_por_selector(resultado["certificates"] + metadatos)
            if r.returncode != 0 or any(h not in selectores_globales for h in globales):
                resultado["error"] = "no se pudieron identificar los certificados de la búsqueda global"
                return resultado
        esperados_locales = {selectores[h] for h in hashes}
        esperados_globales = esperados_locales | {selectores_globales[h] for h in globales}
        comprobar_firma(fria, binario_frio, temporal, 0, esperados_globales, correr)
        variantes = [("name_search", nombre, []), ("name_keychain", nombre, ["--keychain", llavero])]
        for sha1 in hashes:
            variantes += [("hash_search", sha1, []), ("hash_keychain", sha1, ["--keychain", llavero])]
        for indice, (variante, identidad, argumentos) in enumerate(variantes, start=1):
            firma, binario = firmar(temporal, indice, variante, identidad, argumentos, correr)
            esperado = {selectores[identidad]} if variante.startswith("hash_") else (esperados_globales if variante == "name_search" else esperados_locales)
            comprobar_firma(firma, binario, temporal, indice, esperado, correr)
            resultado["signatures"].append(firma)
        # El warning de CMS construye BasicX509, distinto del find-identity CodeSigning.
        # Estas evaluaciones quedan después de todas las firmas, nunca como arreglo.
        resultado["policies"] = []
        for sha1 in hashes:
            for politica in ["basic", "codeSign"]:
                for alcance, argumentos in [("search_list", []), ("keychain", ["-k", llavero])]:
                    sha256 = selectores[sha1]
                    r = correr(["security", "verify-cert", "-L", "-v", "-p", politica, "-c", str(certificados[sha256]), *argumentos])
                    resultado["policies"].append({"sha1": sha1, "sha256": sha256, "policy": politica, "scope": alcance, **salida(r)})
        resultado["expected_signatures"] = len(variantes) + 1
        resultado["production_ok"] = len(resultado["signatures"]) == resultado["expected_signatures"] and fria["ok"]
    return resultado


def main():
    # Solo dentro de un job de GitHub Actions (el runner sytue-macos): toca el llavero.
    if os.environ.get("GITHUB_ACTIONS") != "true" or os.environ.get("SYTUE_PRUEBA") != "si":
        sys.exit("sonda: solo en un job de GitHub Actions (runner sytue-macos) con SYTUE_PRUEBA=si")
    try:
        resultado = sondear(sys.argv[1], sys.argv[2])
    except (OSError, ValueError, subprocess.TimeoutExpired) as error:
        resultado = {"production_ok": False, "error_type": type(error).__name__}
    print("SYTUE_FIRMA_SONDA=" + json.dumps(resultado, sort_keys=True))
    return 0 if resultado["production_ok"] else 1


if __name__ == "__main__":
    sys.exit(main())
