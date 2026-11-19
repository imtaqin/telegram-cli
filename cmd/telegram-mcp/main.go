// telegram-mcp exposes the user's Telegram account as MCP tools over stdio.
//
// Subcommands:
//
//	serve  (default) run the MCP server on stdin/stdout
//	login        interactive phone login, writes the MCP session file
//	login --qr   QR login via an already authorized Telegram app
//	              (~/.local/share/tele-tui/session-mcp.json by default)
package main

import (
	"bufio"
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"log"
	"os"
	"os/signal"
	"strings"
	"sync"
	"syscall"
	"time"

	"golang.org/x/term"

	"github.com/imtaqin/telegram-cli/internal/config"
	"github.com/imtaqin/telegram-cli/internal/mcpserver"
	"github.com/imtaqin/telegram-cli/internal/telegram"
	"github.com/imtaqin/telegram-cli/internal/ui/widgets"
)

const loginHint = "session not authorized, run 'telegram-mcp login' first"

func main() {
	// stdout is reserved for JSON-RPC — everything else goes to stderr.
	log.SetOutput(os.Stderr)
	log.SetPrefix("telegram-mcp: ")

	opts, err := parseCommand(os.Args[1:])
	if errors.Is(err, flag.ErrHelp) {
		printUsage(os.Stdout)
		return
	}
	if err != nil {
		fmt.Fprintf(os.Stderr, "telegram-mcp: %v\n", err)
		printUsage(os.Stderr)
		os.Exit(2)
	}

	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("failed to load config: %v", err)
	}
	if cfg.Telegram.APIID == 0 || cfg.Telegram.APIHash == "" {
		log.Fatalf("missing Telegram API credentials in config (run tele-tui once first, or edit ~/.config/tele-tui/config.toml)")
	}

	// The MCP server uses its own session file by default: sharing one
	// Telegram session between the TUI and (possibly several) MCP server
	// processes splits updates between the connections and breaks
	// realtime delivery. Override with TELETUI_SESSION if ever needed.
	if s := os.Getenv("TELETUI_SESSION"); s != "" {
		cfg.Storage.SessionFile = s
	} else {
		cfg.Storage.SessionFile = strings.TrimSuffix(cfg.Storage.SessionFile, ".json") + "-mcp.json"
	}

	switch opts.command {
	case "login":
		if opts.qr {
			runQRLogin(cfg)
		} else {
			runLogin(cfg)
		}
	case "serve":
		runServe(cfg)
	}
}

type commandOptions struct {
	command string
	qr      bool
}

func parseCommand(args []string) (commandOptions, error) {
	opts := commandOptions{command: "serve"}
	if len(args) > 0 && !strings.HasPrefix(args[0], "-") {
		opts.command = args[0]
		args = args[1:]
	}

	switch opts.command {
	case "login", "serve":
	default:
		return commandOptions{}, fmt.Errorf("unknown command %q", opts.command)
	}

	fs := flag.NewFlagSet("telegram-mcp "+opts.command, flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	fs.BoolVar(&opts.qr, "qr", false, "log in by scanning a QR code")
	if err := fs.Parse(args); err != nil {
		return commandOptions{}, err
	}
	if fs.NArg() != 0 {
		return commandOptions{}, fmt.Errorf("unexpected argument %q", fs.Arg(0))
	}
	if opts.qr && opts.command != "login" {
		return commandOptions{}, errors.New("--qr is only valid with the login command")
	}
	return opts, nil
}

func printUsage(w io.Writer) {
	fmt.Fprintln(w, "usage: telegram-mcp [login [--qr]|serve]")
}

// runLogin performs interactive authentication in the terminal and
// writes the MCP server's own session file.
func runLogin(cfg *config.Config) {
	authorizer := telegram.NewTUIAuthorizer(cfg)
	client := telegram.NewClientAsync(cfg, authorizer)
	defer client.Close()

	var mu sync.Mutex
	state := telegram.AuthStateWaitPhone

	authorizer.SetStateCallback(func(s telegram.AuthState, hint string) {
		mu.Lock()
		state = s
		mu.Unlock()

		switch s {
		case telegram.AuthStateWaitPhone:
			fmt.Fprintln(os.Stderr, "Enter phone number (with country code):")
		case telegram.AuthStateWaitCode:
			fmt.Fprintln(os.Stderr, "Enter the login code:")
		case telegram.AuthStateWaitPassword:
			if hint != "" {
				fmt.Fprintf(os.Stderr, "Enter 2FA password (hint: %s):\n", hint)
			} else {
				fmt.Fprintln(os.Stderr, "Enter 2FA password:")
			}
		}
	})
	authorizer.SetErrorCallback(func(err error) {
		fmt.Fprintf(os.Stderr, "auth error: %v (retrying)\n", err)
	})

	// Feed stdin lines into the authorizer according to the current state.
	go func() {
		scanner := bufio.NewScanner(os.Stdin)
		for scanner.Scan() {
			value := strings.TrimSpace(scanner.Text())
			if value == "" {
				continue
			}
			mu.Lock()
			s := state
			mu.Unlock()
			switch s {
			case telegram.AuthStateWaitPhone:
				authorizer.SubmitPhone(value)
			case telegram.AuthStateWaitCode:
				authorizer.SubmitCode(value)
			case telegram.AuthStateWaitPassword:
				authorizer.SubmitPassword(value)
			}
		}
	}()

	ready := make(chan struct{})
	go func() {
		client.WaitReady()
		close(ready)
	}()

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, os.Interrupt, syscall.SIGTERM)

	select {
	case <-ready:
	case <-sigCh:
		os.Exit(130)
	}

	me, err := client.GetMe()
	if err != nil {
		log.Fatalf("login succeeded but GetMe failed: %v", err)
	}

	printLoggedIn(me)
}

