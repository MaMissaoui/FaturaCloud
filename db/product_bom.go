package db

import (
	"fmt"

	gonanoid "github.com/matoous/go-nanoid/v2"
)

// BillOfMaterialsLine is one component + quantity-per-unit entry in a
// finished product's recipe. ComponentName/ComponentSKU/ComponentUnit are
// joined for display — the same convenience-join shape
// GetInboundDeliveryLineItems uses for its own productName/sku columns.
type BillOfMaterialsLine struct {
	ID                 string  `db:"id"                 json:"id"`
	OrganizationID     string  `db:"organizationId"     json:"organizationId"`
	FinishedProductID  string  `db:"finishedProductId"  json:"finishedProductId"`
	ComponentProductID string  `db:"componentProductId" json:"componentProductId"`
	QuantityPerUnit    float64 `db:"quantityPerUnit"    json:"quantityPerUnit"`
	CreatedAt          *string `db:"createdAt"          json:"createdAt"`
	ComponentName      string  `db:"componentName"      json:"componentName"`
	ComponentSKU       *string `db:"componentSku"       json:"componentSku"`
	ComponentUnit      *string `db:"componentUnit"      json:"componentUnit"`
}

// CreateBillOfMaterialsLineRequest is one line of a ReplaceBillOfMaterials
// call — the wire shape a client sends, before any id/organizationId is
// resolved.
type CreateBillOfMaterialsLineRequest struct {
	ComponentProductID string  `json:"componentProductId"`
	QuantityPerUnit    float64 `json:"quantityPerUnit"`
}

// GetBillOfMaterials returns a finished product's recipe, ordered by when
// each line was added — stable, human-predictable ordering for a list with
// no other natural sort key (mirrors line-item position ordering elsewhere,
// just via createdAt/rowid since these rows are never reordered).
func (d *Database) GetBillOfMaterials(finishedProductID string) ([]BillOfMaterialsLine, error) {
	lines := []BillOfMaterialsLine{}
	err := d.DB.Select(&lines, `
		SELECT bom.*, p.name AS componentName, p.sku AS componentSku, p.unit AS componentUnit
		FROM bill_of_materials bom
		JOIN products p ON bom.componentProductId = p.id
		WHERE bom.finishedProductId = ?
		ORDER BY bom.createdAt ASC, bom.rowid ASC`,
		finishedProductID,
	)
	if err != nil {
		return nil, fmt.Errorf("get_bill_of_materials: %w", err)
	}
	return lines, nil
}

// ReplaceBillOfMaterials wholesale-replaces a finished product's recipe —
// the same "delete then re-insert in order" shape
// replaceInboundDeliveryLineItemsTx uses for line items, since a BOM has no
// natural per-row update identity from the client's perspective (the
// frontend always resends the whole list). organizationID is the finished
// product's own — derived by the caller from the path (router-level
// orgMemberProtected gating already confirmed the caller belongs to it),
// not client-supplied, so there is nothing to cross-check it against;
// every referenced component is instead validated against it (issue #189's
// requireSameOrg pattern, plus a domain-specific category check no other
// line-item replace needs).
func (d *Database) ReplaceBillOfMaterials(finishedProductID string, lines []CreateBillOfMaterialsLineRequest) ([]BillOfMaterialsLine, error) {
	finished, err := d.GetProduct(finishedProductID)
	if err != nil {
		return nil, newValidationError("finished product not found")
	}
	organizationID := finished.OrganizationID
	if finished.Category == nil || *finished.Category != "finished" {
		return nil, newValidationError("a bill of materials can only be defined for a %q product", "finished")
	}

	// Every lookup below runs against d.DB, before Beginx() opens — not
	// inside the transaction — since db.SetMaxOpenConns(1) means a second
	// connection request while a tx holds the only one blocks forever
	// (the same constraint every Phase 7 posting path in gl_posting.go and
	// UpdateInboundDeliveryStatus already follows).
	type validatedLine struct {
		id                 string
		componentProductID string
		quantityPerUnit    float64
	}
	validated := make([]validatedLine, 0, len(lines))
	seen := make(map[string]bool, len(lines))
	for i, line := range lines {
		if line.QuantityPerUnit <= 0 {
			return nil, newValidationError("line %d: quantity per unit must be greater than zero", i+1)
		}
		if seen[line.ComponentProductID] {
			return nil, newValidationError("line %d: component already appears earlier in this bill of materials", i+1)
		}
		seen[line.ComponentProductID] = true

		component, err := d.GetProduct(line.ComponentProductID)
		if err != nil {
			return nil, newValidationError("line %d: component product not found", i+1)
		}
		if err := requireSameOrg(organizationID, component.OrganizationID, fmt.Sprintf("line %d: component product", i+1)); err != nil {
			return nil, err
		}
		if component.Category == nil || *component.Category != "component" {
			return nil, newValidationError("line %d: %q is not a %q product", i+1, component.Name, "component")
		}

		id, _ := gonanoid.New()
		validated = append(validated, validatedLine{id: id, componentProductID: line.ComponentProductID, quantityPerUnit: line.QuantityPerUnit})
	}

	tx, err := d.DB.Beginx()
	if err != nil {
		return nil, fmt.Errorf("replace_bill_of_materials begin: %w", err)
	}
	defer tx.Rollback() //nolint:errcheck

	if _, err := tx.Exec(`DELETE FROM bill_of_materials WHERE finishedProductId = ?`, finishedProductID); err != nil {
		return nil, fmt.Errorf("delete_bill_of_materials: %w", err)
	}
	for _, line := range validated {
		if _, err := tx.Exec(`
			INSERT INTO bill_of_materials (id, organizationId, finishedProductId, componentProductId, quantityPerUnit)
			VALUES (?, ?, ?, ?, ?)`,
			line.id, organizationID, finishedProductID, line.componentProductID, line.quantityPerUnit,
		); err != nil {
			return nil, fmt.Errorf("insert_bill_of_materials_line: %w", err)
		}
	}

	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("replace_bill_of_materials commit: %w", err)
	}
	return d.GetBillOfMaterials(finishedProductID)
}
