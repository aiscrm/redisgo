# 说明

封装redis常用方法，使用 `github.com/garyburd/redigo/redis` 库。

有以下特性：

- 支持连接池
- 支持退出时关闭连接池
- 支持存储struct类型，可以序列化为json存储或存储为哈希表
- 支持有序集合，可以用来做延迟队列或排行榜等用途
- 支持单实例工厂模式，并防止并发问题

## 安装

```
go get github.com/garyburd/redigo/redis
go get github.com/aiscrm/redisgo
```

## 示例：

```
New("localhost", 6379, "This is password", 0)
r := GetInstance()
r.set("keyname", "keyvalue", 30)
```

## 单元测试

完成了部分代码的单元测试