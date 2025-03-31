package step

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/bitrise-io/go-steputils/v2/stepconf"
	v1command "github.com/bitrise-io/go-utils/command"
	v1fileutil "github.com/bitrise-io/go-utils/fileutil"
	logv1 "github.com/bitrise-io/go-utils/log"
	v1pathutil "github.com/bitrise-io/go-utils/pathutil"
	"github.com/bitrise-io/go-utils/retry"
	"github.com/bitrise-io/go-utils/sliceutil"
	"github.com/bitrise-io/go-utils/v2/command"
	"github.com/bitrise-io/go-utils/v2/fileutil"
	"github.com/bitrise-io/go-utils/v2/log"
	"github.com/bitrise-io/go-utils/v2/pathutil"
	"github.com/bitrise-io/go-xcode/exportoptions"
	"github.com/bitrise-io/go-xcode/profileutil"
	"github.com/bitrise-io/go-xcode/v2/autocodesign/certdownloader"
	"github.com/bitrise-io/go-xcode/v2/autocodesign/codesignasset"
	"github.com/bitrise-io/go-xcode/v2/autocodesign/devportalclient"
	"github.com/bitrise-io/go-xcode/v2/autocodesign/localcodesignasset"
	"github.com/bitrise-io/go-xcode/v2/autocodesign/profiledownloader"
	"github.com/bitrise-io/go-xcode/v2/autocodesign/projectmanager"
	"github.com/bitrise-io/go-xcode/v2/codesign"
	"github.com/bitrise-io/go-xcode/v2/devportalservice"
	"github.com/bitrise-io/go-xcode/v2/exportoptionsgenerator"
	"github.com/bitrise-io/go-xcode/v2/xcconfig"
	cache "github.com/bitrise-io/go-xcode/v2/xcodecache"
	"github.com/bitrise-io/go-xcode/v2/xcodecommand"
	"github.com/bitrise-io/go-xcode/xcarchive"
	"github.com/bitrise-io/go-xcode/xcodebuild"
	"github.com/kballard/go-shellquote"
	"howett.net/plist"
)

const (
	minSupportedXcodeMajorVersion = 9

	// Deployed Outputs (moved to the OutputDir)
	bitriseXCArchiveZipPthEnvKey = "BITRISE_XCARCHIVE_ZIP_PATH"
	bitriseDSYMPthEnvKey         = "BITRISE_DSYM_PATH"
	bitriseIPAPthEnvKey          = "BITRISE_IPA_PATH"

	// Deployed logs
	xcodebuildArchiveLogPathEnvKey       = "BITRISE_XCODEBUILD_ARCHIVE_LOG_PATH"
	xcodebuildExportArchiveLogPathEnvKey = "BITRISE_XCODEBUILD_EXPORT_ARCHIVE_LOG_PATH"
	bitriseIDEDistributionLogsPthEnvKey  = "BITRISE_IDEDISTRIBUTION_LOGS_PATH"
	xcodebuildArchiveLogFilename         = "xcodebuild-archive.log"
	xcodebuildExportArchiveLogFilename   = "xcodebuild-export-archive.log"

	// Env Outputs
	bitriseAppDirPthEnvKey    = "BITRISE_APP_DIR_PATH"
	bitriseDSYMDirPthEnvKey   = "BITRISE_DSYM_DIR_PATH"
	bitriseXCArchivePthEnvKey = "BITRISE_XCARCHIVE_PATH"

	// Code Signing Authentication Source
	codeSignSourceOff     = "off"
	codeSignSourceAPIKey  = "api-key"
	codeSignSourceAppleID = "apple-id"

	// Output tools
	XcbeautifyTool = "xcbeautify"
	XcodebuildTool = "xcodebuild"
	XcprettyTool   = "xcpretty"
)

// Inputs ...
type Inputs struct {
	ProjectPath  string `env:"project_path,file"`
	Scheme       string `env:"scheme,required"`
	ExportMethod string `env:"distribution_method,opt[app-store,ad-hoc,enterprise,development]"`

	// xcodebuild configuration
	Configuration      string `env:"configuration"`
	XcconfigContent    string `env:"xcconfig_content"`
	PerformCleanAction bool   `env:"perform_clean_action,opt[yes,no]"`
	XcodebuildOptions  string `env:"xcodebuild_options"`

	// xcodebuild log formatting
	LogFormatter string `env:"log_formatter,opt[xcbeautify,xcodebuild,xcpretty]"`

	// Automatic code signing
	CodeSigningAuthSource           string          `env:"automatic_code_signing,opt[off,api-key,apple-id]"`
	RegisterTestDevices             bool            `env:"register_test_devices,opt[yes,no]"`
	TestDeviceListPath              string          `env:"test_device_list_path"`
	MinDaysProfileValid             int             `env:"min_profile_validity,required"`
	CertificateURLList              string          `env:"certificate_url_list"`
	CertificatePassphraseList       stepconf.Secret `env:"passphrase_list"`
	KeychainPath                    string          `env:"keychain_path"`
	KeychainPassword                stepconf.Secret `env:"keychain_password"`
	FallbackProvisioningProfileURLs string          `env:"fallback_provisioning_profile_url_list"`

	// IPA export configuration
	ExportDevelopmentTeam         string `env:"export_development_team"`
	CompileBitcode                bool   `env:"compile_bitcode,opt[yes,no]"`
	UploadBitcode                 bool   `env:"upload_bitcode,opt[yes,no]"`
	ICloudContainerEnvironment    string `env:"icloud_container_environment"`
	TestFlightInternalTestingOnly bool   `env:"testflight_internal_testing_only,opt[yes,no]"`
	ExportOptionsPlistContent     string `env:"export_options_plist_content"`

	// Step Output Export configuration
	OutputDir      string `env:"output_dir,required"`
	ExportAllDsyms bool   `env:"export_all_dsyms,opt[yes,no]"`
	ArtifactName   string `env:"artifact_name"`

	// Caching
	CacheLevel string `env:"cache_level,opt[none,swift_packages]"`

	// App Store Connect connection override
	APIKeyPath              stepconf.Secret `env:"api_key_path"`
	APIKeyID                string          `env:"api_key_id"`
	APIKeyIssuerID          string          `env:"api_key_issuer_id"`
	APIKeyEnterpriseAccount bool            `env:"api_key_enterprise_account,opt[yes,no]"`

	// Debugging
	VerboseLog bool `env:"verbose_log,opt[yes,no]"`

	// Hidden inputs
	BuildURL      string          `env:"BITRISE_BUILD_URL"`
	BuildAPIToken stepconf.Secret `env:"BITRISE_BUILD_API_TOKEN"`
}

