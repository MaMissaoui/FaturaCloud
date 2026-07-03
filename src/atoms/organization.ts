import { atom } from "jotai";
import { atomWithStorage } from "jotai/utils";
import { message } from "antd";
import { nanoid } from "nanoid";
import { t } from "@lingui/core/macro";
import {
  GetOrganizations,
  GetOrganization,
  CreateOrganization,
  UpdateOrganization,
} from "src/api";

import { generateInvoiceNumber } from "src/utils/invoice";

// Organizations
export const organizationsAtom = atom<any[]>([]);
export const organizationsLoadedAtom = atom<boolean>(false);

export const setOrganizationsAtom = atom(null, async (_get, set) => {
  try {
    const response = await GetOrganizations();
    set(organizationsAtom, response);
    set(organizationsLoadedAtom, true);
  } catch (error) {
    console.error("Failed to fetch organizations:", error);
    message.error(t`Failed to fetch organizations`);
    set(organizationsAtom, []);
    set(organizationsLoadedAtom, true);
  }
});

// Organization
export const organizationIdAtom = atomWithStorage<string | null>(
  "organizationId",
  null,
  undefined,
  {
    getOnInit: true,
  },
);
organizationIdAtom.debugLabel = "organizationIdAtom";

export const organizationAtom = atom(
  async (get) => {
    const organizationId = get(organizationIdAtom);
    if (!organizationId) return null;

    try {
      const organization = await GetOrganization(organizationId);

      // Go's encoding/json marshals []byte as base64 — decode to the data URL string.
      if (organization?.logo && typeof organization.logo === "string") {
        try {
          organization.logo = atob(organization.logo);
        } catch {
          // already a plain string or invalid base64 — leave as-is
        }
      }

      return organization;
    } catch (error) {
      console.error("Failed to fetch organization:", error);
      message.error(t`Failed to fetch organization`);
      return null;
    }
  },
  async (get, set, newValues: any) => {
    const organizationId = get(organizationIdAtom);

    try {
      // Go's encoding/json unmarshals []byte from base64 — encode the data URL string.
      let processedValues = { ...newValues };
      if (processedValues.logo && typeof processedValues.logo === "string") {
        try {
          processedValues.logo = btoa(processedValues.logo);
        } catch {
          // Non-ASCII in data URL (unlikely) — send as-is and let Go handle it
        }
      }

      if (!organizationId) {
        // Strip undefined values so they don't override defaults below.
        const definedValues = Object.fromEntries(
          Object.entries(processedValues).filter(([, v]) => v !== undefined),
        );
        // Insert - provide defaults for fields not set by user
        const organizationData = {
          currency: "EUR",
          minimum_fraction_digits: 2,
          due_days: 7,
          overdueCharge: 0,
          invoiceNumberFormat: "#{number}",
          invoiceNumberCounter: 0,
          ...definedValues,
          id: nanoid(),
        };

        const createdOrganization = await CreateOrganization(organizationData);
        set(setOrganizationsAtom);
        set(organizationIdAtom, createdOrganization.id);
        message.success(t`Organization created`);
      } else {
        // Update
        await UpdateOrganization(organizationId, processedValues);
        message.success(t`Organization updated successfully`);
        set(setOrganizationsAtom);

        // Force refresh by temporarily clearing and resetting the organizationId
        // This will trigger the organizationAtom getter to refetch
        set(organizationIdAtom, null);
        set(organizationIdAtom, organizationId);
      }
    } catch (error) {
      console.error("Organization operation failed:", error);
      if (!organizationId) {
        message.error(t`Organization creation failed`);
      } else {
        message.error(t`Organization update failed`);
      }
    }
  },
);
organizationAtom.debugLabel = "organizationAtom";

// Get next invoice number
export const nextInvoiceNumberAtom = atom(async (get) => {
  const organization = await get(organizationAtom);
  if (!organization) return null;

  const format = organization.invoiceNumberFormat;
  if (!format) {
    return null;
  }

  const counter = (organization.invoiceNumberCounter || 0) + 1;
  return generateInvoiceNumber(format, counter);
});
