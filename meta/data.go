package meta

import (
	"fmt"
	"sort"
	"time"

	"github.com/gogo/protobuf/proto"
	"github.com/influxdata/influxdb"
	"github.com/influxdata/influxdb/influxql"
	"github.com/influxdata/influxdb/services/meta"
	internal "github.com/zhexuany/influxdb-cluster/meta/internal"
)

//go:generate protoc --gogo_out=. internal/meta.proto

const (
	// DefaultRetentionPolicyReplicaN is the default value of RetentionPolicyInfo.ReplicaN.
	DefaultRetentionPolicyReplicaN = 1

	// DefaultRetentionPolicyDuration is the default value of RetentionPolicyInfo.Duration.
	DefaultRetentionPolicyDuration = time.Duration(0)

	// DefaultRetentionPolicyName is the default name for auto generated retention policies.
	DefaultRetentionPolicyName = "autogen"

	// MinRetentionPolicyDuration represents the minimum duration for a policy.
	MinRetentionPolicyDuration = time.Hour
)

// Data represents the top level collection of all metadata.
type Data struct {
	*meta.Data
	MetaNodes meta.NodeInfos
	DataNodes meta.NodeInfos
	Users     []UserInfo
	Roles     RoleInfos
}

// Clone returns a copy of data with a new version.
func (data *Data) Clone() *Data {
	other := *data

	// Copy nodes.
	if data.DataNodes != nil {
		other.DataNodes = make([]meta.NodeInfo, len(data.DataNodes))
		for i := range data.DataNodes {
			other.DataNodes[i] = data.DataNodes[i].clone()
		}
	}

	if data.MetaNodes != nil {
		other.MetaNodes = make([]meta.NodeInfo, len(data.MetaNodes))
		for i := range data.MetaNodes {
			other.MetaNodes[i] = data.MetaNodes[i].clone()
		}
	}

	other.Roles = data.CloneRoles()
	other.Users = data.CloneUsers()

	return &other
}

func (data *Data) MetaNode(id uint64) *meta.NodeInfo {
	for i := range data.MetaNodes {
		if data.MetaNodes[i].ID == id {
			return &data.MetaNodes[i]
		}
	}
	return nil
}

func (data *Data) CreateMetaNode(host, tcpHost string) error {
	// Ensure a node with the same host doesn't already exist.
	for _, n := range data.DataNodes {
		if n.TCPHost == tcpHost {
			return ErrNodeExists
		}
	}

	// If an existing meta node exists with the same TCPHost address,
	// then these nodes are actually the same so re-use the existing ID
	var existingID uint64
	for _, n := range data.MetaNodes {
		if n.TCPHost == tcpHost {
			existingID = n.ID
			break
		}
	}

	// We didn't find an existing node, so assign it a new node ID
	if existingID == 0 {
		data.MaxNodeID++
		existingID = data.MaxNodeID
	}

	// Append new node.
	data.MetaNodes = append(data.MetaNodes, NodeInfo{
		ID:      existingID,
		Host:    host,
		TCPHost: tcpHost,
	})
	sort.Sort(NodeInfos(data.MetaNodes))

	return nil

}

// SetMetaNode adds a meta node with a pre-specified nodeID.
// this should only be used when the cluster is upgrading from 0.9 to 0.10
func (data *Data) SetMetaNode(nodeID uint64, host, tcpHost string) error {
	// Ensure a node with the same host doesn't already exist.
	for _, n := range data.MetaNodes {
		if n.Host == host {
			return ErrNodeExists
		}
	}

	//call CreateMetaNode
	// Append new node.
	data.MetaNodes = append(data.MetaNodes, NodeInfo{
		ID:      nodeID,
		Host:    host,
		TCPHost: tcpHost,
	})

	return nil
}

