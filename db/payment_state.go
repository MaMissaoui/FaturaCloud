package db

import (
	"fmt"
)

// A document's state is otherwise a manual flag, but payments keep "paid" in
// step with the balance (owner decision, 2026-10-05): recording a payment
// that covers the whole total moves a sent invoice or an approved bill to
// "paid", and voiding a payment that leaves a balance again moves it back.
// Both directions are pure state changes — "sent"/"approved" and "paid" carry
// the same GL presence (needsInvoiceGLPresence, needsIncomingInvoiceGLPresence)
// and, for a Cash Book sale, the same stock presence — so nothing is posted
// or reversed; CreateCashSalePayment already did the forward half for its own
// loan payments. Past documents paid before this rule keep their state.

// paidStateDocument names, per payment application type, the document table
// and the open state a payment moves to "paid".
var paidStateDocument = map[string]struct{ table, open string }{
	"invoice":          {"invoices", "sent"},
	"incoming_invoice": {"incoming_invoices", "approved"},
}

// documentPaidStateTx reads a document's state, total and what its non-voided
// payments cover, inside the payment's transaction.
func documentPaidStateTx(tx sqlGetExecer, documentType, documentID string) (state string, total, paid int64, err error) {
	doc, ok := paidStateDocument[documentType]
	if !ok {
		return "", 0, 0, fmt.Errorf("paid_state: unknown document type %q", documentType)
	}
	var row struct {
		State string `db:"state"`
		Total int64  `db:"total"`
	}
	if err := tx.Get(&row, fmt.Sprintf(`SELECT state, total FROM %s WHERE id = ?`, doc.table), documentID); err != nil {
		return "", 0, 0, fmt.Errorf("paid_state %s lookup: %w", documentType, err)
	}
	paid, err = getDocumentAmountPaidTx(tx, documentType, documentID)
	if err != nil {
		return "", 0, 0, err
	}
	return row.State, row.Total, paid, nil
}

// markPaidIfSettledTx moves a document from its open state to "paid" once its
// payments cover the whole total. Called after a payment's applications are
// inserted.
func markPaidIfSettledTx(tx sqlGetExecer, documentType, documentID string) error {
	doc := paidStateDocument[documentType]
	state, total, paid, err := documentPaidStateTx(tx, documentType, documentID)
	if err != nil {
		return err
	}
	if state != doc.open || total <= 0 || paid < total {
		return nil
	}
	if _, err := tx.Exec(
		fmt.Sprintf(`UPDATE %s SET state = 'paid' WHERE id = ? AND state = ?`, doc.table),
		documentID, doc.open,
	); err != nil {
		return fmt.Errorf("mark_paid %s: %w", documentType, err)
	}
	return nil
}

// reopenIfUnsettledTx moves a "paid" document back to its open state when
// voiding a payment leaves a balance. paidBefore is what its payments covered
// before the void: only a document they fully covered is moved, so one marked
// paid by hand on a part payment keeps the state someone chose.
func reopenIfUnsettledTx(tx sqlGetExecer, documentType, documentID string, paidBefore int64) error {
	doc := paidStateDocument[documentType]
	state, total, paid, err := documentPaidStateTx(tx, documentType, documentID)
	if err != nil {
		return err
	}
	if state != "paid" || paidBefore < total || paid >= total {
		return nil
	}
	if _, err := tx.Exec(
		fmt.Sprintf(`UPDATE %s SET state = ? WHERE id = ? AND state = 'paid'`, doc.table),
		doc.open, documentID,
	); err != nil {
		return fmt.Errorf("reopen %s: %w", documentType, err)
	}
	return nil
}
