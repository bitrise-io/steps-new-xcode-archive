package step

import (
	"bufio"
	"errors"
	"fmt"
	"os/exec"
	"regexp"
	"strings"
)

// XCPrettyInstallError is used to signal an error around xcpretty installation
type XCPrettyInstallError struct {
	err error
}

func (e XCPrettyInstallError) Error() string {
	return e.err.Error()
}

type NSError struct {
	Description string
	Suggestion  string
}

func NewNSError(str string) *NSError {
	if !isNSError(str) {
		return nil
	}

	descriptionPattern := `NSLocalizedDescription=(.+?),|NSLocalizedDescription=(.+?)}`
	description := findFirstSubMatch(str, descriptionPattern)
	if description == "" {
		return nil
	}

	suggestionPattern := `NSLocalizedRecoverySuggestion=(.+?),|NSLocalizedRecoverySuggestion=(.+?)}`
	suggestion := findFirstSubMatch(str, suggestionPattern)

	return &NSError{
		Description: description,
		Suggestion:  suggestion,
	}
}

func (e NSError) Error() string {
	msg := e.Description
	if e.Suggestion != "" {
		msg += " " + e.Suggestion
	}
	return msg
}

func isNSError(str string) bool {
	// example: Error Domain=IDEProvisioningErrorDomain Code=9 ""ios-simple-objc.app" requires a provisioning profile."
	//   UserInfo={IDEDistributionIssueSeverity=3, NSLocalizedDescription="ios-simple-objc.app" requires a provisioning profile.,
	//   NSLocalizedRecoverySuggestion=Add a profile to the "provisioningProfiles" dictionary in your Export Options property list.}
	return strings.Contains(str, "Error ") &&
		strings.Contains(str, "Domain=") &&
		strings.Contains(str, "Code=") &&
		strings.Contains(str, "UserInfo=")
}

func findFirstSubMatch(str, pattern string) string {
	exp := regexp.MustCompile(pattern)
	matches := exp.FindStringSubmatch(str)
	if len(matches) > 1 {
		for _, match := range matches[1:] {
			if match != "" {
				return match
			}
		}
	}
	return ""
}

type Printable interface {
	PrintableCmd() string
}

func wrapXcodebuildCommandError(cmd Printable, out string, err error) error {
	if err == nil {
		return nil
	}

	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		reasons := findXcodebuildErrors(out)
		if len(reasons) > 0 {
			return fmt.Errorf("command failed with exit status %d (%s): %w", exitErr.ExitCode(), cmd.PrintableCmd(), errors.New(strings.Join(reasons, "\n")))
		}
		return fmt.Errorf("command failed with exit status %d (%s)", exitErr.ExitCode(), cmd.PrintableCmd())
	}

	return fmt.Errorf("executing command failed (%s): %w", cmd.PrintableCmd(), err)
}

func findXcodebuildErrors(out string) []string {
	var errorLines []string
	var nserrors []NSError

	scanner := bufio.NewScanner(strings.NewReader(out))
	scanner.Split(bufio.ScanLines)
	for scanner.Scan() {
		line := scanner.Text()

		if strings.HasPrefix(line, "error: ") {
			errorLines = append(errorLines, line)
		} else if strings.HasPrefix(line, "Error ") {
			if e := NewNSError(line); e != nil {
				nserrors = append(nserrors, *e)
			}
		}

	}
	if err := scanner.Err(); err != nil {
		return nil
	}

	// Prefer NSErrors if found for all errors,
	// this is because an NSError has a suggestion in addition to the error reason,
	// but we use regular expression for parsing NSErrors.
	if len(nserrors) == len(errorLines) {
		errorLines = []string{}
		for _, nserror := range nserrors {
			errorLines = append(errorLines, nserror.Error())
		}
	}

	return errorLines
}
