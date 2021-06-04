package main

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/bitrise-io/go-steputils/input"
	"github.com/bitrise-io/go-steputils/stepconf"
	"github.com/bitrise-io/go-utils/colorstring"
	"github.com/bitrise-io/go-utils/command"
	"github.com/bitrise-io/go-utils/errorutil"
	"github.com/bitrise-io/go-utils/fileutil"
	"github.com/bitrise-io/go-utils/log"
	"github.com/bitrise-io/go-utils/pathutil"
	"github.com/bitrise-io/go-utils/sliceutil"
	"github.com/bitrise-io/go-utils/stringutil"
	"github.com/bitrise-io/go-xcode/exportoptions"
	"github.com/bitrise-io/go-xcode/models"
	"github.com/bitrise-io/go-xcode/profileutil"
	"github.com/bitrise-io/go-xcode/utility"
	"github.com/bitrise-io/go-xcode/xcarchive"
	"github.com/bitrise-io/go-xcode/xcodebuild"
	cache "github.com/bitrise-io/go-xcode/xcodecache"
	"github.com/bitrise-io/go-xcode/xcpretty"
	"github.com/bitrise-steplib/steps-xcode-archive/utils"
	"github.com/kballard/go-shellquote"
	"howett.net/plist"
)

const (
	minSupportedXcodeMajorVersion = 9
)

const (
	bitriseXcodeRawResultTextEnvKey     = "BITRISE_XCODE_RAW_RESULT_TEXT_PATH"
	bitriseIDEDistributionLogsPthEnvKey = "BITRISE_IDEDISTRIBUTION_LOGS_PATH"
	bitriseXCArchivePthEnvKey           = "BITRISE_XCARCHIVE_PATH"
	bitriseXCArchiveZipPthEnvKey        = "BITRISE_XCARCHIVE_ZIP_PATH"
	bitriseAppDirPthEnvKey              = "BITRISE_APP_DIR_PATH"
	bitriseIPAPthEnvKey                 = "BITRISE_IPA_PATH"
	bitriseDSYMDirPthEnvKey             = "BITRISE_DSYM_DIR_PATH"
	bitriseDSYMPthEnvKey                = "BITRISE_DSYM_PATH"
)

// Inputs ...
type Inputs struct {
	ExportMethod               string `env:"export_method,opt[auto-detect,app-store,ad-hoc,enterprise,development]"`
	UploadBitcode              bool   `env:"upload_bitcode,opt[yes,no]"`
	CompileBitcode             bool   `env:"compile_bitcode,opt[yes,no]"`
	ICloudContainerEnvironment string `env:"icloud_container_environment"`
	TeamID                     string `env:"team_id"`

	ForceTeamID                       string `env:"force_team_id"`
	ForceProvisioningProfileSpecifier string `env:"force_provisioning_profile_specifier"`
	ForceProvisioningProfile          string `env:"force_provisioning_profile"`
	ForceCodeSignIdentity             string `env:"force_code_sign_identity"`
	CustomExportOptionsPlistContent   string `env:"custom_export_options_plist_content"`

	OutputTool                string `env:"output_tool,opt[xcpretty,xcodebuild]"`
	Workdir                   string `env:"workdir"`
	ProjectPath               string `env:"project_path,file"`
	Scheme                    string `env:"scheme,required"`
	Configuration             string `env:"configuration"`
	OutputDir                 string `env:"output_dir,required"`
	IsCleanBuild              bool   `env:"is_clean_build,opt[yes,no]"`
	XcodebuildOptions         string `env:"xcodebuild_options"`
	DisableIndexWhileBuilding bool   `env:"disable_index_while_building,opt[yes,no]"`

	ExportAllDsyms bool   `env:"export_all_dsyms,opt[yes,no]"`
	ArtifactName   string `env:"artifact_name"`
	VerboseLog     bool   `env:"verbose_log,opt[yes,no]"`

	CacheLevel string `env:"cache_level,opt[none,swift_packages]"`
}

// Config ...
type Config struct {
	Inputs
	XcodeMajorVersion int
}

func findIDEDistrubutionLogsPath(output string) (string, error) {
	pattern := `IDEDistribution: -\[IDEDistributionLogging _createLoggingBundleAtPath:\]: Created bundle at path '(?P<log_path>.*)'`
	re := regexp.MustCompile(pattern)

	scanner := bufio.NewScanner(strings.NewReader(output))
	for scanner.Scan() {
		line := scanner.Text()
		if match := re.FindStringSubmatch(line); len(match) == 2 {
			return match[1], nil
		}
	}
	if err := scanner.Err(); err != nil {
		return "", err
	}

	return "", nil
}

func currentTimestamp() string {
	timeStampFormat := "15:04:05"
	currentTime := time.Now()
	return currentTime.Format(timeStampFormat)
}

// ColoringFunc ...
type ColoringFunc func(...interface{}) string

func logWithTimestamp(coloringFunc ColoringFunc, format string, v ...interface{}) {
	message := fmt.Sprintf(format, v...)
	messageWithTimeStamp := fmt.Sprintf("[%s] %s", currentTimestamp(), coloringFunc(message))
	fmt.Println(messageWithTimeStamp)
}

