# redisgo
基于[github.com/gomodule/redigo](https://github.com/gomodule/redigo)，对常用的redis用法进行了封装，让你更方便的使用。

## 特性

- 支持连接池
- 支持退出时关闭连接池
- 支持各种类型数据的存储和读取，可以直接获取指定类型的存储值
- 支持有序集合，可以用来做延迟队列或排行榜等用途
- 支持redis订阅/发布，在redis故障或网络异常时，自动重新订阅
- 支持列表，包含阻塞式和非阻塞式读取

## 安装

```sh
go get -u github.com/aiscrm/cache
```

## 快速开始

```go
package main

import (
	"fmt"

	"github.com/aiscrm/redisgo"
)

type User struct {
	Name string
	Age  int
}

func main() {
	c, err := redisgo.New(
		redisgo.Options{
			Addr:     "127.0.0.1:6379",
			Password: "",
			Prefix:   "aiscrm_",
		})
	if err != nil {
		panic(err)
	}
	c.Set("name", "corel", 30)
	c.Set("age", "23", 30)
	name, err := c.GetString("name")
	age, err := c.GetInt("age")
	fmt.Printf("Name=%s, age=%d", name, age)
	user := &User{
		Name: "corel",
		Age:  23,
	}
	c.Set("user", user, 30)
	user2 := &User{}
	c.GetObject("user", user2)
	fmt.Printf("user: %+v", user2)
}
```

## 特别鸣谢

- redis缓存部分基于 `github.com/gomodule/redigo` 进行封装
- redis命令参考 http://redisdoc.com/ 和 http://www.runoob.com/redis/