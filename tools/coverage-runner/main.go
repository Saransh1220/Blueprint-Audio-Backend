package main

import (
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
)

func main() {
	goBin := flag.String("go", "go", "go binary path")
	coverageDir := flag.String("coverage-dir", "coverage", "coverage output directory")
	coverPkg := flag.String("coverpkg", "./cmd/...,./internal/...,./pkg/...", "coverpkg value")
	packages := flag.String("packages", "./...", "package pattern to test")
	coverMode := flag.String("covermode", "atomic", "covermode value")
	testJSON := flag.String("test-json", "", "go test -json output path")
	coverProfile := flag.String("coverprofile", "", "coverage profile output path")
	functionsOut := flag.String("functions-out", "", "go tool cover -func output path")
	coverageHTML := flag.String("coverage-html", "", "go tool cover -html output path")
	allowTestFailure := flag.Bool("allow-test-failure", false, "exit zero even when tests fail")
	flag.Parse()

	if err := os.MkdirAll(*coverageDir, 0o755); err != nil {
		fatalf("failed to create coverage directory: %v", err)
	}

	if *testJSON == "" {
		*testJSON = filepath.Join(*coverageDir, "test-report.jsonl")
	}
	if *coverProfile == "" {
		*coverProfile = filepath.Join(*coverageDir, "coverage.out")
	}
	if *functionsOut == "" {
		*functionsOut = filepath.Join(*coverageDir, "functions.txt")
	}
	if *coverageHTML == "" {
		*coverageHTML = filepath.Join(*coverageDir, "coverage-details.html")
	}

	testArgs := []string{
		"test",
		"-json",
		"-covermode", *coverMode,
		"-coverpkg", *coverPkg,
		"-coverprofile", *coverProfile,
		*packages,
	}
	testOut, testCode, err := runCommandWithExitCode(*goBin, testArgs...)
	if err != nil {
		fatalf("failed to execute go test: %v", err)
	}
	if err := os.WriteFile(*testJSON, testOut, 0o644); err != nil {
		fatalf("failed to write test json output: %v", err)
	}

	if fileExists(*coverProfile) {
		funcOut, _, err := runCommandWithExitCode(*goBin, "tool", "cover", "-func", *coverProfile)
		if err != nil {
			fatalf("failed to run go tool cover -func: %v", err)
		}
		if err := os.WriteFile(*functionsOut, funcOut, 0o644); err != nil {
			fatalf("failed to write functions report: %v", err)
		}

		if _, _, err := runCommandWithExitCode(*goBin, "tool", "cover", "-html", *coverProfile, "-o", *coverageHTML); err != nil {
			fatalf("failed to run go tool cover -html: %v", err)
		}
	}

	// Propagate go test exit code to caller when tests fail.
	if testCode != 0 && !*allowTestFailure {
		os.Exit(testCode)
	}
}

func runCommandWithExitCode(name string, args ...string) ([]byte, int, error) {
	cmd := exec.Command(name, args...)
	out, err := cmd.CombinedOutput()
	if err == nil {
		return out, 0, nil
	}
	if exitErr, ok := err.(*exec.ExitError); ok {
		return out, exitErr.ExitCode(), nil
	}
	return out, 1, err
}

func fileExists(path string) bool {
	info, err := os.Stat(path)
	if err != nil {
		return false
	}
	return !info.IsDir()
}

func fatalf(format string, args ...interface{}) {
	fmt.Fprintf(os.Stderr, format+"\n", args...)
	os.Exit(1)
}
