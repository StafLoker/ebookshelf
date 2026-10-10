-- Full MVP schema (plan parts 1 and 2). Dates are ISO 8601 UTC text; booleans are INTEGER 0/1.

CREATE TABLE users (
  id              INTEGER PRIMARY KEY AUTOINCREMENT,  -- AUTOINCREMENT: a deleted id is never reused (stale cookies)
  username        TEXT    NOT NULL UNIQUE COLLATE NOCASE,
  password_hash   TEXT    NOT NULL,            -- argon2id; random unknown password for admin- or OIDC-created users
  role            TEXT    NOT NULL CHECK (role IN ('root','admin','user')),
  can_upload      INTEGER NOT NULL DEFAULT 0 CHECK (can_upload IN (0,1)),
  can_edit        INTEGER NOT NULL DEFAULT 0 CHECK (can_edit   IN (0,1)),
  can_delete      INTEGER NOT NULL DEFAULT 0 CHECK (can_delete IN (0,1)),
  session_version INTEGER NOT NULL DEFAULT 1   -- bumping it invalidates all of the user's sessions
);

CREATE TABLE password_reset_tokens (           -- one live token per user: a new one replaces the old
  user_id    INTEGER PRIMARY KEY REFERENCES users(id) ON DELETE CASCADE,
  token_hash TEXT    NOT NULL UNIQUE,          -- SHA-256; the plain token is shown once
  expires_at TEXT    NOT NULL                  -- row is deleted on use, no "used" flag
);

CREATE TABLE books (
  id          INTEGER PRIMARY KEY AUTOINCREMENT,  -- AUTOINCREMENT: a stray books/<id>/ folder never points to a new book
  title       TEXT    NOT NULL,
  language    TEXT    CHECK (language GLOB '[a-z][a-z]' OR language GLOB '[a-z][a-z][a-z]'),  -- ISO 639-1; ISO 639-3 only if the language has no 2-letter code
  publisher   TEXT,
  pub_year    INTEGER,
  isbn        TEXT,
  description TEXT,
  format      TEXT    NOT NULL,                -- extension; valid list is the FORMATS registry in code
  file_hash   TEXT    NOT NULL UNIQUE,         -- SHA-256 of the current file: blocks uploading the same book twice
  source_hash TEXT    NOT NULL,                -- SHA-256 of the uploaded original (original.<ext>)
  created_at  TEXT    NOT NULL
);
CREATE INDEX ix_books_title   ON books(title COLLATE CASEFOLD);  -- NOCASE covers only ASCII and mis-sorts Cyrillic
CREATE INDEX ix_books_created ON books(created_at DESC);
CREATE INDEX ix_books_source  ON books(source_hash);

CREATE TABLE authors (
  id    INTEGER PRIMARY KEY AUTOINCREMENT,
  name  TEXT    NOT NULL UNIQUE COLLATE CASEFOLD   -- one author per spelling, ignoring case and ё/е
);

CREATE TABLE book_authors (
  book_id   INTEGER NOT NULL REFERENCES books(id)   ON DELETE CASCADE,
  author_id INTEGER NOT NULL REFERENCES authors(id) ON DELETE RESTRICT,
  position  INTEGER NOT NULL,                  -- author order in the book
  PRIMARY KEY (book_id, author_id)
);
CREATE INDEX ix_book_authors_author ON book_authors(author_id);

CREATE TABLE tags (
  id    INTEGER PRIMARY KEY,
  name  TEXT    NOT NULL UNIQUE COLLATE CASEFOLD
);
CREATE TABLE book_tags (
  book_id INTEGER NOT NULL REFERENCES books(id) ON DELETE CASCADE,
  tag_id  INTEGER NOT NULL REFERENCES tags(id)  ON DELETE CASCADE,
  PRIMARY KEY (book_id, tag_id)
);
CREATE INDEX ix_book_tags_tag ON book_tags(tag_id);

CREATE TABLE read_status (
  user_id INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  book_id INTEGER NOT NULL REFERENCES books(id) ON DELETE CASCADE,
  PRIMARY KEY (user_id, book_id)               -- row exists = read
);

-- An author or tag exists only while something references it: it is removed in the same
-- transaction that drops its last reference, with no application cleanup.
CREATE TRIGGER tr_authors_orphan AFTER DELETE ON book_authors
BEGIN
  DELETE FROM authors
   WHERE id = OLD.author_id
     AND NOT EXISTS (SELECT 1 FROM book_authors WHERE author_id = OLD.author_id);
END;

CREATE TRIGGER tr_tags_orphan AFTER DELETE ON book_tags
BEGIN
  DELETE FROM tags
   WHERE id = OLD.tag_id
     AND NOT EXISTS (SELECT 1 FROM book_tags WHERE tag_id = OLD.tag_id);
END;

-- Part 2: features built later (same file, not another migration).

CREATE TABLE quotes (                           -- quotes, private to each user
  id      INTEGER PRIMARY KEY,                  -- quote order is creation order
  user_id INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  book_id INTEGER NOT NULL REFERENCES books(id) ON DELETE CASCADE,
  text    TEXT    NOT NULL,                     -- trimmed and whitespace-normalized before saving
  note    TEXT,
  UNIQUE (user_id, book_id, text)               -- reimporting the same HTML does not duplicate; also the per-user-and-book index
);

CREATE TABLE oidc_identities (
  user_id INTEGER PRIMARY KEY REFERENCES users(id) ON DELETE CASCADE,
  issuer  TEXT    NOT NULL,
  subject TEXT    NOT NULL,
  UNIQUE (issuer, subject)
);

CREATE TABLE magic_link_codes (
  code            TEXT PRIMARY KEY,            -- 6 random chars; typed or in the QR
  poll_token_hash TEXT NOT NULL,               -- secret known only to the waiting browser
  expires_at      TEXT NOT NULL,
  approved_by     INTEGER REFERENCES users(id) ON DELETE CASCADE  -- NULL until a signed-in device approves it
);

CREATE TABLE settings (                          -- global settings (root/admin only)
  key   TEXT PRIMARY KEY,
  value TEXT NOT NULL                            -- JSON; secrets encrypted with ENCRYPTION_KEY (AES-256-GCM)
);

CREATE TABLE user_mail_accounts (                -- eReader delivery account, one per user
  user_id INTEGER PRIMARY KEY REFERENCES users(id) ON DELETE CASCADE,
  kind    TEXT NOT NULL CHECK (kind IN ('gmail_oauth','smtp')),
  config  TEXT NOT NULL                          -- encrypted JSON (OAuth tokens or SMTP credentials)
);
CREATE TABLE devices (
  user_id INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  email   TEXT    NOT NULL,
  name    TEXT    NOT NULL,
  PRIMARY KEY (user_id, email)
);

CREATE TABLE magic_shelves (
  id      INTEGER PRIMARY KEY,                   -- sidebar order is creation order
  user_id INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  name    TEXT    NOT NULL,
  icon    TEXT    NOT NULL DEFAULT 'star',
  rules   TEXT    NOT NULL                       -- JSON: all/any group + conditions
);
