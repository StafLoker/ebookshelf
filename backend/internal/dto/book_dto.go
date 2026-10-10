package dto

import "ebookshelf/internal/domain"

type BookFilterRequest struct {
	Query    string
	AuthorID int64
	TagID    int64
	Language string
	Read     string
	Sort     string
}

type BookMetadataResponse struct {
	Title       string               `json:"title"`
	Language    *string              `json:"language"`
	Publisher   *string              `json:"publisher"`
	PubYear     *int                 `json:"pub_year"`
	Description *string              `json:"description"`
	Format      string               `json:"format"`
	Authors     []string             `json:"authors"`
	Tags        []string             `json:"tags"`
	Identifiers []IdentifierResponse `json:"identifiers"`
	HasCover    bool                 `json:"has_cover"`
}

type IdentifierResponse struct {
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
	Authors []string `json:"authors"`
	Read    bool     `json:"read"`
}

type FileResponse struct {
	Path string
	Name string
	Hash string
}
