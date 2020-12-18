// Package redisgo 基于github.com/gomodule/redigo，封装了redis缓存的实现。
// 使用时，可以参考 http://redisdoc.com/ 和 http://www.runoob.com/redis/ 中对redis命令及用法的讲解。
// 对于没有封装的命令，也可以参考这里的实现方法，直接调用 `Do` 方法直接调用redis命令。
package redisgo

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	// "github.com/aiscrm/cache"

	"github.com/gomodule/redigo/redis"
)

// Cacher 先构建一个Cacher实例，然后将配置参数传入该实例的StartAndGC方法来初始化实例和程序进程退出后的清理工作。
type Cacher struct {
	pool      *redis.Pool
	prefix    string
	marshal   func(v interface{}) ([]byte, error)
	unmarshal func(data []byte, v interface{}) error
}

// Options redis配置参数
type Options struct {
	Network     string                                 // 通讯协议，默认为 tcp
	Addr        string                                 // redis服务的地址，默认为 127.0.0.1:6379
	Password    string                                 // redis鉴权密码
	Db          int                                    // 数据库
	MaxActive   int                                    // 最大活动连接数，值为0时表示不限制
	MaxIdle     int                                    // 最大空闲连接数
	IdleTimeout int                                    // 空闲连接的超时时间，超过该时间则关闭连接。单位为秒。默认值是5分钟。值为0时表示不关闭空闲连接。此值应该总是大于redis服务的超时时间。
	Prefix      string                                 // 键名前缀
	Marshal     func(v interface{}) ([]byte, error)    // 数据序列化方法，默认使用json.Marshal序列化
	Unmarshal   func(data []byte, v interface{}) error // 数据反序列化方法，默认使用json.Unmarshal序列化
}

// New 根据配置参数创建redis工具实例
func New(options Options) (*Cacher, error) {
	r := &Cacher{}
	err := r.StartAndGC(options)
	return r, err
}

// StartAndGC 使用 Options 初始化redis，并在程序进程退出时关闭连接池。
func (c *Cacher) StartAndGC(options interface{}) error {
	switch opts := options.(type) {
	case Options:
		if opts.Network == "" {
			opts.Network = "tcp"
		}
		if opts.Addr == "" {
			opts.Addr = "127.0.0.1:6379"
		}
		if opts.MaxIdle == 0 {
			opts.MaxIdle = 3
		}
		if opts.IdleTimeout == 0 {
			opts.IdleTimeout = 300
		}
		if opts.Marshal == nil {
			c.marshal = json.Marshal
		}
		if opts.Unmarshal == nil {
			c.unmarshal = json.Unmarshal
		}
		pool := &redis.Pool{
			MaxActive:   opts.MaxActive,
			MaxIdle:     opts.MaxIdle,
			IdleTimeout: time.Duration(opts.IdleTimeout) * time.Second,

			Dial: func() (redis.Conn, error) {
				conn, err := redis.Dial(opts.Network, opts.Addr)
				if err != nil {
					return nil, err
				}
				if opts.Password != "" {
					if _, err := conn.Do("AUTH", opts.Password); err != nil {
						conn.Close()
						return nil, err
					}
				}
				if _, err := conn.Do("SELECT", opts.Db); err != nil {
					conn.Close()
					return nil, err
				}
				return conn, err
			},

			TestOnBorrow: func(conn redis.Conn, t time.Time) error {
				_, err := conn.Do("PING")
				return err
			},
		}

		c.pool = pool
		c.closePool()
		return nil
	default:
		return errors.New("Unsupported options")
	}
}

// Do 执行redis命令并返回结果。执行时从连接池获取连接并在执行完命令后关闭连接。
func (c *Cacher) Do(commandName string, args ...interface{}) (reply interface{}, err error) {
	conn := c.pool.Get()
	defer conn.Close()
	return conn.Do(commandName, args...)
}

