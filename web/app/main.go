package main

import (
	"encoding/json"
	"fmt"
	"github.com/oschwald/maxminddb-golang"
	"io/ioutil"
	"log"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

/**
 * ip database file
 * reference: https://db-ip.com/db/format/ip-to-country-lite/mmdb.html
 */
type Record struct {
	Country struct {
		ISOCode           string            `maxminddb:"iso_code"`
		GeonameId         uint32            `maxminddb:"geoname_id"`
		IsInEuropeanUnion bool              `maxminddb:"is_in_european_union"`
		Names             map[string]string `maxminddb:"names"`
	} `maxminddb:"country"`
	Continent struct {
		Code      string `maxminddb:"code"`
		GeonameId uint32 `maxminddb:"geoname_id"`
	} `maxminddb:"continent"`
} // Or any appropriate struct

/**
 * response data structure
 */
type Ret struct {
	StatusCode int8          `json:"statusCode"`
	Data       []CountryInfo `json:"data"`
	Msg        string        `json:"msg"`
}

/**
 * the ip with location info
 */
type CountryInfo struct {
	Ip      string `json:"ip"`
	ISOCode string `json:"isoCode"`
	Name    string `json:"country"`
}

func searchIpInfo(db *maxminddb.Reader, ip string, c chan CountryInfo) {
	var record Record
	_ip := net.ParseIP(ip)
	err := db.Lookup(_ip, &record)
	if err != nil {
		log.Fatal(err)
		os.Exit(1)
	}

	_lang, _isLangExist := os.LookupEnv("INFO_LANG")
	if !_isLangExist {
		_lang = "en"
	}

	c <- CountryInfo{
		Ip:      ip,
		Name:    record.Country.Names[_lang],
		ISOCode: record.Country.ISOCode,
	}
}

/**
 * multiple ip is separated by comma
 * get or post method is supported.
 */
func searchRouter(w http.ResponseWriter, r *http.Request) {
	var ipStrList []string
	switch r.Method {
	case "GET":
		query := r.URL.Query()
		if query == nil || query.Get("ip") == "" {
			log.Println("query or ip could not be empty")

			ret, err := json.Marshal(Ret{
				StatusCode: -3,
			})

			if err != nil {
				log.Fatal(err)
				os.Exit(-1)
			}
			fmt.Fprint(w, string(ret))
			return
		}
		ipStrList = strings.Split(r.URL.Query().Get("ip"), ",")
		break
	case "POST":
		var ips map[string][]string
		body, _ := ioutil.ReadAll(r.Body)
		json.Unmarshal(body, &ips)
		ipStrList = ips["ip"]
	}

	pwd, err := os.Getwd()
	if err != nil {
		log.Fatal(err)
		os.Exit(1)
	}
	// https://db-ip.com/db/download/ip-to-country-lite
	_path := filepath.Join(pwd, "/db/dbip-country-lite-2019-12.mmdb")
	db, err := maxminddb.Open(_path)

	if err != nil {
		log.Fatal(err)
		os.Exit(1)
	}
	defer db.Close()

	defer elapsed("check ip(s)")()
	ipLen := len(ipStrList)
	chLen := 0
	log.Printf("current ip len:%d", ipLen)
	ch := make(chan CountryInfo)
	for _, ip := range ipStrList {
		if ip == "" {
			continue
		}
		// 检验ip有效性
		valid, err := regexp.MatchString(`^(([0-9]|[1-9][0-9]|1[0-9]{2}|2[0-4][0-9]|25[0-5])\.){3}([0-9]|[1-9][0-9]|1[0-9]{2}|2[0-4][0-9]|25[0-5])$`, ip)
		if err != nil {
			log.Printf("ip regexp error:%v - %s", err, ip)
			continue
		}

		if !valid {
			log.Printf("ip invalid:%s", ip)
			continue
		}

		go searchIpInfo(db, ip, ch)
		chLen++
	}

	data := make([]CountryInfo, 0)
	for i := 0; i < chLen; i++ {
		data = append(data, <-ch)
	}

	log.Printf("finished ip check len:%d", chLen)

	ret, err := json.Marshal(Ret{
		StatusCode: 1,
		Data:       data,
	})

	w.Header().Set("Content-Type", "application/json")
	fmt.Fprint(w, string(ret))
}

func elapsed(op string) func() {
	start := time.Now()
	return func() {
		log.Printf("%s spent time:%v\n", op, time.Since(start))
	}
}

func main() {
	http.HandleFunc("/search", searchRouter)
	err := http.ListenAndServe(":9999", nil)

	if err != nil {
		log.Fatal("Server start error:", err)
		os.Exit(1)
	}

	log.Println("Server start successfully")
}
