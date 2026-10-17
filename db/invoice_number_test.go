package db

import (
	"errors"
	"strings"
	"testing"
)

func newNumberingInvoice(t *testing.T, d *Database, fx glPostingTestFixture, number string) (*Invoice, error) {
	t.Helper()
	return d.CreateInvoice(CreateInvoiceRequest{
		OrganizationID: fx.orgID, Number: number, ClientID: fx.clientID,
		Date: fx.date, Currency: "EUR",
		SubTotal: 1000, TaxTotal: 200, Total: 1200,
		LineItems: []CreateInvoiceLineItemRequest{
			{Quantity: 1, UnitPrice: 1000, TaxRate: &fx.taxRateID},
		},
	})
}

func invoiceCounter(t *testing.T, d *Database, orgID string) int64 {
	t.Helper()
	var counter int64
	if err := d.DB.Get(&counter, `SELECT invoice_number_counter FROM organizations WHERE id = ?`, orgID); err != nil {
		t.Fatalf("read counter: %v", err)
	}
	return counter
}

func isTakenNumberError(err error) bool {
	var verr *ValidationError
	return errors.As(err, &verr) && strings.Contains(err.Error(), "already used by another invoice")
}

// TestCreateInvoiceAssignsTheNextFreeNumber covers audit F169: an invoice
// sent without a number gets the next free one from the organization's
// format and counter, skipping a number an invoice already has.
func TestCreateInvoiceAssignsTheNextFreeNumber(t *testing.T) {
	t.Parallel()
	d := newTestDB(t)
	fx := newGLPostingTestFixture(t, d, "org-1")
	if _, err := d.DB.Exec(`UPDATE organizations SET invoice_number_format = 'INV-{number}', invoice_number_counter = 0 WHERE id = ?`, fx.orgID); err != nil {
		t.Fatalf("set format: %v", err)
	}

	first, err := newNumberingInvoice(t, d, fx, "")
	if err != nil || first.Number != "INV-1" {
		t.Fatalf("first = %+v, %v; want INV-1", first, err)
	}
	if c := invoiceCounter(t, d, fx.orgID); c != 1 {
		t.Fatalf("counter = %d; want 1", c)
	}

	// Someone types the next number by hand: it is kept, and the counter
	// advances as it always did.
	typed, err := newNumberingInvoice(t, d, fx, "  INV-3 ")
	if err != nil || typed.Number != "INV-3" {
		t.Fatalf("typed = %+v, %v; want INV-3, trimmed", typed, err)
	}
	if c := invoiceCounter(t, d, fx.orgID); c != 2 {
		t.Fatalf("counter = %d; want 2", c)
	}

	// The counter now points at INV-3, which is taken: the next assigned
	// number skips it.
	next, err := newNumberingInvoice(t, d, fx, "")
	if err != nil || next.Number != "INV-4" {
		t.Fatalf("next = %+v, %v; want INV-4 (INV-3 skipped)", next, err)
	}
	if c := invoiceCounter(t, d, fx.orgID); c != 4 {
		t.Fatalf("counter = %d; want 4", c)
	}
}

func TestCreateInvoiceRefusesATakenNumber(t *testing.T) {
	t.Parallel()
	d := newTestDB(t)
	fx := newGLPostingTestFixture(t, d, "org-1")
	if _, err := newNumberingInvoice(t, d, fx, "FAC-1"); err != nil {
		t.Fatalf("first: %v", err)
	}
	before := invoiceCounter(t, d, fx.orgID)
	if _, err := newNumberingInvoice(t, d, fx, "FAC-1"); !isTakenNumberError(err) {
		t.Fatalf("duplicate err = %v; want the taken-number 409", err)
	}
	if c := invoiceCounter(t, d, fx.orgID); c != before {
		t.Fatalf("counter moved to %d on a refused create (was %d)", c, before)
	}

	// Another organization may use the same number.
	other := newGLPostingTestFixture(t, d, "org-2")
	if _, err := newNumberingInvoice(t, d, other, "FAC-1"); err != nil {
		t.Fatalf("same number in another organization: %v", err)
	}
}

func TestUpdateInvoiceChecksOnlyAChangedNumber(t *testing.T) {
	t.Parallel()
	d := newTestDB(t)
	fx := newGLPostingTestFixture(t, d, "org-1")
	a, err := newNumberingInvoice(t, d, fx, "FAC-1")
	if err != nil {
		t.Fatalf("a: %v", err)
	}
	b, err := newNumberingInvoice(t, d, fx, "FAC-2")
	if err != nil {
		t.Fatalf("b: %v", err)
	}

	taken := "FAC-1"
	if _, err := d.UpdateInvoice(b.ID, UpdateInvoiceRequest{Number: &taken}); !isTakenNumberError(err) {
		t.Fatalf("renumber to a taken number: err = %v; want the 409", err)
	}
	free := "FAC-9"
	if got, err := d.UpdateInvoice(b.ID, UpdateInvoiceRequest{Number: &free}); err != nil || got.Number != "FAC-9" {
		t.Fatalf("renumber to a free number = %+v, %v", got, err)
	}
	_ = a
}

// The database itself refuses a second invoice with the same number in one
// organization (migration 0106), outside opening loans and blank numbers.
func TestInvoiceNumberUniqueIndex(t *testing.T) {
	t.Parallel()
	d := newTestDB(t)
	fx := newGLPostingTestFixture(t, d, "org-1")
	inv, err := newNumberingInvoice(t, d, fx, "FAC-1")
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	other, err := newNumberingInvoice(t, d, fx, "FAC-2")
	if err != nil {
		t.Fatalf("create other: %v", err)
	}
	if _, err := d.DB.Exec(`UPDATE invoices SET number = 'FAC-1' WHERE id = ?`, other.ID); err == nil || !strings.Contains(err.Error(), "UNIQUE") {
		t.Fatalf("raw duplicate err = %v; want a UNIQUE violation", err)
	}
	if _, err := d.DB.Exec(`UPDATE invoices SET number = 'FAC-1', origin = 'opening' WHERE id = ?`, other.ID); err != nil {
		t.Fatalf("an opening loan may share a number: %v", err)
	}
	_ = inv
}
