<!-- SPDX-License-Identifier: Apache-2.0 -->
<!-- Copyright (c) 2026 Daniel Delgado Ramos -->

<a id="sellado-de-tiempo-de-las-firmas"></a>
# Timestamping of signatures

<a id="el-hueco"></a>
## The gap

Images are signed with `cosign` and our own key, without uploading anything to the
public transparency log. That decision is reasoned in `deploy/production/03-desplegar.sh`
and is not debated here: publishing to Rekor exposed the service inventory and the
release cadence of a product that has not launched yet.

What that decision leaves open, and was not written down anywhere: **a signature
does not prove when it was made**. If the release key is compromised, there is no way
to tell what was signed before the theft from what was signed after it. The response to
that incident becomes revoking and re-signing EVERYTHING, without being able to save
anything, and without being able to prove to a customer that the version they have
deployed predates the theft.

<a id="lo-que-lo-cubre-y-lo-que-no"></a>
## What covers it, and what does not

An RFC 3161 timestamp says “this signature existed at this time” and nothing more: it
does not carry the image name or its digest, so **it does not reintroduce the problem
that took us off Rekor**. It is not a public log: it is a trusted third party that
countersigns a hash.

It does not replace key custody. With the stolen key, new images can still be signed;
what is gained is the ability to bound the damage in time instead of treating
everything ever signed as compromised.

<a id="receta-probada-con-cosign-313"></a>
## Recipe, tested with cosign 3.1.3

Verified end to end on 2026-08-18 against a public test TSA.

Signing —the timestamp FORCES the new bundle format, it is not optional—:

```bash
cosign sign-blob --key "$CLAVE" --tlog-upload=false --use-signing-config=false --yes \
  --timestamp-server-url "$SYTUE_TSA_URL" \
  --new-bundle-format --bundle firma.bundle artefacto
```

Building the trust material. **The chain goes with the LEAF FIRST**: if the root CA is
put first, `trusted-root create` fails with “first certificate in chain
must be a leaf certificate”, which at no point says that the problem is the
order.

```bash
cat tsa-hoja.crt tsa-ca.pem > tsa-cadena.pem
cosign trusted-root create --no-default-fulcio --no-default-rekor --no-default-ctfe \
  --tsa="url=$SYTUE_TSA_URL,certificate-chain=tsa-cadena.pem" --out trusted-root.json
```

Verifying:

```bash
cosign verify-blob --key cosign.pub --bundle firma.bundle \
  --trusted-root trusted-root.json --use-signed-timestamps \
  --insecure-ignore-tlog=true artefacto
```

`--insecure-ignore-tlog` is still needed even with a timestamp: without Rekor, cosign
requires inclusion in the log and fails with “not enough verified log entries from
transparency log: 0 < 1”. The timestamp does not replace it.

<a id="lo-que-falta-para-activarlo"></a>
## What is missing to enable it

1. **Choose an EU qualified TSA.** It is a business decision: there are
   eIDAS qualified providers in Spain, Italy, Germany and Poland, with a contract
   and a cost. The test one used to validate the recipe is not fit for production,
   neither by jurisdiction nor by guarantees.
2. **Migrate the signatures to the bundle format**, which is what the timestamp forces.
   That touches `sign`, `attest` and the verification in `03-desplegar.sh` AT THE SAME
   TIME, plus the four places watched by `release-workflow-test.sh`. As a bonus, the
   deprecated `--tlog-upload` warning goes away.
3. **Test it end to end in the ephemeral environment**, not in a release. If verification
   fails after the format change, nothing is deployed.

<a id="custodia-de-la-clave"></a>
## Key custody

`COSIGN_PRIVATE_KEY` and `COSIGN_PASSWORD` live as repository secrets in the
forge, so whoever compromises the forge has the key that production is signed with.
Mitigation applied: **hand the secret only to whoever needs it**.

Only four workflows sign, and only on two events:

| | |
| --- | --- |
| Workflows | `release.yml`, `imagenes-rama.yml`, `espejo-upstream.yml`, `release-desktop.yml` |
| Events | `push`, `workflow_dispatch` |

With that, a pull request —our own or from a fork— and any new workflow can no
longer read the signing key. It is configured on each secret in the forge, under
«Limitar a eventos» (Limit to events) and «Workflows».

What it does NOT solve: whoever has administrative access to the forge can still
read it. Removing that requires a non-extractable key in an HSM or KMS, and cosign
does not support Scaleway's Key Manager today.
