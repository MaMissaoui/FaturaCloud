import { atom } from "jotai";
import type { Account } from "src/types/models";
import { message } from "src/utils/message";
import { nanoid } from "nanoid";
import { t } from "@lingui/core/macro";
import orderBy from "lodash/orderBy";
import keyBy from "lodash/keyBy";
import map from "lodash/map";
import reject from "lodash/reject";
import isEqual from "lodash/isEqual";
import { GetAccounts, CreateAccount, UpdateAccount, DeleteAccount } from "src/api";

import { organizationIdAtom } from "./organization";

// Accounts (chart of accounts)
export const accountsAtom = atom<Account[]>([]);
accountsAtom.debugLabel = "accountsAtom";

export const setAccountsAtom = atom(null, async (get, set) => {
  const organizationId = get(organizationIdAtom);
  try {
    const response = await GetAccounts(organizationId!);
    set(accountsAtom, response);
  } catch (error) {
    console.error("Failed to fetch accounts:", error);
    message.error(t`Failed to fetch accounts`);
    set(accountsAtom, []);
  }
});
setAccountsAtom.debugLabel = "setAccountsAtom";

// Account
export const accountIdAtom = atom<string | null>(null);
accountIdAtom.debugLabel = "accountIdAtom";

export const accountAtom = atom(
  (get) => {
    const accountId = get(accountIdAtom);
    if (!accountId) return null;
    return get(accountsAtom).find((a) => a.id === accountId) ?? null;
  },
  async (get, set, newValues: Partial<Account>) => {
    const accountId = get(accountIdAtom);

    try {
      if (!accountId) {
        const created = await CreateAccount({
          ...newValues,
          id: nanoid(),
          organizationId: get(organizationIdAtom)!,
        });
        message.success(t`Account created`);
        const accounts = get(accountsAtom);
        set(accountsAtom, orderBy([...accounts, created], "code", "asc"));
      } else {
        const updated = await UpdateAccount(accountId, newValues);
        message.success(t`Account updated`);
        const accounts = get(accountsAtom);
        const merged = keyBy([...accounts, updated], "id");
        set(accountsAtom, orderBy(map(merged), "code", "asc"));
      }
    } catch (error) {
      console.error("Account operation failed:", error);
      message.error(error instanceof Error ? error.message : t`Account save failed`);
      throw error;
    }
  },
);

// Delete account
export const deleteAccountAtom = atom(null, async (get, set, accountId: string) => {
  try {
    const success = await DeleteAccount(accountId);
    if (success) {
      set(
        accountsAtom,
        reject(get(accountsAtom), (a) => isEqual(a.id, accountId)),
      );
      message.success(t`Account deleted`);
    }
    return success;
  } catch (error) {
    // Refused with a 409 while the account has children or is still in use —
    // that message is meant for the user.
    console.error("Failed to delete account:", error);
    message.error(error instanceof Error ? error.message : t`Account deletion failed`);
    return false;
  }
});
