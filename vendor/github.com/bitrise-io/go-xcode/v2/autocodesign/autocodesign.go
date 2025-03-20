// Package autocodesign is a framework for automatic code signing.
//
// Contains common types, interfaces and logic needed for codesigning.
// Parsing an Xcode project or archive and applying settings is not part of the package, for modularity.
package autocodesign

import (
	"errors"
	"fmt"
	"math/big"

	"github.com/bitrise-io/go-utils/log"
	"github.com/bitrise-io/go-xcode/certificateutil"
	"github.com/bitrise-io/go-xcode/profileutil"
	"github.com/bitrise-io/go-xcode/v2/autocodesign/devportalclient/appstoreconnect"
	"github.com/bitrise-io/go-xcode/v2/devportalservice"
	"github.com/bitrise-io/go-xcode/xcodeproject/serialized"
)

// Profile represents a provisioning profiles
type Profile interface {
	ID() string
	Attributes() appstoreconnect.ProfileAttributes
	CertificateIDs() ([]string, error)
	DeviceUDIDs() ([]string, error)
	BundleID() (appstoreconnect.BundleID, error)
	Entitlements() (Entitlements, error)
}

// AppCodesignAssets is the result of ensuring codesigning assets
type AppCodesignAssets struct {
	ArchivableTargetProfilesByBundleID map[string]Profile
	UITestTargetProfilesByBundleID     map[string]Profile
	Certificate                        certificateutil.CertificateInfoModel
}

// Platform ...
type Platform string

// Const
const (
	IOS   Platform = "iOS"
	TVOS  Platform = "tvOS"
	MacOS Platform = "macOS"
)

// DistributionType ...
type DistributionType string

// DistributionTypes ...
var (
	Development DistributionType = "development"
	AppStore    DistributionType = "app-store"
	AdHoc       DistributionType = "ad-hoc"
	Enterprise  DistributionType = "enterprise"
)

// Entitlement ...
type Entitlement serialized.Object

// Entitlements is all the entitlements that are contained in a target or profile
type Entitlements serialized.Object

// Certificate is certificate present on Apple App Store Connect API, could match a local certificate
type Certificate struct {
	CertificateInfo certificateutil.CertificateInfoModel
	ID              string
}

// DeviceIDs is the opaque ID of a device (from used in the Developer Portal API)
type DeviceIDs []string

// DeviceUDIDs are used in the provisioning profiles, as unique device identifiers
type DeviceUDIDs []string

// DevPortalClient abstract away the Apple Developer Portal API
type DevPortalClient interface {
	Login() error

	QueryCertificateBySerial(serial big.Int) (Certificate, error)
	QueryAllIOSCertificates() (map[appstoreconnect.CertificateType][]Certificate, error)

	ListDevices(UDID string, platform appstoreconnect.DevicePlatform) ([]appstoreconnect.Device, error)
	RegisterDevice(testDevice devportalservice.TestDevice) (*appstoreconnect.Device, error)

	FindProfile(name string, profileType appstoreconnect.ProfileType) (Profile, error)
	DeleteProfile(id string) error
	CreateProfile(name string, profileType appstoreconnect.ProfileType, bundleID appstoreconnect.BundleID, certificateIDs []string, deviceIDs []string) (Profile, error)

	FindBundleID(bundleIDIdentifier string) (*appstoreconnect.BundleID, error)
	CheckBundleIDEntitlements(bundleID appstoreconnect.BundleID, appEntitlements Entitlements) error
	SyncBundleID(bundleID appstoreconnect.BundleID, appEntitlements Entitlements) error
	CreateBundleID(bundleIDIdentifier, appIDName string) (*appstoreconnect.BundleID, error)
}

// AssetWriter ...
type AssetWriter interface {
	Write(codesignAssetsByDistributionType map[DistributionType]AppCodesignAssets) error
	InstallCertificate(certificate certificateutil.CertificateInfoModel) error
	InstallProfile(profile Profile) error
}

// LocalCodeSignAssetManager ...
type LocalCodeSignAssetManager interface {
	FindCodesignAssets(appLayout AppLayout, distrType DistributionType, certsByType map[appstoreconnect.CertificateType][]Certificate, deviceIDs []string, minProfileDaysValid int) (*AppCodesignAssets, *AppLayout, error)
}

// AppLayout contains codesigning related settings that are needed to ensure codesigning files
type AppLayout struct {
	Platform                               Platform
	EntitlementsByArchivableTargetBundleID map[string]Entitlements
	UITestTargetBundleIDs                  []string
}

// CertificateProvider returns codesigning certificates (with private key)
type CertificateProvider interface {
	GetCertificates() ([]certificateutil.CertificateInfoModel, error)
}

// LocalCertificates is a map from the certificate type (development, distribution) to an array of installed certs
type LocalCertificates map[appstoreconnect.CertificateType][]certificateutil.CertificateInfoModel

// LocalProfile ...
type LocalProfile struct {
	Profile Profile
	Info    profileutil.ProvisioningProfileInfoModel
}

// ProfileProvider returns provisioning profiles
type ProfileProvider interface {
	IsAvailable() bool
	GetProfiles() ([]LocalProfile, error)
}

