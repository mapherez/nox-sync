package storage

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net/mail"
	"strings"
	"time"
)

const (
	userIDPrefix    = "user_"
	sessionIDPrefix = "sess_"
	oauthIDPrefix   = "oauth_"
	oauthStateTTL   = 10 * time.Minute
)

// BootstrapAdmins ensures environment-defined admins exist and remain active admins.
func (s *Store) BootstrapAdmins(ctx context.Context, emails []string) error {
	for _, rawEmail := range emails {
		email, err := normalizeEmail(rawEmail)
		if err != nil {
			return err
		}
		if email == "" {
			continue
		}
		if _, err := s.UpsertAllowedUser(ctx, email, UserRoleAdmin); err != nil {
			return err
		}
	}
	return nil
}

// UpsertAllowedUser adds or re-enables an allowlisted user.
func (s *Store) UpsertAllowedUser(ctx context.Context, email string, role string) (User, error) {
	email, err := normalizeEmail(email)
	if err != nil {
		return User{}, err
	}
	if email == "" {
		return User{}, fmt.Errorf("%w: email is required", ErrBadRequest)
	}
	role = normalizeRole(role)

	userID, err := randomID(userIDPrefix)
	if err != nil {
		return User{}, err
	}
	now := timestamp(time.Now())
	if _, err := s.db.ExecContext(ctx, `
		INSERT INTO users (id, email, first_name, display_name, role, status, created_at, updated_at)
		VALUES (?, ?, '', ?, ?, ?, ?, ?)
		ON CONFLICT(email) DO UPDATE SET
			role = excluded.role,
			status = excluded.status,
			updated_at = excluded.updated_at
	`, userID, email, email, role, UserStatusActive, now, now); err != nil {
		return User{}, fmt.Errorf("upsert allowed user: %w", err)
	}

	user, err := s.UserByEmail(ctx, email)
	if err != nil {
		return User{}, err
	}
	if _, err := s.CurrentAPIKey(ctx, user.ID); err != nil {
		return User{}, err
	}
	return user, nil
}

// CompleteGoogleLogin binds a verified Google identity to an allowlisted user.
func (s *Store) CompleteGoogleLogin(ctx context.Context, profile OAuthProfile) (User, error) {
	profile.Email, _ = normalizeEmail(profile.Email)
	profile.Sub = strings.TrimSpace(profile.Sub)
	profile.FirstName = strings.TrimSpace(profile.FirstName)
	profile.DisplayName = strings.TrimSpace(profile.DisplayName)
	if profile.Sub == "" || profile.Email == "" {
		return User{}, fmt.Errorf("%w: Google identity is missing sub or email", ErrBadRequest)
	}
	if profile.DisplayName == "" {
		profile.DisplayName = profile.Email
	}
	if profile.FirstName == "" {
		profile.FirstName = firstNameFallback(profile.DisplayName, profile.Email)
	}

	now := timestamp(time.Now())
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return User{}, fmt.Errorf("begin login transaction: %w", err)
	}
	defer rollback(tx)

	user, err := userByGoogleSubTx(ctx, tx, profile.Sub)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return User{}, err
	}

	if errors.Is(err, sql.ErrNoRows) {
		user, err = userByEmailTx(ctx, tx, profile.Email)
		if err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return User{}, ErrForbidden
			}
			return User{}, err
		}
	}

	if user.Status != UserStatusActive {
		return User{}, ErrForbidden
	}
	if user.GoogleSub != "" && user.GoogleSub != profile.Sub {
		return User{}, ErrForbidden
	}

	if _, err := tx.ExecContext(ctx, `
		UPDATE users
		SET google_sub = ?, email = ?, first_name = ?, display_name = ?, updated_at = ?, last_login_at = ?
		WHERE id = ?
	`, profile.Sub, profile.Email, profile.FirstName, profile.DisplayName, now, now, user.ID); err != nil {
		return User{}, fmt.Errorf("update Google user: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return User{}, fmt.Errorf("commit login transaction: %w", err)
	}

	if _, err := s.CurrentAPIKey(ctx, user.ID); err != nil {
		return User{}, err
	}
	return s.UserByID(ctx, user.ID)
}

// UserByID loads one user.
func (s *Store) UserByID(ctx context.Context, userID string) (User, error) {
	var user User
	if err := s.db.QueryRowContext(ctx, userSelectSQL()+` WHERE id = ?`, strings.TrimSpace(userID)).Scan(userScanDest(&user)...); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return User{}, ErrNotFound
		}
		return User{}, fmt.Errorf("load user: %w", err)
	}
	return user, nil
}

