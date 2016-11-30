package meta

import (
	"bytes"
	crand "crypto/rand"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"http"
	"io"
	"io/ioutil"
	"log"
	"math"
	"math/rand"
	"net/http"
	"os"
	"sort"
	"sync"
	"time"

	"github.com/influxdata/influxdb"
	"github.com/influxdata/influxdb/influxql"
	"github.com/influxdata/influxdb/meta"
	"github.com/zhexuany/influxdb-cluster/meta/internal"

	"github.com/gogo/protobuf/proto"
	"golang.org/x/crypto/bcrypt"
)

const (
	// errSleep is the time to sleep after we've failed on every metaserver
	// before making another pass
	errSleep = time.Second

	// maxRetries is the maximum number of attemps to make before returning
	// a failure to the caller
	maxRetries = 10

	// SaltBytes is the number of bytes used for salts
	SaltBytes = 32
)

var (
	// ErrServiceUnavailable is returned when the meta service is unavailable.
	ErrServiceUnavailable = errors.New("meta service unavailable")

	// ErrService is returned when the meta service returns an error.
	ErrService = errors.New("meta service error")
)

// Client is used to execute commands on and read data from
// a meta service cluster.
type Client struct {
	tls    bool
	logger *log.Logger
	nodeID uint64

	mu          sync.RWMutex
	metaServers []string
	changed     chan struct{}
	closing     chan struct{}
	cacheData   *Data
	Path        string
	AuthInfo    string
	HTTPClient  *http.Client

	// Authentication cache.
	authCache map[string]authUser
}

type authUser struct {
	bhash string
	salt  []byte
	hash  []byte
}

// NewClient returns a new *Client.
func NewClient() *Client {
	return &Client{
		cacheData: &Data{},
		logger:    log.New(os.Stderr, "[metaclient] ", log.LstdFlags),
		authCache: make(map[string]authUser, 0),
	}
}

// Open a connection to a meta service cluster.
func (c *Client) Open() error {
	c.changed = make(chan struct{})
	c.closing = make(chan struct{})
	c.cacheData = c.retryUntilSnapshot(0)

	go c.pollForUpdates()

	return nil
}

// Close the meta service cluster connection.
func (c *Client) Close() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if t, ok := http.DefaultTransport.(*http.Transport); ok {
		t.CloseIdleConnections()
	}

	select {
	case <-c.closing:
		return nil
	default:
		close(c.closing)
	}

	return nil
}
func (c *Client) closed() error {

}

func (c *Client) doHTTP() {

}

func (c *Client) get() {

}

func (c *Client) post() {

}

func (c *Client) Leave() {

}

// GetNodeID returns the client's node ID.
func (c *Client) NodeID() uint64 { return c.nodeID }

// SetMetaServers updates the meta servers on the client.
func (c *Client) SetMetaServers(a []string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.metaServers = a
}

func (c *Client) CheckMetaServers() {

}

func (c *Client) Path() string {
	return c.Path
}

func (c *Client) SetPath(path string) {
	c.Path = path
}

func (c *Client) TLS() bool {
	return c.tls
}

// SetTLS sets whether the client should use TLS when connecting.
// This function is not safe for concurrent use.
func (c *Client) SetTLS(v bool) { c.tls = v }

func (c *Client) AuthInfo() {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.AuthInfo

}

func (c *Client) SetAuthInfo(authInfo string) {
	c.mu.Lock()
	c.AuthInfo = authInfo
	c.mu.Unlock()
}

func (c *Client) SetHTTPClient(httpClient *http.Client) {
	c.mu.Lock()
	c.HTTPClient = httpClient
	c.mu.Unlock()
}

// Ping will hit the ping endpoint for the metaservice and return nil if
// it returns 200. If checkAllMetaServers is set to true, it will hit the
// ping endpoint and tell it to verify the health of all metaservers in the
// cluster
func (c *Client) Ping(checkAllMetaServers bool) error {
	c.mu.RLock()
	server := c.metaServers[0]
	c.mu.RUnlock()
	url := c.url(server) + "/ping"
	if checkAllMetaServers {
		url = url + "?all=true"
	}

	resp, err := http.Get(url)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusOK {
		return nil
	}

	b, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return err
	}
	return fmt.Errorf(string(b))
}

// AcquireLease attempts to acquire the specified lease.
// A lease is a logical concept that can be used by anything that needs to limit
// execution to a single node.  E.g., the CQ service on all nodes may ask for
// the "ContinuousQuery" lease. Only the node that acquires it will run CQs.
// NOTE: Leases are not managed through the CP system and are not fully
// consistent.  Any actions taken after acquiring a lease must be idempotent.
func (c *Client) AcquireLease(name string) (l *Lease, err error) {
	for n := 1; n < 11; n++ {
		if l, err = c.acquireLease(name); err == ErrServiceUnavailable || err == ErrService {
			// exponential backoff
			d := time.Duration(math.Pow(10, float64(n))) * time.Millisecond
			time.Sleep(d)
			continue
		}
		break
	}
	return
}

