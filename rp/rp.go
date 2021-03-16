package rp

import (
	"crypto/tls"
	"fmt"
	"io/ioutil"
	"k8s.io/apimachinery/pkg/util/json"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	ctrl "sigs.k8s.io/controller-runtime"
)

var (
	reverseProxyLog = ctrl.Log.WithName("ReverseProxy")
)

type Rule struct {
	ServiceIP   string `json:"serviceIP"`
	ServiceName string `json:"serviceName"`
}
type ReverseProxyRules struct {
	ActiveRule []Rule `json:"rules"`
}

func ProxyHandler(writer http.ResponseWriter, request *http.Request) {
	reverseProxyLog.Info("Handling Request")
	http.DefaultTransport.(*http.Transport).TLSClientConfig = &tls.Config{InsecureSkipVerify: true}
	// Read rules
	jsonFile, err := os.Open("ProxyRules.json")
	if err != nil {
		fmt.Println(err)
		return
	}
	defer jsonFile.Close()

	byteValue, _ := ioutil.ReadAll(jsonFile)
	var rpRules ReverseProxyRules
	json.Unmarshal(byteValue, &rpRules)

	for _, rule := range rpRules.ActiveRule {
		targetUrl := "http://" + rule.ServiceName + ".simpleingresssap-system.svc.cluster.local"
		parsedTargetUrl, err := url.Parse(targetUrl)
		if err != nil {
			reverseProxyLog.Error(err, "Failed to parse serviceIP to URL")
			return
		}
		if rule.ServiceIP == request.Host {
			reverseProxyLog.Info("Upstream Request")
			proxy := httputil.NewSingleHostReverseProxy(parsedTargetUrl)

			request.URL.Host = parsedTargetUrl.Host
			request.URL.Scheme = parsedTargetUrl.Scheme
			request.Header.Set("X-Forwarded-Host", request.Header.Get("Host"))
			request.Host = parsedTargetUrl.Host

			proxy.ServeHTTP(writer, request)
		}
	}
}
