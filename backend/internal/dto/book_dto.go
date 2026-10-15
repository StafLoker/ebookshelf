package dto

import "ebookshelf/internal/domain"

type BookFilterRequest struct {
	Status   string
	Query    string
	AuthorID int64
	TagID    int64
	Language string
	Read     string
	Sort     string
}

// replaces all the metadata: a field left out is cleared
type UpdateBookRequest struct {
	Title       string       `json:"title"`
	Language    string       `json:"language"`
	Publisher   string       `json:"publisher"`
	PubYear     int          `json:"pub_year"`
	Description string       `json:"description"`
	Authors     []Author     `json:"authors"`
	Tags        []string     `json:"tags"`
	Identifiers []Identifier `json:"identifiers"`
}

// in a request only the name parts are read
type Author struct {
	ID         int64  `json:"id"`
	FirstName  string `json:"first_name"`
	MiddleName string `json:"middle_name"`
	LastName   string `json:"last_name"`
	Name       string `json:"name"`
}

type BookMetadataResponse struct {
	Title       string       `json:"title"`
	Language    *string      `json:"language"`
	Publisher   *string      `json:"publisher"`
	PubYear     *int         `json:"pub_year"`
	Description *string      `json:"description"`
	Format      string       `json:"format"`
	Authors     []Author     `json:"authors"`
	Tags        []string     `json:"tags"`
	Identifiers []Identifier `json:"identifiers"`
	HasCover    bool         `json:"has_cover"`
}

type Identifier struct {
	Type  string `json:"type"`
	Value string `json:"value"`
}

type BookResponse struct {
	ID        int64             `json:"id"`
	Status    domain.BookStatus `json:"status"`
	Read      bool              `json:"read"`
	CreatedAt string            `json:"created_at"`
	BookMetadataResponse
}

type BookSummaryResponse struct {
	ID      int64    `json:"id"`
	Title   string   `json:"title"`
	Authors []Author `json:"authors"`
	Read    bool     `json:"read"`
}

type FileResponse struct {
	Path string
	Name string
	Hash string
}
