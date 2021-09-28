GPU Allocation

When using StudioML gpus specified in the resource section of the config.yaml being used are typically done using slots.

```
resources_needed:
    cpus: 1
    gpus: 1
    hdd: 3gb
    ram: 2gb
    gpuMem: 2gb
    gpuCount: 1
```

When using cards such as the GTX 1050, or GTX 1060 then the slots assigned will be in single digit increments.  When using cloud and data center deployments where higher powered cards are being used the gpu value when expressed as a 1 will result in usage of the higher powered GTX 1070 and GTX 1080 without any problems.

However as the power of the cards deployed within your infrastructure increases it becomes more important to express the gpus values as slots representing the desired upper bound.  For example

|Slots|Card Types|
|---|---|
|2|GTX 1050, GTX 1060|
|2|GTX 1070, GTX 1080|
|4|Titan X, Tesla P40|
|8|Tesla P100|
|16|Tesla V100|
|24|Amphere A100|

If the number of slots you define is above what is available then the system will attempt to create your desired configuration with the gpuCount.

Copyright &copy 2019-2020 Cognizant Digital Business, Evolutionary AI. All rights reserved. Issued under the Apache 2.0 license.
