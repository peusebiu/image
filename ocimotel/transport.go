package ocimotel

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/containers/image/docker/reference"
	"github.com/containers/image/image"
	"github.com/containers/image/transports"
	"github.com/containers/image/types"
	"github.com/pkg/errors"
)

func init() {
	transports.Register(Transport)
}

var Transport = ociMotelTransport{}

type ociMotelTransport struct{}

func (o ociMotelTransport) Name() string {
	return "ocimotel"
}

// NOTE - the transport interface is defined in types/types.go.

// OPEN TO SUGGESTIONS.  I think we want server to be optional, with no server
// meaning query the cluster (todo).  That makes doing //server/rest difficult,
// because we will have a hard time deciding whether the first part of the nae
// is a server or a name.  So I'm saying we separate server from name with a :.
// Valid uris are:
//    omot://:name1/name2/tag
//    omot://server:/name1/name2/name3/tag
//    omot://server:port:/name1/name2/name3/tag
// So the reference passed in here would be e.g.
//    //:name1/name2/tag
//    //server:port:name1/name2/tag
func (s ociMotelTransport) ParseReference(reference string) (types.ImageReference, error) {
	if !strings.HasPrefix(reference, "//") {
		return nil, errors.Errorf("ocimotel: image reference %s does not start with //", reference)
	}
	fields := strings.SplitN(reference, ":", 3)
	port := -1
	server := ""
	fullname := ""
	empty := ociMotelReference{}
	if len(fields) < 2 {
		return empty, fmt.Errorf("ocimotel: bad image reference format: %s", reference)
	}
	if fields[0] != "//" {
		server = fields[0][2:]
	}
	if len(fields) == 3 {
		port, err := strconv.Atoi(fields[1])
		if err != nil {
			return empty, errors.Wrapf(err, "ocimotel: bad port in %s", reference)
		}
		if port < 1 || port > 65535 {
			return empty, fmt.Errorf("ocimotel: bad port in %s", reference)
		}
		fullname = fields[2]
	} else {
		fullname = fields[1]
	}
	nameFields := strings.Split(fullname, "/")
	numFields := len(nameFields)
	if numFields < 2 {
		return empty, fmt.Errorf("ocimotel: no tag or digest in %s", reference)
	}
	return ociMotelReference{
		server:   server,
		port:     port,
		fullname: fullname,
		name:     strings.Join(nameFields[:numFields-1], "/"),
		tag:      nameFields[numFields-1],
	}, nil
}

func (s ociMotelTransport) ValidatePolicyConfigurationScope(scope string) error {
	return nil
}

type ociMotelReference struct {
	server   string
	port     int
	fullname string
	name     string
	tag      string
}

func (ref ociMotelReference) Transport() types.ImageTransport {
	return Transport
}

func (ref ociMotelReference) StringWithinTransport() string {
	port := ""
	if ref.port != -1 {
		port = fmt.Sprintf("%s:", ref.port)
	}
	return fmt.Sprintf("//%s:%s%s", ref.server, port, ref.fullname)
}

func (ref ociMotelReference) DockerReference() reference.Named {
	return nil
}

func (ref ociMotelReference) PolicyConfigurationIdentity() string {
	return ref.StringWithinTransport()
}

func (ref ociMotelReference) PolicyConfigurationNamespaces() []string {
	return []string{}
}

func (ref ociMotelReference) NewImage(ctx context.Context, sys *types.SystemContext) (types.ImageCloser, error) {
	src, err := ref.NewImageSource(ctx, sys)
	if err != nil {
		return nil, err
	}
	return image.FromSource(ctx, sys, src)
}

func (ref ociMotelReference) NewImageSource(ctx context.Context, sys *types.SystemContext) (types.ImageSource, error) {
	s, err := NewOciRepo(&ref)
	if err != nil {
		return nil, errors.Wrap(err, "Failed connecting to server")
	}
	return &ociMotelImageSource{
		ref: ref,
		s:   &s,
	}, nil
}

func (ref ociMotelReference) NewImageDestination(ctx context.Context, sys *types.SystemContext) (types.ImageDestination, error) {
	s, err := NewOciRepo(&ref)
	if err != nil {
		return nil, errors.Wrap(err, "Failed connecting to server")
	}
	return &ociMotelImageDest{
		ref: ref,
		s:   &s,
	}, nil
}

func (ref ociMotelReference) DeleteImage(ctx context.Context, sys *types.SystemContext) error {
	s, err := NewOciRepo(&ref)
	if err != nil {
		return errors.Wrap(err, "Failed connecting to server")
	}
	return s.RemoveManifest()
}
