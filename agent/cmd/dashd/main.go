// Command dashd is a small agent that exposes a server's health, services and
// logs through a web dashboard.
package main

import (
	"bufio"
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/mdp/qrterminal/v3"
	"golang.org/x/term"

	"github.com/findiepman/dashd/internal/api"
	"github.com/findiepman/dashd/internal/audit"
	"github.com/findiepman/dashd/internal/auth"
	"github.com/findiepman/dashd/internal/config"
	"github.com/findiepman/dashd/internal/host"
	"github.com/findiepman/dashd/internal/provider"
	"github.com/findiepman/dashd/internal/providers/docker"
	"github.com/findiepman/dashd/internal/providers/mock"
	"github.com/findiepman/dashd/internal/providers/pterodactyl"
	"github.com/findiepman/dashd/internal/providers/systemd"
	"github.com/findiepman/dashd/internal/webui"
)

const usage = `dashd, a server dashboard agent

Usage:
  dashd [serve] [-config path]     run the dashboard (default config: config.yaml)
  dashd init [-o path] [-mock]     create a config with a new login and TOTP secret
  dashd hash-password              hash a password for auth.password_hash
  dashd totp-code -secret S        print the current code for a secret (for testing)
`

func main() {
	args := os.Args[1:]
	cmd := "serve"
	if len(args) > 0 && !strings.HasPrefix(args[0], "-") {
		cmd, args = args[0], args[1:]
	}
	var err error
	switch cmd {
	case "serve":
		err = serve(args)
	case "init":
		err = initConfig(args)
	case "hash-password":
		err = hashPassword()
	case "totp-code":
		err = totpCode(args)
	case "help", "-h", "--help":
		fmt.Print(usage)
	default:
		fmt.Fprint(os.Stderr, usage)
		os.Exit(2)
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "dashd:", err)
		os.Exit(1)
	}
}

