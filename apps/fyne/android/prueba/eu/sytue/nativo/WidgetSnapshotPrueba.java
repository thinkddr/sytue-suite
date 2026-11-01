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
        for (long percent : new long[] {0, 32, 100}) {
            check(visible(make(1, "mail", "generation-one", NOW, NOW + 120, true,
                    new WidgetSnapshot.Counter("storage_percent", percent))), "valid storage percentage");
        }
        check(!visible(make(1, "mail", "generation-one", NOW, NOW + 120, true,
                new WidgetSnapshot.Counter("storage_percent", 101))), "invalid storage percentage");
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
        WidgetSnapshot.Item okItem = new WidgetSnapshot.Item(
                "Revisión trimestral", "Sala Norte", "10:00", "#953be7", NOW + 60, NOW + 3600, false, true, false);
        WidgetSnapshot.CalendarGrid okCal = new WidgetSnapshot.CalendarGrid(
                2026, 11, Arrays.asList(3, 14, 28), Collections.singletonMap("3", "#953be7"));
        WidgetSnapshot rich = new WidgetSnapshot(
                1, "calendar", "generation-one", NOW, NOW + 120, true,
                Collections.singletonList(new WidgetSnapshot.Counter("events", 2)),
                Collections.singletonList(okItem), okCal);
        check(rich.visible("calendar", "generation-one", false, NOW), "rich calendar snapshot");
        check(!new WidgetSnapshot(1, "mail", "generation-one", NOW, NOW + 120, true, values,
                Collections.singletonList(new WidgetSnapshot.Item("enc:v1:secret", "", "", "", 0, 0, false, false, false)), null)
                .visible("mail", "generation-one", false, NOW), "reject encrypted title");
        check(!new WidgetSnapshot(1, "mail", "generation-one", NOW, NOW + 120, true, values,
                Collections.singletonList(new WidgetSnapshot.Item("Title", "ENC:V1:secret", "", "", 0, 0, false, false, false)), null)
                .visible("mail", "generation-one", false, NOW), "reject encrypted subtitle");
        check(!new WidgetSnapshot(1, "mail", "generation-one", NOW, NOW + 120, true, values,
                Collections.nCopies(9, okItem), null)
                .visible("mail", "generation-one", false, NOW), "reject >8 items");
        check(!new WidgetSnapshot(1, "calendar", "generation-one", NOW, NOW + 120, true, values, null,
                new WidgetSnapshot.CalendarGrid(2026, 2, Collections.singletonList(29), null))
                .visible("calendar", "generation-one", false, NOW), "reject non-leap Feb 29");
        check(!new WidgetSnapshot(1, "calendar", "generation-one", NOW, NOW + 120, true, values, null,
                new WidgetSnapshot.CalendarGrid(2026, 11, Arrays.asList(14, 3), null))
                .visible("calendar", "generation-one", false, NOW), "reject unsorted busy days");
        check(!new WidgetSnapshot(1, "calendar", "generation-one", NOW, NOW + 120, true, values, null,
                new WidgetSnapshot.CalendarGrid(2026, 11, null, Collections.singletonMap("03", "#953be7")))
                .visible("calendar", "generation-one", false, NOW), "reject non-canonical day key");
        System.out.println("Widget snapshot: " + checks + " checks passed");
    }
}
