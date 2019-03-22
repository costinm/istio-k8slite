package main

import (
	"context"
	"flag"
	"fmt"
	"github.com/costinm/istio-k8slite/pkg/kubelite"
	"github.com/ericchiang/k8s"
	"github.com/golang/protobuf/proto"
	"log"
	"strconv"
	"strings"
	"time"

	corev1 "github.com/ericchiang/k8s/apis/core/v1"
	metav1 "github.com/ericchiang/k8s/apis/meta/v1"
)

// Create a large number of namespaces and services. Uses the light client.

var (
	nsCnt          = flag.Int("ns", 3000, "Test namespaces. Pattern loadns_xxx")
	svcPerNs       = flag.Int("svcPerNs", 1, "Services in each namespace")
	localApiserver = flag.Bool("local", true, "Use local apiserver")

)

const (
	nsPrefix = "lns-"
)

// Will use in-cluster or KUBECONFIG to connect to K8S.
func main() {
	flag.Parse()

	k8s, err := kubelite.NewClient("")
	if err != nil {
		log.Fatal("K8S ", err)
	}
	err = ensureNamespaces(k8s, *nsCnt, nsPrefix)
	if err != nil {
		log.Println("Namespaces ", err)
	}

	err = ensureServices(k8s, *nsCnt, *svcPerNs)
	if err != nil {
		log.Println("Namespaces ", err)
	}

}

// On localapi server - there is no controller, we need to create SA.
// Test apiserver creates ClusterIP in 10.99.x.x range
//
// On real k8s - need to wait for them to show up
func ensureServices(k8sc *k8s.Client, ns int, svc int) error {
	for i := 0; i < ns; i++ {
		for j := 0; j < svc; j++ {
			err := k8sc.Create(context.Background(), &corev1.Service{
				Metadata: &metav1.ObjectMeta{
					Name:      proto.String(fmt.Sprintf("svc-%d-%d", i, j)),
					Namespace: proto.String(fmt.Sprintf("%s%d", nsPrefix, i)),
				},
				Spec: &corev1.ServiceSpec{
					Ports: []*corev1.ServicePort{
						{
							Port:     proto.Int32(80),
							Name:     proto.String("http-main"),
							Protocol: proto.String("TCP"),
						},
					},
			}})
			if err != nil {
				//log.Println("Svc create", err)
			}
		}
	}

	return nil
}

func ensureNamespaces(k8sc *k8s.Client, desired int, prefix string) error {
	t0 := time.Now()
	var nsList corev1.NamespaceList
	err := k8sc.List(context.Background(), "", &nsList)
	if err != nil {
		return err
	}
	loadNs := 0
	for _, ns := range nsList.Items {
		if strings.HasPrefix(*ns.Metadata.Name, prefix) {
			loadNs++
		}
	}

	err = k8sc.Create(context.Background(), &corev1.Node{
		Metadata: &metav1.ObjectMeta{
			Name:      proto.String("node1"),
		},
	})
	if err != nil {
		log.Println("Node", err)
	}

	log.Println("Namespaces: ", len(nsList.Items), desired, loadNs, time.Since(t0))
	t0 = time.Now()

	if loadNs < desired {
		for i := loadNs; i < desired; i++ {
			n := prefix + strconv.Itoa(i)
			err = k8sc.Create(context.Background(), &corev1.Namespace{
				Metadata: &metav1.ObjectMeta{
					Name: &n,
					Labels: map[string]string{
						"istio-injection": "enabled",
					},
				},
			})
			if err != nil {
				return err
			}

			if *localApiserver {

				k8sc.Create(context.Background(), &corev1.Secret{
					Metadata: &metav1.ObjectMeta{
						Namespace: &n,
						Name:      proto.String("default-token"),
						Annotations: map[string]string{
							"kubernetes.io/service-account.name": "default",
							"kubernetes.io/service-account.uid":  "1",
						},
					},
					Type: proto.String("kubernetes.io/service-account-token"),
					Data: map[string][]byte{
						"token": []byte("1"),
					},
				})

				k8sc.Create(context.Background(), &corev1.ServiceAccount{
					Metadata: &metav1.ObjectMeta{
						Namespace: &n,
						Name:      proto.String("default"),
						Annotations: map[string]string{
							"kubernetes.io/enforce-mountable-secrets": "false",
						},
					},
					Secrets: []*corev1.ObjectReference{
						&corev1.ObjectReference{
							Name: proto.String("default-token"),
							Uid:  proto.String("1"),
						},
					},
				})

				pod := &corev1.Pod{
					Metadata: &metav1.ObjectMeta{
						Namespace: &n,
						Name:      proto.String("default"),
					},
					Spec: &corev1.PodSpec {
						ServiceAccountName: proto.String("default"),
						NodeName: proto.String("node1"),
						AutomountServiceAccountToken: proto.Bool(false),
						Containers: []*corev1.Container{
							{
								Name: proto.String("test"),
								Image: proto.String("ubuntu"),
							},
						},
					},
					Status: &corev1.PodStatus{
						PodIP: proto.String("10.0.1.1"),
						HostIP: proto.String("10.11.1.1"),
					},
				}
				k8sc.Create(context.Background(), pod)
				err = k8sc.Get(context.Background(), n, "default", pod)
				pod.Status.PodIP = proto.String("10.0.1.1")
				pod.Status.Phase = proto.String("Running")
				err = k8sc.Update(context.Background(), pod)
				if err != nil {
					log.Println("Pod update ", err)
				}
			}
		}
		// local: 1000ns in 4 s, 2000 also in 4 s
		log.Println("Create time: ", time.Since(t0))
	} else if loadNs > desired {
		// Delete doesn't work with local k8s
		for i := desired; i < loadNs ; i++ {
			n := prefix + strconv.Itoa(i)
			err = k8sc.Delete(context.Background(), &corev1.Namespace{
				Metadata: &metav1.ObjectMeta{
					Name: &n,
				},
			}, k8s.DeletePropagationForeground(), k8s.DeleteGracePeriod(1 * time.Second))
			if err != nil {
				//return err
			}
		}
		log.Println("Delete time: ", time.Since(t0))
	}
	return nil
}
