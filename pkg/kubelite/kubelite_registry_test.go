package kubelite

import (
	"context"
	"encoding/json"
	"log"
	"os"
	"testing"
)

func TestK8S(t *testing.T) {
	kl, err := NewClient(os.Getenv("HOME") + "/.istio/k8s.weekly10.yaml")
	if err != nil {
		log.Fatal(err)
	}
	k8sr := NewK8SRegistry(kl)
	k8sr.Sync() // will load the core SE-associated entries



	res, err := k8sr.client.DoRaw(context.Background(), "GET", "/apis/networking.istio.io/v1alpha3/serviceentries", contentTypeJSON, contentTypeJSON, nil)

	var x map[string]interface{}
	err = json.Unmarshal(res, &x)

	// apiVersion: networking.istio.io/v1alpha3
	// items[]
	// kind: ServiceEntryList
	// metadata: continue, resourceVersion, selfLink

	//log.Println(string(res), err )
	log.Println(x, err )

}


// Call path:
// - k8s.Client (client.go) do() - GET,
//   URL - constructed from ResourceList
//
//  resourceType{} holds apiGroup, apiVersion, name, namespaced. Keyed by reflect.Type of the ResourceList
// So it needs an actual type .. struct{}
//
// Client adds authorization header (or Basic)
//
// Unmarshal - pb starts with 'magic bytes', folowed by Unknown: TypeMeta, Raw, ContentEncoding, ContentType
// Standard json package used otherwise.

// Note that it can request json but still unmarshal into a proto, if the proto has an explicit method (.json.Unmarshaler)

const (
	contentTypePB   = "application/vnd.kubernetes.protobuf"
	contentTypeJSON = "application/json"
)

// DoRaw:
// url:
// - apis/APIGROUP/APIVERSION/RESOURCE/NAME
// - api/

// List core: /api/v1/nodes,pods,services,endpoints
// List CR: /apis/networking.istio.io/v1alpha3/serviceentries

// options (query param):
// limit=50
// continue=  needs to be extracted from response metadata
