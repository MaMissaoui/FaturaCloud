// Shapes shared by the document detail pages' forms.

// A line as Form.useWatch("lineItems") sees it mid-edit: a field the user
// hasn't filled yet is missing, and the subtotal treats it as 0.
export type LineAmounts = {
  quantity?: number | string | null;
  unitPrice?: number | string | null;
};

// The detail pages check a loaded document for a `then` before reading it,
// so a value that is still a promise is never shown as the document.
export type Thenable = { then?: unknown };
