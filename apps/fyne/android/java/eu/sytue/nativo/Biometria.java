// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package eu.sytue.nativo;

import android.content.Context;
import android.hardware.biometrics.BiometricManager;
import android.hardware.biometrics.BiometricPrompt;
import android.os.Build;
import android.os.CancellationSignal;
import android.os.Handler;
import android.security.keystore.KeyGenParameterSpec;
import android.security.keystore.KeyProperties;
import java.nio.charset.StandardCharsets;
import java.security.KeyPairGenerator;
import java.security.KeyStore;
import java.security.PrivateKey;
import java.security.Signature;
import java.security.spec.ECGenParameterSpec;
import java.util.UUID;
import java.util.concurrent.ArrayBlockingQueue;
import java.util.concurrent.TimeUnit;

/**
 * BiometricPrompt para `escritorio.Verificar` en Android: huella o cara y, si no, el
 * bloqueo del teléfono, como la política de Apple que acepta la contraseña del equipo.
 * {@link #verificar} ESPERA la respuesta, como Certificados: Go la llama desde un hilo
 * suyo, nunca desde el principal, y nada llama a Go desde aquí.
 */
public final class Biometria {
    private static final String CLAVE_LOCAL = "eu.sytue.one.biometria.local.v1";
    private final ArrayBlockingQueue<Integer> respuesta = new ArrayBlockingQueue<>(1);

    private Biometria() {}

    /** 0 sin nada con que verificar; 1 con huella o cara fuertes; 2 solo con el bloqueo. */
    public static int disponible(Context ctx) {
        if (Build.VERSION.SDK_INT < 30) {
            return 0;
        }
        BiometricManager m = ctx.getSystemService(BiometricManager.class);
        if (m == null) {
            return 0;
        }
        if (m.canAuthenticate(BiometricManager.Authenticators.BIOMETRIC_STRONG) == BiometricManager.BIOMETRIC_SUCCESS) {
            return 1;
        }
        if (m.canAuthenticate(BiometricManager.Authenticators.DEVICE_CREDENTIAL) == BiometricManager.BIOMETRIC_SUCCESS) {
            return 2;
        }
        return 0;
    }

    /**
     * Cómo llama el teléfono a su verificación, en su idioma («Huella digital», «Desbloqueo
     * facial»): el «Touch ID» de Android. "" antes de Android 12, que no lo dice.
     */
    public static String nombre(Context ctx) {
        if (Build.VERSION.SDK_INT < 31) {
            return "";
        }
        BiometricManager m = ctx.getSystemService(BiometricManager.class);
        CharSequence s = m == null ? null
                : m.getStrings(BiometricManager.Authenticators.BIOMETRIC_STRONG).getSettingName();
        return s == null ? "" : s.toString();
    }

    /**
     * Enseña el diálogo en el hilo principal, que es donde Android lo quiere, y espera la
     * respuesta hasta cinco minutos: 0 si se ha verificado; si no, el código de
     * BiometricPrompt (-1 si no hay código o se agotó el tiempo).
     */
    public static int verificar(final Context ctx, final String motivo) throws InterruptedException {
        if (SesionCompartida.habilitada()) {
            return SesionCompartida.verificar(ctx, motivo);
        }
        final Biometria b = new Biometria();
        final CancellationSignal cancelar = new CancellationSignal();
        new Handler(ctx.getMainLooper()).post(new Runnable() {
            @Override
            public void run() {
                try {
                    BiometricPrompt dialog = prompt(ctx,
                            ctx.getApplicationInfo().loadLabel(ctx.getPackageManager()), motivo);
                    final Signature firma = firmaLocal();
                    dialog.authenticate(new BiometricPrompt.CryptoObject(firma), cancelar, ctx.getMainExecutor(),
                            new BiometricPrompt.AuthenticationCallback() {
                                @Override
                                public void onAuthenticationSucceeded(BiometricPrompt.AuthenticationResult r) {
                                    try {
                                        BiometricPrompt.CryptoObject crypto = r.getCryptoObject();
                                        Signature firmaAutorizada = crypto == null ? null : crypto.getSignature();
                                        if (firmaAutorizada == null || firmaAutorizada != firma) {
                                            throw new SecurityException("Biometric proof lost its CryptoObject");
                                        }
                                        firmaAutorizada.update(UUID.randomUUID().toString().getBytes(StandardCharsets.UTF_8));
                                        if (firmaAutorizada.sign().length == 0) {
                                            throw new SecurityException("Biometric proof was empty");
                                        }
                                        b.respuesta.offer(0);
                                    } catch (Exception e) {
                                        b.respuesta.offer(-1);
                                    }
                                }

                                @Override
                                public void onAuthenticationError(int codigo, CharSequence texto) {
                                    b.respuesta.offer(codigo == 0 ? -1 : codigo);
                                }
                            });
                } catch (Exception e) {
                    b.respuesta.offer(-1);
                }
            }
        });
        Integer r = b.respuesta.poll(5, TimeUnit.MINUTES);
        if (r == null) {
            cancelar.cancel();
            return -1;
        }
        return r;
    }

    /** Same OS policy for local verification and the broker's auth-per-use key. */
    static BiometricPrompt prompt(Context ctx, CharSequence titulo, CharSequence motivo) {
        if (Build.VERSION.SDK_INT < 30) {
            throw new IllegalStateException("Secure owner verification requires Android 11");
        }
        return new BiometricPrompt.Builder(ctx).setTitle(titulo).setSubtitle(motivo)
                .setAllowedAuthenticators(BiometricManager.Authenticators.BIOMETRIC_STRONG
                        | BiometricManager.Authenticators.DEVICE_CREDENTIAL).build();
    }

    private static synchronized Signature firmaLocal() throws Exception {
        KeyStore almacen = KeyStore.getInstance("AndroidKeyStore");
        almacen.load(null);
        if (!almacen.containsAlias(CLAVE_LOCAL)) {
            KeyPairGenerator generador = KeyPairGenerator.getInstance(KeyProperties.KEY_ALGORITHM_EC, "AndroidKeyStore");
            generador.initialize(new KeyGenParameterSpec.Builder(CLAVE_LOCAL, KeyProperties.PURPOSE_SIGN)
                    .setAlgorithmParameterSpec(new ECGenParameterSpec("secp256r1"))
                    .setDigests(KeyProperties.DIGEST_SHA256).setUserAuthenticationRequired(true)
                    .setUserAuthenticationParameters(0,
                            KeyProperties.AUTH_BIOMETRIC_STRONG | KeyProperties.AUTH_DEVICE_CREDENTIAL).build());
            generador.generateKeyPair();
            almacen.load(null);
        }
        Signature firma = Signature.getInstance("SHA256withECDSA");
        firma.initSign((PrivateKey) almacen.getKey(CLAVE_LOCAL, null));
        return firma;
    }

}
