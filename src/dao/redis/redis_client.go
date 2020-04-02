package redis

import (
	"errors"
	"fmt"
	"math/rand"
	"reflect"
	"sync"
	"sync/atomic"
	"time"

	"github.com/FZambia/sentinel"
	"github.com/gomodule/redigo/redis"
)

const (
	connDailTimeout  = (2 * time.Second)
	readDailTimeout  = (2 * time.Second)
	writeDailTimeout = (2 * time.Second)
)

// SentinelCfg redis sentinel config
type SentinelCfg struct {
	Name  string   `json:"name"`
	Hosts []string `json:"hosts"`
}

// NewRedisPool 传入的参数是string/SentinelCfg
func NewRedisPool(cfg interface{}) *redis.Pool {
	if s, ok := cfg.(string); ok {
		return InitRedisPool(s)
	} else if sc, ok := cfg.(SentinelCfg); ok {
		return InitSentinelPool(sc.Name, sc.Hosts)
	} else {
		return nil
	}
}

// InitRedisPool init redis pool
func InitRedisPool(host string) *redis.Pool {
	return &redis.Pool{
		MaxIdle:     64,
		IdleTimeout: 60 * time.Second,
		TestOnBorrow: func(c redis.Conn, t time.Time) error {
			_, err := c.Do("PING")
			return err
		},
		Dial: func() (redis.Conn, error) {
			c, err := redis.DialTimeout("tcp", host, connDailTimeout, readDailTimeout, writeDailTimeout)
			if err != nil {
				return nil, err
			}
			return c, err
		},
	}
}

// InitRamcloudPool init redis ramcloud pool
func InitRamcloudPool(host []string) *redis.Pool {
	return &redis.Pool{
		MaxIdle:     64,
		IdleTimeout: 60 * time.Second,
		TestOnBorrow: func(c redis.Conn, t time.Time) error {
			_, err := c.Do("PING")
			return err
		},
		Dial: func() (redis.Conn, error) {
			rand.Seed(time.Now().UnixNano())
			c, err := redis.DialTimeout("tcp", host[rand.Int()%len(host)], connDailTimeout, readDailTimeout, writeDailTimeout)
			if err != nil {
				return nil, err
			}
			return c, err
		},
	}
}

// InitSentinelPool init redis sentinel pool
func InitSentinelPool(sentinelName string, addr []string) *redis.Pool {
	sntnl := &sentinel.Sentinel{
		Addrs:      addr,
		MasterName: sentinelName,
		Dial: func(addr string) (redis.Conn, error) {
			c, err := redis.DialTimeout("tcp", addr, connDailTimeout, readDailTimeout, writeDailTimeout)
			if err != nil {
				return nil, err
			}
			return c, nil
		},
	}
	return &redis.Pool{
		MaxIdle:     64,
		MaxActive:   256,
		Wait:        true,
		IdleTimeout: 60 * time.Second,
		Dial: func() (redis.Conn, error) {
			masterAddr, err := sntnl.MasterAddr()
			if err != nil {
				return nil, err
			}
			c, err := redis.DialTimeout("tcp", masterAddr, connDailTimeout, readDailTimeout, writeDailTimeout)
			if err != nil {
				return nil, err
			}
			return c, nil
		},
		TestOnBorrow: func(c redis.Conn, t time.Time) error {
			if time.Since(t) < time.Second {
				return nil
			}
			_, err := c.Do("PING")
			return err
		},
	}
}

// InitSentinelPoolWithConnNum init redis sentinel pool and limit conn number
func InitSentinelPoolWithConnNum(sentinelName string, addr []string, connNum int) *redis.Pool {
	sntnl := &sentinel.Sentinel{
		Addrs:      addr,
		MasterName: sentinelName,
		Dial: func(addr string) (redis.Conn, error) {
			c, err := redis.DialTimeout("tcp", addr, connDailTimeout, readDailTimeout, writeDailTimeout)
			if err != nil {
				return nil, err
			}
			return c, nil
		},
	}
	return &redis.Pool{
		MaxIdle:     connNum,
		MaxActive:   connNum,
		Wait:        true,
		IdleTimeout: 60 * time.Second,
		Dial: func() (redis.Conn, error) {
			masterAddr, err := sntnl.MasterAddr()
			if err != nil {
				return nil, err
			}
			c, err := redis.DialTimeout("tcp", masterAddr, connDailTimeout, readDailTimeout, writeDailTimeout)
			if err != nil {
				return nil, err
			}
			return c, nil
		},
		TestOnBorrow: func(c redis.Conn, t time.Time) error {
			_, err := c.Do("PING")
			return err
		},
	}
}

