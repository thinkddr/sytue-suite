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


def huella(pem):
    # SHA-1 es el selector de identidad de codesign, no una validación criptográfica.
    return hashlib.sha1(ssl.PEM_cert_to_DER_cert(pem)).hexdigest().upper()


def certificados_publicos(pem, carpeta, prefijo, correr):
    archivos, metadatos = {}, []
    for indice, certificado in enumerate(re.findall(r'-----BEGIN CERTIFICATE-----.*?-----END CERTIFICATE-----', pem, re.S)):
        archivo = carpeta / f"{prefijo}-{indice}.pem"
        archivo.write_text(certificado + "\n")
        sha1 = huella(certificado)
        archivos[sha1] = archivo
        metadatos.append({
            "sha1": sha1,
            "metadata": salida(correr(["openssl", "x509", "-in", str(archivo), "-noout", "-subject", "-issuer", "-dates", "-serial", "-fingerprint", "-sha256"])),
        })
    return archivos, metadatos


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
    for certificado in sorted(temporal.glob(f"cadena-{indice}-*")):
        der = certificado.read_bytes()
        cadena.append({"sha1": hashlib.sha1(der).hexdigest().upper(), "metadata": salida(correr(["openssl", "x509", "-inform", "DER", "-in", str(certificado), "-noout", "-subject", "-issuer", "-fingerprint", "-sha256"]))})
    firma["chain"] = cadena
    firma["ok"] = verificacion.returncode == 0 and extraer.returncode == 0 and bool(cadena) and cadena[0]["sha1"] in esperado


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
        resultado["search_list"] = salida(correr(["security", "list-keychains", "-d", "user"]))
        resultado["keychain_settings"] = salida(correr(["security", "show-keychain-info", llavero]))
        r = correr(["security", "find-certificate", "-a", "-p", llavero])
        certificados, resultado["certificates"] = certificados_publicos(r.stdout, temporal, "efimero", correr)
        resultado["certificates_status"] = r.returncode
        r = correr(["security", "find-certificate", "-a", "-p", "-c", "Apple Root CA", "/System/Library/Keychains/SystemRootCertificates.keychain"])
        _, metadatos = certificados_publicos(r.stdout, temporal, "raiz", correr)
        resultado["system_apple_roots"] = {"status": r.returncode, "certificates": metadatos}
        globales = {i["sha1"] for i in identidades["search_list"]["matching"]}
        hashes = list(dict.fromkeys(i["sha1"] for i in identidades["keychain"]["matching"]))
        resultado["identities_outside_ephemeral"] = sorted(globales - set(hashes))
        if not hashes or len(hashes) > 8 or any(h not in certificados for h in hashes):
            resultado["error"] = "no hay identidades medibles del llavero efímero o hay más de ocho"
            return resultado

        comprobar_firma(fria, binario_frio, temporal, 0, set(hashes) | globales, correr)
        variantes = [("name_search", nombre, []), ("name_keychain", nombre, ["--keychain", llavero])]
        for sha1 in hashes:
            variantes += [("hash_search", sha1, []), ("hash_keychain", sha1, ["--keychain", llavero])]
        for indice, (variante, identidad, argumentos) in enumerate(variantes, start=1):
            firma, binario = firmar(temporal, indice, variante, identidad, argumentos, correr)
            esperado = {identidad} if variante.startswith("hash_") else set(hashes) | (globales if variante == "name_search" else set())
            comprobar_firma(firma, binario, temporal, indice, esperado, correr)
            resultado["signatures"].append(firma)
        # El warning de CMS construye BasicX509, distinto del find-identity CodeSigning.
        # Estas evaluaciones quedan después de todas las firmas, nunca como arreglo.
        resultado["policies"] = []
        for sha1 in hashes:
            for politica in ["basic", "codeSign"]:
                for alcance, argumentos in [("search_list", []), ("keychain", ["-k", llavero])]:
                    r = correr(["security", "verify-cert", "-L", "-v", "-p", politica, "-c", str(certificados[sha1]), *argumentos])
                    resultado["policies"].append({"sha1": sha1, "policy": politica, "scope": alcance, **salida(r)})
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
