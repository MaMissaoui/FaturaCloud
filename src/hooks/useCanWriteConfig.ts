import { useAtomValue } from "jotai";
import { CONFIG_WRITE_ROLES, myOrgRoleSyncAtom, type ConfigArea } from "src/atoms/organization";

// useCanWriteConfig reports whether the current role may change an area of
// organization-wide configuration (audit F170 — see CONFIG_WRITE_ROLES).
export function useCanWriteConfig(area: ConfigArea): boolean {
  return CONFIG_WRITE_ROLES[area].includes(useAtomValue(myOrgRoleSyncAtom));
}