// InitSentinelPoolWithSlaveConnNum init reids sentinel pool(slaves) and limit conn number
func InitSentinelPoolWithSlaveConnNum(sentinelName string, addr []string, connNum int) *redis.Pool {
	sntnl := &sentinel.Sentinel{
		Addrs:      addr,
		MasterName: sentinelName,
		Dial: func(addr string) (redis.Conn, error) {
			c, err := redis.DialTimeout("tcp", addr, connDailTimeout, readDailTimeout, writeDailTimeout)
			if err != nil {
				return nil, err
			}
			return c, nil
		},
	}
	return &redis.Pool{
		MaxIdle:     connNum,
		MaxActive:   connNum,
		Wait:        true,
		IdleTimeout: 60 * time.Second,
		Dial: func() (redis.Conn, error) {
			slaveAddrs, err := sntnl.SlaveAddrs()
			if err != nil {
				return nil, err
			}
			if len(slaveAddrs) == 0 {
				return nil, errors.New("no slave")
			}
			c, err := redis.DialTimeout("tcp", slaveAddrs[rand.Int()%len(slaveAddrs)], connDailTimeout, readDailTimeout, writeDailTimeout)
			if err != nil {
				return nil, err
			}
			return c, nil
		},
		TestOnBorrow: func(c redis.Conn, t time.Time) error {
			_, err := c.Do("PING")
			return err
		},
	}
}

/////////////////////////////////////////////////////////////////////////////////////

var (
	// ErrSlaveInNoConfig error - slave in not active
	ErrSlaveInNoConfig = errors.New("slave is not active")
)

// RedisPool redis pool struct
type RedisPool struct {
	sentinelName  string
	sentinelAddrs []string
	opts          *PoolOptions
	masterPool    *redis.Pool
	slavePools    []*redis.Pool
	snt           *redisSentinel
	mutex         sync.Mutex
	closech       chan bool
	currIdx       uint64
	slaveSize     uint32
	alertCount    uint32
}

// NewRedisSentinelPool new redis pool
func NewRedisSentinelPool(sentinelName string, addrs []string, maxIdle, maxActive int, needSlave bool) *RedisPool {
	return NewRedisSentinelPoolOptions(sentinelName, addrs,
		PoolNeedSlave(needSlave),
		PoolNeedCache(true),
		PoolMaxIdle(maxIdle),
		PoolMaxActive(maxActive),
		PoolDialConnTimeout(defaultConnTimeout),
		PoolDialReadTimeout(defaultReadTimeout),
		PoolDialWriteTimeout(defaultWriteTimeout))
}

// NewRedisSentinelPoolOptions new redis pool with options
func NewRedisSentinelPoolOptions(sentinelName string, addrs []string, options ...RedisPoolOption) *RedisPool {
	do := &PoolOptions{
		needCache:    true,
		maxIdle:      maxIdle,
		maxActive:    maxActive,
		connTimeout:  defaultConnTimeout,
		readTimeout:  defaultReadTimeout,
		writeTimeout: defaultWriteTimeout,
	}
	for _, option := range options {
		option.f(do)
	}
	rand.Seed(time.Now().UnixNano())
	p := &RedisPool{
		sentinelName:  sentinelName,
		sentinelAddrs: addrs,
		opts:          do,
		closech:       make(chan bool, 1),
		currIdx:       uint64(rand.Int63()),
	}
	// init sentinel
	p.initSentinel()
	// init pool
	p.initMasterPool()
	if p.opts.needSlave {
		p.initSlavePool()
	}
	return p
}

// Get get master conn
func (imp *RedisPool) Get() (redis.Conn, error) {
	return imp.masterPool.Get(), nil
}

// GetSlave get slave conn
func (imp *RedisPool) GetSlave() (redis.Conn, error) {
	if !imp.opts.needSlave {
		return nil, ErrSlaveInNoConfig
	}
	atomic.AddUint64(&imp.currIdx, 1)
	size := atomic.LoadUint32(&imp.slaveSize)
	idx := (int)(imp.currIdx % (uint64)(size))
	imp.mutex.Lock()
	pool := imp.slavePools[idx]
	imp.mutex.Unlock()
	return pool.Get(), nil
}