// Get 获取键值。一般不直接使用该值，而是配合下面的工具类方法获取具体类型的值，或者直接使用github.com/gomodule/redigo/redis包的工具方法。
func (c *Cacher) Get(key string) (interface{}, error) {
	return c.Do("GET", c.getKey(key))
}

// GetString 获取string类型的键值
func (c *Cacher) GetString(key string) (string, error) {
	return String(c.Get(key))
}

// GetInt 获取int类型的键值
func (c *Cacher) GetInt(key string) (int, error) {
	return Int(c.Get(key))
}

// GetInt64 获取int64类型的键值
func (c *Cacher) GetInt64(key string) (int64, error) {
	return Int64(c.Get(key))
}

// GetBool 获取bool类型的键值
func (c *Cacher) GetBool(key string) (bool, error) {
	return Bool(c.Get(key))
}

// GetObject 获取非基本类型stuct的键值。在实现上，使用json的Marshal和Unmarshal做序列化存取。
func (c *Cacher) GetObject(key string, val interface{}) error {
	reply, err := c.Get(key)
	return c.decode(reply, err, val)
}

// Set 存并设置有效时长。时长的单位为秒。
// 基础类型直接保存，其他用json.Marshal后转成string保存。
func (c *Cacher) Set(key string, val interface{}, expire int64) error {
	value, err := c.encode(val)
	if err != nil {
		return err
	}
	if expire > 0 {
		_, err := c.Do("SETEX", c.getKey(key), expire, value)
		return err
	}
	_, err = c.Do("SET", c.getKey(key), value)
	return err
}

// Exists 检查键是否存在
func (c *Cacher) Exists(key string) (bool, error) {
	return Bool(c.Do("EXISTS", c.getKey(key)))
}

//Del 删除键
func (c *Cacher) Del(key string) error {
	_, err := c.Do("DEL", c.getKey(key))
	return err
}

// Flush 清空当前数据库中的所有 key，慎用！
func (c *Cacher) Flush() error {
	_, err := c.Do("FLUSHDB")
	return err
}

// TTL 以秒为单位。当 key 不存在时，返回 -2 。 当 key 存在但没有设置剩余生存时间时，返回 -1
func (c *Cacher) TTL(key string) (ttl int64, err error) {
	return Int64(c.Do("TTL", c.getKey(key)))
}

// Expire 设置键过期时间，expire的单位为秒
func (c *Cacher) Expire(key string, expire int64) error {
	_, err := Bool(c.Do("EXPIRE", c.getKey(key), expire))
	return err
}

// Incr 将 key 中储存的数字值增一
func (c *Cacher) Incr(key string) (val int64, err error) {
	return Int64(c.Do("INCR", c.getKey(key)))
}

// IncrBy 将 key 所储存的值加上给定的增量值（increment）。
func (c *Cacher) IncrBy(key string, amount int64) (val int64, err error) {
	return Int64(c.Do("INCRBY", c.getKey(key), amount))
}

// Decr 将 key 中储存的数字值减一。
func (c *Cacher) Decr(key string) (val int64, err error) {
	return Int64(c.Do("DECR", c.getKey(key)))
}

// DecrBy key 所储存的值减去给定的减量值（decrement）。
func (c *Cacher) DecrBy(key string, amount int64) (val int64, err error) {
	return Int64(c.Do("DECRBY", c.getKey(key), amount))
}

// HMSet 将一个map存到Redis hash，同时设置有效期，单位：秒
// Example:
//
// ```golang
// m := make(map[string]interface{})
// m["name"] = "corel"
// m["age"] = 23
// err := c.HMSet("user", m, 10)
// ```
func (c *Cacher) HMSet(key string, val interface{}, expire int) (err error) {
	conn := c.pool.Get()
	defer conn.Close()
	err = conn.Send("HMSET", redis.Args{}.Add(c.getKey(key)).AddFlat(val)...)
	if err != nil {
		return
	}
	if expire > 0 {
		err = conn.Send("EXPIRE", c.getKey(key), int64(expire))
	}
	if err != nil {
		return
	}
	conn.Flush()
	_, err = conn.Receive()
	return
}

