// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package eu.sytue.nativo;

import java.util.ArrayList;
import java.util.Arrays;
import java.util.Collections;
import java.util.List;

/** El mismo contrato de privacidad que widgets_test.go, ejecutado en la JVM remota. */
public final class WidgetSnapshotPrueba {
    private static final long NOW = 1792500000L;
    private static int checks;

    private static void check(boolean ok, String message) {
        checks++;
        if (!ok) {
            throw new AssertionError(message);
        }
    }

    private static WidgetSnapshot make(long version, String product, String generation,
            long updated, long expires, boolean active, WidgetSnapshot.Counter... counters) {
        return new WidgetSnapshot(version, product, generation, updated, expires, active, Arrays.asList(counters));
    }

    private static boolean visible(WidgetSnapshot s) {
        return s.visible("mail", "generation-one", false, NOW);
    }

    public static void main(String[] args) {
        WidgetSnapshot.Counter unread = new WidgetSnapshot.Counter("unread", 7);
        WidgetSnapshot valid = make(1, "mail", "generation-one", NOW, NOW + 120, true, unread);
        check(visible(valid), "valid snapshot");
        for (String kind : Arrays.asList("calendars", "address_books")) {
            check(visible(make(1, "mail", "generation-one", NOW, NOW + 120, true,
                    new WidgetSnapshot.Counter(kind, 2))), "complete collection kind " + kind);
        }
        check(!valid.visible("mail", "generation-two", false, NOW), "late old account response");
        check(!valid.visible("mail", "", false, NOW), "logout");
        check(!valid.visible("rooms", "generation-one", false, NOW), "different product");
        check(!valid.visible("mail", "generation-one", true, NOW), "session/OS lock");
        check(!valid.visible("mail", "generation-one", false, NOW + 120), "expiry");
        check(!visible(make(2, "mail", "generation-one", NOW, NOW + 120, true)), "future schema");
        check(!visible(make(1, "mail", "", NOW, NOW + 120, true)), "empty generation");
        check(!visible(make(1, "mail", "generation-one", 0, NOW + 120, true)), "zero timestamp");
        check(!visible(make(1, "mail", "generation-one", NOW + 6, NOW + 120, true)), "future timestamp");
        check(!visible(make(1, "mail", "generation-one", NOW, NOW + 1801, true)), "excessive TTL");
        check(!visible(make(1, "mail", "generation-one", NOW, NOW, true)), "empty TTL");
        check(!visible(make(1, "mail", "generation-one", NOW, NOW + 120, false)), "inactive");
        check(!visible(make(1, "mail", "generation-one", NOW, NOW + 120, true,
                new WidgetSnapshot.Counter("subject", 1))), "no message content category");
        check(!visible(make(1, "mail", "generation-one", NOW, NOW + 120, true,
                new WidgetSnapshot.Counter("unread", -1))), "negative count");
        check(!visible(make(1, "mail", "generation-one", NOW, NOW + 120, true,
                new WidgetSnapshot.Counter("unread", 1_000_000_000_001L))), "count bound");
        check(!visible(make(1, "mail", "generation-one", NOW, NOW + 120, true, unread, unread)), "duplicate kind");
        List<WidgetSnapshot.Counter> values = new ArrayList<>(Collections.nCopies(13, unread));
        check(!visible(new WidgetSnapshot(1, "mail", "generation-one", NOW, NOW + 120, true, values)), "list bound");
        values.clear();
        values.add(unread);
        WidgetSnapshot detached = new WidgetSnapshot(1, "mail", "generation-one", NOW, NOW + 120, true, values);
        values.clear();
        check(detached.counters.size() == 1, "defensive copy");
        System.out.println("Widget snapshot: " + checks + " checks passed");
    }
}
