package domain

const (
	MB       = 1 << 20
	BookEPUB = "epub"
	BookFB2  = "fb2"
	CoverJPG = "jpg"
	CoverPNG = "png"
)

type UploadPolicy struct {
	AllowedExts  []string
	MaxSizeBytes int64
}

var BookFilePolicy = UploadPolicy{
	AllowedExts:  []string{BookEPUB, BookFB2},
	MaxSizeBytes: 100 * MB,
}

var CoverPolicy = UploadPolicy{
	AllowedExts:  []string{CoverJPG, CoverPNG},
	MaxSizeBytes: 10 * MB,
}
