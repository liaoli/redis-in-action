package model

import (
	"crypto"
	"encoding/hex"
	"encoding/json"
	"github.com/go-redis/redis/v7"
	"log"
	"net/url"
	"redisInAction/Chapter02/common"
	"redisInAction/Chapter02/repository"
	"redisInAction/utils"
	"strings"
	"sync/atomic"
	"time"
)

type Client struct {
	Conn *redis.Client
}

func NewClient(conn *redis.Client) *Client {
	return &Client{Conn: conn}
}

func (r *Client) CheckToken(token string) string {
	//尝试获取并返回令牌对应的用户
	return r.Conn.HGet("login:", token).Val()
}

func (r *Client) UpdateToken(token, user, item string) {
	timestamp := time.Now().Unix()                                             //获取当前时间戳
	r.Conn.HSet("login:", token, user)                                         //维持令牌和已登录用户之间的映射
	r.Conn.ZAdd("recent:", &redis.Z{Score: float64(timestamp), Member: token}) //记录令牌最后一次出现的时间
	if item != "" {
		r.Conn.ZAdd("viewed:"+token, &redis.Z{Score: float64(timestamp), Member: item}) //记录用户浏览过的商品
		r.Conn.ZRemRangeByRank("viewed:"+token, 0, -26)                                 //移除旧的记录，只保留用户最近浏览过的25个商品
	}
}

func (r *Client) CleanSessions() {
	for !common.QUIT {
		size := r.Conn.ZCard("recent:").Val() //找出目前已有令牌的数量
		if size <= common.LIMIT {             //令牌数量未超过限制，休眠1秒后再重新检查
			time.Sleep(1 * time.Second)
			continue
		}

		endIndex := utils.Min(size-common.LIMIT, 100) //获取要移除的令牌ID
		tokens := r.Conn.ZRange("recent:", 0, endIndex-1).Val()

		var sessionKey []string
		for _, token := range tokens {
			sessionKey = append(sessionKey, token)
		}

		//移除旧的那些令牌
		r.Conn.Del(sessionKey...)
		r.Conn.HDel("login:", tokens...)
		r.Conn.ZRem("recent:", tokens)
	}
	defer atomic.AddInt32(&common.FLAG, -1)
}

func (r *Client) AddToCart(session, item string, count int) {
	switch {
	case count <= 0:
		r.Conn.HDel("cart:"+session, item) //从购物车移除指定的商品
	default:
		r.Conn.HSet("cart:"+session, item, count) //将指定的商品添加到购物车
	}
}

func (r *Client) CleanFullSession() {
	for !common.QUIT {
		size := r.Conn.ZCard("recent:").Val()
		if size <= common.LIMIT {
			time.Sleep(1 * time.Second)
			continue
		}

		endIndex := utils.Min(size-common.LIMIT, 100)
		sessions := r.Conn.ZRange("recent:", 0, endIndex-1).Val()

		var sessionKeys []string
		for _, sess := range sessions {
			sessionKeys = append(sessionKeys, "viewed:"+sess) //浏览过的商品
			sessionKeys = append(sessionKeys, "cart:"+sess)   //购物车里面的商品
		}

		r.Conn.Del(sessionKeys...)         //删除 购物车 和 浏览记录
		r.Conn.HDel("login:", sessions...) //删除登录 记录
		r.Conn.ZRem("recent:", sessions)   //删除令牌最后出现的时间
	}
	defer atomic.AddInt32(&common.FLAG, -1)
}

func (r *Client) CacheRequest(request string, callback func(string) string) string {
	if r.CanCache(request) {
		return callback(request) //对于不能被缓存的请求，直接调用回调函数
	}

	pageKey := "cache:" + hashRequest(request) //将请求转换成一个简单的字符串键，方便之后进行查找
	content := r.Conn.Get(pageKey).Val()       //尝试查找被缓存的页面

	if content == "" {
		content = callback(request)                   //如果缓存里面没有的话，那么生成页面。
		r.Conn.Set(pageKey, content, 300*time.Second) //将生成的页面放到缓存里面
	}
	return content
}

