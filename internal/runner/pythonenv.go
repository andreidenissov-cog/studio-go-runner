// Copyright 2018-2020 (c) Cognizant Digital Business, Evolutionary AI. All rights reserved. Issued under the Apache 2.0 License.

package runner

// This file contains the implementation of the python based virtualenv
// runtime for studioML workloads

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"io/ioutil"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"text/template"
	"time"

	"github.com/go-stack/stack"
	"github.com/jjeffery/kv" // MIT License
)

var (
	hostname string
)

func init() {
	hostname, _ = os.Hostname()
}

// VirtualEnv encapsulated the context that a python virtual environment is to be
// instantiated from including items such as the list of pip installables that should
// be loaded and shell script to run.
//
type VirtualEnv struct {
	Request *Request
	Script  string
}

// NewVirtualEnv builds the VirtualEnv data structure from data received across the wire
// from a studioml client.
//
func NewVirtualEnv(rqst *Request, dir string) (env *VirtualEnv, err kv.Error) {

	if errGo := os.MkdirAll(filepath.Join(dir, "_runner"), 0700); errGo != nil {
		return nil, kv.Wrap(errGo).With("stack", stack.Trace().TrimRuntime())
	}

	return &VirtualEnv{
		Request: rqst,
		Script:  filepath.Join(dir, "_runner", "runner.sh"),
	}, nil
}

// pythonModules is used to scan the pip installables and to groom them based upon a
// local distribution of studioML also being included inside the workspace
//
func pythonModules(rqst *Request, alloc *Allocated) (general []string, configured []string, studioML string, tfVer string) {

	hasGPU := len(alloc.GPU) != 0

	general = []string{}

	gpuSeen := false
	for _, pkg := range rqst.Experiment.Pythonenv {
		if strings.HasPrefix(pkg, "studioml==") {
			studioML = pkg
			continue
		}
		// https://bugs.launchpad.net/ubuntu/+source/python-pip/+bug/1635463
		//
		// Groom out bogus package from ubuntu
		if strings.HasPrefix(pkg, "pkg-resources") {
			continue
		}
		if strings.HasPrefix(pkg, "tensorflow_gpu") {
			gpuSeen = true
		}

		if hasGPU && !gpuSeen {
			if strings.HasPrefix(pkg, "tensorflow==") || pkg == "tensorflow" {
				spec := strings.Split(pkg, "==")

				if len(spec) < 2 {
					pkg = "tensorflow_gpu"
				} else {
					pkg = "tensorflow_gpu==" + spec[1]
					tfVer = spec[1]
				}
				fmt.Printf("modified tensorflow in general %+v \n", pkg)
			}
		}
		general = append(general, pkg)
	}

	configured = []string{}
	for _, pkg := range rqst.Config.Pip {
		if strings.HasPrefix(pkg, "studioml==") {
			studioML = pkg
			continue
		}
		if strings.HasPrefix(pkg, "pkg-resources") {
			continue
		}
		if strings.HasPrefix(pkg, "tensorflow_gpu") {
			gpuSeen = true
		}
		if hasGPU && !gpuSeen {
			if strings.HasPrefix(pkg, "tensorflow==") || pkg == "tensorflow" {
				spec := strings.Split(pkg, "==")

				if len(spec) < 2 {
					pkg = "tensorflow_gpu"
				} else {
					pkg = "tensorflow_gpu==" + spec[1]
					tfVer = spec[1]
				}
				fmt.Printf("modified tensorflow in configured %+v \n", pkg)
			}
		}
		configured = append(configured, pkg)
	}

	return general, configured, studioML, tfVer
}

