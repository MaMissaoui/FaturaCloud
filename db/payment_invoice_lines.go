package db

import (
	"fmt"
	"sort"
	"strings"
)

// PaymentInvoiceDetail is one sales invoice a payment was applied to, with
// its lines — what the Cash Book Payment history's product panel shows for a
// payment (GET /api/payments/{id}/invoice-lines).
type PaymentInvoiceDetail struct {
	InvoiceID     string `json:"invoiceId"`
	InvoiceNumber string `json:"invoiceNumber"`
	// InvoiceTotal is the invoice's total (tax, discount and stamp included).
	InvoiceTotal int64 `json:"invoiceTotal"`
	// Applied is how much of this payment went to this invoice — for a loan's
	// upfront amount, less than InvoiceTotal.
	Applied int64 `json:"applied"`
	// WholeInvoice is true when (part of) the payment was applied to the
	// invoice as a whole rather than to named lines.
	WholeInvoice bool                 `json:"wholeInvoice"`
	Lines        []PaymentInvoiceLine `json:"lines"`
}

// PaymentInvoiceLine is one line of a PaymentInvoiceDetail. Amount is the
// line's share of the invoice total (tax, discount and stamp included) —
// the same figure the Cash Book's Loan status shows for it, from
// allocateInvoiceLines — so it matches the gross price entered at the
// counter rather than the stored net unit price.
type PaymentInvoiceLine struct {
	LineID      string  `json:"lineId"`
	ProductName string  `json:"productName"`
	Sku         string  `json:"sku"`
	Quantity    float64 `json:"quantity"`
	Amount      int64   `json:"amount"`
	// PaidByThisPayment is what this payment applied to this line directly
	// (a Cash Book line payment); 0 for a whole-invoice application.
	PaidByThisPayment int64 `json:"paidByThisPayment"`
}

// GetPaymentInvoiceLines returns the sales invoices a payment was applied
// to, in application order, each with its lines in invoice order. A vendor
// payment (applied to incoming invoices) returns an empty list.
func (d *Database) GetPaymentInvoiceLines(paymentID string) ([]PaymentInvoiceDetail, error) {
	var apps []struct {
		InvoiceID         string  `db:"invoiceId"`
		InvoiceNumber     string  `db:"invoiceNumber"`
		InvoiceTotal      int64   `db:"invoiceTotal"`
		InvoiceLineItemID *string `db:"invoiceLineItemId"`
		Amount            int64   `db:"amount"`
	}
	if err := d.DB.Select(&apps, `
		SELECT pa.documentId AS invoiceId, COALESCE(i.number, '') AS invoiceNumber, i.total AS invoiceTotal,
		       pa.invoiceLineItemId, pa.amount
		FROM payment_applications pa
		JOIN invoices i ON i.id = pa.documentId
		WHERE pa.paymentId = ? AND pa.documentType = 'invoice'
		ORDER BY pa.createdAt ASC, pa.rowid ASC`, paymentID,
	); err != nil {
		return nil, fmt.Errorf("get_payment_invoice_lines applications: %w", err)
	}

	details := []PaymentInvoiceDetail{}
	index := map[string]int{}
	linePaid := map[string]int64{}
	args := []any{}
	for _, a := range apps {
		k, ok := index[a.InvoiceID]
		if !ok {
			k = len(details)
			index[a.InvoiceID] = k
			details = append(details, PaymentInvoiceDetail{
				InvoiceID: a.InvoiceID, InvoiceNumber: a.InvoiceNumber, InvoiceTotal: a.InvoiceTotal,
				Lines: []PaymentInvoiceLine{},
			})
			args = append(args, a.InvoiceID)
		}
		details[k].Applied += a.Amount
		if a.InvoiceLineItemID == nil {
			details[k].WholeInvoice = true
		} else {
			linePaid[*a.InvoiceLineItemID] += a.Amount
		}
	}
	if len(details) == 0 {
		return details, nil
	}

	placeholders := strings.TrimSuffix(strings.Repeat("?,", len(args)), ",")
	raw := []loanLineRaw{}
	if err := d.DB.Select(&raw,
		`SELECT * FROM (`+loanLinesQuery(`i.id IN (`+placeholders+`)`)+`) ORDER BY invoiceId ASC, `+loanLineOrder,
		args...,
	); err != nil {
		return nil, fmt.Errorf("get_payment_invoice_lines lines: %w", err)
	}
	position := map[string]int{}
	var positions []struct {
		ID       string `db:"id"`
		Position int    `db:"position"`
	}
	if err := d.DB.Select(&positions,
		`SELECT id, position FROM invoiceLineItems WHERE invoiceId IN (`+placeholders+`)`, args...,
	); err != nil {
		return nil, fmt.Errorf("get_payment_invoice_lines positions: %w", err)
	}
	for _, p := range positions {
		position[p.ID] = p.Position
	}

	// Rows are contiguous per invoice: allocate each invoice's total across
	// its lines exactly as the loan tracker does, then list the lines in the
	// order they appear on the invoice.
	for i := 0; i < len(raw); {
		j := i
		for j < len(raw) && raw[j].InvoiceID == raw[i].InvoiceID {
			j++
		}
		amounts, _ := allocateInvoiceLines(raw[i:j])
		det := &details[index[raw[i].InvoiceID]]
		for k := i; k < j; k++ {
			r := raw[k]
			det.Lines = append(det.Lines, PaymentInvoiceLine{
				LineID: r.LineID, ProductName: r.ProductName, Sku: r.Sku, Quantity: r.Quantity,
				Amount: amounts[k-i], PaidByThisPayment: linePaid[r.LineID],
			})
		}
		sort.SliceStable(det.Lines, func(a, b int) bool {
			return position[det.Lines[a].LineID] < position[det.Lines[b].LineID]
		})
		i = j
	}
	return details, nil
}
