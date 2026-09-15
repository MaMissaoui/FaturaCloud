import { atom } from "jotai";
import type { UnitOfMeasure } from "src/types/models";
import { message } from "src/utils/message";
import { nanoid } from "nanoid";
import { t } from "@lingui/core/macro";
import orderBy from "lodash/orderBy";
import keyBy from "lodash/keyBy";
import map from "lodash/map";
import reject from "lodash/reject";
import isEqual from "lodash/isEqual";
import {
  GetUnitsOfMeasure,
  CreateUnitOfMeasure,
  UpdateUnitOfMeasure,
  DeleteUnitOfMeasure,
} from "src/api";

import { organizationIdAtom } from "./organization";

// Units of measure list
export const unitsOfMeasureAtom = atom<UnitOfMeasure[]>([]);
export const setUnitsOfMeasureAtom = atom(null, async (get, set) => {
  const organizationId = get(organizationIdAtom);
  try {
    const response = await GetUnitsOfMeasure(organizationId!);
    set(unitsOfMeasureAtom, response);
  } catch (error) {
    console.error("Failed to fetch units of measure:", error);
    message.error(t`Failed to fetch units of measure`);
    set(unitsOfMeasureAtom, []);
  }
});

// The server clears the previous default inside the same transaction when a
// row is saved with isDefault = 1, but the client only ever merges back the
// row it saved — so without this two rows render as "Default" until the next
// fetch, and a consumer doing find(list, {isDefault: 1}) can pick the stale
// one (F87). Mirrors the server's own single-default invariant locally.
const applySingleDefault = <T extends { id: string; isDefault?: number | null }>(
  rows: T[],
  savedID: string,
  savedIsDefault: number,
): T[] =>
  savedIsDefault === 1 ? rows.map((r) => (r.id === savedID ? r : { ...r, isDefault: 0 })) : rows;

// Single unit of measure (read+write), for the Settings drawer form. There's
// no GET /units-of-measure/{id} endpoint — the list is always small enough
// that the drawer just looks the record up from the already-loaded list.
export const unitOfMeasureIdAtom = atom<string | null>(null);

// The settings drawer edits isDefault as a boolean (a Checkbox), not the
// wire type's number/0-1.
type UnitOfMeasureFormValues = Omit<Partial<UnitOfMeasure>, "isDefault"> & {
  isDefault?: boolean | number | null;
};

export const unitOfMeasureAtom = atom(
  (get) => {
    const id = get(unitOfMeasureIdAtom);
    if (!id) return null;
    return get(unitsOfMeasureAtom).find((u) => u.id === id) ?? null;
  },
  async (get, set, newValues: UnitOfMeasureFormValues) => {
    const id = get(unitOfMeasureIdAtom);
    try {
      if (!id) {
        const data = {
          ...newValues,
          id: nanoid(),
          organizationId: get(organizationIdAtom)!,
          isDefault: newValues.isDefault ? 1 : 0,
        };
        const created = await CreateUnitOfMeasure(data);
        set(unitOfMeasureIdAtom, created.id);
        message.success(t`Unit of measure created`);
        const units = get(unitsOfMeasureAtom);
        set(
          unitsOfMeasureAtom,
          applySingleDefault(
            orderBy([...units, created], "name", "asc"),
            created.id,
            data.isDefault,
          ),
        );
      } else {
        const data = { ...newValues, isDefault: newValues.isDefault ? 1 : 0 };
        const updated = await UpdateUnitOfMeasure(id, data);
        message.success(t`Unit of measure updated`);
        const units = get(unitsOfMeasureAtom);
        const merged = keyBy([...units, updated], "id");
        set(
          unitsOfMeasureAtom,
          applySingleDefault(orderBy(map(merged), "name", "asc"), updated.id, data.isDefault),
        );
      }
    } catch (error) {
      console.error("Unit of measure operation failed:", error);
      const fallback = id ? t`Unit of measure update failed` : t`Unit of measure creation failed`;
      message.error(error instanceof Error ? error.message : fallback);
      throw error;
    }
  },
);

export const deleteUnitOfMeasureAtom = atom(null, async (get, set, id: string) => {
  try {
    const success = await DeleteUnitOfMeasure(id);
    if (success) {
      const units = reject(get(unitsOfMeasureAtom), (u) => isEqual(u.id, id));
      set(unitsOfMeasureAtom, units);
      message.success(t`Unit of measure deleted`);
    } else {
      message.error(t`Unit of measure deletion failed`);
    }
    return success;
  } catch (error) {
    console.error("Failed to delete unit of measure:", error);
    message.error(error instanceof Error ? error.message : t`Unit of measure deletion failed`);
    return false;
  }
});