func (c *Client) acquireLease(name string) (*Lease, error) {
	c.mu.RLock()
	servers := c.MetaServers()
	c.mu.RUnlock()

	for _, server := range servers {
		url := fmt.Sprintf("%s/lease?name=%s&nodeid=%d", c.url(server), name, c.nodeID)

		resp, err := http.Get(url)
		if err != nil {
			return nil, err
		}
		defer resp.Body.Close()

		switch resp.StatusCode {
		case http.StatusOK:
		case http.StatusConflict:
			err = errors.New("another node owns the lease")
		case http.StatusServiceUnavailable:
			return nil, ErrServiceUnavailable
		case http.StatusBadRequest:
			b, e := ioutil.ReadAll(resp.Body)
			if e != nil {
				return nil, e
			}
			return nil, fmt.Errorf("meta service: %s", string(b))
		case http.StatusInternalServerError:
			return nil, errors.New("meta service internal error")
		default:
			return nil, errors.New("unrecognized meta service error")
		}
	}
	// Read lease JSON from response body.
	b, e := ioutil.ReadAll(resp.Body)
	if e != nil {
		return nil, e
	}
	// Unmarshal JSON into a Lease.
	l := &Lease{}
	if e = json.Unmarshal(b, l); e != nil {
		return nil, e
	}

	return l, err
}

func (c *Client) setData(data *Data) {
	c.mu.Lock()
	c.cacheData = data
	c.mu.Unlock()
}

func (c *Client) data() *Data {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.cacheData
}

// ClusterID returns the ID of the cluster it's connected to.
func (c *Client) ClusterID() uint64 {
	c.mu.RLock()
	defer c.mu.RUnlock()

	return c.data().ClusterID
}

// Node returns a node by id.
func (c *Client) DataNode(id uint64) (*meta.NodeInfo, error) {
	for _, n := range c.data().DataNodes {
		if n.ID == id {
			return &n, nil
		}
	}
	return nil, ErrNodeNotFound
}

// DataNodes returns the data nodes' info.
func (c *Client) DataNodes() (meta.NodeInfos, error) {
	return c.data().DataNodes, nil
}

// CreateDataNode will create a new data node in the metastore
func (c *Client) CreateDataNode(httpAddr, tcpAddr string) (*meta.NodeInfo, error) {
	cmd := &internal.CreateDataNodeCommand{
		HTTPAddr: proto.String(httpAddr),
		TCPAddr:  proto.String(tcpAddr),
	}

	if err := c.retryUntilExec(internal.Command_CreateDataNodeCommand, internal.E_CreateDataNodeCommand_Command, cmd); err != nil {
		return nil, err
	}

	n, err := c.DataNodeByTCPHost(tcpAddr)
	if err != nil {
		return nil, err
	}

	c.nodeID = n.ID

	return n, nil
}

func (c *Client) ShardPendingOwners() {
	// c.data()
}

func (c *Client) RemovePendingShardOwner() {}

func (c *Client) CommitPendingShardOwner() {}

func (c *Client) AddShardOwner() {}

func (c *Client) RemoveShardOwner() {}

func (c *Client) UpdateDataNode() {}

// DataNodeByHTTPHost returns the data node with the give http bind address
func (c *Client) DataNodeByHTTPHost(httpAddr string) (*meta.NodeInfo, error) {
	nodes, _ := c.DataNodes()
	for _, n := range nodes {
		if n.Host == httpAddr {
			return &n, nil
		}
	}

	return nil, ErrNodeNotFound
}

// DataNodeByTCPHost returns the data node with the give http bind address
func (c *Client) DataNodeByTCPHost(tcpAddr string) (*NodeInfo, error) {
	nodes, _ := c.DataNodes()
	for _, n := range nodes {
		if n.TCPHost == tcpAddr {
			return &n, nil
		}
	}

	return nil, ErrNodeNotFound
}

// DeleteDataNode deletes a data node from the cluster.
func (c *Client) DeleteDataNode(id uint64) error {
	cmd := &internal.DeleteDataNodeCommand{
		ID: proto.Uint64(id),
	}

	return c.retryUntilExec(internal.Command_DeleteDataNodeCommand, internal.E_DeleteDataNodeCommand_Command, cmd)
}

// MetaNodes returns the meta nodes' info.
func (c *Client) MetaNodes() (meta.NodeInfos, error) {
	return c.data().MetaNodes, nil
}

// MetaNodeByAddr returns the meta node's info.
func (c *Client) MetaNodeByAddr(addr string) *meta.NodeInfo {
	for _, n := range c.data().MetaNodes {
		if n.Host == addr {
			return &n
		}
	}
	return nil
}

