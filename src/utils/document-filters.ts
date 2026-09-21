import type { Dayjs } from "dayjs";

// The document-list filtering rules, shared by every list page: a
// case-insensitive text search across the given fields, an exact
// status/state match, an exact customer/vendor id match, and an inclusive
// day range on the document's own date field. Pure, so it lives in utils/
// (and has a test) rather than inside the DocumentFilters component.
export function matchesDocumentFilters(opts: {
  search: string;
  searchFields: (string | number | null | undefined)[];
  status: string;
  rowStatus: string | null | undefined;
  partyId: string;
  rowPartyId: string | null | undefined;
  dateRange: [Dayjs, Dayjs] | null;
  rowDate: number | null | undefined;
}): boolean {
  const { search, searchFields, status, rowStatus, partyId, rowPartyId, dateRange, rowDate } = opts;
  if (search) {
    const needle = search.toLowerCase();
    if (!searchFields.some((f) => (f ?? "").toString().toLowerCase().includes(needle))) {
      return false;
    }
  }
  if (status && rowStatus !== status) return false;
  if (partyId && rowPartyId !== partyId) return false;
  // Inclusive of both bounds' whole day: a date exactly on the "to" day must
  // still match (endOf("day"), not the day's 00:00).
  if (dateRange?.[0] && (rowDate ?? 0) < dateRange[0].startOf("day").valueOf()) return false;
  if (dateRange?.[1] && (rowDate ?? 0) > dateRange[1].endOf("day").valueOf()) return false;
  return true;
}
