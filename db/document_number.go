package db

import (
	"database/sql"
	"errors"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/jmoiron/sqlx"
)

// documentNumberDefaults is both the set of document types this settings
// table covers and each one's default format — chosen to reproduce exactly
// what that type's old hardcoded-prefix Next*Number function already
// generated (PO-%04d, ORD-%03d, ...), so an organization that never touches
// this setting sees byte-identical numbering to before this table existed.
// Invoices are deliberately not here — see the migration's comment for why
// invoice numbering stays on its own organizations columns.
var documentNumberDefaults = map[string]string{
	"order":            "ORD-{number:3}",
	"purchase_order":   "PO-{number:4}",
	"delivery":         "DEL-{number:4}",
	"inbound_delivery": "GR-{number:4}",
	"production_order": "PRO-{number:4}",
}

// documentNumberTokenPattern finds every {...} placeholder, same approach as
// invoiceNumberFormatTokenPattern in organization.go.
var documentNumberTokenPattern = regexp.MustCompile(`\{[^}]+\}`)

// documentNumberPaddedTokenPattern matches {number} or {number:N} (N is
// 1-2 digits — no format in this app pads past double digits, and an
// unbounded width would let a hostile format allocate an arbitrarily long
// string). Every other recognized token (date/clientCode) takes no argument.
var documentNumberPaddedTokenPattern = regexp.MustCompile(`^\{number(?::(\d{1,2}))?\}$`)

var documentNumberPlainTokens = map[string]bool{
	"{year}": true, "{y}": true, "{month}": true, "{m}": true, "{day}": true, "{clientCode}": true,
}

// validateDocumentNumberFormat rejects a blank format or an unrecognized
// {...} token, the same shape validateInvoiceNumberFormat (organization.go)
// already established — kept as a separate function rather than a shared
// call so invoice's error wording (which mentions "invoice number format"
// specifically) doesn't have to become generic.
func validateDocumentNumberFormat(format string) error {
	if strings.TrimSpace(format) == "" {
		return newValidationError("number format is required")
	}
	for _, tok := range documentNumberTokenPattern.FindAllString(format, -1) {
		if documentNumberPlainTokens[tok] {
			continue
		}
		if documentNumberPaddedTokenPattern.MatchString(tok) {
			continue
		}
		return newValidationError("number format %q has an unrecognized variable %q", format, tok)
	}
	return nil
}

// generateFormattedDocumentNumber substitutes every recognized token in
// format, the generalized version of cash_sale.go's generateDocumentNumber —
// generalized here to additionally support {number:N}, a zero-padded width
// no format in this app could express before this feature (ORD- historically
// padded to 3 digits, PO-/DEL-/GR-/PRO- to 4, all hardcoded in Go rather than
// stated in the format itself).
func generateFormattedDocumentNumber(format string, counter int64, date time.Time, clientCode string) string {
	if format == "" {
		return ""
	}
	result := documentNumberTokenPattern.ReplaceAllStringFunc(format, func(tok string) string {
		if m := documentNumberPaddedTokenPattern.FindStringSubmatch(tok); m != nil {
			if m[1] == "" {
				return strconv.FormatInt(counter, 10)
			}
			width, _ := strconv.Atoi(m[1])
			return fmt.Sprintf("%0*d", width, counter)
		}
		switch tok {
		case "{year}":
			return strconv.Itoa(date.Year())
		case "{y}":
			return fmt.Sprintf("%02d", date.Year()%100)
		case "{month}":
			return fmt.Sprintf("%02d", int(date.Month()))
		case "{m}":
			return date.Format("Jan")
		case "{day}":
			return fmt.Sprintf("%02d", date.Day())
		case "{clientCode}":
			return clientCode
		default:
			return tok
		}
	})
	return result
}

