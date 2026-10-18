package iam

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

var (
	ErrNotFound  = errors.New("resource not found")
	ErrConflict  = errors.New("resource conflict")
	ErrLastAdmin = errors.New("cannot remove the last administrator")
)

const adminRoleKey = "admin"

type Repository struct {
	db *pgxpool.Pool
}

func NewRepository(db *pgxpool.Pool) *Repository {
	return &Repository{db: db}
}

func clampPagination(page, pageSize int) (int, int) {
	if page < 1 {
		page = 1
	}
	if pageSize < 1 || pageSize > 100 {
		pageSize = 20
	}
	return page, pageSize
}

func isUniqueViolation(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == "23505"
}

func (r *Repository) GetUserAccess(ctx context.Context, userID uuid.UUID) (*UserAccess, error) {
	access := &UserAccess{
		UserID: userID,
		Roles:  make([]string, 0),
	}

	queryUser := `
		SELECT u.canonical_email, COALESCE(p.display_name, ''), COALESCE(p.avatar_url, '')
		FROM users u
		LEFT JOIN profiles p ON p.user_id = u.id
		WHERE u.id = $1
	`
	var email sql.NullString
	err := r.db.QueryRow(ctx, queryUser, userID).Scan(&email, &access.DisplayName, &access.AvatarURL)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("query user: %w", err)
	}
	if email.Valid {
		access.Email = &email.String
	}

	queryRoles := `
		SELECT r.key
		FROM user_roles ur
		JOIN roles r ON r.id = ur.role_id
		WHERE ur.user_id = $1
	`
	rows, err := r.db.Query(ctx, queryRoles, userID)
	if err != nil {
		return nil, fmt.Errorf("query user roles: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var roleKey string
		if err := rows.Scan(&roleKey); err != nil {
			return nil, fmt.Errorf("scan user role: %w", err)
		}
		access.Roles = append(access.Roles, roleKey)
		if roleKey == adminRoleKey {
			access.IsAdmin = true
		}
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate user roles: %w", err)
	}

	return access, nil
}

func (r *Repository) ListUsers(ctx context.Context, page, pageSize int, search, role string) ([]UserSummary, int64, error) {
	page, pageSize = clampPagination(page, pageSize)
	offset := (page - 1) * pageSize

	var conditions []string
	var args []any
	argIndex := 1

	if search != "" {
		conditions = append(conditions, fmt.Sprintf("(u.canonical_email ILIKE $%d OR p.display_name ILIKE $%d)", argIndex, argIndex))
		args = append(args, "%"+search+"%")
		argIndex++
	}
	if role != "" {
		conditions = append(conditions, fmt.Sprintf(`EXISTS (
			SELECT 1 FROM user_roles ur
			JOIN roles r ON r.id = ur.role_id
			WHERE ur.user_id = u.id AND r.key = $%d
		)`, argIndex))
		args = append(args, role)
		argIndex++
	}

	whereClause := ""
	if len(conditions) > 0 {
		whereClause = "WHERE " + strings.Join(conditions, " AND ")
	}

	countQuery := fmt.Sprintf(`
		SELECT COUNT(*)
		FROM users u
		LEFT JOIN profiles p ON p.user_id = u.id
		%s
	`, whereClause)

	var total int64
	if err := r.db.QueryRow(ctx, countQuery, args...).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("count users: %w", err)
	}

	selectQuery := fmt.Sprintf(`
		SELECT u.id, u.canonical_email, COALESCE(p.display_name, ''), COALESCE(p.avatar_url, '')
		FROM users u
		LEFT JOIN profiles p ON p.user_id = u.id
		%s
		ORDER BY u.id ASC
		LIMIT $%d OFFSET $%d
	`, whereClause, argIndex, argIndex+1)

	args = append(args, pageSize, offset)

	rows, err := r.db.Query(ctx, selectQuery, args...)
	if err != nil {
		return nil, 0, fmt.Errorf("query users: %w", err)
	}
	defer rows.Close()

	users := make([]UserSummary, 0)
	var ids []uuid.UUID
	for rows.Next() {
		var u UserSummary
		var email sql.NullString
		if err := rows.Scan(&u.ID, &email, &u.DisplayName, &u.AvatarURL); err != nil {
			return nil, 0, fmt.Errorf("scan user: %w", err)
		}
		if email.Valid {
			u.Email = &email.String
		}
		u.Roles = make([]string, 0)
		users = append(users, u)
		ids = append(ids, u.ID)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("iterate users: %w", err)
	}

	if len(ids) == 0 {
		return users, total, nil
	}

	roleRows, err := r.db.Query(ctx, `
		SELECT ur.user_id, r.key
		FROM user_roles ur
		JOIN roles r ON r.id = ur.role_id
		WHERE ur.user_id = ANY($1)
	`, ids)
	if err != nil {
		return nil, 0, fmt.Errorf("query page user roles: %w", err)
	}
	defer roleRows.Close()

	rolesByUser := make(map[uuid.UUID][]string, len(ids))
	for roleRows.Next() {
		var userID uuid.UUID
		var roleKey string
		if err := roleRows.Scan(&userID, &roleKey); err != nil {
			return nil, 0, fmt.Errorf("scan page user role: %w", err)
		}
		rolesByUser[userID] = append(rolesByUser[userID], roleKey)
	}
	if err := roleRows.Err(); err != nil {
		return nil, 0, fmt.Errorf("iterate page user roles: %w", err)
	}

	for i := range users {
		if roles, ok := rolesByUser[users[i].ID]; ok {
			users[i].Roles = roles
		}
	}

	return users, total, nil
}