/** Redis hash 是一个string类型的field和value的映射表，hash特别适合用于存储对象。 **/

// HSet 将哈希表 key 中的字段 field 的值设为 val
// Example:
//
// ```golang
// _, err := c.HSet("user", "age", 23)
// ```
func (c *Cacher) HSet(key, field string, val interface{}) (interface{}, error) {
	value, err := c.encode(val)
	if err != nil {
		return nil, err
	}
	return c.Do("HSET", c.getKey(key), field, value)
}

// HGet 获取存储在哈希表中指定字段的值
// Example:
//
// ```golang
// val, err := c.HGet("user", "age")
// ```
func (c *Cacher) HGet(key, field string) (reply interface{}, err error) {
	reply, err = c.Do("HGET", c.getKey(key), field)
	return
}

// HGetString HGet的工具方法，当字段值为字符串类型时使用
func (c *Cacher) HGetString(key, field string) (reply string, err error) {
	reply, err = String(c.HGet(key, field))
	return
}

// HGetInt HGet的工具方法，当字段值为int类型时使用
func (c *Cacher) HGetInt(key, field string) (reply int, err error) {
	reply, err = Int(c.HGet(key, field))
	return
}

// HGetInt64 HGet的工具方法，当字段值为int64类型时使用
func (c *Cacher) HGetInt64(key, field string) (reply int64, err error) {
	reply, err = Int64(c.HGet(key, field))
	return
}

// HGetBool HGet的工具方法，当字段值为bool类型时使用
func (c *Cacher) HGetBool(key, field string) (reply bool, err error) {
	reply, err = Bool(c.HGet(key, field))
	return
}

// HGetObject HGet的工具方法，当字段值为非基本类型的stuct时使用
func (c *Cacher) HGetObject(key, field string, val interface{}) error {
	reply, err := c.HGet(key, field)
	return c.decode(reply, err, val)
}

// HGetAll HGetAll("key", &val)
func (c *Cacher) HGetAll(key string, val interface{}) error {
	v, err := redis.Values(c.Do("HGETALL", c.getKey(key)))
	if err != nil {
		return err
	}

	if err := redis.ScanStruct(v, val); err != nil {
		fmt.Println(err)
	}
	//fmt.Printf("%+v\n", val)
	return err
}

/**
Redis列表是简单的字符串列表，按照插入顺序排序。你可以添加一个元素到列表的头部（左边）或者尾部（右边）
**/

// BLPop 它是 LPOP 命令的阻塞版本，当给定列表内没有任何元素可供弹出的时候，连接将被 BLPOP 命令阻塞，直到等待超时或发现可弹出元素为止。
// 超时参数 timeout 接受一个以秒为单位的数字作为值。超时参数设为 0 表示阻塞时间可以无限期延长(block indefinitely) 。
func (c *Cacher) BLPop(key string, timeout int) (interface{}, error) {
	values, err := redis.Values(c.Do("BLPOP", c.getKey(key), timeout))
	if err != nil {
		return nil, err
	}
	if len(values) != 2 {
		return nil, fmt.Errorf("redisgo: unexpected number of values, got %d", len(values))
	}
	return values[1], err
}

// BLPopInt BLPop的工具方法，元素类型为int时
func (c *Cacher) BLPopInt(key string, timeout int) (int, error) {
	return Int(c.BLPop(key, timeout))
}

// BLPopInt64 BLPop的工具方法，元素类型为int64时
func (c *Cacher) BLPopInt64(key string, timeout int) (int64, error) {
	return Int64(c.BLPop(key, timeout))
}

// BLPopString BLPop的工具方法，元素类型为string时
func (c *Cacher) BLPopString(key string, timeout int) (string, error) {
	return String(c.BLPop(key, timeout))
}

