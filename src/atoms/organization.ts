import { atom } from "jotai";
import { atomWithStorage } from "jotai/utils";
import type { Organization } from "src/types/models";
import { message } from "src/utils/message";
import { nanoid } from "nanoid";
import { t } from "@lingui/core/macro";
import {
  GetOrganizations,
  GetOrganization,
  CreateOrganization,
  UpdateOrganization,
  GetOrganizationLogoDataUri,
  GetMyOrganizationRole,
} from "src/api";

import { generateInvoiceNumber } from "src/utils/invoice";

// Organizations
export const organizationsAtom = atom<Organization[]>([]);
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
  async (get, set, newValues: Partial<Organization>) => {
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

// myOrgRoleAtom fetches the current user's role in the currently selected
// organization once — isOrgAdminAtom and isOrgAdminOrAccountingAtom below
// both derive from it rather than each hitting GetMyOrganizationRole
// separately. Reuses organizationRefreshTokenAtom's bump so it refetches
// whenever the organization itself does (e.g. after a membership change
// elsewhere).
export const myOrgRoleAtom = atom(async (get) => {
  const organizationId = get(organizationIdAtom);
  get(organizationRefreshTokenAtom);
  if (!organizationId) return "";

  try {
    const { role } = await GetMyOrganizationRole(organizationId);
    return role;
  } catch (error) {
    console.error("Failed to fetch organization role:", error);
    return "";
  }
});
myOrgRoleAtom.debugLabel = "myOrgRoleAtom";

// isOrgAdminAtom reports whether the current user is an admin member of the
// currently selected organization — the per-org counterpart to
// isPlatformAdminAtom, gating org-scoped admin actions (delete/reset
// organization, membership management) that a platform admin isn't
// automatically entitled to on every organization.
export const isOrgAdminAtom = atom(async (get) => (await get(myOrgRoleAtom)) === "admin");
isOrgAdminAtom.debugLabel = "isOrgAdminAtom";

// isOrgAdminOrAccountingAtom gates the two actions the org role redesign
// folded "accounting" into alongside admin: closing a fiscal year and GL
// export (see api/middleware.go's orgRoleAdmin) — previously admin-only.
export const isOrgAdminOrAccountingAtom = atom(async (get) => {
  const role = await get(myOrgRoleAtom);
  return role === "admin" || role === "accounting";
});
isOrgAdminOrAccountingAtom.debugLabel = "isOrgAdminOrAccountingAtom";

// isCashbookAtom reports whether the current user's role in the selected
// organization is the narrow "cashbook" (counter/till) role. Used purely as
// a UI restriction: the sidebar shows only Cash Book and Clients, and
// BaseLayout bounces any other route back to /cash-book. This is deliberate
// frontend-only gating — reads stay membership-level server-side for every
// role (api/CLAUDE.md), so a cashbook user can still read an invoice they
// navigate to directly; hiding it just keeps the counter workflow focused.
export const isCashbookAtom = atom(async (get) => (await get(myOrgRoleAtom)) === "cashbook");
isCashbookAtom.debugLabel = "isCashbookAtom";

// isGeneralRoleAtom reports whether the current user's role in the selected
// organization is the "general" role. Used to hide and redirect away from the
// sections a general user no longer has access to (Accounting, Imports, Bill
// of Materials, Production Orders) — the same UI-only enforced-view pattern
// as isCashbookAtom, not a server-side authorization boundary. BaseLayout now
// drives that from myOrgRoleAtom's raw value (the focused per-role menu), but
// this stays as the named predicate for any single-role check.
export const isGeneralRoleAtom = atom(async (get) => (await get(myOrgRoleAtom)) === "general");
isGeneralRoleAtom.debugLabel = "isGeneralRoleAtom";

// roleHomePath is the route a role is sent to when it lands on "/" or is
// bounced out of a section outside its scope (src/layouts/base.tsx,
// src/routes/index.tsx) — the focused view's "home". admin, power_user and
// general all default to the invoice list.
export const roleHomePath = (role: string): string => {
  switch (role) {
    case "cashbook":
      return "/cash-book";
    case "purchasing":
      return "/purchase-orders";
    case "accounting":
      return "/accounting/journal-entries";
    default:
      return "/invoices";
  }
};

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
