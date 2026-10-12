package repo

import (
	"errors"
	"sort"
	"strings"
	"time"

	"github.com/Tencent/LoopForge/internal/domain"
)

// ---------- Users ----------

type userWithConfig struct {
	ID                           int64      `gorm:"column:id"`
	LoginName                    string     `gorm:"column:login_name"`
	DisplayName                  string     `gorm:"column:display_name"`
	PasswordHash                 string     `gorm:"column:password_hash"`
	PlatformRole                 string     `gorm:"column:platform_role"`
	PersonalMCPToken             *string    `gorm:"column:personal_mcp_token"`
	GitToken                     *string    `gorm:"column:git_token"`
	CodeBuddyAPIKey              *string    `gorm:"column:codebuddy_api_key"`
	CodeBuddyInternetEnvironment string     `gorm:"column:codebuddy_internet_environment"`
	CodexAPIKey                  *string    `gorm:"column:codex_api_key"`
	CodexBaseURL                 *string    `gorm:"column:codex_base_url"`
	ClaudeCodeAPIKey             *string    `gorm:"column:claude_code_api_key"`
	ClaudeCodeBaseURL            *string    `gorm:"column:claude_code_base_url"`
	Created                      *time.Time `gorm:"column:created_at"`
	Updated                      *time.Time `gorm:"column:updated_at"`
}

const userSelectColumns = `u.id, u.login_name, u.display_name, COALESCE(u.password_hash, '') AS password_hash,
	COALESCE(u.platform_role, '') AS platform_role,
	c.personal_mcp_token, c.git_token, c.codebuddy_api_key, COALESCE(c.codebuddy_internet_environment, '') AS codebuddy_internet_environment,
	c.codex_api_key, c.codex_base_url, c.claude_code_api_key, c.claude_code_base_url,
	u.created_at, u.updated_at`

func (r userWithConfig) toDomain() domain.User {
	u := domain.User{
		ID: r.ID, LoginName: r.LoginName, DisplayName: r.DisplayName,
		PasswordHash: r.PasswordHash, PlatformRole: r.PlatformRole,
		CodeBuddyInternetEnvironment: r.CodeBuddyInternetEnvironment,
	}
	if r.PersonalMCPToken != nil {
		u.PersonalMCPToken = *r.PersonalMCPToken
	}
	if r.GitToken != nil {
		u.GitToken = *r.GitToken
	}
	if r.CodeBuddyAPIKey != nil {
		u.CodeBuddyAPIKey = *r.CodeBuddyAPIKey
	}
	if r.CodexAPIKey != nil {
		u.CodexAPIKey = *r.CodexAPIKey
	}
	if r.CodexBaseURL != nil {
		u.CodexBaseURL = *r.CodexBaseURL
	}
	if r.ClaudeCodeAPIKey != nil {
		u.ClaudeCodeAPIKey = *r.ClaudeCodeAPIKey
	}
	if r.ClaudeCodeBaseURL != nil {
		u.ClaudeCodeBaseURL = *r.ClaudeCodeBaseURL
	}
	if r.Created != nil {
		u.CreatedAt = *r.Created
	}
	if r.Updated != nil {
		u.UpdatedAt = *r.Updated
	}
	return u
}

func (r *UsersRepo) userQuery(where string, args ...any) (domain.User, bool) {
	var row userWithConfig
	q := r.DB.Raw(`SELECT `+userSelectColumns+` FROM users u LEFT JOIN user_configs c ON c.user_id=u.id WHERE `+where, args...)
	if err := q.Scan(&row).Error; err != nil || row.ID == 0 {
		return domain.User{}, false
	}
	return row.toDomain(), true
}

func (r *UsersRepo) Users() []domain.User {
	var rows []userWithConfig
	if err := r.DB.Raw(`SELECT ` + userSelectColumns + ` FROM users u LEFT JOIN user_configs c ON c.user_id=u.id ORDER BY u.login_name ASC`).Scan(&rows).Error; err != nil {
		return []domain.User{}
	}
	out := make([]domain.User, 0, len(rows))
	for _, row := range rows {
		out = append(out, row.toDomain())
	}
	sort.Slice(out, func(i, j int) bool { return out[i].LoginName < out[j].LoginName })
	return out
}

func (r *UsersRepo) User(id int64) (domain.User, bool) {
	if id <= 0 {
		return domain.User{}, false
	}
	return r.userQuery("u.id=?", id)
}

