package control

import ()

type NodeByAddr struct {
}

func (n NodeByAddr) Len() int {
	return len(n)
}

func (n NodeByAddr) Less(i, j int) bool {
}

func (n NodeByAddr) Swap(i, j int) {

}

type DataNodeByAddr struct {
}

func (d DataNodeByAddr) Len() int {

}

func (d DataNodeByAddr) Less(i, j int) bool {

}

func (d DataNodeByAddr) Swap(i, j int) {

}
