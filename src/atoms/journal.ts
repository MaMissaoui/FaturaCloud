import { atom } from "jotai";
import type { Journal } from "src/types/models";
import { message } from "src/utils/message";
import { nanoid } from "nanoid";
import { t } from "@lingui/core/macro";
import orderBy from "lodash/orderBy";
import keyBy from "lodash/keyBy";
import map from "lodash/map";
import reject from "lodash/reject";
import isEqual from "lodash/isEqual";
import { GetJournals, CreateJournal, UpdateJournal, DeleteJournal } from "src/api";

import { organizationIdAtom } from "./organization";

// Journals
export const journalsAtom = atom<Journal[]>([]);
journalsAtom.debugLabel = "journalsAtom";

export const setJournalsAtom = atom(null, async (get, set) => {
  const organizationId = get(organizationIdAtom);
  try {
    const response = await GetJournals(organizationId!);
    set(journalsAtom, response);
  } catch (error) {
    console.error("Failed to fetch journals:", error);
    message.error(t`Failed to fetch journals`);
    set(journalsAtom, []);
  }
});
setJournalsAtom.debugLabel = "setJournalsAtom";

// Journal
export const journalIdAtom = atom<string | null>(null);
journalIdAtom.debugLabel = "journalIdAtom";

export const journalAtom = atom(
  (get) => {
    const journalId = get(journalIdAtom);
    if (!journalId) return null;
    return get(journalsAtom).find((j) => j.id === journalId) ?? null;
  },
  async (get, set, newValues: Partial<Journal>) => {
    const journalId = get(journalIdAtom);

    try {
      if (!journalId) {
        const created = await CreateJournal({
          ...newValues,
          id: nanoid(),
          organizationId: get(organizationIdAtom)!,
        });
        message.success(t`Journal created`);
        const journals = get(journalsAtom);
        set(journalsAtom, orderBy([...journals, created], "code", "asc"));
      } else {
        const updated = await UpdateJournal(journalId, newValues);
        message.success(t`Journal updated`);
        const journals = get(journalsAtom);
        const merged = keyBy([...journals, updated], "id");
        set(journalsAtom, orderBy(map(merged), "code", "asc"));
      }
    } catch (error) {
      console.error("Journal operation failed:", error);
      message.error(error instanceof Error ? error.message : t`Journal save failed`);
      throw error;
    }
  },
);

// Delete journal
export const deleteJournalAtom = atom(null, async (get, set, journalId: string) => {
  try {
    const success = await DeleteJournal(journalId);
    if (success) {
      set(
        journalsAtom,
        reject(get(journalsAtom), (j) => isEqual(j.id, journalId)),
      );
      message.success(t`Journal deleted`);
    }
    return success;
  } catch (error) {
    console.error("Failed to delete journal:", error);
    message.error(error instanceof Error ? error.message : t`Journal deletion failed`);
    return false;
  }
});