func (r *UsersRepo) UserByLoginName(loginName string) (domain.User, bool) {
	if r.isMySQL() {
		// MySQL collation is already case-insensitive and keeps index usage.
		return r.userQuery("u.login_name=?", loginName)
	}
	return r.userQuery("LOWER(u.login_name) = LOWER(?)", loginName)
}

func (r *UsersRepo) PutUser(v domain.User) error {
	if v.LoginName == "" && v.ID == 0 {
		return errors.New("user loginName or ID is required")
	}
	now := time.Now().UTC()
	var old domain.User
	var ok bool
	if v.ID > 0 {
		old, ok = r.User(v.ID)
	} else if v.LoginName != "" {
		old, ok = r.UserByLoginName(v.LoginName)
	}
	if ok {
		if v.ID == 0 {
			v.ID = old.ID
		}
		v.CreatedAt = old.CreatedAt
		// Authentication refreshes identity fields only. Credentials are managed
		// through UpdateUserCredentials and must survive every login refresh.
		v.PersonalMCPToken = old.PersonalMCPToken
		v.GitToken = old.GitToken
		v.CodexAPIKey, v.CodexBaseURL = old.CodexAPIKey, old.CodexBaseURL
		v.ClaudeCodeAPIKey, v.ClaudeCodeBaseURL = old.ClaudeCodeAPIKey, old.ClaudeCodeBaseURL
		v.CodeBuddyAPIKey = old.CodeBuddyAPIKey
		v.CodeBuddyInternetEnvironment = old.CodeBuddyInternetEnvironment
		if strings.TrimSpace(v.PlatformRole) == "" {
			v.PlatformRole = old.PlatformRole
		}
		if v.PasswordHash == "" {
			v.PasswordHash = old.PasswordHash
		}
	} else {
		v.CreatedAt = now
	}
	if IsAllowlistedPlatformAdmin(v.LoginName) {
		v.PlatformRole = domain.PlatformRoleAdmin
	}
	v.UpdatedAt = now
	saved, err := r.writeUser(v)
	if err != nil {
		return err
	}
	if !ok && userHasConfig(saved) {
		if err := r.writeUserConfig(saved); err != nil {
			return err
		}
	}
	return nil
}

func (r *UsersRepo) writeUser(v domain.User) (domain.User, error) {
	if v.ID > 0 {
		err := upsert(r.DB, "users", map[string]any{
			"id": v.ID, "login_name": v.LoginName, "display_name": v.DisplayName,
			"password_hash": v.PasswordHash, "platform_role": v.PlatformRole,
			"created_at": timePtr(v.CreatedAt), "updated_at": timePtr(v.UpdatedAt),
		})
		return v, err
	}
	inserted, err := insertIgnore(r.DB, "users", map[string]any{
		"login_name": v.LoginName, "display_name": v.DisplayName,
		"password_hash": v.PasswordHash, "platform_role": v.PlatformRole,
		"created_at": timePtr(v.CreatedAt), "updated_at": timePtr(v.UpdatedAt),
	})
	if err != nil {
		return domain.User{}, err
	}
	if !inserted {
		// The unique login already exists: adopt that row and refresh it.
		var existing userRow
		if err := r.DB.Where("login_name = ?", v.LoginName).Take(&existing).Error; err != nil {
			return domain.User{}, err
		}
		if err := r.DB.Model(&userRow{}).Where("id = ?", existing.ID).Updates(map[string]any{
			"display_name": v.DisplayName, "password_hash": v.PasswordHash,
			"platform_role": v.PlatformRole, "updated_at": timePtr(v.UpdatedAt),
		}).Error; err != nil {
			return domain.User{}, err
		}
		v.ID = existing.ID
		return v, nil
	}
	var row userRow
	if err := r.DB.Where("login_name = ?", v.LoginName).Take(&row).Error; err != nil {
		return domain.User{}, err
	}
	v.ID = row.ID
	return v, nil
}

// SetUserPassword stores a bcrypt hash; an empty hash disables password sign-in.
func (r *UsersRepo) SetUserPassword(id int64, passwordHash string) error {
	return r.DB.Model(&userRow{}).Where("id = ?", id).Updates(map[string]any{
		"password_hash": passwordHash, "updated_at": timePtr(time.Now().UTC()),
	}).Error
}

