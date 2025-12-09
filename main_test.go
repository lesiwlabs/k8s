package main

import (
	"bytes"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"
	"lesiw.io/command"
	"lesiw.io/command/mock"
	"lesiw.io/fs"
)

func swap[T any](t *testing.T, orig *T, with T) {
	t.Helper()
	o := *orig
	t.Cleanup(func() { *orig = o })
	*orig = with
}

func TestInstallAutopatch(t *testing.T) {
	sh := command.Shell(mock.New())
	swap(t, &getK8s, func() (*command.Sh, error) { return sh, nil })

	if err := installAutopatch(t.Context()); err != nil {
		t.Fatalf("installAutopatch() err: %v", err)
	}

	// Verify autopatch script was written with executable permissions
	got, err := sh.ReadFile(t.Context(), "/usr/local/bin/autopatch")
	if err != nil {
		t.Fatalf("ReadFile(/usr/local/bin/autopatch) err: %v", err)
	}
	if !bytes.Equal(got, autopatch) {
		t.Errorf(
			"ReadFile(/usr/local/bin/autopatch):\n%s",
			cmp.Diff(autopatch, got),
		)
	}
	info, err := sh.Stat(t.Context(), "/usr/local/bin/autopatch")
	if err != nil {
		t.Fatalf("Stat(/usr/local/bin/autopatch) err: %v", err)
	}
	if got, want := info.Mode().Perm(), fs.Mode(0755); got != want {
		t.Errorf("autopatch mode = %04o, want %04o", got, want)
	}

	// Verify cron job was written
	got, err = sh.ReadFile(t.Context(), "/etc/cron.d/autopatch")
	if err != nil {
		t.Fatalf("ReadFile(/etc/cron.d/autopatch) err: %v", err)
	}
	if want := autopatchCron; string(got) != want {
		t.Errorf(
			"ReadFile(/etc/cron.d/autopatch):\n%s",
			cmp.Diff(want, string(got)),
		)
	}
}

func TestUpdateK3s(t *testing.T) {
	sh := command.Shell(mock.New())
	sh.Handle("curl", sh.Unshell())
	sh.Handle("sh", sh.Unshell())
	swap(t, &getK8s, func() (*command.Sh, error) { return sh, nil })

	if err := updateK3s(t.Context()); err != nil {
		t.Fatalf("updateK3s() err: %v", err)
	}

	curlCalls := mock.CallsFor(sh, "curl")
	if got, want := len(curlCalls), 1; got != want {
		t.Fatalf("curl call count = %d, want %d", got, want)
	}
	if diff := cmp.Diff(
		[]string{"curl", "-sfL", "https://get.k3s.io"},
		curlCalls[0].Args,
	); diff != "" {
		t.Errorf("curl args (-want +got):\n%s", diff)
	}

	shCalls := mock.CallsFor(sh, "sh")
	if got, want := len(shCalls), 1; got != want {
		t.Fatalf("sh call count = %d, want %d", got, want)
	}
	if diff := cmp.Diff(
		[]string{"sh", "-s", "-"},
		shCalls[0].Args,
	); diff != "" {
		t.Errorf("sh args (-want +got):\n%s", diff)
	}
}

func TestSetupTraefik(t *testing.T) {
	ctl := mock.New()
	swap(t, &getCtl, func() (command.Machine, error) { return ctl, nil })

	if err := setupTraefik(t.Context()); err != nil {
		t.Fatalf("setupTraefik() err: %v", err)
	}

	applyCalls := mock.CallsFor(ctl, "apply")
	if got, want := len(applyCalls), 1; got != want {
		t.Fatalf("apply call count = %d, want %d", got, want)
	}

	if diff := cmp.Diff(
		[]string{"apply", "-f", "-"},
		applyCalls[0].Args,
	); diff != "" {
		t.Errorf("apply args (-want +got):\n%s", diff)
	}

	if got, want := string(applyCalls[0].Got), traefikConfig; got != want {
		t.Errorf("apply stdin:\n%s", cmp.Diff(want, got))
	}
}

func TestSetupPostgres(t *testing.T) {
	ctl := mock.New()
	swap(t, &getCtl, func() (command.Machine, error) { return ctl, nil })

	if err := setupPostgres(t.Context()); err != nil {
		t.Fatalf("setupPostgres() err: %v", err)
	}

	applyCalls := mock.CallsFor(ctl, "apply")
	if got, want := len(applyCalls), 2; got != want {
		t.Fatalf("apply call count = %d, want %d", got, want)
	}

	// Verify CNPG operator installation
	cnpgURL := "https://raw.githubusercontent.com/cloudnative-pg/" +
		"cloudnative-pg/release-1.25/releases/cnpg-1.25.0.yaml"
	if diff := cmp.Diff(
		[]string{"apply", "--server-side", "--force-conflicts", "-f",
			cnpgURL},
		applyCalls[0].Args,
	); diff != "" {
		t.Errorf("first apply args (-want +got):\n%s", diff)
	}

	// Verify postgres cluster config was applied
	if diff := cmp.Diff(
		[]string{"apply", "-f", "-"},
		applyCalls[1].Args,
	); diff != "" {
		t.Errorf("second apply args (-want +got):\n%s", diff)
	}

	got, want := string(applyCalls[1].Got), postgresClusterCfg
	if got != want {
		t.Errorf("apply stdin:\n%s", cmp.Diff(want, got))
	}
}

