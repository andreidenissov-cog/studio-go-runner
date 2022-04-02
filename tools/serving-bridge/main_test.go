// Copyright 2018-2020 (c) Cognizant Digital Business, Evolutionary AI. All rights reserved. Issued under the Apache 2.0 License.

package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	minio_local "github.com/andreidenissov-cog/go-service/pkg/minio"
	"github.com/go-stack/stack"
	"github.com/leaf-ai/studio-go-runner/internal/cuda"
	"github.com/leaf-ai/studio-go-runner/internal/defense"

	"github.com/jjeffery/kv" // MIT License
	"github.com/karlmutch/envflag"
)

var (
	parsedFlags = false

	TestStopC = make(chan bool)

	TestRunMain string

	// cleanupDirs is a list of working directories that need to be expunged when the test is finally all over
	// within this package
	cleanupDirs = []string{}

	// InitError is used to track an failures occurring during static initialization
	InitError kv.Error

	// TestOptions are externally visible symbols that this package is asking the unit test suite to pickup and use
	// when the testing is managed by an external entity, this allows build level variations that include or
	// exclude GPUs for example to run their tests appropriately.  It also allows the top level build logic
	// to inspect source code for executables and run their testing without knowledge of how they work.
	DuatTestOptions = [][]string{}

	// TestCfgListeners Is used by tests to obtain the latest dynamic configuration for the server
	TestCfgListeners *Listeners = nil

	topDir = flag.String("top-dir", "../..", "The location of the top level source directory for locating test files")

	// Information regarding the test minio instance that will have been initialized for testing
	minioTest *minio_local.MinioTestServer
)

// When the runner tests are done we need to build the scenarios we want tested
// and their command line options for each case
func init() {
	cleanupDirs = append(cleanupDirs, "/tmp/cache-runner")

	// Disable certain checks related to ECC validation for smaller cards that are used during testing
	cuda.CudaInTest = true

}

func cleanup() {
	for _, tmpDir := range cleanupDirs {
		os.RemoveAll(tmpDir)
	}

	// Allow the enclave for secrets to wipe things
	defense.StopSecret()
}

// TestRunMain can be used to run the server in production mode as opposed to
// funit or unit testing mode.  Traditionally gathering coverage data and running
// in production are done separately.  This unit test allows the runner to do
// both at the same time.  To do this a test binary is generated using the command
//
// cd $(GOROOT)/src/github.com/leaf-ai/studio-go-runner
// go test -coverpkg="." -c -o bin/serving-bridge tools/serving-bridge/*.go
//
// Then the resulting /bin/serving-bridge-coverage binary is run as through it were a traditional
// server binary for the daemon using the command below.  The difference being that the
// binary now has coverage instrumentation.  In order to collect the coverage run any production
// workload and use cases you need then CTRL-C the server.
//
// ./bin/serving-bridge-coverage -test.run "^TestRunMain$" -test.coverprofile=system.out
//
// As an additional feature coverage files have is that they can also be merged using
// commands similar to the following:
//
// $ go get github.com/wadey/gocovmerge
// $ gocovmerge unit.out system.out > all.out
// $ go tool cover -html all.out
//
// Using the coverage merge tool testing done using a fully deployed system with
// real projects, proxies, projects, and workloads along with integration testing can be merged
// together from different test steps in an integration and test pipeline.
//

