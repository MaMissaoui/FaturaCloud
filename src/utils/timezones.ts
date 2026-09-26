import { t } from "@lingui/core/macro";

// organizations.timezone (migration 0092) is the IANA zone the server reads
// every stored date's calendar day in — printed document dates, FEC/DATEV,
// document-number date tokens, the Cash Book's daily register. "" means not
// set: the server keeps UTC days, as before the setting existed. Like
// documentLanguage, "Not set" is an explicit "" option rather than
// allowClear, because a cleared Select submits nothing and the server's
// COALESCE update would silently keep the old zone.

// Accessed through a narrow type rather than the ES2022.Intl lib, so a
// browser without supportedValuesOf still gets a usable (if short) list.
const supportedTimezones = (): string[] => {
  const intl = Intl as unknown as { supportedValuesOf?: (key: "timeZone") => string[] };
  try {
    return intl.supportedValuesOf?.("timeZone") ?? [];
  } catch {
    return [];
  }
};

// The browser's own zone, used to prefill a new organization.
export const browserTimezone = (): string => {
  try {
    return Intl.DateTimeFormat().resolvedOptions().timeZone ?? "";
  } catch {
    return "";
  }
};

// The prefill for a new organization: the browser's zone when it's a real,
// listed IANA zone, else "Not set". Some environments report a placeholder
// such as "Etc/Unknown", which the server would reject on save.
export const defaultTimezone = (): string => {
  const zone = browserTimezone();
  const known = supportedTimezones();
  return zone && (zone === "UTC" || known.includes(zone)) ? zone : "";
};

export const normalizeTimezone = (value: string | null | undefined): string => value ?? "";

// Every selectable zone, "Not set" first. UTC and the currently stored
// value are always included: V8's list omits "UTC", and a zone the browser
// doesn't list must still display rather than render as a bare id.
export const timezoneOptions = (current?: string | null): { value: string; label: string }[] => {
  const zones = new Set(supportedTimezones());
  zones.add("UTC");
  if (current) zones.add(current);
  return [
    { value: "", label: t`Not set (UTC)` },
    ...[...zones].sort().map((zone) => ({ value: zone, label: zone.replace(/_/g, " ") })),
  ];
};
