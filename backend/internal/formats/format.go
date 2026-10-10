package formats

import (
	"html"
	"strconv"
	"strings"
	"unicode"

	"github.com/beevik/etree"
	"golang.org/x/text/language"

	"ebookshelf/internal/domain"
)

const (
	yearLen   = 4
	isbn10Len = 10
	isbn13Len = 13
	isbnChars = "0123456789Xx- "
)

type Metadata struct {
	Title       string
	Language    string
	Publisher   string
	Description string
	Year        int
	Authors     []string
	Tags        []string
	Identifiers []domain.Identifier
	Cover       []byte
}

type BookFormat interface {
	Read(path string) (*Metadata, error)
}

func For(ext string) BookFormat {
	var format BookFormat

	switch ext {
	case domain.BookEPUB:
		format = epub{}
	case domain.BookFB2:
		format = fb2{}
	}
	return format
}

func parseLanguage(s string) string {
	tag, err := language.Parse(strings.TrimSpace(s))
	if err != nil {
		return ""
	}
	base, _ := tag.Base()
	return base.String()
}

func stripTags(s string) string {
	var (
		b     strings.Builder
		inTag bool
	)

	for _, r := range s {
		switch {
		case r == '<':
			inTag = true
			b.WriteByte(' ')
		case r == '>' && inTag:
			inTag = false
		case !inTag:
			b.WriteRune(r)
		}
	}
	return strings.Join(strings.Fields(html.UnescapeString(b.String())), " ")
}

// allText joins the text of all descendants; etree's Text() stops at the first child element.
func allText(e *etree.Element) string {
	var b strings.Builder

	for _, token := range e.Child {
		switch t := token.(type) {
		case *etree.CharData:
			b.WriteString(t.Data)
		case *etree.Element:
			b.WriteString(" ")
			b.WriteString(allText(t))
			b.WriteString(" ")
		}
	}
	return b.String()
}

func parseYear(s string) int {
	if len(s) < yearLen {
		return 0
	}
	y, _ := strconv.Atoi(s[:yearLen])
	return max(y, 0)
}

// each part runs from its first digit to the first character an ISBN cannot hold ("(hardcover)" is cut off);
// only values with 10 or 13 characters once hyphens and spaces are gone are kept
func parseISBNs(s string) []string {
	var list []string

	for _, part := range strings.FieldsFunc(s, func(r rune) bool { return r == ',' || r == ';' }) {
		if start := strings.IndexFunc(part, unicode.IsDigit); start >= 0 {
			part = part[start:]
			if end := strings.IndexFunc(part, func(r rune) bool { return !strings.ContainsRune(isbnChars, r) }); end >= 0 {
				part = part[:end]
			}
			isbn := strings.Map(func(r rune) rune {
				if r == '-' || r == ' ' {
					return -1
				}
				return unicode.ToUpper(r)
			}, part)
			if len(isbn) == isbn10Len || len(isbn) == isbn13Len {
				list = append(list, isbn)
			}
		}
	}
	return list
}

// the type comes from the scheme attribute, else from a "urn:type:value" or "type:value" prefix;
// a bare value made only of ISBN characters counts as an ISBN, a URL ("http://…") is not an identifier
func parseIdentifiers(scheme, text string) []domain.Identifier {
	var ids []domain.Identifier

	kind, value := strings.ToLower(strings.TrimSpace(scheme)), strings.TrimSpace(text)
	if kind == "" {
		rest := value
		if head, tail, ok := strings.Cut(value, ":"); ok && strings.EqualFold(head, "urn") {
			rest = tail
		}
		if k, v, ok := strings.Cut(rest, ":"); ok {
			kind, value = strings.ToLower(strings.TrimSpace(k)), strings.TrimSpace(v)
		} else if strings.Trim(rest, isbnChars) == "" {
			kind = "isbn"
		}
	}
	switch {
	case kind == "" || value == "" || kind == "uuid" || kind == "calibre" ||
		strings.ContainsAny(kind, " /") || strings.HasPrefix(value, "//"):
	case kind == "isbn":
		for _, isbn := range parseISBNs(value) {
			ids = append(ids, domain.Identifier{Type: kind, Value: isbn})
		}
	default:
		ids = append(ids, domain.Identifier{Type: kind, Value: value})
	}
	return ids
}

func appendNonEmpty(list []string, s string) []string {
	s = strings.Join(strings.Fields(s), " ")
	if s != "" {
		list = append(list, s)
	}
	return list
}
