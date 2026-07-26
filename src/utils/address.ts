// Shared formatter for the structured address fields (street, house_number,
// postal_code, city, country_code) that clients, vendors, and organizations
// all carry — the single source of truth for address display since none of
// them has a separate free-text address field anymore.

interface StructuredAddress {
  street?: string | null;
  house_number?: string | null;
  postal_code?: string | null;
  city?: string | null;
  country_code?: string | null;
}

function addressLines(entity: StructuredAddress): string[] {
  const streetLine = [entity.street, entity.house_number].filter(Boolean).join(" ");
  const cityLine = [entity.postal_code, entity.city].filter(Boolean).join(" ");
  return [streetLine, cityLine, entity.country_code].filter((line): line is string => !!line);
}

// Multi-line, for PDFs (react-pdf's Text renders literal "\n" as a line break).
export function formatAddress(entity: StructuredAddress): string {
  return addressLines(entity).join("\n");
}

// Single-line, for table cells and search strings.
export function formatAddressOneLine(entity: StructuredAddress): string {
  return addressLines(entity).join(", ");
}
