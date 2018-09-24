package consul_test

import (
	"testing"
	"github.com/hashwing/prom-monitor/cluster/consul"
	"github.com/hashwing/log"
)

func Test_ElectLeader(t *testing.T){
	ld, err := consul.NewConsulLeader("prom-monitor")
	if err != nil {
		t.Error(err)
	}

	// 启动节点
	ld.Start()

	// 使用方法一: 判断当前节点是否是leader
	isLeader, err := ld.IsLeader()
	if err != nil {
		t.Error(err)
	}
	// 是leader,发起业务
	if isLeader {
		// 发起你的业务
	}

	// 使用方法二: 通过channel读取leader信息
	// 当竞选为leader时, 该channel会发出bool信号
	// 若与consul失联或者竞选失败,则发出false信号
	ch := ld.WaitLeaderCh()
	for {
		select {
		case sig := <-ch:
			if sig {
				log.Info("Start business")
			} else {
				log.Info("Can't be a leader")
			}
		}
	}

	// 关闭leader选举
	ld.Close()

}