// GetRandom get random conn
func (imp *RedisPool) GetRandom() (redis.Conn, error) {
	if !imp.opts.needSlave {
		return imp.masterPool.Get(), nil
	}
	randNum := (uint32)(rand.Intn(int(time.Now().Unix())))
	size := atomic.LoadUint32(&imp.slaveSize)
	idx := randNum % (size + 1)
	if idx <= 0 || size <= 0 {
		return imp.masterPool.Get(), nil
	}
	imp.mutex.Lock()
	pool := imp.slavePools[idx-1]
	imp.mutex.Unlock()
	return pool.Get(), nil
}

// GetPool get master pool
func (imp *RedisPool) GetPool() *redis.Pool {
	return imp.masterPool
}

// Close close redis pool
func (imp *RedisPool) Close() {
	if imp.opts.needCache {
		select {
		case imp.closech <- true:
		default:
			fmt.Printf("close fail")
		}
	}
}

func (imp *RedisPool) initSentinel() {
	imp.snt = &redisSentinel{
		sentinel: &sentinel.Sentinel{
			Addrs:      imp.sentinelAddrs,
			MasterName: imp.sentinelName,
			Dial: func(addr string) (redis.Conn, error) {
				c, err := redis.DialTimeout("tcp", addr, imp.opts.connTimeout, imp.opts.readTimeout, imp.opts.writeTimeout)
				if err != nil {
					fmt.Printf("dial fail, redisname=%v addr=%v err=%v", imp.sentinelName, imp.sentinelAddrs, err)
					return nil, err
				}
				return c, nil
			},
		},
	}
	// cache master and slave
	imp.cacheMaster()
	if imp.opts.needSlave {
		imp.cacheSlave()
	}
	if imp.opts.needCache {
		go imp.cacheTick()
	}
}

func (imp *RedisPool) cacheMaster() ([]string, error) {
	imp.snt.mutex.Lock()
	defer imp.snt.mutex.Unlock()
	master, err := imp.snt.sentinel.MasterAddr()
	if err != nil {
		fmt.Printf("MasterAddr fail, sentinelName=%v err=%v", imp.sentinelName, err)
		atomic.AddUint32(&imp.alertCount, 1)
		if atomic.LoadUint32(&imp.alertCount) > alertLimit {
			atomic.StoreUint32(&imp.alertCount, 0)
			fmt.Printf("get master fail, sentinelName=%v err=%v", imp.sentinelName, err)
		}
		return nil, err
	}
	masterAddr := []string{master}
	imp.snt.mp.Store("master", masterAddr)
	return masterAddr, nil
}

func (imp *RedisPool) cacheSlave() ([]string, error) {
	var slaveAddr []string
	slaves, err := imp.snt.sentinel.Slaves()
	if err != nil {
		atomic.AddUint32(&imp.alertCount, 1)
		if atomic.LoadUint32(&imp.alertCount) > alertLimit {
			atomic.StoreUint32(&imp.alertCount, 0)
			fmt.Printf("get slaves fail, sentinelName=%v err=%v", imp.sentinelName, err)
		}
		return nil, err
	}

	for _, slave := range slaves {
		if slave.Available() {
			slaveAddr = append(slaveAddr, slave.Addr())
		} else {
			fmt.Printf("redis  slave down slaveAddr=%v", slave.Addr())
		}
	}

	if len(slaveAddr) <= 0 {
		return nil, fmt.Errorf("not found slaveAddr, sentinelName=%v", imp.sentinelName)
	}

	lastSize := imp.slaveSize

	atomic.StoreUint32(&imp.slaveSize, (uint32)(len(slaveAddr)))

	imp.snt.mutex.Lock()
	imp.snt.mp.Store("slave", slaveAddr)
	imp.snt.mutex.Unlock()

	if len(slaveAddr) != int(lastSize) {
		fmt.Printf("redis  slave change current size=%v,lastSize=%v", len(slaveAddr), lastSize)
		imp.initSlavePool()
	}
	return slaveAddr, nil
}