func determineExportMethod(desiredExportMethod string, archiveExportMethod exportoptions.Method) (exportoptions.Method, error) {
	if desiredExportMethod == "auto-detect" {
		log.Printf("auto-detect export method specified: using the archive profile's export method: %s", archiveExportMethod)
		return archiveExportMethod, nil
	}

	exportMethod, err := exportoptions.ParseMethod(desiredExportMethod)
	if err != nil {
		return "", fmt.Errorf("failed to parse export method: %s", err)
	}
	log.Printf("export method specified: %s", desiredExportMethod)

	return exportMethod, nil
}

func exportDSYMs(dsymDir string, dsyms []string) error {
	for _, dsym := range dsyms {
		if err := command.CopyDir(dsym, dsymDir, false); err != nil {
			return fmt.Errorf("could not copy (%s) to directory (%s): %s", dsym, dsymDir, err)
		}
	}
	return nil
}

type xcodeVersionProvider interface {
	GetXcodeVersion() (models.XcodebuildVersionModel, error)
}

type xcodebuildXcodeVersionProvider struct {
}

func newXcodebuildXcodeVersionProvider() xcodebuildXcodeVersionProvider {
	return xcodebuildXcodeVersionProvider{}
}

// GetXcodeVersion ...
func (p xcodebuildXcodeVersionProvider) GetXcodeVersion() (models.XcodebuildVersionModel, error) {
	return utility.GetXcodeVersion()
}

type stepInputParser interface {
	Parse(conf interface{}) error
}

type envStepInputParser struct {
}

func newEnvStepInputParser() envStepInputParser {
	return envStepInputParser{}
}

// Parse ...
func (p envStepInputParser) Parse(conf interface{}) error {
	return stepconf.Parse(conf)
}

// XcodeArchiveStep ...
type XcodeArchiveStep struct {
	xcodeVersionProvider xcodeVersionProvider
	stepInputParser      stepInputParser
}

// NewXcodeArchiveStep ...
func NewXcodeArchiveStep() XcodeArchiveStep {
	return XcodeArchiveStep{
		xcodeVersionProvider: newXcodebuildXcodeVersionProvider(),
		stepInputParser:      newEnvStepInputParser(),
	}
}

// ProcessInputs ...
func (s XcodeArchiveStep) ProcessInputs() (Config, error) {
	var inputs Inputs
	if err := s.stepInputParser.Parse(&inputs); err != nil {
		return Config{}, fmt.Errorf("issue with input: %s", err)
	}

	stepconf.Print(inputs)
	fmt.Println()

	config := Config{Inputs: inputs}
	log.SetEnableDebugLog(config.VerboseLog)

	if config.ExportMethod == "auto-detect" {
		exportMethods := []exportoptions.Method{exportoptions.MethodAppStore, exportoptions.MethodAdHoc, exportoptions.MethodEnterprise, exportoptions.MethodDevelopment}
		log.Warnf("Export method: auto-detect is DEPRECATED, use a direct export method %s", exportMethods)
		fmt.Println()
	}

	if config.Workdir != "" {
		if err := input.ValidateIfDirExists(config.Workdir); err != nil {
			return Config{}, fmt.Errorf("issue with input Workdir: " + err.Error())
		}
	}

	if config.CustomExportOptionsPlistContent != "" {
		var options map[string]interface{}
		if _, err := plist.Unmarshal([]byte(config.CustomExportOptionsPlistContent), &options); err != nil {
			return Config{}, fmt.Errorf("issue with input CustomExportOptionsPlistContent: " + err.Error())
		}
	}

	if filepath.Ext(config.ProjectPath) != ".xcodeproj" && filepath.Ext(config.ProjectPath) != ".xcworkspace" {
		return Config{}, fmt.Errorf("issue with input ProjectPath: should be and .xcodeproj or .xcworkspace path")
	}

	log.Infof("Xcode version:")

	// Detect Xcode major version
	xcodebuildVersion, err := s.xcodeVersionProvider.GetXcodeVersion()
	if err != nil {
		return Config{}, fmt.Errorf("failed to determine xcode version, error: %s", err)
	}
	log.Printf("%s (%s)", xcodebuildVersion.Version, xcodebuildVersion.BuildVersion)

	xcodeMajorVersion := xcodebuildVersion.MajorVersion
	if xcodeMajorVersion < minSupportedXcodeMajorVersion {
		return Config{}, fmt.Errorf("invalid xcode major version (%d), should not be less then min supported: %d", xcodeMajorVersion, minSupportedXcodeMajorVersion)
	}
	config.XcodeMajorVersion = int(xcodeMajorVersion)

	// Validation CustomExportOptionsPlistContent
	customExportOptionsPlistContent := strings.TrimSpace(config.CustomExportOptionsPlistContent)
	if customExportOptionsPlistContent != config.CustomExportOptionsPlistContent {
		fmt.Println()
		log.Warnf("CustomExportOptionsPlistContent is stripped to remove spaces and new lines:")
		log.Printf(customExportOptionsPlistContent)
	}

	if customExportOptionsPlistContent != "" {
		if xcodeMajorVersion < 7 {
			fmt.Println()
			log.Warnf("CustomExportOptionsPlistContent is set, but CustomExportOptionsPlistContent only used if xcodeMajorVersion > 6")
			customExportOptionsPlistContent = ""
		} else {
			fmt.Println()
			log.Warnf("Ignoring the following options because CustomExportOptionsPlistContent provided:")
			log.Printf("- ExportMethod: %s", config.ExportMethod)
			log.Printf("- UploadBitcode: %s", config.UploadBitcode)
			log.Printf("- CompileBitcode: %s", config.CompileBitcode)
			log.Printf("- TeamID: %s", config.TeamID)
			log.Printf("- ICloudContainerEnvironment: %s", config.ICloudContainerEnvironment)
			fmt.Println()
		}
	}
	config.CustomExportOptionsPlistContent = customExportOptionsPlistContent

	if config.ForceProvisioningProfileSpecifier != "" &&
		xcodeMajorVersion < 8 {
		fmt.Println()
		log.Warnf("ForceProvisioningProfileSpecifier is set, but ForceProvisioningProfileSpecifier only used if xcodeMajorVersion > 7")
		config.ForceProvisioningProfileSpecifier = ""
	}

	if config.ForceTeamID != "" &&
		xcodeMajorVersion < 8 {
		fmt.Println()
		log.Warnf("ForceTeamID is set, but ForceTeamID only used if xcodeMajorVersion > 7")
		config.ForceTeamID = ""
	}

	if config.ForceProvisioningProfileSpecifier != "" &&
		config.ForceProvisioningProfile != "" {
		fmt.Println()
		log.Warnf("both ForceProvisioningProfileSpecifier and ForceProvisioningProfile are set, using ForceProvisioningProfileSpecifier")
		config.ForceProvisioningProfile = ""
	}

	fmt.Println()

	absProjectPath, err := filepath.Abs(config.ProjectPath)
	if err != nil {
		return Config{}, fmt.Errorf("failed to get absolute project path, error: %s", err)
	}
	config.ProjectPath = absProjectPath

	// abs out dir pth
	absOutputDir, err := pathutil.AbsPath(config.OutputDir)
	if err != nil {
		return Config{}, fmt.Errorf("failed to expand OutputDir (%s), error: %s", config.OutputDir, err)
	}
	config.OutputDir = absOutputDir

	if exist, err := pathutil.IsPathExists(config.OutputDir); err != nil {
		return Config{}, fmt.Errorf("failed to check if OutputDir exist, error: %s", err)
	} else if !exist {
		if err := os.MkdirAll(config.OutputDir, 0777); err != nil {
			return Config{}, fmt.Errorf("failed to create OutputDir (%s), error: %s", config.OutputDir, err)
		}
	}

	return config, nil
}

