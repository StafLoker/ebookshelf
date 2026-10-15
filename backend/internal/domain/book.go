package domain

type BookStatus string

type ReadFilter string

type BookSort string

const (
	BookPending BookStatus = "pending"
	BookAdded   BookStatus = "added"

	ReadAny    ReadFilter = ""
	ReadOnly   ReadFilter = "read"
	ReadUnread ReadFilter = "unread"

	SortCreated BookSort = "created"
	SortTitle   BookSort = "title"
)

type Book struct {
	ID          int64        `db:"id"`
	Title       string       `db:"title"`
	Language    *string      `db:"language"`
	Publisher   *string      `db:"publisher"`
	PubYear     *int         `db:"pub_year"`
	Description *string      `db:"description"`
	Format      string       `db:"format"`
	FileHash    string       `db:"file_hash"`
	SourceHash  string       `db:"source_hash"`
	Status      BookStatus   `db:"status"`
	AddedBy     *int64       `db:"added_by"`
	CreatedAt   string       `db:"created_at"`
	Authors     []Author     `db:"-"`
	Tags        []string     `db:"-"`
	Identifiers []Identifier `db:"-"`
}

type Identifier struct {
	Type  string `db:"type"`
	Value string `db:"value"`
}

type BookSummary struct {
	ID      int64    `db:"id"`
	Title   string   `db:"title"`
	Read    bool     `db:"read"`
	Authors []Author `db:"-"`
}

type BookFilter struct {
	Status   BookStatus
	AddedBy  int64 // 0 is any user
	Query    string
	AuthorID int64
	TagID    int64
	Language string
	Read     ReadFilter
	Sort     BookSort
}

func (f BookFilter) Valid() bool {
	return (f.Status == BookAdded || f.Status == BookPending) &&
		(f.Read == ReadAny || f.Read == ReadOnly || f.Read == ReadUnread) &&
		(f.Sort == "" || f.Sort == SortCreated || f.Sort == SortTitle)
}
