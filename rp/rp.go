package rp

import (
	"crypto/tls"
	"github.com/dgraph-io/badger/v3"
	"net/http"
	"net/http/httputil"
	"net/url"
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

	db, err := badger.Open(badger.DefaultOptions("/rp/badger"))
	if err != nil {
		reverseProxyLog.Error(err, "Failed to open reverse proxy rules database")
		return
	}
	defer db.Close()

	err = db.View(func(txn *badger.Txn) error {
		item, err := txn.Get([]byte(request.Host))
		if err != nil {
			reverseProxyLog.Info("Reverse proxy rule is missing for this host")
			return nil
		}
		// handle target url
		var hostName []byte
		err = item.Value(func(val []byte) error {
			hostName = append([]byte{}, val...)
			return nil
		})

		targetUrl := "http://" + string(hostName) + ".simpleingresssap-system.svc.cluster.local"
		parsedTargetUrl, err := url.Parse(targetUrl)
		if err != nil {
			reverseProxyLog.Error(err, "Failed to parse serviceIP to URL")
			return nil
		}

		reverseProxyLog.Info("Upstream Request")
		proxy := httputil.NewSingleHostReverseProxy(parsedTargetUrl)

		request.URL.Host = parsedTargetUrl.Host
		request.URL.Scheme = parsedTargetUrl.Scheme
		request.Header.Set("X-Forwarded-Host", request.Header.Get("Host"))
		request.Host = parsedTargetUrl.Host

		proxy.ServeHTTP(writer, request)
		return nil
	})
}
