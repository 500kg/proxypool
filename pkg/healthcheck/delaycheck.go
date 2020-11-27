package healthcheck

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/back20/proxypool/pkg/proxy"
	"sync"
	"time"

	"github.com/ivpusic/grpool"

	"github.com/Dreamacro/clash/adapters/outbound"
)

// SpeedResults is a map of proxy.Identifier -> delayresult
var DelayResults map[string]uint16

type delayResult struct {
	name  string
	delay uint16
}

const defaultURLTestTimeout = time.Second * 5

func CleanBadProxiesWithGrpool(proxies []proxy.Proxy) (cproxies []proxy.Proxy) {
	// Note: Grpool实现对go并发管理的封装，主要是在数据量大时减少内存占用，不会提高效率。
	pool := grpool.NewPool(500, 200)

	c := make(chan delayResult)
	defer close(c)

	pool.WaitCount(len(proxies))
	// 线程：延迟测试，测试过程通过grpool的job并发
	go func() {
		for _, p := range proxies {
			pp := p // 复制一份，否则job执行时是按当前的p测试的
			pool.JobQueue <- func() {
				defer pool.JobDone()
				delay, err := testDelay(pp)
				if err == nil {
					c <- delayResult{
						name:  pp.Identifier(),
						delay: delay,
					}
				}
			}
		}
	}()
	done := make(chan struct{}) // 用于多线程的运行结束标识
	defer close(done)

	go func() {
		pool.WaitAll()
		pool.Release()
		done <- struct{}{}
	}()

	okMap := make(map[string]struct{})
	if DelayResults == nil {
		DelayResults = make(map[string]uint16)
	}
	for { // Note: 无限循环，直到能读取到done。处理并发也算是挺有创意的写法
		select {
		case r := <-c:
			if r.delay > 0 {
				DelayResults[r.name] = r.delay
				okMap[r.name] = struct{}{}
			}
		case <-done:
			cproxies = make(proxy.ProxyList, 0, 500) // 定义返回的proxylist
			for _, p := range proxies {
				if _, ok := okMap[p.Identifier()]; ok {
					cproxies = append(cproxies, p.Clone())
				}
			}
			return
		}
	}
}

func testDelay(p proxy.Proxy) (delay uint16, err error) {
	pmap := make(map[string]interface{})
	err = json.Unmarshal([]byte(p.String()), &pmap)
	if err != nil {
		return
	}

	pmap["port"] = int(pmap["port"].(float64))
	if p.TypeName() == "vmess" {
		pmap["alterId"] = int(pmap["alterId"].(float64))
	}

	clashProxy, err := outbound.ParseProxy(pmap)
	if err != nil {
		fmt.Println(err.Error())
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), defaultURLTestTimeout)
	delay, err = clashProxy.URLTest(ctx, "http://www.gstatic.com/generate_204")
	cancel()
	return delay, err
}

func testProxyDelayToChan(p proxy.Proxy, c chan delayResult, wg *sync.WaitGroup) {
	defer wg.Done()
	delay, err := testDelay(p)
	if err == nil {
		c <- delayResult{
			name:  p.Identifier(),
			delay: delay,
		}
	}
}

func CleanBadProxies(proxies []proxy.Proxy) (cproxies []proxy.Proxy) {
	c := make(chan delayResult, 40)
	wg := &sync.WaitGroup{}
	wg.Add(len(proxies))
	for _, p := range proxies {
		go testProxyDelayToChan(p, c, wg)
	}
	go func() {
		wg.Wait()
		close(c)
	}()

	okMap := make(map[string]struct{})
	for r := range c {
		if r.delay > 0 {
			okMap[r.name] = struct{}{}
		}
	}
	cproxies = make(proxy.ProxyList, 0, 500)
	for _, p := range proxies {
		if _, ok := okMap[p.Identifier()]; ok {
			p.SetUseable(true)
			cproxies = append(cproxies, p.Clone())
		} else {
			p.SetUseable(false)
		}
	}
	return
}
