package main

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"github.com/MitiaKorotkov/drop-home-works/internal/account"
	"github.com/MitiaKorotkov/drop-home-works/internal/config"
	"github.com/MitiaKorotkov/drop-home-works/internal/db"
	"github.com/jackc/pgx/v5/pgxpool"
	"golang.org/x/term"
)

func readPassword(stdin *os.File) (string, error) {
	fd := int(stdin.Fd())

	if !term.IsTerminal(fd) {
		line, err := bufio.NewReader(stdin).ReadString('\n')
		if err != nil && !errors.Is(err, io.EOF) {
			return "", fmt.Errorf("read password: %w", err)
		}
		return strings.TrimRight(line, "\r\n"), nil
	}

	fmt.Fprint(os.Stderr, "Password: ")
	passwordTry1, err := term.ReadPassword(fd)
	fmt.Fprint(os.Stderr, "\n")
	if err != nil {
		return "", fmt.Errorf("read password: %w", err)
	}

	fmt.Fprint(os.Stderr, "Repeat the password: ")
	passwordTry2, err := term.ReadPassword(fd)
	fmt.Fprint(os.Stderr, "\n")
	if err != nil {
		return "", fmt.Errorf("read password: %w", err)
	}

	if !bytes.Equal(passwordTry1, passwordTry2) {
		return "", errors.New("passwords do not match")
	}

	return string(passwordTry1), nil
}

func runCreateTeacher(ctx context.Context, args []string) error {
	var teacher account.TeacherInput

	flagSet := flag.NewFlagSet("create-teacher", flag.ContinueOnError)
	flagSet.StringVar(&teacher.Login, "login", "", "teachers login")
	flagSet.StringVar(&teacher.FirstName, "first-name", "", "teachers first name")
	flagSet.StringVar(&teacher.LastName, "last-name", "", "teachers last name")
	if err := flagSet.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return nil
		}

		return err
	}

	if _, err := account.NormalizeLogin(teacher.Login); err != nil {
		return fmt.Errorf("login: %w", err)
	}
	if _, err := account.NormalizeName(teacher.FirstName); err != nil {
		return fmt.Errorf("first name: %w", err)
	}
	if _, err := account.NormalizeName(teacher.LastName); err != nil {
		return fmt.Errorf("last name: %w", err)
	}

	// 0. Context
	ctx, stop := signal.NotifyContext(ctx, os.Interrupt, syscall.SIGTERM)
	defer stop()

	// 1. Config
	cfg, err := config.Load(os.LookupEnv)
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}

	password, err := readPassword(os.Stdin)
	if err != nil {
		return err
	}
	teacher.Password = password

	// 2. Logger
	logger := newLogger(os.Stderr, cfg.LogFormat, cfg.LogLevel)
	slog.SetDefault(logger)
	logger.Info("config loaded", "addr", cfg.HTTPAddress, "storage_dir", cfg.StorageDir)

	// 3. Pgx pool
	pool, err := pgxpool.New(ctx, cfg.DatabaseURL)
	if err != nil {
		return fmt.Errorf("create db pool: %w", err)
	}
	defer pool.Close()

	if err := db.Migrate(ctx, pool, logger); err != nil {
		return fmt.Errorf("migrate: %w", err)
	}

	id, err := account.CreateTeacher(ctx, db.New(pool), teacher)
	if err != nil {
		return err
	}

	fmt.Fprintf(os.Stderr, "teacher created, id %d\n", id)
	return nil
}