// Make is used to write a script file that is generated for the specific TF tasks studioml has sent
// to retrieve any python packages etc then to run the task
//
func (p *VirtualEnv) Make(alloc *Allocated, e interface{}) (err kv.Error) {

	pips, cfgPips, studioPIP, tfVer := pythonModules(p.Request, alloc)

	// The tensorflow versions 1.5.x and above all support cuda 9 and 1.4.x is cuda 8,
	// c.f. https://www.tensorflow.org/install/install_sources#tested_source_configurations.
	// Insert the appropriate version explicitly into the LD_LIBRARY_PATH before other paths
	cudaDir := "/usr/local/cuda-10.0/lib64"
	if strings.HasPrefix(tfVer, "1.4") {
		cudaDir = "/usr/local/cuda-8.0/lib64"
	}

	// If the studioPIP was specified but we have a dist directory then we need to clear the
	// studioPIP, otherwise leave it there
	pth, errGo := filepath.Abs(filepath.Join(path.Dir(p.Script), "..", "workspace", "dist", "studioml-*.tar.gz"))
	if errGo != nil {
		return kv.Wrap(errGo).With("stack", stack.Trace().TrimRuntime()).With("path", pth)
	}
	matches, errGo := filepath.Glob(pth)
	if errGo != nil {
		return kv.Wrap(errGo).With("stack", stack.Trace().TrimRuntime()).With("path", pth)
	}
	if len(matches) != 0 {
		// Extract the most recent version of studioML from the dist directory
		sort.Strings(matches)
		studioPIP = matches[len(matches)-1]
	}

	params := struct {
		AllocEnv  []string
		E         interface{}
		Pips      []string
		CfgPips   []string
		StudioPIP string
		CudaDir   string
		Hostname  string
	}{
		AllocEnv:  []string{},
		E:         e,
		Pips:      pips,
		CfgPips:   cfgPips,
		StudioPIP: studioPIP,
		CudaDir:   cudaDir,
		Hostname:  hostname,
	}

	if alloc.GPU != nil {
		for _, resource := range alloc.GPU {
			for k, v := range resource.Env {
				params.AllocEnv = append(params.AllocEnv, k+"="+v)
			}
		}
	}

	// Create a shell script that will do everything needed to run
	// the python environment in a virtual env
	tmpl, errGo := template.New("pythonRunner").Parse(
		`#!/bin/bash -x
sleep 2
# Credit https://github.com/fernandoacorreia/azure-docker-registry/blob/master/tools/scripts/create-registry-server
function fail {
  echo $1 >&2
  exit 1
}

trap 'fail "The execution was aborted because a command exited with an error status code."' ERR

function retry {
  local n=1
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
export LD_LIBRARY_PATH={{.CudaDir}}:$LD_LIBRARY_PATH:/usr/local/cuda/lib64/:/usr/lib/x86_64-linux-gnu:/lib/x86_64-linux-gnu/
mkdir -p {{.E.RootDir}}/blob-cache
mkdir -p {{.E.RootDir}}/queue
mkdir -p {{.E.RootDir}}/artifact-mappings
mkdir -p {{.E.RootDir}}/artifact-mappings/{{.E.Request.Experiment.Key}}
export PATH=/root/.pyenv/bin:$PATH
export PYENV_VERSION={{.E.Request.Experiment.PythonVer}}
IFS=$'\n'; arr=( $(pyenv versions --bare | grep -v studioml) )
for i in ${arr[@]} ; do
    if [[ "$i" == ${PYENV_VERSION}* ]]; then
		export PYENV_VERSION=$i
		echo $PYENV_VERSION
	fi
done
python
eval "$(pyenv init -)"
eval "$(pyenv virtualenv-init -)"
pyenv doctor
pyenv virtualenv-delete -f studioml-{{.E.ExprSubDir}} || true
pyenv virtualenv $PYENV_VERSION studioml-{{.E.ExprSubDir}}
pyenv activate studioml-{{.E.ExprSubDir}}
set +e
retry python -m pip install "pip==19.0.3"
pip freeze --all
{{if .StudioPIP}}
retry python -m pip install -I {{.StudioPIP}}
{{end}}
{{if .Pips}}
{{range .Pips}}
echo "installing project pip {{.}}"
retry python -m pip install {{.}}
{{end}}
{{end}}
echo "finished installing project pips"
retry python -m pip install pyopenssl pipdeptree --upgrade
{{if .CfgPips}}
echo "installing cfg pips"
retry python -m pip install {{range .CfgPips}} {{.}}{{end}}
echo "finished installing cfg pips"
{{end}}
set -e
export STUDIOML_EXPERIMENT={{.E.ExprSubDir}}
export STUDIOML_HOME={{.E.RootDir}}
{{if .AllocEnv}}
{{range .AllocEnv}}
export {{.}}
{{end}}
{{end}}
export
cd {{.E.ExprDir}}/workspace
pip freeze
pip -V
set +x
set +e
echo "{\"studioml\": { \"experiment\" : {\"key\": \"{{.E.Request.Experiment.Key}}\", \"project\": \"{{.E.Request.Experiment.Project}}\"}}}" | jq -c '.'
{{range $key, $value := .E.Request.Experiment.Artifacts}}
echo "{\"studioml\": { \"artifacts\" : {\"{{$key}}\": \"{{$value.Qualified}}\"}}}" | jq -c '.'
{{end}}
echo "{\"studioml\": {\"start_time\": \"` + "`" + `date '+%FT%T.%N%:z'` + "`" + `\"}}" | jq -c '.'
echo "{\"studioml\": {\"host\": \"{{.Hostname}}\"}}" | jq -c '.'
set -x
set -e
python {{.E.Request.Experiment.Filename}} {{range .E.Request.Experiment.Args}}{{.}} {{end}}
result=$?
echo $result
set +e
echo "{\"studioml\": {\"stop_time\": \"` + "`" + `date '+%FT%T.%N%:z'` + "`" + `\"}}" | jq -c '.'
cd -
locale
pyenv deactivate || true
pyenv virtualenv-delete -f studioml-{{.E.ExprSubDir}} || true
date
date -u
exit $result
`)

	if errGo != nil {
		return kv.Wrap(errGo).With("stack", stack.Trace().TrimRuntime())
	}

	content := new(bytes.Buffer)
	if errGo = tmpl.Execute(content, params); errGo != nil {
		return kv.Wrap(errGo).With("stack", stack.Trace().TrimRuntime())
	}

	if errGo = ioutil.WriteFile(p.Script, content.Bytes(), 0700); errGo != nil {
		return kv.Wrap(errGo).With("stack", stack.Trace().TrimRuntime()).With("script", p.Script)
	}
	return nil
}

