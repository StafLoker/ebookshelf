package domain

type Page[T any] struct {
	Items      []T
	Page       int
	PageSize   int
	Total      int
	TotalPages int
}

func NewPage[T any](items []T, page, pageSize, total int) Page[T] {
	return Page[T]{Items: items, Page: page, PageSize: pageSize, Total: total, TotalPages: (total + pageSize - 1) / pageSize}
}
