// Copyright 2018-2021 (c) Cognizant Digital Business, Evolutionary AI. All rights reserved. Issued under the Apache 2.0 License.

package runner

import (
	"bufio"
	"context"
	"io"
	"io/ioutil"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"sync"
	"time"

	"github.com/go-stack/stack"
	"github.com/jjeffery/kv" // MIT License
	runnerReports "github.com/leaf-ai/studio-go-runner/internal/gen/dev.cognizant_dev.ai/genproto/studio-go-runner/reports/v1"
)

func procOutput(stopWriter chan struct{}, f *os.File, outC chan string, errC chan string) {

	defer func() {
		f.Close()
	}()

	for {
		select {
		case <-stopWriter:
			return
		case errLine := <-errC:
			if len(errLine) != 0 {
				f.WriteString(errLine + "\n")
			}
		case outLine := <-outC:
			if len(outLine) != 0 {
				f.WriteString(outLine + "\n")
			}
		}
	}
}

func readToChan(input io.ReadCloser, output chan string, waitOnIO *sync.WaitGroup, result *error) {
	defer waitOnIO.Done()

	time.Sleep(time.Second)
	s := bufio.NewScanner(input)
	s.Split(bufio.ScanLines)
	for s.Scan() {
		out := s.Text()
		output <- out
	}
	*result = s.Err()
}

// Run will use a generated script file and will run it to completion while marshalling
// results and files from the computation.  Run is a blocking call and will only return
// upon completion or termination of the process it starts.
//
func RunScript(ctx context.Context, scriptPath string, output *os.File,
	responseQ chan<- *runnerReports.Report, runKey string, runID string) (err kv.Error) {

	stopCmd, stopCmdCancel := context.WithCancel(context.Background())
	// defers are stacked in LIFO order so cancelling this context is the last
	// thing this function will do, also cancelling the stopCmd will also travel down
	// the context hierarchy cancelling everything else
	defer stopCmdCancel()

	// Cancel our own internal context when the outer context is cancelled
	go func() {
		select {
		case <-stopCmd.Done():
		case <-ctx.Done():
		}
		stopCmdCancel()
	}()

	// Create a new TMPDIR because the script python pip tends to leave dirt behind
	// when doing pip builds etc
	tmpDir, errGo := ioutil.TempDir("", runKey)
	if errGo != nil {
		return kv.Wrap(errGo).With("experimentKey", runKey).With("stack", stack.Trace().TrimRuntime())
	}
	defer os.RemoveAll(tmpDir)

	// Move to starting the process that we will monitor
	// #nosec
	cmd := exec.CommandContext(stopCmd, "/bin/bash", "-c", "export TMPDIR="+tmpDir+"; "+filepath.Clean(scriptPath))
	cmd.Dir = path.Dir(scriptPath)

	outBytes, errGo := cmd.CombinedOutput()
	if errGo == nil {
		output.Write(outBytes)
	} else {
		if err == nil {
			err = kv.Wrap(errGo).With("loc", "cmd.Wait()").With("stack", stack.Trace().TrimRuntime())
		}
	}

	return err
}
