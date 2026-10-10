package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/jmoiron/sqlx"

	"ebookshelf/internal/api"
	"ebookshelf/internal/config"
	"ebookshelf/internal/container"
	"ebookshelf/internal/database"
	apperrors "ebookshelf/internal/errors"
	"ebookshelf/internal/logging"
)

const (
	defaultConfigPath = "/etc/ebookshelf/config.ini"
	dataDirMode       = 0o755
	shutdownTimeout   = 10 * time.Second
	headerTimeout     = 10 * time.Second
	idleTimeout       = 120 * time.Second
	startAttempts     = 3
	startDelay        = time.Second
	usageText         = `Usage: app [-c config] [--version] [command]

Commands:
  serve                  start the server (default)
  root create <username> create the root user and print its password link
  root reset-password    print a new password link for the root user

Flags:
  -c string   config file path (default ` + defaultConfigPath + `)
  --version   print the version and exit
`
)

var version = "dev"

type cli struct {
	configPath string
	rest       []string
}

func main() {
	var (
		cfg     *config.Config
		logger  *slog.Logger
		db      *sqlx.DB
		ctx     context.Context
		id      string
		err     error
		logOut  = os.Stdout
		cmdArgs = parseCLI()
		code    = 0
	)

	if cfg, err = config.Load(cmdArgs.configPath); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	if cmdArgs.rest[0] == "root" {
		logOut = os.Stderr
	}
	if logger, err = logging.New(logOut, cfg.LogLevel); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	slog.SetDefault(logger)

	if id, err = logging.NewID(); err != nil {
		slog.Error("error generating command id", "err", err)
		os.Exit(1)
	}
	ctx = logging.WithCorrelation(context.Background(), "cmd", id)

	if err = os.MkdirAll(cfg.DataDir, dataDirMode); err != nil {
		slog.ErrorContext(ctx, "error creating data dir", "err", err)
		os.Exit(1)
	}
	err = retry(ctx, "open database", func() (e error) {
		db, e = database.Open(ctx, cfg.DataDir+"/app.db", logger)
		return e
	})
	if err != nil {
		slog.ErrorContext(ctx, "error opening database", "err", err)
		os.Exit(1)
	}

	c := container.New(db, cfg)
	switch cmdArgs.rest[0] {
	case "serve":
		c.Files.ClearTmp(ctx)
		if runServer(ctx, cfg, c) != nil {
			code = 1
		}
	case "root":
		code = runRoot(ctx, c, cmdArgs.rest[1:])
	}

	if err = db.Close(); err != nil {
		slog.WarnContext(ctx, "error closing database", "err", err)
	}
	os.Exit(code)
}

func parseCLI() cli {
	var (
		args        cli
		showVersion = flag.Bool("version", false, "print version and exit")
	)

	flag.Usage = func() { fmt.Fprint(os.Stderr, usageText) }
	flag.StringVar(&args.configPath, "c", defaultConfigPath, "config file path")
	flag.Parse()

	if *showVersion {
		fmt.Println(version)
		os.Exit(0)
	}

	args.rest = flag.Args()
	if len(args.rest) == 0 {
		args.rest = []string{"serve"}
	}
	valid := args.rest[0] == "serve" && len(args.rest) == 1 ||
		args.rest[0] == "root" && (len(args.rest) == 3 && args.rest[1] == "create" || len(args.rest) == 2 && args.rest[1] == "reset-password")
	if !valid {
		flag.Usage()
		os.Exit(2)
	}
	return args
}

func retry(ctx context.Context, what string, fn func() error) error {
	var (
		delay = startDelay
		err   = fn()
	)

	for attempt := 1; err != nil && attempt < startAttempts; attempt++ {
		slog.WarnContext(ctx, what+" failed, retrying", "attempt", attempt, "err", err)
		time.Sleep(delay)
		delay *= 2
		err = fn()
	}
	return err
}

func runServer(ctx context.Context, cfg *config.Config, c *container.Container) error {
	var (
		ln           net.Listener
		err          error
		errCh        = make(chan error, 1)
		srv          = &http.Server{Handler: api.NewRouter(c), ReadHeaderTimeout: headerTimeout, IdleTimeout: idleTimeout}
		sigCtx, stop = signal.NotifyContext(ctx, syscall.SIGINT, syscall.SIGTERM)
	)
	defer stop()

	err = retry(ctx, "listen", func() (e error) {
		ln, e = net.Listen("tcp", fmt.Sprintf(":%d", cfg.Port))
		return e
	})
	if err == nil {
		go func() { errCh <- srv.Serve(ln) }()
		slog.InfoContext(ctx, "Server started", "port", cfg.Port)

		select {
		case err = <-errCh:
		case <-sigCtx.Done():
			shutdownCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), shutdownTimeout)
			defer cancel()
			err = srv.Shutdown(shutdownCtx)
		}
	}

	if err != nil {
		slog.ErrorContext(ctx, "server failed", "err", err)
	} else {
		slog.InfoContext(ctx, "Server stopped")
	}
	return err
}

func runRoot(ctx context.Context, c *container.Container, args []string) int {
	var (
		token, username string
		err             error
		code            = 1
	)

	if args[0] == "create" {
		username = args[1]
		token, err = c.UserSvc.CreateRoot(ctx, username)
	} else {
		username, token, err = c.UserSvc.ResetRootPassword(ctx)
	}

	switch {
	case err == nil:
		code = 0
		fmt.Println("/reset-password#token=" + token)
		fmt.Fprintf(os.Stderr, "open this link on the site to set the password of %q\n", username)
	case errors.Is(err, apperrors.ErrInvalidRequest):
		code = 2
		fmt.Fprintln(os.Stderr, err)
	case errors.Is(err, apperrors.ErrRootExists):
		fmt.Fprintln(os.Stderr, "a root user already exists; use \"root reset-password\" to recover access")
	case errors.Is(err, apperrors.ErrNoRoot):
		fmt.Fprintln(os.Stderr, "there is no root user; create one with \"root create <username>\"")
	default:
		fmt.Fprintln(os.Stderr, err)
	}
	return code
}