func (data *Data) DeleteMetaNode(id uint64) error {
	var nodes []NodeInfo

	// Remove the data node from the store's list.
	for _, n := range data.MetaNodes {
		if n.ID != id {
			nodes = append(nodes, n)
		}
	}

	if len(nodes) == len(data.MetaNodes) {
		return ErrNodeNotFound
	}

	data.MetaNodes = nodes

	// Remove node id from all shard infos
	for di, d := range data.Databases {
		for ri, rp := range d.RetentionPolicies {
			for sgi, sg := range rp.ShardGroups {
				var (
					nodeOwnerFreqs = make(map[int]int)
					orphanedShards []ShardInfo
				)
				// Look through all shards in the shard group and
				// determine (1) if a shard no longer has any owner
				// (orphaned); (2) if all shards in the shard group
				// are orphaned; and (3) the number of shards in this
				// group owned by each data node in the cluster.
				for si, s := range sg.Shards {
					// Track of how many shards in the group are
					// owned by each data node in the cluster.
					var nodeIdx = -1
					for i, owner := range s.Owners {
						if owner.NodeID == id {
							nodeIdx = i
						}
						nodeOwnerFreqs[int(owner.NodeID)]++
					}

					if nodeIdx > -1 {
						// Data node owns shard, so relinquish ownerhip
						// and set new owner on the shard.
						s.Owners = append(s.Owners[:nodeIdx], s.Owners[nodeIdx+1:]...)
						data.Databases[di].RetentionPolicies[ri].ShardGroups[sgi].Shards[si].Owners = s.Owners
					}

					// Shard no longer owned. Will need reassigning
					// an owner.
					if len(s.Owners) == 0 {
						orphanedShards = append(orphanedShards, s)
					}
				}

				// Mark the shard group as deleted if it has no shards,
				// or all of its shards are orphaned.
				if len(sg.Shards) == 0 || len(orphanedShards) == len(sg.Shards) {
					data.Databases[di].RetentionPolicies[ri].ShardGroups[sgi].DeletedAt = time.Now().UTC()
					continue
				}

				// Reassign any orphaned shards. Delete the node we're
				// dropping from the list of potential new owner.
				delete(nodeOwnerFreqs, int(id))

				for _, orphan := range orphanedShards {
					newOwnerID, err := newShardOwner(orphan, nodeOwnerFreqs)
					if err != nil {
						return err
					}

					for si, s := range sg.Shards {
						if s.ID == orphan.ID {
							sg.Shards[si].Owners = append(sg.Shards[si].Owners, ShardOwner{NodeID: newOwnerID})
							data.Databases[di].RetentionPolicies[ri].ShardGroups[sgi].Shards = sg.Shards
							break
						}
					}

				}
			}
		}
	}
	return nil
}

// DataNode returns a node by id.
func (data *Data) DataNode(id uint64) *NodeInfo {
	for i := range data.DataNodes {
		if data.DataNodes[i].ID == id {
			return &data.DataNodes[i]
		}
	}
	return nil
}

// CreateDataNode adds a node to the metadata.
func (data *Data) CreateDataNode(host, tcpHost string) error {
	// Ensure a node with the same host doesn't already exist.
	for _, n := range data.DataNodes {
		if n.TCPHost == tcpHost {
			return ErrNodeExists
		}
	}

	// If an existing meta node exists with the same TCPHost address,
	// then these nodes are actually the same so re-use the existing ID
	var existingID uint64
	for _, n := range data.MetaNodes {
		if n.TCPHost == tcpHost {
			existingID = n.ID
			break
		}
	}

	// We didn't find an existing node, so assign it a new node ID
	if existingID == 0 {
		data.MaxNodeID++
		existingID = data.MaxNodeID
	}

	// Append new node.
	data.DataNodes = append(data.DataNodes, NodeInfo{
		ID:      existingID,
		Host:    host,
		TCPHost: tcpHost,
	})
	sort.Sort(NodeInfos(data.DataNodes))

	return nil
}

func (data *Data) UpdateDataNode(nodeID uint64, host, tcpHost string) error {
	for _, n := range data.DataNodes {
		if n.ID == nodeID {
			n.Host = host
			n.TCPHost = tcpHost
			break
		}
	}
	return nil
}