func (imp *RedisPool) getMaster() ([]string, error) {
	val, ok := imp.snt.mp.Load("master")
	if !ok {
		master, err := imp.cacheMaster()
		if err != nil {
			fmt.Printf("cacheMaster fail, err=%v", err)
			return nil, err
		}
		return master, nil
	}
	return val.([]string), nil
}

func (imp *RedisPool) getSlaves() ([]string, error) {
	val, ok := imp.snt.mp.Load("slave")
	if !ok {
		slave, err := imp.cacheSlave()
		if err != nil {
			fmt.Printf("cacheSlave fail, err=%v", err)
			return nil, err
		}
		return slave, nil
	}
	return val.([]string), nil
}

func (imp *RedisPool) cacheTick() {
	timer := time.NewTimer(cacheTickTimeout)
	defer timer.Stop()
Loop:
	for {
		select {
		case <-imp.closech:
			break Loop
		case <-timer.C:
			imp.cacheMaster()
			if imp.opts.needSlave {
				imp.cacheSlave()
			}
			timer.Reset(cacheTickTimeout)
		}
	}
}

func (imp *RedisPool) initMasterPool() {
	imp.masterPool = &redis.Pool{
		MaxIdle:     imp.opts.maxIdle,
		MaxActive:   imp.opts.maxActive,
		Wait:        true,
		IdleTimeout: 60 * time.Second,
		Dial: func() (redis.Conn, error) {
			masterAddr, err := imp.getMaster()
			if err != nil {
				return nil, err
			}
			c, cErr := redis.DialTimeout("tcp", masterAddr[0], imp.opts.connTimeout, imp.opts.readTimeout, imp.opts.writeTimeout)
			if cErr != nil {
				//alert.Alert(fmt.Sprintf("master dial fail, sentinelName=%v addr=%v err=%v", imp.sentinelName, masterAddr, cErr))
				return nil, cErr
			}
			return c, nil
		},
		TestOnBorrow: func(c redis.Conn, t time.Time) error {
			if time.Since(t) < time.Second {
				return nil
			}
			if !sentinel.TestRole(c, "master") {
				return fmt.Errorf("Role check fail")
			}
			return nil
		},
	}
}

func (imp *RedisPool) initSlavePool() {
	slaveAddr, err := imp.getSlaves()
	if err != nil {
		fmt.Printf("initSlavePool getSlaves fail, err=%v", err)
		return
	}
	size := len(slaveAddr)
	tempSlavePools := make([]*redis.Pool, 0, size)
	for i := 0; i < size; i++ {
		slave := slaveAddr[i]
		slavePool := &redis.Pool{
			MaxIdle:     imp.opts.maxIdle,
			MaxActive:   imp.opts.maxActive,
			Wait:        true,
			IdleTimeout: 60 * time.Second,
			Dial: func() (redis.Conn, error) {
				c, err := redis.DialTimeout("tcp", slave, imp.opts.connTimeout, imp.opts.readTimeout, imp.opts.writeTimeout)
				if err != nil {
					//alert.Alert(fmt.Sprintf("slave dial fail, sentinelName=%v addr=%v err=%v", imp.sentinelName, slave, err))
				} else {
					fmt.Printf("initSlavePool  connect slave success addr=%v", slave)
					return c, nil
				}
				return nil, fmt.Errorf("all slave fail, sentinelNam=%v addr=%v", imp.sentinelName, slaveAddr)
			},
			TestOnBorrow: func(c redis.Conn, t time.Time) error {
				if time.Since(t) < time.Second {
					return nil
				}
				_, err := c.Do("PING")
				return err
			},
		}
		tempSlavePools = append(tempSlavePools, slavePool)
	} //end for

	imp.mutex.Lock()
	imp.slavePools = tempSlavePools
	fmt.Printf("initSlavePool set slavepool size=%v", len(imp.slavePools))
	imp.mutex.Unlock()
}

/////////////////////////////////////////////////////////////////////////////////////
// pool utils

const (
	maxIdle             = 64
	maxActive           = 256
	defaultReadTimeout  = (2 * time.Second)
	defaultWriteTimeout = (2 * time.Second)
	defaultConnTimeout  = (2 * time.Second)
	cacheTickTimeout    = (2 * time.Second)
	alertLimit          = 10
)

