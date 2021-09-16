// Copyright 2021 (c) Cognizant Digital Business, Evolutionary AI. All rights reserved. Issued under the Apache 2.0 License.

package main

import (
	"context"
	"encoding/json"
	"io"
	"io/ioutil"
	"math"
	"os"
	"path/filepath"
	"strings"

	"github.com/leaf-ai/studio-go-runner/pkg/stencil"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/autoscaling"
	"github.com/aws/aws-sdk-go/service/ec2"
	"github.com/davecgh/go-spew/spew"
	"github.com/karlmutch/aws-ec2-price/pkg/price"

	"github.com/go-stack/stack"

	"github.com/jjeffery/kv"
)

func addCatalog(group string, item string, groups map[string][]string) {
	if aGroup, isPresent := groups[group]; isPresent {
		groups[group] = append(aGroup, item)
		return
	}
	groups[group] = []string{item}
	return
}

// getGroups extracts from an EKS cluster all of the known node groups and their machine types
//
func getGroups(ctx context.Context, cfg *Config, cluster string) (asGroups map[string][]string, err kv.Error) {

	sess, err := NewSession(ctx, cfg)
	if err != nil {
		return nil, err
	}

	as := autoscaling.New(sess)

	opts := &autoscaling.DescribeAutoScalingGroupsInput{}

	asGroups = map[string][]string{}

	for {
		groups, errGo := as.DescribeAutoScalingGroups(opts)
		if errGo != nil {
			return nil, kv.Wrap(errGo).With("stack", stack.Trace().TrimRuntime())
		}

		for _, aGroup := range groups.AutoScalingGroups {
			for _, aTag := range aGroup.Tags {
				if aTag.Key == nil || aTag.Value == nil {
					continue
				}
				if strings.HasSuffix(*aTag.Key, "eksctl.io/cluster-name") &&
					*aTag.Value == cluster {
					logger.Trace(*aGroup.AutoScalingGroupName, "stack", stack.Trace().TrimRuntime())
					if aGroup.MixedInstancesPolicy == nil {
						// Get the aGroup.LaunchTemplate and describe it
						if aGroup.LaunchTemplate == nil ||
							aGroup.LaunchTemplate.LaunchTemplateId == nil ||
							len(*aGroup.LaunchTemplate.LaunchTemplateId) == 0 {
							continue
						}
						instances, err := GetInstLT(sess, *aGroup.LaunchTemplate.LaunchTemplateId, *aGroup.LaunchTemplate.Version)

						if err != nil {
							logger.Warn(err.Error(), "stack", stack.Trace().TrimRuntime())
							continue
						}

						for _, aType := range instances {
							addCatalog(*aGroup.AutoScalingGroupName, aType, asGroups)
						}
					} else {
						if aGroup.MixedInstancesPolicy.LaunchTemplate == nil ||
							aGroup.MixedInstancesPolicy.LaunchTemplate.Overrides == nil {
							continue
						} else {
							for _, override := range aGroup.MixedInstancesPolicy.LaunchTemplate.Overrides {
								if override.InstanceType != nil {
									addCatalog(*aGroup.AutoScalingGroupName, *override.InstanceType, asGroups)
								}
							}
						}
					}
				}
			}
		}
		if groups.NextToken == nil || len(*groups.NextToken) == 0 {
			break
		}
		opts.NextToken = groups.NextToken
	}
	return asGroups, nil
}

// groomQueues is used to drop any queues from the collection which
// currently are being fully serviced and do not need new runners to
// handling work that is ready
//
func groomQueues(queues *Queues) (err kv.Error) {
	for qName, qDetails := range *queues {
		// If we have enough runners drop the queue as it needs nothing done to it
		if len(qDetails.NodeGroup) == 0 || qDetails.Running >= qDetails.Ready+qDetails.NotVisible {
			if logger.IsTrace() {
				logger.Trace("queue already handled", "queue", qName, "stack", stack.Trace().TrimRuntime())
			}
			delete(*queues, qName)
		}
	}
	return nil
}

// loadNodeGroups selects an appropriate node group for each queue based on their matching
// instance types and updates the queues data structure with the matches.
//
// The instances used for the matches from the queues are matched in the order in which they
// appear, we assume that the array has been sorted according to cost, and the first match would
// be used.  Once we have node groups with matches selected the second step in the function is
// to use the cheapest.
//
func loadNodeGroups(ctx context.Context, cfg *Config, cluster string, queues *Queues, instances map[string][]string) (err kv.Error) {
	for qName, qDetails := range *queues {
		// Use the needed instance types from the queue and find matching groups
		// The key will be an ASG nodeGroup name
		matches, err := func() (matches map[string]*price.Instance, err kv.Error) {
			matches = map[string]*price.Instance{}
			for _, instance := range qDetails.Instances {
				if groups, isPresent := instances[instance.name]; isPresent {
					for _, groupName := range groups {
						// If there was a match found and the group has not yet been discovered
						// then add it
						if _, isPresent := matches[groupName]; !isPresent {
							// Now go through and test the allocation would be successful to this instance type.  This
							// checks we are not using an over spec machine that is over provisioned for the job.
							//
							fits, err := qDetails.Resource.Fit(instance.resource)
							if err != nil {
								logger.Trace("error while checking fit", "instance", instance.name, "error", err.Error())
								return matches, err
							}
							if !fits {
								if logger.IsTrace() {
									logger.Trace("failed to fit", "instance", instance.name, "node", spew.Sdump(*instance.resource), "queue", spew.Sdump(qDetails.Resource), "stack", stack.Trace().TrimRuntime())
								}
								continue
							}
							logger.Debug("adding", groupName, "for queue", qName)
							matches[groupName] = instance.cost
						}
					}
				}
			}
			return matches, nil
		}()
		if err != nil {
			return err
		}

		if len(matches) == 0 {
			logger.Debug("unable to match any of the available ", len(qDetails.Instances), " node groups with queue workload", "queue", qName)
			continue
		}

		// Having found a number of potential groups that we could use find the cheapest and
		// then update the queue with an assigned ASG nodeGroup
		cheapest := &price.Instance{
			Price: math.MaxFloat64,
		}
		for groupName, instance := range matches {
			if instance.Price < cheapest.Price {
				qDetails.NodeGroup = groupName
				(*queues)[qName] = qDetails
				cheapest = instance
				logger.Debug("cheapest now", instance.Type, "for queue", qName)
			}
		}

	}
	return nil
}