// Database returns info for the requested database.
func (c *Client) Database(name string) (*meta.DatabaseInfo, error) {
	for _, d := range c.data().Databases {
		if d.Name == name {
			return &d, nil
		}
	}

	// Can't throw ErrDatabaseNotExists here since it would require some major
	// work around catching the error when needed. Should be revisited.
	return nil, nil
}

// Databases returns a list of all database infos.
func (c *Client) Databases() (meta.Databases, error) {
	dbs := c.data().Databases
	if dbs == nil {
		return []DatabaseInfo{}, nil
	}
	return dbs, nil
}

func (c *Client) defaultRetentionPolicyInfo() {

}

// CreateDatabase creates a database or returns it if it already exists
func (c *Client) CreateDatabase(name string) (*meta.DatabaseInfo, error) {
	if db, _ := c.Database(name); db != nil {
		return db, nil
	}

	cmd := &internal.CreateDatabaseCommand{
		Name: proto.String(name),
	}

	err := c.retryUntilExec(internal.Command_CreateDatabaseCommand, internal.E_CreateDatabaseCommand_Command, cmd)
	if err != nil {
		return nil, err
	}

	return c.Database(name)
}

// CreateDatabaseWithRetentionPolicy creates a database with the specified retention policy.
func (c *Client) CreateDatabaseWithRetentionPolicy(name string, rpi *meta.RetentionPolicyInfo) (*meta.DatabaseInfo, error) {
	if rpi.Duration < MinRetentionPolicyDuration && rpi.Duration != 0 {
		return nil, ErrRetentionPolicyDurationTooLow
	}

	if db, _ := c.Database(name); db != nil {
		// Check if the retention policy already exists. If it does and matches
		// the desired retention policy, exit with no error.
		if rp := db.RetentionPolicy(rpi.Name); rp != nil {
			if rp.ReplicaN != rpi.ReplicaN || rp.Duration != rpi.Duration {
				return nil, ErrRetentionPolicyConflict
			}
			return db, nil
		}
	}

	cmd := &internal.CreateDatabaseCommand{
		Name:            proto.String(name),
		RetentionPolicy: rpi.marshal(),
	}

	err := c.retryUntilExec(internal.Command_CreateDatabaseCommand, internal.E_CreateDatabaseCommand_Command, cmd)
	if err != nil {
		return nil, err
	}

	return c.Database(name)
}

// DropDatabase deletes a database.
func (c *Client) DropDatabase(name string) error {
	cmd := &internal.DropDatabaseCommand{
		Name: proto.String(name),
	}

	return c.retryUntilExec(internal.Command_DropDatabaseCommand, internal.E_DropDatabaseCommand_Command, cmd)
}

// CreateRetentionPolicy creates a retention policy on the specified database.
func (c *Client) CreateRetentionPolicy(database string, rpi *meta.RetentionPolicyInfo) (*meta.RetentionPolicyInfo, error) {
	if rp, _ := c.RetentionPolicy(database, rpi.Name); rp != nil {
		return rp, nil
	}

	if rpi.Duration < MinRetentionPolicyDuration && rpi.Duration != 0 {
		return nil, ErrRetentionPolicyDurationTooLow
	}

	cmd := &internal.CreateRetentionPolicyCommand{
		Database:        proto.String(database),
		RetentionPolicy: rpi.marshal(),
	}

	if err := c.retryUntilExec(internal.Command_CreateRetentionPolicyCommand, internal.E_CreateRetentionPolicyCommand_Command, cmd); err != nil {
		return nil, err
	}

	return c.RetentionPolicy(database, rpi.Name)
}

// RetentionPolicy returns the requested retention policy info.
func (c *Client) RetentionPolicy(database, name string) (rpi *meta.RetentionPolicyInfo, err error) {
	db, err := c.Database(database)
	if err != nil {
		return nil, err
	}

	// TODO: This should not be handled here
	if db == nil {
		return nil, influxdb.ErrDatabaseNotFound(database)
	}

	return db.RetentionPolicy(name), nil
}

// DropRetentionPolicy drops a retention policy from a database.
func (c *Client) DropRetentionPolicy(database, name string) error {
	cmd := &internal.DropRetentionPolicyCommand{
		Database: proto.String(database),
		Name:     proto.String(name),
	}

	return c.retryUntilExec(internal.Command_DropRetentionPolicyCommand, internal.E_DropRetentionPolicyCommand_Command, cmd)
}

// SetDefaultRetentionPolicy sets a database's default retention policy.
func (c *Client) SetDefaultRetentionPolicy(database, name string) error {
	cmd := &internal.SetDefaultRetentionPolicyCommand{
		Database: proto.String(database),
		Name:     proto.String(name),
	}

	return c.retryUntilExec(internal.Command_SetDefaultRetentionPolicyCommand, internal.E_SetDefaultRetentionPolicyCommand_Command, cmd)
}