func (r *Repository) GetUser(ctx context.Context, userID uuid.UUID) (*UserDetail, error) {
	query := `
		SELECT u.id, u.canonical_email, COALESCE(p.display_name, ''), COALESCE(p.avatar_url, '')
		FROM users u
		LEFT JOIN profiles p ON p.user_id = u.id
		WHERE u.id = $1
	`
	var detail UserDetail
	var email sql.NullString
	err := r.db.QueryRow(ctx, query, userID).Scan(&detail.ID, &email, &detail.DisplayName, &detail.AvatarURL)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("query user detail: %w", err)
	}
	if email.Valid {
		detail.Email = &email.String
	}

	roleRows, err := r.db.Query(ctx, `
		SELECT r.key
		FROM user_roles ur
		JOIN roles r ON r.id = ur.role_id
		WHERE ur.user_id = $1
	`, userID)
	if err != nil {
		return nil, fmt.Errorf("query user roles: %w", err)
	}
	defer roleRows.Close()

	detail.Roles = make([]string, 0)
	for roleRows.Next() {
		var roleKey string
		if err := roleRows.Scan(&roleKey); err != nil {
			return nil, fmt.Errorf("scan user role: %w", err)
		}
		detail.Roles = append(detail.Roles, roleKey)
	}
	if err := roleRows.Err(); err != nil {
		return nil, fmt.Errorf("iterate user roles: %w", err)
	}

	idenRows, err := r.db.Query(ctx, `
		SELECT id, user_id, provider, provider_subject, email, email_verified,
		       COALESCE(display_name, ''), COALESCE(avatar_url, ''), created_at
		FROM external_identities
		WHERE user_id = $1
		ORDER BY created_at ASC
	`, userID)
	if err != nil {
		return nil, fmt.Errorf("query identities: %w", err)
	}
	defer idenRows.Close()

	detail.Identities = make([]IdentitySummary, 0)
	for idenRows.Next() {
		var iden IdentitySummary
		var idenEmail sql.NullString
		if err := idenRows.Scan(
			&iden.ID, &iden.UserID, &iden.Provider, &iden.ProviderSubject, &idenEmail,
			&iden.EmailVerified, &iden.DisplayName, &iden.AvatarURL, &iden.CreatedAt,
		); err != nil {
			return nil, fmt.Errorf("scan identity: %w", err)
		}
		if idenEmail.Valid {
			iden.Email = &idenEmail.String
		}
		detail.Identities = append(detail.Identities, iden)
	}
	if err := idenRows.Err(); err != nil {
		return nil, fmt.Errorf("iterate identities: %w", err)
	}

	return &detail, nil
}

