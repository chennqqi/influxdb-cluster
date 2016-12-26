package cluster

import (
	"net"
	"time"

	"github.com/influxdata/influxdb/influxql"
)

type remoteIteratorCreator struct {
}

func (ric *remoteIteratorCreator) shardIDs() {

}

func (ric *remoteIteratorCreator) nodeIDs() {

}

func (ric *remoteIteratorCreator) nodeIteratorCreators() {

}

func (ric *remoteIteratorCreator) CreateIterator(opt influxql.IteratorOptions) (influxql.Iterator, error) {
	return nil
}

func (ric *remoteIteratorCreator) createIterators() {

}

func (ric *remoteIteratorCreator) createNodeIterator() {

}

func (ric *remoteIteratorCreator) FieldDimensions(sources influxql.Sources) (fields, dimensions map[string]struct{}, err error) {

}

func (ric *remoteIteratorCreator) ExpandSources(sources influxql.Sources) (influxql.Sources, error) {

}

type metaIteratorCreator struct {
}

func (mic *metaIteratorCreator) nodeIteratorCreators() {

}

func (mic *metaIteratorCreator) CreateIterator(opt influxql.IteratorOptions) (influxql.Iterator, error) {

}

func (mic *metaIteratorCreator) FieldDimensions(sources influxql.Sources) (fields, dimensions map[string]struct{}, err error) {

}

func (mic *metaIteratorCreator) ExpandSources(sources influxql.Sources) (influxql.Sources, error) {

}

type NodeDialer struct {
	address string
	timeout time.Duration
}

func (nd *NodeDialer) DialNode() error {
	conn, err := net.DialTimeout("tcp", nd.address, nd.timeout)
	if err != nil {
		return err
	}
	now := time.Now()
}
