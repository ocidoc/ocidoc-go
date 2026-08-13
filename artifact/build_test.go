// SPDX-License-Identifier: MIT
// SPDX-FileCopyrightText: Copyright 2026 WoozyMasta
// Source: github.com/ocidoc/ocidoc-go

package artifact

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/ocidoc/ocidoc-go/spec"
)

type collectingObserver struct {
	warnings []string
}

func (o *collectingObserver) Warn(message string) {
	o.warnings = append(o.warnings, message)
}

func TestBuildWritesArchive(t *testing.T) {
	root := newLayoutFixture(t)
	output := filepath.Join(t.TempDir(), "documentation.ocidoc")

	result, err := Build(context.Background(), BuildOptions{
		Root:   root,
		Output: Destination{Path: output},
	})
	if err != nil {
		t.Fatalf("Build: %v", err)
	}

	if result.Output != output {
		t.Fatalf("got Output %q, want %q", result.Output, output)
	}

	if _, err := os.Stat(output); err != nil {
		t.Fatalf("expected archive at %q: %v", output, err)
	}

	reader, err := OpenArchive(output)
	if err != nil {
		t.Fatalf("OpenArchive: %v", err)
	}
	defer reader.Close()

	verification, err := Verify(context.Background(), reader, VerifyOptions{})
	if err != nil {
		t.Fatalf("Verify: %v", err)
	}

	if !verification.Valid {
		t.Fatalf("expected a valid built archive, got issues: %+v", verification.Issues)
	}
}

func TestBuildRejectsCanceledContext(t *testing.T) {
	root := newLayoutFixture(t)
	output := filepath.Join(t.TempDir(), "documentation.ocidoc")

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := Build(ctx, BuildOptions{
		Root:   root,
		Output: Destination{Path: output},
	})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("Build: got %v, want errors.Is(err, context.Canceled)", err)
	}

	if _, err := os.Stat(output); !os.IsNotExist(err) {
		t.Fatalf("expected no output file after cancellation, stat err: %v", err)
	}
}

