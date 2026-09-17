package packageinfo

import (
	"encoding/xml"
	"fmt"
)

// Info defines the PackageInfo structure for macOS flat packages.
type Info struct {
	XMLName              xml.Name `xml:"pkg-info"`
	FormatVersion        string   `xml:"format-version,attr"`
	Identifier           string   `xml:"identifier,attr"`
	Version              string   `xml:"version,attr"`
	InstallLocation      string   `xml:"install-location,attr"`
	Auth                 string   `xml:"auth,attr,omitempty"` // "root" or empty
	OverwritePermissions string   `xml:"overwrite-permissions,attr,omitempty"`
	Relocatable          string   `xml:"relocatable,attr,omitempty"`
	PostinstallAction    string   `xml:"postinstall-action,attr,omitempty"`
	Payload              Payload  `xml:"payload"`
	Bundle               *Bundle  `xml:"bundle,omitempty"`
}

type Payload struct {
	InstallKBytes uint64 `xml:"installKBytes,attr"`
	NumberOfFiles uint64 `xml:"numberOfFiles,attr"`
}

type Bundle struct {
	ID                 string `xml:"id,attr"`
	Path               string `xml:"path,attr"`
	CFBundleIdentifier string `xml:"CFBundleIdentifier,attr"`
	CFBundleVersion    string `xml:"CFBundleVersion,attr"`
}

// Generate formats Info as PackageInfo XML.
func Generate(info Info) ([]byte, error) {
	if info.FormatVersion == "" {
		info.FormatVersion = "2"
	}
	if info.Auth == "" {
		info.Auth = "root"
	}
	if info.InstallLocation == "" {
		info.InstallLocation = "/Applications"
	}
	if info.OverwritePermissions == "" {
		info.OverwritePermissions = "true"
	}
	if info.Relocatable == "" {
		info.Relocatable = "false"
	}
	if info.PostinstallAction == "" {
		info.PostinstallAction = "none"
	}

	xmlBytes, err := xml.MarshalIndent(info, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("marshal PackageInfo: %w", err)
	}

	header := []byte("<?xml version=\"1.0\" encoding=\"utf-8\"?>\n")
	return append(header, xmlBytes...), nil
}
