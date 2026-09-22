import { useEffect } from "react";
import { useAtomValue, useSetAtom } from "jotai";
import { useNavigate } from "react-router";
import first from "lodash/first";

import Loading from "src/components/loading";

import {
  organizationsAtom,
  organizationIdAtom,
  organizationsLoadedAtom,
  myOrgRoleAtom,
} from "src/atoms/organization";
import { roleHomePath } from "src/layouts/role-menu";

const Index = () => {
  const navigate = useNavigate();

  // Atoms
  const organizations = useAtomValue(organizationsAtom);
  const organizationId = useAtomValue(organizationIdAtom);
  const organizationsLoaded = useAtomValue(organizationsLoadedAtom);
  const orgRole = useAtomValue(myOrgRoleAtom);
  const setOrganizationId = useSetAtom(organizationIdAtom);

  // Handle organization-based redirects
  useEffect(() => {
    if (!organizationsLoaded) {
      return;
    }

    // If organizationId exists and is valid, redirect to the role's home
    // section (the focused views — cashbook, purchasing, accounting — each
    // land somewhere other than the invoice list).
    if (organizationId && organizations.some((org) => org.id === organizationId)) {
      navigate(roleHomePath(orgRole), { replace: true });
      return;
    }

    // If no organizationId and no organizations, redirect to create new
    if (!organizationId && organizations.length === 0) {
      navigate("/organizations/new", { replace: true });
      return;
    }

    // If organizationId exists but is not in organizations, clear it
    if (organizationId && !organizations.some((org) => org.id === organizationId)) {
      setOrganizationId(null);
      return;
    }

    // If organizations exist but no organizationId, set the first one
    if (organizations.length > 0 && !organizationId) {
      const firstOrganization = first(organizations);
      if (firstOrganization) {
        setOrganizationId(firstOrganization.id);
      }
    }
  }, [organizationId, organizations, navigate, setOrganizationId, organizationsLoaded, orgRole]);

  // Show loading state while organizations are being loaded
  if (!organizationsLoaded) {
    return <Loading />;
  }

  // This component only handles redirects, no UI after loading
  return null;
};

export default Index;
