package service

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/castrowithcee/qatlas-kit/redact"

	"github.com/castrowithcee/qatlas-host/internal/engine"
	"github.com/castrowithcee/qatlas-host/internal/stacks"
)

func TestSecretLikeEnvValuesAreRedacted(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, "demo-stack")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	files := map[string]string{
		"compose.yaml": "services:\n  app:\n    image: registry.example.test/demo/app:${DEMO_VERSION}\n",
		".env":         "DEMO_VERSION=1.2.3\nDEMO_PASSWORD=not-a-real-secret\napi_token=not-a-real-token\nDEMO_HOST=host-is-not-secret\n",
		"secrets.env":  "OTHER=not-a-real-extra\n",
	}
	for name, body := range files {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	list, err := stacks.Load(root)
	if err != nil || len(list) != 1 || list[0].Err != nil {
		t.Fatalf("Load: %v %+v", err, list)
	}
	var r redact.Redactor
	registerStackSecrets(&r, list[0])
	got := r.Apply("a=not-a-real-secret b=not-a-real-token c=not-a-real-extra d=host-is-not-secret e=1.2.3")
	want := "a=[redacted] b=[redacted] c=[redacted] d=host-is-not-secret e=1.2.3"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

type loadedDeployer struct {
	engine.Deployer
	got string
}

func (l *loadedDeployer) LoadedImage(_ context.Context, ref string, _ *engine.Log) (engine.ImageFingerprint, error) {
	l.got = ref
	return engine.ImageFingerprint{ImageID: "sha256:" + strings.Repeat("a", 64)}, nil
}

func TestRedactingDeployerPassesLoadedImageThrough(t *testing.T) {
	inner := &loadedDeployer{}
	var r redact.Redactor
	var d engine.Deployer = redactingDeployer{next: inner, r: &r}
	reader, ok := d.(engine.LoadedImageReader)
	if !ok {
		t.Fatal("the wrapper is not an engine.LoadedImageReader")
	}
	fp, err := reader.LoadedImage(context.Background(), "demo/app:1", nil)
	if err != nil || inner.got != "demo/app:1" || fp.ImageID == "" {
		t.Errorf("got %+v, %v, ref %q", fp, err, inner.got)
	}
	_, err = redactingDeployer{next: nil, r: &r}.LoadedImage(context.Background(), "demo/app:1", nil)
	if !errors.Is(err, engine.ErrNoImageCheck) {
		t.Errorf("a deployer without the reader: got %v", err)
	}
}
