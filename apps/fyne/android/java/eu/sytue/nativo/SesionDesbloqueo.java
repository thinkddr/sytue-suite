// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package eu.sytue.nativo;

import android.app.Activity;
import android.hardware.biometrics.BiometricPrompt;
import android.net.Uri;
import android.os.Bundle;
import android.os.CancellationSignal;
import java.security.Signature;

/** Private activity: only the broker-created one-shot PendingIntent can enter it. */
public final class SesionDesbloqueo extends Activity {
    private final CancellationSignal cancelar = new CancellationSignal();
    private String nonce;

    @Override public void onCreate(Bundle anterior) {
        super.onCreate(anterior);
        Uri origen = getIntent().getData();
        nonce = origen == null ? "" : origen.getSchemeSpecificPart();
        SesionDesafios.Pendiente reto = "sytue-session-owner".equals(origen == null ? "" : origen.getScheme())
                && nonce.matches("[a-f0-9]{64}") ? SesionProveedor.pendiente(nonce) : null;
        if (!SesionCompartida.habilitada() || anterior != null || reto == null) {
            finish();
            return;
        }
        try {
            // The label is resolved from the previously verified UID/package, never
            // supplied by the client. The private key stays in THIS broker's UID.
            CharSequence titulo = getPackageManager().getApplicationInfo(reto.origen, 0)
                    .loadLabel(getPackageManager());
            Signature firma = SesionBoveda.firmaDueno();
            String motivo = getIntent().getStringExtra("motivo");
            Biometria.prompt(this, titulo, motivo == null ? "" : motivo).authenticate(
                    new BiometricPrompt.CryptoObject(firma), cancelar, getMainExecutor(),
                    new BiometricPrompt.AuthenticationCallback() {
                        @Override public void onAuthenticationSucceeded(BiometricPrompt.AuthenticationResult r) {
                            try {
                                if (r.getCryptoObject() == null || r.getCryptoObject().getSignature() != firma) {
                                    throw new SecurityException("The owner proof lost its CryptoObject");
                                }
                                firma.update(reto.mensaje);
                                if (!SesionProveedor.completar(reto, firma.sign())) {
                                    SesionProveedor.cancelar(nonce);
                                }
                            } catch (Exception fallo) {
                                SesionProveedor.cancelar(nonce);
                            }
                            finish();
                        }
                        @Override public void onAuthenticationError(int codigo, CharSequence texto) {
                            SesionProveedor.cancelar(nonce);
                            finish();
                        }
                    });
        } catch (Exception fallo) {
            SesionProveedor.cancelar(nonce);
            finish();
        }
    }

    @Override public void onDestroy() {
        cancelar.cancel();
        if (nonce != null) {
            SesionProveedor.cancelar(nonce);
        }
        super.onDestroy();
    }
}
