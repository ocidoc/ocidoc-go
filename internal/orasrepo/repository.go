// SPDX-License-Identifier: MIT
// SPDX-FileCopyrightText: Copyright 2026 WoozyMasta
// Source: github.com/ocidoc/ocidoc-go

// Package orasrepo isolates oras-go's remote registry client behind a
// small module-internal interface. It is the only package in this module
// that imports ORAS registry transport types.
package orasrepo

import (
	"context"
	"crypto/tls"
	"fmt"
	"io"
	"net/http"
	"time"

	ocispec "github.com/opencontainers/image-spec/specs-go/v1"
	"github.com/woozymasta/orascope"
	"oras.land/oras-go/v2/registry/remote"
	"oras.land/oras-go/v2/registry/remote/auth"
	"oras.land/oras-go/v2/registry/remote/retry"
)

// Repository provides the registry operations required
// by higher-level OCIDoc workflows without exposing the wider ORAS repository API.
type Repository interface {
	// Resolve resolves reference (a tag or digest) to its descriptor.
	Resolve(ctx context.Context, reference string) (ocispec.Descriptor, error)

	// Fetch opens descriptor's content. The caller must close it.
	Fetch(ctx context.Context, descriptor ocispec.Descriptor) (io.ReadCloser, error)

	// Push uploads content, which must match descriptor's digest and size.
	Push(ctx context.Context, descriptor ocispec.Descriptor, content io.Reader) error

	// PushManifest uploads a manifest and selects whether ORAS maintains
	// the OCI referrers tag-schema fallback.
	PushManifest(ctx context.Context, descriptor ocispec.Descriptor, content io.Reader, indexReferrers bool) error

	// Tag points tag at descriptor, which must already exist in the repository.
	Tag(ctx context.Context, descriptor ocispec.Descriptor, tag string) error

	// Referrers returns all descriptors that refer to subject,
	// optionally filtered by artifactType.
	// It follows every registry result page and uses ORAS native-referrer
	// or tag-schema fallback support as appropriate.
	Referrers(ctx context.Context, subject ocispec.Descriptor, artifactType string) ([]ocispec.Descriptor, error)

	// Delete removes descriptor's manifest or blob from the repository.
	Delete(ctx context.Context, descriptor ocispec.Descriptor) error
}

// Options configures Open's registry authentication and transport.
// Password and Token are already-resolved values;
// this package neither prompts for nor persists credentials.
type Options struct {
	// Username supplies HTTP Basic authentication with Password.
	Username string

	// Password supplies HTTP Basic authentication with Username.
	Password string

	// Token is used as a refresh (identity) token,
	// matching how Docker/ORAS treat a caller-supplied token
	// for a registry's OAuth2 flow (distribution-spec's token authentication)
	// rather than as a short-lived access token used as-is.
	Token string

	// RegistryConfigPath, if set, is the sole Docker-compatible credential source.
	// Automatic credential discovery is disabled for this connection.
	RegistryConfigPath string

	// PlainHTTP allows unencrypted HTTP registry transport.
	PlainHTTP bool

	// TLSSkipVerify disables server-certificate verification.
	TLSSkipVerify bool

	// Timeout bounds each individual HTTP request-response cycle not an overall operation;
	// zero means no client-side timeout beyond ctx's own deadline, if any.
	Timeout time.Duration
}

// Open returns a Repository bound to reference ("host/repository[:tag|@digest]").
// Open performs no I/O itself;
// connection and authentication happen lazily on the first call to a Repository method.
func Open(reference string, opts Options) (Repository, error) {
	repo, err := remote.NewRepository(reference)
	if err != nil {
		return nil, mapError(err)
	}

	repo.PlainHTTP = opts.PlainHTTP
	client, err := newAuthClient(opts)
	if err != nil {
		return nil, err
	}
	repo.Client = client

	return &orasRepository{repo: repo}, nil
}

