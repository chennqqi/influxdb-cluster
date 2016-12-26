package cluster

type Tracker struct {
}

func (t *Tracker) Add() {

}

func (t *Tracker) Remove() {

}

func (t *Tracker) Tasks() {

}

func (t *Tracker) Task() {

}
func (t *Tracker) Exists() {

}

func (t *Tracker) id() {

}

//TODO revist this later
type tasks []int

func (t tasks) Len() int {
	return len(t)
}

func (t tasks) Swap(i, j int) {
	t[i], t[j] = t[j], t[i]
}

func (t tasks) Less(i, j int) bool {
	return t[i] < t[j]
}
