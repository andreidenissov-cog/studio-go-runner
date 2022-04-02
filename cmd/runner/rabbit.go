// Copyright 2018-2020 (c) Cognizant Digital Business, Evolutionary AI. All rights reserved. Issued under the Apache 2.0 License.

package main

import (
	"context"
	"fmt"
	"math"
	"net/url"
	"os"
	"regexp"
	"strings"
	"time"

	"github.com/andreidenissov-cog/go-service/pkg/log"
	"github.com/andreidenissov-cog/go-service/pkg/server"
	"github.com/andreidenissov-cog/go-service/pkg/types"
	"github.com/leaf-ai/studio-go-runner/internal/runner"
	"github.com/leaf-ai/studio-go-runner/internal/task"

	"github.com/go-stack/stack"
	"github.com/jjeffery/kv" // MIT License

	"github.com/prometheus/client_golang/prometheus"
)

var (
	wrapperFailSeen = false
)

func extractCreds(aURL string) (parsedURL string, creds string) {
	qURL, errGo := url.Parse(os.ExpandEnv(aURL))
	if errGo != nil {
		logger.Warn(kv.Wrap(errGo).With("stack", stack.Trace().TrimRuntime()).Error())
	}
	if qURL.User != nil {
		creds = qURL.User.String()
	} else {
		safeURL := qURL.Scheme + "://" + qURL.Host + "..."
		logger.Warn(kv.NewError("missing credentials in url").With("url", safeURL).With("stack", stack.Trace().TrimRuntime()).Error())
	}
	qURL.User = nil

	return qURL.String(), creds
}

// This file contains the implementation of a RabbitMQ service for
// retrieving and handling StudioML workloads within a self hosted
// queue context

func rmqConfig() (creds string, queueURL string, mgtURL string, err kv.Error) {
	if len(*amqpURL) != 0 {
		queueURL, creds = extractCreds(*amqpURL)
	}

	if len(*amqpMgtURL) != 0 {
		mgtURL, creds = extractCreds(*amqpMgtURL)
	}

	return creds, queueURL, mgtURL, nil
}

func initRMQ() (rmq *runner.RabbitMQ) {
	// NewRabbitMQ takes a URL that has no credentials or tokens attached as the
	// first parameter and the user name password as the second parameter
	w, err := getWrapper()
	if err != nil {
		if !wrapperFailSeen {
			logger.Warn(err.Error(), "stack", stack.Trace().TrimRuntime())
			wrapperFailSeen = true
		}
	}

	creds, qURL, mgtURL, err := rmqConfig()
	if err != nil {
		logger.Warn(err.Error(), "stack", stack.Trace().TrimRuntime())
	}

	rmqRef, err := runner.NewRabbitMQ(qURL, mgtURL, creds, w, log.NewLogger("runner"))
	if err != nil {
		logger.Warn(err.Error(), "stack", stack.Trace().TrimRuntime())
	}
	return rmqRef
}

func initRMQStructs() (matcher *regexp.Regexp, mismatcher *regexp.Regexp) {

	// The regular expression is validated in the main.go file
	matcher, errGo := regexp.Compile(*queueMatch)
	if errGo != nil {
		if len(*queueMatch) != 0 {
			logger.Warn(kv.Wrap(errGo).With("matcher", *queueMatch).With("stack", stack.Trace().TrimRuntime()).Error())
		}
		matcher = nil
	}

	// If the length of the mismatcher is 0 then we will get a nil and because this
	// was checked in the main we can ignore that as this is optional

	if len(strings.Trim(*queueMismatch, " \n\r\t")) == 0 {
		mismatcher = nil
	} else {
		mismatcher, errGo = regexp.Compile(*queueMismatch)
		if errGo != nil {
			if len(*queueMismatch) != 0 {
				logger.Warn(kv.Wrap(errGo).With("mismatcher", *queueMismatch).With("stack", stack.Trace().TrimRuntime()).Error())
			}
			mismatcher = nil
		}
	}
	return matcher, mismatcher
}

