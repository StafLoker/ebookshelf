package formats

import (
	"cmp"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/beevik/etree"
	"golang.org/x/text/encoding/htmlindex"
)

type fb2 struct{}

func (fb2) Read(path string) (*Metadata, error) {
	var (
		meta = &Metadata{}
		doc  = etree.NewDocument()
		info *etree.Element
		data []byte
		err  error
	)

	if data, err = os.ReadFile(path); err == nil {
		doc.ReadSettings.CharsetReader = fb2CharsetReader
		if err = doc.ReadFromBytes(data); err != nil {
			err = fmt.Errorf("fb2: parse: %w", err)
		}
	}
	if err == nil {
		if info = doc.FindElement("//description/title-info"); info == nil {
			err = errors.New("fb2: no title-info")
		}
	}
	if err == nil {
		fb2ReadTitleInfo(info, meta)
		fb2ReadPublishInfo(doc, meta)
		meta.Cover = fb2Cover(doc, info)
		if meta.Title == "" {
			err = errors.New("fb2: missing title")
		}
	}
	if err != nil {
		meta = nil
	}
	return meta, err
}

func fb2CharsetReader(label string, input io.Reader) (io.Reader, error) {
	enc, err := htmlindex.Get(label)
	if err != nil {
		return nil, err
	}
	return enc.NewDecoder().Reader(input), nil
}

func fb2ReadTitleInfo(info *etree.Element, meta *Metadata) {
	meta.Title = fb2ChildText(info, "book-title")
	meta.Language = parseLanguage(fb2ChildText(info, "lang"))
	if date := info.SelectElement("date"); date != nil {
		meta.Year = parseYear(cmp.Or(date.SelectAttrValue("value", ""), strings.TrimSpace(date.Text())))
	}
	for _, e := range info.ChildElements() {
		switch e.Tag {
		case "author":
			meta.Authors = appendNonEmpty(meta.Authors, fb2AuthorName(e))
		case "genre":
			meta.Tags = appendNonEmpty(meta.Tags, e.Text())
		case "annotation":
			meta.Description = fb2Annotation(e)
		}
	}
}

func fb2ReadPublishInfo(doc *etree.Document, meta *Metadata) {
	if pub := doc.FindElement("//description/publish-info"); pub != nil {
		meta.Publisher = fb2ChildText(pub, "publisher")
		meta.Identifiers = append(meta.Identifiers, parseIdentifiers("isbn", fb2ChildText(pub, "isbn"))...)
		if y := parseYear(fb2ChildText(pub, "year")); y != 0 {
			meta.Year = y
		}
	}
}

func fb2AuthorName(e *etree.Element) string {
	name := fb2ChildText(e, "first-name") + " " + fb2ChildText(e, "middle-name") + " " + fb2ChildText(e, "last-name")
	return cmp.Or(strings.Join(strings.Fields(name), " "), fb2ChildText(e, "nickname"))
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
