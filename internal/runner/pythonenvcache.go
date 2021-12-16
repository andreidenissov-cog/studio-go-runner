// Copyright 2018-2020 (c) Cognizant Digital Business, Evolutionary AI. All rights reserved. Issued under the Apache 2.0 License.

package runner

// This file contains the implementation of the python based virtualenv
// runtime for studioML workloads

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"fmt"
	"io/ioutil"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"text/template"
	"time"

	"github.com/leaf-ai/go-service/pkg/network"

	runnerReports "github.com/leaf-ai/studio-go-runner/internal/gen/dev.cognizant_dev.ai/genproto/studio-go-runner/reports/v1"
	"github.com/leaf-ai/studio-go-runner/internal/request"
	"github.com/leaf-ai/studio-go-runner/internal/resources"

	"github.com/go-stack/stack"
	"github.com/jjeffery/kv" // MIT License
)

var (
	virtEnvCache VirtualEnvCache
)

type VirtualEnvEntry struct {
	uniqueID  string
}

type VirtualEnvCache struct {
	entries map[string] VirtualEnvEntry
	sync.Mutex
}

func init() {
	virtEnvCache = VirtualEnvCache{
		entries: map[string]VirtualEnvEntry{},
	}
}

func (cache *VirtualEnvCache) getEntry(rqst *request.Request, alloc *resources.Allocated) (entry *VirtualEnvEntry, err kv.Error) {

}

func (cache *VirtualEnvCache) generateScript(pythonVer string, general []string, configured []string,
	                                         envName string, scriptPath string) (err kv.Error) {

	params := struct {
		PythonVer string
		EnvName   string
		Pips      []string
		CfgPips   []string
	}{
		PythonVer: pythonVer,
		EnvName:   envName,
		Pips:      general,
		CfgPips:   configured,
	}

	// Create a shell script that will do everything needed
	// to create required virtual python environment
	tmpl, errGo := template.New("virtEnvCreator").Parse(
		`#!/bin/bash -x
sleep 2
# Credit https://github.com/fernandoacorreia/azure-docker-registry/blob/master/tools/scripts/create-registry-server
function fail {
  echo $1 >&2
  exit 1
}

trap 'fail "The execution was aborted because a command exited with an error status code."' ERR

function retry {
  local n=0
  local max=3
  local delay=10
  while true; do
    "$@" && break || {
      if [[ $n -lt $max ]]; then
        ((n++))
        echo "Command failed. Attempt $n/$max:"
        sleep $delay;
      else
        fail "The command has failed after $n attempts."
      fi
    }
  done
}

set -v
date
date -u
export LC_ALL=en_US.utf8
locale
hostname
set -e
export PATH=/runner/.pyenv/bin:$PATH
export PYENV_VERSION={{.PythonVer}}
IFS=$'\n'; arr=( $(pyenv versions --bare | grep -v studioml || true) )
for i in ${arr[@]} ; do
    if [[ "$i" == ${PYENV_VERSION}* ]]; then
		export PYENV_VERSION=$i
		echo $PYENV_VERSION
	fi
done
eval "$(pyenv init --path)"
eval "$(pyenv init -)"
eval "$(pyenv virtualenv-init -)"
pyenv doctor
pyenv virtualenv-delete -f {{.EnvName}} || true
pyenv virtualenv $PYENV_VERSION {{.EnvName}}
pyenv activate {{.EnvName}}
set +e
retry python3 -m pip install "pip==20.1" "setuptools==44.0.0" "wheel==0.35.1"
python3 -m pip freeze --all
{{if .Pips}}
echo "installing project pip {{ .Pips }}"
retry python3 -m pip install {{range .Pips }} {{.}}{{end}}
{{end}}
echo "finished installing project pips"
retry python3 -m pip install pyopenssl==20.0.1 pipdeptree==2.0.0
{{if .CfgPips}}
echo "installing cfg pips"
retry python3 -m pip install {{range .CfgPips}} {{.}}{{end}}
echo "finished installing cfg pips"
{{end}}
set -e
python3 -m pip freeze
python3 -m pip -V
set -x
cd -
locale
pyenv deactivate || true
date
date -u
exit 0
`)

	if errGo != nil {
		return kv.Wrap(errGo).With("stack", stack.Trace().TrimRuntime())
	}

	content := new(bytes.Buffer)
	if errGo = tmpl.Execute(content, params); errGo != nil {
		return kv.Wrap(errGo).With("stack", stack.Trace().TrimRuntime())
	}

	if errGo = ioutil.WriteFile(scriptPath, content.Bytes(), 0700); errGo != nil {
		return kv.Wrap(errGo).With("stack", stack.Trace().TrimRuntime()).With("script", scriptPath)
	}
	return nil
}

