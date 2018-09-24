package consul

import (
	"fmt"
	"sync"
	"time"

	"encoding/base64"

	"github.com/hashwing/log"
	"github.com/hashicorp/consul/api"
)


type ConsulLeader struct {
	sessionID string
	key       string
	client    *api.Client
	leaderCh  chan bool
	closeCh   chan struct{}
	mux       *sync.RWMutex
	ttl       string
	usech     bool
}

func (c *ConsulLeader) Start() {
	c.pollCheck()
}

func (c *ConsulLeader) Close() {
	// 销毁session
	if _, err := c.client.Session().Destroy(c.sessionID, nil); err != nil {
		log.Error(err)
	}
	close(c.closeCh)
}

func (c *ConsulLeader) WaitLeaderCh() <-chan bool {
	c.usech = true
	return c.leaderCh
}

func (c *ConsulLeader) IsLeader() (bool, error) {
	kvpair, _, err := c.client.KV().Get(c.key, nil)
	if err != nil {
		return false, err
	}
	if kvpair == nil {
		return false, nil
	}
	return kvpair.Session == c.sessionID, nil
}

// pollCheck used to invoke a long poll query
// to check which node is leader now
func (c *ConsulLeader) pollCheck() {
	dur := 2 * time.Second
	go func() {
		for {
			select {
			case <-c.closeCh:
				return
			default:
				// 如果session ID为空则创建
				if c.sessionID == "" {
					sessionID, err := c.genSessionID()
					if err != nil {
						log.Error(err)
					}
					c.sessionID = sessionID
				}

				kvpair, _, err := c.client.KV().Get(c.key, nil)
				if err != nil {
					log.Error(err)
					c.sendNotify(false)
				}

				// kvpair 为空表示还未有节点当选过leader
				// kvpair 的session为空,说明当前所有leader都已退出
				// 但kvpair不为空不代表此时leader就一定存在,中间存在一定时间的延迟
				if kvpair == nil || kvpair.Session == "" {
					log.Info("ppppp",kvpair,kvpair.Session)
					res, err := c.election()
					if err != nil {
						log.Error(err)
					}
					if res {
						log.Info("%s be a leader", c.sessionID)
						c.sendNotify(true)
					} else if !res && err == nil {
						// 竞选失败
						c.sendNotify(false)
					}
				} else if kvpair.Session == c.sessionID {
					// 如果leader已经存在并且是本身, 则为其延长session的ttl
					if err := c.updateSessionID(); err != nil {
						log.Error(err)
					}
				} else {
					// leader已存在但不是自身
					c.sendNotify(false)
				}

				// leader已经存在, 等待下一次查询操作的发起
				time.Sleep(dur)
			}
		}
	}()
}

func (c *ConsulLeader) genSessionID() (string, error) {
	sty := api.SessionEntry{
		TTL:      c.ttl,
		Behavior: "release",
	}
	sessionID, _, err := c.client.Session().Create(&sty, nil)
	if err != nil {
		return "", err
	}
	return sessionID, nil
}

// 如果session存在,更新方式为为其延长ttl
// 如果不存在,则创建
func (c *ConsulLeader) updateSessionID() error {
	var newSessionID string
	sty, _, err := c.client.Session().Renew(c.sessionID, nil)
	if err != nil {
		return err
	}
	// 如果session id不存在,重新生成
	if sty == nil {
		newSessionID, err = c.genSessionID()
		if err != nil {
			return err
		}
	} else {
		newSessionID = sty.ID
	}

	c.mux.Lock()
	c.sessionID = newSessionID
	c.mux.Unlock()

	return err
}

func (c *ConsulLeader) election() (bool, error) {

	// 每次竞选前更新session id
	if err := c.updateSessionID(); err != nil {
		return false, err
	}

	kvpair := api.KVPair{
		Key:     c.key,
		Session: c.sessionID,
	}
	res, _, err := c.client.KV().Acquire(&kvpair, nil)
	return res, err
}

func (c *ConsulLeader) sendNotify(n bool) {
	if c.usech {
		go func() {
			c.leaderCh <- n
		}()
	}
}

// NewConsulLeader return a ConsulLeader which implement Leader interface
func NewConsulLeader(serviceName string) (*ConsulLeader, error) {
	// 服务名不可为空
	if serviceName == "" {
		return nil, fmt.Errorf("service name must no empty")
	}

	// 建立client
	config :=api.DefaultConfig()
	config.Address="10.21.1.254:8500"
	client, err := api.NewClient(config)
	if err != nil {
		return nil, err
	}

	leader := ConsulLeader{
		client:   client,
		key:      genKey(serviceName),
		leaderCh: make(chan bool),
		mux:      &sync.RWMutex{},
		closeCh:  make(chan struct{}),
		ttl:      "10s",
	}

	return &leader, nil
}

func genKey(serviceName string) string {
	baseKey := []byte(serviceName + "leader")
	return fmt.Sprintf("service/%s/%s", serviceName, base64.StdEncoding.EncodeToString([]byte(baseKey)))
}
