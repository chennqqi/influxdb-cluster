package cluster

import (
	"sync"

	"github.com/zhexuany/influxdb-cluster/rpc"
)

type remoteNodeError struct {
}

func (r remoteNodeError) Error() {

}

type MetaExecutor struct {
	wg sync.WaitGroup
}

func (e *MetaExecutor) ExecuteStatement() {

}

func (e *MetaExecutor) executeOnNode() {
}
func (e *MetaExecutor) dial() {

}
func (e *MetaExecutor) CreateShard() {

}
func (e *MetaExecutor) WriteToShard() {

}
func (e *MetaExecutor) DeleteDatabase() {

}
func (e *MetaExecutor) DeleteMeasurement() {

}
func (e *MetaExecutor) DeleteRetentionPolicy() {

}
func (e *MetaExecutor) DeleteSeries() {

}
func (e *MetaExecutor) DeleteShard() {

}
func (e *MetaExecutor) BackupShard() {

}
func (e *MetaExecutor) RestoreShard() {

}
func (e *MetaExecutor) IteratorCreator() {

}
func (e *MetaExecutor) Measurements() {

}
func (e *MetaExecutor) TagValues() {

}
func (e *MetaExecutor) MetaIteratorCreator() {

}

type uint64Slice []uint64

func (u uint64Slice) Len() int {
	return len(u)
}
func (u uint64Slice) Swap(i, j int) {
	u[i], u[j] = u[j], u[i]

}
func (u uint64Slice) Less(i, j int) bool {
	return u[i] < u[j]
}
