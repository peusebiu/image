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

func splitReference(ref string) (fullname, server string, port int, err error) {
	port = 8080
	err = nil
	if ref[0] == '/' {
		fullname = ref[1:]
		return
	}
	fields := strings.SplitN(ref, "/", 2)
	subFields := strings.Split(fields[0], ":")
	if len(subFields) > 2 {
		err = fmt.Errorf("Bad server:port")
		return
	}
	server = subFields[0]
	if len(subFields) == 2 {
		port, err = strconv.Atoi(subFields[1])
		if err != nil {
			return
		}
		if port < 1 || port > 65535 {
			err = fmt.Errorf("bad port %d", port)
			return
		}
	}
	fullname = fields[1]
	return
}

// NOTE - the transport interface is defined in types/types.go.
// Valid uris are:
//    ocimotel:///name1/name2/tag
//    ocimotel://server/name1/name2/name3/tag
// The tag can be separated by either / or :
//    ocimotel://server:port/name1/name2/name3/tag
//    ocimotel://server:port/name1/name2/name3:tag
// So the reference passed in here would be e.g.
//    ///name1/name2/tag
//    //server:port/name1/name2/tag
func (s ociMotelTransport) ParseReference(reference string) (types.ImageReference, error) {
	if !strings.HasPrefix(reference, "//") {
		return nil, errors.Errorf("ocimotel: image reference %s does not start with //", reference)
	}
	fields := strings.Split(reference, "/")
	fullname, server, port, err := splitReference(reference[2:])
	if err != nil {
		return nil, errors.Wrapf(err, "Failed parsing reference: '%s'", reference)
	}

	// support : for tag separateion
	var name, tag string
	fields = strings.Split(fullname, ":")
	if len(fields) != 2 || len(fields[0]) == 0 || len(fields[1]) == 0 {
		return nil, fmt.Errorf("No tag specified in '%s'", fullname)
	}
	name = fields[0]
	tag = fields[1]

	return ociMotelReference{
		server:   server,
		port:     port,
		fullname: fullname,
		name:     name,
		tag:      tag,
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