// UserByEmail loads one user by normalized email.
func (s *Store) UserByEmail(ctx context.Context, email string) (User, error) {
	email, err := normalizeEmail(email)
	if err != nil {
		return User{}, err
	}
	var user User
	if err := s.db.QueryRowContext(ctx, userSelectSQL()+` WHERE email = ?`, email).Scan(userScanDest(&user)...); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return User{}, ErrNotFound
		}
		return User{}, fmt.Errorf("load user by email: %w", err)
	}
	return user, nil
}

// ListUsers returns all allowlisted users for the admin UI.
func (s *Store) ListUsers(ctx context.Context) ([]User, error) {
	rows, err := s.db.QueryContext(ctx, userSelectSQL()+` ORDER BY role, email`)
	if err != nil {
		return nil, fmt.Errorf("list users: %w", err)
	}
	defer rows.Close()

	users := []User{}
	for rows.Next() {
		var user User
		if err := rows.Scan(userScanDest(&user)...); err != nil {
			return nil, fmt.Errorf("scan user: %w", err)
		}
		users = append(users, user)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate users: %w", err)
	}
	return users, nil
}

// SetUserStatus enables or disables a user.
func (s *Store) SetUserStatus(ctx context.Context, userID string, status string) error {
	userID = strings.TrimSpace(userID)
	status = strings.ToUpper(strings.TrimSpace(status))
	if userID == "" || (status != UserStatusActive && status != UserStatusDisabled) {
		return fmt.Errorf("%w: valid userId and status are required", ErrBadRequest)
	}
	if _, err := s.db.ExecContext(ctx, `
		UPDATE users
		SET status = ?, updated_at = ?
		WHERE id = ?
	`, status, timestamp(time.Now()), userID); err != nil {
		return fmt.Errorf("set user status: %w", err)
	}
	return nil
}

// SetUserRole changes a user's dashboard role.
func (s *Store) SetUserRole(ctx context.Context, userID string, role string) error {
	userID = strings.TrimSpace(userID)
	role = normalizeRole(role)
	if userID == "" {
		return fmt.Errorf("%w: userId is required", ErrBadRequest)
	}
	if _, err := s.db.ExecContext(ctx, `
		UPDATE users
		SET role = ?, updated_at = ?
		WHERE id = ?
	`, role, timestamp(time.Now()), userID); err != nil {
		return fmt.Errorf("set user role: %w", err)
	}
	return nil
}

// CreateOAuthState stores a short-lived OAuth state token and returns the raw token.
func (s *Store) CreateOAuthState(ctx context.Context, redirectTo string) (string, error) {
	state, err := randomID("state_")
	if err != nil {
		return "", err
	}
	id, err := randomID(oauthIDPrefix)
	if err != nil {
		return "", err
	}
	redirectTo = safeRedirectPath(redirectTo)
	now := time.Now().UTC()
	if _, err := s.db.ExecContext(ctx, `
		INSERT INTO oauth_states (id, state_hash, redirect_to, expires_at, created_at)
		VALUES (?, ?, ?, ?, ?)
	`, id, hashSecret(state), redirectTo, timestamp(now.Add(oauthStateTTL)), timestamp(now)); err != nil {
		return "", fmt.Errorf("create OAuth state: %w", err)
	}
	return state, nil
}

