package step

import (
	"bytes"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/bitrise-io/go-utils/progress"
	"github.com/bitrise-io/go-utils/v2/log"
	"github.com/bitrise-io/go-xcode/xcodebuild"
	cache "github.com/bitrise-io/go-xcode/xcodecache"
	"github.com/bitrise-io/go-xcode/xcpretty"
)

func runArchiveCommandWithRetry(archiveCmd *xcodebuild.CommandBuilder, useXcpretty bool, swiftPackagesPath string, logger log.Logger) (string, error) {
	output, err := runArchiveCommand(archiveCmd, useXcpretty, logger)
	if err != nil && swiftPackagesPath != "" && strings.Contains(output, cache.SwiftPackagesStateInvalid) {
		logger.Warnf("Archive failed, swift packages cache is in an invalid state, error: %s", err)
		// TODO: analytics log
		//logger.RWarnf("xcode-archive", "swift-packages-cache-invalid", nil, "swift packages cache is in an invalid state")
		if err := os.RemoveAll(swiftPackagesPath); err != nil {
			return output, fmt.Errorf("failed to remove invalid Swift package caches, error: %s", err)
		}
		return runArchiveCommand(archiveCmd, useXcpretty, logger)
	}
	return output, err
}

func runArchiveCommand(archiveCmd *xcodebuild.CommandBuilder, useXcpretty bool, logger log.Logger) (string, error) {
	if useXcpretty {
		xcprettyCmd := xcpretty.New(archiveCmd)

		logger.TDonef("$ %s", xcprettyCmd.PrintableCmd())
		fmt.Println()

		return xcprettyCmd.Run()
	}
	// Using xcodebuild
	logger.TDonef("$ %s", archiveCmd.PrintableCmd())
	fmt.Println()

	var output bytes.Buffer
	archiveRootCmd := archiveCmd.Command()
	archiveRootCmd.SetStdout(&output)
	archiveRootCmd.SetStderr(&output)

	var err error
	progress.SimpleProgress(".", time.Minute, func() {
		err = archiveRootCmd.Run()
	})

	return output.String(), err
}