func (r *Client) ScheduleRowCache(rowId string, delay int64) {
	r.Conn.ZAdd("delay:", &redis.Z{Member: rowId, Score: float64(delay)})                //相设置数据行的延迟值
	r.Conn.ZAdd("schedule:", &redis.Z{Member: rowId, Score: float64(time.Now().Unix())}) //立即对需要缓存的数据进行调度
}

func (r *Client) CacheRows() {
	for !common.QUIT {
		next := r.Conn.ZRangeWithScores("schedule:", 0, 0).Val()
		//尝试获取下一个需要别缓存的数据行
		// 以及该行的调度时间戳，命令会返回一个包含零个或一个元组的列表。
		now := time.Now().Unix()
		if len(next) == 0 || next[0].Score > float64(now) {
			//暂时没有行需要被缓存，休眠50毫秒后重试
			time.Sleep(50 * time.Millisecond)
			continue
		}

		rowId := next[0].Member.(string)
		delay := r.Conn.ZScore("delay:", rowId).Val() //提前获取下一次调度的延迟时间
		if delay <= 0 {                               //不必再缓存这个行，将它从缓存中移除
			r.Conn.ZRem("delay:", rowId)
			r.Conn.ZRem("schedule:", rowId)
			r.Conn.Del("inv:" + rowId)
			continue
		}

		row := repository.Get(rowId)                                                   //读取数据行
		r.Conn.ZAdd("schedule:", &redis.Z{Member: rowId, Score: float64(now) + delay}) //更新调度时间
		jsonRow, err := json.Marshal(row)
		if err != nil {
			log.Fatalf("marshal json failed, data is: %v, err is: %v\n", row, err)
		}
		r.Conn.Set("inv:"+rowId, jsonRow, 0) //并设置缓存值
	}
	defer atomic.AddInt32(&common.FLAG, -1)
}

func (r *Client) UpdateTokenModified(token, user string, item string) {
	timestamp := time.Now().Unix()
	r.Conn.HSet("login:", token, user)
	r.Conn.ZAdd("recent:", &redis.Z{Score: float64(timestamp), Member: token})
	if item != "" {
		r.Conn.ZAdd("viewed:"+token, &redis.Z{Score: float64(timestamp), Member: item})
		r.Conn.ZRemRangeByRank("viewed:"+token, 0, -26)
		r.Conn.ZIncrBy("viewed:", -1, item) // 记录所有商品的浏览次数
	}
}

func (r *Client) RescaleViewed() {
	for !common.QUIT {
		//删除所有排在20000名之后的商品
		r.Conn.ZRemRangeByRank("viewed:", 20000, -1)

		//将浏览次数变成原来的一半
		r.Conn.ZInterStore("viewed:", &redis.ZStore{Weights: []float64{0.5}, Keys: []string{"viewed:"}})
		time.Sleep(300 * time.Second) // 5分钟之后再执行这个操作
	}
}

func (r *Client) CanCache(request string) bool {
	itemId := extractItemId(request)        //尝试从页面里面取出商品ID
	if itemId == "" || isDynamic(request) { //检查这个页面能否被缓存一集这个页面是否为商品页面
		return false
	}
	rank := r.Conn.ZRank("viewed:", itemId).Val() //取得商品的浏览次数排名
	return rank != 0 && rank < 10000              //根据商品的浏览次数排名来判断是否需要缓存这个页面
}

func (r *Client) Reset() {
	r.Conn.FlushDB()

	common.QUIT = false
	common.LIMIT = 10000000
	common.FLAG = 1
}

func extractItemId(request string) string {
	parsed, _ := url.Parse(request)
	queryValue, _ := url.ParseQuery(parsed.RawQuery)
	query := queryValue.Get("item")
	return query
}

func isDynamic(request string) bool {
	parsed, _ := url.Parse(request)
	queryValue, _ := url.ParseQuery(parsed.RawQuery)
	for _, v := range queryValue {
		for _, j := range v {
			if strings.Contains(j, "_") {
				return false
			}
		}
	}
	return true
}

func hashRequest(request string) string {
	hash := crypto.MD5.New()
	hash.Write([]byte(request))
	res := hash.Sum(nil)
	return hex.EncodeToString(res)
}