// BLPopBool BLPop的工具方法，元素类型为bool时
func (c *Cacher) BLPopBool(key string, timeout int) (bool, error) {
	return Bool(c.BLPop(key, timeout))
}

// BLPopObject BLPop的工具方法，元素类型为object时
func (c *Cacher) BLPopObject(key string, timeout int, val interface{}) error {
	reply, err := c.BLPop(key, timeout)
	return c.decode(reply, err, val)
}

// BRPop 它是 RPOP 命令的阻塞版本，当给定列表内没有任何元素可供弹出的时候，连接将被 BRPOP 命令阻塞，直到等待超时或发现可弹出元素为止。
// 超时参数 timeout 接受一个以秒为单位的数字作为值。超时参数设为 0 表示阻塞时间可以无限期延长(block indefinitely) 。
func (c *Cacher) BRPop(key string, timeout int) (interface{}, error) {
	values, err := redis.Values(c.Do("BRPOP", c.getKey(key), timeout))
	if err != nil {
		return nil, err
	}
	if len(values) != 2 {
		return nil, fmt.Errorf("redisgo: unexpected number of values, got %d", len(values))
	}
	return values[1], err
}

// BRPopInt BRPop的工具方法，元素类型为int时
func (c *Cacher) BRPopInt(key string, timeout int) (int, error) {
	return Int(c.BRPop(key, timeout))
}

// BRPopInt64 BRPop的工具方法，元素类型为int64时
func (c *Cacher) BRPopInt64(key string, timeout int) (int64, error) {
	return Int64(c.BRPop(key, timeout))
}

// BRPopString BRPop的工具方法，元素类型为string时
func (c *Cacher) BRPopString(key string, timeout int) (string, error) {
	return String(c.BRPop(key, timeout))
}

// BRPopBool BRPop的工具方法，元素类型为bool时
func (c *Cacher) BRPopBool(key string, timeout int) (bool, error) {
	return Bool(c.BRPop(key, timeout))
}

// BRPopObject BRPop的工具方法，元素类型为object时
func (c *Cacher) BRPopObject(key string, timeout int, val interface{}) error {
	reply, err := c.BRPop(key, timeout)
	return c.decode(reply, err, val)
}

// LPop 移出并获取列表中的第一个元素（表头，左边）
func (c *Cacher) LPop(key string) (interface{}, error) {
	return c.Do("LPOP", c.getKey(key))
}

// LPopInt 移出并获取列表中的第一个元素（表头，左边），元素类型为int
func (c *Cacher) LPopInt(key string) (int, error) {
	return Int(c.LPop(key))
}

// LPopInt64 移出并获取列表中的第一个元素（表头，左边），元素类型为int64
func (c *Cacher) LPopInt64(key string) (int64, error) {
	return Int64(c.LPop(key))
}

// LPopString 移出并获取列表中的第一个元素（表头，左边），元素类型为string
func (c *Cacher) LPopString(key string) (string, error) {
	return String(c.LPop(key))
}

// LPopBool 移出并获取列表中的第一个元素（表头，左边），元素类型为bool
func (c *Cacher) LPopBool(key string) (bool, error) {
	return Bool(c.LPop(key))
}

// LPopObject 移出并获取列表中的第一个元素（表头，左边），元素类型为非基本类型的struct
func (c *Cacher) LPopObject(key string, val interface{}) error {
	reply, err := c.LPop(key)
	return c.decode(reply, err, val)
}

// RPop 移出并获取列表中的最后一个元素（表尾，右边）
func (c *Cacher) RPop(key string) (interface{}, error) {
	return c.Do("RPOP", c.getKey(key))
}

// RPopInt 移出并获取列表中的最后一个元素（表尾，右边），元素类型为int
func (c *Cacher) RPopInt(key string) (int, error) {
	return Int(c.RPop(key))
}

// RPopInt64 移出并获取列表中的最后一个元素（表尾，右边），元素类型为int64
func (c *Cacher) RPopInt64(key string) (int64, error) {
	return Int64(c.RPop(key))
}