func (cache *VirtualEnvCache) createVirtualEnv(ctx context.Context, dir string, output *os.File,
	responseQ chan<- *runnerReports.Report, venvID string) (err kv.Error) {

}

// NewVirtualEnv builds the VirtualEnv data structure from data received across the wire
// from a studioml client.
//
func NewVirtualEnv(rqst *request.Request, dir string, uniqueID string, responseQ chan<- *runnerReports.Report) (env *VirtualEnv, err kv.Error) {

	if errGo := os.MkdirAll(filepath.Join(dir, "_runner"), 0700); errGo != nil {
		return nil, kv.Wrap(errGo).With("stack", stack.Trace().TrimRuntime())
	}

	return &VirtualEnv{
		Request:   rqst,
		Script:    filepath.Join(dir, "_runner", "runner.sh"),
		uniqueID:  uniqueID,
		ResponseQ: responseQ,
	}, nil
}

// pythonModules is used to scan the pip installables
//
func pythonModules(rqst *request.Request, alloc *resources.Allocated) (general []string, configured []string, tfVer string) {
	hasGPU := len(alloc.GPU) != 0
	gpuSeen := false

	general, tfVer, gpuSeen = scanPythonModules(rqst.Experiment.Pythonenv, hasGPU, gpuSeen, "general")
	configured, tfVer, gpuSeen = scanPythonModules(rqst.Config.Pip, hasGPU, gpuSeen, "configured")

	return general, configured, tfVer
}

func scanPythonModules(pipList []string, hasGPU bool, gpuSeen bool, name string) (result []string, tfVersion string, sawGPU bool) {
	result = []string{}
	sawGPU = gpuSeen
	for _, pkg := range pipList {
		// https://bugs.launchpad.net/ubuntu/+source/python-pip/+bug/1635463
		//
		// Groom out bogus package from ubuntu
		if strings.HasPrefix(pkg, "pkg-resources") {
			continue
		}
		if strings.HasPrefix(pkg, "tensorflow_gpu") {
			sawGPU = true
		}

		if hasGPU && !sawGPU {
			if strings.HasPrefix(pkg, "tensorflow==") || pkg == "tensorflow" {
				spec := strings.Split(pkg, "==")

				if len(spec) < 2 {
					pkg = "tensorflow_gpu"
				} else {
					pkg = "tensorflow_gpu==" + spec[1]
					tfVersion = spec[1]
				}
				fmt.Printf("modified tensorflow in %s %+v \n", name, pkg)
			}
		}
		result = append(result, pkg)
	}
	return result, tfVersion, sawGPU
}

func getHashPythonEnv(pythonVer string, general []string, configured []string) string {
	return "xxx"
}