// DeleteDataNode removes a node from the Meta store.
//
// If necessary, DeleteDataNode reassigns ownerhip of any shards that
// would otherwise become orphaned by the removal of the node from the
// cluster.
func (data *Data) DeleteDataNode(id uint64) error {
	var nodes []NodeInfo

	// Remove the data node from the store's list.
	for _, n := range data.DataNodes {
		if n.ID != id {
			nodes = append(nodes, n)
		}
	}

	if len(nodes) == len(data.DataNodes) {
		return ErrNodeNotFound
	}
	data.DataNodes = nodes

	return nil
}
func (data *Data) MarshalBinary() {
	return proto.Marshal(data.marshal())
}

// marshal serializes to a protobuf representation.
func (data *Data) marshal() *internal.Data {
	pb := &internal.Data{
		Term:      proto.Uint64(data.Term),
		Index:     proto.Uint64(data.Index),
		ClusterID: proto.Uint64(data.ClusterID),

		MaxNodeID:       proto.Uint64(data.MaxNodeID),
		MaxShardGroupID: proto.Uint64(data.MaxShardGroupID),
		MaxShardID:      proto.Uint64(data.MaxShardID),
	}

	pb.DataNodes = make([]*internal.NodeInfo, len(data.DataNodes))
	for i := range data.DataNodes {
		pb.DataNodes[i] = data.DataNodes[i].marshal()
	}

	pb.MetaNodes = make([]*internal.NodeInfo, len(data.MetaNodes))
	for i := range data.MetaNodes {
		pb.MetaNodes[i] = data.MetaNodes[i].marshal()
	}

	pb.Databases = make([]*internal.DatabaseInfo, len(data.Databases))
	for i := range data.Databases {
		pb.Databases[i] = data.Databases[i].marshal()
	}

	pb.Users = make([]*internal.UserInfo, len(data.Users))
	for i := range data.Users {
		pb.Users[i] = data.Users[i].marshal()
	}

	//RoleInfo marshal TODO
	return pb
}

// UnmarshalBinary decodes the object from a binary format.
func (data *Data) UnmarshalBinary(buf []byte) error {
	var pb internal.Data
	if err := proto.Unmarshal(buf, &pb); err != nil {
		return err
	}
	data.unmarshal(&pb)
	return nil
}

// unmarshal deserializes from a protobuf representation.
func (data *Data) unmarshal(pb *internal.Data) {
	//Call UmarshalBianry
	data.Term = pb.GetTerm()
	data.Index = pb.GetIndex()
	data.ClusterID = pb.GetClusterID()

	data.MaxShardGroupID = pb.GetMaxShardGroupID()
	data.MaxShardID = pb.GetMaxShardID()

	data.Databases = make([]DatabaseInfo, len(pb.GetDatabases()))
	for i, x := range pb.GetDatabases() {
		data.Databases[i].unmarshal(x)
	}

	//RoleInfo TODO unmarshal
	data.Users = make([]UserInfo, len(pb.GetUsers()))
	for i, x := range pb.GetUsers() {
		data.Users[i].unmarshal(x)
	}
}