// RPopString 移出并获取列表中的最后一个元素（表尾，右边），元素类型为string
func (c *Cacher) RPopString(key string) (string, error) {
	return String(c.RPop(key))
}

// RPopBool 移出并获取列表中的最后一个元素（表尾，右边），元素类型为bool
func (c *Cacher) RPopBool(key string) (bool, error) {
	return Bool(c.RPop(key))
}

// RPopObject 移出并获取列表中的最后一个元素（表尾，右边），元素类型为非基本类型的struct
func (c *Cacher) RPopObject(key string, val interface{}) error {
	reply, err := c.RPop(key)
	return c.decode(reply, err, val)
}

// LPush 将一个值插入到列表头部
func (c *Cacher) LPush(key string, member interface{}) error {
	value, err := c.encode(member)
	if err != nil {
		return err
	}
	_, err = c.Do("LPUSH", c.getKey(key), value)
	return err
}

// RPush 将一个值插入到列表尾部
func (c *Cacher) RPush(key string, member interface{}) error {
	value, err := c.encode(member)
	if err != nil {
		return err
	}
	_, err = c.Do("RPUSH", c.getKey(key), value)
	return err
}

// LREM 根据参数 count 的值，移除列表中与参数 member 相等的元素。
// count 的值可以是以下几种：
// count > 0 : 从表头开始向表尾搜索，移除与 member 相等的元素，数量为 count 。
// count < 0 : 从表尾开始向表头搜索，移除与 member 相等的元素，数量为 count 的绝对值。
// count = 0 : 移除表中所有与 member 相等的值。
// 返回值：被移除元素的数量。
func (c *Cacher) LREM(key string, count int, member interface{}) (int, error) {
	return Int(c.Do("LREM", c.getKey(key), count, member))
}

// LLen 获取列表的长度
func (c *Cacher) LLen(key string) (int64, error) {
	return Int64(c.Do("LLen", c.getKey(key)))
}

// LRange 返回列表 key 中指定区间内的元素，区间以偏移量 start 和 stop 指定。
// 下标(index)参数 start 和 stop 都以 0 为底，也就是说，以 0 表示列表的第一个元素，以 1 表示列表的第二个元素，以此类推。
// 你也可以使用负数下标，以 -1 表示列表的最后一个元素， -2 表示列表的倒数第二个元素，以此类推。
// 和编程语言区间函数的区别：end 下标也在 LRANGE 命令的取值范围之内(闭区间)。
func (c *Cacher) LRange(key string, start, end int) (interface{}, error) {
	return c.Do("LRANGE", c.getKey(key), start, end)
}

/**
Redis 有序集合和集合一样也是string类型元素的集合,且不允许重复的成员。
不同的是每个元素都会关联一个double类型的分数。redis正是通过分数来为集合中的成员进行从小到大的排序。
有序集合的成员是唯一的,但分数(score)却可以重复。
集合是通过哈希表实现的，所以添加，删除，查找的复杂度都是O(1)。
**/

// ZAdd 将一个 member 元素及其 score 值加入到有序集 key 当中。
func (c *Cacher) ZAdd(key string, score int64, member string) (reply interface{}, err error) {
	return c.Do("ZADD", c.getKey(key), score, member)
}

// ZRem 移除有序集 key 中的一个成员，不存在的成员将被忽略。
func (c *Cacher) ZRem(key string, member string) (reply interface{}, err error) {
	return c.Do("ZREM", c.getKey(key), member)
}

// ZScore 返回有序集 key 中，成员 member 的 score 值。 如果 member 元素不是有序集 key 的成员，或 key 不存在，返回 nil 。
func (c *Cacher) ZScore(key string, member string) (int64, error) {
	return Int64(c.Do("ZSCORE", c.getKey(key), member))
}

