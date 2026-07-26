// API (storage) shape. Note `emails` is a JSON-encoded string on the wire
// (e.g. '["a@b.com"]'); the vendor atom parses it to a string[] for the form.
export interface Vendor {
  id: string;
  organizationId: string;
  name: string | null;
  code?: string | null;
  emails: string | null;
  phone?: string | null;
  website?: string | null;
  registration_number?: string | null;
  vatin?: string | null;
  defaultCurrency?: string | null;
  paymentTermsDays?: number | null;
  createdAt?: string | null;
  // Structured address, matching clients/organizations — the single source
  // of truth for display (purchase order PDFs, lists).
  street?: string | null;
  house_number?: string | null;
  postal_code?: string | null;
  city?: string | null;
  country_code?: string | null;
}
