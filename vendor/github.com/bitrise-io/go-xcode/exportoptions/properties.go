package exportoptions

import "fmt"

// CompileBitcodeKey ...
const CompileBitcodeKey = "compileBitcode"

// CompileBitcodeDefault ...
const CompileBitcodeDefault = true

// EmbedOnDemandResourcesAssetPacksInBundleKey ...
const EmbedOnDemandResourcesAssetPacksInBundleKey = "embedOnDemandResourcesAssetPacksInBundle"

// EmbedOnDemandResourcesAssetPacksInBundleDefault ...
const EmbedOnDemandResourcesAssetPacksInBundleDefault = true

// ICloudContainerEnvironmentKey ...
const ICloudContainerEnvironmentKey = "iCloudContainerEnvironment"

// ICloudContainerEnvironment ...
type ICloudContainerEnvironment string

const (
	// ICloudContainerEnvironmentDevelopment ...
	ICloudContainerEnvironmentDevelopment ICloudContainerEnvironment = "Development"
	// ICloudContainerEnvironmentProduction ...
	ICloudContainerEnvironmentProduction ICloudContainerEnvironment = "Production"
)

// DistributionBundleIdentifier ...
const DistributionBundleIdentifier = "distributionBundleIdentifier"

// ManifestKey ...
const ManifestKey = "manifest"

// ManifestAppURLKey ...
const ManifestAppURLKey = "appURL"

// ManifestDisplayImageURLKey ...
const ManifestDisplayImageURLKey = "displayImageURL"

// ManifestFullSizeImageURLKey ...
const ManifestFullSizeImageURLKey = "fullSizeImageURL"

// ManifestAssetPackManifestURLKey ...
const ManifestAssetPackManifestURLKey = "assetPackManifestURL"

// Manifest ...
type Manifest struct {
	AppURL               string
	DisplayImageURL      string
	FullSizeImageURL     string
	AssetPackManifestURL string
}

// IsEmpty ...
func (manifest Manifest) IsEmpty() bool {
	return (manifest.AppURL == "" && manifest.DisplayImageURL == "" && manifest.FullSizeImageURL == "" && manifest.AssetPackManifestURL == "")
}

// ToHash ...
func (manifest Manifest) ToHash() map[string]string {
	hash := map[string]string{}
	if manifest.AppURL != "" {
		hash[ManifestAppURLKey] = manifest.AppURL
	}
	if manifest.DisplayImageURL != "" {
		hash[ManifestDisplayImageURLKey] = manifest.DisplayImageURL
	}
	if manifest.FullSizeImageURL != "" {
		hash[ManifestFullSizeImageURLKey] = manifest.FullSizeImageURL
	}
	if manifest.AssetPackManifestURL != "" {
		hash[ManifestAssetPackManifestURLKey] = manifest.AssetPackManifestURL
	}
	return hash
}

// MethodKey ...
const MethodKey = "method"

// Method ...
type Method string

const (
	// MethodAppStore ...
	MethodAppStore Method = "app-store"
	// MethodAdHoc ...
	MethodAdHoc Method = "ad-hoc"
	// MethodPackage ...
	MethodPackage Method = "package"
	// MethodEnterprise ...
	MethodEnterprise Method = "enterprise"
	// MethodDevelopment ...
	MethodDevelopment Method = "development"
	// MethodDeveloperID ...
	MethodDeveloperID Method = "developer-id"
	// MethodDefault ...
	MethodDefault Method = MethodDevelopment
)

// ParseMethod ...
func ParseMethod(method string) (Method, error) {
	switch method {
	case "app-store":
		return MethodAppStore, nil
	case "ad-hoc":
		return MethodAdHoc, nil
	case "package":
		return MethodPackage, nil
	case "enterprise":
		return MethodEnterprise, nil
	case "development":
		return MethodDevelopment, nil
	case "developer-id":
		return MethodDeveloperID, nil
	default:
		return Method(""), fmt.Errorf("unkown method (%s)", method)
	}
}

// OnDemandResourcesAssetPacksBaseURLKey ....
const OnDemandResourcesAssetPacksBaseURLKey = "onDemandResourcesAssetPacksBaseURL"

// TeamIDKey ...
const TeamIDKey = "teamID"

// ThinningKey ...
const ThinningKey = "thinning"

const (
	// ThinningNone ...
	ThinningNone = "none"
	// ThinningThinForAllVariants ...
	ThinningThinForAllVariants = "thin-for-all-variants"
	// ThinningDefault ...
	ThinningDefault = ThinningNone
)

// UploadBitcodeKey ....
const UploadBitcodeKey = "uploadBitcode"

// UploadBitcodeDefault ...
const UploadBitcodeDefault = true

// UploadSymbolsKey ...
const UploadSymbolsKey = "uploadSymbols"

// UploadSymbolsDefault ...
const UploadSymbolsDefault = true

const (
	manageAppVersionKey     = "manageAppVersionAndBuildNumber"
	manageAppVersionDefault = true
)

// ProvisioningProfilesKey ...
const ProvisioningProfilesKey = "provisioningProfiles"

// SigningCertificateKey ...
const SigningCertificateKey = "signingCertificate"

// InstallerSigningCertificateKey ...
const InstallerSigningCertificateKey = "installerSigningCertificate"

// SigningStyleKey ...
const SigningStyleKey = "signingStyle"

// SigningStyle ...
type SigningStyle string

// SigningStyle ...
const (
	SigningStyleManual    SigningStyle = "manual"
	SigningStyleAutomatic SigningStyle = "automatic"
)

const DestinationKey = "destination"

const TestFlightInternalTestingOnlyKey = "testFlightInternalTestingOnly"

type Destination string

// Destination ...
const (
	DestinationExport  Destination = "export"
	DestinationDefault Destination = DestinationExport
)