// ZRank 返回有序集中指定成员的排名。其中有序集成员按分数值递增(从小到大)顺序排列。score 值最小的成员排名为 0
func (c *Cacher) ZRank(key, member string) (int64, error) {
	return Int64(c.Do("ZRANK", c.getKey(key), member))
}

// ZRevrank 返回有序集中成员的排名。其中有序集成员按分数值递减(从大到小)排序。分数值最大的成员排名为 0 。
func (c *Cacher) ZRevrank(key, member string) (int64, error) {
	return Int64(c.Do("ZREVRANK", c.getKey(key), member))
}

// ZRange 返回有序集中，指定区间内的成员。其中成员的位置按分数值递增(从小到大)来排序。具有相同分数值的成员按字典序(lexicographical order )来排列。
// 以 0 表示有序集第一个成员，以 1 表示有序集第二个成员，以此类推。或 以 -1 表示最后一个成员， -2 表示倒数第二个成员，以此类推。
func (c *Cacher) ZRange(key string, from, to int64) (map[string]int64, error) {
	return redis.Int64Map(c.Do("ZRANGE", c.getKey(key), from, to, "WITHSCORES"))
}

// ZRevrange 返回有序集中，指定区间内的成员。其中成员的位置按分数值递减(从大到小)来排列。具有相同分数值的成员按字典序(lexicographical order )来排列。
// 以 0 表示有序集第一个成员，以 1 表示有序集第二个成员，以此类推。或 以 -1 表示最后一个成员， -2 表示倒数第二个成员，以此类推。
func (c *Cacher) ZRevrange(key string, from, to int64) (map[string]int64, error) {
	return redis.Int64Map(c.Do("ZREVRANGE", c.getKey(key), from, to, "WITHSCORES"))
}

// ZRangeByScore 返回有序集合中指定分数区间的成员列表。有序集成员按分数值递增(从小到大)次序排列。
// 具有相同分数值的成员按字典序来排列
func (c *Cacher) ZRangeByScore(key string, from, to, offset int64, count int) (map[string]int64, error) {
	return redis.Int64Map(c.Do("ZRANGEBYSCORE", c.getKey(key), from, to, "WITHSCORES", "LIMIT", offset, count))
}

// ZRevrangeByScore 返回有序集中指定分数区间内的所有的成员。有序集成员按分数值递减(从大到小)的次序排列。
// 具有相同分数值的成员按字典序来排列
func (c *Cacher) ZRevrangeByScore(key string, from, to, offset int64, count int) (map[string]int64, error) {
	return redis.Int64Map(c.Do("ZREVRANGEBYSCORE", c.getKey(key), from, to, "WITHSCORES", "LIMIT", offset, count))
}

/**
Redis 发布订阅(pub/sub)是一种消息通信模式：发送者(pub)发送消息，订阅者(sub)接收消息。
Redis 客户端可以订阅任意数量的频道。
当有新消息通过 PUBLISH 命令发送给频道 channel 时， 这个消息就会被发送给订阅它的所有客户端。
**/

// Publish 将信息发送到指定的频道，返回接收到信息的订阅者数量
func (c *Cacher) Publish(channel, message string) (int, error) {
	return Int(c.Do("PUBLISH", channel, message))
}

// Subscribe 订阅给定的一个或多个频道的信息。
// 支持redis服务停止或网络异常等情况时，自动重新订阅。
// 一般的程序都是启动后开启一些固定channel的订阅，也不会动态的取消订阅，这种场景下可以使用本方法。
// 复杂场景的使用可以直接参考 https://godoc.org/github.com/gomodule/redigo/redis#hdr-Publish_and_Subscribe
func (c *Cacher) Subscribe(onMessage func(channel string, data []byte) error, channels ...string) error {
	conn := c.pool.Get()
	psc := redis.PubSubConn{Conn: conn}
	err := psc.Subscribe(redis.Args{}.AddFlat(channels)...)
	// 如果订阅失败，休息1秒后重新订阅（比如当redis服务停止服务或网络异常）
	if err != nil {
		fmt.Println(err)
		time.Sleep(time.Second)
		return c.Subscribe(onMessage, channels...)
	}
	quit := make(chan int, 1)

	// 处理消息
	go func() {
		for {
			switch v := psc.Receive().(type) {
			case redis.Message:
				// fmt.Printf("%s: message: %s\n", v.Channel, v.Data)
				go onMessage(v.Channel, v.Data)
			case redis.Subscription:
				fmt.Printf("%s: %s %d\n", v.Channel, v.Kind, v.Count)
			case error:
				quit <- 1
				fmt.Println(err)
				return
			}
		}
	}()

	// 异常情况下自动重新订阅
	go func() {
		<-quit
		time.Sleep(time.Second)
		psc.Close()
		c.Subscribe(onMessage, channels...)
	}()
	return err
}

