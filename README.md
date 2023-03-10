# Redis-In-Action
Rewrite the code from the amazing book Redis-In-Action using `golang` and `go-redis/redis`, using `Go Modules` to manager the dependency.

## 第一章
- redis支持的5种数据类型
- 5种数据类型的实际应用
- 评分系统实现

## 第二章
- cookie缓存
- 购物车实现
- 网页缓存
- 数据行缓存
- 网页分析

## 第三章 redis命令
- string
- list
- set
- hash
- zSet
- 消息队列
- 其他命令
  + 排序
  + 基本的redis事务
  + 键的过期时间  

### Running：

#### Using Docker: 

> Tips for Chinese users: check the Dockerfile before running,  uncomment line 4, this will speed you building process.

- check the config messages in the config/config.go file first, you may need to set your config for redis
- run `docker-compose up -d` in the directory.
- use `docker exec -it redis-in-action-golang go test ./Chapter0*/redisConn_test.go -v` to run the test, use number 1 through 8 to replace the `*` depending on the Chapter's examples you want to run.

#### Using command-line/terminal: 

Open a command-line/terminal in the `golang` directory and execute follow command:

- `go mod download` to download the dependency, then:

- `go test ./Chapter0*/redisConn_test.go -v`, use number 1 through 8 to replace the `*`  depending on the Chapter's examples you want to run.


### Todo：

-[ ] Chapter04：Lack the parts before 4.4

-[ ] Chapter06：Achieve the func DailyCountryAggregate

-[ ] Chapter09 - Chapter11

