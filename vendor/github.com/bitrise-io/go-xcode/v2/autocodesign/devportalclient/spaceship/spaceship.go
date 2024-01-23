// Package spaceship implements autocodesign.DevPortalClient, using Apple ID as the authentication method.
//
// The actual calls are made by the spaceship Ruby package, this is achieved by wrapping a Ruby project.
package spaceship

import (
	"embed"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/bitrise-io/go-steputils/v2/ruby"
	"github.com/bitrise-io/go-utils/errorutil"
	"github.com/bitrise-io/go-utils/log"
	"github.com/bitrise-io/go-utils/v2/command"
	"github.com/bitrise-io/go-xcode/appleauth"
	"github.com/bitrise-io/go-xcode/v2/autocodesign"
)

//go:embed spaceship
var spaceship embed.FS

// Client ...
type Client struct {
	workDir    string
	authConfig appleauth.AppleID
	teamID     string

	cmdFactory ruby.CommandFactory
}

// NewClient ...
func NewClient(authConfig appleauth.AppleID, teamID string, cmdFactory ruby.CommandFactory) (*Client, error) {
	dir, err := prepareSpaceship(cmdFactory)
	if err != nil {
		return nil, err
	}

	return &Client{
		workDir:    dir,
		authConfig: authConfig,
		teamID:     teamID,
		cmdFactory: cmdFactory,
	}, nil
}

// DevPortalClient ...
type DevPortalClient struct {
	*CertificateSource
	*ProfileClient
	*DeviceClient
}

// NewSpaceshipDevportalClient ...
func NewSpaceshipDevportalClient(client *Client) autocodesign.DevPortalClient {
	return DevPortalClient{
		CertificateSource: NewSpaceshipCertificateSource(client),
		DeviceClient:      NewDeviceClient(client),
		ProfileClient:     NewSpaceshipProfileClient(client),
	}
}

type spaceshipCommand struct {
	command              command.Command
	printableCommandArgs string
}

func (c *Client) createSpaceshipCommand(subCommand string, opts ...string) (spaceshipCommand, error) {
	authParams := []string{
		"--username", c.authConfig.Username,
		"--password", c.authConfig.Password,
		"--session", base64.StdEncoding.EncodeToString([]byte(c.authConfig.Session)),
		"--team-id", c.teamID,
	}
	s := []string{"main.rb",
		"--subcommand", subCommand,
	}
	s = append(s, opts...)
	printableCommand := strings.Join(s, " ")
	s = append(s, authParams...)

	cmd := c.cmdFactory.CreateBundleExec("ruby", s, "", &command.Opts{
		Dir: c.workDir,
	})

	return spaceshipCommand{
		command:              cmd,
		printableCommandArgs: printableCommand,
	}, nil
}

func (c *Client) runSpaceshipCommand(subCommand string, opts ...string) (string, error) {
	var spaceshipOut string
	var spaceshipErr error
	for i := 1; i <= 3; i++ {
		cmd, err := c.createSpaceshipCommand(subCommand, opts...)
		if err != nil {
			return "", err
		}

		log.Debugf("$ %s", cmd.printableCommandArgs)

		spaceshipOut, spaceshipErr = c.runSpaceshipCommandOnce(cmd)
		if spaceshipErr == nil {
			return spaceshipOut, nil
		} else if shouldRetrySpaceshipCommand(spaceshipOut) {
			log.Debugf(spaceshipOut)
			log.Warnf("spaceship command failed with a retryable error, retrying (%d. attempt)...", i)

			waitTimeSec := i * 15
			time.Sleep(time.Duration(waitTimeSec) * time.Second)
		} else {
			return "", spaceshipErr
		}
	}

	return spaceshipOut, spaceshipErr
}

func (c *Client) runSpaceshipCommandOnce(cmd spaceshipCommand) (string, error) {
	output, err := cmd.command.RunAndReturnTrimmedCombinedOutput()
	if err != nil {
		// Omitting err from log, to avoid logging plaintext password present in command params
		var exitError *exec.ExitError
		if errors.As(err, &exitError) {
			return output, fmt.Errorf("spaceship command exited with status %d, output: %s", exitError.ProcessState.ExitCode(), output)
		}

		return output, fmt.Errorf("spaceship command failed with output: %s", output)
	}

	jsonRegexp := regexp.MustCompile(`(?m)^\{.*\}$`)
	match := jsonRegexp.FindString(output)
	if match == "" {
		return "", fmt.Errorf("output does not contain response: %s", output)
	}

	var response struct {
		Error       string `json:"error"`
		ShouldRetry bool   `json:"retry"`
	}
	if err := json.Unmarshal([]byte(match), &response); err != nil {
		return "", fmt.Errorf("failed to unmarshal response: %v (%s)", err, match)
	}

	if response.ShouldRetry {
		return "", autocodesign.NewProfilesInconsistentError(errors.New(response.Error))
	}
	if response.Error != "" {
		return "", fmt.Errorf("failed to query Developer Portal: %s", response.Error)
	}

	return match, nil
}

func prepareSpaceship(cmdFactory ruby.CommandFactory) (string, error) {
	targetDir, err := os.MkdirTemp("", "")
	if err != nil {
		return "", err
	}

	fsys, err := fs.Sub(spaceship, "spaceship")
	if err != nil {
		return "", err
	}

	if err := fs.WalkDir(fsys, ".", func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			log.Warnf("%s", err)
			return err
		}

		if d.IsDir() {
			return os.MkdirAll(filepath.Join(targetDir, path), 0700)
		}

		content, err := fs.ReadFile(fsys, path)
		if err != nil {
			return err
		}

		if err := os.WriteFile(filepath.Join(targetDir, path), content, 0700); err != nil {
			return err
		}

		return nil
	}); err != nil {
		return "", err
	}

	bundlerVersion := "2.2.24"
	cmds := cmdFactory.CreateGemInstall("bundler", bundlerVersion, false, true, &command.Opts{
		Dir: targetDir,
	})
	for _, cmd := range cmds {
		fmt.Println()
		log.Donef("$ %s", cmd.PrintableCommandArgs())

		output, err := cmd.RunAndReturnTrimmedCombinedOutput()
		if err != nil {
			if errorutil.IsExitStatusError(err) {
				return "", fmt.Errorf("Installing bundler gem failed: %s", output)
			}

			return "", fmt.Errorf("running command failed: %s", err)
		}
	}

	fmt.Println()
	bundleInstallCmd := cmdFactory.CreateBundleInstall(bundlerVersion, &command.Opts{
		Dir: targetDir,
	})

	fmt.Println()
	log.Donef("$ %s", bundleInstallCmd.PrintableCommandArgs())

	output, err := bundleInstallCmd.RunAndReturnTrimmedCombinedOutput()
	if err != nil {
		if errorutil.IsExitStatusError(err) {
			return "", fmt.Errorf("bundle install failed: %s", output)
		}

		return "", fmt.Errorf("running command failed: %s", err)
	}

	return targetDir, nil
}

func shouldRetrySpaceshipCommand(out string) bool {
	if out == "" {
		return false
	}
	return strings.Contains(out, "503 Service Temporarily Unavailable")
}
