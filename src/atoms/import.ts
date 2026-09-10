import { atom } from "jotai";
import type { Import } from "src/types/models";
import { message } from "src/utils/message";
import { nanoid } from "nanoid";
import { t } from "@lingui/core/macro";
import isEqual from "lodash/isEqual";
import orderBy from "lodash/orderBy";
import keyBy from "lodash/keyBy";
import map from "lodash/map";
import reject from "lodash/reject";
import { GetImports, GetImport, CreateImport, UpdateImport, DeleteImport } from "src/api";

import { organizationIdAtom } from "./organization";

// Imports
export const importsAtom = atom<Import[]>([]);
importsAtom.debugLabel = "importsAtom";

export const setImportsAtom = atom(null, async (get, set) => {
  const organizationId = get(organizationIdAtom);
  try {
    const response = await GetImports(organizationId!);
    set(importsAtom, response);
  } catch (error) {
    console.error("Failed to fetch imports:", error);
    message.error(t`Failed to fetch imports`);
    set(importsAtom, []);
  }
});
setImportsAtom.debugLabel = "setImportsAtom";

// Import
export const importIdAtom = atom<string | null>(null);
importIdAtom.debugLabel = "importIdAtom";

export const importAtom = atom(
  async (get) => {
    const importId = get(importIdAtom);
    if (!importId) return null;

    try {
      return await GetImport(importId);
    } catch (error) {
      console.error("Failed to fetch import:", error);
      message.error(t`Failed to fetch import`);
      return null;
    }
  },
  async (get, set, newValues: Partial<Import>) => {
    const importId = get(importIdAtom);

    try {
      if (!importId) {
        const processedValues = {
          ...newValues,
          id: nanoid(),
          organizationId: get(organizationIdAtom)!,
        };
        const created = await CreateImport(processedValues);
        message.success(t`Import created`);

        const imports = get(importsAtom);
        set(importsAtom, orderBy([...imports, created], "date", "desc"));
      } else {
        const updated = await UpdateImport(importId, newValues);
        message.success(t`Import updated successfully`);

        const imports = get(importsAtom);
        const merged = keyBy([...imports, updated], "id");
        set(importsAtom, orderBy(map(merged), "date", "desc"));
      }
    } catch (error) {
      console.error("Import operation failed:", error);
      if (!importId) {
        message.error(t`Import creation failed`);
      } else {
        message.error(t`Import update failed`);
      }
      throw error;
    }
  },
);

// Delete import
export const deleteImportAtom = atom(null, async (get, set, importId: string) => {
  try {
    const success = await DeleteImport(importId);

    if (success) {
      const imports = reject(get(importsAtom), (obj) => isEqual(obj.id, importId));
      set(importsAtom, imports);
      message.success(t`Import deleted`);
    } else {
      message.error(t`Import deletion failed`);
    }
    return success;
  } catch (error) {
    // The server rejects deletion of an import still linked to a purchase
    // order with a 409 whose message is meant for the user.
    console.error("Failed to delete import:", error);
    message.error(error instanceof Error ? error.message : t`Import deletion failed`);
    return false;
  }
});