/**
GEO 地理位置
*/

// GeoOptions 用于GEORADIUS和GEORADIUSBYMEMBER命令的参数
type GeoOptions struct {
	WithCoord bool
	WithDist  bool
	WithHash  bool
	Order     string // ASC从近到远，DESC从远到近
	Count     int
}

// GeoResult 用于GEORADIUS和GEORADIUSBYMEMBER命令的查询结果
type GeoResult struct {
	Name      string
	Longitude float64
	Latitude  float64
	Dist      float64
	Hash      int64
}

// GeoAdd 将给定的空间元素（纬度、经度、名字）添加到指定的键里面，这些数据会以有序集合的形式被储存在键里面，所以删除可以使用`ZREM`。
func (c *Cacher) GeoAdd(key string, longitude, latitude float64, member string) error {
	_, err := redis.Int(c.Do("GEOADD", c.getKey(key), longitude, latitude, member))
	return err
}

// GeoPos 从键里面返回所有给定位置元素的位置（经度和纬度）。
func (c *Cacher) GeoPos(key string, members ...interface{}) ([]*[2]float64, error) {
	args := redis.Args{}
	args = args.Add(c.getKey(key))
	args = args.Add(members...)
	return redis.Positions(c.Do("GEOPOS", args...))
}

// GeoDist 返回两个给定位置之间的距离。
// 如果两个位置之间的其中一个不存在， 那么命令返回空值。
// 指定单位的参数 unit 必须是以下单位的其中一个：
// m 表示单位为米。
// km 表示单位为千米。
// mi 表示单位为英里。
// ft 表示单位为英尺。
// 如果用户没有显式地指定单位参数， 那么 GEODIST 默认使用米作为单位。
func (c *Cacher) GeoDist(key string, member1, member2, unit string) (float64, error) {
	_, err := redis.Float64(c.Do("GEODIST", c.getKey(key), member1, member2, unit))
	return 0, err
}

// GeoRadius 以给定的经纬度为中心， 返回键包含的位置元素当中， 与中心的距离不超过给定最大距离的所有位置元素。
func (c *Cacher) GeoRadius(key string, longitude, latitude, radius float64, unit string, options GeoOptions) ([]*GeoResult, error) {
	args := redis.Args{}
	args = args.Add(c.getKey(key), longitude, latitude, radius, unit)
	if options.WithDist {
		args = args.Add("WITHDIST")
	}
	if options.WithCoord {
		args = args.Add("WITHCOORD")
	}
	if options.WithHash {
		args = args.Add("WITHHASH")
	}
	if options.Order != "" {
		args = args.Add(options.Order)
	}
	if options.Count > 0 {
		args = args.Add("Count", options.Count)
	}

	reply, err := c.Do("GEORADIUS", args...)
	return toGeoResult(reply, err, options)
}

