package meta

import (
	"compress/gzip"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"net/http"
	// "net/http/pprof"
	// "net/url"
	"os"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"

	// jwt "github.com/dgrijalva/jwt-go"
	"github.com/gogo/protobuf/proto"
	"github.com/hashicorp/raft"
	"github.com/influxdata/influxdb/uuid"
	"github.com/zhexuany/influxdb-cluster/meta/internal"
)

// handler represents an HTTP handler for the meta service.
type handler struct {
	config *MetaConfig

	logger         *log.Logger
	loggingEnabled bool // Log every HTTP access.
	pprofEnabled   bool
	store          interface {
		afterIndex(index uint64) <-chan struct{}
		index() uint64
		leader() string
		leaderHTTP() string
		snapshot() (*Data, error)
		apply(b []byte) error
		join(n *NodeInfo) (*NodeInfo, error)
		otherMetaServersHTTP() []string
		peers() []string
	}
	s *Service

	mu      sync.RWMutex
	closing chan struct{}
	leases  *Leases
}

func (a Announcements) Filter() {

}
func (a *Announcement) validate() {

}
func (a *Announcement) UnmarshalJSON() {

}
func (a Announcement) expired() {

}

// newHandler returns a new instance of handler with routes.
func newHandler(c *MetaConfig, s *Service) *handler {
	h := &handler{
		s:              s,
		config:         c,
		logger:         log.New(os.Stderr, "[meta-http] ", log.LstdFlags),
		loggingEnabled: c.ClusterTracing,
		closing:        make(chan struct{}),
		leases:         NewLeases(time.Duration(c.LeaseDuration)),
	}

	return h
}

// SetRoutes sets the provided routes on the handler.
func (h *handler) WrapHandler(name string, hf http.HandlerFunc) http.Handler {
	var handler http.Handler
	// handler = http.HandlerFunc(hf)
	handler = gzipFilter(handler)
	handler = versionHeader(handler, h)
	handler = requestID(handler)
	if h.loggingEnabled {
		handler = logging(handler, name, h.logger)
	}
	handler = recovery(handler, name, h.logger) // make sure recovery is always last
	//TODO need firgure what is the parameter of this func
	// h.authorize()
	//authorize
	//authenticate
	return handler
}

func verifyCreatingAdmin() {

}

func (h *handler) authorize() {

}

// ServeHTTP responds to HTTP request to the handler.
func (h *handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	// pprof.Cmdline(w, r)
	// pprof.Profile(w, r)
	// pprof.Symbol(w, r)
	// pprof.Index(w, r)
	switch r.Method {
	case "GET":
		switch r.URL.Path {
		case "/ping":
			h.WrapHandler("ping", h.servePing).ServeHTTP(w, r)
		case "/lease":
			h.WrapHandler("lease", h.serveLease).ServeHTTP(w, r)
		default:
			h.WrapHandler("snapshot", h.serveSnapshot).ServeHTTP(w, r)
		}
	case "POST":
		h.WrapHandler("execute", h.serveExec).ServeHTTP(w, r)
	default:
		http.Error(w, "", http.StatusBadRequest)
	}
}

func (h *handler) do() {
	// u := url.URL.String()
	// req := http.NewRequest("http", u)
	// http.Header.Set(k, v) fmt.Sprintf
	// h.mu.RLock()
	// h.mu.RUnlock()
	//http.Do
	//time now
	// jwt.NewWithClaims()
	// jwt.SignedString()
}

func (h *handler) get() {
	h.do()
}

func (h *handler) post() {
	h.do()
}
func (h *handler) postform() {
	//net.url.Va
	// url.Values.Encode()
	h.do()
}

func (h *handler) startGossiping() {

}

func (h *handler) Close() error {
	h.mu.Lock()
	defer h.mu.Unlock()
	select {
	case <-h.closing:
		// do nothing here
	default:
		close(h.closing)
	}
	return nil
}

func (h *handler) isClosed() bool {
	h.mu.RLock()
	defer h.mu.RUnlock()
	select {
	case <-h.closing:
		return true
	default:
		return false
	}
}

func (h *handler) redirectLeader() {
	//http.Redirect
	//jsonError
}

func (h *handler) machineEntitled() {

}