func procOutput(stopWriter context.Context, f *os.File, outC chan []byte, errC chan string) {

	outLine := []byte{}

	defer func() {
		if len(outLine) != 0 {
			f.WriteString(string(outLine))
		}
		f.Close()
	}()

	refresh := time.NewTicker(2 * time.Second)
	defer refresh.Stop()

	for {
		select {
		case <-refresh.C:
			if len(outLine) != 0 {
				f.WriteString(string(outLine))
				outLine = []byte{}
			}
		case <-stopWriter.Done():
			return
		case r := <-outC:
			if len(r) != 0 {
				outLine = append(outLine, r...)
				if !bytes.Contains([]byte{'\n'}, r) {
					continue
				}
			}
			if len(outLine) != 0 {
				f.WriteString(string(outLine))
				outLine = []byte{}
			}
		case errLine := <-errC:
			if len(errLine) != 0 {
				f.WriteString(errLine + "\n")
			}
		}
	}
}

// Run will use a generated script file and will run it to completion while marshalling
// results and files from the computation.  Run is a blocking call and will only return
// upon completion or termination of the process it starts
//
func (p *VirtualEnv) Run(ctx context.Context, refresh map[string]Artifact) (err kv.Error) {

	stopCopy, stopCopyCancel := context.WithCancel(ctx)
	// defers are stacked in LIFO order so cancelling this context is the last
	// thing this function will do, also cancelling the stopCopy will also travel down
	// the context heirarchy cancelling everything else
	defer stopCopyCancel()

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
	cmd := exec.CommandContext(stopCopy, "/bin/bash", "-c", "export TMPDIR="+tmpDir+"; "+filepath.Clean(p.Script))
	cmd.Dir = path.Dir(p.Script)

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

	outputFN := filepath.Join(cmd.Dir, "..", "output", "output")
	f, errGo := os.Create(outputFN)
	if errGo != nil {
		return kv.Wrap(errGo).With("stack", stack.Trace().TrimRuntime())
	}

	go procOutput(stopCopy, f, outC, errC)

	if errGo = cmd.Start(); errGo != nil {
		return kv.Wrap(errGo).With("stack", stack.Trace().TrimRuntime())
	}

	// Protect the err value when running multiple goroutines
	errCheck := sync.Mutex{}

	waitOnIO := sync.WaitGroup{}
	waitOnIO.Add(2)

	go func() {
		defer waitOnIO.Done()

		time.Sleep(time.Second)
		s := bufio.NewScanner(stdout)
		s.Split(bufio.ScanRunes)
		for s.Scan() {
			outC <- s.Bytes()
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
			errC <- s.Text()
		}
		if errGo := s.Err(); errGo != nil {
			errCheck.Lock()
			defer errCheck.Unlock()
			if err != nil && err != os.ErrClosed {
				err = kv.Wrap(errGo).With("stack", stack.Trace().TrimRuntime())
			}
		}
	}()

	// Wait for the process to exit, and store any error code if possible
	// before we continue to wait on the processes output devices finishing
	if errGo = cmd.Wait(); errGo != nil {
		errCheck.Lock()
		if err == nil {
			err = kv.Wrap(errGo).With("stack", stack.Trace().TrimRuntime())
		}
		errCheck.Unlock()
	}

	// Wait for the IO to stop before continuing to tell the background
	// writer to terminate. This means the IO for the process will
	// be able to send on the channels until they have stopped.
	waitOnIO.Wait()

	errCheck.Lock()
	if err == nil && stopCopy.Err() != nil {
		err = kv.Wrap(stopCopy.Err()).With("stack", stack.Trace().TrimRuntime())
	}
	errCheck.Unlock()

	return err
}

// Close is used to close any resources which the encapsulated VirtualEnv may have consumed.
//
func (ve *VirtualEnv) Close() (err kv.Error) {
	return nil
}
