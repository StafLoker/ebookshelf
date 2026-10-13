package formats

import (
	"archive/zip"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path"
	"slices"
	"strings"

	"github.com/beevik/etree"

	"ebookshelf/internal/domain"
)

const (
	maxXMLBytes  = 16 * domain.MB
	roleAuthor   = "aut"
	opfNamespace = "http://www.idpf.org/2007/opf"
	mimetypeName = "mimetype"
)

type epub struct{}

func (epub) Read(file string) (*Metadata, error) {
	meta := &Metadata{}

	zr, err := zip.OpenReader(file)
	if err != nil {
		return nil, err
	}
	defer func() { _ = zr.Close() }()

	opf, opfDoc, err := epubFindOPF(&zr.Reader)
	if err != nil {
		return nil, err
	}
	if err := epubReadOPF(opfDoc, path.Dir(opf), &zr.Reader, meta); err != nil {
		return nil, err
	}
	if meta.Title == "" {
		return nil, errors.New("epub: missing title")
	}
	return meta, nil
}

func (epub) Write(src, dst string, meta *Metadata) error {
	zr, err := zip.OpenReader(src)
	if err != nil {
		return err
	}
	defer func() { _ = zr.Close() }()

	opf, opfDoc, err := epubFindOPF(&zr.Reader)
	if err != nil {
		return err
	}
	cover, err := epubEditOPF(opfDoc, path.Dir(opf), meta)
	if err != nil {
		return err
	}
	data, err := opfDoc.WriteToBytes()
	if err != nil {
		return err
	}
	replace := map[string][]byte{opf: data}
	if cover != "" {
		replace[cover] = meta.Cover
	}
	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	err = epubWriteZip(out, &zr.Reader, replace)
	if closeErr := out.Close(); err == nil {
		err = closeErr
	}
	return err
}

// ---- reading ----

// returns the zip name of the package document and its parsed content
func epubFindOPF(zr *zip.Reader) (string, *etree.Document, error) {
	contDoc := etree.NewDocument()
	opfDoc := etree.NewDocument()

	data, err := epubReadZipFile(zr, "META-INF/container.xml", maxXMLBytes)
	if err != nil {
		return "", nil, err
	}
	if err := contDoc.ReadFromBytes(data); err != nil {
		return "", nil, err
	}
	opf, err := epubRootfilePath(contDoc)
	if err != nil {
		return "", nil, err
	}
	data, err = epubReadZipFile(zr, opf, maxXMLBytes)
	if err != nil {
		return "", nil, err
	}
	if err := opfDoc.ReadFromBytes(data); err != nil {
		return "", nil, err
	}
	return opf, opfDoc, nil
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
		coverName string
		err       error
	)

	pkg := doc.SelectElement("package")
	if pkg == nil {
		err = errors.New("epub: no package element")
	} else {
		if md := pkg.SelectElement("metadata"); md != nil {
			epubReadMetadata(md, meta)
			coverID = epubCoverID(md)
		}
		if item := epubCoverItem(pkg.SelectElement("manifest"), coverID); item != nil {
			coverName = epubZipName(dir, item.SelectAttrValue("href", ""))
		}
	}
	if coverName != "" {
		// a missing cover is not an error: the book is still readable
		// one byte over the policy so the upload rejects an oversized cover instead of storing it cut
		meta.Cover, _ = epubReadZipFile(zr, coverName, domain.CoverPolicy.MaxSizeBytes+1)
	}
	return err
}

// a single-valued field is the first element with its tag, for both reading and writing
func epubReadMetadata(md *etree.Element, meta *Metadata) {
	meta.Title = epubText(md, "title")
	meta.Language = ParseLanguage(epubText(md, "language"))
	meta.Publisher = epubText(md, "publisher")
	if e := md.SelectElement("description"); e != nil {
		meta.Description = stripTags(allText(e))
	}
	for _, e := range md.ChildElements() {
		text := strings.TrimSpace(e.Text())
		switch e.Tag {
		case "creator":
			if epubIsAuthor(e) {
				meta.Authors = epubAppendAuthors(meta.Authors, text, e.SelectAttrValue("file-as", ""))
			}
		case "date":
			if meta.Year == 0 && epubIsPublicationDate(e) {
				meta.Year = parseYear(text)
			}
		case "subject":
			meta.Tags = appendNonEmpty(meta.Tags, text)
		case "identifier":
			meta.Identifiers = append(meta.Identifiers, ParseIdentifiers(e.SelectAttrValue("scheme", ""), text)...)
		}
	}
}

func epubText(md *etree.Element, tag string) string {
	var text string

	if e := md.SelectElement(tag); e != nil {
		text = strings.TrimSpace(e.Text())
	}
	return text
}

