package main

import (
	"flag"
	"log"
	"net/http"
	_ "net/http/pprof"
	"time"

	"github.com/costinm/istio-discovery/pkg/service"
	"github.com/costinm/istio-k8slite/pkg/kubelite"
)

// Starts and runs an K8S-to-MCP adapter.

var (
	grpcAddr = flag.String("grpcAddr", ":15092", "Address of the ADS/MCP server")

	addr = flag.String("httpAddr", ":15093", "Address of the HTTP debug server")
)

// Minimal MCP server exposing k8s and consul synthetic entries
// Currently both are returned to test the timing of the k8s-to-consul sync
// Will use KUBECONFIG or in-cluster config for k8s
func main() {
	flag.Parse()

	k8s, err := kubelite.NewClient("")
	if err != nil {
		log.Fatal("K8S ", err)
	}

	a := service.NewService(*grpcAddr)

	c := kubelite.NewK8SRegistry(k8s)

	t0 := time.Now()
	err = c.Sync()
	if err != nil {
		log.Fatal("K8S inital sync error ", err)
	}

	log.Println("Starting", time.Since(t0), c, a)
	c.Start()


	http.ListenAndServe(*addr, nil)
}