// UpdateRetentionPolicy updates a retention policy.
func (c *Client) UpdateRetentionPolicy(database, name string, rpu *meta.RetentionPolicyUpdate) error {
	var newName *string
	if rpu.Name != nil {
		newName = rpu.Name
	}

	var duration *int64
	if rpu.Duration != nil {
		value := int64(*rpu.Duration)
		duration = &value
	}

	var replicaN *uint32
	if rpu.ReplicaN != nil {
		value := uint32(*rpu.ReplicaN)
		replicaN = &value
	}

	cmd := &internal.UpdateRetentionPolicyCommand{
		Database: proto.String(database),
		Name:     proto.String(name),
		NewName:  newName,
		Duration: duration,
		ReplicaN: replicaN,
	}

	return c.retryUntilExec(internal.Command_UpdateRetentionPolicyCommand, internal.E_UpdateRetentionPolicyCommand_Command, cmd)
}

// IsLeader - should get rid of this
func (c *Client) IsLeader() bool {
	return false
}

func (c *Client) Users() []UserInfo {
	users := c.data().Users

	if users == nil {
		return []UserInfo{}
	}
	return users
}

func (c *Client) User(name string) (*UserInfo, error) {
	for _, u := range c.data().Users {
		if u.Name == name {
			return &u, nil
		}
	}

	return nil, ErrUserNotFound
}

//TODO MOVE hash to auth.go
// bcryptCost is the cost associated with generating password with bcrypt.
// This setting is lowered during testing to improve test suite performance.
var bcryptCost = bcrypt.DefaultCost

// hashWithSalt returns a salted hash of password using salt
func (c *Client) hashWithSalt(salt []byte, password string) []byte {
	hasher := sha256.New()
	hasher.Write(salt)
	hasher.Write([]byte(password))
	return hasher.Sum(nil)
}

// saltedHash returns a salt and salted hash of password
func (c *Client) saltedHash(password string) (salt, hash []byte, err error) {
	salt = make([]byte, SaltBytes)
	if _, err := io.ReadFull(crand.Reader, salt); err != nil {
		return nil, nil, err
	}

	return salt, c.hashWithSalt(salt, password), nil
}

func (c *Client) toOSUser() {}

func (c *Client) CreateUser(name, password string, admin bool) (*UserInfo, error) {
	data := c.cacheData.Clone()

	// See if the user already exists.
	if u := data.User(name); u != nil {
		if err := bcrypt.CompareHashAndPassword([]byte(u.Hash), []byte(password)); err != nil || u.Admin != admin {
			return nil, ErrUserExists
		}
		return u, nil
	}

	// Hash the password before serializing it.
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcryptCost)
	if err != nil {
		return nil, err
	}

	if err := c.retryUntilExec(internal.Command_CreateUserCommand, internal.E_CreateUserCommand_Command,
		&internal.CreateUserCommand{
			Name:  proto.String(name),
			Hash:  proto.String(string(hash)),
			Admin: proto.Bool(admin),
		},
	); err != nil {
		return nil, err
	}
	return c.User(name)
}

func (c *Client) UpdateUser(name, password string) error {
	// Hash the password before serializing it.
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcryptCost)
	if err != nil {
		return err
	}

	return c.retryUntilExec(internal.Command_UpdateUserCommand, internal.E_UpdateUserCommand_Command,
		&internal.UpdateUserCommand{
			Name: proto.String(name),
			Hash: proto.String(string(hash)),
		},
	)
}

func (c *Client) DropUser(name string) error {
	return c.retryUntilExec(internal.Command_DropUserCommand, internal.E_DropUserCommand_Command,
		&internal.DropUserCommand{
			Name: proto.String(name),
		},
	)
}

func (c *Client) SetPrivilege(username, database string, p influxql.Privilege) error {
	return c.retryUntilExec(internal.Command_SetPrivilegeCommand, internal.E_SetPrivilegeCommand_Command,
		&internal.SetPrivilegeCommand{
			Username:  proto.String(username),
			Database:  proto.String(database),
			Privilege: proto.Int32(int32(p)),
		},
	)
}

func (c *Client) SetAdminPrivilege(username string, admin bool) error {
	return c.retryUntilExec(internal.Command_SetAdminPrivilegeCommand, internal.E_SetAdminPrivilegeCommand_Command,
		&internal.SetAdminPrivilegeCommand{
			Username: proto.String(username),
			Admin:    proto.Bool(admin),
		},
	)
}

func (c *Client) UserPrivileges(username string) (map[string]influxql.Privilege, error) {
	p, err := c.data().UserPrivileges(username)
	if err != nil {
		return nil, err
	}
	return p, nil
}

