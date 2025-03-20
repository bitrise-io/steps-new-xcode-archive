package localcodesignasset

import (
	"github.com/bitrise-io/go-xcode/profileutil"
	"github.com/bitrise-io/go-xcode/v2/autocodesign"
	"github.com/bitrise-io/go-xcode/v2/autocodesign/devportalclient/appstoreconnect"
	"github.com/bitrise-io/go-xcode/v2/autocodesign/devportalclient/time"
)

// Profile ...
type Profile struct {
	attributes     appstoreconnect.ProfileAttributes
	id             string
	bundleID       string
	deviceUDIDs    []string
	certificateIDs []string
}

// NewProfile wraps a local profile in the autocodesign.Profile interface
func NewProfile(info profileutil.ProvisioningProfileInfoModel, content []byte) autocodesign.Profile {
	return Profile{
		attributes: appstoreconnect.ProfileAttributes{
			Name:           info.Name,
			UUID:           info.UUID,
			ProfileContent: content,
			Platform:       getBundleIDPlatform(info.Type),
			ExpirationDate: time.Time(info.ExpirationDate),
		},
		id:             "", // only in case of Developer Portal Profiles
		bundleID:       info.BundleID,
		certificateIDs: nil, // only in case of Developer Portal Profiles
		deviceUDIDs:    nil, // (recheck this) only in case of Developer Portal Profiles
	}
}

// ID ...
func (p Profile) ID() string {
	return p.id
}

// Attributes ...
func (p Profile) Attributes() appstoreconnect.ProfileAttributes {
	return p.attributes
}

// CertificateIDs ...
func (p Profile) CertificateIDs() ([]string, error) {
	return p.certificateIDs, nil
}

// DeviceUDIDs ...
func (p Profile) DeviceUDIDs() ([]string, error) {
	return p.deviceUDIDs, nil
}

// BundleID ...
func (p Profile) BundleID() (appstoreconnect.BundleID, error) {
	return appstoreconnect.BundleID{
		ID: p.id,
		Attributes: appstoreconnect.BundleIDAttributes{
			Identifier: p.bundleID,
			Name:       p.attributes.Name,
		},
	}, nil
}

// Entitlements ...
func (p Profile) Entitlements() (autocodesign.Entitlements, error) {
	return autocodesign.ParseRawProfileEntitlements(p.attributes.ProfileContent)
}

func getBundleIDPlatform(profileType profileutil.ProfileType) appstoreconnect.BundleIDPlatform {
	switch profileType {
	case profileutil.ProfileTypeIos, profileutil.ProfileTypeTvOs:
		return appstoreconnect.IOS
	case profileutil.ProfileTypeMacOs:
		return appstoreconnect.MacOS
	}

	return ""
}
