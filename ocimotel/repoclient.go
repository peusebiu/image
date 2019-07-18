package ocimotel

import (
	"bytes"
	"crypto/sha256"
	"crypto/tls"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"net/url"
	"strconv"

	"github.com/containers/image/pkg/tlsclientconfig"
	"github.com/containers/image/types"
	"github.com/opencontainers/go-digest"
	ispec "github.com/opencontainers/image-spec/specs-go/v1"
	"github.com/pkg/errors"
)

type OciRepo struct {
	url       url.URL
	ref       *ociMotelReference
	authCreds string
	client    *http.Client
}

func NewOciRepo(ref *ociMotelReference, sys *types.SystemContext) (r OciRepo, err error) {
	server := "127.0.0.1"
	port := "8080"
	if ref.server != "" {
		server = ref.server
	}
	if ref.port != -1 {
		port = fmt.Sprintf("%d", ref.port)
	}

	insecureSkipVerify := (sys.DockerInsecureSkipTLSVerify == types.OptionalBoolTrue)
	tlsClientConfig := &tls.Config{
		InsecureSkipVerify: insecureSkipVerify,
	}

	if sys.DockerCertPath != "" {
		if err := tlsclientconfig.SetupCertificates(sys.DockerCertPath, tlsClientConfig); err != nil {
			return r, err
		}
	}
	transport := &http.Transport{TLSClientConfig: tlsClientConfig}
	client := &http.Client{Transport: transport}
	creds := ""
	if sys.DockerAuthConfig != nil {
		a := sys.DockerAuthConfig
		creds = base64.StdEncoding.EncodeToString([]byte(a.Username + ":" + a.Password))
	}
	r = OciRepo{ref: ref, authCreds: creds, client: client}

	ping := func(scheme string) error {
		u := url.URL{Scheme: scheme, Host: fmt.Sprintf("%s:%s", server, port)}
		u.Path = fmt.Sprintf("/v2/")
		resp, err := client.Get(u.String())
		if err != nil {
			return err
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusUnauthorized {
			return errors.Errorf("error pinging registry %s:%s, response code %d (%s)", server, port, resp.StatusCode, http.StatusText(resp.StatusCode))
		}
		return nil
	}

	scheme := "https"
	err = ping(scheme)
	if err != nil && insecureSkipVerify {
		scheme = "http"
		err = ping(scheme)
	}
	if err != nil {
		return r, errors.Wrap(err, "unable to ping registry")
	}

	r.url = url.URL{Scheme: scheme, Host: fmt.Sprintf("%s:%s", server, port)}
	return r, nil
}

func (o *OciRepo) GetManifest() ([]byte, *ispec.Manifest, error) {
	name := o.ref.name
	tag := o.ref.tag
	m := &ispec.Manifest{}
	var body []byte
	uri := o.url
	uri.Path = fmt.Sprintf("/v2/%s/manifests/%s", name, tag)
	req, err := http.NewRequest("GET", uri.String(), nil)
	if err != nil {
		return body, m, errors.Wrapf(err, "Couldn't create DELETE request for %s", uri)
	}
	if o.authCreds != "" {
		req.Header.Add("Authorization", "Basic "+o.authCreds)
	}
	resp, err := o.client.Do(req)
	if err != nil {
		return body, m, errors.Wrapf(err, "Error getting manifest %s %s from %s", name, tag, o.url)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return body, m, fmt.Errorf("Bad return code %d getting manifest", resp.StatusCode)
	}
	body, err = ioutil.ReadAll(resp.Body)
	if err != nil {
		return body, m, errors.Wrapf(err, "Error reading response body for %s", tag)
	}
	err = json.Unmarshal(body, m)
	if err != nil {
		return body, m, errors.Wrap(err, "Failed decoding response")
	}
	return body, m, nil
}

func (o *OciRepo) RemoveManifest() error {
	name := o.ref.name
	tag := o.ref.tag
	uri := o.url
	uri.Path = fmt.Sprintf("/v2/%s/manifests/%s", name, tag)
	req, err := http.NewRequest("DELETE", uri.String(), nil)
	if err != nil {
		return errors.Wrapf(err, "Couldn't create DELETE request for %s", uri)
	}
	if o.authCreds != "" {
		req.Header.Add("Authorization", "Basic "+o.authCreds)
	}
	resp, err := o.client.Do(req)
	if err != nil {
		return errors.Wrapf(err, "Error deleting manifest")
	}
	if resp.StatusCode != 202 {
		return fmt.Errorf("Server returned unexpected code %d", resp.StatusCode)
	}
	return nil
}

func (o *OciRepo) PutManifest(body []byte) error {
	name := o.ref.name
	tag := o.ref.tag
	uri := o.url
	uri.Path = fmt.Sprintf("/v2/%s/manifests/%s", name, tag)

	req, err := http.NewRequest("PUT", uri.String(), bytes.NewReader(body))
	if err != nil {
		return errors.Wrapf(err, "Couldn't create PUT request for %s", uri)
	}
	if o.authCreds != "" {
		req.Header.Add("Authorization", "Basic "+o.authCreds)
	}
	req.Header.Set("Content-Type", "application/vnd.oci.image.manifest.v1+json")
	resp, err := o.client.Do(req)
	if err != nil {
		return errors.Wrapf(err, "Error posting manifest")
	}
	if resp.StatusCode != 201 {
		return fmt.Errorf("Server returned unexpected code %d", resp.StatusCode)
	}
	defer resp.Body.Close()

	return nil
}

//HEAD /v2/<name>/blobs/<digest>  -> 200 (has layer)
func (o *OciRepo) HasLayer(ldigest string) bool {
	name := o.ref.name
	uri := o.url
	uri.Path = fmt.Sprintf("/v2/%s/blobs/%s", name, ldigest)
	req, err := http.NewRequest("HEAD", uri.String(), nil)
	if err != nil {
		return false
	}
	if o.authCreds != "" {
		req.Header.Add("Authorization", "Basic "+o.authCreds)
	}
	resp, err := o.client.Do(req)
	if err != nil {
		return false
	}
	defer resp.Body.Close()
	return resp.StatusCode == 200
}

func (o *OciRepo) GetLayer(ldigest string) (io.ReadCloser, int64, error) {
	name := o.ref.name
	uri := o.url
	uri.Path = fmt.Sprintf("/v2/%s/blobs/%s", name, ldigest)
	req, err := http.NewRequest("GET", uri.String(), nil)
	if err != nil {
		return nil, -1, errors.Wrapf(err, "Couldn't create GET request for %s", uri)
	}
	if o.authCreds != "" {
		req.Header.Add("Authorization", "Basic "+o.authCreds)
	}
	resp, err := o.client.Do(req)
	if err != nil {
		return nil, -1, errors.Wrapf(err, "Error getting layer %s", ldigest)
	}
	if resp.StatusCode != 200 {
		resp.Body.Close()
		return nil, -1, fmt.Errorf("Bad return code %d getting layer", resp.StatusCode)
	}
	return resp.Body, -1, err
}

type layerPutResult struct {
	Location string `json:"Location"`
	Length   string `json:"Content-Length"`
	Digest   string `json:"Digest"`
}

type layerPostResult struct {
	Location string `json:"Location"`
	Range    string `json:"Range"`
	Length   string `json:"Content-Length"`
}

func (o *OciRepo) StartLayer() (string, error) {
	name := o.ref.name
	uri := o.url
	uri.Path = fmt.Sprintf("/v2/%s/blobs/uploads/", name)
	req, err := http.NewRequest("POST", uri.String(), nil)
	if err != nil {
		return "", errors.Wrap(err, "Failed opening POST request")
	}
	if o.authCreds != "" {
		req.Header.Add("Authorization", "Basic "+o.authCreds)
	}
	resp, err := o.client.Do(req)
	if err != nil {
		return "", errors.Wrapf(err, "Failed posting request %v", req)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 202 {
		return "", fmt.Errorf("Server returned an error %d", resp.StatusCode)
	}

	_, err = ioutil.ReadAll(resp.Body)
	if err != nil {
		return "", errors.Wrapf(err, "Error reading response body for %s", name)
	}

	loc := resp.Header.Get("Location")
	if loc == "" {
		return "", errors.Wrap(err, "Failed decoding response")
	}

	return loc, nil
}

// @path is the uuid upload path returned by the server to our Post request.
// @stream is the data source for the layer.
// Return the digest and size of the layer that was uploaded.
func (o *OciRepo) CompleteLayer(path string, stream io.Reader) (digest.Digest, int64, error) {
	uri := o.url
	uri.Path = path
	digester := sha256.New()
	hashReader := io.TeeReader(stream, digester)
	// using "chunked" upload
	count := int64(0)
	for {
		const maxSize = 10 * 1024 * 1024
		var buf bytes.Buffer
		size, err := io.CopyN(&buf, hashReader, maxSize)
		if size == 0 {
			if err != io.EOF {
				return "", -1, errors.Wrapf(err, "Failed to copy stream")
			}
			break
		}
		req, err := http.NewRequest("PATCH", uri.String(), &buf)
		if err != nil {
			return "", -1, errors.Wrap(err, "Failed opening Patch request")
		}
		if o.authCreds != "" {
			req.Header.Add("Authorization", "Basic "+o.authCreds)
		}

		req.ContentLength = size
		req.Header.Set("Content-Type", "application/octet-stream")
		req.Header.Set("Content-Range", fmt.Sprintf("%d-%d", count, count+size))
		resp, err := o.client.Do(req)
		if err != nil {
			return "", -1, errors.Wrapf(err, "Failed posting request %v", req)
		}
		defer resp.Body.Close()
		if resp.StatusCode != 202 {
			return "", -1, fmt.Errorf("Server returned an error %d", resp.StatusCode)
		}
		count += size
	}

	ourDigest := fmt.Sprintf("%x", digester.Sum(nil))
	d := digest.NewDigestFromEncoded(digest.SHA256, ourDigest)
	q := uri.Query()
	q.Set("digest", d.String())
	uri.RawQuery = q.Encode()
	req, err := http.NewRequest("PUT", uri.String(), nil)
	if err != nil {
		return "", -1, errors.Wrap(err, "Failed opening Put request")
	}
	if o.authCreds != "" {
		req.Header.Add("Authorization", "Basic "+o.authCreds)
	}
	req.Header.Set("Content-Range", fmt.Sprintf("%d-%d", 0, count))
	putResp, err := o.client.Do(req)
	if err != nil {
		return "", -1, errors.Wrapf(err, "Failed putting request %v", req)
	}
	defer putResp.Body.Close()
	if putResp.StatusCode != 201 {
		return "", -1, fmt.Errorf("Server returned an error %d", putResp.StatusCode)
	}

	servDigest, ok := putResp.Header["Docker-Content-Digest"]
	if !ok || len(servDigest) != 1 {
		return "", -1, fmt.Errorf("Server returned incomplete headers")
	}

	blobLoc, ok := putResp.Header["Location"]
	if !ok || len(blobLoc) != 1 {
		return "", -1, fmt.Errorf("Server returned incomplete headers")
	}

	uri.Path = fmt.Sprintf("/%s", blobLoc[0])
	req, err = http.NewRequest("HEAD", uri.String(), nil)
	if err != nil {
		return "", -1, errors.Wrap(err, "Failed opening Head request")
	}
	if o.authCreds != "" {
		req.Header.Add("Authorization", "Basic "+o.authCreds)
	}
	resp, err := o.client.Do(req)
	if err != nil {
		return "", -1, errors.Wrapf(err, "Failed getting new layer %v", blobLoc[0])
	}

	Length, ok := resp.Header["Content-Length"]
	if !ok || len(Length) != 1 {
		return "", -1, fmt.Errorf("Server returned incomplete headers")
	}
	length, err := strconv.ParseInt(Length[0], 10, 64)
	if err != nil {
		return "", -1, errors.Wrap(err, "Failed decoding length in response")
	}

	if servDigest[0] != d.String() {
		return "", -1, errors.Wrapf(err, "Server calculated digest %s, not our %s", servDigest[0], ourDigest)
	}

	// TODO ocimotel is returning the wrong thing - the hash,
	// not the "digest", which is "sha256:hash"

	return d, length, nil
}
