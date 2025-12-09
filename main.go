package main

import (
	"context"
	_ "embed"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"
	"sync"

	"lesiw.io/command"
	"lesiw.io/command/sub"
	"lesiw.io/command/sys"
	"lesiw.io/defers"
	"lesiw.io/fs"
)

const host = "k8s.lesiw.dev"

var getSpkez = sync.OnceValues(func() (command.Machine, error) {
	ctx := context.Background()
	sh := command.Shell(sys.Machine())

	// Register commands needed during initialization
	sh.Handle("go", sh.Unshell())
	sh.Handle("spkez", sh.Unshell())

	// Check if spkez is installed; install if not found
	_, err := sh.Call(ctx, "spkez", "--version")
	if command.NotFound(err) {
		fmt.Println("Installing spkez...")
		err := sh.Exec(ctx, "go", "install", "lesiw.io/spkez@latest")
		if err != nil {
			return nil, fmt.Errorf("could not install spkez: %w", err)
		}
	} else if err != nil {
		return nil, fmt.Errorf("error checking spkez: %w", err)
	}

	// spkez is just a command prefix, not a full shell
	return sub.Machine(sh, "spkez"), nil
})

var getK8s = sync.OnceValues(func() (*command.Sh, error) {
	ctx := context.Background()
	sh := command.Shell(sys.Machine())
	sh.Handle("ssh", sh.Unshell())

	spkez, err := getSpkez()
	if err != nil {
		return nil, err
	}

	sshkey, err := command.Call(ctx, spkez, "get", "infra/ssh")
	if err != nil {
		return nil, fmt.Errorf("could not get ssh key: %w", err)
	}
	file, err := os.CreateTemp("", "sshkey")
	if err != nil {
		return nil, fmt.Errorf("could not create temp file: %w", err)
	}
	defers.Add(func() { _ = os.Remove(file.Name()) })
	defer file.Close()
	if err := os.Chmod(file.Name(), 0600); err != nil {
		return nil, fmt.Errorf(
			"could not set permissions on temp file: %w", err,
		)
	}
	if _, err := file.WriteString(sshkey + "\n"); err != nil {
		return nil, fmt.Errorf("could not write to temp file: %w", err)
	}
	sshkeyPath := file.Name()

	k8s := command.Shell(sub.Machine(sh, "ssh", "-i", sshkeyPath, host, "--"))
	k8s.Handle("sh", k8s.Unshell())
	k8s.Handle("curl", k8s.Unshell())
	k8s.Handle("kubectl", k8s.Unshell())
	return k8s, nil
})

var getCtl = sync.OnceValues(func() (command.Machine, error) {
	k8s, err := getK8s()
	if err != nil {
		return nil, err
	}

	// kubectl is just a command prefix, not a full shell
	return sub.Machine(k8s, "kubectl"), nil
})

const autopatchCron = `0 2 * * 6 root /usr/local/bin/autopatch >> ` +
	`/var/log/autopatch.log 2>&1
`

func main() {
	defer defers.Run()

	verbose := flag.Bool("v", false, "enable verbose command tracing")
	flag.Parse()

	if *verbose {
		command.Trace = command.ShTrace
	}

	if err := run(context.Background()); err != nil {
		fmt.Fprintln(os.Stderr, err)
		defers.Exit(1)
	}
}

func run(ctx context.Context) error {
	if err := installAutopatch(ctx); err != nil {
		return err
	}
	if err := updateK3s(ctx); err != nil {
		return fmt.Errorf("failed to install or update k3s: %w", err)
	}
	if err := setupTraefik(ctx); err != nil {
		return fmt.Errorf("failed to set up traefik: %w", err)
	}
	if err := setupPostgres(ctx); err != nil {
		return fmt.Errorf("failed to set up postgres: %w", err)
	}
	if err := setupCertManager(ctx); err != nil {
		return fmt.Errorf("failed to set up cert-manager: %w", err)
	}
	if err := setupContainerRegistry(ctx); err != nil {
		return fmt.Errorf("failed to setup container registry: %w", err)
	}
	return nil
}

//go:embed autopatch.sh
var autopatch string

func installAutopatch(ctx context.Context) error {
	k8s, err := getK8s()
	if err != nil {
		return err
	}
	err = k8s.WriteFile(
		fs.WithFileMode(ctx, 0755),
		"/usr/local/bin/autopatch",
		[]byte(autopatch),
	)
	if err != nil {
		return fmt.Errorf("could not install autopatch: %w", err)
	}
	err = k8s.WriteFile(ctx, "/etc/cron.d/autopatch", []byte(autopatchCron))
	if err != nil {
		return fmt.Errorf("could not install autopatch cron job: %w", err)
	}
	err = k8s.WriteFile(ctx, "/var/log/autopatch.log", []byte{})
	if err != nil {
		return fmt.Errorf("could not create autopatch log: %w", err)
	}
	return nil
}