func (r *Repository) ListIdentities(ctx context.Context, page, pageSize int, provider string) ([]IdentitySummary, int64, error) {
	page, pageSize = clampPagination(page, pageSize)
	offset := (page - 1) * pageSize

	whereClause := ""
	var args []any
	if provider != "" {
		whereClause = "WHERE ei.provider = $1"
		args = append(args, provider)
	}

	countQuery := fmt.Sprintf(`SELECT COUNT(*) FROM external_identities ei %s`, whereClause)
	var total int64
	if err := r.db.QueryRow(ctx, countQuery, args...).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("count identities: %w", err)
	}

	argIdx := len(args) + 1
	selectQuery := fmt.Sprintf(`
		SELECT ei.id, ei.user_id, u.canonical_email, ei.provider, ei.provider_subject,
		       ei.email, ei.email_verified, COALESCE(ei.display_name, ''),
		       COALESCE(ei.avatar_url, ''), ei.created_at
		FROM external_identities ei
		LEFT JOIN users u ON u.id = ei.user_id
		%s
		ORDER BY ei.created_at DESC, ei.id DESC
		LIMIT $%d OFFSET $%d
	`, whereClause, argIdx, argIdx+1)

	args = append(args, pageSize, offset)

	rows, err := r.db.Query(ctx, selectQuery, args...)
	if err != nil {
		return nil, 0, fmt.Errorf("query identities: %w", err)
	}
	defer rows.Close()

	list := make([]IdentitySummary, 0)
	for rows.Next() {
		var iden IdentitySummary
		var userEmail, email sql.NullString
		if err := rows.Scan(
			&iden.ID, &iden.UserID, &userEmail, &iden.Provider, &iden.ProviderSubject,
			&email, &iden.EmailVerified, &iden.DisplayName, &iden.AvatarURL, &iden.CreatedAt,
		); err != nil {
			return nil, 0, fmt.Errorf("scan identity: %w", err)
		}
		if userEmail.Valid {
			iden.UserEmail = &userEmail.String
		}
		if email.Valid {
			iden.Email = &email.String
		}
		list = append(list, iden)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("iterate identities: %w", err)
	}

	return list, total, nil
}

func (r *Repository) ListRoles(ctx context.Context, page, pageSize int) ([]RoleItem, int64, error) {
	page, pageSize = clampPagination(page, pageSize)
	offset := (page - 1) * pageSize

	var total int64
	if err := r.db.QueryRow(ctx, `SELECT COUNT(*) FROM roles`).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("count roles: %w", err)
	}

	query := `
		SELECT r.id, r.key, r.name, COALESCE(rc.user_count, 0), r.created_at, r.updated_at
		FROM roles r
		LEFT JOIN (
			SELECT role_id, COUNT(*) AS user_count
			FROM user_roles
			GROUP BY role_id
		) rc ON rc.role_id = r.id
		ORDER BY r.name ASC, r.id ASC
		LIMIT $1 OFFSET $2
	`
	rows, err := r.db.Query(ctx, query, pageSize, offset)
	if err != nil {
		return nil, 0, fmt.Errorf("query roles: %w", err)
	}
	defer rows.Close()

	roles := make([]RoleItem, 0)
	for rows.Next() {
		var role RoleItem
		if err := rows.Scan(&role.ID, &role.Key, &role.Name, &role.UserCount, &role.CreatedAt, &role.UpdatedAt); err != nil {
			return nil, 0, fmt.Errorf("scan role: %w", err)
		}
		roles = append(roles, role)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("iterate roles: %w", err)
	}

	return roles, total, nil
}

func (r *Repository) GetRole(ctx context.Context, roleID uuid.UUID) (*RoleItem, error) {
	query := `
		SELECT r.id, r.key, r.name, COALESCE(rc.user_count, 0), r.created_at, r.updated_at
		FROM roles r
		LEFT JOIN (
			SELECT role_id, COUNT(*) AS user_count
			FROM user_roles
			GROUP BY role_id
		) rc ON rc.role_id = r.id
		WHERE r.id = $1
	`
	var role RoleItem
	err := r.db.QueryRow(ctx, query, roleID).Scan(&role.ID, &role.Key, &role.Name, &role.UserCount, &role.CreatedAt, &role.UpdatedAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("query role: %w", err)
	}
	return &role, nil
}

func (r *Repository) CreateRole(ctx context.Context, key, name string) (*RoleItem, error) {
	if key == "" || name == "" {
		return nil, fmt.Errorf("key and name are required")
	}

	newID, err := uuid.NewV7()
	if err != nil {
		return nil, fmt.Errorf("generate role id: %w", err)
	}

	query := `
		INSERT INTO roles (id, key, name)
		VALUES ($1, $2, $3)
		RETURNING id, key, name, created_at, updated_at
	`
	var role RoleItem
	err = r.db.QueryRow(ctx, query, newID, key, name).Scan(&role.ID, &role.Key, &role.Name, &role.CreatedAt, &role.UpdatedAt)
	if err != nil {
		if isUniqueViolation(err) {
			return nil, ErrConflict
		}
		return nil, fmt.Errorf("insert role: %w", err)
	}

	return &role, nil
}

