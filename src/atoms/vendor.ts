import { atom } from "jotai";
import { message } from "src/utils/message";
import { nanoid } from "nanoid";
import { t } from "@lingui/core/macro";
import isEqual from "lodash/isEqual";
import orderBy from "lodash/orderBy";
import keyBy from "lodash/keyBy";
import map from "lodash/map";
import reject from "lodash/reject";
import {
  GetVendors,
  GetVendor,
  CreateVendor,
  UpdateVendor,
  DeleteVendor,
} from "src/api";

import { organizationIdAtom } from "./organization";

// Vendors
export const vendorsAtom = atom<any[]>([]);
vendorsAtom.debugLabel = "vendorsAtom";

export const setVendorsAtom = atom(null, async (get, set) => {
  const organizationId = get(organizationIdAtom);
  try {
    const response = await GetVendors(organizationId!);
    // Keep emails as JSON string in the vendors list since the table expects it
    set(vendorsAtom, response);
  } catch (error) {
    console.error("Failed to fetch vendors:", error);
    message.error(t`Failed to fetch vendors`);
    set(vendorsAtom, []);
  }
});
setVendorsAtom.debugLabel = "setVendorsAtom";

// Vendor
export const vendorIdAtom = atom<string | null>(null);
vendorIdAtom.debugLabel = "vendorIdAtom";

export const vendorAtom = atom(
  async (get) => {
    const vendorId = get(vendorIdAtom);
    if (!vendorId) return null;

    try {
      const vendor = await GetVendor(vendorId);
      if (!vendor) return null;
      // Parse emails from JSON string to array for the form
      return {
        ...vendor,
        emails: vendor?.emails ? JSON.parse(vendor.emails) : [],
      };
    } catch (error) {
      console.error("Failed to fetch vendor:", error);
      message.error(t`Failed to fetch vendor`);
      return null;
    }
  },
  async (get, set, newValues: any) => {
    const vendorId = get(vendorIdAtom);

    try {
      // Convert emails array to JSON string if it's an array
      const processedValues = {
        ...newValues,
        emails: Array.isArray(newValues.emails)
          ? JSON.stringify(newValues.emails)
          : newValues.emails,
      };

      if (!vendorId) {
        // Insert
        processedValues.id = nanoid();
        processedValues.organizationId = get(organizationIdAtom);
        const createdVendor = await CreateVendor(processedValues);
        message.success(t`Vendor created`);

        // Update the vendors list
        const vendors: any = get(vendorsAtom);
        set(vendorsAtom, orderBy([...vendors, createdVendor], "name", "asc"));
      } else {
        // Update
        const updatedVendor = await UpdateVendor(vendorId, processedValues);
        message.success(t`Vendor updated successfully`);

        // Update the vendors list
        const vendors: any = get(vendorsAtom);
        const mergedVendors: any = keyBy([...vendors, updatedVendor], "id");
        set(vendorsAtom, orderBy(map(mergedVendors), "name", "asc"));
      }
    } catch (error) {
      console.error("Vendor operation failed:", error);
      if (!vendorId) {
        message.error(t`Vendor creation failed`);
      } else {
        message.error(t`Vendor update failed`);
      }
    }
  },
);

// Delete vendor
export const deleteVendorAtom = atom(null, async (get, set, vendorId: string) => {
  try {
    const success = await DeleteVendor(vendorId);

    if (success) {
      // Remove vendor from the list
      const vendors: any = reject(get(vendorsAtom), (obj: any) => isEqual(obj.id, vendorId));
      set(vendorsAtom, vendors);
      message.success(t`Vendor deleted`);
    } else {
      message.error(t`Vendor deletion failed`);
    }
    return success;
  } catch (error) {
    // The server rejects deletion of a vendor that still has purchasing
    // documents with a 409 whose message is meant for the user.
    console.error("Failed to delete vendor:", error);
    message.error(error instanceof Error ? error.message : t`Vendor deletion failed`);
    return false;
  }
});
