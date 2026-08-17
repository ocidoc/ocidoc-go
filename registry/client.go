// SPDX-License-Identifier: MIT
// SPDX-FileCopyrightText: Copyright 2026 WoozyMasta
// Source: github.com/ocidoc/ocidoc-go

package registry

import (
	"context"
	"fmt"
	"time"

	ocispec "github.com/opencontainers/image-spec/specs-go/v1"

	"github.com/ocidoc/ocidoc-go/artifact"
	"github.com/ocidoc/ocidoc-go/internal/orasrepo"
)

// ClientOptions configures every registry connection a Client opens:
// authentication and transport settings supplied by a caller.
type ClientOptions struct {
	// Credentials supplies explicit or Docker-compatible registry credentials.
	Credentials CredentialOptions

	// PlainHTTP allows unencrypted HTTP registry transport.
	PlainHTTP bool

	// TLSSkipVerify disables server-certificate verification.
	TLSSkipVerify bool

	// Timeout bounds each individual HTTP request-response cycle, not an overall operation;
	// zero means no client-side timeout beyond a/ method call's own context deadline, if any.
	Timeout time.Duration
}

// Client publishes, resolves and copies OCIDoc artifacts against a registry.
// It holds no connection state of its own:
// each method opens the repository named by its own reference argument,
// so one Client can address any number of registries and repositories over its lifetime.
type Client struct {
	opts ClientOptions
}

// NewClient builds a Client from opts.
// It performs no I/O: connection and authentication happen lazily, inside each method call.
func NewClient(opts ClientOptions) *Client {
	return &Client{opts: opts}
}

// PushResult is Push's result: the reference it tagged and the pushed root manifest's descriptor.
type PushResult struct {
	// Reference is the tag updated by Push.
	Reference string

	// Manifest identifies the uploaded root manifest.
	Manifest ocispec.Descriptor
}

// Resolve resolves reference ("host/repository:tag" or "host/repository@digest") to its manifest descriptor.
// errors.Is(err, ErrNotFound) when reference does not exist.
func (c *Client) Resolve(ctx context.Context, reference string) (ocispec.Descriptor, error) {
	resolved, err := c.resolveReference(ctx, reference, "reference")
	if err != nil {
		return ocispec.Descriptor{}, err
	}

	return resolved.descriptor, nil
}

// Push publishes source to reference as a standalone artifact (no subject):
// the config blob, every component blob and the root manifest itself,
// tagged with reference's own tag. reference must include a tag;
// pushing by digest alone leaves nothing to tag.
//
// source's manifest, config and component descriptors are pushed exactly as source
// already computed them rather than rebuilt from parsed values.
func (c *Client) Push(ctx context.Context, source artifact.Reader, reference string) (*PushResult, error) {
	_, tag, err := orasrepo.ParseReference(reference)
	if err != nil {
		return nil, wrapError(err)
	}

	if tag == "" {
		return nil, fmt.Errorf("%w: reference %q has no tag to push", ErrInvalid, reference)
	}

	manifest, err := source.Manifest(ctx)
	if err != nil {
		return nil, err
	}

	if manifest.Subject != nil {
		return nil, fmt.Errorf(
			"%w: Push accepts only standalone artifacts; use Attach for subject-bound publication",
			ErrUnsupported)
	}

	root, err := source.Root(ctx)
	if err != nil {
		return nil, err
	}

	repo, err := c.open(ctx, reference)
	if err != nil {
		return nil, err
	}

	if err := c.pushGraph(ctx, source, repo, manifest.Config); err != nil {
		return nil, err
	}

	if err := c.pushManifest(ctx, source, repo, root); err != nil {
		return nil, err
	}

	if err := repo.Tag(ctx, root, tag); err != nil {
		return nil, wrapError(err)
	}

	return &PushResult{Reference: reference, Manifest: root}, nil
}