func (c *Client) UserPrivilege(username, database string) (*influxql.Privilege, error) {
	p, err := c.data().UserPrivilege(username, database)
	if err != nil {
		return nil, err
	}
	return p, nil
}

func (c *Client) AdminUserExists() bool {
	for _, u := range c.data().Users {
		if u.Admin {
			return true
		}
	}
	return false
}

func (c *Client) Authenticate(username, password string) (*UserInfo, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	// Find user.
	userInfo := c.cacheData.User(username)
	if userInfo == nil {
		return nil, ErrUserNotFound
	}

	// Check the local auth cache first.
	if au, ok := c.authCache[username]; ok {
		// verify the password using the cached salt and hash
		if bytes.Equal(c.hashWithSalt(au.salt, password), au.hash) {
			return userInfo, nil
		}

		// fall through to requiring a full bcrypt hash for invalid passwords
	}

	// Compare password with user hash.
	if err := bcrypt.CompareHashAndPassword([]byte(userInfo.Hash), []byte(password)); err != nil {
		return nil, ErrAuthenticate
	}

	// generate a salt and hash of the password for the cache
	salt, hashed, err := c.saltedHash(password)
	if err != nil {
		return nil, err
	}
	c.authCache[username] = authUser{salt: salt, hash: hashed, bhash: userInfo.Hash}

	return userInfo, nil
}

func (c *Client) UserCount() int {
	return len(c.data().Users)
}

// ShardIDs returns a list of all shard ids.
func (c *Client) ShardIDs() []uint64 {
	var a []uint64
	for _, dbi := range c.data().Databases {
		for _, rpi := range dbi.RetentionPolicies {
			for _, sgi := range rpi.ShardGroups {
				for _, si := range sgi.Shards {
					a = append(a, si.ID)
				}
			}
		}
	}
	sort.Sort(uint64Slice(a))
	return a
}

// ShardGroupsByTimeRange returns a list of all shard groups on a database and policy that may contain data
// for the specified time range. Shard groups are sorted by start time.
func (c *Client) ShardGroupsByTimeRange(database, policy string, min, max time.Time) (a meta.ShardGroupInfos, err error) {
	// Find retention policy.
	rpi, err := c.data().RetentionPolicy(database, policy)
	if err != nil {
		return nil, err
	} else if rpi == nil {
		return nil, influxdb.ErrRetentionPolicyNotFound(policy)
	}
	groups := make([]ShardGroupInfo, 0, len(rpi.ShardGroups))
	for _, g := range rpi.ShardGroups {
		if g.Deleted() || !g.Overlaps(min, max) {
			continue
		}
		groups = append(groups, g)
	}
	return groups, nil
}

// ShardsByTimeRange returns a slice of shards that may contain data in the time range.
func (c *Client) ShardsByTimeRange(sources influxql.Sources, tmin, tmax time.Time) (a meta.ShardInfos, err error) {
	m := make(map[*ShardInfo]struct{})
	for _, src := range sources {
		mm, ok := src.(*influxql.Measurement)
		if !ok {
			return nil, fmt.Errorf("invalid source type: %#v", src)
		}

		groups, err := c.ShardGroupsByTimeRange(mm.Database, mm.RetentionPolicy, tmin, tmax)
		if err != nil {
			return nil, err
		}
		for _, g := range groups {
			for i := range g.Shards {
				m[&g.Shards[i]] = struct{}{}
			}
		}
	}

	a = make([]ShardInfo, 0, len(m))
	for sh := range m {
		a = append(a, *sh)
	}

	return a, nil
}

// CreateShardGroup creates a shard group on a database and policy for a given timestamp.
func (c *Client) CreateShardGroup(database, policy string, timestamp time.Time) (*ShardGroupInfo, error) {
	if sg, _ := c.data().data.ShardGroupByTimestamp(database, policy, timestamp); sg != nil {
		return sg, nil
	}

	cmd := &internal.CreateShardGroupCommand{
		Database:  proto.String(database),
		Policy:    proto.String(policy),
		Timestamp: proto.Int64(timestamp.UnixNano()),
	}

	if err := c.retryUntilExec(internal.Command_CreateShardGroupCommand, internal.E_CreateShardGroupCommand_Command, cmd); err != nil {
		return nil, err
	}

	rpi, err := c.RetentionPolicy(database, policy)
	if err != nil {
		return nil, err
	} else if rpi == nil {
		return nil, errors.New("retention policy deleted after shard group created")
	}

	return rpi.ShardGroupByTimestamp(timestamp), nil
}

// DeleteShardGroup removes a shard group from a database and retention policy by id.
func (c *Client) DeleteShardGroup(database, policy string, id uint64) error {
	cmd := &internal.DeleteShardGroupCommand{
		Database:     proto.String(database),
		Policy:       proto.String(policy),
		ShardGroupID: proto.Uint64(id),
	}

	return c.retryUntilExec(internal.Command_DeleteShardGroupCommand, internal.E_DeleteShardGroupCommand_Command, cmd)
}

