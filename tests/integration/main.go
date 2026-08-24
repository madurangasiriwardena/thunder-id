// Copyright 2025 The ThunderID Authors
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strconv"
	"time"

	"github.com/thunder-id/thunderid/tests/integration/testutils"
)

const (
	serverPort = "8095"
)

var (
	zipFilePattern string
	testRun        string
	testPackage    string
)

func main() {
	parseFlags()
	initTests()

	// Step 1: Unzip the product
	err := testutils.UnzipProduct()
	if err != nil {
		fmt.Printf("Failed to unzip product: %v\n", err)
		os.Exit(1)
	}

	// Step 2: Replace the resource files in the unzipped directory.
	err = testutils.ReplaceResources(zipFilePattern)
	if err != nil {
		fmt.Printf("Failed to replace resources: %v\n", err)
		os.Exit(1)
	}

	// Step 3: Copy declarative resource fixtures for composite mode testing
	err = testutils.CopyDeclarativeResources(zipFilePattern)
	if err != nil {
		fmt.Printf("Failed to copy declarative resources: %v\n", err)
		os.Exit(1)
	}

	// Step 4: Run the init script to create the SQLite database
	err = testutils.RunInitScript(zipFilePattern)
	if err != nil {
		fmt.Printf("Failed to run init script: %v\n", err)
		os.Exit(1)
	}

	// Step 5: Run setup.sh
	// This starts server without security, runs bootstrap scripts, and stops server
	fmt.Println("Running bootstrap scripts...")
	err = testutils.RunSetupScript()
	if err != nil {
		fmt.Printf("Failed to run setup script: %v\n", err)
		os.Exit(1)
	}

	// Step 6: Start server
	fmt.Println("Starting server with security enabled...")
	err = testutils.StartServer(serverPort, zipFilePattern)
	if err != nil {
		fmt.Printf("Failed to start server: %v\n", err)
		os.Exit(1)
	}
	defer testutils.StopServer()

	// Wait for the server to start
	fmt.Println("Waiting for the server to start...")
	time.Sleep(5 * time.Second)

	// Step 7: Obtain admin access token once for all test packages
	fmt.Println("Obtaining admin access token...")
	err = testutils.ObtainAdminAccessToken()
	if err != nil {
		fmt.Printf("Failed to obtain admin access token: %v\n", err)
		testutils.StopServer()
		os.Exit(1)
	}

	// Step 8: Run all tests
	err = runTests()
	if err != nil {
		fmt.Printf("there are test failures: %v\n", err)
		testutils.StopServer()
		os.Exit(1)
	}

	fmt.Println("All tests completed successfully!")
}

func parseFlags() {
	flag.StringVar(&testRun, "run", "", "Run only tests matching the regular expression (passed to go test -run)")
	flag.StringVar(&testPackage, "package", "./...", "Package(s) to test (default: ./...)")
	flag.Parse()
}

func initTests() {
	// Read database type from environment variable
	dbType := os.Getenv("DB_TYPE")
	if dbType == "" {
		dbType = "sqlite" // Default to SQLite
	}
	fmt.Printf("Database type: %s\n", dbType)

	zipFilePattern = testutils.GetZipFilePattern()
	if zipFilePattern == "" {
		fmt.Println("Failed to determine the zip file pattern.")
		os.Exit(1)
	}

	// Initialize test context with the detected configuration
	testutils.InitializeTestContext(serverPort, zipFilePattern, dbType)

	fmt.Printf("Using zip file pattern: %s\n", zipFilePattern)
}

func runTests() error {
	// Clean the test cache to avoid getting results from previous runs.
	// This is important to avoid false positives in test results as the
	// server and integration test suite are two separate applications.
	cmd := exec.Command("go", "clean", "-testcache")
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	err := cmd.Run()
	if err != nil {
		return fmt.Errorf("failed to clean test cache: %w", err)
	}

	// Determine command and build args
	_, err = exec.LookPath("gotestsum")
	useGotestsum := err == nil

	var cmdName string
	var args []string

	// TEST_JSON_OUT asks for the machine-readable `go test -json` event stream to be
	// written alongside the human output, so a caller can report per-test results
	// (passed, failed, skipped) instead of inferring them from source. Off unless set.
	jsonOut := os.Getenv("TEST_JSON_OUT")

	if useGotestsum {
		fmt.Println("Running integration tests using gotestsum...")
		cmdName = "gotestsum"
		if jsonOut != "" {
			args = append(args, "--jsonfile", jsonOut)
		}
		args = append(args, "--format", "testname", "--", "-p=1")
	} else {
		fmt.Println("Running integration tests using go test...")
		cmdName = "go"
		args = append(args, "test", "-p=1")
		if jsonOut != "" {
			args = append(args, "-json")
		} else {
			args = append(args, "-v")
		}
	}

	// Add test filters if provided
	if testRun != "" {
		args = append(args, "-run", testRun)
		fmt.Printf("Test filter: -run %s\n", testRun)
	}
	args = append(args, testPackage)
	fmt.Printf("Test package: %s\n", testPackage)

	cmd = exec.Command(cmdName, args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	// gotestsum writes the event stream itself; plain `go test -json` puts it on
	// stdout, so tee it to the file while still streaming to the console.
	if jsonOut != "" && !useGotestsum {
		f, ferr := os.Create(jsonOut)
		if ferr != nil {
			return fmt.Errorf("failed to create %s: %w", jsonOut, ferr)
		}
		defer f.Close()
		cmd.Stdout = io.MultiWriter(os.Stdout, f)
	}

	// Export test-context state so that test subprocesses can self-initialize
	// via ensureInitialized() without requiring an explicit InitializeTestContext call.
	cmd.Env = append(os.Environ(),
		"SERVER_EXTRACTED_HOME="+testutils.GetExtractedProductHome(),
		"SERVER_PORT="+serverPort,
		"ZIP_PATTERN="+zipFilePattern,
		"SERVER_PID="+strconv.Itoa(testutils.GetServerPID()),
	)

	return cmd.Run()
}
