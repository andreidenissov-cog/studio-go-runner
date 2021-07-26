# Interfacing and Integration

This document describes the interface, and interchange format used between the StudioML client and runners that process StudioML experiments.

<!--ts-->

Table of Contents
=================

* [Interfacing and Integration](#interfacing-and-integration)
* [Table of Contents](#table-of-contents)
  * [Introduction](#introduction)
  * [Audience](#audience)
  * [Runners](#runners)
* [Request messages](#request-messages)
  * [Queuing](#queuing)
  * [Experiment Lifecycle](#experiment-lifecycle)
  * [Message Format](#message-format)
    * [Encrypted payloads](#encrypted-payloads)
    * [Signed payloads](#signed-payloads)
    * [Field descriptions](#field-descriptions)
    * [experiment ↠ pythonver](#experiment--pythonver)
    * [experiment ↠ args](#experiment--args)
    * [experiment ↠ max_duration](#experiment--max_duration)
    * [experiment ↠ filename](#experiment--filename)
    * [experiment ↠ project](#experiment--project)
    * [experiment ↠ project_version](#experiment--project_version)
    * [experiment ↠ author](#experiment--author)
    * [experiment ↠ project_experiment](#experiment--project_experiment)
    * [experiment ↠ artifacts](#experiment--artifacts)
    * [experiment ↠ artifacts ↠ [label] ↠ bucket](#experiment--artifacts--label--bucket)
    * [experiment ↠ artifacts ↠ [label] ↠ credentials](#experiment--artifacts--label--credentials)
    * [experiment ↠ artifacts ↠ [label] ↠ credentials ↠ plain](#experiment--artifacts--label--credentials--plain)
    * [experiment ↠ artifacts ↠ [label] ↠ credentials ↠ plain ↠ user](#experiment--artifacts--label--credentials--plain--user)
    * [experiment ↠ artifacts ↠ [label] ↠ credentials ↠ plain ↠ password](#experiment--artifacts--label--credentials--plain--password)
    * [experiment ↠ artifacts ↠ [label] ↠ credentials ↠ jwt](#experiment--artifacts--label--credentials--jwt)
    * [experiment ↠ artifacts ↠ [label] ↠ credentials ↠ aws](#experiment--artifacts--label--credentials--aws)
    * [experiment ↠ artifacts ↠ [label] ↠ credentials ↠ aws ↠ access_key](#experiment--artifacts--label--credentials--aws--access_key)
    * [experiment ↠ artifacts ↠ [label] ↠ credentials ↠ aws ↠ secret_access_key](#experiment--artifacts--label--credentials--aws--secret_access_key)
    * [experiment ↠ artifacts ↠ [label] ↠ key](#experiment--artifacts--label--key)
    * [experiment ↠ artifacts ↠ [label] ↠ qualified](#experiment--artifacts--label--qualified)
    * [experiment ↠ artifacts ↠ [label] ↠ mutable](#experiment--artifacts--label--mutable)
    * [experiment ↠ artifacts ↠ [label] ↠ unpack](#experiment--artifacts--label--unpack)
    * [experiment ↠ artifacts ↠ resources_needed](#experiment--artifacts--resources_needed)
    * [experiment ↠ artifacts ↠ pythonenv](#experiment--artifacts--pythonenv)
    * [experiment ↠ artifacts ↠  time added](#experiment--artifacts---time-added)
    * [experiment ↠ config](#experiment--config)
    * [experiment ↠ config ↠ experimentLifetime](#experiment--config--experimentlifetime)
    * [experiment ↠ config ↠ verbose](#experiment--config--verbose)
    * [experiment ↠ config ↠ saveWorkspaceFrequency](#experiment--config--saveworkspacefrequency)
    * [experiment ↠ config ↠ database](#experiment--config--database)
    * [experiment ↠ config ↠ database ↠ type](#experiment--config--database--type)
    * [experiment ↠ config ↠ database ↠ authentication](#experiment--config--database--authentication)
    * [experiment ↠ config ↠ database ↠ endpoint](#experiment--config--database--endpoint)
    * [experiment ↠ config ↠ database ↠ bucket](#experiment--config--database--bucket)
    * [experiment ↠ config ↠ storage](#experiment--config--storage)
    * [experiment ↠ config ↠ storage ↠ type](#experiment--config--storage--type)
    * [experiment ↠ config ↠ storage ↠ endpoint](#experiment--config--storage--endpoint)
    * [experiment ↠ config ↠ storage ↠ bucket](#experiment--config--storage--bucket)
    * [experiment ↠ config ↠ storage ↠ authentication](#experiment--config--storage--authentication)
    * [experiment ↠ config ↠ resources_needed](#experiment--config--resources_needed)
    * [experiment ↠ config ↠ resources_needed ↠ hdd](#experiment--config--resources_needed--hdd)
    * [experiment ↠ config ↠ resources_needed ↠ cpus](#experiment--config--resources_needed--cpus)
    * [experiment ↠ config ↠ resources_needed ↠ ram](#experiment--config--resources_needed--ram)
    * [experiment ↠ config ↠ resources_needed ↠ gpus](#experiment--config--resources_needed--gpus)
    * [experiment ↠ config ↠ resources_needed ↠ gpuMem](#experiment--config--resources_needed--gpumem)
    * [experiment ↠ config ↠ env](#experiment--config--env)
    * [experiment ↠ config ↠ cloud ↠ queue ↠ rmq](#experiment--config--cloud--queue--rmq)
* [Report messages](#report-messages)
  * [Queuing](#queuing-1)
  * [Message Format](#message-format-1)
    * [report](#report)
    * [report ↠ time](#report--time)
    * [report ↠ experiment_id](#report--experiment_id)
    * [report ↠ unique_id](#report--unique_id)
    * [report ↠ executor_id](#report--executor_id)
    * [report ↠ payload](#report--payload)
    * [report ↠ logging ↠ time](#report--logging--time)
    * [report ↠ logging](#report--logging)
    * [report ↠ logging ↠ severity](#report--logging--severity)
    * [report ↠ logging ↠ message](#report--logging--message)
    * [report ↠ logging ↠ fields](#report--logging--fields)
    * [report ↠ progress](#report--progress)
    * [report ↠ progress ↠ time](#report--progress--time)
    * [report ↠ progress ↠ json](#report--progress--json)
    * [report ↠ progress ↠ state](#report--progress--state)
    * [report ↠ progress ↠ error](#report--progress--error)
    * [report ↠ progress ↠ error ↠ msg](#report--progress--error--msg)
    * [report ↠ progress ↠ error ↠ code](#report--progress--error--code)
<!--te-->

## Introduction

StudioML has two major modules.

. The client, or front end, that shepherds experiments on behalf of users and packaging up experiments that are then placed on to a queue using json messages

. The runner that receives json formatted messages on a message queue and then runs the experiment they describe

There are other tools that StudioML offers for reporting and management of experiment artifacts that are not within the scope of this document.

It is not yet within the scope of this document to describe how data outside of the queuing interface is stored and formatted.

## Audience

This document is intended for developers who wish to implement runners to process StudioML work, or implement clients that generate work for StudioML runners.

## Runners

This project implements a StudioML runner, however it is not specific to StudioML.  This runner could be used to deliver and execute and python code within a virtualenv that the runner supplies.

Any standard runners can accept a standalone virtualenv with no associated container.  The go runner, this present project, has been extended to allow clients to also send work that has a Singularity container specified.

In the first case, virtualenv only, the runner implcitly trusts that any work received is trusted and is not malicous.  In this mode the runner makes not attempt to protect the integrity of the host it is deployed into.

In the second case if a container is specified it will be used to launch work and the runner will rely upon the container runtime to prevent leakage into the host.

# Request messages

## Queuing

The StudioML eco system relies upon a message queue to buffer work being sent by the StudioML client to any arbitrary runner that is subscribed to the experimenters choosen queuing service.  StudioML support multiple queuing technologies including, AWS SQS, local file system, and RabbitMQ.  The reference implementation is RabbitMQ for the purposes of this present project.  The go runner project supports SQS, and RabbitMQ.

Additional queuing technologies can be added if desired to the StudioML (https://github.com/studioml/studio.git), and go runner (https://github.com/SentientTechnologies/studio-go-runner.git) code bases and a pull request submitted.

When using a queue the StudioML eco system relies upon a reliable, at-least-once, messaging system.  An additional requirement for queuing systems is that if the worker disappears, or work is not reclaimed by the worker as progress is made that the work is requeued by the broker automatically.

## Experiment Lifecycle

If you have had a chance to run some of the example experiments within the StudioML github repository then you will have noticed a keras example.  The keras example is used to initiate a single experiment that queues work for a single runner and then immediately returns to the command line prompt without waiting for a result.  Experiments run in this way rely on the user to monitor their cloud storage bucket and look for the output.tar file in a directory named after their experiment.  For simple examples and tests this is a quick but manual way to work.

In more complex experiments there might be multiple phases to a project that is being run.  Each experiment might represent an individual in for example evolutionary computation.  The python software running the project might want to send potentially hundreds of experiments, or individuals to the runners and then wait for these to complete.  Once complete it might select individuals that scored highly, using as one example a fitness screen.  The python StudioML client might then generate a new population that are then marshall individuals from the population into experiments, repeating this cycle potentially for days.

To address the need for longer running experiments StudioML offers a number of python classes within the open source distribution that allows this style of longer running taining scenarios to be implemented by researchers and engineers.  The combination of completion service and session server classes can be used to create these long running StudioML compliant clients.

Completion service based applications that use the StudioML classes generate work in exactly the same way as the CLI based 'studio run' command.  Session servers are an implementation of a completion service combined with logic that once experiments are queued will on a regular interval examine the cloud storage folders for returned archives that runners have rolled up when they either save experiment workspaces, or at the conclusion of the experiment find that the python experiment code had generated files in directories identified as a part of the queued job.  After the requisite numer of experiments are deemed to have finished based on the storage server bucket contents the session server can then examine the uploaded artifacts and determine their next set of training steps.

## Message Format

The following figure shows an example of a job sent from the studioML front end to the runner.  The runner does not always make use of the entire set of json tags, typically a limited but consistent subset of tags are used.  This format is a clear text format, please see below for notes regarding the encrypted format.

```json
{
  "experiment": {
    "status": "waiting",
    "time_finished": null,
    "git": null,
    "key": "1530054412_70d7eaf4-3ce3-493a-a8f6-ffa0212a5c92",
    "time_last_checkpoint": 1530054414.027222,
    "pythonver": "3.6",
    "metric": null,
    "args": [
      "10"
    ],
    "max_duration": "20m",
    "filename": "train_cifar10.py",
    "project": "",
    "project_version": "",
    "project_experiment": "",
    "artifacts": {
      "output": {
        "local": "/home/kmutch/.studioml/experiments/1530054412_70d7eaf4-3ce3-493a-a8f6-ffa0212a5c92/output",
        "bucket": "kmutch-rmq",
        "qualified": "s3://s3-us-west-2.amazonaws.com/kmutch-rmq/experiments/1530054412_70d7eaf4-3ce3-493a-a8f6-ffa0212a5c92/output.tar",
        "key": "experiments/1530054412_70d7eaf4-3ce3-493a-a8f6-ffa0212a5c92/output.tar",
        "credentials": {
            "aws": {
                "access_key": "AKZAIE5G7Q2GZC3OMTYW",
                "secret_key": "rt43wqJ/w5aqAPat659gkkYpphnOFxXejsCBq"
            }
        },
        "mutable": true,
        "unpack": true
      },
      "_metrics": {
        "local": "/home/kmutch/.studioml/experiments/1530054412_70d7eaf4-3ce3-493a-a8f6-ffa0212a5c92/_metrics",
        "bucket": "kmutch-rmq",
        "qualified": "s3://s3-us-west-2.amazonaws.com/kmutch-rmq/experiments/1530054412_70d7eaf4-3ce3-493a-a8f6-ffa0212a5c92/_metrics.tar",
        "key": "experiments/1530054412_70d7eaf4-3ce3-493a-a8f6-ffa0212a5c92/_metrics.tar",
        "credentials": {
            "aws": {
                "access_key": "AKZAIE5G7Q2GZC3OMTYW",
                "secret_key": "rt43wqJ/w5aqAPat659gkkYpphnOFxXejsCBq"
            }
        },
        "mutable": true,
        "unpack": true
      },
      "modeldir": {
        "local": "/home/kmutch/.studioml/experiments/1530054412_70d7eaf4-3ce3-493a-a8f6-ffa0212a5c92/modeldir",
        "bucket": "kmutch-rmq",
        "qualified": "s3://s3-us-west-2.amazonaws.com/kmutch-rmq/experiments/1530054412_70d7eaf4-3ce3-493a-a8f6-ffa0212a5c92/modeldir.tar",
        "key": "experiments/1530054412_70d7eaf4-3ce3-493a-a8f6-ffa0212a5c92/modeldir.tar",
        "credentials": {
            "aws": {
                "access_key": "AKZAIE5G7Q2GZC3OMTYW",
                "secret_key": "rt43wqJ/w5aqAPat659gkkYpphnOFxXejsCBq"
            }
        },
        "mutable": true,
        "unpack": true
      },
      "workspace": {
        "local": "/home/kmutch/studio/examples/keras",
        "bucket": "kmutch-rmq",
        "qualified": "s3://s3-us-west-2.amazonaws.com/kmutch-rmq/blobstore/419411b17e9c851852735901a17bd6d20188cee30a0b589f1bf1ca5b487930b5.tar",
        "key": "blobstore/419411b17e9c851852735901a17bd6d20188cee30a0b589f1bf1ca5b487930b5.tar",
        "credentials": {
            "aws": {
                "access_key": "AKZAIE5G7Q2GZC3OMTYW",
                "secret_key": "rt43wqJ/w5aqAPat659gkkYpphnOFxXejsCBq"
            }
        },
        "mutable": false,
        "unpack": true
      },
      "tb": {
        "local": "/home/kmutch/.studioml/experiments/1530054412_70d7eaf4-3ce3-493a-a8f6-ffa0212a5c92/tb",
        "bucket": "kmutch-rmq",
        "qualified": "s3://s3-us-west-2.amazonaws.com/kmutch-rmq/experiments/1530054412_70d7eaf4-3ce3-493a-a8f6-ffa0212a5c92/tb.tar",
        "key": "experiments/1530054412_70d7eaf4-3ce3-493a-a8f6-ffa0212a5c92/tb.tar",
        "credentials": {
            "aws": {
                "access_key": "AKZAIE5G7Q2GZC3OMTYW",
                "secret_key": "rt43wqJ/w5aqAPat659gkkYpphnOFxXejsCBq"
            }
        },
        "mutable": true,
        "unpack": true
      }
    },
     "info": {},
    "resources_needed": {
      "hdd": "3gb",
      "gpus": 1,
      "ram": "2gb",
      "cpus": 1,
      "gpuMem": "4gb"
    },
    "pythonenv": [
      "APScheduler==3.5.1",
      "argparse==1.2.1",
      "asn1crypto==0.24.0",
      "attrs==17.4.0",
      "autopep8==1.3.5",
      "awscli==1.15.4",
      "boto3==1.7.4",
      "botocore==1.10.4",
...
      "six==1.11.0",
      "sseclient==0.0.19",
      "-e git+https://github.com/SentientTechnologies/studio@685f4891764227a2e1ea5f7fc91b31dcf3557647#egg=studioml",
      "terminaltables==3.1.0",
      "timeout-decorator==0.4.0",
      "tzlocal==1.5.1",
      "uritemplate==3.0.0",
      "urllib3==1.22",
      "Werkzeug==0.14.1",
      "wheel==0.31.0",
      "wsgiref==0.1.2"
    ],
    "author": "guest",
    "time_added": 1530054413.134781,
    "time_started": null
  },
  "config": {
    "optimizer": {
      "visualization": true,
      "load_checkpoint_file": null,
      "cmaes_config": {
        "load_best_only": false,
        "popsize": 100,
        "sigma0": 0.25
      },
      "termination_criterion": {
        "generation": 5,
        "fitness": 999,
        "skip_gen_timeout": 30,
        "skip_gen_thres": 1
      },
      },
      "result_dir": "~/Desktop/",
      "checkpoint_interval": 0
    },
    "verbose": "debug",
    "saveWorkspaceFrequency": "3m",
    "database": {
      "type": "s3",
      "authentication": "none",
      "endpoint": "http://s3-us-west-2.amazonaws.com",
      "bucket": "kmutch-metadata",
      "credentials": {
          "aws": {
              "access_key": "AKZAIE5G7Q2GZC3OMTYW",
              "secret_key": "rt43wqJ/w5aqAPat659gkkYpphnOFxXejsCBq"
          }
      }
    },
    "runner": {
      "slack_destination": "@karl.mutch"
    },
    "storage": {
      "type": "s3",
      "endpoint": "http://s3-us-west-2.amazonaws.com",
      "bucket": "kmutch-rmq",
      "credentials": {
          "aws": {
              "access_key": "AKZAIE5G7Q2GZC3OMTYW",
              "secret_key": "rt43wqJ/w5aqAPat659gkkYpphnOFxXejsCBq"
          }
      }
    },
    "server": {
      "authentication": "None"
    },
    "env": {
      "PATH": "%PATH%:./bin"
    },
    "cloud": {
      "queue": {
        "rmq": "amqp://user:password@10.230.72.19:5672/%2f?connection_attempts=30&retry_delay=.5&socket_timeout=5"
      }
    }
  }
}
```

### Encrypted payloads

In the event that message level encryption is enabled then the payload format will vary from the clear-text format.  The encrypted format will retain a very few blocks in clear-text to assist in scheduling, the status, pythonver, experiment_lifetime, time_added, and the resources needed blocks as in the following example. All other fragments will be rolled up into an encrypted_data block, consisting of Base64 encoded data.  The fields used within the clear-text header retain the same purpose and meaning as those in the Request documented in the [Field Descriptions](#field-descriptions) section

Encrypted payloads use a hybrid cryptosystem, for a detailed description please see https://en.wikipedia.org/wiki/Hybrid_cryptosystem.

A detailed description of the StudioML implementation of this system can be found in the [docs/message_privacy.md](docs/message_privacy.md) documentation.

The following figures shows an example of the clear-text headers and the encrypted payload portion of a message:

```json
{
  "message": {
    "experiment": {
        "status": "waiting",
        "pythonver": "3.6",
    },
    "time_added": 1530054413.134781,
    "experiment_lifetime": "30m",
    "resources_needed": {
        "gpus": 1,
        "hdd": "3gb",
        "ram": "2gb",
        "cpus": 1,
        "gpuMem": "4gb"
    },
    "payload": "Full Base64 encrypted payload"
  }
}
```

The encrypted format will retain a very few blocks in clear-text to assist in scheduling, the status, pythonver, experiment_lifetime, time_added, and the resources needed blocks as in the following example. All other fragments will be rolled up into an encrypted_data block, consisting of Base64 encoded data.  The fields used within the clear-text header retain the same purpose and meaning as those in the Request documented in the [Field Descriptions](#field-descriptions) section

Encrypted payloads use a hybrid cryptosystem, for a detailed description please see https://en.wikipedia.org/wiki/Hybrid_cryptosystem.

A detailed description of the StudioML implementation of this system can be found in the [message_privacy](docs/message_privacy.md) documentation.

The following figures shows an example of the clear-text headers and the encrypted payload portion of a message:

```json
{
  "message": {
    "experiment": {
        "status": "waiting",
        "pythonver": "3.6",
    },
    "time_added": 1530054413.134781,
    "experiment_lifetime": "30m",
    "resources_needed": {
        "gpus": 1,
        "hdd": "3gb",
        "ram": "2gb",
        "cpus": 1,
        "gpuMem": "4gb"
    },
    "payload": "Full Base64 encrypted payload"
  }
}
```

The encrypted payload should consist of a 24 byte nonce, and then the users encrypted data.

When processing messages runners can use the clear-text JSON in an advisory capacity to determine if messages are useful before decrypting their contents, however once decrypted messages will be re-evaluated using the decrypted contents only.  The clear-text portions of the message  will be ignored post decryption.

Private keys and passphrases are provisioned on compute clusters using the Kubernetes secrets service and stored encrypted within etcd when the go runner is used.

### Signed payloads

Message signing is a way of protecting the runner receiving messages from processing spoofed requests.  To prevent this the runner can be configured to read public key information from Kubernetes secrets and then to use this to validate messages that are being received.  The configuration information for the runner signing keys is detailed in the [message\_privacy.md](message_privacy.md) file.

Message signing must be used in combination with message encryption features described in the previous section.

The format of the signature that is transmitted using the StudioML message signature field consists of the Base64 encoded signature blob, encoded from the binary 64 byte signature.

The signing information is encoded into two JSON elements, the fingerprint and signature elements, for example:

```
```json
{
  "message": {
    "experiment": {
        "status": "waiting",
        "pythonver": "3.6",
    },
    "time_added": 1530054413.134781,
    "experiment_lifetime": "30m",
    "resources_needed": {
        "gpus": 1,
        "hdd": "3gb",
        "ram": "2gb",
        "cpus": 1,
        "gpuMem": "4gb"
    },
    "payload": "Full Base64 encrypted payload",
    "fingerprint": "Base64 of sha256 binary fingerprint",
    "signature": "Base64 encoded binary signature for the Base64 representation of the encrypted payload"
  }
}
```

### Field descriptions

### experiment ↠ pythonver

The value for this tag must be an integer 2 or 3 for the specific python version requested by the experimenter.

### experiment ↠ args

A list of the command line arguments to be supplied to the python interpreter that will be passed into the main of the running python job.

### experiment ↠ max\_duration

The period of time that the experiment is permitted to run in a single attempt.  If this time is exceeded the runner can abandon the task at any point but it may continue to run for a short period.

### experiment ↠ filename

The python file in which the experiment code is to be found.  This file should exist within the workspace artifact archive relative to the top level directory.

### experiment ↠ project

All experiments must be assigned to a project.  The project identifier is a LEAF label assigned by the StudioML user and is specific to organization running StudioML or LEAF solution.

### experiment ↠ project_version

This field when defined denotes the specific version of a project this StudioML experiment is associated with.

### experiment ↠ author

All experiments must be assigned to a user that is the designated project experiment author.  The experiment author identifier is typically a globally unique email address or identity label and is specific to organization running StudioML or LEAF solution.  Typical values for this field could include an employee or email address.

### experiment ↠ project_experiment

Within StudioML experiments represent a single individual task, procedure or action.  LEAF projects represent a user namespace and contain one or more LEAF experiments each one of those containing one or more StudioML experiments.  This field is used to gather a collection of individual StudioML experiments within the context of a single LEAF project level experiment using this field as a label.

### experiment ↠ artifacts

Artifacts are assigned labels, some labels have significance.  The workspace artifact should contain any python code that is needed, it may container other assets for the python code to run including configuration files etc.  The output artifact is used to identify where any logging and returned results will be archives to.

Work that is sent to StudioML runners must have at least one workspace artifact consisting of the python code that will be run.  Artifacts are typically tar archives that contain not just python code but also any other data needed by the experiment being run.

Before the experiment commences the artifact will be unrolled onto local disk of the container running it.  When unrolled the artifact label is used to name the peer directory into which any files are placed.

The experiment when running will be placed into the workspace directory which contains the contents of the workspace labeled artifact.  Any other artifacts that were downloaded will be peer directories of the workspace directory.  Artifacts that were mutable and not available for downloading at the start of the experiment will results in empty peer directories that are named based on the label as well.

Artifacts do not have any restriction on the size of the data they identify.

The StudioML runner will download all artifacts that it can prior to starting an experiment.  Should any mutable artifacts be not available then they will be ignored and the experiment will continue.  If non-mutable artifacts are not found then the experiment will fail.

Named non-mutable artifacts are subject to caching to reduce download times and network load.

### experiment ↠ artifacts ↠ [label] ↠ bucket

The bucket identifies the cloud providers storage service bucket.  This value is not used when the go runner is running tasks.  This value is used by the python runner for configurations where the StudioML client is being run in proximoity to a StudioML configuration file.

### experiment ↠ artifacts ↠ [label] ↠ credentials

This block is used to transport credentials for accessing the [label] artifact and can contain platform specific credential information.

If the fields within this block are not defined any default credentials that can be found within the message defined configuration environment variables can be used by the implementation of the runner as long as appropriate flags are defined to allow them to be used.

### experiment ↠ artifacts ↠ [label] ↠ credentials ↠ plain

The plain block is used to store credentials when the artifact platform uses plain user password style credentials such as common with facilities such as FTP.

### experiment ↠ artifacts ↠ [label] ↠ credentials ↠ plain ↠ user

The user name that is to be used to access the resource.  User name password combinations can be used for file transfer protocols.

### experiment ↠ artifacts ↠ [label] ↠ credentials ↠ plain ↠ password

The password that is to be used to access the resource.  User name password combinations can be used for file transfer protocols.

### experiment ↠ artifacts ↠ [label] ↠ credentials ↠ jwt

For some transports JWT bearer style tokens can be used.  These transports are typically used in commercial solutions leveraging proprietary runners.

### experiment ↠ artifacts ↠ [label] ↠ credentials ↠ aws

The aws block is used to store credentials when the artifact platform uses AWS S3 style credentials and is also used for S3 compatible platforms such as minio.

If you wish to use anonymnous access you should define this structure with empty strings for the access_key, and secret_key value.

### experiment ↠ artifacts ↠ [label] ↠ credentials ↠ aws ↠ access_key

AWS blob stores and other services support access via an access key and secret access key both of which can be specified in the credentials block.

### experiment ↠ artifacts ↠ [label] ↠ credentials ↠ aws ↠ secret_access_key

AWS blob stores and other services support access via an access key and secret access key both of which can be specified in the credentials block.

### experiment ↠ artifacts ↠ [label] ↠ key

The key identifies the cloud providers storage service key value for the artifact.  This value is not used when the go runner is running tasks.  This value is used by the python runner for configurations where the StudioML client is being run in proxiomity to a StudioML configuration file.

### experiment ↠ artifacts ↠ [label] ↠ qualified

The qualified field contains a fully specified cloud storage platform reference that includes a schema used for selecting the storage platform implementation.  The host name is used within AWS to select the appropriate endpoint and region for the bucket, when using Minio this identifies the endpoint being used including the port number.  The URI path contains the bucket and file name (key in the case of AWS) for the artifact.

If the artifact is mutable and will be returned to the S3 or Minio storage then the bucket MUST exist otherwise the experiment will fail.

A deprecated feature allows the environment section of the json payload be used to supply the needed credentials for the storage.  The go runner will be extended in future to allow the use of a user:password pair inside the URI to allow for multiple credentials on the cloud storage platform.  This is prone to leakage so it is recommended that the artifacts ↠ credentials section is used.

### experiment ↠ artifacts ↠ [label] ↠ mutable

mutable is a true/false flag for identifying whether an artifact should be returned to the storage platform being used.  mutable artifacts that are not able to be downloaded at the start of an experiment will not cause the runner to terminate the experiment, non-mutable downloads that fail will lead to the experiment stopping.

### experiment ↠ artifacts ↠ [label] ↠ unpack

unpack is a true/false flag that can be used to supress the tar or other compatible archive format archive within the artifact.

### experiment ↠ artifacts ↠ resources\_needed

This section is a repeat of the experiment config resources_needed section, please ignore.

### experiment ↠ artifacts ↠ pythonenv

This section encapsulates a json string array containing pip install dependencies and their versions.  The string elements in this array are a json rendering of what would typically appear in a pip requirements files.  The runner will unpack the frozen pip packages and will install them prior to the experiment running.  Any valid pip reference can be used except for private dependencies that require specialized authentication which is not supported by runners.  If a private dependency is needed then you should add the pip dependency as a file within an artifact and load the dependency in your python experiment implemention to protect it.

### experiment ↠ artifacts ↠  time added

The time that the experiment was initially created expressed as a floating point number representing the seconds since the epoc started, January 1st 1970.

### experiment ↠ config

The StudioML configuration file can be used to store parameters that are not processed by the StudioML client.  These values are passed to the runners and are not validated.  When present to the runner they can then be used to configure it or change its behavior.  If you implement your own runner then you can add values to the configuration file and they will then be placed into the config section of the json payload the runner receives.

Running experiments that make use of Sentient ENN tooling or third party libraries will often require that framework specific configuration values be placed into this section.  Example of frameworks that use these values include the StudioML completion service, and evolutionary strategies used for numerical optimization.

### experiment ↠ config ↠ experimentLifetime

This variable is used to inform the go runner of the date and time that the experiment should be considered to be dead and any work related to it should be abandoned or discarded.  This acts as a gaureentee that the client will no longer need to be concerned with the experiment and work can be requeued in the system, as one example, without fear of repeatition.

The value is expressed as an integer followed by a unit, s,m,h.

### experiment ↠ config ↠ verbose

verbose can be used to adjust the logging level for the runner and for StudioML components.  It has the following valid string values debug, info, warn, error, crit.

### experiment ↠ config ↠ saveWorkspaceFrequency

On a regular basis the runner can upload any logs and intermediate results from the experiments mutable labelled artifact directories.  This variable can be used to set the interval at which these uploads are done.  The primary purpose of this variable is to speed up remote monitoring of intermediate output logging from the runner and the python code within the experiment.

This variable is not intended to be used as a substitute for experiment checkpointing.

### experiment ↠ config ↠ database

The database within StudioML is used to store meta-data that StudioML generates to describe experiments, projects and other useful material related to the progress of experiments such as the start time, owner.

The database can point at blob storage or can be used with structured datastores should you wish to customize it.  The database is used in the event that the API server is launched by a user as a very simply way of accessing experiment and user details.

### experiment ↠ config ↠ database ↠ type

This variable denotes the storage format being used by StudioML to store meta-data and supports three types within the open source offering, firebase, gcloud, s3.  Using s3 does allow other stores such as Azure blob storage when a bridging technology such as Minio is used.

### experiment ↠ config ↠ database ↠ authentication

Not yet widely supported across the database types this variable supports either none, firebase, or github.  Currently its application is only to the gcloud, amnd firebase storage.  The go runner is intended for non vendor dependent implementations and uses the env variable seetings for the AWS authentication currently.  It is planned in the future that the authentication would make use of shortlived tokens using this field.

### experiment ↠ config ↠ database ↠ endpoint

The endpoint variable is used to denote the S3 endpoint that is used to terminate API requests on.  This is used for both native S3 and minio support.  

In the case of a native S3 deployment it will be one of the well known endpoints for S3 and should be biased to using the region specific endpoints for the buckets being used, an example for this use case would be 'http://s3-us-west-2.amazonaws.com'.

In the case of minio this should point at the appropriate endpoint for the minio server along with the port being used, for example http://40.114.110.201:9000/.  If you wish to use HTTPS to increase security the runners deployed must have the appropriate root certificates installed and the certs on your minio server setup to reference one of the publically well known certificate authorities.

### experiment ↠ config ↠ database ↠ bucket

The bucket variable denotes the bucket name being used and should be homed in the region that is configured using the endpoint and any AWS style environment variables captured in the environment variables section, 'env'.

### experiment ↠ config ↠ storage

The storage area within StudioML is used to store the artifacts and assets that are created by the StudioML client.  The typical files placed into the storage are include any directories that are stored on the local workstation of the experimenter and need to be copied to a location that is available to runners.

At a minimum when an experiment starts there will be an workspace artifact placed into the storage area.  Any artifacts placed into the storage will have a key that denotes the exact experiment and the name of the directory that was archived.

Upon completion of the experiment the storage area will be updated with artifacts that are denoted as mutable and that have been changed.

### experiment ↠ config ↠ storage ↠ type

This variable denotes the storage being used as either gs (google cloud storage), or s3.

### experiment ↠ config ↠ storage ↠ endpoint

The endpoint variable is used to denote the S3 endpoint that is used to terminate API requests on.  This is used for both native S3 and minio support.

In the case of a native S3 deployment it will be one of the well known endpoints for S3 and should be biased to using the region specific endpoints for the buckets being used, an example for this use case would be 'http://s3-us-west-2.amazonaws.com'.

In the case of minio this should point at the appropriate endpoint for the minio server along with the port being used, for example http://40.114.110.201:9000/.  If you wish to use HTTPS to increase security the runners deployed must have the appropriate root certificates installed and the certs on your minio server setup to reference one of the publically well known certificate authorities.

### experiment ↠ config ↠ storage ↠ bucket

The bucket variable denotes the bucket name being used and should be homed in the region that is configured using the endpoint.  In the case of AWS any AWS style environment variables captured in the environment variables section, 'env', will be used for authentication.

When the experiment is being initiated within the StudioML client then local AWS environment variables will be used.  When the bucket is accessed by the runner then the authentication details captured inside this json payload will be used to download and upload any data.

### experiment ↠ config ↠ storage ↠ authentication

Not yet widely supported across the database types this variable supports either none, firebase, or github.  Currently its application is only to the gcloud, amnd firebase storage.  The go runner is intended for non vendor dependent implementations and uses the env variable seetings for the AWS authentication currently.  It is planned in the future that the authentication would make use of shortlived tokens using this field.

### experiment ↠ config ↠ resources\_needed

This section details the minimum hardware requirements needed to run the experiment.

Values of the parameters in this section are either integers or integer units.  For units suffixes can include Mb, Gb, Tb for megabytes, gigabytes, or terrabytes.

It should be noted that GPU resources are not virtualized and the requirements are hints to the scheduler only.  A project over committing resources will only affects its own experiments as GPU cards are not shared across projects.  CPU and RAM are virtualized by the container runtime and so are not as prone to abuse.

### experiment ↠ config ↠ resources\_needed ↠ hdd

The minimum disk space required to run the experiment.

### experiment ↠ config ↠ resources\_needed ↠ cpus

The number of CPU Cores that should be available for the experiments.  Remember this value does not account for the power of the CPU.  Consult your cluster operator or administrator for this information and adjust the number of cores to deal with the expectation you have for the hardware.

### experiment ↠ config ↠ resources\_needed ↠ ram

The amount of free CPU RAM that is needed to run the experiment.  It should be noted that StudioML is design to run in a co-operative environment where tasks being sent to runners adequately describe their resource requirements and are scheduled based upon expect consumption.  Runners are free to implement their own strategies to deal with abusers.

### experiment ↠ config ↠ resources\_needed ↠ gpus

gpus are counted as slots using the relative throughput of the physical hardware GPUs. GTX 1060's count as a single slot, GTX1070 is two slots, and a TitanX is considered to be four slots.  GPUs are not virtualized and so the go runner will pack the jobs from one experiment into one GPU device based on the slots.  Cards are not shared between different experiments to prevent noise between projects from affecting other projects.  If a project exceeds its resource consumption promise it will only impact itself.

### experiment ↠ config ↠ resources\_needed ↠ gpuMem

The amount on onboard GPU memory the experiment will require.  Please see above notes concerning the use of GPU hardware.

### experiment ↠ config ↠ env

This section contains a dictionary of environmnet variables and their values.  Prior to the experiment being initiated by the runner the environment table will be loaded.  The envrionment table is current used for AWS authentication for S3 access and so this section should contain as a minimum the AWS_DEFAULT_REGION, AWS_ACCESS_KEY_ID, and AWS_SECRET_ACCESS_KEY variables.  In the future the AWS credentials for the artifacts will be obtained from the artifact block.

### experiment ↠ config ↠ cloud ↠ queue ↠ rmq

This variable will contain the rabbitMQ URI and configuration parameters if rabbitMQ was used by the system to queue this work.  The runner will ignore this value if it is passed through as it gets its queue information from the runner configuration store.

# Report messages

This section describes the report message format sent using response queues.

Response queues offer a way of receiving timely notifications of significant events generated by runners.  In addition to events specific to requests being serviced by runners these queues are also used to send logging information from individual requests, or experients, to the listener.

Report messages encapsulate runner queue servicing information, information about the state of individual tasks, logging information and other data.

Response queues are intended to be read by a single listener.  Queues retain events if the listener is busy and not actively pulling messages,until they are read by the listerner.

The listener is envisioned as being the software service that is orchestrating ENN Generations consisting of multiple tasks.  In the case of the StudioML completion service sending requests the listener would also be a part of the service observing the results of experiments and dispatching further requests for additional experiments.

Response queues use message level encryption documented in the [Report Message Encryption](docs/message_privacy.md#report_message_encryption) document.  Encryption is mandatory as messages sent for logging purposes can contain environment variables and other information that experimenters may have intentionally, or inadvertantlyincluded in their StudioML task requests.

## Queuing

The response queue feature is enabling through the creation of a queue that has a suffix of '\_response' prior to creating the main queue for the tasks.  Runners observing the presence of the response queue as they dispatch StudioML tasks will begin to generate report messages and publish them to the response queue.

Experimenters using the completion service or any other value added SDK offerings for StudioML should examine the documentation of their respective toolkits on how response queues can be activated, you also have the option to create this queue locally within your own code, pull report messages, decrypt them and use them.  An example of using python to do this can be seen inside the runner code repository and can be found at [response catcher example](assets/response_catcher).

## Message Format

Encrypted messages sent using response queues are marshalled as protobuf messages encoded using the JSON mapping documented at, (Protobuf 3 JSON mapping)[https://developers.google.com/protocol-buffers/docs/proto3#json].

The google protobuf libraries contain standardized decoders/encoders for this format across multiple lanauges.

The proto file containing the report message definition can be found at (report.proto)[proto/report.proto)

### report

The report message is the top level message.

Message fields using strings are encoded using the Google defined 'google.protobuf.StringValue' in order to allow empty strings to be differentiated from unspecified strings.

Times are represented using the Google defined 'google.protobuf.Timestamp'.

### report ↠ time

This time value represents the time at the point the message was prepared for transmission prior to its encryption

### report ↠ experiment_id

The unique ID of this experiment assigned by the experimenter.  If the report has no associated or known experiment id this field will not be present. This will occour, for example, when the runner is examining the request queue for prospective tasks.  The value for this field will originate from the original requests experiment_id and can be used for correlation.

### report ↠ unique_id

A unique ID that was generated by the runners attempt to run the experiment. This value is dervied in part using the experiment_id but will change between each attempt. If the report has no associated or known experiment id this field will not be present.

### report ↠ executor_id

A unique ID denoting the host, pod, or node that the event occured on, or alternatively an experiment attempt is being performed on.

This value is useful when making queries about an experiment that did not complete or failed for an unknown reason.

### report ↠ payload

The payload is a one of field that contains the specific fields generated by the runners application layer.

The proto_any field allows for custom third party extensions.

The text field is a free format message encoded as UTF-8.  Typically this report is of use to developers and internal engineers.

The logging field contains a structured logging entry, please see the report ↠ logging field descriptions for further details

The progress field contains information about the state of an experiment attempt currently underway within the runner. please see the report ↠ progress field descriptions for futher information.

### report ↠ logging ↠ time

This field contains the timestamp for the time the logging event was generated, it is expected it will be different from the report message timestamp.

### report ↠ logging

One of the mutually exclusive options of the payload within the report message.

### report ↠ logging ↠ severity

This field contains an enumerated value of the severity level of this log message.  Severity levels are assigned names based on the language being used and the code generator, please refer to your languages documentation for the protoc tool.  Severity levels have the following values:

. Default = 0  The log entry has no assigned severity level.

. Trace = 100  Trace information, very detailed streams of the code executed within the runner of use only to engineering.

. Debug = 200  Debug information of general use to the support organization triaging any potential issues in relation to the runner software.

. Info = 300  Routine information, such as ongoing execution status or performance.

. Warning = 400  Normal but significant events, such as start up, shut down, certain features being ignored/disabled, or a configuration change.

. Error = 500  Error events are likely to cause problems, or failures that are not fatal.

. Fatal = 600  One or more systems are unusable and likely fatal problems are encountered.


### report ↠ logging ↠ message

A human readable text message, formatted as UTF-8.

### report ↠ logging ↠ fields

An unordered collection of key value pairs that represent values with well known names providing meaning context to the log message.

### report ↠ progress

One of the mutually exclusive options of the payload within the report message.

### report ↠ progress ↠ time

This field contains the timestamp for the time the progress event occurred, it is expected it will be different from the report message timestamp.

### report ↠ progress ↠ json

In the event that the task being executed emitts a single line json fragment this field will contain the fragment.  For more information please review the [metadata documentation](docs/metadata.md#JSON-document).

### report ↠ progress ↠ state

This field contains an enumerated value describing the state of any associated task in relation to thismessage.  Enumerations  are assigned names based on the language being used and the code generator, please refer to your languages documentation for the protoc tool.  The state can have the following values:

. Prestart = 0 The Task is in an intialization phase and has not started, or there is no task associated with this message.

. Started = 1  The task is in a starting state, optional transitional state

. Stopping = 2  The task is stopping, optional transitional state

. Failed = 20  Terminal state indicating the task failed

. Success = 21  Terminal state indicating the task completed successfully


### report ↠ progress ↠ error

An optional nested message detailing any error condition associated with the state of the runner or task.

### report ↠ progress ↠ error ↠ msg

This field contains a runner, or task application defined error message.

### report ↠ progress ↠ error ↠ code

This field contains a runner, or task application defined signed integer value for the error code.

Copyright © 2019-2021 Cognizant Digital Business, Evolutionary AI. All rights reserved. Issued under the Apache 2.0 license.
