-- Name: Create
INSERT INTO books (title, language, publisher, pub_year, description, format, file_hash, source_hash, status, uploaded_by, created_at)
VALUES (:title, :language, :publisher, :pub_year, :description, :format, :file_hash, :source_hash, :status, :uploaded_by, :created_at)

-- Name: UpsertAuthor
INSERT INTO authors (name) VALUES (?)
ON CONFLICT (name) DO UPDATE SET name = authors.name
RETURNING id

-- Name: CreateAuthorLink
INSERT OR IGNORE INTO book_authors (book_id, author_id, position) VALUES (?, ?, ?)

-- Name: UpsertTag
INSERT INTO tags (name) VALUES (?)
ON CONFLICT (name) DO UPDATE SET name = tags.name
RETURNING id

-- Name: CreateTagLink
INSERT OR IGNORE INTO book_tags (book_id, tag_id) VALUES (?, ?)

-- Name: FindByID
SELECT id, title, language, publisher, pub_year, description, format, file_hash, source_hash, status, uploaded_by, created_at
FROM books
WHERE id = ?

-- Name: FindByFileHash
SELECT id, title, language, publisher, pub_year, description, format, file_hash, source_hash, status, uploaded_by, created_at
FROM books
WHERE file_hash = ?

-- Name: CreateIdentifier
INSERT OR IGNORE INTO book_identifiers (book_id, type, value) VALUES (?, ?, ?)

-- Name: FindAuthors
SELECT a.name
FROM book_authors ba
JOIN authors a ON a.id = ba.author_id
WHERE ba.book_id = ?
ORDER BY ba.position

-- Name: FindTags
SELECT t.name
FROM book_tags bt
JOIN tags t ON t.id = bt.tag_id
WHERE bt.book_id = ?
ORDER BY t.name

-- Name: FindIdentifiers
SELECT type, value
FROM book_identifiers
WHERE book_id = ?
ORDER BY type, value

-- Name: FindAuthorsByBooks
SELECT ba.book_id, a.name
FROM book_authors ba
JOIN authors a ON a.id = ba.author_id
WHERE ba.book_id IN (?)
ORDER BY ba.book_id, ba.position

-- Name: FindAll
SELECT b.id, b.title, EXISTS (SELECT 1 FROM read_status rs WHERE rs.user_id = ?1 AND rs.book_id = b.id) AS read
FROM books b
WHERE b.status = 'added'
  AND (?2 = '' OR instr(casefold(b.title), casefold(?2)) > 0
       OR EXISTS (SELECT 1 FROM book_authors ba JOIN authors a ON a.id = ba.author_id
                  WHERE ba.book_id = b.id AND instr(casefold(a.name), casefold(?2)) > 0)
       OR EXISTS (SELECT 1 FROM book_tags bt JOIN tags t ON t.id = bt.tag_id
                  WHERE bt.book_id = b.id AND instr(casefold(t.name), casefold(?2)) > 0)
       OR EXISTS (SELECT 1 FROM book_identifiers bi
                  WHERE bi.book_id = b.id AND replace(?2, '-', '') <> ''
                    AND instr(replace(casefold(bi.value), '-', ''), replace(casefold(?2), '-', '')) > 0))
  AND (?3 = 0 OR EXISTS (SELECT 1 FROM book_authors WHERE book_id = b.id AND author_id = ?3))
  AND (?4 = 0 OR EXISTS (SELECT 1 FROM book_tags WHERE book_id = b.id AND tag_id = ?4))
  AND (?5 = '' OR b.language = ?5)
  AND (?6 = '' OR EXISTS (SELECT 1 FROM read_status rs WHERE rs.user_id = ?1 AND rs.book_id = b.id) = (?6 = 'read'))
ORDER BY CASE WHEN ?7 = 'title' THEN b.title END COLLATE CASEFOLD, b.created_at DESC, b.id DESC
LIMIT ?8 OFFSET ?9

-- Name: Count
SELECT COUNT(*)
FROM books b
WHERE b.status = 'added'
  AND (?2 = '' OR instr(casefold(b.title), casefold(?2)) > 0
       OR EXISTS (SELECT 1 FROM book_authors ba JOIN authors a ON a.id = ba.author_id
                  WHERE ba.book_id = b.id AND instr(casefold(a.name), casefold(?2)) > 0)
       OR EXISTS (SELECT 1 FROM book_tags bt JOIN tags t ON t.id = bt.tag_id
                  WHERE bt.book_id = b.id AND instr(casefold(t.name), casefold(?2)) > 0)
       OR EXISTS (SELECT 1 FROM book_identifiers bi
                  WHERE bi.book_id = b.id AND replace(?2, '-', '') <> ''
                    AND instr(replace(casefold(bi.value), '-', ''), replace(casefold(?2), '-', '')) > 0))
  AND (?3 = 0 OR EXISTS (SELECT 1 FROM book_authors WHERE book_id = b.id AND author_id = ?3))
  AND (?4 = 0 OR EXISTS (SELECT 1 FROM book_tags WHERE book_id = b.id AND tag_id = ?4))
  AND (?5 = '' OR b.language = ?5)
  AND (?6 = '' OR EXISTS (SELECT 1 FROM read_status rs WHERE rs.user_id = ?1 AND rs.book_id = b.id) = (?6 = 'read'))

-- Name: FindPending
SELECT id, title, 0 AS read
FROM books
WHERE status = 'pending' AND (?1 = 0 OR uploaded_by = ?1)
ORDER BY id DESC

-- Name: MarkAdded
UPDATE books SET status = 'added', created_at = ? WHERE id = ? AND status = 'pending'

-- Name: DeletePending
DELETE FROM books WHERE id = ? AND status = 'pending'
