package main

import (
	"os"

	"github.com/bitrise-io/go-utils/v2/command"
	"github.com/bitrise-io/go-utils/v2/fileutil"

	"github.com/bitrise-io/go-steputils/v2/stepconf"
	"github.com/bitrise-io/go-utils/v2/env"
	"github.com/bitrise-io/go-utils/v2/pathutil"

	"github.com/bitrise-io/go-utils/v2/log"
	"github.com/bitrise-steplib/steps-xcode-archive/step"
)

func main() {
	os.Exit(run())
}

func run() int {
	logger := log.NewLogger()
	archiver := createXcodebuildArchiver(logger)

	config, err := archiver.ProcessInputs()
	if err != nil {
		logger.Errorf("Process Inputs failed: %s", err)
		return 1
	}

	dependenciesOpts := step.EnsureDependenciesOpts{
		XCPretty: config.LogFormatter == "xcpretty",
	}
	if err := archiver.EnsureDependencies(dependenciesOpts); err != nil {
		logger.Warnf(err.Error())
		logger.Warnf("Switching to xcodebuild for output tool")
		config.LogFormatter = "xcodebuild"
	}

	exitCode := 0
	runOpts := createRunOptions(config)
	result, err := archiver.Run(runOpts)
	if err != nil {
		logger.Errorf("Run failed: %s", err)
		exitCode = 1
		// don't return as step outputs needs to be exported even in case of failure (for example the xcodebuild logs)
	}

	exportOpts := createExportOptions(config, result)
	if err := archiver.ExportOutput(exportOpts); err != nil {
		logger.Errorf("Export Outputs failed: %s", err)
		exitCode = 1
	}

	return exitCode
}

func createXcodebuildArchiver(logger log.Logger) step.XcodebuildArchiver {
	xcodeVersionProvider := step.NewXcodebuildXcodeVersionProvider()
	envRepository := env.NewRepository()
	inputParser := stepconf.NewInputParser(envRepository)
	pathProvider := pathutil.NewPathProvider()
	pathChecker := pathutil.NewPathChecker()
	pathModifier := pathutil.NewPathModifier()
	fileManager := fileutil.NewFileManager()
	cmdFactory := command.NewFactory(envRepository)

	return step.NewXcodebuildArchiver(xcodeVersionProvider, inputParser, pathProvider, pathChecker, pathModifier, fileManager, logger, cmdFactory)
}

func createRunOptions(config step.Config) step.RunOpts {
	return step.RunOpts{
		ProjectPath:       config.ProjectPath,
		Scheme:            config.Scheme,
		Configuration:     config.Configuration,
		LogFormatter:      config.LogFormatter,
		XcodeMajorVersion: config.XcodeMajorVersion,
		ArtifactName:      config.ArtifactName,

		CodesignManager: config.CodesignManager,

		PerformCleanAction:          config.PerformCleanAction,
		XcconfigContent:             config.XcconfigContent,
		XcodebuildAdditionalOptions: config.XcodebuildAdditionalOptions,
		CacheLevel:                  config.CacheLevel,

		CustomExportOptionsPlistContent: config.ExportOptionsPlistContent,
		ExportMethod:                    config.ExportMethod,
		ICloudContainerEnvironment:      config.ICloudContainerEnvironment,
		ExportDevelopmentTeam:           config.ExportDevelopmentTeam,
		UploadBitcode:                   config.UploadBitcode,
		CompileBitcode:                  config.CompileBitcode,
	}
}

func createExportOptions(config step.Config, result step.RunResult) step.ExportOpts {
	return step.ExportOpts{
		OutputDir:      config.OutputDir,
		ArtifactName:   result.ArtifactName,
		ExportAllDsyms: config.ExportAllDsyms,

		Archive: result.Archive,

		ExportOptionsPath: result.ExportOptionsPath,
		IPAExportDir:      result.IPAExportDir,

		XcodebuildArchiveLog:       result.XcodebuildArchiveLog,
		XcodebuildExportArchiveLog: result.XcodebuildExportArchiveLog,
		IDEDistrubutionLogsDir:     result.IDEDistrubutionLogsDir,
	}
}
