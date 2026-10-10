package formats

import (
	"archive/zip"
	"errors"
	"fmt"
	"io"
	"net/url"
	"path"
	"strings"

	"github.com/beevik/etree"

	"ebookshelf/internal/domain"
)

const (
	containerPath = "META-INF/container.xml"
	maxXMLBytes   = 16 * domain.MB
	roleAuthor    = "aut"
)

type epub struct{}

func (epub) Read(file string) (*Metadata, error) {
	var (
		meta    = &Metadata{}
		opfDoc  = etree.NewDocument()
		contDoc = etree.NewDocument()
		zr      *zip.ReadCloser
		data    []byte
		opfPath string
		err     error
	)

	if zr, err = zip.OpenReader(file); err == nil {
		defer func() { _ = zr.Close() }()
		data, err = epubReadZipFile(&zr.Reader, containerPath, maxXMLBytes)
	}
	if err == nil {
		err = contDoc.ReadFromBytes(data)
	}
	if err == nil {
		opfPath, err = epubRootfilePath(contDoc)
	}
	if err == nil {
		data, err = epubReadZipFile(&zr.Reader, opfPath, maxXMLBytes)
	}
	if err == nil {
		err = opfDoc.ReadFromBytes(data)
	}
	if err == nil {
		err = epubReadOPF(opfDoc, path.Dir(opfPath), &zr.Reader, meta)
	}
	if err == nil && meta.Title == "" {
		err = errors.New("epub: missing title")
	}
	if err != nil {
		meta = nil
	}
	return meta, err
}

func epubRootfilePath(doc *etree.Document) (string, error) {
	rootfile := doc.FindElement("//rootfile")
	if rootfile == nil {
		return "", errors.New("epub: no rootfile in container.xml")
	}
	return rootfile.SelectAttrValue("full-path", ""), nil
}

func epubReadOPF(doc *etree.Document, dir string, zr *zip.Reader, meta *Metadata) error {
	var (
		coverID   string
		coverHref string
		err       error
	)

	pkg := doc.SelectElement("package")
	if pkg == nil {
		err = errors.New("epub: no package element")
	} else {
		if md := pkg.SelectElement("metadata"); md != nil {
			coverID = epubReadMetadata(md, meta)
		}
		coverHref = epubCoverHref(pkg.SelectElement("manifest"), coverID)
	}
	if coverHref != "" {
		// a missing cover is not an error: the book is still readable
		href, _, _ := strings.Cut(coverHref, "#")
		// one byte over the policy so the upload rejects an oversized cover instead of storing it cut
		if unescaped, unescapeErr := url.PathUnescape(href); unescapeErr == nil {
			meta.Cover, _ = epubReadZipFile(zr, path.Join(dir, unescaped), domain.CoverPolicy.MaxSizeBytes+1)
		}
	}
	return err
}

func epubReadMetadata(md *etree.Element, meta *Metadata) string {
	var coverID string

	for _, e := range md.ChildElements() {
		text := strings.TrimSpace(e.Text())
		switch e.Tag {
		case "title":
			if meta.Title == "" {
				meta.Title = text
			}
		case "creator":
			if role := e.SelectAttrValue("role", ""); role == "" || role == roleAuthor {
				meta.Authors = appendNonEmpty(meta.Authors, text)
			}
		case "language":
			if meta.Language == "" {
				meta.Language = parseLanguage(text)
			}
		case "publisher":
			meta.Publisher = text
		case "date":
			if meta.Year == 0 {
				meta.Year = parseYear(text)
			}
		case "description":
			meta.Description = stripTags(allText(e))
		case "subject":
			meta.Tags = appendNonEmpty(meta.Tags, text)
		case "identifier":
			meta.Identifiers = append(meta.Identifiers, parseIdentifiers(e.SelectAttrValue("scheme", ""), text)...)
		case "meta":
			if e.SelectAttrValue("name", "") == "cover" && coverID == "" {
				coverID = e.SelectAttrValue("content", "")
			}
		}
	}
	return coverID
}

func epubCoverHref(manifest *etree.Element, coverID string) string {
	var (
		href  string
		items []*etree.Element
	)

	if manifest != nil {
		items = manifest.ChildElements()
	}
	for i := 0; href == "" && i < len(items); i++ {
		isCover := coverID != "" && items[i].SelectAttrValue("id", "") == coverID ||
			strings.Contains(items[i].SelectAttrValue("properties", ""), "cover-image")
		if isCover {
			href = items[i].SelectAttrValue("href", "")
		}
	}
	return href
}

func epubReadZipFile(zr *zip.Reader, name string, limit int64) ([]byte, error) {
	f, err := zr.Open(name)
	if err != nil {
		return nil, fmt.Errorf("epub: open %s: %w", name, err)
	}
	defer func() { _ = f.Close() }()

	return io.ReadAll(io.LimitReader(f, limit))
}
