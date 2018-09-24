package cluster

// Cluster interface
type Cluster interface {
	Start()
	Close()
	IsLeader() (bool, error)

	// WaitLeaderCh 发送bool值
	// 为true时表示成为leader, 为false时表示竞选失败或者与consul失联
	WaitLeaderCh() <-chan bool
}