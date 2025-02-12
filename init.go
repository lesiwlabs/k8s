//go:build !test
// +build !test

package main

import (
	"fmt"
	"os"

	"lesiw.io/cmdio/sub"
	"lesiw.io/cmdio/sys"
	"lesiw.io/defers"
)

func init() {
	if err := runinit(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		defers.Exit(1)
	}
}

func runinit() (err error) {
	rnr = sys.Runner()

	r, err := rnr.Get("which", "spkez")
	if err != nil {
		err := rnr.Run("go", "install", "lesiw.io/spkez@latest")
		if err != nil {
			return fmt.Errorf("could not install spkez: %w", err)
		}
		r, err = rnr.Get("which", "spkez")
		if err != nil {
			return fmt.Errorf("could not find spkez: %w", err)
		}
	}
	spkez := r.Out

	r, err = rnr.Get(spkez, "get", "infra/ssh")
	if err != nil {
		return fmt.Errorf("could not get ssh key: %w", err)
	}
	file, err := os.CreateTemp("", "sshkey")
	if err != nil {
		return fmt.Errorf("could not create temp file: %w", err)
	}
	defers.Add(func() { os.Remove(file.Name()) })
	defer file.Close()
	if err := os.Chmod(file.Name(), 0600); err != nil {
		return fmt.Errorf("could not set permissions on temp file: %w", err)
	}
	if _, err := file.WriteString(r.Out + "\n"); err != nil {
		return fmt.Errorf("could not write to temp file: %w", err)
	}
	sshkey = file.Name()

	k8s = sub.WithRunner(
		rnr, "ssh",
		"-i", sshkey,
		host, "--",
	)
	ctl = sub.WithRunner(k8s, "kubectl")

	return nil
}
