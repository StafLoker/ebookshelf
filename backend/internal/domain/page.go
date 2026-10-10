package domain

type Page[T any] struct {
	Items    []T
	Total    int
	Page     int
	PageSize int
}
