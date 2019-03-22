package kubelite

import (
	"context"
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"sync"
	"time"

	"github.com/ericchiang/k8s"
	corev1 "github.com/ericchiang/k8s/apis/core/v1"
	"github.com/ghodss/yaml"
)

// loadClient parses a kubeconfig from a file or KUBECONFIG
// and returns a Kubernetes client. It does not support extensions or
// client auth providers.
func NewClient(kubeconfigPath string) (*k8s.Client, error) {
	if kubeconfigPath == "" {
		// Attempt in-cluster first
		host := os.Getenv("KUBERNETES_SERVICE_HOST")
		if len(host) != 0 {
			return k8s.NewInClusterClient()
		}

		kubeconfigPath = os.Getenv("KUBECONFIG")
		if kubeconfigPath == "" {
			return k8s.NewClient(&k8s.Config{
				CurrentContext: "default",
				Contexts: []k8s.NamedContext{
					{
						Context: k8s.Context{
							Cluster:  "default",
							AuthInfo: "default", // 'user'
						},
						Name: "default",
					},
				},
				AuthInfos: []k8s.NamedAuthInfo{
					{
						Name: "default",
					},
				},
				Clusters: []k8s.NamedCluster{
					{
						Cluster: k8s.Cluster{Server: "http://localhost:8080"},
						Name:    "default",
					},
				},
			})
		}
	}

	data, err := ioutil.ReadFile(kubeconfigPath)
	if err != nil {
		return nil, fmt.Errorf("read kubeconfig: %v", err)
	}

	// Unmarshal YAML into a Kubernetes config object.
	var config k8s.Config
	if err := yaml.Unmarshal(data, &config); err != nil {
		return nil, fmt.Errorf("unmarshal kubeconfig: %v", err)
	}
	return k8s.NewClient(&config)
}


type K8SRegistry struct {
	client *k8s.Client

	mutex         sync.RWMutex

	// Pod By IP is needed to revert the IP of endpoint to pod
	// The IP in endpoints is the only way to determine ready state.
	podLabelsByIp map[string]podInfo

	// We're looking at node metadata.
	nodes map[string]map[string]string
}

type nodeInfo struct {
	region string
	zone   string
}

type podInfo struct {
	node        string
	labels      map[string]string
	annotations map[string]string
}


func NewK8SRegistry(client *k8s.Client) *K8SRegistry {
	return &K8SRegistry{
		client: client,
		nodes:  map[string]map[string]string{},
		podLabelsByIp:  map[string]podInfo{},

	}
}

const (
	nodeLabelZone   = "failure-domain.beta.kubernetes.io/zone"
	nodeLabelRegion = "failure-domain.beta.kubernetes.io/region"
)


func (kr *K8SRegistry) onNode(ev string, resource k8s.Resource) {
	if node, ok := resource.(*corev1.Node); ok {
		kr.mutex.Lock()
		kr.nodes[*node.Metadata.Name] = node.Metadata.Labels
		kr.mutex.Unlock()
		log.Printf("node %s ns=%s name=%s l=%v\n", ev, *node.Metadata.Namespace, *node.Metadata.Name, node.Metadata.Labels)
	}
}

func (kr *K8SRegistry) onPod(ev string, resource k8s.Resource) {
	if pod, ok := resource.(*corev1.Pod); ok {
		if pod.Status.PodIP == nil {
			return
		}
		kr.mutex.Lock()
		kr.podLabelsByIp[*pod.Status.PodIP] = podInfo {
			node: *pod.Status.HostIP,
			labels: pod.Metadata.Labels,
			annotations: pod.Metadata.Annotations,
		}
		kr.mutex.Unlock()
		log.Println("pod ", ev, *pod.Metadata.Namespace, *pod.Metadata.Name,
			pod.Status.Phase, pod.Status.Conditions,
			pod.Metadata.Labels)
	}
}

func (kr *K8SRegistry) onService(ev string, resource k8s.Resource) {
	if svc, ok := resource.(*corev1.Service); ok {
		kr.mutex.Lock()

		kr.mutex.Unlock()
		log.Printf("svc %s ns=%s name=%s l=%v\n", ev, *svc.Metadata.Namespace, *svc.Metadata.Name, svc.Metadata.Labels)
	}
}

