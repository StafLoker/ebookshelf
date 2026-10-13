package formats

import (
	"cmp"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"slices"
	"strings"

	"github.com/beevik/etree"
	"golang.org/x/text/encoding/htmlindex"

	"ebookshelf/internal/domain"
)

const (
	// child order the FictionBook 2.1 schema requires
	fb2DescriptionOrder = "title-info src-title-info document-info publish-info custom-info output-document-class"
	fb2TitleInfoOrder   = "genre author book-title annotation keywords date coverpage lang src-lang translator sequence"
	fb2PublishInfoOrder = "book-name publisher city year isbn sequence"
)

type fb2 struct{}

func (fb2) Read(path string) (*Metadata, error) {
	meta := &Metadata{}
	doc := etree.NewDocument()

	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	doc.ReadSettings.CharsetReader = fb2CharsetReader
	if err := doc.ReadFromBytes(data); err != nil {
		return nil, fmt.Errorf("fb2: parse: %w", err)
	}
	info := doc.FindElement("//description/title-info")
	if info == nil {
		return nil, errors.New("fb2: no title-info")
	}
	fb2ReadTitleInfo(info, meta)
	fb2ReadPublishInfo(doc, meta)
	meta.Cover = fb2Cover(doc, info)
	if meta.Title == "" {
		return nil, errors.New("fb2: missing title")
	}
	return meta, nil
}

func (fb2) Write(src, dst string, meta *Metadata) error {
	doc := etree.NewDocument()

	doc.ReadSettings.CharsetReader = fb2CharsetReader
	if err := doc.ReadFromFile(src); err != nil {
		return err
	}
	info := doc.FindElement("//description/title-info")
	if info == nil {
		return errors.New("fb2: no title-info")
	}
	fb2EditTitleInfo(info, meta)
	fb2EditPublishInfo(info.Parent(), meta)
	if meta.Cover != nil {
		if err := fb2EditCover(doc, info, meta.Cover); err != nil {
			return err
		}
	}
	fb2SetEncoding(doc)
	return doc.WriteToFile(dst)
}

// ---- reading ----

func fb2CharsetReader(label string, input io.Reader) (io.Reader, error) {
	enc, err := htmlindex.Get(label)
	if err != nil {
		return nil, err
	}
	return enc.NewDecoder().Reader(input), nil
}

func fb2ReadTitleInfo(info *etree.Element, meta *Metadata) {
	meta.Title = fb2ChildText(info, "book-title")
	meta.Language = ParseLanguage(fb2ChildText(info, "lang"))
	if date := info.SelectElement("date"); date != nil {
		meta.Year = parseYear(cmp.Or(date.SelectAttrValue("value", ""), strings.TrimSpace(date.Text())))
	}
	for _, e := range info.ChildElements() {
		switch e.Tag {
		case "author":
			last := cmp.Or(fb2ChildText(e, "last-name"), fb2ChildText(e, "nickname"))
			meta.Authors = appendAuthor(meta.Authors, newAuthor(fb2ChildText(e, "first-name"), fb2ChildText(e, "middle-name"), last))
		case "keywords":
			for _, tag := range strings.FieldsFunc(e.Text(), func(r rune) bool { return r == ',' || r == ';' }) {
				meta.Tags = appendNonEmpty(meta.Tags, tag)
			}
		case "annotation":
			meta.Description = fb2Annotation(e)
		}
	}
}

func fb2ChildText(e *etree.Element, tag string) string {
	var text string

	if child := e.SelectElement(tag); child != nil {
		text = strings.TrimSpace(allText(child))
	}
	return text
}

func fb2Annotation(e *etree.Element) string {
	var paragraphs []string

	for _, p := range e.SelectElements("p") {
		paragraphs = appendNonEmpty(paragraphs, stripTags(allText(p)))
	}
	return strings.Join(paragraphs, "\n")
}

func fb2ReadPublishInfo(doc *etree.Document, meta *Metadata) {
	if pub := doc.FindElement("//description/publish-info"); pub != nil {
		meta.Publisher = fb2ChildText(pub, "publisher")
		meta.Identifiers = append(meta.Identifiers, ParseIdentifiers("isbn", fb2ChildText(pub, "isbn"))...)
		if y := parseYear(fb2ChildText(pub, "year")); y != 0 {
			meta.Year = y
		}
	}
}

func fb2Cover(doc *etree.Document, info *etree.Element) []byte {
	var (
		cover    []byte
		id       string
		binaries []*etree.Element
	)

	if img := info.FindElement("coverpage/image"); img != nil {
		id = strings.TrimPrefix(img.SelectAttrValue("href", ""), "#")
	}
	if id != "" {
		binaries = doc.FindElements("//binary")
	}
	for i := 0; cover == nil && i < len(binaries); i++ {
		if binaries[i].SelectAttrValue("id", "") == id {
			// a corrupt cover is not an error: the book is still readable
			cover, _ = base64.StdEncoding.DecodeString(strings.Join(strings.Fields(binaries[i].Text()), ""))
		}
	}
	return cover
}

// ---- writing ----