func (h *handler) serveJoin(w http.ResponseWriter, r *http.Request) {
	// if h.isClosed() {
	// 	h.httpError(fmt.Errorf("server closed"), w, http.StatusServiceUnavailable)
	// 	return
	// }
	// n := &NodeInfo{}
	// if err := json.Unmarshal(body, n); err != nil {
	// 	h.httpError(err, w, http.StatusInternalServerError)
	// 	return
	// }

	// // if resp, err := http.PostForm(h.config.Meta.RemoteHostname, data); err != nil {

	// // }

	// node, err := h.store.join(n)
	// if err == raft.ErrNotLeader {
	// 	l := h.store.leaderHTTP()
	// 	if l == "" {
	// 		// No cluster leader. Client will have to try again later.
	// 		h.httpError(errors.New("no leader"), w, http.StatusServiceUnavailable)
	// 		return
	// 	}
	// 	scheme := "http://"
	// 	if h.config.HTTPSEnabled {
	// 		scheme = "https://"
	// 	}

	// 	l = scheme + l + "/join"
	// 	http.Redirect(w, r, l, http.StatusTemporaryRedirect)
	// 	return
	// }

	// if err != nil {
	// 	h.httpError(err, w, http.StatusInternalServerError)
	// 	return
	// }

	// // Return the node with newly assigned ID as json
	// w.Header().Add("Content-Type", "application/json")
	// if err := json.NewEncoder(w).Encode(node); err != nil {
	// 	h.jsonError(err, w, http.StatusInternalServerError)
	// }

	// return

	// http.PostForm(url, data)
	// h.get()
	// json.Decoder.Decode(v)
	// jsonError()
	// http.Header.Add(k, v)
	// json.Encoder.Encode(v)
	// jsonError()
	// h.postform()
}

func (h *handler) serveLeave(w http.ResponseWriter, r *http.Request) {
	h.postform()
}

func (h *handler) serveRemoveMeta(w http.ResponseWriter, r *http.Request) {

}

func (h *handler) serveAddData(w http.ResponseWriter, r *http.Request) {

}

func (h *handler) incrementInternalRF(w http.ResponseWriter, r *http.Request) {

}

func (rpc RPCCall) send() {

}

func (h *handler) sendDataNodeJoin() {

}

func (h *handler) serveRemoveData(w http.ResponseWriter, r *http.Request) {

}

func (h *handler) sendDataNodeLeave() {

}

func (h *handler) serveUpdateData(w http.ResponseWriter, r *http.Request) {

}

func (h *handler) verifyDataNode() {

}

// serveExec executes the requested command.
func (h *handler) serveExec(w http.ResponseWriter, r *http.Request) {
	if h.isClosed() {
		h.httpError(fmt.Errorf("server closed"), w, http.StatusServiceUnavailable)
		return
	}

	// Read the command from the request body.
	body, err := ioutil.ReadAll(r.Body)
	if err != nil {
		h.httpError(err, w, http.StatusInternalServerError)
		return
	}

	// Make sure it's a valid command.
	if err := validateCommand(body); err != nil {
		h.httpError(err, w, http.StatusBadRequest)
		return
	}

	// Apply the command to the store.
	var resp *internal.Response
	if err := h.store.apply(body); err != nil {
		// If we aren't the leader, redirect client to the leader.
		if err == raft.ErrNotLeader {
			l := h.store.leaderHTTP()
			if l == "" {
				// No cluster leader. Client will have to try again later.
				h.httpError(errors.New("no leader"), w, http.StatusServiceUnavailable)
				return
			}
			scheme := "http://"
			if h.config.HTTPSEnabled {
				scheme = "https://"
			}

			l = scheme + l + "/execute"
			http.Redirect(w, r, l, http.StatusTemporaryRedirect)
			return
		}

		// Error wasn't a leadership error so pass it back to client.
		resp = &internal.Response{
			OK:    proto.Bool(false),
			Error: proto.String(err.Error()),
		}
	} else {
		// Apply was successful. Return the new store index to the client.
		resp = &internal.Response{
			OK:    proto.Bool(false),
			Index: proto.Uint64(h.store.index()),
		}
	}

	// Marshal the response.
	b, err := proto.Marshal(resp)
	if err != nil {
		h.httpError(err, w, http.StatusInternalServerError)
		return
	}

	// Send response to client.
	w.Header().Add("Content-Type", "application/octet-stream")
	w.Write(b)
}

func (h *handler) serveRemoveShard(w http.ResponseWriter, r *http.Request) {

}

func (h *handler) serveCopyShardStatus(w http.ResponseWriter, r *http.Request) {

}
func (h *handler) serveCopyShard(w http.ResponseWriter, r *http.Request) {

}

func (h *handler) serveKillCopyShard(w http.ResponseWriter, r *http.Request) {

}
func (h *handler) serveTruncateShards(w http.ResponseWriter, r *http.Request) {

}