// EnsureDependenciesOpts ...
type EnsureDependenciesOpts struct {
	XCPretty bool
}

// EnsureDependencies ...
func (s XcodeArchiveStep) EnsureDependencies(opts EnsureDependenciesOpts) error {
	if !opts.XCPretty {
		return nil
	}

	fmt.Println()
	log.Infof("Checking if output tool (xcpretty) is installed")

	installed, err := xcpretty.IsInstalled()
	if err != nil {
		return fmt.Errorf("failed to check if xcpretty is installed, error: %s", err)
	} else if !installed {
		log.Warnf(`xcpretty is not installed`)
		fmt.Println()
		log.Printf("Installing xcpretty")

		cmds, err := xcpretty.Install()
		if err != nil {
			return fmt.Errorf("failed to create xcpretty install command: %s", err)
		}

		for _, cmd := range cmds {
			if out, err := cmd.RunAndReturnTrimmedCombinedOutput(); err != nil {
				if errorutil.IsExitStatusError(err) {
					return fmt.Errorf("%s failed: %s", cmd.PrintableCommandArgs(), out)
				}
				return fmt.Errorf("%s failed: %s", cmd.PrintableCommandArgs(), err)
			}
		}

	}

	xcprettyVersion, err := xcpretty.Version()
	if err != nil {
		return fmt.Errorf("failed to determine xcpretty version, error: %s", err)
	}
	log.Printf("- xcprettyVersion: %s", xcprettyVersion.String())

	return nil
}

type xcodeArchiveOpts struct {
	ProjectPath       string
	Scheme            string
	Configuration     string
	OutputTool        string
	XcodeMajorVersion int
	ArtifactName      string

	ForceTeamID                       string
	ForceProvisioningProfileSpecifier string
	ForceProvisioningProfile          string
	ForceCodeSignIdentity             string
	IsCleanBuild                      bool
	DisableIndexWhileBuilding         bool
	XcodebuildOptions                 string

	CacheLevel string
}

type xcodeArchiveOutput struct {
	ArchivePath        string
	AppPath            string
	AppDSYMPaths       []string
	FrameworkDSYMPaths []string
	XcodebuildLog      string
}

