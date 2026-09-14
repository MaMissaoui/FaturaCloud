package db

import (
	"database/sql"
	"errors"
	"fmt"
	"math"

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

// BOMSummary is one finished product's component-line count — the batch
// counterpart to GetBillOfMaterials, same "one request for every row"
// shape GetImportSummaries already established for imports (issue #147's
// N+1 fix), so the BOM maintenance screen's list page doesn't fire one
// GetBillOfMaterials call per finished product. A finished product with no
// BOM defined yet simply has no entry in the returned slice — unlike
// GetImportSummaries, which backfills a zero-value row for every import,
// callers here already have the full finished-product list from
// productsAtom and can default a missing id to zero themselves.
type BOMSummary struct {
	FinishedProductID string `db:"finishedProductId" json:"finishedProductId"`
	ComponentCount    int    `db:"componentCount"     json:"componentCount"`
}

// BOMVersion is one historical snapshot header of a finished product's
// recipe (bill_of_materials_versions) — the list shape for browsing
// history (component count only); see GetBillOfMaterialsVersionDetail for
// one version's full lines.
type BOMVersion struct {
	ID                string  `db:"id"                json:"id"`
	FinishedProductID string  `db:"finishedProductId" json:"finishedProductId"`
	VersionNumber     int     `db:"versionNumber"     json:"versionNumber"`
	BatchSize         int     `db:"batchSize"         json:"batchSize"`
	ComponentCount    int     `db:"componentCount"    json:"componentCount"`
	CreatedAt         *string `db:"createdAt"         json:"createdAt"`
}

// BOMVersionLine is one denormalized component/quantity entry of a
// historical version. ComponentName/SKU/Unit are captured at snapshot
// time, not live-joined — a version keeps reading correctly even after the
// component product is later renamed or deleted. ComponentProductID is
// nullable for exactly that reason (ON DELETE SET NULL on the migration):
// a nil value means the component behind this historical line no longer
// exists, blocking a restore of this version (see RestoreBillOfMaterialsVersion)
// without losing the historical record of what it used to be.
type BOMVersionLine struct {
	ID                 string  `db:"id"                 json:"id"`
	VersionID          string  `db:"versionId"          json:"versionId"`
	ComponentProductID *string `db:"componentProductId" json:"componentProductId"`
	ComponentName      string  `db:"componentName"      json:"componentName"`
	ComponentSKU       *string `db:"componentSku"       json:"componentSku"`
	ComponentUnit      *string `db:"componentUnit"      json:"componentUnit"`
	QuantityPerUnit    float64 `db:"quantityPerUnit"    json:"quantityPerUnit"`
}

// BOMVersionDetail is one version's header plus its full lines — the read
// shape for viewing or restoring a specific historical recipe.
type BOMVersionDetail struct {
	BOMVersion
	Lines []BOMVersionLine `json:"lines"`
}

// roundBOMQuantity rounds a quantity-per-unit value to 4 decimal places.
// Quantities here aren't money (no math/big exact-rational need, unlike
// db/invoice_totals.go's cents arithmetic) but do need *some* fixed
// precision: the BOM editor's optional "batch size" entry helper divides a
// per-batch quantity by the batch size client-side before saving (e.g. 1
// component over a batch of 3 units -> 0.3333333333333333), and without
// rounding that noise would round-trip visibly on the next load (batch 3 x
// 0.3333333333333333 != 1). 4 decimals comfortably covers the UI's own
// min={0.001} granularity with one digit of headroom.
func roundBOMQuantity(q float64) float64 {
	return math.Round(q*10000) / 10000
}

// GetBillOfMaterialsSummaries returns every finished product's component
// count for an organization in one query.
func (d *Database) GetBillOfMaterialsSummaries(organizationID string) ([]BOMSummary, error) {
	summaries := []BOMSummary{}
	err := d.DB.Select(&summaries, `
		SELECT finishedProductId, COUNT(*) AS componentCount
		FROM bill_of_materials
		WHERE organizationId = ?
		GROUP BY finishedProductId`,
		organizationID,
	)
	if err != nil {
		return nil, fmt.Errorf("get_bill_of_materials_summaries: %w", err)
	}
	return summaries, nil
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

// GetBillOfMaterialsVersions returns every historical version header for a
// finished product, newest first. Filters only on finishedProductId, not
// organizationId — safe today because the handler's orgMemberProtected
// route resolves and checks the org from the product id in the path before
// this ever runs (the same scoping GetBillOfMaterials itself relies on), but
// unlike GetBillOfMaterialsVersionDetail this function has no independent
// org check of its own if ever called from a context that skips that gate.
func (d *Database) GetBillOfMaterialsVersions(finishedProductID string) ([]BOMVersion, error) {
	versions := []BOMVersion{}
	err := d.DB.Select(&versions, `
		SELECT v.id, v.finishedProductId, v.versionNumber, v.batchSize, v.createdAt,
		       COUNT(l.id) AS componentCount
		FROM bill_of_materials_versions v
		LEFT JOIN bill_of_materials_version_lines l ON l.versionId = v.id
		WHERE v.finishedProductId = ?
		GROUP BY v.id
		ORDER BY v.versionNumber DESC`,
		finishedProductID,
	)
	if err != nil {
		return nil, fmt.Errorf("get_bill_of_materials_versions: %w", err)
	}
	return versions, nil
}

// GetBillOfMaterialsVersionDetail returns one version's header and full
// lines — finishedProductID and versionID must both match (the same
// "verify the parent, not just an opaque id" scoping every other by-id
// lookup in this app follows), returning sql.ErrNoRows (mapped to a 404 by
// the handler, matching GetProduct's own convention) if they don't.
func (d *Database) GetBillOfMaterialsVersionDetail(finishedProductID, versionID string) (*BOMVersionDetail, error) {
	var version BOMVersion
	err := d.DB.Get(&version, `
		SELECT v.id, v.finishedProductId, v.versionNumber, v.batchSize, v.createdAt,
		       (SELECT COUNT(*) FROM bill_of_materials_version_lines WHERE versionId = v.id) AS componentCount
		FROM bill_of_materials_versions v
		WHERE v.id = ?`,
		versionID,
	)
	if err != nil {
		return nil, err
	}
	if version.FinishedProductID != finishedProductID {
		return nil, sql.ErrNoRows
	}

	lines := []BOMVersionLine{}
	if err := d.DB.Select(&lines, `
		SELECT id, versionId, componentProductId, componentName, componentSku, componentUnit, quantityPerUnit
		FROM bill_of_materials_version_lines
		WHERE versionId = ?
		ORDER BY createdAt ASC, rowid ASC`,
		versionID,
	); err != nil {
		return nil, fmt.Errorf("get_bill_of_materials_version_detail lines: %w", err)
	}

	return &BOMVersionDetail{BOMVersion: version, Lines: lines}, nil
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
//
// Every successful call also snapshots the resulting NEW state into
// bill_of_materials_versions — version 1 is the very first recipe ever
// saved (not "the state before the first edit"), so history is complete
// from the start. batchSize is purely a UI entry-helper memory (see
// roundBOMQuantity's comment); it's stored on the version so reopening the
// editor replays the same batch view, but never changes what
// quantityPerUnit itself means (always per single unit). A save that
// results in the exact same component set/quantities and the same
// batchSize as the latest existing version is a no-op for history — it
// still rewrites the live table (harmless, keeps this function's shape
// uniform) but doesn't add a duplicate version, so re-running
// cmd/seed-demo's setupBillsOfMaterials or reopening-then-saving the
// editor unchanged doesn't pad every product's history with identical
// entries.
func (d *Database) ReplaceBillOfMaterials(finishedProductID string, lines []CreateBillOfMaterialsLineRequest, batchSize int) ([]BillOfMaterialsLine, error) {
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
		componentName      string
		componentSKU       *string
		componentUnit      *string
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
		validated = append(validated, validatedLine{
			id:                 id,
			componentProductID: line.ComponentProductID,
			quantityPerUnit:    roundBOMQuantity(line.QuantityPerUnit),
			componentName:      component.Name,
			componentSKU:       component.SKU,
			componentUnit:      component.Unit,
		})
	}

	// The latest version's header (for the next version number and to
	// compare batchSize) and its own lines (the no-op baseline — see below)
	// are both read here too, before Beginx() — same constraint as above.
	var latestVersion struct {
		ID            string `db:"id"`
		VersionNumber int    `db:"versionNumber"`
		BatchSize     int    `db:"batchSize"`
	}
	hasLatestVersion := true
	if err := d.DB.Get(&latestVersion, `
		SELECT id, versionNumber, batchSize FROM bill_of_materials_versions
		WHERE finishedProductId = ? ORDER BY versionNumber DESC LIMIT 1`,
		finishedProductID,
	); err != nil {
		if !errors.Is(err, sql.ErrNoRows) {
			return nil, fmt.Errorf("replace_bill_of_materials latest_version: %w", err)
		}
		hasLatestVersion = false
	}

	// batchSize <= 0 means the caller has no batch-size concept of its own
	// (the product form's embedded BOM card always sends 0/omits it — see
	// ReplaceProductBOM's frontend signature) rather than an explicit
	// request to reset to plain per-unit entry. Inheriting the latest
	// version's batchSize in that case, instead of forcing 1, keeps that
	// save a true no-op when nothing else changed either — forcing 1 would
	// flip `changed` below purely because a caller with no batch UI can't
	// echo back whatever batch view the dedicated BOM drawer last saved.
	if batchSize <= 0 {
		if hasLatestVersion {
			batchSize = latestVersion.BatchSize
		} else {
			batchSize = 1
		}
	}

	nextVersionNumber := 1
	changed := true
	if hasLatestVersion {
		nextVersionNumber = latestVersion.VersionNumber + 1

		// Compare against the latest VERSION's own lines, not the live
		// bill_of_materials table — the two can legitimately diverge (e.g.
		// deleting a component product cascades the live row via
		// componentProductId's ON DELETE CASCADE, but the version line
		// survives denormalized via ON DELETE SET NULL), and comparing
		// against live would then silently skip recording that the recipe
		// actually changed on the next save.
		var latestVersionLines []struct {
			ComponentProductID *string `db:"componentProductId"`
			QuantityPerUnit    float64 `db:"quantityPerUnit"`
		}
		if err := d.DB.Select(&latestVersionLines, `
			SELECT componentProductId, quantityPerUnit
			FROM bill_of_materials_version_lines WHERE versionId = ?`,
			latestVersion.ID,
		); err != nil {
			return nil, fmt.Errorf("replace_bill_of_materials latest_version_lines: %w", err)
		}

		if latestVersion.BatchSize == batchSize && len(latestVersionLines) == len(validated) {
			existingByComponent := make(map[string]float64, len(latestVersionLines))
			for _, l := range latestVersionLines {
				if l.ComponentProductID == nil {
					continue
				}
				existingByComponent[*l.ComponentProductID] = l.QuantityPerUnit
			}
			changed = false
			for _, v := range validated {
				qty, ok := existingByComponent[v.componentProductID]
				if !ok || qty != v.quantityPerUnit {
					changed = true
					break
				}
			}
		}
	} else if len(validated) == 0 {
		// Nothing ever saved and nothing to save now — no real state to
		// record a version 1 for.
		changed = false
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

	if changed {
		versionID, _ := gonanoid.New()
		if _, err := tx.Exec(`
			INSERT INTO bill_of_materials_versions (id, organizationId, finishedProductId, versionNumber, batchSize)
			VALUES (?, ?, ?, ?, ?)`,
			versionID, organizationID, finishedProductID, nextVersionNumber, batchSize,
		); err != nil {
			return nil, fmt.Errorf("insert_bill_of_materials_version: %w", err)
		}
		for _, line := range validated {
			lineID, _ := gonanoid.New()
			if _, err := tx.Exec(`
				INSERT INTO bill_of_materials_version_lines
					(id, versionId, componentProductId, componentName, componentSku, componentUnit, quantityPerUnit)
				VALUES (?, ?, ?, ?, ?, ?, ?)`,
				lineID, versionID, line.componentProductID, line.componentName, line.componentSKU, line.componentUnit, line.quantityPerUnit,
			); err != nil {
				return nil, fmt.Errorf("insert_bill_of_materials_version_line: %w", err)
			}
		}
	}

	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("replace_bill_of_materials commit: %w", err)
	}
	return d.GetBillOfMaterials(finishedProductID)
}

// RestoreBillOfMaterialsVersion makes a historical version the current
// recipe again — implemented as an ordinary ReplaceBillOfMaterials call
// using that version's lines, so it goes through the exact same
// validation (component still exists, still category "component", same
// org) and itself creates a new version recording the restore, rather
// than rewriting history. Fails with a 409 naming the line, not a silent
// partial restore, if any of the version's components no longer exists
// (componentProductId went NULL via the migration's ON DELETE SET NULL) —
// a smaller-than-recorded recipe restored silently would be a worse
// surprise than refusing outright.
func (d *Database) RestoreBillOfMaterialsVersion(finishedProductID, versionID string) ([]BillOfMaterialsLine, error) {
	detail, err := d.GetBillOfMaterialsVersionDetail(finishedProductID, versionID)
	if err != nil {
		return nil, err
	}

	lines := make([]CreateBillOfMaterialsLineRequest, 0, len(detail.Lines))
	for _, l := range detail.Lines {
		if l.ComponentProductID == nil {
			return nil, newValidationError(
				"cannot restore version %d: component %q no longer exists",
				detail.VersionNumber, l.ComponentName,
			)
		}
		lines = append(lines, CreateBillOfMaterialsLineRequest{
			ComponentProductID: *l.ComponentProductID,
			QuantityPerUnit:    l.QuantityPerUnit,
		})
	}

	return d.ReplaceBillOfMaterials(finishedProductID, lines, detail.BatchSize)
}
