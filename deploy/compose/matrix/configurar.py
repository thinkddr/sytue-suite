#!/usr/bin/env python3
# SPDX-License-Identifier: Apache-2.0
# Copyright (c) 2026 Daniel Delgado Ramos
"""homeserver.yaml de Synapse para Docker Compose (local, servidor único y todo en uno).

Es la configuración de producción (deploy/helm/charts/matrix y
el generador del despliegue privado) en un solo fichero: sin registro,
sin federación ni directorio, cifrado por defecto en todas las salas, y Sytue ID
como único proveedor de identidad (uno por producto con almacenamiento privado,
cada uno con su usuario Matrix, como en producción).

Lo único que cambia es por dónde habla Synapse con Sytue ID: el emisor y la página
de autorización son las públicas (las ve el navegador), pero el token, el userinfo
y las claves los pide por la red interna (http://id:8081), sin pasar por Traefik
ni depender de que el contenedor confíe en el certificado local.

Uso (lo llama ./sytue-deploy; escribe secretos, la salida va a un fichero 0644
dentro de una carpeta 0700 fuera de Git):

  DOMINIO=sytue.localhost MATRIX_DB_PASSWORD=… MATRIX_REGISTRATION_SECRET=… \\
  MATRIX_MACAROON_SECRET=… MATRIX_FORM_SECRET=… OIDC_MATRIX=… \\
    configurar.py <apps/nucleo/storage/products.txt> > homeserver.yaml
"""
import json
import os
import sys
from pathlib import Path

e = os.environ
if len(sys.argv) != 2:
    sys.exit("uso: configurar.py <products.txt>")
for clave in ("DOMINIO", "MATRIX_DB_PASSWORD", "MATRIX_REGISTRATION_SECRET",
              "MATRIX_MACAROON_SECRET", "MATRIX_FORM_SECRET", "OIDC_MATRIX"):
    if not e.get(clave):
        sys.exit(f"configurar.py: falta {clave}")

dominio = e["DOMINIO"]
servidor = "matrix." + dominio
id_publico = "https://id." + dominio
id_interno = e.get("SYTUE_ID_INTERNAL_URL", "http://id:8081")
rooms = "https://rooms." + dominio

base = {
    "idp_id": "sytue", "idp_name": "Sytue ID",
    "discover": False,
    # El token, el userinfo y las claves van por http interno: Synapse exige
    # https salvo con skip_verification, que solo relaja eso (el id_token se
    # sigue validando contra el emisor y las claves de Sytue ID).
    "skip_verification": True,
    "issuer": id_publico, "client_id": "sytue-matrix",
    "authorization_endpoint": id_publico + "/oauth2/authorize",
    "token_endpoint": id_interno + "/oauth2/token",
    "userinfo_endpoint": id_interno + "/oauth2/userinfo",
    "jwks_uri": id_interno + "/.well-known/jwks.json",
    "client_secret": e["OIDC_MATRIX"], "scopes": ["openid", "profile"],
    "pkce_method": "always", "allow_existing_users": False, "enable_registration": False,
    "user_mapping_provider": {"config": {
        "subject_template": "{{ user.sub }}", "localpart_template": "sytue_{{ user.sub }}",
        "display_name_template": "{{ user.name }}"}},
}
proveedores = [base]
productos = Path(sys.argv[1]).read_text().split()
if not productos or len(productos) != len(set(productos)) or any(
        not p.isascii() or not p.isalpha() or not p.islower() for p in productos):
    sys.exit("configurar.py: el registro de productos no es válido")
for producto in productos:
    p = dict(base)
    p["idp_id"] = "sytue-storage-" + producto
    p["idp_name"] = "Sytue " + (producto.upper() if producto in {"pdf", "erp"} else producto.title())
    p["passthrough_authorization_parameters"] = ["sytue_tenant", "sytue_subject"]
    p["user_mapping_provider"] = {"config": {
        "subject_template": "{{ user.tenant }}|{{ user.sub }}",
        "localpart_template": "sytue_storage_" + producto + "_{{ user.tenant }}_{{ user.sub }}",
        "display_name_template": "{{ user.name }}"}}
    proveedores.append(p)

config = {
    "server_name": servidor,
    "public_baseurl": "https://" + servidor,
    "pid_file": "/tmp/synapse.pid",
    "signing_key_path": "/data/signing.key",
    "media_store_path": "/data/media",
    "report_stats": False,
    "enable_registration": False,
    "allow_guest_access": False,
    "enable_metrics": False,
    "enable_search": False,
    "enable_room_list_search": False,
    "allow_public_rooms_without_auth": False,
    "allow_public_rooms_over_federation": False,
    "federation_domain_whitelist": [],
    "trusted_key_servers": [],
    "suppress_key_server_warning": True,
    "url_preview_enabled": False,
    "presence": {"enabled": False},
    "password_config": {"enabled": False, "localdb_enabled": False},
    "user_directory": {"enabled": False, "search_all_users": False},
    "room_list_publication_rules": [{"action": "deny"}],
    "encryption_enabled_by_default_for_room_type": "all",
    "listeners": [{
        "port": 8008, "bind_addresses": ["0.0.0.0"], "type": "http", "tls": False,
        "x_forwarded": True, "resources": [{"names": ["client"], "compress": False}]}],
    "sso": {"client_whitelist": [
        rooms + "/rooms/matrix/callback", rooms + "/rooms/matrix/native/callback",
        "http://127.0.0.1:"]},
    "max_upload_size": "100M",
    # Cada app abre su propia sesión Matrix (una identidad por producto), y en una
    # instalación de una organización la gente entra a la vez desde la misma IP
    # (Traefik, una oficina tras un NAT). El límite por defecto (ráfaga de 3, luego
    # uno cada ~6 s) devolvía 429 al abrir la tercera app. Sigue habiendo límite.
    "rc_login": {
        "address": {"per_second": 1, "burst_count": 30},
        "account": {"per_second": 0.5, "burst_count": 10},
        "failed_attempts": {"per_second": 0.17, "burst_count": 3}},
    "log_config": "/config/log.yaml",
    "database": {"name": "psycopg2", "args": {
        "host": e.get("MATRIX_DB_HOST", "postgres"), "port": int(e.get("MATRIX_DB_PORT", "5432")),
        "user": "sytue_matrix", "password": e["MATRIX_DB_PASSWORD"], "database": "sytue_matrix",
        "cp_min": 1, "cp_max": 5}},
    "registration_shared_secret": e["MATRIX_REGISTRATION_SECRET"],
    "macaroon_secret_key": e["MATRIX_MACAROON_SECRET"],
    "form_secret": e["MATRIX_FORM_SECRET"],
    "oidc_providers": proveedores,
}
# JSON es YAML válido: sin dependencias para escribirlo.
json.dump(config, sys.stdout, indent=2, ensure_ascii=False)
print()
