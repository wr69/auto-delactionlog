package main

import (
	"bytes"
	"io"
	"io/ioutil"
	"log"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/spf13/viper"
	"github.com/tidwall/gjson"
)

var Error_Log *log.Logger
var Error_LogFile *os.File

func ResetErrorLogOutput() {
	logname := "log/" + "log_" + time.Now().Format("2006-01-02") + ".txt"
	err := os.MkdirAll(filepath.Dir(logname), 0755) //
	if err != nil {
		log.Println("无法创建目录：", err)
	}
	logFile, err := os.OpenFile(logname, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		log.Println("无法打开新的日志文件：", err)
	} else {
		if Error_LogFile != nil {
			defer Error_LogFile.Close()
		}
		Error_Log = log.New(logFile, "", log.Ldate|log.Ltime|log.Lshortfile)
		Error_Log.SetOutput(logFile)
		Error_LogFile = logFile
	}
}

func ErrorLog(v ...any) {
	if Error_Log == nil {
		ResetErrorLogOutput()
	}
	Error_Log.Println(v...)
}

func returnErr(v ...any) {
	log.Println(v...)
	os.Exit(1)
}

var API_URL string
var API_AUTH string
var Del_Ids = make(map[string][]string)
var Del_Total = 1000
var Del_Map = make(map[string]string)

func main() {
	log.Println("本次任务执行", "准备")
	var envConfig *viper.Viper

	getConfig := viper.New()

	getConfig.SetConfigName("auto-delactionlog")
	getConfig.SetConfigType("yaml")             // 如果配置文件的名称中没有扩展名，则需要配置此项
	getConfig.AddConfigPath(".")                // 还可以在工作目录中查找配置
	getConfig.AddConfigPath("../updata/config") // 还可以在工作目录中查找配置

	err := getConfig.ReadInConfig() // 查找并读取配置文件
	if err != nil {                 // 处理读取配置文件的错误
		//returnErr("读取配置文件的错误: ", err)
		envConfig = viper.New()
		envConfig.AutomaticEnv()
	} else {
		envConfig = getConfig.Sub("env") //读取env的配置
	}

	link := envConfig.GetString("LINK")
	if link == "" {
		returnErr("link配置不存在")
	}

	gh_token := envConfig.GetString("GH_TOKEN")
	if gh_token == "" {
		returnErr("gh_token配置不存在")
	}

	log.Println("本次任务执行", "启动")

	API_URL = "https://api.github.com/repos/"
	API_AUTH = "token " + gh_token

	var groups []string
	linkS := strings.Split(link, "\n")
	for _, v := range linkS {
		if v != "" {
			v = strings.TrimSpace(v)
			porp := strings.Split(v, "|")

			Del_Map[porp[1]] = porp[0]
			groups = append(groups, porp[1])
		}
	}

	for i := 1; i < 10; i++ {
		log.Println(i, "轮", Del_Total)
		if i > 6 {
			break
		}
		if Del_Total > 100 {
			log.Println(i, "轮 start")
			startRun(groups)
		}
	}

	log.Println("Del_Total", Del_Total)

	log.Println("本次任务执行", "结束")

}
func startRun(groups []string) {
	Del_Ids = make(map[string][]string)
	for _, group := range groups {
		if group != "" {
			addID(group)
		}
	}

	for del_key, del_repo := range Del_Ids {

		if Del_Total < 1 {
			break
		}

		for _, del_id := range del_repo {

			if Del_Total < 1 {
				break
			}
			delID(del_key, del_id)
		}
	}

}

func delID(space string, run_id string) {
	var scode int
	var rbody string
	scode, rbody = delWorkflowsRunsLogsById(space, run_id)
	if scode > 300 {
		ErrorLog(Del_Map[space], run_id, scode, rbody)
	}
	Del_Total--
	scode, rbody = delWorkflowsRunsById(space, run_id)
	if scode > 300 {
		ErrorLog(Del_Map[space], run_id, scode, rbody)
	}
	Del_Total--
}

