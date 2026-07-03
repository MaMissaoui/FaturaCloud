import { atom } from "jotai";
import { message } from "antd";
import { nanoid } from "nanoid";
import { t } from "@lingui/core/macro";
import orderBy from "lodash/orderBy";
import keyBy from "lodash/keyBy";
import map from "lodash/map";
import reject from "lodash/reject";
import isEqual from "lodash/isEqual";
import {
  GetTaxRates,
  GetTaxRate,
  CreateTaxRate,
  UpdateTaxRate,
  DeleteTaxRate,
} from "src/api";

import { organizationIdAtom } from "./organization";

// Tax rates
export const taxRatesAtom = atom<any[]>([]);
export const setTaxRatesAtom = atom(null, async (get, set) => {
  const organizationId = get(organizationIdAtom);
  try {
    const response = await GetTaxRates(organizationId!);
    set(taxRatesAtom, response);
  } catch (error) {
    console.error("Failed to fetch tax rates:", error);
    message.error(t`Failed to fetch tax rates`);
    set(taxRatesAtom, []);
  }
});

// Tax rate
export const taxRateIdAtom = atom<string | null>(null);
export const taxRateAtom = atom(
  async (get) => {
    const taxRateId = get(taxRateIdAtom);
    if (!taxRateId) return null;

    try {
      const taxRate = await GetTaxRate(taxRateId);
      return taxRate;
    } catch (error) {
      console.error("Failed to fetch tax rate:", error);
      return null;
    }
  },
  async (get, set, newValues: any) => {
    const taxRateId = get(taxRateIdAtom);

    try {
      if (!taxRateId) {
        // Insert
        const taxRateData = {
          ...newValues,
          id: nanoid(),
          organizationId: get(organizationIdAtom),
          // Convert percentage string to number
          percentage: parseFloat(newValues.percentage),
          // Convert boolean to integer for isDefault
          isDefault:
            typeof newValues.isDefault === "boolean"
              ? newValues.isDefault
                ? 1
                : 0
              : newValues.isDefault,
        };

        const createdTaxRate = await CreateTaxRate(taxRateData);
        set(taxRateIdAtom, createdTaxRate.id);
        message.success(t`Tax rate created`);

        // Update the tax rates list
        const taxRates: any = get(taxRatesAtom);
        set(taxRatesAtom, orderBy([...taxRates, createdTaxRate], "name", "asc"));
      } else {
        // Update
        const updateData = {
          ...newValues,
          // Convert percentage string to number if present
          percentage: newValues.percentage ? parseFloat(newValues.percentage) : undefined,
          // Convert boolean to integer for isDefault
          isDefault:
            typeof newValues.isDefault === "boolean"
              ? newValues.isDefault
                ? 1
                : 0
              : newValues.isDefault,
        };

        const updatedTaxRate = await UpdateTaxRate(taxRateId, updateData);
        message.success(t`Tax rate updated successfully`);

        // Update the tax rates list
        const taxRates: any = get(taxRatesAtom);
        const mergedTaxRates: any = keyBy([...taxRates, updatedTaxRate], "id");
        set(taxRatesAtom, orderBy(map(mergedTaxRates), "name", "asc"));
      }
    } catch (error) {
      console.error("Tax rate operation failed:", error);
      if (!taxRateId) {
        message.error(t`Tax rate creation failed`);
      } else {
        message.error(t`Tax rate update failed`);
      }
    }
  },
);

export const deleteTaxRateAtom = atom(null, async (get, set, taxRateId: string) => {
  try {
    const success = await DeleteTaxRate(taxRateId);
    if (success) {
      const taxRates: any = reject(get(taxRatesAtom), (obj: any) => isEqual(obj.id, taxRateId));
      set(taxRatesAtom, taxRates);
      message.success(t`Tax rate deleted`);
    } else {
      message.error(t`Tax rate deletion failed`);
    }
  } catch (error) {
    console.error("Failed to delete tax rate:", error);
    message.error(error instanceof Error ? error.message : t`Tax rate deletion failed`);
  }
});
