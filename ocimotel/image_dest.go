package ocimotel

import (
	"context"
	"fmt"
	"io"

	"github.com/containers/image/pkg/blobinfocache/none"
	"github.com/containers/image/types"
	ispec "github.com/opencontainers/image-spec/specs-go/v1"
	"github.com/pkg/errors"
)

// NOTE - the ImageDestination interface is defined in types.go

type ociMotelImageDest struct {
	s        *OciRepo
	ref      ociMotelReference
	manifest *ispec.Manifest
}

func (o *ociMotelImageDest) Reference() types.ImageReference {
	return o.ref
}

func (o *ociMotelImageDest) Close() error {
	return nil
}

func (o *ociMotelImageDest) SupportedManifestMIMETypes() []string {
	return []string{
		ispec.MediaTypeImageManifest,
	}
}

func (o *ociMotelImageDest) SupportsSignatures(ctx context.Context) error {
	return nil
}

func (o *ociMotelImageDest) DesiredLayerCompression() types.LayerCompression {
	return types.Compress
}

func (o *ociMotelImageDest) AcceptsForeignLayerURLs() bool {
	return true
}

func (o *ociMotelImageDest) MustMatchRuntimeOS() bool {
	return false
}

func (o *ociMotelImageDest) IgnoresEmbeddedDockerReference() bool {
	// Return value does not make a difference if Reference().DockerReference()
	// is nil.
	return true
}

// PutBlob writes contents of stream and returns data representing the result.
// inputInfo.Digest can be optionally provided if known; it is not mandatory for the implementation to verify it.
// inputInfo.Size is the expected length of stream, if known.
// inputInfo.MediaType describes the blob format, if known.
// May update cache.
// WARNING: The contents of stream are being verified on the fly.  Until stream.Read() returns io.EOF, the contents of the data SHOULD NOT be available
// to any other readers for download using the supplied digest.
// If stream.Read() at any time, ESPECIALLY at end of input, returns an error, PutBlob MUST 1) fail, and 2) delete any data stored so far.
func (o *ociMotelImageDest) PutBlob(ctx context.Context, stream io.Reader, inputInfo types.BlobInfo, cache types.BlobInfoCache, isConfig bool) (types.BlobInfo, error) {
	if inputInfo.Digest.String() != "" {
		ok, info, err := o.TryReusingBlob(ctx, inputInfo, none.NoCache, false)
		if err != nil {
			return types.BlobInfo{}, err
		}
		if ok {
			return info, nil
		}
	}

	fmt.Printf("calling startlayer\n")
	// Do this as a chunked upload so we can calculate the digest, since
	// caller is not giving it to us.
	path, err := o.s.StartLayer()
	fmt.Printf("callled startlayer and got %s %v\n", path, err)
	if err != nil {
		return types.BlobInfo{}, err
	}
	digest, size, err := o.s.CompleteLayer(path, stream, inputInfo.Size)
	fmt.Printf("callled completelayer and got %v %d %v\n", digest, size, err)
	return types.BlobInfo{Digest: digest, Size: size}, err
}

// HasThreadSafePutBlob indicates whether PutBlob can be executed concurrently.
func (o *ociMotelImageDest) HasThreadSafePutBlob() bool {
	return true
}

func (o *ociMotelImageDest) TryReusingBlob(ctx context.Context, info types.BlobInfo, cache types.BlobInfoCache, canSubstitute bool) (bool, types.BlobInfo, error) {
	if info.Digest == "" {
		return false, types.BlobInfo{}, errors.Errorf(`"Can not check for a blob with unknown digest`)
	}
	if o.s.HasLayer(info.Digest.String()) {
		return true, types.BlobInfo{Digest: info.Digest, Size: -1}, nil
	}
	return false, types.BlobInfo{}, nil
}

func (o *ociMotelImageDest) PutManifest(ctx context.Context, m []byte) error {
	return o.s.PutManifest(m)
}

func (o *ociMotelImageDest) PutSignatures(ctx context.Context, signatures [][]byte) error {
	return nil // TODO
}

func (o *ociMotelImageDest) Commit(ctx context.Context) error {
	return nil
}