func runQRLogin(cfg *config.Config) {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	me, err := telegram.LoginWithQR(ctx, cfg, telegram.QRLoginOptions{
		ShowQRCode: func(_ context.Context, token telegram.QRLoginToken) error {
			fmt.Fprint(os.Stderr, "\x1b[2J\x1b[H")
			fmt.Fprintln(os.Stderr, "Telegram QR Login")
			fmt.Fprintln(os.Stderr)
			fmt.Fprintln(os.Stderr, "On your logged-in Telegram phone:")
			fmt.Fprintln(os.Stderr, "Settings -> Devices -> Link Desktop Device")
			fmt.Fprintln(os.Stderr)
			fmt.Fprintln(os.Stderr, widgets.RenderQRCode(token.URL, 256))
			fmt.Fprintln(os.Stderr)
			fmt.Fprintf(os.Stderr, "QR expires at %s and will refresh automatically.\n", token.ExpiresAt.Format("15:04:05"))
			return nil
		},
		PasswordPrompt: func(_ context.Context, retry bool) ([]byte, error) {
			if retry {
				fmt.Fprintln(os.Stderr, "The password was empty or invalid. Try again.")
			}
			fmt.Fprint(os.Stderr, "Enter your Telegram 2FA password (input is hidden): ")
			password, err := term.ReadPassword(int(os.Stdin.Fd()))
			fmt.Fprintln(os.Stderr)
			return password, err
		},
	})
	if err != nil {
		if errors.Is(err, context.Canceled) {
			return
		}
		log.Fatalf("QR login failed: %v", err)
	}

	printLoggedIn(me)
}

func printLoggedIn(me *telegram.User) {
	name := strings.TrimSpace(me.FirstName + " " + me.LastName)
	if me.Username != "" {
		fmt.Fprintf(os.Stderr, "Logged in as %s (@%s, id %d)\n", name, me.Username, me.ID)
	} else {
		fmt.Fprintf(os.Stderr, "Logged in as %s (id %d)\n", name, me.ID)
	}
}

// runServe runs the MCP server over stdio. The session must already be
// authorized (via the TUI or `telegram-mcp login`). The client runs in
// RPC-only mode (no update subscription) so it never competes with the
// TUI for realtime updates.
func runServe(cfg *config.Config) {
	authorizer := telegram.NewTUIAuthorizer(cfg)
	authorizer.NonInteractive = true

	client := telegram.NewRPCClientAsync(cfg, authorizer)
	defer client.Close()

	errCh := make(chan error, 1)
	authorizer.SetErrorCallback(func(err error) {
		select {
		case errCh <- err:
		default:
		}
	})

	ready := make(chan struct{})
	go func() {
		client.WaitReady()
		close(ready)
	}()

	if err := waitForClientReady(ready, errCh, time.After(90*time.Second)); err != nil {
		if errors.Is(err, telegram.ErrLoginRequired) {
			fmt.Fprintln(os.Stderr, loginHint)
		} else {
			fmt.Fprintf(os.Stderr, "telegram client error: %v\n", err)
		}
		os.Exit(1)
	}

	srv := mcpserver.New(client)

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	if err := srv.Run(ctx); err != nil {
		log.Fatalf("mcp server: %v", err)
	}
}

// A readiness timeout does not establish that the session is unauthorized.
func waitForClientReady(ready <-chan struct{}, errs <-chan error, timeout <-chan time.Time) error {
	select {
	case <-ready:
		return nil
	case err := <-errs:
		return err
	case <-timeout:
		return errors.New("timed out waiting for Telegram client readiness; check network connectivity and retry")
	}
}