func (s XcodeArchiveStep) xcodeArchive(opts xcodeArchiveOpts) (xcodeArchiveOutput, error) {
	out := xcodeArchiveOutput{}

	// Open Xcode project
	xcodeProj, scheme, configuration, err := utils.OpenArchivableProject(opts.ProjectPath, opts.Scheme, opts.Configuration)
	if err != nil {
		return out, fmt.Errorf("failed to open project: %s: %s", opts.ProjectPath, err)
	}

	platform, err := utils.BuildableTargetPlatform(xcodeProj, scheme, configuration, utils.XcodeBuild{})
	if err != nil {
		return out, fmt.Errorf("failed to read project platform: %s: %s", opts.ProjectPath, err)
	}

	mainTarget, err := archivableApplicationTarget(xcodeProj, scheme, configuration)
	if err != nil {
		return out, fmt.Errorf("failed to read main application target: %s", err)
	}
	if mainTarget.ProductType == appClipProductType {
		log.Errorf("Selected scheme: '%s' targets an App Clip target (%s),", opts.Scheme, mainTarget.Name)
		log.Errorf("'Xcode Archive & Export for iOS' step is intended to archive the project using a scheme targeting an Application target.")
		log.Errorf("Please select a scheme targeting an Application target to archive and export the main Application")
		log.Errorf("and use 'Export iOS and tvOS Xcode archive' step to export an App Clip.")
		os.Exit(1)
	}

	// Create the Archive with Xcode Command Line tools
	log.Infof("Creating the Archive ...")

	isWorkspace := false
	ext := filepath.Ext(opts.ProjectPath)
	if ext == ".xcodeproj" {
		isWorkspace = false
	} else if ext == ".xcworkspace" {
		isWorkspace = true
	} else {
		return out, fmt.Errorf("project file extension should be .xcodeproj or .xcworkspace, but got: %s", ext)
	}

	archiveCmd := xcodebuild.NewCommandBuilder(opts.ProjectPath, isWorkspace, xcodebuild.ArchiveAction)
	archiveCmd.SetScheme(opts.Scheme)
	archiveCmd.SetConfiguration(opts.Configuration)

	if opts.ForceTeamID != "" {
		log.Printf("Forcing Development Team: %s", opts.ForceTeamID)
		archiveCmd.SetForceDevelopmentTeam(opts.ForceTeamID)
	}
	if opts.ForceProvisioningProfileSpecifier != "" {
		log.Printf("Forcing Provisioning Profile Specifier: %s", opts.ForceProvisioningProfileSpecifier)
		archiveCmd.SetForceProvisioningProfileSpecifier(opts.ForceProvisioningProfileSpecifier)
	}
	if opts.ForceProvisioningProfile != "" {
		log.Printf("Forcing Provisioning Profile: %s", opts.ForceProvisioningProfile)
		archiveCmd.SetForceProvisioningProfile(opts.ForceProvisioningProfile)
	}
	if opts.ForceCodeSignIdentity != "" {
		log.Printf("Forcing Code Signing Identity: %s", opts.ForceCodeSignIdentity)
		archiveCmd.SetForceCodeSignIdentity(opts.ForceCodeSignIdentity)
	}

	if opts.IsCleanBuild {
		archiveCmd.SetCustomBuildAction("clean")
	}

	archiveCmd.SetDisableIndexWhileBuilding(opts.DisableIndexWhileBuilding)

	tmpDir, err := pathutil.NormalizedOSTempDirPath("xcodeArchive")
	if err != nil {
		return out, fmt.Errorf("failed to create temp dir, error: %s", err)
	}
	archivePth := filepath.Join(tmpDir, opts.ArtifactName+".xcarchive")

	archiveCmd.SetArchivePath(archivePth)

	destination := "generic/platform=" + string(platform)
	options := []string{"-destination", destination}
	if opts.XcodebuildOptions != "" {
		userOptions, err := shellquote.Split(opts.XcodebuildOptions)
		if err != nil {
			return out, fmt.Errorf("failed to shell split XcodebuildOptions (%s), error: %s", opts.XcodebuildOptions, err)
		}

		if sliceutil.IsStringInSlice("-destination", userOptions) {
			options = userOptions
		} else {
			options = append(options, userOptions...)
		}
	}
	archiveCmd.SetCustomOptions(options)

	var swiftPackagesPath string
	if opts.XcodeMajorVersion >= 11 {
		var err error
		if swiftPackagesPath, err = cache.SwiftPackagesPath(opts.ProjectPath); err != nil {
			return out, fmt.Errorf("failed to get Swift Packages path, error: %s", err)
		}
	}

	xcodebuildLog, err := runArchiveCommandWithRetry(archiveCmd, opts.OutputTool == "xcpretty", swiftPackagesPath)
	if err != nil || opts.OutputTool == "xcodebuild" {
		const lastLinesMsg = "\nLast lines of the Xcode's build log:"
		if err != nil {
			log.Infof(colorstring.Red(lastLinesMsg))
		} else {
			log.Infof(lastLinesMsg)
		}
		fmt.Println(stringutil.LastNLines(xcodebuildLog, 20))

		log.Warnf(`You can find the last couple of lines of Xcode's build log above, but the full log will be also available in the raw-xcodebuild-output.log
The log file will be stored in $BITRISE_DEPLOY_DIR, and its full path will be available in the $BITRISE_XCODE_RAW_RESULT_TEXT_PATH environment variable.`)
	}
	if err != nil {
		out.XcodebuildLog = xcodebuildLog
		return out, fmt.Errorf("archive failed, error: %s", err)
	}

	// Ensure xcarchive exists
	if exist, err := pathutil.IsPathExists(archivePth); err != nil {
		return out, fmt.Errorf("failed to check if archive exist, error: %s", err)
	} else if !exist {
		return out, fmt.Errorf("no archive generated at: %s", archivePth)
	}
	out.ArchivePath = archivePth

	archive, err := xcarchive.NewIosArchive(archivePth)
	if err != nil {
		return out, fmt.Errorf("failed to parse archive, error: %s", err)
	}

	mainApplication := archive.Application
	out.AppPath = mainApplication.Path

	fmt.Println()
	log.Infof("Archive infos:")
	log.Printf("team: %s (%s)", mainApplication.ProvisioningProfile.TeamName, mainApplication.ProvisioningProfile.TeamID)
	log.Printf("profile: %s (%s)", mainApplication.ProvisioningProfile.Name, mainApplication.ProvisioningProfile.UUID)
	log.Printf("export: %s", mainApplication.ProvisioningProfile.ExportType)
	log.Printf("xcode managed profile: %v", profileutil.IsXcodeManaged(mainApplication.ProvisioningProfile.Name))

	appDSYMPaths, frameworkDSYMPaths, err := archive.FindDSYMs()
	if err != nil {
		return out, fmt.Errorf("failed to export dsyms, error: %s", err)
	}
	out.AppDSYMPaths = appDSYMPaths
	out.FrameworkDSYMPaths = frameworkDSYMPaths

	// Cache swift PM
	if opts.XcodeMajorVersion >= 11 && opts.CacheLevel == "swift_packages" {
		if err := cache.CollectSwiftPackages(opts.ProjectPath); err != nil {
			log.Warnf("Failed to mark swift packages for caching, error: %s", err)
		}
	}

	return out, nil
}

