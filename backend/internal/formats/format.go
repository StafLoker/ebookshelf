package formats

import (
	"fmt"
	"html"
	"slices"
	"strconv"
	"strings"

	"github.com/beevik/etree"
	"golang.org/x/text/language"

	"ebookshelf/internal/domain"
)

const (
	yearLen = 4
)

type Metadata struct {
	Title       string
	Language    string
	Publisher   string
	Description string
	Year        int
	Authors     []domain.Author
	Tags        []string
	Identifiers []domain.Identifier
	Cover       []byte
}

type BookFormat interface {
	Read(path string) (*Metadata, error)
	Write(src, dst string, meta *Metadata) error
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

func ParseLanguage(s string) string {
	tag, err := language.Parse(strings.TrimSpace(s))
	if err != nil {
		return ""
	}
	base, _ := tag.Base()
	return base.String()
}

// the type is the scheme attribute; uuid and calibre ids identify the file, not the book, so they are not kept
func ParseIdentifiers(scheme, text string) []domain.Identifier {
	var ids []domain.Identifier

	kind, value := strings.ToLower(strings.TrimSpace(scheme)), strings.TrimSpace(text)
	switch {
	case kind == "" || value == "" || kind == "uuid" || kind == "calibre":
	case kind == "isbn":
		for _, isbn := range parseISBNs(value) {
			ids = append(ids, domain.Identifier{Type: kind, Value: isbn})
		}
	default:
		ids = append(ids, domain.Identifier{Type: kind, Value: value})
	}
	return ids
}

// builds a valid author from loose name parts read from a file: no commas, a one-word first name,
// and a last name always, taken from the given names when the file has none
func newAuthor(first, middle, last string) domain.Author {
	var (
		author domain.Author
		given  = strings.Fields(strings.ReplaceAll(first+" "+middle, ",", " "))
	)

	author.LastName = strings.Join(strings.Fields(strings.ReplaceAll(last, ",", " ")), " ")
	if author.LastName == "" {
		author.LastName, given = strings.Join(given, " "), nil
	}
	if len(given) > 0 {
		author.FirstName, author.MiddleName = given[0], strings.Join(given[1:], " ")
	}
	return author
}

// drops an author without a name and one already in the list
func appendAuthor(list []domain.Author, a domain.Author) []domain.Author {
	if a.LastName != "" && !slices.Contains(list, a) {
		list = append(list, a)
	}
	return list
}

func appendNonEmpty(list []string, s string) []string {
	s = strings.Join(strings.Fields(s), " ")
	if s != "" {
		list = append(list, s)
	}
	return list
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

func textElements(tag string, values []string) []*etree.Element {
	elements := make([]*etree.Element, len(values))
	for i, v := range values {
		elements[i] = etree.NewElement(tag)
		elements[i].SetText(v)
	}
	return elements
}

// "" for year 0, which clears the field; four digits, because a shorter year does not read back
func formatYear(year int) string {
	var text string

	if year != 0 {
		text = fmt.Sprintf("%04d", year)
	}
	return text
}

func parseYear(s string) int {
	if len(s) < yearLen {
		return 0
	}
	y, _ := strconv.Atoi(s[:yearLen])
	return max(y, 0)
}

func parseISBNs(s string) []string {
	var list []string

	for _, part := range strings.Split(s, ",") { // multiple ISBNs can be separated by commas
		isbn := strings.ToUpper(strings.ReplaceAll(strings.ReplaceAll(part, "-", ""), " ", "")) // remove hyphens and spaces, and normalize to uppercase
		if len(isbn) == 10 || len(isbn) == 13 {
			list = append(list, isbn)
		}
	}
	return list
}