func userHasConfig(user domain.User) bool {
	return user.PersonalMCPToken != "" ||
		user.GitToken != "" ||
		user.CodeBuddyAPIKey != "" ||
		user.CodeBuddyInternetEnvironment != "" ||
		user.CodexAPIKey != "" ||
		user.CodexBaseURL != "" ||
		user.ClaudeCodeAPIKey != "" ||
		user.ClaudeCodeBaseURL != ""
}

type UserCredentialUpdate struct {
	PersonalMCPToken             string  `json:"personalMcpToken"`
	PersonalMCPTokenClear        bool    `json:"personalMcpTokenClear"`
	GitToken                     string  `json:"gitToken"`
	GitTokenClear                bool    `json:"gitTokenClear"`
	CodeBuddyAPIKey              string  `json:"codeBuddyApiKey"`
	CodeBuddyAPIKeyClear         bool    `json:"codeBuddyApiKeyClear"`
	CodeBuddyInternetEnvironment *string `json:"codeBuddyInternetEnvironment"`
	CodexAPIKey                  string  `json:"codexApiKey"`
	CodexAPIKeyClear             bool    `json:"codexApiKeyClear"`
	CodexBaseURL                 *string `json:"codexBaseUrl"`
	ClaudeCodeAPIKey             string  `json:"claudeCodeApiKey"`
	ClaudeCodeAPIKeyClear        bool    `json:"claudeCodeApiKeyClear"`
	ClaudeCodeBaseURL            *string `json:"claudeCodeBaseUrl"`
}

func (r *UsersRepo) UpdateUserCredentials(id int64, update UserCredentialUpdate) (domain.User, error) {
	user, ok := r.User(id)
	if !ok {
		return domain.User{}, errors.New("user not found")
	}
	if update.PersonalMCPTokenClear {
		user.PersonalMCPToken = ""
	} else if value := strings.TrimSpace(update.PersonalMCPToken); value != "" {
		if len(value) >= 7 && strings.EqualFold(value[:7], "Bearer ") {
			value = strings.TrimSpace(value[7:])
		}
		user.PersonalMCPToken = value
	}
	if update.GitTokenClear {
		user.GitToken = ""
	} else if value := strings.TrimSpace(update.GitToken); value != "" {
		user.GitToken = value
	}
	if update.CodeBuddyAPIKeyClear {
		user.CodeBuddyAPIKey = ""
	} else if value := strings.TrimSpace(update.CodeBuddyAPIKey); value != "" {
		user.CodeBuddyAPIKey = value
	}
	if update.CodeBuddyInternetEnvironment != nil {
		user.CodeBuddyInternetEnvironment = strings.TrimSpace(*update.CodeBuddyInternetEnvironment)
	}
	if update.CodexAPIKeyClear {
		user.CodexAPIKey = ""
	} else if value := strings.TrimSpace(update.CodexAPIKey); value != "" {
		user.CodexAPIKey = value
	}
	if update.ClaudeCodeAPIKeyClear {
		user.ClaudeCodeAPIKey = ""
	} else if value := strings.TrimSpace(update.ClaudeCodeAPIKey); value != "" {
		user.ClaudeCodeAPIKey = value
	}
	if update.CodexBaseURL != nil {
		user.CodexBaseURL = strings.TrimSpace(*update.CodexBaseURL)
	}
	if update.ClaudeCodeBaseURL != nil {
		user.ClaudeCodeBaseURL = strings.TrimSpace(*update.ClaudeCodeBaseURL)
	}
	user.UpdatedAt = time.Now().UTC()
	if err := r.writeUserConfig(user); err != nil {
		return domain.User{}, err
	}
	return user, nil
}

func (r *UsersRepo) writeUserConfig(v domain.User) error {
	return upsert(r.DB, "user_configs", map[string]any{
		"user_id":            v.ID,
		"personal_mcp_token": nullableString(v.PersonalMCPToken), "git_token": nullableString(v.GitToken),
		"codebuddy_api_key":              nullableString(v.CodeBuddyAPIKey),
		"codebuddy_internet_environment": v.CodeBuddyInternetEnvironment,
		"codex_api_key":                  nullableString(v.CodexAPIKey), "codex_base_url": nullableString(v.CodexBaseURL),
		"claude_code_api_key": nullableString(v.ClaudeCodeAPIKey), "claude_code_base_url": nullableString(v.ClaudeCodeBaseURL),
		"created_at": timePtr(v.CreatedAt), "updated_at": timePtr(v.UpdatedAt),
	})
}

func nullableString(v string) any {
	if v == "" {
		return nil
	}
	return v
}