// CreateShardGroup creates a shard group on a database and policy for a given timestamp.
func (data *Data) CreateShardGroup(database, policy string, timestamp time.Time) error {
	// Ensure there are nodes in the metadata.
	if len(data.DataNodes) == 0 {
		return nil
	}

	// Find retention policy.
	rpi, err := data.RetentionPolicy(database, policy)
	if err != nil {
		return err
	} else if rpi == nil {
		return influxdb.ErrRetentionPolicyNotFound(policy)
	}

	// Verify that shard group doesn't already exist for this timestamp.
	if rpi.ShardGroupByTimestamp(timestamp) != nil {
		return nil
	}

	// Require at least one replica but no more replicas than nodes.
	replicaN := rpi.ReplicaN
	if replicaN == 0 {
		replicaN = 1
	} else if replicaN > len(data.DataNodes) {
		replicaN = len(data.DataNodes)
	}

	// Determine shard count by node count divided by replication factor.
	// This will ensure nodes will get distributed across nodes evenly and
	// replicated the correct number of times.
	shardN := len(data.DataNodes) / replicaN

	//TODO finished generatedShards
	// Create the shard group.
	data.MaxShardGroupID++
	sgi := meta.ShardGroupInfo{}
	sgi.ID = data.MaxShardGroupID
	sgi.StartTime = timestamp.Truncate(rpi.ShardGroupDuration).UTC()
	sgi.EndTime = sgi.StartTime.Add(rpi.ShardGroupDuration).UTC()

	sgi.Shards = data.generatedShards(shardN)
	// Assign data nodes to shards via round robin.
	// Start from a repeatably "random" place in the node list.
	nodeIndex := int(data.Index % uint64(len(data.DataNodes)))
	for i := range sgi.Shards {
		si := &sgi.Shards[i]
		for j := 0; j < replicaN; j++ {
			nodeID := data.DataNodes[nodeIndex%len(data.DataNodes)].ID
			si.Owners = append(si.Owners, meta.ShardOwner{NodeID: nodeID})
			nodeIndex++
		}
	}

	// Retention policy has a new shard group, so update the policy. Shard
	// Groups must be stored in sorted order, as other parts of the system
	// assume this to be the case.
	rpi.ShardGroups = append(rpi.ShardGroups, sgi)
	sort.Sort(meta.ShardGroupInfos(rpi.ShardGroups))

	return nil
}

func (data *Data) gcd() {

}

func (data *Data) generatedShards(shardN int) *meta.ShardGroupInfos {
	// Create shards on the group.
	shards = make([]meta.ShardInfo, shardN)
	for i := range shards {
		data.MaxShardID++
		shards[i] = meta.ShardInfo{ID: data.MaxShardID}
	}

	return s
}

func (data *Data) TruncateShardsGrops(sg *meta.ShardGroupInfo) error {

}

func (data *Data) AddPendingShardOwner(id uint64) error {

}

func (data *Data) RemovePendingShardOwner(id uint64) error {

}

//ShardLocation return NodeInfos which is the o of the Shard
func (data *Data) ShardLocation(shardID uint64) (*meta.ShardInfo, error) {
	for dbidx, dbi := range data.Databases {
		for rpidx, rpi := range dbi.RetentionPolicies {
			for sgidx, sg := range rpi.ShardGroups {
				for sidx, s := range sg.Shards {
					//found such shards, return shards
					if s.ID == shardID {
						return s, nil
					}
				}
			}
		}
	}
	//does not find any shards assoicated with this shardID, just reutn nil, nil
	return nil, fmt.Errorf("failed to find shards assoicated with %d", shardID)
}

// UpdateShard will update ShardOwner of a Shard according to ShardID
func (data *Data) UpdateShard(shardID uint64, newOwners []meta.ShardOwner) {
	for dbidx, dbi := range data.Databases {
		for rpidx, rpi := range dbi.RetentionPolicies {
			for sgidx, sg := range rpi.ShardGroups {
				for sidx, s := range sg.Shards {
					//found such shards, return shards
					if s.ID == shardID {
						s.Owners = newOwners
					}
				}
			}
		}
	}
	return fmt.Errorf("Failed to find Shard assoicated with shard ID", shardID)
}

// AddShardOwner will update a shards labelled by shardID in this node if such shards ownby this newly adding node
func (data *Data) AddShardOwner(shardID, nodeID uint64) error {
	si, err := data.ShardLocation(shardID)
	if err != nil {
		if !si.OwnedBy(nodeID) {
			o := si.Owners
			o = append(o, meta.ShardOwner{NodeID: nodeID})
			sort.Sort(o)
			return data.UpdateShard(shardID, o)
		}
	}
	return err
}

// RemoveShardOwner will remove all shards in this node if such shard owned by this node
func (data *Data) RemoveShardOwner(shardID, nodeID uint64) error {
	si, err := data.ShardLocation(shardID)
	if err != nil {
		if si.OwnedBy(nodeID) {
			O, _ := data.PruneShard(si, nodeID)
			data.UpdateShard(shardID, o)
		}
	}
	return err
}