// GeoRadiusByMember 这个命令和 GEORADIUS 命令一样， 都可以找出位于指定范围内的元素， 但是 GEORADIUSBYMEMBER 的中心点是由给定的位置元素决定的， 而不是像 GEORADIUS 那样， 使用输入的经度和纬度来决定中心点。
func (c *Cacher) GeoRadiusByMember(key string, member string, radius float64, unit string, options GeoOptions) ([]*GeoResult, error) {
	args := redis.Args{}
	args = args.Add(c.getKey(key), member, radius, unit)
	if options.WithDist {
		args = args.Add("WITHDIST")
	}
	if options.WithCoord {
		args = args.Add("WITHCOORD")
	}
	if options.WithHash {
		args = args.Add("WITHHASH")
	}
	if options.Order != "" {
		args = args.Add(options.Order)
	}
	if options.Count > 0 {
		args = args.Add("Count", options.Count)
	}

	reply, err := c.Do("GEORADIUSBYMEMBER", args...)
	return toGeoResult(reply, err, options)
}

// GeoHash 返回一个或多个位置元素的 Geohash 表示。
func (c *Cacher) GeoHash(key string, members ...interface{}) ([]string, error) {
	args := redis.Args{}
	args = args.Add(c.getKey(key))
	args = args.Add(members...)
	return redis.Strings(c.Do("GEOHASH", args...))
}

func toGeoResult(reply interface{}, err error, options GeoOptions) ([]*GeoResult, error) {
	values, err := redis.Values(reply, err)
	if err != nil {
		return nil, err
	}
	results := make([]*GeoResult, len(values))
	for i := range values {
		if values[i] == nil {
			continue
		}
		p, ok := values[i].([]interface{})
		if !ok {
			return nil, fmt.Errorf("redisgo: unexpected element type for interface slice, got type %T", values[i])
		}
		geoResult := &GeoResult{}
		pos := 0
		name, err := redis.String(p[pos], nil)
		if err != nil {
			return nil, err
		}
		geoResult.Name = name
		if options.WithDist {
			pos = pos + 1
			dist, err := redis.Float64(p[pos], nil)
			if err != nil {
				return nil, err
			}
			geoResult.Dist = dist
		}
		if options.WithHash {
			pos = pos + 1
			hash, err := redis.Int64(p[pos], nil)
			if err != nil {
				return nil, err
			}
			geoResult.Hash = hash
		}
		if options.WithCoord {
			pos = pos + 1
			pp, ok := p[pos].([]interface{})
			if !ok {
				return nil, fmt.Errorf("redisgo: unexpected element type for interface slice, got type %T", p[i])
			}
			if len(pp) > 0 {
				lat, err := redis.Float64(pp[0], nil)
				if err != nil {
					return nil, err
				}
				lon, err := redis.Float64(pp[1], nil)
				if err != nil {
					return nil, err
				}
				geoResult.Latitude = lat
				geoResult.Longitude = lon
			}
		}
		if err != nil {
			return nil, err
		}
		results[i] = geoResult
	}
	return results, nil
}

// getKey 将健名加上指定的前缀。
func (c *Cacher) getKey(key string) string {
	return c.prefix + key
}

// encode 序列化要保存的值
func (c *Cacher) encode(val interface{}) (interface{}, error) {
	var value interface{}
	switch v := val.(type) {
	case string, int, uint, int8, int16, int32, int64, float32, float64, bool:
		value = v
	default:
		b, err := c.marshal(v)
		if err != nil {
			return nil, err
		}
		value = string(b)
	}
	return value, nil
}

// decode 反序列化保存的struct对象
func (c *Cacher) decode(reply interface{}, err error, val interface{}) error {
	str, err := String(reply, err)
	if err != nil {
		return err
	}
	return c.unmarshal([]byte(str), val)
}

// closePool 程序进程退出时关闭连接池
func (c *Cacher) closePool() {
	ch := make(chan os.Signal, 1)
	signal.Notify(ch, os.Interrupt)
	signal.Notify(ch, syscall.SIGTERM)
	signal.Notify(ch, syscall.SIGKILL)
	go func() {
		<-ch
		c.pool.Close()
		os.Exit(0)
	}()
}

// init 注册到cache
// func init() {
// 	cache.Register("redis", &Cacher{})
// }
