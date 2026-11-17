<!-- SPDX-License-Identifier: Apache-2.0 -->

<a id="ubicación-aproximada-de-los-accesos"></a>
# Approximate location of sign-ins

Sytue ID's «Accesos recientes» (Recent sign-ins) and «Sesiones activas» (Active sessions) can show where
each sign-in came from. An IP tells nobody whether a sign-in was theirs; “Valencia, Spain”
does, and that is exactly the question of whoever comes in to review their account.

**It is optional.** The table starts empty and, while empty, the interface omits the location without
errors or gaps.

<a id="cargar-los-datos"></a>
## Loading the data

```bash
# Descargar y cargar el mes en curso
deploy/geoip/cargar.py --dsn "$SYTUE_ID_DATABASE_DSN" --descargar 2026-08

# O desde un fichero ya bajado
deploy/geoip/cargar.py --dsn "$SYTUE_ID_DATABASE_DSN" --fichero dbip-city-lite.csv.gz
```

It takes a few minutes (several million ranges). The load goes into a temporary table
and is swapped at the end **within the same transaction**: while it runs, the
table in use keeps answering, and if something fails the previous one stays intact.

It is worth repeating every few months: ranges get reassigned and an old database
starts placing sign-ins where they are not.

<a id="atribución-obligatoria"></a>
## Mandatory attribution

The data comes from **[DB-IP](https://db-ip.com)**, *IP to City Lite* database,
published under **[CC BY 4.0](https://creativecommons.org/licenses/by/4.0/)**.

> IP Geolocation by DB-IP — https://db-ip.com

That attribution is a **condition of the license**, not a courtesy: it must be
kept visible wherever the product that uses it is published. If the source is ever
changed, check what the new one requires.

**MaxMind's GeoLite2 is not used**: since 2019 it requires an account, accepting its EULA and a
license key to download it, which makes it useless for a deployment
that a customer must be able to reproduce on their own.

<a id="por-qué-en-postgresql-y-no-con-una-librería"></a>
## Why in PostgreSQL and not with a library

Reading an MMDB file means adding a dependency to the IdP binary —which today
only uses the stdlib and pgx— and mounting the file on every replica. PostgreSQL is already
deployed, already knows how to compare addresses with the `inet` type, and an index resolves
the lookup in microseconds. The data also stays inside our
infrastructure, which is what a European customer is promised.

<a id="qué-no-hace"></a>
## What it does not do

- **It is not proof of where someone was.** The country is almost always right; the
  city, much less so. A VPN, a mobile phone or a carrier's proxy can
  shift it by hundreds of kilometers.
- **No coordinates or region are stored.** The loader discards them: they are not
  shown, so they are not stored.
- **It does not leave the server.** The lookup runs against our own
  database; no external service is queried with users' IPs.
