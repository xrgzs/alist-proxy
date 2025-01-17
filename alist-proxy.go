package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/alist-org/alist/v3/pkg/sign"
)

type Link struct {
	Modified time.Time `json:"modified"`
	Created  time.Time `json:"created"`
	RawURL   string    `json:"raw_url"`
}

type LinkResp struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
	Data    Link   `json:"data"`
}

var (
	port              int
	https             bool
	help              bool
	certFile, keyFile string
	address, token    string
	s                 sign.Sign
)

func init() {
	flag.IntVar(&port, "port", 5243, "the proxy port.")
	flag.BoolVar(&https, "https", false, "use https protocol.")
	flag.BoolVar(&help, "help", false, "show help")
	flag.StringVar(&certFile, "cert", "server.crt", "cert file")
	flag.StringVar(&keyFile, "key", "server.key", "key file")
	flag.StringVar(&address, "address", "", "alist address")
	flag.StringVar(&token, "token", "", "alist token")
	flag.Parse()
	s = sign.NewHMACSign([]byte(token))
}

var HttpClient = &http.Client{}

type Json map[string]interface{}

type Result struct {
	Code int    `json:"code"`
	Msg  string `json:"msg"`
}

func errorResponse(w http.ResponseWriter, code int, msg string) {
	w.Header().Set("content-type", "text/json")
	res, _ := json.Marshal(Result{Code: code, Msg: msg})
	w.WriteHeader(200)
	_, _ = w.Write(res)
}

func downHandle(w http.ResponseWriter, r *http.Request) {
	sign := r.URL.Query().Get("sign")
	filePath := r.URL.Path
	err := s.Verify(filePath, sign)
	if err != nil {
		errorResponse(w, 401, err.Error())
		return
	}
	data := Json{
		"path": filePath,
	}
	dataByte, _ := json.Marshal(data)
	req, _ := http.NewRequest("POST", fmt.Sprintf("%s/api/fs/link", address), bytes.NewBuffer(dataByte))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", token)
	res, err := HttpClient.Do(req)
	if err != nil {
		errorResponse(w, 500, err.Error())
		return
	}
	defer func() {
		_ = res.Body.Close()
	}()
	dataByte, err = io.ReadAll(res.Body)
	if err != nil {
		errorResponse(w, 500, err.Error())
		return
	}
	var resp LinkResp
	err = json.Unmarshal(dataByte, &resp)
	if err != nil {
		errorResponse(w, 500, err.Error())
		return
	}
	if resp.Code != 200 {
		errorResponse(w, resp.Code, resp.Message)
		return
	}
	if !strings.HasPrefix(resp.Data.RawURL, "http") {
		resp.Data.RawURL = "http:" + resp.Data.RawURL
	}
	fmt.Println("proxy:", resp.Data.RawURL)
	if err != nil {
		errorResponse(w, 500, err.Error())
		return
	}
	req2, _ := http.NewRequest(r.Method, resp.Data.RawURL, nil)
	res2, err := HttpClient.Do(req2)
	if err != nil {
		errorResponse(w, 500, err.Error())
		return
	}
	defer func() {
		_ = res2.Body.Close()
	}()
	res2.Header.Del("Access-Control-Allow-Origin")
	res2.Header.Del("set-cookie")
	res2.Header.Del("Cache-Control")
	res2.Header.Del("P3P")
	res2.Header.Del("X-NetworkStatistics")
	res2.Header.Del("X-SharePointHealthScore")
	res2.Header.Del("docID")
	res2.Header.Del("X-Download-Options")
	res2.Header.Del("CTag")
	res2.Header.Del("X-AspNet-Version")
	res2.Header.Del("X-DataBoundary")
	res2.Header.Del("X-1DSCollectorUrl")
	res2.Header.Del("X-AriaCollectorURL")
	res2.Header.Del("SPRequestGuid")
	res2.Header.Del("request-id")
	res2.Header.Del("MS-CV")
	res2.Header.Del("Alt-Svc")
	res2.Header.Del("Strict-Transport-Security")
	res2.Header.Del("X-FRAME-OPTIONS")
	res2.Header.Del("Content-Security-Policy")
	res2.Header.Del("X-Powered-By")
	res2.Header.Del("MicrosoftSharePointTeamServices")
	res2.Header.Del("X-MS-InvokeApp")
	res2.Header.Del("X-Cache")
	res2.Header.Del("X-MSEdge-Ref")
	for h, v := range res2.Header {
		w.Header()[h] = v
	}
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Access-Control-Allow-Methods", "GET, OPTIONS")
	w.Header().Add("Access-Control-Allow-Headers", "range")
	w.Header().Add("Last-Modified", resp.Data.Modified.Format(time.RFC1123))
	w.WriteHeader(res2.StatusCode)
	_, err = io.Copy(w, res2.Body)
	if err != nil {
		errorResponse(w, 500, err.Error())
		return
	}
}

func main() {
	if help {
		flag.Usage()
		return
	}
	addr := fmt.Sprintf(":%d", port)
	fmt.Printf("listen and serve: %s\n", addr)
	s := http.Server{
		Addr:    addr,
		Handler: http.HandlerFunc(downHandle),
	}
	if !https {
		if err := s.ListenAndServe(); err != nil {
			fmt.Printf("failed to start: %s\n", err.Error())
		}
	} else {
		if err := s.ListenAndServeTLS(certFile, keyFile); err != nil {
			fmt.Printf("failed to start: %s\n", err.Error())
		}
	}
}