// PrecreateShardGroups creates shard groups whose endtime is before the 'to' time passed in, but
// is yet to expire before 'from'. This is to avoid the need for these shards to be created when data
// for the corresponding time range arrives. Shard creation involves Raft consensus, and precreation
// avoids taking the hit at write-time.
func (c *Client) PrecreateShardGroups(from, to time.Time) error {
	for _, di := range c.data().Databases {
		for _, rp := range di.RetentionPolicies {
			if len(rp.ShardGroups) == 0 {
				// No data was ever written to this group, or all groups have been deleted.
				continue
			}
			g := rp.ShardGroups[len(rp.ShardGroups)-1] // Get the last group in time.
			if !g.Deleted() && g.EndTime.Before(to) && g.EndTime.After(from) {
				// Group is not deleted, will end before the future time, but is still yet to expire.
				// This last check is important, so the system doesn't create shards groups wholly
				// in the past.

				// Create successive shard group.
				nextShardGroupTime := g.EndTime.Add(1 * time.Nanosecond)
				if newGroup, err := c.CreateShardGroup(di.Name, rp.Name, nextShardGroupTime); err != nil {
					c.logger.Printf("failed to precreate successive shard group for group %d: %s", g.ID, err.Error())
				} else {
					c.logger.Printf("new shard group %d successfully precreated for database %s, retention policy %s", newGroup.ID, di.Name, rp.Name)
				}
			}
		}
		return nil
	}
	return nil
}

// ShardOwner returns the owning shard group info for a specific shard.
func (c *Client) ShardOwner(shardID uint64) (database, policy string, sgi *ShardGroupInfo) {
	for _, dbi := range c.data().Databases {
		for _, rpi := range dbi.RetentionPolicies {
			for _, g := range rpi.ShardGroups {
				if g.Deleted() {
					continue
				}

				for _, sh := range g.Shards {
					if sh.ID == shardID {
						database = dbi.Name
						policy = rpi.Name
						sgi = &g
						return
					}
				}
			}
		}
	}
	return
}

// JoinMetaServer will add the passed in tcpAddr to the raft peers and add a MetaNode to
// the metastore
func (c *Client) JoinMetaServer(httpAddr, tcpAddr string) (*NodeInfo, error) {
	node := &NodeInfo{
		Host:    httpAddr,
		TCPHost: tcpAddr,
	}
	b, err := json.Marshal(node)
	if err != nil {
		return nil, err
	}

	currentServer := 0
	redirectServer := ""
	for {
		// get the server to try to join against
		var url string
		if redirectServer != "" {
			url = redirectServer
			redirectServer = ""
		} else {
			c.mu.RLock()

			if currentServer >= len(c.metaServers) {
				// We've tried every server, wait a second before
				// trying again
				time.Sleep(time.Second)
				currentServer = 0
			}
			server := c.metaServers[currentServer]
			c.mu.RUnlock()

			url = c.url(server) + "/join"
		}

		resp, err := http.Post(url, "application/json", bytes.NewBuffer(b))
		if err != nil {
			currentServer++
			continue
		}

		// Successfully joined
		if resp.StatusCode == http.StatusOK {
			defer resp.Body.Close()
			if err := json.NewDecoder(resp.Body).Decode(&node); err != nil {
				return nil, err
			}
			break
		}
		resp.Body.Close()

		// We tried to join a meta node that was not the leader, rety at the node
		// they think is the leader.
		if resp.StatusCode == http.StatusTemporaryRedirect {
			redirectServer = resp.Header.Get("Location")
			continue
		}

		// Something failed, try the next node
		currentServer++
	}

	return node, nil
}

func (c *Client) CreateMetaNode(httpAddr, tcpAddr string) (*NodeInfo, error) {
	cmd := &internal.CreateMetaNodeCommand{
		HTTPAddr: proto.String(httpAddr),
		TCPAddr:  proto.String(tcpAddr),
		Rand:     proto.Uint64(uint64(rand.Int63())),
	}

	if err := c.retryUntilExec(internal.Command_CreateMetaNodeCommand, internal.E_CreateMetaNodeCommand_Command, cmd); err != nil {
		return nil, err
	}

	n := c.MetaNodeByAddr(httpAddr)
	if n == nil {
		return nil, errors.New("new meta node not found")
	}

	c.nodeID = n.ID

	return n, nil
}

func (c *Client) DeleteMetaNode(id uint64) error {
	cmd := &internal.DeleteMetaNodeCommand{
		ID: proto.Uint64(id),
	}

	return c.retryUntilExec(internal.Command_DeleteMetaNodeCommand, internal.E_DeleteMetaNodeCommand_Command, cmd)
}