func (data *Data) PruneShard(si meta.ShardInfo, nodeID uint64) ([]meta.ShardOwner, error) {
	found := -1
	for i, o := range si.Owners {
		if o.NodeID == nodeID {
			found = i
			break
		}
	}

	if found != -1 {
		copy(si.Owners[found:], si.Owners[found+1:])
		si.Owners[len(si.Owners)-1] = nil
		si.Owners = si.Owners[:len(si.Owners)-1]
		return si.Owners, nil
	}
	return nil, fmt.Errorf("failed to find shard owner %d", nodeID)
}

//TODO all role can be waited until cluster works fine
func (data *Data) CreateRole() error {
	//make map
}

func (data *Data) DropRole(role *RoleInfo) error {
	ridx := -1
	for i, r := range data.Roles {
		if r == role {
			ridx = i
			break
		}
	}

	if rdx != -1 {
		return
	}
	data.Roles = copy
}

func (data *Data) Role(name string) *RoleInfo {
	// for i := rang data.Roles {
	// 	if data.Roles[i].Name == name {
	// 		return &data.Roles[i]
	// 	}
	// }
	// return nil
}

func (data *Data) role() {

}

func (data *Data) AddRoleUsers() {
}

func (data *Data) RemoveRoleUsers() {
	//roles.RemoveUsers
}

func (data *Data) AddRolePermissions() {

}

func (data *Data) RemoveRolePermissions() {

}

func (data *Data) ChangeRoleName() {

}

// User returns a user by username.
func (data *Data) User(username string) *UserInfo {
	for i := range data.Users {
		if data.Users[i].Name == username {
			return &data.Users[i]
		}
	}
	return nil
}
func (data *Data) user() {

}

// CreateUser creates a new user.
func (data *Data) CreateUser(name, hash string, admin bool) error {
	// Ensure the user doesn't already exist.
	if name == "" {
		return ErrUsernameRequired
	} else if data.User(name) != nil {
		return ErrUserExists
	}

	// Append new user.
	data.Users = append(data.Users, UserInfo{
		Name:  name,
		Hash:  hash,
		Admin: admin,
	})

	return nil
}

// DropUser removes an existing user by name.
func (data *Data) DropUser(name string) error {
	for i := range data.Users {
		if data.Users[i].Name == name {
			data.Users = append(data.Users[:i], data.Users[i+1:]...)
			return nil
		}
	}
	return ErrUserNotFound
}

func (data *Data) SetUserPassword(pass string) error {

}

func (data *Data) AddUserPermissions() error {

}

func (data *Data) RemoveUserPermissions() error {

}

func (data *Data) UserPermissions() *ScopedPermissions {

}

// UserPrivilege gets the privilege for a user on a database.
func (data *Data) UserPrivilege(name, database string) (*influxql.Privilege, error) {
	ui := data.User(name)
	if ui == nil {
		return nil, ErrUserNotFound
	}

	for db, p := range ui.Privileges {
		if db == database {
			return &p, nil
		}
	}

	return influxql.NewPrivilege(influxql.NoPrivileges), nil
}

// UserPrivileges gets the privileges for a user.
func (data *Data) UserPrivileges(name string) (map[string]influxql.Privilege, error) {
	ui := data.User(name)
	if ui == nil {
		return nil, ErrUserNotFound
	}

	return ui.Privileges, nil
}

func (data *Data) OSSUser() {
	users := data.Users()
	for _, usr := range users {
		data.UserPermissions(usr)
		//meta.AddAdminPermissions
		data.Authorized()
	}
}

func (data *Data) OSSAdminExists() {
	data.Authorized()
}

func (data *Data) CloneRoles() []RoleInfo {

}

func (data *Data) CloneUsers() []UserInfo {
	if data.Users == nil {
		return nil
	}

	usr := make([]UserInfo, len(data.Users))
	for i := range data.Users {
		usr[i] = data.Users[i].clone()
	}

	return usr

}

