import { useEffect, useMemo, useRef, useState } from "react";
import type { Account } from "src/types/models";
import { Alert, App, Button, Form } from "antd";
import { useAtom, useSetAtom } from "jotai";
import { ApartmentOutlined } from "@ant-design/icons";
import { Trans } from "@lingui/react/macro";
import { t } from "@lingui/core/macro";
import { useLingui } from "@lingui/react";
import { nanoid } from "nanoid";
import filter from "lodash/filter";
import some from "lodash/some";
import get from "lodash/get";
import includes from "lodash/includes";
import toString from "lodash/toString";

import {
  GetOrganizations,
  GetOrganization,
  CreateOrganization,
  UpdateOrganization,
  DeleteOrganization,
  DeleteOrganizationLogo,
  GetOrganizationUsageCount,
  ResetOrganizationData,
  GetAccounts,
  GetMyOrganizationRoles,
  GetOrganizationMembers,
  AddOrganizationMember,
  UpdateOrganizationMemberRole,
  RemoveOrganizationMember,
  type OrganizationUsageCount,
  type OrganizationMember,
  type OrganizationRole,
} from "src/api";
import {
  organizationIdAtom,
  reloadOrganizationAtom,
  setOrganizationsAtom,
} from "src/atoms/organization";
import PageHeader from "src/components/page-header";
import OrganizationsTable from "src/components/organizations/organizations-table";
import OrganizationEditDrawer from "src/components/organizations/organization-edit-drawer";
import { normalizeDocumentLayout } from "src/types/document-layout";
import { centsToUnits, unitsToCents } from "src/utils/currency";

// The GL-default account Selects the edit drawer renders. A cleared one has
// to be sent as "" rather than dropped, or the server cannot tell "cleared"
// from "not edited" — see the note in handleSubmit (audit 2026-09-14 F95).
const ACCOUNT_DEFAULT_FIELDS = [
  "defaultArAccountId",
  "defaultApAccountId",
  "defaultRevenueAccountId",
  "defaultExpenseAccountId",
  "defaultCashAccountId",
  "defaultInventoryAccountId",
  "defaultGRNIAccountId",
  "defaultCOGSAccountId",
  "defaultInventoryAdjustmentAccountId",
  "defaultImportCostsPayableAccountId",
  "datevClearingAccountId",
  "defaultStampDutyAccountId",
  "defaultCashRegisterAccountId",
] as const;

const clearedAccountDefaults = (values: Record<string, unknown>): Record<string, string> => {
  const cleared: Record<string, string> = {};
  for (const field of ACCOUNT_DEFAULT_FIELDS) {
    // `in` matters: a field the drawer never rendered (the stamp-duty
    // account while fiscal stamps are switched off) is absent rather than
    // undefined, and must stay absent so the server keeps what it holds.
    if (field in values && values[field] == null) cleared[field] = "";
  }
  return cleared;
};

