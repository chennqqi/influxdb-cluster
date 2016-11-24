# influxdb-cluster
A plugin for influxdb which make it distributed. 
In this framework, we have two different nodes. Then main part of this sytem 
is `Meta` and second part of this system is `Data`. They both have different role 
in this system. 

## Meta Nodes

Meta nodes hold the following things as meta data:
1. all nodes' url and their role( meta or data). 

2. All databases and tetention policies that existe in the cluster

3. All shards and shardGroups, and on what nodes exist. To be simpple, ShardLocation
need stored in meta node.

4. Cluster users and their permissions

5. All continuous queries.

## Data Nodes

1. Measurements
2. Tags keys and values
3. Field Keys and Values

All the data is organized b `database/rention_policy/shard_id`. By default, the parent 
directory of this is `/var/lib/influxdb/data`