// Config ...
type Config struct {
	Inputs
	XcodeMajorVersion           int
	XcodebuildAdditionalOptions []string
	CodesignManager             *codesign.Manager // nil if automatic code signing is "off"
}

type XcodebuildArchiveConfigParser struct {
	stepInputParser      stepconf.InputParser
	xcodeVersionProvider XcodeVersionProvider
	fileManager          fileutil.FileManager
	cmdFactory           command.Factory
	logger               log.Logger
}

// XcodebuildArchiver ...
type XcodebuildArchiver struct {
	xcodeCommandRunner xcodecommand.Runner
	logFormatter       string
	pathProvider       pathutil.PathProvider
	pathChecker        pathutil.PathChecker
	pathModifier       pathutil.PathModifier
	fileManager        fileutil.FileManager
	logger             log.Logger
	cmdFactory         command.Factory
}

func NewXcodeArchiveConfigParser(stepInputParser stepconf.InputParser, xcodeVersionProvider XcodeVersionProvider, fileManager fileutil.FileManager, cmdFactory command.Factory, logger log.Logger) XcodebuildArchiveConfigParser {
	return XcodebuildArchiveConfigParser{
		stepInputParser:      stepInputParser,
		xcodeVersionProvider: xcodeVersionProvider,
		fileManager:          fileManager,
		cmdFactory:           cmdFactory,
		logger:               logger,
	}
}

// NewXcodebuildArchiver ...
func NewXcodebuildArchiver(xcodecommandRunner xcodecommand.Runner, logFormatter string, pathProvider pathutil.PathProvider, pathChecker pathutil.PathChecker, pathModifier pathutil.PathModifier, fileManager fileutil.FileManager, cmdFactory command.Factory, logger log.Logger) XcodebuildArchiver {
	return XcodebuildArchiver{
		xcodeCommandRunner: xcodecommandRunner,
		logFormatter:       logFormatter,
		pathProvider:       pathProvider,
		pathChecker:        pathChecker,
		pathModifier:       pathModifier,
		fileManager:        fileManager,
		logger:             logger,
		cmdFactory:         cmdFactory,
	}
}

// ProcessInputs ...
func (s XcodebuildArchiveConfigParser) ProcessInputs() (Config, error) {
	var inputs Inputs
	if err := s.stepInputParser.Parse(&inputs); err != nil {
		return Config{}, fmt.Errorf("issue with input: %s", err)
	}

	stepconf.Print(inputs)
	s.logger.Println()

	config := Config{Inputs: inputs}

	s.logger.EnableDebugLog(config.VerboseLog)
	if config.VerboseLog {
		logv1.SetEnableDebugLog(true)
	}

	var err error
	config.XcodebuildAdditionalOptions, err = shellquote.Split(inputs.XcodebuildOptions)
	if err != nil {
		return Config{}, fmt.Errorf("provided XcodebuildOptions (%s) are not valid CLI parameters: %s", inputs.XcodebuildOptions, err)
	}

	if strings.TrimSpace(config.XcconfigContent) == "" {
		config.XcconfigContent = ""
	}
	if sliceutil.IsStringInSlice("-xcconfig", config.XcodebuildAdditionalOptions) &&
		config.XcconfigContent != "" {
		return Config{}, fmt.Errorf("`-xcconfig` option found in XcodebuildOptions (`xcodebuild_options`), please clear Build settings (xcconfig) (`xcconfig_content`) input as only one can be set")
	}

	if config.ExportOptionsPlistContent != "" {
		var options map[string]interface{}
		if _, err := plist.Unmarshal([]byte(config.ExportOptionsPlistContent), &options); err != nil {
			return Config{}, fmt.Errorf("issue with input ExportOptionsPlistContent: %s", err)
		}
	}

	if filepath.Ext(config.ProjectPath) != ".xcodeproj" && filepath.Ext(config.ProjectPath) != ".xcworkspace" {
		return Config{}, fmt.Errorf("issue with input ProjectPath: should be and .xcodeproj or .xcworkspace path")
	}

	s.logger.Infof("Xcode version:")

	// Detect Xcode major version
	xcodebuildVersion, err := s.xcodeVersionProvider.GetXcodeVersion()
	if err != nil {
		return Config{}, fmt.Errorf("failed to determine xcode version, error: %s", err)
	}
	s.logger.Printf("%s (%s)", xcodebuildVersion.Version, xcodebuildVersion.BuildVersion)

	xcodeMajorVersion := xcodebuildVersion.MajorVersion
	if xcodeMajorVersion < minSupportedXcodeMajorVersion {
		return Config{}, fmt.Errorf("invalid xcode major version (%d), should not be less then min supported: %d", xcodeMajorVersion, minSupportedXcodeMajorVersion)
	}
	config.XcodeMajorVersion = int(xcodeMajorVersion)

	// Validation ExportOptionsPlistContent
	exportOptionsPlistContent := strings.TrimSpace(config.ExportOptionsPlistContent)
	if exportOptionsPlistContent != config.ExportOptionsPlistContent {
		s.logger.Println()
		s.logger.Warnf("ExportOptionsPlistContent is stripped to remove spaces and new lines:")
		s.logger.Printf(exportOptionsPlistContent)
	}

	if exportOptionsPlistContent != "" {
		s.logger.Println()
		s.logger.Warnf("Ignoring the following options because ExportOptionsPlistContent provided:")
		s.logger.Printf("- DistributionMethod: %s", config.ExportMethod)
		s.logger.Printf("- UploadBitcode: %s", config.UploadBitcode)
		s.logger.Printf("- CompileBitcode: %s", config.CompileBitcode)
		s.logger.Printf("- ExportDevelopmentTeam: %s", config.ExportDevelopmentTeam)
		s.logger.Printf("- ICloudContainerEnvironment: %s", config.ICloudContainerEnvironment)
		s.logger.Println()
	}
	config.ExportOptionsPlistContent = exportOptionsPlistContent

	if config.ExportMethod != "app-store" && config.TestFlightInternalTestingOnly {
		s.logger.Println()
		s.logger.Warnf("TestFlightInternalTestingOnly is valid only for Distribution Method app-store.")
		s.logger.Println()
	}

	absProjectPath, err := filepath.Abs(config.ProjectPath)
	if err != nil {
		return Config{}, fmt.Errorf("failed to get absolute project path, error: %s", err)
	}
	config.ProjectPath = absProjectPath

	// abs out dir pth
	absOutputDir, err := v1pathutil.AbsPath(config.OutputDir)
	if err != nil {
		return Config{}, fmt.Errorf("failed to expand OutputDir (%s), error: %s", config.OutputDir, err)
	}
	config.OutputDir = absOutputDir

	if exist, err := v1pathutil.IsPathExists(config.OutputDir); err != nil {
		return Config{}, fmt.Errorf("failed to check if OutputDir exist, error: %s", err)
	} else if !exist {
		if err := os.MkdirAll(config.OutputDir, 0777); err != nil {
			return Config{}, fmt.Errorf("failed to create OutputDir (%s), error: %s", config.OutputDir, err)
		}
	}

	if config.CodeSigningAuthSource != codeSignSourceOff {
		codesignManager, err := s.createCodesignManager(config)
		if err != nil {
			return Config{}, fmt.Errorf("failed to prepare automatic code signing: %w", err)
		}
		config.CodesignManager = &codesignManager
	}

	return config, nil
}