export default function Organizations() {
  useLingui();
  const { message } = App.useApp();
  const [form] = Form.useForm();

  const [orgs, setOrgs] = useState<any[]>([]);
  const [loading, setLoading] = useState(true);
  // A failed list fetch used to leave the table silently empty while loading
  // cleared (the old try/finally had no catch, so the rejection was
  // unhandled). Tracked separately so the page can show a real error with a
  // Retry instead of an indistinguishable empty list.
  const [loadError, setLoadError] = useState(false);
  const [submitting, setSubmitting] = useState(false);
  const [search, setSearch] = useState("");
  const [drawerOpen, setDrawerOpen] = useState(false);
  const [editingId, setEditingId] = useState<string | null>(null);
  const [usageCounts, setUsageCounts] = useState<Record<string, OrganizationUsageCount>>({});
  // Delete/reset are now org-scoped admin actions (see api/middleware.go's
  // orgAdmin) — a platform admin isn't automatically an admin of every
  // organization, so each row's own admin status is fetched per organization
  // rather than a single global isAdmin flag applying to every row.
  const [myOrgAdminIds, setMyOrgAdminIds] = useState<Set<string>>(new Set());
  const [logoKey, setLogoKey] = useState(0);
  const [hasLogo, setHasLogo] = useState(true);
  const [logoBusy, setLogoBusy] = useState(false);
  const [resetMasterData, setResetMasterData] = useState(false);
  const [resetTransactionalData, setResetTransactionalData] = useState(false);
  const [resetting, setResetting] = useState(false);
  // Every secondary section starts collapsed — only Details needs to be
  // visible without scrolling; Logo/Banking/Address/E-invoicing/Formatting/
  // Accounting are edited far less often than the fields above them.
  const [activeSections, setActiveSections] = useState<string[]>([]);
  // Fetched per-editingId (not the shared accountsAtom, which is scoped to
  // the globally-selected organization) — this drawer can edit an org other
  // than the currently-selected one, same reasoning as the Logo card above.
  const [editingAccounts, setEditingAccounts] = useState<Account[]>([]);
  const [members, setMembers] = useState<OrganizationMember[]>([]);
  const [membersLoading, setMembersLoading] = useState(false);
  const [newMemberEmail, setNewMemberEmail] = useState("");
  const [newMemberRole, setNewMemberRole] = useState<OrganizationRole>("general");
  const [addingMember, setAddingMember] = useState(false);
  const [memberActionId, setMemberActionId] = useState<string | null>(null);

  const [organizationId, setOrganizationId] = useAtom(organizationIdAtom);
  const refreshGlobalOrgs = useSetAtom(setOrganizationsAtom);
  const reloadActiveOrganization = useSetAtom(reloadOrganizationAtom);

  // Invalidates an in-flight openEdit if the user opens a different record
  // (or "New organization") before its GetOrganization resolves, so a slow
  // response can't populate or pop open a drawer out of turn.
  const editRequestIdRef = useRef(0);

  const fetchOrgs = async () => {
    setLoading(true);
    setLoadError(false);
    try {
      const list = await GetOrganizations();
      setOrgs(list);
      // One request for every organization's role (issue #147), replacing
      // the old one-GetMyOrganizationRole-call-per-row N+1 pattern.
      const roles = await GetMyOrganizationRoles().catch(
        () => ({}) as Record<string, OrganizationRole>,
      );
      setMyOrgAdminIds(new Set(Object.keys(roles).filter((orgId) => roles[orgId] === "admin")));
    } catch (error) {
      console.error("Failed to load organizations:", error);
      setLoadError(true);
    } finally {
      setLoading(false);
    }
  };

  useEffect(() => {
    fetchOrgs();
  }, []);

  const filteredOrgs = search
    ? filter(orgs, (org) =>
        some(["name", "code", "email", "phone", "iban", "currency"], (field) =>
          includes(toString(get(org, field)).toLowerCase(), search.toLowerCase()),
        ),
      )
    : orgs;

  const openNew = () => {
    editRequestIdRef.current++;
    setEditingId(null);
    form.resetFields();
    form.setFieldsValue({ minimum_fraction_digits: 2, currency: "EUR", documentLayout: "default" });
    setActiveSections([]);
    setDrawerOpen(true);
  };

  const fetchMembers = async (id: string) => {
    setMembersLoading(true);
    try {
      setMembers(await GetOrganizationMembers(id));
    } catch (error) {
      console.error("Failed to fetch organization members:", error);
    } finally {
      setMembersLoading(false);
    }
  };

  const openEdit = async (id: string) => {
    const requestId = ++editRequestIdRef.current;
    form.resetFields();
    setHasLogo(true);
    setLogoKey((k) => k + 1);
    setResetMasterData(false);
    setResetTransactionalData(false);
    setEditingAccounts([]);
    setActiveSections([]);
    setMembers([]);
    setNewMemberEmail("");
    setNewMemberRole("general");
    // Fetch the record before opening the drawer: on failure a blank edit
    // form would be indistinguishable from a record with no data and a save
    // from it could overwrite real fields with empty ones (F119), so surface
    // the error and leave the drawer closed instead.
    try {
      const org = await GetOrganization(id);
      if (requestId !== editRequestIdRef.current) return;
      setEditingId(id);
      // Convert null date_format to undefined so the Select shows placeholder
      form.setFieldsValue({
        ...org,
        date_format: org.date_format ?? undefined,
        documentLayout: normalizeDocumentLayout(org.documentLayout),
        // defaultFiscalStampAmount is stored in cents like every other
        // money column; this form (unlike the invoice form's atom) has no
        // existing cents<->units conversion layer, so it's done here and
        // reversed in handleSubmit below.
        defaultFiscalStampAmount:
          org.defaultFiscalStampAmount != null
            ? centsToUnits(org.defaultFiscalStampAmount)
            : undefined,
      });
      setDrawerOpen(true);
      // Listing members is org-admin gated — only fetch if this actor is known
      // to administer this organization, to avoid a noisy 403 for everyone
      // else opening the drawer to view/edit other fields.
      if (myOrgAdminIds.has(id)) {
        fetchMembers(id);
      }
      try {
        setEditingAccounts(await GetAccounts(id));
      } catch {
        // Accounting selects just render empty if this fails.
      }
    } catch (error) {
      console.error("Failed to load organization:", error);
      message.error(error instanceof Error ? error.message : t`Failed to load organization`);
    }
  };

  const handleClose = () => {
    setDrawerOpen(false);
    setEditingId(null);
    form.resetFields();
    setResetMasterData(false);
    setResetTransactionalData(false);
    setEditingAccounts([]);
    setMembers([]);
  };

  const handleAddMember = async (id: string) => {
    if (!newMemberEmail.trim()) return;
    setAddingMember(true);
    try {
      await AddOrganizationMember(id, { email: newMemberEmail.trim(), role: newMemberRole });
      message.success(t`Member added`);
      setNewMemberEmail("");
      setNewMemberRole("general");
      await fetchMembers(id);
    } catch (error) {
      message.error(error instanceof Error ? error.message : t`Failed to add member`);
    } finally {
      setAddingMember(false);
    }
  };

  const handleMemberRoleChange = async (id: string, userId: string, role: OrganizationRole) => {
    setMemberActionId(userId);
    try {
      await UpdateOrganizationMemberRole(id, userId, role);
      await fetchMembers(id);
    } catch (error) {
      message.error(error instanceof Error ? error.message : t`Failed to update member role`);
    } finally {
      setMemberActionId(null);
    }
  };

  const handleRemoveMember = async (id: string, userId: string) => {
    setMemberActionId(userId);
    try {
      await RemoveOrganizationMember(id, userId);
      message.success(t`Member removed`);
      await fetchMembers(id);
    } catch (error) {
      message.error(error instanceof Error ? error.message : t`Failed to remove member`);
    } finally {
      setMemberActionId(null);
    }
  };

  const leafAccountOptions = useMemo(
    () =>
      editingAccounts
        .filter((a) => !a.isGroup)
        .map((a) => ({ value: a.id, label: `${a.code} · ${a.name}` })),
    [editingAccounts],
  );

  const refreshLogo = () => {
    setHasLogo(true);
    setLogoKey((k) => k + 1);
    if (editingId === organizationId) reloadActiveOrganization();
  };

  const handleLogoRemove = async () => {
    if (!editingId) return;
    setLogoBusy(true);
    try {
      await DeleteOrganizationLogo(editingId);
      setHasLogo(false);
      if (editingId === organizationId) reloadActiveOrganization();
    } catch (error) {
      message.error(error instanceof Error ? error.message : t`Logo removal failed`);
    } finally {
      setLogoBusy(false);
    }
  };

  const handleSubmit = async (values: any) => {
    setSubmitting(true);
    try {
      if (editingId) {
        await UpdateOrganization(editingId, {
          ...values,
          // antd's `Select allowClear` sets the form value to `undefined`
          // on clear, and JSON.stringify drops undefined keys entirely — so
          // a cleared GL default reached the server indistinguishable from
          // a field the user never touched, and nothing was cleared.
          //
          // The server's convention (db/organization.go's
          // UpdateOrganization, audit 2026-09-14 F95) is the one every other
          // nullable organization field already uses: omitted means keep,
          // "" means clear. Send "" for any account Select the user emptied.
          ...clearedAccountDefaults(values),
          defaultFiscalStampAmount:
            values.defaultFiscalStampAmount != null
              ? unitsToCents(values.defaultFiscalStampAmount)
              : undefined,
          // Checkbox valuePropName="checked" emits a boolean; the API
          // stores these as 0/1 like every other boolean column (isDefault,
          // isGroup, ...) — same conversion src/atoms/tax-rate.ts uses.
          fiscalStampEnabled:
            typeof values.fiscalStampEnabled === "boolean"
              ? values.fiscalStampEnabled
                ? 1
                : 0
              : values.fiscalStampEnabled,
          withholdingTaxEnabled:
            typeof values.withholdingTaxEnabled === "boolean"
              ? values.withholdingTaxEnabled
                ? 1
                : 0
              : values.withholdingTaxEnabled,
          amountInWordsEnabled:
            typeof values.amountInWordsEnabled === "boolean"
              ? values.amountInWordsEnabled
                ? 1
                : 0
              : values.amountInWordsEnabled,
        });
        // Logo upload/removal and delete already reload the active
        // organization when they touch the currently-selected org (see
        // below) — a plain field save through this form was the one path
        // that didn't, so header/theme-driving reads of organizationAtom
        // (e.g. app.tsx's brand-color ConfigProvider token) kept serving
        // stale data until something else happened to refetch it.
        if (editingId === organizationId) reloadActiveOrganization();
      } else {
        const newOrg = await CreateOrganization({
          ...values,
          id: nanoid(),
          currency: values.currency || "EUR",
          minimum_fraction_digits: values.minimum_fraction_digits ?? 2,
          due_days: 7,
          overdueCharge: 0,
          invoiceNumberFormat: "#{number}",
          invoiceNumberCounter: 0,
        });
        setOrganizationId(newOrg.id);
      }
      await fetchOrgs();
      refreshGlobalOrgs();
      handleClose();
    } catch (error) {
      // Keep the drawer open with the user's input intact (handleClose is
      // only reached on success above) and surface why the save failed —
      // this catch previously swallowed the error with no toast at all.
      console.error("Failed to save organization:", error);
      message.error(error instanceof Error ? error.message : t`Failed to save organization`);
    } finally {
      setSubmitting(false);
    }
  };

  const handleDelete = async (id: string) => {
    try {
      await DeleteOrganization(id);
      if (id === organizationId) {
        const remaining = orgs.filter((o) => o.id !== id);
        setOrganizationId(remaining.length > 0 ? remaining[0].id : null);
      }
      await fetchOrgs();
      refreshGlobalOrgs();
      message.success(t`Organization deleted`);
    } catch (error) {
      console.error("Failed to delete organization:", error);
      message.error(error instanceof Error ? error.message : t`Organization deletion failed`);
    }
  };

  const fetchUsageCount = async (id: string) => {
    if (usageCounts[id]) return;
    try {
      const counts = await GetOrganizationUsageCount(id);
      setUsageCounts((prev) => ({ ...prev, [id]: counts }));
    } catch {
      // Confirmation still works without the breakdown if this fails.
    }
  };

  const handleReset = async (id: string) => {
    setResetting(true);
    try {
      const deleted = await ResetOrganizationData(id, {
        resetMasterData,
        resetTransactionalData,
      });
      const total = Object.values(deleted).reduce((sum, n) => sum + n, 0);
      message.success(
        total > 0
          ? t`Deleted ${total} record(s)`
          : t`Nothing to delete — this organization was empty`,
      );
      // The counts just shown are now stale (this org has fewer, or no,
      // records); drop the cache entry so the next Popconfirm open refetches.
      setUsageCounts((prev) => {
        const next = { ...prev };
        delete next[id];
        return next;
      });
      setResetMasterData(false);
      setResetTransactionalData(false);
      // Invoice numbering and other profile-derived fields the drawer/settings
      // pages read (e.g. the invoice number preview) just changed under it.
      if (id === organizationId) reloadActiveOrganization();
    } catch (error) {
      console.error("Failed to reset organization data:", error);
      message.error(error instanceof Error ? error.message : t`Reset failed`);
    } finally {
      setResetting(false);
    }
  };

  const isEdit = !!editingId;

  const resetCounts = editingId ? usageCounts[editingId] : undefined;
  const resetBreakdown: [number, string][] = resetCounts
    ? (
        [
          ...(resetMasterData
            ? [
                [resetCounts.clients, t`client(s)`],
                [resetCounts.vendors, t`vendor(s)`],
                [resetCounts.products, t`product(s)`],
                [resetCounts.taxRates, t`tax rate(s)`],
              ]
            : []),
          ...(resetMasterData || resetTransactionalData
            ? [
                [resetCounts.invoices, t`invoice(s)`],
                [resetCounts.orders, t`order(s)`],
                [resetCounts.deliveries, t`delivery(ies)`],
                [resetCounts.purchaseOrders, t`purchase order(s)`],
                [resetCounts.inboundDeliveries, t`goods receipt(s)`],
                [resetCounts.incomingInvoices, t`incoming invoice(s)`],
                [resetCounts.stockMovements, t`stock movement(s)`],
              ]
            : []),
        ] as [number, string][]
      ).filter(([n]) => n > 0)
    : [];
  const resetSelected = resetMasterData || resetTransactionalData;

  // Members/danger-zone Cards are only ever shown for an organization this
  // actor administers — same condition as before, now computed once here
  // instead of inline at each of the two Cards' old call sites.
  const administersEditingOrg = isEdit && !!editingId && myOrgAdminIds.has(editingId);

  return (
    <>
      <PageHeader
        style={{ marginBottom: 12 }}
        icon={<ApartmentOutlined />}
        title={<Trans>Organizations</Trans>}
        search={{
          placeholder: t`Search`,
          value: search,
          onChange: setSearch,
          allowClear: true,
          onClear: () => setSearch(""),
        }}
        actions={
          <Button type="primary" onClick={openNew}>
            <Trans>New organization</Trans>
          </Button>
        }
      />

      {loadError && (
        <Alert
          style={{ marginBottom: 12 }}
          type="error"
          showIcon
          message={<Trans>Couldn't load organizations</Trans>}
          action={
            <Button size="small" onClick={fetchOrgs}>
              <Trans>Retry</Trans>
            </Button>
          }
        />
      )}

      <OrganizationsTable
        dataSource={filteredOrgs}
        loading={loading}
        myOrgAdminIds={myOrgAdminIds}
        usageCounts={usageCounts}
        onRowClick={openEdit}
        onFetchUsageCount={fetchUsageCount}
        onDelete={handleDelete}
      />

      <OrganizationEditDrawer
        open={drawerOpen}
        isEdit={isEdit}
        editingId={editingId}
        form={form}
        submitting={submitting}
        onClose={handleClose}
        onSubmit={handleSubmit}
        activeSections={activeSections}
        onActiveSectionsChange={setActiveSections}
        hasLogo={hasLogo}
        logoKey={logoKey}
        logoBusy={logoBusy}
        onLogoImgError={() => setHasLogo(false)}
        onLogoUploaded={refreshLogo}
        onLogoUploadError={() => message.error(t`Logo upload failed`)}
        onLogoRemove={handleLogoRemove}
        leafAccountOptions={leafAccountOptions}
        membersPanelProps={
          administersEditingOrg && editingId
            ? {
                members,
                membersLoading,
                newMemberEmail,
                newMemberRole,
                addingMember,
                memberActionId,
                onNewMemberEmailChange: setNewMemberEmail,
                onNewMemberRoleChange: setNewMemberRole,
                onAddMember: () => handleAddMember(editingId),
                onMemberRoleChange: (userId, role) =>
                  handleMemberRoleChange(editingId, userId, role),
                onRemoveMember: (userId) => handleRemoveMember(editingId, userId),
              }
            : null
        }
        dangerZoneProps={
          administersEditingOrg && editingId
            ? {
                resetMasterData,
                resetTransactionalData,
                resetting,
                resetBreakdown,
                resetSelected,
                onMasterDataChange: setResetMasterData,
                onTransactionalDataChange: setResetTransactionalData,
                onPopconfirmOpenChange: (open) => {
                  if (open) fetchUsageCount(editingId);
                },
                onConfirmReset: () => handleReset(editingId),
              }
            : null
        }
      />
    </>
  );
}
