CREATE TABLE users (
  id              INTEGER PRIMARY KEY AUTOINCREMENT,
  username        TEXT    NOT NULL UNIQUE COLLATE NOCASE,
  password_hash   TEXT    NOT NULL,
  role            TEXT    NOT NULL CHECK (role IN ('root','admin','user')),
  can_upload      INTEGER NOT NULL DEFAULT 0 CHECK (can_upload IN (0,1)),
  can_edit        INTEGER NOT NULL DEFAULT 0 CHECK (can_edit   IN (0,1)),
  can_delete      INTEGER NOT NULL DEFAULT 0 CHECK (can_delete IN (0,1)),
  session_version INTEGER NOT NULL DEFAULT 1   -- bumping it invalidates all of the user's sessions
);

CREATE TABLE password_reset_tokens (
  user_id    INTEGER PRIMARY KEY REFERENCES users(id) ON DELETE CASCADE,
  token_hash TEXT    NOT NULL UNIQUE,
  expires_at TEXT    NOT NULL
);

CREATE TABLE books (
  id          INTEGER PRIMARY KEY AUTOINCREMENT,
  title       TEXT    NOT NULL CHECK (title <> ''),
  language    TEXT    CHECK (language GLOB '[a-z][a-z]' OR language GLOB '[a-z][a-z][a-z]'),
  publisher   TEXT,
  pub_year    INTEGER CHECK (pub_year BETWEEN 1 AND 9999),
  description TEXT,
  format      TEXT    NOT NULL,
  file_hash   TEXT    NOT NULL UNIQUE,         -- of the current file; an upload with a known hash is a duplicate
  source_hash TEXT    NOT NULL,                -- of the uploaded original, which stays in original.<ext>
  status      TEXT    NOT NULL DEFAULT 'pending' CHECK (status IN ('pending','added')),  -- pending books stay out of the catalog
  uploaded_by INTEGER REFERENCES users(id) ON DELETE SET NULL,
  created_at  TEXT    NOT NULL
);

CREATE TABLE authors (
  id          INTEGER PRIMARY KEY AUTOINCREMENT,
  first_name  TEXT    NOT NULL COLLATE CASEFOLD CHECK (first_name NOT GLOB '* *'),  -- one word
  middle_name TEXT    NOT NULL COLLATE CASEFOLD,
  last_name   TEXT    NOT NULL COLLATE CASEFOLD CHECK (last_name <> ''),  -- a one-word name or nickname (Plato) goes here
  UNIQUE (last_name, first_name, middle_name)
);

CREATE TABLE book_authors (
  book_id   INTEGER NOT NULL REFERENCES books(id)   ON DELETE CASCADE,
  author_id INTEGER NOT NULL REFERENCES authors(id),
  position  INTEGER NOT NULL,
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

CREATE TABLE book_identifiers (
  book_id INTEGER NOT NULL REFERENCES books(id) ON DELETE CASCADE,
  type    TEXT    NOT NULL,                    -- lowercase scheme: isbn, doi, litres, livelib…;
  value   TEXT    NOT NULL,
  PRIMARY KEY (book_id, type, value)
);

CREATE TABLE read_status (
  user_id INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  book_id INTEGER NOT NULL REFERENCES books(id) ON DELETE CASCADE,
  PRIMARY KEY (user_id, book_id)
);

-- Deletes the author of a removed link when no other book uses it, so no author stays without books.
CREATE TRIGGER tr_authors_orphan AFTER DELETE ON book_authors
BEGIN
  DELETE FROM authors
   WHERE id = OLD.author_id
     AND NOT EXISTS (SELECT 1 FROM book_authors WHERE author_id = OLD.author_id);
END;

-- Deletes the tag of a removed link when no other book uses it, so no tag stays without books.
CREATE TRIGGER tr_tags_orphan AFTER DELETE ON book_tags
BEGIN
  DELETE FROM tags
   WHERE id = OLD.tag_id
     AND NOT EXISTS (SELECT 1 FROM book_tags WHERE tag_id = OLD.tag_id);
END;

-- Rejects adding a pending book to the library when it has no author.
CREATE TRIGGER tr_books_need_author BEFORE UPDATE OF status ON books
WHEN NEW.status = 'added' AND NOT EXISTS (SELECT 1 FROM book_authors WHERE book_id = NEW.id)
BEGIN
  SELECT RAISE(ABORT, 'book needs an author');
END;

-- Rejects removing the last author of a book that is already in the library.
CREATE TRIGGER tr_book_authors_keep_one AFTER DELETE ON book_authors
WHEN (SELECT status FROM books WHERE id = OLD.book_id) = 'added'
  AND NOT EXISTS (SELECT 1 FROM book_authors WHERE book_id = OLD.book_id)
BEGIN
  SELECT RAISE(ABORT, 'book needs an author');
END;

-- CREATE TABLE quotes (                           -- quotes, private to each user
--   id      INTEGER PRIMARY KEY,                  -- quote order is creation order
--   user_id INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
--   book_id INTEGER NOT NULL REFERENCES books(id) ON DELETE CASCADE,
--   text    TEXT    NOT NULL,                     -- trimmed and whitespace-normalized before saving
--   note    TEXT,
--   UNIQUE (user_id, book_id, text)               -- reimporting the same HTML does not duplicate; also the per-user-and-book index
-- );

-- CREATE TABLE oidc_identities (
--   user_id INTEGER PRIMARY KEY REFERENCES users(id) ON DELETE CASCADE,
--   issuer  TEXT    NOT NULL,
--   subject TEXT    NOT NULL,
--   UNIQUE (issuer, subject)
-- );

-- CREATE TABLE magic_link_codes (
--   code            TEXT PRIMARY KEY,            -- 6 random chars; typed or in the QR
--   poll_token_hash TEXT NOT NULL,               -- secret known only to the waiting browser
--   expires_at      TEXT NOT NULL,
--   approved_by     INTEGER REFERENCES users(id) ON DELETE CASCADE  -- NULL until a signed-in device approves it
-- );

-- CREATE TABLE settings (                          -- global settings (root/admin only)
--   key   TEXT PRIMARY KEY,
--   value TEXT NOT NULL                            -- JSON; secrets encrypted with ENCRYPTION_KEY (AES-256-GCM)
-- );

-- CREATE TABLE user_mail_accounts (                -- eReader delivery account, one per user
--   user_id INTEGER PRIMARY KEY REFERENCES users(id) ON DELETE CASCADE,
--   kind    TEXT NOT NULL CHECK (kind IN ('gmail_oauth','smtp')),
--   config  TEXT NOT NULL                          -- encrypted JSON (OAuth tokens or SMTP credentials)
-- );

-- CREATE TABLE devices (
--   user_id INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
--   email   TEXT    NOT NULL,
--   name    TEXT    NOT NULL,
--   PRIMARY KEY (user_id, email)
-- );

-- CREATE TABLE magic_shelves (
--   id      INTEGER PRIMARY KEY,                   -- sidebar order is creation order
--   user_id INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
--   name    TEXT    NOT NULL,
--   icon    TEXT    NOT NULL DEFAULT 'star',
--   rules   TEXT    NOT NULL                       -- JSON: all/any group + conditions
-- );