func TestSetupCertManager(t *testing.T) {
	ctl := mock.New()
	spkez := mock.New()
	spkez.Return("get", "fake-cloudflare-token\n")
	swap(t, &getCtl, func() (command.Machine, error) { return ctl, nil })
	swap(t, &getSpkez, func() (command.Machine, error) { return spkez, nil })

	if err := setupCertManager(t.Context()); err != nil {
		t.Fatalf("setupCertManager() err: %v", err)
	}

	applyCalls := mock.CallsFor(ctl, "apply")
	if got, want := len(applyCalls), 3; got != want {
		t.Fatalf("apply call count = %d, want %d", got, want)
	}

	// Verify cert-manager installation
	certURL := "https://github.com/cert-manager/cert-manager/" +
		"releases/download/v1.17.1/cert-manager.yaml"
	if diff := cmp.Diff(
		[]string{"apply", "-f", certURL},
		applyCalls[0].Args,
	); diff != "" {
		t.Errorf("cert-manager install args (-want +got):\n%s", diff)
	}

	// Verify spkez was called to get cloudflare API key
	getCalls := mock.CallsFor(spkez, "get")
	if got, want := len(getCalls), 1; got != want {
		t.Fatalf("spkez get call count = %d, want %d", got, want)
	}
	if diff := cmp.Diff(
		[]string{"get", "k8s/cert-manager/cloudflare"},
		getCalls[0].Args,
	); diff != "" {
		t.Errorf("spkez get args (-want +got):\n%s", diff)
	}

	// Verify cloudflare secret was created
	secretData := string(applyCalls[1].Got)
	if !strings.Contains(secretData, "cert-manager-cloudflare-token") {
		t.Errorf(
			"secret data does not contain cert-manager-cloudflare-token: %q",
			secretData,
		)
	}
	if !strings.Contains(secretData, "fake-cloudflare-token") {
		t.Errorf(
			"secret data does not contain fake-cloudflare-token: %q",
			secretData,
		)
	}

	// Verify issuer was created
	if got, want := string(applyCalls[2].Got), issuerCfg; got != want {
		t.Errorf("issuer config:\n%s", cmp.Diff(want, got))
	}
}

func TestSetupContainerRegistry(t *testing.T) {
	ctl := mock.New()
	spkez := mock.New()
	spkez.Return("get", "fake-registry-password\n")
	swap(t, &getCtl, func() (command.Machine, error) { return ctl, nil })
	swap(t, &getSpkez, func() (command.Machine, error) { return spkez, nil })

	if err := setupContainerRegistry(t.Context()); err != nil {
		t.Fatalf("setupContainerRegistry() err: %v", err)
	}

	// Verify spkez was called to get registry auth
	getCalls := mock.CallsFor(spkez, "get")
	if got, want := len(getCalls), 1; got != want {
		t.Fatalf("spkez get call count = %d, want %d", got, want)
	}
	if diff := cmp.Diff(
		[]string{"get", "ctr.lesiw.dev/auth"},
		getCalls[0].Args,
	); diff != "" {
		t.Errorf("spkez get args (-want +got):\n%s", diff)
	}

	applyCalls := mock.CallsFor(ctl, "apply")
	if got, want := len(applyCalls), 2; got != want {
		t.Fatalf("apply call count = %d, want %d", got, want)
	}

	// Verify registry auth secret was created
	authSecretData := string(applyCalls[0].Got)
	if !strings.Contains(authSecretData, "registry-auth-secret") {
		t.Errorf(
			"auth secret data does not contain registry-auth-secret: %q",
			authSecretData,
		)
	}
	if !strings.Contains(authSecretData, "fake-registry-password") {
		t.Errorf(
			"auth secret data does not contain fake-registry-password: %q",
			authSecretData,
		)
	}

	// Verify registry config was applied
	if got, want := string(applyCalls[1].Got), registryCfg; got != want {
		t.Errorf("registry config:\n%s", cmp.Diff(want, got))
	}

	// Verify regcred existence check was performed
	getSecretCalls := mock.CallsFor(ctl, "get")
	if got, want := len(getSecretCalls), 1; got != want {
		t.Fatalf("get secret call count = %d, want %d", got, want)
	}
	if diff := cmp.Diff(
		[]string{"get", "secret", "regcred"},
		getSecretCalls[0].Args,
	); diff != "" {
		t.Errorf("get secret args (-want +got):\n%s", diff)
	}
}
