// API (storage) shape. Note `emails` is a JSON-encoded string on the wire
// (e.g. '["a@b.com"]'); the client atom parses it to a string[] for the form.
export interface Client {
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
  createdAt?: string | null;
  // EN 16931 (XRechnung) buyer fields. Also the single source of truth for
  // address display everywhere (forms, lists, PDFs) — there is no separate
  // free-text address field. Clients previously had no country at all.
  street?: string | null;
  house_number?: string | null;
  postal_code?: string | null;
  city?: string | null;
  country_code?: string | null;
  tax_number?: string | null;
  // Convenience default (e.g. a Leitweg-ID) copied into Invoice.buyerReference
  // at invoice-creation time; not re-derived on every read.
  default_buyer_reference?: string | null;
  // Cash Book search fields. identity_number is a personal ID/CIN card
  // number, deliberately separate from vatin/tax_number (tax IDs, not
  // identity documents). iban is search/reference only.
  identity_number?: string | null;
  iban?: string | null;
  // Cash Book counter fields (migration 0087): a single free-text address
  // for the quick "New customer" modal, two extra phone numbers, and a
  // guarantor (French: garant).
  phone2?: string | null;
  phone3?: string | null;
  guarantor?: string | null;
  address?: string | null;
}
