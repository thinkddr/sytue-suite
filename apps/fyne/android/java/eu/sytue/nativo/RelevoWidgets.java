// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package eu.sytue.nativo;

import android.content.ContentProvider;
import android.content.ContentValues;
import android.content.Context;
import android.content.pm.ProviderInfo;
import android.database.Cursor;
import android.net.Uri;
import android.os.Bundle;

/**
 * El relevo de One (ADR 0078, enmienda del 2026-11-06, pieza 6): One, que mantiene el SSE
 * del Hub con todo cerrado, le dice a la app de un producto que algo suyo ha cambiado, y
 * esta encola su refresco (RefrescoWidgets.relevo), que lee por su cuenta.
 *
 * No es un broadcast con permiso de firma: cada una de las quince apps va firmada con
 * SU certificado de Play (android/session-signers.json), así que un permiso
 * {@code signature} no se podría conceder entre ellas, y declararlo en las quince haría
 * chocar las instalaciones (INSTALL_FAILED_DUPLICATE_PERMISSION). Es una llamada a este
 * proveedor, que comprueba al llamante con el mismo límite que la sesión compartida
 * (SesionAndroid.llamante: el UID del Binder y el certificado fijado de cada paquete), y
 * solo acepta a One. No lleva datos en ningún sentido: ni producto, ni quién, ni qué.
 */
public final class RelevoWidgets extends ContentProvider {
    static final String SUFIJO = ".sytue.widgets";
    static final String METODO = "refrescar";

    @Override
    public boolean onCreate() {
        return true;
    }

    @Override
    public Bundle call(String metodo, String argumento, Bundle extras) {
        if (!METODO.equals(metodo) || argumento != null || (extras != null && !extras.isEmpty())) {
            throw new SecurityException("Unsupported widget relay call");
        }
        Context ctx = getContext();
        String llamante = SesionAndroid.llamante(ctx, getCallingPackage());
        if (!RefrescoReglas.llamanteValido(llamante, ctx.getPackageName())) {
            throw new SecurityException("Only Sytue One relays widget changes");
        }
        RefrescoWidgets.relevo(ctx);
        return null;
    }

    /**
     * Lo llama Go en One (apps/fyne/sesion, fondo_android.go) con el paquete de la app del
     * producto que ha cambiado. Solo a una app del catálogo fijado, cuyo proveedor sea este
     * y cuya firma sea la suya: a nadie más se le dice ni siquiera que algo ha cambiado.
     */
    public static boolean avisar(Context ctx, String paquete) {
        if (!RefrescoWidgets.conGo(ctx)
                || !RefrescoReglas.destinoValido(paquete, ctx.getPackageName(), SesionFirmas.PAQUETES.keySet())) {
            return false;
        }
        String autoridad = paquete + SUFIJO;
        try {
            ProviderInfo proveedor = ctx.getPackageManager().resolveContentProvider(autoridad, 0);
            if (proveedor == null || !paquete.equals(proveedor.packageName)
                    || !RelevoWidgets.class.getName().equals(proveedor.name)) {
                return false;
            }
            SesionConfianza.Instalada destino = SesionAndroid.instalada(ctx, paquete);
            if (destino == null || !SesionConfianza.permite(paquete, destino.firmantes)) {
                return false;
            }
            ctx.getContentResolver().call(Uri.parse("content://" + autoridad), METODO, null, null);
            return true;
        } catch (RuntimeException fallo) { // no instalada, proceso muerto, rechazo
            return false;
        }
    }

    @Override
    public Cursor query(Uri u, String[] p, String s, String[] a, String o) {
        throw new SecurityException("Widget relay has no data");
    }

    @Override
    public String getType(Uri u) {
        return null;
    }

    @Override
    public Uri insert(Uri u, ContentValues v) {
        throw new SecurityException("Widget relay has no data");
    }

    @Override
    public int delete(Uri u, String s, String[] a) {
        throw new SecurityException("Widget relay has no data");
    }

    @Override
    public int update(Uri u, ContentValues v, String s, String[] a) {
        throw new SecurityException("Widget relay has no data");
    }
}