// EnsureDependencies ...
func (s *XcodebuildArchiver) EnsureDependencies() {
	logFormatterVersion, err := s.xcodeCommandRunner.CheckInstall()
	if err != nil {
		s.logger.Println()
		s.logger.Errorf("Selected log formatter is unavailable: %s", err)
		s.logger.Infof("Switching back to xcodebuild log formatter.")

		s.logFormatter = XcodebuildTool
		s.xcodeCommandRunner = xcodecommand.NewRawCommandRunner(s.logger, s.cmdFactory)
		return
	}

	if logFormatterVersion != nil { // raw xcodebuild runner returns nil
		s.logger.Printf("- log formatter version: %s", logFormatterVersion.String())
	}
}

// RunOpts ...
type RunOpts struct {
	// Shared
	ProjectPath       string
	Scheme            string
	Configuration     string
	XcodeMajorVersion int
	ArtifactName      string

	// Code signing, nil if automatic code signing is "off"
	CodesignManager *codesign.Manager

	// Archive
	PerformCleanAction          bool
	XcconfigContent             string
	XcodebuildAdditionalOptions []string
	CacheLevel                  string

	// IPA Export
	CustomExportOptionsPlistContent string
	ExportMethod                    string
	TestFlightInternalTestingOnly   bool
	ICloudContainerEnvironment      string
	ExportDevelopmentTeam           string
	UploadBitcode                   bool
	CompileBitcode                  bool
}

// RunResult ...
type RunResult struct {
	Archive      *xcarchive.IosArchive
	ArtifactName string

	ExportOptionsPath string
	IPAExportDir      string

	XcodebuildArchiveLog       string
	XcodebuildExportArchiveLog string
	IDEDistrubutionLogsDir     string
}