// pushGraph publishes the immutable config and component blobs shared by standalone and attached manifests.
// Root-manifest publication remains with the caller because its tag/referrer policy differs.
func (c *Client) pushGraph(
	ctx context.Context,
	source artifact.Reader,
	repo orasrepo.Repository,
	config ocispec.Descriptor,
) error {
	if err := c.pushConfig(ctx, source, repo, config); err != nil {
		return err
	}

	return c.pushComponents(ctx, source, repo)
}

// pushConfig pushes the manifest's original config blob without re-serialization.
func (c *Client) pushConfig(
	ctx context.Context,
	source artifact.Reader,
	repo orasrepo.Repository,
	expected ocispec.Descriptor,
) error {
	rc, err := source.OpenBlob(ctx, expected)
	if err != nil {
		return err
	}
	defer rc.Close() //nolint:errcheck // repository push reports read failures.

	if err := repo.Push(ctx, expected, rc); err != nil {
		return wrapError(err)
	}

	return nil
}

// pushComponents pushes every component blob exactly as source streams it, unmodified:
// unlike the manifest and config, a component's bytes are never parsed
// and re-serialized, so no re-hash check is needed here.
func (c *Client) pushComponents(
	ctx context.Context,
	source artifact.Reader,
	repo orasrepo.Repository,
) error {
	components, err := source.Components(ctx)
	if err != nil {
		return err
	}

	for _, comp := range components {
		if err := c.pushComponent(ctx, source, repo, comp); err != nil {
			return fmt.Errorf("component %q: %w", comp.Type, err)
		}
	}

	return nil
}

func (c *Client) pushComponent(
	ctx context.Context,
	source artifact.Reader,
	repo orasrepo.Repository,
	comp artifact.ComponentDescriptor,
) error {
	rc, _, err := source.OpenComponent(ctx, comp.Type)
	if err != nil {
		return err
	}
	//nolint:errcheck // read-only handle; a close error here would not change an already-pushed blob.
	defer rc.Close()

	if err := repo.Push(ctx, comp.Descriptor, rc); err != nil {
		return wrapError(err)
	}

	return nil
}

// pushManifest pushes the root manifest's original blob without re-serialization.
func (c *Client) pushManifest(
	ctx context.Context,
	source artifact.Reader,
	repo orasrepo.Repository,
	root ocispec.Descriptor,
) error {
	rc, err := source.OpenBlob(ctx, root)
	if err != nil {
		return err
	}
	defer rc.Close() //nolint:errcheck // repository push reports read failures.

	if err := repo.Push(ctx, root, rc); err != nil {
		return wrapError(err)
	}

	return nil
}

// open resolves credentials and opens repo's backing orasrepo.Repository.
func (c *Client) open(ctx context.Context, reference string) (orasrepo.Repository, error) {
	opts, err := c.repositoryOptions(ctx)
	if err != nil {
		return nil, err
	}

	repo, err := orasrepo.Open(reference, opts)
	if err != nil {
		return nil, wrapError(err)
	}

	return repo, nil
}

// repositoryOptions resolves c's credential providers
// and translates ClientOptions to orasrepo.Options.
func (c *Client) repositoryOptions(ctx context.Context) (orasrepo.Options, error) {
	password, err := resolveSecret(ctx, c.opts.Credentials.Password)
	if err != nil {
		return orasrepo.Options{}, fmt.Errorf("resolve password: %w", err)
	}

	token, err := resolveSecret(ctx, c.opts.Credentials.Token)
	if err != nil {
		return orasrepo.Options{}, fmt.Errorf("resolve token: %w", err)
	}

	return orasrepo.Options{
		Username:           c.opts.Credentials.Username,
		Password:           password,
		Token:              token,
		RegistryConfigPath: c.opts.Credentials.RegistryConfig,
		PlainHTTP:          c.opts.PlainHTTP,
		TLSSkipVerify:      c.opts.TLSSkipVerify,
		Timeout:            c.opts.Timeout,
	}, nil
}
