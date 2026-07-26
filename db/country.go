package db

import "fmt"

// GetActiveCountryCodes returns the ISO 3166-1 alpha-2 codes marked active,
// i.e. offered in the country picklist on organization/vendor/client forms.
// The full code list is static frontend data; this table only tracks the
// active subset.
func (d *Database) GetActiveCountryCodes() ([]string, error) {
	codes := []string{}
	err := d.DB.Select(&codes, `SELECT code FROM active_countries ORDER BY code ASC`)
	if err != nil {
		return nil, fmt.Errorf("get_active_country_codes: %w", err)
	}
	return codes, nil
}

// SetCountryActive activates or deactivates a single country code. It never
// gates country_code on client/vendor/organization records — deactivating a
// code only removes it from the picklist, it does not touch or validate
// existing records that already use it.
func (d *Database) SetCountryActive(code string, active bool) error {
	if active {
		_, err := d.DB.Exec(`INSERT INTO active_countries (code) VALUES (?) ON CONFLICT DO NOTHING`, code)
		if err != nil {
			return fmt.Errorf("set_country_active: %w", err)
		}
		return nil
	}
	_, err := d.DB.Exec(`DELETE FROM active_countries WHERE code = ?`, code)
	if err != nil {
		return fmt.Errorf("set_country_active: %w", err)
	}
	return nil
}