// serviceRMQ runs for the lifetime of the daemon and uses the ctx to perform orderly shutdowns.
// This function will initiate checks of the queue servers for new queues that require processing
// using the projects server Cycle function.
//
func serviceRMQ(ctx context.Context, checkInterval time.Duration, connTimeout time.Duration) {

	logger.Debug("starting serviceRMQ", stack.Trace().TrimRuntime())
	defer logger.Debug("stopping serviceRMQ", stack.Trace().TrimRuntime())

	if len(*amqpURL) == 0 {
		logger.Info("rabbitMQ services disabled", stack.Trace().TrimRuntime())
		return
	}

	matcher, mismatcher := initRMQStructs()
	rmq := initRMQ()

	// Tracks all known queues and their cancel functions so they can have any
	// running jobs terminated should they disappear
	live := &Projects{
		queueType: "rabbitMQ",
		projects:  map[string]context.CancelFunc{},
	}

	lifecycleC := make(chan server.K8sStateUpdate, 1)
	id, err := server.K8sStateUpdates().Add(lifecycleC)
	if err != nil {
		logger.Warn(err.With("stack", stack.Trace().TrimRuntime()).Error())
	}

	defer func() {
		// Ignore failures to cleanup resources we will never reuse
		func() {
			defer func() {
				_ = recover()
			}()
			server.K8sStateUpdates().Delete(id)
		}()
		close(lifecycleC)
	}()

	host, errGo := os.Hostname()
	if errGo != nil {
		logger.Warn(errGo.Error())
	}

	// first time through make sure the credentials are checked immediately
	qCheck := time.Duration(time.Second)
	currentCheck := qCheck
	qTicker := time.NewTicker(currentCheck)
	defer qTicker.Stop()

	// Watch for when the server should not be getting new work
	state := server.K8sStateUpdate{
		State: types.K8sRunning,
	}

	for {
		// Dont wait an excessive amount of time after server checks fail before
		// retrying
		if qCheck > time.Duration(3*time.Minute) {
			qCheck = time.Duration(3 * time.Minute)
		}

		// If the interval between queue checks changes reset the ticker

		if qCheck != currentCheck {
			currentCheck = qCheck
			qTicker.Stop()
			qTicker = time.NewTicker(currentCheck)
		}

		select {
		case <-ctx.Done():
			live.Lock()
			defer live.Unlock()

			// When shutting down stop all projects
			for _, quiter := range live.projects {
				if quiter != nil {
					quiter()
				}
			}
			logger.Debug("quitC done for serviceRMQ", stack.Trace().TrimRuntime())
			return
		case state = <-lifecycleC:
		case <-qTicker.C:

			ran, _ := GetCounterAccum(queueRan)
			running, _ := GetGaugeAccum(queueRunning)

			msg := fmt.Sprintf("checking serviceRMQ, with %.0f running tasks and %.0f completed tasks", math.Round(running), math.Round(ran))
			logger.Debug(msg, stack.Trace().TrimRuntime())

			qCheck = checkInterval

			// If the pulling of work is currently suspending bail out of checking the queues
			if state.State != types.K8sRunning && state.State != types.K8sUnknown {
				queueIgnored.With(prometheus.Labels{"host": host, "queue_type": live.queueType, "queue_name": "*"}).Inc()
				logger.Trace("k8s has RMQ disabled", "stack", stack.Trace().TrimRuntime())
				continue
			}

			connCtx, cancel := context.WithTimeout(ctx, connTimeout)

			// Found returns a map that contains the queues that were found
			// on the rabbitMQ server specified by the rmq data structure
			found, err := rmq.GetKnown(connCtx, matcher, mismatcher)
			cancel()

			if err != nil {
				qCheck = qCheck * 2
				err = err.With("backoff", qCheck.String())
				logger.Warn("unable to refresh RMQ manifest", err.Error())
				continue
			}
			if len(found) == 0 {
				items := []string{"no queues", "identity", rmq.Identity, "matcher", matcher.String()}

				if mismatcher != nil {
					items = append(items, "mismatcher", mismatcher.String())
				}
				items = append(items, "stack", stack.Trace().TrimRuntime().String())
				logger.Warn(items[0], items[1:])

				qCheck = qCheck * 2
				continue
			}

			// Found needs to just have the main queue servers as their keys, individual queues will be treated as subscriptions

			filtered := make(map[string]task.QueueDesc, len(found))
			for k, v := range found {
				qItems := strings.Split(k, "?")
				filtered[qItems[0]] = v
			}

			// filtered contains a map of keys that have an uncredentialed URL, and the value which is the user name and password for the URL
			//
			// The URL path is going to be the vhost and the queue name
			if err := live.Cycle(ctx, filtered); err != nil {
				logger.Warn(err.Error())
			}
		}
	}
}
