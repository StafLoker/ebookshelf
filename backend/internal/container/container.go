package container

import (
	"github.com/jmoiron/sqlx"

	"ebookshelf/internal/config"
	"ebookshelf/internal/repository"
	"ebookshelf/internal/service"
)

type Container struct {
	AuthSvc *service.AuthService
	UserSvc *service.UserService
	BookSvc *service.BookService
	Files   *service.FileService
}

func New(db *sqlx.DB, cfg *config.Config) *Container {
	userRepo := repository.NewUserRepository(db)
	tokenRepo := repository.NewPasswordResetTokenRepository(db)
	bookRepo := repository.NewBookRepository(db)
	files := service.NewFileService(cfg.DataDir)

	return &Container{
		AuthSvc: service.NewAuthService(userRepo, tokenRepo, cfg.SecretKey),
		UserSvc: service.NewUserService(userRepo, tokenRepo),
		BookSvc: service.NewBookService(bookRepo, repository.NewReadStatusRepository(db), files),
		Files:   files,
	}
}