// one creator can name several authors: "Vicente Garrido & Nieves Abarca", file-as "Garrido, Vicente & Abarca, Nieves";
// the file-as is used only when it has one part per name
func epubAppendAuthors(list []domain.Author, name, fileAs string) []domain.Author {
	isSeparator := func(r rune) bool { return r == '&' || r == ';' }
	names, fileAses := strings.FieldsFunc(name, isSeparator), strings.FieldsFunc(fileAs, isSeparator)

	for i, n := range names {
		var fa string
		if len(fileAses) == len(names) {
			fa = fileAses[i]
		}
		list = appendAuthor(list, epubAuthor(n, fa))
	}
	return list
}

// a date without opf:event or of the publication; "creation" and "modification" are dates of the file
func epubIsPublicationDate(date *etree.Element) bool {
	event := date.SelectAttrValue("event", "")
	return event == "" || event == "publication"
}

// a creator without opf:role counts as an author
func epubIsAuthor(creator *etree.Element) bool {
	role := creator.SelectAttrValue("role", "")
	return role == "" || role == roleAuthor
}

// file-as "Last, First Middle", which this library writes, splits the name when it holds the same words;
// otherwise the last word of the name is the last name, unless the name ends in initials
func epubAuthor(name, fileAs string) domain.Author {
	var (
		author domain.Author
		words  = strings.Fields(name)
	)

	last, given, found := strings.Cut(fileAs, ",")
	switch {
	case found && slices.Equal(slices.Sorted(slices.Values(words)), slices.Sorted(slices.Values(strings.Fields(last+" "+given)))):
		author = newAuthor(given, "", last)
	case len(words) > 0:
		// "Толстой Л.Н.": trailing initials are the given names and the words before them the last name
		initials := len(words)
		for initials > 1 && strings.Contains(words[initials-1], ".") {
			initials--
		}
		if initials < len(words) {
			author = newAuthor(strings.Join(words[initials:], " "), "", strings.Join(words[:initials], " "))
		} else {
			author = newAuthor(strings.Join(words[:len(words)-1], " "), "", words[len(words)-1])
		}
	}
	return author
}

// the id of the cover item named by an EPUB 2 meta name="cover"; "" when there is none
func epubCoverID(md *etree.Element) string {
	var (
		id    string
		metas = md.SelectElements("meta")
	)

	for i := 0; id == "" && i < len(metas); i++ {
		if metas[i].SelectAttrValue("name", "") == "cover" {
			id = metas[i].SelectAttrValue("content", "")
		}
	}
	return id
}

func epubCoverItem(manifest *etree.Element, coverID string) *etree.Element {
	var (
		cover *etree.Element
		items []*etree.Element
	)

	if manifest != nil {
		items = manifest.ChildElements()
	}
	for i := 0; cover == nil && i < len(items); i++ {
		if coverID != "" && items[i].SelectAttrValue("id", "") == coverID {
			cover = items[i]
		}
	}
	return cover
}

// the manifest href is relative to the package document and percent-encoded; "" when it cannot be decoded
func epubZipName(dir, href string) string {
	var name string

	href, _, _ = strings.Cut(href, "#")
	if unescaped, err := url.PathUnescape(href); err == nil {
		name = path.Join(dir, unescaped)
	}
	return name
}

func epubReadZipFile(zr *zip.Reader, name string, limit int64) ([]byte, error) {
	f, err := zr.Open(name)
	if err != nil {
		return nil, fmt.Errorf("epub: open %s: %w", name, err)
	}
	defer func() { _ = f.Close() }()

	return io.ReadAll(io.LimitReader(f, limit))
}

// ---- writing ----

// returns the zip name of the cover to write, "" when meta has no cover
func epubEditOPF(doc *etree.Document, dir string, meta *Metadata) (string, error) {
	var (
		cover string
		md    *etree.Element
		err   error
	)

	pkg := doc.SelectElement("package")
	if pkg != nil {
		md = pkg.SelectElement("metadata")
	}
	if md == nil {
		err = errors.New("epub: no metadata element")
	} else {
		pkg.CreateAttr("xmlns:opf", opfNamespace)
		// title, language and authors are required by the format: an empty value never removes them
		if meta.Title != "" {
			epubSetText(md, "title", meta.Title)
		}
		if meta.Language != "" {
			epubSetText(md, "language", meta.Language)
		}
		if len(meta.Authors) > 0 {
			epubSetAuthors(md, meta.Authors)
		}
		epubSetText(md, "publisher", meta.Publisher)
		epubSetText(md, "description", meta.Description)
		epubSetPublicationDate(md, meta.Year)
		epubReplace(md, "subject", func(*etree.Element) bool { return true }, textElements("dc:subject", meta.Tags))
		epubSetIdentifiers(pkg, md, meta.Identifiers)
		if meta.Cover != nil {
			cover, err = epubEditCover(pkg.SelectElement("manifest"), md, dir, meta.Cover)
		}
	}
	return cover, err
}