// title, language and authors are required by the format: an empty value never removes them
func fb2EditTitleInfo(info *etree.Element, meta *Metadata) {
	if meta.Title != "" {
		fb2SetText(info, "book-title", meta.Title, fb2TitleInfoOrder)
	}
	if meta.Language != "" {
		fb2SetText(info, "lang", meta.Language, fb2TitleInfoOrder)
	}
	if len(meta.Authors) > 0 {
		fresh := make([]*etree.Element, len(meta.Authors))
		for i, a := range meta.Authors {
			fresh[i] = fb2NewAuthor(a)
		}
		fb2Replace(info, "author", fresh, fb2TitleInfoOrder)
	}
	fb2SetAnnotation(info, meta.Description)
	fb2SetText(info, "date", formatYear(meta.Year), fb2TitleInfoOrder)
	if date := info.SelectElement("date"); date != nil {
		date.CreateAttr("value", formatYear(meta.Year))
	}
	fb2SetText(info, "keywords", strings.Join(meta.Tags, ", "), fb2TitleInfoOrder)
}

// the schema wants a first name next to a last name, so an author with only a last name goes as a nickname
func fb2NewAuthor(a domain.Author) *etree.Element {
	author := etree.NewElement("author")
	if a.FirstName == "" {
		author.CreateElement("nickname").SetText(a.LastName)
	} else {
		author.CreateElement("first-name").SetText(a.FirstName)
		if a.MiddleName != "" {
			author.CreateElement("middle-name").SetText(a.MiddleName)
		}
		author.CreateElement("last-name").SetText(a.LastName)
	}
	return author
}

func fb2SetAnnotation(info *etree.Element, text string) {
	if old := info.SelectElement("annotation"); old != nil {
		info.RemoveChild(old)
	}
	if lines := strings.FieldsFunc(text, func(r rune) bool { return r == '\n' }); len(lines) > 0 {
		annotation := etree.NewElement("annotation")
		for _, line := range lines {
			annotation.CreateElement("p").SetText(line)
		}
		fb2Place(info, annotation, fb2TitleInfoOrder)
	}
}

func fb2EditPublishInfo(desc *etree.Element, meta *Metadata) {
	var isbns []string // FB2 has a place for ISBNs only: other identifier types are not written.

	for _, id := range meta.Identifiers {
		if id.Type == "isbn" {
			isbns = append(isbns, id.Value)
		}
	}
	isbn := strings.Join(isbns, ", ")
	pub := desc.SelectElement("publish-info")
	if pub == nil && (meta.Publisher != "" || meta.Year != 0 || isbn != "") {
		pub = etree.NewElement("publish-info")
		fb2Place(desc, pub, fb2DescriptionOrder)
	}
	if pub != nil {
		fb2SetText(pub, "publisher", meta.Publisher, fb2PublishInfoOrder)
		fb2SetText(pub, "year", formatYear(meta.Year), fb2PublishInfoOrder)
		fb2SetText(pub, "isbn", isbn, fb2PublishInfoOrder)
	}
}

func fb2SetText(parent *etree.Element, tag, value, order string) {
	e := parent.SelectElement(tag)
	if e == nil && value != "" {
		e = etree.NewElement(tag)
		fb2Place(parent, e, order)
	}
	switch {
	case e == nil:
	case value == "":
		parent.RemoveChild(e)
	default:
		e.Child = nil
		e.SetText(value)
	}
}

// drops every child named tag and puts the fresh ones in their place of the schema order
func fb2Replace(parent *etree.Element, tag string, fresh []*etree.Element, order string) {
	for _, e := range parent.SelectElements(tag) {
		parent.RemoveChild(e)
	}
	for _, e := range fresh {
		fb2Place(parent, e, order)
	}
}

// inserts el before the first child that the schema order puts after it
func fb2Place(parent, el *etree.Element, order string) {
	var (
		names    = strings.Fields(order)
		rank     = slices.Index(names, el.Tag)
		children = parent.ChildElements()
		at       = len(parent.Child)
		i        int
	)

	for i < len(children) && slices.Index(names, children[i].Tag) <= rank {
		i++
	}
	if i < len(children) {
		at = children[i].Index()
	}
	parent.InsertChildAt(at, el)
}

func fb2EditCover(doc *etree.Document, info *etree.Element, data []byte) error {
	var (
		id       string
		cover    *etree.Element
		binaries = doc.Root().SelectElements("binary")
	)

	if img := info.FindElement("coverpage/image"); img != nil {
		id = strings.TrimPrefix(img.SelectAttrValue("href", ""), "#")
	}
	for i := 0; cover == nil && i < len(binaries); i++ {
		if binaries[i].SelectAttrValue("id", "") == id {
			cover = binaries[i]
		}
	}
	if cover == nil {
		return errors.New("fb2: the book has no cover to replace")
	}
	cover.CreateAttr("content-type", http.DetectContentType(data))
	cover.Child = nil
	cover.SetText(base64.StdEncoding.EncodeToString(data))
	return nil
}

// the file is always written as UTF-8, so a declaration of another encoding must go
func fb2SetEncoding(doc *etree.Document) {
	for _, token := range doc.Child {
		if pi, ok := token.(*etree.ProcInst); ok && pi.Target == "xml" {
			pi.Inst = `version="1.0" encoding="utf-8"`
		}
	}
}
