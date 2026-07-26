import { atom } from "jotai";
import { atomWithStorage } from "jotai/utils";
import { message } from "src/utils/message";
import { nanoid } from "nanoid";
import { t } from "@lingui/core/macro";
import {
  GetOrganizations,
  GetOrganization,
  CreateOrganization,
  UpdateOrganization,
  GetOrganizationLogoDataUri,
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

// Bumped to force organizationAtom's getter to refetch without changing which
// organization is selected (e.g. after a logo upload/removal, or after an
// update). Setting organizationIdAtom to null and back to the same value
// doesn't reliably do this: if both set() calls land in the same render
// batch, subscribers only ever observe the final (unchanged) value and never
// re-run the getter. A monotonically increasing counter has no such
// coincidental-equality problem.
const organizationRefreshTokenAtom = atom(0);

export const organizationAtom = atom(
  async (get) => {
    const organizationId = get(organizationIdAtom);
    get(organizationRefreshTokenAtom);
    if (!organizationId) return null;

    try {
      const [organization, logo] = await Promise.all([
        GetOrganization(organizationId),
        GetOrganizationLogoDataUri(organizationId),
      ]);
      organization.logo = logo;
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
      // logo travels through the dedicated /logo endpoints, never through
      // this create/update JSON — drop it here so a stray value from a form
      // isn't silently sent (and silently ignored by the server).
      const { logo: _logo, ...processedValues } = newValues;

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
        set(organizationRefreshTokenAtom, (v) => v + 1);
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

// Forces organizationAtom to refetch the currently selected organization
// (including its logo) without going through a create/update. Used after a
// logo upload/removal, which happens through the dedicated /logo endpoints
// rather than the organizationAtom setter above.
export const reloadOrganizationAtom = atom(null, (_get, set) => {
  set(organizationRefreshTokenAtom, (v) => v + 1);
});

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