func serve(args []string) error {
	fs := flag.NewFlagSet("serve", flag.ExitOnError)
	path := fs.String("config", "config.yaml", "path to config file")
	fs.Parse(args)

	log := slog.New(slog.NewTextHandler(os.Stderr, nil))
	cfg, err := config.Load(*path)
	if err != nil {
		return err
	}
	if !cfg.ListenIsLoopback() {
		log.Warn("listening on a non-loopback address; put dashd behind cloudflared or a reverse proxy with TLS", "listen", cfg.Listen)
	}
	if cfg.Auth.InsecureCookie {
		log.Warn("insecure_cookie is on; only use this for local development")
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	reg := provider.NewRegistry()
	var collector host.Collector
	if cfg.Mock {
		log.Info("mock mode: all data is fake")
		collector = host.NewMock()
		for _, p := range mock.Set(ctx) {
			must(reg.Add(p))
		}
	} else {
		collector = host.NewSystem()
		if c := cfg.Providers.Docker; c != nil {
			must(reg.Add(docker.New(*c)))
		}
		if c := cfg.Providers.Pterodactyl; c != nil {
			must(reg.Add(pterodactyl.New(*c)))
		}
		if c := cfg.Providers.Systemd; c != nil {
			must(reg.Add(systemd.New(*c)))
		}
	}

	auditLog, err := audit.Open(cfg.Audit.Path)
	if err != nil {
		return fmt.Errorf("open audit log: %w", err)
	}
	defer auditLog.Close()

	hub := api.NewHub(reg, collector, log)
	go hub.Run(ctx)

	srv := &api.Server{
		Hostname:       cfg.Hostname,
		InsecureCookie: cfg.Auth.InsecureCookie,
		Auth:           auth.New(cfg.Auth),
		Registry:       reg,
		Hub:            hub,
		Audit:          auditLog,
		Static:         webui.FS(),
		Log:            log,
	}
	httpSrv := &http.Server{
		Addr:              cfg.Listen,
		Handler:           srv.Handler(),
		ReadHeaderTimeout: 10 * time.Second,
	}
	go func() {
		<-ctx.Done()
		sctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		httpSrv.Shutdown(sctx)
	}()

	names := []string{}
	for _, p := range reg.Providers() {
		names = append(names, p.Name())
	}
	log.Info("dashd listening", "addr", cfg.Listen, "providers", strings.Join(names, ","))
	if err := httpSrv.ListenAndServe(); !errors.Is(err, http.ErrServerClosed) {
		return err
	}
	return nil
}

func must(err error) {
	if err != nil {
		panic(err)
	}
}

// stdin is shared so piped input read line by line isn't lost to buffering.
var stdin = bufio.NewReader(os.Stdin)

func readPassword(prompt string) (string, error) {
	fmt.Fprint(os.Stderr, prompt)
	if term.IsTerminal(int(os.Stdin.Fd())) {
		b, err := term.ReadPassword(int(os.Stdin.Fd()))
		fmt.Fprintln(os.Stderr)
		return string(b), err
	}
	line, err := stdin.ReadString('\n')
	if errors.Is(err, io.EOF) && line != "" {
		err = nil
	}
	return strings.TrimRight(line, "\r\n"), err
}

func hashPassword() error {
	pw, err := readPassword("Password: ")
	if err != nil {
		return err
	}
	h, err := auth.HashPassword(pw)
	if err != nil {
		return err
	}
	fmt.Println(h)
	return nil
}

func totpCode(args []string) error {
	fs := flag.NewFlagSet("totp-code", flag.ExitOnError)
	secret := fs.String("secret", "", "base32 TOTP secret")
	fs.Parse(args)
	code, err := auth.TOTPCode(*secret, time.Now())
	if err != nil {
		return err
	}
	fmt.Println(code)
	return nil
}

func initConfig(args []string) error {
	fs := flag.NewFlagSet("init", flag.ExitOnError)
	out := fs.String("o", "config.yaml", "where to write the config")
	user := fs.String("user", "", "login username")
	mockMode := fs.Bool("mock", false, "write a local development config with fake data")
	fs.Parse(args)

	if _, err := os.Stat(*out); err == nil {
		return fmt.Errorf("%s already exists; move it away first", *out)
	}
	if *user == "" {
		fmt.Fprint(os.Stderr, "Username: ")
		line, err := stdin.ReadString('\n')
		if err != nil {
			return err
		}
		*user = strings.TrimSpace(line)
	}
	if *user == "" {
		return errors.New("username can't be empty")
	}
	pw, err := readPassword("Password (12+ characters): ")
	if err != nil {
		return err
	}
	if len(pw) < 12 && !*mockMode {
		return errors.New("use at least 12 characters; this login guards root-level access")
	}
	hash, err := auth.HashPassword(pw)
	if err != nil {
		return err
	}
	secret, err := auth.NewTOTPSecret()
	if err != nil {
		return err
	}

	var b strings.Builder
	fmt.Fprintf(&b, "listen: 127.0.0.1:7070\n")
	if *mockMode {
		b.WriteString("mock: true\n")
	}
	fmt.Fprintf(&b, "\nauth:\n  username: %s\n  password_hash: %q\n  totp_secret: %s\n  session_ttl: 12h\n", *user, hash, secret)
	if *mockMode {
		b.WriteString("  insecure_cookie: true\n")
	}
	b.WriteString("\naudit:\n  path: audit.jsonl\n")
	if !*mockMode {
		b.WriteString(`
providers:
  docker:
    socket: /var/run/docker.sock
  systemd:
    units: [wings, playit, pteroq, cloudflared]
  # pterodactyl:
  #   url: https://panel.findiepman.dev
  #   api_key: env:DASHD_PTERODACTYL_KEY
`)
	}
	if err := os.WriteFile(*out, []byte(b.String()), 0o600); err != nil {
		return err
	}

	uri := auth.TOTPURI(secret, *user, "dashd")
	fmt.Fprintf(os.Stderr, "\nWrote %s\n\nScan this with your authenticator app:\n\n", *out)
	qrterminal.GenerateHalfBlock(uri, qrterminal.L, os.Stderr)
	fmt.Fprintf(os.Stderr, "\nOr enter the secret by hand: %s\n", secret)
	return nil
}
