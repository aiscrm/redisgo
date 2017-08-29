package redisgo

import (
	"testing"
	"github.com/garyburd/redigo/redis"
)

func TestRedis(t *testing.T) {
	var err error
	New("localhost:6379", "mypassword", 1)
	r := GetInstance()

	r.Do("FLUSHDB")
	reply, err := redis.Values(r.Do("keys", "*"))
	if err != nil {
		t.Error("keys * error", err)
	}
	for _, keysValue := range reply {
		r.Del(string(keysValue.([]byte)))
	}

	_, err = r.Set("t", "corel", 30)
	if err != nil {
		t.Error("set string error", err)
	}

	v, err := r.GetString("t")
	if err != nil {
		t.Error("get string error", err)
	}
	if v != "corel" {
		t.Error("get string error, not equal")
	}

	_, err = r.Set("i", 11, 30)
	if err != nil {
		t.Error("set int error", err)
	}

	i, err := r.GetInt("i")
	if err != nil {
		t.Error("get int error", err)
	}
	if i != 11 {
		t.Error("get int error, not equal")
	}

	type A struct {
		Name string `json:"name"`
		Age int `json:"age"`
	}
	s := A{
		Name: "corel 陈",
		Age: 18,
	}
	_, err = r.Set("s", s, 30)
	if err != nil {
		t.Error("set struct error", err)
	}
	s2 := A{}
	err = r.GetObject("s", &s2)
	if err != nil {
		t.Error("get struct error", err)
	}
	if s2.Name != "corel 陈" || s2.Age != 18 {
		t.Error("get struct error, not equal")
	}

	err = r.Hmset("set", s, 30)
	if err != nil {
		t.Error("hmset struct error", err)
	}
	s3 := A{}
	err = r.HgetAll("set", &s3)
	if err != nil {
		t.Error("hmget struct error", err)
	}
	if s3.Name != "corel 陈" || s3.Age != 18 {
		t.Error("hmget struct error, not equal")
	}
	s3name, err := redis.String(r.Hget("set", "Name"))
	if err != nil {
		t.Error("hget error", err)
	}
	if s3name != "corel 陈" {
		t.Error("hget error, not equal")
	}
	_, err = r.Ttl("set")
	if err != nil {
		t.Error("ttl error")
	}

	err = r.Del("ii")
	if err != nil {
		t.Error("del error", err)
	}

	ii, err := r.Incr("ii")
	if err != nil {
		t.Error("incr error", err)
	}
	if ii != 1 {
		t.Error("incr value error", ii)
	}
	ii, err = r.Incr("ii")
	if err != nil {
		t.Error("incr error", err)
	}
	if ii != 2 {
		t.Error("incr value error", ii)
	}

	r.Del("zz")
	_, err = r.Zadd("zz", 6, "a")
	if err != nil {
		t.Error("zadd error", err)
	}
	r.Zadd("zz", 10, "b")
	r.Zadd("zz", 13, "c")
	r.Zadd("zz", 15, "d")
	r.Zadd("zz", 18, "e")
	r.Zadd("zz", 21, "f")
	r.Zadd("zz", 5, "g")
	score, err := r.Zscore("zz", "c")
	if err != nil {
		t.Error("zscore error", err)
	}
	if score != 13 {
		t.Error("zscore value error", err)
	}

	zz, err := r.ZrangeByScore("zz", 5, 20, 0, 10)
	if err != nil {
		t.Error("zrangeByScore error", err)
	}
	if len(zz) != 6 {
		t.Error("zrangeByScore num error", err)
	}
	//fmt.Printf("%+v\n", zz)
	//for k,v := range zz {
	//	fmt.Println("==", k, v)
	//}
}
