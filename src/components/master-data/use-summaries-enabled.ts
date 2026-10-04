import { useAtomValue } from "jotai";

import { myOrgRoleSyncAtom, organizationAtom } from "src/atoms/organization";
import { roleCanSeeSummaries, SUMMARY_SECTION_ROLES } from "src/layouts/role-menu";

// useSummariesEnabled decides whether a master-data screen shows its
// summaries (balances, recent activity, the summary panel): the organization
// has them switched on (organizations.masterDataSummaries, on unless set to
// false) and the user's role may see that section. When it returns false the
// screen sends no summary request at all — the server would refuse it anyway
// (409 when switched off, 403 for the role).
export function useSummariesEnabled(section: keyof typeof SUMMARY_SECTION_ROLES): boolean {
  const organization = useAtomValue(organizationAtom);
  const role = useAtomValue(myOrgRoleSyncAtom);
  return (
    !!organization &&
    organization.masterDataSummaries !== false &&
    roleCanSeeSummaries(role, section)
  );
}