func (c *Client) CreateContinuousQuery(database, name, query string) error {
	return c.retryUntilExec(internal.Command_CreateContinuousQueryCommand, internal.E_CreateContinuousQueryCommand_Command,
		&internal.CreateContinuousQueryCommand{
			Database: proto.String(database),
			Name:     proto.String(name),
			Query:    proto.String(query),
		},
	)
}

func (c *Client) DropContinuousQuery(database, name string) error {
	return c.retryUntilExec(internal.Command_DropContinuousQueryCommand, internal.E_DropContinuousQueryCommand_Command,
		&internal.DropContinuousQueryCommand{
			Database: proto.String(database),
			Name:     proto.String(name),
		},
	)
}

func (c *Client) CreateSubscription(database, rp, name, mode string, destinations []string) error {
	return c.retryUntilExec(internal.Command_CreateSubscriptionCommand, internal.E_CreateSubscriptionCommand_Command,
		&internal.CreateSubscriptionCommand{
			Database:        proto.String(database),
			RetentionPolicy: proto.String(rp),
			Name:            proto.String(name),
			Mode:            proto.String(mode),
			Destinations:    destinations,
		},
	)
}

func (c *Client) DropSubscription(database, rp, name string) error {
	return c.retryUntilExec(internal.Command_DropSubscriptionCommand, internal.E_DropSubscriptionCommand_Command,
		&internal.DropSubscriptionCommand{
			Database:        proto.String(database),
			RetentionPolicy: proto.String(rp),
			Name:            proto.String(name),
		},
	)
}
func (c *Client) Data() *Data {
	return c.data().Clone()
}

func (c *Client) SetData(data *Data) error {
	return c.retryUntilExec(internal.Command_SetDataCommand, internal.E_SetDataCommand_Command,
		&internal.SetDataCommand{
			Data: data.MarshalBinary(),
		},
	)
}

// WaitForDataChanged will return a channel that will get closed when
// the metastore data has changed
func (c *Client) WaitForDataChanged() chan struct{} {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.changed
}

func (c *Client) MarshalBinary() ([]byte, error) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.cacheData.MarshalBinary()
}
func (c *Client) Logger() *log.Logger {
	return c.logger
}

func (c *Client) SetLogger(l *log.Logger) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.logger = l
}

func (c *Client) index() uint64 {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.cacheData.Index
}

// retryUntilExec will attempt the command on each of the metaservers until it either succeeds or
// hits the max number of tries
func (c *Client) retryUntilExec(typ internal.Command_Type, desc *proto.ExtensionDesc, value interface{}) error {
	var err error
	var index uint64
	tries := 0
	currentServer := 0
	var redirectServer string

	for {
		c.mu.RLock()
		// exit if we're closed
		select {
		case <-c.closing:
			c.mu.RUnlock()
			return nil
		default:
			// we're still open, continue on
		}
		c.mu.RUnlock()

		// build the url to hit the redirect server or the next metaserver
		var url string
		if redirectServer != "" {
			url = redirectServer
			redirectServer = ""
		} else {
			c.mu.RLock()
			if currentServer >= len(c.metaServers) {
				currentServer = 0
			}
			server := c.metaServers[currentServer]
			c.mu.RUnlock()

			url = fmt.Sprintf("://%s/execute", server)
			if c.tls {
				url = "https" + url
			} else {
				url = "http" + url
			}
		}

		index, err = c.exec(url, typ, desc, value)
		tries++
		currentServer++

		if err == nil {
			c.waitForIndex(index)
			return nil
		}

		if tries > maxRetries {
			return err
		}

		if e, ok := err.(errRedirect); ok {
			redirectServer = e.host
			continue
		}

		if _, ok := err.(errCommand); ok {
			return err
		}

		time.Sleep(errSleep)
	}
}

func (c *Client) exec(url string, typ internal.Command_Type, desc *proto.ExtensionDesc, value interface{}) (index uint64, err error) {
	// Create command.
	cmd := &internal.Command{Type: &typ}
	if err := proto.SetExtension(cmd, desc, value); err != nil {
		panic(err)
	}

	b, err := proto.Marshal(cmd)
	if err != nil {
		return 0, err
	}

	resp, err := http.Post(url, "application/octet-stream", bytes.NewBuffer(b))
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()

	// read the response
	if resp.StatusCode == http.StatusTemporaryRedirect {
		return 0, errRedirect{host: resp.Header.Get("Location")}
	} else if resp.StatusCode != http.StatusOK {
		return 0, fmt.Errorf("meta service returned %s", resp.Status)
	}

	res := &internal.Response{}

	b, err = ioutil.ReadAll(resp.Body)
	if err != nil {
		return 0, err
	}

	if err := proto.Unmarshal(b, res); err != nil {
		return 0, err
	}
	es := res.GetError()
	if es != "" {
		return 0, errCommand{msg: es}
	}

	return res.GetIndex(), nil
}