func (kr *K8SRegistry) onEP(ev string, resource k8s.Resource) {
	if ep, ok := resource.(*corev1.Endpoints); ok {
		kr.mutex.Lock()

		kr.mutex.Unlock()
		log.Printf("ep %s ns=%s name=%s l=%v\n", ev, *ep.Metadata.Namespace, *ep.Metadata.Name, ep.Metadata.Labels)
	}
}

func (kr *K8SRegistry) Sync() error {

	var nodes corev1.NodeList
	err := kr.client.List(context.Background(), "", &nodes)
	if err != nil {
		return err
	}

	for _, node := range nodes.Items {
		kr.onNode("sync", node)
	}

	// Collect pod labels, by IP
	var pods corev1.PodList
	err = kr.client.List(context.Background(), "", &pods)
	if err != nil {
		return err
	}

	for _, pod := range pods.Items {
		kr.onPod("sync", pod)
	}

	var svcl corev1.ServiceList
	err = kr.client.List(context.Background(), "", &svcl)
	if err != nil {
		return err
	}
	for _, svc := range svcl.Items {
		kr.onService("sync", svc)
	}

	var epl corev1.EndpointsList
	err = kr.client.List(context.Background(), "", &epl)
	if err != nil {
		return err
	}
	for _, ep := range epl.Items {
		kr.onEP("sync", ep)
	}

	// TODO
	//kr.listPaged(func() k8s.ResourceList{ return &corev1.EndpointsList{}}, "Endpoints", nil)

	return nil
}

func (kr *K8SRegistry) listPaged(nodef func() k8s.ResourceList, n string,
	f func(ev string, resource k8s.Resource)) {

		total := 0

		node := nodef()
		err := kr.client.List(context.Background(), "", node,
			k8s.QueryParam("limit", "50")) // 500
		total += node.GetMetadata().Size()

		if err != nil {
			log.Println("Error in listPaged" , err)
			return
		}

		cnt := *node.GetMetadata().Continue
		for {
			node = nodef()
			err := kr.client.List(context.Background(), "", node,
				k8s.QueryParam("limit", "10"), // 500
				k8s.QueryParam("continue", cnt))
			if err != nil {
				log.Println("Error in listPaged" , err)
				return
			}
			total += node.GetMetadata().Size()
			cnt = *node.GetMetadata().Continue
			if cnt == "" {
				log.Println(n, total)
				return
			}
		}
}

func (kr *K8SRegistry) watch(k8sRes k8s.Resource, n string, f func(ev string, resource k8s.Resource)) {
	var wn *k8s.Watcher
	var err error
	// last resource version, used for watch
	resourceVersion := "0"

	for {
		if wn == nil {
			wn, err = kr.client.Watch(context.Background(), "", k8sRes,
				k8s.QueryParam("resourceVersion", resourceVersion),
				k8s.QueryParam("timeoutSeconds", "300"),
			)
			if err != nil {
				log.Println("Watch restart error ", n, err)
				// TODO: exp backoff
				time.Sleep(5 * time.Second)
				continue
			}
		}

		evtype, err := wn.Next(k8sRes)
		if err != nil {
			wn = nil
			log.Println("Watch error ", n, err)
			// TODO: exp backoff
			time.Sleep(5 * time.Second)
			continue
		}
		resourceVersion = *k8sRes.GetMetadata().ResourceVersion

		if f != nil {
			f(evtype, k8sRes)
		}
	}
}

func (kr *K8SRegistry) Start() error {
	go kr.watch(&corev1.Node{}, "Node", kr.onNode)
	go kr.watch(&corev1.Pod{}, "Pod", kr.onPod)
	go kr.watch(&corev1.Service{}, "Service", kr.onService)
	go kr.watch(&corev1.Endpoints{}, "Endpoints", kr.onEP)

	return nil
}

// TODO: pod changes -> WorkloadUpdate
// Paging
// Watch with Timeout