// DocumentNumberSetting is what the Settings UI reads/writes — Format is
// always populated (the type's default when no row exists yet), Counter is
// the count of documents of this type generated so far (0 for a never-used
// type), and HasOverride reports whether an organization has actually saved
// a row, distinguishing "using the default" from "explicitly set to the
// same value as the default" the same way GetDocumentTemplateOrientation's
// ""-means-no-override return distinguishes those two states.
type DocumentNumberSetting struct {
	DocumentType string `json:"documentType"`
	Format       string `json:"format"`
	Counter      int64  `json:"counter"`
	HasOverride  bool   `json:"hasOverride"`
}

// IsKnownDocumentNumberType reports whether documentType is one of the five
// types this table covers.
func IsKnownDocumentNumberType(documentType string) bool {
	_, ok := documentNumberDefaults[documentType]
	return ok
}

// GetDocumentNumberSetting returns the org's configured format/counter for
// documentType, or the type's default format with a zero counter if the
// organization has never saved one — "absence means default," the same
// convention resolveTemplateBytes/GetDocumentTemplateOrientation both use.
func (d *Database) GetDocumentNumberSetting(organizationID, documentType string) (*DocumentNumberSetting, error) {
	defaultFormat, ok := documentNumberDefaults[documentType]
	if !ok {
		return nil, newValidationError("unknown document type %q", documentType)
	}
	var row struct {
		Format  string `db:"format"`
		Counter int64  `db:"counter"`
	}
	err := d.DB.Get(&row, `
		SELECT format, counter FROM document_number_settings
		WHERE organizationId = ? AND documentType = ?`,
		organizationID, documentType,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return &DocumentNumberSetting{DocumentType: documentType, Format: defaultFormat, Counter: 0}, nil
	}
	if err != nil {
		// A real query failure must surface, not masquerade as "no row yet":
		// swallowing it would make a transient DB error look like an
		// unconfigured type and silently hand callers the default counter.
		return nil, fmt.Errorf("get_document_number_setting: %w", err)
	}
	return &DocumentNumberSetting{
		DocumentType: documentType, Format: row.Format, Counter: row.Counter, HasOverride: true,
	}, nil
}

// UpdateDocumentNumberSettingRequest mirrors the omitted-means-keep
// convention UpdateOrganization's nullable fields use — Counter is a
// pointer so the Settings UI can update the format alone (the common case)
// without also having to know or resend the current counter.
//
// The distinction is load-bearing. A nil Counter means "leave the counter
// exactly as it is" and is written as a no-op in the upsert's DO UPDATE
// clause, so a format-only save can never rewind a counter that a concurrent
// GenerateNextDocumentNumberTx advanced in between this request being sent
// and being applied. A non-nil Counter is an explicit, deliberate set and is
// the only path that writes the counter column directly.
type UpdateDocumentNumberSettingRequest struct {
	Format  string `json:"format"`
	Counter *int64 `json:"counter"`
}

// UpdateDocumentNumberSetting validates and upserts an org's format/counter
// override for documentType.
//
// With req.Counter nil the statement never references the existing counter
// value at all — the ON CONFLICT branch touches only format/updatedAt, and
// the INSERT branch (first-ever save) seeds it to 0 — so there is no
// read-then-write gap for a concurrent GenerateNextDocumentNumberTx to be
// overwritten through. With req.Counter set the caller has explicitly asked
// to set the counter, so writing it is intended, not a stale-snapshot
// hazard.
func (d *Database) UpdateDocumentNumberSetting(organizationID, documentType string, req UpdateDocumentNumberSettingRequest) (*DocumentNumberSetting, error) {
	if !IsKnownDocumentNumberType(documentType) {
		return nil, newValidationError("unknown document type %q", documentType)
	}
	if err := validateDocumentNumberFormat(req.Format); err != nil {
		return nil, err
	}
	if req.Counter != nil && *req.Counter < 0 {
		return nil, newValidationError("counter must be 0 or greater")
	}

	var err error
	if req.Counter != nil {
		_, err = d.DB.Exec(`
			INSERT INTO document_number_settings (organizationId, documentType, format, counter, updatedAt)
			VALUES (?, ?, ?, ?, strftime('%s', 'now') * 1000)
			ON CONFLICT(organizationId, documentType) DO UPDATE SET
				format = excluded.format,
				counter = excluded.counter,
				updatedAt = excluded.updatedAt`,
			organizationID, documentType, req.Format, *req.Counter,
		)
	} else {
		// No counter in the statement: a format-only save can't rewind a
		// counter advanced concurrently by GenerateNextDocumentNumberTx.
		_, err = d.DB.Exec(`
			INSERT INTO document_number_settings (organizationId, documentType, format, counter, updatedAt)
			VALUES (?, ?, ?, 0, strftime('%s', 'now') * 1000)
			ON CONFLICT(organizationId, documentType) DO UPDATE SET
				format = excluded.format,
				updatedAt = excluded.updatedAt`,
			organizationID, documentType, req.Format,
		)
	}
	if err != nil {
		return nil, fmt.Errorf("update_document_number_setting: %w", err)
	}
	return d.GetDocumentNumberSetting(organizationID, documentType)
}

