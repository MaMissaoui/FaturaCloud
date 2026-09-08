package db

import "time"

// dateFormatLayouts maps organizations.date_format's stored values — raw
// dayjs tokens, per src/utils/date.ts's DATE_FORMATS — to Go's reference-time
// layout equivalents (YYYY->2006, MM->01, DD->02; punctuation unchanged).
// nil/"AUTO"/anything unrecognized falls back to defaultDateLayout, the same
// "never hard-fail on an unrecognized stored value" spirit as
// getInvoicePDFLayout's fallback to "default".
var dateFormatLayouts = map[string]string{
	"MM/DD/YYYY": "01/02/2006",
	"DD/MM/YYYY": "02/01/2006",
	"DD.MM.YYYY": "02.01.2006",
	"YYYY-MM-DD": "2006-01-02",
	"YYYY/MM/DD": "2006/01/02",
	"DD-MM-YYYY": "02-01-2006",
}

const defaultDateLayout = "2006-01-02"

// formatOrgDate renders a millisecond Unix timestamp using the
// organization's configured date_format, falling back to ISO 8601.
func formatOrgDate(unixMillis int64, dateFormat *string) string {
	layout := defaultDateLayout
	if dateFormat != nil {
		if l, ok := dateFormatLayouts[*dateFormat]; ok {
			layout = l
		}
	}
	return time.UnixMilli(unixMillis).UTC().Format(layout)
}

// formatOptionalOrgDate is formatOrgDate for a nullable timestamp (e.g.
// invoices.dueDate), returning "" when unset rather than formatting the
// Unix epoch.
func formatOptionalOrgDate(unixMillis *int64, dateFormat *string) string {
	if unixMillis == nil {
		return ""
	}
	return formatOrgDate(*unixMillis, dateFormat)
}
