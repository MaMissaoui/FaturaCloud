package db

import (
	"database/sql"
	"errors"
	"fmt"
)

var documentOrientations = map[string]bool{"portrait": true, "landscape": true}

// GetDocumentTemplateOrientation returns the org's configured page
// orientation override for a document type, or "" if none was ever set —
// the "no override" signal fillTemplate uses to leave the template's own
// authored page setup untouched (see xlsx_export.go). This is deliberately
// NOT "portrait" as the no-row default: a template's own orientation
// (blank on every embedded default, but possibly set on an org's own
// custom upload) must survive unless the org has explicitly overridden it.
func (d *Database) GetDocumentTemplateOrientation(organizationID, documentType string) (string, error) {
	var orientation string
	err := d.DB.Get(&orientation, `
		SELECT orientation FROM document_template_settings
		WHERE organizationId = ? AND documentType = ?`,
		organizationID, documentType,
	)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return "", nil
		}
		return "", fmt.Errorf("get_document_template_orientation: %w", err)
	}
	return orientation, nil
}

// SetDocumentTemplateOrientation upserts the org's page orientation
// preference for a document type. This wins at fill time over whatever the
// template itself (embedded default or an org's own upload) was authored
// with — an explicitly stated organization preference is authoritative, the
// same "the org's own setting wins" precedent organizations.fiscalStampEnabled
// already sets for invoice fields.
func (d *Database) SetDocumentTemplateOrientation(organizationID, documentType, orientation string) error {
	if !documentOrientations[orientation] {
		return newValidationError("orientation must be \"portrait\" or \"landscape\"")
	}
	var exists int
	if err := d.DB.Get(&exists, `SELECT 1 FROM organizations WHERE id = ?`, organizationID); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return err // sql.ErrNoRows, translated by the API layer into a 404
		}
		return fmt.Errorf("set_document_template_orientation: check organization: %w", err)
	}
	_, err := d.DB.Exec(`
		INSERT INTO document_template_settings (organizationId, documentType, orientation, updatedAt)
		VALUES (?, ?, ?, strftime('%s', 'now') * 1000)
		ON CONFLICT(organizationId, documentType) DO UPDATE SET
			orientation = excluded.orientation,
			updatedAt = excluded.updatedAt`,
		organizationID, documentType, orientation,
	)
	if err != nil {
		return fmt.Errorf("set_document_template_orientation: %w", err)
	}
	return nil
}

// DeleteDocumentTemplateOrientation removes an org's orientation override,
// reverting to the template's own authored page setup. Returns false if no
// override existed — the same "was there anything to remove" shape as
// DeleteDocumentTemplate.
func (d *Database) DeleteDocumentTemplateOrientation(organizationID, documentType string) (bool, error) {
	res, err := d.DB.Exec(
		`DELETE FROM document_template_settings WHERE organizationId = ? AND documentType = ?`,
		organizationID, documentType,
	)
	if err != nil {
		return false, fmt.Errorf("delete_document_template_orientation: %w", err)
	}
	n, _ := res.RowsAffected()
	return n > 0, nil
}
