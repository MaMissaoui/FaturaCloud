import { atom } from "jotai";
import { message } from "src/utils/message";
import { nanoid } from "nanoid";
import { t } from "@lingui/core/macro";
import dayjs from "dayjs";
import isEqual from "lodash/isEqual";
import orderBy from "lodash/orderBy";
import keyBy from "lodash/keyBy";
import map from "lodash/map";
import reject from "lodash/reject";

import {
  GetJournalEntries,
  GetJournalEntry,
  GetJournalEntryLines,
  CreateJournalEntry,
  PostJournalEntry,
  ReverseJournalEntry,
  DeleteJournalEntry,
} from "src/api";
import { centsToUnits, unitsToCents } from "src/utils/currency";
import { organizationIdAtom } from "./organization";

// Journal entries list
export const journalEntriesAtom = atom<any[]>([]);
journalEntriesAtom.debugLabel = "journalEntriesAtom";

export const journalEntryFiltersAtom = atom<{ journalId?: string; status?: string }>({});
journalEntryFiltersAtom.debugLabel = "journalEntryFiltersAtom";

export const setJournalEntriesAtom = atom(null, async (get, set) => {
  const organizationId = get(organizationIdAtom);
  try {
    const response = await GetJournalEntries(organizationId!, get(journalEntryFiltersAtom));
    set(journalEntriesAtom, response);
  } catch (error) {
    console.error("Failed to fetch journal entries:", error);
    message.error(t`Failed to fetch journal entries`);
    set(journalEntriesAtom, []);
  }
});

// Single journal entry (read+create). There is no update endpoint — a draft
// is deleted and recreated rather than edited in place, and a posted entry
// is immutable (see ReverseJournalEntry).
export const journalEntryIdAtom = atom<string | null>(null);

export const journalEntryAtom = atom(
  async (get) => {
    const entryId = get(journalEntryIdAtom);
    if (!entryId) return null;
    try {
      const [entry, lines] = await Promise.all([
        GetJournalEntry(entryId),
        GetJournalEntryLines(entryId),
      ]);
      if (!entry) return null;
      return {
        ...entry,
        date: dayjs(entry.date),
        lines: (lines || []).map((line: any) => ({
          ...line,
          debit: centsToUnits(line.debit),
          credit: centsToUnits(line.credit),
        })),
      };
    } catch (error) {
      console.error("Failed to fetch journal entry:", error);
      message.error(t`Failed to fetch journal entry`);
      return null;
    }
  },
  async (get, set, newValues: any) => {
    const toTimestamp = (v: any) => (v?.valueOf ? v.valueOf() : v);
    const data = {
      ...newValues,
      id: nanoid(),
      organizationId: get(organizationIdAtom),
      date: toTimestamp(newValues.date),
      lines: (newValues.lines || []).map((line: any) => ({
        ...line,
        debit: unitsToCents(line.debit || 0),
        credit: unitsToCents(line.credit || 0),
      })),
    };
    try {
      const created = await CreateJournalEntry(data);
      set(journalEntryIdAtom, created.id);
      message.success(t`Journal entry created`);
      const entries = get(journalEntriesAtom);
      set(journalEntriesAtom, orderBy([created, ...entries], "date", "desc"));
      return created;
    } catch (error) {
      console.error("Journal entry creation failed:", error);
      message.error(error instanceof Error ? error.message : t`Journal entry creation failed`);
      throw error;
    }
  },
);

export const postJournalEntryAtom = atom(null, async (get, set, entryId: string) => {
  try {
    const updated = await PostJournalEntry(entryId);
    message.success(t`Journal entry posted`);
    const entries = get(journalEntriesAtom);
    const merged = keyBy([...entries, updated], "id");
    set(journalEntriesAtom, orderBy(map(merged), "date", "desc"));
    return updated;
  } catch (error) {
    console.error("Failed to post journal entry:", error);
    message.error(error instanceof Error ? error.message : t`Failed to post journal entry`);
    return null;
  }
});

export const reverseJournalEntryAtom = atom(
  null,
  async (
    _get,
    set,
    { entryId, reason, date }: { entryId: string; reason: string; date: number },
  ) => {
    try {
      const reversal = await ReverseJournalEntry(entryId, reason, date);
      message.success(t`Journal entry reversed`);
      // Refresh the list so both the original (now 'reversed') and the new
      // reversal entry show their current state.
      await set(setJournalEntriesAtom);
      return reversal;
    } catch (error) {
      console.error("Failed to reverse journal entry:", error);
      message.error(error instanceof Error ? error.message : t`Failed to reverse journal entry`);
      return null;
    }
  },
);

export const deleteJournalEntryAtom = atom(null, async (get, set, entryId: string) => {
  try {
    const success = await DeleteJournalEntry(entryId);
    if (success) {
      set(
        journalEntriesAtom,
        reject(get(journalEntriesAtom), (e: any) => isEqual(e.id, entryId)),
      );
      message.success(t`Journal entry deleted`);
    }
    return success;
  } catch (error) {
    console.error("Failed to delete journal entry:", error);
    message.error(error instanceof Error ? error.message : t`Journal entry deletion failed`);
    return false;
  }
});