type xcodeIPAExportOpts struct {
	ProjectPath       string
	Scheme            string
	Configuration     string
	OutputTool        string
	XcodeMajorVersion int

	ArchivePath                     string
	CustomExportOptionsPlistContent string
	ExportMethod                    string
	ICloudContainerEnvironment      string
	TeamID                          string
	UploadBitcode                   bool
	CompileBitcode                  bool
}

type xcodeIPAExportOutput struct {
	ExportOptionsPath      string
	IPAExportDir           string
	XcodebuildLog          string
	IDEDistrubutionLogsDir string
}

func (s XcodeArchiveStep) xcodeIPAExport(opts xcodeIPAExportOpts) (xcodeIPAExportOutput, error) {
	out := xcodeIPAExportOutput{}

	// Exporting the ipa with Xcode Command Line tools

	/*
		You'll get a "Error Domain=IDEDistributionErrorDomain Code=14 "No applicable devices found."" error
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

	fmt.Println()
	log.Infof("Exporting ipa from the archive...")

	tmpDir, err := pathutil.NormalizedOSTempDirPath("xcodeIPAExport")
	if err != nil {
		return out, fmt.Errorf("failed to create temp dir, error: %s", err)
	}

	exportOptionsPath := filepath.Join(tmpDir, "export_options.plist")

	if opts.CustomExportOptionsPlistContent != "" {
		log.Printf("Custom export options content provided, using it:")
		fmt.Println(opts.CustomExportOptionsPlistContent)

		if err := fileutil.WriteStringToFile(exportOptionsPath, opts.CustomExportOptionsPlistContent); err != nil {
			return out, fmt.Errorf("failed to write export options to file, error: %s", err)
		}
	} else {
		log.Printf("No custom export options content provided, generating export options...")

		archive, err := xcarchive.NewIosArchive(opts.ArchivePath)
		if err != nil {
			return out, fmt.Errorf("failed to parse archive, error: %s", err)
		}
		archiveExportMethod := archive.Application.ProvisioningProfile.ExportType

		exportMethod, err := determineExportMethod(opts.ExportMethod, exportoptions.Method(archiveExportMethod))
		if err != nil {
			return out, err
		}

		xcodeProj, scheme, configuration, err := utils.OpenArchivableProject(opts.ProjectPath, opts.Scheme, opts.Configuration)
		if err != nil {
			return out, fmt.Errorf("failed to open project: %s: %s", opts.ProjectPath, err)
		}

		archiveCodeSignIsXcodeManaged := archive.IsXcodeManaged()

		generator := NewExportOptionsGenerator(xcodeProj, scheme, configuration)
		exportOptions, err := generator.GenerateApplicationExportOptions(exportMethod, opts.ICloudContainerEnvironment, opts.TeamID,
			opts.UploadBitcode, opts.CompileBitcode, archiveCodeSignIsXcodeManaged, int64(opts.XcodeMajorVersion))
		if err != nil {
			return out, err
		}

		fmt.Println()
		log.Printf("generated export options content:")
		fmt.Println()
		fmt.Println(exportOptions.String())

		if err := exportOptions.WriteToFile(exportOptionsPath); err != nil {
			return out, err
		}
	}

	ipaExportDir := filepath.Join(tmpDir, "exported")

	exportCmd := xcodebuild.NewExportCommand()
	exportCmd.SetArchivePath(opts.ArchivePath)
	exportCmd.SetExportDir(ipaExportDir)
	exportCmd.SetExportOptionsPlist(exportOptionsPath)

	if opts.OutputTool == "xcpretty" {
		xcprettyCmd := xcpretty.New(exportCmd)

		fmt.Println()
		logWithTimestamp(colorstring.Green, xcprettyCmd.PrintableCmd())

		xcodebuildLog, err := xcprettyCmd.Run()
		if err != nil {
			out.XcodebuildLog = xcodebuildLog

			log.Warnf(`If you can't find the reason of the error in the log, please check the raw-xcodebuild-output.log
			The log file is stored in $BITRISE_DEPLOY_DIR, and its full path
			is available in the $BITRISE_XCODE_RAW_RESULT_TEXT_PATH environment variable`)

			// xcdistributionlogs
			ideDistrubutionLogsDir, err := findIDEDistrubutionLogsPath(xcodebuildLog)
			if err != nil {
				log.Warnf("Failed to find xcdistributionlogs, error: %s", err)
			} else {
				out.IDEDistrubutionLogsDir = ideDistrubutionLogsDir

				criticalDistLogFilePth := filepath.Join(ideDistrubutionLogsDir, "IDEDistribution.critical.log")
				log.Warnf("IDEDistribution.critical.log:")
				if criticalDistLog, err := fileutil.ReadStringFromFile(criticalDistLogFilePth); err == nil {
					log.Printf(criticalDistLog)
				}

				log.Warnf(`Also please check the xcdistributionlogs
The logs directory is stored in $BITRISE_DEPLOY_DIR, and its full path
is available in the $BITRISE_IDEDISTRIBUTION_LOGS_PATH environment variable`)
			}

			return out, fmt.Errorf("export failed, error: %s", err)
		}
	} else {
		fmt.Println()
		logWithTimestamp(colorstring.Green, exportCmd.PrintableCmd())

		xcodebuildLog, err := exportCmd.RunAndReturnOutput()
		if err != nil {
			out.XcodebuildLog = xcodebuildLog

			// xcdistributionlogs
			ideDistrubutionLogsDir, err := findIDEDistrubutionLogsPath(xcodebuildLog)
			if err != nil {
				log.Warnf("Failed to find xcdistributionlogs, error: %s", err)
			} else {
				out.IDEDistrubutionLogsDir = ideDistrubutionLogsDir

				criticalDistLogFilePth := filepath.Join(ideDistrubutionLogsDir, "IDEDistribution.critical.log")
				log.Warnf("IDEDistribution.critical.log:")
				if criticalDistLog, err := fileutil.ReadStringFromFile(criticalDistLogFilePth); err == nil {
					log.Printf(criticalDistLog)
				}

				log.Warnf(`If you can't find the reason of the error in the log, please check the xcdistributionlogs
The logs directory is stored in $BITRISE_DEPLOY_DIR, and its full path
is available in the $BITRISE_IDEDISTRIBUTION_LOGS_PATH environment variable`)
			}

			return out, fmt.Errorf("export failed, error: %s", err)
		}
	}

	out.ExportOptionsPath = exportOptionsPath
	out.IPAExportDir = ipaExportDir

	return out, nil
}

// RunOpts ...
type RunOpts struct {
	// Shared
	ProjectPath       string
	Scheme            string
	Configuration     string
	OutputTool        string
	XcodeMajorVersion int
	ArtifactName      string

	// Archive
	ForceTeamID                       string
	ForceProvisioningProfileSpecifier string
	ForceProvisioningProfile          string
	ForceCodeSignIdentity             string
	IsCleanBuild                      bool
	DisableIndexWhileBuilding         bool
	XcodebuildOptions                 string
	CacheLevel                        string

	// IPA Export
	CustomExportOptionsPlistContent string
	ExportMethod                    string
	ICloudContainerEnvironment      string
	TeamID                          string
	UploadBitcode                   bool
	CompileBitcode                  bool
}

// RunOut ...
type RunOut struct {
	ArchivePath        string
	AppPath            string
	AppDSYMPaths       []string
	FrameworkDSYMPaths []string

	ExportOptionsPath string
	IPAExportDir      string

	XcodebuildLog          string
	IDEDistrubutionLogsDir string
}

// Run ...
func (s XcodeArchiveStep) Run(opts RunOpts) (RunOut, error) {
	out := RunOut{}

	archiveOpts := xcodeArchiveOpts{
		ProjectPath:       opts.ProjectPath,
		Scheme:            opts.Scheme,
		Configuration:     opts.Configuration,
		OutputTool:        opts.OutputTool,
		XcodeMajorVersion: opts.XcodeMajorVersion,
		ArtifactName:      opts.ArtifactName,

		ForceTeamID:                       opts.ForceTeamID,
		ForceProvisioningProfileSpecifier: opts.ForceProvisioningProfileSpecifier,
		ForceProvisioningProfile:          opts.ForceProvisioningProfile,
		ForceCodeSignIdentity:             opts.ForceCodeSignIdentity,
		IsCleanBuild:                      opts.IsCleanBuild,
		DisableIndexWhileBuilding:         opts.DisableIndexWhileBuilding,
		XcodebuildOptions:                 opts.XcodebuildOptions,
		CacheLevel:                        opts.CacheLevel,
	}
	archiveOut, err := s.xcodeArchive(archiveOpts)
	if err != nil {
		return RunOut{
			XcodebuildLog: archiveOut.XcodebuildLog,
		}, err
	}
	out.ArchivePath = archiveOut.ArchivePath
	out.AppPath = archiveOut.AppPath
	out.AppDSYMPaths = archiveOut.AppDSYMPaths
	out.FrameworkDSYMPaths = archiveOut.FrameworkDSYMPaths

	IPAExportOpts := xcodeIPAExportOpts{
		ProjectPath:       opts.ProjectPath,
		Scheme:            opts.Scheme,
		Configuration:     opts.Configuration,
		OutputTool:        opts.OutputTool,
		XcodeMajorVersion: opts.XcodeMajorVersion,

		ArchivePath:                     archiveOut.ArchivePath,
		CustomExportOptionsPlistContent: opts.CustomExportOptionsPlistContent,
		ExportMethod:                    opts.ExportMethod,
		ICloudContainerEnvironment:      opts.ICloudContainerEnvironment,
		TeamID:                          opts.TeamID,
		UploadBitcode:                   opts.UploadBitcode,
		CompileBitcode:                  opts.CompileBitcode,
	}
	exportOut, err := s.xcodeIPAExport(IPAExportOpts)
	if err != nil {
		return RunOut{
			XcodebuildLog:          exportOut.XcodebuildLog,
			IDEDistrubutionLogsDir: exportOut.IDEDistrubutionLogsDir,
		}, err
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

	ArchivePath        string
	AppPath            string
	AppDSYMPaths       []string
	FrameworkDSYMPaths []string

	ExportOptionsPath string
	IPAExportDir      string

	XcodebuildLog          string
	IDEDistrubutionLogsDir string
}

// ExportOutput ...
func (s XcodeArchiveStep) ExportOutput(opts ExportOpts) error {
	fmt.Println()
	log.Infof("Exporting outputs...")

	cleanup := func(pth string) error {
		if exist, err := pathutil.IsPathExists(pth); err != nil {
			return fmt.Errorf("failed to check if path (%s) exist, error: %s", pth, err)
		} else if exist {
			if err := os.RemoveAll(pth); err != nil {
				return fmt.Errorf("failed to remove path (%s), error: %s", pth, err)
			}
		}
		return nil
	}

	if opts.ArchivePath != "" {
		fmt.Println()
		if err := utils.ExportOutputDir(opts.ArchivePath, opts.ArchivePath, bitriseXCArchivePthEnvKey); err != nil {
			return fmt.Errorf("failed to export %s, error: %s", bitriseXCArchivePthEnvKey, err)
		}
		log.Donef("The xcarchive path is now available in the Environment Variable: %s (value: %s)", bitriseXCArchivePthEnvKey, opts.ArchivePath)

		archiveZipPath := filepath.Join(opts.OutputDir, opts.ArtifactName+".xcarchive.zip")
		if err := cleanup(archiveZipPath); err != nil {
			return err
		}

		if err := utils.ExportOutputDirAsZip(opts.ArchivePath, archiveZipPath, bitriseXCArchiveZipPthEnvKey); err != nil {
			return fmt.Errorf("failed to export %s, error: %s", bitriseXCArchiveZipPthEnvKey, err)
		}
		log.Donef("The xcarchive zip path is now available in the Environment Variable: %s (value: %s)", bitriseXCArchiveZipPthEnvKey, archiveZipPath)
	}

	if opts.AppPath != "" {
		fmt.Println()
		appPath := filepath.Join(opts.OutputDir, opts.ArtifactName+".app")
		if err := cleanup(appPath); err != nil {
			return err
		}

		if err := utils.ExportOutputDir(opts.AppPath, appPath, bitriseAppDirPthEnvKey); err != nil {
			return fmt.Errorf("failed to export %s, error: %s", bitriseAppDirPthEnvKey, err)
		}
		log.Donef("The app directory is now available in the Environment Variable: %s (value: %s)", bitriseAppDirPthEnvKey, opts.AppPath)
	}

	if len(opts.AppDSYMPaths) > 0 || len(opts.FrameworkDSYMPaths) > 0 {
		fmt.Println()
		dsymDir, err := pathutil.NormalizedOSTempDirPath("__dsyms__")
		if err != nil {
			return fmt.Errorf("failed to create tmp dir, error: %s", err)
		}

		if len(opts.AppDSYMPaths) > 0 {
			if err := exportDSYMs(dsymDir, opts.AppDSYMPaths); err != nil {
				return fmt.Errorf("failed to export dSYMs: %v", err)
			}
		} else {
			log.Warnf("no app dsyms found")
		}

		if opts.ExportAllDsyms && len(opts.FrameworkDSYMPaths) > 0 {
			if err := exportDSYMs(dsymDir, opts.FrameworkDSYMPaths); err != nil {
				return fmt.Errorf("failed to export dSYMs: %v", err)
			}
		}

		if err := utils.ExportOutputDir(dsymDir, dsymDir, bitriseDSYMDirPthEnvKey); err != nil {
			return fmt.Errorf("failed to export %s, error: %s", bitriseDSYMDirPthEnvKey, err)
		}
		log.Donef("The dSYM dir path is now available in the Environment Variable: %s (value: %s)", bitriseDSYMDirPthEnvKey, dsymDir)

		dsymZipPath := filepath.Join(opts.OutputDir, opts.ArtifactName+".dSYM.zip")
		if err := cleanup(dsymZipPath); err != nil {
			return err
		}

		if err := utils.ExportOutputDirAsZip(dsymDir, dsymZipPath, bitriseDSYMPthEnvKey); err != nil {
			return fmt.Errorf("failed to export %s, error: %s", bitriseDSYMPthEnvKey, err)
		}
		log.Donef("The dSYM zip path is now available in the Environment Variable: %s (value: %s)", bitriseDSYMPthEnvKey, dsymZipPath)
	}

	if opts.ExportOptionsPath != "" {
		fmt.Println()
		exportOptionsPath := filepath.Join(opts.OutputDir, "export_options.plist")
		if err := cleanup(exportOptionsPath); err != nil {
			return err
		}

		if err := command.CopyFile(opts.ExportOptionsPath, exportOptionsPath); err != nil {
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
			log.Errorf("No .ipa file found at export dir: %s", opts.IPAExportDir)
			log.Printf("File list in the export dir:")
			for _, pth := range fileList {
				log.Printf("- %s", pth)
			}
			return fmt.Errorf("")
		}

		fmt.Println()

		ipaPath := filepath.Join(opts.OutputDir, opts.ArtifactName+".ipa")
		if err := cleanup(ipaPath); err != nil {
			return err
		}

		if err := utils.ExportOutputFile(ipaFiles[0], ipaPath, bitriseIPAPthEnvKey); err != nil {
			return fmt.Errorf("failed to export %s, error: %s", bitriseIPAPthEnvKey, err)
		}
		log.Donef("The ipa path is now available in the Environment Variable: %s (value: %s)", bitriseIPAPthEnvKey, ipaPath)

		if len(ipaFiles) > 1 {
			log.Warnf("More than 1 .ipa file found, exporting first one: %s", ipaFiles[0])
			log.Warnf("Moving every ipa to the BITRISE_DEPLOY_DIR")

			for i, pth := range ipaFiles {
				if i == 0 {
					continue
				}

				base := filepath.Base(pth)
				deployPth := filepath.Join(opts.OutputDir, base)

				if err := command.CopyFile(pth, deployPth); err != nil {
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

		if err := utils.ExportOutputDirAsZip(opts.IDEDistrubutionLogsDir, ideDistributionLogsZipPath, bitriseIDEDistributionLogsPthEnvKey); err != nil {
			log.Warnf("Failed to export %s, error: %s", bitriseIDEDistributionLogsPthEnvKey, err)
		} else {
			log.Donef("The xcdistributionlogs zip path is now available in the Environment Variable: %s (value: %s)", bitriseIDEDistributionLogsPthEnvKey, ideDistributionLogsZipPath)
		}
	}

	if opts.XcodebuildLog != "" {
		xcodebuildLogPath := filepath.Join(opts.OutputDir, "raw-xcodebuild-output.log")
		if err := cleanup(xcodebuildLogPath); err != nil {
			return err
		}

		if err := utils.ExportOutputFileContent(opts.XcodebuildLog, xcodebuildLogPath, bitriseXcodeRawResultTextEnvKey); err != nil {
			log.Warnf("Failed to export %s, error: %s", bitriseXcodeRawResultTextEnvKey, err)
		} else {
			log.Donef("The raw xcodebuild log path is now available in the Environment Variable: %s (value: %s)", bitriseXcodeRawResultTextEnvKey, xcodebuildLogPath)
		}
	}

	return nil
}

// RunStep ...
func RunStep() error {
	step := NewXcodeArchiveStep()

	config, err := step.ProcessInputs()
	if err != nil {
		return err
	}

	dependenciesOpts := EnsureDependenciesOpts{
		XCPretty: config.OutputTool == "xcpretty",
	}
	if err := step.EnsureDependencies(dependenciesOpts); err != nil {
		log.Warnf(err.Error())
		log.Warnf("Switching to xcodebuild for output tool")
		config.OutputTool = "xcodebuild"
	}

	runOpts := RunOpts{
		ProjectPath:       config.ProjectPath,
		Scheme:            config.Scheme,
		Configuration:     config.Configuration,
		OutputTool:        config.OutputTool,
		XcodeMajorVersion: config.XcodeMajorVersion,
		ArtifactName:      config.ArtifactName,

		ForceTeamID:                       config.ForceTeamID,
		ForceProvisioningProfileSpecifier: config.ForceProvisioningProfileSpecifier,
		ForceProvisioningProfile:          config.ForceProvisioningProfile,
		ForceCodeSignIdentity:             config.ForceCodeSignIdentity,
		IsCleanBuild:                      config.IsCleanBuild,
		DisableIndexWhileBuilding:         config.DisableIndexWhileBuilding,
		XcodebuildOptions:                 config.XcodebuildOptions,
		CacheLevel:                        config.CacheLevel,

		CustomExportOptionsPlistContent: config.CustomExportOptionsPlistContent,
		ExportMethod:                    config.ExportMethod,
		ICloudContainerEnvironment:      config.ICloudContainerEnvironment,
		TeamID:                          config.TeamID,
		UploadBitcode:                   config.UploadBitcode,
		CompileBitcode:                  config.CompileBitcode,
	}
	out, runErr := step.Run(runOpts)

	exportOpts := ExportOpts{
		OutputDir:      config.OutputDir,
		ArtifactName:   config.ArtifactName,
		ExportAllDsyms: config.ExportAllDsyms,

		ArchivePath:        out.ArchivePath,
		AppPath:            out.AppPath,
		AppDSYMPaths:       out.AppDSYMPaths,
		FrameworkDSYMPaths: out.FrameworkDSYMPaths,

		ExportOptionsPath: out.ExportOptionsPath,
		IPAExportDir:      out.IPAExportDir,

		XcodebuildLog:          out.XcodebuildLog,
		IDEDistrubutionLogsDir: out.IDEDistrubutionLogsDir,
	}
	exportErr := step.ExportOutput(exportOpts)

	if runErr != nil {
		return runErr
	}
	if exportErr != nil {
		return exportErr
	}

	return nil
}

func main() {
	if err := RunStep(); err != nil {
		log.Errorf(err.Error())
		os.Exit(1)
	}
}