// redisSentinel redis sentinel
type redisSentinel struct {
	sentinel *sentinel.Sentinel
	mp       sync.Map
	mutex    sync.Mutex
}

// PoolOptions pool options
type PoolOptions struct {
	needSlave    bool
	needCache    bool
	maxIdle      int
	maxActive    int
	connTimeout  time.Duration
	readTimeout  time.Duration
	writeTimeout time.Duration
}

// RedisPoolOption redis pool option
type RedisPoolOption struct {
	f func(*PoolOptions)
}

// PoolNeedSlave specifies pool need slave pool
func PoolNeedSlave(slave bool) RedisPoolOption {
	return RedisPoolOption{func(do *PoolOptions) {
		do.needSlave = slave
	}}
}

// PoolNeedCache specifies pool need cache ip (master and slaves)
func PoolNeedCache(cache bool) RedisPoolOption {
	return RedisPoolOption{func(do *PoolOptions) {
		do.needCache = cache
	}}
}

// PoolMaxIdle specifies pool max idle conn
func PoolMaxIdle(maxIdle int) RedisPoolOption {
	return RedisPoolOption{func(do *PoolOptions) {
		do.maxIdle = maxIdle
	}}
}

// PoolMaxActive specifies pool max idle conn
func PoolMaxActive(maxActive int) RedisPoolOption {
	return RedisPoolOption{func(do *PoolOptions) {
		do.maxActive = maxActive
	}}
}

// PoolDialReadTimeout specifies pool dial read timeout
func PoolDialReadTimeout(d time.Duration) RedisPoolOption {
	return RedisPoolOption{func(do *PoolOptions) {
		do.readTimeout = d
	}}
}

// PoolDialWriteTimeout specifies pool dial write timeout
func PoolDialWriteTimeout(d time.Duration) RedisPoolOption {
	return RedisPoolOption{func(do *PoolOptions) {
		do.writeTimeout = d
	}}
}

// PoolDialConnTimeout specifies pool dial connect timeout
func PoolDialConnTimeout(d time.Duration) RedisPoolOption {
	return RedisPoolOption{func(do *PoolOptions) {
		do.connTimeout = d
	}}
}

/////////////////////////////////////////////////////////////////////////////////////
// utils function

// redis pipeline
type command struct {
	args     []interface{}
	expected interface{}
}

func pipeHDEL(client redis.Conn, hkey string, keys []string) error {
	size := len(keys)
	cmds := make([]command, size)
	for i := 0; i < size; i++ {
		cmds[i].args = []interface{}{"HDEL", hkey, keys[i]}
		cmds[i].expected = 1
	}
	for _, cmd := range cmds {
		if sErr := client.Send(cmd.args[0].(string), cmd.args[1:]...); sErr != nil {
			return fmt.Errorf("cmds fail, cmd=%v err=%v", cmd, sErr)
		}
	}
	if fErr := client.Flush(); fErr != nil {
		return fmt.Errorf("Flush fail, err=%v", fErr)
	}
	for _, cmd := range cmds {
		reply, rErr := redis.Int(client.Receive())
		if rErr != nil {
			fmt.Printf("Receive fail, cmd=%v err=%v", cmd, rErr)
		} else {
			if !reflect.DeepEqual(reply, cmd.expected) {
				fmt.Printf("DeepEqual fail, args=%v reply=%v expected=%v", cmd.args, reply, cmd.expected)
			}
		}
	}
	return nil
}

// DelBigHash delete big hash key
func DelBigHash(client redis.Conn, hkey string, step int, stepSleep time.Duration) error {
	cursor := "0"
	for {
		tmp, err := redis.Values(client.Do("HSCAN", hkey, cursor, "COUNT", step))
		if err != nil {
			return err
		}
		ks, ok := tmp[1].([]interface{})
		if !ok {
			return fmt.Errorf("val fail, val=%v", tmp[1])
		}
		size := len(ks)
		keys := make([]string, 0, size)
		for i := 0; i < size; i += 2 {
			keys = append(keys, string(ks[i].([]byte)))
		}
		dErr := pipeHDEL(client, hkey, keys)
		if dErr != nil {
			return dErr
		}
		cursor = string(tmp[0].([]byte))
		if cursor == "0" {
			break
		}
		if stepSleep > 0 {
			time.Sleep(stepSleep)
		}
	}
	return nil
}

