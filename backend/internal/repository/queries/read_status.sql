-- Name: Create
INSERT OR IGNORE INTO read_status (user_id, book_id) VALUES (?, ?)

-- Name: Exists
SELECT EXISTS (SELECT 1 FROM read_status WHERE user_id = ? AND book_id = ?)

-- Name: Delete
DELETE FROM read_status WHERE user_id = ? AND book_id = ?