// Run ...
func (s XcodebuildArchiver) Run(opts RunOpts) (RunResult, error) {
	var (
		out         = RunResult{}
		authOptions *xcodebuild.AuthenticationParams
	)

	s.logger.Println()

	if opts.XcodeMajorVersion >= 11 {
		s.logger.Infof("Running resolve Swift package dependencies")
		// Resolve Swift package dependencies, so running -showBuildSettings later is faster later
		// Specifying a scheme is required for workspaces
		resolveDepsCmd := xcodebuild.NewResolvePackagesCommandModel(opts.ProjectPath, opts.Scheme, opts.Configuration)
		resolveDepsCmd.SetCustomOptions(opts.XcodebuildAdditionalOptions)
		if err := resolveDepsCmd.Run(); err != nil {
			s.logger.Warnf("%s", err)
		}
	}

	if opts.ArtifactName == "" {
		s.logger.Infof("Looking for artifact name as field is empty")

		cmdModel := xcodebuild.NewShowBuildSettingsCommand(opts.ProjectPath)
		cmdModel.SetScheme(opts.Scheme)
		cmdModel.SetConfiguration(opts.Configuration)
		settings, err := cmdModel.RunAndReturnSettings()
		if err != nil {
			return out, fmt.Errorf("failed to read build settings: %w", err)
		}
		productName, err := settings.String("PRODUCT_NAME")
		if err != nil || productName == "" {
			s.logger.Warnf("Product name not found in build settings, using scheme (%s) as artifact name", opts.Scheme)
			productName = opts.Scheme
		}

		opts.ArtifactName = productName
	}
	out.ArtifactName = opts.ArtifactName

	if opts.CodesignManager != nil {
		s.logger.Infof("Preparing code signing assets (certificates, profiles) before Archive action")

		xcodebuildAuthParams, err := opts.CodesignManager.PrepareCodesigning()
		if err != nil {
			return RunResult{}, fmt.Errorf("failed to manage code signing: %s", err)
		}

		if xcodebuildAuthParams != nil {
			privateKey, err := xcodebuildAuthParams.WritePrivateKeyToFile()
			if err != nil {
				return RunResult{}, err
			}

			defer func() {
				if err := os.Remove(privateKey); err != nil {
					s.logger.Warnf("failed to remove private key file: %s", err)
				}
			}()

			authOptions = &xcodebuild.AuthenticationParams{
				KeyID:     xcodebuildAuthParams.KeyID,
				IsssuerID: xcodebuildAuthParams.IssuerID,
				KeyPath:   privateKey,
			}
		}
	} else {
		s.logger.Infof("Automatic code signing is disabled, skipped downloading code sign assets")
	}
	s.logger.Println()

	archiveOpts := xcodeArchiveOpts{
		ProjectPath:       opts.ProjectPath,
		Scheme:            opts.Scheme,
		Configuration:     opts.Configuration,
		XcodeMajorVersion: opts.XcodeMajorVersion,
		ArtifactName:      opts.ArtifactName,
		XcodeAuthOptions:  authOptions,

		PerformCleanAction: opts.PerformCleanAction,
		XcconfigContent:    opts.XcconfigContent,
		AdditionalOptions:  opts.XcodebuildAdditionalOptions,
		CacheLevel:         opts.CacheLevel,
	}
	archiveOut, err := s.xcodeArchive(archiveOpts)
	out.XcodebuildArchiveLog = archiveOut.XcodebuildArchiveLog
	if err != nil {
		return out, err
	}

	out.Archive = archiveOut.Archive

	IPAExportOpts := xcodeIPAExportOpts{
		ProjectPath:       opts.ProjectPath,
		Scheme:            opts.Scheme,
		Configuration:     opts.Configuration,
		XcodeMajorVersion: opts.XcodeMajorVersion,
		XcodeAuthOptions:  authOptions,

		Archive:                         *archiveOut.Archive,
		CustomExportOptionsPlistContent: opts.CustomExportOptionsPlistContent,
		ExportMethod:                    opts.ExportMethod,
		TestFlightInternalTestingOnly:   opts.TestFlightInternalTestingOnly,
		ICloudContainerEnvironment:      opts.ICloudContainerEnvironment,
		ExportDevelopmentTeam:           opts.ExportDevelopmentTeam,
		UploadBitcode:                   opts.UploadBitcode,
		CompileBitcode:                  opts.CompileBitcode,
	}
	exportOut, err := s.xcodeIPAExport(IPAExportOpts)
	out.XcodebuildExportArchiveLog = exportOut.XcodebuildExportArchiveLog
	if err != nil {
		out.IDEDistrubutionLogsDir = exportOut.IDEDistrubutionLogsDir
		return out, err
	}

	out.ExportOptionsPath = exportOut.ExportOptionsPath
	out.IPAExportDir = exportOut.IPAExportDir

	return out, nil
}

// ExportOpts ...
type ExportOpts struct {
	OutputDir      string
	ArtifactName   string
	ExportAllDsyms bool

	Archive *xcarchive.IosArchive

	ExportOptionsPath string
	IPAExportDir      string

	XcodebuildArchiveLog       string
	XcodebuildExportArchiveLog string
	IDEDistrubutionLogsDir     string
}

