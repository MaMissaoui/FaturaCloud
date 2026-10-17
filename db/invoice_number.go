package db

import (
	"fmt"
	"strings"
	"time"
)

// Invoice numbers are assigned server-side, inside the transaction that
// creates the invoice, and are never reused within an organization (audit
// F169). Before, CreateInvoice stored whatever number the browser sent — the
// form prefilled the next number when it opened — so two people creating an
// invoice at once got the same number, and nothing stopped a typed one from
// repeating an existing invoice's.

// maxInvoiceNumberSkips bounds the search for a free number: a format without
// {number} produces the same number every time, and that must end in an
// error, not a loop.
const maxInvoiceNumberSkips = 1000

// invoiceNumberTaken reports whether another invoice of the organization
// already carries number. Migrated opening loans are left out: their paper
// register references are unique among themselves (db/loan_import.go), not
// against invoice numbers.
func invoiceNumberTaken(q sqlGetExecer, organizationID, number, exceptInvoiceID string) (bool, error) {
	var n int
	if err := q.Get(&n, `
		SELECT COUNT(*) FROM invoices
		WHERE organizationId = ? AND number = ? AND id <> ? AND COALESCE(origin, '') <> ?`,
		organizationID, number, exceptInvoiceID, OpeningOrigin,
	); err != nil {
		return false, fmt.Errorf("invoice_number_taken: %w", err)
	}
	return n > 0, nil
}

// refuseTakenInvoiceNumber is the 409 for a number another invoice already
// has. Clearing the field gets the next free one.
func refuseTakenInvoiceNumber(number string) error {
	return newValidationError(
		"invoice number %q is already used by another invoice — clear the number to get the next free one",
		number,
	)
}

// nextInvoiceNumberTx derives the next free invoice number from the
// organization's invoice_number_format and invoice_number_counter, inside the
// caller's transaction, skipping any number an invoice already has (one typed
// by hand, or reissued after the counter was set back in Settings). It
// returns the number and the counter it used; the caller stores that counter
// in the same transaction.
func nextInvoiceNumberTx(tx sqlGetExecer, organizationID string, date int64, clientCode string) (string, int64, error) {
	var counters struct {
		Format  *string `db:"invoice_number_format"`
		Counter *int64  `db:"invoice_number_counter"`
	}
	if err := tx.Get(&counters, `SELECT invoice_number_format, invoice_number_counter FROM organizations WHERE id = ?`, organizationID); err != nil {
		return "", 0, fmt.Errorf("next_invoice_number read_counter: %w", err)
	}
	// An organization stored without a format (created through the API
	// before CreateOrganization defaulted it) still gets numbered invoices.
	format := DefaultInvoiceNumberFormat
	if counters.Format != nil && strings.TrimSpace(*counters.Format) != "" {
		format = *counters.Format
	}
	var counter int64
	if counters.Counter != nil {
		counter = *counters.Counter
	}
	loc, err := organizationLocation(tx, organizationID)
	if err != nil {
		return "", 0, err
	}
	when := time.UnixMilli(date).In(loc)
	for range maxInvoiceNumberSkips {
		counter++
		number := generateDocumentNumber(format, counter, when, clientCode)
		taken, err := invoiceNumberTaken(tx, organizationID, number, "")
		if err != nil {
			return "", 0, err
		}
		if !taken {
			return number, counter, nil
		}
	}
	return "", 0, newValidationError(
		"no free invoice number after %d tries — check that the invoice number format in Settings contains {number}",
		maxInvoiceNumberSkips,
	)
}
