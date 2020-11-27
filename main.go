package main

import (
	"flag"
	"fmt"
	_ "net/http/pprof"
	"os"

	"github.com/back20/proxypool/api"
	"github.com/back20/proxypool/internal/app"
	"github.com/back20/proxypool/internal/cron"
	"github.com/back20/proxypool/internal/database"
	"github.com/back20/proxypool/pkg/proxy"
)

var configFilePath = ""

func main() {
	//go func() {
	//	http.ListenAndServe("0.0.0.0:6060", nil)
	//}()

	flag.StringVar(&configFilePath, "c", "", "path to config file: config.yaml")
	flag.Parse()

	if configFilePath == "" {
		configFilePath = os.Getenv("CONFIG_FILE")
	}
	if configFilePath == "" {
		configFilePath = "config.yaml"
	}
	err := app.InitConfigAndGetters(configFilePath)
	if err != nil {
		panic(err)
	}

	database.InitTables()
	// init GeoIp db reader and map between emoji's and countries
	// return: struct geoIp (dbreader, emojimap)
	err = proxy.InitGeoIpDB()
	if err != nil {
		os.Exit(1)
	}
	fmt.Println("Do the first crawl...")
	go app.CrawlGo() // 抓取主程序
	go cron.Cron()   // 定时运行
	api.Run()        // Web Serve
}