// gpuEnv is used to pull out of the allocated GPU roster the needed environment variables for running
// the python environment
func gpuEnv(alloc *resources.Allocated) (envs []string) {
	if len(alloc.GPU) != 0 {
		gpuSettings := map[string][]string{}
		for _, resource := range alloc.GPU {
			for k, v := range resource.Env {
				if k == "CUDA_VISIBLE_DEVICES" || k == "NVIDIA_VISIBLE_DEVICES" {
					if setting, isPresent := gpuSettings[k]; isPresent {
						gpuSettings[k] = append(setting, v)
					} else {
						gpuSettings[k] = []string{v}
					}
				} else {
					envs = append(envs, k+"="+v)
				}
			}
		}
		for k, v := range gpuSettings {
			envs = append(envs, k+"="+strings.Join(v, ","))
		}
	} else {
		// Force CUDA GPUs offline manually rather than leaving this undefined
		envs = append(envs, "CUDA_VISIBLE_DEVICES=\"-1\"")
		envs = append(envs, "NVIDIA_VISIBLE_DEVICES=\"-1\"")
	}
	return envs
}

// Run will use a generated script file and will run it to completion while marshalling
// results and files from the computation.  Run is a blocking call and will only return
// upon completion or termination of the process it starts.  Run is called by the processor
// runScript receiver.
//
func (p *VirtualEnv) Run(ctx context.Context, refresh map[string]request.Artifact) (err kv.Error) {

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

	// Create a new TMPDIR because the python pip tends to leave dirt behind
	// when doing pip builds etc
	tmpDir, errGo := ioutil.TempDir("", p.Request.Experiment.Key)
	if errGo != nil {
		return kv.Wrap(errGo).With("experimentKey", p.Request.Experiment.Key).With("stack", stack.Trace().TrimRuntime())
	}
	defer os.RemoveAll(tmpDir)

	// Move to starting the process that we will monitor with the experiment running within
	// it

	// #nosec
	cmd := exec.CommandContext(stopCmd, "/bin/bash", "-c", "export TMPDIR="+tmpDir+"; "+filepath.Clean(p.Script))
	cmd.Dir = path.Dir(p.Script)

	// Pipes are used to allow the output to be tracked interactively from the cmd
	stdout, errGo := cmd.StdoutPipe()
	if errGo != nil {
		return kv.Wrap(errGo).With("stack", stack.Trace().TrimRuntime())
	}
	stderr, errGo := cmd.StderrPipe()
	if errGo != nil {
		return kv.Wrap(errGo).With("stack", stack.Trace().TrimRuntime())
	}

	outC := make(chan []byte)
	defer close(outC)
	errC := make(chan string)
	defer close(errC)

	// Prepare an output file into which the command line stdout and stderr will be written
	outputFN := filepath.Join(cmd.Dir, "..", "output")
	if errGo := os.Mkdir(outputFN, 0600); errGo != nil {
		perr, ok := errGo.(*os.PathError)

		if ok {
			if !errors.Is(perr.Err, os.ErrExist) {
				return kv.Wrap(errGo).With("stack", stack.Trace().TrimRuntime())
			}
		} else {
			return kv.Wrap(errGo).With("stack", stack.Trace().TrimRuntime())
		}
	}
	outputFN = filepath.Join(outputFN, "output")
	f, errGo := os.Create(outputFN)
	if errGo != nil {
		return kv.Wrap(errGo).With("stack", stack.Trace().TrimRuntime())
	}

	// A quit channel is used to allow fine grained control over when the IO
	// copy and output task should be created
	stopOutput := make(chan struct{}, 1)

	// Being the go routine that takes cmd output and appends it to a file on disk
	go procOutput(stopOutput, f, outC, errC)

	// Start begins the processing asynchronously, the procOutput above will collect the
	// run results are they are output asynchronously
	if errGo = cmd.Start(); errGo != nil {
		return kv.Wrap(errGo).With("stack", stack.Trace().TrimRuntime())
	}

	// Protect the err value when running multiple goroutines
	errCheck := sync.Mutex{}

	// This code connects the pipes being used by the golang exec command process to the channels that
	// will be used to bring the output into a single file
	waitOnIO := sync.WaitGroup{}
	waitOnIO.Add(2)

	go func() {
		defer waitOnIO.Done()

		time.Sleep(time.Second)

		responseLine := strings.Builder{}
		s := bufio.NewScanner(stdout)
		s.Split(bufio.ScanRunes)
		for s.Scan() {
			out := s.Bytes()
			outC <- out
			if bytes.Compare(out, []byte{'\n'}) == 0 {
				responseLine.Write(out)
			} else {
				if p.ResponseQ != nil && responseLine.Len() != 0 {
					select {
					case p.ResponseQ <- &runnerReports.Report{
						Time: timestamppb.Now(),
						ExecutorId: &wrappers.StringValue{
							Value: network.GetHostName(),
						},
						UniqueId: &wrappers.StringValue{
							Value: p.uniqueID,
						},
						ExperimentId: &wrappers.StringValue{
							Value: p.Request.Experiment.Key,
						},
						Payload: &runnerReports.Report_Logging{
							Logging: &runnerReports.LogEntry{
								Time:     timestamppb.Now(),
								Severity: runnerReports.LogSeverity_Info,
								Message: &wrappers.StringValue{
									Value: responseLine.String(),
								},
								Fields: map[string]string{},
							},
						},
					}:
						responseLine.Reset()
					default:
						// Dont respond to back pressure
					}
				}
			}
		}
		if errGo := s.Err(); errGo != nil {
			errCheck.Lock()
			defer errCheck.Unlock()
			if err != nil && err != os.ErrClosed {
				err = kv.Wrap(errGo).With("stack", stack.Trace().TrimRuntime())
			}
		}
	}()

	go func() {
		defer waitOnIO.Done()

		time.Sleep(time.Second)
		s := bufio.NewScanner(stderr)
		s.Split(bufio.ScanLines)
		for s.Scan() {
			out := s.Text()
			errC <- out
			if p.ResponseQ != nil {
				select {
				case p.ResponseQ <- &runnerReports.Report{
					Time: timestamppb.Now(),
					ExecutorId: &wrappers.StringValue{
						Value: network.GetHostName(),
					},
					UniqueId: &wrappers.StringValue{
						Value: p.uniqueID,
					},
					ExperimentId: &wrappers.StringValue{
						Value: p.Request.Experiment.Key,
					},
					Payload: &runnerReports.Report_Logging{
						Logging: &runnerReports.LogEntry{
							Time:     timestamppb.Now(),
							Severity: runnerReports.LogSeverity_Error,
							Message: &wrappers.StringValue{
								Value: string(out),
							},
							Fields: map[string]string{},
						},
					},
				}:
				default:
					// Dont respond to back preassure
				}
			}
		}
		if errGo := s.Err(); errGo != nil {
			errCheck.Lock()
			defer errCheck.Unlock()
			if err != nil && err != os.ErrClosed {
				err = kv.Wrap(errGo).With("stack", stack.Trace().TrimRuntime())
			}
		}
	}()

	// Wait for the IO to stop before continuing to tell the background
	// writer to terminate. This means the IO for the process will
	// be able to send on the channels until they have stopped.
	waitOnIO.Wait()

	// Now manually stop the process output copy goroutine once the exec package
	// has finished
	close(stopOutput)

	// Wait for the process to exit, and store any error code if possible
	// before we continue to wait on the processes output devices finishing
	if errGo = cmd.Wait(); errGo != nil {
		errCheck.Lock()
		if err == nil {
			err = kv.Wrap(errGo).With("loc", "cmd.Wait()").With("stack", stack.Trace().TrimRuntime())
		}
		errCheck.Unlock()
	}

	errCheck.Lock()
	if err == nil && stopCmd.Err() != nil {
		err = kv.Wrap(stopCmd.Err()).With("loc", "stopCmd").With("stack", stack.Trace().TrimRuntime())
	}
	errCheck.Unlock()

	return err
}