// ExportOutput ...
func (s XcodebuildArchiver) ExportOutput(opts ExportOpts) error {
	s.logger.Println()
	s.logger.TInfof("Exporting outputs...")

	cleanup := func(pth string) error {
		if exist, err := v1pathutil.IsPathExists(pth); err != nil {
			return fmt.Errorf("failed to check if path (%s) exist, error: %s", pth, err)
		} else if exist {
			if err := os.RemoveAll(pth); err != nil {
				return fmt.Errorf("failed to remove path (%s), error: %s", pth, err)
			}
		}
		return nil
	}

	if opts.Archive != nil {
		archivePath := opts.Archive.Path
		if err := ExportOutputDir(s.cmdFactory, archivePath, archivePath, bitriseXCArchivePthEnvKey, s.logger); err != nil {
			return fmt.Errorf("failed to export %s, error: %s", bitriseXCArchivePthEnvKey, err)
		}
		s.logger.Donef("The xcarchive path is now available in the Environment Variable: %s (value: %s)", bitriseXCArchivePthEnvKey, archivePath)

		archiveZipPath := filepath.Join(opts.OutputDir, opts.ArtifactName+".xcarchive.zip")
		if err := cleanup(archiveZipPath); err != nil {
			return err
		}

		if err := ExportOutputDirAsZip(s.cmdFactory, archivePath, archiveZipPath, bitriseXCArchiveZipPthEnvKey, s.logger); err != nil {
			return fmt.Errorf("failed to export %s, error: %s", bitriseXCArchiveZipPthEnvKey, err)
		}
		s.logger.Donef("The xcarchive zip path is now available in the Environment Variable: %s (value: %s)", bitriseXCArchiveZipPthEnvKey, archiveZipPath)

		appPath := filepath.Join(opts.OutputDir, opts.ArtifactName+".app")
		if err := cleanup(appPath); err != nil {
			return err
		}

		if err := ExportOutputDir(s.cmdFactory, opts.Archive.Application.Path, appPath, bitriseAppDirPthEnvKey, s.logger); err != nil {
			return fmt.Errorf("failed to export %s, error: %s", bitriseAppDirPthEnvKey, err)
		}
		s.logger.Donef("The app directory is now available in the Environment Variable: %s (value: %s)", bitriseAppDirPthEnvKey, appPath)

		s.logger.Printf("Looking for app and framework dSYMs.")

		appDSYMPaths, frameworkDSYMPaths, err := opts.Archive.FindDSYMs()
		if err != nil {
			return fmt.Errorf("failed to export dSYMs, error: %s", err)
		}

		appDSYMPathsCount := len(appDSYMPaths)
		frameworkDSYMPathsCount := len(frameworkDSYMPaths)

		s.logger.Printf("Found %d app dSYMs and %d framework dSYMs.", appDSYMPathsCount, frameworkDSYMPathsCount)

		if appDSYMPathsCount > 0 || frameworkDSYMPathsCount > 0 {
			dsymDir, err := v1pathutil.NormalizedOSTempDirPath("__dsyms__")
			if err != nil {
				return fmt.Errorf("failed to create tmp dir, error: %s", err)
			}

			if appDSYMPathsCount > 0 {
				if err := ExportDSYMs(dsymDir, appDSYMPaths); err != nil {
					return fmt.Errorf("failed to export dSYMs: %v", err)
				}
			} else {
				s.logger.Warnf("No app dSYMs found to export")
			}

			if opts.ExportAllDsyms && frameworkDSYMPathsCount > 0 {
				if err := ExportDSYMs(dsymDir, frameworkDSYMPaths); err != nil {
					return fmt.Errorf("failed to export dSYMs: %v", err)
				}
			}

			if err := ExportOutputDir(s.cmdFactory, dsymDir, dsymDir, bitriseDSYMDirPthEnvKey, s.logger); err != nil {
				return fmt.Errorf("failed to export %s, error: %s", bitriseDSYMDirPthEnvKey, err)
			}
			s.logger.Donef("The dSYM dir path is now available in the Environment Variable: %s (value: %s)", bitriseDSYMDirPthEnvKey, dsymDir)

			dsymZipPath := filepath.Join(opts.OutputDir, opts.ArtifactName+".dSYM.zip")
			if err := cleanup(dsymZipPath); err != nil {
				return err
			}

			if err := ExportOutputDirAsZip(s.cmdFactory, dsymDir, dsymZipPath, bitriseDSYMPthEnvKey, s.logger); err != nil {
				return fmt.Errorf("failed to export %s, error: %s", bitriseDSYMPthEnvKey, err)
			}
			s.logger.Donef("The dSYM zip path is now available in the Environment Variable: %s (value: %s)", bitriseDSYMPthEnvKey, dsymZipPath)
		}
	}

	if opts.ExportOptionsPath != "" {
		exportOptionsPath := filepath.Join(opts.OutputDir, "export_options.plist")
		if err := cleanup(exportOptionsPath); err != nil {
			return err
		}

		if err := v1command.CopyFile(opts.ExportOptionsPath, exportOptionsPath); err != nil {
			return err
		}
	}

	if opts.IPAExportDir != "" {
		fileList := []string{}
		ipaFiles := []string{}
		if walkErr := filepath.Walk(opts.IPAExportDir, func(pth string, info os.FileInfo, err error) error {
			if err != nil {
				return err
			}

			fileList = append(fileList, pth)

			if filepath.Ext(pth) == ".ipa" {
				ipaFiles = append(ipaFiles, pth)
			}

			return nil
		}); walkErr != nil {
			return fmt.Errorf("failed to search for .ipa file, error: %s", walkErr)
		}

		if len(ipaFiles) == 0 {
			s.logger.Printf("File list in the export dir:")
			for _, pth := range fileList {
				s.logger.Printf("- %s", pth)
			}
			return fmt.Errorf("No .ipa file found at export dir: %s", opts.IPAExportDir)
		}

		ipaPath := filepath.Join(opts.OutputDir, opts.ArtifactName+".ipa")
		if err := cleanup(ipaPath); err != nil {
			return err
		}

		if err := ExportOutputFile(s.cmdFactory, ipaFiles[0], ipaPath, bitriseIPAPthEnvKey); err != nil {
			return fmt.Errorf("failed to export %s, error: %s", bitriseIPAPthEnvKey, err)
		}
		s.logger.Donef("The ipa path is now available in the Environment Variable: %s (value: %s)", bitriseIPAPthEnvKey, ipaPath)

		if len(ipaFiles) > 1 {
			s.logger.Warnf("More than 1 .ipa file found, exporting first one: %s", ipaFiles[0])
			s.logger.Warnf("Moving every ipa to the BITRISE_DEPLOY_DIR")

			for i, pth := range ipaFiles {
				if i == 0 {
					continue
				}

				base := filepath.Base(pth)
				deployPth := filepath.Join(opts.OutputDir, base)

				if err := v1command.CopyFile(pth, deployPth); err != nil {
					return fmt.Errorf("failed to copy (%s) -> (%s), error: %s", pth, deployPth, err)
				}
			}
		}
	}

	if opts.IDEDistrubutionLogsDir != "" {
		ideDistributionLogsZipPath := filepath.Join(opts.OutputDir, "xcodebuild.xcdistributionlogs.zip")
		if err := cleanup(ideDistributionLogsZipPath); err != nil {
			return err
		}

		if err := ExportOutputDirAsZip(s.cmdFactory, opts.IDEDistrubutionLogsDir, ideDistributionLogsZipPath, bitriseIDEDistributionLogsPthEnvKey, s.logger); err != nil {
			s.logger.Warnf("Failed to export %s, error: %s", bitriseIDEDistributionLogsPthEnvKey, err)
		} else {
			s.logger.Donef("The xcdistributionlogs zip path is now available in the Environment Variable: %s (value: %s)", bitriseIDEDistributionLogsPthEnvKey, ideDistributionLogsZipPath)
		}
	}

	if opts.XcodebuildArchiveLog != "" {
		xcodebuildArchiveLogPath := filepath.Join(opts.OutputDir, xcodebuildArchiveLogFilename)
		if err := cleanup(xcodebuildArchiveLogPath); err != nil {
			return err
		}

		if err := ExportOutputFileContent(s.cmdFactory, opts.XcodebuildArchiveLog, xcodebuildArchiveLogPath, xcodebuildArchiveLogPathEnvKey); err != nil {
			s.logger.Warnf("Failed to export %s, error: %s", xcodebuildArchiveLogPathEnvKey, err)
		} else {
			s.logger.Donef("The xcodebuild archive log path is now available in the Environment Variable: %s (value: %s)", xcodebuildArchiveLogPathEnvKey, xcodebuildArchiveLogPath)
		}
	}

	if opts.XcodebuildExportArchiveLog != "" {
		xcodebuildExportArchiveLogPath := filepath.Join(opts.OutputDir, xcodebuildExportArchiveLogFilename)
		if err := cleanup(xcodebuildExportArchiveLogPath); err != nil {
			return err
		}

		if err := ExportOutputFileContent(s.cmdFactory, opts.XcodebuildExportArchiveLog, xcodebuildExportArchiveLogPath, xcodebuildExportArchiveLogPathEnvKey); err != nil {
			s.logger.Warnf("Failed to export %s, error: %s", xcodebuildExportArchiveLogPathEnvKey, err)
		} else {
			s.logger.Donef("The xcodebuild -exportArchive log path is now available in the Environment Variable: %s (value: %s)", xcodebuildExportArchiveLogPathEnvKey, xcodebuildExportArchiveLogPath)
		}
	}

	return nil
}