func addID(repo string) {

	scode, runlist := getWorkflowsRunsList(repo, "")
	if scode > 300 {
		ErrorLog(repo, runlist)
		return
	}

	result := gjson.Parse(runlist)
	//total := result.Get("total_count").Int()
	runs := result.Get("workflow_runs")

	Del_Ids[repo] = []string{}
	for _, run := range runs.Array() {
		runID := run.Get("id").String()
		if runID != "" {
			runST := run.Get("conclusion").String()
			if runST == "success" {
				Del_Ids[repo] = append(Del_Ids[repo], runID)
			}
		}
	}

}

func delApi(apiurl string) (int, string) {
	StatusCode, resBody := reqApi("delete", apiurl, http.Header{}, "")
	return StatusCode, resBody
}

func getApi(apiurl string) (int, string) {
	StatusCode, resBody := reqApi("get", apiurl, http.Header{}, "")
	return StatusCode, resBody
}

func reqApi(method string, apiurl string, headers http.Header, data string) (int, string) {
	var req *http.Request
	var err error
	var dataSend io.Reader

	transport := &http.Transport{} // 创建自定义的 Transport

	proxy_url := ""
	//proxy_url = "http://127.0.0.1:8888"

	if proxy_url != "" {
		proxyURL, err := url.Parse(proxy_url) // 代理设置
		if err != nil {
			log.Println(apiurl, "代理出问题", err)
		} else {
			transport.Proxy = http.ProxyURL(proxyURL)
		}
	}

	// 创建客户端，并使用自定义的 Transport
	client := &http.Client{
		Timeout:   15 * time.Second, // 设置超时时间为15秒
		Transport: transport,        //
	}

	headers.Set("User-Agent", "Mozilla / 5.0 (Windows NT 10.0; Win64; x64) AppleWebKit / 537.36 (KHTML, like Gecko) Chrome / 114.0.0.0 Safari / 537.36 Edg / 114.0.1823.37")
	headers.Set("Accept", "application/vnd.github.v3+json")
	headers.Set("Authorization", API_AUTH)

	if strings.HasPrefix(data, "{") {
		dataSend = bytes.NewBuffer([]byte(data))
	} else {
		dataSend = strings.NewReader(data)
	}

	if method == "post" {
		req, err = http.NewRequest("POST", apiurl, dataSend)
	} else if method == "put" {
		req, err = http.NewRequest("PUT", apiurl, dataSend)
	} else if method == "delete" {
		req, err = http.NewRequest("DELETE", apiurl, nil)
	} else {
		req, err = http.NewRequest("GET", apiurl, nil)
	}
	if err != nil {
		return 9999, "http.NewRequest " + err.Error()
	}
	req.Header = headers

	resp, err := client.Do(req)
	if err != nil {
		return 9999, "client.Do " + err.Error()
	}
	defer resp.Body.Close()

	resBody, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return resp.StatusCode, "io resp.Body " + err.Error()
	}

	return resp.StatusCode, string(resBody)

}

// 列出存储库的所有工作流运行。您可以使用参数来缩小结果列表的范围。有关使用参数的详细信息，请参见参数。任何对存储库具有读访问权限的人都可以使用此端点。如果存储库是私有的，则必须在存储库范围内使用访问令牌。GitHub Apps必须具有actions：read权限才能使用此端点。
func getWorkflowsRunsList(space string, suffix string) (int, string) {
	apiurl := API_URL + "" + space + "/actions/runs" //+ "?created=<2023-12-31"
	if suffix != "" {
		apiurl = apiurl + "?" + suffix
	}
	//查询参数
	//actor 返回某人的工作流运行。使用创建与检查套件或工作流运行关联的推送的用户的登录名。

	statusCode, resBody := getApi(apiurl)

	return statusCode, resBody
}

// 删除工作流运行的所有日志。
func delWorkflowsRunsLogsById(space string, run_id string) (int, string) {
	apiurl := API_URL + "" + space + "/actions/runs" + "/" + run_id + "/" + "logs"
	statusCode, resBody := delApi(apiurl)
	return statusCode, resBody
}

// 删除特定工作流运行。
func delWorkflowsRunsById(space string, run_id string) (int, string) {
	apiurl := API_URL + "" + space + "/actions/runs" + "/" + run_id
	statusCode, resBody := delApi(apiurl)
	return statusCode, resBody
}
