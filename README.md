# 说明

封装redis常用方法，使用 `github.com/garyburd/redigo/redis` 库。

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