// TestMain is invoked by the GoLang entry point for the runtime of compiled GoLang
// programs when the compiled and linked image has been run using the 'go test'
// command
//
// This function will invoke the applications entry point to initiate the normal execution flow
// of the server with the tests remaining under the scheduling control of the
// GoLang test runtime. For more information please read https://golang.org/pkg/testing/
//
func TestMain(m *testing.M) {

	defer cleanup()

	TestMode = true

	if InitError != nil {
		fmt.Fprintln(os.Stderr, InitError)
	}
	// Only perform this Parsed check inside the test framework. Do not be tempted
	// to do this in the main of our production package
	//
	if !flag.Parsed() {
		envflag.Parse()
	}
	parsedFlags = true

	// Make sure that any test files can be found via a valid topDir argument on the CLI
	if stat, err := os.Stat(*topDir); os.IsNotExist(err) {
		fmt.Println(kv.Wrap(err).With("top-dir", *topDir).With("stack", stack.Trace().TrimRuntime()))
		os.Exit(-1)
	} else {
		if !stat.Mode().IsDir() {
			fmt.Println(kv.NewError("not a directory").With("top-dir", *topDir).With("stack", stack.Trace().TrimRuntime()))
			os.Exit(-1)
		}

	}
	if dir, err := filepath.Abs(*topDir); err != nil {
		fmt.Println((kv.Wrap(err).With("top-dir", *topDir).With("stack", stack.Trace().TrimRuntime())))
	} else {
		flag.Set("top-dir", dir)
	}

	// Initialize a top level context for the entire server
	ctx, cancel := context.WithCancel(context.Background())

	// Setup a ready channel that the server will close when it is
	// in a state where all services are dispatched, they might not be
	// fully available but will exist and be processing
	readyC := make(chan *Listeners)

	resultCode := -1
	errC := make(chan kv.Error, 1)

	{
		// Start the server under test
		go func() {
			logger.Info("starting server")
			if errs := EntryPoint(ctx, readyC); len(errs) != 0 {
				for _, err := range errs {
					logger.Error(err.Error())
				}
				logger.Fatal("test setup failed, aborting all testing")
			}
			<-ctx.Done()
			// When using benchmarking in production mode, that is no tests running the
			// user can park the server on a single unit test that only completes when this
			// channel is close, which happens only when there is a quitCtx from the application
			// due to a CTRL-C key sequence or kill -n command
			//
			// If the test was not selected for by the tester then this will be essentially a
			// NOP
			//
			close(TestStopC)

			logger.Info("forcing test mode server down")
			func() {
				defer func() {
					recover()
				}()
				cancel()
			}()

		}()

		// The initialization is done inline so that we know the test S3 server is
		// running prior to any testing starting
		logger.Info("starting, or discovering interfaces for minio (S3).")
		minioTest, errC = minio_local.InitTestingMinio(ctx, *debugOpt)

		go func() {

			// Wait for any kv.from the S3 server and log them, continuing until
			// the testing stops
			for {
				select {
				case err := <-errC:
					if err != nil {
						logger.Error(err.Error())
					}
				case <-ctx.Done():
					return
				}
			}
		}()

		// Now wait for the main server to signal it has begun the internal components
		listeners := <-readyC
		if listeners != nil {
			TestCfgListeners = listeners
		} else {
			logger.Fatal("dynamic configuration channel not available")
		}

		// Check that the S3 test server has been started and we can contact it before proceeding
		alive, err := WaitForMinioTest(context.Background(), minioTest, TestCfgListeners)

		if err != nil {
			logger.Fatal("Minio wait failure", "error", err.Error(), "stack", stack.Trace().TrimRuntime())
		}

		if !alive {
			logger.Fatal("Minio not available", "stack", stack.Trace().TrimRuntime())
		}

		// Now generate a temporary dir and stubbed TFX server config file. Then update the
		// configuration of the server with the location
		if err = SetupTfxCfgTest(context.Background(), TestCfgListeners, logger); err != nil {
			logger.Fatal("TFX Configuration setup failure", "error", err.Error(), "stack", stack.Trace().TrimRuntime())
		}

		// If there are any tests to be done we now start them
		if len(TestRunMain) != 0 {
			<-TestStopC
		} else {
			if resultCode = m.Run(); resultCode != 0 {
				logger.Fatal("TFX Configuration run failure", "result", resultCode, "stack", stack.Trace().TrimRuntime())
			}

			cancel()
		}
	}

	logger.Info("waiting for server down to complete")

	// Wait until the main server is shutdown
	<-ctx.Done()

	// Have a grace period before exiting entirely
	time.Sleep(2 * time.Second)

	if resultCode != 0 {
		os.Exit(resultCode)
	}
}