func TestBuildRefusesOverwriteByDefault(t *testing.T) {
	root := newLayoutFixture(t)
	output := filepath.Join(t.TempDir(), "documentation.ocidoc")

	if err := os.WriteFile(output, []byte("pre-existing"), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	_, err := Build(context.Background(), BuildOptions{
		Root:   root,
		Output: Destination{Path: output},
	})
	if err == nil {
		t.Fatal("expected an error when the output already exists and Overwrite is false")
	}

	got, readErr := os.ReadFile(output)
	if readErr != nil {
		t.Fatalf("ReadFile: %v", readErr)
	}

	if string(got) != "pre-existing" {
		t.Fatalf("expected the pre-existing output to be left untouched, got %q", got)
	}
}

func TestBuildOverwriteReplacesExistingFile(t *testing.T) {
	root := newLayoutFixture(t)
	output := filepath.Join(t.TempDir(), "documentation.ocidoc")

	if err := os.WriteFile(output, []byte("pre-existing"), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	_, err := Build(context.Background(), BuildOptions{
		Root:   root,
		Output: Destination{Path: output, Overwrite: true},
	})
	if err != nil {
		t.Fatalf("Build: %v", err)
	}

	reader, err := OpenArchive(output)
	if err != nil {
		t.Fatalf("OpenArchive: %v", err)
	}
	defer reader.Close()
}

func TestBuildReportsWarningsToObserver(t *testing.T) {
	root := t.TempDir()
	writeConfigTree(t, root, map[string]string{
		"ocidoc.yaml": `
schemaVersion: v1beta
components:
  documentation:
    - /README.md
  changelog:
    - /CHANGELOG.md
`,
		"README.md": "# hi",
	})

	observer := &collectingObserver{}

	result, err := Build(context.Background(), BuildOptions{
		Root:     root,
		Output:   Destination{Path: filepath.Join(t.TempDir(), "documentation.ocidoc")},
		Observer: observer,
	})
	if err != nil {
		t.Fatalf("Build: %v", err)
	}

	if len(observer.warnings) != 1 {
		t.Fatalf("expected exactly one warning, got %v", observer.warnings)
	}

	if len(result.Plan.Warnings) != 1 {
		t.Fatalf("expected BuildResult.Plan.Warnings to carry the same warning, got %v", result.Plan.Warnings)
	}
}

func TestBuildStrictModeFailsInsteadOfWarning(t *testing.T) {
	root := t.TempDir()
	writeConfigTree(t, root, map[string]string{
		"ocidoc.yaml": `
schemaVersion: v1beta
settings:
  strict: true
components:
  documentation:
    - /README.md
  changelog:
    - /CHANGELOG.md
`,
		"README.md": "# hi",
	})

	_, err := Build(context.Background(), BuildOptions{
		Root:   root,
		Output: Destination{Path: filepath.Join(t.TempDir(), "documentation.ocidoc")},
	})

	if err == nil {
		t.Fatal("expected strict mode to fail the build")
	}

	var emptyErr *EmptyComponentsError
	if !errors.As(err, &emptyErr) {
		t.Fatalf("expected an *EmptyComponentsError, got %v", err)
	}
}

func TestBuildRequiresOutputPath(t *testing.T) {
	root := newLayoutFixture(t)

	_, err := Build(context.Background(), BuildOptions{Root: root})
	if err == nil {
		t.Fatal("expected an error when Output.Path is empty")
	}
}

func TestBuildDeterministicWithFixedModTime(t *testing.T) {
	root := newLayoutFixture(t)

	modTime := time.Unix(1700000000, 0).UTC()

	outputA := filepath.Join(t.TempDir(), "a.ocidoc")
	outputB := filepath.Join(t.TempDir(), "b.ocidoc")

	if _, err := Build(context.Background(), BuildOptions{
		Root: root, Output: Destination{Path: outputA}, ModTime: modTime,
	}); err != nil {
		t.Fatalf("Build A: %v", err)
	}

	if _, err := Build(context.Background(), BuildOptions{
		Root: root, Output: Destination{Path: outputB}, ModTime: modTime,
	}); err != nil {
		t.Fatalf("Build B: %v", err)
	}

	dataA, err := os.ReadFile(outputA)
	if err != nil {
		t.Fatalf("ReadFile A: %v", err)
	}

	dataB, err := os.ReadFile(outputB)
	if err != nil {
		t.Fatalf("ReadFile B: %v", err)
	}

	if string(dataA) != string(dataB) {
		t.Fatal("expected two builds with the same fixed ModTime to be byte-identical")
	}
}

func TestBuildGoldenReproducibility(t *testing.T) {
	const modTime = 1700000000

	tests := []struct {
		name             string
		compression      spec.CompressionType
		level            int
		rootDigest       string
		configDigest     string
		componentDigests map[spec.ComponentType]string
	}{
		{
			name:         "gzip",
			compression:  spec.CompressionGzip,
			level:        6,
			rootDigest:   "sha256:70f0086739931c66ec1366dd5e290cf98adf3f7aae72c967a3520d2325ffaaae",
			configDigest: "sha256:0ebadf061aeb58c27c9151239872fab15c0ccd5fc5f515eee5870ec300668a14",
			componentDigests: map[spec.ComponentType]string{
				spec.ComponentDocumentation: "sha256:dfad74ebf66f3028e3f7a7ca6b59f3843d1efdd76fa7ad44a774edb417c292a7",
				spec.ComponentLicense:       "sha256:94c92722dbb1c5731cee97d0e7a42c60ec03ba6f0dfe68ec43487a52d7cc3e15",
			},
		},
		{
			name:         "zstd",
			compression:  spec.CompressionZstd,
			level:        3,
			rootDigest:   "sha256:dd4bc75df90f434a99440f6677830208b61920bf47df83608046f20122227260",
			configDigest: "sha256:0ebadf061aeb58c27c9151239872fab15c0ccd5fc5f515eee5870ec300668a14",
			componentDigests: map[spec.ComponentType]string{
				spec.ComponentDocumentation: "sha256:e621401645ad3e6740cb3d2732c49235cf0544d869a31f1a24e611adba8fda46",
				spec.ComponentLicense:       "sha256:cd81b387e6ab91a6ed28055321467b0303eff7473945347dff0ca6d1f84fd157",
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			root := newLayoutFixture(t)
			layoutDir := t.TempDir()
			level := test.level
			_, _, err := BuildLayout(t.Context(), root, layoutDir, BuildLayoutOptions{
				ModTime: time.Unix(modTime, 0).UTC(),
				Plan: PlanOptions{Settings: &spec.BuildSettings{
					Compression: &spec.CompressionSettings{Type: test.compression, Level: &level},
				}},
			})
			if err != nil {
				t.Fatalf("BuildLayout: %v", err)
			}

			reader, err := OpenLayout(layoutDir)
			if err != nil {
				t.Fatalf("OpenLayout: %v", err)
			}
			defer reader.Close() //nolint:errcheck // reader owns no persistent resource.

			rootDescriptor, err := reader.Root(t.Context())
			if err != nil {
				t.Fatalf("Root: %v", err)
			}
			manifest, err := reader.Manifest(t.Context())
			if err != nil {
				t.Fatalf("Manifest: %v", err)
			}
			components, err := reader.Components(t.Context())
			if err != nil {
				t.Fatalf("Components: %v", err)
			}

			if got := rootDescriptor.Digest.String(); got != test.rootDigest {
				t.Errorf("root digest = %s, want %s", got, test.rootDigest)
			}
			if got := manifest.Config.Digest.String(); got != test.configDigest {
				t.Errorf("config digest = %s, want %s", got, test.configDigest)
			}
			if len(components) != len(test.componentDigests) {
				t.Fatalf("got %d components, want %d", len(components), len(test.componentDigests))
			}
			for _, component := range components {
				want, ok := test.componentDigests[component.Type]
				if !ok {
					t.Errorf("unexpected component %q", component.Type)
					continue
				}
				if got := component.Descriptor.Digest.String(); got != want {
					t.Errorf("%s digest = %s, want %s", component.Type, got, want)
				}
			}
		})
	}
}