func GetInstLT(sess *session.Session, templateId string, version string) (instanceTypes []string, err kv.Error) {
	svc := ec2.New(sess)

	input := &ec2.DescribeLaunchTemplateVersionsInput{
		LaunchTemplateId: aws.String(templateId),
	}

	for {
		lts, errGo := svc.DescribeLaunchTemplateVersions(input)
		if errGo != nil {
			return instanceTypes, kv.Wrap(errGo).With("stack", stack.Trace().TrimRuntime())
		}

		for _, lt := range lts.LaunchTemplateVersions {
			if lt.DefaultVersion == nil || !*lt.DefaultVersion {
				continue
			}
			if lt.LaunchTemplateData.InstanceType != nil {
				instanceTypes = append(instanceTypes, *lt.LaunchTemplateData.InstanceType)
			}
		}

		if lts.NextToken == nil || len(*lts.NextToken) == 0 {
			break
		}
		input.NextToken = lts.NextToken
	}
	return instanceTypes, nil
}

// jobQAssign will visit all of the known queues and will discover any node pools that
// can address their work and assign those pools to the queues
//
func jobQAssign(ctx context.Context, cfg *Config, cluster string, queues *Queues) (err kv.Error) {

	// Obtain a list of all of the known node groups in the cluster and the machine types they
	// are provisioning
	groups, err := getGroups(ctx, cfg, cluster)
	if err != nil {
		return err
	}

	instances := map[string][]string{}
	// Create a map from the groups, node group major, for a ec2 instance type major collection
	for aGroup, instTypes := range groups {
		for _, instType := range instTypes {
			addCatalog(instType, aGroup, instances)
		}
	}

	if logger.IsTrace() {
		logger.Trace(spew.Sdump(groups), "stack", stack.Trace().TrimRuntime())
		logger.Trace(spew.Sdump(instances), "stack", stack.Trace().TrimRuntime())
	}

	// Assign the known machine types based on the Queues and then match them up
	if err = loadNodeGroups(ctx, cfg, cluster, queues, instances); err != nil {
		return err
	}

	return nil
}

// jobGenerate examine queues for needed runners and will apply a supplied template to generate
// Kubernetes resources
func jobGenerate(ctx context.Context, cfg *Config, cluster string, tmplFN string, output io.Writer, queues *Queues) (err kv.Error) {

	// Open the template file to be used
	tmplFile, errGo := os.Open(tmplFN)
	if errGo != nil {
		return kv.Wrap(errGo).With("stack", stack.Trace().TrimRuntime())
	}

	// Get a working directory to be used for generating values files
	tmpDir, errGo := ioutil.TempDir("", "")
	if errGo != nil {
		return kv.Wrap(errGo).With("stack", stack.Trace().TrimRuntime())
	}
	defer func() {
		os.RemoveAll(tmpDir)
	}()

	if logger.IsDebug() {
		names := []string{}
		for qName, _ := range *queues {
			names = append(names, qName)
		}
		logger.Debug("generating job templates", "queues", strings.Join(names, ", "), "stack", stack.Trace().TrimRuntime())
	}

	for qName, qDetails := range *queues {
		json, errGo := json.MarshalIndent(qDetails, "", "    ")
		if errGo != nil {
			return kv.Wrap(errGo).With("stack", stack.Trace().TrimRuntime())
		}

		qVarsFN := filepath.Join(tmpDir, qName+".json")
		if errGo = ioutil.WriteFile(qVarsFN, json, 0600); errGo != nil {
			return kv.Wrap(errGo).With("stack", stack.Trace().TrimRuntime())
		}

		opts := stencil.TemplateOptions{
			IOFiles: []stencil.TemplateIOFiles{{
				In:  tmplFile,
				Out: output,
			}},
			ValueFiles: []string{qVarsFN},
			OverrideValues: map[string]string{
				"QueueName": qName,
			},
		}
		err, warns := stencil.Template(opts)
		logger.Warn(spew.Sdump(warns))
		if err != nil {
			return err
		}
		tmplFile.Seek(0, io.SeekStart) // The template is read multiple times so we rewindow between each
		// Write comments in the output to ensure kubernetes resource sections in the file are split
		output.Write([]byte("\n---\n"))
	}
	return nil
}
