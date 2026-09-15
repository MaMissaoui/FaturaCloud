package db

import (
	"fmt"

	gonanoid "github.com/matoous/go-nanoid/v2"
)

// Line-item id reconciliation (F70 / F93, audit 2026-09-14).
//
// Two line-item tables in this schema are pointed at by other tables'
// foreign keys:
//
//	outbound_delivery_line_items.orderLineItemId   -> orderLineItems(id)
//	inbound_delivery_line_items.purchaseOrderLineItemId
//	incoming_invoice_line_items.purchaseOrderLineItemId
//	                                              -> purchase_order_line_items(id)
//
// All three are ON DELETE SET NULL. Both documents used to save their lines
// by deleting every row and reinserting with fresh nanoids, which silently
// nulled those links on every edit — permanently, since nothing re-derives
// them. The damage was not cosmetic: the purchase-order side defeated the
// billed-receipt cancel guard (db/inbound_delivery.go's
// grniClearedQtyForPOLine loop skips any line whose id is nil), left GRNI
// permanently accrued, and made 3-way matching report every line
// "unlinked"; the order side re-offered already-shipped quantity in the
// delivery prefill.
//
// The other four line-item tables (invoices, incoming invoices, outbound
// deliveries, inbound deliveries) have no inbound foreign key to their own
// ids at all, so their delete-and-reinsert is harmless and is deliberately
// left alone — see the audit's 1.1 enumeration.
//
// Reconciling by id rather than by position is the point: position matching
// would keep an id while the row under it became a different product, which
// is worse than nulling — a stale link that silently points at the wrong
// line instead of an obviously absent one.

// lineItemSlot is the resolved identity of one incoming line item: which
// row id it should occupy, and whether that row already exists (so the
// caller UPDATEs) or not (so the caller INSERTs).
type lineItemSlot struct {
	ID       string
	Existing bool
}

// reconcileLineItemIDs maps a document's incoming line items onto its
// existing rows, preserving the id of every line the request still carries.
//
// requestedIDs is positional: one entry per incoming line item, nil (or
// empty) for a line the client is adding. A non-nil id is honoured **only**
// if it is genuinely one of this parent's own rows — a stale id, another
// document's id, or a client-invented one falls back to a fresh nanoid
// rather than being trusted as a primary key. The same id sent twice is
// honoured once, for the same reason.
//
// It returns one slot per incoming item, plus the ids of this parent's rows
// that the request no longer carries, which the caller must delete.
//
// table and parentColumn are internal constants, never user input.
func reconcileLineItemIDs(
	exec sqlSelectExecer, table, parentColumn, parentID string, requestedIDs []*string,
) (slots []lineItemSlot, obsolete []string, err error) {
	var currentIDs []string
	if err := exec.Select(&currentIDs, fmt.Sprintf(
		`SELECT id FROM %s WHERE %s = ?`, table, parentColumn,
	), parentID); err != nil {
		return nil, nil, fmt.Errorf("reconcile_line_items %s select: %w", table, err)
	}

	current := make(map[string]bool, len(currentIDs))
	for _, id := range currentIDs {
		current[id] = true
	}

	kept := make(map[string]bool, len(requestedIDs))
	slots = make([]lineItemSlot, 0, len(requestedIDs))
	for _, requested := range requestedIDs {
		if requested != nil && *requested != "" && current[*requested] && !kept[*requested] {
			kept[*requested] = true
			slots = append(slots, lineItemSlot{ID: *requested, Existing: true})
			continue
		}
		fresh, err := gonanoid.New()
		if err != nil {
			return nil, nil, fmt.Errorf("reconcile_line_items %s nanoid: %w", table, err)
		}
		slots = append(slots, lineItemSlot{ID: fresh})
	}

	// Deterministic order: currentIDs comes back in whatever order SQLite
	// returns it, but the caller only ever feeds these to a DELETE.
	for _, id := range currentIDs {
		if !kept[id] {
			obsolete = append(obsolete, id)
		}
	}
	return slots, obsolete, nil
}

// deleteLineItemsByID removes the rows reconcileLineItemIDs reported as no
// longer carried by the request. Scoped by the parent as well as the id, so
// a bug upstream can never reach another document's rows.
func deleteLineItemsByID(exec sqlExecer, table, parentColumn, parentID string, ids []string) error {
	for _, id := range ids {
		if _, err := exec.Exec(fmt.Sprintf(
			`DELETE FROM %s WHERE id = ? AND %s = ?`, table, parentColumn,
		), id, parentID); err != nil {
			return fmt.Errorf("reconcile_line_items %s delete: %w", table, err)
		}
	}
	return nil
}
