package redis

import (
	"new_frame/src/lgames"
	"strings"
)


type RedisDao struct {
	redisPool *RedisPool
	log *lgames.Logger
}


func NewRedis(addr string, masterName string, log *lgames.Logger) *RedisDao {
	addList := make([]string, 0)
	addSec := strings.Split(addr, ",")
	for _, v := range addSec {
		addList = append(addList, v)
	}

	redisPool := NewRedisSentinelPool(masterName, addList, 1, 1, false)
	if redisPool == nil {
		return nil
	}
	return &RedisDao{
		redisPool: redisPool,
		log: log,
	}
}

func (s *RedisDao) EXISTS(key string) (data interface{}, err error) {
	c, err := s.redisPool.Get()
	if err != nil {
		s.log.Error("EXISTS err key {}", key)
		return nil, err
	}
	defer c.Close()
	data, err = c.Do("EXISTS", key)

	return
}

func (s *RedisDao) Get(key string) (data interface{}, err error) {
	c, err := s.redisPool.Get()
	if err != nil {
		s.log.Error("Get err key {}", key)
		return nil, err
	}

	defer c.Close()
	data, err = c.Do("GET", key)
	return
}

func (s *RedisDao) GetSlave(key string) (data interface{}, err error) {
	c, err := s.redisPool.GetSlave()
	if err != nil {
		s.log.Error("Get err key {}", key)
		return nil, err
	}

	defer c.Close()
	data, err = c.Do("GET", key)

	return
}

func (s *RedisDao) GetRandom(key string) (data interface{}, err error) {
	c, err := s.redisPool.GetRandom()
	if err != nil {
		return nil, err
	}

	defer c.Close()
	data, err = c.Do("GET", key)

	return
}

func (s *RedisDao) Set(key string, val interface{}) (reply interface{}, err error) {
	c , err := s.redisPool.Get()
	if err != nil {
		s.log.Error("Set err key {}", key)
		return nil, err
	}

	defer c.Close()
	reply, err = c.Do("SET", key, val)
	return
}

func (s *RedisDao) SetWithExpired(key string, val interface{}, expired int) (reply interface{}, err error) {
	c, err := s.redisPool.Get()
	if err != nil {
		s.log.Error("SetWithExpired err key {}", key)
		return nil, err
	}

	defer c.Close()
	reply, err = c.Do("SET", key, val, "EX", expired)
	return
}

func (s *RedisDao) LPush(key string, val interface{}) (reply interface{}, err error) {
	c , err := s.redisPool.Get()
	if err != nil {
		s.log.Error("LPush err key {}", key)
		return nil, err
	}

	defer c.Close()
	reply, err = c.Do("LPUSH", key, val)
	return
}

func (s *RedisDao) SADD(key string, val interface{}) (reply interface{}, err error) {
	c, err  := s.redisPool.Get()
	if err != nil {
		s.log.Error("SADD err key {}", key)
		return nil, err
	}

	defer c.Close()
	reply, err = c.Do("SADD", key, val)
	return
}

func (s *RedisDao) SISMMBER(key string, val interface{}) (reply interface{}, err error) {
	c, err := s.redisPool.Get()
	if err != nil {
		s.log.Error("SISMMBER err key {}", key)
		return nil, err
	}

	defer c.Close()
	reply, err = c.Do("SISMEMBER", key, val)
	return
}

func (s *RedisDao) SMEMBERS(key string) (reply interface{}, err error) {
	c, err := s.redisPool.Get()
	if err != nil {
		s.log.Error("SMEMBERS err key {}", key)
		return nil, err
	}

	defer c.Close()
	reply, err = c.Do("SMEMBERS", key)
	return
}

func (s *RedisDao) SREM(key string, members ...string) (reply interface{}, err error) {
	c, err := s.redisPool.Get()
	if err != nil {
		s.log.Error("SREM err key {}", key)
		return nil, err
	}

	defer c.Close()
	params := make([]interface{}, len(members)+1)
	params[0] = key
	for i, v := range members {
		params[i+1] = v
	}
	reply, err = c.Do("SREM", params)
	return
}


func (s *RedisDao) HGET(key string, filed string) (reply interface{}, err error) {
	c, err := s.redisPool.Get()
	if err != nil {
		s.log.Error("HGET err key {}", key)
		return nil, err
	}

	defer c.Close()
	reply, err = c.Do("HGET", key, filed)
	return
}

func (s *RedisDao) HGETALL(key string) (reply interface{}, err error) {
	c, err  := s.redisPool.Get()
	if err != nil {
		s.log.Error("HGETALL err key {}", key)
		return nil, err
	}

	defer c.Close()
	reply, err = c.Do("HGETALL", key)
	return
}

func (s *RedisDao) HSET(key string, filed string, value int64) (reply interface{}, err error) {
	c, err := s.redisPool.Get()
	if err != nil {
		s.log.Error("HSET err key {}", key)
		return nil, err
	}

	defer c.Close()
	reply, err = c.Do("HSET", key, filed, value)
	return
}

func (s *RedisDao) HDEL(key string, fields []string) (reply interface{}, err error) {
	c , err := s.redisPool.Get()
	if err != nil {
		s.log.Error("HDEL err key {}", key)
		return nil, err
	}

	defer c.Close()
	reply, err = c.Do("HDEL", key, fields)
	return
}

func (s *RedisDao) HINCRBY(key string, filed string, increment int64) (reply interface{}, err error) {
	c, err := s.redisPool.Get()
	if err != nil {
		s.log.Error("HINCRBY err key {}", key)
		return nil, err
	}

	defer c.Close()
	reply, err = c.Do("HINCRBY", key, filed, increment)
	return
}

func (s *RedisDao) Push(key string, value string) (err error) {
	c , err := s.redisPool.Get()
	if err != nil {
		s.log.Error("Push err key {}", key)
		return  err
	}

	defer c.Close()
	_, err = c.Do("RPush", key, value)
	return err
}

func (s *RedisDao) CheckList(key string, lmtcount int64) (err error) {
	c, err  := s.redisPool.Get()
	if err != nil {
		s.log.Error("CheckList err key {}", key)
		return err
	}

	defer c.Close()
	listcnt, _ := c.Do("LLen", key)
	cnt := listcnt.(int64)
	s.log.Info("checklist %s have %v record\n", key, cnt)
	if cnt > lmtcount {
		for i := lmtcount; i <= cnt; i++ {
			c.Do("LPop", key)
		}
	}

	return nil
}

func (s *RedisDao) Publich(channel string, value string) (err error) {
	c, err := s.redisPool.Get()
	if err != nil {
		s.log.Error("Publich err channel {}", channel)
		return err
	}

	defer c.Close()
	_, err = c.Do("PUBLISH", channel, value)
	return err
}

func (s *RedisDao) LTRIM(key string, start int64, stop int64) (err error) {
	c, err := s.redisPool.Get()
	if err != nil {
		s.log.Error("LTRIM err key {}", key)
		return err
	}

	defer c.Close()
	_, err = c.Do("LTRIM", key, start, stop)
	return err
}
