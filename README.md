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

## Writes in a Cluster
### introduction
Storgae in a clustr is not a trival question to answer. 
We have to deal with replication factor which brings the high availability to 
system even for node crashed or network participation. 
### Shards Groups
All data stored in cluster as a form of `Shards`.  Consering the replication factor is `x` and there is
`n` node available in cluster. If we assume there are `m` shrads has to be written into cluster, then for every node, `nm/x` shards
will be written into cluster. 

When a write comes in with values that have a timestamp, we first determine which `ShardGroup` that this write goes to. After this, 
we take the concatatention of `measurement` and `tagset` as out key and hash such key for bucketing into the correct shard. In Go, it will
be the following.

~~~go
// key is measurement + tagset
// shardGroup is the group for the values based on timestamp
// hash with fnv and then bucket
shard := shardGroup.shards[fnv.New64a(key) % len(shardGroup.Shards)]
~~~

There are multiple implications to this scheme for determining where data lives in a cluster. 
First, for any given metaseries all data on any given day will exist in a single shard, and 
thus only on those servers hosting a copy of that shard. Second, once a shard group is created, 
adding new servers to the cluster won’t scale out write capacity for that shard group. 

The replication is fixed when the shard group is created. However, there is a method for expanding 
writes in the current shard group (i.e. today) when growing a cluster. 
The current shard group can be truncated to stop at the current time using `influxd-ctl truncate-shards`. 
This immediately closes the current shard group, forcing a new shard group to be created. 
That new shard group will inherit the latest retention policy and data node changes and 
will then copy itself appropriately to the newly available data nodes. 
Run `influxd-ctl truncate-shards help` for more information on the command.


## Queries in a Cluster

Queries in a cluster are distributed based on the time range being queried and the replication factor of the data. 
For example if the retention policy has a replication factor of 4, the coordinating data node receiving the query 
randomly picks any of the 4 data nodes that store a replica of the shard(s) to receive the query. If we assume that 
the system has shard durations of one day, then for each day of time covered by a query the coordinating node will 
select one data node to receive the query for that day. The coordinating node will execute and fulfill the query 
locally whenever possible. If a query must scan multiple shard groups (multiple days in our example above), 
the coordinating node will will forward queries to other nodes for shard(s) it does not have locally. 
The queries are forwarded in parallel to scanning its own local data. The queries are distributed to as 
many nodes as required to query each shard group once. As the results come back from each data node, 
the coordinating data node combines them into the final result that gets returned to the user.


