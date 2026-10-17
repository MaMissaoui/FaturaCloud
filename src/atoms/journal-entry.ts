import { atom } from "jotai";
import type { Dayjs } from "dayjs";
import type { JournalEntry, JournalLine } from "src/types/models";
import { message } from "src/utils/message";
import { nanoid } from "nanoid";
import { t } from "@lingui/core/macro";
import dayjs from "dayjs";
import isEqual from "lodash/isEqual";
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
export const journalEntriesAtom = atom<JournalEntry[]>([]);
journalEntriesAtom.debugLabel = "journalEntriesAtom";

export const journalEntryFiltersAtom = atom<{ journalId?: string; status?: string }>({});
journalEntryFiltersAtom.debugLabel = "journalEntryFiltersAtom";

// The list is read one page at a time (audit F176), so the page, its size
// and the sort live here with the filters, and the total beside the rows.
export type JournalEntryPaging = {
  page: number;
  pageSize: number;
  sort?: string;
  order?: "asc" | "desc";
};
export const journalEntryPagingAtom = atom<JournalEntryPaging>({ page: 1, pageSize: 25 });
journalEntryPagingAtom.debugLabel = "journalEntryPagingAtom";
export const journalEntriesTotalAtom = atom(0);
journalEntriesTotalAtom.debugLabel = "journalEntriesTotalAtom";

export const setJournalEntriesAtom = atom(null, async (get, set) => {
  const organizationId = get(organizationIdAtom);
  const { page, pageSize, sort, order } = get(journalEntryPagingAtom);
  try {
    const response = await GetJournalEntries(organizationId!, {
      ...get(journalEntryFiltersAtom),
      limit: pageSize,
      offset: (page - 1) * pageSize,
      sort,
      order,
    });
    set(journalEntriesAtom, response.data);
    set(journalEntriesTotalAtom, response.total);
  } catch (error) {
    console.error("Failed to fetch journal entries:", error);
    message.error(t`Failed to fetch journal entries`);
    set(journalEntriesAtom, []);
    set(journalEntriesTotalAtom, 0);
  }
});

// Single journal entry (read+create). There is no update endpoint — a draft
// is deleted and recreated rather than edited in place, and a posted entry
// is immutable (see ReverseJournalEntry).
// The journal entry form edits date as a dayjs object and each line's
// debit/credit in display currency units — not the wire shape either field
// is stored as.
type JournalLineFormValues = Omit<Partial<JournalLine>, "debit" | "credit"> & {
  debit?: number;
  credit?: number;
};
type JournalEntryFormValues = Omit<Partial<JournalEntry>, "date"> & {
  date?: Dayjs | number;
  lines?: JournalLineFormValues[];
};

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
        lines: (lines || []).map((line) => ({
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
  async (get, set, newValues: JournalEntryFormValues) => {
    const toTimestamp = (v: Dayjs | number | null | undefined) =>
      v && typeof v === "object" && "valueOf" in v ? v.valueOf() : v;
    const data = {
      ...newValues,
      id: nanoid(),
      organizationId: get(organizationIdAtom)!,
      date: toTimestamp(newValues.date),
      lines: (newValues.lines || []).map((line) => ({
        ...line,
        debit: unitsToCents(line.debit || 0),
        credit: unitsToCents(line.credit || 0),
      })),
    };
    try {
      const created = await CreateJournalEntry(data);
      set(journalEntryIdAtom, created.id);
      message.success(t`Journal entry created`);
      // The list holds one page; it reloads when the user returns to it.
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
    // The list holds one page: update the entry where it is.
    set(
      journalEntriesAtom,
      get(journalEntriesAtom).map((e) => (e.id === updated.id ? updated : e)),
    );
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
        reject(get(journalEntriesAtom), (e) => isEqual(e.id, entryId)),
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
