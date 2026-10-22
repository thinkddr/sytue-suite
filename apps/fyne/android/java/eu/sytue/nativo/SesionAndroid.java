// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package eu.sytue.nativo;

import android.content.Context;
import android.content.pm.PackageInfo;
import android.content.pm.PackageManager;
import android.content.pm.ProviderInfo;
import android.content.pm.Signature;
import android.os.Binder;
import android.os.Build;
import java.security.MessageDigest;
import java.security.NoSuchAlgorithmException;
import java.util.ArrayList;
import java.util.Collection;
import java.util.Collections;
import java.util.List;

/** Android APIs stay here; the trust/election rules also run against JVM fixtures. */
final class SesionAndroid {
    static final String SUFIJO = ".sytue.sesion";
    static final String META_PROTOCOLO = "eu.sytue.sesion.PROTOCOLO";

    private SesionAndroid() {}

    @SuppressWarnings("deprecation")
    static SesionConfianza.Instalada instalada(Context contexto, String paquete) {
        try {
            int flags = Build.VERSION.SDK_INT >= 28
                    ? PackageManager.GET_SIGNING_CERTIFICATES : PackageManager.GET_SIGNATURES;
            PackageInfo info = contexto.getPackageManager().getPackageInfo(paquete, flags);
            Signature[] firmas = Build.VERSION.SDK_INT >= 28 && info.signingInfo != null
                    ? info.signingInfo.getApkContentsSigners() : info.signatures;
            List<String> hashes = new ArrayList<>();
            if (firmas != null) {
                for (Signature firma : firmas) {
                    hashes.add(sha256(firma.toByteArray()));
                }
            }
            ProviderInfo provider = contexto.getPackageManager().resolveContentProvider(
                    paquete + SUFIJO, PackageManager.GET_META_DATA);
            boolean protocolo = provider != null && provider.exported && provider.enabled
                    && paquete.equals(provider.packageName)
                    && "eu.sytue.nativo.SesionProveedor".equals(provider.name)
                    && provider.metaData != null && provider.metaData.getInt(META_PROTOCOLO, 0) == 1;
            return new SesionConfianza.Instalada(paquete, info.firstInstallTime,
                    info.applicationInfo.uid, hashes, protocolo);
        } catch (PackageManager.NameNotFoundException falta) {
            return null;
        }
    }

    static SesionConfianza.Instalada elegido(Context contexto) {
        List<SesionConfianza.Instalada> apps = new ArrayList<>();
        for (String paquete : SesionFirmas.PAQUETES.keySet()) {
            SesionConfianza.Instalada app = instalada(contexto, paquete);
            if (app != null) {
                apps.add(app);
            }
        }
        return SesionConfianza.elegir(apps);
    }

    static String llamante(Context contexto, String nombreFramework) {
        int uid = Binder.getCallingUid();
        String[] paquetes = contexto.getPackageManager().getPackagesForUid(uid);
        if (paquetes == null || paquetes.length == 0) {
            throw new SecurityException("Unknown session caller UID");
        }
        List<SesionConfianza.Instalada> apps = new ArrayList<>();
        for (String paquete : paquetes) {
            SesionConfianza.Instalada app = instalada(contexto, paquete);
            if (app == null) {
                throw new SecurityException("Missing session caller package");
            }
            apps.add(app);
        }
        // ContentProvider verifies getCallingPackage(). In an in-process call it
        // can be absent; a unique OS UID-to-package mapping is equally unambiguous.
        String nombre = nombreFramework;
        if (nombre == null && paquetes.length == 1) {
            nombre = paquetes[0];
        }
        if (!SesionConfianza.llamante(uid, nombre, apps)) {
            throw new SecurityException("Untrusted session caller");
        }
        return nombre;
    }

    static void verificarDestino(Context contexto, SesionConfianza.Instalada elegido) {
        if (elegido == null) {
            throw new SecurityException("No trusted session broker installed");
        }
        ProviderInfo provider = contexto.getPackageManager().resolveContentProvider(
                elegido.paquete + SUFIJO, PackageManager.GET_META_DATA);
        if (provider == null) {
            throw new IllegalStateException("Update the first installed Sytue application: " + elegido.paquete);
        }
        SesionConfianza.Instalada actual = instalada(contexto, provider.packageName);
        Collection<String> firmas = actual == null ? Collections.emptySet() : actual.firmantes;
        if (!elegido.protocolo || !SesionConfianza.destino(elegido, provider.packageName, firmas)) {
            throw new SecurityException("Session provider identity or protocol rejected");
        }
    }

    static String sha256(byte[] bytes) {
        try {
            byte[] hash = MessageDigest.getInstance("SHA-256").digest(bytes);
            char[] resultado = new char[hash.length * 2];
            final char[] hex = "0123456789abcdef".toCharArray();
            for (int i = 0; i < hash.length; i++) {
                resultado[2 * i] = hex[(hash[i] & 255) >>> 4];
                resultado[2 * i + 1] = hex[hash[i] & 15];
            }
            return new String(resultado);
        } catch (NoSuchAlgorithmException imposible) {
            throw new IllegalStateException("SHA256 unavailable", imposible);
        }
    }
}
