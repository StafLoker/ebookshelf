-- Name: Create
INSERT INTO users (username, password_hash, role, can_add, can_edit, can_delete)
VALUES (:username, :password_hash, :role, :can_add, :can_edit, :can_delete)

-- Name: FindByID
SELECT id, username, password_hash, role, can_add, can_edit, can_delete, session_version
FROM users
WHERE id = ?

-- Name: FindByUsername
SELECT id, username, password_hash, role, can_add, can_edit, can_delete, session_version
FROM users
WHERE username = ?

-- Name: FindByRole
SELECT id, username, password_hash, role, can_add, can_edit, can_delete, session_version
FROM users
WHERE role = ?
LIMIT 1

-- Name: FindAll
SELECT id, username, password_hash, role, can_add, can_edit, can_delete, session_version
FROM users
ORDER BY username
LIMIT ? OFFSET ?

-- Name: Count
SELECT COUNT(*) FROM users

-- Name: Update
UPDATE users
SET role = ?, can_add = ?, can_edit = ?, can_delete = ?
WHERE id = ?

-- Name: ChangePassword
UPDATE users
SET password_hash = ?, session_version = session_version + 1
WHERE id = ?

-- Name: Delete
DELETE FROM users WHERE id = ?