func validateCommand(b []byte) error {
	// Ensure command can be deserialized before applying.
	if err := proto.Unmarshal(b, &internal.Command{}); err != nil {
		return fmt.Errorf("unable to unmarshal command: %s", err)
	}

	return nil
}

// serveSnapshot is a long polling http connection to server cache updates
func (h *handler) serveSnapshot(w http.ResponseWriter, r *http.Request) {
	if h.isClosed() {
		h.httpError(fmt.Errorf("server closed"), w, http.StatusInternalServerError)
		return
	}

	// get the current index that client has
	index, err := strconv.ParseUint(r.URL.Query().Get("index"), 10, 64)
	if err != nil {
		http.Error(w, "error parsing index", http.StatusBadRequest)
	}

	select {
	case <-h.store.afterIndex(index):
		// Send updated snapshot to client.
		ss, err := h.store.snapshot()
		if err != nil {
			h.httpError(err, w, http.StatusInternalServerError)
			return
		}
		b, err := ss.MarshalBinary()
		if err != nil {
			h.httpError(err, w, http.StatusInternalServerError)
			return
		}
		w.Header().Add("Content-Type", "application/octet-stream")
		w.Write(b)
		return
	case <-w.(http.CloseNotifier).CloseNotify():
		// Client closed the connection so we're done.
		return
	case <-h.closing:
		h.httpError(fmt.Errorf("server closed"), w, http.StatusInternalServerError)
		return
	}
}

func (h *handler) serveRestoreMeta(w http.ResponseWriter, r *http.Request) {

}

// servePing will return if the server is up, or if specified will check the status
// of the other metaservers as well
func (h *handler) servePing(w http.ResponseWriter, r *http.Request) {
	// if they're not asking to check all servers, just return who we think
	// the leader is
	if r.URL.Query().Get("all") == "" {
		w.Write([]byte(h.store.leader()))
		return
	}

	leader := h.store.leader()
	healthy := true
	for _, n := range h.store.otherMetaServersHTTP() {
		scheme := "http://"
		if h.config.HTTPSEnabled {
			scheme = "https://"
		}
		url := scheme + n + "/ping"

		resp, err := http.Get(url)
		if err != nil {
			healthy = false
			break
		}

		defer resp.Body.Close()
		b, err := ioutil.ReadAll(resp.Body)
		if err != nil {
			healthy = false
			break
		}

		if leader != string(b) {
			healthy = false
			break
		}
	}

	if healthy {
		w.Write([]byte(h.store.leader()))
		return
	}

	h.httpError(fmt.Errorf("one or more metaservers not up"), w, http.StatusInternalServerError)
}

func (h *handler) serveStatus(w http.ResponseWriter, r *http.Request) {

}
func (h *handler) serveShowCluster(w http.ResponseWriter, r *http.Request) {

}
func (h *handler) serveShowShards(w http.ResponseWriter, r *http.Request) {

}

// serveLease
func (h *handler) serveLease(w http.ResponseWriter, r *http.Request) {
	var name, nodeIDStr string
	q := r.URL.Query()

	// Get the requested lease name.
	name = q.Get("name")
	if name == "" {
		http.Error(w, "lease name required", http.StatusBadRequest)
		return
	}

	// Get the ID of the requesting node.
	nodeIDStr = q.Get("nodeid")
	if nodeIDStr == "" {
		http.Error(w, "node ID required", http.StatusBadRequest)
		return
	}

	// Redirect to leader if necessary.
	leader := h.store.leaderHTTP()
	if leader != h.s.remoteAddr(h.s.httpAddr) {
		if leader == "" {
			// No cluster leader. Client will have to try again later.
			h.httpError(errors.New("no leader"), w, http.StatusServiceUnavailable)
			return
		}
		scheme := "http://"
		if h.config.HTTPSEnabled {
			scheme = "https://"
		}

		leader = scheme + leader + "/lease?" + q.Encode()
		http.Redirect(w, r, leader, http.StatusTemporaryRedirect)
		return
	}

	// Convert node ID to an int.
	nodeID, err := strconv.ParseUint(nodeIDStr, 10, 64)
	if err != nil {
		http.Error(w, "invalid node ID", http.StatusBadRequest)
		return
	}

	// Try to acquire the requested lease.
	// Always returns a lease. err determins if we own it.
	l, err := h.leases.Acquire(name, nodeID)
	// Marshal the lease to JSON.
	b, e := json.Marshal(l)
	if e != nil {
		h.httpError(e, w, http.StatusInternalServerError)
		return
	}
	// Write HTTP status.
	if err != nil {
		// Another node owns the lease.
		w.WriteHeader(http.StatusConflict)
	} else {
		// Lease successfully acquired.
		w.WriteHeader(http.StatusOK)
	}
	// Write the lease data.
	w.Header().Add("Content-Type", "application/json")
	w.Write(b)
	return
}

