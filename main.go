package main

import (
	_ "embed"
	"fmt"
	"os"
	"strings"

	"lesiw.io/cmdio"
	"lesiw.io/defers"
)

const host = "188.245.162.205"

var rnr, k8s, ctl *cmdio.Runner
var sshkey string

//go:embed autopatch.sh
var autopatch string

const autopatchCron = `0 2 * * 6 root /usr/local/bin/autopatch 2>&1 ` +
	`>> /var/log/autopatch.log`

func main() {
	defer defers.Run()
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		defers.Exit(1)
	}
}

func run() error {
	if err := installAutopatch(); err != nil {
		return err
	}
	if err := updateK3s(); err != nil {
		return err
	}
	if err := installCNPG(); err != nil {
		return err
	}
	return nil
}

func installAutopatch() error {
	_, err := cmdio.GetPipe(
		strings.NewReader(autopatch),
		k8s.Command("tee", "/usr/local/bin/autopatch"),
	)
	if err != nil {
		return fmt.Errorf("could not install autopatch: %w", err)
	}
	_, err = cmdio.GetPipe(
		strings.NewReader(autopatchCron),
		k8s.Command("tee", "/etc/cron.d/autopatch"),
	)
	if err != nil {
		return fmt.Errorf("could not install autopatch cron job: %w", err)
	}
	err = k8s.Run("chmod", "+x", "/usr/local/bin/autopatch")
	if err != nil {
		return fmt.Errorf("could not mark autopatch as executable: %w", err)
	}
	err = k8s.Run("touch", "/var/log/autopatch.log")
	if err != nil {
		return fmt.Errorf("could not create autopatch log: %w", err)
	}
	return nil
}

func updateK3s() error {
	err := cmdio.Pipe(
		k8s.Command("curl", "-sfL", "https://get.k3s.io"),
		k8s.Command("sh", "-s", "-"),
	)
	if err != nil {
		return fmt.Errorf("could not update k3s: %w", err)
	}
	return nil
}

func installCNPG() error {
	err := k8s.Run("kubectl", "apply", "-f",
		"https://raw.githubusercontent.com/cloudnative-pg/cloudnative-pg"+
			"/release-1.17/releases/cnpg-1.17.5.yaml")
	if err != nil {
		return fmt.Errorf("could not install CNPG: %w", err)
	}
	return nil
}
