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
}

func New(db *sqlx.DB, cfg *config.Config) *Container {
	userRepo := repository.NewUserRepository(db)
	tokenRepo := repository.NewPasswordResetTokenRepository(db)

	return &Container{
		AuthSvc: service.NewAuthService(userRepo, tokenRepo, cfg.SecretKey),
		UserSvc: service.NewUserService(userRepo, tokenRepo),
	}
}