func (data *Data) Authorized(userName string) {
	usr := data.User(userName)
	//iter a map
	permission := data.hasPermissions(usr)
}

func (data *Data) hasPermissions(usr UserInfo) bool {
	//ScopedPermissions.Contains(usr)
	//RoleInfo.Authorized
}

//TODO finish this until we have a demo to run
func (data *Data) ImportData(buf []bytes) error {
	// other := Data{}
	// if err := other.UnmarshalBinary(buf); err != nil {
	// 	return err
	// }

	// // Restrict(other)
	// for dbidx, db := range data.Databases {
	// 	dbn := other.Database(db.Name)
	// 	if dbn == nil {
	// 		if err = other.CreateDatabase(db.Name); err != nil {
	// 			return err
	// 		}
	// 	}
	// 	for _, rpi := range db.RetentionPolicies {
	// 		other.CreateRetentionPolicy(dbn.Name, dbn.RetentionPolicy(rpi.Name), false)
	// 		data.generatedShards(rpi.ShardGroups)
	// 	}

	// }
	//sort
	//call gcd
}

type UserInfo struct {
	Name       string
	Hash       string
	Admin      bool
	Privileges ScopedPermissions
}

func (u *UserInfo) unmarshal(pb internal.UserInfo) error {
	u.Privileges.unmarshal()
}

func (u *UserInfo) InfluxDBUser() *UserInfo {

}

type PermissionsSet struct {
}

func (ps *PermissionsSet) Len() int {

}

func (ps *PermissionsSet) Swap(i, j int) {}

func (ps *PermissionsSet) Less() {}

func (ps *PermissionsSet) Clone()  {}
func (ps *PermissionsSet) Add()    {}
func (ps *PermissionsSet) Delete() {}

func (ps *PermissionsSet) Contains() {}

func (ps *PermissionsSet) Clone()  {}
func (ps *PermissionsSet) Delete() {}

type RoleInfo struct {
	Users []UserInfo
}

func (r *RoleInfo) clone() *RoleInfo {

}

func (r *RoleInfo) marshal() {

}

func (r *RoleInfo) unmarshal() {

}

func (r *RoleInfo) AddUsers(users []UserInfo) {
	for _, usr := range users {
		r.AddUser(usr)
	}
}

func (r *RoleInfo) RemoveUsers(users []UserInfo) {
	for _, usr := range users {
		r.RemoveUser(usr)
	}
}

func (r *RoleInfo) AddUser(user UserInfo) bool {
	r.Users = append(r.Users, user)
}

func (r *RoleInfo) RemoveUser(user UserInfo) {
	deleteIndex := -1
	for i, usr := range r.Users {
		if usr == user {
			deleteIndex = i
			break
		}
	}

	if deleteIndex == -1 {
		return
	}
	r.Users = append(r.Users[:deleteIndex], r.Users[deleteIndex+1:])
}

func (r *RoleInfo) HasUser(user UserInfo) bool {
	res := sort.Search(len(r.Users), func(i int) bool {
		return r.Users[i] == user
	})

	return res < len(r.Users) && r.Users[i] == user
}

type RoleInfos []RoleInfo

func (rs *RoleInfos) Len()        {}
func (rs *RoleInfos) Swap()       {}
func (rs *RoleInfos) Less()       {}
func (rs *RoleInfos) Authorized() {}
func (rs *RoleInfos) Len()        {}

type uint64arr []uint64

func (u *uint64arr) Len()  {}
func (u *uint64arr) Swap() {}
func (u *uint64arr) Less() {}

type ScopedPermissions struct {
}

func (scp *ScopedPermissions) unmarshal(buf []byte) error {
	//call add
}

func (scp *ScopedPermissions) Clone() *ScopedPermissions {

}

func (scp *ScopedPermissions) Add() error {

}

func (scp *ScopedPermissions) Delete() error {

}

func (scp *ScopedPermissions) Contains() bool {

}

func (scp *ScopedPermissions) Matches() bool {

}
