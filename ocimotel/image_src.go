package ocimotel

import (
	"context"
	"fmt"
	"io"

	"github.com/containers/image/types"
	"github.com/opencontainers/go-digest"
	ispec "github.com/opencontainers/image-spec/specs-go/v1"
	"github.com/pkg/errors"
)

// NOTE - the ImageSource interface is defined and commented in types.go

type ociMotelImageSource struct {
	s              *OciRepo
	ref            ociMotelReference
	manifest       *ispec.Manifest
	cachedManifest []byte
}

func (o *ociMotelImageSource) Reference() types.ImageReference {
	return o.ref
}

func (o *ociMotelImageSource) Close() error {
	return nil
}

func (o *ociMotelImageSource) GetManifest(ctx context.Context, instanceDigest *digest.Digest) ([]byte, string, error) {
	if instanceDigest != nil {
		return nil, "", fmt.Errorf("GetManifest with instanceDigest is not implemented")
	}
	if o.manifest == nil {
		bytes, m, err := o.s.GetManifest()
		if err != nil {
			return nil, "", errors.Wrap(err, "Failed fetching manifest")
		}
		o.cachedManifest = bytes
		o.manifest = m
	}
	return o.cachedManifest, ispec.MediaTypeImageManifest, nil
}

func (o *ociMotelImageSource) GetBlob(ctx context.Context, info types.BlobInfo, cache types.BlobInfoCache) (io.ReadCloser, int64, error) {
	digest := info.Digest.Encoded()
	return o.s.GetLayer(digest)
}

func (o *ociMotelImageSource) HasThreadSafeGetBlob() bool {
	return true
}

func (o *ociMotelImageSource) GetSignatures(ctx context.Context, instanceDigest *digest.Digest) ([][]byte, error) {
	return [][]byte{}, nil // TODO
}

func (o *ociMotelImageSource) LayerInfosForCopy(ctx context.Context) ([]types.BlobInfo, error) {
	return nil, nil
}