func (s XcodebuildArchiveConfigParser) createCodesignManager(config Config) (codesign.Manager, error) {
	var authType codesign.AuthType
	switch config.CodeSigningAuthSource {
	case codeSignSourceAppleID:
		authType = codesign.AppleIDAuth
	case codeSignSourceAPIKey:
		authType = codesign.APIKeyAuth
	case codeSignSourceOff:
		return codesign.Manager{}, fmt.Errorf("automatic code signing is disabled")
	}

	codesignInputs := codesign.Input{
		AuthType:                     authType,
		DistributionMethod:           config.ExportMethod,
		CertificateURLList:           config.CertificateURLList,
		CertificatePassphraseList:    config.CertificatePassphraseList,
		KeychainPath:                 config.KeychainPath,
		KeychainPassword:             config.KeychainPassword,
		FallbackProvisioningProfiles: config.FallbackProvisioningProfileURLs,
	}

	codesignConfig, err := codesign.ParseConfig(codesignInputs, s.cmdFactory)
	if err != nil {
		return codesign.Manager{}, err
	}

	devPortalClientFactory := devportalclient.NewFactory(s.logger, s.fileManager)

	var serviceConnection *devportalservice.AppleDeveloperConnection = nil
	if config.BuildURL != "" && config.BuildAPIToken != "" {
		if serviceConnection, err = devPortalClientFactory.CreateBitriseConnection(config.BuildURL, string(config.BuildAPIToken)); err != nil {
			return codesign.Manager{}, err
		}
	}

	connectionInputs := codesign.ConnectionOverrideInputs{
		APIKeyPath:              config.Inputs.APIKeyPath,
		APIKeyID:                config.Inputs.APIKeyID,
		APIKeyIssuerID:          config.Inputs.APIKeyIssuerID,
		APIKeyEnterpriseAccount: config.Inputs.APIKeyEnterpriseAccount,
	}

	appleAuthCredentials, err := codesign.SelectConnectionCredentials(authType, serviceConnection, connectionInputs, s.logger)
	if err != nil {
		return codesign.Manager{}, err
	}

	opts := codesign.Opts{
		AuthType:                   authType,
		ShouldConsiderXcodeSigning: true,
		TeamID:                     config.ExportDevelopmentTeam,
		ExportMethod:               codesignConfig.DistributionMethod,
		XcodeMajorVersion:          config.XcodeMajorVersion,
		RegisterTestDevices:        config.RegisterTestDevices,
		SignUITests:                false,
		MinDaysProfileValidity:     config.MinDaysProfileValid,
		IsVerboseLog:               config.VerboseLog,
	}

	project, err := projectmanager.NewProject(projectmanager.InitParams{
		ProjectOrWorkspacePath: config.ProjectPath,
		SchemeName:             config.Scheme,
		ConfigurationName:      config.Configuration,
	})
	if err != nil {
		return codesign.Manager{}, err
	}

	client := retry.NewHTTPClient().StandardClient()

	var testDevices []devportalservice.TestDevice
	if config.TestDeviceListPath != "" {
		testDevices, err = devportalservice.ParseTestDevicesFromFile(config.TestDeviceListPath, time.Now())
		if err != nil {
			return codesign.Manager{}, fmt.Errorf("failed to process device list (%s): %s", config.TestDeviceListPath, err)
		}
	} else if serviceConnection != nil {
		testDevices = serviceConnection.TestDevices
	}

	return codesign.NewManagerWithProject(
		opts,
		appleAuthCredentials,
		testDevices,
		devPortalClientFactory,
		certdownloader.NewDownloader(codesignConfig.CertificatesAndPassphrases, client),
		profiledownloader.New(codesignConfig.FallbackProvisioningProfiles, client),
		codesignasset.NewWriter(codesignConfig.Keychain),
		localcodesignasset.NewManager(localcodesignasset.NewProvisioningProfileProvider(), localcodesignasset.NewProvisioningProfileConverter()),
		project,
		s.logger,
	), nil
}

type xcodeArchiveOpts struct {
	ProjectPath       string
	Scheme            string
	Configuration     string
	XcodeMajorVersion int
	ArtifactName      string
	XcodeAuthOptions  *xcodebuild.AuthenticationParams

	PerformCleanAction bool
	XcconfigContent    string
	AdditionalOptions  []string

	CacheLevel string
}

type xcodeArchiveResult struct {
	Archive              *xcarchive.IosArchive
	XcodebuildArchiveLog string
}

