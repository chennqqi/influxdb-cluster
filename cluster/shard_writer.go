package cluster

type ShardWriter struct {
	cp clientPool
}

func (s *ShardWriter) WriteShard() {

}

func (s *ShardWriter) WriteShardBinary() {

}

func (s *ShardWriter) dial(nodeID uint64) {
	p, ok := s.cp.getPool(nodeID)
	p := NewBoundedPool(0, 0, 0, f)
	conn, err := s.cp.conn(nodeID)
	s.cp.setPool(nodeID, p)
}

func (s *ShardWriter) Close() {
	s.cp.close()
}

type connFactory struct {
}

func (c *connFactory) dial() {

}
