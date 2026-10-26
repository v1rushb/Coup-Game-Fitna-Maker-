package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"

	"github.com/v1rushb/neoncoup/config"
	"github.com/v1rushb/neoncoup/sshd"
)

func cmdServeSSH(args []string) error {
	var g globals
	fs := newFlags("serve-ssh", &g)
	addr := fs.String("addr", ":2222", "SSH listen address")
	key := fs.String("host-key", "", "host key path (created if missing; default in the config dir)")
	httpAddr := fs.String("http", "", "also serve rooms over WebSocket at this address (e.g. :7777) so app users can join")
	if _, err := parse(fs, args); err != nil {
		return err
	}
	store, err := config.Open(g.configDir)
	if err != nil {
		return err
	}
	if *key == "" {
		*key = filepath.Join(store.Dir(), "ssh_host_ed25519")
	}
	log := slog.New(slog.NewTextHandler(os.Stderr, nil))
	s, err := sshd.New(sshd.Options{Addr: *addr, HostKeyPath: *key, Logger: log, Version: version, HTTPAddr: *httpAddr})
	if err != nil {
		return err
	}
	fmt.Fprintf(os.Stderr, "NEON//COUP over SSH on %s: players run  ssh -p PORT %s\n", *addr, "THIS-HOST")
	ctx, cancel := context.WithCancel(context.Background())
	go func() { <-signalChan(); cancel() }()
	return s.ListenAndServe(ctx)
}