// PreviewNextDocumentNumber formats what the NEXT document of this type
// would be numbered, without consuming it — the read-only counterpart to
// GenerateNextDocumentNumberTx below, used by each document type's existing
// "next-number" prefill endpoint (nextPurchaseOrderNumber and siblings).
func (d *Database) PreviewNextDocumentNumber(organizationID, documentType string) (string, error) {
	setting, err := d.GetDocumentNumberSetting(organizationID, documentType)
	if err != nil {
		return "", err
	}
	return generateFormattedDocumentNumber(setting.Format, setting.Counter+1, time.Now(), ""), nil
}

// GenerateNextDocumentNumberTx atomically advances documentType's counter by
// one and returns the number generated from it, inside the caller's own
// transaction — the same "read+increment inside the document's own insert
// transaction" shape CreateInvoice already uses for invoice_number_counter,
// generalized to the other five document types via this shared table. Must
// run inside tx, never against d.DB directly: db.SetMaxOpenConns(1) means a
// second connection attempting this while tx is open would deadlock, not
// error (see CreateCashSale's doc comment for the same constraint).
//
// The counter always advances exactly once per document created, regardless
// of whether the caller ends up storing the generated number or a
// user-edited one — mirroring invoices' own existing behavior, where the
// counter increments unconditionally on every create. This keeps the
// "next" preview monotonically increasing even across edits/deletes, a
// deliberate change from the old MAX(...)+1 scan these five types used
// before (which silently reissued a deleted document's number).
func GenerateNextDocumentNumberTx(tx *sqlx.Tx, organizationID, documentType string, date time.Time, clientCode string) (string, error) {
	defaultFormat, ok := documentNumberDefaults[documentType]
	if !ok {
		return "", newValidationError("unknown document type %q", documentType)
	}
	var row struct {
		Format  string `db:"format"`
		Counter int64  `db:"counter"`
	}
	format := defaultFormat
	var nextCounter int64 = 1
	err := tx.Get(&row, `
		SELECT format, counter FROM document_number_settings
		WHERE organizationId = ? AND documentType = ?`,
		organizationID, documentType,
	)
	if err == nil {
		format = row.Format
		nextCounter = row.Counter + 1
	} else if !errors.Is(err, sql.ErrNoRows) {
		// Only "no row yet" (a never-used type, first counter of 1) is a
		// legitimate default. Any other failure must abort the caller's
		// transaction rather than silently starting over at 1 and reissuing
		// numbers — the caller's document insert shares this tx, so the
		// error propagating is what rolls the whole thing back.
		return "", fmt.Errorf("generate_next_document_number: %w", err)
	}
	_, err = tx.Exec(`
		INSERT INTO document_number_settings (organizationId, documentType, format, counter, updatedAt)
		VALUES (?, ?, ?, ?, strftime('%s', 'now') * 1000)
		ON CONFLICT(organizationId, documentType) DO UPDATE SET
			counter = excluded.counter,
			updatedAt = excluded.updatedAt`,
		organizationID, documentType, format, nextCounter,
	)
	if err != nil {
		return "", fmt.Errorf("generate_next_document_number: %w", err)
	}
	return generateFormattedDocumentNumber(format, nextCounter, date, clientCode), nil
}