func (s XcodebuildArchiver) xcodeArchive(opts xcodeArchiveOpts) (xcodeArchiveResult, error) {
	out := xcodeArchiveResult{}

	// Open Xcode project
	s.logger.TInfof("Opening xcode project at path: %s for scheme: %s", opts.ProjectPath, opts.Scheme)

	xcodeProj, scheme, configuration, err := OpenArchivableProject(opts.ProjectPath, opts.Scheme, opts.Configuration)
	if err != nil {
		return out, fmt.Errorf("failed to open project: %s: %s", opts.ProjectPath, err)
	}

	s.logger.TInfof("Reading xcode project")

	// When Package.resolved is embedded in the workspace, passing additional options can cause failure ("-onlyUsePackageVersionsFromResolvedFile" "-skipMacroValidation" "-skipPackagePluginValidation")
	//  a resolved file is required when automatic dependency resolution is disabled and should be placed at /git/sample.xcodeproj/project.xcworkspace/xcshareddata/swiftpm/Package.resolved.
	// An alternative to passing "-onlyUsePackageVersionsFromResolvedFile" or "-disableAutomaticPackageResolution" is to set the following defaults in a Script Step:
	// defaults write com.apple.dt.Xcode IDEPackageOnlyUseVersionsFromResolvedFile
	// defaults write com.apple.dt.Xcode IDEDisableAutomaticPackageResolution
	platform, err := BuildableTargetPlatform(xcodeProj, scheme, configuration, []string{}, XcodeBuild{}, s.logger)
	if err != nil {
		return out, fmt.Errorf("failed to read project platform: %s: %s", opts.ProjectPath, err)
	}

	s.logger.TInfof("Reading main target")

	mainTarget, err := exportoptionsgenerator.ArchivableApplicationTarget(xcodeProj, scheme)
	if err != nil {
		return out, fmt.Errorf("failed to read main application target: %s", err)
	}
	if mainTarget.ProductType == exportoptionsgenerator.AppClipProductType {
		return out, fmt.Errorf(`Selected scheme: '%s' targets an App Clip target (%s),
'Xcode Archive & Export for iOS' step is intended to archive the project using a scheme targeting an Application target.
Please select a scheme targeting an Application target to archive and export the main Application
and use 'Export iOS and tvOS Xcode archive' step to export an App Clip.`, opts.Scheme, mainTarget.Name)
	}

	// Create the Archive with Xcode Command Line tools
	s.logger.Println()
	s.logger.TInfof("Creating the Archive ...")

	var actions []string
	if opts.PerformCleanAction {
		actions = []string{"clean", "archive"}
	} else {
		actions = []string{"archive"}
	}

	archiveCmd := xcodebuild.NewCommandBuilder(opts.ProjectPath, actions...)
	archiveCmd.SetScheme(opts.Scheme)
	archiveCmd.SetConfiguration(opts.Configuration)

	if opts.XcconfigContent != "" {
		xcconfigWriter := xcconfig.NewWriter(s.pathProvider, s.fileManager, s.pathChecker, s.pathModifier)
		xcconfigPath, err := xcconfigWriter.Write(opts.XcconfigContent)
		if err != nil {
			return out, fmt.Errorf("failed to write xcconfig file contents: %w", err)
		}
		archiveCmd.SetXCConfigPath(xcconfigPath)
	}

	tmpDir, err := v1pathutil.NormalizedOSTempDirPath("xcodeArchive")
	if err != nil {
		return out, fmt.Errorf("failed to create temp dir, error: %s", err)
	}
	archivePth := filepath.Join(tmpDir, opts.ArtifactName+".xcarchive")

	archiveCmd.SetArchivePath(archivePth)
	if opts.XcodeAuthOptions != nil {
		archiveCmd.SetAuthentication(*opts.XcodeAuthOptions)
	}

	additionalOptions := generateAdditionalOptions(string(platform), opts.AdditionalOptions)
	archiveCmd.SetCustomOptions(additionalOptions)

	var swiftPackagesPath string
	if opts.XcodeMajorVersion >= 11 {
		var err error
		if swiftPackagesPath, err = cache.NewSwiftPackageCache().SwiftPackagesPath(opts.ProjectPath); err != nil {
			return out, fmt.Errorf("failed to get Swift Packages path, error: %s", err)
		}
	}

	xcodebuildLog, err := runArchiveCommandWithRetry(s.xcodeCommandRunner, s.logFormatter, archiveCmd, swiftPackagesPath, s.logger)
	out.XcodebuildArchiveLog = xcodebuildLog
	if err != nil {
		return out, fmt.Errorf("failed to archive the project: %w", err)
	}

	// Ensure xcarchive exists
	if exist, err := v1pathutil.IsPathExists(archivePth); err != nil {
		return out, fmt.Errorf("failed to check if archive exist, error: %s", err)
	} else if !exist {
		return out, fmt.Errorf("no archive generated at: %s", archivePth)
	}

	archive, err := xcarchive.NewIosArchive(archivePth)
	if err != nil {
		return out, fmt.Errorf("failed to parse archive, error: %s", err)
	}
	out.Archive = &archive

	mainApplication := archive.Application

	s.logger.Println()
	s.logger.Infof("Archive info:")
	s.logger.Printf("team: %s (%s)", mainApplication.ProvisioningProfile.TeamName, mainApplication.ProvisioningProfile.TeamID)
	s.logger.Printf("profile: %s (%s)", mainApplication.ProvisioningProfile.Name, mainApplication.ProvisioningProfile.UUID)
	s.logger.Printf("export: %s", mainApplication.ProvisioningProfile.ExportType)
	s.logger.Printf("xcode managed profile: %v", profileutil.IsXcodeManaged(mainApplication.ProvisioningProfile.Name))

	// Cache swift PM
	if opts.XcodeMajorVersion >= 11 && opts.CacheLevel == "swift_packages" {
		if err := cache.NewSwiftPackageCache().CollectSwiftPackages(opts.ProjectPath); err != nil {
			s.logger.Warnf("Failed to mark swift packages for caching, error: %s", err)
		}
	}

	return out, nil
}

type xcodeIPAExportOpts struct {
	ProjectPath       string
	Scheme            string
	Configuration     string
	XcodeMajorVersion int
	XcodeAuthOptions  *xcodebuild.AuthenticationParams

	Archive                         xcarchive.IosArchive
	CustomExportOptionsPlistContent string
	ExportMethod                    string
	TestFlightInternalTestingOnly   bool
	ICloudContainerEnvironment      string
	ExportDevelopmentTeam           string
	UploadBitcode                   bool
	CompileBitcode                  bool
}

type xcodeIPAExportResult struct {
	ExportOptionsPath          string
	IPAExportDir               string
	XcodebuildExportArchiveLog string
	IDEDistrubutionLogsDir     string
}

