# Dedicated Request Serving Node Autoscaler

This is a simple autoscaler that ensures that a minimum and maximum number of nodes are available for request serving components.

If available nodes < minimum, machinesets are selected to be scaled up. If available nodes > maximum, machinesets are selected to be scaled down.

```
Usage:
  autoscaler [flags]

Flags:
  -h, --help                           help for autoscaler
      --maximum-warm int               Maximum warm sets of nodes (number of 2-node groups from different zones for a HostedCluster) (default 8)
      --minimum-warm int               Minimum warm sets of nodes (number of 2-node groups from different zones for a HostedCluster) (default 5)
      --scale-down-interval duration   Interval between reconciles for scaling down (specify quantity and unit ie. 30s, 2m, etc) (default 30s)
      --scale-up-interval duration     Interval between reconciles for scaling up (specify quantity and unit ie. 30s, 2m, etc) (default 10s)
```
