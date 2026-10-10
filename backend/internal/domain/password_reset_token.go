package domain

type PasswordResetToken struct {
	UserID    int64  `db:"user_id"`
	TokenHash string `db:"token_hash"`
	ExpiresAt string `db:"expires_at"`
}
