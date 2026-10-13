package repo

import (
	"encoding/json"
	"os"
	"strings"
	"time"

	"gorm.io/gorm/clause"
)

const authSettingsID = "default"

// AuthSettings are the platform-wide sign-in rules.
type AuthSettings struct {
	// SignupOpen lets anyone who reaches the instance create an account.
	SignupOpen bool `json:"signupOpen"`
}

// DefaultSignupOpen is the rule before an admin saves one:
// LOOPFORGE_SIGNUP=closed starts closed, anything else open.
func DefaultSignupOpen() bool {
	return !strings.EqualFold(strings.TrimSpace(os.Getenv("LOOPFORGE_SIGNUP")), "closed")
}

func (r *UsersRepo) AuthSettings() AuthSettings {
	var row authSettingsRow
	if err := r.DB.Where("id = ?", authSettingsID).Take(&row).Error; err != nil {
		return AuthSettings{SignupOpen: DefaultSignupOpen()}
	}
	var out AuthSettings
	if json.Unmarshal(row.Payload, &out) != nil {
		return AuthSettings{SignupOpen: DefaultSignupOpen()}
	}
	return out
}

func (r *UsersRepo) PutAuthSettings(settings AuthSettings) error {
	payload, err := json.Marshal(settings)
	if err != nil {
		return err
	}
	now := time.Now().UTC()
	return r.DB.Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "id"}},
		DoUpdates: clause.AssignmentColumns([]string{"payload", "updated_at"}),
	}).Create(&authSettingsRow{ID: authSettingsID, Payload: payload, Updated: &now}).Error
}

// HasPasswordAccount reports whether anyone can sign in yet. Until then the
// first visitor creates the admin account.
func (r *UsersRepo) HasPasswordAccount() bool {
	var n int64
	r.DB.Model(&userRow{}).Where("password_hash IS NOT NULL AND password_hash <> ''").Count(&n)
	return n > 0
}
