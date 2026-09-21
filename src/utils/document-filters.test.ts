import { describe, expect, it } from "vitest";
import dayjs from "dayjs";
import { matchesDocumentFilters } from "src/utils/document-filters";

// A minimal row for the common shape: a text field, a status, a party id and
// an epoch-ms date.
const row = {
  number: "INV-2026-0042",
  clientName: "Anis Sassi",
  status: "sent",
  clientId: "c-1",
  date: dayjs("2026-09-20T12:00:00").valueOf(),
};

const base = {
  search: "",
  searchFields: [row.number, row.clientName],
  status: "",
  rowStatus: row.status,
  partyId: "",
  rowPartyId: row.clientId,
  dateRange: null,
  rowDate: row.date,
};

describe("matchesDocumentFilters", () => {
  it("matches everything when no filter is set", () => {
    expect(matchesDocumentFilters(base)).toBe(true);
  });

  it("searches case-insensitively across the given fields", () => {
    expect(matchesDocumentFilters({ ...base, search: "inv-2026" })).toBe(true);
    expect(matchesDocumentFilters({ ...base, search: "sassi" })).toBe(true);
    expect(matchesDocumentFilters({ ...base, search: "nope" })).toBe(false);
  });

  it("treats a missing/null search field as an empty string, not a crash", () => {
    expect(
      matchesDocumentFilters({
        ...base,
        search: "widget",
        searchFields: [null, undefined, "Widget Co"],
      }),
    ).toBe(true);
  });

  it("requires an exact status match", () => {
    expect(matchesDocumentFilters({ ...base, status: "sent" })).toBe(true);
    expect(matchesDocumentFilters({ ...base, status: "paid" })).toBe(false);
  });

  it("requires an exact party id match", () => {
    expect(matchesDocumentFilters({ ...base, partyId: "c-1" })).toBe(true);
    expect(matchesDocumentFilters({ ...base, partyId: "c-2" })).toBe(false);
  });

  it("includes both ends of the date range's day", () => {
    const onTheDay = dayjs("2026-09-20T00:00:00").valueOf();
    const range = [dayjs("2026-09-20"), dayjs("2026-09-20")] as [dayjs.Dayjs, dayjs.Dayjs];
    // A row whose date is the very start of the "to" day must still match.
    expect(matchesDocumentFilters({ ...base, rowDate: onTheDay, dateRange: range })).toBe(true);
    // A row one day later must not.
    expect(
      matchesDocumentFilters({
        ...base,
        rowDate: dayjs("2026-09-21T00:00:00").valueOf(),
        dateRange: range,
      }),
    ).toBe(false);
  });

  it("combines filters with AND", () => {
    expect(
      matchesDocumentFilters({ ...base, search: "sassi", status: "sent", partyId: "c-1" }),
    ).toBe(true);
    expect(
      matchesDocumentFilters({ ...base, search: "sassi", status: "paid", partyId: "c-1" }),
    ).toBe(false);
  });
});
