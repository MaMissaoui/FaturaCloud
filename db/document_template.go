package db

import (
	"bytes"
	"database/sql"
	"errors"
	"fmt"

	gonanoid "github.com/matoous/go-nanoid/v2"
	"github.com/xuri/excelize/v2"
)

// DocumentTemplate is an organization's uploaded override for one document
// type's export template. Absence of a row for (organizationId,
// documentType) means "use the embedded default" — see resolveTemplateBytes.
type DocumentTemplate struct {
	ID             string `db:"id"             json:"id"`
	OrganizationID string `db:"organizationId" json:"organizationId"`
	DocumentType   string `db:"documentType"   json:"documentType"`
	Filename       string `db:"filename"       json:"filename"`
	CreatedAt      string `db:"createdAt"      json:"createdAt"`
	UpdatedAt      string `db:"updatedAt"      json:"updatedAt"`
}

// IsKnownDocumentType reports whether documentType has an embedded default —
// the API layer's allowlist check before ever touching the database, so an
// arbitrary caller-supplied path segment can't be used to store an unbounded
// number of blobs under made-up keys.
func IsKnownDocumentType(documentType string) bool {
	_, ok := embeddedDefaultTemplates[documentType]
	return ok
}

// GetDocumentTemplates lists an org's uploaded overrides (metadata only,
// never content — this is what the Settings page uses to know which
// document types currently have a custom template versus the default).
func (d *Database) GetDocumentTemplates(organizationID string) ([]DocumentTemplate, error) {
	rows := []DocumentTemplate{} // non-nil so an empty result serializes as [] over JSON, not null
	err := d.DB.Select(&rows, `
		SELECT id, organizationId, documentType, filename, createdAt, updatedAt
		FROM document_templates WHERE organizationId = ? ORDER BY documentType`,
		organizationID,
	)
	if err != nil {
		return nil, fmt.Errorf("get_document_templates: %w", err)
	}
	return rows, nil
}

// GetDocumentTemplate returns the org's override row for a document type, or
// sql.ErrNoRows if none exists — that's the normal "use the default" signal,
// not an error state, mirroring GetOrganizationLogo's empty-result handling.
func (d *Database) GetDocumentTemplate(organizationID, documentType string) (*DocumentTemplate, []byte, error) {
	var row struct {
		DocumentTemplate
		Content []byte `db:"content"`
	}
	err := d.DB.Get(&row, `
		SELECT id, organizationId, documentType, filename, content, createdAt, updatedAt
		FROM document_templates WHERE organizationId = ? AND documentType = ?`,
		organizationID, documentType,
	)
	if err != nil {
		return nil, nil, err
	}
	return &row.DocumentTemplate, row.Content, nil
}

// UploadDocumentTemplate validates content as a real .xlsx (it will later be
// opened and filled, not just displayed — a stronger bar than the logo's
// content-type sniff) and upserts it as the org's override for documentType.
// Checks the organization exists first: without this, a bad organizationId
// would otherwise fail the INSERT's FK constraint and surface as an opaque
// 500 instead of the clean 404 every other lookup-by-org-id path in this
// codebase returns.
func (d *Database) UploadDocumentTemplate(organizationID, documentType, filename string, content []byte) error {
	if _, err := excelize.OpenReader(bytes.NewReader(content)); err != nil {
		return newValidationError("file is not a valid Excel (.xlsx) workbook")
	}

	var exists int
	if err := d.DB.Get(&exists, `SELECT 1 FROM organizations WHERE id = ?`, organizationID); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return err // sql.ErrNoRows, translated by the API layer into a 404
		}
		return fmt.Errorf("upload_document_template: check organization: %w", err)
	}

	id, _ := gonanoid.New()
	_, err := d.DB.Exec(`
		INSERT INTO document_templates (id, organizationId, documentType, filename, content, updatedAt)
		VALUES (?, ?, ?, ?, ?, strftime('%s', 'now') * 1000)
		ON CONFLICT(organizationId, documentType) DO UPDATE SET
			filename = excluded.filename,
			content = excluded.content,
			updatedAt = excluded.updatedAt`,
		id, organizationID, documentType, filename, content,
	)
	if err != nil {
		return fmt.Errorf("upload_document_template: %w", err)
	}
	return nil
}

// DeleteDocumentTemplate removes an org's override, reverting future exports
// to the embedded default. Returns false if no override existed.
func (d *Database) DeleteDocumentTemplate(organizationID, documentType string) (bool, error) {
	res, err := d.DB.Exec(
		`DELETE FROM document_templates WHERE organizationId = ? AND documentType = ?`,
		organizationID, documentType,
	)
	if err != nil {
		return false, fmt.Errorf("delete_document_template: %w", err)
	}
	n, _ := res.RowsAffected()
	return n > 0, nil
}

// ExportDocumentTemplateBytes is what the download endpoint streams: an
// org's uploaded override if one exists (with its original filename), else
// the embedded default (named "<documentType>_default.xlsx") — so
// "download the current template" and "download the default" (no override
// yet) share one handler.
func (d *Database) ExportDocumentTemplateBytes(organizationID, documentType string) ([]byte, string, error) {
	row, content, err := d.GetDocumentTemplate(organizationID, documentType)
	if err == nil {
		return content, row.Filename, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return nil, "", fmt.Errorf("export_document_template_bytes: %w", err)
	}
	def, ok := embeddedDefaultTemplates[documentType]
	if !ok {
		return nil, "", newValidationError("unknown document type %q", documentType)
	}
	return def, documentType + "_default.xlsx", nil
}

// resolveTemplateBytes returns an org's uploaded override for documentType,
// falling back to the embedded default shipped with the binary. An unknown
// documentType returns the same *ValidationError ExportDocumentTemplateBytes
// does — every caller passes a constant today, but this stays a clean error
// rather than a panic so the two functions agree on this condition.
func resolveTemplateBytes(d *Database, organizationID, documentType string) ([]byte, string, error) {
	_, content, err := d.GetDocumentTemplate(organizationID, documentType)
	if err == nil {
		return content, "override", nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return nil, "", fmt.Errorf("resolve_template_bytes: %w", err)
	}
	def, ok := embeddedDefaultTemplates[documentType]
	if !ok {
		return nil, "", newValidationError("unknown document type %q", documentType)
	}
	return def, "default", nil
}
