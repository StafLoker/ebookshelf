-- Name: Create
INSERT INTO password_reset_tokens (user_id, token_hash, expires_at)
VALUES (?, ?, ?)
ON CONFLICT (user_id) DO UPDATE
SET token_hash = excluded.token_hash, expires_at = excluded.expires_at

-- Name: FindByTokenHash
SELECT user_id, token_hash, expires_at
FROM password_reset_tokens
WHERE token_hash = ?

-- Name: DeleteByTokenHash
DELETE FROM password_reset_tokens WHERE token_hash = ?