func (r *Repository) UpdateRole(ctx context.Context, roleID uuid.UUID, name string) (*RoleItem, error) {
	query := `
		UPDATE roles SET name = $1, updated_at = now()
		WHERE id = $2
		RETURNING id, key, name, created_at, updated_at
	`
	var role RoleItem
	err := r.db.QueryRow(ctx, query, name, roleID).Scan(&role.ID, &role.Key, &role.Name, &role.CreatedAt, &role.UpdatedAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("update role: %w", err)
	}

	countQuery := `SELECT COUNT(*) FROM user_roles WHERE role_id = $1`
	if err := r.db.QueryRow(ctx, countQuery, roleID).Scan(&role.UserCount); err != nil {
		return nil, fmt.Errorf("count role users: %w", err)
	}

	return &role, nil
}

func (r *Repository) DeleteRole(ctx context.Context, roleID uuid.UUID) error {
	var key string
	if err := r.db.QueryRow(ctx, `SELECT key FROM roles WHERE id = $1`, roleID).Scan(&key); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrNotFound
		}
		return fmt.Errorf("query role: %w", err)
	}
	if key == adminRoleKey {
		return fmt.Errorf("%w: cannot delete the admin role", ErrConflict)
	}

	var assigned bool
	checkQuery := `SELECT EXISTS (SELECT 1 FROM user_roles WHERE role_id = $1)`
	if err := r.db.QueryRow(ctx, checkQuery, roleID).Scan(&assigned); err != nil {
		return fmt.Errorf("check role assignment: %w", err)
	}
	if assigned {
		return fmt.Errorf("%w: cannot delete role currently assigned to users", ErrConflict)
	}

	res, err := r.db.Exec(ctx, `DELETE FROM roles WHERE id = $1`, roleID)
	if err != nil {
		return fmt.Errorf("delete role: %w", err)
	}
	if res.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

