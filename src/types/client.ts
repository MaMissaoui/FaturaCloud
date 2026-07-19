// API (storage) shape. Note `emails` is a JSON-encoded string on the wire
// (e.g. '["a@b.com"]'); the client atom parses it to a string[] for the form.
export interface Client {
  id: string;
  organizationId: string;
  name: string | null;
  code?: string | null;
  address?: string | null;
  emails: string | null;
  phone?: string | null;
  website?: string | null;
  registration_number?: string | null;
  vatin?: string | null;
  createdAt?: string | null;
}