func (c *Client) waitForIndex(idx uint64) {
	for {
		c.mu.RLock()
		if c.cacheData.Index >= idx {
			c.mu.RUnlock()
			return
		}
		ch := c.changed
		c.mu.RUnlock()
		<-ch
	}
}

func (c *Client) pollForUpdates() {
	for {
		data := c.retryUntilSnapshot(c.index())
		if data == nil {
			// this will only be nil if the client has been closed,
			// so we can exit out
			return
		}

		// update the data and notify of the change
		c.mu.Lock()
		idx := c.cacheData.Index
		c.cacheData = data
		c.updateAuthCache()
		if idx < data.Index {
			close(c.changed)
			c.changed = make(chan struct{})
		}
		c.mu.Unlock()
	}
}

func (c *Client) getSnapshot(server string, index uint64) (*Data, error) {
	resp, err := http.Get(c.url(server) + fmt.Sprintf("?index=%d", index))
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("meta server returned non-200: %s", resp.Status)
	}

	b, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	data := &Data{}
	if err := data.UnmarshalBinary(b); err != nil {
		return nil, err
	}

	return data, nil
}

// peers returns the TCPHost addresses of all the metaservers
//TODO: zhexuany delete these code peers and url
func (c *Client) peers() []string {

	var peers Peers
	// query each server and keep track of who their peers are
	for _, server := range c.metaServers {
		url := c.url(server) + "/peers"
		resp, err := http.Get(url)
		if err != nil {
			continue
		}
		defer resp.Body.Close()

		// This meta-server might not be ready to answer, continue on
		if resp.StatusCode != http.StatusOK {
			continue
		}

		dec := json.NewDecoder(resp.Body)
		var p []string
		if err := dec.Decode(&p); err != nil {
			continue
		}
		peers = peers.Append(p...)
	}

	// Return the unique set of peer addresses
	return []string(peers.Unique())
}

//TODO get rid of url
func (c *Client) url(server string) string {
	url := fmt.Sprintf("://%s", server)

	if c.tls {
		url = "https" + url
	} else {
		url = "http" + url
	}

	return url
}

func (c *Client) retryUntilSnapshot(idx uint64) *Data {
	currentServer := 0
	for {
		// get the index to look from and the server to poll
		c.mu.RLock()

		// exit if we're closed
		select {
		case <-c.closing:
			c.mu.RUnlock()
			return nil
		default:
			// we're still open, continue on
		}

		if currentServer >= len(c.metaServers) {
			currentServer = 0
		}
		server := c.metaServers[currentServer]
		c.mu.RUnlock()

		data, err := c.getSnapshot(server, idx)

		if err == nil {
			return data
		}

		c.logger.Printf("failure getting snapshot from %s: %s", server, err.Error())
		time.Sleep(errSleep)

		currentServer++
	}
}

func (c *Client) updateAuthCache() {
	// copy cached user info for still-present users
	newCache := make(map[string]authUser, len(c.authCache))

	for _, userInfo := range c.cacheData.Users {
		if cached, ok := c.authCache[userInfo.Name]; ok {
			if cached.bhash == userInfo.Hash {
				newCache[userInfo.Name] = cached
			}
		}
	}

	c.authCache = newCache
}
func (c *CLient) updateMetaServers() {}

func (c *Client) saveMetaServers() {}

func (c *Client) loadMetaServers() {}

func (c *Client) resetMetaServers() {}

func (c *Client) MetaServers() []string {
	return c.metaServers
}

type Peers []string

func (peers Peers) Append(p ...string) Peers {
	peers = append(peers, p...)

	return peers.Unique()
}

func (peers Peers) Unique() Peers {
	distinct := map[string]struct{}{}
	for _, p := range peers {
		distinct[p] = struct{}{}
	}

	var u Peers
	for k := range distinct {
		u = append(u, k)
	}
	return u
}

func (peers Peers) Contains(peer string) bool {
	for _, p := range peers {
		if p == peer {
			return true
		}
	}
	return false
}

type errRedirect struct {
	host string
}

func (e errRedirect) Error() string {
	return fmt.Sprintf("redirect to %s", e.host)
}

type errCommand struct {
	msg string
}

func (e errCommand) Error() string {
	return e.msg
}

type uint64Slice []uint64

func (a uint64Slice) Len() int           { return len(a) }
func (a uint64Slice) Swap(i, j int)      { a[i], a[j] = a[j], a[i] }
func (a uint64Slice) Less(i, j int) bool { return a[i] < a[j] }

func (c *Client) tryHTTP() {}

func (c *Client) doHTTPWithRedirect() {}