func (r *Repository) UpdateUserRoles(ctx context.Context, userID uuid.UUID, roleIDs []uuid.UUID) error {
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback(ctx)

	var userExists bool
	if err := tx.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM users WHERE id = $1)`, userID).Scan(&userExists); err != nil {
		return fmt.Errorf("check user exists: %w", err)
	}
	if !userExists {
		return ErrNotFound
	}

	if len(roleIDs) > 0 {
		var validCount int
		checkRoles := `SELECT COUNT(*) FROM roles WHERE id = ANY($1)`
		if err := tx.QueryRow(ctx, checkRoles, roleIDs).Scan(&validCount); err != nil {
			return fmt.Errorf("check role ids: %w", err)
		}
		if validCount != len(roleIDs) {
			return fmt.Errorf("%w: unknown role id", ErrNotFound)
		}
	}

	var adminRoleID uuid.UUID
	err = tx.QueryRow(ctx, `SELECT id FROM roles WHERE key = $1 FOR UPDATE`, adminRoleKey).Scan(&adminRoleID)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return fmt.Errorf("lock admin role: %w", err)
	}

	if err == nil {
		willBeAdmin := false
		for _, id := range roleIDs {
			if id == adminRoleID {
				willBeAdmin = true
				break
			}
		}

		if !willBeAdmin {
			var otherAdmins int
			countQuery := `SELECT COUNT(*) FROM user_roles WHERE role_id = $1 AND user_id != $2`
			if err := tx.QueryRow(ctx, countQuery, adminRoleID, userID).Scan(&otherAdmins); err != nil {
				return fmt.Errorf("count other admins: %w", err)
			}
			if otherAdmins == 0 {
				return ErrLastAdmin
			}
		}
	}

	if _, err := tx.Exec(ctx, `DELETE FROM user_roles WHERE user_id = $1`, userID); err != nil {
		return fmt.Errorf("delete user roles: %w", err)
	}

	for _, roleID := range roleIDs {
		if _, err := tx.Exec(ctx, `INSERT INTO user_roles (user_id, role_id) VALUES ($1, $2)`, userID, roleID); err != nil {
			return fmt.Errorf("insert user role: %w", err)
		}
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit tx: %w", err)
	}
	return nil
}

func (r *Repository) ListSessions(ctx context.Context, page, pageSize int, userID *uuid.UUID, activeOnly bool) ([]SessionItem, int64, error) {
	page, pageSize = clampPagination(page, pageSize)
	offset := (page - 1) * pageSize

	var conditions []string
	var args []any
	argIndex := 1

	if activeOnly {
		conditions = append(conditions, "s.revoked_at IS NULL AND s.expires_at > now()")
	}
	if userID != nil {
		conditions = append(conditions, fmt.Sprintf("s.user_id = $%d", argIndex))
		args = append(args, *userID)
		argIndex++
	}

	whereClause := ""
	if len(conditions) > 0 {
		whereClause = "WHERE " + strings.Join(conditions, " AND ")
	}

	countQuery := fmt.Sprintf(`SELECT COUNT(*) FROM sessions s %s`, whereClause)
	var total int64
	if err := r.db.QueryRow(ctx, countQuery, args...).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("count sessions: %w", err)
	}

	selectQuery := fmt.Sprintf(`
		SELECT s.id, s.user_id, u.canonical_email, COALESCE(p.display_name, ''),
		       c.id, c.name, ei.provider,
		       s.expires_at, s.revoked_at, s.created_at
		FROM sessions s
		JOIN users u ON u.id = s.user_id
		LEFT JOIN profiles p ON p.user_id = u.id
		JOIN clients c ON c.id = s.client_ref
		JOIN external_identities ei ON ei.id = s.identity_id
		%s
		ORDER BY s.created_at DESC, s.id DESC
		LIMIT $%d OFFSET $%d
	`, whereClause, argIndex, argIndex+1)

	args = append(args, pageSize, offset)

	rows, err := r.db.Query(ctx, selectQuery, args...)
	if err != nil {
		return nil, 0, fmt.Errorf("query sessions: %w", err)
	}
	defer rows.Close()

	list := make([]SessionItem, 0)
	for rows.Next() {
		var s SessionItem
		var email sql.NullString
		if err := rows.Scan(
			&s.ID, &s.UserID, &email, &s.UserName,
			&s.ClientID, &s.ClientName, &s.Provider,
			&s.ExpiresAt, &s.RevokedAt, &s.CreatedAt,
		); err != nil {
			return nil, 0, fmt.Errorf("scan session: %w", err)
		}
		if email.Valid {
			s.UserEmail = &email.String
		}
		list = append(list, s)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("iterate sessions: %w", err)
	}

	return list, total, nil
}

func (r *Repository) RevokeSession(ctx context.Context, sessionID uuid.UUID) error {
	_, err := r.db.Exec(ctx, `UPDATE sessions SET revoked_at = now(), updated_at = now() WHERE id = $1 AND revoked_at IS NULL`, sessionID)
	if err != nil {
		return fmt.Errorf("revoke session: %w", err)
	}
	return nil
}

func (r *Repository) ListClients(ctx context.Context, page, pageSize int) ([]ClientRegistrationItem, int64, error) {
	page, pageSize = clampPagination(page, pageSize)
	offset := (page - 1) * pageSize

	var total int64
	if err := r.db.QueryRow(ctx, `SELECT COUNT(*) FROM clients`).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("count clients: %w", err)
	}

	query := `
		SELECT id, provider, client_id, name, redirect_url, redirect_uris, enabled, created_at, updated_at
		FROM clients
		ORDER BY created_at DESC, id DESC
		LIMIT $1 OFFSET $2
	`
	rows, err := r.db.Query(ctx, query, pageSize, offset)
	if err != nil {
		return nil, 0, fmt.Errorf("query clients: %w", err)
	}
	defer rows.Close()

	list := make([]ClientRegistrationItem, 0)
	for rows.Next() {
		var c ClientRegistrationItem
		if err := rows.Scan(
			&c.ID, &c.Provider, &c.ClientID, &c.Name, &c.RedirectURL, &c.RedirectURIs,
			&c.Enabled, &c.CreatedAt, &c.UpdatedAt,
		); err != nil {
			return nil, 0, fmt.Errorf("scan client: %w", err)
		}
		list = append(list, c)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("iterate clients: %w", err)
	}

	return list, total, nil
}

func (r *Repository) GetClient(ctx context.Context, clientID uuid.UUID) (*ClientRegistrationItem, error) {
	query := `
		SELECT id, provider, client_id, name, redirect_url, redirect_uris, enabled, created_at, updated_at
		FROM clients
		WHERE id = $1
	`
	var c ClientRegistrationItem
	err := r.db.QueryRow(ctx, query, clientID).Scan(
		&c.ID, &c.Provider, &c.ClientID, &c.Name, &c.RedirectURL, &c.RedirectURIs,
		&c.Enabled, &c.CreatedAt, &c.UpdatedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("query client: %w", err)
	}
	return &c, nil
}

func (r *Repository) UpdateClientEnabled(ctx context.Context, clientID uuid.UUID, enabled bool) error {
	res, err := r.db.Exec(ctx, `UPDATE clients SET enabled = $1, updated_at = now() WHERE id = $2`, enabled, clientID)
	if err != nil {
		return fmt.Errorf("update client: %w", err)
	}
	if res.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}