func epubSetPublicationDate(md *etree.Element, year int) {
	var fresh []*etree.Element

	if year != 0 {
		date := etree.NewElement("dc:date")
		date.SetText(formatYear(year))
		date.CreateAttr("opf:event", "publication")
		fresh = append(fresh, date)
	}
	epubReplace(md, "date", epubIsPublicationDate, fresh)
}

func epubSetText(md *etree.Element, tag, value string) {
	e := md.SelectElement(tag)
	switch {
	case e == nil && value != "":
		md.CreateElement("dc:" + tag).SetText(value)
	case e == nil:
	case value == "":
		md.RemoveChild(e)
	default:
		e.Child = nil
		e.SetText(value)
	}
}

func epubSetAuthors(md *etree.Element, authors []domain.Author) {
	fresh := make([]*etree.Element, len(authors))
	for i, a := range authors {
		fresh[i] = etree.NewElement("dc:creator")
		fresh[i].SetText(a.Name())
		fresh[i].CreateAttr("opf:role", roleAuthor)
		fresh[i].CreateAttr("opf:file-as", a.SortName())
	}
	epubReplace(md, "creator", epubIsAuthor, fresh)
}

// the package names one identifier as its unique id: removing it would break the book, so it stays and is not added twice
func epubSetIdentifiers(pkg, md *etree.Element, ids []domain.Identifier) {
	var (
		uid    = pkg.SelectAttrValue("unique-identifier", "")
		unique []domain.Identifier
		fresh  []*etree.Element
	)

	isUnique := func(e *etree.Element) bool { return uid != "" && e.SelectAttrValue("id", "") == uid }
	for _, e := range md.SelectElements("identifier") {
		if isUnique(e) {
			unique = ParseIdentifiers(e.SelectAttrValue("scheme", ""), strings.TrimSpace(e.Text()))
		}
	}
	for _, id := range ids {
		if !slices.Contains(unique, id) {
			e := etree.NewElement("dc:identifier")
			e.SetText(id.Value)
			e.CreateAttr("opf:scheme", id.Type)
			fresh = append(fresh, e)
		}
	}
	epubReplace(md, "identifier", func(e *etree.Element) bool {
		return !isUnique(e) && len(ParseIdentifiers(e.SelectAttrValue("scheme", ""), strings.TrimSpace(e.Text()))) > 0
	}, fresh)
}

// drops the managed children named tag and puts the fresh ones where the first dropped one was
func epubReplace(md *etree.Element, tag string, managed func(*etree.Element) bool, fresh []*etree.Element) {
	at := len(md.Child)

	for _, e := range md.SelectElements(tag) {
		if managed(e) {
			at = min(at, e.Index())
			md.RemoveChild(e)
		}
	}
	for i, e := range fresh {
		md.InsertChildAt(at+i, e)
	}
}

// the cover is the manifest item named by meta name="cover"
func epubEditCover(manifest, md *etree.Element, dir string, data []byte) (string, error) {
	item := epubCoverItem(manifest, epubCoverID(md))

	if item == nil {
		return "", errors.New("epub: the book has no cover to replace")
	}
	item.CreateAttr("media-type", http.DetectContentType(data))
	name := epubZipName(dir, item.SelectAttrValue("href", ""))
	if name == "" {
		return "", errors.New("epub: invalid cover href")
	}
	return name, nil
}

// mimetype goes first and uncompressed, as the format requires; every other entry is copied as it was
// unless replace holds new content for it
func epubWriteZip(out io.Writer, zr *zip.Reader, replace map[string][]byte) error {
	zw := zip.NewWriter(out)

	err := epubAddFile(zw, mimetypeName, []byte("application/epub+zip"), zip.Store)
	for i := 0; err == nil && i < len(zr.File); i++ {
		f := zr.File[i]
		data, changed := replace[f.Name]
		switch {
		case f.Name == mimetypeName:
		case changed:
			err = epubAddFile(zw, f.Name, data, zip.Deflate)
		default:
			err = zw.Copy(f)
		}
	}
	if closeErr := zw.Close(); err == nil {
		err = closeErr
	}
	return err
}

func epubAddFile(zw *zip.Writer, name string, data []byte, method uint16) error {
	w, err := zw.CreateHeader(&zip.FileHeader{Name: name, Method: method})
	if err != nil {
		return err
	}
	_, err = w.Write(data)
	return err
}