// newAuthClient configures ORAS authentication.
// Explicit credentials override Docker configuration.
// Otherwise ORAScope resolves repository-scoped inline auth entries
// while ORAS retains host credentials, token exchange and retries.
func newAuthClient(opts Options) (*auth.Client, error) {
	client := &auth.Client{
		Client: httpClient(opts),
		Cache:  auth.NewCache(),
	}
	if opts.Username != "" || opts.Password != "" || opts.Token != "" {
		client.Credential = resolveCredential(opts)

		return client, nil
	}

	var (
		adapter *orascope.Adapter
		err     error
	)
	if opts.RegistryConfigPath == "" {
		adapter, err = orascope.NewDefault()
	} else {
		adapter, err = orascope.New(
			orascope.WithDockerConfigPath(opts.RegistryConfigPath),
			orascope.WithoutDiscovery(),
		)
	}
	if err != nil {
		return nil, fmt.Errorf("configure scoped registry credentials: %w", err)
	}

	wrapped, err := adapter.WrapAuthClient(client)
	if err != nil {
		return nil, fmt.Errorf("wrap scoped registry credentials: %w", err)
	}

	return wrapped, nil
}

// httpClient builds the retrying HTTP client used by ORAS authentication.
// TLSSkipVerify is an explicit opt-out from certificate verification.
func httpClient(opts Options) *http.Client {
	var base http.RoundTripper

	if opts.TLSSkipVerify {
		//nolint:forcetypeassert // http.DefaultTransport is always *http.Transport in the standard library.
		transport := http.DefaultTransport.(*http.Transport).Clone()
		//nolint:gosec // explicit, caller-requested opt-out via Options.TLSSkipVerify.
		transport.TLSClientConfig = &tls.Config{InsecureSkipVerify: true}
		base = transport
	}

	return &http.Client{
		Transport: retry.NewTransport(base),
		Timeout:   opts.Timeout,
	}
}

// orasRepository adapts *remote.Repository to Repository.
type orasRepository struct {
	// repo is the ORAS repository receiving adapter operations.
	repo *remote.Repository
}

// Resolve implements Repository.
func (r *orasRepository) Resolve(ctx context.Context, reference string) (ocispec.Descriptor, error) {
	desc, err := r.repo.Resolve(ctx, reference)
	if err != nil {
		return ocispec.Descriptor{}, mapError(err)
	}

	return desc, nil
}

// Fetch implements Repository.
func (r *orasRepository) Fetch(ctx context.Context, descriptor ocispec.Descriptor) (io.ReadCloser, error) {
	rc, err := r.repo.Fetch(ctx, descriptor)
	if err != nil {
		return nil, mapError(err)
	}

	return rc, nil
}

// Push implements Repository.
func (r *orasRepository) Push(ctx context.Context, descriptor ocispec.Descriptor, content io.Reader) error {
	if err := r.repo.Push(ctx, descriptor, content); err != nil {
		return mapError(err)
	}

	return nil
}

// PushManifest implements Repository.
// Disabling indexReferrers suppresses only the client-maintained tag-schema fallback;
// native registries index subjects.
func (r *orasRepository) PushManifest(
	ctx context.Context,
	descriptor ocispec.Descriptor,
	content io.Reader,
	indexReferrers bool,
) error {
	if !indexReferrers {
		if err := r.repo.SetReferrersCapability(true); err != nil {
			return mapError(err)
		}
	}

	return r.Push(ctx, descriptor, content)
}

// Tag implements Repository.
func (r *orasRepository) Tag(ctx context.Context, descriptor ocispec.Descriptor, tag string) error {
	if err := r.repo.Tag(ctx, descriptor, tag); err != nil {
		return mapError(err)
	}

	return nil
}

// Referrers implements Repository by accumulating ORAS callback pages into one result slice.
func (r *orasRepository) Referrers(ctx context.Context, subject ocispec.Descriptor, artifactType string) ([]ocispec.Descriptor, error) {
	var all []ocispec.Descriptor

	err := r.repo.Referrers(ctx, subject, artifactType, func(referrers []ocispec.Descriptor) error {
		all = append(all, referrers...)
		return nil
	})
	if err != nil {
		return nil, mapError(err)
	}

	return all, nil
}

// Delete implements Repository.
func (r *orasRepository) Delete(ctx context.Context, descriptor ocispec.Descriptor) error {
	if err := r.repo.Delete(ctx, descriptor); err != nil {
		return mapError(err)
	}

	return nil
}