func updateK3s(ctx context.Context) error {
	k8s, err := getK8s()
	if err != nil {
		return err
	}
	_, err = command.Copy(
		k8s.Command(ctx, "sh", "-s", "-"),
		k8s.Command(ctx, "curl", "-sfL", "https://get.k3s.io"),
	)
	if err != nil {
		return fmt.Errorf("could not update k3s: %w", err)
	}
	return nil
}

//go:embed traefik.yml
var traefikConfig string

func setupTraefik(ctx context.Context) error {
	// k3s comes with traefik already installed.
	// This function applies configuration to the existing installation.
	ctl, err := getCtl()
	if err != nil {
		return err
	}
	_, err = command.Copy(
		ctl.Command(ctx, "apply", "-f", "-"),
		strings.NewReader(traefikConfig),
	)
	if err != nil {
		return fmt.Errorf("could not configure traefik: %w", err)
	}
	return nil
}

//go:embed cluster.yml
var postgresClusterCfg string

func setupPostgres(ctx context.Context) error {
	ctl, err := getCtl()
	if err != nil {
		return err
	}
	err = command.Exec(
		ctx,
		ctl,
		"apply",
		"--server-side",     // github.com/cloudnative-pg/charts/issues/325
		"--force-conflicts", // necessary to install over existing versions
		"-f",
		"https://raw.githubusercontent.com/cloudnative-pg/cloudnative-pg/"+
			"release-1.25/releases/cnpg-1.25.0.yaml",
	)
	if err != nil {
		return fmt.Errorf("could not install CNPG: %w", err)
	}
	_, err = command.Copy(
		ctl.Command(ctx, "apply", "-f", "-"),
		strings.NewReader(postgresClusterCfg),
	)
	if err != nil {
		return fmt.Errorf("could not install PG cluster: %w", err)
	}
	return nil
}

const secretCfg = `apiVersion: v1
kind: Secret
metadata:
  name: %s
type: Opaque
stringData:
  %s: %s`

//go:embed issuer.yml
var issuerCfg string

func setupCertManager(ctx context.Context) error {
	ctl, err := getCtl()
	if err != nil {
		return err
	}
	spkez, err := getSpkez()
	if err != nil {
		return err
	}
	err = command.Exec(
		ctx,
		ctl,
		"apply",
		"-f",
		"https://github.com/cert-manager/cert-manager/"+
			"releases/download/v1.17.1/cert-manager.yaml",
	)
	if err != nil {
		return fmt.Errorf("could not install cert-manager: %w", err)
	}
	r, err := command.Call(ctx, spkez, "get", "k8s/cert-manager/cloudflare")
	if err != nil {
		return fmt.Errorf("could not get cloudflare API key: %w", err)
	}
	_, err = command.Copy(
		ctl.Command(ctx, "apply", "-f", "-"),
		strings.NewReader(fmt.Sprintf(
			secretCfg, "cert-manager-cloudflare-token", "api-token", r,
		)),
	)
	if err != nil {
		return fmt.Errorf("could not store cloudflare secret: %w", err)
	}
	_, err = command.Copy(
		ctl.Command(ctx, "apply", "-f", "-"),
		strings.NewReader(issuerCfg),
	)
	if err != nil {
		return fmt.Errorf("could not create cloudflare issuer: %w", err)
	}
	return nil
}

//go:embed registry.yml
var registryCfg string

const basicAuthCfg = `apiVersion: v1
kind: Secret
metadata:
  name: %s
type: kubernetes.io/basic-auth
stringData:
  username: %s
  password: %s`

func setupContainerRegistry(ctx context.Context) error {
	ctl, err := getCtl()
	if err != nil {
		return err
	}
	spkez, err := getSpkez()
	if err != nil {
		return err
	}
	r, err := command.Call(ctx, spkez, "get", "ctr.lesiw.dev/auth")
	if err != nil {
		return fmt.Errorf("could not get registry auth secret: %w", err)
	}
	reguser, regpass := "ll", r
	_, err = command.Copy(
		ctl.Command(ctx, "apply", "-f", "-"),
		strings.NewReader(fmt.Sprintf(
			basicAuthCfg, "registry-auth-secret", reguser, regpass,
		)),
	)
	if err != nil {
		return fmt.Errorf("could not store registry auth secret: %w", err)
	}
	_, err = command.Copy(
		ctl.Command(ctx, "apply", "-f", "-"),
		strings.NewReader(registryCfg),
	)
	if err != nil {
		return fmt.Errorf("could not install registry: %w", err)
	}

	err = command.Exec(ctx, ctl, "get", "secret", "regcred")
	if err != nil {
		trace := command.Trace
		defer func() { command.Trace = trace }()
		command.Trace = io.Discard // Hide the registry secret.
		err = command.Exec(
			ctx,
			ctl,
			"create", "secret", "docker-registry", "regcred",
			"--docker-server=ctr.lesiw.dev",
			"--docker-username="+reguser,
			"--docker-password="+regpass,
		)
		if err != nil {
			return fmt.Errorf("could not store registry secret: %w", err)
		}
		command.Trace = trace
	}
	return nil
}