// CodesignAssetsOpts are codesigning related parameters that are not specified by the project (or archive)
type CodesignAssetsOpts struct {
	DistributionType                  DistributionType
	TypeToLocalCertificates           LocalCertificates
	BitriseTestDevices                []devportalservice.TestDevice
	MinProfileValidityDays            int
	FallbackToLocalAssetsOnAPIFailure bool
	VerboseLog                        bool
}

// CodesignAssetManager ...
type CodesignAssetManager interface {
	EnsureCodesignAssets(appLayout AppLayout, opts CodesignAssetsOpts) (map[DistributionType]AppCodesignAssets, error)
}

type codesignAssetManager struct {
	devPortalClient           DevPortalClient
	assetWriter               AssetWriter
	localCodeSignAssetManager LocalCodeSignAssetManager
}

// NewCodesignAssetManager ...
func NewCodesignAssetManager(devPortalClient DevPortalClient, assetWriter AssetWriter, localCodeSignAssetManager LocalCodeSignAssetManager) CodesignAssetManager {
	return codesignAssetManager{
		devPortalClient:           devPortalClient,
		assetWriter:               assetWriter,
		localCodeSignAssetManager: localCodeSignAssetManager,
	}
}

// EnsureCodesignAssets is the main entry point of the codesigning logic
func (m codesignAssetManager) EnsureCodesignAssets(appLayout AppLayout, opts CodesignAssetsOpts) (map[DistributionType]AppCodesignAssets, error) {
	signUITestTargets := len(appLayout.UITestTargetBundleIDs) > 0
	certsByType, distrTypes, err := selectCertificatesAndDistributionTypes(
		m.devPortalClient,
		opts.TypeToLocalCertificates,
		opts.DistributionType,
		signUITestTargets,
		opts.VerboseLog,
	)
	if err != nil {
		return nil, err
	}

	var devPortalDeviceIDs DeviceIDs
	var devPortalDeviceUDIDs DeviceUDIDs
	if DistributionTypeRequiresDeviceList(distrTypes) {
		devPortalDevices, err := EnsureTestDevices(m.devPortalClient, opts.BitriseTestDevices, appLayout.Platform)
		if err != nil {
			return nil, fmt.Errorf("failed to ensure test devices: %w", err)
		}

		for _, devPortalDevice := range devPortalDevices {
			devPortalDeviceIDs = append(devPortalDeviceIDs, devPortalDevice.ID)
			devPortalDeviceUDIDs = append(devPortalDeviceUDIDs, devPortalDevice.Attributes.UDID)
		}
	}

	codesignAssetsByDistributionType := map[DistributionType]AppCodesignAssets{}

	for _, distrType := range distrTypes {
		localCodesignAssets, missingAppLayout, err := m.localCodeSignAssetManager.FindCodesignAssets(appLayout, distrType, certsByType, devPortalDeviceUDIDs, opts.MinProfileValidityDays)
		if err != nil {
			return nil, fmt.Errorf("failed to collect local code signing assets: %w", err)
		}

		printExistingCodesignAssets(localCodesignAssets, distrType)
		if localCodesignAssets != nil {
			// Did not check if selected certificate is installed yet
			fmt.Println()
			log.Infof("Installing certificate")
			log.Printf("certificate: %s", localCodesignAssets.Certificate.CommonName)
			if err := m.assetWriter.InstallCertificate(localCodesignAssets.Certificate); err != nil {
				return nil, fmt.Errorf("failed to install certificate: %w", err)
			}
		}

		finalAssets := localCodesignAssets
		if missingAppLayout != nil {
			printMissingCodeSignAssets(missingAppLayout)

			// Ensure Profiles
			newCodesignAssets, err := ensureProfiles(m.devPortalClient, distrType, certsByType, *missingAppLayout, devPortalDeviceIDs, devPortalDeviceUDIDs, opts.MinProfileValidityDays)
			if err != nil {
				switch {
				case errors.As(err, &ErrAppClipAppID{}):
					log.Warnf("Can't create Application Identifier for App Clip targets.")
					log.Warnf("Please generate the Application Identifier manually on Apple Developer Portal, after that the Step will continue working.")
				case errors.As(err, &ErrAppClipAppIDWithAppleSigning{}):
					log.Warnf("Can't manage Application Identifier for App Clip target with 'Sign In With Apple' capability.")
					log.Warnf("Please configure Capabilities on Apple Developer Portal for App Clip target manually, after that the Step will continue working.")
				}

				return nil, fmt.Errorf("failed to ensure profiles: %w", err)
			}

			// Install new certificates and profiles
			fmt.Println()
			log.Infof("Installing certificates and profiles")
			if err := m.assetWriter.Write(map[DistributionType]AppCodesignAssets{distrType: *newCodesignAssets}); err != nil {
				return nil, fmt.Errorf("failed to install codesigning files: %w", err)
			}

			// Merge local and recently generated code signing assets
			finalAssets = mergeCodeSignAssets(localCodesignAssets, newCodesignAssets)
		}

		if finalAssets != nil {
			codesignAssetsByDistributionType[distrType] = *finalAssets
		}
	}

	return codesignAssetsByDistributionType, nil
}
