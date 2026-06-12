/*
Copyright © 2025-2026 SUSE LLC
SPDX-License-Identifier: Apache-2.0

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package deployment

import (
	"fmt"
	"net/url"
	"path/filepath"

	"github.com/distribution/reference"
	"go.yaml.in/yaml/v3"
)

type ImageSrcType int

const (
	Dir ImageSrcType = iota + 1
	OCI
	Raw
	Tar
)

func ParseSrcImageType(i string) (ImageSrcType, error) {
	switch i {
	case "", "oci":
		return OCI, nil
	case "dir":
		return Dir, nil
	case "raw":
		return Raw, nil
	case "tar":
		return Tar, nil
	default:
		return ImageSrcType(0), fmt.Errorf("image source type not supported: %s", i)
	}
}

func (i ImageSrcType) String() string {
	switch i {
	case OCI:
		return "oci"
	case Dir:
		return "dir"
	case Raw:
		return "raw"
	case Tar:
		return "tar"
	default:
		return Unknown
	}
}

type ImageSource struct {
	uri        string
	digest     string
	srcType    ImageSrcType
	provenance *ImageSource
}

var (
	_ yaml.Marshaler   = ImageSource{}
	_ yaml.Unmarshaler = (*ImageSource)(nil)
)

func (i *ImageSource) SetDigest(digest string) {
	i.digest = digest
}

func (i ImageSource) GetDigest() string {
	return i.digest
}

func (i *ImageSource) SetProvenance(provenance *ImageSource) {
	if provenance == nil || provenance.IsEmpty() {
		i.provenance = nil
		return
	}
	i.provenance = provenance
}

func (i ImageSource) Provenance() *ImageSource {
	return i.provenance
}

func (i ImageSource) URI() string {
	return i.uri
}

func (i ImageSource) IsOCI() bool {
	return i.srcType == OCI
}

func (i ImageSource) IsDir() bool {
	return i.srcType == Dir
}

func (i ImageSource) IsRaw() bool {
	return i.srcType == Raw
}

func (i ImageSource) IsTar() bool {
	return i.srcType == Tar
}

func (i ImageSource) IsEmpty() bool {
	if i.srcType == 0 {
		return true
	}
	if i.uri == "" {
		return true
	}
	return false
}

func (i ImageSource) String() string {
	if i.IsEmpty() {
		return ""
	}
	return fmt.Sprintf("%s://%s", i.srcType, i.uri)
}

func NewSrcFromURI(uri string) (*ImageSource, error) {
	src := ImageSource{}
	err := src.updateFromURI(uri)
	return &src, err
}

func NewEmptySrc() *ImageSource {
	return &ImageSource{}
}

func NewOCISrc(src string) *ImageSource {
	return &ImageSource{uri: src, srcType: OCI}
}

func NewRawSrc(src string) *ImageSource {
	return &ImageSource{uri: src, srcType: Raw}
}

func NewDirSrc(src string) *ImageSource {
	return &ImageSource{uri: src, srcType: Dir}
}

func NewTarSrc(src string) *ImageSource {
	return &ImageSource{uri: src, srcType: Tar}
}

func (i ImageSource) MarshalYAML() (any, error) {
	type imageSource struct {
		Digest     string       `yaml:"digest,omitempty"`
		URI        string       `yaml:"uri"`
		Provenance *ImageSource `yaml:"provenance,omitempty"`
	}
	imgSrc := imageSource{}
	if i.digest != "" {
		imgSrc.Digest = i.digest
	}
	imgSrc.URI = i.String()
	if i.provenance != nil && !i.provenance.IsEmpty() {
		imgSrc.Provenance = i.provenance
	}

	n := &yaml.Node{}
	err := n.Encode(imgSrc)

	return n, err
}

func (i *ImageSource) UnmarshalYAML(data *yaml.Node) (err error) {
	type imageSource struct {
		Digest     string       `yaml:"digest,omitempty"`
		URI        string       `yaml:"uri"`
		Provenance *ImageSource `yaml:"provenance,omitempty"`
	}
	imgSrc := imageSource{}
	if err = data.Decode(&imgSrc); err != nil {
		return err
	}
	if imgSrc.URI == "" {
		return fmt.Errorf("no 'uri' provided for the image source: %s", string(data.Value))
	}

	err = i.updateFromURI(imgSrc.URI)
	if err != nil {
		return err
	}
	i.digest = imgSrc.Digest
	i.SetProvenance(imgSrc.Provenance)
	return err
}

func (i *ImageSource) updateFromURI(uri string) error {
	u, err := url.Parse(uri)
	if err != nil {
		return err
	}
	scheme := u.Scheme
	value := u.Opaque
	if value == "" {
		value = filepath.Join(u.Host, u.Path)
	}
	srcType, err := ParseSrcImageType(scheme)
	if err != nil {
		return err
	}
	i.srcType = srcType
	i.uri = value
	if scheme == "" {
		uri, err = parseImageReference(uri)
		if err != nil {
			return err
		}
		i.uri = uri
		return nil
	}
	if srcType == OCI {
		uri, err = parseImageReference(value)
		if err != nil {
			return err
		}
		i.uri = uri
	}
	return nil
}

func parseImageReference(ref string) (string, error) {
	n, err := reference.ParseNormalizedNamed(ref)
	if err != nil {
		return "", fmt.Errorf("invalid image reference %s", ref)
	} else if reference.IsNameOnly(n) {
		ref += ":latest"
	}
	return ref, nil
}
