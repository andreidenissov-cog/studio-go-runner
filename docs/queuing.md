# Motivation

The runner is designed to allow projects within studioml to have their experiments run across shared infrastructure in an equitable manner, at least initially at the machine level.

To do this the runner, when started, will query the credentials being used to access Google PubSub to discover the name of the project with which it is to be associated.  
Projects identify a group of experiments that share a single financially responsible party.  Projects can be thought of as a type of tenant identifier.  

Within a project google PubSub subscriptions act as a queues of experiments being submitted to the studioml eco system.

Current the runner on starting reads the credentials supplied for Google PubSub and will use it to identify the projects used by the runner.  The runner will then repeatedly scan the project(s) looking for queues that it can match with the GPU and CPU hardware it has available.

As GPU resources become available the runner will associate those resources with a queue of experiments until such time as the queue is drained at which point the GPU will be returned to the free pool and the runner will look for a new queue for that GPU.

Should no GPU resources be available but there are CPU resources the runner will begin looking for queues that contain work that is CPU only and assign CPU resources to those queues.

Queued experiments that have been queried once are assumed to contain the same resource demands for all future experiments and the runner will assume this when selecting which queues to poll for work.

studioml users using this runner can indicate that queues are no longer producing work by deleting their topics.


Copyright &copy 2019-2020 Cognizant Digital Business, Evolutionary AI. All rights reserved. Issued under the Apache 2.0 license.
