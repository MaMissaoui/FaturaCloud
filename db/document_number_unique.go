package db

import (
	"fmt"
	"strings"
	"time"

	"github.com/jmoiron/sqlx"
)

// documentNumberColumn names where each counter-numbered document type
// stores its number. Migration 0108 makes every one unique within its
// organization (blank numbers aside), the same rule invoices follow
// (db/invoice_number.go, audit F169).
type documentNumberColumn struct {
	table, column, label string
}

var documentNumberColumns = map[string]documentNumberColumn{
	"order":            {"orders", "orderNumber", "order"},
	"purchase_order":   {"purchase_orders", "orderNumber", "purchase order"},
	"delivery":         {"outbound_deliveries", "deliveryNumber", "delivery"},
	"inbound_delivery": {"inbound_deliveries", "deliveryNumber", "goods receipt"},
	"production_order": {"production_orders", "orderNumber", "production order"},
}

// maxDocumentNumberSkips bounds the search for a free number, like
// maxInvoiceNumberSkips: a format without {number} repeats itself.
const maxDocumentNumberSkips = 1000

// documentNumberTaken reports whether another document of documentType in the
// organization already carries number. Table and column names come only from
// documentNumberColumns.
func documentNumberTaken(q sqlGetExecer, documentType, organizationID, number, exceptID string) (bool, error) {
	col, ok := documentNumberColumns[documentType]
	if !ok {
		return false, newValidationError("unknown document type %q", documentType)
	}
	var n int
	if err := q.Get(&n,
		`SELECT COUNT(*) FROM `+col.table+` WHERE organizationId = ? AND `+col.column+` = ? AND id <> ?`,
		organizationID, number, exceptID,
	); err != nil {
		return false, fmt.Errorf("document_number_taken %s: %w", documentType, err)
	}
	return n > 0, nil
}

// refuseTakenDocumentNumber is the 409 for a number another document of the
// same type already carries.
func refuseTakenDocumentNumber(documentType, number string) error {
	label := documentNumberColumns[documentType].label
	return newValidationError(
		"%s number %q is already used by another %s — clear the number to get the next free one",
		label, number, label,
	)
}

// isDuplicateDocumentNumber recognizes migration 0108's unique index refusing
// a write for documentType. Matched on table.column, since both delivery
// tables call the column deliveryNumber. The in-transaction checks run first,
// so this is a backstop that turns the raw constraint error into the same 409.
func isDuplicateDocumentNumber(err error, documentType string) bool {
	col, ok := documentNumberColumns[documentType]
	return ok && err != nil && strings.Contains(err.Error(), "UNIQUE constraint failed") &&
		strings.Contains(err.Error(), col.table+"."+col.column)
}

// AssignDocumentNumberTx returns the number a new document of documentType
// stores, inside the document's own transaction. A blank explicit number gets
// the next free one (GenerateNextDocumentNumberTx). An explicit one is kept
// unless another document of the type already has it (409); the counter
// advances either way, as it always has.
func AssignDocumentNumberTx(tx *sqlx.Tx, organizationID, documentType string, date time.Time, clientCode, explicit string) (string, error) {
	generated, err := GenerateNextDocumentNumberTx(tx, organizationID, documentType, date, clientCode)
	if err != nil {
		return "", err
	}
	if explicit == "" {
		return generated, nil
	}
	taken, err := documentNumberTaken(tx, documentType, organizationID, explicit, "")
	if err != nil {
		return "", err
	}
	if taken {
		return "", refuseTakenDocumentNumber(documentType, explicit)
	}
	return explicit, nil
}

// checkDocumentNumberChangeTx refuses an update that renames a document to a
// number another document of the type already has. Only a changed, non-blank
// number is checked, as for invoices.
func checkDocumentNumberChangeTx(q sqlGetExecer, documentType, organizationID, documentID, current string, next *string) error {
	if next == nil || *next == "" || *next == current {
		return nil
	}
	taken, err := documentNumberTaken(q, documentType, organizationID, *next, documentID)
	if err != nil {
		return err
	}
	if taken {
		return refuseTakenDocumentNumber(documentType, *next)
	}
	return nil
}
