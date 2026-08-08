import { atom } from "jotai";
import type { FiscalYear, FiscalPeriod } from "src/types/models";
import { message } from "src/utils/message";
import { nanoid } from "nanoid";
import { t } from "@lingui/core/macro";
import orderBy from "lodash/orderBy";

import {
  GetFiscalYears,
  CreateFiscalYear,
  GetFiscalPeriods,
  CreateFiscalPeriod,
  UpdateFiscalPeriodStatus,
} from "src/api";

import { organizationIdAtom } from "./organization";

// Fiscal years
export const fiscalYearsAtom = atom<FiscalYear[]>([]);
fiscalYearsAtom.debugLabel = "fiscalYearsAtom";

export const setFiscalYearsAtom = atom(null, async (get, set) => {
  const organizationId = get(organizationIdAtom);
  try {
    const response = await GetFiscalYears(organizationId!);
    set(fiscalYearsAtom, response);
  } catch (error) {
    console.error("Failed to fetch fiscal years:", error);
    message.error(t`Failed to fetch fiscal years`);
    set(fiscalYearsAtom, []);
  }
});
setFiscalYearsAtom.debugLabel = "setFiscalYearsAtom";

// Fiscal years are immutable once created (no update endpoint) — only
// created and, eventually, closed (Phase 6).
export const createFiscalYearAtom = atom(null, async (get, set, values: Partial<FiscalYear>) => {
  try {
    const created = await CreateFiscalYear({
      ...values,
      id: nanoid(),
      organizationId: get(organizationIdAtom)!,
    });
    message.success(t`Fiscal year created`);
    set(fiscalYearsAtom, orderBy([...get(fiscalYearsAtom), created], "startDate", "desc"));
    return created;
  } catch (error) {
    console.error("Failed to create fiscal year:", error);
    message.error(error instanceof Error ? error.message : t`Fiscal year creation failed`);
    return null;
  }
});

// Fiscal periods, cached per fiscal year (a year's periods are only ever
// looked at in the context of that year, so there's no flat list atom).
export const fiscalPeriodsAtom = atom<Record<string, FiscalPeriod[]>>({});
fiscalPeriodsAtom.debugLabel = "fiscalPeriodsAtom";

export const loadFiscalPeriodsAtom = atom(null, async (get, set, fiscalYearId: string) => {
  try {
    const periods = await GetFiscalPeriods(fiscalYearId);
    set(fiscalPeriodsAtom, { ...get(fiscalPeriodsAtom), [fiscalYearId]: periods });
  } catch (error) {
    console.error("Failed to fetch fiscal periods:", error);
    message.error(t`Failed to fetch fiscal periods`);
  }
});

export const createFiscalPeriodAtom = atom(
  null,
  async (get, set, values: Partial<FiscalPeriod>) => {
    try {
      const created = await CreateFiscalPeriod({
        ...values,
        id: nanoid(),
        organizationId: get(organizationIdAtom)!,
      });
      message.success(t`Fiscal period created`);
      const existing = get(fiscalPeriodsAtom)[created.fiscalYearId] ?? [];
      set(fiscalPeriodsAtom, {
        ...get(fiscalPeriodsAtom),
        [created.fiscalYearId]: orderBy([...existing, created], "startDate", "asc"),
      });
      return created;
    } catch (error) {
      console.error("Failed to create fiscal period:", error);
      message.error(error instanceof Error ? error.message : t`Fiscal period creation failed`);
      return null;
    }
  },
);

export const updateFiscalPeriodStatusAtom = atom(
  null,
  async (get, set, { id, status }: { id: string; status: "open" | "closed" }) => {
    try {
      const updated = await UpdateFiscalPeriodStatus(id, status);
      message.success(t`Fiscal period updated`);
      const existing = get(fiscalPeriodsAtom)[updated.fiscalYearId] ?? [];
      set(fiscalPeriodsAtom, {
        ...get(fiscalPeriodsAtom),
        [updated.fiscalYearId]: existing.map((p) => (p.id === updated.id ? updated : p)),
      });
      return updated;
    } catch (error) {
      console.error("Failed to update fiscal period:", error);
      message.error(error instanceof Error ? error.message : t`Fiscal period update failed`);
      return null;
    }
  },
);