func (h *handler) serveAnnounce(w http.ResponseWriter, r *http.Request) {

}
func (h *handler) gossipAnnouncements() {

}
func (h *handler) mergeAnnouncements() {

}

type gzipResponseWriter struct {
	io.Writer
	http.ResponseWriter
}

func (w gzipResponseWriter) Write(b []byte) (int, error) {
	return w.Writer.Write(b)
}

func (w gzipResponseWriter) Flush() {
	w.Writer.(*gzip.Writer).Flush()
}

func (w gzipResponseWriter) CloseNotify() <-chan bool {
	return w.ResponseWriter.(http.CloseNotifier).CloseNotify()
}

// determines if the client can accept compressed responses, and encodes accordingly
func gzipFilter(inner http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.Contains(r.Header.Get("Accept-Encoding"), "gzip") {
			inner.ServeHTTP(w, r)
			return
		}
		w.Header().Set("Content-Encoding", "gzip")
		gz := gzip.NewWriter(w)
		defer gz.Close()
		gzw := gzipResponseWriter{Writer: gz, ResponseWriter: w}
		inner.ServeHTTP(gzw, r)
	})
}

// versionHeader takes a HTTP handler and returns a HTTP handler
// and adds the X-INFLUXBD-VERSION header to outgoing responses.
func versionHeader(inner http.Handler, h *handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Add("X-InfluxDB-Version", h.s.Version())
		inner.ServeHTTP(w, r)
	})
}

func requestID(inner http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		uid := uuid.TimeUUID()
		r.Header.Set("Request-Id", uid.String())
		w.Header().Set("Request-Id", r.Header.Get("Request-Id"))

		inner.ServeHTTP(w, r)
	})
}

func logging(inner http.Handler, name string, weblog *log.Logger) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		l := &responseLogger{w: w}
		inner.ServeHTTP(l, r)
		logLine := buildLogLine(l, r, start)
		weblog.Println(logLine)
	})
}

func recovery(inner http.Handler, name string, weblog *log.Logger) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		l := &responseLogger{w: w}

		defer func() {
			if err := recover(); err != nil {
				b := make([]byte, 1024)
				runtime.Stack(b, false)
				logLine := buildLogLine(l, r, start)
				logLine = fmt.Sprintf("%s [panic:%s]\n%s", logLine, err, string(b))
				weblog.Println(logLine)
			}
		}()

		inner.ServeHTTP(l, r)
	})
}

func (h *handler) jsonError(err error, w http.ResponseWriter, status int) {
	if h.loggingEnabled {
		h.logger.Println(err)
	}
	http.Error(w, "", status)
}

func (h *handler) httpError(err error, w http.ResponseWriter, status int) {
	if h.loggingEnabled {
		h.logger.Println(err)
	}
	http.Error(w, "", status)
}

func (h *handler) serveGetUser(w http.ResponseWriter, r *http.Request) {

}

func readUserAction() {

}

func (h *handler) servePostUser(w http.ResponseWriter, r *http.Request) {

}

func (h *handler) createUser() {

}
func (h *handler) deleteUser() {

}
func (h *handler) changeUserPassword() {

}
func (h *handler) addUserPermissions() {

}
func (h *handler) removeUserPermissions() {

}

func (h *handler) serveGetRole(w http.ResponseWriter, r *http.Request) {

}

func (h *handler) servePostRole(w http.ResponseWriter, r *http.Request) {

}

func (h *handler) createrole() {

}
func (h *handler) deleterole() {

}
func (h *handler) addroleusers() {

}
func (h *handler) removeroleusers() {

}
func (h *handler) addrolepermissions() {

}
func (h *handler) removerolepermissions() {

}
func (h *handler) changerolename() {

}

func (h *handler) serveAuthorized(w http.ResponseWriter, r *http.Request) {

}

func parseCredentials() {

}
func (h *handler) jwtKeyLookup() {

}
func (h *handler) authenticate() {

}

func (p Permissions) ScopedPermissions() {

}
func (p Permissions) FromScopedPermissions() {

}
func requiredParameters() {

}

//TODO add lease