func (s XcodebuildArchiver) xcodeIPAExport(opts xcodeIPAExportOpts) (xcodeIPAExportResult, error) {
	out := xcodeIPAExportResult{}

	// Exporting the ipa with Xcode Command Line tools

	/*
		You'll get an "Error Domain=IDEDistributionErrorDomain Code=14 "No applicable devices found."" error
		if $GEM_HOME is set and the project's directory includes a Gemfile - to fix this
		we'll unset GEM_HOME as that's not required for xcodebuild anyway.
		This probably fixes the RVM issue too, but that still should be tested.
		See also:
		- http://stackoverflow.com/questions/33041109/xcodebuild-no-applicable-devices-found-when-exporting-archive
		- https://gist.github.com/claybridges/cea5d4afd24eda268164
	*/
	envsToUnset := []string{"GEM_HOME", "GEM_PATH", "RUBYLIB", "RUBYOPT", "BUNDLE_BIN_PATH", "_ORIGINAL_GEM_PATH", "BUNDLE_GEMFILE"}
	for _, key := range envsToUnset {
		if err := os.Unsetenv(key); err != nil {
			return out, fmt.Errorf("failed to unset (%s), error: %s", key, err)
		}
	}

	s.logger.Println()
	s.logger.Infof("Collecting export options...")

	tmpDir, err := v1pathutil.NormalizedOSTempDirPath("xcodeIPAExport")
	if err != nil {
		return out, fmt.Errorf("failed to create temp dir, error: %s", err)
	}

	exportOptionsPath := filepath.Join(tmpDir, "export_options.plist")

	if opts.CustomExportOptionsPlistContent != "" {
		s.logger.Printf("Custom export options content provided, using it:")
		s.logger.Printf(opts.CustomExportOptionsPlistContent)

		if err := v1fileutil.WriteStringToFile(exportOptionsPath, opts.CustomExportOptionsPlistContent); err != nil {
			return out, fmt.Errorf("failed to write export options to file, error: %s", err)
		}
	} else {
		s.logger.Printf("No custom export options content provided, generating export options...")

		archiveExportMethod := opts.Archive.Application.ProvisioningProfile.ExportType

		exportMethod, err := determineExportMethod(opts.ExportMethod, archiveExportMethod, s.logger)
		if err != nil {
			return out, err
		}

		s.logger.TPrintf("Opening Xcode project at path: %s.", opts.ProjectPath)

		xcodeProj, scheme, configuration, err := OpenArchivableProject(opts.ProjectPath, opts.Scheme, opts.Configuration)
		if err != nil {
			return out, fmt.Errorf("failed to open project: %s: %s", opts.ProjectPath, err)
		}

		archiveCodeSignIsXcodeManaged := opts.Archive.IsXcodeManaged()
		signingStyle := exportoptions.SigningStyleManual
		if opts.XcodeAuthOptions != nil {
			signingStyle = exportoptions.SigningStyleAutomatic
		}

		generator := exportoptionsgenerator.New(xcodeProj, scheme, configuration, s.logger)
		exportOptions, err := generator.GenerateApplicationExportOptions(exportMethod, opts.ICloudContainerEnvironment, opts.ExportDevelopmentTeam,
			opts.UploadBitcode, opts.CompileBitcode, archiveCodeSignIsXcodeManaged, signingStyle, int64(opts.XcodeMajorVersion), opts.TestFlightInternalTestingOnly)
		if err != nil {
			return out, err
		}

		s.logger.Println()
		s.logger.Printf("generated export options content:")
		s.logger.Println()
		exportOptionsContent, err := exportOptions.String()
		if err != nil {
			return out, err
		}
		s.logger.Printf("%s", exportOptionsContent)

		if err := exportOptions.WriteToFile(exportOptionsPath); err != nil {
			return out, err
		}
	}

	ipaExportDir := filepath.Join(tmpDir, "exported")

	exportCmd := xcodebuild.NewExportCommand()
	exportCmd.SetArchivePath(opts.Archive.Path)
	exportCmd.SetExportDir(ipaExportDir)
	exportCmd.SetExportOptionsPlist(exportOptionsPath)
	if opts.XcodeAuthOptions != nil {
		exportCmd.SetAuthentication(*opts.XcodeAuthOptions)
	}

	s.logger.Println()
	s.logger.Infof("Exporting IPA from the archive...")
	exportArchiveLog, exportErr := runIPAExportCommand(s.xcodeCommandRunner, s.logFormatter, exportCmd, s.logger)
	out.XcodebuildExportArchiveLog = exportArchiveLog
	if exportErr != nil {
		s.logger.Println()
		isRawLogOutput := s.logFormatter == XcodebuildTool
		if !isRawLogOutput {
			s.logger.Warnf(`If you can't find the reason of the error in the log, please check the %s
The log file will be stored in $BITRISE_DEPLOY_DIR, and its full path
will be available in the $%s environment variable`, xcodebuildExportArchiveLogFilename, xcodebuildExportArchiveLogPathEnvKey)
		}

		// xcdistributionlogs
		ideDistrubutionLogsDir, err := findIDEDistrubutionLogsPath(exportArchiveLog, s.logger)
		if err != nil {
			s.logger.Warnf("Failed to find xcdistributionlogs, error: %s", err)
		} else {
			out.IDEDistrubutionLogsDir = ideDistrubutionLogsDir

			criticalDistLogFilePth := filepath.Join(ideDistrubutionLogsDir, "IDEDistribution.critical.log")
			s.logger.Warnf("IDEDistribution.critical.log:")
			if criticalDistLog, err := v1fileutil.ReadStringFromFile(criticalDistLogFilePth); err == nil {
				s.logger.Printf(criticalDistLog)
			}

			if !isRawLogOutput {
				s.logger.Warnf(`Also please check the xcdistributionlogs
The logs directory is stored in $BITRISE_DEPLOY_DIR, and its full path
is available in the $BITRISE_IDEDISTRIBUTION_LOGS_PATH environment variable`)
			} else {
				s.logger.Warnf(`If you can't find the reason of the error in the log, please check the xcdistributionlogs
The logs directory is stored in $BITRISE_DEPLOY_DIR, and its full path
is available in the $BITRISE_IDEDISTRIBUTION_LOGS_PATH environment variable`)
			}
		}

		return out, fmt.Errorf("failed to export IPA: %w", exportErr)
	}

	out.ExportOptionsPath = exportOptionsPath
	out.IPAExportDir = ipaExportDir

	return out, nil
}
