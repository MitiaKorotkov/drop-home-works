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

	flagSet := flag.NewFlagSet("create-teacher", flag.ExitOnError)
	flagSet.StringVar(&teacher.Login, "login", "", "teacher's login")
	flagSet.StringVar(&teacher.FirstName, "first-name", "", "teacher's first name")
	flagSet.StringVar(&teacher.LastName, "last-name", "", "teacher's last name")
	flagSet.Usage = func() {
		fmt.Fprintln(flagSet.Output(), "Usage: server create-teacher -login LOGIN -first-name NAME -last-name NAME")
		fmt.Fprintln(flagSet.Output(), "The password is read from stdin: typed twice in a terminal, or the first line of piped input.")
		flagSet.PrintDefaults()
	}
	flagSet.Parse(args)

	if _, err := account.NormalizeLogin(teacher.Login); err != nil {
		return fmt.Errorf("login: %w", err)
	}
	if _, err := account.NormalizeName(teacher.FirstName); err != nil {
		return fmt.Errorf("first name: %w", err)
	}
	if _, err := account.NormalizeName(teacher.LastName); err != nil {
		return fmt.Errorf("last name: %w", err)
	}

	cfg, err := config.Load(os.LookupEnv)
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}

	password, err := readPassword(os.Stdin)
	if err != nil {
		return err
	}
	teacher.Password = password

	ctx, stop := signal.NotifyContext(ctx, os.Interrupt, syscall.SIGTERM)
	defer stop()

	logger := newLogger(os.Stderr, cfg.LogFormat, cfg.LogLevel)
	slog.SetDefault(logger)

	pool, err := openDB(ctx, cfg.DatabaseURL, logger)
	if err != nil {
		return err
	}
	defer pool.Close()

	id, err := account.CreateTeacher(ctx, db.New(pool), teacher)
	if err != nil {
		return err
	}

	fmt.Fprintf(os.Stderr, "teacher created, id %d\n", id)
	return nil
}