// ConsumeOAuthState validates and removes one OAuth state token.
func (s *Store) ConsumeOAuthState(ctx context.Context, state string) (string, error) {
	state = strings.TrimSpace(state)
	if state == "" {
		return "", ErrForbidden
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return "", fmt.Errorf("begin OAuth state transaction: %w", err)
	}
	defer rollback(tx)

	var redirectTo string
	var expiresAt string
	if err := tx.QueryRowContext(ctx, `
		SELECT redirect_to, expires_at
		FROM oauth_states
		WHERE state_hash = ?
	`, hashSecret(state)).Scan(&redirectTo, &expiresAt); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return "", ErrForbidden
		}
		return "", fmt.Errorf("load OAuth state: %w", err)
	}

	if expired, err := isExpired(expiresAt, time.Now().UTC()); err != nil || expired {
		_, _ = tx.ExecContext(ctx, `DELETE FROM oauth_states WHERE state_hash = ?`, hashSecret(state))
		return "", ErrForbidden
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM oauth_states WHERE state_hash = ?`, hashSecret(state)); err != nil {
		return "", fmt.Errorf("delete OAuth state: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return "", fmt.Errorf("commit OAuth state transaction: %w", err)
	}
	return redirectTo, nil
}

// CreateWebSession creates a server-side session and returns the opaque cookie token.
func (s *Store) CreateWebSession(ctx context.Context, userID string, duration time.Duration) (string, error) {
	userID = strings.TrimSpace(userID)
	if userID == "" {
		return "", fmt.Errorf("%w: userId is required", ErrBadRequest)
	}
	token, err := randomID(sessionIDPrefix)
	if err != nil {
		return "", err
	}
	id, err := randomID("websess_")
	if err != nil {
		return "", err
	}
	now := time.Now().UTC()
	if _, err := s.db.ExecContext(ctx, `
		INSERT INTO web_sessions (id, user_id, token_hash, expires_at, created_at)
		VALUES (?, ?, ?, ?, ?)
	`, id, userID, hashSecret(token), timestamp(now.Add(duration)), timestamp(now)); err != nil {
		return "", fmt.Errorf("create web session: %w", err)
	}
	return token, nil
}

// UserBySessionToken resolves a web session cookie to an active user.
func (s *Store) UserBySessionToken(ctx context.Context, token string) (User, bool, error) {
	token = strings.TrimSpace(token)
	if token == "" {
		return User{}, false, nil
	}

	var user User
	var expiresAt string
	if err := s.db.QueryRowContext(ctx, `
		SELECT
			users.id,
			COALESCE(users.google_sub, ''),
			users.email,
			users.first_name,
			users.display_name,
			users.role,
			users.status,
			COALESCE(users.last_login_at, ''),
			ws.expires_at
		FROM users
		JOIN web_sessions ws ON ws.user_id = users.id
		WHERE ws.token_hash = ?
	`, hashSecret(token)).Scan(append(userScanDest(&user), &expiresAt)...); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return User{}, false, nil
		}
		return User{}, false, fmt.Errorf("load web session: %w", err)
	}
	if expired, err := isExpired(expiresAt, time.Now().UTC()); err != nil || expired || user.Status != UserStatusActive {
		_ = s.DeleteWebSession(ctx, token)
		return User{}, false, nil
	}
	return user, true, nil
}

// DeleteWebSession removes a session token.
func (s *Store) DeleteWebSession(ctx context.Context, token string) error {
	token = strings.TrimSpace(token)
	if token == "" {
		return nil
	}
	if _, err := s.db.ExecContext(ctx, `DELETE FROM web_sessions WHERE token_hash = ?`, hashSecret(token)); err != nil {
		return fmt.Errorf("delete web session: %w", err)
	}
	return nil
}

func userSelectSQL() string {
	return `
		SELECT
			users.id,
			COALESCE(users.google_sub, ''),
			users.email,
			users.first_name,
			users.display_name,
			users.role,
			users.status,
			COALESCE(users.last_login_at, '')
		FROM users`
}

func userScanDest(user *User) []any {
	return []any{
		&user.ID,
		&user.GoogleSub,
		&user.Email,
		&user.FirstName,
		&user.DisplayName,
		&user.Role,
		&user.Status,
		&user.LastLoginAt,
	}
}

func userByGoogleSubTx(ctx context.Context, tx *sql.Tx, sub string) (User, error) {
	var user User
	err := tx.QueryRowContext(ctx, userSelectSQL()+` WHERE google_sub = ?`, sub).Scan(userScanDest(&user)...)
	return user, err
}

func userByEmailTx(ctx context.Context, tx *sql.Tx, email string) (User, error) {
	var user User
	err := tx.QueryRowContext(ctx, userSelectSQL()+` WHERE email = ?`, email).Scan(userScanDest(&user)...)
	return user, err
}

func normalizeEmail(value string) (string, error) {
	value = strings.ToLower(strings.TrimSpace(value))
	if value == "" {
		return "", nil
	}
	parsed, err := mail.ParseAddress(value)
	if err != nil || strings.ToLower(strings.TrimSpace(parsed.Address)) != value {
		return "", fmt.Errorf("%w: valid email is required", ErrBadRequest)
	}
	return value, nil
}

func normalizeRole(role string) string {
	role = strings.ToUpper(strings.TrimSpace(role))
	if role == UserRoleAdmin {
		return UserRoleAdmin
	}
	return UserRoleUser
}

func firstNameFallback(displayName string, email string) string {
	parts := strings.Fields(displayName)
	if len(parts) > 0 {
		return parts[0]
	}
	if idx := strings.Index(email, "@"); idx > 0 {
		return email[:idx]
	}
	return email
}

func safeRedirectPath(value string) string {
	value = strings.TrimSpace(value)
	if value == "" || !strings.HasPrefix(value, "/") || strings.HasPrefix(value, "//") {
		return "/vault-dashboard"
	}
	return value
}

func hashSecret(value string) string {
	return hashAPIKey(value)
}
